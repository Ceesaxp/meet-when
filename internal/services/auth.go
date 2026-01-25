package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrEmailExists        = errors.New("email already registered")
	ErrTenantExists       = errors.New("tenant slug already exists")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrWeakPassword       = errors.New("password must be at least 8 characters")
)

// AuthService handles authentication operations
type AuthService struct {
	cfg        *config.Config
	repos      *repository.Repositories
	session    *SessionService
	auditLog   *AuditLogService
}

// NewAuthService creates a new auth service
func NewAuthService(cfg *config.Config, repos *repository.Repositories, session *SessionService, auditLog *AuditLogService) *AuthService {
	return &AuthService{
		cfg:      cfg,
		repos:    repos,
		session:  session,
		auditLog: auditLog,
	}
}

// RegisterInput represents the registration request
type RegisterInput struct {
	TenantName string
	TenantSlug string
	Name       string
	Email      string
	Password   string
	Timezone   string
}

// RegisterResult represents the registration result
type RegisterResult struct {
	Tenant       *models.Tenant
	Host         *models.Host
	SessionToken string
}

// Register creates a new tenant and host
func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*RegisterResult, error) {
	// Validate email
	if !isValidEmail(input.Email) {
		return nil, ErrInvalidEmail
	}

	// Validate password
	if len(input.Password) < 8 {
		return nil, ErrWeakPassword
	}

	// Normalize slug
	slug := slugify(input.TenantSlug)
	if slug == "" {
		slug = slugify(input.TenantName)
	}

	// Check if tenant exists
	existing, err := s.repos.Tenant.GetBySlug(ctx, slug)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrTenantExists
	}

	// Create tenant
	now := models.Now()
	tenant := &models.Tenant{
		ID:        uuid.New().String(),
		Slug:      slug,
		Name:      input.TenantName,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := s.repos.Tenant.Create(ctx, tenant); err != nil {
		return nil, err
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Default timezone
	timezone := input.Timezone
	if timezone == "" {
		timezone = s.cfg.App.DefaultTimezone
	}

	// Create host
	hostSlug := slugify(input.Name)
	if hostSlug == "" {
		hostSlug = strings.Split(input.Email, "@")[0]
		hostSlug = slugify(hostSlug)
	}

	host := &models.Host{
		ID:           uuid.New().String(),
		TenantID:     tenant.ID,
		Email:        strings.ToLower(input.Email),
		PasswordHash: string(hashedPassword),
		Name:         input.Name,
		Slug:         hostSlug,
		Timezone:     timezone,
		IsAdmin:      true, // First user is admin
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.Host.Create(ctx, host); err != nil {
		return nil, err
	}

	// Create default working hours (Mon-Fri 9:00-17:00)
	defaultHours := createDefaultWorkingHours(host.ID)
	if err := s.repos.WorkingHours.SetForHost(ctx, host.ID, defaultHours); err != nil {
		return nil, err
	}

	// Create session
	sessionToken, err := s.session.CreateSession(ctx, host.ID)
	if err != nil {
		return nil, err
	}

	// Audit log
	s.auditLog.Log(ctx, tenant.ID, &host.ID, "host.registered", "host", host.ID, nil, "")

	return &RegisterResult{
		Tenant:       tenant,
		Host:         host,
		SessionToken: sessionToken,
	}, nil
}

// LoginInput represents the login request
type LoginInput struct {
	TenantSlug string
	Email      string
	Password   string
}

// Login authenticates a user
func (s *AuthService) Login(ctx context.Context, input LoginInput) (*HostWithTenant, string, error) {
	// Get tenant
	tenant, err := s.repos.Tenant.GetBySlug(ctx, input.TenantSlug)
	if err != nil {
		return nil, "", err
	}
	if tenant == nil {
		return nil, "", ErrInvalidCredentials
	}

	// Get host
	host, err := s.repos.Host.GetByEmail(ctx, tenant.ID, strings.ToLower(input.Email))
	if err != nil {
		return nil, "", err
	}
	if host == nil {
		return nil, "", ErrInvalidCredentials
	}

	// Check password
	if err := bcrypt.CompareHashAndPassword([]byte(host.PasswordHash), []byte(input.Password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	// Create session
	sessionToken, err := s.session.CreateSession(ctx, host.ID)
	if err != nil {
		return nil, "", err
	}

	// Audit log
	s.auditLog.Log(ctx, tenant.ID, &host.ID, "host.login", "host", host.ID, nil, "")

	return &HostWithTenant{
		Host:   host,
		Tenant: tenant,
	}, sessionToken, nil
}

// Helper functions

func isValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimSpace(s)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func createDefaultWorkingHours(hostID string) []*models.WorkingHours {
	now := models.Now()
	var hours []*models.WorkingHours

	// Monday (1) through Friday (5)
	for day := 1; day <= 5; day++ {
		hours = append(hours, &models.WorkingHours{
			ID:        uuid.New().String(),
			HostID:    hostID,
			DayOfWeek: day,
			StartTime: "09:00",
			EndTime:   "17:00",
			IsEnabled: true,
			CreatedAt: now,
			UpdatedAt: now,
		})
	}

	return hours
}

// GetTenantBySlug retrieves a tenant by slug
func (s *AuthService) GetTenantBySlug(ctx context.Context, slug string) (*models.Tenant, error) {
	return s.repos.Tenant.GetBySlug(ctx, slug)
}

// GetHostBySlug retrieves a host by tenant ID and slug
func (s *AuthService) GetHostBySlug(ctx context.Context, tenantID, slug string) (*models.Host, error) {
	return s.repos.Host.GetBySlug(ctx, tenantID, slug)
}

// GetHostByID retrieves a host by ID
func (s *AuthService) GetHostByID(ctx context.Context, id string) (*models.Host, error) {
	return s.repos.Host.GetByID(ctx, id)
}

// UpdateHost updates a host's profile
func (s *AuthService) UpdateHost(ctx context.Context, host *models.Host) error {
	return s.repos.Host.Update(ctx, host)
}

package services

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials     = errors.New("invalid email or password")
	ErrEmailExists            = errors.New("email already registered")
	ErrTenantExists           = errors.New("tenant slug already exists")
	ErrInvalidEmail           = errors.New("invalid email format")
	ErrWeakPassword           = errors.New("password must be at least 8 characters")
	ErrInvalidSelectionToken  = errors.New("invalid or expired selection token")
	ErrHostNotFound           = errors.New("host not found")
)

// SelectionTokenExpiry is the duration for which selection tokens are valid
const SelectionTokenExpiry = 5 * time.Minute

// AuthService handles authentication operations
type AuthService struct {
	cfg      *config.Config
	repos    *repository.Repositories
	session  *SessionService
	auditLog *AuditLogService
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

// SimplifiedLoginInput represents the simplified login request (email + password only)
type SimplifiedLoginInput struct {
	Email    string
	Password string
}

// OrgOption represents an organization option for multi-org users
type OrgOption struct {
	TenantID       string
	TenantSlug     string
	TenantName     string
	HostID         string
	SelectionToken string // Token to use when selecting this org
}

// SimplifiedLoginResult represents the result of a simplified login attempt
type SimplifiedLoginResult struct {
	RequiresOrgSelection bool           // True if user has multiple orgs and must select one
	AvailableOrgs        []OrgOption    // Populated when RequiresOrgSelection is true
	SelectionToken       string         // Token for completing org selection (when RequiresOrgSelection is true)
	SessionToken         string         // Populated when single org match (direct login)
	Host                 *models.Host   // Populated when single org match
	Tenant               *models.Tenant // Populated when single org match
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

// SimplifiedLogin authenticates a user with just email and password (no org required).
// If the email exists in multiple organizations, it returns RequiresOrgSelection=true
// with the list of available orgs. If single org, it creates a session directly.
func (s *AuthService) SimplifiedLogin(ctx context.Context, input SimplifiedLoginInput) (*SimplifiedLoginResult, error) {
	email := strings.ToLower(input.Email)

	// Get all hosts with this email across all tenants
	hosts, err := s.repos.Host.GetAllByEmail(ctx, email)
	if err != nil {
		return nil, err
	}

	// Timing attack prevention: always perform a bcrypt comparison even if no hosts found
	// This ensures consistent response time regardless of whether the email exists
	dummyHash := "$2a$10$dummy.hash.for.timing.attack.prevention.placeholder"
	if len(hosts) == 0 {
		bcrypt.CompareHashAndPassword([]byte(dummyHash), []byte(input.Password))
		return nil, ErrInvalidCredentials
	}

	// Check password against each host and collect valid matches
	var validHosts []*models.Host
	for _, host := range hosts {
		if err := bcrypt.CompareHashAndPassword([]byte(host.PasswordHash), []byte(input.Password)); err == nil {
			validHosts = append(validHosts, host)
		}
	}

	// No valid matches (wrong password for all accounts)
	if len(validHosts) == 0 {
		return nil, ErrInvalidCredentials
	}

	// Single valid match: create session and return directly
	if len(validHosts) == 1 {
		host := validHosts[0]

		// Get tenant for this host
		tenant, err := s.repos.Tenant.GetByID(ctx, host.TenantID)
		if err != nil {
			return nil, err
		}
		if tenant == nil {
			return nil, ErrInvalidCredentials
		}

		// Create session
		sessionToken, err := s.session.CreateSession(ctx, host.ID)
		if err != nil {
			return nil, err
		}

		// Audit log
		s.auditLog.Log(ctx, tenant.ID, &host.ID, "host.login", "host", host.ID, nil, "")

		return &SimplifiedLoginResult{
			RequiresOrgSelection: false,
			SessionToken:         sessionToken,
			Host:                 host,
			Tenant:               tenant,
		}, nil
	}

	// Multiple valid matches: return org selection required
	var availableOrgs []OrgOption
	for _, host := range validHosts {
		tenant, err := s.repos.Tenant.GetByID(ctx, host.TenantID)
		if err != nil {
			return nil, err
		}
		if tenant == nil {
			continue // Skip hosts with missing tenants
		}

		// Generate a selection token for this org option
		selectionToken, err := s.generateSelectionToken(host.ID)
		if err != nil {
			return nil, err
		}

		availableOrgs = append(availableOrgs, OrgOption{
			TenantID:       tenant.ID,
			TenantSlug:     tenant.Slug,
			TenantName:     tenant.Name,
			HostID:         host.ID,
			SelectionToken: selectionToken,
		})
	}

	return &SimplifiedLoginResult{
		RequiresOrgSelection: true,
		AvailableOrgs:        availableOrgs,
	}, nil
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

// CompleteOnboarding marks a host's onboarding as complete
func (s *AuthService) CompleteOnboarding(ctx context.Context, hostID string) error {
	return s.repos.Host.UpdateOnboardingCompleted(ctx, hostID, true)
}

// generateSelectionToken creates a short-lived cryptographically signed token for org selection.
// The token format is: base64(hostID:expiry:signature)
// This approach doesn't require storing tokens in a database - the signature validates authenticity.
func (s *AuthService) generateSelectionToken(hostID string) (string, error) {
	expiry := time.Now().Add(SelectionTokenExpiry).Unix()
	payload := fmt.Sprintf("%s:%d", hostID, expiry)

	// Create HMAC signature using the encryption key
	mac := hmac.New(sha256.New, []byte(s.cfg.App.EncryptionKey))
	mac.Write([]byte(payload))
	signature := hex.EncodeToString(mac.Sum(nil))

	// Combine payload and signature
	token := fmt.Sprintf("%s:%s", payload, signature)
	return base64.URLEncoding.EncodeToString([]byte(token)), nil
}

// validateSelectionToken validates a selection token and returns the host ID if valid.
// Returns an error if the token is invalid, expired, or tampered with.
func (s *AuthService) validateSelectionToken(token string) (string, error) {
	// Decode the base64 token
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return "", ErrInvalidSelectionToken
	}

	// Parse the token parts: hostID:expiry:signature
	parts := strings.Split(string(decoded), ":")
	if len(parts) != 3 {
		return "", ErrInvalidSelectionToken
	}

	hostID := parts[0]
	expiryStr := parts[1]
	providedSignature := parts[2]

	// Validate expiry
	expiry, err := strconv.ParseInt(expiryStr, 10, 64)
	if err != nil {
		return "", ErrInvalidSelectionToken
	}
	if time.Now().Unix() > expiry {
		return "", ErrInvalidSelectionToken
	}

	// Verify signature
	payload := fmt.Sprintf("%s:%s", hostID, expiryStr)
	mac := hmac.New(sha256.New, []byte(s.cfg.App.EncryptionKey))
	mac.Write([]byte(payload))
	expectedSignature := hex.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(providedSignature), []byte(expectedSignature)) {
		return "", ErrInvalidSelectionToken
	}

	return hostID, nil
}

// CompleteOrgSelectionInput represents the input for completing org selection
type CompleteOrgSelectionInput struct {
	HostID         string
	SelectionToken string
}

// CompleteOrgSelection completes the login flow after a user selects their organization.
// It validates the selection token to ensure the request is legitimate (not enumeration attack).
func (s *AuthService) CompleteOrgSelection(ctx context.Context, input CompleteOrgSelectionInput) (*HostWithTenant, string, error) {
	// Validate the selection token
	tokenHostID, err := s.validateSelectionToken(input.SelectionToken)
	if err != nil {
		return nil, "", err
	}

	// The host ID in the token must match the requested host ID
	// This prevents using a valid token for one host to access a different host
	if tokenHostID != input.HostID {
		return nil, "", ErrInvalidSelectionToken
	}

	// Get the host
	host, err := s.repos.Host.GetByID(ctx, input.HostID)
	if err != nil {
		return nil, "", err
	}
	if host == nil {
		return nil, "", ErrHostNotFound
	}

	// Get the tenant
	tenant, err := s.repos.Tenant.GetByID(ctx, host.TenantID)
	if err != nil {
		return nil, "", err
	}
	if tenant == nil {
		return nil, "", ErrInvalidCredentials
	}

	// Create session
	sessionToken, err := s.session.CreateSession(ctx, host.ID)
	if err != nil {
		return nil, "", err
	}

	// Audit log
	s.auditLog.Log(ctx, tenant.ID, &host.ID, "host.login", "host", host.ID, nil, "via org selection")

	return &HostWithTenant{
		Host:   host,
		Tenant: tenant,
	}, sessionToken, nil
}

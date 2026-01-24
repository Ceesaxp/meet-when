package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// HostWithTenant represents a host with their tenant information
type HostWithTenant struct {
	Host   *models.Host
	Tenant *models.Tenant
}

// SessionService handles session operations
type SessionService struct {
	cfg   *config.Config
	repos *repository.Repositories
}

// NewSessionService creates a new session service
func NewSessionService(cfg *config.Config, repos *repository.Repositories) *SessionService {
	return &SessionService{
		cfg:   cfg,
		repos: repos,
	}
}

// CreateSession creates a new session for a host
func (s *SessionService) CreateSession(ctx context.Context, hostID string) (string, error) {
	token, err := generateToken(32)
	if err != nil {
		return "", err
	}

	session := &models.Session{
		ID:        uuid.New().String(),
		HostID:    hostID,
		Token:     token,
		ExpiresAt: time.Now().Add(s.cfg.App.SessionDuration),
		CreatedAt: time.Now(),
	}

	if err := s.repos.Session.Create(ctx, session); err != nil {
		return "", err
	}

	return token, nil
}

// ValidateSession validates a session token and returns the host
func (s *SessionService) ValidateSession(ctx context.Context, token string) (*HostWithTenant, error) {
	session, err := s.repos.Session.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, ErrInvalidCredentials
	}

	host, err := s.repos.Host.GetByID(ctx, session.HostID)
	if err != nil {
		return nil, err
	}
	if host == nil {
		return nil, ErrInvalidCredentials
	}

	tenant, err := s.repos.Tenant.GetByID(ctx, host.TenantID)
	if err != nil {
		return nil, err
	}

	return &HostWithTenant{
		Host:   host,
		Tenant: tenant,
	}, nil
}

// DeleteSession removes a session
func (s *SessionService) DeleteSession(ctx context.Context, token string) error {
	return s.repos.Session.Delete(ctx, token)
}

// CleanupExpiredSessions removes expired sessions
func (s *SessionService) CleanupExpiredSessions(ctx context.Context) error {
	return s.repos.Session.DeleteExpired(ctx)
}

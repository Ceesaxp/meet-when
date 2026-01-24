package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// AuditLogService handles audit logging
type AuditLogService struct {
	repos *repository.Repositories
}

// NewAuditLogService creates a new audit log service
func NewAuditLogService(repos *repository.Repositories) *AuditLogService {
	return &AuditLogService{repos: repos}
}

// Log creates an audit log entry
func (s *AuditLogService) Log(ctx context.Context, tenantID string, hostID *string, action, entityType, entityID string, details models.JSONMap, ipAddress string) {
	log := &models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		HostID:     hostID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    details,
		IPAddress:  ipAddress,
		CreatedAt:  time.Now(),
	}

	// Fire and forget - don't block on audit log failures
	go func() {
		_ = s.repos.AuditLog.Create(context.Background(), log)
	}()
}

// GetLogs retrieves audit logs for a tenant
func (s *AuditLogService) GetLogs(ctx context.Context, tenantID string, limit, offset int) ([]*models.AuditLog, error) {
	return s.repos.AuditLog.GetByTenantID(ctx, tenantID, limit, offset)
}

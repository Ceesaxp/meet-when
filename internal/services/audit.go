package services

import (
	"context"
	"log"

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
	entry := &models.AuditLog{
		ID:         uuid.New().String(),
		TenantID:   tenantID,
		HostID:     hostID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Details:    details,
		IPAddress:  ipAddress,
		CreatedAt:  models.Now(),
	}

	// Fire and forget - don't block on audit log failures, but log errors
	go func() {
		if err := s.repos.AuditLog.Create(context.Background(), entry); err != nil {
			log.Printf("Failed to create audit log: %v (action=%s, entity=%s/%s)", err, action, entityType, entityID)
		}
	}()
}

// GetLogs retrieves audit logs for a tenant
func (s *AuditLogService) GetLogs(ctx context.Context, tenantID string, limit, offset int) ([]*models.AuditLog, error) {
	return s.repos.AuditLog.GetByTenantID(ctx, tenantID, limit, offset)
}

// GetLogsCount returns the total count of audit logs for a tenant
func (s *AuditLogService) GetLogsCount(ctx context.Context, tenantID string) (int, error) {
	return s.repos.AuditLog.CountByTenantID(ctx, tenantID)
}

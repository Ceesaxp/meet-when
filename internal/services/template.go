package services

import (
	"context"
	"errors"
	"strconv"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

var (
	ErrTemplateNotFound     = errors.New("meeting template not found")
	ErrTemplateSlugExists   = errors.New("template slug already exists")
	ErrPooledHostNotFound   = errors.New("pooled host not found")
	ErrPooledHostExists     = errors.New("host is already pooled on this template")
	ErrCannotRemoveOwner    = errors.New("cannot remove the template owner")
	ErrPooledHostLimit      = errors.New("maximum 5 hosts per template")
	ErrHostNotInTenant      = errors.New("host must be in the same tenant")
	MaxPooledHostsPerTemplate = 5
)

// TemplateService handles meeting template operations
type TemplateService struct {
	repos    *repository.Repositories
	auditLog *AuditLogService
}

// NewTemplateService creates a new template service
func NewTemplateService(repos *repository.Repositories, auditLog *AuditLogService) *TemplateService {
	return &TemplateService{
		repos:    repos,
		auditLog: auditLog,
	}
}

// CreateTemplateInput represents the input for creating a template
type CreateTemplateInput struct {
	HostID            string
	TenantID          string
	Slug              string
	Name              string
	Description       string
	Durations         []int
	LocationType      models.ConferencingProvider
	CustomLocation    string
	CalendarID        string
	RequiresApproval  bool
	MinNoticeMinutes  int
	MaxScheduleDays   int
	PreBufferMinutes  int
	PostBufferMinutes int
	AvailabilityRules models.JSONMap
	InviteeQuestions  models.JSONArray
	ConfirmationEmail string
	ReminderEmail     string
	IsPrivate         bool
}

// CreateTemplate creates a new meeting template
func (s *TemplateService) CreateTemplate(ctx context.Context, input CreateTemplateInput) (*models.MeetingTemplate, error) {
	// Check if slug exists
	existing, err := s.repos.Template.GetByHostAndSlug(ctx, input.HostID, input.Slug)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrTemplateSlugExists
	}

	// Set defaults
	if len(input.Durations) == 0 {
		input.Durations = []int{30}
	}
	if input.MinNoticeMinutes == 0 {
		input.MinNoticeMinutes = 60 // 1 hour default
	}
	if input.MaxScheduleDays == 0 {
		input.MaxScheduleDays = 14 // 2 weeks default
	}

	now := models.Now()
	template := &models.MeetingTemplate{
		ID:                uuid.New().String(),
		HostID:            input.HostID,
		Slug:              input.Slug,
		Name:              input.Name,
		Description:       input.Description,
		Durations:         input.Durations,
		LocationType:      input.LocationType,
		CustomLocation:    input.CustomLocation,
		CalendarID:        input.CalendarID,
		RequiresApproval:  input.RequiresApproval,
		MinNoticeMinutes:  input.MinNoticeMinutes,
		MaxScheduleDays:   input.MaxScheduleDays,
		PreBufferMinutes:  input.PreBufferMinutes,
		PostBufferMinutes: input.PostBufferMinutes,
		AvailabilityRules: input.AvailabilityRules,
		InviteeQuestions:  input.InviteeQuestions,
		ConfirmationEmail: input.ConfirmationEmail,
		ReminderEmail:     input.ReminderEmail,
		IsActive:          true,
		IsPrivate:         input.IsPrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repos.Template.Create(ctx, template); err != nil {
		return nil, err
	}

	// Create template_hosts entry for the owner
	if err := s.EnsureOwnerInPool(ctx, template); err != nil {
		// Log but don't fail - the template was created successfully
		// The migration should have handled existing templates
	}

	// Audit log
	s.auditLog.Log(ctx, input.TenantID, &input.HostID, "template.created", "template", template.ID, nil, "")

	return template, nil
}

// UpdateTemplateInput represents the input for updating a template
type UpdateTemplateInput struct {
	ID                string
	HostID            string
	TenantID          string
	Slug              string
	Name              string
	Description       string
	Durations         []int
	LocationType      models.ConferencingProvider
	CustomLocation    string
	CalendarID        string
	RequiresApproval  bool
	MinNoticeMinutes  int
	MaxScheduleDays   int
	PreBufferMinutes  int
	PostBufferMinutes int
	AvailabilityRules models.JSONMap
	InviteeQuestions  models.JSONArray
	ConfirmationEmail string
	ReminderEmail     string
	IsActive          bool
	IsPrivate         bool
}

// UpdateTemplate updates an existing template
func (s *TemplateService) UpdateTemplate(ctx context.Context, input UpdateTemplateInput) (*models.MeetingTemplate, error) {
	template, err := s.repos.Template.GetByID(ctx, input.ID)
	if err != nil {
		return nil, err
	}
	if template == nil || template.HostID != input.HostID {
		return nil, ErrTemplateNotFound
	}

	// Check slug uniqueness if changed
	if input.Slug != template.Slug {
		existing, err := s.repos.Template.GetByHostAndSlug(ctx, input.HostID, input.Slug)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return nil, ErrTemplateSlugExists
		}
	}

	template.Slug = input.Slug
	template.Name = input.Name
	template.Description = input.Description
	template.Durations = input.Durations
	template.LocationType = input.LocationType
	template.CustomLocation = input.CustomLocation
	template.CalendarID = input.CalendarID
	template.RequiresApproval = input.RequiresApproval
	template.MinNoticeMinutes = input.MinNoticeMinutes
	template.MaxScheduleDays = input.MaxScheduleDays
	template.PreBufferMinutes = input.PreBufferMinutes
	template.PostBufferMinutes = input.PostBufferMinutes
	template.AvailabilityRules = input.AvailabilityRules
	template.InviteeQuestions = input.InviteeQuestions
	template.ConfirmationEmail = input.ConfirmationEmail
	template.ReminderEmail = input.ReminderEmail
	template.IsActive = input.IsActive
	template.IsPrivate = input.IsPrivate

	if err := s.repos.Template.Update(ctx, template); err != nil {
		return nil, err
	}

	// Audit log
	s.auditLog.Log(ctx, input.TenantID, &input.HostID, "template.updated", "template", template.ID, nil, "")

	return template, nil
}

// GetTemplate retrieves a template by ID
func (s *TemplateService) GetTemplate(ctx context.Context, hostID, templateID string) (*models.MeetingTemplate, error) {
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if template == nil || template.HostID != hostID {
		return nil, ErrTemplateNotFound
	}
	return template, nil
}

// GetTemplateBySlug retrieves a template by host ID and slug
func (s *TemplateService) GetTemplateBySlug(ctx context.Context, hostID, slug string) (*models.MeetingTemplate, error) {
	return s.repos.Template.GetByHostAndSlug(ctx, hostID, slug)
}

// GetTemplates retrieves all templates for a host
func (s *TemplateService) GetTemplates(ctx context.Context, hostID string) ([]*models.MeetingTemplate, error) {
	return s.repos.Template.GetByHostID(ctx, hostID)
}

// DeleteTemplate deletes a template
func (s *TemplateService) DeleteTemplate(ctx context.Context, hostID, tenantID, templateID string) error {
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil {
		return err
	}
	if template == nil || template.HostID != hostID {
		return ErrTemplateNotFound
	}

	if err := s.repos.Template.Delete(ctx, templateID); err != nil {
		return err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "template.deleted", "template", templateID, nil, "")

	return nil
}

// DuplicateTemplate creates a copy of an existing template
func (s *TemplateService) DuplicateTemplate(ctx context.Context, hostID, tenantID, templateID string) (*models.MeetingTemplate, error) {
	// Get the original template
	original, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if original == nil || original.HostID != hostID {
		return nil, ErrTemplateNotFound
	}

	// Generate unique slug
	baseSlug := original.Slug + "-copy"
	slug := baseSlug
	counter := 1

	// Keep trying until we find a unique slug
	for {
		existing, err := s.repos.Template.GetByHostAndSlug(ctx, hostID, slug)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			break
		}
		counter++
		slug = baseSlug + "-" + strconv.Itoa(counter)
	}

	now := models.Now()
	duplicate := &models.MeetingTemplate{
		ID:                uuid.New().String(),
		HostID:            original.HostID,
		Slug:              slug,
		Name:              original.Name + " (Copy)",
		Description:       original.Description,
		Durations:         original.Durations,
		LocationType:      original.LocationType,
		CustomLocation:    original.CustomLocation,
		CalendarID:        original.CalendarID,
		RequiresApproval:  original.RequiresApproval,
		MinNoticeMinutes:  original.MinNoticeMinutes,
		MaxScheduleDays:   original.MaxScheduleDays,
		PreBufferMinutes:  original.PreBufferMinutes,
		PostBufferMinutes: original.PostBufferMinutes,
		AvailabilityRules: original.AvailabilityRules,
		InviteeQuestions:  original.InviteeQuestions,
		ConfirmationEmail: original.ConfirmationEmail,
		ReminderEmail:     original.ReminderEmail,
		IsActive:          false, // New copies are inactive by default
		IsPrivate:         original.IsPrivate,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := s.repos.Template.Create(ctx, duplicate); err != nil {
		return nil, err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &hostID, "template.duplicated", "template", duplicate.ID, map[string]interface{}{
		"original_id": original.ID,
	}, "")

	return duplicate, nil
}

// GetTemplateWithHosts retrieves a template with its pooled hosts
func (s *TemplateService) GetTemplateWithHosts(ctx context.Context, templateID string) (*models.MeetingTemplate, error) {
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil || template == nil {
		return nil, ErrTemplateNotFound
	}

	// Load pooled hosts
	pooledHosts, err := s.repos.TemplateHost.GetByTemplateIDWithHost(ctx, templateID)
	if err != nil {
		return nil, err
	}
	template.PooledHosts = pooledHosts

	return template, nil
}

// AddPooledHost adds a host to a template's pool
func (s *TemplateService) AddPooledHost(ctx context.Context, tenantID, templateID, hostID string, isOptional bool) (*models.TemplateHost, error) {
	// Get the template to verify ownership
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil || template == nil {
		return nil, ErrTemplateNotFound
	}

	// Verify the host being added is in the same tenant
	host, err := s.repos.Host.GetByID(ctx, hostID)
	if err != nil || host == nil {
		return nil, errors.New("host not found")
	}
	if host.TenantID != tenantID {
		return nil, ErrHostNotInTenant
	}

	// Check if host is already pooled
	existing, err := s.repos.TemplateHost.GetByTemplateAndHost(ctx, templateID, hostID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, ErrPooledHostExists
	}

	// Check pool limit
	count, err := s.repos.TemplateHost.CountByTemplateID(ctx, templateID)
	if err != nil {
		return nil, err
	}
	if count >= MaxPooledHostsPerTemplate {
		return nil, ErrPooledHostLimit
	}

	// Determine role (owner or sibling)
	role := models.TemplateHostRoleSibling
	if hostID == template.HostID {
		role = models.TemplateHostRoleOwner
	}

	now := models.Now()
	templateHost := &models.TemplateHost{
		ID:           uuid.New().String(),
		TemplateID:   templateID,
		HostID:       hostID,
		Role:         role,
		IsOptional:   isOptional,
		DisplayOrder: count, // Add at the end
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := s.repos.TemplateHost.Create(ctx, templateHost); err != nil {
		return nil, err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &template.HostID, "template.host_added", "template", templateID, map[string]interface{}{
		"added_host_id": hostID,
		"is_optional":   isOptional,
	}, "")

	return templateHost, nil
}

// RemovePooledHost removes a host from a template's pool
func (s *TemplateService) RemovePooledHost(ctx context.Context, tenantID, templateID, hostID string) error {
	// Get the template to verify ownership
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil || template == nil {
		return ErrTemplateNotFound
	}

	// Cannot remove the template owner
	if hostID == template.HostID {
		return ErrCannotRemoveOwner
	}

	// Get the template host record
	templateHost, err := s.repos.TemplateHost.GetByTemplateAndHost(ctx, templateID, hostID)
	if err != nil {
		return err
	}
	if templateHost == nil {
		return ErrPooledHostNotFound
	}

	if err := s.repos.TemplateHost.Delete(ctx, templateHost.ID); err != nil {
		return err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &template.HostID, "template.host_removed", "template", templateID, map[string]interface{}{
		"removed_host_id": hostID,
	}, "")

	return nil
}

// UpdatePooledHost updates a pooled host's optional status
func (s *TemplateService) UpdatePooledHost(ctx context.Context, tenantID, templateID, hostID string, isOptional bool) error {
	// Get the template to verify it exists
	template, err := s.repos.Template.GetByID(ctx, templateID)
	if err != nil || template == nil {
		return ErrTemplateNotFound
	}

	// Get the template host record
	templateHost, err := s.repos.TemplateHost.GetByTemplateAndHost(ctx, templateID, hostID)
	if err != nil {
		return err
	}
	if templateHost == nil {
		return ErrPooledHostNotFound
	}

	templateHost.IsOptional = isOptional
	templateHost.UpdatedAt = models.Now()

	if err := s.repos.TemplateHost.Update(ctx, templateHost); err != nil {
		return err
	}

	// Audit log
	s.auditLog.Log(ctx, tenantID, &template.HostID, "template.host_updated", "template", templateID, map[string]interface{}{
		"host_id":     hostID,
		"is_optional": isOptional,
	}, "")

	return nil
}

// GetPooledHosts returns all pooled hosts for a template
func (s *TemplateService) GetPooledHosts(ctx context.Context, templateID string) ([]*models.TemplateHost, error) {
	return s.repos.TemplateHost.GetByTemplateIDWithHost(ctx, templateID)
}

// EnsureOwnerInPool ensures the template owner has a record in template_hosts
// This is called when creating a template to maintain the pooled hosts invariant
func (s *TemplateService) EnsureOwnerInPool(ctx context.Context, template *models.MeetingTemplate) error {
	// Check if owner already exists in pool
	existing, err := s.repos.TemplateHost.GetByTemplateAndHost(ctx, template.ID, template.HostID)
	if err != nil {
		return err
	}
	if existing != nil {
		return nil // Owner already in pool
	}

	// Add owner to pool
	now := models.Now()
	templateHost := &models.TemplateHost{
		ID:           uuid.New().String(),
		TemplateID:   template.ID,
		HostID:       template.HostID,
		Role:         models.TemplateHostRoleOwner,
		IsOptional:   false,
		DisplayOrder: 0,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	return s.repos.TemplateHost.Create(ctx, templateHost)
}

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
	ErrTemplateNotFound   = errors.New("meeting template not found")
	ErrTemplateSlugExists = errors.New("template slug already exists")
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

package services

import (
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/repository"
)

// Services holds all service instances
type Services struct {
	Auth         *AuthService
	Session      *SessionService
	Calendar     *CalendarService
	Conferencing *ConferencingService
	Template     *TemplateService
	Booking      *BookingService
	Availability *AvailabilityService
	Email        *EmailService
	AuditLog     *AuditLogService
}

// New creates all services
func New(cfg *config.Config, repos *repository.Repositories) *Services {
	emailSvc := NewEmailService(cfg)
	calendarSvc := NewCalendarService(cfg, repos)
	conferencingSvc := NewConferencingService(cfg, repos)
	availabilitySvc := NewAvailabilityService(repos, calendarSvc)
	auditLogSvc := NewAuditLogService(repos)

	bookingSvc := NewBookingService(cfg, repos, calendarSvc, conferencingSvc, emailSvc, auditLogSvc)
	templateSvc := NewTemplateService(repos, auditLogSvc)
	sessionSvc := NewSessionService(cfg, repos)
	authSvc := NewAuthService(cfg, repos, sessionSvc, auditLogSvc)

	return &Services{
		Auth:         authSvc,
		Session:      sessionSvc,
		Calendar:     calendarSvc,
		Conferencing: conferencingSvc,
		Template:     templateSvc,
		Booking:      bookingSvc,
		Availability: availabilitySvc,
		Email:        emailSvc,
		AuditLog:     auditLogSvc,
	}
}

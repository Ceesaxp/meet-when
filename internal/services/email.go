package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/smtp"
	"strings"
	"time"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
)

// EmailTemplate represents a custom email template with subject and body
type EmailTemplate struct {
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// EmailTemplateData contains the data available for email template placeholders
type EmailTemplateData struct {
	InviteeName    string
	HostName       string
	MeetingName    string
	MeetingTime    string
	Duration       int
	Location       string
	CancelLink     string
	RescheduleLink string
}

// EmailService handles email sending
type EmailService struct {
	cfg *config.Config
}

// NewEmailService creates a new email service
func NewEmailService(cfg *config.Config) *EmailService {
	return &EmailService{cfg: cfg}
}

// parseEmailTemplate parses a JSON email template string
func parseEmailTemplate(templateJSON string) *EmailTemplate {
	if templateJSON == "" {
		return nil
	}
	var tmpl EmailTemplate
	if err := json.Unmarshal([]byte(templateJSON), &tmpl); err != nil {
		log.Printf("Failed to parse email template: %v", err)
		return nil
	}
	// Only return if at least subject or body is set
	if tmpl.Subject == "" && tmpl.Body == "" {
		return nil
	}
	return &tmpl
}

// renderEmailTemplate replaces placeholders in the template with actual values
func renderEmailTemplate(templateStr string, data *EmailTemplateData) string {
	result := templateStr
	result = strings.ReplaceAll(result, "{{invitee_name}}", data.InviteeName)
	result = strings.ReplaceAll(result, "{{host_name}}", data.HostName)
	result = strings.ReplaceAll(result, "{{meeting_name}}", data.MeetingName)
	result = strings.ReplaceAll(result, "{{meeting_time}}", data.MeetingTime)
	result = strings.ReplaceAll(result, "{{duration}}", fmt.Sprintf("%d", data.Duration))
	result = strings.ReplaceAll(result, "{{location}}", data.Location)
	result = strings.ReplaceAll(result, "{{cancel_link}}", data.CancelLink)
	result = strings.ReplaceAll(result, "{{reschedule_link}}", data.RescheduleLink)
	return result
}

// buildEmailTemplateData creates template data from booking details for invitee emails
func (s *EmailService) buildEmailTemplateData(details *BookingWithDetails, timezone *time.Location) *EmailTemplateData {
	startTime := details.Booking.StartTime.In(timezone)

	location := "To be determined"
	if details.Template.LocationType == models.ConferencingProviderPhone {
		// Format phone location with "Call" prefix
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	return &EmailTemplateData{
		InviteeName:    details.Booking.InviteeName,
		HostName:       details.Host.Name,
		MeetingName:    details.Template.Name,
		MeetingTime:    startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		Duration:       details.Booking.Duration,
		Location:       location,
		CancelLink:     fmt.Sprintf("%s/booking/%s", s.cfg.Server.BaseURL, details.Booking.Token),
		RescheduleLink: fmt.Sprintf("%s/booking/%s/reschedule", s.cfg.Server.BaseURL, details.Booking.Token),
	}
}

// defaultInviteeConfirmationBody returns the default confirmation email body
func (s *EmailService) defaultInviteeConfirmationBody(data *EmailTemplateData) string {
	return fmt.Sprintf(`Hello %s,

Your meeting has been confirmed!

Meeting: %s
With: %s
When: %s
Duration: %d minutes
Location: %s

Need to make changes?
Cancel: %s
Reschedule: %s

Best regards,
Meet When`,
		data.InviteeName,
		data.MeetingName,
		data.HostName,
		data.MeetingTime,
		data.Duration,
		data.Location,
		data.CancelLink,
		data.RescheduleLink,
	)
}

// SendBookingRequested sends notification to host about new booking request
func (s *EmailService) SendBookingRequested(ctx context.Context, details *BookingWithDetails) {
	subject := fmt.Sprintf("New booking request from %s", details.Booking.InviteeName)

	// Format time in host's timezone
	hostLoc, _ := time.LoadLocation(details.Host.Timezone)
	startTime := details.Booking.StartTime.In(hostLoc)

	// Extract agenda if provided
	agendaSection := ""
	if details.Booking.Answers != nil {
		if agenda, ok := details.Booking.Answers["agenda"].(string); ok && agenda != "" {
			agendaSection = fmt.Sprintf("\nAgenda:\n%s\n", agenda)
		}
	}

	body := fmt.Sprintf(`Hello %s,

You have a new booking request:

Meeting: %s
From: %s (%s)
When: %s
Duration: %d minutes
%s
Please log in to approve or reject this request:
%s/dashboard/bookings

Best regards,
Meet When`,
		details.Host.Name,
		details.Template.Name,
		details.Booking.InviteeName,
		details.Booking.InviteeEmail,
		startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		details.Booking.Duration,
		agendaSection,
		s.cfg.Server.BaseURL,
	)

	go func() {
		if err := s.sendEmail(details.Host.Email, subject, body, ""); err != nil {
			log.Printf("Error sending email to host %s: %v", details.Host.Email, err)
		}
	}()
}

// SendBookingConfirmed sends confirmation to both host and invitee
func (s *EmailService) SendBookingConfirmed(ctx context.Context, details *BookingWithDetails) {
	// Send to invitee
	s.sendInviteeConfirmation(ctx, details)

	// Send to host
	s.sendHostConfirmation(ctx, details)
}

func (s *EmailService) sendInviteeConfirmation(ctx context.Context, details *BookingWithDetails) {
	// Format time in invitee's timezone
	inviteeLoc, _ := time.LoadLocation(details.Booking.InviteeTimezone)
	if inviteeLoc == nil {
		inviteeLoc = time.UTC
	}

	// Build template data for custom templates
	templateData := s.buildEmailTemplateData(details, inviteeLoc)

	var subject, body string

	// Check for custom confirmation email template
	customTemplate := parseEmailTemplate(details.Template.ConfirmationEmail)
	if customTemplate != nil {
		// Use custom template
		if customTemplate.Subject != "" {
			subject = renderEmailTemplate(customTemplate.Subject, templateData)
		} else {
			subject = fmt.Sprintf("Confirmed: %s with %s", details.Template.Name, details.Host.Name)
		}
		if customTemplate.Body != "" {
			body = renderEmailTemplate(customTemplate.Body, templateData)
		} else {
			body = s.defaultInviteeConfirmationBody(templateData)
		}
	} else {
		// Use default template
		subject = fmt.Sprintf("Confirmed: %s with %s", details.Template.Name, details.Host.Name)
		body = s.defaultInviteeConfirmationBody(templateData)
	}

	// Generate ICS attachment
	ics := s.generateICS(details)

	go func() {
		if err := s.sendEmail(details.Booking.InviteeEmail, subject, body, ics); err != nil {
			log.Printf("Error sending email to invitee %s: %v", details.Booking.InviteeEmail, err)
		}
	}()
}

func (s *EmailService) sendHostConfirmation(ctx context.Context, details *BookingWithDetails) {
	subject := fmt.Sprintf("Meeting confirmed: %s with %s", details.Template.Name, details.Booking.InviteeName)

	hostLoc, _ := time.LoadLocation(details.Host.Timezone)
	startTime := details.Booking.StartTime.In(hostLoc)

	location := "To be determined"
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	// Extract agenda if provided
	agendaSection := ""
	if details.Booking.Answers != nil {
		if agenda, ok := details.Booking.Answers["agenda"].(string); ok && agenda != "" {
			agendaSection = fmt.Sprintf("\nAgenda:\n%s\n", agenda)
		}
	}

	body := fmt.Sprintf(`Hello %s,

A meeting has been confirmed:

Meeting: %s
With: %s (%s)
When: %s
Duration: %d minutes
Location: %s
%s
View all bookings: %s/dashboard/bookings

Best regards,
Meet When`,
		details.Host.Name,
		details.Template.Name,
		details.Booking.InviteeName,
		details.Booking.InviteeEmail,
		startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		details.Booking.Duration,
		location,
		agendaSection,
		s.cfg.Server.BaseURL,
	)

	go func() {
		if err := s.sendEmail(details.Host.Email, subject, body, ""); err != nil {
			log.Printf("Error sending email to host %s: %v", details.Host.Email, err)
		}
	}()
}

// SendBookingCancelled sends cancellation notice
func (s *EmailService) SendBookingCancelled(ctx context.Context, details *BookingWithDetails) {
	// Determine who cancelled and who to notify
	if details.Booking.CancelledBy == "invitee" {
		s.sendCancellationToHost(ctx, details)
	} else {
		s.sendCancellationToInvitee(ctx, details)
	}
}

func (s *EmailService) sendCancellationToHost(ctx context.Context, details *BookingWithDetails) {
	subject := fmt.Sprintf("Meeting cancelled: %s", details.Template.Name)

	hostLoc, _ := time.LoadLocation(details.Host.Timezone)
	startTime := details.Booking.StartTime.In(hostLoc)

	body := fmt.Sprintf(`Hello %s,

A meeting has been cancelled:

Meeting: %s
With: %s (%s)
Was scheduled for: %s

%s

Best regards,
Meet When`,
		details.Host.Name,
		details.Template.Name,
		details.Booking.InviteeName,
		details.Booking.InviteeEmail,
		startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		formatCancelReason(details.Booking.CancelReason),
	)

	go func() {
		if err := s.sendEmail(details.Host.Email, subject, body, ""); err != nil {
			log.Printf("Error sending email to host %s: %v", details.Host.Email, err)
		}
	}()
}

func (s *EmailService) sendCancellationToInvitee(ctx context.Context, details *BookingWithDetails) {
	subject := fmt.Sprintf("Meeting cancelled: %s", details.Template.Name)

	inviteeLoc, _ := time.LoadLocation(details.Booking.InviteeTimezone)
	if inviteeLoc == nil {
		inviteeLoc = time.UTC
	}
	startTime := details.Booking.StartTime.In(inviteeLoc)

	body := fmt.Sprintf(`Hello %s,

Your meeting has been cancelled:

Meeting: %s
With: %s
Was scheduled for: %s

%s

You can book a new time at:
%s/%s/%s

Best regards,
Meet When`,
		details.Booking.InviteeName,
		details.Template.Name,
		details.Host.Name,
		startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		formatCancelReason(details.Booking.CancelReason),
		s.cfg.Server.BaseURL,
		details.Tenant.Slug,
		details.Host.Slug,
	)

	go func() {
		if err := s.sendEmail(details.Booking.InviteeEmail, subject, body, ""); err != nil {
			log.Printf("Error sending email to invitee %s: %v", details.Booking.InviteeEmail, err)
		}
	}()
}

// SendBookingRejected sends rejection notice to invitee
func (s *EmailService) SendBookingRejected(ctx context.Context, details *BookingWithDetails) {
	subject := fmt.Sprintf("Booking request declined: %s", details.Template.Name)

	inviteeLoc, _ := time.LoadLocation(details.Booking.InviteeTimezone)
	if inviteeLoc == nil {
		inviteeLoc = time.UTC
	}
	startTime := details.Booking.StartTime.In(inviteeLoc)

	body := fmt.Sprintf(`Hello %s,

Unfortunately, your booking request was not approved:

Meeting: %s
With: %s
Requested time: %s

%s

You can try booking a different time at:
%s/%s/%s

Best regards,
Meet When`,
		details.Booking.InviteeName,
		details.Template.Name,
		details.Host.Name,
		startTime.Format("Monday, January 2, 2006 at 3:04 PM MST"),
		formatCancelReason(details.Booking.CancelReason),
		s.cfg.Server.BaseURL,
		details.Tenant.Slug,
		details.Host.Slug,
	)

	go func() {
		if err := s.sendEmail(details.Booking.InviteeEmail, subject, body, ""); err != nil {
			log.Printf("Error sending email to invitee %s: %v", details.Booking.InviteeEmail, err)
		}
	}()
}

// SendBookingRescheduled sends reschedule notification to both host and invitee
func (s *EmailService) SendBookingRescheduled(ctx context.Context, details *BookingWithDetails, oldStartTime time.Time) {
	// Send to invitee
	s.sendInviteeRescheduleNotification(ctx, details, oldStartTime)

	// Send to host
	s.sendHostRescheduleNotification(ctx, details, oldStartTime)
}

func (s *EmailService) sendInviteeRescheduleNotification(ctx context.Context, details *BookingWithDetails, oldStartTime time.Time) {
	subject := fmt.Sprintf("Rescheduled: %s with %s", details.Template.Name, details.Host.Name)

	// Format times in invitee's timezone
	inviteeLoc, _ := time.LoadLocation(details.Booking.InviteeTimezone)
	if inviteeLoc == nil {
		inviteeLoc = time.UTC
	}
	oldTimeFormatted := oldStartTime.In(inviteeLoc).Format("Monday, January 2, 2006 at 3:04 PM MST")
	newTimeFormatted := details.Booking.StartTime.In(inviteeLoc).Format("Monday, January 2, 2006 at 3:04 PM MST")

	location := "To be determined"
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	body := fmt.Sprintf(`Hello %s,

Your meeting has been rescheduled.

Meeting: %s
With: %s

Previous time: %s
New time: %s
Duration: %d minutes
Location: %s

Need to make changes?
Cancel: %s/booking/%s
Reschedule: %s/booking/%s/reschedule

Best regards,
Meet When`,
		details.Booking.InviteeName,
		details.Template.Name,
		details.Host.Name,
		oldTimeFormatted,
		newTimeFormatted,
		details.Booking.Duration,
		location,
		s.cfg.Server.BaseURL,
		details.Booking.Token,
		s.cfg.Server.BaseURL,
		details.Booking.Token,
	)

	// Generate ICS attachment with updated time
	ics := s.generateICS(details)

	go func() {
		if err := s.sendEmail(details.Booking.InviteeEmail, subject, body, ics); err != nil {
			log.Printf("Error sending reschedule email to invitee %s: %v", details.Booking.InviteeEmail, err)
		}
	}()
}

// defaultReminderBody returns the default reminder email body
func (s *EmailService) defaultReminderBody(data *EmailTemplateData) string {
	return fmt.Sprintf(`Hello %s,

This is a reminder about your upcoming meeting:

Meeting: %s
With: %s
When: %s
Duration: %d minutes
Location: %s

Need to make changes?
Cancel: %s
Reschedule: %s

Best regards,
Meet When`,
		data.InviteeName,
		data.MeetingName,
		data.HostName,
		data.MeetingTime,
		data.Duration,
		data.Location,
		data.CancelLink,
		data.RescheduleLink,
	)
}

// SendBookingReminder sends a reminder email to the invitee before their meeting
func (s *EmailService) SendBookingReminder(ctx context.Context, details *BookingWithDetails) {
	// Format time in invitee's timezone
	inviteeLoc, _ := time.LoadLocation(details.Booking.InviteeTimezone)
	if inviteeLoc == nil {
		inviteeLoc = time.UTC
	}

	// Build template data for custom templates
	templateData := s.buildEmailTemplateData(details, inviteeLoc)

	var subject, body string

	// Check for custom reminder email template
	customTemplate := parseEmailTemplate(details.Template.ReminderEmail)
	if customTemplate != nil {
		// Use custom template
		if customTemplate.Subject != "" {
			subject = renderEmailTemplate(customTemplate.Subject, templateData)
		} else {
			subject = fmt.Sprintf("Reminder: %s with %s tomorrow", details.Template.Name, details.Host.Name)
		}
		if customTemplate.Body != "" {
			body = renderEmailTemplate(customTemplate.Body, templateData)
		} else {
			body = s.defaultReminderBody(templateData)
		}
	} else {
		// Use default template
		subject = fmt.Sprintf("Reminder: %s with %s tomorrow", details.Template.Name, details.Host.Name)
		body = s.defaultReminderBody(templateData)
	}

	// Generate ICS attachment
	ics := s.generateICS(details)

	go func() {
		if err := s.sendEmail(details.Booking.InviteeEmail, subject, body, ics); err != nil {
			log.Printf("Error sending reminder email to invitee %s: %v", details.Booking.InviteeEmail, err)
		}
	}()
}

func (s *EmailService) sendHostRescheduleNotification(ctx context.Context, details *BookingWithDetails, oldStartTime time.Time) {
	subject := fmt.Sprintf("Meeting rescheduled: %s with %s", details.Template.Name, details.Booking.InviteeName)

	// Format times in host's timezone
	hostLoc, _ := time.LoadLocation(details.Host.Timezone)
	if hostLoc == nil {
		hostLoc = time.UTC
	}
	oldTimeFormatted := oldStartTime.In(hostLoc).Format("Monday, January 2, 2006 at 3:04 PM MST")
	newTimeFormatted := details.Booking.StartTime.In(hostLoc).Format("Monday, January 2, 2006 at 3:04 PM MST")

	location := "To be determined"
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	body := fmt.Sprintf(`Hello %s,

A meeting has been rescheduled.

Meeting: %s
With: %s (%s)

Previous time: %s
New time: %s
Duration: %d minutes
Location: %s

View all bookings: %s/dashboard/bookings

Best regards,
Meet When`,
		details.Host.Name,
		details.Template.Name,
		details.Booking.InviteeName,
		details.Booking.InviteeEmail,
		oldTimeFormatted,
		newTimeFormatted,
		details.Booking.Duration,
		location,
		s.cfg.Server.BaseURL,
	)

	go func() {
		if err := s.sendEmail(details.Host.Email, subject, body, ""); err != nil {
			log.Printf("Error sending reschedule email to host %s: %v", details.Host.Email, err)
		}
	}()
}

// generateICS creates an ICS calendar attachment
func (s *EmailService) generateICS(details *BookingWithDetails) string {
	location := ""
	if details.Template.LocationType == models.ConferencingProviderPhone {
		if details.Template.CustomLocation != "" {
			location = "Call " + details.Template.CustomLocation
		}
	} else if details.Booking.ConferenceLink != "" {
		location = details.Booking.ConferenceLink
	} else if details.Template.CustomLocation != "" {
		location = details.Template.CustomLocation
	}

	// Build description with template description and agenda if provided
	description := details.Template.Description
	if details.Booking.Answers != nil {
		if agenda, ok := details.Booking.Answers["agenda"].(string); ok && agenda != "" {
			if description != "" {
				description += "\n\n"
			}
			description += "Agenda:\n" + agenda
		}
	}

	ics := fmt.Sprintf(`BEGIN:VCALENDAR
VERSION:2.0
PRODID:-//MeetWhen//EN
METHOD:REQUEST
BEGIN:VEVENT
UID:%s@meetwhen
DTSTART:%s
DTEND:%s
SUMMARY:%s with %s
DESCRIPTION:%s
LOCATION:%s
ORGANIZER;CN=%s:mailto:%s
ATTENDEE;CN=%s:mailto:%s
STATUS:CONFIRMED
END:VEVENT
END:VCALENDAR`,
		details.Booking.ID,
		details.Booking.StartTime.UTC().Format("20060102T150405Z"),
		details.Booking.EndTime.UTC().Format("20060102T150405Z"),
		details.Template.Name,
		details.Host.Name,
		escapeICS(description),
		escapeICS(location),
		details.Host.Name,
		details.Host.Email,
		details.Booking.InviteeName,
		details.Booking.InviteeEmail,
	)

	return ics
}

// GenerateICS creates an ICS calendar file content for a booking (public wrapper)
func (s *EmailService) GenerateICS(details *BookingWithDetails) string {
	return s.generateICS(details)
}

// sendEmail sends an email (supports both SMTP and Mailgun)
func (s *EmailService) sendEmail(to, subject, body, icsAttachment string) error {
	if s.cfg.Email.Provider == "mailgun" {
		return s.sendMailgun(to, subject, body, icsAttachment)
	}
	return s.sendSMTP(to, subject, body, icsAttachment)
}

func (s *EmailService) sendSMTP(to, subject, body, icsAttachment string) error {
	from := s.cfg.Email.FromAddress
	host := s.cfg.Email.SMTPHost
	port := s.cfg.Email.SMTPPort

	var msg bytes.Buffer

	// Headers
	msg.WriteString(fmt.Sprintf("From: %s <%s>\r\n", s.cfg.Email.FromName, from))
	msg.WriteString(fmt.Sprintf("To: %s\r\n", to))
	msg.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))

	if icsAttachment != "" {
		// Multipart message with attachment
		boundary := "----=_Part_0_123456789"
		msg.WriteString("MIME-Version: 1.0\r\n")
		msg.WriteString(fmt.Sprintf("Content-Type: multipart/mixed; boundary=\"%s\"\r\n", boundary))
		msg.WriteString("\r\n")

		// Text part
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(body)
		msg.WriteString("\r\n")

		// ICS attachment
		msg.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		msg.WriteString("Content-Type: text/calendar; charset=utf-8; method=REQUEST\r\n")
		msg.WriteString("Content-Disposition: attachment; filename=\"invite.ics\"\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(icsAttachment)
		msg.WriteString("\r\n")

		msg.WriteString(fmt.Sprintf("--%s--\r\n", boundary))
	} else {
		msg.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
		msg.WriteString("\r\n")
		msg.WriteString(body)
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	var auth smtp.Auth
	if s.cfg.Email.SMTPUser != "" {
		auth = smtp.PlainAuth("", s.cfg.Email.SMTPUser, s.cfg.Email.SMTPPassword, host)
	}

	return smtp.SendMail(addr, auth, from, []string{to}, msg.Bytes())
}

func (s *EmailService) sendMailgun(to, subject, body, icsAttachment string) error {
	// Mailgun API implementation
	// For MVP, we'll use the SMTP relay which is simpler
	return s.sendSMTP(to, subject, body, icsAttachment)
}

func formatCancelReason(reason string) string {
	if reason == "" {
		return ""
	}
	return fmt.Sprintf("Reason: %s", reason)
}

func escapeICS(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// HTMLEmail represents an HTML email template
type HTMLEmail struct {
	Subject string
	Body    template.HTML
}

// RenderHTMLEmail renders an HTML email template
func (s *EmailService) RenderHTMLEmail(templateName string, data interface{}) (*HTMLEmail, error) {
	// For MVP, we use plain text emails
	// HTML email templates can be added later
	return nil, nil
}

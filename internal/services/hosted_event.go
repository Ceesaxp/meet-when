package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// AttendeeInput is the host-supplied attendee shape used by Create / Update
// inputs. ContactID is non-nil when the attendee was picked from the contact
// list, nil for free-form entries.
type AttendeeInput struct {
	Email     string
	Name      string
	ContactID *string
}

// CreateHostedEventInput is the host-supplied data needed to schedule a new
// hosted event. Times are passed as time.Time and converted to SQLiteTime
// internally.
type CreateHostedEventInput struct {
	HostID         string
	TenantID       string
	TemplateID     *string
	Title          string
	Description    string
	Start          time.Time
	Duration       int // minutes
	Timezone       string
	LocationType   models.ConferencingProvider
	CustomLocation string
	CalendarID     string
	Attendees      []AttendeeInput
}

// UpdateHostedEventInput uses pointer fields for "change if non-nil"
// semantics so the caller can express partial updates without ambiguity
// between "no change" and "set to zero value".
type UpdateHostedEventInput struct {
	HostID                   string
	TenantID                 string
	EventID                  string
	Title                    *string
	Description              *string
	CustomLocation           *string
	Start                    *time.Time
	Duration                 *int
	Timezone                 *string
	LocationType             *models.ConferencingProvider
	CalendarID               *string
	Attendees                *[]AttendeeInput
	RegenerateConferenceLink bool
}

// HostedEventWithDetails bundles a hosted event with the joined entities
// callers usually need (host, tenant, optional template, attendees).
type HostedEventWithDetails struct {
	Event     *models.HostedEvent
	Host      *models.Host
	Tenant    *models.Tenant
	Template  *models.MeetingTemplate
	Attendees []*models.HostedEventAttendee
}

// hostedEventEmailSender is the narrow surface HostedEventService needs from
// the email layer. *EmailService satisfies it; tests inject a spy.
type hostedEventEmailSender interface {
	SendHostedEventInvited(ctx context.Context, event *models.HostedEvent, attendee *models.HostedEventAttendee, host *models.Host, tenant *models.Tenant)
	SendHostedEventUpdated(ctx context.Context, event *models.HostedEvent, attendee *models.HostedEventAttendee, host *models.Host, tenant *models.Tenant, changedFields []string)
	SendHostedEventCancelled(ctx context.Context, event *models.HostedEvent, attendee *models.HostedEventAttendee, host *models.Host, tenant *models.Tenant)
	SendHostedEventCancelledForAttendee(ctx context.Context, event *models.HostedEvent, attendee *models.HostedEventAttendee, host *models.Host, tenant *models.Tenant)
	SendHostedEventReminder(ctx context.Context, event *models.HostedEvent, attendee *models.HostedEventAttendee, host *models.Host, tenant *models.Tenant)
}

// HostedEventService orchestrates host-driven event scheduling. The shared
// per-host calendar fan-out lives in CalendarEventSyncer (PR #43); this
// service owns the entity-specific orchestration (validation, conferencing,
// attendee diff, contact upsert, audit, email).
type HostedEventService struct {
	cfg          *config.Config
	repos        *repository.Repositories
	calendar     *CalendarService
	conferencing *ConferencingService
	syncer       *CalendarEventSyncer
	email        hostedEventEmailSender
	contact      *ContactService
	audit        *AuditLogService
}

// NewHostedEventService constructs a HostedEventService.
func NewHostedEventService(
	cfg *config.Config,
	repos *repository.Repositories,
	calendar *CalendarService,
	conferencing *ConferencingService,
	syncer *CalendarEventSyncer,
	email hostedEventEmailSender,
	contact *ContactService,
	audit *AuditLogService,
) *HostedEventService {
	return &HostedEventService{
		cfg:          cfg,
		repos:        repos,
		calendar:     calendar,
		conferencing: conferencing,
		syncer:       syncer,
		email:        email,
		contact:      contact,
		audit:        audit,
	}
}

// ---------------------------------------------------------------------------
// Read paths
// ---------------------------------------------------------------------------

// Get returns a hosted event with its joined details, scoped to the caller.
// Returns (nil, nil) when not found; (nil, error) when the lookup itself
// fails. Returns an error when the host/tenant scope mismatches.
func (s *HostedEventService) Get(ctx context.Context, hostID, tenantID, eventID string) (*HostedEventWithDetails, error) {
	event, err := s.repos.HostedEvent.GetByID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, nil
	}
	if event.HostID != hostID || event.TenantID != tenantID {
		return nil, errors.New("hosted event not found")
	}
	return s.loadDetails(ctx, event)
}

// List returns the host's hosted events ordered by start_time descending.
func (s *HostedEventService) List(ctx context.Context, hostID string, includeArchived bool) ([]*models.HostedEvent, error) {
	return s.repos.HostedEvent.ListByHost(ctx, hostID, includeArchived)
}

func (s *HostedEventService) loadDetails(ctx context.Context, event *models.HostedEvent) (*HostedEventWithDetails, error) {
	host, err := s.repos.Host.GetByID(ctx, event.HostID)
	if err != nil {
		return nil, fmt.Errorf("load host: %w", err)
	}
	tenant, err := s.repos.Tenant.GetByID(ctx, event.TenantID)
	if err != nil {
		return nil, fmt.Errorf("load tenant: %w", err)
	}
	var template *models.MeetingTemplate
	if event.TemplateID != nil && *event.TemplateID != "" {
		template, _ = s.repos.Template.GetByID(ctx, *event.TemplateID) // best-effort
	}
	attendees, err := s.repos.HostedEventAttendee.ListByEvent(ctx, event.ID)
	if err != nil {
		return nil, fmt.Errorf("load attendees: %w", err)
	}
	return &HostedEventWithDetails{
		Event:     event,
		Host:      host,
		Tenant:    tenant,
		Template:  template,
		Attendees: attendees,
	}, nil
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// Create schedules a new hosted event:
//  1. validates input
//  2. resolves the calendar to write to
//  3. persists the event + attendees in a tx
//  4. creates a conferencing link if needed (best-effort; reauth-required
//     surfaces but doesn't abort)
//  5. fans out the calendar event via the syncer; the calendar layer may
//     surface a conference link of its own (Google Meet) which is then
//     persisted with a second write
//  6. notifies attendees via email
//  7. upserts contact rows for each attendee
//  8. writes an audit entry
func (s *HostedEventService) Create(ctx context.Context, input CreateHostedEventInput) (*HostedEventWithDetails, error) {
	if err := s.validateCreate(input); err != nil {
		return nil, err
	}

	host, err := s.repos.Host.GetByID(ctx, input.HostID)
	if err != nil {
		return nil, fmt.Errorf("load host: %w", err)
	}
	if host == nil || host.TenantID != input.TenantID {
		return nil, errors.New("host not found")
	}
	tenant, err := s.repos.Tenant.GetByID(ctx, input.TenantID)
	if err != nil || tenant == nil {
		return nil, fmt.Errorf("load tenant: %w", err)
	}

	calendarID, err := s.resolveCalendarID(ctx, host, input.TemplateID, input.CalendarID)
	if err != nil {
		return nil, err
	}

	now := models.Now()
	end := input.Start.Add(time.Duration(input.Duration) * time.Minute)
	tz := input.Timezone
	if tz == "" {
		tz = host.Timezone
	}

	event := &models.HostedEvent{
		ID:             uuid.New().String(),
		TenantID:       input.TenantID,
		HostID:         input.HostID,
		TemplateID:     input.TemplateID,
		Title:          input.Title,
		Description:    input.Description,
		StartTime:      models.NewSQLiteTime(input.Start),
		EndTime:        models.NewSQLiteTime(end),
		Duration:       input.Duration,
		Timezone:       tz,
		LocationType:   input.LocationType,
		CustomLocation: input.CustomLocation,
		CalendarID:     calendarID,
		Status:         models.HostedEventStatusScheduled,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.repos.HostedEvent.Create(ctx, event); err != nil {
		return nil, fmt.Errorf("persist event: %w", err)
	}

	attendees := s.buildAttendeeRows(event.ID, input.Attendees, now)
	if err := s.repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, attendees); err != nil {
		return nil, fmt.Errorf("persist attendees: %w", err)
	}

	// Conferencing — best-effort. ErrConferencingReauthRequired is logged
	// but does not abort the event creation: the calendar event still goes
	// out and the host can fix the conferencing link via retry.
	if event.LocationType == models.ConferencingProviderZoom ||
		event.LocationType == models.ConferencingProviderGoogleMeet ||
		event.LocationType == models.ConferencingProviderPhone ||
		event.LocationType == models.ConferencingProviderCustom {
		link, err := s.conferencing.CreateMeetingForHostedEvent(ctx, host.ID, event.Title, event.StartTime.Time, event.Duration, event.LocationType, event.CustomLocation)
		if err != nil {
			if errors.Is(err, ErrConferencingReauthRequired) {
				log.Printf("[HOSTED_EVENT] Conferencing reauth required for host %s on event %s; event will be created without a link", host.ID, event.ID)
			} else {
				log.Printf("[HOSTED_EVENT] Conferencing error for event %s: %v", event.ID, err)
			}
		} else if link != "" {
			event.ConferenceLink = link
		}
	}

	// Persist the conferencing-link write before fan-out so the calendar
	// event carries the same link.
	if event.ConferenceLink != "" {
		event.UpdatedAt = models.Now()
		if err := s.repos.HostedEvent.Update(ctx, event); err != nil {
			log.Printf("[HOSTED_EVENT] Error persisting conference link on event %s: %v", event.ID, err)
		}
	}

	// Calendar fan-out via the shared syncer. v1 hosted events have no
	// pooled hosts — just the owner.
	syncInput := s.buildCalendarEventInput(event, attendees, host, "" /* eventID empty for create */)
	firstID, syncerLink, err := s.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindHostedEvent,
		ItemID: event.ID,
		Input:  syncInput,
		Hosts:  []HostTarget{{HostID: host.ID, CalendarID: calendarID, IsOwner: true}},
	})
	if err != nil {
		log.Printf("[HOSTED_EVENT] Syncer.Create error for event %s: %v", event.ID, err)
	}
	// Google Meet returns the link from the calendar create response; persist
	// it (and the first event ID) — second write.
	persistChanged := false
	if syncerLink != "" && event.ConferenceLink == "" {
		event.ConferenceLink = syncerLink
		persistChanged = true
	}
	if persistChanged {
		event.UpdatedAt = models.Now()
		if err := s.repos.HostedEvent.Update(ctx, event); err != nil {
			log.Printf("[HOSTED_EVENT] Error persisting post-sync link on event %s: %v", event.ID, err)
		}
	}
	_ = firstID // tracked in hosted_event_calendar_events; not needed on the event row for v1

	// Email each attendee.
	for _, a := range attendees {
		s.email.SendHostedEventInvited(ctx, event, a, host, tenant)
	}

	// Contact upsert for every attendee at create time (each attendee is
	// "new" in the sense that this event is the first occurrence with them).
	for _, a := range attendees {
		s.contact.UpsertFromHostedEventAttendee(ctx, event.TenantID, a.Email, a.Name, event.StartTime)
	}

	hostID := host.ID
	s.audit.Log(ctx, event.TenantID, &hostID, "hosted_event.created", "hosted_event", event.ID, models.JSONMap{
		"title":     event.Title,
		"start":     event.StartTime.UTC().Format(time.RFC3339),
		"duration":  event.Duration,
		"attendees": attendeeEmails(attendees),
	}, "")

	return &HostedEventWithDetails{
		Event:     event,
		Host:      host,
		Tenant:    tenant,
		Attendees: attendees,
	}, nil
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// Update applies a partial change to an existing event. Pointer fields on
// UpdateHostedEventInput follow "change if non-nil" semantics. Returns the
// updated details and the list of changed field names (for the audit entry
// and email content).
//
// Behavioural notes:
//   - When Start changes, reminder_sent is reset so the reminder loop fires
//     against the new time.
//   - Conferencing is regenerated only if RegenerateConferenceLink is set or
//     LocationType changes — same shape as the booking flow.
//   - Attendee diff is computed on lowercased email; added attendees get an
//     "invited" email, removed get "cancelled-for-attendee", retained get
//     an "updated" email iff a material field changed.
//   - Contact upsert fires only for newly added attendees, so meeting_count
//     and last_met don't get re-incremented on a benign edit.
func (s *HostedEventService) Update(ctx context.Context, input UpdateHostedEventInput) (*HostedEventWithDetails, []string, error) {
	if input.HostID == "" || input.TenantID == "" || input.EventID == "" {
		return nil, nil, errors.New("host, tenant, and event ID are required")
	}

	details, err := s.Get(ctx, input.HostID, input.TenantID, input.EventID)
	if err != nil {
		return nil, nil, err
	}
	if details == nil {
		return nil, nil, errors.New("hosted event not found")
	}
	if details.Event.Status == models.HostedEventStatusCancelled {
		return nil, nil, errors.New("cannot update a cancelled event")
	}

	event := details.Event
	previousStart := event.StartTime
	prevAttendees := details.Attendees

	// ---- Field diff ----
	changed := []string{}

	if input.Title != nil && *input.Title != event.Title {
		if strings.TrimSpace(*input.Title) == "" {
			return nil, nil, errors.New("title cannot be empty")
		}
		event.Title = *input.Title
		changed = append(changed, "title")
	}
	if input.Description != nil && *input.Description != event.Description {
		event.Description = *input.Description
		changed = append(changed, "description")
	}
	if input.CustomLocation != nil && *input.CustomLocation != event.CustomLocation {
		event.CustomLocation = *input.CustomLocation
		changed = append(changed, "custom_location")
	}
	if input.LocationType != nil && *input.LocationType != event.LocationType {
		event.LocationType = *input.LocationType
		changed = append(changed, "location_type")
	}
	if input.Timezone != nil && *input.Timezone != event.Timezone {
		event.Timezone = *input.Timezone
		changed = append(changed, "timezone")
	}
	if input.CalendarID != nil && *input.CalendarID != event.CalendarID {
		event.CalendarID = *input.CalendarID
		changed = append(changed, "calendar_id")
	}

	// Start / Duration changes ripple into EndTime.
	startChanged := false
	durationChanged := false
	if input.Duration != nil && *input.Duration > 0 && *input.Duration != event.Duration {
		event.Duration = *input.Duration
		changed = append(changed, "duration")
		durationChanged = true
	}
	if input.Start != nil && !input.Start.Equal(event.StartTime.Time) {
		event.StartTime = models.NewSQLiteTime(*input.Start)
		changed = append(changed, "start")
		startChanged = true
	}
	if startChanged || durationChanged {
		event.EndTime = models.NewSQLiteTime(event.StartTime.Time.Add(time.Duration(event.Duration) * time.Minute))
	}

	// ---- Attendee diff ----
	addedAttendeeRows := []*models.HostedEventAttendee{}
	removedAttendees := []*models.HostedEventAttendee{}
	retainedAttendees := []*models.HostedEventAttendee{}
	finalAttendees := prevAttendees

	if input.Attendees != nil {
		now := models.Now()
		incoming := s.buildAttendeeRows(event.ID, *input.Attendees, now)
		if len(incoming) == 0 {
			return nil, nil, errors.New("at least one attendee is required")
		}

		prevByEmail := indexAttendeesByEmail(prevAttendees)
		incomingByEmail := indexAttendeesByEmail(incoming)

		for email, row := range incomingByEmail {
			if existing, ok := prevByEmail[email]; ok {
				// Retain — but inherit the prior row's ID and CreatedAt so the
				// attendee history is preserved when ReplaceForEvent re-inserts.
				row.ID = existing.ID
				row.CreatedAt = existing.CreatedAt
				retainedAttendees = append(retainedAttendees, row)
			} else {
				addedAttendeeRows = append(addedAttendeeRows, row)
			}
		}
		for email, row := range prevByEmail {
			if _, ok := incomingByEmail[email]; !ok {
				removedAttendees = append(removedAttendees, row)
			}
		}

		// Sort for deterministic order downstream (emails, audit detail).
		sort.Slice(retainedAttendees, func(i, j int) bool { return retainedAttendees[i].Email < retainedAttendees[j].Email })
		sort.Slice(addedAttendeeRows, func(i, j int) bool { return addedAttendeeRows[i].Email < addedAttendeeRows[j].Email })
		sort.Slice(removedAttendees, func(i, j int) bool { return removedAttendees[i].Email < removedAttendees[j].Email })

		finalAttendees = make([]*models.HostedEventAttendee, 0, len(retainedAttendees)+len(addedAttendeeRows))
		finalAttendees = append(finalAttendees, retainedAttendees...)
		finalAttendees = append(finalAttendees, addedAttendeeRows...)

		if len(addedAttendeeRows) > 0 || len(removedAttendees) > 0 {
			changed = append(changed, "attendees")
		}
	}

	// Nothing changed — fast exit.
	if len(changed) == 0 && !input.RegenerateConferenceLink {
		return details, nil, nil
	}

	// ---- Conferencing regeneration ----
	conferencingChanged := false
	if input.RegenerateConferenceLink || sliceContains(changed, "location_type") {
		link, err := s.conferencing.CreateMeetingForHostedEvent(ctx, details.Host.ID, event.Title, event.StartTime.Time, event.Duration, event.LocationType, event.CustomLocation)
		if err != nil {
			if errors.Is(err, ErrConferencingReauthRequired) {
				return nil, nil, ErrConferencingReauthRequired
			}
			log.Printf("[HOSTED_EVENT] Conferencing error during update of event %s: %v", event.ID, err)
		} else if link != event.ConferenceLink {
			event.ConferenceLink = link
			conferencingChanged = true
		}
	}
	if conferencingChanged && !sliceContains(changed, "conference_link") {
		changed = append(changed, "conference_link")
	}

	// ---- reminder_sent reset on time change ----
	if startChanged && event.ReminderSent {
		event.ReminderSent = false
	}

	// ---- Persist event + attendees ----
	event.UpdatedAt = models.Now()
	if err := s.repos.HostedEvent.Update(ctx, event); err != nil {
		return nil, nil, fmt.Errorf("persist event update: %w", err)
	}
	if input.Attendees != nil {
		if err := s.repos.HostedEventAttendee.ReplaceForEvent(ctx, event.ID, finalAttendees); err != nil {
			return nil, nil, fmt.Errorf("persist attendee update: %w", err)
		}
	}

	// ---- Calendar fan-out (Update across all tracked rows) ----
	syncInput := s.buildCalendarEventInput(event, finalAttendees, details.Host, "" /* per-row EventID is filled by syncer */)
	if err := s.syncer.Update(ctx, CalendarSyncRequest{
		Kind:   ItemKindHostedEvent,
		ItemID: event.ID,
		Input:  syncInput,
		Hosts:  []HostTarget{{HostID: details.Host.ID, CalendarID: event.CalendarID, IsOwner: true}},
	}); err != nil {
		log.Printf("[HOSTED_EVENT] Syncer.Update error for event %s: %v", event.ID, err)
	}

	// ---- Email fan-out ----
	materialChanged := hasMaterialChange(changed)
	for _, a := range removedAttendees {
		s.email.SendHostedEventCancelledForAttendee(ctx, event, a, details.Host, details.Tenant)
	}
	for _, a := range addedAttendeeRows {
		s.email.SendHostedEventInvited(ctx, event, a, details.Host, details.Tenant)
	}
	if materialChanged {
		for _, a := range retainedAttendees {
			s.email.SendHostedEventUpdated(ctx, event, a, details.Host, details.Tenant, changed)
		}
	}

	// ---- Contact upsert (added attendees only) ----
	for _, a := range addedAttendeeRows {
		s.contact.UpsertFromHostedEventAttendee(ctx, event.TenantID, a.Email, a.Name, event.StartTime)
	}

	// ---- Audit ----
	hID := input.HostID
	auditDetails := models.JSONMap{
		"changed_fields": changed,
	}
	if startChanged {
		auditDetails["previous_start"] = previousStart.UTC().Format(time.RFC3339)
		auditDetails["new_start"] = event.StartTime.UTC().Format(time.RFC3339)
	}
	if len(addedAttendeeRows) > 0 {
		auditDetails["added_attendees"] = attendeeEmails(addedAttendeeRows)
	}
	if len(removedAttendees) > 0 {
		auditDetails["removed_attendees"] = attendeeEmails(removedAttendees)
	}
	s.audit.Log(ctx, event.TenantID, &hID, "hosted_event.updated", "hosted_event", event.ID, auditDetails, "")

	out := &HostedEventWithDetails{
		Event:     event,
		Host:      details.Host,
		Tenant:    details.Tenant,
		Template:  details.Template,
		Attendees: finalAttendees,
	}
	return out, changed, nil
}

// indexAttendeesByEmail returns a lowercase-email-keyed index of attendee
// rows. Inputs from buildAttendeeRows are already lowercased; this is mostly
// a defensive convenience for the existing-rows side.
func indexAttendeesByEmail(rows []*models.HostedEventAttendee) map[string]*models.HostedEventAttendee {
	out := make(map[string]*models.HostedEventAttendee, len(rows))
	for _, r := range rows {
		out[strings.ToLower(r.Email)] = r
	}
	return out
}

// hasMaterialChange returns true when any of the changed fields warrants
// emailing retained attendees. "Calendar ID" alone is internal plumbing —
// it doesn't change what the attendee sees.
func hasMaterialChange(changed []string) bool {
	for _, f := range changed {
		switch f {
		case "title", "description", "start", "duration", "location_type",
			"custom_location", "conference_link", "timezone", "attendees":
			return true
		}
	}
	return false
}

func sliceContains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// Cancel marks the event cancelled, removes it from every tracked host
// calendar, emails attendees, and writes an audit entry.
func (s *HostedEventService) Cancel(ctx context.Context, hostID, tenantID, eventID, reason string) error {
	details, err := s.Get(ctx, hostID, tenantID, eventID)
	if err != nil {
		return err
	}
	if details == nil {
		return errors.New("hosted event not found")
	}
	if details.Event.Status == models.HostedEventStatusCancelled {
		return nil
	}

	details.Event.Status = models.HostedEventStatusCancelled
	details.Event.CancelReason = reason
	details.Event.UpdatedAt = models.Now()
	if err := s.repos.HostedEvent.Update(ctx, details.Event); err != nil {
		return fmt.Errorf("persist cancel: %w", err)
	}

	if _, err := s.syncer.Delete(ctx, ItemKindHostedEvent, eventID); err != nil {
		log.Printf("[HOSTED_EVENT] Syncer.Delete error for event %s: %v", eventID, err)
	}

	for _, a := range details.Attendees {
		s.email.SendHostedEventCancelled(ctx, details.Event, a, details.Host, details.Tenant)
	}

	hID := hostID
	s.audit.Log(ctx, tenantID, &hID, "hosted_event.cancelled", "hosted_event", eventID, models.JSONMap{
		"reason": reason,
	}, "")

	return nil
}

// ---------------------------------------------------------------------------
// Archive / Unarchive / Retry
// ---------------------------------------------------------------------------

// Archive marks the event archived (removes it from the default list view
// without affecting calendar state).
func (s *HostedEventService) Archive(ctx context.Context, hostID, tenantID, eventID string) error {
	return s.toggleArchive(ctx, hostID, tenantID, eventID, true, "hosted_event.archived")
}

// Unarchive reverses Archive.
func (s *HostedEventService) Unarchive(ctx context.Context, hostID, tenantID, eventID string) error {
	return s.toggleArchive(ctx, hostID, tenantID, eventID, false, "hosted_event.unarchived")
}

func (s *HostedEventService) toggleArchive(ctx context.Context, hostID, tenantID, eventID string, archived bool, action string) error {
	event, err := s.repos.HostedEvent.GetByID(ctx, eventID)
	if err != nil {
		return err
	}
	if event == nil || event.HostID != hostID || event.TenantID != tenantID {
		return errors.New("hosted event not found")
	}
	if event.IsArchived == archived {
		return nil
	}
	event.IsArchived = archived
	event.UpdatedAt = models.Now()
	if err := s.repos.HostedEvent.Update(ctx, event); err != nil {
		return err
	}
	hID := hostID
	s.audit.Log(ctx, tenantID, &hID, action, "hosted_event", eventID, nil, "")
	return nil
}

// RetryCalendarEvent re-creates the calendar event for an event that has no
// tracking rows (i.e. the original creation failed). Useful when the host
// reconnects a calendar after the event was scheduled.
func (s *HostedEventService) RetryCalendarEvent(ctx context.Context, hostID, tenantID, eventID string) error {
	details, err := s.Get(ctx, hostID, tenantID, eventID)
	if err != nil {
		return err
	}
	if details == nil {
		return errors.New("hosted event not found")
	}
	if details.Event.Status != models.HostedEventStatusScheduled {
		return errors.New("only scheduled events can have calendar events retried")
	}

	syncInput := s.buildCalendarEventInput(details.Event, details.Attendees, details.Host, "")
	if _, _, err := s.syncer.Create(ctx, CalendarSyncRequest{
		Kind:   ItemKindHostedEvent,
		ItemID: details.Event.ID,
		Input:  syncInput,
		Hosts:  []HostTarget{{HostID: details.Host.ID, CalendarID: details.Event.CalendarID, IsOwner: true}},
	}); err != nil {
		return fmt.Errorf("syncer create: %w", err)
	}

	hID := hostID
	s.audit.Log(ctx, tenantID, &hID, "hosted_event.calendar_retried", "hosted_event", eventID, nil, "")
	return nil
}

// ---------------------------------------------------------------------------
// Conflict detection
// ---------------------------------------------------------------------------

// DetectBusyConflicts merges three sources to surface scheduling conflicts:
// provider busy times, the host's confirmed bookings, and other hosted events.
// excludeEventID skips an event from the hosted-events lookup so editing an
// existing event doesn't report itself. Returns the overlapping windows; the
// caller decides whether to warn or block (host-driven scheduling never
// blocks — soft warning only).
func (s *HostedEventService) DetectBusyConflicts(ctx context.Context, hostID string, start, end time.Time, excludeEventID string) ([]models.TimeSlot, error) {
	var conflicts []models.TimeSlot

	// Source 1: provider busy times.
	busy, err := s.calendar.GetBusyTimes(ctx, hostID, start, end)
	if err == nil {
		for _, b := range busy {
			if b.Start.Before(end) && b.End.After(start) {
				conflicts = append(conflicts, b)
			}
		}
	}

	// Source 2: confirmed bookings in the window.
	bookings, err := s.repos.Booking.GetByHostIDAndTimeRange(ctx, hostID, start, end)
	if err == nil {
		for _, b := range bookings {
			if b.Status != models.BookingStatusConfirmed {
				continue
			}
			conflicts = append(conflicts, models.TimeSlot{
				Start: b.StartTime.Time,
				End:   b.EndTime.Time,
			})
		}
	}

	// Source 3: other hosted events (scheduled status only; excluding self).
	var exclude *string
	if excludeEventID != "" {
		exclude = &excludeEventID
	}
	others, err := s.repos.HostedEvent.GetByHostIDAndTimeRange(ctx, hostID, start, end, exclude)
	if err == nil {
		for _, o := range others {
			conflicts = append(conflicts, models.TimeSlot{
				Start: o.StartTime.Time,
				End:   o.EndTime.Time,
			})
		}
	}

	// Sort for stable output.
	sort.Slice(conflicts, func(i, j int) bool {
		return conflicts[i].Start.Before(conflicts[j].Start)
	})

	return conflicts, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (s *HostedEventService) validateCreate(input CreateHostedEventInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("title is required")
	}
	if input.Duration <= 0 {
		return errors.New("duration must be positive")
	}
	if input.HostID == "" || input.TenantID == "" {
		return errors.New("host and tenant are required")
	}
	if len(input.Attendees) == 0 {
		return errors.New("at least one attendee is required")
	}
	for _, a := range input.Attendees {
		if !isValidEmail(a.Email) {
			return fmt.Errorf("invalid attendee email: %s", a.Email)
		}
	}
	return nil
}

// resolveCalendarID picks the calendar to write to: explicit input → template
// default → host default. Falls back to "" so the syncer's
// resolveWritableCalendarID can pick the first writable provider calendar.
func (s *HostedEventService) resolveCalendarID(ctx context.Context, host *models.Host, templateID *string, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if templateID != nil && *templateID != "" {
		template, err := s.repos.Template.GetByID(ctx, *templateID)
		if err == nil && template != nil && template.CalendarID != "" {
			return template.CalendarID, nil
		}
	}
	if host.DefaultCalendarID != nil && *host.DefaultCalendarID != "" {
		return *host.DefaultCalendarID, nil
	}
	return "", nil
}

// buildAttendeeRows converts AttendeeInputs into HostedEventAttendee rows.
// Emails are lower-cased and de-duplicated (case-insensitive) so the unique
// constraint on (hosted_event_id, email) is never tripped by minor user
// inconsistencies.
func (s *HostedEventService) buildAttendeeRows(eventID string, inputs []AttendeeInput, now models.SQLiteTime) []*models.HostedEventAttendee {
	seen := make(map[string]bool, len(inputs))
	out := make([]*models.HostedEventAttendee, 0, len(inputs))
	for _, in := range inputs {
		email := strings.ToLower(strings.TrimSpace(in.Email))
		if email == "" || seen[email] {
			continue
		}
		seen[email] = true
		out = append(out, &models.HostedEventAttendee{
			ID:            uuid.New().String(),
			HostedEventID: eventID,
			Email:         email,
			Name:          strings.TrimSpace(in.Name),
			ContactID:     in.ContactID,
			CreatedAt:     now,
		})
	}
	return out
}

// buildCalendarEventInput composes the provider-agnostic input the syncer
// hands to the calendar layer. existingEventID is set on update flows so the
// calendar provider patches in place; empty for create.
func (s *HostedEventService) buildCalendarEventInput(event *models.HostedEvent, attendees []*models.HostedEventAttendee, host *models.Host, existingEventID string) CalendarEventInput {
	emails := make([]string, 0, len(attendees))
	for _, a := range attendees {
		if a.Email != "" {
			emails = append(emails, a.Email)
		}
	}
	return CalendarEventInput{
		Summary:            event.Title,
		Description:        event.Description,
		Start:              event.StartTime.Time,
		End:                event.EndTime.Time,
		LocationType:       event.LocationType,
		CustomLocation:     event.CustomLocation,
		ConferenceLink:     event.ConferenceLink,
		Attendees:          emails,
		HostName:           host.Name,
		HostEmail:          host.Email,
		EventID:            existingEventID,
		MeetIdempotencyKey: event.ID,
	}
}

func attendeeEmails(attendees []*models.HostedEventAttendee) []string {
	out := make([]string, 0, len(attendees))
	for _, a := range attendees {
		out = append(out, a.Email)
	}
	return out
}

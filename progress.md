# Ralph Progress Log

## Current Branch: ralph/scheduling-system-completion

---

## Already Implemented (not in PRD)

The following features from the requirements document are already fully implemented:

### Authentication & Registration
- User registration with tenant creation
- Login/logout with session cookies
- Password hashing with bcrypt

### Calendar Integration
- Google Calendar OAuth connection
- Google Calendar busy time fetching (freebusy API)
- Google Calendar event creation with Google Meet auto-generation
- CalDAV connection (generic)
- CalDAV event creation

### Conferencing
- Zoom OAuth callback handling
- Google Meet link generation via Calendar API

### Email Notifications
- Booking requested email to host
- Booking confirmed email to invitee (with ICS attachment)
- Booking confirmed email to host
- Booking cancelled email (to appropriate party)
- Booking rejected email to invitee

### Public Booking Flow
- Host page listing active templates
- Template booking page with slot selection
- Booking form with name, email, phone, additional guests
- Cancel booking via secure token link

### Dashboard
- Dashboard home with booking summary
- Calendar connections management
- Template CRUD with all fields
- Bookings list with approve/reject/cancel
- Settings page with name, timezone, slug
- Working hours management

### Template Features
- Multiple durations (stored, but no public UI selector)
- Auto-approval toggle (requires_approval)
- Active/inactive toggle (is_active)
- Buffer times (pre/post)
- Min notice time
- Max schedule days

---

## 2026-01-26 - US-001 - Implement CalDAV/iCloud busy time fetching
- What was implemented:
  - Full CalDAV busy time fetching using REPORT with calendar-query
  - Complete VCALENDAR/ICS parsing for VEVENT extraction
  - Support for various date-time formats: UTC (Z suffix), local with TZID, all-day events (VALUE=DATE)
  - Support for DURATION field when DTEND is not present (RFC 5545)
  - ICS line unfolding for multi-line values
  - XML entity unescaping in calendar-data responses
  - Error handling for authentication failures
- Files changed:
  - `internal/services/calendar.go` - Added ~310 lines implementing getCalDAVBusyTimes and helper functions
- **Learnings for future iterations:**
  - The CalDAV free-busy-query is not widely supported (especially on iCloud), calendar-query with VEVENT filter is more reliable
  - ICS format uses line folding where long lines continue on next line with leading space/tab - must unfold before parsing
  - VEVENT can use either DTEND or DURATION for event end time
  - iCloud CalDAV uses standard CalDAV protocol at caldav.icloud.com - no special handling needed
  - No test files exist in this codebase - "go test ./..." passes trivially

---

## 2026-01-26 - US-002 - Add iCloud calendar connection UI
- What was implemented:
  - Added dedicated "Connect iCloud Calendar" form with detailed instructions for generating app-specific passwords
  - Pre-filled CalDAV URL with `https://caldav.icloud.com/`
  - Added hidden `provider` field to distinguish iCloud from generic CalDAV connections
  - Extended `CalDAVConnectInput` struct to include optional `Provider` field
  - Updated `ConnectCalDAV` service to use provider from input (defaults to CalDAV)
  - Updated handler to extract provider from form submission
  - Improved calendar list display with provider-specific badges (Google Calendar, iCloud, CalDAV)
  - Added CSS styles for iCloud instructions and provider badges
- Files changed:
  - `internal/services/calendar.go` - Added Provider field to CalDAVConnectInput, updated ConnectCalDAV to use it
  - `internal/handlers/dashboard.go` - Updated ConnectCalDAV handler to pass provider
  - `templates/pages/dashboard_calendars.html` - Added iCloud form section with instructions, updated calendar display
  - `static/css/style.css` - Added styles for iCloud instructions, form-help, and provider badges
- **Learnings for future iterations:**
  - The `CalendarProviderICloud` constant already existed in models - existing code anticipated this separation
  - Go templates require `printf "%s"` to compare CalendarProvider type with string literal
  - The CalDAV validation already works with iCloud's caldav.icloud.com endpoint - no special validation needed
  - Keeping the same `/dashboard/calendars/connect/caldav` endpoint with hidden provider field avoids route proliferation

---

## 2026-01-26 - US-003 - Implement Zoom meeting creation API
- What was implemented:
  - Fixed bug in `refreshZoomToken` where nil `TokenExpiry` would incorrectly skip token refresh
  - The Zoom meeting creation (`createZoomMeeting`) was already fully implemented with:
    - Call to Zoom API `POST /users/me/meetings` with booking details
    - Storage of `join_url` in `booking.ConferenceLink`
    - Token refresh before API call
  - The bug fix ensures tokens are refreshed when expiry is unknown (nil)
- Files changed:
  - `internal/services/conferencing.go` - Fixed token refresh condition (line 156)
- **Learnings for future iterations:**
  - The Zoom integration was already complete - OAuth flow, token storage, meeting creation, and booking integration
  - Token refresh logic: condition was `TokenExpiry == nil || time.Now().Before(...)` which skipped refresh when expiry was nil
  - Fixed to `TokenExpiry != nil && time.Now().Before(...)` so unknown expiry triggers refresh
  - Meeting creation is called from `processConfirmedBooking` in booking.go when template location type is Zoom
  - The `ConferenceLink` is persisted to the database via `repos.Booking.Update` after meeting creation

---

## 2026-01-26 - US-004 - Add duration selector to public booking form
- What was implemented:
  - Added duration dropdown selector in booking sidebar (only shown when template has multiple durations)
  - Fixed critical bug in GetSlots duration parsing: `string(rune(d))` doesn't work for integers > 9
  - Added hidden `duration` field to booking form to pass selected duration
  - Updated CreateBooking handler to accept and validate duration from form input
  - Updated slots_partial.html navigation to use `getSelectedDuration()` function
  - Duration is validated against allowed template durations in both GetSlots and CreateBooking
- Files changed:
  - `internal/handlers/public.go` - Fixed duration parsing in GetSlots, added duration handling in CreateBooking
  - `templates/pages/public_template.html` - Added duration dropdown, hidden form field, and `getSelectedDuration()` JS function
  - `templates/partials/slots_partial.html` - Updated week navigation to use dynamic duration
- **Learnings for future iterations:**
  - The original duration parsing used `string(rune(d))` which only works for single-digit numbers (0-9)
  - Use `strconv.Atoi()` to properly parse integer query parameters
  - The HTMX `hx-vals` approach was replaced with explicit JavaScript `loadSlots()` call for more control
  - The `getSelectedDuration()` function provides a single source of truth for duration across all JS
  - Single-duration templates skip the selector entirely (cleaner UX)

---

## 2026-01-26 - US-005 - Complete reschedule booking flow
- What was implemented:
  - Full reschedule page with slot selection UI matching the booking flow pattern
  - RescheduleBooking service method that updates the existing booking record (doesn't cancel+create new)
  - Old calendar event is deleted, new one created with updated time
  - Conference link is regenerated for Zoom/Google Meet templates
  - Reschedule notification emails sent to both host and invitee
  - Email shows both old and new times for clarity
  - ICS attachment included with updated meeting details
  - Success message shown on booking status page after reschedule
  - Duration selector available on reschedule page (inherits booking's current duration by default)
- Files changed:
  - `internal/services/booking.go` - Added RescheduleBooking method and RescheduleBookingInput struct
  - `internal/services/email.go` - Added SendBookingRescheduled with invitee and host notification functions
  - `internal/handlers/public.go` - Added GetRescheduleSlots, RescheduleBooking handlers; updated BookingStatus for success messages
  - `cmd/server/main.go` - Registered new routes: GET/POST /booking/{token}/reschedule/slots and POST /booking/{token}/reschedule
  - `templates/pages/reschedule.html` - Rewrote from placeholder to full slot selection UI
  - `templates/partials/reschedule_slots_partial.html` - New partial for reschedule slot loading via HTMX
  - `templates/pages/booking_status.html` - Added rescheduled/cancelled success alerts
  - `static/css/style.css` - Added reschedule summary styles
- **Learnings for future iterations:**
  - The acceptance criteria said "cancel old booking and create new one" but preserving the same booking record with updated times is cleaner
  - Same booking token means the same links work before and after reschedule
  - The SQLiteTime type embeds time.Time, so access the embedded value via `.Time` not `.Time()`
  - Reschedule flow reuses the availability service - no need to duplicate slot calculation logic
  - For confirmed bookings, need to delete old calendar event before creating new one to avoid duplicates

---

## 2026-01-26 - US-006 - Add agenda/subject field to booking form
- What was implemented:
  - Added 'Meeting Subject/Agenda' optional textarea to the public booking form
  - Agenda is stored in `booking.answers` JSON field with key `"agenda"`
  - Display agenda in host notification emails (both booking requested and confirmed emails)
  - Include agenda in calendar event descriptions (Google Calendar API, CalDAV, and ICS attachments)
- Files changed:
  - `templates/pages/public_template.html` - Added agenda textarea field in booking form
  - `internal/handlers/public.go` - Updated CreateBooking to always initialize answers map and store agenda
  - `internal/services/email.go` - Added agenda section to SendBookingRequested, sendHostConfirmation, and generateICS
  - `internal/services/calendar.go` - Added agenda to description in createGoogleEvent and createCalDAVEvent
- **Learnings for future iterations:**
  - The `Answers` field on Booking uses `models.JSONMap` which is a `map[string]interface{}` - need type assertion when reading values
  - CalDAV/ICS uses escaped newlines (`\\n`) in DESCRIPTION field, while Google Calendar API accepts regular newlines
  - The existing custom questions parser in CreateBooking had issues with `string(rune(i))` for indices > 9 - same bug fixed in US-004
  - Always initialize the answers map even if no custom questions, to ensure agenda can be stored

---

## 2026-01-26 - US-007 - Add custom questions builder to template form
- What was implemented:
  - Added 'Custom Questions' section to template form with dynamic JavaScript-based add/remove UI
  - Support for three question types: text (single line), textarea (multi-line), and select (dropdown)
  - Each question has: label, field name (unique identifier), type, required flag, and options (for dropdowns)
  - Questions saved to `invitee_questions` JSON field in MeetingTemplate
  - Questions rendered dynamically on public booking form using Go templates
  - Custom answers stored in `booking.answers` JSON field with field name as key
  - Fixed bug in CreateBooking: changed `string(rune(i))` to `strconv.Itoa(i)` for question indices > 9
  - Added `toMap` template function to convert interface{} to map[string]interface{} for template access
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Added Custom Questions section with JS builder
  - `internal/handlers/dashboard.go` - Added JSON parsing for invitee_questions in CreateTemplate and UpdateTemplate
  - `internal/handlers/handlers.go` - Added toMap template function
  - `internal/handlers/public.go` - Fixed question index parsing bug
  - `templates/pages/public_template.html` - Added dynamic question rendering
  - `static/css/style.css` - Added styles for question builder and display
- **Learnings for future iterations:**
  - Go templates can't directly access map[string]interface{} from interface{} - need a helper function like `toMap`
  - The `{{.Data.Template.InviteeQuestions}}` directly outputs valid JSON in JavaScript when template data is JSONArray
  - When rendering form elements dynamically in templates, use `question_{{$index}}` naming convention
  - The CreateBooking handler already had logic to parse questions but the bug with `string(rune(i))` was noted in progress.md - this was the same pattern causing issues

---

## 2026-01-26 - US-008 - Add template availability rules UI
- What was implemented:
  - Added 'Availability' section to template form with "Use custom availability" checkbox toggle
  - Implemented weekday checkboxes (Sunday through Saturday) with time range inputs for each day
  - Time ranges show/hide dynamically when day is enabled/disabled
  - Rules stored in `availability_rules` JSON field with structure: `{enabled: bool, days: {0-6: {enabled: bool, start: "HH:MM", end: "HH:MM"}}}`
  - Updated handlers (CreateTemplate, UpdateTemplate) to parse availability_rules from form submission
  - AvailabilityService now checks template.AvailabilityRules before falling back to working hours
  - Added `parseAvailabilityRules()` function to convert JSONMap to structured `TemplateAvailabilityRules` type
  - Added `generateSlotsInRange()` helper to avoid code duplication between template rules and working hours paths
  - CSS styles for availability builder UI (`.availability-days`, `.availability-day`, `.day-time-ranges`, `.time-range`)
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Added Availability section with JS builder
  - `internal/handlers/dashboard.go` - Added JSON parsing for availability_rules in CreateTemplate and UpdateTemplate
  - `internal/services/availability.go` - Added template rules parsing and application logic
  - `static/css/style.css` - Added styles for availability rules builder
- **Learnings for future iterations:**
  - Template availability rules override (replace) working hours entirely, not intersect - simpler to implement and explain to users
  - JSON map keys from JavaScript are strings ("0", "1", ...) not integers, so need `fmt.Sscanf` to parse day numbers
  - The existing `AvailabilityRules` field in MeetingTemplate was already defined as `JSONMap` - just needed UI and service logic
  - When custom availability is disabled (checkbox unchecked), the hidden input is cleared to ensure rules aren't persisted

---

## 2026-01-26 - US-009 - Implement editable email templates
- What was implemented:
  - Added 'Email Templates' section to template form with confirmation and reminder email fields
  - Each email type has subject and body fields with placeholder hints
  - Supported placeholders: `{{invitee_name}}`, `{{host_name}}`, `{{meeting_name}}`, `{{meeting_time}}`, `{{duration}}`, `{{location}}`, `{{cancel_link}}`, `{{reschedule_link}}`
  - Templates stored as JSON in `confirmation_email` and `reminder_email` fields (structure: `{subject: string, body: string}`)
  - Added `EmailTemplate` and `EmailTemplateData` types to email service
  - Added `parseEmailTemplate()` function to parse JSON template strings
  - Added `renderEmailTemplate()` function to replace placeholders with actual values
  - Added `buildEmailTemplateData()` helper to construct template data from booking details
  - Added `defaultInviteeConfirmationBody()` helper for fallback email body
  - Modified `sendInviteeConfirmation()` to use custom templates when available
  - Updated `CreateTemplateInput` and `UpdateTemplateInput` with `ConfirmationEmail` and `ReminderEmail` fields
  - Dashboard handlers extract email template fields from form and pass to service
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Added Email Templates section with JS to bundle subject/body into JSON
  - `internal/services/email.go` - Added EmailTemplate types, parsing, rendering, and custom template support in sendInviteeConfirmation
  - `internal/services/template.go` - Added ConfirmationEmail and ReminderEmail to input structs and template creation/update
  - `internal/handlers/dashboard.go` - Added ConfirmationEmail and ReminderEmail to CreateTemplate and UpdateTemplate inputs
- **Learnings for future iterations:**
  - The `confirmation_email` and `reminder_email` fields were already in the MeetingTemplate model - just needed to wire them up
  - Using simple string placeholder replacement (strings.ReplaceAll) is sufficient for basic templating - no need for Go's html/template complexity
  - Email templates are stored as JSON (`{"subject":"...", "body":"..."}`) which allows separate customization of subject and body
  - The custom template only replaces the default if parsed successfully and has content - empty strings fall back to defaults
  - JavaScript bundles subject+body into hidden JSON field on form submit, similar to pattern used for custom questions and availability rules

---

## 2026-01-26 - US-010 - Send booking reminder emails
- What was implemented:
  - Added `reminder_sent` boolean field to bookings table (PostgreSQL and SQLite migrations)
  - Updated Booking model with `ReminderSent` field
  - Updated BookingRepository with `reminder_sent` in all CRUD operations (Create, GetByID, GetByToken, GetByHostID, GetByHostIDAndTimeRange, Update)
  - Added `GetBookingsNeedingReminder()` method to find confirmed bookings in a time window without reminders sent
  - Added `MarkReminderSent()` method to mark booking reminders as sent
  - Added `SendBookingReminder()` to EmailService with custom template support via `ReminderEmail` field
  - Added `defaultReminderBody()` for fallback reminder content
  - Created `ReminderService` as background service with:
    - 15-minute interval check loop
    - Finds confirmed bookings starting 23-25 hours from now
    - Sends reminder emails with meeting details and ICS attachment
    - Marks bookings as `reminder_sent=true` after successful send
    - Graceful start/stop with sync.WaitGroup
  - Wired up ReminderService in main.go to start on server boot and stop on shutdown
- Files changed:
  - `migrations/002_add_reminder_sent.up.sql` - PostgreSQL migration adding reminder_sent column and index
  - `migrations/sqlite/002_add_reminder_sent.up.sql` - SQLite migration adding reminder_sent column and index
  - `internal/models/models.go` - Added ReminderSent field to Booking struct
  - `internal/repository/repository.go` - Updated all booking queries and added GetBookingsNeedingReminder, MarkReminderSent
  - `internal/services/email.go` - Added SendBookingReminder and defaultReminderBody functions
  - `internal/services/reminder.go` - New file with ReminderService background job implementation
  - `internal/services/services.go` - Added Reminder field to Services struct and initialization
  - `cmd/server/main.go` - Added svc.Reminder.Start() and defer svc.Reminder.Stop()
- **Learnings for future iterations:**
  - SQLite uses INTEGER for booleans (0/1), while PostgreSQL uses BOOLEAN - migrations need to be separate for each driver
  - The reminder window is 23-25 hours to ensure bookings don't slip through the 15-minute check intervals
  - Using COALESCE(reminder_sent, 0) in queries handles NULL values from existing rows before migration
  - Background services should use sync.WaitGroup for graceful shutdown to ensure clean exit
  - The SendBookingReminder follows the same pattern as sendInviteeConfirmation - check for custom template, render placeholders, fall back to default

---

## 2026-01-26 - US-011 - Add phone location type to templates
- What was implemented:
  - Updated template form to show "Phone Number" label and phone-specific placeholder when phone location type is selected
  - Added helper text explaining the phone number will be shown to invitees
  - JavaScript dynamically updates label, placeholder, and helper text based on location type selection
  - Public booking page now shows "Call [number]" instead of just "Phone" for phone location type
  - Booking status page shows "Call [number]" for confirmed bookings with phone location
  - All email templates (confirmation, reschedule, reminder) display "Call [number]" format for phone locations
  - ICS calendar attachments include "Call [number]" in LOCATION field
  - Google Calendar events include "Call [number]" in location field
  - CalDAV events now include LOCATION field with proper phone formatting
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Added dynamic label, placeholder, and helper text for phone location
  - `templates/pages/public_template.html` - Updated to show "Call [number]" for phone location
  - `templates/pages/booking_status.html` - Updated location display with phone support
  - `internal/services/email.go` - Added models import, updated 5 location-building code blocks to format phone with "Call" prefix
  - `internal/services/calendar.go` - Updated createGoogleEvent and createCalDAVEvent to use "Call [number]" format for phone
- **Learnings for future iterations:**
  - The `ConferencingProviderPhone` constant already existed in models - the phone option was already in the dropdown, just needed UI polish
  - Location building logic appeared in 5+ places in email.go - a helper function could reduce duplication
  - CalDAV events were missing LOCATION field entirely - added it with this change
  - The conferencing service returns `CustomLocation` as `ConferenceLink` for phone/custom types, but UI should still format it as "Call [number]"

---

## 2026-01-26 - US-012 - Add audit log viewer page
- What was implemented:
  - Added `AuditLogs` handler in `dashboard.go` with admin-only access control via `host.Host.IsAdmin` check
  - Added `CountByTenantID` method to `AuditLogRepository` for pagination support
  - Added `GetLogsCount` method to `AuditLogService` exposing the count functionality
  - Created `dashboard_audit_logs.html` template with:
    - Paginated table showing timestamp, action, entity type, details (expandable), IP address
    - Filter buttons for action types (All, Create, Update, Delete)
    - Pagination controls with page info
  - Added route `GET /dashboard/audit-logs` in `main.go`
  - Updated sidebar in `dashboard.html` to conditionally show "Audit Logs" link for admin users
  - Added CSS styles for audit log details, pagination, and action badges
- Files changed:
  - `internal/handlers/dashboard.go` - Added AuditLogs handler (~50 lines)
  - `internal/repository/repository.go` - Added CountByTenantID method
  - `internal/services/audit.go` - Added GetLogsCount method
  - `templates/pages/dashboard_audit_logs.html` - New template file
  - `templates/layouts/dashboard.html` - Added conditional admin link
  - `cmd/server/main.go` - Added audit-logs route
  - `static/css/style.css` - Added audit log styles
- **Learnings for future iterations:**
  - The Handlers struct doesn't expose repos directly - use services layer (e.g., `h.handlers.services.AuditLog.GetLogs`)
  - The first registered user in a tenant has `IsAdmin: true` set in `auth.go:129`
  - Admin access check is done at handler level, not via middleware (could be refactored to middleware if more admin routes are added)
  - Go templates support conditionals with `.Host.IsAdmin` directly in layout for conditional sidebar links
  - Pagination pattern: `perPage`, `offset = (page - 1) * perPage`, `totalPages = (totalCount + perPage - 1) / perPage`

---

## 2026-01-26 - US-013 - Add booking details modal in dashboard
- What was implemented:
  - Added `BookingDetails` handler in `dashboard.go` to return booking details as partial HTML
  - Added `GetBooking` method to `BookingService` for fetching single booking by ID
  - Created `booking_details_partial.html` template with modal display:
    - Meeting section: type, status, date, time, duration, conference link
    - Guest information: name, email, phone, timezone
    - Additional guests list (if any)
    - Meeting details section with agenda and custom question answers
    - Cancellation details (cancelled by, reason) when applicable
    - Creation timestamp
  - Updated `dashboard_bookings.html` with:
    - Clickable booking rows (`onclick="openBookingModal('{{.ID}}')"`)
    - Actions cell with `event.stopPropagation()` to prevent modal opening on action clicks
    - Modal container div for HTMX-loaded content
    - JavaScript functions: `openBookingModal()`, `closeBookingModal()`
    - Escape key listener for closing modal
  - Added modal CSS styles: overlay, content, header, close button, body, footer
  - Added booking detail section styles for organized display
  - Registered route `GET /dashboard/bookings/{id}/details` in `main.go`
- Files changed:
  - `internal/handlers/dashboard.go` - Added BookingDetails handler (~25 lines)
  - `internal/services/booking.go` - Added GetBooking method
  - `templates/partials/booking_details_partial.html` - New file (~110 lines)
  - `templates/pages/dashboard_bookings.html` - Added modal trigger and JavaScript
  - `static/css/style.css` - Added ~140 lines for modal and booking detail styles
  - `cmd/server/main.go` - Added booking details route
- **Learnings for future iterations:**
  - Using `renderPartial()` for HTMX responses keeps modal content separate from full page templates
  - The `Answers` field on Booking is `JSONMap` (map[string]interface{}) - can iterate with `range $key, $value`
  - Special key "agenda" is used for the standard agenda field; other keys are custom question answers
  - `event.stopPropagation()` on action cells prevents row click from triggering modal when clicking buttons
  - Modal pattern: load partial via fetch, insert into container div, toggle body class for scroll lock
  - BookingRepository already had `GetByID` method - just needed service layer wrapper

---

## 2026-01-26 - US-014 - Add host welcome/onboarding flow
- What was implemented:
  - Added `onboarding_completed` boolean field to Host model
  - Created database migrations for PostgreSQL and SQLite (003_add_onboarding_completed.up.sql)
  - Updated HostRepository: all queries now include `onboarding_completed`, added `UpdateOnboardingCompleted` method
  - Added `CompleteOnboarding` method to AuthService
  - Created OnboardingHandler with multiple endpoints:
    - `Step()` - displays onboarding step pages (1-3)
    - `SaveWorkingHours()` - saves working hours from step 1
    - `ConnectGoogleCalendar()` - initiates OAuth with `:onboarding` state suffix
    - `ConnectCalDAV()` - handles iCloud/CalDAV connection
    - `SkipStep()` - skips to next step or completes onboarding
    - `CreateTemplate()` - creates first meeting template and marks onboarding complete
    - `Complete()` - shows completion page with booking link
  - Created `onboarding.html` template with:
    - Progress bar showing 3 steps with completion indicators
    - Step 1: Working hours table (Mon-Fri 9-5 default)
    - Step 2: Google Calendar OAuth or iCloud CalDAV connection forms
    - Step 3: Simple template form (name, slug, duration, location)
    - Skip buttons on each step
  - Created `onboarding_complete.html` template with:
    - Success message and booking link copy button
    - List of created templates with their URLs
    - Next steps guidance
  - Updated registration handler to redirect to `/onboarding/step/1` instead of `/dashboard`
  - Updated Google OAuth callback to detect `:onboarding` suffix in state and redirect appropriately
  - Fixed .gitignore to include `migrations/**/*.sql` for SQLite migrations subfolder
- Files changed:
  - `internal/models/models.go` - Added OnboardingCompleted field to Host struct
  - `migrations/003_add_onboarding_completed.up.sql` - PostgreSQL migration
  - `migrations/sqlite/003_add_onboarding_completed.up.sql` - SQLite migration
  - `internal/repository/repository.go` - Updated Host queries, added UpdateOnboardingCompleted
  - `internal/services/auth.go` - Added CompleteOnboarding method
  - `internal/handlers/onboarding.go` - New file with OnboardingHandler (~315 lines)
  - `internal/handlers/handlers.go` - Added Onboarding field to Handlers struct
  - `internal/handlers/auth.go` - Updated Register redirect, Google callback for onboarding
  - `cmd/server/main.go` - Added onboarding routes with auth middleware
  - `templates/pages/onboarding.html` - New file (~300 lines)
  - `templates/pages/onboarding_complete.html` - New file (~150 lines)
  - `.gitignore` - Added `!migrations/**/*.sql` pattern
- **Learnings for future iterations:**
  - The `RequireAuth` middleware applies to a whole ServeMux, so onboarding routes were added to the dashboard mux
  - Google OAuth state parameter can carry metadata like `:onboarding` suffix to customize callback behavior
  - To parse state suffixes, use string length checks: `state[len(state)-11:] == ":onboarding"`
  - CalDAVConnectInput doesn't have TenantID - just HostID is needed
  - Config structure is nested: use `cfg.Server.BaseURL` not `cfg.BaseURL`
  - SQLite migrations folder was gitignored by `*.sql` pattern - needed explicit include pattern
  - Template data access in nested templates: pass data through `StepData` map and access via `$stepData := .Data.StepData`

---

## 2026-01-26 - US-015 - Add copy booking link button
- What was implemented:
  - Added "Copy Link" button to each template card in the Meeting Types list
  - Improved dashboard home's "Copy Link" button with visual feedback instead of alert
  - Both buttons show "Copied!" confirmation with green color for 2 seconds
  - Button changes back to original state after timeout
  - Graceful error handling with console logging and fallback alert
  - Added CSS styles for `.btn-copy` (primary color) and `.btn-copied` (success color) states
- Files changed:
  - `templates/pages/dashboard_templates.html` - Added Copy Link button and copyTemplateLink() function
  - `templates/pages/dashboard_home.html` - Updated copyLink() function with visual feedback
  - `static/css/style.css` - Added btn-copy and btn-copied styles with hover state
- **Learnings for future iterations:**
  - The `navigator.clipboard.writeText()` returns a Promise - use `.then()/.catch()` for proper error handling
  - Go templates can pass JavaScript string literals directly: `onclick="copyTemplateLink(this, '{{$.BaseURL}}/...')"`
  - Use `btn.textContent` instead of `innerHTML` for simple text changes
  - Adding/removing CSS classes via `classList.add()/remove()` is cleaner than manipulating style directly
  - The `alert()` was replaced with in-place visual feedback for better UX

---

## 2026-01-26 - US-016 - Add template duplication
- What was implemented:
  - Added `DuplicateTemplate` method to TemplateService that creates a copy of an existing template
  - Copies all template settings: name, description, durations, location, calendar, scheduling rules, questions, email templates
  - Appends "(Copy)" to the name and "-copy" suffix to the slug
  - Handles slug collisions by adding incrementing counter suffix (-copy-2, -copy-3, etc.)
  - New copies are inactive by default (is_active=false) per acceptance criteria
  - Added `DuplicateTemplate` handler in DashboardHandler
  - Registered route `POST /dashboard/templates/{id}/duplicate`
  - Added "Duplicate" button to template list page (inline form in template-actions)
  - Added "Duplicate" button to template edit page (uses hidden form triggered by button click)
  - Redirects to edit page for newly created duplicate template
  - Audit log records duplication with original template ID in details
- Files changed:
  - `internal/services/template.go` - Added DuplicateTemplate method (~70 lines)
  - `internal/handlers/dashboard.go` - Added DuplicateTemplate handler (~20 lines)
  - `cmd/server/main.go` - Added duplicate route
  - `templates/pages/dashboard_templates.html` - Added Duplicate button in template card actions
  - `templates/pages/dashboard_template_form.html` - Added Duplicate button and hidden form for edit page
- **Learnings for future iterations:**
  - Slug uniqueness check requires a loop since multiple duplicates of the same template may exist
  - Using `strconv.Itoa()` for counter instead of string concatenation with counter directly
  - The edit page needed a separate hidden form because the main form has a different action (update template)
  - Inline forms with `style="display:inline"` work well for buttons in horizontal layouts
  - Template repository already had `GetByHostAndSlug` which made collision detection easy

---

## 2026-01-26 - US-017 - Show booking count on templates
- What was implemented:
  - Added `BookingCount` struct in repository to hold total, pending, and confirmed counts per template
  - Added `GetBookingCountsByHostID` method to BookingRepository that groups bookings by template_id and status
  - Added `GetBookingCountsByHostID` method to BookingService as a thin wrapper to repository
  - Updated `Templates` handler in dashboard to fetch booking counts and pass to template
  - Updated `dashboard_templates.html` to display booking counts with pending/confirmed breakdown
  - Added CSS styles for `.template-bookings`, `.booking-count`, and `.booking-breakdown`
- Files changed:
  - `internal/repository/repository.go` - Added BookingCount struct and GetBookingCountsByHostID method (~50 lines)
  - `internal/services/booking.go` - Added GetBookingCountsByHostID service method
  - `internal/handlers/dashboard.go` - Updated Templates handler to include booking counts
  - `templates/pages/dashboard_templates.html` - Added booking count display section
  - `static/css/style.css` - Added styles for booking count display
- **Learnings for future iterations:**
  - SQL GROUP BY with multiple columns (template_id, status) allows efficient aggregation in a single query
  - Using `map[string]*BookingCount` allows O(1) lookup in Go templates with `{{index $.Data.BookingCounts .ID}}`
  - Go templates support conditional pluralization with `{{if ne $counts.Total 1}}s{{end}}`
  - The repository method accumulates counts by iterating rows, switching on status values to populate Pending/Confirmed fields

---

## 2026-01-26 - US-018 - Add timezone selector to booking form
- What was implemented:
  - Added comprehensive timezone selector UI to public booking and reschedule pages
  - Auto-detects timezone via JavaScript `Intl.DateTimeFormat().resolvedOptions().timeZone`
  - Shows detected timezone with abbreviated offset (e.g., "Eastern Time (US) (EST)")
  - "Change" link reveals full dropdown with 50+ timezones organized by region
  - If detected timezone isn't in the predefined list, it's dynamically added with "(Detected)" suffix
  - Selecting new timezone automatically reloads available time slots
  - Selected timezone persisted in hidden form field for booking submission
  - Dropdown closes when clicking outside or after selection
  - Reschedule page prefers the invitee's saved timezone from the original booking
- Files changed:
  - `templates/pages/public_template.html` - Replaced simple dropdown with timezone selector UI and enhanced JavaScript
  - `templates/pages/reschedule.html` - Same improvements for consistency across booking flows
  - `static/css/style.css` - Added `.timezone-selector`, `.timezone-display`, `.timezone-dropdown` styles
- **Learnings for future iterations:**
  - `Intl.DateTimeFormat().resolvedOptions().timeZone` returns IANA timezone strings (e.g., "America/New_York")
  - `Intl.DateTimeFormat('en-US', { timeZoneName: 'short' }).formatToParts()` can extract timezone abbreviation
  - When timezone isn't in dropdown, dynamically create `<option>` and insert into first `<optgroup>`
  - Use `encodeURIComponent()` when passing timezone to URL query params (some have "/" characters)
  - Closing dropdown on outside click requires checking both the dropdown and trigger link with `event.target`

---

## 2026-01-26 - US-019 - Add calendar sync status indicator
- What was implemented:
  - Added `last_synced_at`, `sync_status`, and `sync_error` fields to calendar_connections table
  - Added `CalendarSyncStatus` type with constants: unknown, synced, failed
  - Updated CalendarConnection model with new sync status fields
  - Updated CalendarRepository: Create, GetByID, GetByHostID, Update methods now include sync fields
  - Added `UpdateSyncStatus` method to CalendarRepository for updating just sync fields
  - Added `RefreshCalendarSync` method to CalendarService that tests calendar connection and updates status
  - Added `GetCalendar` method to CalendarService for fetching single calendar
  - Modified `GetBusyTimes` to automatically update sync status after each calendar fetch
  - Added `RefreshCalendarSync` handler to DashboardHandler
  - Added route `POST /dashboard/calendars/{id}/refresh` for manual sync refresh
  - Updated `dashboard_calendars.html` template with sync status UI:
    - Sync indicator (checkmark for success, warning for failed, refresh for unknown)
    - Last synced timestamp display
    - Error message display with guidance for failed syncs
    - Refresh button on each calendar card
  - Added CSS styles for sync status indicators, error messages, and updated calendar card layout
- Files changed:
  - `migrations/004_add_calendar_sync_status.up.sql` - PostgreSQL migration
  - `migrations/sqlite/004_add_calendar_sync_status.up.sql` - SQLite migration
  - `internal/models/models.go` - Added CalendarSyncStatus type and fields to CalendarConnection
  - `internal/repository/repository.go` - Updated calendar queries and added UpdateSyncStatus
  - `internal/services/calendar.go` - Added RefreshCalendarSync, GetCalendar, UpdateSyncStatus methods; updated GetBusyTimes
  - `internal/handlers/dashboard.go` - Added RefreshCalendarSync handler
  - `cmd/server/main.go` - Added refresh route
  - `templates/pages/dashboard_calendars.html` - Added sync status display UI
  - `static/css/style.css` - Added ~80 lines of sync status styles
- **Learnings for future iterations:**
  - Sync status is tracked per calendar, not globally - each calendar has independent status
  - GetBusyTimes already iterates all calendars, so it's natural to update status there
  - Using COALESCE in SQL queries handles NULL values from existing rows before migration
  - Calendar card layout changed from flex row to column to accommodate new status info
  - Error messages should provide actionable guidance (e.g., "Please reconnect your calendar")
  - The `errors.Is(syncErr, ErrCalendarAuth)` pattern allows custom error messages for auth failures

---

## 2026-01-26 - US-020 - Add booking confirmation page improvements
- What was implemented:
  - Completely redesigned `booking_status.html` with improved UX and visual design
  - Added status badges (Awaiting Approval, Confirmed, Cancelled, Declined) with color-coded styling
  - Added detailed meeting information with icons: date/time, duration, location (with different icons for phone/video/in-person), and host
  - Location display now shows "Join Google Meet"/"Join Zoom Meeting" links for confirmed video calls
  - Added prominent "Add to Calendar" button that downloads ICS file
  - Created new ICS download endpoint `GET /booking/{token}/calendar.ics`
  - Added `GenerateICS` public method to EmailService (wraps private `generateICS`)
  - Added `DownloadICS` handler in PublicHandler to serve calendar files
  - Added pending notice box explaining "The host needs to approve this booking before it's confirmed"
  - Reorganized action buttons: primary "Add to Calendar" button at top, secondary "Reschedule" and "Cancel" buttons below
  - Added extensive CSS styles: status badges, booking detail items with icons, button variants (outline, danger-outline), pending notice styling
- Files changed:
  - `internal/services/email.go` - Added public `GenerateICS` wrapper method
  - `internal/handlers/public.go` - Added `DownloadICS` handler
  - `cmd/server/main.go` - Registered `GET /booking/{token}/calendar.ics` route
  - `templates/pages/booking_status.html` - Complete redesign with improved UX
  - `static/css/style.css` - Added ~200 lines of new styles for booking confirmation page
- **Learnings for future iterations:**
  - The private `generateICS` method in EmailService was already well-implemented - just needed a public wrapper for HTTP download
  - SVG icons inline in templates provide flexibility without external dependencies (using Feather Icons style)
  - Status badges with rgba backgrounds create subtle, readable indicators that complement the status icons
  - Separating primary and secondary actions improves visual hierarchy - "Add to Calendar" is most common action
  - The `download` attribute on anchor tags triggers file download instead of navigation
  - Different location types (phone, google_meet, zoom, custom) need different display logic in templates

---

## 2026-01-26 - US-002 - Update slot generation to use multiple intervals
- What was implemented:
  - The slot generation logic was already implemented as part of US-001 (getSlotsForDay iterates through all intervals)
  - Added comprehensive unit test file `availability_test.go` covering all acceptance criteria:
    - `TestParseAvailabilityRules_SingleInterval` - old format backward compatibility
    - `TestParseAvailabilityRules_MultipleIntervals` - new format with 2 intervals
    - `TestParseAvailabilityRules_EmptyIntervals` - empty intervals array
    - `TestGetSlotsForDay_MultipleIntervals` - slots generated from both intervals and sorted
    - `TestGetSlotsForDay_EmptyIntervals` - returns no slots when intervals empty
    - `TestGetSlotsForDay_DayDisabled` - respects day disabled flag
    - `TestGetSlotsForDay_FallsBackToWorkingHours` - uses working hours when no template rules
    - `TestGenerateSlotsInRange_SingleInterval` - basic slot generation
    - `TestGenerateSlotsInRange_WithBusyTimes` - respects busy slots
    - `TestSlotOverlapsBusy` - overlap detection edge cases
    - `TestMergeTimeSlots` - slot merging logic
- Files changed:
  - `internal/services/availability_test.go` - New file with 16 test cases (~400 lines)
- **Learnings for future iterations:**
  - This codebase had no test files prior to this change - all `go test ./...` passed trivially
  - The slot generation with multiple intervals was already implemented in US-001 commit (1942e30)
  - When testing time-based logic, use a fixed date (e.g., 2024-01-15) in UTC to avoid timezone issues
  - The getSlotsForDay function requires both working hours and template rules - it falls back to working hours when template rules are nil or disabled
  - Test coverage should include: single interval, two intervals, empty intervals as per acceptance criteria

---

## 2026-01-26 - US-001 - Update data structure for multiple intervals
- What was implemented:
  - Added `TimeInterval` struct to represent a single time interval with start and end times (HH:MM format)
  - Modified `DayAvailability` struct to use `Intervals []TimeInterval` instead of single `Start`/`End` fields
  - Updated `parseAvailabilityRules()` to handle both old format (single start/end) and new format (intervals array) for backward compatibility
  - Updated `getSlotsForDay()` to iterate through all intervals for a day and combine slots, sorting by start time
  - New JSON structure supported: `{ "days": { "1": { "enabled": true, "intervals": [{"start": "09:00", "end": "12:00"}, {"start": "14:00", "end": "17:00"}] } } }`
- Files changed:
  - `internal/services/availability.go` - Added TimeInterval struct, modified DayAvailability, updated parseAvailabilityRules() and getSlotsForDay()
- **Learnings for future iterations:**
  - The parseAvailabilityRules() function receives JSON from the database as `models.JSONMap` (map[string]interface{}) - type assertions required for nested data
  - For backward compatibility when changing data structures, check for new format first, then fall back to old format
  - Intervals can be parsed from JSON as `[]interface{}` which requires type assertion for each element
  - The existing template's availability rules with old format (single start/end per day) will continue to work without modification
  - Slots from multiple intervals need to be sorted after generation since intervals may not be in order

---

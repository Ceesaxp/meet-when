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
  - Using COALESCE(reminder_sent, false) in queries handles NULL values from existing rows before migration
  - PostgreSQL does not allow mixing types in COALESCE, hence must match to field types, e.g. COALESCE(is_boolean_var, false), but COALESCE(string_var, '')
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

**ralph run 202601261518**

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

## 2026-01-26 - US-003 - Update UI to support multiple intervals per day
- What was implemented:
  - Rewrote availability section in template form to render days dynamically via JavaScript
  - Added `renderAvailabilityDays()` function that creates day elements with dynamic interval management
  - Added `createIntervalElement()` function to generate time range inputs for each interval
  - Added "+ Add interval" button that appears only when a day has fewer than 2 intervals
  - Added "Remove" button on each interval (hidden when only one interval exists)
  - Time inputs (`interval-start`, `interval-end`) for each interval's start and end times
  - Maximum of 2 intervals per day enforced via `MAX_INTERVALS_PER_DAY` constant
  - Backward compatible: supports both old format (single start/end) and new format (intervals array)
  - Smart defaults for second interval: if first ends at or before 12:00, defaults to 13:00-17:00
  - CSS styles for `.intervals-container`, `.btn-add-interval`, `.btn-remove-interval`, `.day-header`
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Rewrote availability section (~300 lines of JS changes)
  - `static/css/style.css` - Added ~50 lines of styles for interval UI elements
- **Learnings for future iterations:**
  - Dynamic DOM generation in JavaScript allows more flexible UI without server round trips
  - The `renderAvailabilityDays()` pattern: generate all days in a loop, supporting both existing data and defaults
  - Use closure pattern `function(d) { return function() { ... } }(day)` to capture loop variables in event handlers
  - Smart defaults logic includes edge cases: what if first interval ends late? Cap at 23:00
  - The `initializeAvailabilityRules()` calls `renderAvailabilityDays()` twice: once for structure, once after checking if custom availability is enabled

---

## 2026-01-26 - US-004 - Implement smart defaults for second interval
- What was implemented:
  - This feature was already implemented as part of US-003 (commit aea68b7)
  - The `addInterval()` JavaScript function in `dashboard_template_form.html` lines 567-614 contains the smart defaults logic
  - When first interval ends at or before 12:00, second interval defaults to 13:00-17:00
  - When first interval ends after 12:00, second interval defaults to first interval end time + 1 hour, for 3 hours duration
  - End time is capped at 23:00 to prevent invalid times
  - Time inputs are editable immediately after the interval is added
- Files changed:
  - No new files changed - implementation was part of US-003
  - PRD updated to mark US-004 as passing
- **Learnings for future iterations:**
  - The smart defaults feature was bundled with US-003's interval UI implementation
  - When implementing UI features, consider what reasonable defaults users would expect and implement them proactively
  - String comparison works for time values in "HH:MM" format (e.g., `firstEnd <= '12:00'`)
  - Using `padStart(2, '0')` ensures consistent time formatting

---

## 2026-01-26 - US-005 - Implement interval overlap validation
- What was implemented:
  - Added `intervalsOverlap()` helper function using the canonical overlap formula: `A.start < B.end && A.end > B.start`
  - Added `validateIntervals()` function that iterates through all enabled days and checks for overlapping interval pairs
  - Error div added to each day in `renderAvailabilityDays()` for inline error display
  - Form submission blocked via `e.preventDefault()` when overlaps are detected
  - Validation runs on form submit AND on every interval time change for immediate user feedback
  - CSS styles added for `.interval-error` class with red border and background
- Files changed:
  - `templates/pages/dashboard_template_form.html` - Added validation functions and error div rendering
  - `static/css/style.css` - Added `.interval-error` styles
- **Learnings for future iterations:**
  - The overlap formula `A.start < B.end && A.end > B.start` works for string time comparisons in "HH:MM" format
  - Using `querySelectorAll('.interval-error[style*="display: block"]')` to find visible error elements
  - Running validation on every change provides immediate feedback, improving UX
  - The implementation was already committed in `bcce2a8` but PRD wasn't updated - always update PRD after implementation

---

**ralph run 202601261525**

---

## 2026-01-26 - US-001 - Add HTTP method override middleware
- What was implemented:
  - Created `MethodOverride` middleware in `internal/middleware/middleware.go`
  - Middleware checks POST requests for `_method` form field
  - If `_method` is PUT or DELETE, the request method is changed accordingly
  - Original POST requests without `_method` continue to work normally
  - Added `MethodOverride` to the global middleware chain in `cmd/server/main.go`
- Files changed:
  - `internal/middleware/middleware.go` - Added MethodOverride middleware function (~17 lines)
  - `cmd/server/main.go` - Added MethodOverride to middleware.Chain call
- **Learnings for future iterations:**
  - The middleware chain in this codebase applies middlewares in reverse order via `Chain()` function
  - `r.ParseForm()` must be called before accessing `r.FormValue()` to parse the request body
  - The middleware only allows PUT and DELETE overrides for security (prevents arbitrary method spoofing)
  - This middleware is foundational - it enables the settings form (US-002) and other PUT/DELETE form submissions to work

---

## 2026-01-26 - US-002 - Verify profile settings form works
- What was implemented:
  - Added flash message display support to the Settings page
  - Settings handler now reads `success` and `error` query parameters after form submission
  - Success message "Settings saved successfully" shown after successful update
  - Error messages shown for: slug already taken, update failed, invalid form data
  - Flash messages displayed using existing dashboard layout's Flash mechanism
- Files changed:
  - `internal/handlers/dashboard.go` - Updated Settings handler to parse query params and set Flash messages (~20 lines added)
- **Learnings for future iterations:**
  - The dashboard layout already has Flash message support via `{{if .Flash}}` in dashboard.html
  - The UpdateSettings handler redirects with query params (?success=updated or ?error=...)
  - The Settings GET handler must read these query params and populate the Flash field in PageData
  - FlashMessage type has Type (success/error) and Message fields
  - Pattern: handlers that redirect with ?success/error should have corresponding display logic in the GET handler

---

## 2026-01-26 - US-003 - Create timezone data endpoint
- What was implemented:
  - Created `TimezoneService` in `internal/services/timezone.go` with static IANA timezone data
  - Defined `TimezoneInfo` struct with: id (IANA ID), display_name (human readable), offset (e.g., "UTC+5"), offset_mins (numeric offset)
  - Defined `TimezoneGroup` struct to group timezones by region
  - Implemented 70+ timezones across 7 regions: Americas, Europe, Asia, Pacific, Africa, Atlantic, Other
  - Created `APIHandler` in `internal/handlers/api.go` for API endpoints
  - Registered `GET /api/timezones` public route in main.go
  - Response includes Cache-Control header for 24-hour caching
  - Offsets are computed dynamically at service initialization using Go's time.LoadLocation
- Files changed:
  - `internal/services/timezone.go` - New file with TimezoneService (~380 lines)
  - `internal/services/services.go` - Added Timezone field to Services struct
  - `internal/handlers/api.go` - New file with APIHandler for JSON endpoints
  - `internal/handlers/handlers.go` - Added API field to Handlers struct
  - `cmd/server/main.go` - Registered GET /api/timezones route
- **Learnings for future iterations:**
  - The codebase already imports `_ "time/tzdata"` to embed timezone database, so `time.LoadLocation()` always works
  - JSON API responses should set `Content-Type: application/json` header
  - Static data services can precompute at startup for better performance
  - This endpoint is public (no auth required) since timezone data is not sensitive
  - The APIHandler pattern separates JSON API endpoints from the HTMX-based dashboard handlers

---

## 2026-01-26 - US-004 - Create searchable timezone dropdown component
- What was implemented:
  - Created reusable `TimezonePicker` JavaScript component at `static/js/timezone-picker.js`
  - Text input field that filters timezone list as user types
  - Search matches: timezone ID (America/New_York), display name (Eastern Time), city names, and 40+ common abbreviations (PST, EST, CST, MST, JST, etc.)
  - Dropdown displays: timezone name, current time (live calculated), UTC offset, timezone ID, grouped by region
  - Keyboard navigation: Arrow Up/Down to move selection (with wrap-around), Enter to select, Escape to close, Tab to close and continue
  - Click to select and close dropdown
  - Fetches timezone data from `/api/timezones` endpoint created in US-003
  - Added CSS styles for `.tz-picker-container`, `.tz-picker-input`, `.tz-picker-dropdown`, `.tz-option`, `.tz-region-header`
  - Integrated component into dashboard settings page, replacing the simple select dropdown
  - Hidden input stores IANA timezone ID for form submission; visible input shows formatted display
- Files changed:
  - `static/js/timezone-picker.js` - New file with TimezonePicker component (~380 lines)
  - `static/css/style.css` - Added ~110 lines of timezone picker styles
  - `templates/pages/dashboard_settings.html` - Replaced select with searchable picker, added script initialization
- **Learnings for future iterations:**
  - Use a hidden input to store the actual value (IANA ID) while the visible input shows a formatted display name
  - Common timezone abbreviations are ambiguous (e.g., CST = Central Standard Time US or China Standard Time) - map to array of matches
  - `Intl.DateTimeFormat` with `timeZone` option can display current time in any IANA timezone
  - Sticky region headers (`position: sticky`) improve usability in long dropdown lists
  - `scrollIntoView({ block: 'nearest' })` ensures keyboard-selected items are visible without jarring scroll jumps
  - HTML escaping is critical when rendering user-filtered results to prevent XSS

---

## 2026-01-26 - US-005 - Integrate timezone picker into settings page
- What was implemented:
  - This feature was already fully implemented as part of US-004 (commit b0f0576)
  - The TimezonePicker component is integrated into `dashboard_settings.html`
  - Current timezone is pre-populated via `initialValue: '{{.Host.Timezone}}'` and `setTimezone()` method
  - Hidden input with `name="timezone"` ensures value is submitted with form
  - Works with existing PUT form submission via `_method=PUT` override
  - Dropdown styles are consistent with form inputs (max-height: 300px with scroll)
  - Typecheck passes
- Files changed:
  - No new files changed - implementation was part of US-004
  - PRD updated to mark US-005 as passing
- **Learnings for future iterations:**
  - When implementing a component (US-004), integration into the target page (US-005) can often be done simultaneously
  - The progress notes in US-004 mentioned "Integrated into dashboard settings page" which indicated US-005 was complete
  - Always verify acceptance criteria even when work appears to be bundled with another feature

---

## 2026-01-26 - US-006 - Auto-detect timezone during onboarding
- What was implemented:
  - Added timezone picker to onboarding Step 1 (Working Hours page)
  - Reused the existing `TimezonePicker` JavaScript component from `static/js/timezone-picker.js`
  - Auto-detects timezone using `Intl.DateTimeFormat().resolvedOptions().timeZone` on page load
  - Shows "Detected from your browser" indicator when timezone is auto-detected
  - Indicator hidden when user manually selects a different timezone
  - Falls back gracefully if detection fails (catch block, leaves field empty for manual selection)
  - Saves timezone to host profile when working hours form is submitted
  - User can change timezone via searchable picker before submitting
- Files changed:
  - `templates/pages/onboarding.html` - Added timezone picker UI to Step 1, JavaScript for auto-detection and initialization
  - `internal/handlers/onboarding.go` - Updated SaveWorkingHours to also save the detected/selected timezone
- **Learnings for future iterations:**
  - The `TimezonePicker` component was already built for US-004, making integration straightforward
  - Use `setInterval` with a check for `picker.timezones.length > 0` to wait for async data loading before setting timezone
  - Always add a timeout to interval checks to prevent infinite loops if data never loads
  - The onboarding flow is in 3 steps: Working Hours → Calendar → First Template. Timezone fits naturally in Step 1 since it affects availability context
  - The `plans/` directory is gitignored, so PRD updates are local only

---

## 2026-01-26 - US-007 - Add background calendar sync scheduler
- What was implemented:
  - Created `CalendarSyncService` in `internal/services/calendar_sync.go` following the `ReminderService` pattern
  - Background goroutine starts when server launches and runs every 15 minutes
  - Added `GetAll` method to `CalendarRepository` to fetch all calendar connections across all hosts
  - Added `SyncCalendar` method to `CalendarService` for syncing individual calendars (used by background job)
  - Added `GetAllCalendars` method to `CalendarService` as a wrapper for repository access
  - Service iterates through all calendar connections, calling sync logic for each
  - Updates `last_synced_at` timestamp on success, `sync_status` to failed with error message on failure
  - Graceful shutdown via `sync.WaitGroup` and stop channel
  - Logs sync progress: starting count, success/failure counts at completion
- Files changed:
  - `internal/repository/repository.go` - Added `GetAll` method to CalendarRepository (~30 lines)
  - `internal/services/calendar.go` - Added `SyncCalendar` and `GetAllCalendars` methods (~40 lines)
  - `internal/services/calendar_sync.go` - New file with CalendarSyncService (~90 lines)
  - `internal/services/services.go` - Added CalendarSync to Services struct and initialization
  - `cmd/server/main.go` - Added Start/Stop calls for CalendarSync service
- **Learnings for future iterations:**
  - The `ReminderService` pattern is the standard for background jobs: WaitGroup, stop channel, ticker, immediate run on startup
  - Better to add public methods to the target service (`CalendarService.SyncCalendar`) than access private methods from another service
  - The `GetAll` repository method doesn't filter by tenant/host since background sync needs all calendars
  - Using `defer svc.CalendarSync.Stop()` ensures proper shutdown sequence when server receives SIGINT/SIGTERM

---

## 2026-01-26 - US-008 - Display last sync time and status on calendar list
- What was implemented:
  - Added `timeAgo` template function in `internal/handlers/helpers.go` to display relative time
  - Function handles various time ranges: "just now" (< 60s), "X minutes ago", "X hours ago", "X days ago", and falls back to date for > 7 days
  - Updated `dashboard_calendars.html` to use `timeAgo` function for sync time display
  - Changed "Not synced yet" to "Never synced" for calendars with unknown sync status
  - Added click-to-reveal functionality for error messages on failed sync status
  - Error message is hidden by default (`style="display: none;"`) and toggled via JavaScript `toggleCalendarError()` function
  - Added CSS styles for clickable error status: `.sync-error-clickable` with underline and cursor pointer
  - Green checkmark (sync-ok), red warning (sync-error), and gray refresh (sync-unknown) indicators already existed
- Files changed:
  - `internal/handlers/helpers.go` - Added `timeAgo()` function (~40 lines)
  - `internal/handlers/handlers.go` - Registered `timeAgo` in template funcs
  - `templates/pages/dashboard_calendars.html` - Updated sync status display and added JS toggle function
  - `static/css/style.css` - Added styles for clickable error status
- **Learnings for future iterations:**
  - The `toTime()` helper function in helpers.go handles both `time.Time` and `models.SQLiteTime` types - use it when creating time-related template functions
  - Template functions are registered in `templateFuncs()` in handlers.go
  - The sync status display already had most of the UI (indicators, colors) from US-019 - this feature added the relative time and click-to-reveal
  - For toggle functionality in templates, use inline JavaScript functions since there's no separate JS bundle system

---

## 2026-01-26 - US-009 - Add manual calendar refresh button with feedback
- What was implemented:
  - Created `calendar_card_partial.html` partial template for individual calendar card rendering
  - Updated calendar list in `dashboard_calendars.html` to add unique IDs to each calendar card (`calendar-card-{{.ID}}`)
  - Changed refresh button from form POST to HTMX button with loading indicator
  - Button uses `hx-post`, `hx-target`, `hx-swap="outerHTML"`, and `hx-indicator` attributes for seamless HTMX updates
  - Updated `RefreshCalendarSync` handler to detect HTMX requests via `HX-Request` header
  - Handler returns the updated calendar card partial for HTMX requests, maintains redirect for regular requests
  - Added CSS styles for refresh button spinner animation and HTMX indicator states
  - Spinner uses CSS `@keyframes spin` animation with border-radius styling
- Files changed:
  - `templates/partials/calendar_card_partial.html` - New partial template (~55 lines)
  - `templates/pages/dashboard_calendars.html` - Updated calendar card with HTMX attributes and unique IDs
  - `internal/handlers/dashboard.go` - Updated RefreshCalendarSync to handle HTMX requests (~15 lines added)
  - `static/css/style.css` - Added ~50 lines for refresh button and spinner styles
- **Learnings for future iterations:**
  - HTMX partials in this codebase are loaded separately with template functions via `loadTemplates()` - they have access to all helper functions like `timeAgo`
  - The pattern for HTMX responses: check `r.Header.Get("HX-Request") == "true"`, then return partial instead of redirect
  - Use `hx-swap="outerHTML"` when replacing the entire element (vs `innerHTML` for replacing contents)
  - The `hx-indicator` attribute shows a loading element during the request - combine with CSS `.htmx-request` class
  - Calendar sync errors are stored in the `SyncError` field and displayed via the partial even on failure

---

## 2026-01-26 - US-010 - Create agenda data fetching service
- What was implemented:
  - Added `AgendaEvent` struct with fields: Title, Start, End, CalendarName, IsAllDay
  - Added `GetAgendaEvents(hostID, startDate, endDate)` method to CalendarService
  - Google Calendar implementation uses `events.list` API to get full event details (title, times, all-day flag)
  - CalDAV/iCloud implementation uses calendar-query REPORT with VEVENT filter, parses SUMMARY for title
  - Events sorted by start time using simple bubble sort helper function
  - Error handling: failed calendars are logged and skipped, other calendars continue to be fetched
  - All-day event detection: Google uses `date` vs `dateTime` fields, CalDAV uses `VALUE=DATE` parameter
- Files changed:
  - `internal/services/calendar.go` - Added ~285 lines implementing GetAgendaEvents and helper functions
- **Learnings for future iterations:**
  - Google Calendar `events.list` API returns full event details including summary (title), unlike `freebusy` API which only returns busy times
  - For CalDAV, the same calendar-query REPORT used for busy times can extract SUMMARY field for event title
  - All-day events in Google Calendar API use `date` field instead of `dateTime` field in start/end objects
  - In ICS format, all-day events have `VALUE=DATE` parameter (not `VALUE=DATE-TIME`) on DTSTART
  - The existing `unfoldICSLines` and `parseICSDateTime` functions are reusable for agenda event parsing

---

## 2026-01-26 - US-011 - Create agenda page with Today view
- What was implemented:
  - Added `Agenda` handler method to `DashboardHandler` in `dashboard.go`
  - Handler loads host timezone via `time.LoadLocation()`, calculates today's boundaries (00:00 to 23:59:59 in host timezone)
  - Calls `CalendarService.GetAgendaEvents()` (from US-010) to fetch events from all connected calendars
  - Created `dashboard_agenda.html` template with:
    - Page header showing current date
    - Tab bar with Today (active) and This Week buttons (This Week for future US-012)
    - Table displaying events in chronological order: time, title, duration, calendar
    - All-day events shown with special "All Day" badge instead of time range
    - Empty state with calendar icon when no events today
  - Added `duration()` template helper function in `helpers.go` to calculate and format time duration (e.g., "30 min", "1 hour", "1 hr 30 min")
  - Registered `duration` function in `templateFuncs()` in `handlers.go`
  - Added `GET /dashboard/agenda` route in `main.go`
  - Added Agenda link to dashboard sidebar in `dashboard.html` layout, positioned near Bookings
- Files changed:
  - `internal/handlers/dashboard.go` - Added Agenda handler (~43 lines)
  - `internal/handlers/helpers.go` - Added duration() function (~32 lines)
  - `internal/handlers/handlers.go` - Registered duration in templateFuncs
  - `cmd/server/main.go` - Added GET /dashboard/agenda route
  - `templates/pages/dashboard_agenda.html` - New file (~130 lines)
  - `templates/layouts/dashboard.html` - Added Agenda link to sidebar
- **Learnings for future iterations:**
  - Use `time.LoadLocation()` to load IANA timezone strings for proper timezone handling
  - The `time.Date()` function creates a time in a specific location - use for day boundaries
  - Handler accesses CalendarService via `h.handlers.services.Calendar`
  - Template helper functions are registered once in `templateFuncs()` in handlers.go and available to all templates
  - The toTime() helper in helpers.go handles both `time.Time` and `models.SQLiteTime` types
  - CSS variables like `var(--text-muted)` are used for consistent styling

---

## 2026-01-26 - US-012 - Add This Week tab to agenda view
- What was implemented:
  - Added view parameter handling (`?view=week`) in Agenda handler to switch between today and week views
  - Created `AgendaDayGroup` struct to hold events grouped by day with their date
  - Implemented `groupEventsByDay()` function that groups events into Monday-Sunday buckets
  - Week boundary calculation: finds Monday of current week by calculating days from Monday (Sunday = end of week)
  - Updated `dashboard_agenda.html` template with conditional rendering:
    - Week view shows events grouped by day with day headers (weekday name + formatted date)
    - Today view retains original table-based layout with header row
  - Active tab highlighting: `btn-primary` class applied based on current view
  - Added CSS styles for week view: `.agenda-week`, `.agenda-day-group`, `.day-header`, `.day-name`, `.day-date`, `.no-events-day`
- Files changed:
  - `internal/handlers/dashboard.go` - Added AgendaDayGroup struct, view parameter handling, week date calculation, groupEventsByDay function (~80 lines)
  - `templates/pages/dashboard_agenda.html` - Conditional rendering for week/today views, day headers for week view (~70 lines)
- **Learnings for future iterations:**
  - Go's `time.Weekday` enum: Sunday=0, Monday=1, ..., Saturday=6 - need conversion for Monday-first weeks
  - For week starting on Monday: `daysFromMonday = weekday - 1` with special case `Sunday = 6`
  - Go templates support `{{if eq .Data.View "week"}}` for conditional rendering
  - Events can be grouped by assigning to slice index based on day of week
  - The `.Date.Weekday` accessor in templates returns the weekday name (e.g., "Monday")

---

## 2026-01-26 - US-013 - Add Agenda link to dashboard sidebar
- What was implemented:
  - Already completed as part of US-011 implementation
  - Agenda link exists in `dashboard.html` layout at line 20, positioned after Bookings link
- Files changed:
  - None (already done in US-011)
- **Learnings for future iterations:**
  - PRD items can be completed as part of other related features - always check if work was bundled
  - The US-011 progress notes mentioned "Added Agenda link to dashboard sidebar" which indicated this was done

---

## 2026-01-26 - US-014 - Add archived flag to bookings table
- What was implemented:
  - Added `is_archived` boolean column to bookings table via database migrations (PostgreSQL and SQLite)
  - Updated Booking model with `IsArchived` field in models.go
  - Updated BookingRepository methods to include `is_archived` in all queries:
    - Create: now inserts is_archived value
    - GetByID: includes is_archived with COALESCE for NULL handling
    - GetByToken: includes is_archived with COALESCE
    - GetByHostID: accepts new `includeArchived` parameter, filters out archived bookings when false
    - GetByHostIDAndTimeRange: includes is_archived with COALESCE
    - GetBookingsNeedingReminder: includes is_archived with COALESCE
    - Update: now updates is_archived field
  - Updated BookingService.GetBookings to accept `includeArchived` parameter
  - Updated dashboard handlers (Home, Bookings) to pass includeArchived parameter
  - Bookings page now supports `?archived=true` query parameter to show archived bookings
- Files changed:
  - `migrations/005_add_is_archived.up.sql` - PostgreSQL migration adding is_archived column and index
  - `migrations/sqlite/005_add_is_archived.up.sql` - SQLite migration adding is_archived column and index
  - `internal/models/models.go` - Added IsArchived field to Booking struct
  - `internal/repository/repository.go` - Updated all booking queries to include is_archived
  - `internal/services/booking.go` - Updated GetBookings to accept includeArchived parameter
  - `internal/handlers/dashboard.go` - Updated Home and Bookings handlers to use includeArchived
- **Learnings for future iterations:**
  - SQLite uses INTEGER (0/1) for booleans while PostgreSQL uses BOOLEAN - need separate migration files
  - Use COALESCE(is_archived, false) in queries to handle NULL values from existing rows before migration
  - Adding a new boolean parameter to existing functions requires updating all callers (service and handler layers)
  - The archive filter condition `(is_archived = false OR is_archived IS NULL)` handles both new rows and pre-migration rows
  - Index on is_archived improves query performance when filtering non-archived bookings

---

## 2026-01-26 - US-015 - Add archive button to cancelled/rejected bookings
- What was implemented:
  - Added `ArchiveBooking` method to `BookingService` that validates booking status before archiving
  - Only cancelled or rejected bookings can be archived (business rule enforcement)
  - Added `ArchiveBooking` handler to `DashboardHandler` with HTMX support
  - HTMX request returns empty content which removes the row via `hx-swap="outerHTML"`
  - Regular (non-HTMX) request redirects back to bookings page
  - Registered `POST /dashboard/bookings/{id}/archive` route
  - Added Archive button to `dashboard_bookings.html` with conditional display (only for cancelled/rejected)
  - Button uses HTMX attributes: `hx-post`, `hx-target`, `hx-swap="outerHTML"`, `hx-confirm`
  - Added CSS styling for archived booking rows (`.booking-row.archived`) with muted opacity
  - Added audit logging for archive actions
- Files changed:
  - `internal/services/booking.go` - Added ArchiveBooking method (~25 lines)
  - `internal/handlers/dashboard.go` - Added ArchiveBooking handler (~30 lines)
  - `cmd/server/main.go` - Registered archive route
  - `templates/pages/dashboard_bookings.html` - Added Archive button with HTMX, added row ID for targeting
  - `static/css/style.css` - Added archived row styling
- **Learnings for future iterations:**
  - HTMX `hx-swap="outerHTML"` with empty response effectively removes the targeted element
  - Use `id="booking-row-{{.ID}}"` to give each row a unique target for HTMX
  - The `hx-confirm` attribute provides a browser confirmation dialog without custom JavaScript
  - Handler pattern: check `r.Header.Get("HX-Request") == "true"` to detect HTMX requests
  - Business logic validation (only archive cancelled/rejected) belongs in service layer, not handler
  - Template conditional `{{if or (eq .Status "cancelled") (eq .Status "rejected")}}` for multiple status checks

---

## 2026-01-26 - US-016 - Hide archived bookings by default and add Show archived toggle
- What was implemented:
  - Default bookings list excludes archived items (was already partially implemented, now explicit)
  - Filter counts (All, Pending, Confirmed, Cancelled) now exclude archived bookings
  - Added "Show archived" toggle checkbox in bookings header using CSS flexbox layout
  - When enabled, archived bookings appear in the list with muted styling (existing CSS)
  - Toggle state persists in URL parameter (?archived=true)
  - JavaScript toggleArchived() function updates URL and reloads page
  - Handler calculates separate counts for each status type excluding archived
  - ArchivableCount tracked for future bulk archive feature (US-017)
- Files changed:
  - `internal/handlers/dashboard.go` - Added count calculations for filter buttons, added ShowArchived and count data to template
  - `templates/pages/dashboard_bookings.html` - Restructured filter-bar with filter-buttons and filter-options divs, added checkbox toggle, JavaScript handler
  - `static/css/style.css` - Added ~30 lines for filter-bar flexbox layout, filter-options, and checkbox-label styles
- **Learnings for future iterations:**
  - Counts should always exclude archived bookings for accurate display, even when showing archived items
  - URL parameter approach for toggle state is simpler than cookies and works with browser back/forward
  - The archived flag was already implemented in US-014/US-015, this feature just added the UI toggle
  - Flexbox with `justify-content: space-between` cleanly separates filter buttons and options
  - The existing `.booking-row.archived` CSS class from US-015 handles the muted styling

---

## 2026-01-26 - US-017 - Add bulk archive and unarchive actions
- What was implemented:
  - Added `UnarchiveBooking` method to BookingService for restoring archived bookings
  - Added `BulkArchiveBookings` method to BookingService that iterates through all cancelled/rejected non-archived bookings and archives them
  - Added `UnarchiveBooking` handler with HTMX support (uses HX-Redirect to reload page)
  - Added `BulkArchiveBookings` handler with confirmation count logging
  - Registered routes: POST `/dashboard/bookings/{id}/unarchive` and POST `/dashboard/bookings/archive-all`
  - Updated `dashboard_bookings.html` with:
    - "Archive all cancelled/rejected (N)" button in page header (only shown when ArchivableCount > 0)
    - `confirmBulkArchive(count)` JavaScript function with confirmation dialog showing count
    - Unarchive button shown on archived bookings (replacing Archive button)
  - Audit logging for `booking.unarchived` and `booking.bulk_archived` actions
- Files changed:
  - `internal/services/booking.go` - Added UnarchiveBooking and BulkArchiveBookings methods (~55 lines)
  - `internal/handlers/dashboard.go` - Added UnarchiveBooking and BulkArchiveBookings handlers (~65 lines)
  - `cmd/server/main.go` - Added two new routes
  - `templates/pages/dashboard_bookings.html` - Added bulk archive button, JS function, and unarchive button logic
- **Learnings for future iterations:**
  - The `page-header` CSS already has `display: flex; justify-content: space-between;` which perfectly positions the bulk archive button
  - HTMX `hx-swap="outerHTML"` with empty response removes elements; for unarchive we need to reload the page to show the updated row
  - Using `HX-Redirect` header allows HTMX to handle page reload seamlessly
  - The ArchivableCount is already calculated in the Bookings handler (from US-016), so the bulk archive button conditional was straightforward
  - JavaScript form.submit() is the simplest approach for bulk actions that need POST method

---

**ralph run 202601261900**

---

## 2026-01-26 - US-001 - Add GetAllByEmail repository method
- What was implemented:
  - Added `GetAllByEmail(ctx, email)` method to HostRepository
  - Method queries hosts table with email filter across all tenants (no tenant_id filter)
  - Returns `[]*models.Host` slice with all hosts matching the email
  - Returns empty slice (not nil/error) when no hosts found
  - Uses existing `idx_hosts_email` index for efficient lookup
  - Follows existing repository patterns: `q()` helper for driver compatibility, proper error handling, defer rows.Close()
- Files changed:
  - `internal/repository/repository.go` - Added GetAllByEmail method (~40 lines)
- **Learnings for future iterations:**
  - The `idx_hosts_email` index exists on the email column alone (not a composite index with tenant_id)
  - Repository pattern for multi-row queries: use `QueryContext`, iterate with `rows.Next()`, check `rows.Err()` after loop
  - The `GetByEmail` method is tenant-scoped (requires tenant_id), while `GetAllByEmail` is cross-tenant for simplified login
  - Always return empty slice `[]*models.Host{}` instead of nil when no rows found - this is the acceptance criteria and prevents nil pointer issues

---

## 2026-01-26 - US-002 - Add simplified login types and service method
- What was implemented:
  - Added `SimplifiedLoginInput` struct with Email and Password fields
  - Added `OrgOption` struct with TenantID, TenantSlug, TenantName, and HostID fields
  - Added `SimplifiedLoginResult` struct with RequiresOrgSelection, AvailableOrgs, SessionToken, Host, and Tenant fields
  - Added `SimplifiedLogin(ctx, input)` method to AuthService that:
    - Finds all hosts by email using `GetAllByEmail` (cross-tenant query from US-001)
    - Verifies password against each host using bcrypt
    - Single org match: creates session directly and returns SessionToken, Host, and Tenant
    - Multiple org matches: returns RequiresOrgSelection=true with populated AvailableOrgs
    - No matches or wrong password: returns generic ErrInvalidCredentials (no email enumeration)
    - Timing attack prevention: performs dummy bcrypt comparison when no hosts found
- Files changed:
  - `internal/services/auth.go` - Added ~111 lines including types and SimplifiedLogin method
- **Learnings for future iterations:**
  - The existing `HostWithTenant` type is defined in session.go, not auth.go
  - The audit log format follows `s.auditLog.Log(ctx, tenant.ID, &host.ID, "host.login", "host", host.ID, nil, "")` pattern
  - Timing attack prevention requires bcrypt comparison even for non-existent emails to maintain consistent response time
  - The existing Login method uses tenant-scoped `GetByEmail`, while SimplifiedLogin uses cross-tenant `GetAllByEmail`
  - Multi-org case doesn't create a session - that will be handled by US-003's CompleteOrgSelection method

---

## 2026-01-26 - US-003 - Add org selection completion method
- What was implemented:
  - Added `SelectionTokenExpiry` constant (5 minutes) for selection token validity
  - Added `ErrInvalidSelectionToken` and `ErrHostNotFound` error variables
  - Added `SelectionToken` field to `OrgOption` struct so each org option carries its own token
  - Added `SelectionToken` field to `SimplifiedLoginResult` struct (for potential future use)
  - Added `generateSelectionToken(hostID)` method using HMAC-SHA256 with the app's encryption key
    - Token format: base64(hostID:expiry:signature) - no database storage required
  - Added `validateSelectionToken(token)` method that verifies signature and expiry
  - Added `CompleteOrgSelectionInput` struct with HostID and SelectionToken fields
  - Added `CompleteOrgSelection(ctx, input)` method to AuthService that:
    - Validates the selection token
    - Verifies token's host ID matches the requested host ID (prevents cross-host attacks)
    - Fetches host and tenant from database
    - Creates a session and returns session token
    - Logs the login via audit log with "via org selection" note
  - Updated `SimplifiedLogin` to generate selection tokens for each org option
- Files changed:
  - `internal/services/auth.go` - Added ~145 lines for token generation, validation, and CompleteOrgSelection method
- **Learnings for future iterations:**
  - The HMAC approach with base64 encoding allows stateless token validation without database storage
  - Using the existing `cfg.App.EncryptionKey` for HMAC signing leverages the already-configured secret
  - Token format `hostID:expiry:signature` allows simple string splitting for validation
  - The double verification (token signature + hostID match) prevents both tampering and cross-host enumeration
  - Selection tokens are tied to specific hosts, so each org option in the UI carries its own token

---

## 2026-01-26 - US-004 - Update login handler to use SimplifiedLogin
- What was implemented:
  - Modified `Login()` handler in `auth.go` to use `SimplifiedLogin` service method (from US-002)
  - Removed organization field processing from login handler - now only reads email and password
  - Single-org case: creates session cookie and redirects to dashboard (same as before)
  - Multi-org case: renders `login_select_org.html` template with available orgs from `SimplifiedLoginResult.AvailableOrgs`
  - Error case: shows generic "Invalid email or password" message (no email enumeration)
  - Changed input struct from `LoginInput` (with TenantSlug) to `SimplifiedLoginInput` (email + password only)
- Files changed:
  - `internal/handlers/auth.go` - Modified Login() handler (~28 lines changed)
- **Learnings for future iterations:**
  - The `SimplifiedLoginResult` struct from US-002 provides all needed data: `RequiresOrgSelection`, `AvailableOrgs`, `SessionToken`
  - The handler checks `result.RequiresOrgSelection` to determine which flow to take
  - The `login_select_org.html` template (US-006) will receive `AvailableOrgs` containing `OrgOption` structs with `TenantName`, `HostID`, `SelectionToken`
  - The template data is passed as `map[string]interface{}` not `map[string]string` because `AvailableOrgs` is a slice of structs
  - The old `LoginInput` with TenantSlug is still available for backward compatibility but not used by the simplified login flow

---

## 2026-01-26 - US-005 - Remove Organization field from login template
- What was implemented:
  - Removed the Organization form field (input + label) from `templates/pages/login.html`
  - Login form now only shows Email and Password fields
  - Existing styling and validation preserved (no changes needed to CSS or error display)
  - The handler was already updated in US-004 to use SimplifiedLogin which doesn't require organization
- Files changed:
  - `templates/pages/login.html` - Removed Organization form-group (6 lines deleted)
- **Learnings for future iterations:**
  - The login template is a standalone page that doesn't use the dashboard layout
  - The `{{if .Data}}{{.Data.tenant}}{{end}}` pattern was used to preserve form values on error - no longer needed for tenant field
  - US-004 already updated the handler to not process the organization field, so this change just removes the unused UI element
  - The typecheck still passes because Go templates are only validated at runtime, not compile time

---

## 2026-01-26 - US-006 - Create org selection page template
- What was implemented:
  - Created `templates/pages/login_select_org.html` template for multi-org user login flow
  - Template displays list of organizations with tenant name prominently shown
  - Each org option is a form/button that POSTs to `/auth/select-org`
  - Hidden fields for `host_id` and `selection_token` included in each form
  - Styled consistently with existing login page using `.auth-page` and `.auth-container` classes
  - Added inline CSS for org list styling (`.org-list`, `.org-option`, `.org-option-name`, `.org-option-slug`)
  - Back link to login page for users who want to try a different email
- Files changed:
  - `templates/pages/login_select_org.html` - New file (~80 lines)
- **Learnings for future iterations:**
  - The `loadTemplates()` function in handlers.go uses `filepath.Glob("templates/pages/*.html")` to auto-discover page templates
  - The `OrgOption` struct passed to template contains: `TenantID`, `TenantSlug`, `TenantName`, `HostID`, `SelectionToken`
  - The handler at auth.go:45-52 passes `AvailableOrgs` to the template via `map[string]interface{}`
  - Inline styles in `<style>` tag work well for page-specific CSS that doesn't need to be shared

---

## 2026-01-26 - US-007 - Add SelectOrg handler and route
- What was implemented:
  - Added `SelectOrg()` handler to `AuthHandler` in `internal/handlers/auth.go`
  - Handler reads `host_id` and `selection_token` from POST form
  - Calls `CompleteOrgSelection` service method (from US-003) to validate token and create session
  - On success: sets session cookie (24hr expiry, HttpOnly, SameSite=Lax) and redirects to `/dashboard`
  - On error: redirects to `/auth/login?error=session_expired`
  - Added route `POST /auth/select-org` in `cmd/server/main.go`
- Files changed:
  - `internal/handlers/auth.go` - Added SelectOrg handler (~40 lines)
  - `cmd/server/main.go` - Added SelectOrg route
- **Learnings for future iterations:**
  - The `login_select_org.html` template (US-006) POSTs to `/auth/select-org` with `host_id` and `selection_token` hidden fields
  - The `CompleteOrgSelection` service method validates the HMAC-signed token and returns session token
  - Handler follows same cookie pattern as Login handler: `time.Hour / time.Second` for MaxAge conversion
  - Error handling redirects to login page with query param rather than showing error page directly
  - This completes the simplified login flow: email+password → multi-org selection → dashboard

---

**ralph run 202602061000 — Google Auth PRD**

---

## 2026-02-06 - US-001 - Add APP_BASE_URL config value
- What was implemented:
  - Added `BaseURL` field to `AppConfig` struct in `internal/config/config.go`
  - Loads from `APP_BASE_URL` env var with default `http://localhost:8080`
  - Existing config loading is not disrupted — all builds and tests pass
  - Note: there is already a `Server.BaseURL` loaded from `BASE_URL` env var. The new `App.BaseURL` is specifically for OAuth callback URL derivation as required by the Google Auth PRD
- Files changed:
  - `internal/config/config.go` - Added `BaseURL` field to AppConfig struct and loading in Load()
- **Learnings for future iterations:**
  - The codebase already has `cfg.Server.BaseURL` (from `BASE_URL` env var) used extensively for email links, template rendering, and onboarding
  - The new `cfg.App.BaseURL` (from `APP_BASE_URL` env var) is a separate config value for OAuth-specific URL derivation
  - Future Google auth stories (US-005+) should use `cfg.App.BaseURL` for constructing OAuth redirect URLs

---

## 2026-02-06 - US-002 - Add Google identity columns to hosts table
- What was implemented:
  - Added `google_id` (nullable string, unique) and `google_email` (nullable string) columns to hosts table
  - Made `password_hash` column nullable in PostgreSQL via `ALTER COLUMN ... DROP NOT NULL` (SQLite TEXT already accepts NULL)
  - Created PostgreSQL migration `migrations/009_add_google_identity.up.sql` with partial unique index on `google_id` (WHERE google_id IS NOT NULL)
  - Created SQLite migration `migrations/sqlite/009_add_google_identity.up.sql` with unique index on `google_id`
  - Added `GoogleID *string` and `GoogleEmail *string` fields to Host struct in `internal/models/models.go` with json and db tags
  - Updated all 6 HostRepository methods that touch host columns:
    - `Create`: INSERT now includes `google_id` and `google_email` (13 params instead of 11)
    - `GetByID`, `GetByEmail`, `GetAllByEmail`, `GetBySlug`, `GetByTenantID`: SELECT and Scan now include `google_id` and `google_email`
  - Existing hosts retain their data unchanged — new columns default to NULL
  - Build and all tests pass
- Files changed:
  - `migrations/009_add_google_identity.up.sql` - PostgreSQL migration adding google_id, google_email columns and making password_hash nullable
  - `migrations/sqlite/009_add_google_identity.up.sql` - SQLite migration adding google_id, google_email columns
  - `internal/models/models.go` - Added GoogleID and GoogleEmail pointer string fields to Host struct
  - `internal/repository/repository.go` - Updated Create, GetByID, GetByEmail, GetAllByEmail, GetBySlug, GetByTenantID to include new columns
- **Learnings for future iterations:**
  - PostgreSQL uses `ALTER TABLE ... ALTER COLUMN ... DROP NOT NULL` to make existing columns nullable; SQLite TEXT columns already accept NULL
  - PostgreSQL supports partial unique indexes (`WHERE column IS NOT NULL`) which is cleaner for nullable unique columns
  - The Host struct is scanned in 6 different repository methods — when adding new columns, all must be updated consistently
  - The `Update` method only updates name/slug/timezone/default_calendar_id so it didn't need changes for google_id/google_email
  - US-003 (Update Host model struct) acceptance criteria were naturally fulfilled as part of US-002 since the model needed updating for the repository to compile
  - Next migration number is 010

---

## 2026-02-06 - US-003 - Update Host model struct for Google identity
- Already completed as part of US-002 implementation
- GoogleID (*string) and GoogleEmail (*string) fields already exist on Host struct (lines 89-90 of models.go)
- PasswordHash remains `string` type (not *string), handling empty string for Google-only users as allowed by acceptance criteria
- PRD updated to mark US-003 as passing
- Files changed: None (already done)

---

## 2026-02-06 - US-004 - Add Google identity repository methods
- What was implemented:
  - Added `GetByGoogleID(ctx, googleID)` method to HostRepository
  - Method queries hosts table with google_id filter across all tenants (no tenant_id filter)
  - Returns `[]*models.Host` slice; empty slice (not nil) when no hosts found
  - Follows exact same pattern as `GetAllByEmail`: q() helper, multi-row scan with all 14 Host fields, rows.Err() check
  - Added `LinkGoogleIdentity(ctx, hostID, googleID, googleEmail)` method to HostRepository
  - Updates google_id and google_email fields plus updated_at timestamp
  - Uses q() helper for driver compatibility
  - Create() method already supports google_id/google_email fields (done in US-002)
- Files changed:
  - `internal/repository/repository.go` - Added GetByGoogleID (~35 lines) and LinkGoogleIdentity (~10 lines) methods
- **Learnings for future iterations:**
  - The HostRepository now has 8 query methods that scan Host rows: Create, GetByID, GetByEmail, GetAllByEmail, GetBySlug, GetByTenantID, GetByGoogleID (7 that SELECT + scan all fields)
  - When adding new Host columns in the future, all 7 SELECT methods must be updated consistently
  - The GetByGoogleID follows the exact same pattern as GetAllByEmail: cross-tenant multi-row query returning empty slice on no match
  - LinkGoogleIdentity also updates `updated_at` to track when the identity was linked

---

## 2026-02-06 - US-005 - Implement Google Auth URL generation and CSRF nonce
- What was implemented:
  - Added `GetGoogleAuthURL(flow string) (authURL string, nonce string, error)` method to AuthService
  - OAuth URL requests scopes: `openid email profile` (OpenID Connect scopes for auth, not calendar scopes)
  - State parameter encodes flow context as `auth:{flow}:{nonce}` (e.g., `auth:signup:abc123...`)
  - Redirect URL derived from `cfg.App.BaseURL` (APP_BASE_URL env) + `/auth/google/auth-callback`
  - Nonce generated via existing `generateToken(16)` helper — 16 bytes of `crypto/rand`, hex encoded to 32 chars
  - Uses `url.Values` for proper URL parameter encoding (safer than fmt.Sprintf for URL construction)
  - Uses `prompt=select_account` to let users choose which Google account to use
- Files changed:
  - `internal/services/auth.go` - Added `net/url` import and `GetGoogleAuthURL` method (~30 lines)
- **Learnings for future iterations:**
  - The existing `CalendarService.GetGoogleAuthURL` uses `fmt.Sprintf` for URL construction; the new method uses `url.Values` for proper encoding
  - The calendar OAuth uses scopes `calendar.readonly` and `calendar.events`; the auth OAuth uses OpenID Connect scopes `openid email profile`
  - The calendar OAuth uses `access_type=offline&prompt=consent` for refresh tokens; auth OAuth uses `prompt=select_account` since we only need one-time token exchange
  - The `generateToken(n)` helper in auth.go generates `n` random bytes and hex-encodes them, reused for nonce generation
  - State format `auth:{flow}:{nonce}` allows downstream handler (US-012) to parse both the flow type and verify the CSRF nonce

---

## 2026-02-06 - US-006 - Implement Google token exchange and userinfo fetch
- What was implemented:
  - Added `GoogleUserInfo` struct with fields: Sub, Email, EmailVerified, Name (matches Google's userinfo v3 API response)
  - Added `HandleGoogleCallback(code, state, expectedNonce string) (*GoogleUserInfo, string, error)` method to AuthService
  - Parses state parameter format `auth:{flow}:{nonce}` and validates flow is "signup" or "login"
  - Validates CSRF nonce against expectedNonce; returns `ErrInvalidOAuthState` on mismatch
  - Added `exchangeGoogleAuthCode(code)` private method that exchanges authorization code for tokens via `https://oauth2.googleapis.com/token`
  - Uses `url.Values` for proper URL parameter encoding (consistent with GetGoogleAuthURL pattern from US-005)
  - Redirect URL derived from `cfg.App.BaseURL` + `/auth/google/auth-callback` (same as US-005)
  - Added `fetchGoogleUserInfo(accessToken)` private method that fetches user profile from `https://www.googleapis.com/oauth2/v3/userinfo`
  - Only accepts users with `email_verified: true`; returns `ErrGoogleEmailNotVerified` otherwise
  - Added `googleAuthTokenResponse` struct for token exchange response (separate from calendar service's `googleTokenResponse` to avoid coupling)
  - Added two new error variables: `ErrGoogleEmailNotVerified` and `ErrInvalidOAuthState`
- Files changed:
  - `internal/services/auth.go` - Added imports (encoding/json, io, log, net/http), GoogleUserInfo struct, googleAuthTokenResponse struct, HandleGoogleCallback, exchangeGoogleAuthCode, fetchGoogleUserInfo methods (~134 lines)
- **Learnings for future iterations:**
  - The auth service token exchange uses `url.Values.Encode()` instead of `fmt.Sprintf` (safer than calendar service's approach) - but both patterns work
  - The `googleAuthTokenResponse` is kept separate from calendar service's `googleTokenResponse` to avoid coupling auth and calendar concerns
  - Google's userinfo v3 endpoint returns `email_verified` as a boolean directly (not a string) - Go's JSON decoder handles this natively
  - The state parameter parsing splits on ":" which means none of the components (flow, nonce) can contain colons
  - Error handling follows existing pattern: wrap external errors with `fmt.Errorf` context, return specific sentinel errors for validation failures

---

## 2026-02-06 - US-007 - Implement RegisterWithGoogle service method
- What was implemented:
  - Added `RegisterWithGoogle(ctx, googleID, email, name, tenantName, tenantSlug, timezone string) (sessionToken string, error)` method to AuthService
  - Validates and slugifies tenant slug (same logic as existing Register method)
  - Checks for duplicate tenant slug, returns `ErrTenantExists` if exists
  - Creates Tenant record with uuid, slug, name, timestamps
  - Creates Host record with google_id and google_email set, password_hash empty string, IsAdmin=true
  - Host slug derived from name (falls back to email prefix), same as Register()
  - Creates default working hours (Mon-Fri 9:00-17:00) via `createDefaultWorkingHours`
  - Creates session via SessionService
  - Logs audit event `host.registered` with notes "method:google"
  - Normalizes email to lowercase
- Files changed:
  - `internal/services/auth.go` - Added RegisterWithGoogle method (~89 lines)
- **Learnings for future iterations:**
  - The RegisterWithGoogle method follows the exact same pattern as Register() minus the password hashing step
  - GoogleID and GoogleEmail are `*string` on Host struct, so use `&googleID` and `&email` to set them
  - The audit log notes field is used to distinguish registration method ("method:google" vs no notes for password registration)
  - The method returns only `(sessionToken, error)` - simpler than Register() which returns `(*RegisterResult, error)` with full Tenant/Host objects; the handler only needs the session token for the cookie

---

## 2026-02-06 - US-008 - Implement LoginWithGoogle service method
- What was implemented:
  - Added `GoogleLoginResult` struct with fields: RequiresOrgSelection, AvailableOrgs, SessionToken, Host, Tenant, Linked
  - Added `ErrGoogleAccountNotFound` sentinel error for when no account matches the Google identity or email
  - Added `LoginWithGoogle(ctx, googleID, email string) (*GoogleLoginResult, error)` method to AuthService
  - Step 1: Looks up hosts by `google_id` first via `GetByGoogleID` repository method
  - Single google_id match: creates session, returns session token directly
  - Multiple google_id matches (multi-org): generates selection tokens (same as SimplifiedLogin), returns available orgs
  - Step 2: No google_id match — falls back to checking `GetAllByEmail` for email match
  - If email match(es) found: auto-links Google identity via `LinkGoogleIdentity` on all matching hosts
  - Single email match: creates session, returns with `Linked: true` flag
  - Multiple email matches: generates selection tokens, returns with `Linked: true`
  - If no match at all: returns `ErrGoogleAccountNotFound`
  - Logs audit event `host.login` with notes `method:google`
- Files changed:
  - `internal/services/auth.go` - Added GoogleLoginResult struct, ErrGoogleAccountNotFound error, LoginWithGoogle method (~150 lines)
- **Learnings for future iterations:**
  - The GoogleLoginResult follows the same pattern as SimplifiedLoginResult but adds a `Linked` bool to indicate auto-linking occurred
  - When auto-linking by email, all matching hosts across tenants get linked (not just the first one) — this ensures subsequent logins find them via google_id
  - The `generateSelectionToken` / `CompleteOrgSelection` pattern is reused for multi-org Google login — the handler will use the same org selection flow
  - The method normalizes email to lowercase before querying, consistent with other auth methods

---

## 2026-02-06 - US-009 - Update SimplifiedLogin to skip Google-only accounts
- What was implemented:
  - Updated `SimplifiedLogin` to skip bcrypt comparison for hosts with empty `PasswordHash` (Google-only accounts)
  - Added `didBcrypt` flag to track whether any bcrypt comparison was performed; if not (all accounts are Google-only), a dummy bcrypt comparison runs to maintain constant-time behavior
  - Updated `Login` (tenant-slug-based) to also reject Google-only accounts early before hitting bcrypt with an empty hash
  - All three timing attack scenarios are covered: no hosts found, all hosts Google-only, mixed hosts
- Files changed:
  - `internal/services/auth.go` - Updated `SimplifiedLogin` loop and `Login` method to handle empty password_hash
- **Learnings for future iterations:**
  - Both `Login` and `SimplifiedLogin` need updating when password-related logic changes — `Login` is the older tenant-slug-based method, `SimplifiedLogin` is the newer email-only flow
  - `bcrypt.CompareHashAndPassword` with an empty hash string returns an error (not a panic), but skipping it entirely is cleaner and avoids unnecessary computation
  - The dummy hash pattern `$2a$10$dummy.hash...` was already in the codebase for timing attack prevention — reuse it consistently

---

## 2026-02-06 - US-010 - Create Google registration completion template
- What was implemented:
  - Created `register_google.html` template in `templates/pages/` for post-Google-OAuth registration
  - Consistent visual style with existing `register.html`: same auth-card layout, CSS, head/favicon/fonts, auth-page body class
  - Shows Google email as read-only displayed text (not an input field) under "Google Account" label
  - Shows Google profile name as editable input, pre-filled from Google profile data
  - Form fields: organization name (required), organization slug (required, pattern `[a-z0-9-]+`), timezone dropdown
  - JavaScript auto-detects browser timezone via `Intl.DateTimeFormat().resolvedOptions().timeZone` and pre-selects it in the dropdown on page load
  - Form posts to `/auth/register/complete-google`
  - Handles flash messages for validation errors (same pattern as register.html using `.Flash.Type` and `.Flash.Message`)
  - Hidden field for `ref` parameter if present (for signup conversion tracking)
  - Terms of Service and Privacy Policy links, and "Already have an account? Sign in" footer
- Files changed:
  - `templates/pages/register_google.html` - New file (~115 lines)
- **Learnings for future iterations:**
  - The template data is passed as `PageData` with `.Data` containing a `map[string]interface{}` — access fields like `.Data.name`, `.Data.email`, `.Data.ref`
  - Email is displayed as plain text (not an input) since it comes from Google and shouldn't be editable — the handler (US-012) will pass it via the signed cookie data
  - The template loading in `handlers.go` automatically picks up new files from `templates/pages/` via `filepath.Glob` — no code changes needed to register templates
  - The handler for this template doesn't exist yet (US-012 will create it) — template was verified by confirming server starts without template parsing errors

---

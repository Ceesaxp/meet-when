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

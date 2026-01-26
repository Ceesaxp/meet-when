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

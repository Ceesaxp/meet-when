# PRD: Visual agenda for MeetWhen

**Status:** Draft, revised after review (2026-04-20)
**Owner:** Andrei
**Component:** `internal/services/calendar.go`, `internal/handlers/dashboard.go`, `templates/pages/dashboard_agenda.html`, calendar settings UI
**Estimated effort:** 1.5–2 days end-to-end (PR 1: day view + color; PR 2: week view)

## 1. Why

The agenda page currently renders all events from all subscribed calendars as a single chronological table. As soon as a host has more than one calendar (which is the default case for the target user — work, personal, partner-shared, board), three usability problems compound:

- **No calendar identity at a glance.** The calendar is a small text badge in the rightmost column. You have to read it to know whose meeting this is.
- **No sense of density.** "Am I free between 3 and 6?" requires reading every row and doing the math.
- **No sense of distribution across calendars.** "Which hat am I wearing most today?" is invisible.

The fix is to add visual structure that makes calendar membership and time density preattentive.

## 2. Goals & non-goals

### Goals

- Make calendar identity readable at a glance via consistent per-calendar color.
- Make a day's busy/free distribution scannable without reading event titles.
- Make a week's load visible on one screen, with drill-down to a single day.
- Ship within the existing Go + HTMX template stack — no SPA, no client-side calendar library.

### Non-goals (this iteration)

- Editing events from the agenda view.
- Drag-to-reschedule.
- All-day event row treatment beyond what already exists.
- Recurring event expansion improvements (separate ticket).
- Mobile-specific layout (degrade gracefully; full mobile pass later).

## 3. Working backwards: the end state

A host opens `/dashboard/agenda` and sees:

- **Today (default):** a horizontal time strip across the top with one row per calendar, events as colored blocks proportional to duration. Below the strip, the existing chronological list, with each row visually anchored by a colored left border matching the calendar color.
- **This week:** seven stacked day strips covering Mon–Sun (in the host's locale week start), each showing all of that day's events as overlaid colored blocks on a single 24h-bounded row. The currently selected day is highlighted; clicking another day swaps the detail list below the strip via HTMX, no full page reload.
- **Calendar settings (`/dashboard/calendars`):** each connected calendar has a color picker. Colors come from a fixed 9-swatch palette aligned with the app's design tokens. New calendars get a color auto-assigned from the palette by least-recently-used.

## 4. Domain model changes

### 4.1. `CalendarConnection.Color`

Add a `Color` field to `models.CalendarConnection`:

```go
type CalendarConnection struct {
    // ...existing fields...
    Color string `json:"color" db:"color"` // Hex, e.g. "#378ADD". Empty = unset.
}
```

**Storage:** `varchar(7)` in Postgres, `TEXT` in SQLite. Empty string is the "unset" sentinel; the service layer fills it on read.

**Migration:**

```sql
-- migrations/NNNN_add_calendar_color.up.sql
ALTER TABLE calendar_connections ADD COLUMN color VARCHAR(7) NOT NULL DEFAULT '';
```

```sql
-- migrations/NNNN_add_calendar_color.down.sql
ALTER TABLE calendar_connections DROP COLUMN color;
```

**Backfill:** Existing calendars with no color get one assigned via `AssignColors` on first read. New calendars get a color persisted at creation time (see §4.3). This ensures color stability without a data migration backfill.

### 4.2. `AgendaEvent` extensions

`AgendaEvent` currently carries `CalendarName` only. Matching events back to their calendar by name is brittle (names can collide, change, contain user input). Restore the entity link:

```go
type AgendaEvent struct {
    ID           string    `json:"id"`            // NEW: provider event ID, for keys/tooltips
    Title        string    `json:"title"`
    Start        time.Time `json:"start"`
    End          time.Time `json:"end"`
    CalendarID   string    `json:"calendar_id"`   // NEW: FK to CalendarConnection.ID
    CalendarName string    `json:"calendar_name"`
    CalendarColor string   `json:"calendar_color"` // NEW: resolved hex, never empty at this layer
    IsAllDay     bool      `json:"is_all_day"`
}
```

`getGoogleAgendaEvents` and `getCalDAVAgendaEvents` already have `cal *models.CalendarConnection` in scope — populate the new fields there. `CalendarColor` resolves through the auto-assignment helper (§4.3) so the template never has to handle an empty value.

### 4.3. Color palette and assignment

A fixed palette of 9 swatches that work in both light and dark mode:

```go
// internal/services/calendar_palette.go
package services

var CalendarPalette = []string{
    "#378ADD", // blue
    "#1D9E75", // teal
    "#D85A30", // coral
    "#7F77DD", // purple
    "#639922", // green
    "#BA7517", // amber
    "#D4537E", // pink
    "#E24B4A", // red
    "#5F5E5A", // gray
}

// AssignColors resolves Color for any calendar with an empty Color value.
// It fills unset slots in palette order, skipping colors already used by
// the host's other calendars.
//
// IMPORTANT: Input must be sorted by a stable, immutable key (created_at ASC)
// before calling. Do NOT sort by is_default or any mutable field — changing
// the default calendar would reshuffle all unset colors.
func AssignColors(calendars []*models.CalendarConnection) {
    // Sort by created_at to guarantee stable assignment regardless of
    // how the caller loaded the slice.
    slices.SortFunc(calendars, func(a, b *models.CalendarConnection) int {
        return a.CreatedAt.Compare(b.CreatedAt)
    })

    used := make(map[string]bool, len(calendars))
    for _, c := range calendars {
        if c.Color != "" {
            used[c.Color] = true
        }
    }
    paletteIdx := 0
    for _, c := range calendars {
        if c.Color != "" {
            continue
        }
        for paletteIdx < len(CalendarPalette) && used[CalendarPalette[paletteIdx]] {
            paletteIdx++
        }
        if paletteIdx >= len(CalendarPalette) {
            // More calendars than palette colors — wrap. Acceptable; user can override.
            paletteIdx = 0
        }
        c.Color = CalendarPalette[paletteIdx]
        used[c.Color] = true
        paletteIdx++
    }
}
```

**Assignment and persistence strategy:**

1. **At calendar creation:** `CalendarService.CreateConnection` calls `AssignColors` on the host's existing calendars + the new one, then persists the assigned color on the new row. This makes color stable from the start.
2. **On read (settings UI, agenda):** `AssignColors` is called to resolve any legacy calendars with empty colors. These are presentation defaults — not persisted until the user explicitly saves via settings.
3. **User override is authoritative:** When a user picks a color in settings, it is persisted and `AssignColors` skips that calendar on subsequent calls.

This ensures color stability without depending on query order at read time, while still allowing the auto-assignment to handle legacy data gracefully.

## 5. Service layer changes

### 5.1. New: `GetAgendaView`

The handler should not have to call multiple service methods and stitch the result. Add a single entry point that returns a fully-prepared view model:

```go
// internal/services/agenda.go
package services

type AgendaView struct {
    Calendars   []*models.CalendarConnection // all of host's calendars, with Color resolved
    Events      []AgendaEvent                // sorted, colored
    DayStart    time.Time                    // window start in host TZ
    DayEnd      time.Time                    // window end in host TZ
    HostTZ      *time.Location
}

type AgendaService struct {
    repos    *repository.Repositories
    calendar *CalendarService
}

func (s *AgendaService) GetDay(ctx context.Context, hostID string, date time.Time) (*AgendaView, error) { ... }
func (s *AgendaService) GetWeek(ctx context.Context, hostID string, weekStart time.Time) (*AgendaView, error) { ... }
```

Both methods:

1. Load host (for timezone).
2. Load calendars; call `AssignColors` to resolve.
3. Compute `DayStart`/`DayEnd` in host TZ (day: 00:00–24:00; week: 7×24h spanning Mon 00:00 → next Mon 00:00 by default, configurable later).
4. Call `CalendarService.GetAgendaEventsWithCalendars` (see §5.4), passing the preloaded calendars so they are not double-fetched. Events come back with `CalendarID`/`CalendarColor` already populated.
5. Sort events by start time using `slices.SortFunc` (replacing the current O(n²) bubble sort in `sortAgendaEvents`).

**Note:** `GetWeek` returns all 7 days of events in a single call. The week view template receives the full dataset and pre-renders all day-detail partials. Intra-week day switching uses client-side show/hide (or HTMX swap from pre-rendered hidden blocks), not a server round-trip. This avoids redundant provider API calls on each day click. The standalone `GET /dashboard/agenda/day-detail` endpoint is only used for direct navigation, not intra-week switching.

### 5.4. Refactor: `GetAgendaEventsWithCalendars`

Current `GetAgendaEvents` loads calendars internally and has `cal *models.CalendarConnection` in scope during provider iteration but does not propagate `CalendarID`/`CalendarColor` onto events. Refactor to:

```go
// GetAgendaEventsWithCalendars accepts preloaded calendars (with colors resolved)
// and populates CalendarID + CalendarColor on each returned AgendaEvent.
// This avoids double-loading calendars when called from AgendaService.
func (s *CalendarService) GetAgendaEventsWithCalendars(
    ctx context.Context,
    calendars []*models.CalendarConnection,
    host *models.Host,
    start, end time.Time,
) ([]AgendaEvent, error) { ... }
```

The existing `GetAgendaEvents` becomes a thin wrapper that loads calendars and delegates to this method, preserving backward compatibility.

### 5.2. Strip rendering helper

A pure function that converts events to positioned blocks for template consumption:

```go
// internal/services/agenda_strip.go
package services

type StripBlock struct {
    LeftPct  float64 // 0..100, position within the visible window
    WidthPct float64 // 0..100, clamped so LeftPct+WidthPct ≤ 100
    Color    string
    Title    string  // for HTML title attribute (tooltip)
    EventID  string  // for hx-target if we wire click later
}
// CSS uses: style="left: {{.LeftPct}}%; width: {{.WidthPct}}%; background: {{.Color}}"
// Do NOT use left+right positioning — keep the struct and CSS on the same model.

type CalendarLane struct {
    Calendar *models.CalendarConnection
    Blocks   []StripBlock
}

// LanesByCalendar returns one lane per calendar, with the calendar's events
// positioned within [windowStart, windowEnd]. Events outside the window are
// clipped. Used by the day view (4 lanes per day).
func LanesByCalendar(view *AgendaView) []CalendarLane { ... }

// FlatLane returns a single lane containing all events overlaid by color.
// Used by the week view (one lane per day).
func FlatLane(events []AgendaEvent, windowStart, windowEnd time.Time) []StripBlock { ... }
```

**Window choice for the strip:**

- **Day strip:** adaptive visible window. Default baseline: `09:00–18:00` host-local. Extend in both directions to fit events outside the baseline: `min(09:00, earliest_event - 30min) → max(18:00, latest_event + 30min)`. Also **contract** if all events fall within a narrower range (e.g., 10:00–15:00 → show 09:00–16:00). Display the strip's actual hour range as the column headers.
- **Week strip:** compute a **shared** visible window from the union of all events across all 7 days, using the same adaptive logic. This ensures columns align across days. Events that fall outside the shared window get an overflow indicator (small arrow at the strip edge).

**Multi-day / overnight events:** Strip generation operates on the intersection of each event with the day window. An event spanning 22:00–02:00 appears as a block from 22:00 to midnight on day 1 and midnight to 02:00 on day 2. Tests must cover: overnight events, all-day events (excluded from strip), and DST boundary transitions.

All-day events do not appear in the strip (they have no meaningful position). They appear in the detail list only, with the existing all-day badge.

### 5.3. Sort fix

In the same PR, replace `sortAgendaEvents` bubble sort with `slices.SortFunc` (preferred over `sort.Slice` since Go 1.21+ — avoids interface conversion overhead). Standalone, low-risk.

## 6. Handler & routing changes

Routes (using `net/http` 1.22+ patterns):

```
GET  /dashboard/agenda                  → day view, today
GET  /dashboard/agenda?date=2026-04-17  → day view, specific date
GET  /dashboard/agenda?view=week        → week view, current week
GET  /dashboard/agenda?view=week&week=2026-04-13 → week view, specific week
GET  /dashboard/agenda/day-detail?date=2026-04-17 → HTMX partial, day strip + list (standalone navigation only)
POST /dashboard/calendars/{id}/color    → update calendar color
```

```go
mux.HandleFunc("GET /dashboard/agenda", h.AgendaView)
mux.HandleFunc("GET /dashboard/agenda/day-detail", h.AgendaDayPartial)
mux.HandleFunc("POST /dashboard/calendars/{id}/color", h.UpdateCalendarColor)
```

**Week view day switching:** The week page pre-renders all 7 day-detail blocks server-side (hidden). Clicking a day strip swaps visibility client-side (CSS class toggle or HTMX `hx-swap` from a local `<template>` element). No server round-trip for intra-week navigation. The `day-detail` endpoint exists for standalone direct navigation only.

Handler responsibilities:

- Parse `view`, `date`, `week` from query string with safe defaults.
- Call `AgendaService.GetDay` or `GetWeek`.
- Pass `LanesByCalendar(view)` and the event list to the template.
- For `GetWeek`: pass all 7 days of `CalendarLane` + event data so the template can pre-render.
- For `AgendaDayPartial`: render a `{{define "day_detail"}}` block instead of the full page.

## 7. Template changes

### 7.1. New shared partials

```
templates/partials/calendar_legend.html   — color-dot + name list of calendars (not color-only — accessibility)
templates/partials/day_strip.html         — N-lane horizontal strip for one day (dynamic lane height)
templates/partials/day_detail.html        — colored list table + strip header
templates/partials/week_strip.html        — 7 stacked single-lane day strips with shared time axis
```

`day_detail.html` is the HTMX swap target — it's both used standalone and embedded in the full page.

**Accessibility note:** Color must not be the only identity channel. The legend includes calendar names. On wider strip blocks (>8% width), render a short label or initials inside the block. This matters for color blindness, palette collisions (>9 calendars), and low-contrast displays.

### 7.2. Updated `dashboard_agenda.html`

Replace the current single-table layout with:

**Day view:**
1. `{{template "day_strip" .}}` — the 4-calendar strip.
2. `{{template "day_detail" .}}` — the colored list.

**Week view:**
1. `{{template "week_strip" .}}` — 7 day-rows with shared time axis. Clicking a day toggles visibility of the corresponding pre-rendered detail block (no server round-trip).
2. All 7 day-detail blocks rendered inside hidden containers. The active day's container is shown; the rest are hidden via CSS class. A small inline script or HTMX `hx-on:click` toggles the active class.
3. Initially: today (or first day with events) is the active detail.

### 7.3. CSS additions

Add to `static/css/style.css`:

Add calendar color palette as CSS custom properties (the current design system is monochrome-accent and does not define these):

```css
:root {
  --cal-blue: #378ADD;
  --cal-teal: #1D9E75;
  --cal-coral: #D85A30;
  --cal-purple: #7F77DD;
  --cal-green: #639922;
  --cal-amber: #BA7517;
  --cal-pink: #D4537E;
  --cal-red: #E24B4A;
  --cal-gray: #5F5E5A;
}
```

Component classes:

- `.strip-lane` — `position: relative`, `border-radius: 4px`, `background: var(--gray-100)`. Height is **dynamic**: `8px` when the lane has no events (`.strip-lane--empty`), `22px` when it has events. Transition height for smooth visual.
- `.strip-block` — absolute-positioned block, `border-radius: 3px`, inline `style="left: {{.LeftPct}}%; width: {{.WidthPct}}%; background: {{.Color}}"`. Uses `left`+`width` positioning (matches `StripBlock` struct).
- `.strip-overflow` — small arrow indicator at lane edge for events clipped by the visible window.
- `.strip-header` — grid for hour labels.
- `.calendar-dot` — 8px circle, inline-flex.
- `.calendar-legend` — horizontal list of calendar-dot + name pairs. Always visible alongside the strip, not color-only (accessibility).
- `.event-row` — uses CSS custom property for the left border. Set `style="--cal-color: {{.CalendarColor}}"` on the row, then `border-left: 3px solid var(--cal-color)` in CSS.

No JS required for the visualization itself. HTMX handles day switching.

## 8. Calendar settings UI changes

In the existing `/dashboard/calendars` page, each calendar row gets a color picker. Implementation: a row of 9 clickable radio-button circles with visible color names. This is more reliable cross-browser than a styled `<select>` (native `<option>` styling is ignored in Safari/Firefox).

```html
<form hx-post="/dashboard/calendars/{{.ID}}/color" hx-swap="outerHTML">
  <fieldset class="color-swatches">
    {{range .Palette}}
    <label class="color-swatch" title="{{.Name}}">
      <input type="radio" name="color" value="{{.Hex}}"
             {{if eq .Hex $.Color}}checked{{end}}
             onchange="this.form.requestSubmit()">
      <span class="swatch-circle" style="background: {{.Hex}}"></span>
      <span class="swatch-name">{{.Name}}</span>
    </label>
    {{end}}
  </fieldset>
</form>
```

New handler: `POST /dashboard/calendars/{id}/color` (POST, not PATCH — consistent with existing dashboard routes and supports progressive fallback). Validates the color is in the palette (reject arbitrary hex — keeps the visual system clean), updates the row via a dedicated `CalendarRepository.UpdateColor(ctx, hostID, calendarID, color)` method (separate from token-refresh update paths), returns the swatch HTML.

**Authorization:** Handler must verify the calendar belongs to the authenticated host (tenant isolation).

## 9. Edge cases & open questions

| Case | Decision |
|---|---|
| More calendars than palette colors (>9) | Palette wraps. Two calendars can share a color. User can manually pick to disambiguate. Acceptable trade-off; a host with 10+ calendars is an outlier. |
| Two calendars assigned the same color manually | Allowed. We don't enforce uniqueness — user's choice wins. |
| Event spans midnight | Clip to day window via event/window intersection. A 22:00–02:00 event renders as two blocks: 22:00–00:00 on day 1, 00:00–02:00 on day 2. Full duration in the list. Tests required for overnight, all-day, and DST boundary cases. |
| All-day events | Skip in strip. Show in list with existing badge. |
| Week starts on Sunday vs Monday | v1: hardcode Monday. Add a host preference in v2 (`Host.WeekStart`). |
| Empty day in week view | Render the empty lane row. Visual quiet is information. |
| Calendar deleted while user is on the page | Stale event references won't render colored (no matching calendar). Service layer falls back to neutral gray. Logged as a warning. |
| User switches days rapidly in week view | No provider calls — day details are pre-rendered server-side from the initial `GetWeek` data. Switching is client-side visibility toggle only. |

**Resolved questions:**

1. **Inherit from upstream calendar color?** Deferred to v2. Mixing Google's aggressive primaries and iCloud's pastels with our curated palette would look inconsistent.
2. **Click-through from event block to source calendar?** Deferred. Not in this iteration.
3. **Density tolerance / overlapping events in day strip?** Accept the visual merge for v1. The "you're slammed" signal is useful information. Stacking adds layout complexity (overlap detection, column packing) for a corner case — revisit if users report confusion.
4. **Dynamic lane height for empty calendar lanes?** Yes — collapse empty lanes to ~8px, expand to 22px when events are present. Keeps the strip compact for hosts with many calendars.

## 10. Acceptance criteria

### PR 1 (day view + color infrastructure)

- [ ] `CalendarConnection` has a persisted `Color` field; migration applies cleanly forward and back on both SQLite and Postgres.
- [ ] New calendars get a color assigned and persisted at creation time.
- [ ] `AssignColors` sorts input by `created_at` (immutable), produces stable non-colliding colors for ≤9 calendars, wraps for >9.
- [ ] `CalendarRepository.UpdateColor(ctx, hostID, calendarID, color)` exists as a dedicated method, separate from token-refresh update paths.
- [ ] `GetAgendaEventsWithCalendars` accepts preloaded calendars, populates `CalendarID`/`CalendarColor` on each event inline (no double-load).
- [ ] `AgendaEvent` carries `CalendarID` and `CalendarColor`, populated correctly for both Google and CalDAV providers.
- [ ] Day view shows an N-lane strip on top (dynamic lane height: 8px empty, 22px with events) and a color-anchored list below.
- [ ] Calendar legend shows color dot + calendar name (not color-only).
- [ ] Calendar settings page has a working radio-swatch color picker (not `<select>`) that persists and reflects in the agenda immediately.
- [ ] Color picker endpoint validates calendar ownership (tenant isolation).
- [ ] Calendar color palette defined as CSS custom properties in the design system.
- [ ] `StripBlock` uses `LeftPct`+`WidthPct` matching the CSS `left`+`width` positioning model.
- [ ] Strip generation operates on event/window intersection (overnight events split correctly across days).
- [ ] `sortAgendaEvents` uses `slices.SortFunc`.
- [ ] Tests for: palette assignment stability, strip block positioning, overnight event splitting, DST boundary, all-day exclusion.
- [ ] All new code uses `log/slog` (per project conventions).
- [ ] No regression: existing list rendering still works for users with one calendar.

### PR 2 (week view)

- [ ] Week view shows 7 stacked day strips with shared time axis and overflow indicators.
- [ ] Week page pre-renders all 7 day-detail blocks; day switching is client-side (no provider round-trip).
- [ ] `GetWeek` returns all 7 days of events in a single provider call.

## 11. Rollout

Two PRs (see §10 acceptance criteria for split). Both are purely visual/additive. No flag needed — if the migration applies and the templates render, it's live. Worst case for any one user: their colors are all default-assigned until they visit settings, which is fine.

**Mobile graceful degradation (both PRs):** Mobile-specific layout is out of scope, but the strip must not become unusable. Minimum requirements:
- Horizontal scroll on the strip container when viewport is narrow.
- Minimum block width of 4px (so thin events remain tappable).
- Calendar legend wraps naturally.
- No clipped text on event rows.

## 12. Out of scope (future work)

- Drag-to-reschedule.
- Inline event editing.
- Calendar color sync from upstream (Google/iCloud) — deferred to v2.
- Click-to-open event in source provider — deferred.
- Configurable week start, configurable strip hour window.
- Mobile-optimized layout (graceful degradation is in scope; dedicated mobile pass is not).
- Caching of `GetAgendaEvents` results.
- Recurring event expansion fixes in `getGoogleAgendaEvents`.
- Stacked collision rows for overlapping events in strips — revisit if users report confusion.

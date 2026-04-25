package services

import (
	"fmt"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// StripBlock represents a single event positioned within the visible day strip.
type StripBlock struct {
	LeftPct       float64 // percentage from the left edge of the visible window
	WidthPct      float64 // percentage of the visible window width
	Color         string
	Title         string
	EventID       string
	OverflowLeft  bool // event's original start is before windowStart
	OverflowRight bool // event's original end is after windowEnd
}

// CalendarLane holds positioned blocks for one calendar within the day strip.
type CalendarLane struct {
	Calendar *models.CalendarConnection
	Blocks   []StripBlock
	Empty    bool
}

// ComputeVisibleWindow returns the start and end of the time range that should
// be rendered in the day strip. The baseline is 09:00–18:00 in the timezone
// embedded in dayStart. The window extends earlier by 30 minutes before the
// earliest non-all-day event, and later by 30 minutes after the latest one.
// When all events fit within a narrower range the window contracts accordingly.
// The window is always clamped to [dayStart, dayEnd].
func ComputeVisibleWindow(events []AgendaEvent, dayStart, dayEnd time.Time) (time.Time, time.Time) {
	const padding = 30 * time.Minute

	loc := dayStart.Location()
	y, m, d := dayStart.Date()
	winStart := time.Date(y, m, d, 9, 0, 0, 0, loc)
	winEnd := time.Date(y, m, d, 18, 0, 0, 0, loc)

	var minStart, maxEnd time.Time
	hasTimedEvent := false
	for _, e := range events {
		if e.IsAllDay {
			continue
		}
		if !hasTimedEvent || e.Start.Before(minStart) {
			minStart = e.Start
		}
		if !hasTimedEvent || e.End.After(maxEnd) {
			maxEnd = e.End
		}
		hasTimedEvent = true
	}

	if hasTimedEvent {
		if padded := minStart.Add(-padding); padded.Before(winStart) {
			winStart = padded
		}
		// Contract or extend: always use maxEnd + padding (replaces 18:00 baseline
		// so the window shrinks when events end early, or grows when they end late).
		winEnd = maxEnd.Add(padding)
	}

	// Clamp to the full day boundary.
	if winStart.Before(dayStart) {
		winStart = dayStart
	}
	if winEnd.After(dayEnd) {
		winEnd = dayEnd
	}

	return winStart, winEnd
}

// HourLabel represents a single hour marker for the strip header.
type HourLabel struct {
	Label   string  // e.g. "9am", "12pm", "5pm"
	LeftPct float64 // percentage position within the visible window
}

// GenerateHourLabels creates hour markers for the strip header, positioned
// as percentages within the [windowStart, windowEnd] range.
func GenerateHourLabels(windowStart, windowEnd time.Time) []HourLabel {
	winDuration := windowEnd.Sub(windowStart).Seconds()
	if winDuration <= 0 {
		return nil
	}

	loc := windowStart.Location()
	y, m, d := windowStart.Date()

	// Start from the first full hour at or after windowStart.
	startHour := windowStart.Hour()
	if windowStart.Minute() > 0 || windowStart.Second() > 0 {
		startHour++
	}
	endHour := windowEnd.Hour()

	labels := make([]HourLabel, 0, endHour-startHour+1)
	for h := startHour; h <= endHour; h++ {
		t := time.Date(y, m, d, h, 0, 0, 0, loc)
		if t.Before(windowStart) || t.After(windowEnd) {
			continue
		}

		var label string
		switch {
		case h == 0:
			label = "12am"
		case h < 12:
			label = formatHour(h, "am")
		case h == 12:
			label = "12pm"
		default:
			label = formatHour(h-12, "pm")
		}

		leftPct := t.Sub(windowStart).Seconds() / winDuration * 100
		labels = append(labels, HourLabel{Label: label, LeftPct: leftPct})
	}
	return labels
}

func formatHour(h int, suffix string) string {
	return fmt.Sprintf("%d%s", h, suffix)
}

// LanesByCalendar converts an AgendaView into one CalendarLane per calendar.
// Each lane contains StripBlocks positioned as percentages within the visible
// window computed by ComputeVisibleWindow. All-day events are excluded.
// Events that extend beyond the day window are clipped to the day boundary
// before percentage conversion.
func LanesByCalendar(view *AgendaView) []CalendarLane {
	winStart, winEnd := ComputeVisibleWindow(view.Events, view.DayStart, view.DayEnd)
	winDuration := winEnd.Sub(winStart).Seconds()

	// Group events by CalendarID.
	eventsByCalendar := make(map[string][]AgendaEvent, len(view.Calendars))
	for _, e := range view.Events {
		if e.IsAllDay {
			continue
		}
		eventsByCalendar[e.CalendarID] = append(eventsByCalendar[e.CalendarID], e)
	}

	lanes := make([]CalendarLane, 0, len(view.Calendars))
	for _, cal := range view.Calendars {
		calEvents := eventsByCalendar[cal.ID]
		if len(calEvents) == 0 {
			lanes = append(lanes, CalendarLane{Calendar: cal, Empty: true})
			continue
		}

		blocks := make([]StripBlock, 0, len(calEvents))
		for _, e := range calEvents {
			// Clip to day window.
			eStart := e.Start
			eEnd := e.End
			if eStart.Before(view.DayStart) {
				eStart = view.DayStart
			}
			if eEnd.After(view.DayEnd) {
				eEnd = view.DayEnd
			}
			if !eEnd.After(eStart) {
				continue
			}

			// Skip events entirely outside the visible window.
			if !eEnd.After(winStart) || !eStart.Before(winEnd) {
				continue
			}

			// Further clip to visible window.
			if eStart.Before(winStart) {
				eStart = winStart
			}
			if eEnd.After(winEnd) {
				eEnd = winEnd
			}

			leftPct := eStart.Sub(winStart).Seconds() / winDuration * 100
			widthPct := eEnd.Sub(eStart).Seconds() / winDuration * 100

			// Clamp so the block does not overflow the right edge.
			if leftPct+widthPct > 100 {
				widthPct = 100 - leftPct
			}
			if widthPct < 0 {
				widthPct = 0
			}

			color := e.CalendarColor
			if color == "" {
				color = cal.Color
			}
			if color == "" {
				color = "#5F5E5A"
			}

			blocks = append(blocks, StripBlock{
				LeftPct:       leftPct,
				WidthPct:      widthPct,
				Color:         color,
				Title:         e.Title,
				EventID:       e.ID,
				OverflowLeft:  e.Start.Before(winStart),
				OverflowRight: e.End.After(winEnd),
			})
		}

		lanes = append(lanes, CalendarLane{
			Calendar: cal,
			Blocks:   blocks,
			Empty:    len(blocks) == 0,
		})
	}

	return lanes
}

// ComputeSharedWindow returns a single visible time window expressed on day 0
// (Monday) that covers all non-all-day events across all 7 days. It applies
// the same adaptive logic as ComputeVisibleWindow: baseline 09:00–18:00,
// extended 30 minutes before the earliest start and after the latest end.
//
// Critically, the window is derived from time-of-day only — each event's
// start/end is clipped to its own day boundary, then converted to an offset
// from that day's midnight. This ensures the returned window is a pure
// time-of-day range: e.g. 07:30–19:30 on day 0. Callers (BuildWeekDayViews)
// shift this to each day's own coordinates before calling FlatLane, so every
// day row aligns with the shared hour-label header.
func ComputeSharedWindow(days [7]DayEvents) (time.Time, time.Time) {
	const padding = 30 * time.Minute

	// Express baseline on day 0.
	loc := days[0].DayStart.Location()
	y, m, d := days[0].DayStart.Date()
	winStart := time.Date(y, m, d, 9, 0, 0, 0, loc)
	winEnd := time.Date(y, m, d, 18, 0, 0, 0, loc)

	// Track min/max as time-of-day durations from midnight.
	var minTOD, maxTOD time.Duration
	hasTimedEvent := false
	for _, day := range days {
		for _, e := range day.Events {
			if e.IsAllDay {
				continue
			}
			// Clip event to this day's boundaries before measuring TOD so that
			// overnight events (e.g. 22:00 Mon → 02:00 Tue) contribute at most
			// 00:00–24:00 on each day they appear in.
			eStart := e.Start
			eEnd := e.End
			if eStart.Before(day.DayStart) {
				eStart = day.DayStart
			}
			if eEnd.After(day.DayEnd) {
				eEnd = day.DayEnd
			}
			if !eEnd.After(eStart) {
				continue
			}
			startTOD := eStart.Sub(day.DayStart)
			endTOD := eEnd.Sub(day.DayStart)

			if !hasTimedEvent || startTOD < minTOD {
				minTOD = startTOD
			}
			if !hasTimedEvent || endTOD > maxTOD {
				maxTOD = endTOD
			}
			hasTimedEvent = true
		}
	}

	if hasTimedEvent {
		if padded := days[0].DayStart.Add(minTOD - padding); padded.Before(winStart) {
			winStart = padded
		}
		winEnd = days[0].DayStart.Add(maxTOD + padding)
	}

	// Clamp to [days[0].DayStart, days[0].DayEnd] — one calendar day.
	if winStart.Before(days[0].DayStart) {
		winStart = days[0].DayStart
	}
	if winEnd.After(days[0].DayEnd) {
		winEnd = days[0].DayEnd
	}

	return winStart, winEnd
}

// FlatLane converts a slice of AgendaEvents into a single flat lane of
// StripBlocks positioned within [windowStart, windowEnd]. All-day events are
// excluded. Events from all calendars are overlaid into one lane (unlike
// LanesByCalendar which separates by calendar). Each block is clipped to the
// window boundary; events that extend beyond the window get OverflowLeft or
// OverflowRight set to true so templates can render edge arrow indicators.
func FlatLane(events []AgendaEvent, windowStart, windowEnd time.Time) []StripBlock {
	winDuration := windowEnd.Sub(windowStart).Seconds()
	if winDuration <= 0 {
		return nil
	}

	blocks := make([]StripBlock, 0, len(events))
	for _, e := range events {
		if e.IsAllDay {
			continue
		}

		// Determine overflow before clipping.
		overflowLeft := e.Start.Before(windowStart)
		overflowRight := e.End.After(windowEnd)

		// Clip to window boundary.
		eStart := e.Start
		eEnd := e.End
		if eStart.Before(windowStart) {
			eStart = windowStart
		}
		if eEnd.After(windowEnd) {
			eEnd = windowEnd
		}
		if !eEnd.After(eStart) {
			continue
		}

		leftPct := eStart.Sub(windowStart).Seconds() / winDuration * 100
		widthPct := eEnd.Sub(eStart).Seconds() / winDuration * 100

		// Clamp so the block does not overflow the right edge due to float arithmetic.
		if leftPct+widthPct > 100 {
			widthPct = 100 - leftPct
		}
		if widthPct < 0 {
			widthPct = 0
		}

		color := e.CalendarColor
		if color == "" {
			color = "#5F5E5A"
		}

		blocks = append(blocks, StripBlock{
			LeftPct:       leftPct,
			WidthPct:      widthPct,
			Color:         color,
			Title:         e.Title,
			EventID:       e.ID,
			OverflowLeft:  overflowLeft,
			OverflowRight: overflowRight,
		})
	}
	return blocks
}

// WeekDayView bundles all per-day data needed to render one row of the week
// strip and its corresponding detail panel.
type WeekDayView struct {
	Date              time.Time
	DayName           string // e.g. "Mon"
	DateFormatted     string // e.g. "Apr 13"
	FullDateFormatted string // e.g. "Monday, April 13"
	Blocks            []StripBlock
	Events            []AgendaEvent // original unclipped events for the detail list
	EventCount        int
	IsToday           bool
	IsActive          bool
}

// BuildWeekDayViews produces 7 WeekDayViews for the given week. Blocks are
// positioned within the shared time-of-day window via FlatLane. Events are the
// original unclipped events from DayEvents for use in the detail panel.
// IsToday is set for the day whose calendar date matches today. IsActive is set
// for today (or for the first day that has events when today has none).
//
// sharedWindowStart and sharedWindowEnd are expressed on day 0 (Monday). They
// represent a time-of-day range (e.g. Monday 07:30–Monday 19:30). For each day
// row, the window is shifted to that day's own coordinates before calling
// FlatLane, ensuring every row's blocks align with the shared hour-label header.
func BuildWeekDayViews(week *WeekView, sharedWindowStart, sharedWindowEnd time.Time, today time.Time) []WeekDayView {
	todayLocal := today.In(week.HostTZ)
	todayY, todayM, todayD := todayLocal.Date()

	// sharedWindowStart/End are on day 0. Compute offsets from day 0's midnight
	// so we can re-anchor the window onto each day.
	day0Start := week.Days[0].DayStart
	windowStartOffset := sharedWindowStart.Sub(day0Start)
	windowEndOffset := sharedWindowEnd.Sub(day0Start)

	views := make([]WeekDayView, 7)
	activeFallback := -1 // index of first day with events

	for i, day := range week.Days {
		dayLocal := day.DayStart.In(week.HostTZ)
		y, m, d := dayLocal.Date()
		isToday := y == todayY && m == todayM && d == todayD

		// Shift the shared window to this day's coordinates.
		dayWindowStart := day.DayStart.Add(windowStartOffset)
		dayWindowEnd := day.DayStart.Add(windowEndOffset)

		views[i] = WeekDayView{
			Date:              day.DayStart,
			DayName:           dayLocal.Format("Mon"),
			DateFormatted:     dayLocal.Format("Jan 2"),
			FullDateFormatted: dayLocal.Format("Monday, January 2"),
			Blocks:            FlatLane(day.Events, dayWindowStart, dayWindowEnd),
			Events:            day.Events,
			EventCount:        len(day.Events),
			IsToday:           isToday,
		}

		if activeFallback == -1 && len(day.Events) > 0 {
			activeFallback = i
		}
	}

	// Set IsActive: today if today is in this week, else first day with events,
	// else Monday (index 0).
	activeSet := false
	for i := range views {
		if views[i].IsToday {
			views[i].IsActive = true
			activeSet = true
			break
		}
	}
	if !activeSet {
		idx := activeFallback
		if idx == -1 {
			idx = 0
		}
		views[idx].IsActive = true
	}

	return views
}

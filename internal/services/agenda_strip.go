package services

import (
	"fmt"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// StripBlock represents a single event positioned within the visible day strip.
type StripBlock struct {
	LeftPct float64 // percentage from the left edge of the visible window
	WidthPct float64 // percentage of the visible window width
	Color   string
	Title   string
	EventID string
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
				LeftPct:  leftPct,
				WidthPct: widthPct,
				Color:    color,
				Title:    e.Title,
				EventID:  e.ID,
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

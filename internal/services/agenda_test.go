package services

import (
	"testing"
	"time"
)

// buildWeekBuckets creates a [7]DayEvents rooted at monday in loc, matching
// the same AddDate arithmetic used in GetWeek.
func buildWeekBuckets(monday time.Time) [7]DayEvents {
	var days [7]DayEvents
	for i := range 7 {
		days[i] = DayEvents{
			DayStart: monday.AddDate(0, 0, i),
			DayEnd:   monday.AddDate(0, 0, i+1),
		}
	}
	return days
}

// ─── assignEventsToBuckets ───────────────────────────────────────────────────

// TestAssignEventsToBuckets_OvernightEventAppearsInBothDays verifies that an
// event starting 22:00 Monday and ending 02:00 Tuesday is assigned to BOTH
// days with its original Start/End preserved (not clipped).
func TestAssignEventsToBuckets_OvernightEventAppearsInBothDays(t *testing.T) {
	loc := time.UTC
	monday := time.Date(2026, 4, 20, 0, 0, 0, 0, loc)
	days := buildWeekBuckets(monday)

	evtStart := time.Date(2026, 4, 20, 22, 0, 0, 0, loc)
	evtEnd := time.Date(2026, 4, 21, 2, 0, 0, 0, loc)
	evt := AgendaEvent{
		ID:    "e1",
		Title: "Overnight",
		Start: evtStart,
		End:   evtEnd,
	}

	assignEventsToBuckets([]AgendaEvent{evt}, &days, loc)

	if len(days[0].Events) != 1 {
		t.Errorf("Monday (days[0]): want 1 event, got %d", len(days[0].Events))
	}
	if len(days[1].Events) != 1 {
		t.Errorf("Tuesday (days[1]): want 1 event, got %d", len(days[1].Events))
	}

	// Original Start/End must be preserved (not clipped to day boundary).
	for _, dayName := range []string{"Monday", "Tuesday"} {
		var events []AgendaEvent
		switch dayName {
		case "Monday":
			events = days[0].Events
		case "Tuesday":
			events = days[1].Events
		}
		if len(events) == 0 {
			continue
		}
		e := events[0]
		if !e.Start.Equal(evtStart) {
			t.Errorf("%s: Start clipped: want %v, got %v", dayName, evtStart, e.Start)
		}
		if !e.End.Equal(evtEnd) {
			t.Errorf("%s: End clipped: want %v, got %v", dayName, evtEnd, e.End)
		}
	}

	// Wednesday and beyond must be empty.
	for i := 2; i < 7; i++ {
		if len(days[i].Events) != 0 {
			t.Errorf("days[%d]: want 0 events, got %d", i, len(days[i].Events))
		}
	}
}

// TestAssignEventsToBuckets_AllDayEventInCorrectDay verifies that an all-day
// event stored as UTC midnight appears in the correct local day for a host
// west of UTC (America/Los_Angeles).
func TestAssignEventsToBuckets_AllDayEventInCorrectDay(t *testing.T) {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Skip("America/Los_Angeles not available:", err)
	}

	// Monday 2026-04-20 00:00 local
	monday := time.Date(2026, 4, 20, 0, 0, 0, 0, loc)
	days := buildWeekBuckets(monday)

	// Tuesday 2026-04-21 all-day event, stored as UTC midnight (as providers do).
	// In LA (UTC-8) this UTC midnight is actually Monday April 20 evening;
	// without the fix it would be bucketed into Monday.
	evtStart := time.Date(2026, 4, 21, 0, 0, 0, 0, time.UTC) // "2026-04-21" UTC midnight
	evtEnd := time.Date(2026, 4, 22, 0, 0, 0, 0, time.UTC)
	evt := AgendaEvent{
		ID:       "e1",
		Title:    "Tuesday All-Day",
		Start:    evtStart,
		End:      evtEnd,
		IsAllDay: true,
	}

	assignEventsToBuckets([]AgendaEvent{evt}, &days, loc)

	if len(days[0].Events) != 0 {
		t.Errorf("Monday (days[0]): want 0 events (UTC-midnight mis-bucketing fix), got %d", len(days[0].Events))
	}
	if len(days[1].Events) != 1 {
		t.Errorf("Tuesday (days[1]): want 1 event, got %d", len(days[1].Events))
	}
}

package services

import (
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// helpers

func newEvent(calID, calColor string, start, end time.Time, allDay bool) AgendaEvent {
	return AgendaEvent{
		ID:            "evt-" + calID,
		CalendarID:    calID,
		CalendarColor: calColor,
		Title:         "Test Event",
		Start:         start,
		End:           end,
		IsAllDay:      allDay,
	}
}

func newCal(id, color string) *models.CalendarConnection {
	return &models.CalendarConnection{ID: id, Name: id, Color: color}
}

func localTime(loc *time.Location, hour, minute int) time.Time {
	return time.Date(2024, 3, 15, hour, minute, 0, 0, loc)
}

func dayBounds(loc *time.Location) (time.Time, time.Time) {
	start := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	return start, start.Add(24 * time.Hour)
}

// ─── ComputeVisibleWindow ────────────────────────────────────────────────────

// TestComputeVisibleWindow_NoEvents verifies the baseline 09:00-18:00 window is
// returned when there are no events.
func TestComputeVisibleWindow_NoEvents(t *testing.T) {
	dayStart, dayEnd := dayBounds(time.UTC)
	winStart, winEnd := ComputeVisibleWindow(nil, dayStart, dayEnd)

	want09 := localTime(time.UTC, 9, 0)
	want18 := localTime(time.UTC, 18, 0)

	if !winStart.Equal(want09) {
		t.Errorf("winStart: want %v, got %v", want09, winStart)
	}
	if !winEnd.Equal(want18) {
		t.Errorf("winEnd: want %v, got %v", want18, winEnd)
	}
}

// TestComputeVisibleWindow_EarlyEvent verifies the window extends 30min before an
// event that starts before 09:00.
func TestComputeVisibleWindow_EarlyEvent(t *testing.T) {
	dayStart, dayEnd := dayBounds(time.UTC)
	events := []AgendaEvent{
		newEvent("c1", "#378ADD", localTime(time.UTC, 7, 0), localTime(time.UTC, 8, 0), false),
	}
	winStart, _ := ComputeVisibleWindow(events, dayStart, dayEnd)

	want := localTime(time.UTC, 6, 30) // 07:00 - 30min
	if !winStart.Equal(want) {
		t.Errorf("winStart: want %v, got %v", want, winStart)
	}
}

// TestComputeVisibleWindow_LateEvent verifies the window extends 30min after an
// event that ends after 18:00.
func TestComputeVisibleWindow_LateEvent(t *testing.T) {
	dayStart, dayEnd := dayBounds(time.UTC)
	// Event ending at 20:00 → winEnd should be 20:30
	events := []AgendaEvent{
		newEvent("c1", "#378ADD", localTime(time.UTC, 19, 0), localTime(time.UTC, 20, 0), false),
	}
	_, winEnd := ComputeVisibleWindow(events, dayStart, dayEnd)

	want := localTime(time.UTC, 20, 30) // 20:00 + 30min
	if !winEnd.Equal(want) {
		t.Errorf("winEnd: want %v, got %v", want, winEnd)
	}
}

// TestComputeVisibleWindow_ContractsWhenEventsEndEarly verifies the window end
// contracts below 18:00 when all events finish well before that.
func TestComputeVisibleWindow_ContractsWhenEventsEndEarly(t *testing.T) {
	dayStart, dayEnd := dayBounds(time.UTC)
	// Events entirely within 10:00-14:00 → winEnd = 14:30, well below 18:00.
	events := []AgendaEvent{
		newEvent("c1", "#378ADD", localTime(time.UTC, 10, 0), localTime(time.UTC, 12, 0), false),
		newEvent("c1", "#378ADD", localTime(time.UTC, 12, 30), localTime(time.UTC, 14, 0), false),
	}
	winStart, winEnd := ComputeVisibleWindow(events, dayStart, dayEnd)

	want09 := localTime(time.UTC, 9, 0)  // start does not contract past 09:00
	want14h30 := localTime(time.UTC, 14, 30) // 14:00 + 30min

	if !winStart.Equal(want09) {
		t.Errorf("winStart: want %v, got %v", want09, winStart)
	}
	if !winEnd.Equal(want14h30) {
		t.Errorf("winEnd: want %v (contracted), got %v", want14h30, winEnd)
	}
	// Verify contraction: winEnd must be before 18:00
	if !winEnd.Before(localTime(time.UTC, 18, 0)) {
		t.Errorf("expected contraction: winEnd %v should be before 18:00", winEnd)
	}
}

// TestComputeVisibleWindow_AllDayEventsIgnored verifies that all-day events do
// not affect the visible window.
func TestComputeVisibleWindow_AllDayEventsIgnored(t *testing.T) {
	dayStart, dayEnd := dayBounds(time.UTC)
	events := []AgendaEvent{
		newEvent("c1", "#378ADD", localTime(time.UTC, 6, 0), localTime(time.UTC, 7, 0), true),
	}
	winStart, winEnd := ComputeVisibleWindow(events, dayStart, dayEnd)

	want09 := localTime(time.UTC, 9, 0)
	want18 := localTime(time.UTC, 18, 0)
	if !winStart.Equal(want09) {
		t.Errorf("winStart: want %v, got %v (all-day event should be ignored)", want09, winStart)
	}
	if !winEnd.Equal(want18) {
		t.Errorf("winEnd: want %v, got %v (all-day event should be ignored)", want18, winEnd)
	}
}

// ─── LanesByCalendar ────────────────────────────────────────────────────────

// TestLanesByCalendar_SingleCalendarSingleEvent verifies LeftPct and WidthPct
// calculations for a simple case.
func TestLanesByCalendar_SingleCalendarSingleEvent(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal := newCal("c1", "#378ADD")
	// Event from 10:00-12:00.  Expected window: 09:30-12:30 (min-30, max+30).
	event := newEvent("c1", "#378ADD", localTime(loc, 10, 0), localTime(loc, 12, 0), false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal},
		Events:    []AgendaEvent{event},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 1 {
		t.Fatalf("expected 1 lane, got %d", len(lanes))
	}
	if lanes[0].Empty {
		t.Fatal("lane should not be empty")
	}
	if len(lanes[0].Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(lanes[0].Blocks))
	}

	b := lanes[0].Blocks[0]
	// Event 10:00-12:00.  minStart-30min = 09:30 > baseline 09:00, so winStart
	// stays at 09:00.  winEnd = 12:00+30min = 12:30.
	// Window: 09:00-12:30 = 210 min.
	// LeftPct = (10:00-09:00) / 210min * 100 = 60/210*100 ≈ 28.57%
	// WidthPct = 120min / 210min * 100 ≈ 57.14%
	const eps = 0.01
	wantLeft := 60.0 / 210.0 * 100
	wantWidth := 120.0 / 210.0 * 100

	if diff := b.LeftPct - wantLeft; diff < -eps || diff > eps {
		t.Errorf("LeftPct: want %.4f, got %.4f", wantLeft, b.LeftPct)
	}
	if diff := b.WidthPct - wantWidth; diff < -eps || diff > eps {
		t.Errorf("WidthPct: want %.4f, got %.4f", wantWidth, b.WidthPct)
	}
	if b.LeftPct+b.WidthPct > 100+eps {
		t.Errorf("LeftPct+WidthPct exceeds 100: %.4f", b.LeftPct+b.WidthPct)
	}
}

// TestLanesByCalendar_TwoCalendarsProduceTwoLanes verifies the lane count.
func TestLanesByCalendar_TwoCalendarsProduceTwoLanes(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal1 := newCal("c1", "#378ADD")
	cal2 := newCal("c2", "#1D9E75")
	e1 := newEvent("c1", "#378ADD", localTime(loc, 10, 0), localTime(loc, 11, 0), false)
	e2 := newEvent("c2", "#1D9E75", localTime(loc, 14, 0), localTime(loc, 15, 0), false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal1, cal2},
		Events:    []AgendaEvent{e1, e2},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 2 {
		t.Errorf("expected 2 lanes, got %d", len(lanes))
	}
}

// TestLanesByCalendar_EmptyCalendarHasEmptyLane verifies that a calendar with no
// events produces a lane with Empty=true and no blocks.
func TestLanesByCalendar_EmptyCalendarHasEmptyLane(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	calWithEvents := newCal("c1", "#378ADD")
	calEmpty := newCal("c2", "#1D9E75")
	e1 := newEvent("c1", "#378ADD", localTime(loc, 10, 0), localTime(loc, 11, 0), false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{calWithEvents, calEmpty},
		Events:    []AgendaEvent{e1},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 2 {
		t.Fatalf("expected 2 lanes, got %d", len(lanes))
	}

	var emptyLane *CalendarLane
	for i := range lanes {
		if lanes[i].Calendar.ID == "c2" {
			emptyLane = &lanes[i]
		}
	}
	if emptyLane == nil {
		t.Fatal("lane for c2 not found")
	}
	if !emptyLane.Empty {
		t.Error("expected Empty=true for calendar with no events")
	}
	if len(emptyLane.Blocks) != 0 {
		t.Errorf("expected 0 blocks, got %d", len(emptyLane.Blocks))
	}
}

// TestLanesByCalendar_AllDayEventsExcluded verifies all-day events are not
// converted to strip blocks.
func TestLanesByCalendar_AllDayEventsExcluded(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal := newCal("c1", "#378ADD")
	allDay := newEvent("c1", "#378ADD", localTime(loc, 0, 0), localTime(loc, 0, 0), true)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal},
		Events:    []AgendaEvent{allDay},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 1 {
		t.Fatalf("expected 1 lane, got %d", len(lanes))
	}
	if !lanes[0].Empty {
		t.Error("lane should be empty — all-day event must be excluded")
	}
}

// TestLanesByCalendar_OvernightEventClipped verifies that an event spanning
// midnight is clipped to the day window (22:00-00:00, not 22:00-02:00).
func TestLanesByCalendar_OvernightEventClipped(t *testing.T) {
	loc := time.UTC
	// Day window 00:00-24:00 on 2024-03-15.
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal := newCal("c1", "#378ADD")
	// Event starts at 22:00 and ends at 02:00 the next day.
	evtStart := time.Date(2024, 3, 15, 22, 0, 0, 0, loc)
	evtEnd := time.Date(2024, 3, 16, 2, 0, 0, 0, loc)
	event := newEvent("c1", "#378ADD", evtStart, evtEnd, false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal},
		Events:    []AgendaEvent{event},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 1 {
		t.Fatalf("expected 1 lane, got %d", len(lanes))
	}
	if lanes[0].Empty || len(lanes[0].Blocks) == 0 {
		t.Fatal("expected non-empty lane for overnight event")
	}

	b := lanes[0].Blocks[0]
	// The event start (22:00) does not push winStart below the 09:00 baseline
	// (22:00 - 30min = 21:30 > 09:00).  maxEnd = 02:00 next day + 30min is
	// clamped to dayEnd = 00:00 next day.  So window = 09:00–00:00 = 900 min.
	// Event clipped to 22:00–00:00 = 120 min.
	// LeftPct = (22:00 - 09:00) / 900 * 100 = 780/900*100 ≈ 86.67%
	// WidthPct = 120/900*100 ≈ 13.33%
	const eps = 0.01
	wantLeft := 780.0 / 900.0 * 100
	wantWidth := 120.0 / 900.0 * 100

	if diff := b.LeftPct - wantLeft; diff < -eps || diff > eps {
		t.Errorf("LeftPct: want %.4f, got %.4f", wantLeft, b.LeftPct)
	}
	if diff := b.WidthPct - wantWidth; diff < -eps || diff > eps {
		t.Errorf("WidthPct: want %.4f, got %.4f", wantWidth, b.WidthPct)
	}
}

// TestLanesByCalendar_LeftPctPlusWidthPctNeverExceeds100 verifies that an event
// clipped to the right edge of the visible window does not overflow.
func TestLanesByCalendar_LeftPctPlusWidthPctNeverExceeds100(t *testing.T) {
	loc := time.UTC
	dayStart := time.Date(2024, 3, 15, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal := newCal("c1", "#378ADD")
	// Event starts at 17:30 and ends at 19:00, extending past the 18:00 baseline.
	event := newEvent("c1", "#378ADD", localTime(loc, 17, 30), localTime(loc, 19, 0), false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal},
		Events:    []AgendaEvent{event},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 1 || len(lanes[0].Blocks) != 1 {
		t.Fatalf("expected 1 block, got lanes=%d", len(lanes))
	}

	b := lanes[0].Blocks[0]
	if b.LeftPct+b.WidthPct > 100.0001 {
		t.Errorf("LeftPct+WidthPct = %.4f exceeds 100", b.LeftPct+b.WidthPct)
	}
}

// ─── FlatLane ────────────────────────────────────────────────────────────────

// TestFlatLane_TwoCalendarsTwoBlocks verifies that 2 events from different
// calendars on the same day produce 2 blocks in the flat lane.
func TestFlatLane_TwoCalendarsTwoBlocks(t *testing.T) {
	loc := time.UTC
	winStart := time.Date(2024, 3, 15, 9, 0, 0, 0, loc)
	winEnd := time.Date(2024, 3, 15, 18, 0, 0, 0, loc)

	e1 := newEvent("c1", "#378ADD", localTime(loc, 10, 0), localTime(loc, 11, 0), false)
	e2 := newEvent("c2", "#1D9E75", localTime(loc, 14, 0), localTime(loc, 15, 0), false)

	blocks := FlatLane([]AgendaEvent{e1, e2}, winStart, winEnd)
	if len(blocks) != 2 {
		t.Errorf("want 2 blocks, got %d", len(blocks))
	}
}

// TestFlatLane_AllDayEventsExcluded verifies that all-day events are not
// converted to strip blocks by FlatLane.
func TestFlatLane_AllDayEventsExcluded(t *testing.T) {
	loc := time.UTC
	winStart := time.Date(2024, 3, 15, 9, 0, 0, 0, loc)
	winEnd := time.Date(2024, 3, 15, 18, 0, 0, 0, loc)

	allDay := newEvent("c1", "#378ADD", localTime(loc, 0, 0), localTime(loc, 0, 0), true)
	timed := newEvent("c2", "#1D9E75", localTime(loc, 10, 0), localTime(loc, 11, 0), false)

	blocks := FlatLane([]AgendaEvent{allDay, timed}, winStart, winEnd)
	if len(blocks) != 1 {
		t.Errorf("want 1 block (all-day excluded), got %d", len(blocks))
	}
}

// TestFlatLane_OverflowLeft verifies that an event starting before windowStart
// produces a block with OverflowLeft=true and LeftPct=0.
func TestFlatLane_OverflowLeft(t *testing.T) {
	loc := time.UTC
	winStart := time.Date(2024, 3, 15, 9, 0, 0, 0, loc)
	winEnd := time.Date(2024, 3, 15, 18, 0, 0, 0, loc)

	// Event starts at 07:00, before the 09:00 window start.
	e := newEvent("c1", "#378ADD", localTime(loc, 7, 0), localTime(loc, 10, 0), false)
	blocks := FlatLane([]AgendaEvent{e}, winStart, winEnd)

	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if !b.OverflowLeft {
		t.Error("OverflowLeft should be true for event starting before windowStart")
	}
	if b.LeftPct != 0 {
		t.Errorf("LeftPct should be 0 for overflow-left block, got %.4f", b.LeftPct)
	}
}

// TestFlatLane_OverflowRight verifies that an event ending after windowEnd
// produces a block with OverflowRight=true.
func TestFlatLane_OverflowRight(t *testing.T) {
	loc := time.UTC
	winStart := time.Date(2024, 3, 15, 9, 0, 0, 0, loc)
	winEnd := time.Date(2024, 3, 15, 18, 0, 0, 0, loc)

	// Event ends at 20:00, after the 18:00 window end.
	e := newEvent("c1", "#378ADD", localTime(loc, 16, 0), localTime(loc, 20, 0), false)
	blocks := FlatLane([]AgendaEvent{e}, winStart, winEnd)

	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d", len(blocks))
	}
	b := blocks[0]
	if !b.OverflowRight {
		t.Error("OverflowRight should be true for event ending after windowEnd")
	}
}

// ─── ComputeSharedWindow ─────────────────────────────────────────────────────

func buildWeekDays(loc *time.Location, events map[int][]AgendaEvent) [7]DayEvents {
	monday := time.Date(2024, 4, 14, 0, 0, 0, 0, loc) // a known Monday
	var days [7]DayEvents
	for i := range 7 {
		days[i] = DayEvents{
			DayStart: monday.AddDate(0, 0, i),
			DayEnd:   monday.AddDate(0, 0, i+1),
		}
		if evts, ok := events[i]; ok {
			days[i].Events = evts
		}
	}
	return days
}

// TestComputeSharedWindow_EventsOnMondayAndThursday verifies that events on
// non-adjacent days both influence the shared window.
func TestComputeSharedWindow_EventsOnMondayAndThursday(t *testing.T) {
	loc := time.UTC
	monday := time.Date(2024, 4, 14, 0, 0, 0, 0, loc)
	thursday := monday.AddDate(0, 0, 3)

	// Monday 08:00-09:00, Thursday 19:00-20:00
	evtMon := AgendaEvent{
		ID: "mon", Title: "Mon",
		Start: time.Date(monday.Year(), monday.Month(), monday.Day(), 8, 0, 0, 0, loc),
		End:   time.Date(monday.Year(), monday.Month(), monday.Day(), 9, 0, 0, 0, loc),
	}
	evtThu := AgendaEvent{
		ID: "thu", Title: "Thu",
		Start: time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 19, 0, 0, 0, loc),
		End:   time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 20, 0, 0, 0, loc),
	}

	days := buildWeekDays(loc, map[int][]AgendaEvent{0: {evtMon}, 3: {evtThu}})
	winStart, winEnd := ComputeSharedWindow(days)

	// minStart = 08:00 Mon → padded 07:30 < baseline 09:00 → winStart = 07:30
	wantStart := time.Date(monday.Year(), monday.Month(), monday.Day(), 7, 30, 0, 0, loc)
	// maxEnd = 20:00 Thu → winEnd = 20:30 Thu
	wantEnd := time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 20, 30, 0, 0, loc)

	if !winStart.Equal(wantStart) {
		t.Errorf("winStart: want %v, got %v", wantStart, winStart)
	}
	if !winEnd.Equal(wantEnd) {
		t.Errorf("winEnd: want %v, got %v", wantEnd, winEnd)
	}
}

// TestComputeSharedWindow_EmptyWeekReturnsBaseline verifies that a week with no
// timed events returns the 09:00-18:00 baseline in the first day's timezone.
func TestComputeSharedWindow_EmptyWeekReturnsBaseline(t *testing.T) {
	loc := time.UTC
	days := buildWeekDays(loc, nil)
	winStart, winEnd := ComputeSharedWindow(days)

	y, m, d := days[0].DayStart.Date()
	wantStart := time.Date(y, m, d, 9, 0, 0, 0, loc)
	wantEnd := time.Date(y, m, d, 18, 0, 0, 0, loc)

	if !winStart.Equal(wantStart) {
		t.Errorf("winStart: want %v, got %v", wantStart, winStart)
	}
	if !winEnd.Equal(wantEnd) {
		t.Errorf("winEnd: want %v, got %v", wantEnd, winEnd)
	}
}

// TestComputeSharedWindow_FlatLaneIntegration verifies that when a week has an
// event at 07:00 Mon and 20:00 Thu, the shared window covers both and FlatLane
// for a day with an event starting before the shared window produces a block
// with OverflowLeft=true and LeftPct=0.
func TestComputeSharedWindow_FlatLaneIntegration(t *testing.T) {
	loc := time.UTC
	monday := time.Date(2024, 4, 14, 0, 0, 0, 0, loc)
	thursday := monday.AddDate(0, 0, 3)

	// Monday 07:00-08:00 (before 09:00 baseline — will push window left)
	evtMon := AgendaEvent{
		ID: "mon", Title: "Early Mon",
		Start: time.Date(monday.Year(), monday.Month(), monday.Day(), 7, 0, 0, 0, loc),
		End:   time.Date(monday.Year(), monday.Month(), monday.Day(), 8, 0, 0, 0, loc),
	}
	// Thursday 20:00-21:00
	evtThu := AgendaEvent{
		ID: "thu", Title: "Late Thu",
		Start: time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 20, 0, 0, 0, loc),
		End:   time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 21, 0, 0, 0, loc),
	}
	days := buildWeekDays(loc, map[int][]AgendaEvent{0: {evtMon}, 3: {evtThu}})
	winStart, winEnd := ComputeSharedWindow(days)

	// winStart should be at most 06:30 (07:00 - 30min padding)
	latestAllowedStart := time.Date(monday.Year(), monday.Month(), monday.Day(), 6, 30, 0, 0, loc)
	if winStart.After(latestAllowedStart) {
		t.Errorf("winStart %v should be at or before %v (early event not covered)", winStart, latestAllowedStart)
	}

	// winEnd should cover 21:00 Thu + 30min = 21:30 Thu
	earliestAllowedEnd := time.Date(thursday.Year(), thursday.Month(), thursday.Day(), 21, 30, 0, 0, loc)
	if winEnd.Before(earliestAllowedEnd) {
		t.Errorf("winEnd %v should be at or after %v (late event not covered)", winEnd, earliestAllowedEnd)
	}

	// Now add a Saturday event that starts before winStart — FlatLane should
	// produce a block with OverflowLeft=true and LeftPct=0.
	saturday := monday.AddDate(0, 0, 5)
	earlyEvt := AgendaEvent{
		ID: "sat", Title: "Before window",
		// Start 30 min before winStart
		Start: winStart.Add(-30 * time.Minute),
		End:   winStart.Add(30 * time.Minute),
	}
	blocks := FlatLane([]AgendaEvent{earlyEvt}, winStart, winEnd)
	if len(blocks) != 1 {
		t.Fatalf("want 1 block, got %d (saturday=%v)", len(blocks), saturday)
	}
	b := blocks[0]
	if !b.OverflowLeft {
		t.Error("OverflowLeft should be true for event starting before shared window")
	}
	if b.LeftPct != 0 {
		t.Errorf("LeftPct should be 0 for overflow-left block, got %.4f", b.LeftPct)
	}
}

// TestLanesByCalendar_DSTSpringForward verifies that an event spanning the
// spring-forward transition (America/New_York, 2024-03-10 02:00 → 03:00) is
// positioned correctly based on wall-clock duration.
func TestLanesByCalendar_DSTSpringForward(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("America/New_York timezone not available:", err)
	}

	// Spring-forward in 2024 is 2024-03-10 02:00 local → 03:00.
	// An event 01:00-03:00 local spans the gap; real duration is only 1h (the
	// clock jumps from 02:00 to 03:00, so 01:00-03:00 = 1 real hour).
	evtStart := time.Date(2024, 3, 10, 1, 0, 0, 0, loc)
	evtEnd := time.Date(2024, 3, 10, 3, 0, 0, 0, loc)

	dayStart := time.Date(2024, 3, 10, 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	cal := newCal("c1", "#378ADD")
	event := newEvent("c1", "#378ADD", evtStart, evtEnd, false)

	view := &AgendaView{
		Calendars: []*models.CalendarConnection{cal},
		Events:    []AgendaEvent{event},
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}

	lanes := LanesByCalendar(view)
	if len(lanes) != 1 || len(lanes[0].Blocks) != 1 {
		t.Fatalf("expected 1 block")
	}

	b := lanes[0].Blocks[0]
	// The block must be non-zero width.
	if b.WidthPct <= 0 {
		t.Errorf("expected positive WidthPct for DST event, got %.4f", b.WidthPct)
	}
	// The real duration of 01:00-03:00 on DST day is 1h = 3600s.
	realDuration := evtEnd.Sub(evtStart).Seconds()
	if realDuration != 3600 {
		t.Errorf("expected real event duration 3600s, got %.0f", realDuration)
	}
	// LeftPct+WidthPct must not exceed 100.
	if b.LeftPct+b.WidthPct > 100.0001 {
		t.Errorf("LeftPct+WidthPct=%.4f exceeds 100", b.LeftPct+b.WidthPct)
	}
}

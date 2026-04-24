package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// makeCalendar is a test helper that builds a *models.CalendarConnection with
// the given createdAt offset (in minutes from epoch) and optional pre-set color.
func makeCalendar(minuteOffset int, color string) *models.CalendarConnection {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	return &models.CalendarConnection{
		ID:        fmt.Sprintf("cal-%d", minuteOffset),
		CreatedAt: models.NewSQLiteTime(base.Add(time.Duration(minuteOffset) * time.Minute)),
		Color:     color,
	}
}

// TestAssignColors_StableOrderThreeCalendars verifies that the same colors are
// assigned regardless of the order the slice is passed in.
func TestAssignColors_StableOrderThreeCalendars(t *testing.T) {
	build := func() []*models.CalendarConnection {
		return []*models.CalendarConnection{
			makeCalendar(2, ""), // second oldest
			makeCalendar(0, ""), // oldest
			makeCalendar(4, ""), // newest
		}
	}

	// First call: shuffled order as above.
	cals1 := build()
	AssignColors(cals1)

	// Second call: already in ascending order.
	cals2 := build()
	// Sort ascending manually before calling so input order differs.
	cals2[0], cals2[1] = cals2[1], cals2[0] // now [0, 2, 4] by minute
	AssignColors(cals2)

	// Both should produce the same color assignment when keyed by calendar offset.
	colorFor := func(cals []*models.CalendarConnection, minuteOffset int) string {
		for _, c := range cals {
			base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
			if c.CreatedAt.Time.Equal(base.Add(time.Duration(minuteOffset) * time.Minute)) {
				return c.Color
			}
		}
		return ""
	}

	for _, offset := range []int{0, 2, 4} {
		c1 := colorFor(cals1, offset)
		c2 := colorFor(cals2, offset)
		if c1 == "" {
			t.Errorf("calendar offset %d has empty color after AssignColors", offset)
		}
		if c1 != c2 {
			t.Errorf("offset %d: got %q vs %q — assignment is not stable", offset, c1, c2)
		}
	}

	// The oldest calendar (offset 0) should get palette[0].
	if got := colorFor(cals1, 0); got != CalendarPalette[0] {
		t.Errorf("oldest calendar: want %s, got %s", CalendarPalette[0], got)
	}
}

// TestAssignColors_NineDistinctColors verifies that 9 calendars each receive a
// unique color drawn from the full palette.
func TestAssignColors_NineDistinctColors(t *testing.T) {
	cals := make([]*models.CalendarConnection, 9)
	for i := range cals {
		cals[i] = makeCalendar(i, "")
	}

	AssignColors(cals)

	seen := map[string]bool{}
	for _, cal := range cals {
		if cal.Color == "" {
			t.Error("calendar has empty color after AssignColors")
		}
		if seen[cal.Color] {
			t.Errorf("duplicate color assigned: %s", cal.Color)
		}
		seen[cal.Color] = true
	}

	if len(seen) != len(CalendarPalette) {
		t.Errorf("expected %d distinct colors, got %d", len(CalendarPalette), len(seen))
	}
}

// TestAssignColors_TenthCalendarWraps verifies that the 10th calendar receives
// the first palette color (CalendarPalette[0]) when all 9 slots have been used.
func TestAssignColors_TenthCalendarWraps(t *testing.T) {
	cals := make([]*models.CalendarConnection, 10)
	for i := range cals {
		cals[i] = makeCalendar(i, "")
	}

	AssignColors(cals)

	// After sorting by CreatedAt, cals[9] (offset 9) is the newest = 10th assigned.
	// Find the calendar with the largest CreatedAt.
	var newest *models.CalendarConnection
	for _, cal := range cals {
		if newest == nil || cal.CreatedAt.Time.After(newest.CreatedAt.Time) {
			newest = cal
		}
	}

	if newest == nil {
		t.Fatal("no calendars found")
	}
	if newest.Color != CalendarPalette[0] {
		t.Errorf("10th calendar: want %s (palette[0]), got %s", CalendarPalette[0], newest.Color)
	}
}

// TestAssignColors_PreserveOverride verifies that a calendar whose Color is
// already set keeps its value and that the overridden color is not given to
// other calendars.
func TestAssignColors_PreserveOverride(t *testing.T) {
	overrideColor := CalendarPalette[0] // #378ADD

	cals := []*models.CalendarConnection{
		makeCalendar(0, overrideColor), // pre-assigned, oldest
		makeCalendar(1, ""),
		makeCalendar(2, ""),
	}

	AssignColors(cals)

	// Pre-assigned calendar must keep its color.
	if cals[0].Color != overrideColor {
		t.Errorf("overridden color was changed: want %s, got %s", overrideColor, cals[0].Color)
	}

	// Unset calendars must not receive the overridden color.
	for _, cal := range cals[1:] {
		if cal.Color == "" {
			t.Error("unset calendar still has empty color after AssignColors")
		}
		if cal.Color == overrideColor {
			t.Errorf("unset calendar received the override color %s", overrideColor)
		}
	}

	// The two unset calendars should receive palette[1] and palette[2] in order.
	if cals[1].Color != CalendarPalette[1] {
		t.Errorf("second calendar: want %s, got %s", CalendarPalette[1], cals[1].Color)
	}
	if cals[2].Color != CalendarPalette[2] {
		t.Errorf("third calendar: want %s, got %s", CalendarPalette[2], cals[2].Color)
	}
}

// TestAssignColors_EmptyInput verifies that calling AssignColors with a nil or
// empty slice does not panic and is a no-op.
func TestAssignColors_EmptyInput(t *testing.T) {
	// Should not panic.
	AssignColors(nil)
	AssignColors([]*models.CalendarConnection{})
}

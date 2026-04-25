package services

import (
	"fmt"
	"strings"
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

// TestAssignColors_WrapsThroughFullPaletteAfterOverrides verifies that once all
// 9 palette colors are in use (some via override, the rest newly assigned), the
// next unset calendar wraps back to CalendarPalette[0] — not to the first entry
// of the reduced assignQueue.
func TestAssignColors_WrapsThroughFullPaletteAfterOverrides(t *testing.T) {
	// Calendar at offset 0 has palette[0] pre-assigned.
	// 9 uncolored calendars follow (offsets 1-9).
	// The 9 uncolored ones fill palette[1]..palette[8] then need to wrap.
	// The 9th uncolored calendar (10th total) should get CalendarPalette[0].
	cals := make([]*models.CalendarConnection, 10)
	cals[0] = makeCalendar(0, CalendarPalette[0]) // pre-assigned
	for i := 1; i <= 9; i++ {
		cals[i] = makeCalendar(i, "")
	}

	AssignColors(cals)

	// cals[0] keeps its override.
	if cals[0].Color != CalendarPalette[0] {
		t.Errorf("pre-assigned calendar color changed: want %s, got %s", CalendarPalette[0], cals[0].Color)
	}

	// After sort cals is in CreatedAt order (offsets 0..9).
	// Uncolored calendars are at index 1..9 after the sort.
	// The first 8 uncolored get palette[1]..palette[8].
	for i := 1; i <= 8; i++ {
		want := CalendarPalette[i]
		if cals[i].Color != want {
			t.Errorf("calendar %d: want %s, got %s", i, want, cals[i].Color)
		}
	}
	// The 9th uncolored (cals[9]) must wrap to CalendarPalette[0].
	if cals[9].Color != CalendarPalette[0] {
		t.Errorf("9th uncolored calendar: want wrap to %s, got %s", CalendarPalette[0], cals[9].Color)
	}
}

// TestAssignColors_TieBreakByID verifies that calendars with identical CreatedAt
// timestamps are still ordered deterministically (by ID) so colors are stable
// regardless of the incoming slice order.
func TestAssignColors_TieBreakByID(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC) // same second for both

	// Two calendars with identical CreatedAt; IDs chosen so "aaa" < "zzz".
	calA := &models.CalendarConnection{ID: "aaa", CreatedAt: models.NewSQLiteTime(base)}
	calZ := &models.CalendarConnection{ID: "zzz", CreatedAt: models.NewSQLiteTime(base)}

	// Call 1: [Z, A]
	cals1 := []*models.CalendarConnection{calZ, calA}
	// Reset colors before each call.
	for _, c := range cals1 {
		c.Color = ""
	}
	AssignColors(cals1)
	colorA1 := calA.Color
	colorZ1 := calZ.Color

	// Call 2: [A, Z]
	calA.Color = ""
	calZ.Color = ""
	cals2 := []*models.CalendarConnection{calA, calZ}
	AssignColors(cals2)
	colorA2 := calA.Color
	colorZ2 := calZ.Color

	if colorA1 != colorA2 {
		t.Errorf("calA color not stable across input orders: %s vs %s", colorA1, colorA2)
	}
	if colorZ1 != colorZ2 {
		t.Errorf("calZ color not stable across input orders: %s vs %s", colorZ1, colorZ2)
	}
	// "aaa" sorts before "zzz" so calA should get palette[0].
	if colorA1 != CalendarPalette[0] {
		t.Errorf("calA (ID=aaa): want %s, got %s", CalendarPalette[0], colorA1)
	}
	if colorZ1 != CalendarPalette[1] {
		t.Errorf("calZ (ID=zzz): want %s, got %s", CalendarPalette[1], colorZ1)
	}
}

// TestAssignColors_CaseInsensitiveOverride verifies that a stored color using
// lowercase hex (e.g. "#378add") is treated as equivalent to the uppercase
// palette entry ("#378ADD") and is not handed to another calendar.
func TestAssignColors_CaseInsensitiveOverride(t *testing.T) {
	lowercaseColor := "#378add" // same as CalendarPalette[0] but lowercase

	cals := []*models.CalendarConnection{
		makeCalendar(0, lowercaseColor), // pre-assigned, oldest
		makeCalendar(1, ""),
	}

	AssignColors(cals)

	// Pre-assigned calendar keeps its original casing.
	if cals[0].Color != lowercaseColor {
		t.Errorf("override color casing was changed: want %s, got %s", lowercaseColor, cals[0].Color)
	}

	// Unset calendar must not receive the same color (regardless of case).
	if strings.EqualFold(cals[1].Color, lowercaseColor) {
		t.Errorf("unset calendar received the same color as the override: %s", cals[1].Color)
	}

	// Unset calendar should get palette[1] (the next available slot).
	if cals[1].Color != CalendarPalette[1] {
		t.Errorf("second calendar: want %s, got %s", CalendarPalette[1], cals[1].Color)
	}
}

// TestAssignColors_EmptyInput verifies that calling AssignColors with a nil or
// empty slice does not panic and is a no-op.
func TestAssignColors_EmptyInput(t *testing.T) {
	// Should not panic.
	AssignColors(nil)
	AssignColors([]*models.CalendarConnection{})
}

// TestAssignColors_CustomColorDoesNotShiftWrapIndex verifies that a
// pre-existing calendar with a custom (non-palette) color does not shift the
// wrap index.  After all 9 palette slots are consumed the next uncolored
// calendar must receive CalendarPalette[0], not CalendarPalette[1].
func TestAssignColors_CustomColorDoesNotShiftWrapIndex(t *testing.T) {
	customColor := "#ABCDEF" // not in CalendarPalette

	// 1 custom-colored + 10 uncolored: first 9 uncolored fill palette[0..8],
	// the 10th must wrap to palette[0].
	cals := make([]*models.CalendarConnection, 11)
	cals[0] = makeCalendar(0, customColor)
	for i := 1; i <= 10; i++ {
		cals[i] = makeCalendar(i, "")
	}

	AssignColors(cals)

	if cals[0].Color != customColor {
		t.Errorf("custom color was changed: want %s, got %s", customColor, cals[0].Color)
	}
	for i := 1; i <= 9; i++ {
		want := CalendarPalette[i-1]
		if cals[i].Color != want {
			t.Errorf("calendar %d: want %s, got %s", i, want, cals[i].Color)
		}
	}
	// 10th uncolored calendar must wrap to palette[0], not palette[1].
	if cals[10].Color != CalendarPalette[0] {
		t.Errorf("10th uncolored (after custom): want wrap to %s (palette[0]), got %s", CalendarPalette[0], cals[10].Color)
	}
}

// TestAssignColors_SequentialAdditionsContinueRotation verifies that when
// calendars are connected one at a time (each Connect* call adds a single new
// calendar to an already-colored list), the palette rotation continues past the
// 9th slot rather than restarting at index 0 on every wrap.
func TestAssignColors_SequentialAdditionsContinueRotation(t *testing.T) {
	// Simulate the real Connect* flow: a growing slice where all existing
	// calendars already have colors and each call adds exactly one new entry.
	var cals []*models.CalendarConnection
	assigned := make([]string, 12)

	for i := 0; i < 12; i++ {
		cal := makeCalendar(i, "")
		cals = append(cals, cal)
		// AssignColors skips already-colored entries and assigns the new one.
		AssignColors(cals)
		assigned[i] = cal.Color
	}

	// Calendars 0-8 should each receive a distinct palette entry in order.
	for i := 0; i < 9; i++ {
		if assigned[i] != CalendarPalette[i] {
			t.Errorf("calendar %d: want %s, got %s", i, CalendarPalette[i], assigned[i])
		}
	}
	// Calendar 9 wraps to palette[0].
	if assigned[9] != CalendarPalette[0] {
		t.Errorf("10th calendar: want wrap to %s (palette[0]), got %s", CalendarPalette[0], assigned[9])
	}
	// Calendar 10 must continue the rotation to palette[1], not restart at palette[0].
	if assigned[10] != CalendarPalette[1] {
		t.Errorf("11th calendar: want %s (palette[1]), got %s — wrap restarted instead of continuing", CalendarPalette[1], assigned[10])
	}
	// Calendar 11 continues to palette[2].
	if assigned[11] != CalendarPalette[2] {
		t.Errorf("12th calendar: want %s (palette[2]), got %s", CalendarPalette[2], assigned[11])
	}
}

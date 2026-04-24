package services

import (
	"cmp"
	"slices"
	"strings"

	"github.com/meet-when/meet-when/internal/models"
)

// CalendarPalette is the ordered set of calendar colors.
var CalendarPalette = []string{
	"#378ADD",
	"#1D9E75",
	"#D85A30",
	"#7F77DD",
	"#639922",
	"#BA7517",
	"#D4537E",
	"#E24B4A",
	"#5F5E5A",
}

// AssignColors assigns palette colors to calendars that have no color set.
// Input is sorted by CreatedAt ASC before assignment so results are stable
// regardless of the order the slice was passed in.
// Calendars with a non-empty Color are left unchanged; their color is treated
// as already-used and skipped when assigning to unset calendars.
// Once the available palette slots are exhausted, assignment wraps back to
// index 0 of the full palette.
func AssignColors(calendars []*models.CalendarConnection) {
	if len(calendars) == 0 {
		return
	}

	// Sort by CreatedAt ascending; break ties with ID so the order is fully
	// deterministic even when two calendars were persisted within the same
	// second (SQLiteTime stores whole-second precision after a DB round-trip).
	slices.SortFunc(calendars, func(a, b *models.CalendarConnection) int {
		if c := cmp.Compare(a.CreatedAt.Time.Unix(), b.CreatedAt.Time.Unix()); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})

	// Collect colors already in use (pre-assigned by the user or a prior call).
	// Normalize to uppercase so that stored values like "#378add" are treated the
	// same as the palette canonical form "#378ADD".
	usedColors := make(map[string]bool, len(CalendarPalette))
	for _, cal := range calendars {
		if cal.Color != "" {
			usedColors[strings.ToUpper(cal.Color)] = true
		}
	}

	// Build the assignment queue: palette entries not yet in use, in index order.
	// CalendarPalette is already uppercase, so direct lookup against usedColors works.
	assignQueue := make([]string, 0, len(CalendarPalette))
	for _, c := range CalendarPalette {
		if !usedColors[c] {
			assignQueue = append(assignQueue, c)
		}
	}
	// Walk sorted calendars assigning the next queue slot to each unset one.
	// Once the queue is exhausted (all palette colors now in use), wrap back
	// through the full palette starting at index 0.
	idx := 0
	for _, cal := range calendars {
		if cal.Color != "" {
			continue
		}
		if idx < len(assignQueue) {
			cal.Color = assignQueue[idx]
		} else {
			// All available (non-preassigned) slots used; cycle the full palette.
			cal.Color = CalendarPalette[(idx-len(assignQueue))%len(CalendarPalette)]
		}
		idx++
	}
}

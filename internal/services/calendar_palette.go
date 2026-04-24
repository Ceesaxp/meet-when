package services

import (
	"cmp"
	"slices"

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

	// Sort by CreatedAt ascending (stable identity across call orders).
	slices.SortFunc(calendars, func(a, b *models.CalendarConnection) int {
		return cmp.Compare(a.CreatedAt.Time.UnixNano(), b.CreatedAt.Time.UnixNano())
	})

	// Collect colors already in use (pre-assigned by the user or a prior call).
	usedColors := make(map[string]bool, len(CalendarPalette))
	for _, cal := range calendars {
		if cal.Color != "" {
			usedColors[cal.Color] = true
		}
	}

	// Build the assignment queue: palette entries not yet in use, in index order.
	assignQueue := make([]string, 0, len(CalendarPalette))
	for _, c := range CalendarPalette {
		if !usedColors[c] {
			assignQueue = append(assignQueue, c)
		}
	}
	// If every palette color is already taken, fall back to the full palette.
	if len(assignQueue) == 0 {
		assignQueue = CalendarPalette
	}

	// Walk sorted calendars, assigning the next queue slot to each unset one.
	idx := 0
	for _, cal := range calendars {
		if cal.Color != "" {
			continue
		}
		cal.Color = assignQueue[idx%len(assignQueue)]
		idx++
	}
}

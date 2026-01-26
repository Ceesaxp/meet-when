package services

import (
	"testing"
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

func TestParseAvailabilityRules_SingleInterval(t *testing.T) {
	// Test old format with single start/end per day
	rules := models.JSONMap{
		"enabled": true,
		"days": map[string]interface{}{
			"1": map[string]interface{}{
				"enabled": true,
				"start":   "09:00",
				"end":     "17:00",
			},
		},
	}

	result := parseAvailabilityRules(rules)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	if !result.Enabled {
		t.Error("Expected Enabled to be true")
	}
	day, ok := result.Days[1]
	if !ok {
		t.Fatal("Expected day 1 to exist")
	}
	if !day.Enabled {
		t.Error("Expected day 1 to be enabled")
	}
	if len(day.Intervals) != 1 {
		t.Errorf("Expected 1 interval, got %d", len(day.Intervals))
	}
	if day.Intervals[0].Start != "09:00" {
		t.Errorf("Expected interval start 09:00, got %s", day.Intervals[0].Start)
	}
	if day.Intervals[0].End != "17:00" {
		t.Errorf("Expected interval end 17:00, got %s", day.Intervals[0].End)
	}
}

func TestParseAvailabilityRules_MultipleIntervals(t *testing.T) {
	// Test new format with multiple intervals per day
	rules := models.JSONMap{
		"enabled": true,
		"days": map[string]interface{}{
			"1": map[string]interface{}{
				"enabled": true,
				"intervals": []interface{}{
					map[string]interface{}{"start": "09:00", "end": "12:00"},
					map[string]interface{}{"start": "14:00", "end": "17:00"},
				},
			},
		},
	}

	result := parseAvailabilityRules(rules)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	day, ok := result.Days[1]
	if !ok {
		t.Fatal("Expected day 1 to exist")
	}
	if len(day.Intervals) != 2 {
		t.Errorf("Expected 2 intervals, got %d", len(day.Intervals))
	}
	if day.Intervals[0].Start != "09:00" || day.Intervals[0].End != "12:00" {
		t.Errorf("First interval: expected 09:00-12:00, got %s-%s", day.Intervals[0].Start, day.Intervals[0].End)
	}
	if day.Intervals[1].Start != "14:00" || day.Intervals[1].End != "17:00" {
		t.Errorf("Second interval: expected 14:00-17:00, got %s-%s", day.Intervals[1].Start, day.Intervals[1].End)
	}
}

func TestParseAvailabilityRules_EmptyIntervals(t *testing.T) {
	// Test with no intervals
	rules := models.JSONMap{
		"enabled": true,
		"days": map[string]interface{}{
			"1": map[string]interface{}{
				"enabled":   true,
				"intervals": []interface{}{},
			},
		},
	}

	result := parseAvailabilityRules(rules)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	day, ok := result.Days[1]
	if !ok {
		t.Fatal("Expected day 1 to exist")
	}
	if len(day.Intervals) != 0 {
		t.Errorf("Expected 0 intervals, got %d", len(day.Intervals))
	}
}

func TestParseAvailabilityRules_NilRules(t *testing.T) {
	result := parseAvailabilityRules(nil)
	if result != nil {
		t.Error("Expected nil result for nil rules")
	}
}

func TestParseAvailabilityRules_DayDisabled(t *testing.T) {
	rules := models.JSONMap{
		"enabled": true,
		"days": map[string]interface{}{
			"1": map[string]interface{}{
				"enabled": false,
				"intervals": []interface{}{
					map[string]interface{}{"start": "09:00", "end": "17:00"},
				},
			},
		},
	}

	result := parseAvailabilityRules(rules)

	if result == nil {
		t.Fatal("Expected non-nil result")
	}
	day, ok := result.Days[1]
	if !ok {
		t.Fatal("Expected day 1 to exist")
	}
	if day.Enabled {
		t.Error("Expected day 1 to be disabled")
	}
}

func TestGenerateSlotsInRange_SingleInterval(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	workStart := time.Date(2024, 1, 15, 9, 0, 0, 0, loc)
	workEnd := time.Date(2024, 1, 15, 12, 0, 0, 0, loc)
	duration := 30 * time.Minute
	increment := 15 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	slots := svc.generateSlotsInRange(workStart, workEnd, duration, increment, nil, earliestStart, latestEnd)

	// From 9:00 to 12:00 with 30min duration and 15min increment:
	// 9:00-9:30, 9:15-9:45, 9:30-10:00, ..., 11:30-12:00
	// That's (3*60 - 30)/15 + 1 = 11 slots
	expectedCount := 11
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots, got %d", expectedCount, len(slots))
	}

	// Verify first slot
	if len(slots) > 0 {
		if slots[0].Start.Hour() != 9 || slots[0].Start.Minute() != 0 {
			t.Errorf("First slot should start at 9:00, got %v", slots[0].Start)
		}
		if slots[0].End.Hour() != 9 || slots[0].End.Minute() != 30 {
			t.Errorf("First slot should end at 9:30, got %v", slots[0].End)
		}
	}

	// Verify last slot
	if len(slots) > 0 {
		last := slots[len(slots)-1]
		if last.Start.Hour() != 11 || last.Start.Minute() != 30 {
			t.Errorf("Last slot should start at 11:30, got %v", last.Start)
		}
		if last.End.Hour() != 12 || last.End.Minute() != 0 {
			t.Errorf("Last slot should end at 12:00, got %v", last.End)
		}
	}
}

func TestGenerateSlotsInRange_WithBusyTimes(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	workStart := time.Date(2024, 1, 15, 9, 0, 0, 0, loc)
	workEnd := time.Date(2024, 1, 15, 11, 0, 0, 0, loc)
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	// Block 9:30-10:00
	busySlots := []models.TimeSlot{
		{
			Start: time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
			End:   time.Date(2024, 1, 15, 10, 0, 0, 0, loc),
		},
	}

	slots := svc.generateSlotsInRange(workStart, workEnd, duration, increment, busySlots, earliestStart, latestEnd)

	// Without busy: 9:00-9:30, 9:30-10:00, 10:00-10:30, 10:30-11:00 (4 slots)
	// With busy (9:30-10:00): 9:00-9:30, 10:00-10:30, 10:30-11:00 (3 slots)
	// The 9:30-10:00 slot is blocked
	expectedCount := 3
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots, got %d", expectedCount, len(slots))
	}

	// Make sure the blocked slot is not included
	for _, slot := range slots {
		if slot.Start.Hour() == 9 && slot.Start.Minute() == 30 {
			t.Error("Slot 9:30-10:00 should be blocked but was included")
		}
	}
}

func TestGenerateSlotsInRange_EarliestStartConstraint(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	workStart := time.Date(2024, 1, 15, 9, 0, 0, 0, loc)
	workEnd := time.Date(2024, 1, 15, 11, 0, 0, 0, loc)
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 10, 0, 0, 0, loc) // Can only start at 10:00 or later
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	slots := svc.generateSlotsInRange(workStart, workEnd, duration, increment, nil, earliestStart, latestEnd)

	// Slots before 10:00 should be excluded
	// Only 10:00-10:30, 10:30-11:00 should be available
	expectedCount := 2
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots, got %d", expectedCount, len(slots))
	}

	// Verify no slots start before earliest
	for _, slot := range slots {
		if slot.Start.Before(earliestStart) {
			t.Errorf("Slot %v starts before earliest allowed %v", slot.Start, earliestStart)
		}
	}
}

func TestGetSlotsForDay_MultipleIntervals(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	day := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Monday
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	// Custom template rules with two intervals
	templateRules := &TemplateAvailabilityRules{
		Enabled: true,
		Days: map[int]DayAvailability{
			1: { // Monday
				Enabled: true,
				Intervals: []TimeInterval{
					{Start: "09:00", End: "10:00"}, // 2 slots: 9:00-9:30, 9:30-10:00
					{Start: "14:00", End: "15:00"}, // 2 slots: 14:00-14:30, 14:30-15:00
				},
			},
		},
	}

	slots := svc.getSlotsForDay(day, nil, loc, duration, increment, nil, earliestStart, latestEnd, templateRules)

	// Two 1-hour intervals with 30min slots = 2 + 2 = 4 slots
	expectedCount := 4
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots, got %d", expectedCount, len(slots))
	}

	// Verify slots are sorted by start time
	for i := 1; i < len(slots); i++ {
		if slots[i].Start.Before(slots[i-1].Start) {
			t.Errorf("Slots not sorted: %v comes after %v", slots[i].Start, slots[i-1].Start)
		}
	}

	// Verify we have slots in both intervals
	morningSlots := 0
	afternoonSlots := 0
	for _, slot := range slots {
		if slot.Start.Hour() < 12 {
			morningSlots++
		} else {
			afternoonSlots++
		}
	}
	if morningSlots != 2 {
		t.Errorf("Expected 2 morning slots, got %d", morningSlots)
	}
	if afternoonSlots != 2 {
		t.Errorf("Expected 2 afternoon slots, got %d", afternoonSlots)
	}
}

func TestGetSlotsForDay_EmptyIntervals(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	day := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Monday
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	// Template rules with empty intervals
	templateRules := &TemplateAvailabilityRules{
		Enabled: true,
		Days: map[int]DayAvailability{
			1: { // Monday
				Enabled:   true,
				Intervals: []TimeInterval{}, // Empty
			},
		},
	}

	slots := svc.getSlotsForDay(day, nil, loc, duration, increment, nil, earliestStart, latestEnd, templateRules)

	if len(slots) != 0 {
		t.Errorf("Expected 0 slots for empty intervals, got %d", len(slots))
	}
}

func TestGetSlotsForDay_DayDisabled(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	day := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Monday
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	// Template rules with day disabled
	templateRules := &TemplateAvailabilityRules{
		Enabled: true,
		Days: map[int]DayAvailability{
			1: { // Monday
				Enabled: false, // Disabled
				Intervals: []TimeInterval{
					{Start: "09:00", End: "17:00"},
				},
			},
		},
	}

	slots := svc.getSlotsForDay(day, nil, loc, duration, increment, nil, earliestStart, latestEnd, templateRules)

	if len(slots) != 0 {
		t.Errorf("Expected 0 slots for disabled day, got %d", len(slots))
	}
}

func TestGetSlotsForDay_FallsBackToWorkingHours(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	day := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Monday
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	workingHours := []*models.WorkingHours{
		{
			DayOfWeek: 1, // Monday
			StartTime: "09:00",
			EndTime:   "10:00",
			IsEnabled: true,
		},
	}

	// No template rules - should fall back to working hours
	slots := svc.getSlotsForDay(day, workingHours, loc, duration, increment, nil, earliestStart, latestEnd, nil)

	// 1 hour with 30min slots = 2 slots
	expectedCount := 2
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots from working hours, got %d", expectedCount, len(slots))
	}
}

func TestGetSlotsForDay_TemplateRulesDisabled(t *testing.T) {
	svc := &AvailabilityService{}
	loc, _ := time.LoadLocation("UTC")

	day := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Monday
	duration := 30 * time.Minute
	increment := 30 * time.Minute
	earliestStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	latestEnd := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	workingHours := []*models.WorkingHours{
		{
			DayOfWeek: 1, // Monday
			StartTime: "09:00",
			EndTime:   "10:00",
			IsEnabled: true,
		},
	}

	// Template rules exist but are disabled - should fall back to working hours
	templateRules := &TemplateAvailabilityRules{
		Enabled: false, // Disabled
		Days: map[int]DayAvailability{
			1: {
				Enabled: true,
				Intervals: []TimeInterval{
					{Start: "14:00", End: "17:00"}, // Different times than working hours
				},
			},
		},
	}

	slots := svc.getSlotsForDay(day, workingHours, loc, duration, increment, nil, earliestStart, latestEnd, templateRules)

	// Should use working hours (9-10), not template rules (14-17)
	expectedCount := 2
	if len(slots) != expectedCount {
		t.Errorf("Expected %d slots from working hours, got %d", expectedCount, len(slots))
	}

	// Verify slots are from working hours time (morning), not template rules (afternoon)
	for _, slot := range slots {
		if slot.Start.Hour() >= 12 {
			t.Errorf("Expected morning slots from working hours, got afternoon slot: %v", slot.Start)
		}
	}
}

func TestMergeTimeSlots(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	tests := []struct {
		name     string
		input    []models.TimeSlot
		expected int
	}{
		{
			name:     "empty input",
			input:    []models.TimeSlot{},
			expected: 0,
		},
		{
			name: "no overlap",
			input: []models.TimeSlot{
				{Start: time.Date(2024, 1, 15, 9, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 10, 0, 0, 0, loc)},
				{Start: time.Date(2024, 1, 15, 11, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 12, 0, 0, 0, loc)},
			},
			expected: 2,
		},
		{
			name: "overlapping slots",
			input: []models.TimeSlot{
				{Start: time.Date(2024, 1, 15, 9, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 10, 30, 0, 0, loc)},
				{Start: time.Date(2024, 1, 15, 10, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 11, 0, 0, 0, loc)},
			},
			expected: 1,
		},
		{
			name: "adjacent slots",
			input: []models.TimeSlot{
				{Start: time.Date(2024, 1, 15, 9, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 10, 0, 0, 0, loc)},
				{Start: time.Date(2024, 1, 15, 10, 0, 0, 0, loc), End: time.Date(2024, 1, 15, 11, 0, 0, 0, loc)},
			},
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeTimeSlots(tt.input)
			if len(result) != tt.expected {
				t.Errorf("Expected %d slots, got %d", tt.expected, len(result))
			}
		})
	}
}

func TestSlotOverlapsBusy(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	busySlots := []models.TimeSlot{
		{
			Start: time.Date(2024, 1, 15, 10, 0, 0, 0, loc),
			End:   time.Date(2024, 1, 15, 11, 0, 0, 0, loc),
		},
	}

	tests := []struct {
		name        string
		slotStart   time.Time
		slotEnd     time.Time
		shouldBlock bool
	}{
		{
			name:        "slot before busy",
			slotStart:   time.Date(2024, 1, 15, 9, 0, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
			shouldBlock: false,
		},
		{
			name:        "slot after busy",
			slotStart:   time.Date(2024, 1, 15, 11, 30, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 12, 0, 0, 0, loc),
			shouldBlock: false,
		},
		{
			name:        "slot overlaps busy start",
			slotStart:   time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 10, 30, 0, 0, loc),
			shouldBlock: true,
		},
		{
			name:        "slot overlaps busy end",
			slotStart:   time.Date(2024, 1, 15, 10, 30, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 11, 30, 0, 0, loc),
			shouldBlock: true,
		},
		{
			name:        "slot inside busy",
			slotStart:   time.Date(2024, 1, 15, 10, 15, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 10, 45, 0, 0, loc),
			shouldBlock: true,
		},
		{
			name:        "slot contains busy",
			slotStart:   time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 11, 30, 0, 0, loc),
			shouldBlock: true,
		},
		{
			name:        "slot ends at busy start",
			slotStart:   time.Date(2024, 1, 15, 9, 30, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 10, 0, 0, 0, loc),
			shouldBlock: false,
		},
		{
			name:        "slot starts at busy end",
			slotStart:   time.Date(2024, 1, 15, 11, 0, 0, 0, loc),
			slotEnd:     time.Date(2024, 1, 15, 11, 30, 0, 0, loc),
			shouldBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := slotOverlapsBusy(tt.slotStart, tt.slotEnd, busySlots)
			if result != tt.shouldBlock {
				t.Errorf("Expected shouldBlock=%v, got %v", tt.shouldBlock, result)
			}
		})
	}
}

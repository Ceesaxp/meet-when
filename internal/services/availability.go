package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// AvailabilityService handles availability calculations
type AvailabilityService struct {
	repos    *repository.Repositories
	calendar *CalendarService
}

// NewAvailabilityService creates a new availability service
func NewAvailabilityService(repos *repository.Repositories, calendar *CalendarService) *AvailabilityService {
	return &AvailabilityService{
		repos:    repos,
		calendar: calendar,
	}
}

// GetAvailableSlotsInput represents input for getting available slots
type GetAvailableSlotsInput struct {
	HostID     string
	TemplateID string
	StartDate  time.Time
	EndDate    time.Time
	Duration   int // minutes
	Timezone   string
}

// GetAvailableSlots returns available time slots for booking
func (s *AvailabilityService) GetAvailableSlots(ctx context.Context, input GetAvailableSlotsInput) ([]models.TimeSlot, error) {
	// Load template
	template, err := s.repos.Template.GetByID(ctx, input.TemplateID)
	if err != nil || template == nil {
		return nil, err
	}

	// Parse invitee timezone
	inviteeLoc, err := time.LoadLocation(input.Timezone)
	if err != nil {
		inviteeLoc = time.UTC
	}

	// Calculate date range
	now := time.Now()
	minNotice := time.Duration(template.MinNoticeMinutes) * time.Minute
	earliestStart := now.Add(minNotice)

	// Ensure start date is not before earliest allowed
	if input.StartDate.Before(earliestStart) {
		input.StartDate = earliestStart
	}

	// Ensure end date is within max schedule days
	maxEnd := now.AddDate(0, 0, template.MaxScheduleDays)
	if input.EndDate.After(maxEnd) {
		input.EndDate = maxEnd
	}

	// Load pooled hosts for this template
	pooledHosts, err := s.repos.TemplateHost.GetByTemplateIDWithHost(ctx, input.TemplateID)
	if err != nil {
		return nil, err
	}

	// Get required hosts (non-optional) for availability calculation
	var requiredHosts []*models.TemplateHost
	for _, th := range pooledHosts {
		if !th.IsOptional {
			requiredHosts = append(requiredHosts, th)
		}
	}

	// If no pooled hosts or only one required host, use single-host logic
	if len(requiredHosts) <= 1 {
		slots, err := s.getSingleHostSlots(ctx, input, template, earliestStart)
		if err != nil {
			return nil, err
		}
		return convertToInviteeTimezone(slots, inviteeLoc), nil
	}

	// For pooled templates, compute intersection of all required hosts' availability
	slots, err := s.getPooledHostSlots(ctx, input, template, requiredHosts, earliestStart)
	if err != nil {
		return nil, err
	}

	return convertToInviteeTimezone(slots, inviteeLoc), nil
}

// getSingleHostSlots returns available slots for a single host
func (s *AvailabilityService) getSingleHostSlots(ctx context.Context, input GetAvailableSlotsInput, template *models.MeetingTemplate, earliestStart time.Time) ([]models.TimeSlot, error) {
	// Load host
	host, err := s.repos.Host.GetByID(ctx, input.HostID)
	if err != nil || host == nil {
		return nil, err
	}

	// Parse host timezone
	hostLoc, err := time.LoadLocation(host.Timezone)
	if err != nil {
		hostLoc = time.UTC
	}

	// Get working hours
	workingHours, err := s.repos.WorkingHours.GetByHostID(ctx, input.HostID)
	if err != nil {
		return nil, err
	}

	// Get existing bookings (as busy times)
	bookings, err := s.repos.Booking.GetByHostIDAndTimeRange(ctx, input.HostID, input.StartDate, input.EndDate)
	if err != nil {
		return nil, err
	}

	// Get calendar busy times
	calendarBusy, err := s.calendar.GetBusyTimes(ctx, input.HostID, input.StartDate, input.EndDate)
	if err != nil {
		// Log but don't fail - calendar might be disconnected
		calendarBusy = nil
	}

	// Combine all busy times
	var busySlots []models.TimeSlot

	// Add booking busy times (with buffers)
	for _, b := range bookings {
		busyStart := b.StartTime.Add(-time.Duration(template.PreBufferMinutes) * time.Minute)
		busyEnd := b.EndTime.Add(time.Duration(template.PostBufferMinutes) * time.Minute)
		busySlots = append(busySlots, models.TimeSlot{Start: busyStart, End: busyEnd})
	}

	// Add calendar busy times
	busySlots = append(busySlots, calendarBusy...)

	// Sort and merge overlapping busy slots
	busySlots = mergeTimeSlots(busySlots)

	// Generate available slots
	duration := time.Duration(input.Duration) * time.Minute
	slotIncrement := 15 * time.Minute // 15-minute slot intervals

	var availableSlots []models.TimeSlot

	// Parse template availability rules
	var templateRules *TemplateAvailabilityRules
	if template.AvailabilityRules != nil {
		templateRules = parseAvailabilityRules(template.AvailabilityRules)
	}

	// Iterate through each day
	current := input.StartDate.Truncate(24 * time.Hour)
	for current.Before(input.EndDate) {
		daySlots := s.getSlotsForDay(current, workingHours, hostLoc, duration, slotIncrement, busySlots, earliestStart, input.EndDate, templateRules)
		availableSlots = append(availableSlots, daySlots...)
		current = current.AddDate(0, 0, 1)
	}

	return availableSlots, nil
}

// getPooledHostSlots computes the intersection of availability for multiple required hosts
func (s *AvailabilityService) getPooledHostSlots(ctx context.Context, input GetAvailableSlotsInput, template *models.MeetingTemplate, requiredHosts []*models.TemplateHost, earliestStart time.Time) ([]models.TimeSlot, error) {
	if len(requiredHosts) == 0 {
		return nil, nil
	}

	// Get first host's slots as base
	firstHostInput := GetAvailableSlotsInput{
		HostID:     requiredHosts[0].HostID,
		TemplateID: input.TemplateID,
		StartDate:  input.StartDate,
		EndDate:    input.EndDate,
		Duration:   input.Duration,
		Timezone:   "UTC", // Use UTC internally, convert at the end
	}
	slots, err := s.getSingleHostSlots(ctx, firstHostInput, template, earliestStart)
	if err != nil {
		return nil, err
	}

	// Intersect with each additional required host's availability
	for _, th := range requiredHosts[1:] {
		hostInput := GetAvailableSlotsInput{
			HostID:     th.HostID,
			TemplateID: input.TemplateID,
			StartDate:  input.StartDate,
			EndDate:    input.EndDate,
			Duration:   input.Duration,
			Timezone:   "UTC",
		}
		hostSlots, err := s.getSingleHostSlots(ctx, hostInput, template, earliestStart)
		if err != nil {
			return nil, err
		}
		slots = intersectSlots(slots, hostSlots)

		// Early exit if no common slots
		if len(slots) == 0 {
			return slots, nil
		}
	}

	return slots, nil
}

// intersectSlots returns time slots that exist in both slices
// Slots must match exactly (same start and end time)
func intersectSlots(a, b []models.TimeSlot) []models.TimeSlot {
	if len(a) == 0 || len(b) == 0 {
		return nil
	}

	// Create a map of slots from b for O(1) lookup
	bSet := make(map[string]bool)
	for _, slot := range b {
		key := slot.Start.UTC().Format(time.RFC3339) + "|" + slot.End.UTC().Format(time.RFC3339)
		bSet[key] = true
	}

	// Find slots that exist in both
	var result []models.TimeSlot
	for _, slot := range a {
		key := slot.Start.UTC().Format(time.RFC3339) + "|" + slot.End.UTC().Format(time.RFC3339)
		if bSet[key] {
			result = append(result, slot)
		}
	}

	return result
}

// convertToInviteeTimezone converts all slots to the invitee's timezone
func convertToInviteeTimezone(slots []models.TimeSlot, loc *time.Location) []models.TimeSlot {
	for i := range slots {
		slots[i].Start = slots[i].Start.In(loc)
		slots[i].End = slots[i].End.In(loc)
	}
	return slots
}

// TemplateAvailabilityRules represents parsed availability rules from a template
type TemplateAvailabilityRules struct {
	Enabled bool
	Days    map[int]DayAvailability
}

// TimeInterval represents a single time interval with start and end times
type TimeInterval struct {
	Start string // HH:MM format
	End   string // HH:MM format
}

// DayAvailability represents availability for a specific day
// Supports multiple intervals per day (e.g., morning 9-12, afternoon 14-17)
type DayAvailability struct {
	Enabled   bool
	Intervals []TimeInterval
}

// parseAvailabilityRules parses JSONMap availability rules into a structured format
// Supports both old format (single start/end) and new format (intervals array) for backward compatibility
func parseAvailabilityRules(rules models.JSONMap) *TemplateAvailabilityRules {
	if rules == nil {
		return nil
	}

	result := &TemplateAvailabilityRules{
		Days: make(map[int]DayAvailability),
	}

	// Parse enabled flag
	if enabled, ok := rules["enabled"].(bool); ok {
		result.Enabled = enabled
	}

	// Parse days
	if days, ok := rules["days"].(map[string]interface{}); ok {
		for dayStr, dayData := range days {
			var dayNum int
			if _, err := fmt.Sscanf(dayStr, "%d", &dayNum); err != nil {
				continue
			}
			if dayMap, ok := dayData.(map[string]interface{}); ok {
				dayAvail := DayAvailability{}
				if enabled, ok := dayMap["enabled"].(bool); ok {
					dayAvail.Enabled = enabled
				}

				// Try new format first: intervals array
				if intervals, ok := dayMap["intervals"].([]interface{}); ok {
					for _, intervalData := range intervals {
						if intervalMap, ok := intervalData.(map[string]interface{}); ok {
							interval := TimeInterval{}
							if start, ok := intervalMap["start"].(string); ok {
								interval.Start = start
							}
							if end, ok := intervalMap["end"].(string); ok {
								interval.End = end
							}
							if interval.Start != "" && interval.End != "" {
								dayAvail.Intervals = append(dayAvail.Intervals, interval)
							}
						}
					}
				} else {
					// Fall back to old format: single start/end
					var start, end string
					if s, ok := dayMap["start"].(string); ok {
						start = s
					}
					if e, ok := dayMap["end"].(string); ok {
						end = e
					}
					if start != "" && end != "" {
						dayAvail.Intervals = []TimeInterval{{Start: start, End: end}}
					}
				}

				result.Days[dayNum] = dayAvail
			}
		}
	}

	return result
}

// getSlotsForDay returns available slots for a specific day
func (s *AvailabilityService) getSlotsForDay(
	day time.Time,
	workingHours []*models.WorkingHours,
	hostLoc *time.Location,
	duration time.Duration,
	increment time.Duration,
	busySlots []models.TimeSlot,
	earliestStart time.Time,
	latestEnd time.Time,
	templateRules *TemplateAvailabilityRules,
) []models.TimeSlot {
	// Get day of week (0=Sunday)
	dayOfWeek := int(day.In(hostLoc).Weekday())

	// If template has custom availability rules, use those instead of working hours
	if templateRules != nil && templateRules.Enabled {
		// Check if this day is enabled in template rules
		dayRule, dayExists := templateRules.Days[dayOfWeek]
		if !dayExists || !dayRule.Enabled || len(dayRule.Intervals) == 0 {
			return nil
		}

		var slots []models.TimeSlot
		dayInHostTz := day.In(hostLoc)

		// Iterate through all intervals for this day
		for _, interval := range dayRule.Intervals {
			startTime, err := time.ParseInLocation("15:04", interval.Start, hostLoc)
			if err != nil {
				continue
			}
			endTime, err := time.ParseInLocation("15:04", interval.End, hostLoc)
			if err != nil {
				continue
			}

			// Create full datetime for this day
			workStart := time.Date(
				dayInHostTz.Year(), dayInHostTz.Month(), dayInHostTz.Day(),
				startTime.Hour(), startTime.Minute(), 0, 0, hostLoc,
			)
			workEnd := time.Date(
				dayInHostTz.Year(), dayInHostTz.Month(), dayInHostTz.Day(),
				endTime.Hour(), endTime.Minute(), 0, 0, hostLoc,
			)

			slots = append(slots, s.generateSlotsInRange(workStart, workEnd, duration, increment, busySlots, earliestStart, latestEnd)...)
		}

		// Sort slots by start time (in case intervals were not in order)
		sort.Slice(slots, func(i, j int) bool {
			return slots[i].Start.Before(slots[j].Start)
		})

		return slots
	}

	// Fall back to working hours
	var dayWorkingHours []*models.WorkingHours
	for _, wh := range workingHours {
		if wh.DayOfWeek == dayOfWeek && wh.IsEnabled {
			dayWorkingHours = append(dayWorkingHours, wh)
		}
	}

	if len(dayWorkingHours) == 0 {
		return nil
	}

	var slots []models.TimeSlot

	for _, wh := range dayWorkingHours {
		// Parse working hours times
		startTime, err := time.ParseInLocation("15:04", wh.StartTime[:5], hostLoc)
		if err != nil {
			continue
		}
		endTime, err := time.ParseInLocation("15:04", wh.EndTime[:5], hostLoc)
		if err != nil {
			continue
		}

		// Create full datetime for this day
		dayInHostTz := day.In(hostLoc)
		workStart := time.Date(
			dayInHostTz.Year(), dayInHostTz.Month(), dayInHostTz.Day(),
			startTime.Hour(), startTime.Minute(), 0, 0, hostLoc,
		)
		workEnd := time.Date(
			dayInHostTz.Year(), dayInHostTz.Month(), dayInHostTz.Day(),
			endTime.Hour(), endTime.Minute(), 0, 0, hostLoc,
		)

		slots = append(slots, s.generateSlotsInRange(workStart, workEnd, duration, increment, busySlots, earliestStart, latestEnd)...)
	}

	return slots
}

// generateSlotsInRange generates available slots within a time range
func (s *AvailabilityService) generateSlotsInRange(
	workStart, workEnd time.Time,
	duration, increment time.Duration,
	busySlots []models.TimeSlot,
	earliestStart, latestEnd time.Time,
) []models.TimeSlot {
	var slots []models.TimeSlot

	slotStart := workStart
	for slotStart.Add(duration).Before(workEnd) || slotStart.Add(duration).Equal(workEnd) {
		slotEnd := slotStart.Add(duration)

		// Check constraints
		if slotStart.Before(earliestStart) || slotEnd.After(latestEnd) {
			slotStart = slotStart.Add(increment)
			continue
		}

		// Check if slot overlaps with any busy time
		if !slotOverlapsBusy(slotStart, slotEnd, busySlots) {
			slots = append(slots, models.TimeSlot{
				Start: slotStart.UTC(),
				End:   slotEnd.UTC(),
			})
		}

		slotStart = slotStart.Add(increment)
	}

	return slots
}

// slotOverlapsBusy checks if a slot overlaps with any busy time
func slotOverlapsBusy(start, end time.Time, busySlots []models.TimeSlot) bool {
	for _, busy := range busySlots {
		// Overlap occurs if: start < busy.End AND end > busy.Start
		if start.Before(busy.End) && end.After(busy.Start) {
			return true
		}
	}
	return false
}

// mergeTimeSlots sorts and merges overlapping time slots
func mergeTimeSlots(slots []models.TimeSlot) []models.TimeSlot {
	if len(slots) == 0 {
		return slots
	}

	// Sort by start time
	sort.Slice(slots, func(i, j int) bool {
		return slots[i].Start.Before(slots[j].Start)
	})

	// Merge overlapping
	merged := []models.TimeSlot{slots[0]}
	for i := 1; i < len(slots); i++ {
		last := &merged[len(merged)-1]
		current := slots[i]

		if current.Start.Before(last.End) || current.Start.Equal(last.End) {
			// Overlapping or adjacent, extend if necessary
			if current.End.After(last.End) {
				last.End = current.End
			}
		} else {
			// No overlap, add as new slot
			merged = append(merged, current)
		}
	}

	return merged
}

// GetWorkingHours returns working hours for a host
func (s *AvailabilityService) GetWorkingHours(ctx context.Context, hostID string) ([]*models.WorkingHours, error) {
	return s.repos.WorkingHours.GetByHostID(ctx, hostID)
}

// SetWorkingHours sets working hours for a host
func (s *AvailabilityService) SetWorkingHours(ctx context.Context, hostID string, hours []*models.WorkingHours) error {
	return s.repos.WorkingHours.SetForHost(ctx, hostID, hours)
}

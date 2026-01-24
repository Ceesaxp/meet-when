package services

import (
	"context"
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
	// Load host
	host, err := s.repos.Host.GetByID(ctx, input.HostID)
	if err != nil || host == nil {
		return nil, err
	}

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

	// Parse host timezone
	hostLoc, err := time.LoadLocation(host.Timezone)
	if err != nil {
		hostLoc = time.UTC
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

	// Iterate through each day
	current := input.StartDate.Truncate(24 * time.Hour)
	for current.Before(input.EndDate) {
		daySlots := s.getSlotsForDay(current, workingHours, hostLoc, duration, slotIncrement, busySlots, earliestStart, input.EndDate)
		availableSlots = append(availableSlots, daySlots...)
		current = current.AddDate(0, 0, 1)
	}

	// Convert to invitee timezone for display
	for i := range availableSlots {
		availableSlots[i].Start = availableSlots[i].Start.In(inviteeLoc)
		availableSlots[i].End = availableSlots[i].End.In(inviteeLoc)
	}

	return availableSlots, nil
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
) []models.TimeSlot {
	// Get day of week (0=Sunday)
	dayOfWeek := int(day.In(hostLoc).Weekday())

	// Find working hours for this day
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
		startTime, err := time.ParseInLocation("15:04", wh.StartTime, hostLoc)
		if err != nil {
			continue
		}
		endTime, err := time.ParseInLocation("15:04", wh.EndTime, hostLoc)
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

		// Generate slots within working hours
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

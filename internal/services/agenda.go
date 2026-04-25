package services

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

// DayEvents holds the events that touch a single calendar day.
// Events are NOT clipped to day boundaries — the original Start/End are
// preserved so the detail list can show the full duration of overnight events.
type DayEvents struct {
	DayStart time.Time
	DayEnd   time.Time
	Events   []AgendaEvent
}

// WeekView is the view model for a full 7-day week.
type WeekView struct {
	Calendars []*models.CalendarConnection
	Days      [7]DayEvents
	WeekStart time.Time
	HostTZ    *time.Location
}

// AgendaService composes the full agenda day view model.
type AgendaService struct {
	repos    *repository.Repositories
	calendar *CalendarService
}

// NewAgendaService creates a new AgendaService.
func NewAgendaService(repos *repository.Repositories, calendar *CalendarService) *AgendaService {
	return &AgendaService{repos: repos, calendar: calendar}
}

// AgendaView is the view model for a single agenda day.
type AgendaView struct {
	Calendars []*models.CalendarConnection
	Events    []AgendaEvent
	DayStart  time.Time
	DayEnd    time.Time
	HostTZ    *time.Location
}

// GetDay returns the AgendaView for the given host and date.
// The day window is 00:00–24:00 in the host's configured timezone.
func (s *AgendaService) GetDay(ctx context.Context, hostID string, date time.Time) (*AgendaView, error) {
	host, err := s.repos.Host.GetByID(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("load host: %w", err)
	}

	loc, err := time.LoadLocation(host.Timezone)
	if err != nil {
		loc = time.UTC
	}

	calendars, err := s.repos.Calendar.GetByHostID(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("load calendars: %w", err)
	}

	AssignColors(calendars)

	// Day window in host local time.
	localDate := date.In(loc)
	dayStart := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)

	events, err := s.calendar.GetAgendaEventsWithCalendars(ctx, calendars, host, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("fetch events: %w", err)
	}

	slices.SortFunc(events, func(a, b AgendaEvent) int {
		return a.Start.Compare(b.Start)
	})

	return &AgendaView{
		Calendars: calendars,
		Events:    events,
		DayStart:  dayStart,
		DayEnd:    dayEnd,
		HostTZ:    loc,
	}, nil
}

// GetWeek returns a WeekView for the 7-day period starting on weekStart (a
// Monday in the host's timezone). Events are assigned to each day they touch
// (event.Start < dayEnd AND event.End > dayStart) without clipping — the
// original Start/End are preserved so overnight events appear with their full
// duration in the detail list. Events appear in BOTH days they span across
// midnight. Each day's events are sorted by start time.
func (s *AgendaService) GetWeek(ctx context.Context, hostID string, weekStart time.Time) (*WeekView, error) {
	host, err := s.repos.Host.GetByID(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("load host: %w", err)
	}

	loc, err := time.LoadLocation(host.Timezone)
	if err != nil {
		loc = time.UTC
	}

	calendars, err := s.repos.Calendar.GetByHostID(ctx, hostID)
	if err != nil {
		return nil, fmt.Errorf("load calendars: %w", err)
	}

	AssignColors(calendars)

	// Normalize weekStart to 00:00 Monday in host timezone.
	localStart := weekStart.In(loc)
	monday := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)
	nextMonday := monday.AddDate(0, 0, 7)

	events, err := s.calendar.GetAgendaEventsWithCalendars(ctx, calendars, host, monday, nextMonday)
	if err != nil {
		return nil, fmt.Errorf("fetch events: %w", err)
	}

	// Build 7 day buckets and assign events to all days they touch.
	var days [7]DayEvents
	for i := range 7 {
		dayStart := monday.AddDate(0, 0, i)
		dayEnd := dayStart.Add(24 * time.Hour)
		days[i] = DayEvents{DayStart: dayStart, DayEnd: dayEnd}
	}

	for _, e := range events {
		for i := range days {
			// Event touches this day if it starts before dayEnd AND ends after dayStart.
			if e.Start.Before(days[i].DayEnd) && e.End.After(days[i].DayStart) {
				days[i].Events = append(days[i].Events, e)
			}
		}
	}

	// Sort each day's events by start time.
	for i := range days {
		slices.SortFunc(days[i].Events, func(a, b AgendaEvent) int {
			return a.Start.Compare(b.Start)
		})
	}

	return &WeekView{
		Calendars: calendars,
		Days:      days,
		WeekStart: monday,
		HostTZ:    loc,
	}, nil
}

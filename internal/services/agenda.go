package services

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/meet-when/meet-when/internal/models"
	"github.com/meet-when/meet-when/internal/repository"
)

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

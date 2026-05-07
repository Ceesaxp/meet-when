package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/database"
	"github.com/meet-when/meet-when/internal/handlers"
	"github.com/meet-when/meet-when/internal/middleware"
	"github.com/meet-when/meet-when/internal/repository"
	"github.com/meet-when/meet-when/internal/services"

	_ "time/tzdata"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize database
	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Error closing database: %v", err)
		}
	}()

	// Run migrations
	if err := database.Migrate(db, cfg.Database); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Initialize repositories
	repos := repository.NewRepositories(db, cfg.Database.Driver)

	// Initialize services
	svc := services.New(cfg, repos)

	// Start background services
	svc.Reminder.Start()
	defer svc.Reminder.Stop()

	svc.CalendarSync.Start()
	defer svc.CalendarSync.Stop()

	// Initialize handlers
	h := handlers.New(cfg, svc, repos)

	// Set up router
	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Landing page
	mux.HandleFunc("GET /{$}", h.Landing)

	// SEO files
	mux.HandleFunc("GET /robots.txt", h.RobotsTxt)
	mux.HandleFunc("GET /sitemap.xml", h.Sitemap)
	mux.HandleFunc("GET /llms.txt", h.LlmsTxt)

	// Public routes (booking pages) - prefixed with /m/ to avoid route conflicts
	mux.HandleFunc("GET /m/{tenant}/{host}", h.Public.HostPage)
	mux.HandleFunc("GET /m/{tenant}/{host}/{template}", h.Public.TemplatePage)
	mux.HandleFunc("GET /m/{tenant}/{host}/{template}/slots", h.Public.GetSlots)
	mux.HandleFunc("POST /m/{tenant}/{host}/{template}/book", h.Public.CreateBooking)
	mux.HandleFunc("GET /m/{tenant}/{host}/{template}/reschedule/{booking_id}", h.Public.RescheduleByID)
	mux.HandleFunc("GET /booking/{token}", h.Public.BookingStatus)
	mux.HandleFunc("POST /booking/{token}/cancel", h.Public.CancelBooking)
	mux.HandleFunc("GET /booking/{token}/calendar.ics", h.Public.DownloadICS)
	mux.HandleFunc("GET /booking/{token}/reschedule", h.Public.ReschedulePage)
	mux.HandleFunc("GET /booking/{token}/reschedule/slots", h.Public.GetRescheduleSlots)
	mux.HandleFunc("POST /booking/{token}/reschedule", h.Public.RescheduleBooking)

	// Auth routes
	mux.HandleFunc("GET /auth/login", h.Auth.LoginPage)
	mux.HandleFunc("POST /auth/login", h.Auth.Login)
	mux.HandleFunc("POST /auth/select-org", h.Auth.SelectOrg)
	mux.HandleFunc("GET /auth/register", h.Auth.RegisterPage)
	mux.HandleFunc("POST /auth/register", h.Auth.Register)
	mux.HandleFunc("POST /auth/logout", h.Auth.Logout)
	mux.HandleFunc("GET /signup/track", h.Auth.TrackSignupCTA)

	// Google auth flow (login/signup)
	mux.HandleFunc("GET /auth/google/signup", h.Auth.GoogleSignupStart)
	mux.HandleFunc("GET /auth/google/login", h.Auth.GoogleLoginStart)
	mux.HandleFunc("GET /auth/google/auth-callback", h.Auth.GoogleAuthCallback)
	mux.HandleFunc("GET /auth/register/complete-google", h.Auth.CompleteGoogleRegisterPage)
	mux.HandleFunc("POST /auth/register/complete-google", h.Auth.CompleteGoogleRegister)

	// OAuth callbacks (calendar/conferencing)
	mux.HandleFunc("GET /auth/google/callback", h.Auth.GoogleCallback)
	mux.HandleFunc("GET /auth/zoom/callback", h.Auth.ZoomCallback)

	// Protected dashboard routes
	dashboard := http.NewServeMux()
	dashboard.HandleFunc("GET /dashboard", h.Dashboard.Home)

	// Calendar management. The {id} on the connection-level routes is a
	// calendar_connections.id; the /sub/{id} routes operate on a
	// provider_calendars.id (an individual calendar within a connection).
	dashboard.HandleFunc("GET /dashboard/calendars", h.Dashboard.Calendars)
	dashboard.HandleFunc("POST /dashboard/calendars/connect/google", h.Dashboard.ConnectGoogle)
	dashboard.HandleFunc("POST /dashboard/calendars/connect/caldav", h.Dashboard.ConnectCalDAV)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/disconnect", h.Dashboard.DisconnectCalendar)
	dashboard.HandleFunc("POST /dashboard/conferencing/{provider}/disconnect", h.Dashboard.DisconnectConferencing)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/default", h.Dashboard.SetDefaultCalendar)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/refresh", h.Dashboard.RefreshCalendarSync)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/color", h.Dashboard.UpdateCalendarColor)
	dashboard.HandleFunc("POST /dashboard/calendars/sub/{id}/poll", h.Dashboard.ToggleSubCalendarPoll)
	dashboard.HandleFunc("POST /dashboard/calendars/sub/{id}/color", h.Dashboard.UpdateSubCalendarColor)
	dashboard.HandleFunc("POST /dashboard/calendars/sub/{id}/default", h.Dashboard.SetDefaultSubCalendar)
	dashboard.HandleFunc("GET /dashboard/agenda/day-detail", h.Dashboard.AgendaDayPartial)

	// Meeting templates
	dashboard.HandleFunc("GET /dashboard/templates", h.Dashboard.Templates)
	dashboard.HandleFunc("GET /dashboard/templates/new", h.Dashboard.NewTemplatePage)
	dashboard.HandleFunc("POST /dashboard/templates", h.Dashboard.CreateTemplate)
	dashboard.HandleFunc("GET /dashboard/templates/{id}", h.Dashboard.EditTemplatePage)
	dashboard.HandleFunc("PUT /dashboard/templates/{id}", h.Dashboard.UpdateTemplate)
	dashboard.HandleFunc("DELETE /dashboard/templates/{id}", h.Dashboard.DeleteTemplate)
	dashboard.HandleFunc("POST /dashboard/templates/{id}/duplicate", h.Dashboard.DuplicateTemplate)

	// Pooled hosts management
	dashboard.HandleFunc("POST /dashboard/templates/{id}/hosts", h.Dashboard.AddPooledHost)
	dashboard.HandleFunc("DELETE /dashboard/templates/{id}/hosts/{hostId}", h.Dashboard.RemovePooledHost)
	dashboard.HandleFunc("PUT /dashboard/templates/{id}/hosts/{hostId}", h.Dashboard.UpdatePooledHost)

	// Bookings management
	dashboard.HandleFunc("GET /dashboard/bookings", h.Dashboard.Bookings)
	dashboard.HandleFunc("GET /dashboard/bookings/{id}/details", h.Dashboard.BookingDetails)
	dashboard.HandleFunc("GET /dashboard/bookings/{id}/edit", h.Dashboard.EditBookingForm)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/edit", h.Dashboard.UpdateBooking)

	// Agenda view
	dashboard.HandleFunc("GET /dashboard/agenda", h.Dashboard.Agenda)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/approve", h.Dashboard.ApproveBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/reject", h.Dashboard.RejectBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/cancel", h.Dashboard.CancelBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/archive", h.Dashboard.ArchiveBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/unarchive", h.Dashboard.UnarchiveBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/archive-all", h.Dashboard.BulkArchiveBookings)
	dashboard.HandleFunc("POST /dashboard/bookings/archive-all-past", h.Dashboard.BulkArchivePastBookings)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/retry-calendar", h.Dashboard.RetryCalendarEvent)
	dashboard.HandleFunc("POST /dashboard/bookings/retry-calendar-all", h.Dashboard.BulkRetryCalendarEvents)

	// Contacts
	dashboard.HandleFunc("GET /dashboard/contacts", h.Dashboard.Contacts)
	dashboard.HandleFunc("GET /dashboard/contacts/{email}/bookings", h.Dashboard.ContactBookings)

	// Hosted events (host-driven scheduling). HTMX partials for autocomplete
	// + conflict-warning live under the same prefix so the auth middleware
	// covers them, and use distinct paths so they don't shadow {id}.
	dashboard.HandleFunc("GET /dashboard/events", h.DashboardEvents.List)
	dashboard.HandleFunc("GET /dashboard/events/new", h.DashboardEvents.NewForm)
	dashboard.HandleFunc("POST /dashboard/events", h.DashboardEvents.Create)
	dashboard.HandleFunc("GET /dashboard/events/check-conflicts", h.DashboardEvents.CheckConflicts)
	dashboard.HandleFunc("GET /dashboard/events/attendee-search", h.DashboardEvents.AttendeeSearch)
	dashboard.HandleFunc("GET /dashboard/events/{id}/details", h.DashboardEvents.Details)
	dashboard.HandleFunc("GET /dashboard/events/{id}/edit", h.DashboardEvents.EditForm)
	dashboard.HandleFunc("POST /dashboard/events/{id}/edit", h.DashboardEvents.Update)
	dashboard.HandleFunc("POST /dashboard/events/{id}/cancel", h.DashboardEvents.Cancel)
	dashboard.HandleFunc("POST /dashboard/events/{id}/archive", h.DashboardEvents.Archive)
	dashboard.HandleFunc("POST /dashboard/events/{id}/unarchive", h.DashboardEvents.Unarchive)
	dashboard.HandleFunc("POST /dashboard/events/{id}/retry-calendar", h.DashboardEvents.RetryCalendar)

	// Settings
	dashboard.HandleFunc("GET /dashboard/settings", h.Dashboard.Settings)
	dashboard.HandleFunc("PUT /dashboard/settings", h.Dashboard.UpdateSettings)
	dashboard.HandleFunc("PUT /dashboard/settings/working-hours", h.Dashboard.UpdateWorkingHours)

	// Audit logs (admin only)
	dashboard.HandleFunc("GET /dashboard/audit-logs", h.Dashboard.AuditLogs)

	// Onboarding routes (also protected)
	dashboard.HandleFunc("GET /onboarding/step/{step}", h.Onboarding.Step)
	dashboard.HandleFunc("POST /onboarding/working-hours", h.Onboarding.SaveWorkingHours)
	dashboard.HandleFunc("GET /onboarding/connect/google", h.Onboarding.ConnectGoogleCalendar)
	dashboard.HandleFunc("POST /onboarding/connect/caldav", h.Onboarding.ConnectCalDAV)
	dashboard.HandleFunc("GET /onboarding/skip/{step}", h.Onboarding.SkipStep)
	dashboard.HandleFunc("POST /onboarding/template", h.Onboarding.CreateTemplate)
	dashboard.HandleFunc("GET /onboarding/complete", h.Onboarding.Complete)

	// Apply auth middleware to dashboard and onboarding
	mux.Handle("/dashboard", middleware.RequireAuth(svc.Session)(dashboard))
	mux.Handle("/dashboard/", middleware.RequireAuth(svc.Session)(dashboard))
	mux.Handle("/onboarding/", middleware.RequireAuth(svc.Session)(dashboard))

	// API routes (legacy)
	mux.HandleFunc("GET /api/timezones", h.API.GetTimezones)

	// API v1 routes (JSON, for native clients)
	// Public auth endpoints (no session required)
	mux.HandleFunc("POST /api/v1/auth/login", h.APIV1.Login)
	mux.HandleFunc("POST /api/v1/auth/login/select-org", h.APIV1.SelectOrg)
	mux.HandleFunc("GET /api/v1/auth/google", h.APIV1.GoogleLogin)

	// Protected API v1 endpoints (require Bearer token or session cookie)
	apiv1 := http.NewServeMux()
	apiv1.HandleFunc("POST /api/v1/auth/logout", h.APIV1.Logout)
	apiv1.HandleFunc("GET /api/v1/me", h.APIV1.Me)
	apiv1.HandleFunc("GET /api/v1/bookings", h.APIV1.ListBookings)
	apiv1.HandleFunc("GET /api/v1/bookings/today", h.APIV1.TodayBookings)
	apiv1.HandleFunc("GET /api/v1/bookings/pending", h.APIV1.PendingBookings)
	apiv1.HandleFunc("GET /api/v1/bookings/{id}", h.APIV1.GetBooking)
	apiv1.HandleFunc("POST /api/v1/bookings/{id}/approve", h.APIV1.ApproveBooking)
	apiv1.HandleFunc("POST /api/v1/bookings/{id}/reject", h.APIV1.RejectBooking)
	apiv1.HandleFunc("POST /api/v1/bookings/{id}/cancel", h.APIV1.CancelBooking)

	mux.Handle("/api/v1/", middleware.RequireAuth(svc.Session)(apiv1))

	// Health check
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Error writing health check response: %v", err)
		}
	})

	// Apply global middleware
	// MethodOverride must be applied before route matching so PUT/DELETE form submissions work
	handler := middleware.Chain(
		mux,
		middleware.Logger,
		middleware.Recover,
		middleware.RequestID,
		middleware.MethodOverride,
	)

	// Create server
	server := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start auto-archive background worker
	archiveCtx, archiveCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		log.Println("Auto-archive worker started (runs every 24 hours, archives bookings older than 14 days)")
		for {
			select {
			case <-ticker.C:
				cutoff := time.Now().Add(-14 * 24 * time.Hour)
				count, err := repos.Booking.ArchiveOldBookings(archiveCtx, cutoff)
				if err != nil {
					log.Printf("Auto-archive error: %v", err)
				} else if count > 0 {
					log.Printf("Auto-archive: archived %d old bookings", count)
				}
			case <-archiveCtx.Done():
				log.Println("Auto-archive worker stopped")
				return
			}
		}
	}()

	// Start server in goroutine
	go func() {
		log.Printf("Server starting on %s", cfg.Server.Address)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down...")
	archiveCancel()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

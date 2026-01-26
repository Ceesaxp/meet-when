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
	h := handlers.New(cfg, svc)

	// Set up router
	mux := http.NewServeMux()

	// Static files
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Landing page
	mux.HandleFunc("GET /{$}", h.Landing)

	// Public routes (booking pages) - prefixed with /m/ to avoid route conflicts
	mux.HandleFunc("GET /m/{tenant}/{host}", h.Public.HostPage)
	mux.HandleFunc("GET /m/{tenant}/{host}/{template}", h.Public.TemplatePage)
	mux.HandleFunc("GET /m/{tenant}/{host}/{template}/slots", h.Public.GetSlots)
	mux.HandleFunc("POST /m/{tenant}/{host}/{template}/book", h.Public.CreateBooking)
	mux.HandleFunc("GET /booking/{token}", h.Public.BookingStatus)
	mux.HandleFunc("POST /booking/{token}/cancel", h.Public.CancelBooking)
	mux.HandleFunc("GET /booking/{token}/calendar.ics", h.Public.DownloadICS)
	mux.HandleFunc("GET /booking/{token}/reschedule", h.Public.ReschedulePage)
	mux.HandleFunc("GET /booking/{token}/reschedule/slots", h.Public.GetRescheduleSlots)
	mux.HandleFunc("POST /booking/{token}/reschedule", h.Public.RescheduleBooking)

	// Auth routes
	mux.HandleFunc("GET /auth/login", h.Auth.LoginPage)
	mux.HandleFunc("POST /auth/login", h.Auth.Login)
	mux.HandleFunc("GET /auth/register", h.Auth.RegisterPage)
	mux.HandleFunc("POST /auth/register", h.Auth.Register)
	mux.HandleFunc("POST /auth/logout", h.Auth.Logout)

	// OAuth callbacks
	mux.HandleFunc("GET /auth/google/callback", h.Auth.GoogleCallback)
	mux.HandleFunc("GET /auth/zoom/callback", h.Auth.ZoomCallback)

	// Protected dashboard routes
	dashboard := http.NewServeMux()
	dashboard.HandleFunc("GET /dashboard", h.Dashboard.Home)

	// Calendar management
	dashboard.HandleFunc("GET /dashboard/calendars", h.Dashboard.Calendars)
	dashboard.HandleFunc("POST /dashboard/calendars/connect/google", h.Dashboard.ConnectGoogle)
	dashboard.HandleFunc("POST /dashboard/calendars/connect/caldav", h.Dashboard.ConnectCalDAV)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/disconnect", h.Dashboard.DisconnectCalendar)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/default", h.Dashboard.SetDefaultCalendar)
	dashboard.HandleFunc("POST /dashboard/calendars/{id}/refresh", h.Dashboard.RefreshCalendarSync)

	// Meeting templates
	dashboard.HandleFunc("GET /dashboard/templates", h.Dashboard.Templates)
	dashboard.HandleFunc("GET /dashboard/templates/new", h.Dashboard.NewTemplatePage)
	dashboard.HandleFunc("POST /dashboard/templates", h.Dashboard.CreateTemplate)
	dashboard.HandleFunc("GET /dashboard/templates/{id}", h.Dashboard.EditTemplatePage)
	dashboard.HandleFunc("PUT /dashboard/templates/{id}", h.Dashboard.UpdateTemplate)
	dashboard.HandleFunc("DELETE /dashboard/templates/{id}", h.Dashboard.DeleteTemplate)
	dashboard.HandleFunc("POST /dashboard/templates/{id}/duplicate", h.Dashboard.DuplicateTemplate)

	// Bookings management
	dashboard.HandleFunc("GET /dashboard/bookings", h.Dashboard.Bookings)
	dashboard.HandleFunc("GET /dashboard/bookings/{id}/details", h.Dashboard.BookingDetails)

	// Agenda view
	dashboard.HandleFunc("GET /dashboard/agenda", h.Dashboard.Agenda)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/approve", h.Dashboard.ApproveBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/reject", h.Dashboard.RejectBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/cancel", h.Dashboard.CancelBooking)
	dashboard.HandleFunc("POST /dashboard/bookings/{id}/archive", h.Dashboard.ArchiveBooking)

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

	// API routes
	mux.HandleFunc("GET /api/timezones", h.API.GetTimezones)

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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server stopped")
}

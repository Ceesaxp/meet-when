# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Meet-When is a self-hosted scheduling system (like Calendly) built in Go with HTMX for the frontend. It supports multi-tenant architecture where hosts can create meeting templates that external invitees can book.

## Common Commands

```bash
# Build and run locally (requires .env file)
make dev

# Run tests
make test

# Run a single test
go test -v ./internal/services -run TestFunctionName

# Format code
make fmt

# Lint (requires golangci-lint)
make lint

# Docker development (Postgres + Mailhog)
make dev-docker

# Stop Docker containers
make docker-down
```

## Architecture

### Layer Structure
The app follows a clean architecture with clear separation:

- **cmd/server/main.go**: Entry point, wires all dependencies and sets up HTTP routes
- **internal/config**: Environment-based configuration (loads from `.env`)
- **internal/models**: Domain entities with custom JSON/SQL types for SQLite/Postgres compatibility
- **internal/repository**: Data access layer with driver-agnostic SQL (uses `q()` helper to convert `$1` to `?` for SQLite)
- **internal/services**: Business logic layer
- **internal/handlers**: HTTP handlers organized by domain (Auth, Public, Dashboard)
- **internal/middleware**: HTTP middleware (auth, logging, recovery)

### Key Services
- **AuthService**: Registration, login, OAuth callbacks (Google, Zoom)
- **SessionService**: Cookie-based session management
- **CalendarService**: Google Calendar, iCloud, CalDAV integration
- **ConferencingService**: Google Meet, Zoom link generation
- **AvailabilityService**: Calculates available time slots from working hours + calendar busy times
- **BookingService**: Creates bookings, handles approval flow, sends notifications
- **TemplateService**: Meeting template CRUD with audit logging

### Database
- Supports both SQLite (development) and PostgreSQL (production)
- Migrations in `migrations/` directory (driver-specific subdirectories supported)
- Custom `SQLiteTime` type in models handles time parsing differences between drivers
- Repository methods use `q(driver, query)` helper for placeholder conversion

### Frontend
- HTMX + server-rendered HTML templates
- Templates in `templates/` with layouts (`base.html`, `dashboard.html`) and pages
- Static files served from `static/`

#### Go Template Rendering Note
When using Go's `html/template` package, avoid parsing all templates together with `ParseGlob`. If multiple page templates define the same block (e.g., `{{define "content"}}`), only the **last definition survives** in the shared namespace.

**Solution**: Load each page template separately using `ParseFiles`, combining it with layouts and partials into an isolated template instance. Store these in a `map[string]*template.Template` keyed by page name. This ensures each page's `{{define}}` blocks don't conflict with others. See `internal/handlers/handlers.go` for the implementation.

### Multi-tenancy
- Hard tenant isolation via `tenant_id` on most tables
- Public booking URLs: `/m/{tenant}/{host}/{template}`
- Dashboard routes protected by auth middleware

## Configuration

All configuration via environment variables (see `internal/config/config.go`):
- `DB_DRIVER`: `sqlite` or `postgres`
- `DB_NAME`: Database name or SQLite file path
- `GOOGLE_CLIENT_ID/SECRET`: For Google Calendar OAuth
- `ZOOM_CLIENT_ID/SECRET`: For Zoom OAuth
- `EMAIL_PROVIDER`: `smtp` or `mailgun`


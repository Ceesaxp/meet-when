# MeetWhen

A self-hosted scheduling system (like Calendly) that allows external users to book meetings based on a host's real availability across multiple calendars, with automatic video-conference creation.

## Features

- **Multi-Calendar Integration** — Connect Google Calendar, iCloud, or any CalDAV-compatible calendar to show real-time availability
- **Video Conferencing** — Automatically generate Google Meet or Zoom links for confirmed bookings
- **Meeting Templates** — Create reusable meeting types with custom durations, questions, and approval workflows
- **Booking Management** — Approve, reschedule, or cancel bookings from a central dashboard
- **Multi-Tenant Architecture** — Support multiple organizations and hosts with isolated data
- **Email Notifications** — Automatic confirmation and reminder emails via SMTP or Mailgun
- **Self-Hosted** — Run on your own infrastructure with SQLite or PostgreSQL

## Quick Start

### Prerequisites

- Go 1.21+
- (Optional) Docker & Docker Compose for containerized deployment

### Local Development

1. Clone the repository:
   ```bash
   git clone https://github.com/Ceesaxp/meet-when.git
   cd meet-when
   ```

2. Copy the example environment file and configure:
   ```bash
   cp .env.example .env
   # Edit .env with your settings
   ```

3. Run the application:
   ```bash
   make dev
   ```

4. Open http://localhost:8080 in your browser

### Docker Development

For a full development environment with PostgreSQL and Mailhog:

```bash
make dev-docker
```

This starts:
- MeetWhen app on port 8080
- PostgreSQL database
- Mailhog for email testing (UI on port 8025)

## Configuration

All configuration is via environment variables:

### Server
| Variable | Default | Description |
|----------|---------|-------------|
| `SERVER_ADDRESS` | `:8080` | HTTP server bind address |
| `BASE_URL` | `http://localhost:8080` | Public URL for links in emails |

### Database
| Variable | Default | Description |
|----------|---------|-------------|
| `DB_DRIVER` | `sqlite` | Database driver (`sqlite` or `postgres`) |
| `DB_NAME` | `meetwhen.db` | Database name or SQLite file path |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_USER` | `meetwhen` | PostgreSQL user |
| `DB_PASSWORD` | `meetwhen` | PostgreSQL password |

### OAuth (Calendar & Conferencing)
| Variable | Description |
|----------|-------------|
| `GOOGLE_CLIENT_ID` | Google OAuth client ID |
| `GOOGLE_CLIENT_SECRET` | Google OAuth client secret |
| `GOOGLE_REDIRECT_URL` | Google OAuth callback URL |
| `ZOOM_CLIENT_ID` | Zoom OAuth client ID |
| `ZOOM_CLIENT_SECRET` | Zoom OAuth client secret |
| `ZOOM_REDIRECT_URL` | Zoom OAuth callback URL |

### Email
| Variable | Default | Description |
|----------|---------|-------------|
| `EMAIL_PROVIDER` | `smtp` | Email provider (`smtp` or `mailgun`) |
| `EMAIL_FROM_ADDRESS` | `noreply@localhost` | Sender email address |
| `EMAIL_FROM_NAME` | `Meet When` | Sender display name |
| `SMTP_HOST` | `localhost` | SMTP server host |
| `SMTP_PORT` | `587` | SMTP server port |
| `SMTP_USER` | | SMTP username |
| `SMTP_PASSWORD` | | SMTP password |
| `MAILGUN_DOMAIN` | | Mailgun domain |
| `MAILGUN_API_KEY` | | Mailgun API key |

### Application
| Variable | Default | Description |
|----------|---------|-------------|
| `APP_ENV` | `development` | Environment (`development` or `production`) |
| `MAX_SCHEDULING_DAYS` | `90` | How far ahead guests can book |
| `SESSION_DURATION_HOURS` | `24` | Session cookie lifetime |
| `DEFAULT_TIMEZONE` | `UTC` | Default timezone for new users |
| `ENCRYPTION_KEY` | | 32-byte key for encrypting OAuth tokens (required in production) |

## Architecture

```
cmd/server/          # Application entry point
internal/
  config/            # Environment-based configuration
  handlers/          # HTTP handlers (Auth, Public, Dashboard)
  middleware/        # Auth, logging, recovery middleware
  models/            # Domain entities
  repository/        # Data access layer (SQLite/Postgres)
  services/          # Business logic
migrations/          # Database migrations
static/              # CSS, JavaScript, images
templates/           # HTML templates (layouts, pages, partials)
```

### Key Services

- **AuthService** — Registration, login, OAuth callbacks
- **CalendarService** — Google Calendar, iCloud, CalDAV integration
- **ConferencingService** — Google Meet, Zoom link generation
- **AvailabilityService** — Calculates slots from working hours minus busy times
- **BookingService** — Booking lifecycle, approvals, notifications
- **TemplateService** — Meeting template CRUD with audit logging

## Development

```bash
# Run tests
make test

# Format code
make fmt

# Lint (requires golangci-lint)
make lint

# Build binary
make build
```

## Deployment

### Docker

```bash
# Build image
make docker-build

# Run production stack
make prod
```

### Manual

1. Build the binary:
   ```bash
   make build
   ```

2. Set environment variables (see Configuration above)

3. Run migrations and start:
   ```bash
   ./server
   ```

## Public URLs

Booking pages are accessible at:
- Host page: `/m/{tenant}/{host}`
- Meeting template: `/m/{tenant}/{host}/{template}`
- Booking status: `/booking/{token}`

## License

MIT

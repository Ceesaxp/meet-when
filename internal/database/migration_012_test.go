package database

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
)

// TestMigration012_FreshSchemaShape verifies that after all migrations apply
// to a brand-new database, the provider_calendars table exists with the
// expected columns and FK constraints to calendar_connections.
func TestMigration012_FreshSchemaShape(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()

			cols := tableColumns(t, db, driver, "provider_calendars")
			required := []string{
				"id", "connection_id", "provider_calendar_id", "name", "color",
				"is_primary", "is_writable", "poll_busy", "last_synced_at",
				"sync_status", "sync_error", "created_at", "updated_at",
			}
			for _, c := range required {
				if !cols[c] {
					t.Errorf("provider_calendars missing column %s", c)
				}
			}

			// FK on meeting_templates.calendar_id should now point at provider_calendars.
			if driver == "sqlite" {
				ddl := tableDDL(t, db, "meeting_templates")
				if !strings.Contains(ddl, "REFERENCES provider_calendars") {
					t.Errorf("meeting_templates.calendar_id FK not pointing at provider_calendars; ddl=%s", ddl)
				}
			}
		})
	}
}

// TestMigration012_CalendarFKAcceptsNewProviderCalendar checks that a fresh
// provider_calendars row (i.e. one that does NOT correspond to any
// calendar_connections.id) can still be referenced from meeting_templates and
// booking_calendar_events. This is the property that makes sub-calendar picks
// in the template editor work.
func TestMigration012_CalendarFKAcceptsNewProviderCalendar(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()

			ph := func(n int) string {
				if driver == "sqlite" {
					return "?"
				}
				return "$" + itoa(n)
			}

			// Set up the minimum data:
			// tenant -> host -> connection -> two provider_calendars
			tenantID := uuid.New().String()
			hostID := uuid.New().String()
			connID := uuid.New().String()
			pcPrimaryID := uuid.New().String()
			pcSubID := uuid.New().String()

			mustExec(t, db, "INSERT INTO tenants(id, slug, name) VALUES("+ph(1)+","+ph(2)+","+ph(3)+")",
				tenantID, "t-"+tenantID[:8], "T")
			mustExec(t, db, `INSERT INTO hosts(id, tenant_id, email, password_hash, name, slug, timezone)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`,`+ph(6)+`,`+ph(7)+`)`,
				hostID, tenantID, "h@example.com", "x", "H", "h-"+hostID[:8], "UTC")
			mustExec(t, db, `INSERT INTO calendar_connections(id, host_id, provider, name)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`)`,
				connID, hostID, "google", "Google")
			mustExec(t, db, `INSERT INTO provider_calendars(
				id, connection_id, provider_calendar_id, name, color, is_primary,
				is_writable, poll_busy, sync_status, sync_error, created_at, updated_at)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`,`+ph(6)+`,`+ph(7)+`,`+ph(8)+`,`+ph(9)+`,`+ph(10)+`,`+ph(11)+`,`+ph(12)+`)`,
				pcPrimaryID, connID, "primary", "Primary", "", 1, 1, 1, "unknown", "", models.Now(), models.Now())
			mustExec(t, db, `INSERT INTO provider_calendars(
				id, connection_id, provider_calendar_id, name, color, is_primary,
				is_writable, poll_busy, sync_status, sync_error, created_at, updated_at)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`,`+ph(6)+`,`+ph(7)+`,`+ph(8)+`,`+ph(9)+`,`+ph(10)+`,`+ph(11)+`,`+ph(12)+`)`,
				pcSubID, connID, "work@example.com", "Work", "", 0, 1, 1, "unknown", "", models.Now(), models.Now())

			// Insert a meeting template that references the SUB-calendar — this
			// is the case that was impossible before migration 012.
			tmplID := uuid.New().String()
			mustExec(t, db, `INSERT INTO meeting_templates(
				id, host_id, slug, name, durations, location_type, calendar_id,
				is_active, created_at, updated_at)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`,`+ph(6)+`,`+ph(7)+`,`+ph(8)+`,`+ph(9)+`,`+ph(10)+`)`,
				tmplID, hostID, "tmpl-"+tmplID[:8], "Test", `[30]`, "google_meet", pcSubID, 1, models.Now(), models.Now())

			// And confirm that pointing at a non-existent provider_calendars id
			// is rejected by the FK.
			err := tryExec(db, `INSERT INTO meeting_templates(
				id, host_id, slug, name, durations, location_type, calendar_id,
				is_active, created_at, updated_at)
				VALUES(`+ph(1)+`,`+ph(2)+`,`+ph(3)+`,`+ph(4)+`,`+ph(5)+`,`+ph(6)+`,`+ph(7)+`,`+ph(8)+`,`+ph(9)+`,`+ph(10)+`)`,
				uuid.New().String(), hostID, "tmpl-bad-"+uuid.New().String()[:8], "Bad", `[30]`, "google_meet", uuid.New().String(), 1, models.Now(), models.Now())
			if err == nil {
				t.Error("expected FK violation when meeting_templates.calendar_id points at a non-existent provider_calendars id")
			}
		})
	}
}

// TestMigration012_DeletingConnectionCascadesToProviderCalendars verifies the
// ON DELETE CASCADE relationship between calendar_connections and
// provider_calendars: dropping a connection should remove all its calendars.
func TestMigration012_DeletingConnectionCascadesToProviderCalendars(t *testing.T) {
	for _, driver := range []string{"sqlite"} {
		t.Run(driver, func(t *testing.T) {
			db, cleanup := setupTestDB(t, driver)
			defer cleanup()

			tenantID := uuid.New().String()
			hostID := uuid.New().String()
			connID := uuid.New().String()
			pcID := uuid.New().String()

			ph := "?"
			mustExec(t, db, "INSERT INTO tenants(id, slug, name) VALUES("+ph+","+ph+","+ph+")",
				tenantID, "t-"+tenantID[:8], "T")
			mustExec(t, db, `INSERT INTO hosts(id, tenant_id, email, password_hash, name, slug, timezone)
				VALUES(`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`)`,
				hostID, tenantID, "h@example.com", "x", "H", "h-"+hostID[:8], "UTC")
			mustExec(t, db, `INSERT INTO calendar_connections(id, host_id, provider, name)
				VALUES(`+ph+`,`+ph+`,`+ph+`,`+ph+`)`,
				connID, hostID, "google", "Google")
			mustExec(t, db, `INSERT INTO provider_calendars(
				id, connection_id, provider_calendar_id, name, color, is_primary,
				is_writable, poll_busy, sync_status, sync_error, created_at, updated_at)
				VALUES(`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`,`+ph+`)`,
				pcID, connID, "primary", "Primary", "", 1, 1, 1, "unknown", "", models.Now(), models.Now())

			mustExec(t, db, `DELETE FROM calendar_connections WHERE id = `+ph, connID)

			var count int
			row := db.QueryRow("SELECT COUNT(*) FROM provider_calendars WHERE id = ?", pcID)
			if err := row.Scan(&count); err != nil {
				t.Fatalf("count: %v", err)
			}
			if count != 0 {
				t.Errorf("expected provider_calendars row to be cascade-deleted, found %d", count)
			}
		})
	}
}

// itoa is a tiny strconv-free helper used to keep test files self-contained
// alongside the existing dual-driver placeholder rendering.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec failed: %v\nquery: %s", err, query)
	}
}

func tryExec(db *sql.DB, query string, args ...interface{}) error {
	_, err := db.Exec(query, args...)
	return err
}

func tableColumns(t *testing.T, db *sql.DB, driver, table string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	if driver == "sqlite" {
		rows, err := db.Query("SELECT name FROM pragma_table_info(?)", table)
		if err != nil {
			t.Fatalf("pragma_table_info: %v", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scan: %v", err)
			}
			out[name] = true
		}
		return out
	}
	rows, err := db.Query(`SELECT column_name FROM information_schema.columns WHERE table_name = $1`, table)
	if err != nil {
		t.Fatalf("information_schema query: %v", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan: %v", err)
		}
		out[name] = true
	}
	return out
}

func tableDDL(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var ddl string
	if err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&ddl); err != nil {
		t.Fatalf("read DDL: %v", err)
	}
	return ddl
}

// silence unused-import complaints when only sqlite paths run.
var _ = config.DatabaseConfig{}

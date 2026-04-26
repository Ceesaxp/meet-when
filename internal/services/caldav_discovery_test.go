package services

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/models"
)

const sampleHomeSetMultistatus = `<?xml version="1.0" encoding="utf-8"?>
<multistatus xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <response>
    <href>/principals/users/alice/</href>
    <propstat>
      <prop>
        <current-user-principal><href>/principals/users/alice/</href></current-user-principal>
        <C:calendar-home-set><href>/calendars/alice/</href></C:calendar-home-set>
        <resourcetype><collection/></resourcetype>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`

const sampleHomeListMultistatus = `<?xml version="1.0" encoding="utf-8"?>
<multistatus xmlns="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:I="http://apple.com/ns/ical/">
  <response>
    <href>/calendars/alice/</href>
    <propstat>
      <prop>
        <resourcetype><collection/></resourcetype>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
  <response>
    <href>/calendars/alice/work/</href>
    <propstat>
      <prop>
        <displayname>Work</displayname>
        <I:calendar-color>#378ADD</I:calendar-color>
        <resourcetype><collection/><C:calendar/></resourcetype>
        <C:supported-calendar-component-set><C:comp name="VEVENT"/></C:supported-calendar-component-set>
        <current-user-privilege-set><privilege><write-content/></privilege></current-user-privilege-set>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
  <response>
    <href>/calendars/alice/holidays/</href>
    <propstat>
      <prop>
        <displayname>Holidays</displayname>
        <resourcetype><collection/><C:calendar/></resourcetype>
        <C:supported-calendar-component-set><C:comp name="VEVENT"/></C:supported-calendar-component-set>
        <current-user-privilege-set><privilege><read/></privilege></current-user-privilege-set>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
  <response>
    <href>/calendars/alice/contacts/</href>
    <propstat>
      <prop>
        <displayname>Contacts</displayname>
        <resourcetype><collection/></resourcetype>
      </prop>
      <status>HTTP/1.1 200 OK</status>
    </propstat>
  </response>
</multistatus>`

// TestParseCalDAVMultistatus_ExtractsHomeSet checks the small XML parser used
// for discovery understands current-user-principal and calendar-home-set.
func TestParseCalDAVMultistatus_ExtractsHomeSet(t *testing.T) {
	ms, err := parseCalDAVMultistatus([]byte(sampleHomeSetMultistatus))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(ms.Responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(ms.Responses))
	}
	prop := ms.Responses[0].Propstat[0].Prop
	if prop.CurrentUserPrincipal == nil || prop.CurrentUserPrincipal.Href != "/principals/users/alice/" {
		t.Errorf("current-user-principal not extracted: %+v", prop.CurrentUserPrincipal)
	}
	if prop.CalendarHomeSet == nil || prop.CalendarHomeSet.Href != "/calendars/alice/" {
		t.Errorf("calendar-home-set not extracted: %+v", prop.CalendarHomeSet)
	}
}

// TestListCalDAVCalendars_FiltersNonCalendarsAndDetectsWritability runs the
// full PROPFIND-driven discovery against an in-process CalDAV mock and checks:
//   - non-calendar collections are skipped
//   - writability is read off current-user-privilege-set
//   - the supplied URL is treated as a home-set when responses don't redirect
func TestListCalDAVCalendars_FiltersNonCalendarsAndDetectsWritability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PROPFIND" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		// Depth 0 = home-set discovery query; Depth 1 = list calendars under home-set.
		if r.Header.Get("Depth") == "0" {
			_, _ = w.Write([]byte(sampleHomeSetMultistatus))
		} else {
			_, _ = w.Write([]byte(sampleHomeListMultistatus))
		}
	}))
	defer srv.Close()

	cfg := minimalConfig()
	cal := NewCalendarService(cfg, nil)
	conn := &models.CalendarConnection{
		ID:             "conn-1",
		Provider:       models.CalendarProviderCalDAV,
		CalDAVURL:      srv.URL + "/dav/",
		CalDAVUsername: "alice",
		CalDAVPassword: "pw",
	}

	cals, err := cal.listCalDAVCalendars(context.Background(), conn)
	if err != nil {
		t.Fatalf("listCalDAVCalendars: %v", err)
	}
	if len(cals) != 2 {
		t.Fatalf("expected 2 calendars, got %d (%v)", len(cals), cals)
	}

	byName := map[string]*models.ProviderCalendar{}
	for _, c := range cals {
		byName[c.Name] = c
	}
	work := byName["Work"]
	if work == nil {
		t.Fatal("Work calendar missing")
	}
	if !work.IsWritable {
		t.Error("Work should be writable (write-content privilege)")
	}
	if work.Color != "#378ADD" {
		t.Errorf("Work color: want #378ADD, got %s", work.Color)
	}

	holidays := byName["Holidays"]
	if holidays == nil {
		t.Fatal("Holidays calendar missing")
	}
	if holidays.IsWritable {
		t.Error("Holidays should be read-only (only <read/> privilege)")
	}

	if byName["Contacts"] != nil {
		t.Error("Contacts collection should be filtered out (no calendar resourcetype)")
	}
}

// TestRefreshCalDAVCalendarList_FallsBackToConnectionURL verifies that when
// PROPFIND yields no calendar collections, we still surface the supplied URL
// as a single calendar so non-discoverable servers continue to work.
func TestRefreshCalDAVCalendarList_FallsBackToConnectionURL(t *testing.T) {
	// PROPFIND returns an empty multistatus.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><multistatus xmlns="DAV:"></multistatus>`))
	}))
	defer srv.Close()

	_, repos, cal := setupServiceTestDB(t)
	_, conn := seedHostAndConnection(t, repos, models.CalendarProviderCalDAV, "", srv.URL+"/dav/")

	saved, err := cal.refreshCalDAVCalendarList(context.Background(), conn)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected fallback single calendar, got %d", len(saved))
	}
	if saved[0].ProviderCalendarID != srv.URL+"/dav/" {
		t.Errorf("fallback ProviderCalendarID: got %s", saved[0].ProviderCalendarID)
	}
}

// TestResolveHref_RelativeAndAbsolute is a focused test for the URL resolution
// helper; CalDAV servers return mixed relative/absolute hrefs.
func TestResolveHref_RelativeAndAbsolute(t *testing.T) {
	base := mustParseURL(t, "https://caldav.example.com/dav/")
	if got := resolveHref(base, "/calendars/alice/"); got != "https://caldav.example.com/calendars/alice/" {
		t.Errorf("relative href resolution wrong: %s", got)
	}
	if got := resolveHref(base, "https://other.example.com/foo"); got != "https://other.example.com/foo" {
		t.Errorf("absolute href resolution wrong: %s", got)
	}
	if got := resolveHref(base, ""); got != "" {
		t.Errorf("empty href should return empty: %s", got)
	}
}

// minimalConfig returns a Config with just enough fields for CalendarService
// instantiation in tests that don't touch OAuth flows.
func minimalConfig() *config.Config {
	cfg := &config.Config{}
	cfg.Server.BaseURL = "http://test"
	return cfg
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return u
}

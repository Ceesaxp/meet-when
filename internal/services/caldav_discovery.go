package services

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/meet-when/meet-when/internal/models"
)

// caldavMultistatus mirrors the bits of a WebDAV multistatus document we care
// about. Namespaces are stripped via xml.Name matching.
type caldavMultistatus struct {
	XMLName   xml.Name         `xml:"multistatus"`
	Responses []caldavResponse `xml:"response"`
}

type caldavResponse struct {
	Href     string           `xml:"href"`
	Propstat []caldavPropstat `xml:"propstat"`
}

type caldavPropstat struct {
	Prop   caldavProp `xml:"prop"`
	Status string     `xml:"status"`
}

type caldavProp struct {
	DisplayName            string                  `xml:"displayname"`
	CalendarColor          string                  `xml:"calendar-color"`
	CurrentUserPrincipal   *caldavHrefHolder       `xml:"current-user-principal"`
	CalendarHomeSet        *caldavHrefHolder       `xml:"calendar-home-set"`
	ResourceType           *caldavResourceType     `xml:"resourcetype"`
	CalendarUserPrivileges *caldavPrivilegeSet     `xml:"current-user-privilege-set"`
	SupportedCalendarComps *caldavSupportedCompSet `xml:"supported-calendar-component-set"`
}

type caldavHrefHolder struct {
	Href string `xml:"href"`
}

type caldavResourceType struct {
	InnerXML string `xml:",innerxml"`
}

func (rt *caldavResourceType) IsCalendar() bool {
	if rt == nil {
		return false
	}
	return strings.Contains(rt.InnerXML, "calendar")
}

type caldavPrivilegeSet struct {
	InnerXML string `xml:",innerxml"`
}

func (p *caldavPrivilegeSet) HasWrite() bool {
	if p == nil {
		// Without privilege info, default to writable; the user will be told
		// otherwise on first write attempt.
		return true
	}
	xml := p.InnerXML
	return strings.Contains(xml, "write-content") || strings.Contains(xml, "<write/>") || strings.Contains(xml, "<write></write>") || strings.Contains(xml, ":write/>") || strings.Contains(xml, "all/")
}

type caldavSupportedCompSet struct {
	InnerXML string `xml:",innerxml"`
}

func (s *caldavSupportedCompSet) SupportsVEvent() bool {
	if s == nil {
		return true
	}
	return strings.Contains(strings.ToUpper(s.InnerXML), "VEVENT")
}

// listCalDAVCalendars discovers all calendar collections under a CalDAV
// endpoint. The supplied URL may be a discovery endpoint or already a
// calendar-home-set URL — we try the standard discovery chain
// (current-user-principal → calendar-home-set → calendars).
func (s *CalendarService) listCalDAVCalendars(ctx context.Context, conn *models.CalendarConnection) ([]*models.ProviderCalendar, error) {
	homeSetURL, err := s.discoverCalDAVHomeSet(ctx, conn)
	if err != nil {
		return nil, err
	}

	body := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav" xmlns:I="http://apple.com/ns/ical/">
  <D:prop>
    <D:resourcetype/>
    <D:displayname/>
    <D:current-user-privilege-set/>
    <C:supported-calendar-component-set/>
    <I:calendar-color/>
  </D:prop>
</D:propfind>`

	ms, err := s.caldavPropfind(ctx, conn, homeSetURL, "1", body)
	if err != nil {
		return nil, err
	}

	base, err := url.Parse(homeSetURL)
	if err != nil {
		return nil, err
	}

	var out []*models.ProviderCalendar
	for _, resp := range ms.Responses {
		var prop *caldavProp
		for i := range resp.Propstat {
			ps := &resp.Propstat[i]
			if !strings.Contains(ps.Status, "200") {
				continue
			}
			prop = &ps.Prop
			break
		}
		if prop == nil || !prop.ResourceType.IsCalendar() {
			continue
		}
		if !prop.SupportedCalendarComps.SupportsVEvent() {
			continue
		}
		absURL := resolveHref(base, resp.Href)
		name := strings.TrimSpace(prop.DisplayName)
		if name == "" {
			name = lastPathSegment(absURL)
		}
		isWritable := prop.CalendarUserPrivileges.HasWrite()
		out = append(out, &models.ProviderCalendar{
			ProviderCalendarID: absURL,
			Name:               name,
			Color:              normalizeColor(prop.CalendarColor),
			IsWritable:         isWritable,
		})
	}
	return out, nil
}

// discoverCalDAVHomeSet walks the standard CalDAV discovery chain to find the
// user's calendar-home-set URL. If the supplied connection URL already looks
// like a calendar-home-set (PROPFIND returns calendar collections), it is used
// directly.
func (s *CalendarService) discoverCalDAVHomeSet(ctx context.Context, conn *models.CalendarConnection) (string, error) {
	caldavURL := strings.TrimSpace(conn.CalDAVURL)
	if caldavURL == "" {
		return "", fmt.Errorf("connection has no CalDAV URL")
	}

	// Step 1: PROPFIND on the supplied URL to get current-user-principal.
	body := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <D:current-user-principal/>
    <D:resourcetype/>
    <C:calendar-home-set/>
  </D:prop>
</D:propfind>`

	ms, err := s.caldavPropfind(ctx, conn, caldavURL, "0", body)
	if err != nil {
		// On PROPFIND failure assume the supplied URL is already a calendar URL.
		return caldavURL, nil
	}

	base, err := url.Parse(caldavURL)
	if err != nil {
		return "", err
	}

	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if !strings.Contains(ps.Status, "200") {
				continue
			}
			if ps.Prop.CalendarHomeSet != nil && ps.Prop.CalendarHomeSet.Href != "" {
				return resolveHref(base, ps.Prop.CalendarHomeSet.Href), nil
			}
			if ps.Prop.CurrentUserPrincipal != nil && ps.Prop.CurrentUserPrincipal.Href != "" {
				principalURL := resolveHref(base, ps.Prop.CurrentUserPrincipal.Href)
				if home, err := s.calendarHomeSetForPrincipal(ctx, conn, principalURL); err == nil && home != "" {
					return home, nil
				}
			}
			if ps.Prop.ResourceType.IsCalendar() {
				return caldavURL, nil
			}
		}
	}

	// Fall back to using the supplied URL directly.
	return caldavURL, nil
}

func (s *CalendarService) calendarHomeSetForPrincipal(ctx context.Context, conn *models.CalendarConnection, principalURL string) (string, error) {
	body := `<?xml version="1.0" encoding="utf-8" ?>
<D:propfind xmlns:D="DAV:" xmlns:C="urn:ietf:params:xml:ns:caldav">
  <D:prop>
    <C:calendar-home-set/>
  </D:prop>
</D:propfind>`
	ms, err := s.caldavPropfind(ctx, conn, principalURL, "0", body)
	if err != nil {
		return "", err
	}
	base, err := url.Parse(principalURL)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if !strings.Contains(ps.Status, "200") {
				continue
			}
			if ps.Prop.CalendarHomeSet != nil && ps.Prop.CalendarHomeSet.Href != "" {
				return resolveHref(base, ps.Prop.CalendarHomeSet.Href), nil
			}
		}
	}
	return "", fmt.Errorf("calendar-home-set not found")
}

// caldavPropfind issues a PROPFIND request and returns the parsed multistatus.
func (s *CalendarService) caldavPropfind(ctx context.Context, conn *models.CalendarConnection, url, depth, body string) (*caldavMultistatus, error) {
	req, err := http.NewRequestWithContext(ctx, "PROPFIND", url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(conn.CalDAVUsername, conn.CalDAVPassword)
	req.Header.Set("Content-Type", "application/xml; charset=utf-8")
	req.Header.Set("Depth", depth)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("Error closing response body: %v", cerr)
		}
	}()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrCalendarAuth
	}
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("PROPFIND %s returned %d: %s", url, resp.StatusCode, string(raw))
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parseCalDAVMultistatus(raw)
}

func parseCalDAVMultistatus(raw []byte) (*caldavMultistatus, error) {
	var ms caldavMultistatus
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	dec.Strict = false
	// Unmarshal ignoring namespace prefixes.
	dec.DefaultSpace = ""
	if err := dec.Decode(&ms); err != nil {
		return nil, fmt.Errorf("parse multistatus: %w", err)
	}
	return &ms, nil
}

// resolveHref resolves an href (which may be relative) against a base URL.
func resolveHref(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	if u.IsAbs() {
		return u.String()
	}
	return base.ResolveReference(u).String()
}

func lastPathSegment(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	parts := strings.Split(strings.TrimSuffix(u.Path, "/"), "/")
	if len(parts) == 0 {
		return rawURL
	}
	last := parts[len(parts)-1]
	if decoded, err := url.PathUnescape(last); err == nil {
		return decoded
	}
	return last
}

// refreshCalDAVCalendarList re-enumerates the connection's calendars from the
// CalDAV server and upserts them into provider_calendars.
func (s *CalendarService) refreshCalDAVCalendarList(ctx context.Context, conn *models.CalendarConnection) ([]*models.ProviderCalendar, error) {
	discovered, err := s.listCalDAVCalendars(ctx, conn)
	if err != nil {
		return nil, err
	}

	// If discovery returned nothing, treat the supplied URL itself as a single
	// calendar so users with non-discoverable servers still work.
	if len(discovered) == 0 && conn.CalDAVURL != "" {
		discovered = []*models.ProviderCalendar{{
			ProviderCalendarID: conn.CalDAVURL,
			Name:               connectionFallbackName(conn),
			IsWritable:         true,
		}}
	}

	var saved []*models.ProviderCalendar
	keep := make([]string, 0, len(discovered))
	for i, d := range discovered {
		isPrimary := i == 0
		pc, err := s.repos.ProviderCalendar.UpsertFromProvider(
			ctx, conn.ID, d.ProviderCalendarID, d.Name, d.Color, isPrimary, d.IsWritable,
		)
		if err != nil {
			return nil, err
		}
		saved = append(saved, pc)
		keep = append(keep, d.ProviderCalendarID)
	}

	if err := s.repos.ProviderCalendar.DeleteMissing(ctx, conn.ID, keep); err != nil {
		log.Printf("[CALENDAR] DeleteMissing failed for connection %s: %v", conn.ID, err)
	}

	AssignProviderCalendarColors(saved)
	for _, pc := range saved {
		_ = s.repos.ProviderCalendar.UpdateColor(ctx, conn.HostID, pc.ID, pc.Color)
	}

	return saved, nil
}

func connectionFallbackName(conn *models.CalendarConnection) string {
	if conn.Name != "" {
		return conn.Name
	}
	if conn.Provider == models.CalendarProviderICloud {
		return "iCloud Calendar"
	}
	return "CalDAV Calendar"
}

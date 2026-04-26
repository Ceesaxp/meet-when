package handlers

import (
	"bytes"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
)

// TestColorSwatchFieldset_HonorsFormAction guards against the regression
// reported during review: when UpdateSubCalendarColor returns the partial
// with a FormAction pointed at /dashboard/calendars/sub/{id}/color, that URL
// must end up on the rendered <form>'s hx-post — not the connection-level
// /dashboard/calendars/{id}/color fallback.
func TestColorSwatchFieldset_HonorsFormAction(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFiles(filepath.Join("..", "..", "templates", "partials", "color_swatch_fieldset.html"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	data := map[string]interface{}{
		"CalendarID":   "pc-1",
		"CurrentColor": "#378ADD",
		"Palette":      []map[string]string{{"Hex": "#378ADD", "Name": "Blue"}},
		"FormAction":   "/dashboard/calendars/sub/pc-1/color",
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "color_swatch_fieldset.html", data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	html := out.String()

	if !strings.Contains(html, `hx-post="/dashboard/calendars/sub/pc-1/color"`) {
		t.Errorf("FormAction not honored, got: %s", html)
	}
	if strings.Contains(html, `hx-post="/dashboard/calendars/pc-1/color"`) {
		t.Errorf("partial fell through to the connection-level color route despite FormAction being set")
	}
}

// TestColorSwatchFieldset_DefaultsToConnectionRoute checks the legacy callers
// (those that don't pass FormAction) still render the connection-level route.
func TestColorSwatchFieldset_DefaultsToConnectionRoute(t *testing.T) {
	tmpl, err := template.New("").Funcs(templateFuncs()).ParseFiles(filepath.Join("..", "..", "templates", "partials", "color_swatch_fieldset.html"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	data := map[string]interface{}{
		"CalendarID":   "conn-1",
		"CurrentColor": "#378ADD",
		"Palette":      []map[string]string{{"Hex": "#378ADD", "Name": "Blue"}},
		// Intentionally no FormAction.
	}
	var out bytes.Buffer
	if err := tmpl.ExecuteTemplate(&out, "color_swatch_fieldset.html", data); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), `hx-post="/dashboard/calendars/conn-1/color"`) {
		t.Errorf("expected fallback to connection-level route, got: %s", out.String())
	}
}

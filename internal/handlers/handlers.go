package handlers

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/repository"
	"github.com/meet-when/meet-when/internal/services"
)

// Handlers holds all handler instances
type Handlers struct {
	cfg       *config.Config
	repos     *repository.Repositories
	services  *services.Services
	templates map[string]*template.Template

	Auth       *AuthHandler
	Public     *PublicHandler
	Dashboard  *DashboardHandler
	Onboarding *OnboardingHandler
	API        *APIHandler
}

// New creates all handlers
func New(cfg *config.Config, svc *services.Services, repos *repository.Repositories) *Handlers {
	// Load templates - each page gets its own template instance with layouts
	templates := loadTemplates()

	h := &Handlers{
		cfg:       cfg,
		repos:     repos,
		services:  svc,
		templates: templates,
	}

	h.Auth = &AuthHandler{handlers: h}
	h.Public = &PublicHandler{handlers: h}
	h.Dashboard = &DashboardHandler{handlers: h}
	h.Onboarding = &OnboardingHandler{handlers: h}
	h.API = &APIHandler{handlers: h}

	return h
}

// loadTemplates loads all templates, giving each page its own template set
func loadTemplates() map[string]*template.Template {
	templates := make(map[string]*template.Template)
	funcs := templateFuncs()

	// Find all layout files
	layoutFiles, _ := filepath.Glob("templates/layouts/*.html")

	// Find all page files
	pageFiles, _ := filepath.Glob("templates/pages/*.html")

	// Find all partial files
	partialFiles, _ := filepath.Glob("templates/partials/*.html")

	// Combine layouts and partials as base files
	baseFiles := append(layoutFiles, partialFiles...)

	// For each page, create a template that includes layouts + partials + that page
	for _, page := range pageFiles {
		pageName := filepath.Base(page)

		// Create a new template for this page
		files := append([]string{page}, baseFiles...)
		tmpl, err := template.New(pageName).Funcs(funcs).ParseFiles(files...)
		if err != nil {
			log.Printf("Error parsing template %s: %v", pageName, err)
			continue
		}

		templates[pageName] = tmpl
	}

	// Also load partials as standalone templates for HTMX responses
	for _, partial := range partialFiles {
		partialName := filepath.Base(partial)

		tmpl, err := template.New(partialName).Funcs(funcs).ParseFiles(partial)
		if err != nil {
			log.Printf("Error parsing partial %s: %v", partialName, err)
			continue
		}

		templates[partialName] = tmpl
	}

	return templates
}

// render renders a template with the given data
func (h *Handlers) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := h.templates[name]
	if !ok {
		log.Printf("Template not found: %s", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Error executing template %s: %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderPartial renders a partial template (for HTMX)
func (h *Handlers) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	tmpl, ok := h.templates[name]
	if !ok {
		log.Printf("Partial template not found: %s", name)
		http.Error(w, "Template not found", http.StatusInternalServerError)
		return
	}

	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("Error executing partial %s: %v", name, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// redirect performs an HTTP redirect
func (h *Handlers) redirect(w http.ResponseWriter, r *http.Request, url string) {
	// For HTMX requests, use HX-Redirect header
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// error renders an error page
func (h *Handlers) error(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.WriteHeader(status)
	h.render(w, "error.html", map[string]interface{}{
		"Status":  status,
		"Message": message,
	})
}

// Landing renders the landing page
func (h *Handlers) Landing(w http.ResponseWriter, r *http.Request) {
	h.render(w, "landing.html", map[string]interface{}{
		"Year":                   time.Now().Year(),
		"BaseURL":                strings.TrimRight(h.cfg.Server.BaseURL, "/"),
		"GoogleSiteVerification": h.cfg.App.GoogleSiteVerification,
		"GoogleAnalyticsID":      h.cfg.App.GoogleAnalyticsID,
	})
}

// RobotsTxt serves the robots.txt file
func (h *Handlers) RobotsTxt(w http.ResponseWriter, r *http.Request) {
	baseURL := strings.TrimRight(h.cfg.Server.BaseURL, "/")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `User-agent: *
Allow: /$
Allow: /llms.txt
Disallow: /m/
Disallow: /booking/
Disallow: /dashboard/
Disallow: /auth/
Disallow: /onboarding/
Disallow: /api/
Disallow: /*?*

User-agent: CCBot
Allow: /$
Allow: /llms.txt
Disallow: /

User-agent: ia_archiver
Allow: /$
Allow: /llms.txt
Disallow: /

Host: %s
Sitemap: %s/sitemap.xml
`, baseURL, baseURL)
}

// Sitemap serves the sitemap.xml file
func (h *Handlers) Sitemap(w http.ResponseWriter, r *http.Request) {
	baseURL := strings.TrimRight(h.cfg.Server.BaseURL, "/")

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url>
    <loc>%s/</loc>
    <changefreq>weekly</changefreq>
    <priority>1.0</priority>
  </url>
</urlset>
`, baseURL)
}

// LlmsTxt serves the llms.txt file for AI agents
func (h *Handlers) LlmsTxt(w http.ResponseWriter, r *http.Request) {
	baseURL := strings.TrimRight(h.cfg.Server.BaseURL, "/")

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprintf(w, `# Meet When

> Scheduling made simple. Share your availability, let others book time with you, and eliminate the back-and-forth.

Meet When is a self-hosted scheduling platform. Hosts connect their calendars, define availability, and share booking links. Invitees pick a time that works, and everyone gets a calendar invite.

## Features

- Calendar sync (Google Calendar, CalDAV, iCloud)
- Shareable booking pages per host and meeting type
- Automatic timezone conversion for invitees
- Video conferencing links (Google Meet, Zoom)
- Booking approval workflows
- Email notifications and reminders
- Multi-tenant and pooled host support

## Links

- Homepage: %s/
`, baseURL)
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDate":     formatDate,
		"formatTime":     formatTime,
		"formatDateTime": formatDateTime,
		"formatTimeHHMM": formatTimeHHMM,
		"timeAgo":        timeAgo,
		"duration":       duration,
		"add":            func(a, b int) int { return a + b },
		"sub":            func(a, b int) int { return a - b },
		"mul":            func(a, b int) int { return a * b },
		"div":            func(a, b int) int { return a / b },
		"seq": func(start, end int) []int {
			var result []int
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
			return result
		},
		"contains": func(slice []int, item int) bool {
			for _, i := range slice {
				if i == item {
					return true
				}
			}
			return false
		},
		"toMap": func(v interface{}) map[string]interface{} {
			if m, ok := v.(map[string]interface{}); ok {
				return m
			}
			return nil
		},
	}
}

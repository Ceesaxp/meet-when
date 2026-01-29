package handlers

import (
	"html/template"
	"log"
	"net/http"
	"path/filepath"
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
		"Year": time.Now().Year(),
	})
}

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDate":     formatDate,
		"formatTime":     formatTime,
		"formatDateTime": formatDateTime,
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

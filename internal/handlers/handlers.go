package handlers

import (
	"html/template"
	"net/http"
	"path/filepath"

	"github.com/meet-when/meet-when/internal/config"
	"github.com/meet-when/meet-when/internal/services"
)

// Handlers holds all handler instances
type Handlers struct {
	cfg       *config.Config
	services  *services.Services
	templates *template.Template

	Auth      *AuthHandler
	Public    *PublicHandler
	Dashboard *DashboardHandler
}

// New creates all handlers
func New(cfg *config.Config, svc *services.Services) *Handlers {
	// Load templates
	tmpl := template.Must(template.New("").Funcs(templateFuncs()).ParseGlob(filepath.Join("templates", "**", "*.html")))

	h := &Handlers{
		cfg:       cfg,
		services:  svc,
		templates: tmpl,
	}

	h.Auth = &AuthHandler{handlers: h}
	h.Public = &PublicHandler{handlers: h}
	h.Dashboard = &DashboardHandler{handlers: h}

	return h
}

// render renders a template with the given data
func (h *Handlers) render(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// renderPartial renders a partial template (for HTMX)
func (h *Handlers) renderPartial(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.templates.ExecuteTemplate(w, name, data); err != nil {
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

// templateFuncs returns custom template functions
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"formatDate": formatDate,
		"formatTime": formatTime,
		"formatDateTime": formatDateTime,
		"add": func(a, b int) int { return a + b },
		"sub": func(a, b int) int { return a - b },
		"mul": func(a, b int) int { return a * b },
		"div": func(a, b int) int { return a / b },
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
	}
}

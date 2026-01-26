package handlers

import (
	"encoding/json"
	"net/http"
)

// APIHandler handles API endpoints
type APIHandler struct {
	handlers *Handlers
}

// GetTimezones returns all IANA timezones grouped by region
func (h *APIHandler) GetTimezones(w http.ResponseWriter, r *http.Request) {
	groups := h.handlers.services.Timezone.GetTimezones()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours

	if err := json.NewEncoder(w).Encode(groups); err != nil {
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

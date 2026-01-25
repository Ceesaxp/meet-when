package handlers

import (
	"time"

	"github.com/meet-when/meet-when/internal/models"
)

// toTime converts various time types to time.Time
func toTime(t interface{}) time.Time {
	switch v := t.(type) {
	case time.Time:
		return v
	case models.SQLiteTime:
		return v.Time
	case *time.Time:
		if v != nil {
			return *v
		}
	case *models.SQLiteTime:
		if v != nil {
			return v.Time
		}
	}
	return time.Time{}
}

// formatDate formats a time as a date string
func formatDate(t interface{}) string {
	return toTime(t).Format("Monday, January 2, 2006")
}

// formatTime formats a time as a time string
func formatTime(t interface{}) string {
	return toTime(t).Format("3:04 PM")
}

// formatDateTime formats a time as a full datetime string
func formatDateTime(t interface{}) string {
	return toTime(t).Format("Monday, January 2, 2006 at 3:04 PM")
}

// PageData represents common page data
type PageData struct {
	Title       string
	Description string
	BaseURL     string
	Host        interface{}
	Tenant      interface{}
	Flash       *FlashMessage
	Data        interface{}
}

// FlashMessage represents a flash message
type FlashMessage struct {
	Type    string // success, error, warning, info
	Message string
}

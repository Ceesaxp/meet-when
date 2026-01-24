package handlers

import (
	"time"
)

// formatDate formats a time as a date string
func formatDate(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}

// formatTime formats a time as a time string
func formatTime(t time.Time) string {
	return t.Format("3:04 PM")
}

// formatDateTime formats a time as a full datetime string
func formatDateTime(t time.Time) string {
	return t.Format("Monday, January 2, 2006 at 3:04 PM")
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

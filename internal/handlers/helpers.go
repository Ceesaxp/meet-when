package handlers

import (
	"fmt"
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

// timeAgo formats a time as a relative string like "5 minutes ago" or "2 hours ago"
func timeAgo(t interface{}) string {
	tm := toTime(t)
	if tm.IsZero() {
		return "Never"
	}

	now := time.Now()
	diff := now.Sub(tm)

	// Handle future times (shouldn't happen but just in case)
	if diff < 0 {
		return "just now"
	}

	seconds := int(diff.Seconds())
	minutes := int(diff.Minutes())
	hours := int(diff.Hours())
	days := hours / 24

	switch {
	case seconds < 60:
		return "just now"
	case minutes == 1:
		return "1 minute ago"
	case minutes < 60:
		return fmt.Sprintf("%d minutes ago", minutes)
	case hours == 1:
		return "1 hour ago"
	case hours < 24:
		return fmt.Sprintf("%d hours ago", hours)
	case days == 1:
		return "1 day ago"
	case days < 7:
		return fmt.Sprintf("%d days ago", days)
	default:
		return tm.Format("Jan 2, 2006")
	}
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

package services

import (
	"sort"
	"time"
)

// TimezoneInfo represents a single timezone with its metadata
type TimezoneInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Offset      string `json:"offset"`
	OffsetMins  int    `json:"offset_mins"`
}

// TimezoneGroup represents a group of timezones by region
type TimezoneGroup struct {
	Region    string         `json:"region"`
	Timezones []TimezoneInfo `json:"timezones"`
}

// TimezoneService provides timezone-related operations
type TimezoneService struct {
	groups []TimezoneGroup
}

// NewTimezoneService creates a new timezone service with precomputed data
func NewTimezoneService() *TimezoneService {
	return &TimezoneService{
		groups: buildTimezoneGroups(),
	}
}

// GetTimezones returns all timezone groups
func (s *TimezoneService) GetTimezones() []TimezoneGroup {
	return s.groups
}

// GetAllTimezones returns a flat list of all timezones
func (s *TimezoneService) GetAllTimezones() []TimezoneInfo {
	var all []TimezoneInfo
	for _, g := range s.groups {
		all = append(all, g.Timezones...)
	}
	return all
}

// buildTimezoneGroups constructs the timezone data grouped by region
func buildTimezoneGroups() []TimezoneGroup {
	// Define common IANA timezones organized by region
	timezonesByRegion := map[string][]string{
		"Americas": {
			"America/New_York",
			"America/Chicago",
			"America/Denver",
			"America/Los_Angeles",
			"America/Phoenix",
			"America/Anchorage",
			"America/Toronto",
			"America/Vancouver",
			"America/Mexico_City",
			"America/Bogota",
			"America/Lima",
			"America/Santiago",
			"America/Buenos_Aires",
			"America/Sao_Paulo",
			"America/Caracas",
			"America/Halifax",
			"America/St_Johns",
			"Pacific/Honolulu",
		},
		"Europe": {
			"Europe/London",
			"Europe/Dublin",
			"Europe/Paris",
			"Europe/Berlin",
			"Europe/Amsterdam",
			"Europe/Brussels",
			"Europe/Madrid",
			"Europe/Rome",
			"Europe/Vienna",
			"Europe/Zurich",
			"Europe/Stockholm",
			"Europe/Oslo",
			"Europe/Copenhagen",
			"Europe/Helsinki",
			"Europe/Warsaw",
			"Europe/Prague",
			"Europe/Budapest",
			"Europe/Bucharest",
			"Europe/Athens",
			"Europe/Istanbul",
			"Europe/Moscow",
			"Europe/Kiev",
			"Europe/Lisbon",
		},
		"Asia": {
			"Asia/Dubai",
			"Asia/Riyadh",
			"Asia/Tehran",
			"Asia/Karachi",
			"Asia/Kolkata",
			"Asia/Dhaka",
			"Asia/Bangkok",
			"Asia/Ho_Chi_Minh",
			"Asia/Jakarta",
			"Asia/Singapore",
			"Asia/Kuala_Lumpur",
			"Asia/Manila",
			"Asia/Hong_Kong",
			"Asia/Shanghai",
			"Asia/Taipei",
			"Asia/Seoul",
			"Asia/Tokyo",
			"Asia/Vladivostok",
		},
		"Pacific": {
			"Pacific/Auckland",
			"Pacific/Fiji",
			"Pacific/Guam",
			"Pacific/Port_Moresby",
			"Australia/Sydney",
			"Australia/Melbourne",
			"Australia/Brisbane",
			"Australia/Perth",
			"Australia/Adelaide",
			"Australia/Darwin",
		},
		"Africa": {
			"Africa/Cairo",
			"Africa/Johannesburg",
			"Africa/Lagos",
			"Africa/Nairobi",
			"Africa/Casablanca",
			"Africa/Tunis",
			"Africa/Algiers",
		},
		"Atlantic": {
			"Atlantic/Azores",
			"Atlantic/Cape_Verde",
			"Atlantic/Reykjavik",
		},
		"Other": {
			"UTC",
		},
	}

	// Region display order
	regionOrder := []string{"Americas", "Europe", "Asia", "Pacific", "Africa", "Atlantic", "Other"}

	now := time.Now()
	groups := make([]TimezoneGroup, 0, len(regionOrder))

	for _, region := range regionOrder {
		tzNames, ok := timezonesByRegion[region]
		if !ok {
			continue
		}

		timezones := make([]TimezoneInfo, 0, len(tzNames))
		for _, tzName := range tzNames {
			loc, err := time.LoadLocation(tzName)
			if err != nil {
				continue
			}

			_, offset := now.In(loc).Zone()
			offsetMins := offset / 60
			offsetHours := offsetMins / 60
			offsetRemMins := offsetMins % 60
			if offsetRemMins < 0 {
				offsetRemMins = -offsetRemMins
			}

			var offsetStr string
			if offsetMins >= 0 {
				offsetStr = "UTC+"
			} else {
				offsetStr = "UTC"
			}

			if offsetRemMins == 0 {
				offsetStr += formatOffset(offsetHours)
			} else {
				offsetStr += formatOffsetWithMins(offsetHours, offsetRemMins)
			}

			displayName := formatDisplayName(tzName)

			timezones = append(timezones, TimezoneInfo{
				ID:          tzName,
				DisplayName: displayName,
				Offset:      offsetStr,
				OffsetMins:  offsetMins,
			})
		}

		// Sort by offset within each region
		sort.Slice(timezones, func(i, j int) bool {
			return timezones[i].OffsetMins < timezones[j].OffsetMins
		})

		groups = append(groups, TimezoneGroup{
			Region:    region,
			Timezones: timezones,
		})
	}

	return groups
}

// formatOffset formats an integer offset as string
func formatOffset(hours int) string {
	if hours >= 0 {
		return itoa(hours)
	}
	return itoa(hours)
}

// formatOffsetWithMins formats offset with minutes
func formatOffsetWithMins(hours, mins int) string {
	if hours >= 0 {
		return itoa(hours) + ":" + padZero(mins)
	}
	return itoa(hours) + ":" + padZero(mins)
}

// itoa converts int to string without importing strconv
func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	negative := n < 0
	if negative {
		n = -n
	}

	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// padZero pads a number with leading zero if needed
func padZero(n int) string {
	if n < 10 {
		return "0" + itoa(n)
	}
	return itoa(n)
}

// formatDisplayName converts IANA timezone to human-readable name
func formatDisplayName(tzName string) string {
	// Map of IANA timezone IDs to display names
	displayNames := map[string]string{
		// Americas
		"America/New_York":    "Eastern Time (US & Canada)",
		"America/Chicago":     "Central Time (US & Canada)",
		"America/Denver":      "Mountain Time (US & Canada)",
		"America/Los_Angeles": "Pacific Time (US & Canada)",
		"America/Phoenix":     "Arizona",
		"America/Anchorage":   "Alaska",
		"America/Toronto":     "Toronto",
		"America/Vancouver":   "Vancouver",
		"America/Mexico_City": "Mexico City",
		"America/Bogota":      "Bogota",
		"America/Lima":        "Lima",
		"America/Santiago":    "Santiago",
		"America/Buenos_Aires": "Buenos Aires",
		"America/Sao_Paulo":   "Sao Paulo",
		"America/Caracas":     "Caracas",
		"America/Halifax":     "Atlantic Time (Canada)",
		"America/St_Johns":    "Newfoundland",
		"Pacific/Honolulu":    "Hawaii",

		// Europe
		"Europe/London":     "London",
		"Europe/Dublin":     "Dublin",
		"Europe/Paris":      "Paris",
		"Europe/Berlin":     "Berlin",
		"Europe/Amsterdam":  "Amsterdam",
		"Europe/Brussels":   "Brussels",
		"Europe/Madrid":     "Madrid",
		"Europe/Rome":       "Rome",
		"Europe/Vienna":     "Vienna",
		"Europe/Zurich":     "Zurich",
		"Europe/Stockholm":  "Stockholm",
		"Europe/Oslo":       "Oslo",
		"Europe/Copenhagen": "Copenhagen",
		"Europe/Helsinki":   "Helsinki",
		"Europe/Warsaw":     "Warsaw",
		"Europe/Prague":     "Prague",
		"Europe/Budapest":   "Budapest",
		"Europe/Bucharest":  "Bucharest",
		"Europe/Athens":     "Athens",
		"Europe/Istanbul":   "Istanbul",
		"Europe/Moscow":     "Moscow",
		"Europe/Kiev":       "Kyiv",
		"Europe/Lisbon":     "Lisbon",

		// Asia
		"Asia/Dubai":        "Dubai",
		"Asia/Riyadh":       "Riyadh",
		"Asia/Tehran":       "Tehran",
		"Asia/Karachi":      "Karachi",
		"Asia/Kolkata":      "Mumbai, Kolkata, New Delhi",
		"Asia/Dhaka":        "Dhaka",
		"Asia/Bangkok":      "Bangkok",
		"Asia/Ho_Chi_Minh":  "Ho Chi Minh City",
		"Asia/Jakarta":      "Jakarta",
		"Asia/Singapore":    "Singapore",
		"Asia/Kuala_Lumpur": "Kuala Lumpur",
		"Asia/Manila":       "Manila",
		"Asia/Hong_Kong":    "Hong Kong",
		"Asia/Shanghai":     "Beijing, Shanghai",
		"Asia/Taipei":       "Taipei",
		"Asia/Seoul":        "Seoul",
		"Asia/Tokyo":        "Tokyo",
		"Asia/Vladivostok":  "Vladivostok",

		// Pacific / Australia
		"Pacific/Auckland":      "Auckland",
		"Pacific/Fiji":          "Fiji",
		"Pacific/Guam":          "Guam",
		"Pacific/Port_Moresby":  "Port Moresby",
		"Australia/Sydney":      "Sydney",
		"Australia/Melbourne":   "Melbourne",
		"Australia/Brisbane":    "Brisbane",
		"Australia/Perth":       "Perth",
		"Australia/Adelaide":    "Adelaide",
		"Australia/Darwin":      "Darwin",

		// Africa
		"Africa/Cairo":        "Cairo",
		"Africa/Johannesburg": "Johannesburg",
		"Africa/Lagos":        "Lagos",
		"Africa/Nairobi":      "Nairobi",
		"Africa/Casablanca":   "Casablanca",
		"Africa/Tunis":        "Tunis",
		"Africa/Algiers":      "Algiers",

		// Atlantic
		"Atlantic/Azores":     "Azores",
		"Atlantic/Cape_Verde": "Cape Verde",
		"Atlantic/Reykjavik":  "Reykjavik",

		// Other
		"UTC": "Coordinated Universal Time (UTC)",
	}

	if name, ok := displayNames[tzName]; ok {
		return name
	}

	// Fallback: extract city name from IANA ID
	for i := len(tzName) - 1; i >= 0; i-- {
		if tzName[i] == '/' {
			city := tzName[i+1:]
			// Replace underscores with spaces
			result := make([]byte, 0, len(city))
			for j := range len(city) {
				if city[j] == '_' {
					result = append(result, ' ')
				} else {
					result = append(result, city[j])
				}
			}
			return string(result)
		}
	}

	return tzName
}

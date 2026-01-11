package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ParseDuration parses duration strings in format "1h", "2d", "1w".
// Supports: h (hours), d (days), w (weeks).
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid duration format: %s", s)
	}

	unit := s[len(s)-1]
	valueStr := s[:len(s)-1]
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("invalid duration value: %s", valueStr)
	}

	switch unit {
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil
	case 'w':
		return time.Duration(value) * 7 * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown duration unit: %c (use h, d, or w)", unit)
	}
}

// FormatDuration formats a duration into a human-readable string.
// Examples: "2d 4h", "1w 2d", "3h".
func FormatDuration(d time.Duration) string {
	if d <= 0 {
		return "0h"
	}

	weeks := d / (7 * 24 * time.Hour)
	d -= weeks * 7 * 24 * time.Hour

	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour

	hours := d / time.Hour

	var parts []string
	if weeks > 0 {
		parts = append(parts, fmt.Sprintf("%dw", weeks))
	}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}

	return strings.Join(parts, " ")
}

// CalculateWorkingTime calculates working time between start and end,
// excluding weekends (Saturday, Sunday) and holidays stored in the database.
func CalculateWorkingTime(db *gorm.DB, repoID uint, start, end time.Time) time.Duration {
	if end.Before(start) {
		return 0
	}

	// Fetch holidays for this repository
	var holidays []models.Holiday
	db.Where("repository_id = ?", repoID).Find(&holidays)

	// Create a set of holiday dates for fast lookup (normalized to date only)
	holidaySet := make(map[string]bool)
	for _, h := range holidays {
		dateKey := h.Date.Format("2006-01-02")
		holidaySet[dateKey] = true
	}

	// Count working hours
	var totalHours float64

	// Normalize start to beginning of hour
	current := start.Truncate(time.Hour)

	for current.Before(end) {
		weekday := current.Weekday()
		dateKey := current.Format("2006-01-02")

		// Skip weekends and holidays
		if weekday != time.Saturday && weekday != time.Sunday && !holidaySet[dateKey] {
			// Calculate hours contribution for this slot
			slotEnd := current.Add(time.Hour)
			if slotEnd.After(end) {
				slotEnd = end
			}

			slotStart := current
			if slotStart.Before(start) {
				slotStart = start
			}

			contribution := slotEnd.Sub(slotStart).Hours()
			totalHours += contribution
		}

		current = current.Add(time.Hour)
	}

	return time.Duration(totalHours * float64(time.Hour))
}

// CheckSLAStatus checks if elapsed time exceeds the SLA threshold.
// Returns whether SLA is exceeded and the percentage of threshold used.
func CheckSLAStatus(elapsed, threshold time.Duration) (exceeded bool, percentage float64) {
	if threshold <= 0 {
		// No SLA configured
		return false, 0
	}

	percentage = float64(elapsed) / float64(threshold) * 100
	exceeded = elapsed > threshold

	return exceeded, percentage
}

// SLAStatusString returns a formatted string representing SLA status.
// Examples: "50%", "100% ⚠️", "150% ❌"
func SLAStatusString(elapsed, threshold time.Duration) string {
	if threshold <= 0 {
		return "N/A"
	}

	exceeded, percentage := CheckSLAStatus(elapsed, threshold)

	if exceeded {
		return fmt.Sprintf("%.0f%% ❌", percentage)
	} else if percentage >= 80 {
		return fmt.Sprintf("%.0f%% ⚠️", percentage)
	}
	return fmt.Sprintf("%.0f%%", percentage)
}

// DefaultSLADuration is the default SLA duration (48 hours).
const DefaultSLADuration = models.Duration(48 * time.Hour)

// GetRepositorySLA retrieves or creates default SLA settings for a repository.
func GetRepositorySLA(db *gorm.DB, repoID uint) (*models.RepositorySLA, error) {
	var sla models.RepositorySLA
	err := db.Where("repository_id = ?", repoID).First(&sla).Error
	if err == gorm.ErrRecordNotFound {
		// Return default values (not persisted)
		return &models.RepositorySLA{
			RepositoryID:   repoID,
			ReviewDuration: DefaultSLADuration,
			FixesDuration:  DefaultSLADuration,
			AssignCount:    1,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	return &sla, nil
}

// IsWorkingDay returns true if the given date is a working day
// (not weekend, not holiday).
func IsWorkingDay(db *gorm.DB, repoID uint, date time.Time) bool {
	weekday := date.Weekday()
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}

	dateKey := date.Format("2006-01-02")
	var count int64
	db.Model(&models.Holiday{}).
		Where("repository_id = ? AND DATE(date) = ?", repoID, dateKey).
		Count(&count)

	return count == 0
}

package utils

import (
	"devstreamlinebot/models"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

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

func isWorkingDaySimple(weekday time.Weekday, dateKey string, holidaySet map[string]bool) bool {
	if weekday == time.Saturday || weekday == time.Sunday {
		return false
	}
	return !holidaySet[dateKey]
}

func countWeekendsInRange(start, end time.Time) int {
	if !start.Before(end) {
		return 0
	}

	totalDays := int(end.Sub(start).Hours() / 24)
	fullWeeks := totalDays / 7
	count := fullWeeks * 2 // 2 weekend days per week

	remaining := totalDays % 7
	current := start.AddDate(0, 0, fullWeeks*7)
	for i := 0; i < remaining; i++ {
		if current.Weekday() == time.Saturday || current.Weekday() == time.Sunday {
			count++
		}
		current = current.AddDate(0, 0, 1)
	}
	return count
}

func CalculateWorkingTime(db *gorm.DB, repoID uint, start, end time.Time) time.Duration {
	if end.Before(start) || end.Equal(start) {
		return 0
	}

	start = start.UTC()
	end = end.UTC()

	var holidays []models.Holiday
	db.Where("repository_id = ?", repoID).Find(&holidays)
	holidaySet := make(map[string]bool)
	for _, h := range holidays {
		holidaySet[h.Date.Format("2006-01-02")] = true
	}

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	if startDay.Equal(endDay) {
		if isWorkingDaySimple(start.Weekday(), startDay.Format("2006-01-02"), holidaySet) {
			return end.Sub(start)
		}
		return 0
	}

	var totalHours float64

	if isWorkingDaySimple(start.Weekday(), startDay.Format("2006-01-02"), holidaySet) {
		nextMidnight := startDay.AddDate(0, 0, 1)
		totalHours += nextMidnight.Sub(start).Hours()
	}

	middleStart := startDay.AddDate(0, 0, 1)
	middleEnd := endDay
	if middleStart.Before(middleEnd) {
		totalMiddleDays := int(middleEnd.Sub(middleStart).Hours() / 24)
		weekendDays := countWeekendsInRange(middleStart, middleEnd)

		holidaysOnWeekdays := 0
		for dateKey := range holidaySet {
			date, err := time.ParseInLocation("2006-01-02", dateKey, start.Location())
			if err != nil {
				continue
			}
			if !date.Before(middleStart) && date.Before(middleEnd) {
				if date.Weekday() != time.Saturday && date.Weekday() != time.Sunday {
					holidaysOnWeekdays++
				}
			}
		}

		workingDays := totalMiddleDays - weekendDays - holidaysOnWeekdays
		if workingDays > 0 {
			totalHours += float64(workingDays) * 24
		}
	}

	if isWorkingDaySimple(end.Weekday(), endDay.Format("2006-01-02"), holidaySet) {
		totalHours += end.Sub(endDay).Hours()
	}

	return time.Duration(totalHours * float64(time.Hour))
}

func CheckSLAStatus(elapsed, threshold time.Duration) (exceeded bool, percentage float64) {
	if threshold <= 0 {
		return false, 0
	}

	percentage = float64(elapsed) / float64(threshold) * 100
	exceeded = elapsed > threshold

	return exceeded, percentage
}

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

const DefaultSLADuration = models.Duration(48 * time.Hour)

func GetRepositorySLA(db *gorm.DB, repoID uint) (*models.RepositorySLA, error) {
	var sla models.RepositorySLA
	err := db.Where("repository_id = ?", repoID).First(&sla).Error
	if err == gorm.ErrRecordNotFound {
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

// CalculateWorkingTimeFromCache calculates working time excluding weekends and holidays using cached holiday data.
func CalculateWorkingTimeFromCache(repoID uint, start, end time.Time, holidaySet map[string]bool) time.Duration {
	if end.Before(start) || end.Equal(start) {
		return 0
	}

	start = start.UTC()
	end = end.UTC()

	if holidaySet == nil {
		holidaySet = make(map[string]bool)
	}

	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())

	if startDay.Equal(endDay) {
		if isWorkingDaySimple(start.Weekday(), startDay.Format("2006-01-02"), holidaySet) {
			return end.Sub(start)
		}
		return 0
	}

	var totalHours float64

	if isWorkingDaySimple(start.Weekday(), startDay.Format("2006-01-02"), holidaySet) {
		nextMidnight := startDay.AddDate(0, 0, 1)
		totalHours += nextMidnight.Sub(start).Hours()
	}

	middleStart := startDay.AddDate(0, 0, 1)
	middleEnd := endDay
	if middleStart.Before(middleEnd) {
		totalMiddleDays := int(middleEnd.Sub(middleStart).Hours() / 24)
		weekendDays := countWeekendsInRange(middleStart, middleEnd)

		holidaysOnWeekdays := 0
		for dateKey := range holidaySet {
			date, err := time.ParseInLocation("2006-01-02", dateKey, start.Location())
			if err != nil {
				continue
			}
			if !date.Before(middleStart) && date.Before(middleEnd) {
				if date.Weekday() != time.Saturday && date.Weekday() != time.Sunday {
					holidaysOnWeekdays++
				}
			}
		}

		workingDays := totalMiddleDays - weekendDays - holidaysOnWeekdays
		if workingDays > 0 {
			totalHours += float64(workingDays) * 24
		}
	}

	if isWorkingDaySimple(end.Weekday(), endDay.Format("2006-01-02"), holidaySet) {
		totalHours += end.Sub(endDay).Hours()
	}

	return time.Duration(totalHours * float64(time.Hour))
}

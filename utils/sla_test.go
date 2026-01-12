package utils

import (
	"testing"
	"time"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    time.Duration
		wantErr bool
	}{
		// Valid inputs
		{"1 hour", "1h", 1 * time.Hour, false},
		{"24 hours", "24h", 24 * time.Hour, false},
		{"1 day", "1d", 24 * time.Hour, false},
		{"3 days", "3d", 72 * time.Hour, false},
		{"1 week", "1w", 7 * 24 * time.Hour, false},
		{"2 weeks", "2w", 14 * 24 * time.Hour, false},
		{"uppercase", "2D", 48 * time.Hour, false},
		{"with spaces", "  3h  ", 3 * time.Hour, false},

		// Invalid inputs
		{"empty string", "", 0, true},
		{"single char", "h", 0, true},
		{"no value", "d", 0, true},
		{"invalid unit", "5x", 0, true},
		{"invalid value", "abch", 0, true},
		{"negative", "-5h", -5 * time.Hour, false}, // negative values are accepted
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseDuration(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseDuration(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name  string
		input time.Duration
		want  string
	}{
		{"zero", 0, "0h"},
		{"negative", -1 * time.Hour, "0h"},
		{"1 hour", 1 * time.Hour, "1h"},
		{"3 hours", 3 * time.Hour, "3h"},
		{"1 day", 24 * time.Hour, "1d"},
		{"1 day 4 hours", 28 * time.Hour, "1d 4h"},
		{"2 days", 48 * time.Hour, "2d"},
		{"1 week", 7 * 24 * time.Hour, "1w"},
		{"1 week 2 days", 9 * 24 * time.Hour, "1w 2d"},
		{"1 week 2 days 5 hours", 9*24*time.Hour + 5*time.Hour, "1w 2d 5h"},
		{"2 weeks 3 days 12 hours", 17*24*time.Hour + 12*time.Hour, "2w 3d 12h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatDuration(tt.input)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCheckSLAStatus(t *testing.T) {
	tests := []struct {
		name           string
		elapsed        time.Duration
		threshold      time.Duration
		wantExceeded   bool
		wantPercentage float64
	}{
		{"zero threshold", 10 * time.Hour, 0, false, 0},
		{"negative threshold", 10 * time.Hour, -1 * time.Hour, false, 0},
		{"50% used", 24 * time.Hour, 48 * time.Hour, false, 50},
		{"80% used", 40 * time.Hour, 50 * time.Hour, false, 80},
		{"100% used", 48 * time.Hour, 48 * time.Hour, false, 100},
		{"exceeded 150%", 72 * time.Hour, 48 * time.Hour, true, 150},
		{"exceeded 200%", 96 * time.Hour, 48 * time.Hour, true, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exceeded, percentage := CheckSLAStatus(tt.elapsed, tt.threshold)
			if exceeded != tt.wantExceeded {
				t.Errorf("CheckSLAStatus() exceeded = %v, want %v", exceeded, tt.wantExceeded)
			}
			if percentage != tt.wantPercentage {
				t.Errorf("CheckSLAStatus() percentage = %v, want %v", percentage, tt.wantPercentage)
			}
		})
	}
}

func TestSLAStatusString(t *testing.T) {
	tests := []struct {
		name      string
		elapsed   time.Duration
		threshold time.Duration
		want      string
	}{
		{"no threshold", 10 * time.Hour, 0, "N/A"},
		{"50%", 24 * time.Hour, 48 * time.Hour, "50%"},
		{"79%", 79 * time.Hour, 100 * time.Hour, "79%"},
		{"80% warning", 40 * time.Hour, 50 * time.Hour, "80% ⚠️"},
		{"99% warning", 99 * time.Hour, 100 * time.Hour, "99% ⚠️"},
		{"100% warning", 48 * time.Hour, 48 * time.Hour, "100% ⚠️"},
		{"150% exceeded", 72 * time.Hour, 48 * time.Hour, "150% ❌"},
		{"200% exceeded", 96 * time.Hour, 48 * time.Hour, "200% ❌"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SLAStatusString(tt.elapsed, tt.threshold)
			if got != tt.want {
				t.Errorf("SLAStatusString() = %q, want %q", got, tt.want)
			}
		})
	}
}

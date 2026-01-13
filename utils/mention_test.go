package utils

import (
	"testing"

	"devstreamlinebot/models"
)

func TestFormatSLAFromDigest_NotBlocked(t *testing.T) {
	tests := []struct {
		name     string
		dmr      DigestMR
		expected string
	}{
		{
			name: "Zero percentage",
			dmr: DigestMR{
				SLAPercentage: 0,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "N/A",
		},
		{
			name: "Normal percentage",
			dmr: DigestMR{
				SLAPercentage: 50,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "50%",
		},
		{
			name: "Warning percentage (80%+)",
			dmr: DigestMR{
				SLAPercentage: 85,
				SLAExceeded:   false,
				Blocked:       false,
			},
			expected: "85% ⚠️",
		},
		{
			name: "Exceeded percentage",
			dmr: DigestMR{
				SLAPercentage: 120,
				SLAExceeded:   true,
				Blocked:       false,
			},
			expected: "120% ❌",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSLAFromDigest(&tt.dmr)
			if result != tt.expected {
				t.Errorf("formatSLAFromDigest() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatSLAFromDigest_BlockedWithPercentage(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 50,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "50% ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedWithWarning(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 90,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "90% ⚠️ ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedExceeded(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 150,
		SLAExceeded:   true,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "150% ❌ ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestFormatSLAFromDigest_BlockedNA(t *testing.T) {
	dmr := DigestMR{
		SLAPercentage: 0,
		SLAExceeded:   false,
		Blocked:       true,
	}

	result := formatSLAFromDigest(&dmr)
	expected := "N/A ⏸"

	if result != expected {
		t.Errorf("formatSLAFromDigest() = %q, want %q", result, expected)
	}
}

func TestSanitizeTitle(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Simple title", "Simple title"},
		{"Title\nwith\nnewlines", "Title with newlines"},
		{"Title\r\nwith\r\nCRLF", "Title with CRLF"},
		{"Title  with   extra   spaces", "Title with extra spaces"},
		{"  Trimmed  ", "Trimmed"},
		{"Multiple\n\nNewlines", "Multiple Newlines"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := SanitizeTitle(tt.input)
			if result != tt.expected {
				t.Errorf("SanitizeTitle(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestBuildReviewDigest_Empty(t *testing.T) {
	result := BuildReviewDigest(nil, []models.MergeRequest{})

	if result != "No pending reviews found." {
		t.Errorf("BuildReviewDigest() = %q, want %q", result, "No pending reviews found.")
	}
}

func TestBuildEnhancedReviewDigest_Empty(t *testing.T) {
	result := BuildEnhancedReviewDigest(nil, []DigestMR{})

	if result != "No pending reviews found." {
		t.Errorf("BuildEnhancedReviewDigest() = %q, want %q", result, "No pending reviews found.")
	}
}

func TestBuildUserActionsDigest_Empty(t *testing.T) {
	result := BuildUserActionsDigest(nil, []DigestMR{}, []DigestMR{}, "testuser")

	expected := "No pending actions for testuser."
	if result != expected {
		t.Errorf("BuildUserActionsDigest() = %q, want %q", result, expected)
	}
}

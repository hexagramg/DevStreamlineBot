package consumers

import "testing"

func TestIsValidHexColor_Valid3Char(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#abc", true},
		{"#ABC", true},
		{"#123", true},
		{"#fff", true},
		{"#FFF", true},
		{"#a1b", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidHexColor_Valid6Char(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#aabbcc", true},
		{"#AABBCC", true},
		{"#123456", true},
		{"#ffffff", true},
		{"#FFFFFF", true},
		{"#a1b2c3", true},
		{"#dc143c", true}, // Default color
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidHexColor_Invalid_NoHash(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"aabbcc", false},
		{"AABBCC", false},
		{"123456", false},
		{"abc", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidHexColor_Invalid_WrongLength(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#", false},
		{"#a", false},
		{"#ab", false},
		{"#abcd", false},
		{"#abcde", false},
		{"#abcdefg", false},
		{"#1234567", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidHexColor_Invalid_NonHexChars(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"#GGHHII", false},
		{"#ghijkl", false},
		{"#zzzzzz", false},
		{"#12345g", false},
		{"#ghi", false},
		{"#g12", false},
		{"#12-456", false},
		{"#12_456", false},
		{"#12 456", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsValidHexColor_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"", false},
		{"#", false},
		{"##abc", false},
		{"# abc", false},
		{"#000", true},
		{"#000000", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isValidHexColor(tt.input)
			if got != tt.want {
				t.Errorf("isValidHexColor(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

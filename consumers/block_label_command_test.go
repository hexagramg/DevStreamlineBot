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

func TestParseLabelSpecs_SingleLabel(t *testing.T) {
	specs := parseLabelSpecs("blocked")
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "blocked" {
		t.Errorf("expected name 'blocked', got %q", specs[0].name)
	}
	if specs[0].color != "#dc143c" {
		t.Errorf("expected default color '#dc143c', got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_SingleLabelWithColor(t *testing.T) {
	specs := parseLabelSpecs("blocked #ff0000")
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "blocked" {
		t.Errorf("expected name 'blocked', got %q", specs[0].name)
	}
	if specs[0].color != "#ff0000" {
		t.Errorf("expected color '#ff0000', got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_MultipleLabels(t *testing.T) {
	specs := parseLabelSpecs("blocked, wip, hold")
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
	expected := []string{"blocked", "wip", "hold"}
	for i, name := range expected {
		if specs[i].name != name {
			t.Errorf("spec[%d]: expected name %q, got %q", i, name, specs[i].name)
		}
		if specs[i].color != "#dc143c" {
			t.Errorf("spec[%d]: expected default color, got %q", i, specs[i].color)
		}
	}
}

func TestParseLabelSpecs_MultipleLabelsWithMixedColors(t *testing.T) {
	specs := parseLabelSpecs("blocked #ff0000, wip, hold #ffa500")
	if len(specs) != 3 {
		t.Fatalf("expected 3 specs, got %d", len(specs))
	}
	if specs[0].name != "blocked" || specs[0].color != "#ff0000" {
		t.Errorf("spec[0]: expected {blocked, #ff0000}, got {%s, %s}", specs[0].name, specs[0].color)
	}
	if specs[1].name != "wip" || specs[1].color != "#dc143c" {
		t.Errorf("spec[1]: expected {wip, #dc143c}, got {%s, %s}", specs[1].name, specs[1].color)
	}
	if specs[2].name != "hold" || specs[2].color != "#ffa500" {
		t.Errorf("spec[2]: expected {hold, #ffa500}, got {%s, %s}", specs[2].name, specs[2].color)
	}
}

func TestParseLabelSpecs_EmptyEntries(t *testing.T) {
	specs := parseLabelSpecs("blocked,, wip")
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "blocked" {
		t.Errorf("spec[0]: expected 'blocked', got %q", specs[0].name)
	}
	if specs[1].name != "wip" {
		t.Errorf("spec[1]: expected 'wip', got %q", specs[1].name)
	}
}

func TestParseLabelSpecs_ExtraWhitespace(t *testing.T) {
	specs := parseLabelSpecs("  blocked  #ff0000 ,  wip  ")
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "blocked" || specs[0].color != "#ff0000" {
		t.Errorf("spec[0]: expected {blocked, #ff0000}, got {%s, %s}", specs[0].name, specs[0].color)
	}
	if specs[1].name != "wip" || specs[1].color != "#dc143c" {
		t.Errorf("spec[1]: expected {wip, #dc143c}, got {%s, %s}", specs[1].name, specs[1].color)
	}
}

func TestParseLabelSpecs_InvalidColorUsesDefault(t *testing.T) {
	specs := parseLabelSpecs("blocked #xyz")
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "blocked" {
		t.Errorf("expected name 'blocked', got %q", specs[0].name)
	}
	if specs[0].color != "#dc143c" {
		t.Errorf("expected default color when invalid color given, got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_EmptyInput(t *testing.T) {
	specs := parseLabelSpecs("")
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for empty input, got %d", len(specs))
	}
}

func TestParseLabelSpecs_OnlyCommas(t *testing.T) {
	specs := parseLabelSpecs(",,")
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for only commas, got %d", len(specs))
	}
}

func TestParseLabelSpecs_ThreeCharColor(t *testing.T) {
	specs := parseLabelSpecs("blocked #f00")
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].color != "#f00" {
		t.Errorf("expected 3-char color '#f00', got %q", specs[0].color)
	}
}

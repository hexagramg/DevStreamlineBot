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

// Tests for quoted labels with spaces

func TestParseLabelSpecs_QuotedLabelWithSpaces(t *testing.T) {
	specs := parseLabelSpecs(`"need test"`)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "need test" {
		t.Errorf("expected name 'need test', got %q", specs[0].name)
	}
	if specs[0].color != "#dc143c" {
		t.Errorf("expected default color, got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_QuotedLabelWithSpacesAndColor(t *testing.T) {
	specs := parseLabelSpecs(`"need test" #FFFFFF`)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "need test" {
		t.Errorf("expected name 'need test', got %q", specs[0].name)
	}
	if specs[0].color != "#FFFFFF" {
		t.Errorf("expected color '#FFFFFF', got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_MultipleQuotedLabels(t *testing.T) {
	specs := parseLabelSpecs(`"label one", "label two"`)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "label one" {
		t.Errorf("spec[0]: expected 'label one', got %q", specs[0].name)
	}
	if specs[1].name != "label two" {
		t.Errorf("spec[1]: expected 'label two', got %q", specs[1].name)
	}
}

func TestParseLabelSpecs_MultipleQuotedLabelsWithColors(t *testing.T) {
	specs := parseLabelSpecs(`"big label" #FF0000, "another label" #00FF00`)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "big label" || specs[0].color != "#FF0000" {
		t.Errorf("spec[0]: expected {big label, #FF0000}, got {%s, %s}", specs[0].name, specs[0].color)
	}
	if specs[1].name != "another label" || specs[1].color != "#00FF00" {
		t.Errorf("spec[1]: expected {another label, #00FF00}, got {%s, %s}", specs[1].name, specs[1].color)
	}
}

func TestParseLabelSpecs_MixedQuotedAndUnquoted(t *testing.T) {
	specs := parseLabelSpecs(`simple, "with spaces" #FF0000`)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "simple" || specs[0].color != "#dc143c" {
		t.Errorf("spec[0]: expected {simple, #dc143c}, got {%s, %s}", specs[0].name, specs[0].color)
	}
	if specs[1].name != "with spaces" || specs[1].color != "#FF0000" {
		t.Errorf("spec[1]: expected {with spaces, #FF0000}, got {%s, %s}", specs[1].name, specs[1].color)
	}
}

func TestParseLabelSpecs_MixedQuotedAndUnquotedReverse(t *testing.T) {
	specs := parseLabelSpecs(`"quoted first" #abc, unquoted`)
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %d", len(specs))
	}
	if specs[0].name != "quoted first" || specs[0].color != "#abc" {
		t.Errorf("spec[0]: expected {quoted first, #abc}, got {%s, %s}", specs[0].name, specs[0].color)
	}
	if specs[1].name != "unquoted" || specs[1].color != "#dc143c" {
		t.Errorf("spec[1]: expected {unquoted, #dc143c}, got {%s, %s}", specs[1].name, specs[1].color)
	}
}

func TestParseLabelSpecs_QuotedLabelWithCommaInside(t *testing.T) {
	specs := parseLabelSpecs(`"label, with comma"`)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "label, with comma" {
		t.Errorf("expected name 'label, with comma', got %q", specs[0].name)
	}
}

func TestParseLabelSpecs_QuotedLabelUnclosedQuote(t *testing.T) {
	specs := parseLabelSpecs(`"unclosed label`)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "unclosed label" {
		t.Errorf("expected name 'unclosed label', got %q", specs[0].name)
	}
}

func TestParseLabelSpecs_QuotedEmptyLabel(t *testing.T) {
	specs := parseLabelSpecs(`""`)
	if len(specs) != 0 {
		t.Errorf("expected 0 specs for empty quoted string, got %d", len(specs))
	}
}

func TestParseLabelSpecs_QuotedLabelWithExtraWhitespace(t *testing.T) {
	specs := parseLabelSpecs(`  "spaced label"   #fff  `)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "spaced label" {
		t.Errorf("expected name 'spaced label', got %q", specs[0].name)
	}
	if specs[0].color != "#fff" {
		t.Errorf("expected color '#fff', got %q", specs[0].color)
	}
}

func TestParseLabelSpecs_ComplexMixed(t *testing.T) {
	specs := parseLabelSpecs(`blocked #dc143c, "needs review", wip, "do not merge" #FF0000`)
	if len(specs) != 4 {
		t.Fatalf("expected 4 specs, got %d", len(specs))
	}
	expected := []struct {
		name  string
		color string
	}{
		{"blocked", "#dc143c"},
		{"needs review", "#dc143c"},
		{"wip", "#dc143c"},
		{"do not merge", "#FF0000"},
	}
	for i, e := range expected {
		if specs[i].name != e.name || specs[i].color != e.color {
			t.Errorf("spec[%d]: expected {%s, %s}, got {%s, %s}", i, e.name, e.color, specs[i].name, specs[i].color)
		}
	}
}

func TestParseLabelSpecs_QuotedLabelInvalidColor(t *testing.T) {
	specs := parseLabelSpecs(`"my label" #xyz`)
	if len(specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(specs))
	}
	if specs[0].name != "my label" {
		t.Errorf("expected name 'my label', got %q", specs[0].name)
	}
	if specs[0].color != "#dc143c" {
		t.Errorf("expected default color when invalid color given, got %q", specs[0].color)
	}
}

// Tests for splitRespectingQuotes helper

func TestSplitRespectingQuotes_SimpleCommas(t *testing.T) {
	result := splitRespectingQuotes("a,b,c", ',')
	if len(result) != 3 {
		t.Fatalf("expected 3 parts, got %d", len(result))
	}
	expected := []string{"a", "b", "c"}
	for i, e := range expected {
		if result[i] != e {
			t.Errorf("part[%d]: expected %q, got %q", i, e, result[i])
		}
	}
}

func TestSplitRespectingQuotes_QuotedComma(t *testing.T) {
	result := splitRespectingQuotes(`"a,b",c`, ',')
	if len(result) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result))
	}
	if result[0] != `"a,b"` {
		t.Errorf("part[0]: expected '\"a,b\"', got %q", result[0])
	}
	if result[1] != "c" {
		t.Errorf("part[1]: expected 'c', got %q", result[1])
	}
}

func TestSplitRespectingQuotes_MultipleQuoted(t *testing.T) {
	result := splitRespectingQuotes(`"a,b","c,d"`, ',')
	if len(result) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(result))
	}
	if result[0] != `"a,b"` {
		t.Errorf("part[0]: expected '\"a,b\"', got %q", result[0])
	}
	if result[1] != `"c,d"` {
		t.Errorf("part[1]: expected '\"c,d\"', got %q", result[1])
	}
}

func TestSplitRespectingQuotes_Empty(t *testing.T) {
	result := splitRespectingQuotes("", ',')
	if len(result) != 0 {
		t.Errorf("expected 0 parts for empty string, got %d", len(result))
	}
}

// Tests for parseLabelEntry helper

func TestParseLabelEntry_SimpleLabel(t *testing.T) {
	name, color := parseLabelEntry("blocked")
	if name != "blocked" {
		t.Errorf("expected name 'blocked', got %q", name)
	}
	if color != "#dc143c" {
		t.Errorf("expected default color, got %q", color)
	}
}

func TestParseLabelEntry_SimpleLabelWithColor(t *testing.T) {
	name, color := parseLabelEntry("blocked #ff0000")
	if name != "blocked" {
		t.Errorf("expected name 'blocked', got %q", name)
	}
	if color != "#ff0000" {
		t.Errorf("expected color '#ff0000', got %q", color)
	}
}

func TestParseLabelEntry_QuotedLabel(t *testing.T) {
	name, color := parseLabelEntry(`"my label"`)
	if name != "my label" {
		t.Errorf("expected name 'my label', got %q", name)
	}
	if color != "#dc143c" {
		t.Errorf("expected default color, got %q", color)
	}
}

func TestParseLabelEntry_QuotedLabelWithColor(t *testing.T) {
	name, color := parseLabelEntry(`"my label" #00ff00`)
	if name != "my label" {
		t.Errorf("expected name 'my label', got %q", name)
	}
	if color != "#00ff00" {
		t.Errorf("expected color '#00ff00', got %q", color)
	}
}

func TestParseLabelEntry_EmptyString(t *testing.T) {
	name, _ := parseLabelEntry("")
	if name != "" {
		t.Errorf("expected empty name, got %q", name)
	}
	// Color value doesn't matter for empty names since they're filtered out
}

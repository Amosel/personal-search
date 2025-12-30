package chatgpt

import (
	"strings"
	"testing"
)

func TestCanonicalizeText_BasicNormalization(t *testing.T) {
	cases := map[string]string{
		"  text  ":      "text",
		"a\n\n\n\nb":   "a\n\nb",
		"   \n   ":      "",
		"":              "",
		"  ":            "",
		"\n\n\n":        "",
		"a\nb\nc":       "a\nb\nc",
		"  a  \n  b  ":  "a\nb",
		"a\n\nb\n\nc":   "a\n\nb\n\nc",
		"a\n\n\nb\n\nc": "a\n\nb\n\nc",
	}

	for input, expected := range cases {
		got := CanonicalizeText(input)
		if got != expected {
			t.Errorf("CanonicalizeText(%q) = %q, want %q", input, got, expected)
		}
	}
}

func TestCanonicalizeText_WindowsLineEndings(t *testing.T) {
	input := "line1\r\nline2\r\n\r\nline3"
	expected := "line1\nline2\n\nline3"
	got := CanonicalizeText(input)
	if got != expected {
		t.Errorf("Windows line endings: got %q, want %q", got, expected)
	}
}

func TestCanonicalizeText_TrailingWhitespace(t *testing.T) {
	input := "line1   \nline2\t\t\nline3  "
	expected := "line1\nline2\nline3"
	got := CanonicalizeText(input)
	if got != expected {
		t.Errorf("Trailing whitespace: got %q, want %q", got, expected)
	}
}

func TestCanonicalizeText_PreserveParagraphs(t *testing.T) {
	input := "Para 1\n\nPara 2\n\nPara 3"
	expected := "Para 1\n\nPara 2\n\nPara 3"
	got := CanonicalizeText(input)
	if got != expected {
		t.Errorf("Paragraphs: got %q, want %q", got, expected)
	}
}

func TestCanonicalizeText_NoPanic(t *testing.T) {
	// Ensure no panic on any input
	inputs := []string{
		"",
		"   ",
		"\n\n\n",
		"normal text",
		strings.Repeat("\n", 1000),
		strings.Repeat(" ", 1000),
	}

	for _, input := range inputs {
		_ = CanonicalizeText(input)
	}
}

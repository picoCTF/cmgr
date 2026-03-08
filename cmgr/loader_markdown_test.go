package cmgr

import (
	"testing"
)

func TestParseBool(t *testing.T) {
	trueCases := []string{"yes", "YES", "Yes", "true", "TRUE", "True", "1", "t", "T", "y", "Y"}
	for _, tc := range trueCases {
		val, err := parseBool(tc)
		if err != nil {
			t.Errorf("parseBool(%q) returned error: %s", tc, err)
		}
		if !val {
			t.Errorf("parseBool(%q) = false, want true", tc)
		}
	}

	falseCases := []string{"no", "NO", "No", "false", "FALSE", "False", "0", "f", "F", "n", "N"}
	for _, tc := range falseCases {
		val, err := parseBool(tc)
		if err != nil {
			t.Errorf("parseBool(%q) returned error: %s", tc, err)
		}
		if val {
			t.Errorf("parseBool(%q) = true, want false", tc)
		}
	}

	invalidCases := []string{"maybe", "2", "yep", "nope", "", "tru", "fals"}
	for _, tc := range invalidCases {
		_, err := parseBool(tc)
		if err == nil {
			t.Errorf("parseBool(%q) should have returned error", tc)
		}
	}
}

func TestSectionRegex(t *testing.T) {
	tests := []struct {
		input   string
		matches bool
		section string
	}{
		{"## Description", true, "Description"},
		{"## Hints", true, "Hints"},
		{"## Challenge Options", true, "Challenge Options"},
		{"##Tags", true, "Tags"},
		{"# Name", false, ""},
		{"not a section", false, ""},
		{"## spaces after", true, "spaces after"},
	}

	for _, tt := range tests {
		match := sectionRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match section regex", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match section regex, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.section {
			t.Errorf("expected section %q, got %q for input %q", tt.section, match[1], tt.input)
		}
	}
}

func TestKvLineRegex(t *testing.T) {
	tests := []struct {
		input string
		key   string
		value string
		match bool
	}{
		{"- type: custom", "type", "custom", true},
		{"  - namespace: picoctf", "namespace", "picoctf", true},
		{"  - points: 100", "points", "100", true},
		{"- category: Web Exploitation", "category", "Web Exploitation", true},
		{"- templatable: true", "templatable", "true", true},
		{"not a kv line", "", "", false},
		{"- : value", "", "", false},
	}

	for _, tt := range tests {
		match := kvLineRe.FindStringSubmatch(tt.input)
		if tt.match && match == nil {
			t.Errorf("expected %q to match kvLine regex", tt.input)
		} else if !tt.match && match != nil {
			t.Errorf("expected %q to not match kvLine regex", tt.input)
		} else if tt.match {
			if match[1] != tt.key {
				t.Errorf("expected key %q, got %q for input %q", tt.key, match[1], tt.input)
			}
			if match[2] != tt.value {
				t.Errorf("expected value %q, got %q for input %q", tt.value, match[2], tt.input)
			}
		}
	}
}

func TestTagLineRegex(t *testing.T) {
	tests := []struct {
		input string
		tag   string
		match bool
	}{
		{"- web", "web", true},
		{"  - crypto", "crypto", true},
		{"- forensics", "forensics", true},
		{"not a tag", "", false},
		{"- two words", "", false},
	}

	for _, tt := range tests {
		match := tagLineRe.FindStringSubmatch(tt.input)
		if tt.match && match == nil {
			t.Errorf("expected %q to match tagLine regex", tt.input)
		} else if !tt.match && match != nil {
			t.Errorf("expected %q to not match tagLine regex, got %v", tt.input, match)
		} else if tt.match && match[1] != tt.tag {
			t.Errorf("expected tag %q, got %q for input %q", tt.tag, match[1], tt.input)
		}
	}
}

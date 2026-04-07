package cmgr

import (
	"testing"
)

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Hello World", "hello-world"},
		{"My Challenge!", "my-challenge"},
		{"test_challenge_1", "test-challenge-1"},
		{"Simple", "simple"},
		{"UPPERCASE", "uppercase"},
		{"with spaces", "with-spaces"},
		{"special!@#chars", "special---chars"},
		{"  leading-trailing  ", "leading-trailing"},
		{"multiple---dashes", "multiple---dashes"},
		{"CamelCase", "camelcase"},
		{"123numeric", "123numeric"},
		{"a", "a"},
		{"a b c", "a-b-c"},
	}

	for _, tt := range tests {
		result := sanitizeName(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestPathInDirectory(t *testing.T) {
	tests := []struct {
		path     string
		dir      string
		expected bool
	}{
		{"/home/user/challenges/web", "/home/user/challenges", true},
		{"/home/user/challenges", "/home/user/challenges", true},
		{"/home/user/challenge", "/home/user/challenges", false},
		{"/home/user/challenges-extra/web", "/home/user/challenges", false},
		{"/other/path", "/home/user/challenges", false},
		{"/home", "/home/user", false},
		{"/home/user/challenges/deep/nested/path", "/home/user/challenges", true},
	}

	for _, tt := range tests {
		result := pathInDirectory(tt.path, tt.dir)
		if result != tt.expected {
			t.Errorf("pathInDirectory(%q, %q) = %v, want %v", tt.path, tt.dir, result, tt.expected)
		}
	}
}

func TestChecksumIgnore(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{".hidden", true},
		{".git", true},
		{".dockerignore", false},
		{"README", true},
		{"README.md", true},
		{"problem.md", true},
		{"solver", true},
		{"cmgr.db", true},
		{"Dockerfile", false},
		{"Makefile", false},
		{"main.c", false},
		{"solver.py", false},
	}

	for _, tt := range tests {
		result := checksumIgnore(tt.name)
		if result != tt.expected {
			t.Errorf("checksumIgnore(%q) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

func TestContextIgnore(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{".hidden", true},
		{".git", true},
		{".dockerignore", false},
		{"README", true},
		{"README.md", true},
		{"problem.md", true},
		{"solver", true},
		{"cmgr.db", true},
		{"Dockerfile", false},
		{"Makefile", false},
		{"main.c", false},
	}

	for _, tt := range tests {
		result := contextIgnore(tt.name)
		if result != tt.expected {
			t.Errorf("contextIgnore(%q) = %v, want %v", tt.name, result, tt.expected)
		}
	}
}

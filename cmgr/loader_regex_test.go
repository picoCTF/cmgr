package cmgr

import (
	"testing"
)

func TestUrlRegex(t *testing.T) {
	tests := []struct {
		input    string
		matches  bool
		filename string
	}{
		{`{{url("flag.txt")}}`, true, "flag.txt"},
		{`{{ url("data.zip") }}`, true, "data.zip"},
		{`{{url('file.tar.gz')}}`, true, "file.tar.gz"},
		{`{{url("my_file-1.0.bin")}}`, true, "my_file-1.0.bin"},
		{`{{port("http")}}`, false, ""},
		{`regular text`, false, ""},
	}

	for _, tt := range tests {
		match := urlRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match urlRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match urlRe, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.filename {
			t.Errorf("expected filename %q, got %q for input %q", tt.filename, match[1], tt.input)
		}
	}
}

func TestUrlForRegex(t *testing.T) {
	tests := []struct {
		input       string
		matches     bool
		filename    string
		displayText string
	}{
		{`{{url_for("flag.txt", "Download Flag")}}`, true, "flag.txt", "Download Flag"},
		{`{{ url_for("data.zip", "Get Data") }}`, true, "data.zip", "Get Data"},
		{`{{url_for('file.bin', 'Click Here')}}`, true, "file.bin", "Click Here"},
		{`regular text`, false, "", ""},
	}

	for _, tt := range tests {
		match := urlForRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match urlForRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match urlForRe, got %v", tt.input, match)
		} else if tt.matches {
			if match[1] != tt.filename {
				t.Errorf("expected filename %q, got %q for input %q", tt.filename, match[1], tt.input)
			}
			if match[2] != tt.displayText {
				t.Errorf("expected displayText %q, got %q for input %q", tt.displayText, match[2], tt.input)
			}
		}
	}
}

func TestPortRegex(t *testing.T) {
	tests := []struct {
		input    string
		matches  bool
		portName string
	}{
		{`{{port("http")}}`, true, "http"},
		{`{{ port("ssh") }}`, true, "ssh"},
		{`{{port('web')}}`, true, "web"},
		{`{{url("file")}}`, false, ""},
		{`plain text`, false, ""},
	}

	for _, tt := range tests {
		match := portRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match portRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match portRe, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.portName {
			t.Errorf("expected portName %q, got %q for input %q", tt.portName, match[1], tt.input)
		}
	}
}

func TestServerRegex(t *testing.T) {
	tests := []struct {
		input    string
		matches  bool
		portName string
	}{
		{`{{server("http")}}`, true, "http"},
		{`{{ server("ssh") }}`, true, "ssh"},
		{`{{server('web')}}`, true, "web"},
		{`{{port("http")}}`, false, ""},
	}

	for _, tt := range tests {
		match := serverRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match serverRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match serverRe, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.portName {
			t.Errorf("expected portName %q, got %q for input %q", tt.portName, match[1], tt.input)
		}
	}
}

func TestHttpBaseRegex(t *testing.T) {
	tests := []struct {
		input    string
		matches  bool
		portName string
	}{
		{`{{http_base("http")}}`, true, "http"},
		{`{{ http_base("web") }}`, true, "web"},
		{`{{http_base('ssh')}}`, true, "ssh"},
		{`{{port("http")}}`, false, ""},
	}

	for _, tt := range tests {
		match := httpBaseRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match httpBaseRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match httpBaseRe, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.portName {
			t.Errorf("expected portName %q, got %q for input %q", tt.portName, match[1], tt.input)
		}
	}
}

func TestLookupRegex(t *testing.T) {
	tests := []struct {
		input   string
		matches bool
		key     string
	}{
		{`{{lookup("username")}}`, true, "username"},
		{`{{ lookup("password") }}`, true, "password"},
		{`{{lookup('key')}}`, true, "key"},
		{`{{port("http")}}`, false, ""},
	}

	for _, tt := range tests {
		match := lookupRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match lookupRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match lookupRe, got %v", tt.input, match)
		} else if tt.matches && match[1] != tt.key {
			t.Errorf("expected key %q, got %q for input %q", tt.key, match[1], tt.input)
		}
	}
}

func TestShortRegexPatterns(t *testing.T) {
	// Short patterns (no port name argument)
	shortPatterns := []struct {
		name  string
		re    interface{ MatchString(string) bool }
		input string
		match bool
	}{
		{"shortHttpBase", shortHttpBaseRe, `{{http_base}}`, true},
		{"shortHttpBase", shortHttpBaseRe, `{{ http_base }}`, true},
		{"shortHttpBase", shortHttpBaseRe, `{{http_base("port")}}`, false},
		{"shortPort", shortPortRe, `{{port}}`, true},
		{"shortPort", shortPortRe, `{{ port }}`, true},
		{"shortPort", shortPortRe, `{{port("http")}}`, false},
		{"shortServer", shortServerRe, `{{server}}`, true},
		{"shortServer", shortServerRe, `{{ server }}`, true},
		{"shortServer", shortServerRe, `{{server("http")}}`, false},
	}

	for _, tt := range shortPatterns {
		result := tt.re.MatchString(tt.input)
		if result != tt.match {
			t.Errorf("%s.MatchString(%q) = %v, want %v", tt.name, tt.input, result, tt.match)
		}
	}
}

func TestLinkRegex(t *testing.T) {
	tests := []struct {
		input    string
		matches  bool
		portName string
		path     string
	}{
		{`{{link("http", "/admin")}}`, true, "http", "admin"},
		{`{{ link("web", "/api/v1") }}`, true, "web", "api/v1"},
		{`{{link('ssh', '/')}}`, true, "ssh", ""},
	}

	for _, tt := range tests {
		match := linkRe.FindStringSubmatch(tt.input)
		if tt.matches && match == nil {
			t.Errorf("expected %q to match linkRe", tt.input)
		} else if !tt.matches && match != nil {
			t.Errorf("expected %q to not match linkRe, got %v", tt.input, match)
		} else if tt.matches {
			if match[1] != tt.portName {
				t.Errorf("expected portName %q, got %q for input %q", tt.portName, match[1], tt.input)
			}
			if match[2] != tt.path {
				t.Errorf("expected path %q, got %q for input %q", tt.path, match[2], tt.input)
			}
		}
	}
}

func TestTemplateRegex(t *testing.T) {
	tests := []struct {
		input   string
		matches bool
	}{
		{`{{url("file")}}`, true},
		{`{{port("http")}}`, true},
		{`{{server}}`, true},
		{`{{lookup("key")}}`, true},
		{`not a template`, false},
		{`{single brace}`, false},
		{`{{ spaced }}`, true},
	}

	for _, tt := range tests {
		matches := templateRe.FindAllString(tt.input, -1)
		hasMatch := len(matches) > 0
		if hasMatch != tt.matches {
			t.Errorf("templateRe match for %q: got %v, want %v", tt.input, hasMatch, tt.matches)
		}
	}
}

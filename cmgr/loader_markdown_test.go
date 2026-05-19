package cmgr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestManager() *Manager {
	mgr := new(Manager)
	mgr.log = newLogger(DISABLED)
	return mgr
}

func TestParseMarkdownNativeSyntax(t *testing.T) {
	mgr := newTestManager()
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{"backtick inline code", "Use `foo` here.", []string{"`foo`"}},
		{"fenced code block", "```\nhello\n```", []string{"```", "hello"}},
		{"heading", "## A heading", []string{"## A heading"}},
		{"bold and italic", "**bold** and *italic*", []string{"**bold**", "_italic_"}},
		{"unordered list", "- one\n- two", []string{"- one", "- two"}},
		{"ordered list", "1. one\n2. two", []string{"1. one", "2. two"}},
		{"link", "[example](https://example.com)", []string{"[example](https://example.com)"}},
		{"horizontal rule", "before\n\n---\n\nafter", []string{"* * *"}},
		{"blockquote", "> quoted", []string{"> quoted"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mgr.parseMarkdown(tt.input)
			if err != nil {
				t.Fatalf("parseMarkdown error: %s", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %s", want, out)
				}
			}
		})
	}
}

func TestParseMarkdownNormalizesRawHTML(t *testing.T) {
	mgr := newTestManager()
	tests := []struct {
		name     string
		input    string
		contains []string
	}{
		{"raw code tag", "Use <code>foo</code> here.", []string{"`foo`"}},
		{"raw anchor tag", `Click <a href="https://example.com">here</a>.`, []string{"[here](https://example.com)"}},
		{"raw emphasis", "<em>italic</em> and <strong>bold</strong> and <del>gone</del>", []string{"_italic_", "**bold**", "~~gone~~"}},
		{"raw heading", "<h2>Section</h2>", []string{"## Section"}},
		{"raw hr", "before<hr>after", []string{"* * *"}},
		{"raw unordered list", "<ul><li>a</li><li>b</li></ul>", []string{"- a", "- b"}},
		{"raw ordered list", "<ol><li>a</li><li>b</li></ol>", []string{"1. a", "2. b"}},
		{"raw pre/code block", "<pre><code>hello\nworld</code></pre>", []string{"```", "hello", "world"}},
		{"raw table", "<table><thead><tr><th>h1</th><th>h2</th></tr></thead><tbody><tr><td>a</td><td>b</td></tr></tbody></table>",
			[]string{"| h1 | h2 |", "| --- | --- |", "| a | b |"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mgr.parseMarkdown(tt.input)
			if err != nil {
				t.Fatalf("parseMarkdown error: %s", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %s", want, out)
				}
			}
			if strings.Contains(out, "raw HTML omitted") {
				t.Errorf("output contains 'raw HTML omitted' placeholder\ngot: %s", out)
			}
		})
	}
}

func TestParseMarkdownStripsUnsupportedHTML(t *testing.T) {
	mgr := newTestManager()
	tests := []struct {
		name     string
		input    string
		excludes []string
	}{
		{"script tag", "before<script>alert('x')</script>after", []string{"<script", "alert("}},
		{"iframe tag", `<iframe src="evil"></iframe>`, []string{"<iframe", "evil"}},
		{"style tag", "<style>body{color:red}</style>", []string{"<style", "color:red"}},
		{"inline event handler", `<a href="x" onclick="evil()">link</a>`, []string{"onclick", "evil()"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mgr.parseMarkdown(tt.input)
			if err != nil {
				t.Fatalf("parseMarkdown error: %s", err)
			}
			for _, bad := range tt.excludes {
				if strings.Contains(out, bad) {
					t.Errorf("output should not contain %q\ngot: %s", bad, out)
				}
			}
		})
	}
}

func TestParseMarkdownPreservesParagraphsAndBreaks(t *testing.T) {
	mgr := newTestManager()
	tests := []struct {
		name     string
		input    string
		contains []string
		excludes []string
	}{
		{
			name:     "paragraph breaks preserved",
			input:    "First paragraph.\n\nSecond paragraph.\n\nThird.",
			contains: []string{"First paragraph.\n\nSecond paragraph.\n\nThird."},
		},
		{
			name:     "md hard break preserved as <br>",
			input:    "Line one  \nLine two\n\nNew para.",
			contains: []string{"Line one<br>", "Line two", "New para."},
			excludes: []string{"Line one\n\nLine two"},
		},
		{
			name:     "raw <br> preserved",
			input:    "<p>First<br>Second</p>",
			contains: []string{"First<br>Second"},
			excludes: []string{"First\n\nSecond"},
		},
		{
			name:     "list and paragraph boundaries",
			input:    "Intro.\n\n- one\n- two\n\nClose.",
			contains: []string{"Intro.\n\n- one\n- two\n\nClose."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mgr.parseMarkdown(tt.input)
			if err != nil {
				t.Fatalf("parseMarkdown error: %s", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q\ngot: %q", want, out)
				}
			}
			for _, bad := range tt.excludes {
				if strings.Contains(out, bad) {
					t.Errorf("output should not contain %q\ngot: %q", bad, out)
				}
			}
		})
	}
}

func TestParseMarkdownMixedSource(t *testing.T) {
	mgr := newTestManager()
	input := "# Title\n\nSome `inline` and <code>raw</code> code, plus [md link](https://md.example) and <a href=\"https://html.example\">html link</a>."
	out, err := mgr.parseMarkdown(input)
	if err != nil {
		t.Fatalf("parseMarkdown error: %s", err)
	}
	wants := []string{
		"# Title",
		"`inline`",
		"`raw`",
		"[md link](https://md.example)",
		"[html link](https://html.example)",
	}
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\ngot: %s", want, out)
		}
	}
}

func TestLoadMarkdownChallengeMixedHTML(t *testing.T) {
	mgr := newTestManager()

	fence := "```"
	content := `# Mixed Content Challenge

- Namespace: cmgr/test
- Type: custom
- Category: Web Exploitation
- Points: 100

## Description

Markdown **bold** alongside <strong>raw HTML bold</strong>.
Inline ` + "`code`" + ` and <code>raw inline code</code> should both render.
Visit <a href="https://example.com">our site</a> or [the docs](https://docs.example.com).

## Details

Bullet list in markdown:

- alpha
- beta

The same list as raw HTML:

<ul>
  <li>gamma</li>
  <li>delta</li>
</ul>

A raw HTML table with a template inside:

<table>
  <thead><tr><th>field</th><th>value</th></tr></thead>
  <tbody><tr><td>flag</td><td>{{flag("default")}}</td></tr></tbody>
</table>

A fenced code block with templates:

` + fence + `
Hostname: {{server}}
Port:     {{port}}
` + fence + `

<hr>

Closing line with <em>emphasis</em> and ~~strikethrough~~.

## Hints

- Plain hint with ` + "`inline code`" + ` inside.
- HTML hint with <code>raw code</code> and <strong>bold text</strong>.

## Tags

- test
- mixed
`

	dir := t.TempDir()
	path := filepath.Join(dir, "problem.md")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write problem.md: %s", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat problem.md: %s", err)
	}

	md, err := mgr.loadMarkdownChallenge(path, info)
	if err != nil {
		t.Fatalf("loadMarkdownChallenge error: %s", err)
	}

	if md.Name != "Mixed Content Challenge" {
		t.Errorf("Name = %q, want %q", md.Name, "Mixed Content Challenge")
	}
	if md.Namespace != "cmgr/test" {
		t.Errorf("Namespace = %q, want %q", md.Namespace, "cmgr/test")
	}
	if md.Points != 100 {
		t.Errorf("Points = %d, want 100", md.Points)
	}

	descWants := []string{
		"**bold**",
		"**raw HTML bold**",
		"`code`",
		"`raw inline code`",
		"[our site](https://example.com)",
		"[the docs](https://docs.example.com)",
	}
	for _, want := range descWants {
		if !strings.Contains(md.Description, want) {
			t.Errorf("Description missing %q\ngot: %s", want, md.Description)
		}
	}

	detailWants := []string{
		"- alpha", "- beta",
		"- gamma", "- delta",
		"| field | value |",
		`| flag | {{flag("default")}} |`,
		"```", "Hostname:", "{{server}}", "{{port}}",
		"* * *",
		"_emphasis_",
		"~~strikethrough~~",
	}
	for _, want := range detailWants {
		if !strings.Contains(md.Details, want) {
			t.Errorf("Details missing %q\ngot: %s", want, md.Details)
		}
	}

	if strings.Contains(md.Description, "raw HTML omitted") || strings.Contains(md.Details, "raw HTML omitted") {
		t.Errorf("output contains 'raw HTML omitted' placeholder")
	}
	if strings.Contains(md.Details, "&quot;") {
		t.Errorf("template still contains &quot;\ngot: %s", md.Details)
	}

	if len(md.Hints) != 2 {
		t.Fatalf("expected 2 hints, got %d: %v", len(md.Hints), md.Hints)
	}
	if !strings.Contains(md.Hints[0], "`inline code`") {
		t.Errorf("Hints[0] missing markdown inline code\ngot: %s", md.Hints[0])
	}
	if !strings.Contains(md.Hints[1], "`raw code`") || !strings.Contains(md.Hints[1], "**bold text**") {
		t.Errorf("Hints[1] missing raw HTML rendering\ngot: %s", md.Hints[1])
	}

	wantTags := map[string]bool{"test": true, "mixed": true}
	for _, tag := range md.Tags {
		delete(wantTags, tag)
	}
	if len(wantTags) > 0 {
		t.Errorf("missing tags: %v (got %v)", wantTags, md.Tags)
	}
}

func TestParseMarkdownPreservesTemplateQuotes(t *testing.T) {
	mgr := newTestManager()
	out, err := mgr.parseMarkdown(`Flag: {{flag("default")}}`)
	if err != nil {
		t.Fatalf("parseMarkdown error: %s", err)
	}
	if !strings.Contains(out, `{{flag("default")}}`) {
		t.Errorf("template quotes not preserved verbatim\ngot: %s", out)
	}
	if strings.Contains(out, "&quot;") {
		t.Errorf("output still contains &quot; inside template\ngot: %s", out)
	}
}

func TestParseMarkdownPreservesTemplatesVerbatim(t *testing.T) {
	mgr := newTestManager()
	// Each input must round-trip with the template substring intact —
	// no backslash-escapes on `_`, no HTML entity encoding on quotes,
	// no Markdown emphasis from `_` pairs inside the template.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"url_for single quotes", "Found {{url_for('time.txt', 'here')}}.", "{{url_for('time.txt', 'here')}}"},
		{"url_for double quotes", `Find {{url_for("lockbox", "here")}} now.`, `{{url_for("lockbox", "here")}}`},
		{"url_for dotted filename", "Get {{url_for('disks.tar.gz', 'here')}} now.", "{{url_for('disks.tar.gz', 'here')}}"},
		{"url_for mixed-case filename", "See {{url_for('BinEx101', 'program')}}.", "{{url_for('BinEx101', 'program')}}"},
		{"lookup with underscore key", `User: {{lookup("user_name")}}`, `{{lookup("user_name")}}`},
		{"bare server/port", "Connect to {{server}}:{{port}}", "{{server}}"},
		{"template in list item", "- It is here: {{url_for('time.txt', 'here')}}.", "{{url_for('time.txt', 'here')}}"},
		{"same template twice", "{{server}} then {{server}} again", "{{server}}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := mgr.parseMarkdown(tt.input)
			if err != nil {
				t.Fatalf("parseMarkdown error: %s", err)
			}
			if !strings.Contains(out, tt.want) {
				t.Errorf("output missing %q\ngot: %s", tt.want, out)
			}
			if strings.Contains(out, `\_`) {
				t.Errorf("output contains escaped underscore \\_\ngot: %s", out)
			}
			if strings.Contains(out, "&quot;") || strings.Contains(out, "&#39;") {
				t.Errorf("output contains HTML-encoded quote\ngot: %s", out)
			}
			if strings.Contains(out, "@@@") {
				t.Errorf("placeholder leaked into output\ngot: %s", out)
			}
		})
	}
}

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

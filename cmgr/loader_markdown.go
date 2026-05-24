package cmgr

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	goldmarktext "github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v2"
)

var (
	markdownParser = goldmark.DefaultParser()
	htmlConverter  = func() *converter.Converter {
		c := converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(
					commonmark.WithEmDelimiter("_"),
				),
				strikethrough.NewStrikethroughPlugin(),
				table.NewTablePlugin(
					table.WithCellPaddingBehavior(table.CellPaddingBehaviorMinimal),
				),
			),
		)
		// Preserve <br> as an inline HTML tag in the converted output. The
		// default rule turns <br> into a paragraph break (two newlines),
		// which would collapse hard line breaks inside an HTML span.
		c.Register.RendererFor("br", converter.TagTypeInline, renderBr, converter.PriorityEarly)
		return c
	}()
)

func renderBr(ctx converter.Context, w converter.Writer, n *html.Node) converter.RenderStatus {
	w.WriteString("<br>")
	return converter.RenderSuccess
}

func parseBool(s string) (bool, error) {
	s = strings.ToLower(s)
	switch s {
	case "yes":
		fallthrough
	case "true":
		fallthrough
	case "1":
		fallthrough
	case "t":
		fallthrough
	case "y":
		return true, nil
	case "no":
		fallthrough
	case "false":
		fallthrough
	case "0":
		fallthrough
	case "f":
		fallthrough
	case "n":
		return false, nil
	default:
		return false, fmt.Errorf("cannot interpret '%s' as boolean", s)
	}
}

var sectionRe *regexp.Regexp = regexp.MustCompile(`^##\s*(.+)`)
var kvLineRe *regexp.Regexp = regexp.MustCompile(`^\s*-\s*(\w+):\s*(.*)`)
var tagLineRe *regexp.Regexp = regexp.MustCompile(`^\s*-\s*(\w+)\s*$`)

func (m *Manager) loadMarkdownChallenge(path string, info os.FileInfo) (*ChallengeMetadata, error) {
	m.log.debugf("Found challenge Markdown at %s", path)

	// Validate the file, and record the identifier
	data, err := ioutil.ReadFile(path)
	if err != nil {
		m.log.errorf("could not read challenge file: %s", err)
		return nil, err
	}

	md := new(ChallengeMetadata)
	md.Path = path
	md.Attributes = make(map[string]string)

	lines := strings.Split(string(data), "\n")
	idx := 0
	var line string

	// Find the name line
	nameRe := regexp.MustCompile(`^#\s*(.+)`)
	for idx < len(lines) {
		line = strings.TrimSpace(lines[idx])
		match := nameRe.FindStringSubmatch(line)
		idx++
		if match != nil {
			md.Name = match[1]
			break
		}
	}

	// Read the top-level metadata
	for idx < len(lines) {
		line = strings.TrimSpace(lines[idx])
		if sectionRe.MatchString(line) {
			break
		}

		match := kvLineRe.FindStringSubmatch(line)
		idx++

		if len(line) == 0 {
			continue
		}

		if match == nil {
			err = fmt.Errorf("unrecognized metadata text on line %d: %s", idx, path)
			m.log.error(err)
			continue
		}

		switch strings.ToLower(match[1]) {
		case "id":
			md.Id = ChallengeId(match[2])
		case "namespace":
			md.Namespace = match[2]
		case "type":
			md.ChallengeType = match[2]
		case "category":
			md.Category = match[2]
		case "templatable":
			val, tmpErr := parseBool(match[2])
			md.Templatable = val
			err = tmpErr
		case "points":
			i, tmpErr := strconv.Atoi(match[2])
			md.Points = i
			err = tmpErr
		case "maxusers":
			i, tmpErr := strconv.Atoi(match[2])
			md.MaxUsers = i
			err = tmpErr
		default:
			err = fmt.Errorf("unrecognized top-level attribute '%s' on line %d: %s", match[1], idx, path)
			m.log.error(err)
		}
	}

	section := ""
	startIdx := 0
	for idx < len(lines) {
		line = strings.TrimSpace(lines[idx])
		match := sectionRe.FindStringSubmatch(line)
		if match != nil && section != "" {
			err = m.processMarkdownSection(md, section, lines, startIdx, idx)
		}
		if match != nil {
			section = match[1]
			startIdx = idx + 1
		}
		idx++
	}

	if section != "" {
		err = m.processMarkdownSection(md, section, lines, startIdx, idx)
	}

	h := crc32.NewIEEE()
	_, err = h.Write(append(data, []byte(path)...))
	if err != nil {
		return nil, err
	}
	md.MetadataChecksum = h.Sum32()

	return md, nil
}

func (m *Manager) processMarkdownSection(md *ChallengeMetadata, section string, lines []string, startIdx, endIdx int) error {
	var err error
	m.log.debugf("processing markdown: section='%s' start=%d end=%d", section, startIdx, endIdx)
	switch strings.ToLower(section) {
	case "description":
		text, tmpErr := m.parseMarkdown(strings.Join(lines[startIdx:endIdx], "\n"))
		md.Description = text
		err = tmpErr
	case "details":
		text, tmpErr := m.parseMarkdown(strings.Join(lines[startIdx:endIdx], "\n"))
		md.Details = text
		err = tmpErr
	case "hints":
		hints, tmpErr := m.parseHints(lines[startIdx:endIdx])
		md.Hints = hints
		err = tmpErr
	case "tags":
		md.Tags = []string{}
		for i, rawLine := range lines[startIdx:endIdx] {
			line := strings.TrimSpace(rawLine)
			if line == "" {
				continue
			}

			match := tagLineRe.FindStringSubmatch(line)
			if match == nil {
				err = fmt.Errorf("unexpected text in 'tags' section on line %d: %s", i+1, md.Path)
				m.log.error(err)
				continue
			}

			md.Tags = append(md.Tags, match[1])
		}
	case "attributes":
		for i := startIdx; i < endIdx; i++ {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}

			match := kvLineRe.FindStringSubmatch(line)
			if match == nil {
				err = fmt.Errorf("unexpected text in 'attributes' section on line %d: %s", i+1, md.Path)
				m.log.error(err)
				continue
			}

			md.Attributes[match[1]] = match[2]
		}
	case "challenge options":
		yamlStart := 0
		yamlEnd := 0
		for i := startIdx; i < endIdx; i++ {
			if lines[i] == "```yaml" {
				if yamlStart != 0 {
					err = fmt.Errorf("found multiple start markers for yaml at lines %d and %d", yamlStart-1, i)
					m.log.error(err)
				}
				yamlStart = i + 1
			} else if lines[i] == "```" {
				if yamlEnd != 0 {
					err = fmt.Errorf("found multiple end markers for yaml at lines %d and %d", yamlEnd, i)
					m.log.error(err)
				}
				yamlEnd = i
			}
		}

		if yamlStart == 0 && yamlEnd == 0 {
			m.log.debug("addining implicit delimiters for challenge options")
			yamlStart = startIdx
			yamlEnd = endIdx
		} else if (yamlStart == 0) != (yamlEnd == 0) {
			err = fmt.Errorf("found a start/end marker but missing its pair: startline=%d endline=%d", yamlStart, yamlEnd)
			m.log.error(err)
			yamlStart = 0
			yamlEnd = 0
		}

		opts := ChallengeOptions{}
		yamlData := []byte(strings.Join(lines[yamlStart:yamlEnd], "\n"))
		err = yaml.Unmarshal(yamlData, &opts)
		if err != nil {
			m.log.error(err)
		}

		md.ChallengeOptions = opts

	default:
		attrVal := strings.TrimSpace(strings.Join(lines[startIdx:endIdx], "\n"))
		md.Attributes[section] = attrVal
	}
	return err
}

var lineStartRe *regexp.Regexp = regexp.MustCompile(`^    |^\t`)

func (m *Manager) parseHints(lines []string) ([]string, error) {
	hints := []string{}
	hintLines := []string{}
	var err error
	for _, rawLine := range lines {
		if len(rawLine) > 0 && rawLine[0] == '-' {
			if len(hintLines) > 0 {
				hint, tmpErr := m.parseMarkdown(strings.Join(hintLines, "\n"))
				if tmpErr != nil {
					err = tmpErr
				}
				hint = strings.TrimSpace(hint)
				if hint != "" {
					hints = append(hints, hint)
				}
			}
			hintLines = []string{strings.TrimSpace(rawLine[1:])}
		} else {
			hintLines = append(hintLines, lineStartRe.ReplaceAllString(rawLine, ""))
		}
	}
	if len(hintLines) > 0 {
		hint, tmpErr := m.parseMarkdown(strings.Join(hintLines, "\n"))
		if tmpErr != nil {
			err = tmpErr
		}
		hint = strings.TrimSpace(hint)
		if hint != "" {
			hints = append(hints, hint)
		}
	}
	return hints, err
}

// parseMarkdown returns the source with author-written HTML elements
// converted to their markdown equivalents in place. Markdown syntax outside
// of HTML islands (text, headings, emphasis, lists, code, templates, math,
// backslash escapes, etc.) is passed through verbatim — the source is never
// rendered through HTML, so nothing gets normalized, escaped, or reformatted
// behind the author's back.
func (m *Manager) parseMarkdown(text string) (string, error) {
	src := []byte(text)
	reader := goldmarktext.NewReader(src)
	doc := markdownParser.Parse(reader)

	spans, err := collectHTMLSpans(doc, src)
	if err != nil {
		return "", err
	}

	// Sort by start offset. Within a tie, the longer span wins (so a
	// containing span is processed instead of an inner sibling). The
	// pairing algorithm shouldn't emit overlapping spans, but the cursor
	// check below guards against any straggler.
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].start != spans[j].start {
			return spans[i].start < spans[j].start
		}
		return spans[i].end > spans[j].end
	})

	var out bytes.Buffer
	cursor := 0
	for _, sp := range spans {
		if sp.start < cursor {
			continue
		}
		if sp.start > cursor {
			out.Write(src[cursor:sp.start])
		}
		converted, err := htmlConverter.ConvertString(string(src[sp.start:sp.end]))
		if err != nil {
			return "", err
		}
		out.WriteString(converted)
		cursor = sp.end
	}
	if cursor < len(src) {
		out.Write(src[cursor:])
	}

	return strings.TrimSpace(out.String()), nil
}

type htmlSpan struct {
	start, end int
}

type rawTag struct {
	start, end int
	name       string
	kind       int
}

const (
	tagOpen        = 1
	tagClose       = 2
	tagSelfClosing = 3
)

// HTML void elements: never have a closing tag. Per the WHATWG spec.
var voidTags = map[string]bool{
	"area": true, "base": true, "br": true, "col": true,
	"embed": true, "hr": true, "img": true, "input": true,
	"link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

var rawTagRe = regexp.MustCompile(`^<\s*(/?)\s*([a-zA-Z][a-zA-Z0-9-]*)\b[^>]*?(/?)\s*>$`)

func parseRawTag(content string) (name string, kind int) {
	m := rawTagRe.FindStringSubmatch(content)
	if m == nil {
		return "", 0
	}
	name = strings.ToLower(m[2])
	if m[1] == "/" {
		return name, tagClose
	}
	if m[3] == "/" || voidTags[name] {
		return name, tagSelfClosing
	}
	return name, tagOpen
}

// collectHTMLSpans walks the AST and returns byte ranges in src that should
// be substituted with their HTML→markdown converted form. HTMLBlock nodes
// contribute their full span; inline RawHTML tags are paired open-with-close
// via a stack so that `<a href="x">y</a>` resolves to one span covering both
// tags and the text in between.
func collectHTMLSpans(doc ast.Node, src []byte) ([]htmlSpan, error) {
	var spans []htmlSpan
	var inlineTags []rawTag

	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.HTMLBlock:
			lines := n.Lines()
			if lines.Len() > 0 {
				start := lines.At(0).Start
				end := lines.At(lines.Len() - 1).Stop
				spans = append(spans, htmlSpan{start, end})
			}
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			segs := n.Segments
			if segs == nil {
				return ast.WalkContinue, nil
			}
			for i := 0; i < segs.Len(); i++ {
				seg := segs.At(i)
				content := string(src[seg.Start:seg.Stop])
				name, kind := parseRawTag(content)
				if kind == 0 {
					// Comment, CDATA, doctype, processing instruction, etc.
					// Skip — leave the source bytes untouched.
					continue
				}
				inlineTags = append(inlineTags, rawTag{
					start: seg.Start,
					end:   seg.Stop,
					name:  name,
					kind:  kind,
				})
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}

	spans = append(spans, pairInlineTags(inlineTags)...)
	return spans, nil
}

// pairInlineTags consumes a source-ordered list of inline raw-HTML tags and
// returns a list of spans. Open tags are paired with the next matching close
// tag via a stack; unmatched openers/closers and void/self-closing tags each
// form their own single-tag span.
func pairInlineTags(tags []rawTag) []htmlSpan {
	var spans []htmlSpan
	var stack []rawTag

	for _, t := range tags {
		switch t.kind {
		case tagSelfClosing:
			spans = append(spans, htmlSpan{t.start, t.end})
		case tagOpen:
			stack = append(stack, t)
		case tagClose:
			matched := -1
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == t.name {
					matched = i
					break
				}
			}
			if matched >= 0 {
				spans = append(spans, htmlSpan{stack[matched].start, t.end})
				// Any unclosed openers above the match are orphans.
				for _, orphan := range stack[matched+1:] {
					spans = append(spans, htmlSpan{orphan.start, orphan.end})
				}
				stack = stack[:matched]
			} else {
				spans = append(spans, htmlSpan{t.start, t.end})
			}
		}
	}
	for _, orphan := range stack {
		spans = append(spans, htmlSpan{orphan.start, orphan.end})
	}
	return spans
}

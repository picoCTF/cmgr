package cmgr

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"

	htmltomd "github.com/JohannesKaufmann/html-to-markdown"
	"github.com/JohannesKaufmann/html-to-markdown/plugin"
	"github.com/PuerkitoBio/goquery"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
	"gopkg.in/yaml.v2"
)

var (
	markdownUnsafe = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithRendererOptions(html.WithUnsafe()),
	)
	htmlConverter = func() *htmltomd.Converter {
		c := htmltomd.NewConverter("", true, nil)
		c.Use(plugin.GitHubFlavored())
		// Preserve <br> as an inline HTML tag in the markdown output. The
		// default rule converts <br> to a paragraph break (two newlines),
		// which collapses a markdown hard line break ("  \n") into a
		// paragraph break during the round-trip. Emitting <br> keeps the
		// line break inside the paragraph; CommonMark renderers treat
		// inline <br> as a hard line break natively.
		c.AddRules(htmltomd.Rule{
			Filter: []string{"br"},
			Replacement: func(content string, sel *goquery.Selection, opt *htmltomd.Options) *string {
				br := "<br>"
				return &br
			},
		})
		return c
	}()
)

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

func (m *Manager) parseMarkdown(text string) (string, error) {
	// Templates like {{url_for('time.txt', 'here')}} contain characters
	// (underscores, quotes) that the markdown round-trip would escape or
	// HTML-encode, breaking downstream regex matching against the strict
	// urlForRe/lookupRe/etc. patterns. Substitute each template with a
	// neutral alphanumeric placeholder before rendering, restore them
	// verbatim afterward.
	templates := templateRe.FindAllString(text, -1)
	for i, tmpl := range templates {
		text = strings.Replace(text, tmpl, templatePlaceholder(i), 1)
	}

	// First pass: render markdown to HTML with raw HTML preserved, so any
	// inline <code>, <a>, etc. in the source live alongside markdown-derived
	// tags in a single HTML document.
	var rawBuf bytes.Buffer
	if err := markdownUnsafe.Convert([]byte(text), &rawBuf); err != nil {
		return "", err
	}

	// Second pass: convert that HTML back to pure markdown. This normalizes
	// raw HTML tags into their markdown equivalents and drops any tag with
	// no markdown representation. The result is pure markdown for the
	// frontend to render.
	section, err := htmlConverter.ConvertString(rawBuf.String())
	if err != nil {
		return "", err
	}
	section = strings.TrimSpace(section)

	for i, tmpl := range templates {
		section = strings.Replace(section, templatePlaceholder(i), tmpl, 1)
	}
	return section, nil
}

func templatePlaceholder(i int) string {
	return fmt.Sprintf("@@@%d@@@", i)
}

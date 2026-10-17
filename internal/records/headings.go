package records

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/config"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// Heading is one heading of a record body: its level, its raw inline text and the line it starts on (0-based).
type Heading struct {
	Level int
	Text  string
	Line  int
}

var (
	headingParser = goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()
	emptyATX      = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+#*)?[ \t]*$`)
)

// Headings is each heading as the renderer parses the body (CommonMark, raw HTML and
// tables), so a '#' line inside a code fence or an HTML block is no heading.
func Headings(body string) []Heading {
	src := []byte(body)
	doc := headingParser.Parse(text.NewReader(src))
	lineAt := func(off int) int { return strings.Count(body[:off], "\n") }
	var out []Heading
	after := 0 // the line after the last heading found: where an empty heading's search starts
	err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		lines := h.Lines()
		if lines.Len() == 0 { // an empty ATX heading carries no segment: find its line
			all := strings.Split(body, "\n")
			line := after
			for i := after; i < len(all); i++ {
				if emptyATX.MatchString(all[i]) {
					line = i
					break
				}
			}
			out = append(out, Heading{h.Level, "", line})
			after = line + 1
			return ast.WalkSkipChildren, nil
		}
		// The raw text from the first line's content to the last's, trimmed: markdown-it keeps a setext heading's
		// continuation lines with their indentation, where goldmark's segments drop it.
		first, last := lines.At(0), lines.At(lines.Len()-1)
		line := lineAt(first.Start)
		out = append(out, Heading{h.Level, strings.TrimSpace(body[first.Start:last.Stop]), line})
		after = line + 1
		return ast.WalkSkipChildren, nil
	})
	if err != nil { // the walker returns none
		panic(err)
	}
	return out
}

// Section is one section of a record, from its heading through the next heading of the same or a higher level;
// an unknown or ambiguous name is refused with the record's section names.
func Section(rec *Record, name string) (string, error) {
	lines, heads := strings.Split(rec.Body, "\n"), Headings(rec.Body)
	var found []Heading
	for _, h := range heads {
		if h.Text == name {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		names := make([]string, len(heads))
		for i, h := range heads {
			names[i] = strings.Repeat("  ", max(h.Level-2, 0)) + "  " + h.Text
		}
		why := "has no section"
		if len(found) > 0 {
			why = fmt.Sprintf("has %d sections", len(found))
		}
		return "", fmt.Errorf("records/%s.md %s named %s; its sections:\n%s", rec.Rel, why, strconv.Quote(name), strings.Join(names, "\n"))
	}
	return sectionText(lines, heads, found[0]), nil
}

// sectionText is a heading's section: from its line through the next heading of the same or a higher level.
func sectionText(lines []string, heads []Heading, h Heading) string {
	end := len(lines)
	for _, o := range heads {
		if o.Line > h.Line && o.Level <= h.Level {
			end = o.Line
			break
		}
	}
	return config.PyStrip(strings.Join(lines[h.Line:min(end, len(lines))], "\n"))
}

// Outline is what pm show --record prints without --section: the record's path, its header fields, and each
// heading with its section's line count and the command that prints that section, so an agent reads one part, not
// the whole file.
func Outline(rec *Record, target string) string {
	out := []string{"records/" + rec.Rel + ".md"}
	keys := make([]string, 0, len(rec.Meta))
	for k := range rec.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := rec.Meta[k]
		if v.Kind == KindList {
			items := make([]string, len(v.List))
			for i, it := range v.List {
				items[i] = it.Str
			}
			out = append(out, fmt.Sprintf("  %s: %s", k, strings.Join(items, ", ")))
			continue
		}
		out = append(out, fmt.Sprintf("  %s: %s", k, v.Str))
	}
	lines, heads := strings.Split(rec.Body, "\n"), Headings(rec.Body)
	if len(heads) == 0 {
		return strings.Join(append(out, "no sections"), "\n")
	}
	out = append(out, "sections, each with its line count and the command that prints it:")
	for _, h := range heads {
		n := strings.Count(sectionText(lines, heads, h), "\n") + 1
		out = append(out, fmt.Sprintf("%s%s %s  (%d lines)  pm show --record %s --section %s",
			strings.Repeat("  ", max(h.Level-1, 1)), strings.Repeat("#", h.Level), h.Text, n, ShellQuote(target),
			ShellQuote(h.Text)))
	}
	return strings.Join(out, "\n")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// ShellQuote is Python's shlex.quote.
func ShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

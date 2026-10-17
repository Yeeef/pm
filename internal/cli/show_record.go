package cli

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Yeeef/pm/internal/cli/fsutil"
	"github.com/Yeeef/pm/internal/config"
	"github.com/Yeeef/pm/internal/records"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

// showHeading is one heading of a record body: its level, its raw inline text and the line it starts on (0-based).
type showHeading struct {
	level int
	text  string
	line  int
}

var (
	showParser   = goldmark.New(goldmark.WithExtensions(extension.Table)).Parser()
	showEmptyATX = regexp.MustCompile(`^ {0,3}#{1,6}(?:[ \t]+#*)?[ \t]*$`)
)

// showHeadings is each heading as the renderer parses the body (CommonMark, raw HTML and
// tables), so a '#' line inside a code fence or an HTML block is no heading.
func showHeadings(body string) []showHeading {
	src := []byte(body)
	doc := showParser.Parse(text.NewReader(src))
	lineAt := func(off int) int { return strings.Count(body[:off], "\n") }
	var out []showHeading
	after := 0 // the line after the last heading found: where an empty heading's search starts
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		h, ok := n.(*ast.Heading)
		if !entering || !ok {
			return ast.WalkContinue, nil
		}
		lines := h.Lines()
		if lines.Len() == 0 { // an empty ATX heading carries no segment: find its line
			all := strings.Split(body, "\n")
			line := after
			for i := after; i < len(all); i++ {
				if showEmptyATX.MatchString(all[i]) {
					line = i
					break
				}
			}
			out = append(out, showHeading{h.Level, "", line})
			after = line + 1
			return ast.WalkSkipChildren, nil
		}
		// The raw text from the first line's content to the last's, trimmed: markdown-it keeps a setext heading's
		// continuation lines with their indentation, where goldmark's segments drop it.
		first, last := lines.At(0), lines.At(lines.Len()-1)
		line := lineAt(first.Start)
		out = append(out, showHeading{h.Level, strings.TrimSpace(body[first.Start:last.Stop]), line})
		after = line + 1
		return ast.WalkSkipChildren, nil
	})
	return out
}

// recordSection is one section of a record, from its heading through the next heading of the same or a higher level;
// an unknown or ambiguous name is refused with the record's section names.
func recordSection(rec *records.Record, name string) (string, error) {
	lines, heads := strings.Split(rec.Body, "\n"), showHeadings(rec.Body)
	var found []showHeading
	for _, h := range heads {
		if h.text == name {
			found = append(found, h)
		}
	}
	if len(found) != 1 {
		names := make([]string, len(heads))
		for i, h := range heads {
			names[i] = strings.Repeat("  ", max(h.level-2, 0)) + "  " + h.text
		}
		why := "has no section"
		if len(found) > 0 {
			why = fmt.Sprintf("has %d sections", len(found))
		}
		return "", refuse("records/%s.md %s named %s; its sections:\n%s", rec.Rel, why, strconv.Quote(name), strings.Join(names, "\n"))
	}
	return sectionText(lines, heads, found[0]), nil
}

// sectionText is a heading's section: from its line through the next heading of the same or a higher level.
func sectionText(lines []string, heads []showHeading, h showHeading) string {
	end := len(lines)
	for _, o := range heads {
		if o.line > h.line && o.level <= h.level {
			end = o.line
			break
		}
	}
	return config.PyStrip(strings.Join(lines[h.line:min(end, len(lines))], "\n"))
}

// recordOutline is what pm show --record prints without --section: the record's path, its header fields, and each
// heading with its section's line count and the command that prints that section, so an agent reads one part, not
// the whole file.
func recordOutline(rec *records.Record, target string) string {
	out := []string{"records/" + rec.Rel + ".md"}
	keys := make([]string, 0, len(rec.Meta))
	for k := range rec.Meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := rec.Meta[k]
		if v.Kind == records.KindList {
			items := make([]string, len(v.List))
			for i, it := range v.List {
				items[i] = it.Str
			}
			out = append(out, fmt.Sprintf("  %s: %s", k, strings.Join(items, ", ")))
			continue
		}
		out = append(out, fmt.Sprintf("  %s: %s", k, v.Str))
	}
	lines, heads := strings.Split(rec.Body, "\n"), showHeadings(rec.Body)
	if len(heads) == 0 {
		return strings.Join(append(out, "no sections"), "\n")
	}
	out = append(out, "sections, each with its line count and the command that prints it:")
	for _, h := range heads {
		n := strings.Count(sectionText(lines, heads, h), "\n") + 1
		out = append(out, fmt.Sprintf("%s%s %s  (%d lines)  pm show --record %s --section %s",
			strings.Repeat("  ", max(h.level-1, 1)), strings.Repeat("#", h.level), h.text, n, shellQuote(target),
			shellQuote(h.text)))
	}
	return strings.Join(out, "\n")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

// shellQuote is Python's shlex.quote.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// showPosix is Python's Path(p).as_posix() for a relative path: empty and "." parts dropped.
func showPosix(p string) string {
	var parts []string
	for _, s := range strings.Split(p, "/") {
		if s != "" && s != "." {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return "."
	}
	return strings.Join(parts, "/")
}

// linkTarget is the record a target names: a record path (records/ and .md optional), a sprint or project id, a
// sprint name (<project>-<n>), a project name or a design slug; the old path of a sprint pm sprint move moved names
// its record now. A target that fits several records is refused, never guessed.
func linkTarget(e *env, recs []*records.Record, target string) (*records.Record, error) {
	var found []*records.Record
	store := e.records
	if strings.Contains(target, "/") || strings.HasSuffix(target, ".md") {
		given := target
		if filepath.IsAbs(given) {
			rel, err := filepath.Rel(fsutil.Resolved(store), fsutil.Resolved(given))
			if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
				return nil, refuse("%s is not in the records store %s", target, store)
			}
			given = filepath.ToSlash(rel)
		}
		rel := strings.TrimSuffix(strings.TrimPrefix(showPosix(given), "records/"), ".md")
		for _, r := range recs {
			if r.Rel == rel {
				found = append(found, r)
			}
		}
		if len(found) == 0 && strings.HasPrefix(rel, "sprints/") { // a moved sprint's old path: the work store knows
			ws, err := e.work()
			if err != nil {
				return nil, err
			}
			items, err := ws.Items()
			if err != nil {
				return nil, err
			}
			if r := records.MovedSprint(recs, records.NewItems(items), rel); r != nil {
				found = append(found, r)
			}
		}
	} else {
		for _, r := range recs {
			bead, typed := r.ID("bead")
			if ((r.Type() == "sprint" || r.Type() == "project") && typed && bead == target) ||
				((r.Type() == "project" || r.Type() == "sprint" || r.Type() == "design") && r.Name() == target) {
				found = append(found, r)
			}
		}
	}
	if len(found) == 0 {
		return nil, refuse("no record matches %s; give a sprint id or name, a project name, a design slug or a record path "+
			"such as records/sprints/<name>.md", strconv.Quote(target))
	}
	if len(found) > 1 {
		names := make([]string, len(found))
		for i, r := range found {
			names[i] = "records/" + r.Rel + ".md"
		}
		return nil, refuse("%s names %s; give the record path instead", strconv.Quote(target), strings.Join(names, " and "))
	}
	return found[0], nil
}

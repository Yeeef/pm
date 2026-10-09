package records

import (
	"regexp"
	"strings"

	"github.com/Yeeef/pm/internal/config"
)

// SplitLines is Python's str.splitlines(): it breaks at \n, \r\n, \r, \v, \f, \x1c-\x1e, \x85,   and  , and
// leaves no empty last line for a trailing break.
func SplitLines(s string) []string {
	var out []string
	start := 0
	for i, r := range s {
		switch r {
		case '\n', '\v', '\f', 0x1c, 0x1d, 0x1e, 0x85, 0x2028, 0x2029:
			if i > 0 && s[i-1] == '\r' && r == '\n' {
				start = i + 1
				continue
			}
			out = append(out, s[start:i])
			start = i + len(string(r))
		case '\r':
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// Strip is Python's str.strip().
func Strip(s string) string { return config.PyStrip(s) }

var (
	fenceRE   = regexp.MustCompile("^(```|~~~)")
	headingRE = regexp.MustCompile(`^(#{1,6}) (.*)$`)
)

// Range is one section: its name, its heading's line and the line after its end.
type Range struct {
	Name       string
	Start, End int
}

// SectionRanges is each heading of the level, skipping code fences; a section ends at the next heading of the same or
// a higher level.
func SectionRanges(text string, level int) []Range {
	lines := strings.Split(text, "\n")
	type head struct {
		level int
		name  string
		line  int
	}
	var heads []head
	fence := ""
	for n, line := range lines {
		if f := fenceRE.FindString(line); f != "" {
			switch fence {
			case f:
				fence = ""
			case "":
				fence = f
			}
			continue
		}
		if h := headingRE.FindStringSubmatch(line); fence == "" && h != nil {
			heads = append(heads, head{len(h[1]), Strip(h[2]), n})
		}
	}
	var out []Range
	for i, h := range heads {
		if h.level != level {
			continue
		}
		end := len(lines)
		for _, next := range heads[i+1:] {
			if next.level <= level {
				end = next.line
				break
			}
		}
		out = append(out, Range{h.name, h.line, end})
	}
	return out
}

// SectionRange is the lines of the first level-2 section with the name.
func SectionRange(text, name string) (Range, bool) {
	for _, r := range SectionRanges(text, 2) {
		if r.Name == name {
			return r, true
		}
	}
	return Range{}, false
}

// StripPrompts is the text without its prompt lines ("> …"), stripped.
func StripPrompts(text string) string {
	var keep []string
	for _, l := range SplitLines(Strip(text)) {
		if !strings.HasPrefix(l, ">") {
			keep = append(keep, l)
		}
	}
	return Strip(strings.Join(keep, "\n"))
}

// part is the text under the first line "<marks> <name>" up to the next line starting "<marks> ", or the end: Python's
// re.search(rf"^{marks} {name}\n(.*?)(?=^{marks} |\Z)", text, re.S | re.M), whose lookahead RE2 cannot express.
func part(text, marks, name string) (string, bool) {
	head := marks + " " + name + "\n"
	start := -1
	for i := 0; i <= len(text)-len(head); i++ {
		if (i == 0 || text[i-1] == '\n') && strings.HasPrefix(text[i:], head) {
			start = i + len(head)
			break
		}
	}
	if start < 0 {
		return "", false
	}
	next := marks + " "
	for p := start; p < len(text); p++ {
		if (p == 0 || text[p-1] == '\n') && strings.HasPrefix(text[p:], next) {
			return text[start:p], true
		}
	}
	return text[start:], true
}

// SectionText is the text of a "## name" section, without its prompt lines.
func SectionText(body, name string) string {
	t, ok := part(body, "##", name)
	if !ok {
		return ""
	}
	return StripPrompts(t)
}

// SubsectionText is the text of a "### name" subsection within a section's text, without prompt lines.
func SubsectionText(text, name string) string {
	t, ok := part(text, "###", name)
	if !ok {
		return ""
	}
	return StripPrompts(t)
}

var spaceRun = regexp.MustCompile(pyS + `+`)

// FirstPara is the text's first paragraph on one line.
func FirstPara(text string) string {
	first, _, _ := strings.Cut(text, "\n\n")
	return Strip(spaceRun.ReplaceAllString(first, " "))
}

var bulletRE = regexp.MustCompile(`(?m)^` + pyS + `*[-*]` + pyS + `+(.+?)` + pyS + `*$`)

// SummaryLine is a day summary on one line for lists: its bullets ("- " or "* ") joined with " · ", else its first
// paragraph.
func SummaryLine(text string) string {
	var items []string
	for _, m := range bulletRE.FindAllStringSubmatch(text, -1) {
		items = append(items, m[1])
	}
	if items != nil {
		return strings.Join(items, " · ")
	}
	return FirstPara(text)
}

func report(rec *Record) string {
	t, _ := part(rec.Body, "##", "Delivery report")
	return t
}

var outcomeWord = regexp.MustCompile(`(?i)^(done|partial|voided)`)

// Outcome is the delivery report's outcome, or "" while the sprint is not closed.
func Outcome(rec *Record) (string, error) {
	rep := report(rec)
	for _, p := range ReportParts {
		if !regexp.MustCompile(`(?m)^### ` + regexp.QuoteMeta(p) + `$`).MatchString(rep) {
			return "", errorf("%s: Delivery report needs a '### %s' subsection", rec.Rel, p)
		}
	}
	line := FirstPara(SubsectionText(rep, "Outcome"))
	if line == NotClosed {
		return "", nil
	}
	m := outcomeWord.FindString(line)
	if m == "" || wordAt(line, len(m)) {
		return "", errorf("%s: Delivery report Outcome must start with done, partial or voided", rec.Rel)
	}
	return line, nil
}

// wordAt says whether s holds a word character (Python's Unicode \w) at byte i: where \b after a word fails.
func wordAt(s string, i int) bool {
	if i >= len(s) {
		return false
	}
	return wordRE.MatchString(s[i:])
}

var wordRE = regexp.MustCompile(`^` + pyW)

// ReportPart is one subsection of the delivery report, without prompt lines.
func ReportPart(rec *Record, name string) string { return SubsectionText(report(rec), name) }

// InsertEntry inserts entry (one or more lines) at the end of a section, replacing its "None yet." placeholder when
// that is all the section holds; the rest of the file is unchanged.
func InsertEntry(text, section, entry string) (string, error) {
	rng, ok := SectionRange(text, section)
	if !ok {
		return "", errorf("no '## %s' section", section)
	}
	lines := strings.Split(text, "\n")
	var body []int
	for n := rng.Start + 1; n < rng.End; n++ {
		if Strip(lines[n]) != "" && !strings.HasPrefix(lines[n], ">") {
			body = append(body, n)
		}
	}
	add := strings.Split(strings.TrimRight(entry, "\n"), "\n")
	if len(body) == 1 && Strip(lines[body[0]]) == NoneYet {
		lines = splice(lines, body[0], body[0]+1, add)
	} else {
		last := -1
		if body != nil {
			last = body[len(body)-1]
		} else {
			for n := rng.Start; n < rng.End; n++ {
				if Strip(lines[n]) != "" {
					last = n
				}
			}
		}
		lines = splice(lines, last+1, last+1, append([]string{""}, add...))
	}
	return strings.Join(lines, "\n"), nil
}

func splice(lines []string, from, to int, with []string) []string {
	out := append([]string{}, lines[:from]...)
	out = append(out, with...)
	return append(out, lines[to:]...)
}

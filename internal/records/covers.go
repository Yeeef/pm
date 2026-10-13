package records

import (
	"path"
	"regexp"
	"strings"
)

// A design page's covers: the code its area is about, as path patterns relative to the repo root, in its header
// (covers: [internal/records/**, prime.md]). A pattern is a doublestar glob over slash paths: each segment matches as
// path.Match does (*, ?, [...] and \ escapes, never across a /), and a segment that is exactly ** matches zero or more
// whole segments. ** is only ever a whole segment, so git's :(glob) pathspec magic reads a pattern the same way.

// Covers is the design page's covers patterns; nil when its header has none.
func (r *Record) Covers() []string {
	var out []string
	for _, v := range r.Meta["covers"].List {
		out = append(out, v.Str)
	}
	return out
}

// CoversOf is the covers patterns in the header of a page's text, read but not checked: a past version of a page,
// which a later pm's rules may refuse.
func CoversOf(text, rel string) ([]string, error) {
	head, _, ok := header(text)
	if !ok {
		return nil, errorf("%s: missing YAML header", rel)
	}
	meta, err := parseHeader(head, rel)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range meta["covers"].List {
		out = append(out, v.Str)
	}
	return out, nil
}

// checkCovers fails on a covers field outside a design page, or one that is not a list of patterns.
func checkCovers(meta Meta, rel string) error {
	v, ok := meta["covers"]
	if !ok {
		return nil
	}
	if meta["type"].Str != "design" {
		return errorf("%s: covers is a design page's header field; remove it", rel)
	}
	if v.Kind != KindList {
		return errorf("%s: covers is a list of path patterns relative to the repo root, e.g. covers: "+
			"[internal/records/**, prime.md]; got %s", rel, v.Repr())
	}
	for _, p := range v.List {
		if p.Kind != KindStr {
			return errorf("%s: covers pattern %s is not a string; quote it", rel, p.Repr())
		}
		if err := CheckPattern(p.Str); err != nil {
			return errorf("%s: covers pattern %s %s", rel, p.Repr(), err.Error())
		}
	}
	return nil
}

// CheckPattern fails on a pattern Match cannot read: empty, absolute, with an empty, . or .. segment, a ** inside a
// segment, or a segment path.Match refuses.
func CheckPattern(p string) error {
	if p == "" || strings.HasPrefix(p, "/") {
		return errorf("is not relative to the repo root (no leading /)")
	}
	for _, seg := range strings.Split(p, "/") {
		switch {
		case seg == "" || seg == "." || seg == "..":
			return errorf("has an empty, . or .. segment")
		case seg != "**" && strings.Contains(seg, "**"):
			return errorf("has ** inside a segment; ** stands alone between slashes (internal/**/x.go)")
		}
		if _, err := path.Match(seg, ""); err != nil {
			return errorf("is no glob: %v", err)
		}
	}
	return nil
}

// Match reports whether the slash path file, relative to the repo root, matches the pattern.
func Match(pattern, file string) bool {
	return matchParts(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchParts(pat, parts []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(parts); i++ {
				if matchParts(pat[1:], parts[i:]) {
					return true
				}
			}
			return false
		}
		if len(parts) == 0 {
			return false
		}
		if ok, err := path.Match(pat[0], parts[0]); err != nil || !ok { // CheckPattern refuses a bad pattern first
			return false
		}
		pat, parts = pat[1:], parts[1:]
	}
	return len(parts) == 0
}

// MatchAny reports whether file matches any of the patterns.
func MatchAny(patterns []string, file string) bool {
	for _, p := range patterns {
		if Match(p, file) {
			return true
		}
	}
	return false
}

var designLink = regexp.MustCompile(`\(\.\./design/([^/()#\s]+)\.md[)#]`)

// DesignLinks is the slug of each design page a sprint or project record links in its Design pages section, in order.
func DesignLinks(rec *Record) []string {
	var out []string
	for _, m := range designLink.FindAllStringSubmatch(SectionText(rec.Body, "Design pages"), -1) {
		out = append(out, m[1])
	}
	return out
}

package records

import (
	"strings"
	"testing"
)

func TestMatchReadsDoublestarGlobs(t *testing.T) {
	cases := []struct {
		pattern, file string
		want          bool
	}{
		{"prime.md", "prime.md", true},
		{"prime.md", "docs/prime.md", false},
		{"internal/records/**", "internal/records/yaml.go", true},
		{"internal/records/**", "internal/records/testdata/expected.json", true},
		{"internal/records/**", "internal/recordsx/a.go", false},
		{"internal/records", "internal/records/yaml.go", false}, // a directory's files are dir/**
		{"internal/*/deliver.go", "internal/service/deliver.go", true},
		{"internal/*.go", "internal/service/deliver.go", false}, // * stays within one segment
		{"**/*_test.go", "internal/cli/a_test.go", true},
		{"**/*_test.go", "a_test.go", true}, // ** matches zero segments
		{"internal/**/x.go", "internal/x.go", true},
		{"internal/**/x.go", "internal/a/b/x.go", true},
		{"prompts/owner_request_*.txt", "prompts/owner_request_reason.txt", true},
		{"prompts/owner_request_*.txt", "prompts/day_summary.txt", false},
		{"internal/cli/[ns]*.go", "internal/cli/needs.go", true},
	}
	for _, c := range cases {
		if got := Match(c.pattern, c.file); got != c.want {
			t.Errorf("Match(%q, %q) = %v, want %v", c.pattern, c.file, got, c.want)
		}
	}
}

func TestCoversIsADesignPagesListOfPatterns(t *testing.T) {
	page := func(header string) string {
		return "---\ntype: design\ntitle: T\nproject: demo\n" + header + "---\n\nx\n"
	}
	cases := []struct{ text, err string }{
		{page("covers: [internal/records/**, prime.md]\n"), ""},
		{page("covers:\n  - internal/records/**\n  - prime.md\n"), ""},
		{page("covers: []\n"), ""},
		{page("covers: internal/records/**\n"), "design/t: covers is a list of path patterns relative to the repo " +
			"root, e.g. covers: [internal/records/**, prime.md]; got 'internal/records/**'"},
		{page("covers: [5]\n"), "design/t: covers pattern 5 is not a string; quote it"},
		{page("covers: [/abs/x.go]\n"), "design/t: covers pattern '/abs/x.go' is not relative to the repo root " +
			"(no leading /)"},
		{page("covers: [internal/../x.go]\n"), "design/t: covers pattern 'internal/../x.go' has an empty, . or .. " +
			"segment"},
		{page("covers: [internal/**.go]\n"), "design/t: covers pattern 'internal/**.go' has ** inside a segment; ** " +
			"stands alone between slashes (internal/**/x.go)"},
		{page("covers: ['internal/[x.go']\n"), "design/t: covers pattern 'internal/[x.go' is no glob: syntax error " +
			"in pattern"},
		{"---\ntype: doc\ntitle: T\ndate: 2026-10-01\nproject: demo\ncovers: [x]\n---\n\nx\n",
			"docs/2026-10-01-t: covers is a design page's header field; remove it"},
	}
	for _, c := range cases {
		rel := "design/t"
		if strings.Contains(c.text, "type: doc") {
			rel = "docs/2026-10-01-t"
		}
		rec, err := Parse(rel+".md", rel, c.text)
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != c.err {
			t.Errorf("%q: got error %q, want %q", c.text, got, c.err)
		}
		if err == nil && c.err == "" && len(rec.Covers()) != map[bool]int{true: 0, false: 2}[c.text == page("covers: []\n")] {
			t.Errorf("%q: Covers() = %v", c.text, rec.Covers())
		}
	}
}

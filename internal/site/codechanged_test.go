package site

import (
	"testing"

	"github.com/Yeeef/pm/internal/design"
	"github.com/Yeeef/pm/internal/records"
	"github.com/Yeeef/pm/internal/store"
)

// A merge a sprint stamped is left out of a design page's covered-code line when that sprint updated the page (links
// it in Design pages) or waived it: the page edit came before the merge, yet it answers for it.
func TestCodeChangedLeavesOutTheMergesOfSprintsThatUpdatedOrWaivedThePage(t *testing.T) {
	parse := func(rel, text string) *records.Record {
		t.Helper()
		rec, err := records.Parse(rel+".md", rel, text)
		if err != nil {
			t.Fatal(err)
		}
		return rec
	}
	page := parse("design/parser", "---\ntype: design\ntitle: P\nproject: demo\ncovers: [src/**]\n---\n\nx\n")
	sprint := func(rel, designPages, decisions string) *records.Record {
		return parse(rel, "---\ntype: sprint\ntitle: S\nbead: demo.1\n---\n\n## Design pages\n\n"+designPages+
			"\n\n## Decisions\n\n"+decisions+"\n\n## Delivery report\n\n### Outcome\n\nDone: it.\n\n"+
			"Merged as aaaaaaa (PR #1). Merged as bbbbbbb.\n")
	}
	commits := []store.CodeCommit{{SHA: "aaaaaaa111", Day: "2026-10-09"}, {SHA: "bbbbbbb222", Day: "2026-10-08"},
		{SHA: "ccccccc333", Day: "2026-10-07"}}
	waiver := "::: decision {source=agent date=2026-10-08}\n" + design.WaiverText("parser", "Holds.") + "\n:::"
	cases := []struct {
		name string
		recs []*records.Record
		want string
	}{
		{"no sprint names it", []*records.Record{page, sprint("sprints/a", "None yet.", "None yet.")},
			`<p class="note draft">The code this page covers changed after its last edit: 3 commits on the main ` +
				`branch, the latest on 2026-10-09.</p>`},
		{"a sprint updated it", []*records.Record{page, sprint("sprints/a", "- [P](../design/parser.md)", "None yet.")},
			`<p class="note draft">The code this page covers changed after its last edit: 1 commit on the main ` +
				`branch, the latest on 2026-10-07.</p>`},
		{"a sprint waived it", []*records.Record{page, sprint("sprints/a", "None yet.", waiver)},
			`<p class="note draft">The code this page covers changed after its last edit: 1 commit on the main ` +
				`branch, the latest on 2026-10-07.</p>`},
		{"nothing left", []*records.Record{page, sprint("sprints/a", "- [P](../design/parser.md)", "None yet.")}, ""},
	}
	for i, c := range cases {
		in := commits
		if i == 3 {
			in = commits[:2]
		}
		if got := codeChanged(page, c.recs, in); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

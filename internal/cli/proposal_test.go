package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"

	"github.com/Yeeef/pm/internal/work"
	"github.com/Yeeef/pm/internal/work/worktest"
)

// The class mechanism of the sprint-proposals design page (Every command on a proposal): every command argument that
// takes an item id is classed for a proposal. testdata/proposal_commands.json holds the cases, each a command run
// with a proposal's id in that argument and its expected outcome; tests/test_proposals.py runs them against a real
// store. This test holds the cases to the command tree: a new id argument without a case fails here.

// idArgs is each argument of c that takes an item id: a positional sprint, task, need or item id, or a flag whose
// metavar names one (--sprint ID, --task TASK, --on BLOCKER, task move's --to SPRINT_ID); a project name, a path and
// a session id (--session ID) are not one.
func idArgs(c *command) []string {
	var out []string
	for _, a := range c.Args {
		meta := strings.Join(a.Metavar, " ")
		switch {
		case a.Positional() && slices.Contains([]string{"sprint_id", "task_id", "need_id", "id", "ids", "target"}, a.Dest):
			out = append(out, a.Dest)
		case !a.Positional() && a.Flags[0] != "--session" &&
			slices.Contains([]string{"ID", "TASK", "SPRINT_ID", "BLOCKER"}, meta):
			out = append(out, a.Flags[0])
		}
	}
	return out
}

// TestEveryItemIDArgumentIsClassedForAProposal: the cases cover each id argument of every command, the tree's and
// the work-store commands' (their forms included), and name no other.
func TestEveryItemIDArgumentIsClassedForAProposal(t *testing.T) {
	want := map[string]bool{}
	var walk func(c *command, path []string)
	walk = func(c *command, path []string) {
		if c.Subs == nil && c.Store == nil {
			for _, a := range idArgs(c) {
				want[strings.Join(path, " ")+" "+a] = true
			}
		}
		for _, s := range c.Subs {
			walk(s, append(append([]string{}, path...), s.Name))
		}
	}
	walk(tree, nil)
	for name, sc := range storeCommands {
		fs := pflag.NewFlagSet(name, pflag.ContinueOnError)
		if sc.flags != nil {
			sc.flags(fs)
		}
		path := strings.TrimSuffix(strings.TrimSuffix(name, " ID"), " --task") // a form's path is its command's
		for _, a := range idArgs(storeHelpCommand(sc, fs)) {
			want[path+" "+a] = true
		}
	}
	b, err := os.ReadFile("testdata/proposal_commands.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Covers  string   `json:"covers"`
		Argv    []string `json:"argv"`
		Outcome string   `json:"outcome"`
	}
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, c := range cases {
		got[c.Covers] = true
		if !want[c.Covers] {
			t.Errorf("case %q covers no id argument of the command tree", c.Covers)
		}
		if !slices.Contains([]string{"allowed", "starts", "drops", "refused"}, c.Outcome) {
			t.Errorf("case %q: outcome %q is none of allowed, starts, drops, refused", c.Covers, c.Outcome)
		}
	}
	for k := range want {
		if !got[k] {
			t.Errorf("pm %s takes an item id, and no case in testdata/proposal_commands.json classes it for a "+
				"proposal", k)
		}
	}
}

// A drop writes no records, so it refuses what it would strand or leave open (the sprint-proposals design page,
// Commands): an open need under the proposal, named with how to close it, and a design-page draft tagged with it,
// until a commit tags the page with another sprint; then the drop closes it as dismissed.
func TestADropRefusesAnOpenNeedAndADraftTaggedWithTheProposal(t *testing.T) {
	_, storeDir, one, _ := workspaces(t)
	writeFile(t, filepath.Join(storeDir, "sprints/demo-2.md"), recordsSprint("Two", "demo.2",
		"[Parser](../design/parser.md)"))
	run(t, storeDir, "git commit -qam 'sprint 2 links the parser'")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	items := []work.Item{{ID: "demo", Type: work.Project, Title: "Demo", Status: work.Open, CreatedAt: at},
		{ID: "demo.1", Type: work.Sprint, Parent: "demo", Title: "One", Number: 1, Status: work.Open, CreatedAt: at},
		{ID: "demo.2", Type: work.Sprint, Parent: "demo", Title: "Two", Number: 2, Status: work.Open, CreatedAt: at,
			Labels: []string{work.ProposalLabel}},
		{ID: "demo.2.1", Type: work.Need, Parent: "demo.2", Title: "Which retry?", Status: work.Open, CreatedAt: at,
			Need: &work.NeedInfo{Kind: work.Decision}},
		{ID: "demo.1.1", Type: work.Task, Parent: "demo.1", Title: "T", Status: work.Open, CreatedAt: at,
			Holder: &work.Holder{Session: "s1", ClaimedAt: at}}}
	closed := &[]string{}
	use := func(items []work.Item) {
		OpenWork = func(string) (work.Store, error) { return closingStore{worktest.New(items), closed}, nil }
	}
	drop := []string{"sprint", "close", "demo.2", "--dropped", "--reason", "duplicate of demo.1"}
	use(items)
	if _, stderr, code := runPM(t, one.Code, drop...); code == 0 || !strings.Contains(stderr, "proposal demo.2 holds "+
		`an open need, demo.2.1 (Which retry?); close it first: pm need close demo.2.1 --dropped`) {
		t.Errorf("a drop with an open need: exit %d, %q", code, stderr)
	}
	use(slices.Delete(items, 3, 4)) // the need closed
	page := filepath.Join(one.Dir, "design/parser.md")
	edit(t, page, "\nP1\n", "\nP1 for the proposal\n")
	mustPM(t, one.Code, "commit", "-m", "the parser, for the proposal", "--sprint", "demo.2", "records/design/parser.md")
	if _, stderr, code := runPM(t, one.Code, drop...); code == 0 || !strings.Contains(stderr,
		"proposal demo.2 holds design-page drafts only its close publishes (design/parser.md)") {
		t.Errorf("a drop with a draft tagged with it: exit %d, %q", code, stderr)
	}
	edit(t, page, "\nP1 for the proposal\n", "\nP1\n") // the draft put back to the published page: no draft left
	mustPM(t, one.Code, "commit", "-m", "the parser as published", "--sprint", "demo.2", "records/design/parser.md")
	if out := mustPM(t, one.Code, drop...); !strings.Contains(out, "dropped proposal demo.2") || len(*closed) != 1 {
		t.Errorf("the drop said %q, closed %v", out, *closed)
	}
}

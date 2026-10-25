package cli

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/pflag"
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

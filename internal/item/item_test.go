package item

import (
	"os/exec"
	"strings"
	"testing"
)

// The packages that only read items build, and link their test binaries, without Dolt: a Dolt link costs 5-8 s of
// CPU on macOS CI per test binary. A Dolt import reaching any of them, through work or otherwise, fails here, naming
// the package that imports Dolt. go test's cache does not see what go list reads: CI runs it with -count=1.
func TestTheItemReadersLinkNoDolt(t *testing.T) {
	readers := []string{".", "../records", "../store", "../design", "../site"}
	args := append([]string{"list", "-tags", "gms_pure_go", "-test", "-deps", "-f",
		`{{.ImportPath}}{{range .Imports}} {{.}}{{end}}`}, readers...)
	out, err := exec.Command("go", args...).Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pkg, imports, _ := strings.Cut(line, " ")
		if strings.HasPrefix(pkg, "github.com/dolthub/") {
			continue
		}
		for _, im := range strings.Fields(imports) {
			if strings.HasPrefix(im, "github.com/dolthub/") {
				t.Errorf("%s imports %s, so %v link Dolt", pkg, im, readers)
			}
		}
	}
}

// A proposal's state follows from its label and its tasks alone (the sprint-proposals design page, Representation):
// no task leaves it unstarted, open or closed; any task, of any status, starts it, and once every task moved out it
// is unstarted again; a need under it does not count, and a sprint without the label is never one.
func TestAProposalIsUnstartedUntilATaskIsUnderIt(t *testing.T) {
	proposal := &Item{ID: "p.1", Type: Sprint, Labels: []string{ProposalLabel}}
	closed := &Item{ID: "p.2", Type: Sprint, Labels: []string{"bug", ProposalLabel}, Status: Closed}
	plain := &Item{ID: "p.3", Type: Sprint}
	task := &Item{ID: "p.1.1", Type: Task, Parent: "p.1", Status: Closed}
	need := &Item{ID: "p.1.2", Type: Need, Parent: "p.1"}
	for _, c := range []struct {
		name string
		s    *Item
		kids []*Item
		want bool
	}{
		{"no task", proposal, nil, true},
		{"a need only", proposal, []*Item{need}, true},
		{"a closed task", proposal, []*Item{need, task}, false},
		{"closed with no task", closed, nil, true},
		{"no label", plain, nil, false},
		{"a task labelled proposal", &Item{ID: "p.1.3", Type: Task, Labels: []string{ProposalLabel}}, nil, false},
	} {
		if got := Unstarted(c.s, c.kids); got != c.want {
			t.Errorf("%s: Unstarted = %v, want %v", c.name, got, c.want)
		}
	}
}

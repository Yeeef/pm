package item

import (
	"os/exec"
	"strings"
	"testing"
	"time"
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

// A proposal is a sprint carrying the label (the sprint-proposals design page, Representation), whatever is under it;
// a task carrying it is none. Promoted dates the promote by its note, and is zero without one.
func TestAProposalIsASprintLabelledProposalAndItsPromoteIsDatedByItsNote(t *testing.T) {
	at := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		it   *Item
		want bool
	}{
		{"labelled", &Item{Type: Sprint, Labels: []string{ProposalLabel}}, true},
		{"labelled and closed", &Item{Type: Sprint, Labels: []string{"bug", ProposalLabel}, Status: Closed}, true},
		{"promoted", &Item{Type: Sprint}, false},
		{"a task labelled proposal", &Item{Type: Task, Labels: []string{ProposalLabel}}, false},
	} {
		if got := IsProposal(c.it); got != c.want {
			t.Errorf("%s: IsProposal = %v, want %v", c.name, got, c.want)
		}
	}
	promoted := &Item{Type: Sprint, Comments: []Comment{{Kind: Reply, Text: PromoteNote, CreatedAt: at.AddDate(0, 0, 1)},
		{Kind: Note, Author: "sess", Text: PromoteNote, CreatedAt: at}}}
	if got := Promoted(promoted); !got.Equal(at) {
		t.Errorf("Promoted = %v, want %v", got, at)
	}
	if got := Promoted(&Item{Type: Sprint}); !got.IsZero() {
		t.Errorf("Promoted of a sprint never promoted = %v, want zero", got)
	}
}

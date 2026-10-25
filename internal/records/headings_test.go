package records

import "testing"

// pm show --record without --section prints the header and each heading with its section's line count and the
// command that prints it; a name with a space is shell-quoted, and a ### heading counts only its own lines.
func TestRecordOutlineListsEachSectionWithItsCommand(t *testing.T) {
	rec := &Record{
		Rel: "sprints/demo-1",
		Meta: Meta{
			"type":  {Kind: KindStr, Str: "sprint"},
			"title": {Kind: KindStr, Str: "First"},
			"bead":  {Kind: KindStr, Str: "repo-demo.1"},
		},
		Body: "\n## Goal\n\nShip it.\n\n## Delivery report\n\n### Outcome\n\ndone.\n",
	}
	want := `records/sprints/demo-1.md
  bead: repo-demo.1
  title: First
  type: sprint
sections, each with its line count and the command that prints it:
  ## Goal  (3 lines)  pm show --record demo-1 --section Goal
  ## Delivery report  (5 lines)  pm show --record demo-1 --section 'Delivery report'
    ### Outcome  (3 lines)  pm show --record demo-1 --section Outcome`
	if got := Outline(rec, "demo-1"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

// pm show --record --section: a section cuts at the next heading of its level; an unknown name is refused with the
// record's sections. The expected values were Python pm 0.1.x's on the same input, frozen.
func TestSectionCutsAtTheNextHeadingOfItsLevelAndRefusesAnUnknownName(t *testing.T) {
	rec := &Record{Rel: "sprints/demo-1", Body: "## Goal\n> prompt\n\nG.\n\n### Sub\n\ns\n\n## Scope\n\nIn.\n"}
	got, err := Section(rec, "Goal")
	if err != nil || got != "## Goal\n> prompt\n\nG.\n\n### Sub\n\ns" {
		t.Fatalf("Goal: %q %v", got, err)
	}
	if got, err = Section(rec, "Sub"); err != nil || got != "### Sub\n\ns" {
		t.Fatalf("Sub: %q %v", got, err)
	}
	_, err = Section(rec, "Nope")
	want := "records/sprints/demo-1.md has no section named \"Nope\"; its sections:\n  Goal\n    Sub\n  Scope"
	if err == nil || err.Error() != want {
		t.Fatalf("Nope: %v", err)
	}
}

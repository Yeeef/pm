package cli

import (
	"testing"

	"github.com/Yeeef/pm/internal/records"
)

// pm show --record without --section prints the header and each heading with its section's line count and the
// command that prints it; a name with a space is shell-quoted, and a ### heading counts only its own lines.
func TestRecordOutlineListsEachSectionWithItsCommand(t *testing.T) {
	rec := &records.Record{
		Rel: "sprints/demo-1",
		Meta: records.Meta{
			"type":  {Kind: records.KindStr, Str: "sprint"},
			"title": {Kind: records.KindStr, Str: "First"},
			"bead":  {Kind: records.KindStr, Str: "repo-demo.1"},
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
	if got := recordOutline(rec, "demo-1"); got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

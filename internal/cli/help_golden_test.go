package cli

import (
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The --help text of every command in the tree, frozen in testdata/help.golden, so a change to any help text shows in
// the PR's diff, a rename's search-and-replace reaching into a help string included. After an intended change,
// rewrite it with
//
//	go test -tags gms_pure_go ./internal/cli -run Golden -update
//
// and review the diff.

var update = flag.Bool("update", false, "rewrite testdata/help.golden from the help texts printed now")

const helpGolden = "testdata/help.golden"

// helpTexts is pm <path> --help for c and every command under it, each after a line naming its path, and
// pm <path> --help --all for a noun with hidden subcommands.
func helpTexts(t *testing.T, c *command, path []string, out *strings.Builder) {
	t.Helper()
	out.WriteString("===== pm " + strings.Join(append(append([]string{}, path...), "--help"), " ") + "\n")
	out.WriteString(help(t, path...))
	if slices.ContainsFunc(c.Subs, func(s *command) bool { return s.Hidden }) {
		out.WriteString("===== pm " + strings.Join(append(append([]string{}, path...), "--help", "--all"), " ") + "\n")
		out.WriteString(help(t, append(append([]string{}, path...), "--all")...))
	}
	for _, s := range c.Subs {
		helpTexts(t, s, append(append([]string{}, path...), s.Name), out)
	}
}

func TestHelpTextsEqualTheGolden(t *testing.T) {
	t.Setenv("COLUMNS", "80")
	var got strings.Builder
	helpTexts(t, tree, nil, &got)
	if *update {
		if err := os.MkdirAll(filepath.Dir(helpGolden), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(helpGolden, []byte(got.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(helpGolden)
	if err != nil {
		t.Fatalf("%v; write it with -update", err)
	}
	if got.String() != string(want) {
		gotLines, wantLines := strings.Split(got.String(), "\n"), strings.Split(string(want), "\n")
		for i := 0; i < len(gotLines) || i < len(wantLines); i++ {
			var g, w string
			if i < len(gotLines) {
				g = gotLines[i]
			}
			if i < len(wantLines) {
				w = wantLines[i]
			}
			if g != w {
				t.Fatalf("help texts differ from %s at line %d:\n got: %q\nwant: %q\nrewrite it with -update after an "+
					"intended change", helpGolden, i+1, g, w)
			}
		}
	}
}

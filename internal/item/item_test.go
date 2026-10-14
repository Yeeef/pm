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

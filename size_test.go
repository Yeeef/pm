package pm

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// maxPackageLines caps a package's non-test Go lines: a package past it is split along a seam, not grown.
const maxPackageLines = 5000

// oversized holds the packages already past the cap at the size each may reach, a ratchet: lower an entry as its
// package shrinks, and drop it once the package fits the cap.
var oversized = map[string]int{
	"internal/cli": 7414,
}

// TestPackagesStayUnderTheSizeCap counts each package directory's lines in its non-test .go files.
func TestPackagesStayUnderTheSizeCap(t *testing.T) {
	sizes := map[string]int{}
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p != "." && (strings.HasPrefix(name, ".") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lines := bytes.Count(data, []byte("\n"))
		if strings.HasPrefix(filepath.ToSlash(p), "internal/cli/") && lines > 800 {
			t.Errorf("%s has %d lines, over the command file cap of 800", p, lines)
		}
		if !strings.HasSuffix(name, "_test.go") {
			sizes[filepath.ToSlash(filepath.Dir(p))] += lines
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for pkg, n := range sizes {
		allowed, listed := oversized[pkg]
		switch {
		case !listed && n > maxPackageLines:
			t.Errorf("%s has %d non-test Go lines, over the cap of %d: split it along a seam", pkg, n,
				maxPackageLines)
		case listed && n > allowed:
			t.Errorf("%s has %d non-test Go lines, over the %d it is allowed while it is cut down: split it, "+
				"do not raise its entry in oversized", pkg, n, allowed)
		case listed && n <= maxPackageLines:
			t.Errorf("%s has %d non-test Go lines, within the cap: drop its entry in oversized", pkg, n)
		}
	}
	for pkg := range oversized {
		if _, ok := sizes[pkg]; !ok {
			t.Errorf("oversized names %s, which holds no Go file: drop its entry", pkg)
		}
	}
}

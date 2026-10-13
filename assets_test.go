package pm

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestAssetsBuildWithNoEntryFiles builds this package from a copy of the files assets.go embeds in which changelog.d/
// holds no entry file, as a checkout right after a release does: go:embed refuses a directory with nothing in it, so
// changelog.d/README.md, tracked and never deleted by a release, must keep the build working.
func TestAssetsBuildWithNoEntryFiles(t *testing.T) {
	if _, err := os.Stat("changelog.d/README.md"); err != nil {
		t.Fatalf("changelog.d/README.md keeps changelog.d/ from being empty: %v", err)
	}
	src, err := os.ReadFile("assets.go")
	if err != nil {
		t.Fatal(err)
	}
	mod, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	goLine := regexp.MustCompile(`(?m)^go .*$`).Find(mod)
	write := func(rel string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", []byte("module github.com/Yeeef/pm\n\n"+string(goLine)+"\n"))
	write("assets.go", src)
	for _, m := range regexp.MustCompile(`(?m)^//go:embed (.+)$`).FindAllSubmatch(src, -1) {
		for _, pattern := range strings.Fields(string(m[1])) {
			err := filepath.WalkDir(pattern, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				if filepath.Dir(p) == "changelog.d" && d.Name() != "README.md" {
					return nil // an entry file: the copy has none
				}
				data, err := os.ReadFile(p)
				if err == nil {
					write(p, data)
				}
				return err
			})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build with no entry files: %v\n%s", err, out)
	}
}

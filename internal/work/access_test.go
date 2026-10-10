package work

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// One access path (the pm-go page, Store access): only the host opens the store, and only pm service run starts a
// host. Over every non-test Go file in the module: every call into dolthub/driver (its LoadMultiEnvFromDir, its
// NewConnector) and into Dolt's engine package (NewSqlEngine) is in host.go alone, and the calls of NewHost are in pm
// service run (internal/cli/serve.go) and the test helper worktest. A new direct open fails this test, so it fails CI.
func TestOnlyTheHostOpensTheStoreAndOnlyTheServiceStartsIt(t *testing.T) {
	engines := map[string]bool{"github.com/dolthub/driver/v2": true,
		"github.com/dolthub/dolt/go/cmd/dolt/commands/engine": true}
	got := map[string][]string{} // call -> the files that make it
	root := filepath.Join("..", "..")
	files, err := moduleGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		imports := map[string]string{} // the name a file calls a package by -> its path
		for _, im := range f.Imports {
			p := strings.Trim(im.Path.Value, `"`)
			name := filepath.Base(p)
			if p == "github.com/dolthub/driver/v2" {
				name = "embedded" // the package's own name
			}
			if im.Name != nil {
				name = im.Name.Name
			}
			imports[name] = p
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			var name string
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				if x, ok := fn.X.(*ast.Ident); ok && engines[imports[x.Name]] {
					name = imports[x.Name] + "." + fn.Sel.Name
				} else if fn.Sel.Name == "NewHost" {
					name = "NewHost"
				}
			case *ast.Ident:
				if fn.Name == "NewHost" {
					name = "NewHost"
				}
			}
			if name != "" {
				got[name] = append(got[name], filepath.ToSlash(rel))
			}
			return true
		})
	}
	for name, files := range got {
		sort.Strings(files)
		want := []string{"internal/work/host.go"}
		if name == "NewHost" {
			want = []string{"internal/cli/serve.go", "internal/work/worktest/worktest.go"}
		}
		if strings.Join(dedupe(files), " ") != strings.Join(want, " ") {
			t.Errorf("%s is called in %v; only %v may call it", name, dedupe(files), want)
		}
	}
	for _, name := range []string{"github.com/dolthub/driver/v2.LoadMultiEnvFromDir",
		"github.com/dolthub/dolt/go/cmd/dolt/commands/engine.NewSqlEngine", "NewHost"} {
		if len(got[name]) == 0 {
			t.Errorf("no call of %s found: the scan is broken", name)
		}
	}
}

// moduleGoFiles is every non-test Go file of the module at root, as the go command counts it: a directory with a
// go.mod of its own (another module, or a worktree of this repo under .claude/worktrees) is not in it, nor is .git,
// testdata, .go (the build output) or .venv.
func moduleGoFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			if e.Name() == ".git" || e.Name() == ".go" || e.Name() == "testdata" || e.Name() == ".venv" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

// A worktree of the repo inside the checkout (.claude/worktrees/<name>) holds its own copy of every file; the scan
// leaves it out, so the access test passes from the main checkout too.
func TestModuleGoFilesLeaveOutNestedModulesAndWorktrees(t *testing.T) {
	root := t.TempDir()
	for _, f := range []string{"go.mod", "a.go", "a_test.go", "sub/b.go", "testdata/c.go", "nested/go.mod", "nested/d.go",
		".claude/worktrees/x/go.mod", ".claude/worktrees/x/internal/work/host.go", ".claude/settings.go"} {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	files, err := moduleGoFiles(root)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		got = append(got, filepath.ToSlash(rel))
	}
	sort.Strings(got)
	if want := []string{".claude/settings.go", "a.go", "sub/b.go"}; strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("module files %v, want %v", got, want)
	}
}

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

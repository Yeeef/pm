package work

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
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
	err := filepath.WalkDir(root, func(path string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() && (e.Name() == ".git" || e.Name() == ".go" || e.Name() == "testdata" || e.Name() == ".venv") {
			return filepath.SkipDir
		}
		if e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
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
		return nil
	})
	if err != nil {
		t.Fatal(err)
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

func dedupe(s []string) []string {
	var out []string
	for i, v := range s {
		if i == 0 || v != s[i-1] {
			out = append(out, v)
		}
	}
	return out
}

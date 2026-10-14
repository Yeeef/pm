package pm

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// No test takes a port from service.FreePort. FreePort is the first free port from 8000 that no pm service unit in this
// HOME names, and a test fakes a HOME of its own, so two test processes running at once (make test-go runs
// internal/service plain and under -race in parallel; other worktrees run it too) get the same port, and the service
// that binds it second fails (":8002 is held by another store"). A test takes a port the kernel picks instead: listen
// on 127.0.0.1:0, or give the service port 0. Over every Go file of the module, each call of FreePort is in a non-test
// file. It is the root package's, whose tests read every package's files, so a pull request that changes any package
// runs it (.github/scripts/go_changes.py select).
func TestNoTestTakesFreePortsSitePort(t *testing.T) {
	files, err := moduleGoFiles(".", true)
	if err != nil {
		t.Fatal(err)
	}
	var callers, inTests []string
	for _, path := range files {
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := ""
			switch fn := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			case *ast.Ident:
				name = fn.Name
			}
			if name == "FreePort" {
				at := fset.Position(call.Pos())
				where := filepath.ToSlash(at.Filename)
				callers = append(callers, where)
				if strings.HasSuffix(where, "_test.go") {
					inTests = append(inTests, where+":"+strconv.Itoa(at.Line))
				}
			}
			return true
		})
	}
	if len(inTests) > 0 {
		t.Errorf("a test takes its port from FreePort, which gives test processes running at once the same port: %v; "+
			"take a port the kernel picks (net.Listen on 127.0.0.1:0)", inTests)
	}
	if !slices.Contains(callers, "internal/cli/setup/init.go") {
		t.Errorf("no call of FreePort found in internal/cli/setup/init.go (calls in %v): the scan is broken", callers)
	}
}

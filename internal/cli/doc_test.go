package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Every func's doc comment in this package and its subpackages starts with the func's name, so a comment left behind
// when code moves between files, or a func renamed without its comment, fails here.
func TestEveryFuncDocStartsWithItsName(t *testing.T) {
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Doc == nil {
				continue
			}
			if first, _, _ := strings.Cut(fn.Doc.Text(), " "); first != fn.Name.Name {
				t.Errorf("%s: the doc comment of %s starts with %q", p, fn.Name.Name, first)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

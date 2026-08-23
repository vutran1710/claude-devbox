package shell

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// This package once set os.Setenv("PATH", FullPATH) in an init(), so merely
// importing it rewrote the PATH of the whole process. On a Mac that erased
// /opt/homebrew/bin, making tmux and gh invisible to unrelated code — an
// integration test in another package skipped itself for a week before anyone
// noticed why. Nothing here may mutate process state at import time again.
func TestPackageDeclaresNoInit(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse package: %v", err)
	}
	for _, pkg := range pkgs {
		for path, file := range pkg.Files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if ok && fn.Name.Name == "init" && fn.Recv == nil {
					t.Errorf("%s declares init() — importing this package must not change process state", path)
				}
			}
		}
	}
}

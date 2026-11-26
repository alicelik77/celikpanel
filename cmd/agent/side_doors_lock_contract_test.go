package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// S4 R1, fourth fix round (finding 5): every RPC that moves a tool, rewrites
// its web server configuration or swaps its tree takes sideDoorsMu after its
// job step, so the hourly read never probes a tool in the middle of one. The
// helpers they share (bindPHPToolWith, configureWebmailWith,
// provePHPToolWith, updateRoundcubeTo) do not take it: the step already holds
// it when it calls them, and the lock is not re-entrant. Fails on 336c1da28
// (the proof's fallback move, ConfigureWebmail and the webmail update ran
// beside the read).
func TestEveryToolMoveTakesTheSideDoorsLockAfterItsStep(t *testing.T) {
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	methods := map[string]*ast.FuncDecl{}
	helpers := map[string]*ast.FuncDecl{}
	for _, name := range files {
		// The Linux build: test files and other systems' stand-ins are left out.
		if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_other.go") || strings.HasSuffix(name, "_stub.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			if fn.Recv != nil {
				methods[fn.Name.Name] = fn
			} else {
				helpers[fn.Name.Name] = fn
			}
		}
	}
	// position of the first call named name inside fn (selector or ident).
	firstCall := func(fn *ast.FuncDecl, name string) token.Pos {
		found := token.NoPos
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || found != token.NoPos {
				return found == token.NoPos
			}
			switch f := call.Fun.(type) {
			case *ast.SelectorExpr:
				if ident, ok := f.X.(*ast.Ident); ok && ident.Name+"."+f.Sel.Name == name {
					found = call.Pos()
				} else if f.Sel.Name == name {
					found = call.Pos()
				}
			case *ast.Ident:
				if f.Name == name {
					found = call.Pos()
				}
			}
			return true
		})
		return found
	}
	for _, name := range []string{"CloseSideDoorsV1", "BindPHPToolV1", "ProvePHPToolV1", "ConfigureWebmail", "UpdateRoundcubeV1"} {
		fn := methods[name]
		if fn == nil {
			t.Fatalf("%s not found", name)
		}
		step, lock := firstCall(fn, "requiredServiceMutationStep"), firstCall(fn, "sideDoorsMu.Lock")
		if lock == token.NoPos {
			t.Errorf("%s does not take sideDoorsMu", name)
			continue
		}
		if step == token.NoPos || lock < step {
			t.Errorf("%s takes sideDoorsMu before its job step", name)
		}
	}
	for _, name := range []string{"bindPHPToolWith", "provePHPToolWith", "updateRoundcubeTo", "closeSideDoorsTools", "closeStockFPMPoolsFor"} {
		fn := helpers[name]
		if fn == nil {
			t.Fatalf("%s not found", name)
		}
		if firstCall(fn, "sideDoorsMu.Lock") != token.NoPos {
			t.Errorf("%s takes sideDoorsMu itself (it is called with it held)", name)
		}
	}
	if fn := methods["configureWebmailWith"]; fn == nil || firstCall(fn, "sideDoorsMu.Lock") != token.NoPos {
		t.Error("configureWebmailWith is called under the step's lock and must not take it")
	}
}

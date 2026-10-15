package commands

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// TestNextStepAvoidsWallClockBudget guards D-0492: shared runners can pause
// after a deadline expires, so elapsed time cannot determine test success.
func TestNextStepAvoidsWallClockBudget(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "next_step_test.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "TestNextStep" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			pkg, ok := sel.X.(*ast.Ident)
			if ok && pkg.Name == "time" && (sel.Sel.Name == "Now" || sel.Sel.Name == "Since") {
				t.Error("TestNextStep must assert the context deadline, not elapsed wall time")
			}
			return true
		})
		return
	}
	t.Fatal("TestNextStep not found")
}

// Package main — middleware.go
// Classifies a route's middleware chain against the allowlist.
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
)

// CheckResult holds the outcome of checking one route.
type CheckResult struct {
	Route      RouteRegistration
	Passed     bool     // true = has an allowed middleware or is exempt
	Exempt     bool     // true = path matched an ExemptRoute
	MatchedMW  string   // which allowlisted middleware matched (empty if exempt)
	UnknownMWs []string // middlewares that are not in the allowlist and not exempt
}

// ClassifyRoute checks a single route against the allowlist and exempt list.
func ClassifyRoute(r RouteRegistration, failOnUnknown bool) CheckResult {
	cr := CheckResult{Route: r}
	// Only the private Server mux qualifies: Handler wraps it in authorized.
	// A global http.HandleFunc in the same directory still fails closed.
	if privateHTTPAPIMux(r) {
		cr.Passed, cr.MatchedMW = true, "httpapi.Server.authorized"
		return cr
	}

	// 1. Check exemptions first.
	if r.Path != "" && isExempt(r.Path) {
		cr.Passed = true
		cr.Exempt = true
		return cr
	}

	// 2. Check each middleware against the allowlist.
	for _, mw := range r.Middlewares {
		if _, ok := AllowedAuthMiddleware[mw]; ok {
			cr.Passed = true
			cr.MatchedMW = mw
			return cr
		}
	}

	// 3. Collect unknown middlewares for reporting.
	for _, mw := range r.Middlewares {
		if _, ok := AllowedAuthMiddleware[mw]; !ok {
			cr.UnknownMWs = append(cr.UnknownMWs, mw)
		}
	}

	// 4. No auth middleware found.
	cr.Passed = false
	return cr
}

// privateHTTPAPIMux recognizes the exact registration and authenticated mux
// wrapper in Server's source, rather than exempting a directory or route name.
func privateHTTPAPIMux(r RouteRegistration) bool {
	if filepath.Base(r.File) != "server.go" || filepath.Base(filepath.Dir(r.File)) != "httpapi" || filepath.Base(filepath.Dir(filepath.Dir(r.File))) != "internal" {
		return false
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, r.File, nil, 0)
	if err != nil {
		return false
	}
	registered, wrapped := false, false
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		isServerMethod := false
		if fn.Recv != nil && len(fn.Recv.List) == 1 {
			if ptr, ok := fn.Recv.List[0].Type.(*ast.StarExpr); ok {
				if name, ok := ptr.X.(*ast.Ident); ok {
					isServerMethod = name.Name == "Server"
				}
			}
		}
		if fn.Name.Name == "Handler" && isServerMethod {
			returns, safe := 0, true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if ret, ok := n.(*ast.ReturnStmt); ok {
					returns++
					safe = safe && len(ret.Results) == 1 && authenticatedMux(ret.Results[0])
				}
				return true
			})
			wrapped = returns > 0 && safe
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if fset.Position(call.Pos()).Line == r.Line && serverMux(sel.X) && (fn.Name.Name == "New" || fn.Name.Name == "Mount" && isServerMethod) && (sel.Sel.Name == "Handle" || sel.Sel.Name == "HandleFunc") {
				registered = true
			}
			return true
		})
	}
	return registered && wrapped
}

func authenticatedMux(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	name, ok := sel.X.(*ast.Ident)
	if !ok || name.Name != "s" {
		return false
	}
	if sel.Sel.Name == "authorized" {
		return serverMux(call.Args[0])
	}
	return sel.Sel.Name == "logged" && authenticatedMux(call.Args[0])
}

func serverMux(expr ast.Expr) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "mux" {
		return false
	}
	name, ok := sel.X.(*ast.Ident)
	return ok && name.Name == "s"
}

// ClassifyAll runs ClassifyRoute on every route and returns only the failures.
func ClassifyAll(routes []RouteRegistration) []CheckResult {
	var failures []CheckResult
	for _, r := range routes {
		cr := ClassifyRoute(r, true)
		if !cr.Passed {
			failures = append(failures, cr)
		}
	}
	return failures
}

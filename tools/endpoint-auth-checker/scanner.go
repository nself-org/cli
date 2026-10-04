// Package main — scanner.go
// AST walker that finds HTTP route registrations across chi/gin/echo/mux patterns.
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RouteRegistration represents a single HTTP route found during scanning.
type RouteRegistration struct {
	File        string
	Line        int
	Method      string   // GET, POST, PUT, DELETE, PATCH, Handle, Use, etc.
	Path        string   // extracted string literal path, or "" if dynamic
	Middlewares []string // function names in the middleware chain (extracted from args)
}

// skipDirs are directory names ScanDirs does not descend into below a scan
// root: vendored and fixture code is not this repo's route surface. A root
// that itself points into one of them (e.g. testdata/compliant_plugin) is
// still scanned. node_modules is deliberately not listed: this is a Go-only
// checker and the Go toolchain compiles a package at x/node_modules/y, so
// skipping it would let real routes through.
var skipDirs = map[string]bool{
	"vendor":   true,
	"testdata": true,
}

// ScanDirs walks each directory recursively, parses all .go files,
// and returns every route registration found. It fails closed: a directory
// that does not exist, is not a directory, or cannot be read is an error, a
// symlinked root is resolved before walking, and a root in which no .go file
// was parsed is an error (a scan that inspects nothing must not look green).
// Individual .go files that fail to parse are skipped but do not count as
// scanned.
func ScanDirs(dirs []string) ([]RouteRegistration, error) {
	var results []RouteRegistration
	for _, dir := range dirs {
		info, err := os.Stat(dir)
		if err != nil {
			return nil, fmt.Errorf("directory %q: %w", dir, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("directory %q: not a directory", dir)
		}
		// WalkDir does not follow a symlink root; resolve it so a link given
		// without a trailing slash is scanned instead of silently skipped.
		root, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return nil, fmt.Errorf("directory %q: %w", dir, err)
		}
		scanned := 0
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return fmt.Errorf("walking %q: %w", path, err)
			}
			if d.IsDir() {
				if path != root && skipDirs[d.Name()] {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") {
				return nil
			}
			routes, err := scanFile(path)
			if err != nil {
				return nil // skip unparseable files
			}
			scanned++
			results = append(results, routes...)
			return nil
		})
		if err != nil {
			return nil, err
		}
		if scanned == 0 {
			return nil, fmt.Errorf("directory %q: no .go files were scanned", dir)
		}
	}
	return results, nil
}

// routerMethods is the set of method names on router objects we treat as route registrations.
// Includes chi, gin, echo, and http.ServeMux patterns plus common nSelf wrappers.
var routerMethods = map[string]bool{
	"Handle":  true,
	"Get":     true,
	"Post":    true,
	"Put":     true,
	"Delete":  true,
	"Patch":   true,
	"Options": true,
	"Head":    true,
	// http.ServeMux uses HandleFunc
	"HandleFunc": true,
	// echo
	"GET":     true,
	"POST":    true,
	"PUT":     true,
	"DELETE":  true,
	"PATCH":   true,
	"OPTIONS": true,
	"HEAD":    true,
}

func scanFile(path string) ([]RouteRegistration, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, err
	}

	var routes []RouteRegistration

	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}

		method := sel.Sel.Name
		if !routerMethods[method] {
			return true
		}

		// Extract path (first string literal arg) and middlewares (remaining function-valued args).
		rr := RouteRegistration{
			File:   path,
			Line:   fset.Position(call.Pos()).Line,
			Method: method,
		}

		for i, arg := range call.Args {
			if i == 0 {
				if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					rr.Path = strings.Trim(lit.Value, `"`)
				}
				continue
			}
			// Remaining args: extract function/identifier names as middleware candidates.
			rr.Middlewares = append(rr.Middlewares, extractFuncNames(arg)...)
		}

		routes = append(routes, rr)
		return true
	})

	return routes, nil
}

// extractFuncNames extracts all function-like identifiers from an AST expression.
// Handles: bare ident (MyMiddleware), call expr (MyMiddleware()), selector (pkg.Func).
func extractFuncNames(expr ast.Expr) []string {
	var names []string
	switch e := expr.(type) {
	case *ast.Ident:
		names = append(names, e.Name)
	case *ast.SelectorExpr:
		// pkg.Func — return the function name only (not the package).
		names = append(names, e.Sel.Name)
	case *ast.CallExpr:
		// MyMiddleware(args) — the function being called.
		names = append(names, extractFuncNames(e.Fun)...)
	case *ast.FuncLit:
		// Inline function literal — skip (no named middleware).
	}
	return names
}

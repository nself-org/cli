package health

// Purpose: guard against a redis-cli password on a process argument list.
// ProbeRedis once passed "-a <password>" to docker exec, which exposes the
// password in the process table. It was removed (D-0549, no callers). This
// test keeps any redis-cli call with a password flag from returning.
// Inputs:  every non-test .go file in this package directory.
// Outputs: a test failure naming file:line for each function body that holds
//          both a "redis-cli" string literal and a password flag literal.
// Constraints: parse only, no exec, no network. A positive control proves the
//              detector flags the old ProbeRedis argv.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// redisPasswordFlags are the flag literals that carry a credential.
var redisPasswordFlags = map[string]bool{"-a": true, "--pass": true, "-u": true, "--user": true}

// redisArgvViolations parses src and returns "name:line" for each function
// body that holds both "redis-cli" and a password flag literal.
func redisArgvViolations(name string, src any) ([]string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, name, src, 0)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		hasCLI, hasFlag := false, false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			v, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			if v == "redis-cli" {
				hasCLI = true
			}
			if redisPasswordFlags[v] {
				hasFlag = true
			}
			return true
		})
		if hasCLI && hasFlag {
			out = append(out, name+":"+strconv.Itoa(fset.Position(fn.Pos()).Line))
		}
	}
	return out, nil
}

func TestNoRedisCLIPasswordArgv(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil || len(files) == 0 {
		t.Fatalf("no go files found: %v", err)
	}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		v, err := redisArgvViolations(f, nil)
		if err != nil {
			t.Fatalf("parse %s: %v", f, err)
		}
		for _, loc := range v {
			t.Errorf("%s: redis-cli with a password flag puts the secret on argv; use REDISCLI_AUTH in the container env", loc)
		}
	}
}

// TestRedisArgvDetectorPositiveControl proves the detector is not a no-op.
func TestRedisArgvDetectorPositiveControl(t *testing.T) {
	old := `package x
func ProbeRedis(id, password string) {
	args := []string{"exec", id, "redis-cli"}
	if password != "" {
		args = append(args, "-a", password)
	}
}
`
	clean := `package x
func ProbeRedis(id string) {
	args := []string{"exec", id, "redis-cli", "ping"}
	_ = args
}
func Other() { _ = []string{"-a", "x"} }
`
	got, err := redisArgvViolations("old.go", old)
	if err != nil || len(got) != 1 {
		t.Fatalf("old ProbeRedis argv must be flagged once, got %v err %v", got, err)
	}
	got, err = redisArgvViolations("clean.go", clean)
	if err != nil || len(got) != 0 {
		t.Fatalf("clean source must not be flagged, got %v err %v", got, err)
	}
}

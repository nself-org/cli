package repoqa

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// TestCommandTestShardsInCI keeps D-0735's package timeout headroom without
// dropping or repeating tests. It reads the real workflow and test declarations,
// so new command tests must also belong to exactly one bounded CI invocation.
func TestCommandTestShardsInCI(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, ".github/workflows/ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name            string `yaml:"name"`
				Run             string `yaml:"run"`
				If              string `yaml:"if"`
				ContinueOnError bool   `yaml:"continue-on-error"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		t.Fatal(err)
	}
	var shards []*regexp.Regexp
	selector := regexp.MustCompile(`-run '([^']+)' ./cmd/commands$`)
	for _, step := range workflow.Jobs["ci"].Steps {
		if step.Name == "Test" && !strings.Contains(step.Run, `grep -v '/cmd/commands$'`) {
			t.Error("the module test run must exclude cmd/commands, which runs in shards")
		}
		if !strings.HasPrefix(step.Name, "Test (commands ") {
			continue
		}
		if step.If != "matrix.skip_docker_tests != 'true'" || step.ContinueOnError {
			t.Errorf("%s must run on every non-Windows matrix leg and propagate failures", step.Name)
		}
		if !strings.HasPrefix(step.Run, "go test -mod=vendor -timeout 10m ") {
			t.Errorf("%s must preserve the 10m timeout", step.Name)
		}
		match := selector.FindStringSubmatch(strings.TrimSpace(step.Run))
		if match == nil {
			t.Fatalf("%s needs an explicit command test selector", step.Name)
		}
		shards = append(shards, regexp.MustCompile(match[1]))
	}
	if len(shards) != 3 {
		t.Fatalf("want three command test shards, got %d", len(shards))
	}
	checkCommandShardCoverage(t, root, shards)
}

// checkCommandShardCoverage parses all platform test files so a selector cannot
// silently omit a test that is compiled only on another runner.
func checkCommandShardCoverage(t *testing.T, root string, shards []*regexp.Regexp) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "cmd/commands/*_test.go"))
	if err != nil || len(files) == 0 {
		t.Fatalf("command test files: %v", err)
	}
	for _, path := range files {
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !isCommandTestEntry(fn.Name.Name) {
				continue
			}
			matches := 0
			for _, shard := range shards {
				if shard.MatchString(fn.Name.Name) {
					matches++
				}
			}
			if matches != 1 {
				t.Errorf("%s belongs to %d CI shards, want exactly one", fn.Name.Name, matches)
			}
		}
	}
}

// isCommandTestEntry mirrors go test's exported entry-point naming rule.
func isCommandTestEntry(name string) bool {
	if name == "TestMain" {
		return false
	}
	for _, prefix := range []string{"Test", "Example", "Fuzz"} {
		if strings.HasPrefix(name, prefix) {
			r, _ := utf8.DecodeRuneInString(strings.TrimPrefix(name, prefix))
			return !unicode.IsLower(r)
		}
	}
	return false
}

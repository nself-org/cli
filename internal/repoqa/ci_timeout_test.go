package repoqa

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestCIGoTestTimeout(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".nself/ci.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var pipeline struct {
		Jobs map[string]struct {
			Run     []string `yaml:"run"`
			Timeout string   `yaml:"timeout"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &pipeline); err != nil {
		t.Fatal(err)
	}
	job, ok := pipeline.Jobs["go-test"]
	if !ok {
		t.Fatal("go-test job missing")
	}
	var timeout time.Duration
	for i, arg := range job.Run {
		if arg == "-timeout" && i+1 < len(job.Run) {
			timeout, err = time.ParseDuration(job.Run[i+1])
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	outer, err := time.ParseDuration(job.Timeout)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		ok   bool
	}{
		{"allows the full suite twenty minutes", timeout >= 20*time.Minute},
		{"runner allows Go to report its own timeout", outer > timeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.ok {
				t.Fatalf("go-test timeout=%v runner timeout=%v", timeout, outer)
			}
		})
	}
}

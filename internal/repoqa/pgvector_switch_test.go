package repoqa

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestPGVectorSwitchMajor exercises fixture setup without starting Docker.
func TestPGVectorSwitchMajor(t *testing.T) {
	for _, major := range []string{"", "15", "17"} {
		t.Run("major_"+major, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			if err := os.Mkdir(bin, 0700); err != nil {
				t.Fatal(err)
			}
			mocks := map[string]string{
				"docker": "#!/bin/sh\nexit 0\n",
				"nself":  "#!/bin/sh\ncase $1 in\ninit) printf 'POSTGRES_VERSION=16-alpine\nPOSTGRES_IMAGE=postgres:16-alpine\n' > .env; touch .env.local;;\nbuild) cat .env .env.local > \"$W/captured.env\"; exit 42;;\nesac\n",
			}
			for name, script := range mocks {
				if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command("bash", "scripts/e2e/pgvector-switch.sh")
			cmd.Dir = repoRoot(t)
			cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "W="+dir, "POSTGRES_VERSION="+major)
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("fixture must stop at mocked build")
			}
			local, err := os.ReadFile(filepath.Join(dir, "e2e-adopt-pgs", ".env.local"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(local), "POSTGRES_IMAGE=") {
				t.Fatal("local image override would block the switch")
			}
			data, err := os.ReadFile(filepath.Join(dir, "captured.env"))
			if err != nil {
				t.Fatalf("fixture did not reach build: %v\n%s", err, out)
			}
			want := major
			if want == "" {
				want = "16"
			}
			for _, line := range []string{"POSTGRES_VERSION=" + want + "-alpine", "POSTGRES_IMAGE=postgres:" + want + "-alpine"} {
				if !strings.Contains("\n"+string(data), "\n"+line+"\n") {
					t.Errorf("fixture missing %q", line)
				}
			}
		})
	}
}

// TestPGVectorSwitchNightly keeps both non-default majors in the live lane.
func TestPGVectorSwitchNightly(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(repoRoot(t), ".github/workflows/e2e-pgvector-switch.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var w struct {
		On   struct{ Schedule []struct{ Cron string } } `yaml:"on"`
		Jobs map[string]struct {
			Strategy struct{ Matrix struct{ Postgres []string } }
			Env      map[string]string
			Steps    []struct{ Run string }
		}
	}
	if err := yaml.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	if len(w.On.Schedule) == 0 || !strings.HasSuffix(w.On.Schedule[0].Cron, " * * *") {
		t.Fatal("missing nightly schedule")
	}
	for _, job := range w.Jobs {
		if strings.Join(job.Strategy.Matrix.Postgres, ",") != "15,16,17" {
			continue
		}
		if job.Env["POSTGRES_VERSION"] != "${{ matrix.postgres }}" {
			continue
		}
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "bash scripts/e2e/pgvector-switch.sh") {
				return
			}
		}
	}
	t.Fatal("nightly must invoke pgvector-switch.sh with matrix majors 15, 16 and 17")
}

package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDoctorFixItEngineAllowlistAndReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executables are POSIX shell scripts")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=demo\nENV=dev\nBASE_DOMAIN=example.test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "runs")
	script := "#!/bin/sh\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$FIX_LOG\"\nexit ${FAKE_RC:-0}\n"
	for _, name := range []string{"nself", "docker", "sudo", "Set"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FIX_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	in := []CheckResult{
		{Name: "config", Status: "fail", FixCmd: "nself config set X Y"},
		{Name: "redis", Status: "fail", FixCmd: "docker restart demo_redis"},
		{Name: "prune", Status: "fail", FixCmd: "docker system prune -f"},
		{Name: "sudo", Status: "fail", FixCmd: "sudo systemctl restart docker"},
		{Name: "prose", Status: "fail", FixCmd: "Set MINIO_ROOT_USER=secret"},
		{Name: "foreign", Status: "fail", FixCmd: "docker restart other_redis"},
	}
	got := FixItEngine(context.Background(), dir, in)
	runs, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if string(runs) != "nself config set X Y\ndocker restart demo_redis\n" {
		t.Fatalf("commands run: %s", runs)
	}
	for i, r := range got {
		if i < 2 && (r.Status != "pass" || !strings.Contains(r.Message, "auto-fixed")) {
			t.Errorf("not propagated: %+v", r)
		}
		if i >= 2 && (r.Status != "fail" || !strings.Contains(r.Message, "manual step")) {
			t.Errorf("manual step lost: %+v", r)
		}
	}
}

func TestDoctorFixItEngineFailureInReport(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake executable is a POSIX shell script")
	}
	dir := t.TempDir()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "nself"), []byte("#!/bin/sh\nexit 7\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got := FixItEngine(context.Background(), dir, []CheckResult{{Name: "x", Status: "fail", FixCmd: "nself config set X Y"}})
	if len(got) != 1 || got[0].Status != "fail" || !strings.Contains(got[0].Message, "auto-fix failed") {
		t.Fatalf("failed fix lost: %+v", got)
	}
}

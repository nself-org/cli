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
	script := "#!/bin/sh\nif [ \"$1\" = inspect ]; then project=demo; case \"$*\" in *other_redis*) project=foreign;; esac; printf '{\"Config\":{\"Labels\":{\"com.docker.compose.project\":\"%s\"}}}\\n' \"$project\"; exit 0; fi\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$FIX_LOG\"\nexit ${FAKE_RC:-0}\n"
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
		t.Fatalf("commands run: %s; results: %+v", runs, got)
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

func TestReviewFixItRejectsMetacharactersAndUnownedContainer(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("POSIX shell unavailable")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PROJECT_NAME=demo\nENV=dev\nBASE_DOMAIN=example.test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "executed")
	stub := "#!/bin/sh\nif [ \"$1\" = inspect ]; then printf '%s\\n' '{\"Config\":{\"Labels\":{\"com.docker.compose.project\":\"foreign\"}}}'; exit 0; fi\nprintf '%s %s\\n' \"$(basename \"$0\")\" \"$*\" >> \"$REVIEW_FIX_LOG\"\n"
	for _, n := range []string{"docker", "nself"} {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(stub), 0755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("REVIEW_FIX_LOG", log)
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	got := FixItEngine(context.Background(), dir, []CheckResult{
		{Name: "unknown container", Status: "fail", FixCmd: "docker restart demo_totally_unowned"},
		{Name: "shell syntax", Status: "fail", FixCmd: "nself config set X Y && docker system prune -f"},
		{Name: "substitution", Status: "fail", FixCmd: "nself secrets set X=$(touch /tmp/never)"},
		{Name: "newline", Status: "fail", FixCmd: "nself config set X Y\ndocker system prune -f"},
	})
	body, _ := os.ReadFile(log)
	if len(body) != 0 {
		t.Errorf("disallowed suggestions executed: %q", body)
	}
	for _, r := range got {
		if !strings.Contains(r.Message, "manual step") {
			t.Errorf("not manual: %+v", r)
		}
	}
}

//go:build !windows

package report

// Tests for the doctor child (S4), the 0600 mode under umask 0 and the symlinked
// nginx root (S8). TestMain turns this test binary into the slow child.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const helperEnv = "NSELF_REPORT_TEST_CHILD"

func TestMain(m *testing.M) {
	switch os.Getenv(helperEnv) {
	case "slow": // prints, then outlives the timeout
		fmt.Println(os.Getenv(helperEnv+"_OUT") + " partial output")
		time.Sleep(3 * time.Second)
		os.Exit(0)
	case "slowgrand": // like slow, plus a grandchild that keeps the pipe open
		c := exec.Command("/bin/sleep", "3")
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		_ = c.Start()
		fmt.Println(os.Getenv(helperEnv+"_OUT") + " partial output")
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func selfExe() (string, error) { return os.Args[0], nil }

func TestDoctorReportTimeout(t *testing.T) {
	for _, mode := range []string{"slow", "slowgrand"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(helperEnv, mode)
			t.Setenv(helperEnv+"_OUT", "hello")
			start := time.Now()
			out, err := runDoctorChild(context.Background(), t.TempDir(), selfExe, 300*time.Millisecond, 300*time.Millisecond)
			var te *TimeoutError
			if !errors.As(err, &te) {
				t.Fatalf("err = %v, want *TimeoutError", err)
			}
			if el := time.Since(start); el > 2500*time.Millisecond {
				t.Errorf("child outlived its timeout: %s", el)
			}
			if !strings.Contains(out, "hello partial output") {
				t.Errorf("partial output lost: %q", out)
			}
		})
	}
}

func TestDoctorReportTimeoutKeepsPartialScrubbed(t *testing.T) {
	dir := t.TempDir()
	secret := "tmo" + rep("Zz5", 7)
	writeFile(t, filepath.Join(dir, ".env"), "ADMIN_SECRET="+secret+"\n")
	t.Setenv(helperEnv, "slow")
	t.Setenv(helperEnv+"_OUT", secret)
	src := Sources{
		Doctor: func(ctx context.Context) (string, error) {
			return runDoctorChild(ctx, dir, selfExe, 300*time.Millisecond, 300*time.Millisecond)
		},
		Now: func() time.Time { return fixedNow },
	}
	files := collectFiles(t, Options{ProjectDir: dir, Environ: noEnviron}, src)
	doc := files["doctor.txt"]
	if strings.Contains(doc, secret) || !strings.Contains(doc, "partial output") || !strings.Contains(doc, "timed out") {
		t.Errorf("doctor.txt = %q", doc)
	}
	if !strings.Contains(files["manifest.json"], "doctor timed out") {
		t.Errorf("manifest lacks the timeout note: %s", files["manifest.json"])
	}
}

func TestDoctorReportModeUnderUmaskZero(t *testing.T) {
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	dir := t.TempDir()
	b, err := Collect(context.Background(), Options{ProjectDir: dir, Environ: noEnviron}, Sources{Now: func() time.Time { return fixedNow }})
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "r.tar.gz")
	if _, err := WriteTarball(b, out, fixedNow); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(out); st.Mode().Perm() != 0o600 {
		t.Errorf("mode under umask 0 = %v, want 0600", st.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestDoctorReportNginxRootSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real-nginx")
	writeFile(t, filepath.Join(real, "sites", "a.conf"), "server { server_name a.example.org; }\n")
	link := filepath.Join(dir, "nginx")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	files := collectFiles(t, Options{ProjectDir: dir, NginxDir: link, Environ: noEnviron}, Sources{Now: func() time.Time { return fixedNow }})
	for n := range files {
		if strings.HasPrefix(n, "nginx/") {
			t.Errorf("followed a symlinked nginx root: %s", n)
		}
	}
}

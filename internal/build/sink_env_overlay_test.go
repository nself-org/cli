package build

// sink_env_overlay_test.go — the plan sink with a pending env change
// (P7-LIVE-05): reads, stats and writes see the overlay, a file the build
// planned wins over it, and write mode refuses an overlay.

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildRefusesEnvOverlayInWriteMode(t *testing.T) {
	_, err := Build(t.TempDir(), BuildOptions{EnvOverlay: map[string][]byte{".env": []byte("A=1\n")}})
	if !errors.Is(err, errEnvOverlayWrite) {
		t.Fatalf("err = %v, want the write-mode refusal", err)
	}
}

func TestEnvOverlaySink(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("A=disk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "other"), []byte("o"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newEnvOverlaySink(newMemSink(dir, ""), map[string][]byte{".env": []byte("A=pending\n"), ".env.secrets": []byte("S=1\n")})
	if planSink(s) == nil || planSink(newDiskSink(dir)) != nil {
		t.Fatal("planSink must unwrap the overlay sink and reject the disk sink")
	}
	for name, want := range map[string]string{".env": "A=pending\n", ".env.secrets": "S=1\n", "other": "o"} {
		got, err := s.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("ReadFile %s = %q %v, want %q", name, got, err, want)
		}
		if b, err := envLayerBytes(s, filepath.Join(dir, name)); err != nil || string(b) != want {
			t.Fatalf("envLayerBytes %s = %q %v", name, b, err)
		}
	}
	info, err := s.Stat(filepath.Join(dir, ".env.secrets"))
	if err != nil || info.Mode().Perm() != envOverlayPerm || info.Size() != 4 {
		t.Fatalf("Stat of a pending file: %v %v", info, err)
	}
	if info, err := s.Stat(filepath.Join(dir, "other")); err != nil || info.Size() != 1 {
		t.Fatalf("Stat falls through to disk: %v %v", info, err)
	}
	// An append to the pending file keeps 0600 and from then on the planned
	// bytes win over the overlay.
	if err := s.WriteFile(filepath.Join(dir, ".env"), []byte("A=pending\nB=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if f := s.files[".env"]; string(f.Data) != "A=pending\nB=2\n" || f.Perm != envOverlayPerm {
		t.Fatalf("planned .env = %q %v", f.Data, f.Perm)
	}
	if got, _ := s.ReadFile(filepath.Join(dir, ".env")); string(got) != "A=pending\nB=2\n" {
		t.Fatalf("planned bytes must win: %q", got)
	}
	if err := s.WriteFile(filepath.Join(dir, "other"), []byte("p"), 0o600); err != nil {
		t.Fatal(err)
	}
	if f := s.files["other"]; runtime.GOOS != "windows" && f.Perm != 0o644 {
		t.Fatalf("a non-overlaid file keeps its disk mode: %v", f.Perm)
	}
	if b, err := envLayerBytes(newDiskSink(dir), filepath.Join(dir, ".env")); err != nil || string(b) != "A=disk\n" {
		t.Fatalf("write mode reads the disk: %q %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".env.secrets")); !os.IsNotExist(err) {
		t.Fatal("the overlay sink wrote to disk")
	}
}

// TestEnvOverlayPlanMatchesPlainPlan: on every plan fixture (including the
// fronted layout, whose files get "@fronting/" keys), an overlay that holds
// the .env bytes already on disk plans exactly what a plain plan does.
func TestEnvOverlayPlanMatchesPlainPlan(t *testing.T) {
	for _, name := range []string{"dev-minimal", "fronted", "prod-plugins"} {
		t.Run(name, func(t *testing.T) {
			f := newPlanFixture(t, name)
			dotenv, err := os.ReadFile(filepath.Join(f.workdir, ".env"))
			if err != nil {
				t.Fatal(err)
			}
			plan := func(overlay map[string][]byte) *PlannedBuild {
				res, err := Build(f.workdir, BuildOptions{Mode: ModePlan, Rand: strings.NewReader(strings.Repeat("s", 4096)), EnvOverlay: overlay})
				if err != nil {
					t.Fatal(err)
				}
				return res.Planned
			}
			plain, over := plan(nil), plan(map[string][]byte{".env": dotenv})
			if len(plain.Files) != len(over.Files) {
				t.Fatalf("planned %d files with the overlay, %d without", len(over.Files), len(plain.Files))
			}
			for k, pf := range plain.Files {
				if of, ok := over.Files[k]; !ok || string(of.Data) != string(pf.Data) || of.Perm != pf.Perm {
					t.Fatalf("planned %s differs with the overlay (present=%v)", k, ok)
				}
			}
		})
	}
}

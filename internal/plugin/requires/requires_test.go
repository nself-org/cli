package requires

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

// fakeProbe is a scripted Probe that counts calls.
type fakeProbe struct {
	running           bool
	runErr, availErr  error
	have              map[string]bool
	runCalls, avCalls int
}

func (f *fakeProbe) Running(context.Context) (bool, error) { f.runCalls++; return f.running, f.runErr }
func (f *fakeProbe) Available(context.Context) (map[string]bool, error) {
	f.avCalls++
	return f.have, f.availErr
}

func cfgWith(image, version string) *config.Config {
	return &config.Config{ProjectName: "t", Postgres: config.PostgresConfig{Image: image, Version: version}}
}

func code(t *testing.T, err error) *errs.CLIError {
	t.Helper()
	var ce *errs.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("want *errs.CLIError, got %T %v", err, err)
	}
	return ce
}

func TestCheckRunningCluster(t *testing.T) {
	p := &fakeProbe{running: true, have: map[string]bool{"plpgsql": true}}
	ce := code(t, Check(context.Background(), cfgWith("", "16-alpine"), []string{"vector"}, p))
	if ce.Code != "E507" || !strings.Contains(ce.What, "vector") || !strings.Contains(ce.Fix, "nself db image switch --to pgvector") {
		t.Errorf("E507 content wrong: %v", ce)
	}
	p = &fakeProbe{running: true, have: map[string]bool{"vector": true}}
	if err := Check(context.Background(), cfgWith("", "16-alpine"), []string{"vector"}, p); err != nil {
		t.Errorf("with vector: %v", err)
	}
}

func TestCheckNotRunningUsesImage(t *testing.T) {
	cases := []struct {
		name, image, version string
		wantCode             string
		wantWarn             bool
	}{
		{"alpine refuses", "", "16-alpine", "E507", false},
		{"explicit postgres image refuses", "postgres:16-alpine", "", "E507", false},
		{"pgvector image passes", "pgvector/pgvector:pg16", "", "", false},
		{"custom image warns", "ghcr.io/x/pg:1", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var warn bytes.Buffer
			old := Stderr
			Stderr = &warn
			defer func() { Stderr = old }()
			p := &fakeProbe{}
			err := Check(context.Background(), cfgWith(c.image, c.version), []string{"vector"}, p)
			if c.wantCode == "" && err != nil {
				t.Fatalf("unexpected error %v", err)
			}
			if c.wantCode != "" {
				ce := code(t, err)
				if ce.Code != c.wantCode || !strings.Contains(ce.What, "not running") {
					t.Errorf("got %v", ce)
				}
			}
			if got := strings.Contains(warn.String(), "E508"); got != c.wantWarn {
				t.Errorf("E508 warning = %v, want %v (%q)", got, c.wantWarn, warn.String())
			}
			if p.avCalls != 0 {
				t.Error("Available must not be called for a stopped database")
			}
		})
	}
}

func TestCheckEmptyRequiresNoProbe(t *testing.T) {
	p := &fakeProbe{}
	if err := Check(context.Background(), cfgWith("", "16"), nil, p); err != nil {
		t.Fatal(err)
	}
	if p.runCalls+p.avCalls != 0 {
		t.Error("an empty requires list must not probe")
	}
}

func TestCheckProbeFailuresAreNotMissing(t *testing.T) {
	for _, p := range []*fakeProbe{
		{runErr: errors.New("cannot connect to the Docker daemon")},
		{running: true, availErr: errors.New("psql: connection refused")},
	} {
		err := Check(context.Background(), cfgWith("", "16-alpine"), []string{"vector"}, p)
		if err == nil {
			t.Fatal("want an error")
		}
		var ce *errs.CLIError
		if errors.As(err, &ce) {
			t.Errorf("a probe failure must not be an E507: %v", err)
		}
	}
}

func TestCheckRejectsBadNamesAndDedupes(t *testing.T) {
	for _, bad := range []string{"pgvector;drop", "Vector", "", "a b", "1x"} {
		if err := Check(context.Background(), cfgWith("", "16"), []string{bad}, &fakeProbe{running: true}); err == nil ||
			!strings.Contains(err.Error(), "invalid postgres extension name") {
			t.Errorf("%q: %v", bad, err)
		}
	}
	p := &fakeProbe{running: true, have: map[string]bool{}}
	ce := code(t, Check(context.Background(), cfgWith("", "16"), []string{"vector", "vector", "postgis"}, p))
	if strings.Count(ce.What, "vector") != 1 || !strings.Contains(ce.What, "postgis") {
		t.Errorf("What = %q", ce.What)
	}
	if !strings.Contains(ce.Fix, "db image switch") {
		t.Errorf("vector among the missing must name the switch command: %q", ce.Fix)
	}
}

func TestCheckFixForNonVector(t *testing.T) {
	ce := code(t, Check(context.Background(), cfgWith("", "16"), []string{"postgis"}, &fakeProbe{running: true, have: map[string]bool{}}))
	if strings.Contains(ce.Fix, "db image switch") || !strings.Contains(ce.Fix, "postgis") {
		t.Errorf("fix = %q", ce.Fix)
	}
}

func TestImageProvides(t *testing.T) {
	for img, want := range map[string]struct{ vec, known bool }{
		"pgvector/pgvector:pg16":                    {true, true},
		"docker.io/pgvector/pgvector:pg17@sha256:a": {true, true},
		"postgres:16-alpine":                        {false, true},
		"postgres":                                  {false, true},
		"library/postgres:15":                       {false, true},
		"ghcr.io/x/pg:1":                            {false, false},
		"localhost:5000/postgres:16":                {false, false},
	} {
		set, known := imageProvides(img)
		if known != want.known || set["vector"] != want.vec {
			t.Errorf("%s: known=%v vector=%v", img, known, set["vector"])
		}
		if known && !set["pgcrypto"] {
			t.Errorf("%s: contrib missing", img)
		}
	}
}

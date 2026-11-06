package restoreconsent

// Purpose: pin the restore consent rule (C2-01, C2-03) and the remote restore
// command line deploy and promote rollback run.

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/config"
)

func TestConfirm(t *testing.T) {
	for _, tc := range []struct {
		name, env, project, input, proof string
		yes, wantOK                      bool
	}{
		{"local yes", "dev", "p", "", "", true, true},
		{"local typed yes", "dev", "p", "yes\n", "", false, true},
		{"local EOF", "dev", "p", "", "", false, false},
		{"prod bare yes refused", "prod", "p", "", "", true, false},
		{"prod typed name", "prod", "p", "p\n", "", true, true},
		{"prod proof, no stdin", "prod", "p", "", "p", true, true},
		{"production proof", "production", "p", "", "p", false, true},
		{"prod wrong proof", "prod", "p", "p\n", "q", true, false},
		{"local wrong proof", "dev", "p", "", "q", true, false},
		{"empty project proof", "prod", "", "", "x", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cfg := &config.Config{Env: tc.env, ProjectName: tc.project}
			err := Confirm(strings.NewReader(tc.input), &out, cfg.ProjectName, cfg.IsProduction(), tc.yes, tc.proof)
			if (err == nil) != tc.wantOK {
				t.Fatalf("error=%v, want success=%v", err, tc.wantOK)
			}
			if tc.proof != "" && out.Len() != 0 {
				t.Fatalf("--confirm-project must not prompt, got %q", out.String())
			}
		})
	}
}

func TestCommandCarriesProof(t *testing.T) {
	got, err := Command("demo_full_1", "demo")
	if want := "nself backup restore 'demo_full_1' --yes --confirm-project 'demo'"; err != nil || got != want {
		t.Fatalf("command = %q %v, want %q", got, err, want)
	}
	if _, err := Command("demo_full_1", " "); err == nil {
		t.Fatal("an empty project name was accepted")
	}
	if err := Precheck("demo_full_1", "", true); err != nil {
		t.Fatalf("an already restored backup needs no project: %v", err)
	}
	if err := Precheck("demo_full_1", "", false); err == nil {
		t.Fatal("a pending restore without a project passed")
	}
	if h := Hint("demo_full_1", ""); h != "nself backup restore 'demo_full_1' --confirm-project <project>" {
		t.Fatalf("hint %q", h)
	}
}

type fakeRunner struct{ cmds []string }

func (f *fakeRunner) Run(_ context.Context, cmd string) ([]byte, error) {
	f.cmds = append(f.cmds, cmd)
	return nil, nil
}
func (*fakeRunner) Label() string { return "h" }

func TestRunRestore(t *testing.T) {
	f, done := &fakeRunner{}, 0
	if err := RunRestore(context.Background(), f, "/srv/app", "b1", "demo", &bytes.Buffer{}, func() error { done++; return nil }); err != nil {
		t.Fatal(err)
	}
	if len(f.cmds) != 1 || !strings.HasPrefix(f.cmds[0], "sh -c ") || !strings.Contains(f.cmds[0], "--confirm-project") || done != 1 {
		t.Fatalf("cmds=%v done=%d", f.cmds, done)
	}
	if err := RunRestore(context.Background(), &fakeRunner{}, "/srv/app", "b1", "", &bytes.Buffer{}, nil); err == nil {
		t.Fatal("restore without a project ran")
	}
}

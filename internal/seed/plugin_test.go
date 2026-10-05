package seed

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nself-org/cli/internal/errs"
)

type stubRT struct {
	findErr, execErr error
	gotService       string
	gotArgv          []string
	stderr           string
}

func (s *stubRT) FindContainer(_ context.Context, svc string) (string, error) {
	s.gotService = svc
	return "c1", s.findErr
}

func (s *stubRT) Exec(_ context.Context, _ string, argv []string) (string, string, error) {
	s.gotArgv = argv
	return "ok", s.stderr, s.execErr
}

func codeOf(err error) string {
	var ce *errs.CLIError
	if errors.As(err, &ce) {
		return ce.Code
	}
	return ""
}

func TestPluginSeedArgvVerbatim(t *testing.T) {
	rt := &stubRT{}
	argv := []string{"psql", "-c", "select 1; select 2"}
	out, err := RunPluginSeed(context.Background(), rt, PluginTarget{Name: "p", Service: "svc", Argv: argv})
	if err != nil || out != "ok" {
		t.Fatalf("out=%q err=%v", out, err)
	}
	if !reflect.DeepEqual(rt.gotArgv, argv) || rt.gotService != "svc" {
		t.Fatalf("argv=%v service=%s", rt.gotArgv, rt.gotService)
	}
}

func TestPluginSeedErrorsAreCoded(t *testing.T) {
	cases := []struct {
		name string
		rt   *stubRT
		tgt  PluginTarget
		code string
	}{
		{"no argv", &stubRT{}, PluginTarget{Name: "p", Service: "s"}, "E127"},
		{"empty element", &stubRT{}, PluginTarget{Name: "p", Service: "s", Argv: []string{"a", " "}}, "E127"},
		{"no service", &stubRT{}, PluginTarget{Name: "p", Argv: []string{"a"}}, "E252"},
		{"not running", &stubRT{findErr: errors.New("none")}, PluginTarget{Name: "p", Service: "s", Argv: []string{"a"}}, "E252"},
		{"exec fails", &stubRT{execErr: errors.New("exit 1"), stderr: "x\nbad row"}, PluginTarget{Name: "p", Service: "s", Argv: []string{"a"}}, "E250"},
	}
	for _, c := range cases {
		_, err := RunPluginSeed(context.Background(), c.rt, c.tgt)
		if codeOf(err) != c.code {
			t.Errorf("%s: code %q, err %v", c.name, codeOf(err), err)
		}
	}
}

func TestFirstComposeService(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.yml")
	if err := os.WriteFile(p, []byte("services:\n  zebra:\n    image: a\n  apple:\n    image: b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FirstComposeService(p)
	if err != nil || got != "zebra" {
		t.Fatalf("got %q err %v (document order, not sorted)", got, err)
	}
	if err := os.WriteFile(p, []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := FirstComposeService(p); codeOf(err) != "E252" {
		t.Fatalf("empty services: %v", err)
	}
}

package docker

// compose_test.go — the "--" that ends flag parsing before compose service
// names (D-0314): a service named "-x" must reach compose as a name, not a flag.

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// fakeDockerArgs writes a docker stub that records each argument on its own
// line and returns the stub path and a reader of the recorded argv.
func fakeDockerArgs(t *testing.T) (string, func() []string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	bin := filepath.Join(dir, "docker")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > '"+log+"'\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
		t.Fatal(err)
	}
	return bin, func() []string {
		b, err := os.ReadFile(log)
		if err != nil {
			t.Fatal(err)
		}
		return strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	}
}

func TestComposeServicesAfterDoubleDash(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		run  func(c *Compose, svc string) error
		tail func(svc string) []string // argv after "compose -f /m/base.yml"
	}{
		{"up", func(c *Compose, s string) error { return c.ComposeUp(ctx, ".", s) },
			func(s string) []string { return []string{"up", "-d", "--build", "--", s} }},
		{"up --no-deps", func(c *Compose, s string) error { return c.ComposeUpNoDeps(ctx, ".", s) },
			func(s string) []string { return []string{"up", "-d", "--no-deps", "--", s} }},
		{"restart", func(c *Compose, s string) error { return c.ComposeRestart(ctx, ".", s) },
			func(s string) []string { return []string{"restart", "--", s} }},
		{"stop", func(c *Compose, s string) error { return c.ComposeStop(ctx, ".", s) },
			func(s string) []string { return []string{"stop", "--", s} }},
		{"pull service", func(c *Compose, s string) error { return c.ComposePullService(ctx, ".", s) },
			func(s string) []string { return []string{"pull", "--", s} }},
		{"scale", func(c *Compose, s string) error { return c.ComposeScale(ctx, ".", s, 2) },
			func(s string) []string { return []string{"up", "-d", "--no-deps", "--scale=" + s + "=2", "--", s} }},
	}
	for _, tc := range cases {
		for _, svc := range []string{"-x", "web"} {
			t.Run(tc.name+" "+svc, func(t *testing.T) {
				bin, argv := fakeDockerArgs(t)
				c := NewCompose("/m/base.yml")
				c.DockerPath = bin
				if err := tc.run(c, svc); err != nil {
					t.Fatal(err)
				}
				want := append([]string{"compose", "-f", "/m/base.yml"}, tc.tail(svc)...)
				if got := argv(); !reflect.DeepEqual(got, want) {
					t.Errorf("argv = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestComposeNoServicesNoDoubleDash(t *testing.T) {
	ctx := context.Background()
	for name, run := range map[string]func(c *Compose) error{
		"up":      func(c *Compose) error { return c.ComposeUp(ctx, ".") },
		"restart": func(c *Compose) error { return c.ComposeRestart(ctx, ".") },
		"stop":    func(c *Compose) error { return c.ComposeStop(ctx, ".") },
	} {
		t.Run(name, func(t *testing.T) {
			bin, argv := fakeDockerArgs(t)
			c := NewCompose("/m/base.yml")
			c.DockerPath = bin
			if err := run(c); err != nil {
				t.Fatal(err)
			}
			for _, a := range argv() {
				if a == "--" {
					t.Errorf("argv %q carries a trailing \"--\" with no services", argv())
				}
			}
		})
	}
}

func TestStartedAtArgv(t *testing.T) {
	bin, argv := fakeDockerArgs(t)
	c := NewCompose("/m/base.yml")
	c.DockerPath = bin
	if _, err := c.startedAt(context.Background(), "-abc"); err != nil {
		t.Fatal(err)
	}
	want := []string{"inspect", "-f", "{{.State.StartedAt}}", "--", "-abc"}
	if got := argv(); !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
}

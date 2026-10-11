package docker

// compose_states_test.go — the container state read: one-off `compose run`
// containers are dropped, both ps JSON shapes parse, and FreshStates carries
// the daemon's StartedAt for the running gate (round 2 of P7-DEPL-16).

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// psWithOneOff is `compose ps -a --format json` for a service with one replica
// and one leftover `compose run` container, Labels as a map (Compose 2.21+).
const psWithOneOff = `[
 {"ID":"c1","Name":"p-bare-1","State":"running","Labels":{"com.docker.compose.oneoff":"False","com.docker.compose.service":"bare"}},
 {"ID":"c2","Name":"p-bare-run-723a92ad7995","State":"exited","ExitCode":0,"Labels":{"com.docker.compose.oneoff":"True","com.docker.compose.service":"bare"}}]`

func TestComposeServiceStatesDropsOneOff(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake docker is a shell script")
	}
	cases := map[string]string{
		"map labels": psWithOneOff,
		"csv labels (older compose)": `{"ID":"c1","Name":"p-bare-1","State":"running","Labels":"com.docker.compose.oneoff=False,com.docker.compose.service=bare"}` + "\r\n\r\n" +
			`{"ID":"c2","Name":"p-bare-run-1","State":"exited","Labels":"a=b,com.docker.compose.oneoff=True"}` + "\r\n",
	}
	for name, ps := range cases {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "ps.json"), []byte(ps), 0o600); err != nil {
			t.Fatal(err)
		}
		bin := filepath.Join(dir, "docker")
		if err := os.WriteFile(bin, []byte("#!/bin/sh\ncat '"+filepath.Join(dir, "ps.json")+"'\n"), 0o755); err != nil { //nolint:gosec // test stub must be executable
			t.Fatal(err)
		}
		c := NewCompose("/m/base.yml")
		c.DockerPath = bin
		cs, err := c.ComposeServiceStates(context.Background(), dir, "bare")
		if err != nil || len(cs) != 1 || cs[0].Name != "p-bare-1" {
			t.Errorf("%s: states = %+v, err = %v; want only the replica p-bare-1", name, cs, err)
		}
	}
}

func TestContainerStateOneOff(t *testing.T) {
	for raw, want := range map[string]bool{
		`{"Labels":{"com.docker.compose.oneoff":"True"}}`:  true,
		`{"Labels":{"com.docker.compose.oneoff":"true"}}`:  true,
		`{"Labels":{"com.docker.compose.oneoff":"False"}}`: false,
		`{"Labels":{"com.docker.compose.service":"x"}}`:    false,
		`{"Labels":"x=y,com.docker.compose.oneoff=True"}`:  true,
		`{"Labels":"com.docker.compose.oneoff=False"}`:     false,
		`{"Labels":""}`:   false,
		`{"Labels":null}`: false,
		`{"Name":"no labels at all (old compose): a replica, never dropped"}`: false,
	} {
		got, err := parseContainerStates([]byte("[" + raw + "]"))
		if err != nil || len(got) != 1 || got[0].OneOff() != want {
			t.Errorf("%s: OneOff = %v (err %v), want %v", raw, len(got) == 1 && got[0].OneOff(), err, want)
		}
	}
}

func TestParseContainerStatesLines(t *testing.T) {
	raw := "{\"Name\":\"a\",\"State\":\"running\"}\r\n\r\n{\"Name\":\"b\",\"State\":\"exited\",\"ExitCode\":2}\n\n"
	got, err := parseContainerStates([]byte(raw))
	if err != nil || len(got) != 2 || got[1].Name != "b" || got[1].ExitCode != 2 {
		t.Fatalf("CRLF and blank lines: got %+v, err %v", got, err)
	}
	if _, err := parseContainerStates([]byte("{\"Name\":\"a\"}\nnot json\n")); err == nil {
		t.Error("a bad line must error, not be skipped")
	}
}

func TestRegressed(t *testing.T) {
	run := func(id, at string) ContainerState {
		return ContainerState{ID: id, Name: "p-" + id, State: "running", StartedAt: at}
	}
	hc := func(id, at, health string) ContainerState {
		c := run(id, at)
		c.Health = health
		return c
	}
	exited := func(id string, code int) ContainerState {
		return ContainerState{ID: id, Name: "p-" + id, State: "exited", ExitCode: code}
	}
	plain := []ContainerState{run("a", "t1"), run("b", "t1")}
	healthy := []ContainerState{hc("a", "t1", "healthy"), hc("b", "t1", "healthy")}
	oneShot := []ContainerState{exited("a", 0)}
	cases := []struct {
		name          string
		before, after []ContainerState
		want          string // substring of the error, "" for none
	}{
		{"same run", plain, []ContainerState{run("a", "t1"), run("b", "t1")}, ""},
		{"new StartedAt", plain, []ContainerState{run("a", "t1"), run("b", "t2")}, "p-b restarted"},
		{"restarting", plain, []ContainerState{run("a", "t1"), {ID: "b", Name: "p-b", State: "restarting"}}, "p-b is restarting"},
		{"exited non-zero", plain, []ContainerState{run("a", "t1"), exited("b", 3)}, "p-b exited 3"},
		{"exited zero, no healthcheck, is not a crash", plain, []ContainerState{run("a", "t1"), exited("b", 0)}, ""},
		{"no StartedAt cannot be compared", plain, []ContainerState{{ID: "a", Name: "p-a", State: "running"}, run("b", "t1")}, ""},
		{"unknown container was not seen by its gate", plain, []ContainerState{run("a", "t1"), run("b", "t1"), run("c", "t9")}, "p-c was not seen by its gate"},
		{"new ID replaces the old one", plain, []ContainerState{run("a", "t1"), run("z", "t5")}, "p-z was not seen by its gate"},
		{"vanished one of two", plain, []ContainerState{run("a", "t1")}, "p-b is gone"},
		{"empty after, running before", plain, nil, "is gone"},
		{"healthy stays healthy", healthy, []ContainerState{hc("a", "t1", "healthy"), hc("b", "t1", "healthy")}, ""},
		{"healthy to unhealthy", healthy, []ContainerState{hc("a", "t1", "healthy"), hc("b", "t1", "unhealthy")}, "p-b is running/unhealthy"},
		{"healthy to starting", healthy, []ContainerState{hc("a", "t1", "starting"), hc("b", "t1", "healthy")}, "p-a is running/starting"},
		{"healthy to running with empty health", healthy, []ContainerState{hc("a", "t1", ""), hc("b", "t1", "healthy")}, "p-a is running/"},
		{"healthcheck service exited 0", healthy, []ContainerState{hc("a", "t1", "healthy"), exited("b", 0)}, "p-b is exited/"},
		{"one-shot exited 0 and identical", oneShot, []ContainerState{exited("a", 0)}, ""},
		{"one-shot exited 0 present, none running before", oneShot, []ContainerState{exited("a", 0)}, ""},
		{"rows without ID match by Name", []ContainerState{{Name: "p-a", State: "running"}}, []ContainerState{{Name: "p-a", State: "running"}}, ""},
		{"rows without ID: other name is new", []ContainerState{{Name: "p-a", State: "running"}}, []ContainerState{{Name: "p-b", State: "running"}}, "p-b was not seen"},
		{"rows without ID: vanished", []ContainerState{{Name: "p-a", State: "running"}}, nil, "p-a is gone"},
		{"remote rows, ID but no StartedAt, same", []ContainerState{{ID: "a", Name: "p-a", State: "running", Health: "healthy"}}, []ContainerState{{ID: "a", Name: "p-a", State: "running", Health: "healthy"}}, ""},
		{"remote rows, ID but no StartedAt, replaced", []ContainerState{{ID: "a", Name: "p-a", State: "running"}}, []ContainerState{{ID: "b", Name: "p-b", State: "running"}}, "was not seen"},
		{"remote rows, ID but no StartedAt, unhealthy", []ContainerState{{ID: "a", Name: "p-a", State: "running", Health: "healthy"}}, []ContainerState{{ID: "a", Name: "p-a", State: "running", Health: "unhealthy"}}, "unhealthy"},
		{"image HEALTHCHECK, gated on running: starting stays starting", []ContainerState{hc("a", "t1", "starting")}, []ContainerState{hc("a", "t1", "starting")}, ""},
		{"docker reports unhealthy after the gate, whatever it waited for", []ContainerState{hc("a", "t1", "starting")}, []ContainerState{hc("a", "t1", "unhealthy")}, "p-a is running/unhealthy"},
		{"one-shot exited 0 before, after empty", oneShot, nil, "p-a is gone"},
		{"running before, now paused", plain[:1], []ContainerState{{ID: "a", Name: "p-a", State: "paused"}}, "p-a is paused"},
		{"running before, now dead", plain[:1], []ContainerState{{ID: "a", Name: "p-a", State: "dead"}}, "p-a is dead"},
		{"healthy to unhealthy", healthy[:1], []ContainerState{hc("a", "t1", "unhealthy")}, "p-a is running/unhealthy"},
		{"healthy to exited 0", healthy[:1], []ContainerState{exited("a", 0)}, "p-a is exited/"},
		{"nothing before, anything after", nil, []ContainerState{run("a", "t1")}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Regressed(tc.before, tc.after)
			if tc.want == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if !errors.Is(err, ErrServiceCrashed) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want ErrServiceCrashed containing %q, got %v", tc.want, err)
			}
		})
	}
}

package docker

// compose_states_test.go — the container state read: one-off `compose run`
// containers are dropped, both ps JSON shapes parse, and FreshStates carries
// the daemon's StartedAt for the running gate (round 2 of P7-DEPL-16).

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
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

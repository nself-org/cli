package deploy

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareHostEnvNamesOnly(t *testing.T) {
	r, local := newFakeRemote(t), t.TempDir()
	for dir, body := range map[string]string{
		local:  "export SHARED='same'\nCHANGED=local-secret\nLOCAL_ONLY=private\nEMPTY=\nMULTI=\"line1\nline2\"\n",
		r.home: "SHARED=same\nCHANGED=host-secret\nHOST_ONLY=private-host\nEMPTY=\nMULTI=\"line1\nline2\"\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	drift, err := CompareHostEnv(context.Background(), r, local, r.home)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(drift)
	want := `{"local_only":["LOCAL_ONLY"],"host_only":["HOST_ONLY"],"changed":["CHANGED"]}`
	if string(data) != want || !drift.Different() {
		t.Fatalf("got %s; want %s", data, want)
	}
	for _, dir := range []string{local, r.home} {
		body, _ := os.ReadFile(filepath.Join(dir, ".env"))
		if !strings.Contains(string(body), "secret") {
			t.Fatal("comparison changed .env")
		}
	}
	if len(r.log) != 1 || len(r.flags) != 0 {
		t.Fatalf("expected one read only: %v", r.log)
	}
}

func TestCompareHostEnvFailuresNeverLeak(t *testing.T) {
	for _, tc := range []struct {
		name, local, host string
		transport         bool
	}{
		{"missing-local", "", "A=host-secret", false},
		{"missing-host", "A=local-secret", "", false},
		{"bad-local", "A=\"local-secret", "A=host-secret", false},
		{"bad-host", "A=local-secret", "A=\"host-secret", false},
		{"hostile-key", "A=local-secret", "KEY.DOT=host-secret", false},
		{"transport", "A=local-secret", "A=host-secret", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, local := newFakeRemote(t), t.TempDir()
			for dir, body := range map[string]string{local: tc.local, r.home: tc.host} {
				if body != "" {
					if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if tc.transport {
				r.fail["cat .env"] = errors.New("host-secret")
			}
			d, err := CompareHostEnv(context.Background(), r, local, r.home)
			if err == nil || strings.Contains(err.Error(), "secret") || d.Different() {
				t.Fatalf("unsafe or missing error: %+v %v", d, err)
			}
		})
	}
}

func TestCompareHostEnvIdenticalAndEmpty(t *testing.T) {
	for _, body := range []string{"", "A=same\nEMPTY=\n"} {
		r, local := newFakeRemote(t), t.TempDir()
		for _, dir := range []string{local, r.home} {
			if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
		}
		d, err := CompareHostEnv(context.Background(), r, local, r.home)
		if err != nil || d.Different() {
			t.Fatalf("identical files: %+v %v", d, err)
		}
	}
}

func TestCompareHostEnvRefusesUnsafePath(t *testing.T) {
	r := newFakeRemote(t)
	_, err := CompareHostEnv(context.Background(), r, t.TempDir(), "/opt/nself; touch INJECTED")
	if err == nil || len(r.log) != 0 {
		t.Fatalf("unsafe path reached SSH: %v %v", err, r.log)
	}
}

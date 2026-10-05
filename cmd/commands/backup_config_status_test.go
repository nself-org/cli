package commands

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWithDestinationKindsText(t *testing.T) {
	out, err := withDestinationKinds("Backup Configuration:\n", "table", "path:///mnt/b")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"configured: path", "* path", "  rclone", "  host"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	out, _ = withDestinationKinds("x\n", "table", "")
	if !strings.Contains(out, "configured: none") {
		t.Errorf("unset remote should read none:\n%s", out)
	}
}

func TestWithDestinationKindsJSON(t *testing.T) {
	out, err := withDestinationKinds(`{"remote": "s3://b/p"}`, "json", "s3://b/p")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		Remote      string `json:"remote"`
		Destination string `json:"destination"`
		Kinds       []struct {
			Kind string `json:"kind"`
		} `json:"destination_kinds"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatal(err)
	}
	if v.Remote != "s3://b/p" || v.Destination != "rclone" || len(v.Kinds) != 3 {
		t.Fatalf("%+v", v)
	}
	if _, err := withDestinationKinds("not json", "json", ""); err == nil {
		t.Fatal("bad json accepted")
	}
}

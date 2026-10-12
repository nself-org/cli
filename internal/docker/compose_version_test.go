package docker

// Purpose: table tests for the compose version floor. No subprocess runs.

import (
	"strings"
	"testing"
)

func TestCheckComposeVersion(t *testing.T) {
	cases := []struct {
		in   string
		want ComposeVerdict
	}{
		{"1.29.2", ComposeTooOld},
		{"2.17.3", ComposeTooOld},
		{"2.19.1", ComposeTooOld},
		{"2.20.0", ComposeOK},
		{"2.20.0-rc.1", ComposeOK},
		{"v2.29.7", ComposeOK},
		{"2.29.7-desktop.1", ComposeOK},
		{"2.21.0+azure-1", ComposeOK},
		{"3.0.0", ComposeOK},
		{"2.100.0", ComposeOK},
		{"2.20", ComposeOK},
		{"dev", ComposeUnknown},
		{"", ComposeUnknown},
		{"garbage", ComposeUnknown},
	}
	for _, c := range cases {
		if got := CheckComposeVersion(c.in); got != c.want {
			t.Errorf("CheckComposeVersion(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseComposeVersion(t *testing.T) {
	good := []struct {
		in         string
		ma, mi, pa int
	}{
		{"2.29.7", 2, 29, 7},
		{"v2.29.7", 2, 29, 7},
		{"2.29.7-desktop.1", 2, 29, 7},
		{"2.21.0+azure-1", 2, 21, 0},
		{"2.20", 2, 20, 0},
		{" 2.17.3\n", 2, 17, 3},
	}
	for _, c := range good {
		ma, mi, pa, ok := ParseComposeVersion(c.in)
		if !ok || ma != c.ma || mi != c.mi || pa != c.pa {
			t.Errorf("ParseComposeVersion(%q) = %d.%d.%d ok=%v", c.in, ma, mi, pa, ok)
		}
	}
	for _, in := range []string{"", "dev", "garbage", "2", "v", "2.x.1", "1.2.3.4", "-1.2"} {
		if _, _, _, ok := ParseComposeVersion(in); ok {
			t.Errorf("ParseComposeVersion(%q) must be rejected", in)
		}
	}
}

func TestComposeTooOldMessage(t *testing.T) {
	m := ComposeTooOldMessage("2.17.3")
	for _, want := range []string{"2.17.3", "2.20", "backslash", "compose.env", "Docker Desktop", "docker-compose-plugin"} {
		if !strings.Contains(m, want) {
			t.Errorf("message %q lacks %q", m, want)
		}
	}
}

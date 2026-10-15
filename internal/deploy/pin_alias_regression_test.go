package deploy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestPinAliasEndpointIsolation(t *testing.T) {
	for _, pair := range [][2]string{
		{"[2001:db8::1]:2222", "2001-db8--1:2222"},
		{"a.test:2222", "nself-a.test-2222"},
	} {
		if remote.PinAlias(mustSpec(t, pair[0])) == remote.PinAlias(mustSpec(t, pair[1])) {
			t.Errorf("distinct endpoints %q share a pin alias", pair)
		}
	}
	if remote.PinAlias(mustSpec(t, "host.test")) != remote.PinAlias(mustSpec(t, "host.test:22")) {
		t.Error("omitted and explicit port 22 must share a pin")
	}
}

func TestOperatorTrustExcludesLegacyPins(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	home := pinHome(t)
	t.Setenv("HOME", home)
	installFakeKeyscan(t)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "known_hosts"), []byte("nself-a.test-2222 ssh-ed25519 AQID\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add("nself-a.test-2222", "ssh-ed25519 BAUG"); err != nil {
		t.Fatal(err)
	}
	opts, err := HostKeyOptions(context.Background(), "nself-a.test-2222:22", "live", "web", true)
	if err != nil || !strings.Contains(strings.Join(opts, " "), `GlobalKnownHostsFile="/dev/null"`) {
		t.Fatalf("operator trust exposes colliding legacy pins: %q, %v", opts, err)
	}
}

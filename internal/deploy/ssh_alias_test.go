package deploy

// Purpose: keep synthetic deploy pins separate from real hosts (D-0696).
// Inputs: fake key scans and isolated pin files, in both compatibility modes.
// Outputs: collision refusals and read-only legacy lookup assertions.
// Constraints: no network or operator pin-file changes.

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

func TestDeployPinAliasIsolation(t *testing.T) {
	requireKeygen(t)
	for _, mode := range []string{"0", "1"} {
		for _, pair := range [][2]string{
			{"203.0.113.7", "nself-203.0.113.7-22"},
			{"x:22", "nself-x-22"},
			{"[1::2]:22", "1--2:22"},
		} {
			t.Run(mode+"/"+pair[0], func(t *testing.T) {
				t.Setenv("NSELF_V15", mode)
				t.Setenv("HOME", pinHome(t))
				installFakeKeyscan(t)
				fp, err := remote.Fingerprint("h ssh-ed25519 AQID")
				if err != nil {
					t.Fatal(err)
				}
				if err := TrustHostKey(context.Background(), pair[0], fp); err != nil {
					t.Fatal(err)
				}
				if _, err := HostKeyPolicy(context.Background(), pair[0], "live", "web", true, true); err != nil {
					t.Fatalf("enrolled host refused: %v", err)
				}
				if _, err := HostKeyPolicy(context.Background(), pair[1], "live", "web", true, true); err == nil {
					t.Fatalf("pin for %s trusted for %s", pair[0], pair[1])
				}
			})
		}
	}
}

func TestDeployLegacyAliasReadOnly(t *testing.T) {
	requireKeygen(t)
	for _, host := range []string{"203.0.113.7", "x:22", "[1::2]:22", "nself-x-22"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			t.Setenv("HOME", pinHome(t))
			installFakeKeyscan(t)
			spec := mustSpec(t, host)
			alias := spec.Host
			if spec.Port != 0 || strings.Contains(host, "203.0.113.7") {
				alias = "nself-" + strings.ReplaceAll(spec.Host, ":", "-") + "-22"
			}
			pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
			if err := pin.Add(alias, "ssh-ed25519 AQID"); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(pin.Path)
			if err != nil {
				t.Fatal(err)
			}
			opts, err := HostKeyPolicy(context.Background(), host, "live", "web", true, true)
			if err != nil {
				t.Fatal(err)
			}
			if alias != spec.Host && !strings.Contains(strings.Join(opts, " "), "HostKeyAlias="+alias) {
				t.Fatalf("legacy alias missing: %v", opts)
			}
			after, err := os.ReadFile(pin.Path)
			if err != nil || string(before) != string(after) {
				t.Fatalf("compat lookup changed pin file: %v", err)
			}
		})
	}
}

func TestDeployNewAliasOverridesLegacy(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", pinHome(t))
	installFakeKeyscan(t)
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	for alias, key := range map[string]string{
		"nself-203.0.113.7-22": "ssh-ed25519 AQID",
		"nself+203.0.113.7+22": "ssh-ed25519 AQIE",
	} {
		if err := pin.Add(alias, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := HostKeyPolicy(context.Background(), "203.0.113.7", "live", "web", true, true); err == nil {
		t.Fatal("stale legacy key accepted despite a new pin")
	}
}

func TestPinAliasLegacyFallbackForOtherCallers(t *testing.T) {
	requireKeygen(t)
	t.Setenv("HOME", pinHome(t))
	spec := mustSpec(t, "u@example.test:2222")
	if got := PinAlias(context.Background(), spec); got != "nself+example.test+2222" {
		t.Fatalf("no pins: alias %q", got)
	}
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	if err := pin.Add("nself-example.test-2222", "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	if got := PinAlias(context.Background(), spec); got != "nself-example.test-2222" {
		t.Fatalf("legacy-only pin: alias %q", got)
	}
	if err := pin.Add("nself+example.test+2222", "ssh-ed25519 AQIE"); err != nil {
		t.Fatal(err)
	}
	if got := PinAlias(context.Background(), spec); got != "nself+example.test+2222" {
		t.Fatalf("new pin must win: alias %q", got)
	}
	if got := PinAlias(context.Background(), mustSpec(t, "example.test")); got != "example.test" {
		t.Fatalf("plain host alias %q", got)
	}
}

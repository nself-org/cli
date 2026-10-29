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

// TestDeployLegacyAliasNeverRewritten: a pin an older release wrote under its
// own alias keeps working, ssh checks the shared remote.PinAlias, and no line
// already in the pin file is rewritten or removed (D-0696, D-0463).
func TestDeployLegacyAliasNeverRewritten(t *testing.T) {
	requireKeygen(t)
	for _, host := range []string{"203.0.113.7", "x:22", "[1::2]:22"} {
		t.Run(host, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			t.Setenv("HOME", pinHome(t))
			installFakeKeyscan(t)
			spec := mustSpec(t, host)
			legacy := remote.LegacyPinAliases(spec)
			if len(legacy) != 1 {
				t.Fatalf("legacy aliases for %s: %v", host, legacy)
			}
			pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
			if err := pin.Add(legacy[0], "ssh-ed25519 AQID"); err != nil {
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
			alias := remote.PinAlias(spec)
			if alias != strings.ToLower(spec.Host) && !strings.Contains(strings.Join(opts, " "), "HostKeyAlias="+alias) {
				t.Fatalf("shared alias missing: %v", opts)
			}
			after, err := os.ReadFile(pin.Path)
			if err != nil || !strings.HasPrefix(string(after), string(before)) {
				t.Fatalf("legacy pin lines rewritten: %v\nbefore:\n%s\nafter:\n%s", err, before, after)
			}
		})
	}
	// Ordinary nself-* hostnames stay valid: a fresh enrolment pins and passes.
	t.Run("nself-x-22", func(t *testing.T) {
		t.Setenv("NSELF_V15", "1")
		t.Setenv("HOME", pinHome(t))
		installFakeKeyscan(t)
		fp, err := remote.Fingerprint("h ssh-ed25519 AQID")
		if err != nil {
			t.Fatal(err)
		}
		if err := TrustHostKey(context.Background(), "nself-x-22", fp); err != nil {
			t.Fatal(err)
		}
		opts, err := HostKeyPolicy(context.Background(), "nself-x-22", "live", "web", true, true)
		if err != nil {
			t.Fatalf("enrolled nself-* host refused: %v", err)
		}
		if alias := remote.PinAlias(mustSpec(t, "nself-x-22")); alias != "nself-x-22" && !strings.Contains(strings.Join(opts, " "), "HostKeyAlias="+alias) {
			t.Fatalf("nself-* host not checked under its pin alias %s: %v", alias, opts)
		}
	})
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

// TestPinAliasLegacyFallbackForOtherCallers: access reads the deploy pin file
// through EffectivePinAlias, which falls back to the legacy alias read-only.
func TestPinAliasLegacyFallbackForOtherCallers(t *testing.T) {
	requireKeygen(t)
	t.Setenv("HOME", pinHome(t))
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	effective := func(host string) string {
		t.Helper()
		got, err := pin.EffectivePinAlias(mustSpec(t, host))
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := effective("u@example.test:2222"); got != "nself+example.test+2222" {
		t.Fatalf("no pins: alias %q", got)
	}
	if err := pin.Add("nself-example.test-2222", "ssh-ed25519 AQID"); err != nil {
		t.Fatal(err)
	}
	if got := effective("u@example.test:2222"); got != "nself-example.test-2222" {
		t.Fatalf("legacy-only pin: alias %q", got)
	}
	if err := pin.Add("nself+example.test+2222", "ssh-ed25519 AQIE"); err != nil {
		t.Fatal(err)
	}
	if got := effective("u@example.test:2222"); got != "nself+example.test+2222" {
		t.Fatalf("new pin must win: alias %q", got)
	}
	if got := effective("example.test"); got != "example.test" {
		t.Fatalf("plain host alias %q", got)
	}
}

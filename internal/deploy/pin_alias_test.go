package deploy

// Purpose: D-0463. Deploy pins under remote.PinAlias, so a key trusted for
// host:22 is the key admin and access find for host, and a pin an older release
// wrote under "nself-<host>-<port>" moves to the shared alias once.

import (
	"context"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/remote"
)

const aqidKey = "ssh-ed25519 AQID"

func TestTrustHostKeyExplicitPort22IsTheBareHostPin(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", pinHome(t))
	installFakeKeyscan(t)
	fp, err := remote.Fingerprint("h " + aqidKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := TrustHostKey(ctx, "u@host.example:22", fp); err != nil {
		t.Fatal(err)
	}
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	if keys, err := pin.Lookup("host.example"); err != nil || len(keys) != 1 {
		t.Fatalf("host:22 pin under the bare host = %v, %v", keys, err)
	}
	if keys, _ := pin.Lookup("nself-host.example-22"); len(keys) != 0 {
		t.Fatalf("a legacy alias was written: %v", keys)
	}
	// The same endpoint spelled without the port is trusted by that pin.
	opts, err := HostKeyOptions(ctx, "u@host.example", "live", "web", true)
	if err != nil {
		t.Fatalf("host without port: %v", err)
	}
	if strings.Contains(strings.Join(opts, " "), "HostKeyAlias") {
		t.Errorf("a bare-host pin needs no HostKeyAlias: %q", opts)
	}
}

func TestLegacyDeployPinMigratesOnce(t *testing.T) {
	requireKeygen(t)
	for host, legacy := range map[string]string{
		"u@host.example:22":   "nself-host.example-22",
		"u@host.example:2222": "nself-host.example-2222",
		"u@203.0.113.7":       "nself-203.0.113.7-22",
	} {
		host, legacy := host, legacy
		t.Run(host, func(t *testing.T) {
			t.Setenv("NSELF_V15", "1")
			t.Setenv("HOME", pinHome(t))
			installFakeKeyscan(t)
			pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
			if err := pin.Add(legacy, aqidKey); err != nil {
				t.Fatal(err)
			}
			spec := mustSpec(t, host)
			if _, err := HostKeyOptions(context.Background(), host, "live", "web", true); err != nil {
				t.Fatalf("a legacy pin of the presented key was refused: %v", err)
			}
			if keys, err := pin.Lookup(remote.PinAlias(spec)); err != nil || len(keys) != 1 {
				t.Fatalf("the pin was not moved to %s: %v, %v", remote.PinAlias(spec), keys, err)
			}
			if keys, _ := pin.Lookup(legacy); len(keys) != 1 {
				t.Fatalf("the legacy line must stay: %v", keys)
			}
		})
	}
}

func TestLegacyDeployPinOfAnotherKeyStaysRefused(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", pinHome(t))
	installFakeKeyscan(t)
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	other := "ssh-ed25519 AQIDBA==" // not what the scan presents
	if err := pin.Add("nself-host.example-2222", other); err != nil {
		t.Fatal(err)
	}
	_, err := HostKeyOptions(context.Background(), "u@host.example:2222", "live", "web", true)
	if err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("a legacy pin of a different key was accepted: %v", err)
	}
	if keys, _ := pin.Lookup("nself+host.example+2222"); len(keys) != 0 {
		t.Fatalf("a non-matching legacy key was migrated: %v", keys)
	}
}

// TestCollidingEndpointsKeepSeparatePins is the collision from the debt row: an
// IPv6 literal on 2222 and a host named like its folded form used one pin.
func TestCollidingEndpointsKeepSeparatePins(t *testing.T) {
	requireKeygen(t)
	t.Setenv("NSELF_V15", "1")
	t.Setenv("HOME", pinHome(t))
	installFakeKeyscan(t)
	fp, err := remote.Fingerprint("h " + aqidKey)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := TrustHostKey(ctx, "u@[2001:db8::1]:2222", fp); err != nil {
		t.Fatal(err)
	}
	_, err = HostKeyOptions(ctx, "u@2001-db8--1:2222", "live", "web", true)
	if err == nil || !strings.Contains(err.Error(), "E487") {
		t.Fatalf("trusting the IPv6 endpoint trusted a different host: %v", err)
	}
}

package deploy

// Purpose: deploy host-key trust: the dedicated pin file, enrolment, and the
//          ssh host-key policy DeployViaSsh and PushSecrets pass to ssh.
// Inputs:  a host spec string, the inventory env and server names, and whether
//          the call is strict or ships secrets.
// Outputs: "-o ..." option tokens, or an E487 error before any copy starts.
// Constraints: nself's own code never writes ~/.ssh/known_hosts (ssh's
//              UpdateHostKeys may, on the operator-match path); the pin file is
//              read through remote.PinnedHostKeys, which refuses foreign lines
//              (fail closed).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/compat"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/hostpin"
	"github.com/nself-org/cli/sdk/go/v2/remote"
)

// DeployKnownHostsPath is the dedicated pin file. The operator's SSH file is read only.
func DeployKnownHostsPath() string {
	return hostpin.KnownHostsPath()
}

// knownHostsOption quotes the absolute pin path for OpenSSH.
func knownHostsOption() (string, error) {
	return remote.GlobalKnownHostsFileOption(DeployKnownHostsPath())
}

func deployHostPort(spec remote.HostSpec) int    { return hostpin.HostPort(spec.Port) }
func deployKeyAlias(spec remote.HostSpec) string { return hostpin.KeyAlias(spec.Host, spec.Port) }
func legacyKeyAlias(spec remote.HostSpec) string { return hostpin.LegacyKeyAlias(spec.Host, spec.Port) }

// TrustHostKey scans live keys and pins only the key with the offered fingerprint.
func TrustHostKey(ctx context.Context, host, offered string) error {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return errs.New("E484", err.Error())
	}
	if !strings.HasPrefix(offered, "SHA256:") {
		return errs.New("E487", "--trust-host-key needs a SHA256 fingerprint")
	}
	keys, err := ScanDeployKeys(ctx, spec)
	if err != nil {
		return errs.New("E487", fmt.Sprintf("host key scan for %s failed: %v", spec.Host, err))
	}
	for _, k := range keys {
		if k.Fingerprint == offered {
			return (remote.PinnedHostKeys{Path: DeployKnownHostsPath()}).Add(deployKeyAlias(spec), k.Line)
		}
	}
	return errs.New("E487", fmt.Sprintf("host %s presented no key matching %s", spec.Host, offered))
}

// HostKeyOptions returns accept-new or strict policy after validating the live key.
func HostKeyOptions(ctx context.Context, host, env, server string, strict bool) ([]string, error) {
	return HostKeyPolicy(ctx, host, env, server, strict, false)
}

// pinnedFingerprints returns the fingerprints pinned for alias in the deploy
// pin file. The sdk Lookup is only the guard: a pin file it refuses (foreign
// wildcard or CA line, bad mode) is an error, fail closed before any copy. The
// set itself comes from ssh-keygen -F, the matcher ssh uses on this file.
func pinnedFingerprints(ctx context.Context, alias string) (map[string]bool, error) {
	pin := remote.PinnedHostKeys{Path: DeployKnownHostsPath()}
	if _, err := pin.Lookup(alias); err != nil {
		return nil, errs.New("E487", fmt.Sprintf("deploy pin file %s refused: %v", DeployKnownHostsPath(), err))
	}
	return hostpin.TrustedFingerprints(ctx, alias, DeployKnownHostsPath(), remote.Fingerprint), nil
}

// deployPinnedFingerprints reads legacy IP/port pins only when no new pin
// exists. It returns the alias SSH must check and never rewrites the pin file.
func deployPinnedFingerprints(ctx context.Context, spec remote.HostSpec) (string, map[string]bool, error) {
	alias := deployKeyAlias(spec)
	pinned, err := pinnedFingerprints(ctx, alias)
	if err != nil || len(pinned) != 0 || alias == spec.Host {
		return alias, pinned, err
	}
	legacy := legacyKeyAlias(spec)
	pinned, err = pinnedFingerprints(ctx, legacy)
	return legacy, pinned, err
}

// PinAlias delegates to hostpin.PinAlias; kept here so existing deploy callers
// keep compiling without importing hostpin directly.
func PinAlias(ctx context.Context, spec remote.HostSpec) string {
	return hostpin.PinAlias(ctx, spec.Host, spec.Port, remote.Fingerprint)
}

// hostKeyPolicy is the unexported spelling of HostKeyPolicy, kept for the
// callers in this package that predate the export.
func hostKeyPolicy(ctx context.Context, host, env, server string, strict, shipsSecrets bool) ([]string, error) {
	return HostKeyPolicy(ctx, host, env, server, strict, shipsSecrets)
}

// HostKeyPolicy is the shared host-key policy. shipsSecrets forces strict
// checking in v1.4 too (EPIC D28, D-0248): secrets never go to an unverified
// key. Every caller that sends a secret off this machine passes shipsSecrets.
func HostKeyPolicy(ctx context.Context, host, env, server string, strict, shipsSecrets bool) ([]string, error) {
	spec, err := remote.ParseHostSpec(host)
	if err != nil {
		return nil, errs.New("E484", err.Error())
	}
	// compat.V15(P7-DEPL-13): accept-new -> strict for secret shipping and prod targets.
	if !shipsSecrets && (!compat.V15() || !strict) {
		return acceptNewPolicy(), nil
	}
	pinOption, err := knownHostsOption()
	if err != nil {
		return nil, err
	}
	keys, err := ScanDeployKeys(ctx, spec)
	if err != nil {
		return nil, errs.New("E487", fmt.Sprintf("host key scan for %s failed: %v", spec.Host, err))
	}
	home, _ := os.UserHomeDir()
	operatorAlias := spec.Host
	if spec.Port != 0 {
		operatorAlias = "[" + spec.Host + "]:" + strconv.Itoa(spec.Port)
	}
	operator := hostpin.TrustedFingerprints(ctx, operatorAlias, filepath.Join(home, ".ssh", "known_hosts"), remote.Fingerprint)
	alias, pinned, err := deployPinnedFingerprints(ctx, spec)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		if operator[k.Fingerprint] || pinned[k.Fingerprint] {
			// A client that joins an existing ControlMaster socket does no host-key
			// check, so a strict connection never shares one (an accept-new probe
			// could have opened it): ControlMaster=no, ControlPath=none.
			opts := []string{"-o", "StrictHostKeyChecking=yes", "-o", pinOption, "-o", "ControlMaster=no", "-o", "ControlPath=none"}
			if alias != spec.Host && !operator[k.Fingerprint] {
				opts = append(opts, "-o", "HostKeyAlias="+alias)
			}
			return opts, nil
		}
	}
	fp := keys[0].Fingerprint
	return nil, errs.New("E487", fmt.Sprintf("unknown host key %s for %s; verify it, then run nself env target add %s %s --trust-host-key %s\n(or connect once with plain ssh and accept the key into ~/.ssh/known_hosts)", fp, host, env, server, fp))
}

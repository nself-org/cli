// Purpose: the one pin-alias scheme for nSelf's own host-key pin file, shared
//          by deploy, admin connect and access.
// Inputs:  a HostSpec (host, optional port).
// Outputs: the alias under which that endpoint's keys are pinned and passed to
//          ssh as HostKeyAlias, the aliases older releases used for it, and a
//          one-time copy of a legacy pin to the new alias.
// Constraints: the alias is a function of (lowercased host, effective port)
//              alone and is injective: a name host never contains '+' or ':',
//              so "nself+<host>+<port>" cannot be produced by any host name
//              and two endpoints never share an alias. Port 22 and no port are
//              the same endpoint (ssh's default), one rule. A name host on
//              port 22 keeps its bare name (the operator's known_hosts form);
//              an IP literal always gets the synthetic form because the pin
//              file refuses an IP as an alias. A name starting "nself-" also
//              gets the synthetic form: that bare prefix is the legacy alias
//              namespace, so a pin written by this scheme is never read as
//              another endpoint's legacy pin. Legacy aliases are only read:
//              nothing is deleted from the pin file.

package remote

import (
	"fmt"
	"strconv"
	"strings"
)

// defaultSSHPort is the port ssh uses when none is given.
const defaultSSHPort = 22

// pinAliasPrefix starts every synthetic alias. '+' is outside what a host name
// can hold, so no host name collides with it.
const pinAliasPrefix = "nself+"

// legacyAliasPrefix starts every legacy alias. PinAlias never returns a bare
// name with this prefix, so a current pin cannot be mistaken for a legacy one.
const legacyAliasPrefix = "nself-"

func effectivePort(spec HostSpec) int {
	if spec.Port == 0 {
		return defaultSSHPort
	}
	return spec.Port
}

// PinAlias returns the alias spec's host keys are pinned under.
func PinAlias(spec HostSpec) string {
	host := strings.ToLower(spec.Host)
	port := effectivePort(spec)
	if port == defaultSSHPort && !isIPLiteral(spec.Host) && !strings.HasPrefix(host, legacyAliasPrefix) {
		return host
	}
	return pinAliasPrefix + host + "+" + strconv.Itoa(port)
}

// LegacyPinAliases returns the aliases releases before PinAlias pinned spec
// under, other than PinAlias(spec) itself. They were
// "nself-<host with ':' as '-'>-<port>" for any host with a port and for any IP
// literal (port 22 when none was given), which collided across endpoints and
// made deploy's "host:22" pin invisible to admin.
func LegacyPinAliases(spec HostSpec) []string {
	if spec.Port == 0 && !isIPLiteral(spec.Host) {
		return nil // the bare name, which is also the current alias
	}
	return []string{legacyAliasPrefix + strings.ReplaceAll(strings.ToLower(spec.Host), ":", "-") + "-" + strconv.Itoa(effectivePort(spec))}
}

// keyBlob is the "<type> <base64>" form of a key line.
func keyBlob(line string) (string, error) {
	typ, b64, err := parseKeyLine(line)
	if err != nil {
		return "", err
	}
	return typ + " " + b64, nil
}

// MigrateLegacyPin copies the keys pinned for spec under a legacy alias to
// PinAlias(spec), but only those the endpoint presents now (live): a legacy
// pin that names a different key is left alone and the caller's normal check
// refuses it. A key already pinned, or a different key of a type already
// pinned at the new alias, copies nothing. The legacy lines stay in the file.
func (p PinnedHostKeys) MigrateLegacyPin(spec HostSpec, live []HostKey) error {
	legacy := LegacyPinAliases(spec)
	if len(legacy) == 0 {
		return nil
	}
	alias := PinAlias(spec)
	current, err := p.Lookup(alias)
	if err != nil {
		return err
	}
	pinnedTypes := map[string]bool{}
	for _, k := range current {
		pinnedTypes[strings.Fields(k)[0]] = true
	}
	for _, old := range legacy {
		keys, err := p.Lookup(old)
		if err != nil {
			return err
		}
		for _, k := range keys {
			for _, l := range live {
				blob, err := keyBlob(l.Line)
				if err != nil || blob != k || pinnedTypes[l.Type] {
					continue
				}
				if err := p.Add(alias, l.Line); err != nil {
					return fmt.Errorf("migrating the pin for %s: %w", alias, err)
				}
				pinnedTypes[l.Type] = true
			}
		}
	}
	return nil
}

// EffectivePinAlias is PinAlias(spec), or the legacy alias when only that one
// holds pins: the read-only form for a caller that has no live key to migrate
// with. Nothing is written.
func (p PinnedHostKeys) EffectivePinAlias(spec HostSpec) (string, error) {
	alias := PinAlias(spec)
	keys, err := p.Lookup(alias)
	if err != nil || len(keys) > 0 {
		return alias, err
	}
	for _, old := range LegacyPinAliases(spec) {
		keys, err := p.Lookup(old)
		if err != nil {
			return alias, err
		}
		if len(keys) > 0 {
			return old, nil
		}
	}
	return alias, nil
}

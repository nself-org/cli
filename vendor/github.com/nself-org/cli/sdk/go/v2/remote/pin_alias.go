// Purpose: one endpoint namespace for deploy, admin and access host-key pins.
// Inputs: a parsed HostSpec and the dedicated nSelf pin store.
// Outputs: a collision-free alias and canonical pinned keys.
// Constraints: '+' cannot occur in a parsed host; ambiguous legacy synthetic
// aliases never confer trust. A canonical pin always wins over legacy entries.
package remote

import (
	"net"
	"strconv"
	"strings"
)

// PinAlias identifies a host and effective port, independently of SSH user.
// Port 0 and 22 share a pin. The '+' delimiters cannot occur in HostSpec.Host.
func PinAlias(spec HostSpec) string {
	port := spec.Port
	if port == 0 {
		port = 22
	}
	return "nself+" + strings.ToLower(spec.Host) + "+" + strconv.Itoa(port)
}

// LookupHost reads canonical pins, migrating unambiguous bare DNS pins once.
// Legacy synthetic aliases lack endpoint provenance and require explicit
// fingerprint enrollment. Legacy entries remain for older clients, but are
// never consulted again after a canonical pin exists.
func (p PinnedHostKeys) LookupHost(spec HostSpec) ([]string, error) {
	alias := PinAlias(spec)
	keys, err := p.Lookup(alias)
	if err != nil || len(keys) > 0 {
		return keys, err
	}
	if (spec.Port != 0 && spec.Port != 22) || net.ParseIP(spec.Host) != nil ||
		strings.HasPrefix(strings.ToLower(spec.Host), "nself-") {
		return nil, nil
	}
	legacy, err := checkPinAlias(spec.Host)
	if err != nil {
		return nil, err
	}
	keys, err = p.Lookup(legacy)
	if err != nil || len(keys) == 0 {
		return keys, err
	}
	return p.migrateHostPin(alias, legacy)
}

// migrateHostPin rechecks both aliases under the writer lock before copying
// all legacy key types atomically, so concurrent enrollment cannot be replaced.
func (p PinnedHostKeys) migrateHostPin(alias, legacy string) ([]string, error) {
	pinMu.Lock()
	defer pinMu.Unlock()
	unlock, err := p.acquire(true)
	if err != nil {
		return nil, err
	}
	defer unlock()
	lines, err := p.read()
	if err != nil {
		return nil, err
	}
	var canonical, old []string
	for _, line := range lines {
		f := hostFields(line)
		if f == nil {
			continue
		}
		if strings.EqualFold(f[0], alias) {
			canonical = append(canonical, f[1]+" "+f[2])
		}
		if strings.EqualFold(f[0], legacy) {
			old = append(old, f[1]+" "+f[2])
		}
	}
	if len(canonical) > 0 || len(old) == 0 {
		return canonical, nil
	}
	for _, key := range old {
		lines = append(lines, alias+" "+key)
	}
	if err := p.write(lines); err != nil {
		return nil, err
	}
	return old, nil
}

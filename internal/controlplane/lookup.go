package controlplane

// lookup.go: pure Inventory lookups shared by deploy and promote (moved out of
// cmd/commands, package-size ratchet).

import "strings"

// EnvName finds env in inv by case-insensitive name. It reports false
// for a nil inventory, no match, or when two inventory keys differ only by
// case (even if one matches exactly): the caller refuses rather than guesses.
func EnvName(inv *Inventory, env string) (string, bool) {
	if inv == nil {
		return "", false
	}
	found := ""
	for k := range inv.Environments {
		if strings.EqualFold(k, env) {
			if found != "" {
				return "", false
			}
			found = k
		}
	}
	return found, found != ""
}

// PrimaryServer picks the server the build is snapshotted against.
func PrimaryServer(inv *Inventory, env string) (Server, bool) {
	e, ok := inv.Environments[env]
	if !ok || len(e.Servers) == 0 {
		return Server{}, false
	}
	for _, s := range e.Servers {
		if s.Primary {
			return s, true
		}
	}
	for _, s := range e.Servers {
		if s.Role == RoleApp {
			return s, true
		}
	}
	return e.Servers[0], true
}

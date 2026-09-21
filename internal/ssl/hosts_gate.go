package ssl

// hosts_gate.go — decides whether `nself build` is allowed to mutate
// /etc/hosts at all, before applyTrustAndHosts (generator_trust.go) ever
// touches the file.
//
// Purpose: a production box was found with 14 "*.local.nself.org ->
// 127.0.0.1" lines appended to /etc/hosts by a bare `nself build` run — a
// dev-only convenience with no business running against a live server.
// shouldManageHosts is the one gate every automatic (non-dns-setup)
// hosts-writing path in this package goes through now.
// Inputs: cfg.Env (already normalized by config.ApplyDefaults by the time
// Build() reaches SSL generation) and cfg.BaseDomain, plus explicitHosts —
// the operator's own --hosts flag, threaded down from
// build.BuildOptions.Hosts.
// Outputs: true only when /etc/hosts may be written for this project.
// Constraints: pure — no filesystem access. The separate, filesystem-
// touching permission pre-check is canWriteHostsFile in hosts.go.

import "strings"

// shouldManageHosts reports whether nself build may write to /etc/hosts for
// this project.
//
//   - cfg.Env == "prod" is an absolute veto: never write, regardless of
//     explicitHosts or how the domain looks. Env is the strongest signal
//     available that this checkout is running against a live box, and
//     domain-name heuristics alone were exactly what let the incident
//     through (BASE_DOMAIN looked plausible; nothing checked Env).
//   - Otherwise, a recognized local-development domain shape (exactly
//     "localhost", or ending in ".local.nself.org", ".localhost", or
//     ".local") is allowed automatically — this is nself's normal local
//     dev experience and must keep working with zero flags.
//   - Any other domain is written only when the operator explicitly opts
//     in via explicitHosts (--hosts / -F build.BuildOptions.Hosts). Without
//     that, an unrecognized domain is treated as "possibly a real server"
//     rather than guessed at.
func shouldManageHosts(env, baseDomain string, explicitHosts bool) bool {
	if env == "prod" {
		return false
	}
	if isLocalDevDomain(baseDomain) {
		return true
	}
	return explicitHosts
}

// isLocalDevDomain reports whether domain has the shape of a local
// development-only name: exactly "localhost", or ending in
// ".local.nself.org", ".localhost", or ".local".
func isLocalDevDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	if d == "" {
		return false
	}
	if d == "localhost" {
		return true
	}
	return strings.HasSuffix(d, ".local.nself.org") ||
		strings.HasSuffix(d, ".localhost") ||
		strings.HasSuffix(d, ".local")
}

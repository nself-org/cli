package config

// nginx_mode.go — NGINX_MODE, the owner's opt-in to a shared host proxy.
//
// Purpose: EPIC ADOPT D2 (P7-ADOPT-03). A project that sets NGINX_MODE=shared
// owns an nginx that fronted projects nested under it register with. The key
// existed (loader_known_vars_ops.go) and nothing read it; this file gives it
// one meaning and one validation.
// Inputs: Config.Nginx.Mode and Config.Nginx.FrontedBy.
// Outputs: NginxModeShared, (*Config).NginxSharedOwner and NginxModeProblem.
// Constraints: pure functions over the loaded Config, no I/O. The compat gate
// (warn in v1.4, E505 in v1.5) lives where the build acts on the problem, so
// this file stays free of compat.

// NginxModeShared is the only NGINX_MODE value: this project owns a shared
// host proxy.
const NginxModeShared = "shared"

// NginxSharedOwner reports whether this project is a valid shared-proxy owner:
// NGINX_MODE=shared and no NGINX_FRONTED_BY (an owner serves itself).
func (c *Config) NginxSharedOwner() bool {
	return c.Nginx.Mode == NginxModeShared && c.Nginx.FrontedBy == ""
}

// NginxModeProblem returns "" when NGINX_MODE is valid, otherwise one sentence
// naming the problem and the fix: an unknown value, or NGINX_MODE=shared
// together with NGINX_FRONTED_BY.
func (c *Config) NginxModeProblem() string {
	switch {
	case c.Nginx.Mode != "" && c.Nginx.Mode != NginxModeShared:
		return "NGINX_MODE=" + c.Nginx.Mode + " is not a known mode; the only value is \"shared\" (unset it for a stack that serves only itself)"
	case c.Nginx.Mode == NginxModeShared && c.Nginx.FrontedBy != "":
		return "NGINX_MODE=shared cannot be combined with NGINX_FRONTED_BY=" + c.Nginx.FrontedBy +
			": the owner of a shared proxy is not itself fronted; unset one of the two"
	}
	return ""
}

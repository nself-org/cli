package machineauth

// bind.go — which bind addresses may carry the machine token.
//
// Purpose: a bearer token over plain HTTP is only acceptable on loopback, or
//   when the operator named the host (--allowed-host) and configured the token
//   on purpose.
// Inputs:  the --host value, the --allowed-host list, the token source from
//   Lookup.
// Outputs: IsLoopback, and CheckBind's E426 refusal.
// Constraints: a generated token never counts as configured: a non-loopback
//   bind needs NSELF_MCP_TOKEN or an existing token file on that run.

import (
	"fmt"
	"net"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// IsLoopback reports whether host is a loopback bind address: "localhost",
// any 127.0.0.0/8 address, or ::1 (with or without brackets). The empty host,
// 0.0.0.0 and :: bind every interface and are not loopback.
func IsLoopback(host string) bool {
	h := strings.TrimSpace(host)
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// CheckBind returns nil for a loopback host. A non-loopback host passes only
// with at least one non-empty allowedHosts entry and a configured token
// (source "env" or "file"); otherwise it is refused with E426.
func CheckBind(host string, allowedHosts []string, source string) error {
	if IsLoopback(host) {
		return nil
	}
	named := false
	for _, a := range allowedHosts {
		if strings.TrimSpace(a) != "" {
			named = true
			break
		}
	}
	configured := source == SourceEnv || source == SourceFile
	if named && configured {
		return nil
	}
	var missing []string
	if !named {
		missing = append(missing, "--allowed-host <name>")
	}
	if !configured {
		missing = append(missing, "a configured token ("+EnvVar+" or the file ~/.nself/mcp-token)")
	}
	return errs.New("E426", fmt.Sprintf("refusing a non-loopback bind: it needs %s", strings.Join(missing, " and "))).
		WithFix("Bind 127.0.0.1, or pass --allowed-host <name> and set " + EnvVar + " or create ~/.nself/mcp-token (mode 0600).")
}

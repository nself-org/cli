package httpapi

// Purpose: validate host allowlist entries before serving requests.
// Inputs: host names or IP literals. Outputs: a user error for invalid entries.
// Constraints: no ports, URL components, whitespace or unbracketed IPv6.

import (
	"net"
	"strings"

	"github.com/nself-org/cli/internal/errs"
)

// ValidateAllowedHosts accepts DNS names, IPv4 literals and bracketed IPv6
// literals. An empty list adds no hosts; an invalid entry returns a user error
// naming that entry. It does not resolve DNS or accept ports or URL components.
func ValidateAllowedHosts(hosts []string) error {
	for _, host := range hosts {
		if !validAllowedHost(host) {
			return errs.Newf("E430", "Invalid allowed host %q: use a DNS name, IPv4 literal or bracketed IPv6 literal without a port", host).
				WithWhy("an allowed host is a DNS name, an IPv4 literal or a bracketed IPv6 literal, without scheme, port or path").
				WithFix("pass --allowed-host <name>; the server adds the port itself")
		}
	}
	return nil
}

func validAllowedHost(host string) bool {
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		literal := host[1 : len(host)-1]
		return strings.Contains(literal, ":") && net.ParseIP(literal) != nil
	}
	if ip := net.ParseIP(host); ip != nil {
		return !strings.Contains(host, ":") && ip.To4() != nil
	}
	name := strings.TrimSuffix(host, ".")
	if len(name) == 0 || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if !validDNSLabel(label) {
			return false
		}
	}
	return true
}

func validDNSLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
		return false
	}
	for _, c := range label {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

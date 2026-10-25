package nginxtopo

// ssl_path.go — the one in-container path both the nginx conf generator and
// the compose service definition must agree on for TLS to actually start.
//
// Purpose: a production box was found with generated nginx confs
// referencing "/etc/nginx/ssl/certificates/local-nself-org/..." — the
// generator's literal was correct in isolation, but it and the compose
// volume mount target ("./ssl:/etc/nginx/ssl:ro") were two independently
// hand-typed string literals in two different packages (internal/nginx,
// internal/compose) with nothing forcing them to agree. NginxSSLContainerPath
// lives here — a package with zero internal dependencies, so both can import
// it without a cycle — as the single place either would need to change.
// Inputs: none for the constant; a domain for CertDirName.
// Outputs: NginxSSLContainerPath, the absolute in-container directory nginx's
// TLS material is mounted at, and the certificate directory name of a domain.
// Constraints: this is the MOUNT TARGET only. The certificate subdirectory
// name under "<NginxSSLContainerPath>/certificates/" is a second, separate
// question — see CertDirName below, the one implementation of
// domain-to-directory-name both internal/ssl (which writes the certs) and
// internal/nginx (which references them) share.

import "strings"

const NginxSSLContainerPath = "/etc/nginx/ssl"

// CertDirName is the directory name under "<NginxSSLContainerPath>/certificates/"
// a domain's certificate lives in: dots become dashes. It is the one domain to
// directory rule: internal/ssl (which writes the certs), internal/nginx and
// internal/nginx/routemodel (which reference them) and the plugin snippet
// ${SSL_DIR} all call it, so the copies cannot drift (D-0573).
func CertDirName(domain string) string {
	return strings.ReplaceAll(domain, ".", "-")
}

// CertDirNameOrLocalhost is CertDirName with the empty-domain fallback
// "localhost", for readers of the certificate dir (BaseDomain is never empty
// in a real build, but generator tests and ad-hoc callers may pass "").
func CertDirNameOrLocalhost(domain string) string {
	if dir := CertDirName(domain); dir != "" {
		return dir
	}
	return "localhost"
}

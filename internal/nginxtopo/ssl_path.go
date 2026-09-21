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
// Inputs: none (a constant).
// Outputs: NginxSSLContainerPath, the absolute in-container directory nginx's
// TLS material is mounted at.
// Constraints: this is the MOUNT TARGET only. The certificate subdirectory
// name under "<NginxSSLContainerPath>/certificates/" is a second, separate
// question — see internal/ssl.DomainToDirName, the one implementation of
// domain-to-directory-name both internal/ssl (which writes the certs) and
// internal/nginx (which references them) now share.
const NginxSSLContainerPath = "/etc/nginx/ssl"

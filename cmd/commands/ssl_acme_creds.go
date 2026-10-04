package commands

// ssl_acme_creds.go: read a certbot DNS-plugin INI for --dns-credential-file.
// cloudflare dns_cloudflare_api_token, digitalocean dns_digitalocean_token,
// route53 aws_access_key_id + aws_secret_access_key; exactly one provider.
// Errors name keys, never values.

import (
	"os"

	"github.com/nself-org/cli/internal/ssl/acme"
)

// parseDNSCredentialFile returns the provider and secret-store name -> value
// pairs found in the INI at path.
func parseDNSCredentialFile(path string) (provider string, secrets map[string]string, err error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-chosen credential file
	if err != nil {
		return "", nil, acmeRefuse("check the --dns-credential-file path", "cannot read the credential file: %v", err)
	}
	m, found := acme.ParseINI(data), map[string]map[string]string{}
	if v := m["dns_cloudflare_api_token"]; v != "" {
		found["cloudflare"] = map[string]string{"SSL_DNS_CLOUDFLARE_API_TOKEN": v}
	}
	if v := m["dns_digitalocean_token"]; v != "" {
		found["digitalocean"] = map[string]string{"SSL_DNS_DIGITALOCEAN_TOKEN": v}
	}
	if id, sec := m["aws_access_key_id"], m["aws_secret_access_key"]; id != "" && sec != "" {
		found["route53"] = map[string]string{"SSL_DNS_AWS_ACCESS_KEY_ID": id, "SSL_DNS_AWS_SECRET_ACCESS_KEY": sec}
	} else if id != "" || sec != "" {
		return "", nil, acmeRefuse("give both aws_access_key_id and aws_secret_access_key", "the route53 credential needs both keys")
	}
	for p, s := range found {
		if len(found) == 1 {
			return p, s, nil
		}
	}
	if m["dns_cloudflare_email"] != "" || m["dns_cloudflare_api_key"] != "" {
		return "", nil, acmeRefuse("create a scoped API token with Zone:DNS:Edit and pass it as dns_cloudflare_api_token", "a Cloudflare global API key is not accepted")
	}
	return "", nil, acmeRefuse("give exactly one of dns_cloudflare_api_token, dns_digitalocean_token, aws_access_key_id + aws_secret_access_key",
		"the credential file has %d DNS providers", len(found))
}

// acmeRefuse builds a refusal that e151 turns into an E151 error.
func acmeRefuse(fix, format string, a ...any) error { return acme.Refuse(fix, format, a...) }

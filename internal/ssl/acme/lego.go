package acme

// lego.go: run the pinned lego image once to issue over DNS-01. Credentials go
// in as `-e NAME` (values only in the child env); no Docker socket; container
// user = invoking uid:gid; the served ssl dir is /ssl. Output: <ssl>/.acme
// [/staging]/certificates/<name>.crt (fullchain) + .key. NSELF_ACME_* test
// hooks and provider `exec` are refused unless the directory is a
// non-production CA, so a hook can never reach Let's Encrypt.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/docker"
)

// Directory URLs.
const (
	ProdDirectory    = "https://acme-v02.api.letsencrypt.org/directory"
	StagingDirectory = "https://acme-staging-v02.api.letsencrypt.org/directory"
)

// providers maps provider -> {secret-store name, lego env name}.
var providers = map[string][][2]string{
	"cloudflare":   {{"SSL_DNS_CLOUDFLARE_API_TOKEN", "CF_DNS_API_TOKEN"}},
	"digitalocean": {{"SSL_DNS_DIGITALOCEAN_TOKEN", "DO_AUTH_TOKEN"}},
	"route53":      {{"SSL_DNS_AWS_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID"}, {"SSL_DNS_AWS_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY"}},
}

// SecretNames returns the secret names a provider needs; ok is false for an
// unknown provider. Provider "exec" (test hook) needs none.
func SecretNames(provider string) (names []string, ok bool) {
	for _, e := range providers[provider] {
		names = append(names, e[0])
	}
	return names, len(names) > 0 || provider == "exec"
}

// Hooks are the test-only knobs read from NSELF_ACME_*.
type Hooks struct{ Directory, CABundle, Network, Resolvers, Fault string }

// HooksFromEnv reads the hooks; any of them without a non-Let's-Encrypt
// NSELF_ACME_DIRECTORY (i.e. against production) is refused.
func HooksFromEnv(get func(string) string) (Hooks, error) {
	h := Hooks{get("NSELF_ACME_DIRECTORY"), get("NSELF_ACME_CA_BUNDLE"), get("NSELF_ACME_NETWORK"), get("NSELF_ACME_DNS_RESOLVERS"), get("NSELF_ACME_FAULT")}
	if h != (Hooks{}) && (h.Directory == "" || strings.Contains(h.Directory, "api.letsencrypt.org")) {
		return h, Refuse("unset the NSELF_ACME_* variables", "NSELF_ACME_* test hooks are refused against Let's Encrypt production")
	}
	return h, nil
}

// IssueReq describes one issuance. AcceptTOS adds --accept-tos (set only when
// NeedsTOS and the terms were agreed); Secret reads the secret store.
type IssueReq struct {
	SSLDir, Name, Provider, Contact string
	Domains                         []string
	Staging, AcceptTOS              bool
	Hooks                           Hooks
	Secret                          func(name string) (string, error)
	Run                             func(context.Context, docker.RunSpec) (string, string, error)
}

func stateRoot(sslDir string, staging bool) string {
	if staging {
		return filepath.Join(StateDir(sslDir), "staging")
	}
	return StateDir(sslDir)
}

// NeedsTOS reports whether no ACME account for contact exists yet.
func NeedsTOS(sslDir string, staging bool, contact string) bool {
	m, _ := filepath.Glob(filepath.Join(stateRoot(sslDir, staging), "accounts", "*", contact))
	return len(m) == 0
}

// Issue runs `lego run` and returns the certificate and key paths on the host.
func Issue(ctx context.Context, r IssueReq) (cert, key string, err error) {
	server, root := ProdDirectory, "/ssl/.acme"
	if r.Staging {
		server, root = StagingDirectory, root+"/staging"
	}
	if r.Hooks.Directory != "" {
		server = r.Hooks.Directory
	}
	args := []string{"--path", root, "--email", r.Contact, "--server", server, "--key-type", "ec256", "--dns", r.Provider}
	if r.Hooks.Resolvers != "" {
		// challtestsrv answers no SOA, so the authoritative-NS pre-check cannot run.
		args = append(args, "--dns.resolvers", r.Hooks.Resolvers, "--dns.propagation-disable-ans")
	}
	for _, d := range r.Domains {
		args = append(args, "--domains", d)
	}
	if r.AcceptTOS {
		args = append(args, "--accept-tos")
	}
	spec := docker.RunSpec{Image: LegoImage, Args: append(args, "run"), Network: r.Hooks.Network,
		EnvPass: map[string]string{}, Mounts: []docker.Mount{{Source: r.SSLDir, Destination: "/ssl"}}}
	if os.Getuid() >= 0 {
		spec.User = strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	}
	if err := credentials(&spec, r); err != nil {
		return "", "", err
	}
	if r.Hooks.CABundle != "" {
		spec.Mounts = append(spec.Mounts, docker.Mount{Source: r.Hooks.CABundle, Destination: "/ca.pem", ReadOnly: true})
		spec.EnvPass["LEGO_CA_CERTIFICATES"] = "/ca.pem"
	}
	if r.Run == nil {
		r.Run = docker.RunOneShot
	}
	if _, stderr, rerr := r.Run(ctx, spec); rerr != nil {
		return "", "", fmt.Errorf("lego run for %s failed: %w\n%s", r.Name, rerr, strings.TrimSpace(stderr[max(0, len(stderr)-400):]))
	}
	base := filepath.Join(stateRoot(r.SSLDir, r.Staging), "certificates", strings.ReplaceAll(r.Domains[0], "*", "_"))
	return base + ".crt", base + ".key", nil
}

// credentials fills spec.EnvPass from the secret store; provider exec forwards
// EXEC_PATH and mounts that script read-only.
func credentials(spec *docker.RunSpec, r IssueReq) error {
	if r.Provider == "exec" {
		p := os.Getenv("EXEC_PATH")
		if r.Hooks.Directory == "" || p == "" {
			return Refuse("provider exec is a test hook", "provider exec needs NSELF_ACME_DIRECTORY and EXEC_PATH")
		}
		spec.EnvPass["EXEC_PATH"] = p
		spec.Mounts = append(spec.Mounts, docker.Mount{Source: p, Destination: p, ReadOnly: true})
		return nil
	}
	for _, e := range providers[r.Provider] {
		v, err := r.Secret(e[0])
		if err != nil || v == "" {
			return Refuse("store it with `nself secrets set "+e[0]+" <value>`", "DNS credential %s is not in the secret store", e[0])
		}
		spec.EnvPass[e[1]] = v
	}
	if r.Provider == "route53" {
		if spec.EnvPass["AWS_REGION"] = os.Getenv("AWS_REGION"); spec.EnvPass["AWS_REGION"] == "" {
			spec.EnvPass["AWS_REGION"] = "us-east-1"
		}
	}
	return nil
}

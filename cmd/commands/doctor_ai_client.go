package commands

// The small AI client `nself doctor` needs.
//
// Purpose: doctor probes a local Ollama install and the ai plugin daemon as
// part of its AI checks. Those probes used to share helpers with the `nself ai`
// command family, which moved to the ai-cli plugin under CLI-R11; doctor stayed
// in core, so the handful of helpers it actually calls stayed with it.
//
// Inputs: OLLAMA_BASE_URL / OLLAMA_HOST, PLUGIN_AI_INTERNAL_URL and
// PLUGIN_INTERNAL_SECRET from the environment.
//
// Outputs: a base URL, and the raw bytes plus status of a plugin request.
//
// Constraints: this is a deliberate duplicate of code the ai-cli plugin also
// carries, and the smaller half of it — doctor only reads. Merging them back
// would mean either dragging doctor into the plugin or dragging the plugin's
// command tree back into core. If the plugin's port or auth header changes,
// this has to change with it, which is why both sides name the same env vars
// rather than hardcoding anything.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/nself-org/cli/internal/httptimeout"
)

// Test seams. Production values: the system resolver, no dial hook, stderr.
var (
	aiLookupIP    = net.DefaultResolver.LookupIPAddr
	aiDialContext func(ctx context.Context, network, addr string) (net.Conn, error)
	aiWarnOut     io.Writer = os.Stderr
	aiWarnOnce    sync.Once
)

// aiIPLocal reports whether ip is loopback or private (10/8, 172.16/12,
// 192.168/16, fc00::/7). Docker IPv6 networks use ULA, so it is accepted on
// purpose. Link-local, unspecified and multicast addresses are not local.
func aiIPLocal(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate())
}

// aiTokenHostLocal reports whether PLUGIN_INTERNAL_SECRET may be attached to a
// request for rawURL: the scheme is http or https and the URL's hostname is
// `localhost`, a loopback or private IP literal, or `plugin-ai` with every
// resolved address loopback or private. Everything else, including a lookup
// error or an empty result, is not local. url.Hostname is used, so
// http://127.0.0.1@evil.example is judged as evil.example.
func aiTokenHostLocal(ctx context.Context, rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return aiIPLocal(ip)
	}
	if host != "plugin-ai" {
		return false
	}
	addrs, err := aiLookupIP(ctx, host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		if !aiIPLocal(a.IP) {
			return false
		}
	}
	return true
}

// aiTokenClient is the only client allowed to carry the token: no proxy, no
// redirects (httptimeout.NoProxy). aiDialContext, when set, redirects dials so
// tests can point host plugin-ai at an httptest server.
func aiTokenClient() *http.Client {
	c := httptimeout.NoProxy(httptimeout.Default.Timeout)
	if aiDialContext != nil {
		c.Transport.(*http.Transport).DialContext = aiDialContext
	}
	return c
}

// aiWarnTokenWithheld writes one warn line per process to aiWarnOut (stderr by
// default). It never names the secret value, and it is not an error.
func aiWarnTokenWithheld(rawURL string) {
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = u.Hostname()
	}
	aiWarnOnce.Do(func() {
		_, _ = fmt.Fprintf(aiWarnOut, "warn: PLUGIN_INTERNAL_SECRET not sent: PLUGIN_AI_INTERNAL_URL host %s is not a loopback or private address\n", host)
	})
}

// ollamaBaseURL resolves where a local Ollama is listening.
// OLLAMA_HOST is accepted without a scheme because that is how Ollama's own
// tooling sets it.
func ollamaBaseURL() string {
	if u := os.Getenv("OLLAMA_BASE_URL"); u != "" {
		return u
	}
	if u := os.Getenv("OLLAMA_HOST"); u != "" {
		if !strings.HasPrefix(u, "http") {
			return "http://" + u
		}
		return u
	}
	return "http://127.0.0.1:11434"
}

// aiPluginURL resolves the ai plugin daemon's internal address.
//
// plugin-ai listens on 3709. The default hostname matches the docker-compose
// service name `nself build` generates; older templates used "ai:3680", so
// PLUGIN_AI_INTERNAL_URL overrides it.
func aiPluginURL() string {
	if u := os.Getenv("PLUGIN_AI_INTERNAL_URL"); u != "" {
		return u
	}
	return "http://plugin-ai:3709"
}

// aiPluginRequest performs a request against the ai plugin daemon, returning
// the body and status rather than an error for non-2xx: doctor reports what it
// found rather than failing on it.
func aiPluginRequest(ctx context.Context, method, path string, body []byte) ([]byte, int, error) {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	base := aiPluginURL()
	req, err := http.NewRequestWithContext(ctx, method, base+path, rdr)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// The secret goes only to a local daemon, through a client that never uses a
	// proxy and never follows a redirect (D15). Any other host gets the request
	// without the token and one stderr warning; doctor's result is unchanged.
	client := httptimeout.Default
	if tok := os.Getenv("PLUGIN_INTERNAL_SECRET"); tok != "" {
		if aiTokenHostLocal(ctx, base) {
			req.Header.Set("X-Internal-Token", tok)
			client = aiTokenClient()
		} else {
			aiWarnTokenWithheld(base)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	// Capped read: this talks to a local daemon, but doctor must not be a way to
	// exhaust memory if that daemon misbehaves.
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return b, resp.StatusCode, nil
}

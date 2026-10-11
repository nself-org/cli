package nginx

import (
	"bytes"
	"cmp"
	"fmt"
	"strings"
	"text/template"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginx/routemodel"
	"github.com/nself-org/cli/internal/nginxtopo"
)

// securityHeaders is retained for callers of the single-route renderer.
const securityHeaders = `    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header Permissions-Policy "camera=(), microphone=(), geolocation=()" always;
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains" always;
    server_tokens off;`

// upstreamHideHeaders lists response headers from upstream services that
// should be stripped before forwarding to the client.
var upstreamHideHeaders = []string{
	"X-Powered-By",
	"Server",
	"X-Runtime",
	"X-Version",
	"X-Generator",
}

// proxyHideHeaderDirectives returns the nginx proxy_hide_header directives.
func proxyHideHeaderDirectives() string {
	var sb strings.Builder
	for _, h := range upstreamHideHeaders {
		fmt.Fprintf(&sb, "    proxy_hide_header %s;\n", h)
	}
	return sb.String()
}

// Generator builds nginx configuration files from project configuration.
type Generator struct {
	cfg        *config.Config
	tmpl       *template.Template
	seenRoutes map[string]bool // tracks server names seen in the current generation batch
	workdir    string          // project root directory for conf.d/ conflict scanning
	hasSSL     bool            // true when a cert pair for BaseDomain is expected/present on disk — see sslShouldEmit
	model      *routemodel.Model
}

// WithModel selects the route snapshot prepared by build preflight.
func (g *Generator) WithModel(m *routemodel.Model) *Generator {
	g.model, g.hasSSL = m, m.DefaultServer.HTTP.RedirectHTTPS
	return g
}

// HasSSL reports whether this generator will emit HTTPS server blocks.
func (g *Generator) HasSSL() bool { return g.hasSSL }

// NewGenerator creates an nginx Generator from the given config.
// When SSL_MODE is "local" (or empty, which defaults to "local" in dev),
// nginx server blocks are generated with ssl_certificate directives pointing
// to the locally generated cert files. For "letsencrypt"/"custom" modes the
// HTTPS server blocks are emitted only once a certificate pair already
// exists on disk for the domain — see sslShouldEmit. "none" always omits
// them, regardless of what cert files happen to be present.
func NewGenerator(cfg *config.Config, workdir string) *Generator {
	mode := cfg.SSLMode
	if mode == "" {
		mode = "local"
	}
	return &Generator{cfg: cfg, workdir: workdir, hasSSL: sslShouldEmit(cfg, workdir, mode, nil)}
}

// WithAssumedCerts marks domains whose certificate pair a plan-mode build will create.
func (g *Generator) WithAssumedCerts(domains []string) *Generator {
	g.hasSSL = sslShouldEmit(g.cfg, g.workdir, cmp.Or(g.cfg.SSLMode, "local"), domains)
	return g
}

// Generate produces all nginx config files.
// Returns map of filepath to content (relative to project root).
func (g *Generator) Generate() (map[string]string, error) {
	if err := g.parseTemplates(); err != nil {
		return nil, fmt.Errorf("parsing nginx templates: %w", err)
	}
	if err := g.ensureModel(); err != nil {
		return nil, err
	}

	files := make(map[string]string)

	// Main nginx.conf
	main, err := g.generateMainConf()
	if err != nil {
		return nil, err
	}
	files["nginx/nginx.conf"] = main

	// Rate limits
	rates, err := g.generateRateLimits()
	if err != nil {
		return nil, err
	}
	files["nginx/includes/rate-limits.conf"] = rates

	// Default server (HTTP->HTTPS + /health)
	def, err := g.generateDefaultServer()
	if err != nil {
		return nil, err
	}
	files["nginx/conf.d/default.conf"] = def

	// Per-service routes
	routes, err := g.generateAllRoutes()
	if err != nil {
		return nil, err
	}
	for path, content := range routes {
		files[path] = content
	}

	return files, nil
}

// parseTemplates loads all embedded templates from the templateFS.
func (g *Generator) parseTemplates() error {
	tmpl, err := template.ParseFS(templateFS, "templates/*.tmpl")
	if err != nil {
		return fmt.Errorf("parsing embedded templates: %w", err)
	}
	g.tmpl = tmpl
	return nil
}

// mainConfData holds template data for nginx.conf.tmpl.
type mainConfData struct {
	ProjectName string
	MaxBody     string
	Env         string
}

// generateMainConf projects model defaults into the unchanged nginx template.
func (g *Generator) generateMainConf() (string, error) {
	if err := g.ensureModel(); err != nil {
		return "", err
	}
	data := mainConfData{
		ProjectName: g.model.Project,
		MaxBody:     g.model.MaxBodyLiteral,
		Env:         g.model.Env,
	}
	if data.MaxBody == "" {
		data.MaxBody = fmt.Sprintf("%d", g.model.Defaults.MaxBodyBytes)
	}
	return g.render("nginx.conf.tmpl", data)
}

// defaultConfData holds template data for default.conf.tmpl.
type defaultConfData struct {
	HTTPPort int
	SSLPort  int
	SSLDir   string
	// SSLBasePath is the in-container directory the ssl_certificate
	// directives are rooted at — always nginxtopo.NginxSSLContainerPath,
	// the same constant the compose nginx service mounts "./ssl" onto. Set
	// by the generator; not caller-configurable.
	SSLBasePath string
	// HasSSL controls whether the HTTPS default_server block is emitted.
	// Set to false for letsencrypt/custom/none SSL modes so nginx can start
	// before certificate files exist on disk.
	HasSSL bool
}

// generateDefaultServer renders the default server block with HTTP->HTTPS redirect.
func (g *Generator) generateDefaultServer() (string, error) {
	if err := g.ensureModel(); err != nil {
		return "", err
	}
	httpPort := g.cfg.Nginx.HTTPPort
	if httpPort == 0 {
		httpPort = 80
	}
	sslPort := g.cfg.Nginx.SSLPort
	if sslPort == 0 {
		sslPort = 443
	}

	data := defaultConfData{
		HTTPPort:    httpPort,
		SSLPort:     sslPort,
		SSLDir:      sslDirName(g.model.BaseDomain),
		SSLBasePath: nginxtopo.NginxSSLContainerPath,
		HasSSL:      g.model.DefaultServer.HTTP.RedirectHTTPS,
	}
	return g.render("default.conf.tmpl", data)
}

// ServiceRouteData holds template data for service.conf.tmpl.
type ServiceRouteData struct {
	Route      string
	BaseDomain string
	Upstream   string
	// UpstreamName is the sanitized name used for the nginx proxy_pass
	// variable ($up_<UpstreamName>) that forces per-request re-resolution
	// of Upstream via the Docker embedded DNS resolver. Set automatically
	// by the generator; callers do not populate it.
	UpstreamName string
	// ProxyTarget is Upstream as a proxy_pass URL with exactly one scheme
	// (bare "host:port" gets "http://"; an already-schemed internal-route
	// target passes through). Set by the generator; see proxyTarget below.
	ProxyTarget string
	SSLDir      string
	// SSLBasePath: see defaultConfData.SSLBasePath above. Set by the
	// generator in RenderServiceRoute; callers do not populate it.
	SSLBasePath string
	RateZone    string
	Burst       int
	ConnLimit   int
	WebSocket   bool
	PathZones   []PathZone // SEC-HARDENING-06, defaulted when nil — see pathzones.go
	// HasSSL controls whether ssl_certificate directives and the listen 443
	// directives are emitted. Set to false for letsencrypt/custom/none modes.
	HasSSL bool
}

// RenderServiceRoute renders a single service route config from the service.conf.tmpl.
// Exported so external callers can render individual service routes.
// HasSSL in the provided data is always overridden by the generator's own
// hasSSL value, which is derived from SSL_MODE. Callers do not need to set it.
func (g *Generator) RenderServiceRoute(data ServiceRouteData) (string, error) {
	if g.tmpl == nil {
		if err := g.parseTemplates(); err != nil {
			return "", err
		}
	}
	g.finalizeServiceRoute(&data)
	return g.render("service.conf.tmpl", data)
}

// render executes a named template with the given data and returns the result as a string.
func (g *Generator) render(name string, data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := g.tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("rendering template %s: %w", name, err)
	}
	return buf.String(), nil
}

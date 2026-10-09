package routemodel

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/nginxtopo"
)

const ciphers = "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-CHACHA20-POLY1305:ECDHE-RSA-CHACHA20-POLY1305:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384"

var gzipTypes = []string{"text/plain", "text/css", "text/xml", "text/javascript", "application/json", "application/javascript", "application/xml", "application/xml+rss", "application/vnd.api+json", "image/svg+xml", "font/woff", "font/woff2"}

type routeSpec struct {
	id, source, owner, file, name, legacy, upstream, zone string
	burst, conn                                           int
	websocket                                             bool
}

// Build collects the route data before any provider renders it.
func Build(cfg *config.Config, workdir string, hasSSL bool, trusted func(string) bool) (*Model, error) {
	maxBody := cfg.Nginx.MaxBody
	if maxBody == "" {
		maxBody = "100M"
	}
	bytes, err := parseSize(maxBody)
	if err != nil {
		return nil, err
	}
	env := cfg.Env
	if env == "" {
		env = "dev"
	}
	webroot, health := "/etc/nginx/ssl/.acme-webroot", "/health"
	m := &Model{Generated: Generated, SchemaVersion: "1", Project: cfg.ProjectName, MaxBodyLiteral: maxBody,
		Env: env, BaseDomain: cfg.BaseDomain, UnmodelledGlobal: []string{}, Routes: []Route{},
		Defaults: Defaults{MaxBodyBytes: bytes, Gzip: Gzip{Enabled: true, Types: append([]string{}, gzipTypes...)},
			TLS: TLSDefaults{Protocols: []string{"TLSv1.2", "TLSv1.3"}, Ciphers: ptr(ciphers)}},
		DefaultServer: DefaultServer{HTTP: DefaultHTTP{ACMEWebroot: &webroot, HealthPath: &health, RedirectHTTPS: hasSSL}, HTTPS: DefaultHTTPS{Action: "close"}},
		Zones:         buildZones(cfg)}
	sslDir := strings.ReplaceAll(cfg.BaseDomain, ".", "-")
	if sslDir == "" {
		sslDir = "localhost"
	}
	var specs []routeSpec
	add := func(id, source, owner, file, name, legacy, upstream, zone string, burst, conn int, websocket bool) {
		specs = append(specs, routeSpec{id, source, owner, file, name, legacy, upstream, zone, burst, conn, websocket})
	}
	core := func(route, fallback string) string {
		if route == "" {
			route = fallback
		}
		if cfg.AppName != "" {
			route += "." + cfg.AppName
		}
		return route
	}
	old := func(route, fallback string) string {
		if route == "" {
			return fallback
		}
		return route
	}
	add("core:hasura", "core", "", "hasura.conf", core(cfg.Hasura.Route, "api"), old(cfg.Hasura.Route, "api"), "hasura:8080", "graphql_api", 20, 10, true)
	port := cfg.Auth.Port
	if port == 0 {
		port = 4000
	}
	add("core:auth", "core", "", "auth.conf", core(cfg.Auth.Route, "auth"), old(cfg.Auth.Route, "auth"), fmt.Sprintf("auth:%d", port), "auth", 5, 5, false)
	if cfg.Minio.Enabled {
		add("core:storage", "core", "", "storage.conf", core(cfg.Minio.StorageRoute, "storage"), old(cfg.Minio.StorageRoute, "storage"), fmt.Sprintf("minio:%d", cfg.Minio.Port), "uploads", 2, 5, false)
		p := cfg.Minio.ConsolePort
		if p == 0 {
			p = 9001
		}
		add("core:storage-console", "core", "", "storage-console.conf", core(cfg.Minio.ConsoleRoute, "storage-console"), old(cfg.Minio.ConsoleRoute, "storage-console"), fmt.Sprintf("minio:%d", p), "", 0, 0, false)
	}
	if cfg.Admin.Enabled {
		up := "nself-admin"
		p := cfg.Admin.Port
		if p == 0 {
			p = 3021
		}
		if cfg.Admin.DevMode {
			up = "host.docker.internal"
			p = cfg.Admin.DevPort
			if p == 0 {
				p = 3000
			}
			if isLinux() {
				up = "172.17.0.1"
			}
		}
		add("optional:admin", "optional", "", "admin.conf", old(cfg.Admin.Route, "admin"), old(cfg.Admin.Route, "admin"), fmt.Sprintf("%s:%d", up, p), "general", 10, 0, false)
	}
	if cfg.Search.Enabled {
		p := cfg.Search.Port
		if p == 0 {
			p = 7700
		}
		engine := cfg.Search.Engine
		if engine == "" {
			engine = "meilisearch"
		}
		add("optional:search", "optional", "", "search.conf", old(cfg.Search.Route, "search"), old(cfg.Search.Route, "search"), fmt.Sprintf("%s:%d", engine, p), "general", 10, 0, false)
	}
	if cfg.Mailpit.Enabled {
		p := cfg.Mailpit.UIPort
		if p == 0 {
			p = 8025
		}
		add("optional:mail", "optional", "", "mail.conf", old(cfg.Mailpit.Route, "mail"), old(cfg.Mailpit.Route, "mail"), fmt.Sprintf("mailpit:%d", p), "general", 10, 0, false)
	}
	if cfg.Functions.Enabled {
		p := cfg.Functions.Port
		if p == 0 {
			p = 3008
		}
		add("optional:functions", "optional", "", "functions.conf", old(cfg.Functions.Route, "functions"), old(cfg.Functions.Route, "functions"), fmt.Sprintf("functions:%d", p), "functions", 15, 10, false)
	}
	for _, cs := range cfg.CustomServices {
		if cs.Route == "" || cs.Port == 0 {
			continue
		}
		add(fmt.Sprintf("cs:%d", cs.Index), "custom_service", cs.Name, "cs-"+cs.Name+".conf", cs.Route, cs.Route, fmt.Sprintf("%s:%d", cs.Name, cs.Port), "general", 10, 0, false)
	}
	for _, fe := range cfg.FrontendApps {
		if fe.Route == "" || fe.Port == 0 {
			continue
		}
		if fe.SystemName == "" {
			continue
		}
		add(fmt.Sprintf("frontend:%d", fe.Index), "frontend", "", "frontend-"+fe.SystemName+".conf", fe.Route, fe.Route, fmt.Sprintf("host.docker.internal:%d", fe.Port), "static", 10, 0, true)
	}
	for _, ir := range cfg.InternalRoutes {
		if ir.Subdomain == "" || ir.Target == "" {
			continue
		}
		if err := validateTarget(ir.Target); err != nil {
			return nil, fmt.Errorf("internal route %q: invalid target: %w", ir.Name, err)
		}
		zone := ir.RateZone
		if zone == "" {
			zone = "general"
		}
		add(fmt.Sprintf("internal:%d", ir.Index), "internal", "", "ir-"+ir.Name+".conf", ir.Subdomain, ir.Subdomain, ir.Target, zone, 10, 10, ir.WebSocket)
	}
	for _, s := range specs {
		m.Routes = append(m.Routes, makeRoute(cfg, workdir, sslDir, hasSSL, trusted, s))
	}
	return m, nil
}

func ptr(s string) *string { return &s }

// SetSSL refreshes TLS facts after the build's certificate phase.
func SetSSL(m *Model, hasSSL bool, trusted func(string) bool) {
	m.DefaultServer.HTTP.RedirectHTTPS = hasSSL
	sslDir := strings.ReplaceAll(m.BaseDomain, ".", "-")
	if sslDir == "" {
		sslDir = "localhost"
	}
	for i := range m.Routes {
		r := &m.Routes[i]
		r.Listen = Listen{HTTP: !hasSSL, HTTPS: hasSSL}
		r.SecurityHeaders = headers(hasSSL)
		if hasSSL {
			chain := false
			if trusted != nil {
				chain = trusted(sslDir)
			}
			r.TLS = &RouteTLS{SSLDir: sslDir, HasTrustedChain: chain, Protocols: []string{"TLSv1.2", "TLSv1.3"}, Ciphers: ptr(ciphers)}
		} else {
			r.TLS = nil
		}
		filtered := make([]Location, 0, len(r.Locations)+1)
		for _, loc := range r.Locations {
			if loc.StaticRoot == nil {
				filtered = append(filtered, loc)
			}
		}
		if !hasSSL {
			filtered = append(filtered, acmeLocation())
		}
		r.Locations = filtered
	}
}
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))
	multiplier := int64(1)
	if len(s) > 0 {
		switch s[len(s)-1] {
		case 'K':
			multiplier = 1024
			s = s[:len(s)-1]
		case 'M':
			multiplier = 1024 * 1024
			s = s[:len(s)-1]
		case 'G':
			multiplier = 1024 * 1024 * 1024
			s = s[:len(s)-1]
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid nginx body size: %w", err)
	}
	return n * multiplier, nil
}

func validateTarget(target string) error {
	for _, c := range "\n\r;{}" {
		if strings.ContainsRune(target, c) {
			return fmt.Errorf("target contains forbidden character %q", c)
		}
	}
	u, err := url.Parse(target)
	if err != nil {
		return fmt.Errorf("target is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("target scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("target is missing host")
	}
	return nil
}

func shadowFile(cfg *config.Config, workdir, serverName string) *string {
	dir, err := nginxtopo.ServedNginxDir(workdir, cfg.Nginx.FrontedBy)
	if err != nil {
		dir, _ = nginxtopo.ServedNginxDir(workdir, "")
	}
	for _, sub := range []string{"conf.d", "conf.d-" + cfg.Env} {
		entries, err := os.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || e.Name() == "default.conf" {
				continue
			}
			path := filepath.Join(dir, sub, e.Name())
			data, err := os.ReadFile(path)
			if err == nil && strings.Contains(string(data), "server_name") && strings.Contains(string(data), serverName) {
				return ptr("nginx/" + filepath.ToSlash(filepath.Join(sub, e.Name())))
			}
		}
	}
	return nil
}

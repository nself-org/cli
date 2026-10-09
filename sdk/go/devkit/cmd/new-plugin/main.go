// Command new-plugin scaffolds a fresh nSelf plugin (Go) from the default
// template. Usage:
//
//	new-plugin --name mywidget --tier pro --bundle nClaw --dest paid/mywidget
//
// It writes plugin.json, Dockerfile, docker-compose.plugin.yml, go.mod,
// cmd/main.go, internal/config/config.go, internal/server/server.go, and a
// smoke test — every file a plugin needs to run against plugin-sdk-go.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"
)

// Params is the variable set available to every template.
type Params struct {
	Name        string // plugin slug, e.g. "mywidget"
	PascalName  string // e.g. "Mywidget"
	EnvPrefix   string // e.g. "MYWIDGET" (upper-cased, dashes → underscores)
	RepoBucket  string // "plugins-pro" or "plugins"
	Tier        string // "free" or "pro"
	Bundle      string // bundle display, e.g. "nClaw" (empty allowed for free)
	Description string
	MinCLI      string
	MinSDK      string
	Category    string
	Port        int
	Year        int
}

var slugRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,40}$`)

func main() {
	var (
		name        = flag.String("name", "", "plugin slug (lowercase-dash)")
		tier        = flag.String("tier", "licensed", "licence: free or licensed")
		bundle      = flag.String("bundle", "", "bundle display name (optional for free)")
		description = flag.String("description", "", "one-line description")
		category    = flag.String("category", "integrations", "plugin category (see F04)")
		minCLI      = flag.String("min-cli", "1.0.9", "minimum nSelf CLI version")
		minSDK      = flag.String("min-sdk", "0.1.0", "minimum plugin-sdk-go version")
		port        = flag.Int("port", 8080, "default listen port")
		dest        = flag.String("dest", "", "destination directory (default ./paid/<name> or ./free/<name>)")
		force       = flag.Bool("force", false, "overwrite files if they already exist")
	)
	flag.Parse()

	if !slugRE.MatchString(*name) {
		fatalf("invalid --name %q (must be lowercase letters/digits/dashes, 2-41 chars)", *name)
	}
	if *tier == "licensed" {
		*tier = "pro"
	}
	if *tier != "free" && *tier != "pro" {
		fatalf("--tier must be 'free' or 'licensed', got %q", *tier)
	}
	if *description == "" {
		*description = fmt.Sprintf("nSelf %s plugin.", *name)
	}

	target := *dest
	bucket := "paid"
	if *tier == "free" {
		bucket = "free"
	}
	if target == "" {
		target = filepath.Join(bucket, *name)
	}
	if err := ensureEmpty(target, *force); err != nil {
		fatalf("%v", err)
	}

	repoBucket := "plugins-pro"
	if *tier == "free" {
		repoBucket = "plugins"
	}

	params := Params{
		Name:        *name,
		PascalName:  toPascal(*name),
		EnvPrefix:   envPrefix(*name),
		RepoBucket:  repoBucket,
		Tier:        *tier,
		Bundle:      *bundle,
		Description: *description,
		Category:    *category,
		MinCLI:      *minCLI,
		MinSDK:      *minSDK,
		Port:        *port,
		Year:        time.Now().Year(),
	}

	for _, f := range files {
		out := filepath.Join(target, f.Path)
		if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
			fatalf("mkdir %s: %v", out, err)
		}
		tpl, err := template.New(f.Path).Parse(f.Body)
		if err != nil {
			fatalf("template %s: %v", f.Path, err)
		}
		fh, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode)
		if err != nil {
			fatalf("open %s: %v", out, err)
		}
		if err := tpl.Execute(fh, params); err != nil {
			_ = fh.Close()
			fatalf("render %s: %v", out, err)
		}
		if err := fh.Close(); err != nil {
			fatalf("close %s: %v", out, err)
		}
	}

	fmt.Printf("scaffolded %s plugin at %s\n", params.Name, target)
	fmt.Println("next steps:")
	fmt.Println("  cd", target)
	fmt.Println("  go mod tidy")
	fmt.Println("  go test ./...")
	fmt.Println("  docker build -t nself/" + params.Name + ":dev .")
}

func ensureEmpty(dir string, force bool) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("stat %s: %w", dir, err)
	}
	if len(entries) > 0 && !force {
		return fmt.Errorf("destination %q is not empty (pass --force to overwrite)", dir)
	}
	return nil
}

func toPascal(s string) string {
	parts := strings.Split(s, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, "")
}

func envPrefix(s string) string {
	return strings.ToUpper(strings.ReplaceAll(s, "-", "_"))
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "new-plugin: "+format+"\n", args...)
	os.Exit(1)
}

// file is a single template entry.
type file struct {
	Path string
	Mode os.FileMode
	Body string
}

var files = []file{
	{Path: "plugin.json", Mode: 0o644, Body: tmplPluginJSON},
	{Path: "go.mod", Mode: 0o644, Body: tmplGoMod},
	{Path: "cmd/main.go", Mode: 0o644, Body: tmplMain},
	{Path: "internal/config/config.go", Mode: 0o644, Body: tmplConfig},
	{Path: "internal/server/server.go", Mode: 0o644, Body: tmplServer},
	{Path: "internal/server/server_test.go", Mode: 0o644, Body: tmplServerTest},
	{Path: "Dockerfile", Mode: 0o644, Body: tmplDockerfile},
	{Path: "docker-compose.plugin.yml", Mode: 0o644, Body: tmplCompose},
	{Path: ".dockerignore", Mode: 0o644, Body: tmplDockerignore},
	{Path: ".air.toml", Mode: 0o644, Body: tmplAirToml},
	{Path: "README.md", Mode: 0o644, Body: tmplReadme},
}

const tmplPluginJSON = `{
  "name": "{{.Name}}",
  "version": "0.1.0",
  "description": "{{.Description}}",
  "author": "nself",
  "license": {{if eq .Tier "pro"}}"Source-Available"{{else}}"MIT"{{end}},
  "isCommercial": {{if eq .Tier "pro"}}true{{else}}false{{end}},
  {{if eq .Tier "pro"}}"licenseType": "pro",
  "requiredEntitlements": ["pro"],
  "requires_license": true,
  {{end}}"homepage": "https://nself.org/plugins",
  "repository": "https://github.com/nself-org/{{.RepoBucket}}",
  "minNselfVersion": "{{.MinCLI}}",
  "minSdkVersion": "{{.MinSDK}}",
  "category": "{{.Category}}",
  {{if .Bundle}}"bundle": "{{.Bundle}}",
  {{end}}"tags": ["{{.Name}}"]
}
`

const tmplGoMod = `module github.com/nself-org/{{.RepoBucket}}/{{.Name}}

go 1.23.0

require github.com/nself-org/cli/sdk/go/v2 v2.0.0
`

const tmplMain = `// Package main is the entrypoint for the {{.Name}} plugin.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nself-org/{{.RepoBucket}}/{{.Name}}/internal/config"
	"github.com/nself-org/{{.RepoBucket}}/{{.Name}}/internal/server"

	"github.com/nself-org/cli/sdk/go/v2/logger"
)

// Version is stamped at build time via -ldflags.
var Version = "0.1.0"

func main() {
	cfg := config.FromEnv()
	log := logger.New(logger.Options{
		Plugin:  "{{.Name}}",
		Version: Version,
		Level:   logger.ParseLevel(cfg.LogLevel),
	})

	srv := server.New(server.Deps{Config: cfg, Logger: log, Version: Version})

	httpSrv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		log.Info("{{.Name}} listening", "addr", cfg.ListenAddr)
		errCh <- httpSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}
	log.Info("{{.Name}} stopped cleanly")
}
`

const tmplConfig = `// Package config loads {{.Name}} config from environment variables.
package config

import (
	"fmt"
	"os"
	"strings"
)

// Config holds runtime config.
type Config struct {
	ListenAddr  string
	LogLevel    string
	DatabaseURL string
}

// FromEnv reads config from env vars with sensible defaults.
func FromEnv() Config {
	return Config{
		ListenAddr:  envOr("{{.EnvPrefix}}_LISTEN_ADDR", ":{{.Port}}"),
		LogLevel:    envOr("LOG_LEVEL", "info"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}
}

// Validate returns an error if required fields are missing.
func (c Config) Validate() error {
	if c.ListenAddr == "" {
		return fmt.Errorf("{{.Name}}: listen address must not be empty")
	}
	return nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
`

const tmplServer = `// Package server wires the HTTP router for the {{.Name}} plugin.
package server

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/nself-org/{{.RepoBucket}}/{{.Name}}/internal/config"

	sdkmetrics "github.com/nself-org/cli/sdk/go/v2/metrics"
	sdkserver "github.com/nself-org/cli/sdk/go/v2/server"
)

// Deps wires runtime dependencies.
type Deps struct {
	Config  config.Config
	Logger  *slog.Logger
	Version string
}

type readyFn func(ctx context.Context) error

func (f readyFn) Ready(ctx context.Context) error { return f(ctx) }

// New returns a ready-to-serve http.Handler.
func New(d Deps) http.Handler {
	return sdkserver.New(sdkserver.Options{
		Plugin:  "{{.Name}}",
		Version: d.Version,
		Ready: readyFn(func(ctx context.Context) error {
			return d.Config.Validate()
		}),
		Routes: func(r chi.Router, m *sdkmetrics.Registry) {
			r.Route("/v1", func(r chi.Router) {
				r.With(m.Middleware("/v1/hello")).Get("/hello", helloHandler(d.Logger))
			})
		},
	})
}

func helloHandler(log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if log != nil {
			log.Info("hello called", "method", r.Method, "path", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(HelloBody))
	}
}

// HelloBody is the canned JSON response for /v1/hello.
const HelloBody = ` + "`{\"plugin\":\"{{.Name}}\",\"hello\":\"world\"}`" + `
`

const tmplServerTest = `package server

import (
	"net/http/httptest"
	"testing"

	"github.com/nself-org/{{.RepoBucket}}/{{.Name}}/internal/config"
)

func TestHelloEndpoint(t *testing.T) {
	h := New(Deps{Config: config.Config{ListenAddr: ":{{.Port}}"}, Version: "test"})

	req := httptest.NewRequest("GET", "/v1/hello", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected json, got %q", got)
	}
}

func TestHealthz(t *testing.T) {
	h := New(Deps{Config: config.Config{ListenAddr: ":{{.Port}}"}, Version: "test"})

	req := httptest.NewRequest("GET", "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != 200 {
		t.Fatalf("expected 200 from /healthz, got %d", rr.Code)
	}
}
`

const tmplDockerfile = `# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
ARG VERSION=0.1.0
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.Version=${VERSION}" -o /out/{{.Name}} ./cmd

FROM gcr.io/distroless/static:nonroot
USER nonroot:nonroot
COPY --from=build /out/{{.Name}} /{{.Name}}
EXPOSE {{.Port}}
ENTRYPOINT ["/{{.Name}}"]
`

const tmplCompose = `# docker-compose.plugin.yml for {{.Name}}
# Merged into the generated stack by ` + "`nself build`" + `. Do not hand-edit.
services:
  {{.Name}}:
    image: nself/{{.Name}}:${ {{.EnvPrefix}}_VERSION:-latest}
    container_name: ${PROJECT_NAME:-nself}_{{.Name}}
    restart: unless-stopped
    environment:
      LOG_LEVEL: ${LOG_LEVEL:-info}
      DATABASE_URL: ${DATABASE_URL}
      {{.EnvPrefix}}_LISTEN_ADDR: ":{{.Port}}"
    ports:
      - "127.0.0.1:{{.Port}}:{{.Port}}"
    networks:
      - nself_net
networks:
  nself_net:
    external: true
`

const tmplDockerignore = `.git
.gitignore
README.md
Dockerfile
docker-compose*.yml
.air.toml
tmp/
*.test
coverage.out
`

const tmplAirToml = `# air.toml — hot-reload for {{.Name}} dev (pair with nself dev)
root = "."
tmp_dir = "tmp"

[build]
  cmd = "go build -o ./tmp/{{.Name}} ./cmd"
  bin = "tmp/{{.Name}}"
  delay = 500
  include_ext = ["go", "yaml", "yml"]
  exclude_dir = ["tmp", "vendor", ".git"]

[log]
  time = true

[color]
  app = "magenta"
`

const tmplReadme = `# {{.PascalName}} Plugin

{{.Description}}

Tier: ` + "`{{.Tier}}`" + `{{if .Bundle}}  ·  Bundle: ` + "`{{.Bundle}}`" + `{{end}}  ·  Category: ` + "`{{.Category}}`" + `

## Local development

` + "```bash" + `
go mod tidy
go test ./...
go run ./cmd        # runs on :{{.Port}}
` + "```" + `

With hot-reload (install [air](https://github.com/air-verse/air)):

` + "```bash" + `
air
` + "```" + `

## Endpoints

- ` + "`GET /healthz`" + ` — liveness
- ` + "`GET /readyz`" + ` — readiness (checks config)
- ` + "`GET /metrics`" + ` — Prometheus metrics (` + "`nself_plugin_*`" + `)
- ` + "`GET /version`" + ` — plugin version
- ` + "`GET /v1/hello`" + ` — starter handler

## Plugin SDK

Uses [plugin-sdk-go](https://github.com/nself-org/cli/tree/main/sdk/go) for metrics,
logging, server boilerplate, and license helpers. Run ` + "`go mod tidy`" + ` to sync.

## License

{{if eq .Tier "pro"}}Source-Available (Licensed plugin in a Bundle). Requires an active nSelf license key.{{else}}MIT.{{end}}
`

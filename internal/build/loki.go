// Package build — loki.go: Programmatic Loki + Promtail YAML rendering for the
// monitoring bundle.
//
// When the monitoring bundle is enabled, `nself build` must emit
// monitoring/loki.yml + monitoring/promtail.yml with a configurable retention
// period (default 30d / 720h), optional S3-backed chunk storage, and per-project
// tenant labels. This file owns the build-pipeline integration — the actual YAML
// templates live in internal/compose/monitoring/loki.go and are reused here.
//
// Behavior:
//   - LokiBuildOptions wraps the monitoring.LokiConfig + monitoring.PromtailConfig
//     pair with build-time overrides (retention, S3 storage, project name).
//   - WriteLokiConfigs renders both files into <workdir>/monitoring/ idempotently
//     (atomic temp-file + rename, 0644 permissions, parent dir 0755).
//   - Re-running the build with the same options yields byte-identical files so
//     downstream Docker volume hashes don't churn.
//   - Returns the count of files written (always 0 or 2 when monitoring on).
package build

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compose/monitoring"
)

// LokiBuildOptions captures the build-time overrides applied on top of
// monitoring.DefaultLokiConfig + DefaultPromtailConfig. Zero-value fields fall
// back to the package defaults.
type LokiBuildOptions struct {
	// ProjectName is attached as a static label on every shipped log line so
	// operators can distinguish multiple projects on one host. Required.
	ProjectName string
	// RetentionPeriod overrides DefaultLokiConfig().RetentionPeriod. Loki
	// duration syntax (e.g. "720h", "30d", "168h"). Empty = use default (30d).
	// Per S9.T10 OPS-LOKI-RETENTION the doctor enforces a >= 720h floor.
	RetentionPeriod string
	// S3Bucket, when non-empty, switches Loki + Promtail chunk storage from
	// the default filesystem driver to s3 (via Cloudflare R2 / AWS S3). The
	// bucket must exist; credentials live in the runtime env. Empty disables.
	S3Bucket string
	// S3Region is the bucket's region (e.g. "auto" for R2, "us-east-1" for AWS).
	// Required when S3Bucket is non-empty.
	S3Region string
	// MultiTenant turns on Loki multi-tenant mode. Each nSelf project becomes
	// a tenant via the X-Scope-OrgID header set on Promtail pushes.
	MultiTenant bool
	// TenantID is the Promtail tenant header. Required when MultiTenant is true.
	TenantID string
}

// validate ensures the option set is internally consistent. Missing project
// name is a hard error; S3 + multi-tenant require their companion fields.
func (o LokiBuildOptions) validate() error {
	if strings.TrimSpace(o.ProjectName) == "" {
		return fmt.Errorf("loki build: ProjectName is required")
	}
	if o.S3Bucket != "" && strings.TrimSpace(o.S3Region) == "" {
		return fmt.Errorf("loki build: S3Region required when S3Bucket is set")
	}
	if o.MultiTenant && strings.TrimSpace(o.TenantID) == "" {
		return fmt.Errorf("loki build: TenantID required when MultiTenant=true")
	}
	return nil
}

// resolveLokiConfig builds the monitoring.LokiConfig that RenderLokiYAML wants,
// applying build-time overrides on top of the package defaults.
func resolveLokiConfig(opts LokiBuildOptions) *monitoring.LokiConfig {
	cfg := monitoring.DefaultLokiConfig()
	if opts.RetentionPeriod != "" {
		cfg.RetentionPeriod = opts.RetentionPeriod
	}
	if opts.MultiTenant {
		cfg.MultiTenantEnabled = true
	}
	return cfg
}

// resolvePromtailConfig builds the monitoring.PromtailConfig with the same
// override chain applied to its surface area (project name + tenant id).
func resolvePromtailConfig(opts LokiBuildOptions) *monitoring.PromtailConfig {
	cfg := monitoring.DefaultPromtailConfig(opts.ProjectName)
	if opts.MultiTenant {
		cfg.TenantID = opts.TenantID
	}
	return cfg
}

// RenderLokiBundle returns the (lokiYAML, promtailYAML) pair for opts. It is
// pure — no filesystem side effects — so unit tests can assert on the rendered
// bytes without touching disk. The orchestrator calls WriteLokiConfigs which
// wraps this in atomic file writes.
func RenderLokiBundle(opts LokiBuildOptions) (lokiYAML []byte, promtailYAML []byte, err error) {
	if err := opts.validate(); err != nil {
		return nil, nil, err
	}

	lokiCfg := resolveLokiConfig(opts)
	loki, err := monitoring.RenderLokiYAML(lokiCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("loki build: render loki.yml: %w", err)
	}

	// Append S3 backend stanza when requested — appended after the base render
	// so the template stays simple. The compactor + ingester reuse the same
	// chunk store so a single block suffices.
	if opts.S3Bucket != "" {
		loki = appendS3Backend(loki, opts.S3Bucket, opts.S3Region)
	}

	prom, err := monitoring.RenderPromtailYAML(resolvePromtailConfig(opts))
	if err != nil {
		return nil, nil, fmt.Errorf("loki build: render promtail.yml: %w", err)
	}

	return loki, prom, nil
}

// appendS3Backend adds an S3 storage block to a rendered loki.yml. It is
// deliberately additive — re-rendering with the same bucket/region produces
// byte-identical output so the build is idempotent.
func appendS3Backend(loki []byte, bucket, region string) []byte {
	var buf bytes.Buffer
	buf.Write(loki)
	if !bytes.HasSuffix(loki, []byte("\n")) {
		buf.WriteByte('\n')
	}
	fmt.Fprintf(&buf, `
# S3-backed chunk storage (appended by nself build — S9.T08).
storage_config:
  aws:
    s3: s3://%s
    region: %s
    s3forcepathstyle: true
`, bucket, region)
	return buf.Bytes()
}

// WriteLokiConfigs renders the Loki + Promtail YAML and writes both into
// <workdir>/monitoring/ idempotently. Returns the number of files written
// (always 2 on success). The temp-file + rename pattern guarantees readers
// (Docker bind mounts) never see a partially-written file.
func WriteLokiConfigs(workdir string, opts LokiBuildOptions) (int, error) {
	return writeLokiConfigsVia(newDiskSink(workdir), workdir, opts)
}

// writeLokiConfigsVia is WriteLokiConfigs writing through sink.
func writeLokiConfigsVia(sink Sink, workdir string, opts LokiBuildOptions) (int, error) {
	loki, prom, err := RenderLokiBundle(opts)
	if err != nil {
		return 0, err
	}

	monDir := filepath.Join(workdir, "monitoring")
	if err := sink.MkdirAll(monDir, 0o755); err != nil {
		return 0, fmt.Errorf("loki build: mkdir %s: %w", monDir, err)
	}

	if err := sink.WriteAtomic(filepath.Join(monDir, "loki.yml"), loki, 0o644); err != nil {
		return 0, err
	}
	if err := sink.WriteAtomic(filepath.Join(monDir, "promtail.yml"), prom, 0o644); err != nil {
		return 1, err
	}
	return 2, nil
}

// atomicWrite writes data to path via a temp file + rename so concurrent
// readers never see truncation. Mode is the final permission bits; the temp
// file inherits restrictive perms (0o600) until rename.
func atomicWrite(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("loki build: create temp %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("loki build: write temp %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("loki build: close temp %s: %w", path, err)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		cleanup()
		return fmt.Errorf("loki build: chmod temp %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("loki build: rename %s -> %s: %w", tmpPath, path, err)
	}
	return nil
}

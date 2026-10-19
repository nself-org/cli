package build

// orchestrator_secrets_scan.go — the env-file scan behind persistGeneratedSecrets.
// Inputs:  workdir and the build Sink (so a plan's pending env bytes are read).
// Outputs: which keys any env file defines, plus a one-line warning when
//          .env.secrets and another file give an owned key different values.
// Constraints: key names and file names only in logs, never values.

import (
	"bufio"
	"bytes"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// secretScanFiles are the fixed env files persistGeneratedSecrets looks at.
// Custom env files (.env.qa, .env.live) are added by secretScanList.
var secretScanFiles = []string{".env", ".env.dev", ".env.staging", ".env.prod", ".env.secrets", ".env.local"}

// secretScanList is secretScanFiles plus every .env.<name> in workdir that the
// loader could cascade for some ENV. config.EnvCascadeOrder owns that rule
// (cascade.go customEnvNameRe), so ask it instead of mirroring the regex: a
// file counts when it appears in the cascade of the env it is named for.
// .env.example and .env.computed are never cascade files.
func secretScanList(workdir string, sink Sink) []string {
	list := slices.Clone(secretScanFiles)
	entries, err := sink.ReadDir(workdir)
	if err != nil {
		return list
	}
	for _, e := range entries {
		n := e.Name()
		env, ok := strings.CutPrefix(n, ".env.")
		if !ok || e.IsDir() || n == ".env.example" || n == ".env.computed" || slices.Contains(list, n) {
			continue
		}
		if slices.Contains(config.EnvCascadeOrder(env, false), n) {
			list = append(list, n)
		}
	}
	return list
}

// persistedSecretKeys are the keys persistGeneratedSecrets owns.
var persistedSecretKeys = []string{
	"PLUGIN_INTERNAL_SECRET", "NOTIFY_INTERNAL_SECRET", "CRON_INTERNAL_SECRET",
	"HASURA_GRAPHQL_JWT_SECRET", "POSTGRES_PASSWORD", "HASURA_GRAPHQL_ADMIN_SECRET", "AUTH_ENCRYPTION_KEY",
}

// scanEnvFiles returns the set of keys defined in any scanned env file (or
// pending in a plan's env overlay, P7-LIVE-05) and warns about shadowing.
// envName is cfg.Env; it picks the cascade order the warning relies on.
func scanEnvFiles(workdir, envName string, sink Sink) map[string]bool {
	onDisk := make(map[string]bool)
	vals := make(map[string]map[string]string) // file name → key → parsed value
	for _, name := range secretScanList(workdir, sink) {
		data, err := envLayerBytes(sink, filepath.Join(workdir, name))
		if err != nil {
			continue
		}
		kv := make(map[string]string)
		sc := bufio.NewScanner(bytes.NewReader(data))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || line[0] == '#' {
				continue
			}
			if idx := strings.IndexByte(line, '='); idx > 0 {
				onDisk[line[:idx]] = true
				kv[line[:idx]] = parseEnvValue(line[idx+1:])
			}
		}
		vals[name] = kv
	}
	warnShadowedSecrets(vals, config.EnvCascadeOrder(envName, config.LegacyOrderActive()))
	return onDisk
}

// warnShadowedSecrets logs once per owned key that .env.secrets defines with a
// value that differs from a file loaded BEFORE it in order (the active
// cascade). Those files lose to .env.secrets, so their value is silently
// ignored. Files after it (.env.local in the canonical order) win over
// .env.secrets and files outside the cascade never load, so neither warns.
func warnShadowedSecrets(vals map[string]map[string]string, order []string) {
	secrets := vals[".env.secrets"]
	at := slices.Index(order, ".env.secrets")
	if at < 0 {
		return
	}
	for _, key := range persistedSecretKeys {
		sv, ok := secrets[key]
		if !ok {
			continue
		}
		for _, name := range order[:at] {
			if ov, has := vals[name][key]; has && ov != sv {
				slog.Warn(".env.secrets overrides a different value for this key; .env.secrets wins because it loads last",
					"key", key, "file", name, "winner", ".env.secrets")
				break
			}
		}
	}
}

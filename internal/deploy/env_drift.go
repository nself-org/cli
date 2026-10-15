package deploy

// Purpose: compare the operator and host .env without changing either file.
// Inputs: local project path and the existing read-only remote transport.
// Outputs: sorted key names only, including keys whose parsed values differ.
// Constraints: never source dotenv, expose values or return transport payloads.
// SPORT: D-0418, host .env remains authoritative.

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/nself-org/cli/internal/config"
)

// EnvDrift contains names only; no parsed values escape the comparison.
type EnvDrift struct {
	LocalOnly []string `json:"local_only"`
	HostOnly  []string `json:"host_only"`
	Changed   []string `json:"changed"`
}

// Different reports whether either .env has a missing or changed key.
func (d EnvDrift) Different() bool {
	return len(d.LocalOnly)+len(d.HostOnly)+len(d.Changed) > 0
}

var envDriftKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// CompareHostEnv reads both files and discards values after comparison. Missing
// files are reported explicitly, rather than confused with an empty .env.
func CompareHostEnv(ctx context.Context, r Remote, localDir, remotePath string) (EnvDrift, error) {
	d := EnvDrift{LocalOnly: []string{}, HostOnly: []string{}, Changed: []string{}}
	if err := ValidateRemotePath(remotePath); err != nil {
		return d, fmt.Errorf("invalid host project path for .env comparison")
	}
	local, err := config.ReadEnvFile(filepath.Join(localDir, ".env"))
	if err != nil {
		return d, fmt.Errorf("local .env unavailable or invalid; comparison skipped")
	}
	raw, err := r.Run(ctx, shScript(fmt.Sprintf("cd %s && test -f .env && cat .env", q(remotePath))))
	if err != nil {
		return d, fmt.Errorf("host .env unavailable; check SSH access, path and file permissions")
	}
	host, err := config.ParseEnvBytes("host .env", raw)
	if err != nil {
		return d, fmt.Errorf("host .env has invalid dotenv syntax; comparison skipped")
	}
	return compareEnvKeys(local, host, d)
}

func compareEnvKeys(local, host map[string]string, d EnvDrift) (EnvDrift, error) {
	for _, values := range []map[string]string{local, host} {
		for key := range values {
			if !envDriftKey.MatchString(key) {
				return d, fmt.Errorf(".env contains a nonstandard key name; comparison skipped")
			}
		}
	}
	for key, value := range local {
		if remote, ok := host[key]; !ok {
			d.LocalOnly = append(d.LocalOnly, key)
		} else if value != remote {
			d.Changed = append(d.Changed, key)
		}
	}
	for key := range host {
		if _, ok := local[key]; !ok {
			d.HostOnly = append(d.HostOnly, key)
		}
	}
	sort.Strings(d.LocalOnly)
	sort.Strings(d.HostOnly)
	sort.Strings(d.Changed)
	return d, nil
}

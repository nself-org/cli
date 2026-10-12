package commands

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// Purpose: Checkpoint host rollback evidence between tar creation and sync.
// Inputs: Authoritative artifact paths, prior deploy paths, and checkpoint seam.
// Outputs: A Remote preserving recovery evidence even if apply never returns.
// Constraints: Database authority stays with the snapshot; no image pinning.
type promoteEvidenceRemote struct {
	deploy.Remote
	rp       string
	changed  []string
	previous []string
	st       *cpromote.State
	persist  func() error
}

func newPromoteEvidenceRemote(r deploy.Remote, rp string, set deploy.ArtifactSet, previous []string, st *cpromote.State, persist func() error) deploy.Remote {
	changed := append(append(append([]string{}, set.Files...), set.EnvFiles...), deploy.ShippedFile)
	var prior []string
	seen := map[string]bool{}
	for _, f := range changed {
		seen[f] = true
	}
	for _, f := range previous {
		if !seen[f] && promoteRecoveryPath(f) {
			changed, prior = append(changed, f), append(prior, f)
			seen[f] = true
		}
	}
	return &promoteEvidenceRemote{Remote: r, rp: rp, changed: changed, previous: prior, st: st, persist: persist}
}

var promoteTarOutput = regexp.MustCompile(`out='([^']+/\.nself/backups/pre-deploy-[0-9]{8}T[0-9]{6}Z-[0-9a-f]{6}\.tar\.gz)'`)

// Purpose: Intercept the known pre-deploy tar operation before any host writes.
// Inputs: Fixed ApplyRemote shell command. Outputs: Saved recovery checkpoint.
// Constraints: A checkpoint error aborts ApplyRemote before prune or rsync.
func (r *promoteEvidenceRemote) Run(ctx context.Context, command string) ([]byte, error) {
	script := strings.TrimPrefix(command, "sh -c ")
	if strings.HasPrefix(script, "'") && strings.HasSuffix(script, "'") {
		script = strings.ReplaceAll(script[1:len(script)-1], `'\''`, "'")
	}
	match := promoteTarOutput.FindStringSubmatch(script)
	if !strings.Contains(script, `tar -hczf "$out"`) || len(match) != 2 {
		return r.Remote.Run(ctx, command)
	}
	if path.Dir(match[1]) != path.Join(r.rp, deploy.BackupDir) {
		return nil, fmt.Errorf("promotion pre-deploy tar is outside the target backup directory")
	}
	prior, err := cpromote.ReadManifestIdentity(ctx, r.Remote, r.rp)
	if err != nil {
		return nil, fmt.Errorf("capturing prior promotion identity: %w", err)
	}
	for _, f := range r.previous {
		quoted := remotesdk.ShellQuote(f)
		script += "; if [ ! -e " + quoted + " ] && [ ! -L " + quoted + " ]; then printf 'PRIOR_ABSENT %s\\n' " + quoted + "; fi"
	}
	out, err := r.Remote.Run(ctx, "sh -c "+remotesdk.ShellQuote(script))
	if err != nil {
		return out, err
	}
	var absent []string
	var returned []string
	for _, line := range strings.Split(string(out), "\n") {
		if f, ok := strings.CutPrefix(line, "PRIOR_ABSENT "); ok {
			absent = append(absent, f)
			continue
		}
		if f, ok := strings.CutPrefix(line, "ABSENT "); ok {
			absent = append(absent, f)
		}
		returned = append(returned, line)
	}
	r.record(match[1], absent, prior)
	if err := r.persist(); err != nil {
		return nil, fmt.Errorf("persisting promotion recovery evidence before sync: %w", err)
	}
	return []byte(strings.Join(returned, "\n")), nil
}

func (r *promoteEvidenceRemote) record(tar string, absent []string, prior cpromote.ManifestIdentity) {
	st, host := r.st, r.Label()
	if st.PreDeployTar == nil {
		st.PreDeployTar = map[string]string{}
	}
	if st.PreDeployChanged == nil {
		st.PreDeployChanged = map[string][]string{}
	}
	if st.PreDeployAbsent == nil {
		st.PreDeployAbsent = map[string][]string{}
	}
	if st.RemotePaths == nil {
		st.RemotePaths = map[string]string{}
	}
	if st.PriorManifestIdentity == nil {
		st.PriorManifestIdentity = map[string]cpromote.ManifestIdentity{}
	}
	if st.Values == nil {
		st.Values = map[string]any{}
	}
	st.PreDeployTar[host], st.PreDeployChanged[host], st.PreDeployAbsent[host] = tar, r.changed, absent
	st.RemotePaths[host], st.PriorManifestIdentity[host] = r.rp, prior
	st.Values["prewrite_evidence"] = true
}

func promoteRecoveryPath(f string) bool {
	if f == "" || f == ".env" || path.IsAbs(f) || path.Clean(f) != f || strings.ContainsAny(f, "\r\n") {
		return false
	}
	for _, part := range strings.Split(f, "/") {
		if part == "." || part == ".." || strings.HasPrefix(part, "conf.d") {
			return false
		}
	}
	return true
}

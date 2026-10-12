package commands

// Purpose: Prove the applied running release before authorizing migrations.
// Inputs: Compared platform digests and the apply's target remotes.
// Outputs: E490 on missing or changed running identities.
// Constraints: Inspect RepoDigests through source capture; never pin compose images.

import (
	"context"
	"fmt"
	"sort"

	cpromote "github.com/nself-org/cli/internal/controlplane/promote"
	"github.com/nself-org/cli/internal/deploy"
	"github.com/nself-org/cli/internal/errs"
	remotesdk "github.com/nself-org/cli/sdk/go/v2/remote"
)

// provePromoteApplied inspects every changed host using its applied compose manifest.
func provePromoteApplied(ctx context.Context, w *promoteWiring, st *cpromote.State) error {
	remotes, _ := st.Values["apply_remotes"].(map[string]deploy.Remote)
	paths, _ := st.Values["apply_paths"].(map[string]string)
	if len(remotes) == 0 {
		remotes = map[string]deploy.Remote{w.targetRemote.Label(): w.targetRemote}
		paths = map[string]string{w.targetRemote.Label(): promotePath(w.targetServer, w.workdir)}
	}
	hosts := make([]string, 0, len(remotes))
	for host := range remotes {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)
	for _, host := range hosts {
		r, root := remotes[host], paths[host]
		if err := deploy.ValidateRemotePath(root); err != nil {
			return err
		}
		run := func(c context.Context, command string) ([]byte, error) {
			return r.Run(c, "cd "+remotesdk.ShellQuote(root)+" && "+command)
		}
		captured, raw, err := cpromote.CaptureSourceManifest(ctx, run, root)
		if err != nil {
			return errs.Wrap("E490", "post-apply identity capture on "+host, err)
		}
		images, err := cpromote.CaptureManifestSource(ctx, captured, raw, nil)
		if err != nil {
			return errs.Wrap("E490", "post-apply identity inspect on "+host, err)
		}
		if err := matchPromoteRunning(st.Digests, images); err != nil {
			return errs.Wrap("E490", "post-apply identity on "+host, err)
		}
	}
	return nil
}

// matchPromoteRunning requires one running identity for each compared service.
func matchPromoteRunning(rows []cpromote.DigestRow, images []cpromote.SourceImage) error {
	if len(rows) == 0 {
		return fmt.Errorf("no compared image identities")
	}
	running := make(map[string]string, len(images))
	for _, image := range images {
		running[image.Service] = image.Digest
	}
	for _, row := range rows {
		if row.Status == "unverifiable" {
			delete(running, row.Service)
			continue
		}
		digest, found := running[row.Service]
		if !found || row.Target == "" || digest != row.Target || row.Source != row.Target {
			return fmt.Errorf("service %s running digest %s differs from compared digest %s", row.Service, digest, row.Target)
		}
		delete(running, row.Service)
	}
	if len(running) > 0 {
		return fmt.Errorf("running release has uncompared services")
	}
	return nil
}

package commands

// Purpose: stream Hasura metadata through the existing db SSH transport.
// Inputs: resolved target and archive stream. Outputs: remote export or apply.
// Constraints: shell-quote every argument; probe version before mutation.
import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/controlplane"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/reconcile"
	"github.com/nself-org/cli/sdk/go/v2/remote"
	"github.com/spf13/cobra"
)

// dbRemoteSSHOptions is the one ssh option list of the db remote transport
// (key, batch mode, no agent forwarding, host-key policy). Every db remote
// call (runRemoteNselfCommand, the metadata stream, the drift probes) starts
// from it, so a change to how the transport authenticates lands in one place.
func dbRemoteSSHOptions(ctx context.Context, rt dbRemoteTarget) ([]string, error) {
	keyPath := rt.KeyPath
	if keyPath == "" {
		keyPath = defaultSSHKeyPath()
	}
	host := strings.TrimPrefix(rt.SSHTarget, "ssh://")
	keyOpts, err := controlplane.HostKeyOptions(ctx, rt.EnvName, rt.ServerName, rt.Tier, host, false)
	if err != nil {
		return nil, err
	}
	opts := []string{
		"-i", keyPath,
		"-o", "BatchMode=yes",
		"-o", "ForwardAgent=no",
	}
	return append(opts, keyOpts...), nil
}

// validSSHDestination reports whether host can go into ssh argv right before
// the remote command: a value starting with "-" would be read as an ssh option
// (-oProxyCommand=...), and whitespace or control bytes would split it.
func validSSHDestination(host string) bool {
	return host != "" && !strings.HasPrefix(host, "-") &&
		strings.IndexFunc(host, func(r rune) bool { return r <= ' ' || r == 0x7f }) < 0
}

var runSSHStream = func(ctx context.Context, argv []string, stdin io.Reader, stdout io.Writer) (string, error) {
	var stderr bytes.Buffer
	c := exec.CommandContext(ctx, "ssh", argv...)
	c.Stdin, c.Stdout, c.Stderr = stdin, stdout, &stderr
	err := c.Run()
	return stderr.String(), err
}

type boundedMetadataWriter struct {
	w         io.Writer
	remaining int64
}

func (b *boundedMetadataWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > b.remaining {
		return 0, fmt.Errorf("remote metadata archive exceeds 80 MiB transport limit")
	}
	n, err := b.w.Write(p)
	b.remaining -= int64(n)
	return n, err
}

func runRemoteNselfStream(ctx context.Context, rt dbRemoteTarget, stdin io.Reader, stdout io.Writer, args ...string) error {
	// The ssh:// spelling is retained for LIVE-12's direct transport fixture.
	// Inventory targets use the canonical HostSpec spelling.
	spec, err := remote.ParseHostSpec(strings.TrimPrefix(rt.SSHTarget, "ssh://"))
	if err != nil {
		return err
	}
	sshArgs, err := dbRemoteSSHOptions(ctx, rt)
	if err != nil {
		return err
	}
	if !rt.AllowVersionDrift {
		if err := checkRemoteVersionDrift(ctx, rt, sshArgs, joinRemoteArgs(args)); err != nil {
			return err
		}
	}
	remote := "cd " + shellQuoteArg(rt.RemotePath) + " && nself " + strings.Join(shellQuoteArgs(args), " ")
	sshArgs = append(sshArgs, spec.SSHArgs()...)
	sshArgs = append(sshArgs, remote)
	out, err := runSSHStream(ctx, sshArgs, stdin, stdout)
	if err != nil {
		lower := strings.ToLower(out)
		if strings.Contains(lower, "unknown flag: --archive") || strings.Contains(lower, "flag provided but not defined: -archive") {
			return fmt.Errorf("remote nself on %s (env=%s) does not support --archive; upgrade the remote CLI before streaming Hasura metadata: %s", rt.SSHTarget, rt.EnvName, strings.TrimSpace(out))
		}
		return wrapRemoteVersionDriftError(rt, args, err, out)
	}
	return nil
}

func runDBHasuraRemoteSync(cmd *cobra.Command, rt dbRemoteTarget) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	status, err := exec.Command("git", "-C", dir, "status", "--porcelain", "--ignored", "--", "hasura/metadata").Output() // --ignored: sync replaces the whole tree, so ignored local files would go too
	if err != nil {
		return fmt.Errorf("checking local metadata changes: %w", err)
	}
	if len(status) != 0 {
		return fmt.Errorf("uncommitted or ignored files in hasura/metadata: %s; commit, stash or move them before remote sync", strings.TrimSpace(string(status)))
	}
	archive, err := remoteMetadataExport(cmd, rt)
	if err != nil {
		return err
	}
	if err := database.Unpack(bytes.NewReader(archive), filepath.Join(dir, "hasura", "metadata")); err != nil {
		return err
	}
	msg, _ := cmd.Flags().GetString("message")
	hash, err := database.CommitMetadata(cmd.Context(), dir, msg)
	if err != nil {
		return fmt.Errorf("syncing metadata: %w", err)
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Metadata exported and committed: %s\n", hash)
	return err
}

func runDBHasuraRemoteApplyRef(cmd *cobra.Command, rt dbRemoteTarget, ref string) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	desired, err := database.ArchiveMetadataRef(cmd.Context(), dir, ref)
	if err != nil {
		return err
	}
	current, err := remoteMetadataExport(cmd, rt)
	if err != nil {
		return err
	}
	plan, err := metadataArchivePlan(rt.EnvName, ref, current, desired)
	if err != nil {
		return err
	}
	if err := reconcile.RenderHuman(cmd.OutOrStdout(), plan); err != nil {
		return err
	}
	if plan.Empty {
		return nil
	}
	yes, _ := cmd.Flags().GetBool("yes")
	if err := reconcile.Confirm(plan, reconcile.ApplyOptions{Yes: yes, Interactive: interactiveConfirm(cmd)}, true, cmd.ErrOrStderr()); err != nil {
		return err
	}
	if err := runRemoteNselfStream(cmd.Context(), rt, bytes.NewReader(desired), cmd.OutOrStdout(), "db", "hasura", "metadata", "apply", "--archive", "-"); err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "Applied Hasura metadata from git ref: %s\n", ref)
	return err
}

func metadataArchivePlan(env, ref string, current, desired []byte) (reconcile.Plan, error) {
	tmp, err := os.MkdirTemp("", "nself-hasura-plan-")
	if err != nil {
		return reconcile.Plan{}, err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	beforeDir, afterDir := filepath.Join(tmp, "before", "metadata"), filepath.Join(tmp, "after", "metadata")
	for _, p := range []string{filepath.Dir(beforeDir), filepath.Dir(afterDir)} {
		if err := os.MkdirAll(p, 0700); err != nil {
			return reconcile.Plan{}, err
		}
	}
	if err := database.Unpack(bytes.NewReader(current), beforeDir); err != nil {
		return reconcile.Plan{}, fmt.Errorf("remote metadata archive: %w", err)
	}
	if err := database.Unpack(bytes.NewReader(desired), afterDir); err != nil {
		return reconcile.Plan{}, fmt.Errorf("ref metadata archive: %w", err)
	}
	before, err := metadataArtifactSet(beforeDir)
	if err != nil {
		return reconcile.Plan{}, err
	}
	after, err := metadataArtifactSet(afterDir)
	if err != nil {
		return reconcile.Plan{}, err
	}
	p := reconcile.Plan{Command: reconcile.CmdDBHasuraApplyRef, Trigger: reconcile.Trigger{Kind: reconcile.TriggerMetadata, Subject: ref}, Env: env, Artifacts: reconcile.Diff(before, after, nil), Containers: reconcile.Containers{Known: true}}
	err = p.Finalize()
	return p, err
}

func metadataArtifactSet(dir string) (reconcile.ArtifactSet, error) {
	files := reconcile.ArtifactSet{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		files["hasura/metadata/"+filepath.ToSlash(rel)] = reconcile.File{Data: data, Perm: 0644}
		return nil
	})
	return files, err
}

// Archive wrappers preserve the old handler exactly when --archive is absent.
func runDBHasuraMetadataExportArchive(cmd *cobra.Command, args []string) error {
	archive, _ := cmd.Flags().GetString("archive")
	if archive == "" {
		return runDBHasuraMetadataExport(cmd, args)
	}
	if archive != "-" {
		return fmt.Errorf("--archive supports only -")
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	return database.ExportMetadataArchive(cmd.Context(), cfg, cmd.OutOrStdout())
}

func runDBHasuraMetadataApplyArchive(cmd *cobra.Command, args []string) error {
	archive, _ := cmd.Flags().GetString("archive")
	if archive == "" {
		return runDBHasuraMetadataApply(cmd, args)
	}
	if archive != "-" {
		return fmt.Errorf("--archive supports only -")
	}
	env, _ := cmd.Flags().GetString("env")
	if env != "" {
		return fmt.Errorf("--archive - cannot be combined with --env")
	}
	cfg, err := loadProjectConfig()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "nself-hasura-apply-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if err := os.MkdirAll(filepath.Join(tmp, "hasura"), 0700); err != nil {
		return err
	}
	if err := database.Unpack(cmd.InOrStdin(), filepath.Join(tmp, "hasura", "metadata")); err != nil {
		return err
	}
	if err := database.ApplyMetadataArchive(cmd.Context(), cfg, filepath.Join(tmp, "hasura", "metadata")); err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), "Hasura metadata applied.")
	return err
}

func remoteMetadataExport(cmd *cobra.Command, rt dbRemoteTarget) ([]byte, error) {
	var data bytes.Buffer
	limit := &boundedMetadataWriter{w: &data, remaining: 80 << 20}
	if err := runRemoteNselfStream(cmd.Context(), rt, nil, limit, "db", "hasura", "metadata", "export", "--archive", "-"); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

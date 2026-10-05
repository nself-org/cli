package backup

// restore_container.go — a throwaway postgres container and the restore into it.
//
// Purpose: one helper for every "restore a dump into a scratch database"
// check: `backup verify --restore-test` and `backup drill --from`.
// Inputs: a ContainerSpec and a dump file (custom-format archive or plain SQL).
// Outputs: a running Container that is removed by Remove, and an error when
// the restore fails.
// Constraints: a drill container gets a random name (nself-drill-<16 hex>) and
// a random password, publishes no port and joins no network, so nothing can
// reach it but docker exec. The password is passed to docker through the
// environment, never argv, and is never logged. Remove refuses any name that
// is not a throwaway or the verify test container, so the live project
// containers (<project>_postgres and the rest) can never be removed here.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/errs"
)

const (
	// DrillImage is the image of the drill container.
	DrillImage      = "postgres:16-alpine"
	throwawayPrefix = "nself-drill-"
	// containerLabel marks every container started here; its value is the run id.
	containerLabel = "org.nself.drill"
	verifySuffix   = "_pg_restore_test"
	pgDumpMagic    = "PGDMP"
)

var throwawayNameRe = regexp.MustCompile(`^nself-drill-[0-9a-f]{16}$`)

// removable reports whether name may be removed by this package: a drill
// throwaway, or the verify test container. Never a live project container.
func removable(name string) bool {
	return throwawayNameRe.MatchString(name) || (strings.HasSuffix(name, verifySuffix) && len(name) > len(verifySuffix))
}

// dockerCmd is the one place this package builds a docker invocation.
func dockerCmd(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", args...)
}

// ContainerSpec describes the container to start. A zero Name selects a
// throwaway drill container (random name, random password, no network).
type ContainerSpec struct {
	Name     string // "" = throwaway; otherwise must end in _pg_restore_test
	Volume   string // named data volume ("" = anonymous, removed with the container)
	User     string
	DB       string
	Password string // "" = random
	Image    string
	Keep     bool          // leave the container for inspection
	Ready    time.Duration // wait for postgres; default 60s
}

// Container is a started postgres container.
type Container struct {
	Name, User, DB, Volume string
	keep                   bool
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StartContainer starts the container and waits until postgres accepts TCP
// connections on its own loopback (the init server listens on the socket only,
// so this cannot return before the final server is up).
func StartContainer(ctx context.Context, spec ContainerSpec) (*Container, error) {
	runID := "verify"
	throwaway := spec.Name == ""
	if throwaway {
		id, err := randHex(8)
		if err != nil {
			return nil, err
		}
		runID, spec.Name = id, throwawayPrefix+id
	}
	if !removable(spec.Name) {
		return nil, fmt.Errorf("refusing to start container %q: not a throwaway name", spec.Name)
	}
	if spec.Password == "" {
		pw, err := randHex(24)
		if err != nil {
			return nil, err
		}
		spec.Password = pw
	}
	if spec.User == "" {
		spec.User = "postgres"
	}
	if spec.DB == "" {
		spec.DB = "nself"
	}
	if spec.Image == "" {
		spec.Image = DrillImage
	}
	if spec.Ready <= 0 {
		spec.Ready = 60 * time.Second
	}
	args := []string{"run", "-d", "--name", spec.Name, "--label", containerLabel + "=" + runID}
	if throwaway {
		args = append(args, "--network", "none")
	}
	if spec.Volume != "" {
		args = append(args, "-v", spec.Volume+":/var/lib/postgresql/data")
	}
	args = append(args, "-e", "POSTGRES_USER="+spec.User, "-e", "POSTGRES_DB="+spec.DB, "-e", "POSTGRES_PASSWORD", spec.Image)
	cmd := dockerCmd(ctx, args...)
	cmd.Env = append(os.Environ(), "POSTGRES_PASSWORD="+spec.Password)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("start test container: %s: %w", strings.TrimSpace(strings.ReplaceAll(string(out), spec.Password, "***")), err)
	}
	c := &Container{Name: spec.Name, User: spec.User, DB: spec.DB, Volume: spec.Volume, keep: spec.Keep}
	// Assert by label that this is the container just started before anything runs in it.
	out, err := dockerCmd(ctx, "inspect", "-f", `{{index .Config.Labels "`+containerLabel+`"}}`, c.Name).Output()
	if err != nil || strings.TrimSpace(string(out)) != runID {
		c.Remove()
		return nil, fmt.Errorf("container %s does not carry this run's label; refusing to use it", c.Name)
	}
	slog.Info("started restore container", "container", c.Name)
	if err := c.waitReady(ctx, spec.Ready); err != nil {
		c.Remove()
		return nil, err
	}
	return c, nil
}

func (c *Container) waitReady(ctx context.Context, d time.Duration) error {
	deadline := time.Now().Add(d)
	for {
		if c.cmd(ctx, nil, "pg_isready", "-h", "127.0.0.1", "-U", c.User).Run() == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%w: postgres container not ready after %s", errs.ErrBackupVerifyFailed, d)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// cmd builds `docker exec -i <container> args...`.
func (c *Container) cmd(ctx context.Context, stdin io.Reader, args ...string) *exec.Cmd {
	cmd := dockerCmd(ctx, append([]string{"exec", "-i", c.Name}, args...)...)
	cmd.Stdin = stdin
	return cmd
}

// Query runs sql through psql (stdin) and returns the tab-separated rows.
func (c *Container) Query(ctx context.Context, sql string) (string, error) {
	var out, stderr bytes.Buffer
	cmd := c.cmd(ctx, strings.NewReader(sql), "psql", "-X", "-A", "-t", "-F", "\t", "-v", "ON_ERROR_STOP=1", "-U", c.User, "-d", c.DB, "-f", "-")
	cmd.Stdout, cmd.Stderr = &out, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql: %v: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out.String(), nil
}

// Remove removes the container and its volumes on every exit path, using a
// fresh context so a cancelled run still cleans up. It refuses a name that is
// not a throwaway.
func (c *Container) Remove() {
	if c == nil {
		return
	}
	if c.keep {
		slog.Info("keeping test container for inspection", "container", c.Name)
		return
	}
	if !removable(c.Name) {
		slog.Error("refusing to remove a container that is not a throwaway", "container", c.Name)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	slog.Info("cleaning up test container", "container", c.Name)
	if out, err := dockerCmd(ctx, "rm", "-f", "-v", c.Name).CombinedOutput(); err != nil {
		slog.Warn("remove test container", "container", c.Name, "error", err, "output", strings.TrimSpace(string(out)))
	}
	if c.Volume != "" {
		_ = dockerCmd(ctx, "volume", "rm", "-f", c.Volume).Run()
	}
}

// RestoreIntoContainer restores file into the container. The format comes from
// the content, never the extension: a file that starts with the PGDMP magic is
// a custom-format archive (pg_restore --no-owner --no-acl, since production
// roles do not exist here), anything else is plain SQL (psql). Stream backups
// are custom-format archives named .sql.
func RestoreIntoContainer(ctx context.Context, c *Container, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open backup file: %w", err)
	}
	defer func() { _ = f.Close() }()
	magic := make([]byte, len(pgDumpMagic))
	n, _ := io.ReadFull(f, magic)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	args := []string{"psql", "-X", "-q", "-U", c.User, "-d", c.DB}
	tool := "psql"
	if string(magic[:n]) == pgDumpMagic {
		args = []string{"pg_restore", "--no-owner", "--no-acl", "-U", c.User, "-d", c.DB}
		tool = "pg_restore"
	}
	var stderr bytes.Buffer
	cmd := c.cmd(ctx, f, args...)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		// pg_restore and psql exit non-zero on warnings too (a missing
		// extension, a role). Only a failure to connect or read the file is
		// fatal here; missing data is caught by the row-count comparison.
		for _, fatal := range []string{"FATAL", "unrecognized archive", "input file", "No such file", "connection to server"} {
			if strings.Contains(msg, fatal) {
				return fmt.Errorf("%w: %s: %s", errs.ErrBackupRestoreFailed, tool, strings.TrimSpace(msg))
			}
		}
		slog.Warn(tool+" completed with warnings", "output", strings.TrimSpace(msg))
	}
	return nil
}

// runRestoreTest spins up a temporary postgres container, restores the backup,
// and runs the full smoke-query catalog plus a sentinel CRUD round-trip to
// verify data integrity. A schema-only restore (empty DB) fails this check.
//
// Flag: --restore-test (confirmed per S41-T11 drift fix).
func runRestoreTest(ctx context.Context, cfg *config.Config, backupFile string, opts VerifyOptions) error {
	// Only the pg_dump (.dump) format produced by `nself backup create` has an
	// automated restore path. A legacy base-backup tar cannot be restored here,
	// so fail with a clear message instead of running smoke queries against an
	// empty database (which would mis-report a "schema-only restore").
	if !strings.HasSuffix(backupFile, ".dump") {
		return fmt.Errorf("%w: %s is not a restorable pg_dump (.dump) backup; recreate it with the default format before running --restore-test",
			errs.ErrBackupVerifyFailed, backupFile)
	}
	pgVersion := cfg.Postgres.Version
	if pgVersion == "" {
		pgVersion = "16-alpine"
	}
	c, err := StartContainer(ctx, ContainerSpec{
		Name:     cfg.ProjectName + verifySuffix,
		Volume:   cfg.ProjectName + "_restore_test_data",
		User:     cfg.Postgres.User,
		DB:       cfg.Postgres.DB,
		Password: cfg.Postgres.Password,
		Image:    "postgres:" + pgVersion,
		Keep:     opts.Keep,
		Ready:    30 * time.Second,
	})
	if err != nil {
		return err
	}
	// Cleanup deferred unconditionally so the sentinel schema is always removed.
	defer c.Remove()

	if err := RestoreIntoContainer(ctx, c, backupFile); err != nil {
		return fmt.Errorf("restore test failed: %w", err)
	}
	return smokeAndSentinel(ctx, c)
}

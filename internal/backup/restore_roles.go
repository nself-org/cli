package backup

// restore_roles.go — create the roles that an archive's RLS policies name.
//
// Purpose: `pg_restore --no-owner --no-acl` still runs CREATE POLICY ... TO
// <role>, which fails when <role> is missing ("role mux_user does not exist",
// P7-PROD-88). Before the restore, read the archive's schema-only SQL, collect
// the TO role lists and create each missing role as NOLOGIN.
// Inputs: the archive (stdin of `pg_restore --schema-only -f -` in the
// container) and the target container.
// Outputs: NOLOGIN roles created in the target cluster, or ErrBackupRestoreFailed.
// Constraints: nothing from the archive runs; only names matching
// policyroles.NamePattern are taken (anything else refuses the restore before
// any SQL runs), and a created role gets no LOGIN, password or privilege.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/nself-org/cli/internal/backup/policyroles"
	"github.com/nself-org/cli/internal/errs"
)

// ensurePolicyRoles reads archive's schema in the container and creates the
// policy roles that do not exist yet. It runs no SQL when a name is refused.
func ensurePolicyRoles(ctx context.Context, c *Container, archive io.Reader) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stderr bytes.Buffer
	cmd := c.cmd(ctx, archive, "pg_restore", "--schema-only", "-f", "-")
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%w: pg_restore --schema-only: %v", errs.ErrBackupRestoreFailed, err)
	}
	roles, perr := policyroles.Collect(stdout)
	if perr != nil {
		cancel()
	}
	_, _ = io.Copy(io.Discard, stdout)
	werr := cmd.Wait()
	if perr != nil {
		return fmt.Errorf("%w: %v", errs.ErrBackupRestoreFailed, perr)
	}
	if text := fatalRestoreText("pg_restore", stderr.String()); text != "" {
		return fmt.Errorf("%w: pg_restore --schema-only: %s", errs.ErrBackupRestoreFailed, text)
	}
	// Any unsuccessful extraction is fatal, whatever stderr says (an OOM kill
	// exits 137 with none): the role list may be partial.
	if werr != nil {
		if ctx.Err() != nil {
			werr = ctx.Err()
		}
		return fmt.Errorf("%w: pg_restore --schema-only: %v: %s", errs.ErrBackupRestoreFailed, werr, strings.TrimSpace(stderr.String()))
	}
	return createMissingRoles(ctx, c, roles)
}

// createMissingRoles creates each role that pg_roles does not list, NOLOGIN
// and nothing else. roles are already validated against policyroles.NamePattern.
func createMissingRoles(ctx context.Context, c *Container, roles []string) error {
	if len(roles) == 0 {
		return nil
	}
	lits := make([]string, len(roles))
	for i, r := range roles {
		lits[i] = "'" + r + "'"
	}
	out, err := c.Query(ctx, "SELECT rolname FROM pg_roles WHERE rolname IN ("+strings.Join(lits, ", ")+");\n")
	if err != nil {
		return fmt.Errorf("%w: list roles: %v", errs.ErrBackupRestoreFailed, err)
	}
	exists := map[string]bool{}
	for _, l := range strings.Fields(out) {
		exists[l] = true
	}
	var sql strings.Builder
	var created []string
	for _, r := range roles {
		if !exists[r] {
			fmt.Fprintf(&sql, "CREATE ROLE %s NOLOGIN;\n", quoteIdent(r))
			created = append(created, r)
		}
	}
	if len(created) == 0 {
		return nil
	}
	if _, err := c.Query(ctx, sql.String()); err != nil {
		return fmt.Errorf("%w: create policy roles: %v", errs.ErrBackupRestoreFailed, err)
	}
	slog.Info("created NOLOGIN roles named by the archive's policies", "roles", created)
	return nil
}

// policyTOCLimit bounds the stream prefix restore-remote buffers for the schema
// read. A custom-format archive carries its whole table of contents (every
// CREATE POLICY) before the first data block, and pg_restore --schema-only
// never reads data, so the prefix is enough and the stream is read once.
var policyTOCLimit int64 = 64 << 20

// ensurePolicyRolesStream reads up to policyTOCLimit bytes of r, creates the
// policy roles from that prefix and returns a reader that yields the whole
// stream (prefix, then the rest) for the load.
func ensurePolicyRolesStream(ctx context.Context, c *Container, r io.Reader) (io.Reader, error) {
	var prefix bytes.Buffer
	n, err := io.Copy(&prefix, io.LimitReader(r, policyTOCLimit))
	if err != nil {
		return nil, fmt.Errorf("%w: read archive: %v", errs.ErrBackupRestoreFailed, err)
	}
	if err := ensurePolicyRoles(ctx, c, bytes.NewReader(prefix.Bytes())); err != nil {
		if n == policyTOCLimit {
			return nil, fmt.Errorf("%w (the archive's table of contents must fit in its first %d MiB)", err, policyTOCLimit>>20)
		}
		return nil, err
	}
	return io.MultiReader(&prefix, r), nil
}

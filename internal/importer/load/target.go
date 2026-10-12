// Package load applies verified bundles to fresh, started targets.
// Purpose: target preconditions. Inputs: loaded config. Outputs: refusal before writes.
// Constraints: Docker reads use the funnel; SQL identifiers are sanitized.
package load

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/database"
	"github.com/nself-org/cli/internal/docker"
	"github.com/nself-org/cli/internal/errs"
	"strings"
)

func SystemTable(schema, name string, storage bool) bool {
	return strings.HasPrefix(name, "np_") || strings.HasPrefix(schema, "np_") || strings.HasPrefix(schema, "pg_") || schema == "information_schema" || schema == "hdb_catalog" || schema == "nself_ops" || schema == "auth" || (storage && schema == "storage")
}
func Started(ctx context.Context, dir string) error {
	compose := docker.NewCompose()
	for _, service := range []string{"postgres", "hasura"} {
		states, err := compose.ComposeServiceStates(ctx, dir, service)
		if err != nil || len(states) == 0 {
			return errs.New("E521", "target must be started with healthy postgres and hasura")
		}
		for _, s := range states {
			if s.State != "running" || s.Health != "healthy" {
				return errs.New("E521", "target must be started with healthy postgres and hasura")
			}
		}
		if err = docker.HealthGate(ctx, docker.ServiceSpec{Name: service, HasHealthcheck: true}, func(context.Context) ([]docker.ContainerState, error) { return states, nil }, nil); err != nil {
			return errs.New("E521", "target health gate failed")
		}
	}
	return nil
}
func Connect(ctx context.Context, cfg *config.Config) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, cfg.HostDatabaseURL())
	if err != nil {
		return nil, errs.New("E521", "target database connection failed")
	}
	return conn, nil
}
func Empty(ctx context.Context, db *pgx.Conn, schemas []string, storage bool) error {
	for _, schema := range schemas {
		if _, err := database.SanitizeIdentifier(schema); err != nil {
			return errs.New("E519", "unsupported target schema identifier")
		}
	}
	rows, err := db.Query(ctx, `SELECT table_schema, table_name FROM information_schema.tables WHERE table_schema = ANY($1) AND table_type='BASE TABLE'`, schemas)
	if err != nil {
		return errs.New("E521", "target table check failed")
	}
	defer rows.Close()
	for rows.Next() {
		var schema, name string
		if rows.Scan(&schema, &name) != nil {
			return errs.New("E521", "target table check failed")
		}
		if !SystemTable(schema, name, storage) {
			return errs.New("E519", "target schema contains user tables")
		}
	}
	if rows.Err() != nil {
		return errs.New("E521", "target table check failed")
	}
	return nil
}

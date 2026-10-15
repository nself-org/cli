package tenant

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
)

// CollectUsageOptions holds flags for usage collection.
type CollectUsageOptions struct {
	TenantID string // empty = all tenants
	Day      string // YYYY-MM-DD, empty = today
}

// CollectUsage runs the daily metering collection: pg catalog row counts,
// active users from auth sessions, MinIO bucket storage, and nginx bandwidth
// via Prometheus query API (when monitoring is enabled).
func CollectUsage(ctx context.Context, cfg *config.Config, opts CollectUsageOptions) error {
	container := cfg.ProjectName + "_postgres"
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}

	day := opts.Day
	if day == "" {
		day = "CURRENT_DATE"
	} else {
		if err := validateDate(opts.Day); err != nil {
			return err
		}
		day = fmt.Sprintf("'%s'::date", sanitize(opts.Day))
	}

	tenantFilter := ""
	if opts.TenantID != "" {
		if err := validateUUID(opts.TenantID); err != nil {
			return err
		}
		tenantFilter = fmt.Sprintf(" AND t.id = '%s'", sanitize(opts.TenantID))
	}

	// Collect rows_stored per tenant via pg catalog estimates.
	// Scoped to n.nspname = 'public' so the catalog scan only touches application
	// tables (np_* prefix) and avoids traversing pg_catalog / information_schema
	// entries that can never carry tenant data. Values are unchanged — all
	// nSelf application tables with a tenant_id column live in the public schema.
	rowsSQL := fmt.Sprintf(`
INSERT INTO nself_ops.usage_daily (tenant_id, day, metric, value)
SELECT t.id, %s, 'rows_stored', COALESCE(counts.total, 0)
FROM public.tenants t
LEFT JOIN LATERAL (
    SELECT sum(c.reltuples::bigint) AS total
    FROM pg_class c
    JOIN pg_namespace n ON n.oid = c.relnamespace
    JOIN information_schema.columns col ON col.table_schema = n.nspname AND col.table_name = c.relname
    WHERE col.column_name = 'tenant_id'
      AND c.relkind = 'r'
      AND n.nspname = 'public'
) counts ON true
WHERE t.status = 'active'%s
ON CONFLICT (tenant_id, day, metric) DO UPDATE SET value = EXCLUDED.value;`, day, tenantFilter)

	if err := runSQL(ctx, container, user, db, rowsSQL); err != nil {
		slog.Warn("failed to collect rows_stored metric", "error", err)
	}

	// Collect active_users per tenant via auth sessions.
	activeUsersSQL := fmt.Sprintf(`
INSERT INTO nself_ops.usage_daily (tenant_id, day, metric, value)
SELECT t.id, %s, 'active_users',
    COALESCE((
        SELECT count(DISTINCT user_id)
        FROM auth.sessions s
        WHERE s.created_at >= CURRENT_DATE
          AND s.created_at < CURRENT_DATE + interval '1 day'
          AND s.user_id IN (
              SELECT id FROM auth.users u WHERE u.tenant_id = t.id
          )
    ), 0)
FROM public.tenants t
WHERE t.status = 'active'%s
ON CONFLICT (tenant_id, day, metric) DO UPDATE SET value = EXCLUDED.value;`, day, tenantFilter)

	if err := runSQL(ctx, container, user, db, activeUsersSQL); err != nil {
		slog.Warn("failed to collect active_users metric", "error", err)
	}

	// Collect storage bytes per tenant bucket from the active object store (D-0683).
	switch compose.CurrentStore(cfg) {
	case "minio-core", "minio-plugin":
		if err := collectMinIOStorage(ctx, cfg, container, user, db, day, tenantFilter); err != nil {
			slog.Warn("failed to collect storage_bytes metric", "error", err)
		}
	case "seaweedfs":
		if err := collectS3Storage(ctx, cfg, container, user, db, day, tenantFilter); err != nil {
			slog.Warn("failed to collect storage_bytes metric", "error", err)
		}
	}

	// Collect nginx bandwidth bytes via Prometheus (when monitoring is enabled).
	if cfg.Monitoring.PrometheusEnabled && cfg.Monitoring.PrometheusPort > 0 {
		if err := collectNginxBandwidth(ctx, cfg, container, user, db, day, tenantFilter); err != nil {
			slog.Warn("failed to collect bandwidth_bytes metric", "error", err)
		}
	}

	slog.Info("usage collection complete")
	return nil
}

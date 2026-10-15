package tenant

// Purpose: per-tenant storage metering for the v1.5 SeaweedFS core store.
// Inputs: config (S3 identity, SEAWEEDFS_PORT), the Postgres container, a day string.
// Outputs: storage_bytes rows in nself_ops.usage_daily, one per active tenant bucket.
// Constraints: SeaweedFS has no MinIO admin API, so bucket size is the sum of listed object
// sizes over the S3 API; a missing bucket counts as 0 bytes (D-0683).

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/objstore"
)

// bucketBytes sums the object sizes of one bucket; a missing bucket is 0 bytes.
func bucketBytes(ctx context.Context, client *objstore.Client, bucket string) (int64, error) {
	objects, err := client.List(ctx, bucket, "")
	if err != nil {
		if e, ok := err.(*objstore.Error); ok && e.IsNotFound() {
			return 0, nil
		}
		return 0, err
	}
	var total int64
	for _, o := range objects {
		total += o.Size
	}
	return total, nil
}

// collectS3Storage records storage_bytes per tenant bucket from the SeaweedFS S3 API.
func collectS3Storage(ctx context.Context, cfg *config.Config, pgContainer, pgUser, pgDB, day, tenantFilter string) error {
	port, err := compose.SeaweedPort()
	if err != nil {
		return err
	}
	client := &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
		AccessKey: cfg.Minio.S3AccessKey, SecretKey: cfg.Minio.S3SecretKey, Region: cfg.Minio.Region}
	slugSQL := fmt.Sprintf("SELECT id, slug FROM public.tenants WHERE status = 'active'%s;", tenantFilter)
	out, err := exec.CommandContext(ctx, "docker", "exec", pgContainer,
		"psql", "-U", pgUser, "-d", pgDB, "-tAc", slugSQL).Output()
	if err != nil {
		return fmt.Errorf("listing tenants for storage metering: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 || parts[0] == "" {
			continue
		}
		if err := validateUUID(parts[0]); err != nil {
			slog.Warn("skipping invalid tenant id in storage metering", "id", parts[0])
			continue
		}
		bucket := parts[1]
		if bucket == "" {
			bucket = parts[0]
		}
		size, err := bucketBytes(ctx, client, bucket)
		if err != nil {
			slog.Warn("object store listing failed", "bucket", bucket, "error", err)
			continue
		}
		if err := upsertUsage(ctx, pgContainer, pgUser, pgDB, parts[0], day, MetricStorageBytes, size); err != nil {
			slog.Warn("failed to upsert storage metric", "bucket", bucket, "error", err)
		}
	}
	return nil
}

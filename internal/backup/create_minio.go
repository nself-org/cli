package backup

// Purpose: back up MinIO buckets without a host mc installation or alias.
// Inputs: project MinIO configuration and backup destination. Outputs: private bucket/object tree.
// Constraints: stage until complete; reject keys that cannot be represented faithfully as local paths.

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/objstore"
)

// Purpose: list the backup types `backup all` runs. Inputs: config. Outputs: full and
// metadata, plus the object type when a store (MinIO or v1.5 SeaweedFS) is active.
func allBackupTypes(cfg *config.Config) []BackupType {
	types := []BackupType{BackupTypeFull, BackupTypeMetadata}
	if compose.CurrentStore(cfg) != "none" {
		types = append(types, BackupTypeMinio)
	}
	return types
}

// Purpose: build the S3 client for the active object store.
// Inputs: config, SEAWEEDFS_PORT. Outputs: client and true; false when no store is active.
// Constraints: MinIO keeps its root credentials; SeaweedFS uses the S3 identity on 127.0.0.1.
func objectStoreClient(cfg *config.Config) (*objstore.Client, bool, error) {
	switch compose.CurrentStore(cfg) {
	case "seaweedfs":
		port, err := compose.SeaweedPort()
		if err != nil {
			return nil, false, err
		}
		return &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
			AccessKey: cfg.Minio.S3AccessKey, SecretKey: cfg.Minio.S3SecretKey, Region: cfg.Minio.Region}, true, nil
	case "minio-core", "minio-plugin":
		port := cfg.Minio.Port
		if port == 0 {
			port = 9000
		}
		return &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port),
			AccessKey: cfg.Minio.RootUser, SecretKey: cfg.Minio.RootPassword, Region: cfg.Minio.Region}, true, nil
	}
	return nil, false, nil
}

func createMinioBackup(ctx context.Context, cfg *config.Config, backupDir, ts, tag string) error {
	client, ok, err := objectStoreClient(cfg)
	if err != nil {
		return err
	}
	if !ok {
		slog.Info("object storage not enabled, skipping object backup")
		return nil
	}
	buckets, err := client.ListBuckets(ctx)
	if err != nil {
		return fmt.Errorf("minio backup: %w", err)
	}
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(backupDir, ".minio-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(stage) }()
	for _, bucket := range buckets {
		if err := mirrorMinioBucket(ctx, client, bucket, stage); err != nil {
			return fmt.Errorf("minio backup bucket %q: %w", bucket, err)
		}
	}
	dest := filepath.Join(backupDir, fmt.Sprintf("minio_%s%s", ts, tag))
	if err := os.Rename(stage, dest); err != nil {
		return fmt.Errorf("publish minio backup: %w", err)
	}
	slog.Info("minio backup created", "path", dest)
	return nil
}

// Purpose: mirror one bucket. Inputs: signed client and private staging tree.
// Outputs: bucket directory. Constraints: listing failures stop publication.
func mirrorMinioBucket(ctx context.Context, client *objstore.Client, bucket, stage string) error {
	objects, err := client.List(ctx, bucket, "")
	if err != nil {
		return err
	}
	dir := filepath.Join(stage, bucket)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, object := range objects {
		if err := downloadMinioObject(ctx, client, root, bucket, object); err != nil {
			return err
		}
	}
	return nil
}

// Purpose: stream one object. Inputs: listed object and rooted destination.
// Outputs: private file or directory marker. Constraints: no path or size normalization.
func downloadMinioObject(ctx context.Context, client *objstore.Client, root *os.Root, bucket string, object objstore.Object) error {
	key := object.Key
	marker := strings.HasSuffix(key, "/") && object.Size == 0
	if marker {
		key = strings.TrimSuffix(key, "/")
	}
	// S3 allows dot segments, empty segments and backslashes. Never normalize
	// these into a different object's filename or allow them outside the root.
	if key == "." || !fs.ValidPath(key) || strings.ContainsAny(key, "\\\x00") || !filepath.IsLocal(key) {
		return fmt.Errorf("object key %q cannot be represented safely in a local backup", key)
	}
	if marker {
		return root.MkdirAll(key, 0700)
	}
	if err := root.MkdirAll(filepath.Dir(key), 0700); err != nil {
		return err
	}
	body, err := client.Get(ctx, bucket, key)
	if err != nil {
		return err
	}
	defer func() { _ = body.Close() }()
	f, err := root.OpenFile(key, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(f, io.LimitReader(body, object.Size+1))
	closeErr := f.Close()
	if copyErr != nil {
		return fmt.Errorf("download object %q: %w", key, copyErr)
	}
	if n != object.Size {
		return fmt.Errorf("object %q size changed: listed %d bytes, downloaded %d", key, object.Size, n)
	}
	return closeErr
}

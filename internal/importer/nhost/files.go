package nhost

// Purpose: read the hasura-storage file table and fetch its objects from the S3 bucket.
// Inputs: source connection, S3 flags. Outputs: bundle storage members and the independent count.
// Constraints: object bytes stream straight into the hashing writer; errors name no key or id.
import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/hasurameta"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
)

type rower interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

// uploadedFilter is the storage.files predicate for rows whose object exists. Older
// hasura-storage has no is_uploaded column, so every row counts there.
func uploadedFilter(ctx context.Context, q rower) (string, error) {
	var has bool
	if q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='storage' AND table_name='files' AND column_name='is_uploaded')`).Scan(&has) != nil {
		return "", errs.New("E517", "source storage lookup failed")
	}
	if has {
		return "is_uploaded", nil
	}
	return "true", nil
}

// uploadedFiles counts storage.files rows whose object exists; 0 when the table is absent.
func uploadedFiles(ctx context.Context, q rower) (int64, error) {
	var present bool
	if q.QueryRow(ctx, `SELECT to_regclass('storage.files') IS NOT NULL`).Scan(&present) != nil {
		return 0, errs.New("E517", "source storage lookup failed")
	}
	if !present {
		return 0, nil
	}
	filter, err := uploadedFilter(ctx, q)
	if err != nil {
		return 0, err
	}
	var n int64
	if q.QueryRow(ctx, "SELECT count(*) FROM storage.files WHERE "+filter).Scan(&n) != nil {
		return 0, errs.New("E517", "source storage lookup failed")
	}
	return n, nil
}

// needStorageFlags refuses a source that holds files when the S3 flags are missing or invalid.
func needStorageFlags(in importer.Input) error {
	for _, f := range []string{"storage-endpoint", "s3-access-key-env", "s3-secret-env", "bucket"} {
		if in.Values[f] == "" {
			return errs.New("E517", "source holds uploaded files: --storage-endpoint, --s3-access-key-env, --s3-secret-env and --bucket are required")
		}
	}
	_, err := s3Client(in)
	return err
}

func s3Client(in importer.Input) (*objstore.Client, error) {
	base, err := hasurameta.Base(in.Values["storage-endpoint"])
	if err != nil || base.Path != "" {
		return nil, errs.New("E517", "source S3 endpoint must be https (or http on loopback) with no path")
	}
	access, err := in.Secret("s3-access-key-env")
	if err != nil {
		return nil, err
	}
	secret, err := in.Secret("s3-secret-env")
	if err != nil {
		return nil, err
	}
	return &objstore.Client{Endpoint: base.String(), AccessKey: access, SecretKey: secret}, nil
}

type fileRow struct {
	id   string
	size *int64
}

// readFiles lists the uploaded files in one repeatable-read transaction, ordered by id.
func readFiles(ctx context.Context, in importer.Input) ([]fileRow, error) {
	db, err := connect(ctx, in)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close(ctx) }()
	tx, err := db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, errs.New("E520", "source storage snapshot failed")
	}
	defer func() { _ = tx.Rollback(ctx) }()
	filter, err := uploadedFilter(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id::text, size::bigint FROM storage.files WHERE "+filter+" ORDER BY id")
	if err != nil {
		return nil, errs.New("E520", "source storage listing failed")
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var r fileRow
		if rows.Scan(&r.id, &r.size) != nil {
			return nil, errs.New("E520", "source storage listing failed")
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, errs.New("E520", "source storage listing failed")
	}
	return out, nil
}

// storageState reads the independent count (own connection, outside the dump) and the file list
// (one repeatable-read transaction). Tests replace it so no database is needed.
var storageState = func(ctx context.Context, in importer.Input) (int64, []fileRow, error) {
	db, err := connect(ctx, in)
	if err != nil {
		return 0, nil, err
	}
	count, err := uploadedFiles(ctx, db)
	_ = db.Close(ctx)
	if err != nil || count == 0 {
		return count, nil, err
	}
	rows, err := readFiles(ctx, in)
	return count, rows, err
}

// addStorage returns the bundle edit that fetches every uploaded file's object into storage/ and
// records the independent count(storage.files).
func addStorage(ctx context.Context, in importer.Input) func(*portable.Writer, *portable.Manifest) error {
	return func(w *portable.Writer, m *portable.Manifest) error {
		count, rows, err := storageState(ctx, in)
		if err != nil || count == 0 {
			return err
		}
		if err = needStorageFlags(in); err != nil {
			return err
		}
		client, _ := s3Client(in)
		bucket := in.Values["bucket"]
		var total int64
		for _, r := range rows {
			member, e := portable.StorageMember(bucket, r.id)
			if e != nil {
				return errs.New("E520", "source object name unsupported")
			}
			body, e := client.Get(ctx, bucket, r.id)
			if e != nil {
				return errs.New("E520", "source object unavailable for a storage.files row")
			}
			f, e := w.WriteFile(member, body)
			_ = body.Close()
			if e != nil {
				return errs.New("E520", "source object copy failed")
			}
			if r.size != nil && *r.size != f.Bytes {
				return errs.New("E520", "source object size differs from storage.files")
			}
			obj, e := portable.NewObject(bucket, r.id, f)
			if e != nil {
				return errs.New("E520", "source object name unsupported")
			}
			m.Storage.Objects = append(m.Storage.Objects, obj)
			total += f.Bytes
		}
		m.Storage.Buckets = append(m.Storage.Buckets, portable.Bucket{Name: bucket, Objects: len(rows), Bytes: total})
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: bucket, Count: count, Method: "independent count(storage.files)"})
		return nil
	}
}

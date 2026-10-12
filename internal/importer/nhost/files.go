package nhost

// Purpose: read the hasura-storage file table and fetch its objects from the S3 bucket.
// Inputs: source connection, S3 flags. Outputs: bundle storage members and the independent count.
// Constraints: object bytes stream straight into the hashing writer; errors name no key or id.
import (
	"context"
	"errors"
	"strings"

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
		return "", errs.New("E517", msgStorageLookup)
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
		return 0, errs.New("E517", msgStorageLookup)
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
		return 0, errs.New("E517", msgStorageLookup)
	}
	return n, nil
}

// needStorageFlags refuses a source that holds files when the S3 flags are missing or invalid.
func needStorageFlags(in importer.Input) error {
	for _, f := range []string{"storage-endpoint", "s3-access-key-env", "s3-secret-env", "bucket"} {
		if in.Values[f] == "" {
			return errs.New("E517", msgStorageFlags)
		}
	}
	_, err := s3Client(in)
	return err
}

func s3Client(in importer.Input) (*objstore.Client, error) {
	base, err := hasurameta.Base(in.Values["storage-endpoint"])
	if err != nil || base.Path != "" {
		return nil, errs.New("E517", msgS3Endpoint)
	}
	access, err := in.Secret("s3-access-key-env")
	if err != nil {
		return nil, err
	}
	secret, err := in.Secret("s3-secret-env")
	if err != nil {
		return nil, err
	}
	return &objstore.Client{Endpoint: base.String(), Region: in.Values["s3-region"], AccessKey: access, SecretKey: secret}, nil
}

// objectKey is the S3 key of a file: its id, under the root folder when hasura-storage has one.
func objectKey(in importer.Input, id string) string {
	if root := strings.Trim(in.Values["s3-root-folder"], "/"); root != "" {
		return root + "/" + id
	}
	return id
}

// getFailure tells a missing object (the row has no file in the bucket) from a store, endpoint,
// region or key problem, which the user fixes differently.
func getFailure(err error) error {
	var apiErr *objstore.Error
	if errors.As(err, &apiErr) && apiErr.IsNotFound() {
		return errs.New("E520", msgObjectMissing)
	}
	return errs.New("E520", msgObjectRead)
}

// probeBucket proves the endpoint, region, keys and bucket with a one-key listing, so --plan
// fails before the dump instead of midway through extraction.
func probeBucket(ctx context.Context, in importer.Input) error {
	client, err := s3Client(in)
	if err != nil {
		return err
	}
	client.PageSize = 1
	if _, err = client.ListPage(ctx, in.Values["bucket"], "", ""); err != nil {
		return errs.New("E517", msgBucket)
	}
	return nil
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
		return nil, errs.New("E520", msgSnapshot)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	filter, err := uploadedFilter(ctx, tx)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, "SELECT id::text, size::bigint FROM storage.files WHERE "+filter+" ORDER BY id")
	if err != nil {
		return nil, errs.New("E520", msgListing)
	}
	defer rows.Close()
	var out []fileRow
	for rows.Next() {
		var r fileRow
		if rows.Scan(&r.id, &r.size) != nil {
			return nil, errs.New("E520", msgListing)
		}
		out = append(out, r)
	}
	if rows.Err() != nil {
		return nil, errs.New("E520", msgListing)
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
		if int64(len(rows)) != count {
			return errs.New("E520", msgStorageChanged)
		}
		bucket := in.Values["bucket"]
		var total int64
		for _, r := range rows {
			member, e := portable.StorageMember(bucket, r.id)
			if e != nil {
				return errs.New("E520", msgObjectName)
			}
			body, e := client.Get(ctx, bucket, objectKey(in, r.id))
			if e != nil {
				return getFailure(e)
			}
			f, e := w.WriteFile(member, body)
			_ = body.Close()
			if e != nil {
				return errs.New("E520", msgObjectCopy)
			}
			if r.size != nil && *r.size != f.Bytes {
				return errs.New("E520", msgObjectSizeDiff)
			}
			obj, e := portable.NewObject(bucket, r.id, f)
			if e != nil {
				return errs.New("E520", msgObjectName)
			}
			m.Storage.Objects = append(m.Storage.Objects, obj)
			total += f.Bytes
		}
		m.Storage.Buckets = append(m.Storage.Buckets, portable.Bucket{Name: bucket, Objects: len(rows), Bytes: total})
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: bucket, Count: count, Method: "independent count(storage.files)"})
		return nil
	}
}

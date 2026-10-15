package supabase

// Purpose: storage objects. The snapshot counts storage.objects per bucket (the independent
// count); after the snapshot commits the REST API supplies the bytes, and the two must agree.
// Inputs: snapshot transaction, storage client. Outputs: storage/ members, manifest buckets,
// objects and source counts. Constraints: bucket names must be valid for the target store.
import (
	"context"
	"io"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// s3Bucket is the object store's bucket rule (internal/objstore).
// maxObjectBytes is the largest single object the target store takes (S3 single PUT limit).
const maxObjectBytes = 5 << 30

var s3Bucket = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func (r *run) catalogCounts(ctx context.Context, tx pgx.Tx, m *portable.Manifest) error {
	fail := errs.New("E520", "source storage lookup failed")
	var has bool
	if tx.QueryRow(ctx, `SELECT to_regclass('storage.objects') IS NOT NULL`).Scan(&has) != nil {
		return fail
	}
	r.catalog = map[string]int64{}
	r.names = map[string]map[string]int64{}
	if !has {
		return nil
	}
	if r.skipStorage {
		return exemptStorage(ctx, tx, m)
	}
	rows, err := tx.Query(ctx, `SELECT bucket_id::text, count(*) FROM storage.objects GROUP BY 1 ORDER BY 1`)
	if err != nil {
		return fail
	}
	defer rows.Close()
	for rows.Next() {
		var bucket string
		var n int64
		if rows.Scan(&bucket, &n) != nil {
			return fail
		}
		if !s3Bucket.MatchString(bucket) {
			return errs.New("E520", "storage bucket name not valid for the target object store; use --skip-storage to exempt all storage objects")
		}
		r.catalog[bucket] = n
		r.total += n
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: bucket, Count: n, Method: "count(*) storage.objects per bucket"})
	}
	if rows.Err() != nil {
		return fail
	}
	rows.Close()
	// The exact key set and sizes of the snapshot: the listing and the bytes must match them.
	names, err := tx.Query(ctx, `SELECT bucket_id::text, name::text, coalesce((metadata->>'size')::bigint, -1) FROM storage.objects`)
	if err != nil {
		return fail
	}
	defer names.Close()
	for names.Next() {
		var bucket, name string
		var size int64
		if names.Scan(&bucket, &name, &size) != nil {
			return fail
		}
		if r.names[bucket] == nil {
			r.names[bucket] = map[string]int64{}
		}
		r.names[bucket][name] = size
	}
	if names.Err() != nil {
		return fail
	}
	if r.total > 0 {
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", Count: r.total, Method: "count(*) storage.objects"})
	}
	return nil
}

// exemptStorage records snapshot identities without requiring valid target bucket names.
// Inputs: snapshot transaction. Outputs: per-object exemptions, no counts or bytes to load.
// Constraints: explicit --skip-storage only; query failures still fail the import.
func exemptStorage(ctx context.Context, tx pgx.Tx, m *portable.Manifest) error {
	fail := errs.New("E520", "source storage lookup failed")
	rows, err := tx.Query(ctx, `SELECT bucket_id::text, name::text FROM storage.objects ORDER BY bucket_id, name`)
	if err != nil {
		return fail
	}
	defer rows.Close()
	for rows.Next() {
		var bucket, key string
		if rows.Scan(&bucket, &key) != nil {
			return fail
		}
		m.Exemptions = append(m.Exemptions, portable.Exemption{Kind: "object", ID: bucket + "/" + key, Reason: "--skip-storage"})
	}
	if rows.Err() != nil {
		return fail
	}
	return nil
}

// finish downloads every object through the API and checks the listing against the catalog.
func (r *run) finish(ctx context.Context, w *portable.Writer, m *portable.Manifest) error {
	if r.total == 0 {
		return nil
	}
	if r.api == nil {
		return errs.New("E520", "source has storage objects; --api-url (or --project-ref) and --service-key-env are required")
	}
	buckets, err := r.api.buckets(ctx)
	if err != nil {
		return err
	}
	sort.Strings(buckets)
	var listed int64
	for _, bucket := range buckets {
		if !s3Bucket.MatchString(bucket) {
			if r.catalog[bucket] > 0 {
				return errs.New("E520", "storage bucket name not valid for the target object store")
			}
			continue
		}
		keys, err := r.api.keys(ctx, bucket, r.catalog[bucket])
		if err != nil {
			return err
		}
		if int64(len(keys)) != r.catalog[bucket] {
			return errs.New("E520", "storage listing differs from the storage catalog")
		}
		for _, key := range keys {
			if _, ok := r.names[bucket][key]; !ok {
				return errs.New("E520", "storage listing differs from the storage catalog")
			}
		}
		b := portable.Bucket{Name: bucket, Objects: len(keys)}
		for _, key := range keys {
			member, err := portable.StorageMember(bucket, key)
			if err != nil {
				return errs.New("E520", "storage object name not valid for the bundle")
			}
			body, err := r.api.get(ctx, bucket, key)
			if err != nil {
				return err
			}
			// Never read past the catalog size (or the single-object limit when it is unknown).
			limit := r.names[bucket][key]
			if limit < 0 {
				limit = maxObjectBytes
			}
			f, err := w.WriteFile(member, io.LimitReader(body, limit+1))
			_ = body.Close()
			if err != nil {
				return err
			}
			if want := r.names[bucket][key]; (want >= 0 && f.Bytes != want) || f.Bytes > maxObjectBytes {
				return errs.New("E520", "storage object does not match the storage catalog")
			}
			o, err := portable.NewObject(bucket, key, f)
			if err != nil {
				return err
			}
			m.Storage.Objects = append(m.Storage.Objects, o)
			b.Bytes += f.Bytes
		}
		listed += int64(len(keys))
		m.Storage.Buckets = append(m.Storage.Buckets, b)
	}
	if listed != r.total {
		return errs.New("E520", "storage listing differs from the storage catalog")
	}
	return nil
}

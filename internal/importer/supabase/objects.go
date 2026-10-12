package supabase

// Purpose: storage objects. The snapshot counts storage.objects per bucket (the independent
// count); after the snapshot commits the REST API supplies the bytes, and the two must agree.
// Inputs: snapshot transaction, storage client. Outputs: storage/ members, manifest buckets,
// objects and source counts. Constraints: bucket names must be valid for the target store.
import (
	"context"
	"regexp"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

// s3Bucket is the object store's bucket rule (internal/objstore).
var s3Bucket = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

func (r *run) catalogCounts(ctx context.Context, tx pgx.Tx, m *portable.Manifest) error {
	fail := errs.New("E520", "source storage lookup failed")
	var has bool
	if tx.QueryRow(ctx, `SELECT to_regclass('storage.objects') IS NOT NULL`).Scan(&has) != nil {
		return fail
	}
	r.catalog = map[string]int64{}
	if !has {
		return nil
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
			return errs.New("E520", "storage bucket name not valid for the target object store")
		}
		r.catalog[bucket] = n
		r.total += n
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: bucket, Count: n, Method: "count(*) storage.objects per bucket"})
	}
	if rows.Err() != nil {
		return fail
	}
	if r.total > 0 {
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", Count: r.total, Method: "count(*) storage.objects"})
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
		keys, err := r.api.keys(ctx, bucket)
		if err != nil {
			return err
		}
		if int64(len(keys)) != r.catalog[bucket] {
			return errs.New("E520", "storage listing differs from the storage catalog")
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
			f, err := w.WriteFile(member, body)
			_ = body.Close()
			if err != nil {
				return err
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

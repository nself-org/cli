package load

// Purpose: refuse missing storage before restore and load verified object streams.
// Inputs: bundle, active S3 client and explicit skip choice.
// Outputs: item-specific exemptions. Constraints: no keys or object content in errors.
import (
	"context"
	"errors"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
	"sort"
)

func StoragePreflight(m portable.Manifest, store *objstore.Client, skip bool) error {
	if len(m.Storage.Objects) > 0 && store == nil && !skip {
		return errs.New("E529", "bundle contains objects but target has no object store")
	}
	return nil
}
func ApplyStorage(ctx context.Context, r *portable.Reader, store *objstore.Client, skip bool) ([]portable.Exemption, error) {
	m := r.Manifest()
	if err := StoragePreflight(m, store, skip); err != nil {
		return nil, err
	}
	exemptions := []portable.Exemption{}
	for _, o := range m.Storage.Objects {
		if skip {
			exemptions = append(exemptions, portable.Exemption{Kind: "object", ID: o.Bucket + "/" + o.Key, Reason: "--skip-storage"})
			continue
		}
		if err := store.EnsureBucket(ctx, o.Bucket); err != nil {
			return nil, errs.New("E521", "object bucket load failed")
		}
		f, e := r.Open(o.Member)
		if e != nil {
			return nil, errs.New("E521", "object bundle member invalid")
		}
		e = store.Put(ctx, o.Bucket, o.Key, f, o.Bytes)
		_ = f.Close()
		if e != nil {
			return nil, errs.New("E521", "object load failed")
		}
	}
	return exemptions, nil
}

// KeysFree refuses a bundle whose objects would overwrite keys the target store already holds
// (D-0723): the object PUT is unconditional, and verify would pass because the stored sha equals
// the source's. It reads only the bucket listings and runs before the first write; the error
// names the bucket and never a key.
func KeysFree(ctx context.Context, m portable.Manifest, store *objstore.Client, skip bool) error {
	if skip || store == nil || len(m.Storage.Objects) == 0 {
		return nil
	}
	want := map[string]map[string]bool{}
	for _, o := range m.Storage.Objects {
		if want[o.Bucket] == nil {
			want[o.Bucket] = map[string]bool{}
		}
		want[o.Bucket][o.Key] = true
	}
	buckets := make([]string, 0, len(want))
	for b := range want {
		buckets = append(buckets, b)
	}
	sort.Strings(buckets)
	for _, bucket := range buckets {
		listed, err := store.List(ctx, bucket, "")
		if err != nil {
			var api *objstore.Error
			if errors.As(err, &api) && api.IsNotFound() {
				continue
			}
			return errs.New("E521", "target object store lookup failed")
		}
		for _, o := range listed {
			if want[bucket][o.Key] {
				return errs.New("E519", "target bucket "+bucket+" already holds a key the bundle would write")
			}
		}
	}
	return nil
}

package load

// Purpose: refuse missing storage before restore and load verified object streams.
// Inputs: bundle, active S3 client and explicit skip choice.
// Outputs: item-specific exemptions. Constraints: no keys or object content in errors.
import (
	"context"
	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
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

package load

// Purpose: refuse missing storage before restore and load verified object streams.
// Inputs: bundle, active S3 client and explicit skip choice.
// Outputs: item-specific exemptions. Constraints: no keys or object content in errors.
import (
	"context"
	"errors"
	"fmt"
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

// ObjectsAbsent refuses, before the first write, a bundle whose object keys already exist in the
// target store (D-0723): the load is an unconditional PUT, so it would replace a target object and
// verify would still pass because the sha equals the source's. Read-only; it names no key.
func ObjectsAbsent(ctx context.Context, m portable.Manifest, store *objstore.Client, skip bool) error {
	if skip || store == nil {
		return nil
	}
	present := 0
	for _, o := range m.Storage.Objects {
		body, err := store.Get(ctx, o.Bucket, o.Key)
		if err == nil {
			_ = body.Close()
			present++
			continue
		}
		var apiErr *objstore.Error
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			continue
		}
		return errs.New("E521", "target object store lookup failed")
	}
	if present > 0 {
		return errs.New("E519", fmt.Sprintf("target object store already holds %d of the bundle's objects; nothing was written", present))
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

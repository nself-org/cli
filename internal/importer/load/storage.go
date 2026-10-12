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
	"strings"
)

func StoragePreflight(m portable.Manifest, store *objstore.Client, skip bool) error {
	if len(m.Storage.Objects) > 0 && store == nil && !skip {
		return errs.New("E529", "bundle contains objects but target has no object store")
	}
	return nil
}

// lister is the part of the object store client that StorageCollisions reads.
type lister interface {
	List(ctx context.Context, bucket, prefix string) ([]objstore.Object, error)
}

// canonicalKey returns the key as the default object store (SeaweedFS) will keep it: empty path
// segments are dropped, so "a//b" and "/a/b" are both "a/b". A "." or ".." segment, or a key with
// no segment at all, has no canonical form.
func canonicalKey(key string) (string, bool) {
	var out []string
	for _, seg := range strings.Split(key, "/") {
		if seg == "" {
			continue
		}
		if seg == "." || seg == ".." {
			return "", false
		}
		out = append(out, seg)
	}
	return strings.Join(out, "/"), len(out) > 0
}

// StorageCollisions refuses, before the first write, a bundle whose objects would replace objects
// the target store already holds (D-0723): the load is an unconditional PUT and verification
// compares against the bundle, so an overwrite would otherwise pass. Keys are compared in the
// store's canonical form, because a PUT to "a//b" replaces "a/b" without listing differently; a key
// with no canonical form, or two bundle keys that share one, are refused. A bucket the target does
// not have yet is empty. Errors name no key.
func StorageCollisions(ctx context.Context, m portable.Manifest, store lister, skip bool) error {
	if skip || len(m.Storage.Objects) == 0 {
		return nil
	}
	want := map[string]map[string]bool{}
	for _, o := range m.Storage.Objects {
		key, ok := canonicalKey(o.Key)
		if !ok {
			return errs.New("E519", "bundle holds an object key the target object store cannot keep as written")
		}
		if want[o.Bucket] == nil {
			want[o.Bucket] = map[string]bool{}
		}
		if want[o.Bucket][key] {
			return errs.New("E519", "bundle holds two object keys that the target object store would store as one")
		}
		want[o.Bucket][key] = true
	}
	for bucket, keys := range want {
		held, err := store.List(ctx, bucket, "")
		var apiErr *objstore.Error
		if errors.As(err, &apiErr) && apiErr.IsNotFound() {
			continue
		}
		if err != nil {
			return errs.New("E521", "target object store check failed")
		}
		for _, o := range held {
			if key, ok := canonicalKey(o.Key); ok && keys[key] {
				return errs.New("E519", "target object store already has an object the bundle would overwrite")
			}
		}
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

package firebase

// Purpose: assemble the portable bundle: scratch-built database members, auth users, storage objects.
// Inputs: settings, the target config and a stage path. Outputs: a finished v1 bundle.
// Constraints: the scratch Postgres bundle is rebuilt into the final writer so users and objects
// join the same manifest; work files live in a private directory removed on return.
import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/importer"
	"github.com/nself-org/cli/internal/importer/scratch"
	"github.com/nself-org/cli/internal/portable"
)

// scratchBuild is replaced in tests so no container is needed.
var scratchBuild = scratch.Build

func fail(msg string) error { return errs.New("E520", msg) }

func (Source) Extract(ctx context.Context, in importer.Input, stage string) (importer.Extracted, error) {
	s, err := resolve(in)
	if err != nil {
		return importer.Extracted{}, err
	}
	if in.Target == nil {
		return importer.Extracted{}, fail("target config required for firebase extraction")
	}
	if err = os.MkdirAll(filepath.Dir(stage), 0o700); err != nil {
		return importer.Extracted{}, fail("snapshot staging unavailable")
	}
	work, err := os.MkdirTemp(filepath.Dir(stage), ".extract-")
	if err != nil {
		return importer.Extracted{}, fail("snapshot staging unavailable")
	}
	defer func() { _ = os.RemoveAll(work) }()
	fs, store, err := s.clients()
	if err != nil {
		return importer.Extracted{}, err
	}
	col := newCollector(fs, work)
	walkErr := col.walk(ctx, "", nil)
	if err = col.close(); walkErr == nil {
		walkErr = err
	}
	if walkErr != nil {
		return importer.Extracted{}, fail("firestore read failed: " + walkErr.Error())
	}
	tables := col.sorted()
	if len(tables) == 0 {
		return importer.Extracted{}, fail("project has no Firestore collections")
	}
	script, copies, notes, err := emit(tables, work)
	if err != nil {
		return importer.Extracted{}, fail(err.Error())
	}
	built, err := scratchBuild(ctx, in.Target, script, copies, filepath.Join(work, "db-bundle"))
	if err != nil {
		return importer.Extracted{}, fail("scratch postgres build failed")
	}
	w, err := portable.NewWriter(stage)
	if err != nil {
		return importer.Extracted{}, fail("bundle creation failed")
	}
	if err = copyDBMembers(filepath.Join(work, "db-bundle"), w); err != nil {
		return importer.Extracted{}, fail("dump member write failed")
	}
	m := portable.Manifest{Producer: portable.Producer{Tool: "nself", Version: "1", Source: "firebase"}, DB: built.DB}
	for _, t := range tables {
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "table", ID: schemaName + "." + t.name, Count: t.counted, Method: "firestore runAggregationQuery COUNT"})
	}
	userExempt, err := addUsers(w, &m, s.AuthExport, work)
	if err != nil {
		return importer.Extracted{}, fail(err.Error())
	}
	m.Exemptions = append(notes, userExempt...)
	if store != nil {
		if err = addObjects(ctx, w, &m, store); err != nil {
			return importer.Extracted{}, fail("storage read failed: " + err.Error())
		}
	}
	if _, err = w.Finish(m); err != nil {
		return importer.Extracted{}, fail("bundle manifest write failed")
	}
	return importer.Extracted{Stage: stage}, nil
}

// copyDBMembers moves db/data.dump and db/schema.sql from the scratch bundle into the final one.
func copyDBMembers(scratchDir string, w *portable.Writer) error {
	r, err := portable.Open(scratchDir)
	if err != nil {
		return err
	}
	for _, name := range []string{"db/data.dump", "db/schema.sql"} {
		rc, err := r.Open(name)
		if err != nil {
			return err
		}
		_, err = w.WriteFile(name, rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// addUsers reads the auth export into a spool file, then writes it as auth/users.jsonl and fills
// the manifest fields. The independent source count is the raw element count of the export minus
// the users deliberately left out (exemptions).
func addUsers(w *portable.Writer, m *portable.Manifest, exportPath, work string) ([]portable.Exemption, error) {
	spool, err := os.OpenFile(filepath.Join(work, "users.jsonl"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, errors.New("auth users spool unavailable")
	}
	a, err := readAuthExport(exportPath, spool)
	if cerr := spool.Close(); err == nil && cerr != nil {
		err = errors.New("auth users write failed")
	}
	if err != nil {
		return nil, err
	}
	m.Auth = portable.Auth{Users: a.Written, HashAlgorithms: a.Algorithms, ResetRequired: a.ResetRequired}
	m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "users", Count: a.Count - int64(len(a.Exemptions)), Method: "auth export JSON element count"})
	notes := a.Exemptions
	if a.Anonymous > 0 {
		notes = append(notes, portable.Exemption{Kind: "metadata", ID: "firebase.auth#anonymous", Reason: fmt.Sprintf("%d anonymous users imported as ordinary accounts without any credential (metadata.firebase.anonymous)", a.Anonymous)})
	}
	if a.Written == 0 {
		return notes, nil
	}
	f, err := os.Open(filepath.Join(work, "users.jsonl"))
	if err != nil {
		return nil, errors.New("auth users spool unreadable")
	}
	defer func() { _ = f.Close() }()
	if _, err = w.WriteFile("auth/users.jsonl", f); err != nil {
		return nil, errors.New("auth users write failed")
	}
	return notes, nil
}

// checkObject compares the streamed bytes with the size and MD5 the bucket listing reported, so a
// truncated or re-encoded download cannot pass as the stored object. A composite object has no MD5.
func checkObject(it objectInfo, written int64, md5sum []byte) error {
	if n, err := strconv.ParseInt(it.Size, 10, 64); it.Size != "" && (err != nil || n != written) {
		return errors.New("object size differs from the bucket listing")
	}
	if it.MD5Hash != "" && it.MD5Hash != base64.StdEncoding.EncodeToString(md5sum) {
		return errors.New("object checksum differs from the bucket listing")
	}
	return nil
}

// addObjects copies every bucket object into the bundle. The first listing drives the copy; a
// second listing with another page size gives the independent count.
func addObjects(ctx context.Context, w *portable.Writer, m *portable.Manifest, store *gcs) error {
	items, err := store.list(ctx, 1000)
	if err != nil {
		return err
	}
	second, err := store.count(ctx, 100)
	if err != nil {
		return err
	}
	bucket := portable.Bucket{Name: store.bucket}
	for _, it := range items {
		rc, err := store.open(ctx, it.Name)
		if err != nil {
			return err
		}
		member, merr := portable.StorageMember(store.bucket, it.Name)
		if merr != nil {
			_ = rc.Close()
			return errors.New("object name unsupported")
		}
		sum := md5.New()
		f, werr := w.WriteFile(member, io.TeeReader(rc, sum))
		_ = rc.Close()
		if werr != nil {
			return errors.New("object write failed")
		}
		if err = checkObject(it, f.Bytes, sum.Sum(nil)); err != nil {
			return err
		}
		o, oerr := portable.NewObject(store.bucket, it.Name, f)
		if oerr != nil {
			return errors.New("object record invalid")
		}
		m.Storage.Objects = append(m.Storage.Objects, o)
		bucket.Objects++
		bucket.Bytes += f.Bytes
	}
	m.Storage.Buckets = append(m.Storage.Buckets, bucket)
	m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: store.bucket, Count: second, Method: "gcs bucket listing (second pass)"})
	return nil
}

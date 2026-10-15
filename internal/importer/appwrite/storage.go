package appwrite

// Purpose: stream Appwrite buckets and files into the bundle's storage members.
// Inputs: the Storage API. Outputs: portable objects and per-bucket source counts.
// Constraints: file bytes stream straight into the bundle writer (no temp copy); bucket names must
// be valid S3 names after lowercasing and a collision is an error, never a silent merge.
import (
	"context"
	"crypto/md5" // Appwrite's file signature is MD5; used for compatibility, not authentication.
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/nself-org/cli/internal/errs"
	"github.com/nself-org/cli/internal/portable"
)

var bucketRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,62}$`)

// bucketName maps a bucket id to a valid object-store bucket name.
func bucketName(id string) (string, error) {
	n := strings.ToLower(strings.NewReplacer("_", "-", ".", "-").Replace(id))
	if len(n) < 3 {
		n += strings.Repeat("0", 3-len(n))
	}
	if strings.HasSuffix(n, "-") {
		n += "0"
	}
	if !bucketRe.MatchString(n) {
		return "", errs.New("E520", "source tables unavailable")
	}
	return n, nil
}

// objectKey is "<file id>/<file name>" with path separators in the name replaced.
func objectKey(id, name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_").Replace(name)
	if name == "" {
		return id
	}
	return id + "/" + name
}

type storageResult struct {
	Storage portable.Storage
	// Totals holds each bucket's files-list `total`, in bucket order.
	Totals []bucketTotal
	// Notes are exemptions for what the import does not carry (permissions, renamed files, empty buckets).
	Notes []portable.Exemption
}

type bucketTotal struct {
	Bucket string
	Total  int64
}

func (c *client) exportStorage(ctx context.Context, w *portable.Writer) (storageResult, error) {
	res := storageResult{Storage: portable.Storage{Buckets: []portable.Bucket{}, Objects: []portable.Object{}}}
	seen := map[string]bool{}
	_, err := c.each(ctx, "/storage/buckets", "buckets", func(raw json.RawMessage) error {
		var b struct {
			ID    string   `json:"$id"`
			Perms []string `json:"$permissions"`
		}
		if json.Unmarshal(raw, &b) != nil {
			return errs.New("E520", "source tables unavailable")
		}
		name, e := bucketName(b.ID)
		if e != nil {
			return e
		}
		if seen[name] {
			return errs.New("E520", "source tables unavailable")
		}
		seen[name] = true
		bucket := portable.Bucket{Name: name}
		var filePerms int64
		total, e := c.each(ctx, "/storage/buckets/"+url.PathEscape(b.ID)+"/files", "files", func(raw json.RawMessage) error {
			var f struct {
				ID        string   `json:"$id"`
				Name      string   `json:"name"`
				Size      int64    `json:"sizeOriginal"`
				Signature string   `json:"signature"`
				Perms     []string `json:"$permissions"`
			}
			if json.Unmarshal(raw, &f) != nil || f.ID == "" {
				return errs.New("E520", "source tables unavailable")
			}
			if len(f.Perms) > 0 {
				filePerms++
			}
			key := objectKey(f.ID, f.Name)
			if strings.ContainsAny(f.Name, `/\`) {
				res.Notes = append(res.Notes, portable.Exemption{Kind: "metadata", ID: "bucket:" + name + "#renamed:" + f.ID, Reason: "file name contains a path separator; stored as " + key})
			}
			member, e := portable.StorageMember(name, key)
			if e != nil {
				return errs.New("E520", "source tables unavailable")
			}
			body, e := c.download(ctx, b.ID, f.ID)
			if e != nil {
				return errs.New("E520", "source tables unavailable")
			}
			signature := md5.New()
			file, e := w.WriteFile(member, io.TeeReader(body, signature))
			_ = body.Close()
			if e != nil {
				return errs.New("E520", "dump member write failed")
			}
			obj, e := portable.NewObject(name, key, file)
			if e != nil || (f.Size > 0 && f.Size != file.Bytes) ||
				(f.Signature != "" && !strings.EqualFold(f.Signature, fmt.Sprintf("%x", signature.Sum(nil)))) {
				return errs.New("E520", "dump member write failed")
			}
			res.Storage.Objects = append(res.Storage.Objects, obj)
			bucket.Objects++
			bucket.Bytes += obj.Bytes
			return nil
		})
		if e != nil {
			return e
		}
		if len(b.Perms) > 0 || filePerms > 0 {
			res.Notes = append(res.Notes, portable.Exemption{Kind: "metadata", ID: "bucket:" + name + "#permissions", Reason: fmt.Sprintf("Appwrite permissions not mapped (bucket: %d, files with permissions: %d)", len(b.Perms), filePerms)})
		}
		if total == 0 {
			res.Notes = append(res.Notes, portable.Exemption{Kind: "metadata", ID: "bucket:" + name + "#empty", Reason: "bucket has no files; the target creates buckets only for objects, so it is not created"})
		}
		res.Storage.Buckets = append(res.Storage.Buckets, bucket)
		res.Totals = append(res.Totals, bucketTotal{name, total})
		return nil
	})
	return res, err
}

// listObjects returns the bucket/key pairs the import will load, without downloading anything.
// Detect fills Detection.Objects with it so --plan runs the object-store preflights of the real run.
func (c *client) listObjects(ctx context.Context) ([]portable.Object, error) {
	out := []portable.Object{}
	_, err := c.each(ctx, "/storage/buckets", "buckets", func(raw json.RawMessage) error {
		var b struct {
			ID string `json:"$id"`
		}
		if json.Unmarshal(raw, &b) != nil {
			return errs.New("E517", "source schema lookup failed")
		}
		name, e := bucketName(b.ID)
		if e != nil {
			return errs.New("E517", "source schema lookup failed")
		}
		_, e = c.each(ctx, "/storage/buckets/"+url.PathEscape(b.ID)+"/files", "files", func(raw json.RawMessage) error {
			var f struct {
				ID   string `json:"$id"`
				Name string `json:"name"`
			}
			if json.Unmarshal(raw, &f) != nil {
				return errs.New("E517", "source schema lookup failed")
			}
			out = append(out, portable.Object{Bucket: name, Key: objectKey(f.ID, f.Name)})
			return nil
		})
		return e
	})
	return out, err
}

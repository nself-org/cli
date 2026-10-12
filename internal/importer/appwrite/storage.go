package appwrite

// Purpose: stream Appwrite buckets and files into the bundle's storage members.
// Inputs: the Storage API. Outputs: portable objects and per-bucket source counts.
// Constraints: file bytes stream straight into the bundle writer (no temp copy); bucket names must
// be valid S3 names after lowercasing and a collision is an error, never a silent merge.
import (
	"context"
	"encoding/json"
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
	if !bucketRe.MatchString(n) {
		return "", errs.New("E520", "unsupported source table")
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
	Counts  []portable.SourceCount
}

func (c *client) exportStorage(ctx context.Context, w *portable.Writer) (storageResult, error) {
	res := storageResult{Storage: portable.Storage{Buckets: []portable.Bucket{}, Objects: []portable.Object{}}}
	seen := map[string]bool{}
	var sum int64
	_, err := c.each(ctx, "/storage/buckets", "buckets", func(raw json.RawMessage) error {
		var b struct {
			ID string `json:"$id"`
		}
		if json.Unmarshal(raw, &b) != nil {
			return errs.New("E520", "source tables unavailable")
		}
		name, e := bucketName(b.ID)
		if e != nil {
			return e
		}
		if seen[name] {
			return errs.New("E520", "unsupported source table")
		}
		seen[name] = true
		bucket := portable.Bucket{Name: name}
		total, e := c.each(ctx, "/storage/buckets/"+url.PathEscape(b.ID)+"/files", "files", func(raw json.RawMessage) error {
			var f struct {
				ID, Name string
			}
			var fields map[string]json.RawMessage
			if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields["$id"], &f.ID) != nil {
				return errs.New("E520", "source tables unavailable")
			}
			_ = json.Unmarshal(fields["name"], &f.Name)
			key := objectKey(f.ID, f.Name)
			member, e := portable.StorageMember(name, key)
			if e != nil {
				return errs.New("E520", "unsupported source table")
			}
			body, e := c.download(ctx, b.ID, f.ID)
			if e != nil {
				return e
			}
			file, e := w.WriteFile(member, body)
			_ = body.Close()
			if e != nil {
				return errs.New("E520", "dump member write failed")
			}
			obj, e := portable.NewObject(name, key, file)
			if e != nil {
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
		res.Storage.Buckets = append(res.Storage.Buckets, bucket)
		if total > 0 {
			res.Counts = append(res.Counts, portable.SourceCount{Kind: "objects", ID: name, Count: total, Method: "files list total"})
		}
		sum += total
		return nil
	})
	if err == nil && len(res.Storage.Buckets) > 0 {
		res.Counts = append(res.Counts, portable.SourceCount{Kind: "objects", Count: sum, Method: "files list total"})
	}
	return res, err
}

package exporter

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/nself-org/cli/internal/compose"
	"github.com/nself-org/cli/internal/config"
	"github.com/nself-org/cli/internal/httptimeout"
	"github.com/nself-org/cli/internal/objstore"
	"github.com/nself-org/cli/internal/portable"
)

type storagePlan struct {
	client  *objstore.Client
	buckets map[string][]objstore.Object
	bytes   uint64
}

// Purpose: enumerate every bucket at the active store's loopback S3 port.
// Inputs: loaded config. Outputs: storage plan and no-store note.
// Constraints: never follow S3_ENDPOINT to a remote store; no writes.
func listStorage(ctx context.Context, cfg *config.Config, skip bool) (storagePlan, string, error) {
	p := storagePlan{buckets: map[string][]objstore.Object{}}
	if skip {
		return p, "Storage omitted by --no-storage", nil
	}
	client, err := storageClient(cfg)
	if err != nil {
		return p, "", err
	}
	if client == nil {
		return p, "No object store enabled", nil
	}
	p.client = client
	names, err := listBuckets(ctx, p.client)
	if err != nil {
		return p, "", err
	}
	for _, name := range names {
		objects, err := p.client.List(ctx, name, "")
		if err != nil {
			return p, "", err
		}
		p.buckets[name] = objects
		for _, o := range objects {
			if o.Size < 0 || uint64(o.Size) > ^uint64(0)-p.bytes {
				return p, "", safeFailure("estimate objects")
			}
			p.bytes += uint64(o.Size)
		}
	}
	return p, "", nil
}

// Purpose: select the active store's loopback S3 connection.
// Inputs: loaded config and store mode. Outputs: client, or nil without a store.
// Constraints: credentials stay in memory; arbitrary remote endpoints ignored.
func storageClient(cfg *config.Config) (*objstore.Client, error) {
	home, _ := os.UserHomeDir()
	plugins := os.Getenv("NSELF_PLUGIN_DIR")
	if plugins == "" {
		plugins = filepath.Join(home, ".nself", "plugins")
	}
	fi, err := os.Stat(filepath.Join(plugins, "minio", "plugin.json"))
	store := compose.ActiveStore(cfg, err == nil && !fi.IsDir())
	if store == "none" {
		return nil, nil
	}
	port, key, secret, region := cfg.Minio.Port, cfg.Minio.RootUser, cfg.Minio.RootPassword, cfg.Minio.Region
	if port == 0 {
		port = 9000
	}
	if store == "seaweedfs" {
		port = 8333
		if raw := os.Getenv("SEAWEEDFS_PORT"); raw != "" {
			port, err = strconv.Atoi(raw)
			if err != nil || port < 1 || port > 65535 {
				return nil, safeFailure("store port")
			}
		}
		key, secret = cfg.Minio.S3AccessKey, cfg.Minio.S3SecretKey
	}
	if key == "" || secret == "" {
		return nil, safeFailure("store credentials")
	}
	if region == "" {
		region = "us-east-1"
	}
	return &objstore.Client{Endpoint: fmt.Sprintf("http://127.0.0.1:%d", port), AccessKey: key, SecretKey: secret, Region: region}, nil
}

// Purpose: S3 ListBuckets (objstore exposes only per-bucket ListObjectsV2).
// Inputs: loopback client. Outputs: sorted bucket names. Constraints: signed,
// bounded HTTP through httptimeout with no redirects or proxy.
func listBuckets(ctx context.Context, c *objstore.Client) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/", nil)
	if err != nil {
		return nil, err
	}
	objstore.Signer{AccessKey: c.AccessKey, SecretKey: c.SecretKey, Region: c.Region, Service: "s3"}.Sign(req, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", time.Now())
	resp, err := httptimeout.NoProxy(time.Minute).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, safeFailure("list buckets")
	}
	var result struct {
		Buckets []struct {
			Name string `xml:"Name"`
		} `xml:"Buckets>Bucket"`
	}
	if err = xml.NewDecoder(io.LimitReader(resp.Body, 64<<20)).Decode(&result); err != nil {
		return nil, err
	}
	names := []string{}
	for _, b := range result.Buckets {
		names = append(names, b.Name)
	}
	sort.Strings(names)
	return names, nil
}

// Purpose: stream objects through portable.Writer. Inputs: enumerated plan.
// Outputs: manifest records. Constraints: size changes fail closed.
func (p storagePlan) write(ctx context.Context, w *portable.Writer, m *portable.Manifest) error {
	m.Storage = portable.Storage{Buckets: []portable.Bucket{}, Objects: []portable.Object{}}
	for name, objects := range p.buckets {
		b := portable.Bucket{Name: name}
		for _, o := range objects {
			member, err := portable.StorageMember(name, o.Key)
			if err != nil {
				return err
			}
			r, err := p.client.Get(ctx, name, o.Key)
			if err != nil {
				return err
			}
			file, err := w.WriteFile(member, io.LimitReader(r, o.Size+1))
			cerr := r.Close()
			if err != nil {
				return err
			}
			if cerr != nil {
				return cerr
			}
			if file.Bytes != o.Size {
				return safeFailure("object size changed")
			}
			m.Storage.Objects = append(m.Storage.Objects, portable.Object{Bucket: name, Key: o.Key, Member: member, SHA256: file.SHA256, Bytes: file.Bytes})
			b.Objects++
			b.Bytes += file.Bytes
		}
		m.Storage.Buckets = append(m.Storage.Buckets, b)
		m.SourceCounts = append(m.SourceCounts, portable.SourceCount{Kind: "objects", ID: name, Count: int64(len(objects)), Method: "S3 ListObjectsV2"})
	}
	return nil
}

//go:build integration

package objstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Containers run on the default Docker bridge and are reached by container IP,
// so the test works from the host on Linux and from a sibling container that
// shares the Docker socket (the verification command). A loopback TCP forwarder
// in the test process gives the second pass over http://127.0.0.1, which is the
// only place the client streams UNSIGNED-PAYLOAD.

const (
	itAccess = "AKIAITACCESS0000001"
	itSecret = "it-secret/with+chars=MustNeverLeak"
)

func docker(t *testing.T, args ...string) string {
	t.Helper()
	out, err := exec.Command("docker", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// store describes one object store container.
type store struct {
	name  string
	image string
	port  int
	start func(t *testing.T, name string)
}

var stores = []store{
	{name: "seaweedfs", image: "chrislusf/seaweedfs", port: 8333, start: func(t *testing.T, name string) {
		cfg := fmt.Sprintf(`{"identities":[{"name":"it","credentials":[{"accessKey":%q,"secretKey":%q}],"actions":["Admin","Read","Write","List","Tagging"]}]}`, itAccess, itSecret)
		f := filepath.Join(t.TempDir(), "s3.json")
		if err := os.WriteFile(f, []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		docker(t, "create", "--name", name, "chrislusf/seaweedfs", "server", "-s3", "-s3.config=/etc/s3.json",
			"-dir=/data", "-master.volumeSizeLimitMB=64", "-volume.max=8")
		docker(t, "cp", f, name+":/etc/s3.json")
		docker(t, "start", name)
	}},
	{name: "pgsty-minio", image: "pgsty/minio", port: 9000, start: func(t *testing.T, name string) {
		docker(t, "run", "-d", "--name", name, "-e", "MINIO_ROOT_USER="+itAccess, "-e", "MINIO_ROOT_PASSWORD="+itSecret,
			"pgsty/minio", "server", "/data")
	}},
}

// forward listens on 127.0.0.1 and relays every connection to target.
func forward(t *testing.T, target string) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = c.Close() }()
				up, err := net.Dial("tcp", target)
				if err != nil {
					return
				}
				defer func() { _ = up.Close() }()
				done := make(chan struct{}, 2)
				go func() { _, _ = io.Copy(up, c); done <- struct{}{} }()
				go func() { _, _ = io.Copy(c, up); done <- struct{}{} }()
				<-done
			}()
		}
	}()
	return ln.Addr().String()
}

func parallel(n int, items []string, fn func(i int, k string) error) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var first error
	sem := make(chan struct{}, n)
	for i, k := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, k string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(i, k); err != nil {
				mu.Lock()
				if first == nil {
					first = err
				}
				mu.Unlock()
			}
		}(i, k)
	}
	wg.Wait()
	return first
}

// TestObjstoreIntegration runs the client against SeaweedFS and pgsty/minio
// (ADR 0028), over a direct (signed payload) and a loopback (unsigned stream)
// endpoint.
func TestObjstoreIntegration(t *testing.T) {
	for _, st := range stores {
		st := st
		t.Run(st.name, func(t *testing.T) {
			name := fmt.Sprintf("a23-it-os-%d-%s", os.Getpid(), st.name)
			_ = exec.Command("docker", "rm", "-f", name).Run()
			st.start(t, name)
			t.Cleanup(func() { _ = exec.Command("docker", "rm", "-f", "-v", name).Run() })
			ip := docker(t, "inspect", "-f", "{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}", name)
			direct := fmt.Sprintf("http://%s:%d", ip, st.port)
			loop := "http://" + forward(t, fmt.Sprintf("%s:%d", ip, st.port))

			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
			defer cancel()
			c := &Client{Endpoint: direct, AccessKey: itAccess, SecretKey: itSecret}
			deadline := time.Now().Add(120 * time.Second)
			for {
				err := c.EnsureBucket(ctx, "ready-probe")
				if err == nil {
					break
				}
				if time.Now().After(deadline) {
					if out, lerr := exec.Command("docker", "logs", "--tail", "30", name).CombinedOutput(); lerr == nil {
						t.Logf("container log tail:\n%s", out)
					}
					t.Fatalf("%s not ready or rejects path-style SigV4: %v", st.name, err)
				}
				time.Sleep(2 * time.Second)
			}

			for mode, endpoint := range map[string]string{"direct-signed-payload": direct, "loopback-unsigned-stream": loop} {
				mode, endpoint := mode, endpoint
				t.Run(mode, func(t *testing.T) { exercise(ctx, t, endpoint, strings.ReplaceAll(mode, "_", "-")) })
			}
		})
	}
}

func exercise(ctx context.Context, t *testing.T, endpoint, mode string) {
	c := &Client{Endpoint: endpoint, AccessKey: itAccess, SecretKey: itSecret}
	bucket := "it-" + strings.NewReplacer("direct-signed-payload", "direct", "loopback-unsigned-stream", "loop").Replace(mode)

	if err := c.EnsureBucket(ctx, bucket); err != nil {
		t.Fatalf("EnsureBucket: %v", err)
	}
	if err := c.EnsureBucket(ctx, bucket); err != nil {
		t.Fatalf("EnsureBucket again: %v", err)
	}

	keys := specialKeys(1100)
	body := func(k string) []byte { return []byte("payload of " + k + strings.Repeat("-", len(k))) }
	err := parallel(8, keys, func(i int, k string) error {
		b := body(k)
		if err := c.Put(ctx, bucket, k, bytes.NewReader(b), int64(len(b))); err != nil {
			return fmt.Errorf("Put(%q): %w", k, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for _, ps := range []int{0, 100} {
		cc := *c
		cc.PageSize = ps
		got, err := cc.List(ctx, bucket, "")
		if err != nil {
			t.Fatalf("List (page size %d): %v", ps, err)
		}
		if len(got) != len(keys) {
			t.Fatalf("List (page size %d): %d objects, want %d", ps, len(got), len(keys))
		}
		seen := map[string]int64{}
		for _, o := range got {
			seen[o.Key] = o.Size
		}
		for _, k := range keys {
			if sz, ok := seen[k]; !ok || sz != int64(len(body(k))) {
				t.Fatalf("List (page size %d): key %q missing or wrong size (%d)", ps, k, sz)
			}
		}
	}
	first, err := c.ListPage(ctx, bucket, "", "")
	if err != nil || first.NextToken == "" {
		t.Fatalf("1,100 keys must paginate at the default page size: %d objects, token %q, err %v", len(first.Objects), first.NextToken, err)
	}
	for prefix, want := range map[string]int{"plus+sign/": 220, "dir one/": 220, "pct%20/": 220, "a=b&c=d/": 220} {
		got, err := c.List(ctx, bucket, prefix)
		if err != nil || len(got) != want {
			t.Errorf("List prefix %q: %d objects (err %v), want %d", prefix, len(got), err, want)
		}
	}

	err = parallel(8, keys, func(i int, k string) error {
		rc, err := c.Get(ctx, bucket, k)
		if err != nil {
			return fmt.Errorf("Get(%q): %w", k, err)
		}
		defer func() { _ = rc.Close() }()
		b, err := io.ReadAll(rc)
		if err != nil || sha256.Sum256(b) != sha256.Sum256(body(k)) {
			return fmt.Errorf("Get(%q): sha256 differs (err %v)", k, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A larger body, and a zero-length one.
	big := bytes.Repeat([]byte("0123456789abcdef"), 1<<19) // 8 MiB
	if err := c.Put(ctx, bucket, "big/blob.bin", bytes.NewReader(big), int64(len(big))); err != nil {
		t.Fatalf("Put 8 MiB: %v", err)
	}
	rc, err := c.Get(ctx, bucket, "big/blob.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if sha256.Sum256(got) != sha256.Sum256(big) {
		t.Error("8 MiB round trip: sha256 differs")
	}
	if err := c.Put(ctx, bucket, "zero", nil, 0); err != nil {
		t.Fatalf("Put empty: %v", err)
	}

	// Errors: missing key; a wrong secret never shows up in the error.
	_, err = c.Get(ctx, bucket, "no/such/key")
	var ae *Error
	if !errors.As(err, &ae) || !ae.IsNotFound() {
		t.Errorf("Get missing key: want a 404 *Error, got %v", err)
	}
	wrong := &Client{Endpoint: endpoint, AccessKey: itAccess, SecretKey: "WrongSecretValue/123+abc"}
	for name, run := range map[string]func() error{
		"ensure": func() error { return wrong.EnsureBucket(ctx, bucket) },
		"put":    func() error { return wrong.Put(ctx, bucket, "k", strings.NewReader("x"), 1) },
		"list":   func() error { _, err := wrong.List(ctx, bucket, ""); return err },
		"get":    func() error { _, err := wrong.Get(ctx, bucket, keys[0]); return err },
	} {
		err := run()
		if err == nil {
			t.Errorf("%s with a wrong secret must fail", name)
			continue
		}
		for _, leak := range []string{"WrongSecretValue", itSecret} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error leaks a secret: %v", name, err)
			}
		}
	}
}

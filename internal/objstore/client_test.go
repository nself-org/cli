package objstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// specialKeys is n keys that exercise space, unicode, '+', '%', '=' and '&'.
func specialKeys(n int) []string {
	var keys []string
	for i := 0; i < n; i++ {
		switch i % 5 {
		case 0:
			keys = append(keys, fmt.Sprintf("dir one/файл %04d.txt", i))
		case 1:
			keys = append(keys, fmt.Sprintf("plus+sign/k+%04d", i))
		case 2:
			keys = append(keys, fmt.Sprintf("pct%%20/100%%-%04d", i))
		case 3:
			keys = append(keys, fmt.Sprintf("a=b&c=d/%04d~x", i))
		default:
			keys = append(keys, fmt.Sprintf("plain/%04d", i))
		}
	}
	return keys
}

func testClient(srv *httptest.Server) *Client {
	c := &Client{Endpoint: srv.URL, AccessKey: fakeAccess, SecretKey: fakeSecret}
	if strings.HasPrefix(srv.URL, "https:") {
		c.HTTP = srv.Client()
	}
	return c
}

func TestClientAgainstFakeS3(t *testing.T) {
	for _, tls := range []bool{false, true} {
		name := map[bool]string{false: "http-loopback-unsigned-stream", true: "https-signed-payload"}[tls]
		t.Run(name, func(t *testing.T) {
			fake := newFakeS3()
			srv := fake.server(tls)
			defer srv.Close()
			c := testClient(srv)
			ctx := context.Background()

			if err := c.EnsureBucket(ctx, "data-bkt"); err != nil {
				t.Fatal(err)
			}
			if err := c.EnsureBucket(ctx, "data-bkt"); err != nil {
				t.Fatalf("second EnsureBucket: %v", err)
			}
			keys := specialKeys(1100)
			want := map[string][32]byte{}
			for i, k := range keys {
				body := []byte(fmt.Sprintf("body-%d-%s", i, k))
				want[k] = sha256.Sum256(body)
				var rd io.Reader = bytes.NewReader(body)
				if i%2 == 1 {
					rd = io.MultiReader(bytes.NewReader(body)) // not seekable: spool path
				}
				if err := c.Put(ctx, "data-bkt", k, rd, int64(len(body))); err != nil {
					t.Fatalf("Put(%q): %v", k, err)
				}
			}
			if err := c.Put(ctx, "data-bkt", "empty", nil, 0); err != nil {
				t.Fatalf("empty Put: %v", err)
			}
			if tls && (fake.unsigned != 0 || fake.signed == 0) || !tls && (fake.signed != 0 && fake.unsigned == 0) {
				t.Errorf("payload mode: unsigned=%d signed=%d", fake.unsigned, fake.signed)
			}
			if !tls && fake.signed > 1 { // only the empty body hashes (emptySHA256)
				t.Errorf("loopback http must stream unsigned, got %d signed puts", fake.signed)
			}

			for _, ps := range []int{0, 100, 7} {
				c.PageSize = ps
				got, err := c.List(ctx, "data-bkt", "")
				if err != nil {
					t.Fatalf("List page size %d: %v", ps, err)
				}
				if len(got) != len(keys)+1 {
					t.Fatalf("List page size %d: %d objects, want %d", ps, len(got), len(keys)+1)
				}
			}
			c.PageSize = 0
			pre, err := c.List(ctx, "data-bkt", "plus+sign/")
			if err != nil || len(pre) != 220 {
				t.Fatalf("prefix list with '+': %d objects, err %v", len(pre), err)
			}
			first, err := c.ListPage(ctx, "data-bkt", "", "")
			if err != nil || first.NextToken == "" || len(first.Objects) != 1000 {
				t.Fatalf("first page: %d objects, token %q, err %v", len(first.Objects), first.NextToken, err)
			}

			for _, k := range keys {
				rc, err := c.Get(ctx, "data-bkt", k)
				if err != nil {
					t.Fatalf("Get(%q): %v", k, err)
				}
				b, _ := io.ReadAll(rc)
				_ = rc.Close()
				if sha256.Sum256(b) != want[k] {
					t.Fatalf("Get(%q): sha256 differs", k)
				}
			}
			// Wire encoding: the raw request path is the SigV4 encoding.
			var sawSpace, sawPlus, sawPct, sawUnicode bool
			for _, p := range fake.rawPaths {
				sawSpace = sawSpace || strings.Contains(p, "dir%20one")
				sawPlus = sawPlus || strings.Contains(p, "plus%2Bsign")
				sawPct = sawPct || strings.Contains(p, "pct%2520")
				sawUnicode = sawUnicode || strings.Contains(p, "%D1%84%D0%B0%D0%B9%D0%BB")
			}
			if !(sawSpace && sawPlus && sawPct && sawUnicode) {
				t.Errorf("wire paths not SigV4-encoded: space=%v plus=%v pct=%v unicode=%v", sawSpace, sawPlus, sawPct, sawUnicode)
			}
		})
	}
}

func TestClientErrorsRedactSecrets(t *testing.T) {
	fake := newFakeS3()
	srv := fake.server(false)
	defer srv.Close()
	ctx := context.Background()

	bad := &Client{Endpoint: srv.URL, AccessKey: fakeAccess, SecretKey: "wrong-secret-VALUE"}
	for name, run := range map[string]func() error{
		"ensure": func() error { return bad.EnsureBucket(ctx, "bkt-one") },
		"put":    func() error { return bad.Put(ctx, "bkt-one", "k", strings.NewReader("x"), 1) },
		"list":   func() error { _, err := bad.List(ctx, "bkt-one", ""); return err },
		"get":    func() error { _, err := bad.Get(ctx, "bkt-one", "k"); return err },
	} {
		err := run()
		if err == nil {
			t.Fatalf("%s with a wrong secret must fail", name)
		}
		for _, leak := range []string{"wrong-secret-VALUE", fakeSecret} {
			if strings.Contains(err.Error(), leak) {
				t.Errorf("%s: error leaks a secret: %v", name, err)
			}
		}
	}
	// A server that echoes the client's secret in its message: redacted.
	echo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprintf(w, "<Error><Code>AccessDenied</Code><Message>bad key %s (sig Signature=%s)</Message></Error>",
			fakeSecret, strings.Repeat("ab", 32))
	}))
	defer echo.Close()
	good := &Client{Endpoint: echo.URL, AccessKey: fakeAccess, SecretKey: fakeSecret}
	_, err := good.List(ctx, "bkt-one", "")
	var ae *Error
	if !errors.As(err, &ae) || ae.Status != 403 || ae.Code != "AccessDenied" {
		t.Fatalf("want 403 AccessDenied, got %v", err)
	}
	if strings.Contains(err.Error(), fakeSecret) || strings.Contains(err.Error(), "abababab") || !strings.Contains(err.Error(), "[redacted]") {
		t.Errorf("server-echoed secret not redacted: %v", err)
	}

	// Transport error: nothing listens; the secret still never shows.
	dead := &Client{Endpoint: "http://127.0.0.1:1", AccessKey: fakeAccess, SecretKey: "dead-secret-VALUE"}
	err = dead.EnsureBucket(ctx, "bkt-one")
	if err == nil || strings.Contains(err.Error(), "dead-secret-VALUE") {
		t.Errorf("transport error: %v", err)
	}
}

func TestRedactForms(t *testing.T) {
	c := &Client{SecretKey: "a/b+c=d%e"}
	in := "x a/b+c=d%e y a%2Fb%2Bc%3Dd%25e z Signature=0123456789abcdef0123456789abcdef"
	out := c.redact(in)
	for _, leak := range []string{"a/b+c=d%e", "a%2Fb%2Bc%3Dd%25e", "0123456789abcdef"} {
		if strings.Contains(out, leak) {
			t.Errorf("redact left %q in %q", leak, out)
		}
	}
}

func TestEndpointAndInputValidation(t *testing.T) {
	ctx := context.Background()
	for _, ep := range []string{"", "ftp://h", "http://", "http://u:p@h:9000", "http://h:9000/base", "http://h:9000?x=1", "h:9000"} {
		c := &Client{Endpoint: ep, AccessKey: "a", SecretKey: "s"}
		if err := c.EnsureBucket(ctx, "bkt-one"); err == nil {
			t.Errorf("endpoint %q must be refused", ep)
		}
	}
	c := &Client{Endpoint: "http://127.0.0.1:1", AccessKey: "a", SecretKey: "s"}
	for _, b := range []string{"", "A", "ab", "UPPER", "a_b", "../x", "a..b", "-ab", strings.Repeat("a", 64)} {
		if err := c.EnsureBucket(ctx, b); err == nil || strings.Contains(err.Error(), "127.0.0.1") {
			t.Errorf("bucket %q must be refused before any request: %v", b, err)
		}
	}
	if err := c.Put(ctx, "bkt-one", "", strings.NewReader("x"), 1); err == nil {
		t.Error("empty key must be refused")
	}
	if err := c.Put(ctx, "bkt-one", "k", strings.NewReader("x"), -1); err == nil {
		t.Error("negative size must be refused")
	}
	if (&Client{Endpoint: "http://127.0.0.1:1"}).EnsureBucket(ctx, "bkt-one") == nil {
		t.Error("missing credentials must be refused")
	}
}

func TestIsLoopbackHTTP(t *testing.T) {
	for u, want := range map[string]bool{
		"http://127.0.0.1:9000": true, "http://localhost:8333": true, "http://[::1]:9000": true, "http://127.5.5.5": true,
		"https://127.0.0.1:9000": false, "http://10.0.0.5:9000": false, "http://minio:9000": false, "http://example.com": false,
		"http://127.0.0.1.evil.com": false,
	} {
		c := &Client{Endpoint: u, AccessKey: "a", SecretKey: "s"}
		pu, err := c.endpoint()
		if err != nil {
			t.Fatal(err)
		}
		if got := isLoopbackHTTP(pu); got != want {
			t.Errorf("isLoopbackHTTP(%q) = %v, want %v", u, got, want)
		}
	}
}

func TestPutBodyLengthMismatch(t *testing.T) {
	// Over https the payload is hashed first: a wrong size never reaches the
	// store. Over loopback http the body streams, so a wrong size is detected
	// while sending and Put returns an error (the server may hold a partial
	// object; the caller owns cleanup).
	for _, tls := range []bool{true, false} {
		fake := newFakeS3()
		srv := fake.server(tls)
		c := testClient(srv)
		ctx := context.Background()
		_ = c.EnsureBucket(ctx, "bkt-one")
		if err := c.Put(ctx, "bkt-one", "k", strings.NewReader("12345"), 3); err == nil {
			t.Errorf("tls=%v: body longer than size must fail", tls)
		}
		if err := c.Put(ctx, "bkt-one", "k2", strings.NewReader("12"), 5); err == nil {
			t.Errorf("tls=%v: body shorter than size must fail", tls)
		}
		srv.Close() // waits for the handlers, so the reads below do not race
		fake.mu.Lock()
		if _, short := fake.buckets["bkt-one"]["k2"]; short {
			t.Errorf("tls=%v: a short body must not be stored", tls)
		}
		if tls && len(fake.buckets["bkt-one"]) != 0 {
			t.Error("a mismatched signed body must not be stored")
		}
		fake.mu.Unlock()
	}
}

// repeatingList serves a truncated page with the same token forever.
func TestListStopsOnRepeatedToken(t *testing.T) {
	srv := httptest.NewServer(httpHandler(func() string {
		return `<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>same</NextContinuationToken></ListBucketResult>`
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s"}
	if _, err := c.List(context.Background(), "bkt-one", ""); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("want repeated-token error, got %v", err)
	}
	srv2 := httptest.NewServer(httpHandler(func() string {
		return `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`
	}))
	defer srv2.Close()
	c.Endpoint = srv2.URL
	if _, err := c.List(context.Background(), "bkt-one", ""); err == nil {
		t.Fatal("truncated without token must fail")
	}
}

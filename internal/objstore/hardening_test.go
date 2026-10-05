package objstore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

// TestListKeysEncodedOnTheWire: List asks for encoding-type=url, so keys with
// characters XML cannot carry still list, and encoded keys decode exactly.
func TestListKeysEncodedOnTheWire(t *testing.T) {
	fake := newFakeS3()
	srv := fake.server(false)
	defer srv.Close()
	c := testClient(srv)
	ctx := context.Background()
	if err := c.EnsureBucket(ctx, "enc-bkt"); err != nil {
		t.Fatal(err)
	}
	keys := []string{"ctl\x01char", "sp ace+plus%25", "tab\tkey", "ключ/файл", "a=b&c"}
	for _, k := range keys {
		if err := c.Put(ctx, "enc-bkt", k, strings.NewReader("v"), 1); err != nil {
			t.Fatalf("put %q: %v", k, err)
		}
	}
	got, err := c.List(ctx, "enc-bkt", "")
	if err != nil {
		t.Fatalf("List with a U+0001 key: %v", err)
	}
	have := map[string]bool{}
	for _, o := range got {
		have[o.Key] = true
	}
	for _, k := range keys {
		if !have[k] {
			t.Errorf("key %q missing or mangled; listed %v", k, got)
		}
	}
	sawEncoding := false
	for _, p := range fake.rawPaths {
		if strings.Contains(p, "list-type=2") && strings.Contains(p, "encoding-type=url") {
			sawEncoding = true
		}
	}
	if !sawEncoding {
		t.Errorf("List must send encoding-type=url: %v", fake.rawPaths)
	}
}

// TestListDecodesServerStyles: AWS writes a space as "+", MinIO as "%20"; a
// server that ignores encoding-type leaves keys alone and they are not decoded.
func TestListDecodesServerStyles(t *testing.T) {
	serve := func(encodingType, key string) *httptest.Server {
		return httptest.NewServer(httpHandler(func() string {
			enc := ""
			if encodingType != "" {
				enc = "<EncodingType>" + encodingType + "</EncodingType>"
			}
			return `<ListBucketResult><IsTruncated>false</IsTruncated>` + enc +
				`<Contents><Key>` + key + `</Key><Size>1</Size></Contents></ListBucketResult>`
		}))
	}
	for name, tc := range map[string]struct{ enc, wire, want string }{
		"aws plus for space": {"url", "a+b%2Bc", "a b+c"},
		"minio percent 20":   {"url", "a%20b%2Bc", "a b+c"},
		"not encoded":        {"", "a+b%2Bc", "a+b%2Bc"},
	} {
		srv := serve(tc.enc, tc.wire)
		c := &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s"}
		got, err := c.List(context.Background(), "bkt-one", "")
		srv.Close()
		if err != nil || len(got) != 1 || got[0].Key != tc.want {
			t.Errorf("%s: got %v (%v), want key %q", name, got, err, tc.want)
		}
	}
	srv := serve("url", "bad%zz")
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s"}
	if _, err := c.List(context.Background(), "bkt-one", ""); err == nil {
		t.Error("an invalid escape in an encoded key must be an error")
	}
}

func TestListHasPageAndObjectLimits(t *testing.T) {
	var n atomic.Int64
	srv := httptest.NewServer(httpHandler(func() string {
		i := n.Add(1)
		return fmt.Sprintf(`<ListBucketResult><IsTruncated>true</IsTruncated><NextContinuationToken>t%d</NextContinuationToken>`+
			`<Contents><Key>k%d</Key><Size>1</Size></Contents></ListBucketResult>`, i, i)
	}))
	defer srv.Close()
	c := &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s", MaxListPages: 5}
	_, err := c.List(context.Background(), "bkt-one", "")
	if err == nil || !strings.Contains(err.Error(), "more than 5 pages") {
		t.Fatalf("want a page-limit error, got %v", err)
	}
	if got := n.Load(); got > 6 {
		t.Errorf("fetched %d pages past a limit of 5", got)
	}
	n.Store(0)
	c = &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: "s", MaxListObjects: 3}
	_, err = c.List(context.Background(), "bkt-one", "")
	if err == nil || !strings.Contains(err.Error(), "more than 3 objects") {
		t.Fatalf("want an object-limit error, got %v", err)
	}
	// A finite listing under both limits still works.
	fake := newFakeS3()
	fs := fake.server(false)
	defer fs.Close()
	c = testClient(fs)
	c.MaxListPages, c.MaxListObjects, c.PageSize = 3, 6, 2
	ctx := context.Background()
	if err := c.EnsureBucket(ctx, "lim-bkt"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		if err := c.Put(ctx, "lim-bkt", fmt.Sprintf("k%d", i), strings.NewReader("v"), 1); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := c.List(ctx, "lim-bkt", ""); err != nil || len(got) != 6 {
		t.Fatalf("6 objects in 3 pages must fit exactly: %d, %v", len(got), err)
	}
}

// TestErrorRedactsBeforeTruncating: a secret that crosses the 300-character
// cut must not leak its first half.
func TestErrorRedactsBeforeTruncating(t *testing.T) {
	const secret = "SuperSecretValue-0123456789-ABCDEF"
	for _, pad := range []int{250, 270, 280, 290, 295, 298, 299, 300} {
		msg := strings.Repeat("x", pad) + secret + " tail"
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprintf(w, "<Error><Code>AccessDenied</Code><Message>%s</Message></Error>", msg)
		}))
		c := &Client{Endpoint: srv.URL, AccessKey: "a", SecretKey: secret}
		_, err := c.List(context.Background(), "bkt-one", "")
		srv.Close()
		if err == nil {
			t.Fatal("want an error")
		}
		for _, frag := range []string{"SuperSecr", "SuperSecretValue", "ecretValue-0123", "ABCDEF"} {
			if strings.Contains(err.Error(), frag) {
				t.Errorf("pad %d: error leaks %q of the secret: %v", pad, frag, err)
			}
		}
	}
}

func TestClientAndSignerPrintWithoutSecret(t *testing.T) {
	const secret = "TopSecret-ValueXYZ"
	c := Client{Endpoint: "http://h:9000", AccessKey: "AK", SecretKey: secret, Region: "r"}
	s := Signer{AccessKey: "AK", SecretKey: secret, Region: "r", Service: "s3"}
	for _, v := range []any{c, &c, s, &s} {
		for _, format := range []string{"%v", "%+v", "%#v", "%s"} {
			if out := fmt.Sprintf(format, v); strings.Contains(out, secret) || !strings.Contains(out, "AK") {
				t.Errorf("%s of %T = %q", format, v, out)
			}
		}
	}
	if !strings.Contains(fmt.Sprint(c), "[redacted]") {
		t.Error("the redaction marker must show")
	}
}

// TestDefaultClientHasNoWholeRequestTimeout: a large object may take longer
// than any fixed deadline; the transport bounds dialing, TLS and the response
// header instead, and the caller's context bounds the rest.
func TestDefaultClientHasNoWholeRequestTimeout(t *testing.T) {
	cl := (&Client{}).httpClient()
	if cl.Timeout != 0 {
		t.Errorf("Client.Timeout = %v, want 0 (it would cap the body transfer)", cl.Timeout)
	}
	tr, ok := cl.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport is %T", cl.Transport)
	}
	if tr.ResponseHeaderTimeout <= 0 || tr.TLSHandshakeTimeout <= 0 || tr.DialContext == nil {
		t.Errorf("transport lacks dial/TLS/header timeouts: header=%v tls=%v dial=%v", tr.ResponseHeaderTimeout, tr.TLSHandshakeTimeout, tr.DialContext != nil)
	}
	if tr.Proxy != nil {
		t.Error("the default client must not use a proxy")
	}
	if cl != (&Client{}).httpClient() {
		t.Error("the default client must be shared so connections are reused")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &Client{Endpoint: "http://127.0.0.1:1", AccessKey: "a", SecretKey: "s"}
	if _, err := c.List(ctx, "bkt-one", ""); err == nil {
		t.Error("a cancelled context must stop the call")
	}
}

// TestSigV4CanonicalFromEscapedForms: the canonical URI keeps an encoded slash
// inside its segment, and the canonical query never form-decodes.
func TestSigV4CanonicalFromEscapedForms(t *testing.T) {
	u, _ := url.Parse("http://h/bucket/a%2Fb/c%20d+e")
	if got, want := (Signer{Service: "s3"}).canonicalURI(u), "/bucket/a%2Fb/c%20d%2Be"; got != want {
		t.Errorf("canonicalURI = %q, want %q", got, want)
	}
	if got, want := (Signer{Service: "iam"}).canonicalURI(u), "/bucket/a%252Fb/c%2520d%252Be"; got != want {
		t.Errorf("double-encoded canonicalURI = %q, want %q", got, want)
	}
	u2, _ := url.Parse("http://h/bucket/key")
	if got := (Signer{Service: "s3"}).canonicalURI(u2); got != "/bucket/key" {
		t.Errorf("plain path = %q", got)
	}
	u3, _ := url.Parse("http://h")
	if got := (Signer{Service: "s3"}).canonicalURI(u3); got != "/" {
		t.Errorf("empty path = %q", got)
	}
	// A request built with a literal '+' in the query is signed as %2B, the
	// same as one that spelled it %2B.
	r1, _ := http.NewRequest("GET", "http://h/b?prefix=a+b", nil)
	r2, _ := http.NewRequest("GET", "http://h/b?prefix=a%2Bb", nil)
	if canonicalQuery(r1.URL.RawQuery) != canonicalQuery(r2.URL.RawQuery) {
		t.Error("a literal plus and %2B must sign identically")
	}
}

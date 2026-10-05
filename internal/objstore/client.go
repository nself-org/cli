package objstore

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/nself-org/cli/internal/httptimeout"
)

// Client talks to one S3-compatible endpoint in path style
// (http://host:port/<bucket>/<key>).
//
// Inputs: Endpoint is scheme://host[:port] with no userinfo, path or query;
// Region defaults to "us-east-1" when empty; AccessKey and SecretKey are the
// credentials (the secret never appears in an error, in String or in %#v).
// PageSize caps keys per List page (0 = server default); MaxListPages and
// MaxListObjects bound List (0 = DefaultMaxListPages and DefaultMaxListObjects).
// HTTP defaults to a shared no-proxy, no-redirect client with dial, TLS and
// response-header timeouts and no whole-request timeout, so a large object can
// take as long as it needs: bound a call with its context. Now defaults to
// time.Now (tests pin it).
// Constraints: a PUT streams with UNSIGNED-PAYLOAD only over plain http to a
// loopback endpoint; every other PUT is signed over the payload hash.
type Client struct {
	Endpoint  string
	Region    string
	AccessKey string
	SecretKey string
	PageSize  int

	MaxListPages   int
	MaxListObjects int

	HTTP *http.Client
	Now  func() time.Time
}

var bucketRe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)

// maxKeyBytes is the S3 key limit.
const maxKeyBytes = 1024

// String hides the secret key; Client is often printed whole.
func (c Client) String() string {
	return "objstore.Client{Endpoint: " + c.Endpoint + ", Region: " + c.Region + ", AccessKey: " + c.AccessKey + ", SecretKey: [redacted]}"
}

// GoString is String for %#v.
func (c Client) GoString() string { return c.String() }

var (
	defaultHTTPOnce sync.Once
	defaultHTTP     *http.Client
)

// headerTimeout is how long the server may take to start its response once the
// request is fully sent (a PUT answers after the whole body arrived).
func headerTimeout() time.Duration { return httptimeout.Backup.Timeout }

// sharedHTTP returns the default client: no proxy, no redirects (so the
// credentials reach only the checked host), and timeouts on dialing, the TLS
// handshake and the response header instead of a deadline on the whole request.
func sharedHTTP() *http.Client {
	defaultHTTPOnce.Do(func() {
		cl := httptimeout.NoProxy(0)
		if tr, ok := cl.Transport.(*http.Transport); ok {
			tr.ResponseHeaderTimeout = headerTimeout()
		}
		defaultHTTP = cl
	})
	return defaultHTTP
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return sharedHTTP()
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Client) signer() Signer {
	r := c.Region
	if r == "" {
		r = "us-east-1"
	}
	return Signer{AccessKey: c.AccessKey, SecretKey: c.SecretKey, Region: r, Service: "s3"}
}

// endpoint parses and validates c.Endpoint.
func (c *Client) endpoint() (*url.URL, error) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("objstore: endpoint must be http(s)://host[:port]")
	}
	if u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("objstore: endpoint must not carry userinfo, path or query")
	}
	if c.AccessKey == "" || c.SecretKey == "" {
		return nil, errors.New("objstore: access key and secret key are required")
	}
	return u, nil
}

// isLoopbackHTTP reports whether u is plain http to localhost or a loopback IP.
func isLoopbackHTTP(u *url.URL) bool {
	if u.Scheme != "http" {
		return false
	}
	h := u.Hostname()
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

func checkBucket(b string) error {
	if !bucketRe.MatchString(b) || strings.Contains(b, "..") {
		return fmt.Errorf("objstore: invalid bucket name %q", b)
	}
	return nil
}

func checkKey(k string) error {
	if k == "" || len(k) > maxKeyBytes || !utf8.ValidString(k) {
		return errors.New("objstore: object key must be 1-1024 bytes of UTF-8")
	}
	return nil
}

// build creates and signs the http.Request for path-style bucket/key.
func (c *Client) build(ctx context.Context, method, bucket, key string, query [][2]string, payloadHash string) (*http.Request, error) {
	u, err := c.endpoint()
	if err != nil {
		return nil, err
	}
	path := "/" + bucket
	if key != "" {
		path += "/" + key
	}
	var q []string
	for _, kv := range query {
		q = append(q, uriEncode(kv[0], true)+"="+uriEncode(kv[1], true))
	}
	raw := u.Scheme + "://" + u.Host + uriEncode(path, false)
	if len(q) > 0 {
		raw += "?" + strings.Join(q, "&")
	}
	req, err := http.NewRequestWithContext(ctx, method, raw, nil)
	if err != nil {
		return nil, errors.New("objstore: " + c.redact(err.Error()))
	}
	c.signer().Sign(req, payloadHash, c.now())
	return req, nil
}

// send performs req and redacts transport errors.
func (c *Client) send(ctx context.Context, op string, req *http.Request) (*http.Response, error) {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		msg := fmt.Sprintf("objstore: %s: %s", op, c.redact(err.Error()))
		if ce := ctx.Err(); ce != nil {
			return nil, fmt.Errorf("%s: %w", msg, ce)
		}
		return nil, errors.New(msg)
	}
	return resp, nil
}

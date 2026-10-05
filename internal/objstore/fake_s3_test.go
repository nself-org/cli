package objstore

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	fakeAccess = "AKIAFAKEFAKEFAKE0001"
	fakeSecret = "s3cr3t/with+chars=ThatMustNeverLeak"
)

// fakeS3 is an in-memory path-style S3 that verifies SigV4 on every request
// (it recomputes the signature from what arrived on the wire).
type fakeS3 struct {
	mu      sync.Mutex
	buckets map[string]map[string][]byte
	// rawPaths records the raw request URIs seen (wire encoding checks).
	rawPaths []string
	unsigned int // PUTs that arrived with UNSIGNED-PAYLOAD
	signed   int // PUTs that arrived with a payload hash
}

func newFakeS3() *fakeS3 { return &fakeS3{buckets: map[string]map[string][]byte{}} }

func (f *fakeS3) server(tls bool) *httptest.Server {
	if tls {
		return httptest.NewTLSServer(f)
	}
	return httptest.NewServer(f)
}

func (f *fakeS3) fail(w http.ResponseWriter, status int, code, msg string) {
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><Error><Code>%s</Code><Message>%s</Message></Error>`, code, msg)
}

func (f *fakeS3) verify(r *http.Request, payload string) bool {
	amz := r.Header.Get("X-Amz-Date")
	t, err := time.Parse("20060102T150405Z", amz)
	if err != nil {
		return false
	}
	// Rebuild a request exactly as it arrived, keeping only signed headers.
	signedList := ""
	for _, part := range strings.Split(r.Header.Get("Authorization"), ", ") {
		if strings.HasPrefix(part, "SignedHeaders=") {
			signedList = strings.TrimPrefix(part, "SignedHeaders=")
		}
	}
	u := &url.URL{Scheme: "http", Host: r.Host, RawQuery: r.URL.RawQuery}
	u.Path, u.RawPath = r.URL.Path, r.URL.RawPath
	chk, _ := http.NewRequest(r.Method, u.String(), nil)
	for _, h := range strings.Split(signedList, ";") {
		if h != "host" && h != "x-amz-date" && h != "x-amz-content-sha256" {
			chk.Header.Set(h, r.Header.Get(h))
		}
	}
	Signer{AccessKey: fakeAccess, SecretKey: fakeSecret, Region: "us-east-1", Service: "s3"}.Sign(chk, payload, t)
	return chk.Header.Get("Authorization") == r.Header.Get("Authorization")
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rawPaths = append(f.rawPaths, r.RequestURI)
	payload := r.Header.Get("X-Amz-Content-Sha256")
	if !strings.Contains(r.Header.Get("Authorization"), "Credential="+fakeAccess+"/") || !f.verify(r, payload) {
		f.fail(w, http.StatusForbidden, "SignatureDoesNotMatch",
			"The request signature does not match")
		return
	}
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	objs, ok := f.buckets[bucket]
	switch {
	case key == "" && r.Method == http.MethodHead:
		if !ok {
			w.WriteHeader(http.StatusNotFound)
		}
	case key == "" && r.Method == http.MethodPut:
		if ok {
			f.fail(w, http.StatusConflict, "BucketAlreadyOwnedByYou", "exists")
			return
		}
		f.buckets[bucket] = map[string][]byte{}
	case key == "" && r.Method == http.MethodGet:
		if !ok {
			f.fail(w, http.StatusNotFound, "NoSuchBucket", "no such bucket")
			return
		}
		f.list(w, r, objs)
	case r.Method == http.MethodPut:
		if !ok {
			f.fail(w, http.StatusNotFound, "NoSuchBucket", "no such bucket")
			return
		}
		body, rerr := io.ReadAll(r.Body)
		if rerr != nil {
			f.fail(w, http.StatusBadRequest, "IncompleteBody", "incomplete body")
			return
		}
		if payload == unsignedPayload {
			f.unsigned++
		} else {
			sum := sha256.Sum256(body)
			if hex.EncodeToString(sum[:]) != payload {
				f.fail(w, http.StatusBadRequest, "XAmzContentSHA256Mismatch", "payload hash mismatch")
				return
			}
			f.signed++
		}
		objs[key] = body
	case r.Method == http.MethodGet:
		b, found := objs[key]
		if !found {
			f.fail(w, http.StatusNotFound, "NoSuchKey", "no such key")
			return
		}
		_, _ = w.Write(b)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) list(w http.ResponseWriter, r *http.Request, objs map[string][]byte) {
	q := r.URL.Query()
	prefix, token := q.Get("prefix"), q.Get("continuation-token")
	max := 1000
	if v, err := strconv.Atoi(q.Get("max-keys")); err == nil {
		max = v
	}
	var keys []string
	for k := range objs {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	start := 0
	if token != "" {
		raw, err := base64.StdEncoding.DecodeString(token)
		if err != nil {
			f.fail(w, http.StatusBadRequest, "InvalidArgument", "bad continuation token")
			return
		}
		start = sort.SearchStrings(keys, string(raw))
	}
	end, next := start+max, ""
	if end < len(keys) {
		next = base64.StdEncoding.EncodeToString([]byte(keys[end])) // contains + / =
	} else {
		end = len(keys)
	}
	encoded := q.Get("encoding-type") == "url"
	var b strings.Builder
	b.WriteString(`<?xml version="1.0"?><ListBucketResult><IsTruncated>` + strconv.FormatBool(next != "") + `</IsTruncated>`)
	if encoded {
		b.WriteString("<EncodingType>url</EncodingType>")
	}
	if next != "" {
		b.WriteString("<NextContinuationToken>" + next + "</NextContinuationToken>")
	}
	for _, k := range keys[start:end] {
		var esc strings.Builder
		if encoded {
			esc.WriteString(url.QueryEscape(k)) // AWS style: space is "+", plus is %2B
		} else {
			_ = xml.EscapeText(&esc, []byte(k))
		}
		fmt.Fprintf(&b, "<Contents><Key>%s</Key><Size>%d</Size><ETag>\"e\"</ETag></Contents>", esc.String(), len(objs[k]))
	}
	b.WriteString("</ListBucketResult>")
	_, _ = w.Write([]byte(b.String()))
}

// httpHandler serves a fixed XML body for every request.
func httpHandler(body func() string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body()))
	})
}

package objstore

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// emptySHA256 is the SHA-256 of zero bytes.
const emptySHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Payload hash markers.
const unsignedPayload = "UNSIGNED-PAYLOAD"

// Signer signs requests with AWS Signature Version 4.
//
// Purpose: stdlib-only SigV4 (crypto/hmac, crypto/sha256).
// Inputs: Service is "s3" for object stores (the path is encoded once and
// x-amz-content-sha256 is set); any other service is double-path-encoded and
// gets no content header, matching the AWS SigV4 test suite.
// Constraints: never log a Signer; SecretKey is the secret.
type Signer struct {
	AccessKey string
	SecretKey string
	Region    string
	Service   string
}

// String and GoString hide the secret, so a Signer in a log line or a %+v
// never prints it.
func (s Signer) String() string {
	return "objstore.Signer{AccessKey: " + s.AccessKey + ", SecretKey: [redacted], Region: " + s.Region + "}"
}

// GoString is String for %#v.
func (s Signer) GoString() string { return s.String() }

// Sign sets X-Amz-Date, (for s3) X-Amz-Content-Sha256 and Authorization on
// req. payloadSHA256 is the lowercase hex SHA-256 of the body, or
// "UNSIGNED-PAYLOAD". Every header already on req, plus Host, is signed.
func (s Signer) Sign(req *http.Request, payloadSHA256 string, t time.Time) {
	amz := t.UTC().Format("20060102T150405Z")
	date := amz[:8]
	req.Header.Set("X-Amz-Date", amz)
	if s.Service == "s3" {
		req.Header.Set("X-Amz-Content-Sha256", payloadSHA256)
	}
	names, canonHeaders := canonicalHeaders(req)
	signed := strings.Join(names, ";")
	canonical := strings.Join([]string{
		req.Method,
		s.canonicalURI(req.URL),
		canonicalQuery(req.URL.RawQuery),
		canonHeaders,
		signed,
		payloadSHA256,
	}, "\n")
	scope := date + "/" + s.Region + "/" + s.Service + "/aws4_request"
	sts := "AWS4-HMAC-SHA256\n" + amz + "\n" + scope + "\n" + hexSHA256([]byte(canonical))
	sig := hex.EncodeToString(hmacSHA256(s.signingKey(date), sts))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+s.AccessKey+"/"+scope+
		", SignedHeaders="+signed+", Signature="+sig)
}

// signingKey derives the SigV4 key for a date (YYYYMMDD).
func (s Signer) signingKey(date string) []byte {
	k := hmacSHA256([]byte("AWS4"+s.SecretKey), date)
	k = hmacSHA256(k, s.Region)
	k = hmacSHA256(k, s.Service)
	return hmacSHA256(k, "aws4_request")
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func hexSHA256(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// canonicalHeaders returns the sorted signed header names and the canonical
// header block (each "name:value\n"). Values are trimmed with inner runs of
// spaces collapsed; repeated headers are joined with ",".
func canonicalHeaders(req *http.Request) ([]string, string) {
	vals := map[string][]string{}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	vals["host"] = []string{host}
	for k, vs := range req.Header {
		lk := strings.ToLower(k)
		if lk == "authorization" || lk == "host" {
			continue
		}
		vals[lk] = append(vals[lk], vs...)
	}
	names := make([]string, 0, len(vals))
	for k := range vals {
		names = append(names, k)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, k := range names {
		clean := make([]string, len(vals[k]))
		for i, v := range vals[k] {
			clean[i] = strings.Join(strings.Fields(v), " ")
		}
		b.WriteString(k + ":" + strings.Join(clean, ",") + "\n")
	}
	return names, b.String()
}

// canonicalURI builds the canonical path from the request's escaped path
// (URL.EscapedPath, so an encoded slash %2F stays inside its segment): each
// segment is percent-decoded and encoded again once (s3) or twice (other
// services). Decoding never turns "+" into a space.
func (s Signer) canonicalURI(u *url.URL) string {
	p := u.EscapedPath()
	if p == "" {
		p = "/"
	}
	segs := strings.Split(p, "/")
	for i, seg := range segs {
		segs[i] = uriEncode(pctDecode(seg), true)
		if s.Service != "s3" {
			segs[i] = uriEncode(segs[i], true)
		}
	}
	return strings.Join(segs, "/")
}

// canonicalQuery sorts the query by encoded name then value and re-encodes it.
// The raw query is split on "&" and "=" and percent-decoded without form
// decoding: a literal "+" is a plus sign (encoded %2B), not a space.
func canonicalQuery(raw string) string {
	if raw == "" {
		return ""
	}
	type kv struct{ k, v string }
	var pairs []kv
	for _, part := range strings.Split(raw, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		pairs = append(pairs, kv{uriEncode(pctDecode(k), true), uriEncode(pctDecode(v), true)})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].k != pairs[j].k {
			return pairs[i].k < pairs[j].k
		}
		return pairs[i].v < pairs[j].v
	})
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.k + "=" + p.v
	}
	return strings.Join(parts, "&")
}

// pctDecode decodes every valid %XX escape in s and leaves everything else,
// including "+" and a "%" that is not followed by two hex digits, as it is.
func pctDecode(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '%' && i+2 < len(s) && isHex(s[i+1]) && isHex(s[i+2]) {
			b = append(b, unhex(s[i+1])<<4|unhex(s[i+2]))
			i += 2
			continue
		}
		b = append(b, s[i])
	}
	return string(b)
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func unhex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

// uriEncode percent-encodes every byte except A-Z a-z 0-9 - _ . ~ (and '/'
// unless encodeSlash), with uppercase hex, as SigV4 requires. Space is %20,
// '+' is %2B, '%' is %25; UTF-8 is encoded byte by byte.
func uriEncode(s string, encodeSlash bool) string {
	const hexd = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		case c == '/' && !encodeSlash:
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hexd[c>>4])
			b.WriteByte(hexd[c&15])
		}
	}
	return b.String()
}

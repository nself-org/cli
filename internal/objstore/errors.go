package objstore

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// maxErrorBody bounds how much of an error response is read.
const maxErrorBody = 64 << 10

// Error is a non-success S3 response.
type Error struct {
	// Op is the operation ("list", "get", "put", "head-bucket", "create-bucket").
	Op string
	// Status is the HTTP status code.
	Status int
	// Code is the S3 error code ("NoSuchBucket", "SignatureDoesNotMatch", ...), if the body had one.
	Code string
	// Message is the server's message, redacted.
	Message string
}

func (e *Error) Error() string {
	s := fmt.Sprintf("objstore: %s: HTTP %d", e.Op, e.Status)
	if e.Code != "" {
		s += " " + e.Code
	}
	if e.Message != "" {
		s += ": " + e.Message
	}
	return s
}

// IsNotFound reports whether the response was 404.
func (e *Error) IsNotFound() bool { return e.Status == http.StatusNotFound }

var signatureRe = regexp.MustCompile(`(?i)(signature=|signature:\s*)[0-9a-f]{16,}`)

// redact removes the secret key (raw and URL-escaped forms) and any SigV4
// signature from s. It runs on every string that reaches an error.
func (c *Client) redact(s string) string {
	if c.SecretKey != "" {
		for _, v := range []string{c.SecretKey, url.QueryEscape(c.SecretKey), url.PathEscape(c.SecretKey),
			uriEncode(c.SecretKey, true)} {
			s = strings.ReplaceAll(s, v, "[redacted]")
		}
	}
	return signatureRe.ReplaceAllString(s, "${1}[redacted]")
}

// apiError builds an *Error from a non-success response and closes its body.
func (c *Client) apiError(op string, resp *http.Response) error {
	defer func() { _ = resp.Body.Close() }()
	e := &Error{Op: op, Status: resp.StatusCode}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	var x struct {
		Code    string `xml:"Code"`
		Message string `xml:"Message"`
	}
	if xml.Unmarshal(raw, &x) == nil {
		// Redact first: truncating first could cut a secret in two and leak
		// its first half.
		e.Code = oneLine(c.redact(x.Code), 80)
		e.Message = oneLine(c.redact(x.Message), 300)
	}
	return e
}

// oneLine collapses whitespace and truncates s.
func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "..."
	}
	return s
}

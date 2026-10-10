package invoke

// Purpose: decode a machine request and derive its redacted request id.
// Inputs: raw request bytes; a registry command for redaction.
// Outputs: Request, RequestID, RedactRequest, SecretValues.
// Constraints: unknown members and trailing data are E420. The id hashes the
// request after secret args and flags are replaced, so it never derives from a
// secret; it names a request in logs and is never a confirmation.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/cmdregistry"
	"github.com/nself-org/cli/internal/errs"
)

// Request is the machine request (every member optional).
type Request struct {
	// Args are the positional values, one argv element each.
	Args []string `json:"args,omitempty"`
	// Flags maps a flag name to bool, integer, number, string or []string.
	Flags map[string]any `json:"flags,omitempty"`
	// Argv is the verbatim argv of a plugin command that declares no args or flags.
	Argv []string `json:"argv,omitempty"`
	// Confirm is a plan id or a server-issued nonce (consumed by the gate).
	Confirm string `json:"confirm,omitempty"`
}

var confirmPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// badRequest is an E420 error. Callers pass names, never values.
func badRequest(format string, a ...any) error { return errs.Newf("E420", format, a...) }

// quoteName renders a caller-supplied name for an error message: quoted, bounded.
func quoteName(s string) string {
	if r := []rune(s); len(r) > 48 {
		s = string(r[:48]) + "..."
	}
	return strconv.Quote(s)
}

// DecodeRequest parses one request document. Empty input is the empty request.
func DecodeRequest(data []byte) (Request, error) {
	var r Request
	if len(data) > MaxRequestBytes {
		return r, badRequest("request is larger than %d bytes", MaxRequestBytes)
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return r, nil
	}
	if trimmed[0] != '{' {
		return r, badRequest("request must be a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return Request{}, badRequest("request is not valid: %v", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return Request{}, badRequest("request has data after the JSON object")
	}
	if r.Confirm != "" && !confirmPattern.MatchString(r.Confirm) {
		return Request{}, badRequest("confirm must be 64 lowercase hex characters")
	}
	return r, nil
}

// SetFlags returns the names of the flags the request names.
func SetFlags(r Request) map[string]bool {
	set := make(map[string]bool, len(r.Flags))
	for name := range r.Flags {
		set[name] = true
	}
	return set
}

// barePath strips the root name: "nself config get" -> "config get".
func barePath(path string) string {
	return strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(path), "nself ")), " ")
}

// secretArg reports whether positional i is secret (a variadic secret covers the rest).
func secretArg(cmd *cmdregistry.Command, i int) bool {
	if cmd == nil {
		return true
	}
	for j, a := range cmd.Args {
		if j == i || (a.Variadic && i >= j) {
			return a.Secret
		}
	}
	return false
}

func secretFlag(cmd *cmdregistry.Command, name string) bool {
	if cmd == nil {
		return true
	}
	for _, f := range cmd.Flags {
		if f.Name == name {
			return f.Secret
		}
	}
	return false
}

// RedactRequest returns a copy of r with every registry-secret arg and flag
// value replaced by Redacted. A nil cmd (unknown path) redacts all of them.
func RedactRequest(cmd *cmdregistry.Command, r Request) Request {
	out := Request{Confirm: r.Confirm, Argv: append([]string(nil), r.Argv...)}
	for i, a := range r.Args {
		if secretArg(cmd, i) {
			a = Redacted
		}
		out.Args = append(out.Args, a)
	}
	if r.Flags != nil {
		out.Flags = make(map[string]any, len(r.Flags))
		for name, v := range r.Flags {
			if secretFlag(cmd, name) {
				v = Redacted
			}
			out.Flags[name] = v
		}
	}
	return out
}

// SecretValues lists every secret value the request carries, as the strings a
// child could echo (longest first), for scrubbing stderr tails.
func SecretValues(cmd *cmdregistry.Command, r Request) []string {
	var vals []string
	add := func(s string) {
		if s != "" && s != Redacted {
			vals = append(vals, s)
		}
	}
	for i, a := range r.Args {
		if secretArg(cmd, i) {
			add(a)
		}
	}
	for name, v := range r.Flags {
		if !secretFlag(cmd, name) {
			continue
		}
		switch x := v.(type) {
		case []string:
			for _, s := range x {
				add(s)
			}
		case []any:
			for _, e := range x {
				add(fmt.Sprint(e))
			}
		default:
			add(fmt.Sprint(x))
		}
	}
	sort.SliceStable(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	return vals
}

// canonNumber renders a numeric value in one form whatever its Go type.
func canonNumber(v any) (json.Number, bool) {
	var f float64
	switch x := v.(type) {
	case json.Number:
		if i, err := strconv.ParseInt(string(x), 10, 64); err == nil {
			return json.Number(strconv.FormatInt(i, 10)), true
		}
		g, err := strconv.ParseFloat(string(x), 64)
		if err != nil {
			return "", false
		}
		f = g
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int:
		return json.Number(strconv.Itoa(x)), true
	case int64:
		return json.Number(strconv.FormatInt(x, 10)), true
	case uint64:
		return json.Number(strconv.FormatUint(x, 10)), true
	default:
		return "", false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return "", false
	}
	if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return json.Number(strconv.FormatInt(int64(f), 10)), true
	}
	return json.Number(strconv.FormatFloat(f, 'g', -1, 64)), true
}

func canonValue(v any) any {
	if n, ok := canonNumber(v); ok {
		return n
	}
	switch x := v.(type) {
	case []string:
		return x
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = canonValue(e)
		}
		return out
	}
	return v
}

// RequestID is the lowercase hex sha256 of the canonical JSON
// {"args":[..],"argv":[..],"command":"<path>","flags":{..}}: keys sorted, no
// whitespace, confirm excluded, absent members empty, secret values replaced
// (RedactRequest). path may carry the "nself " prefix; cmd may be nil.
func RequestID(path string, cmd *cmdregistry.Command, r Request) string {
	red := RedactRequest(cmd, r)
	doc := map[string]any{"args": []string{}, "argv": []string{}, "command": barePath(path), "flags": map[string]any{}}
	if red.Args != nil {
		doc["args"] = red.Args
	}
	if red.Argv != nil {
		doc["argv"] = red.Argv
	}
	if red.Flags != nil {
		flags := make(map[string]any, len(red.Flags))
		for name, v := range red.Flags {
			flags[name] = canonValue(v)
		}
		doc["flags"] = flags
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(doc); err != nil {
		buf.Reset()
		buf.WriteString(fmt.Sprintf("unencodable:%q", barePath(path)))
	}
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return hex.EncodeToString(sum[:])
}

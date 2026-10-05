package portable

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Limits on member names.
const (
	maxMemberPath    = 4096
	maxMemberSegment = 255
)

// ErrUnsafePath is wrapped by CheckMember failures.
var ErrUnsafePath = errors.New("unsafe bundle member path")

// windowsReserved are device names Windows resolves in any directory, with or
// without an extension.
var windowsReserved = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// CheckMember reports whether rel is a safe member path: relative, slash
// separated, made only of plain segments.
//
// Purpose: a bundle is untrusted. A listed path must never leave the bundle
// directory on any platform, so the check is the strictest common subset.
// Refused: empty or over-long names; invalid UTF-8; control characters and
// NUL; a backslash (Windows separator); a leading slash; empty, "." and ".."
// segments (so no "../" and no "//"); ":" in any segment (Windows drive
// letters "C:", alternate data streams); a segment ending in "." or space;
// Windows device names (CON, NUL, COM1, LPT1 and the like, with any
// extension).
// Outputs: nil, or an error wrapping ErrUnsafePath that says why (never more
// than the name, which is not a secret).
func CheckMember(rel string) error {
	bad := func(why string) error { return fmt.Errorf("%w: %s", ErrUnsafePath, why) }
	if rel == "" {
		return bad("empty path")
	}
	if len(rel) > maxMemberPath {
		return bad("path too long")
	}
	if !utf8.ValidString(rel) {
		return bad("not valid UTF-8")
	}
	if strings.HasPrefix(rel, "/") {
		return bad("absolute path")
	}
	for _, r := range rel {
		switch {
		case r < 0x20 || r == 0x7f:
			return bad("control character")
		case r == '\\':
			return bad("backslash")
		}
	}
	for _, seg := range strings.Split(rel, "/") {
		switch {
		case seg == "":
			return bad("empty segment")
		case seg == "." || seg == "..":
			return bad("dot segment")
		case len(seg) > maxMemberSegment:
			return bad("segment too long")
		case strings.Contains(seg, ":"):
			return bad("colon in segment")
		case strings.HasSuffix(seg, ".") || strings.HasSuffix(seg, " "):
			return bad("segment ends with dot or space")
		}
		stem, _, _ := strings.Cut(seg, ".")
		if windowsReserved[strings.ToLower(strings.TrimRight(stem, " "))] {
			return bad("reserved device name")
		}
	}
	return nil
}

// StorageMember returns the member path of an object's bytes:
// storage/<bucket>/<key>, with each key segment escaped so any S3 key maps to
// a safe member.
//
// Purpose: object keys are arbitrary; member paths are not. The manifest
// carries the real key; this function only names the file.
// Escaping, per segment, as %XX of the UTF-8 bytes: control characters, DEL,
// '\', ':', '%', '*', '?', '"', '<', '>', '|'; a trailing '.' or ' '; a
// segment that is exactly "." or ".." (escaped dots); a Windows device name
// (first character escaped). An empty segment (from "a//b" or a trailing "/")
// becomes the lone character "%", which the escaping never produces otherwise.
// Outputs: the member path, or an error when bucket is not a plain name or
// the result is still unsafe (for example a key that is only spaces).
// Constraints: a key that is a directory prefix of another ("a" and "a/b")
// cannot both be files; the Writer reports that clash.
func StorageMember(bucket, key string) (string, error) {
	if bucket == "" || strings.ContainsAny(bucket, "/\\") || CheckMember(bucket) != nil {
		return "", fmt.Errorf("%w: bucket name %q", ErrUnsafePath, bucket)
	}
	if key == "" || !utf8.ValidString(key) {
		return "", fmt.Errorf("%w: object key is empty or not UTF-8", ErrUnsafePath)
	}
	segs := strings.Split(key, "/")
	for i, s := range segs {
		segs[i] = escapeSegment(s)
	}
	m := "storage/" + bucket + "/" + strings.Join(segs, "/")
	if err := CheckMember(m); err != nil {
		return "", err
	}
	return m, nil
}

// escapeSegment applies the StorageMember escaping to one key segment.
func escapeSegment(s string) string {
	if s == "" {
		return "%"
	}
	if s == "." || s == ".." {
		return strings.ReplaceAll(s, ".", "%2E")
	}
	stem, _, _ := strings.Cut(s, ".")
	reserved := windowsReserved[strings.ToLower(strings.TrimRight(stem, " "))]
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		last := i == len(s)-1
		esc := c < 0x20 || c == 0x7f || strings.IndexByte(`\:%*?"<>|`, c) >= 0 ||
			(last && (c == '.' || c == ' ')) || (i == 0 && reserved)
		if esc {
			fmt.Fprintf(&b, "%%%02X", c)
		} else {
			b.WriteByte(c)
		}
	}
	return b.String()
}

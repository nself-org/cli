package portable

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
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
// extension); a name that is not in Unicode NFC (so one name has one
// spelling, whatever the file system normalises).
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
	if !norm.NFC.IsNormalString(rel) {
		return bad("not in Unicode NFC")
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

// StorageMemberPrefix is the directory of every stored object's bytes.
const StorageMemberPrefix = "storage/objects/"

// StorageMemberPattern matches a storage member path.
const StorageMemberPattern = `^storage/objects/[0-9a-f]{64}$`

// StorageMember returns the member path of an object's bytes:
// storage/objects/<hex sha256(bucket + "\x00" + key)>.
//
// Purpose: object keys are arbitrary and case- and Unicode-sensitive; file
// systems are not. Naming the file by a digest gives every (bucket, key) pair
// its own member on every platform, so keys like Readme.md and README.md, or
// the NFC and NFD spellings of one name, export on a case-insensitive or
// normalising file system as they do on S3. The real bucket and key live in
// the manifest (storage.objects[]); a reader maps key to member with this
// function and never parses the file name.
// Outputs: the member path, or an error wrapping ErrUnsafePath when the
// bucket is empty or holds NUL (it would make the digest input ambiguous), or
// the key is empty or not UTF-8.
func StorageMember(bucket, key string) (string, error) {
	if bucket == "" || !utf8.ValidString(bucket) || strings.IndexByte(bucket, 0) >= 0 {
		return "", fmt.Errorf("%w: bucket name %q", ErrUnsafePath, bucket)
	}
	if key == "" || !utf8.ValidString(key) {
		return "", fmt.Errorf("%w: object key is empty or not UTF-8", ErrUnsafePath)
	}
	sum := sha256.Sum256([]byte(bucket + "\x00" + key))
	return StorageMemberPrefix + hex.EncodeToString(sum[:]), nil
}

// NewObject returns the manifest entry for an object whose bytes were written
// as member f (the File that WriteFile returned for StorageMember(bucket, key)).
func NewObject(bucket, key string, f File) (Object, error) {
	m, err := StorageMember(bucket, key)
	if err != nil {
		return Object{}, err
	}
	if f.Path != m {
		return Object{}, fmt.Errorf("%w: object %q/%q belongs at %s, not %s", ErrUnsafePath, bucket, key, m, f.Path)
	}
	return Object{Bucket: bucket, Key: key, Member: m, SHA256: f.SHA256, Bytes: f.Bytes}, nil
}

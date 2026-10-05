package signing

import (
	"bufio"
	"crypto/ed25519"
	"fmt"
	"io"
	"strings"
)

const (
	maxLine    = 4096
	maxFile    = 1 << 20
	maxEntries = 1024
)

type fileConfig struct {
	scope string
}

// FileOption configures ParseKeysFile.
type FileOption func(*fileConfig)

// KeysScope sets Key.Scope on every parsed key.
func KeysScope(s string) FileOption { return func(c *fileConfig) { c.scope = s } }

// isBlank is the field separator: space or tab.
func isBlank(c rune) bool {
	return c == ' ' || c == '\t'
}

// readLines yields trimmed content lines (comments and blanks skipped) with
// their line numbers, enforcing the size limits.
func readLines(r io.Reader, fn func(n int, fields []string) error) error {
	if r == nil {
		return fmt.Errorf("%w: nil reader", ErrMalformed)
	}
	lr := &io.LimitedReader{R: r, N: maxFile + 1}
	br := bufio.NewReaderSize(lr, maxLine+2)
	for n := 1; ; n++ {
		line, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			return fmt.Errorf("%w: line %d too long", ErrMalformed, n)
		}
		if err != nil && err != io.EOF {
			return fmt.Errorf("signing: read: %w", err)
		}
		if lr.N <= 0 {
			return fmt.Errorf("%w: file larger than %d bytes", ErrMalformed, maxFile)
		}
		text := strings.TrimSuffix(strings.TrimSuffix(string(line), "\n"), "\r")
		if len(text) > maxLine {
			return fmt.Errorf("%w: line %d too long", ErrMalformed, n)
		}
		if t := strings.Trim(text, " \t"); t != "" && t[0] != '#' {
			if ferr := fn(n, strings.FieldsFunc(t, isBlank)); ferr != nil {
				return ferr
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}

// ParseKeysFile parses a trust file (".nself/trust/<purpose>.keys"): lines of
// "<key-id> <base64 raw public key>". See the package doc for the exact rules.
// On any error no keys are returned.
func ParseKeysFile(r io.Reader, p Purpose, opts ...FileOption) ([]Key, error) {
	if !p.Valid() {
		return nil, fmt.Errorf("%w: purpose %q", ErrMalformed, clip(string(p)))
	}
	var cfg fileConfig
	for _, o := range opts {
		if o != nil {
			o(&cfg)
		}
	}
	var keys []Key
	seen := map[string]bool{}
	seenPub := map[string]string{}
	err := readLines(r, func(n int, f []string) error {
		if len(f) != 2 {
			return fmt.Errorf("%w: line %d: want \"<key-id> <base64 public key>\"", ErrMalformed, n)
		}
		if !validKeyID(f[0]) {
			return fmt.Errorf("%w: line %d: key id %q", ErrMalformed, n, clip(f[0]))
		}
		if seen[f[0]] {
			return fmt.Errorf("%w: line %d: duplicate key id %q", ErrMalformed, n, f[0])
		}
		raw, derr := decodeStrict(f[1])
		if derr != nil {
			return fmt.Errorf("%w: line %d: public key base64 (%q)", ErrMalformed, n, f[0])
		}
		if len(raw) != ed25519.PublicKeySize {
			return fmt.Errorf("%w: line %d: public key length %d (%q)", ErrMalformed, n, len(raw), f[0])
		}
		pub := ed25519.PublicKey(raw)
		key := Key{ID: f[0], Purpose: p, Scope: cfg.scope, Public: pub}
		if cerr := checkKey(key); cerr != nil {
			return fmt.Errorf("line %d: %w", n, cerr)
		}
		if prev, dup := seenPub[string(raw)]; dup {
			return fmt.Errorf("%w: line %d: key %q repeats the public key of %q", ErrMalformed, n, f[0], prev)
		}
		if len(keys) >= maxEntries {
			return fmt.Errorf("%w: more than %d entries", ErrMalformed, maxEntries)
		}
		seen[f[0]] = true
		seenPub[string(raw)] = f[0]
		keys = append(keys, key)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, nil
}

// ParseRevokedFile parses a ".revoked" file: one key id per line. Duplicates
// are collapsed, first-seen order kept. On any error nothing is returned.
func ParseRevokedFile(r io.Reader) ([]string, error) {
	var ids []string
	seen := map[string]bool{}
	err := readLines(r, func(n int, f []string) error {
		if len(f) != 1 || !validKeyID(f[0]) {
			return fmt.Errorf("%w: line %d: want one key id", ErrMalformed, n)
		}
		if len(ids) >= maxEntries {
			return fmt.Errorf("%w: more than %d entries", ErrMalformed, maxEntries)
		}
		if !seen[f[0]] {
			seen[f[0]] = true
			ids = append(ids, f[0])
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ids, nil
}

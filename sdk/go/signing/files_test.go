package signing_test

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/nself-org/cli/sdk/go/v2/signing"
)

func pubB64(seed string) (ed25519.PublicKey, string) {
	h := sha256.Sum256([]byte(seed))
	return h[:], base64.StdEncoding.EncodeToString(h[:])
}

func TestParseKeysFileOK(t *testing.T) {
	p1, b1 := pubB64("one")
	p2, b2 := pubB64("two")
	in := "# trust file\n\n   \t\n" +
		"  # indented comment\n" +
		"alpha " + b1 + "\n" +
		"\tbeta\t \t" + b2 + "\t\r\n" +
		"\r\n"
	keys, err := signing.ParseKeysFile(strings.NewReader(in), signing.PurposeCIRelease, signing.KeysScope("tier"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys", len(keys))
	}
	if keys[0].ID != "alpha" || keys[1].ID != "beta" || !bytes.Equal(keys[0].Public, p1) || !bytes.Equal(keys[1].Public, p2) {
		t.Fatalf("wrong keys: %+v", keys)
	}
	for _, k := range keys {
		if k.Purpose != signing.PurposeCIRelease || k.Scope != "tier" {
			t.Fatalf("purpose/scope not set: %+v", k)
		}
	}
	// No trailing newline; empty file.
	if k, err := signing.ParseKeysFile(strings.NewReader("a "+b1), signing.PurposeAgent); err != nil || len(k) != 1 || k[0].Scope != "" {
		t.Fatalf("no trailing newline: %v %v", k, err)
	}
	if k, err := signing.ParseKeysFile(strings.NewReader(""), signing.PurposeAgent); err != nil || len(k) != 0 {
		t.Fatalf("empty: %v %v", k, err)
	}
}

func TestParseKeysFileRejects(t *testing.T) {
	p, b := pubB64("x")
	_, b31 := func() (int, string) { return 0, base64.StdEncoding.EncodeToString(p[:31]) }()
	b33 := base64.StdEncoding.EncodeToString(append(append([]byte{}, p...), 0))
	cases := map[string]string{
		"duplicate id":     "a " + b + "\na " + b + "\n",
		"bad base64":       "a !!!\n",
		"url-safe base64":  "a " + strings.NewReplacer("+", "-", "/", "_").Replace(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xfb\xff", 16)))) + "\n",
		"unpadded":         "a " + strings.TrimRight(b, "=") + "\n",
		"short key":        "a " + b31 + "\n",
		"long key":         "a " + b33 + "\n",
		"one field":        "a\n",
		"three fields":     "a " + b + " extra\n",
		"trailing comment": "a " + b + " # note\n",
		"bad id":           "a/b " + b + "\n",
		"nul in id":        "a\x00 " + b + "\n",
		"bom":              "\xef\xbb\xbfa " + b + "\n",
		"bare cr inside":   "a\r" + b + "\n",
		"long id":          strings.Repeat("a", 129) + " " + b + "\n",
		"error after good": "a " + b + "\nbroken\n",
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			keys, err := signing.ParseKeysFile(strings.NewReader(in), signing.PurposePlugins)
			only(t, err, signing.ErrMalformed)
			if keys != nil {
				t.Fatal("keys returned on error")
			}
		})
	}
	_, err := signing.ParseKeysFile(strings.NewReader(""), "bogus")
	only(t, err, signing.ErrMalformed)
	_, err = signing.ParseKeysFile(nil, signing.PurposePlugins)
	only(t, err, signing.ErrMalformed)
	// 128 character id accepted.
	if _, err := signing.ParseKeysFile(strings.NewReader(strings.Repeat("a", 128)+" "+b), signing.PurposePlugins); err != nil {
		t.Fatal(err)
	}
	// Reader errors are returned, not swallowed and not a sentinel success.
	bad := io.MultiReader(strings.NewReader("a "+b+"\n"), errReader{})
	if keys, err := signing.ParseKeysFile(bad, signing.PurposePlugins); err == nil || keys != nil {
		t.Fatal("reader error ignored")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("disk gone") }

func TestRequireDerivedID(t *testing.T) {
	pub, b := pubB64("derive")
	id := signing.KeyID(signing.PurposeCIRelease, pub)
	k, err := signing.ParseKeysFile(strings.NewReader(id+" "+b+"\n"), signing.PurposeCIRelease, signing.RequireDerivedID())
	if err != nil || len(k) != 1 {
		t.Fatal(err)
	}
	for _, line := range []string{
		"custom " + b,
		id + "0 " + b,
		strings.ToUpper(id) + " " + b,
		signing.KeyID(signing.PurposeAgent, pub) + " " + b, // another purpose's derivation
	} {
		keys, err := signing.ParseKeysFile(strings.NewReader(line+"\n"), signing.PurposeCIRelease, signing.RequireDerivedID())
		only(t, err, signing.ErrMalformed)
		if keys != nil {
			t.Fatal("keys returned on error")
		}
	}
	// Without the option a custom id is accepted.
	if _, err := signing.ParseKeysFile(strings.NewReader("custom "+b+"\n"), signing.PurposeCIRelease, nil); err != nil {
		t.Fatal(err)
	}
}

func TestParseKeysFileLimits(t *testing.T) {
	_, b := pubB64("limit")
	line := func(n int) string { return "#" + strings.Repeat("c", n-1) }
	// Line length: 4096 is allowed (LF, CRLF, and at EOF), 4097 is not.
	for _, ok := range []string{line(4096) + "\n", line(4096) + "\r\n", line(4096)} {
		if _, err := signing.ParseKeysFile(strings.NewReader(ok), signing.PurposePlugins); err != nil {
			t.Fatalf("4096-byte line rejected: %v", err)
		}
	}
	for _, bad := range []string{line(4097) + "\n", line(4097), line(4098) + "\n", line(10000) + "\n"} {
		_, err := signing.ParseKeysFile(strings.NewReader(bad), signing.PurposePlugins)
		only(t, err, signing.ErrMalformed)
	}
	// File size: exactly 1 MiB of comments is allowed, one byte more is not.
	filler := func(total int) string {
		var sb strings.Builder
		for sb.Len() < total-1024 {
			sb.WriteString(line(1023) + "\n")
		}
		sb.WriteString(line(total - sb.Len()))
		return sb.String()
	}
	if s := filler(1 << 20); len(s) != 1<<20 {
		t.Fatalf("filler is %d bytes", len(s))
	} else if _, err := signing.ParseKeysFile(strings.NewReader(s), signing.PurposePlugins); err != nil {
		t.Fatalf("1 MiB file rejected: %v", err)
	}
	for _, total := range []int{1<<20 + 1, 3 << 20} {
		_, err := signing.ParseKeysFile(strings.NewReader(filler(total)), signing.PurposePlugins)
		only(t, err, signing.ErrMalformed)
	}
	// Entry count: 1024 allowed, 1025 not.
	var sb strings.Builder
	for i := 0; i < 1025; i++ {
		fmt.Fprintf(&sb, "k%d %s\n", i, b)
	}
	all := strings.SplitAfter(sb.String(), "\n")
	if k, err := signing.ParseKeysFile(strings.NewReader(strings.Join(all[:1024], "")), signing.PurposePlugins); err != nil || len(k) != 1024 {
		t.Fatalf("1024 entries: %v", err)
	}
	_, err := signing.ParseKeysFile(strings.NewReader(strings.Join(all[:1025], "")), signing.PurposePlugins)
	only(t, err, signing.ErrMalformed)
}

func TestParseRevokedFile(t *testing.T) {
	ids, err := signing.ParseRevokedFile(strings.NewReader("# revoked\n\nb-1\r\n  a-2\t\nb-1\n#c\n"))
	if err != nil || strings.Join(ids, ",") != "b-1,a-2" {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	if ids, err := signing.ParseRevokedFile(strings.NewReader("")); err != nil || len(ids) != 0 {
		t.Fatalf("empty: %v %v", ids, err)
	}
	for name, in := range map[string]string{
		"two fields":    "a b\n",
		"bad id":        "a/b\n",
		"comment after": "a # note\n",
		"long id":       strings.Repeat("a", 129) + "\n",
		"good then bad": "ok\n!\n",
	} {
		t.Run(name, func(t *testing.T) {
			ids, err := signing.ParseRevokedFile(strings.NewReader(in))
			only(t, err, signing.ErrMalformed)
			if ids != nil {
				t.Fatal("ids returned on error")
			}
		})
	}
	_, err = signing.ParseRevokedFile(nil)
	only(t, err, signing.ErrMalformed)
	var sb strings.Builder
	for i := 0; i < 1025; i++ {
		fmt.Fprintf(&sb, "k%d\n", i)
	}
	all := strings.SplitAfter(sb.String(), "\n")
	if ids, err := signing.ParseRevokedFile(strings.NewReader(strings.Join(all[:1024], ""))); err != nil || len(ids) != 1024 {
		t.Fatalf("1024: %v", err)
	}
	_, err = signing.ParseRevokedFile(strings.NewReader(strings.Join(all[:1025], "")))
	only(t, err, signing.ErrMalformed)
	// A repeated id does not count twice toward the limit.
	if ids, err := signing.ParseRevokedFile(strings.NewReader(strings.Repeat("same\n", 5000))); err != nil || len(ids) != 1 {
		t.Fatalf("repeats: %v %v", ids, err)
	}
	_, err = signing.ParseRevokedFile(io.MultiReader(strings.NewReader("a\n"), errReader{}))
	if err == nil {
		t.Fatal("reader error ignored")
	}
}

// Parsed files feed the verifier end to end.
func TestParsedFilesDriveVerifier(t *testing.T) {
	k, s := newKeyPair(t, signing.PurposeCIRelease)
	keys, err := signing.ParseKeysFile(strings.NewReader(k.ID+" "+base64.StdEncoding.EncodeToString(k.Public)+"\n"), signing.PurposeCIRelease, signing.RequireDerivedID())
	if err != nil {
		t.Fatal(err)
	}
	sg := sign(t, s, msg)
	only(t, newVerifier(t, signing.PurposeCIRelease, keys, nil).Verify(msg, sg), nil)
	rev, _ := signing.ParseRevokedFile(strings.NewReader(k.ID + "\n"))
	only(t, newVerifier(t, signing.PurposeCIRelease, keys, rev).Verify(msg, sg), signing.ErrRevoked)
}

// Errors name the 1-based line that failed.
func TestFileErrorsNameTheLine(t *testing.T) {
	_, b := pubB64("line")
	_, err := signing.ParseKeysFile(strings.NewReader("# c\n\na "+b+"\nbroken\n"), signing.PurposePlugins)
	if err == nil || !strings.Contains(err.Error(), "line 4:") {
		t.Fatalf("err = %v", err)
	}
	_, err = signing.ParseRevokedFile(strings.NewReader("a\n/\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2:") {
		t.Fatalf("err = %v", err)
	}
}

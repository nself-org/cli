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

// entry returns the derived key id and the "<id> <b64>" file text for a seed.
func entry(p signing.Purpose, seed string) (id string, pub ed25519.PublicKey, text string) {
	pub, b := pubB64(seed)
	id = signing.KeyID(p, pub)
	return id, pub, id + " " + b
}

func TestParseKeysFileOK(t *testing.T) {
	id1, p1, l1 := entry(signing.PurposeCIRelease, "one")
	id2, p2, l2 := entry(signing.PurposeCIRelease, "two")
	b2 := strings.Fields(l2)[1]
	in := "# trust file\n\n   \t\n" +
		"  # indented comment\n" +
		l1 + "\n" +
		"\t" + id2 + "\t \t" + b2 + "\t\r\n" +
		"\r\n"
	keys, err := signing.ParseKeysFile(strings.NewReader(in), signing.PurposeCIRelease, signing.KeysScope("tier"))
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 {
		t.Fatalf("got %d keys", len(keys))
	}
	if keys[0].ID != id1 || keys[1].ID != id2 || !bytes.Equal(keys[0].Public, p1) || !bytes.Equal(keys[1].Public, p2) {
		t.Fatalf("wrong keys: %+v", keys)
	}
	for _, k := range keys {
		if k.Purpose != signing.PurposeCIRelease || k.Scope != "tier" {
			t.Fatalf("purpose/scope not set: %+v", k)
		}
	}
	_, _, la := entry(signing.PurposeAgent, "a")
	if k, err := signing.ParseKeysFile(strings.NewReader(la), signing.PurposeAgent); err != nil || len(k) != 1 || k[0].Scope != "" {
		t.Fatalf("no trailing newline: %v %v", k, err)
	}
	if k, err := signing.ParseKeysFile(strings.NewReader(""), signing.PurposeAgent); err != nil || len(k) != 0 {
		t.Fatalf("empty: %v %v", k, err)
	}
}

func TestParseKeysFileRejects(t *testing.T) {
	id, p, l := entry(signing.PurposePlugins, "x")
	b := strings.Fields(l)[1]
	short := base64.StdEncoding.EncodeToString(p[:31])
	long := base64.StdEncoding.EncodeToString(append(append([]byte{}, p...), 0))
	cases := map[string]string{
		"duplicate id":            l + "\n" + l + "\n",
		"duplicate key, other id": l + "\n" + signing.KeyID(signing.PurposeAgent, p) + " " + b + "\n",
		"bad base64":              id + " !!!\n",
		"url-safe base64":         id + " " + strings.NewReplacer("+", "-", "/", "_").Replace(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("\xfb\xff", 16)))) + "\n",
		"unpadded":                id + " " + strings.TrimRight(b, "=") + "\n",
		"short key":               id + " " + short + "\n",
		"long key":                id + " " + long + "\n",
		"one field":               id + "\n",
		"three fields":            l + " extra\n",
		"trailing comment":        l + " # note\n",
		"bad id":                  "a/b " + b + "\n",
		"nul in id":               "a\x00 " + b + "\n",
		"bom":                     "\xef\xbb\xbf" + l + "\n",
		"bare cr inside":          id + "\r" + b + "\n",
		"long id":                 strings.Repeat("a", 129) + " " + b + "\n",
		"error after good":        l + "\nbroken\n",
		"custom id":               "custom " + b + "\n",
		"id off by one":           id + "0 " + b + "\n",
		"upper-case id":           strings.ToUpper(id) + " " + b + "\n",
		"other purpose id":        signing.KeyID(signing.PurposeAgent, p) + " " + b + "\n",
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
	// Reader errors are returned, not swallowed.
	bad := io.MultiReader(strings.NewReader(l+"\n"), errReader{})
	if keys, err := signing.ParseKeysFile(bad, signing.PurposePlugins); err == nil || keys != nil {
		t.Fatal("reader error ignored")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, fmt.Errorf("disk gone") }

func TestParseKeysFileLimits(t *testing.T) {
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
		_, _, e := entry(signing.PurposePlugins, fmt.Sprintf("e%d", i))
		sb.WriteString(e + "\n")
	}
	all := strings.SplitAfter(sb.String(), "\n")
	if k, err := signing.ParseKeysFile(strings.NewReader(strings.Join(all[:1024], "")), signing.PurposePlugins); err != nil || len(k) != 1024 {
		t.Fatalf("1024 entries: %v", err)
	}
	_, err := signing.ParseKeysFile(strings.NewReader(strings.Join(all[:1025], "")), signing.PurposePlugins)
	only(t, err, signing.ErrMalformed)
}

// rid is a well-formed derived-shape id for tests.
func rid(p signing.Purpose, n int) string { return fmt.Sprintf("%s-%016x", p, n) }

// Revocation ids that can never match a derived key id are refused.
var badRevokedIDs = map[string]string{
	"upper-case hex":      "plugins-0123456789ABCDEF",
	"custom id":           "release-2026",
	"unknown purpose":     "other-0123456789abcdef",
	"no purpose":          "0123456789abcdef",
	"short hex":           "plugins-0123456789abcde",
	"long hex":            "plugins-0123456789abcdef0",
	"non-hex":             "plugins-0123456789abcdeg",
	"non-hex high":        "plugins-0123456789abcde:",
	"non-hex low":         "plugins-0123456789abcde/",
	"prefix only":         "plugins-",
	"purpose no dash":     "plugins0123456789abcdef",
	"upper-case purpose":  "Plugins-0123456789abcdef",
	"purpose is a prefix": "ci-0123456789abcdef",
}

func TestParseRevokedFile(t *testing.T) {
	a, b := rid(signing.PurposeCIRelease, 1), rid(signing.PurposePlugins, 0xabcdef)
	ids, err := signing.ParseRevokedFile(strings.NewReader("# revoked\n\n" + a + "\r\n  " + b + "\t\n" + a + "\n#c\n"))
	if err != nil || strings.Join(ids, ",") != a+","+b {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	if ids, err := signing.ParseRevokedFile(strings.NewReader("")); err != nil || len(ids) != 0 {
		t.Fatalf("empty: %v %v", ids, err)
	}
	for _, p := range allPurposes {
		if _, err := signing.ParseRevokedFile(strings.NewReader(rid(p, 7) + "\n")); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
	cases := map[string]string{
		"two fields":    a + " " + b + "\n",
		"comment after": a + " # note\n",
		"good then bad": a + "\n!\n",
	}
	for name, id := range badRevokedIDs {
		cases[name] = id + "\n"
		cases[name+" after good"] = a + "\n" + id + "\n"
	}
	for name, in := range cases {
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
		sb.WriteString(rid(signing.PurposeAgent, i) + "\n")
	}
	all := strings.SplitAfter(sb.String(), "\n")
	if ids, err := signing.ParseRevokedFile(strings.NewReader(strings.Join(all[:1024], ""))); err != nil || len(ids) != 1024 {
		t.Fatalf("1024: %v", err)
	}
	_, err = signing.ParseRevokedFile(strings.NewReader(strings.Join(all[:1025], "")))
	only(t, err, signing.ErrMalformed)
	// A repeated id does not count twice toward the limit.
	if ids, err := signing.ParseRevokedFile(strings.NewReader(strings.Repeat(a+"\n", 5000))); err != nil || len(ids) != 1 {
		t.Fatalf("repeats: %v %v", ids, err)
	}
	_, err = signing.ParseRevokedFile(io.MultiReader(strings.NewReader(a+"\n"), errReader{}))
	if err == nil {
		t.Fatal("reader error ignored")
	}
}

// NewVerifier refuses the same unmatchable revocation entries.
func TestNewVerifierRefusesUnmatchableRevocations(t *testing.T) {
	for name, id := range badRevokedIDs {
		t.Run(name, func(t *testing.T) {
			_, err := signing.NewVerifier(signing.PurposePlugins, nil, []string{rid(signing.PurposePlugins, 1), id})
			only(t, err, signing.ErrMalformed)
		})
	}
	for _, p := range allPurposes {
		if _, err := signing.NewVerifier(signing.PurposePlugins, nil, []string{rid(p, 255)}); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

// Parsed files feed the verifier end to end.
func TestParsedFilesDriveVerifier(t *testing.T) {
	k, s := newKeyPair(t, signing.PurposeCIRelease)
	keys, err := signing.ParseKeysFile(strings.NewReader(k.ID+" "+base64.StdEncoding.EncodeToString(k.Public)+"\n"), signing.PurposeCIRelease)
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
	_, _, e := entry(signing.PurposePlugins, "line")
	_, err := signing.ParseKeysFile(strings.NewReader("# c\n\n"+e+"\nbroken\n"), signing.PurposePlugins)
	if err == nil || !strings.Contains(err.Error(), "line 4:") {
		t.Fatalf("err = %v", err)
	}
	_, err = signing.ParseRevokedFile(strings.NewReader(rid(signing.PurposeAgent, 1) + "\n/\n"))
	if err == nil || !strings.Contains(err.Error(), "line 2:") {
		t.Fatalf("err = %v", err)
	}
}

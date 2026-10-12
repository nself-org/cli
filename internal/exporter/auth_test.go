package exporter

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

// fakeBcrypt has the full bcrypt shape but is obviously not a real hash.
const fakeBcrypt = "$2b$10$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0"

type fakeUserRows struct {
	data                []string
	index               int
	scanErr, errorAtEnd error
}

func (r *fakeUserRows) Next() bool { return r.index < len(r.data) }
func (r *fakeUserRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	*dest[0].(*[]byte) = []byte(r.data[r.index])
	r.index++
	return nil
}
func (r *fakeUserRows) Err() error { return r.errorAtEnd }

func TestAuthUnknownHashAlgorithm(t *testing.T) {
	rows := &fakeUserRows{data: []string{`{"id":"fake-user-3","password_hash":"fake-argon-hash"}`, `{"id":"fake-user-4"}`}}
	var b bytes.Buffer
	a := portable.Auth{HashAlgorithms: map[string]int{}}
	if err := streamAuth(rows, &b, &a); err != nil {
		t.Fatal(err)
	}
	if a.HashAlgorithms["unknown"] != 1 || a.HashAlgorithms["none"] != 1 || a.HashAlgorithms["bcrypt"] != 0 {
		t.Fatal("wrong algorithm classes", a.HashAlgorithms)
	}
}

func TestAuthPortableFields(t *testing.T) {
	rows := &fakeUserRows{data: []string{`{"id":"fake-user-1","password_hash":"` + fakeBcrypt + `","metadata":{"fixture":true},"roles":["user"]}`, `{"id":"fake-user-2","password_hash":null}`}}
	var b bytes.Buffer
	a := portable.Auth{HashAlgorithms: map[string]int{}}
	if err := streamAuth(rows, &b, &a); err != nil {
		t.Fatal(err)
	}
	if a.Users != 2 || a.HashAlgorithms["bcrypt"] != 1 || a.HashAlgorithms["none"] != 1 {
		t.Fatal("wrong auth counts")
	}
	if strings.Count(b.String(), "\n") != 2 || !strings.Contains(b.String(), `"roles":["user"]`) {
		t.Fatal("portable auth fields lost")
	}
	for _, rows := range []*fakeUserRows{{data: []string{`{`}}, {data: []string{`{}`}, scanErr: errors.New("fake scan failure")}, {errorAtEnd: errors.New("fake row failure")}} {
		if err := streamAuth(rows, &b, &a); err == nil {
			t.Fatal("auth read failed open")
		}
	}
}

// Integers past float64 precision must reach users.jsonl unchanged.
func TestAuthKeepsLargeIntegers(t *testing.T) {
	rows := &fakeUserRows{data: []string{`{"id":"fake-user-5","metadata":{"acct":12345678901234567891,"ratio":1.50,"big":[98765432109876543210]}}`}}
	var out bytes.Buffer
	a := portable.Auth{HashAlgorithms: map[string]int{}}
	if err := streamAuth(rows, &out, &a); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"acct":12345678901234567891`, `"ratio":1.50`, `98765432109876543210`} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("number corrupted, want %s in %s", want, out.String())
		}
	}
}

func TestBcryptShape(t *testing.T) {
	if !bcryptHash.MatchString(fakeBcrypt) {
		t.Fatal("full bcrypt shape rejected")
	}
	for _, bad := range []string{"", "$2b$FAKE-HASH", "$2b$10$short", fakeBcrypt + "x", "x" + fakeBcrypt, "$2c$10$" + fakeBcrypt[7:]} {
		if bcryptHash.MatchString(bad) {
			t.Fatalf("%q accepted as bcrypt", bad)
		}
	}
}

package exporter

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/nself-org/cli/internal/portable"
)

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
	rows := &fakeUserRows{data: []string{`{"id":"fake-user-1","password_hash":"$2b$FAKE-HASH","metadata":{"fixture":true},"roles":["user"]}`, `{"id":"fake-user-2","password_hash":null}`}}
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

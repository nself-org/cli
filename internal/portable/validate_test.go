package portable

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sumHex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// objectManifest is a valid manifest with one stored object bound to its file.
func objectManifest() Manifest {
	m := baseManifest()
	m.Format, m.SchemaVersion, m.CreatedAt = Format, SchemaVersion, "2026-10-05T12:00:00Z"
	mem, _ := StorageMember("b", "k")
	m.Files = []File{{Path: mem, SHA256: sumHex("x"), Bytes: 1}}
	m.Storage.Objects = []Object{{Bucket: "b", Key: "k", Member: mem, SHA256: sumHex("x"), Bytes: 1}}
	m.Sort()
	return m
}

func TestValidateBindsObjectsToFiles(t *testing.T) {
	base := objectManifest()
	if err := base.Validate(); err != nil {
		t.Fatalf("the base manifest must be valid: %v", err)
	}
	other, _ := StorageMember("b", "other")
	cases := map[string]func(m *Manifest){
		"object with no member in files":  func(m *Manifest) { m.Files = nil },
		"object naming another member":    func(m *Manifest) { m.Storage.Objects[0].Member = other },
		"object with an empty member":     func(m *Manifest) { m.Storage.Objects[0].Member = "" },
		"object sha256 differs from file": func(m *Manifest) { m.Storage.Objects[0].SHA256 = sumHex("y") },
		"object bytes differ from file":   func(m *Manifest) { m.Storage.Objects[0].Bytes = 2 },
		"object listed twice":             func(m *Manifest) { m.Storage.Objects = append(m.Storage.Objects, m.Storage.Objects[0]) },
		"object with a bad sha256":        func(m *Manifest) { m.Storage.Objects[0].SHA256 = "ABC" },
		"object with an empty key":        func(m *Manifest) { m.Storage.Objects[0].Key = "" },
		"table listed twice":              func(m *Manifest) { m.DB.Tables = append(m.DB.Tables, m.DB.Tables[0]) },
		"table listed twice, other hash":  func(m *Manifest) { t2 := m.DB.Tables[0]; t2.Hash = "7"; m.DB.Tables = append(m.DB.Tables, t2) },
		"table hash with leading zeros":   func(m *Manifest) { m.DB.Tables[0].Hash = "007" },
		"table hash minus zero":           func(m *Manifest) { m.DB.Tables[0].Hash = "-0" },
		"table hash with a plus sign":     func(m *Manifest) { m.DB.Tables[0].Hash = "+5" },
		"table hash empty":                func(m *Manifest) { m.DB.Tables[0].Hash = "" },
		"member not in NFC": func(m *Manifest) {
			m.Files = append(m.Files, File{Path: "db/café.sql", SHA256: sumHex("a"), Bytes: 1})
		},
		"members equal after NFC and fold": func(m *Manifest) {
			m.Files = append(m.Files, File{Path: "db/A.sql", SHA256: sumHex("a"), Bytes: 1}, File{Path: "db/a.SQL", SHA256: sumHex("a"), Bytes: 1})
		},
		"members equal under full folding": func(m *Manifest) {
			m.Files = append(m.Files, File{Path: "db/straße", SHA256: sumHex("a"), Bytes: 1}, File{Path: "db/STRASSE", SHA256: sumHex("a"), Bytes: 1})
		},
	}
	for name, mutate := range cases {
		m := objectManifest()
		mutate(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: Validate must refuse", name)
		}
	}
	for _, h := range []string{"0", "5", "-5", "59416778138680917247", "-9223372036854775808"} {
		m := objectManifest()
		m.DB.Tables[0].Hash = h
		if err := m.Validate(); err != nil {
			t.Errorf("canonical hash %q refused: %v", h, err)
		}
	}
	m := objectManifest()
	m.Files = append(m.Files, File{Path: "db/a.sql", SHA256: sumHex("a"), Bytes: 1}, File{Path: "db/b.sql", SHA256: sumHex("a"), Bytes: 1})
	if err := m.Validate(); err != nil {
		t.Errorf("distinct members must pass: %v", err)
	}
	m = objectManifest()
	m.Files = append(m.Files, File{Path: "db/A.sql", SHA256: sumHex("a"), Bytes: 1}, File{Path: "db/a.sql", SHA256: sumHex("a"), Bytes: 1})
	if err := m.Validate(); !errors.Is(err, ErrDuplicate) {
		t.Errorf("case duplicate: want ErrDuplicate, got %v", err)
	}
}

func TestCheckMemberRequiresNFC(t *testing.T) {
	if err := CheckMember("db/café.sql"); err != nil {
		t.Errorf("NFC name refused: %v", err)
	}
	err := CheckMember("db/café.sql")
	if !errors.Is(err, ErrUnsafePath) || !strings.Contains(err.Error(), "NFC") {
		t.Errorf("NFD name: want an NFC refusal, got %v", err)
	}
	w, _ := NewWriter(filepath.Join(t.TempDir(), "b"))
	if _, err := w.WriteFile("db/café.sql", strings.NewReader("x")); err == nil {
		t.Error("the Writer must refuse a non-NFC member name")
	}
	if _, err := w.WriteFile("db/café.sql", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteFile("db/CAFÉ.sql", strings.NewReader("x")); !errors.Is(err, ErrDuplicate) {
		t.Errorf("a case variant of an NFC name must be a duplicate, got %v", err)
	}
}

// rawManifest rewrites manifest.json text through edit.
func rawManifest(t *testing.T, dir string, edit func(s string) string) {
	t.Helper()
	p := filepath.Join(dir, ManifestName)
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(edit(string(b))), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestManifestDecodeIsStrict(t *testing.T) {
	cases := []struct {
		name string
		edit func(string) string
		want bool // true: Open must refuse
	}{
		{"unknown top-level field at 1", func(s string) string { return strings.Replace(s, `"compat"`, `"surprise": 1, "compat"`, 1) }, true},
		{"unknown nested field at 1", func(s string) string { return strings.Replace(s, `"tool"`, `"extra": "x", "tool"`, 1) }, true},
		{"unknown field at 1.0", func(s string) string {
			return strings.Replace(strings.Replace(s, `"schema_version": "1"`, `"schema_version": "1.0"`, 1), `"compat"`, `"x": 1, "compat"`, 1)
		}, true},
		{"unknown field at minor 1.2 is tolerated", func(s string) string {
			return strings.Replace(strings.Replace(s, `"schema_version": "1"`, `"schema_version": "1.2"`, 1), `"compat"`, `"x": 1, "compat"`, 1)
		}, false},
		{"duplicate top-level key", func(s string) string {
			return strings.Replace(s, `"compat"`, `"created_at": "2020-01-01T00:00:00Z", "compat"`, 1)
		}, true},
		{"duplicate nested key", func(s string) string {
			return strings.Replace(s, `"tool": "nself"`, `"tool": "nself", "tool": "other"`, 1)
		}, true},
		{"case-variant key overriding a field", func(s string) string {
			return strings.Replace(s, `"compat"`, `"CREATED_AT": "2020-01-01T00:00:00Z", "compat"`, 1)
		}, true},
		{"only a case-variant spelling of a field", func(s string) string { return strings.Replace(s, `"created_at"`, `"Created_At"`, 1) }, true},
		{"case-variant in an array element", func(s string) string { return strings.Replace(s, `"schema":`, `"Schema":`, 1) }, true},
		{"case-variant at a tolerated minor", func(s string) string {
			return strings.Replace(strings.Replace(s, `"schema_version": "1"`, `"schema_version": "1.1"`, 1), `"compat"`, `"COMPAT": [], "compat"`, 1)
		}, true},
		{"duplicate key at a tolerated minor", func(s string) string {
			return strings.Replace(strings.Replace(s, `"schema_version": "1"`, `"schema_version": "1.1"`, 1), `"compat"`, `"x": 1, "x": 2, "compat"`, 1)
		}, true},
		{"unchanged", func(s string) string { return s }, false},
	}
	for _, c := range cases {
		dir := fixture(t)
		rawManifest(t, dir, c.edit)
		_, err := Open(dir)
		if c.want {
			wantCode(t, err, "E516", ErrManifest)
		} else if err != nil {
			t.Errorf("%s: Open refused: %v", c.name, err)
		}
	}
}

func TestManifestHashAlgorithmsKeepCaseDistinctNames(t *testing.T) {
	dir := fixture(t)
	rawManifest(t, dir, func(s string) string {
		return strings.Replace(s, `"hash_algorithms": {}`, `"hash_algorithms": {"bcrypt": 1, "BCRYPT": 2}`, 1)
	})
	if _, err := Open(dir); err != nil {
		t.Fatalf("hash_algorithms keys are data: %v", err)
	}
	dir = fixture(t)
	rawManifest(t, dir, func(s string) string {
		return strings.Replace(s, `"hash_algorithms": {}`, `"hash_algorithms": {"bcrypt": 1, "bcrypt": 2}`, 1)
	})
	_, err := Open(dir)
	wantCode(t, err, "E516", ErrManifest)
}

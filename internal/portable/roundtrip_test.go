package portable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"encoding/json"
	"github.com/google/jsonschema-go/jsonschema"
)

// randManifest builds a random manifest body and member files from rng.
func randManifest(rng *rand.Rand) (Manifest, map[string][]byte) {
	word := func() string { return fmt.Sprintf("w%d", rng.Intn(1000)) }
	m := baseManifest()
	m.DB.Schemas = nil
	for i := 0; i < rng.Intn(3)+1; i++ {
		m.DB.Schemas = append(m.DB.Schemas, "s"+word())
	}
	m.DB.Tables = nil
	for i := 0; i < rng.Intn(6); i++ {
		m.DB.Tables = append(m.DB.Tables, Table{Schema: "s" + word(), Name: "t" + word(), Rows: int64(rng.Intn(1e6)),
			Hash: fmt.Sprintf("%d", rng.Int63()-rng.Int63()), PK: []string{"c" + word(), "d" + word()}})
	}
	m.Auth = Auth{Users: rng.Intn(50), HashAlgorithms: map[string]int{"bcrypt": rng.Intn(9), "argon2": rng.Intn(9)}}
	for i := 0; i < rng.Intn(4); i++ {
		m.Auth.ResetRequired = append(m.Auth.ResetRequired, word())
	}
	files := map[string][]byte{}
	for i := 0; i < rng.Intn(8); i++ {
		b := make([]byte, rng.Intn(4096))
		rng.Read(b)
		files[fmt.Sprintf("db/f%s-%d.bin", word(), i)] = b
	}
	for i := 0; i < rng.Intn(4); i++ {
		key := fmt.Sprintf("dir %d/ключ+%%%s.txt", i, word())
		mem, err := StorageMember("bkt", key)
		if err != nil {
			panic(err)
		}
		b := make([]byte, rng.Intn(64))
		rng.Read(b)
		files[mem] = b
		sum := sha256.Sum256(b)
		m.Storage.Objects = append(m.Storage.Objects, Object{Bucket: "bkt", Key: key, Member: mem, SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(b))})
	}
	m.Exemptions = []Exemption{{Kind: "table", ID: "z" + word(), Reason: "a"}, {Kind: "user", ID: "a" + word(), Reason: "b"}}
	m.Compat = []string{"b" + word(), "a" + word()}
	return m, files
}

// writeAll writes files in the order of keys and finishes with m.
func writeAll(t *testing.T, dir string, m Manifest, files map[string][]byte, order []string) Manifest {
	t.Helper()
	w, err := NewWriter(dir)
	if err != nil {
		t.Fatal(err)
	}
	w.Now = fixedNow
	for _, rel := range order {
		if _, err := w.WriteFile(rel, bytes.NewReader(files[rel])); err != nil {
			t.Fatal(err)
		}
	}
	out, err := w.Finish(m)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestWriterReaderRoundTrip: 500 seeded random bundles survive Write -> Open
// losslessly, the manifest bytes are identical across two runs (members written
// in different orders), and the manifest validates against the schema.
func TestWriterReaderRoundTrip(t *testing.T) {
	schema := loadSchema(t)
	for seed := int64(0); seed < 500; seed++ {
		rng := rand.New(rand.NewSource(seed))
		m, files := randManifest(rng)
		var order []string
		for rel := range files {
			order = append(order, rel)
		}
		d1, d2 := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
		want := writeAll(t, d1, m, files, order)
		rng.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
		writeAll(t, d2, m, files, order)

		b1, _ := os.ReadFile(filepath.Join(d1, ManifestName))
		b2, _ := os.ReadFile(filepath.Join(d2, ManifestName))
		if !bytes.Equal(b1, b2) {
			t.Fatalf("seed %d: manifest differs between runs", seed)
		}
		if seed%25 == 0 {
			validateAgainst(t, schema, b1)
		}
		r, err := Open(d1)
		if err != nil {
			t.Fatalf("seed %d: Open: %v", seed, err)
		}
		if got := r.Manifest(); !reflect.DeepEqual(got, want) {
			t.Fatalf("seed %d: manifest changed through the round trip", seed)
		}
		for rel, body := range files {
			rc, err := r.Open(rel)
			if err != nil {
				t.Fatalf("seed %d: Open(%q): %v", seed, rel, err)
			}
			got, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil || !bytes.Equal(got, body) {
				t.Fatalf("seed %d: %q bytes differ (err %v)", seed, rel, err)
			}
		}
		if err := r.Verify(); err != nil {
			t.Fatalf("seed %d: Verify: %v", seed, err)
		}
	}
}

func loadSchema(t *testing.T) *jsonschema.Resolved {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "schemas", "portable-export.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s jsonschema.Schema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	r, err := s.Resolve(nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func validateAgainst(t *testing.T, r *jsonschema.Resolved, doc []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(doc, &v); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(v); err != nil {
		t.Fatalf("manifest does not validate against the generated schema: %v", err)
	}
}

// TestManifestSchemaRejects: the generated schema is not vacuous.
func TestManifestSchemaRejects(t *testing.T) {
	r := loadSchema(t)
	dir := makeBundle(t, map[string]string{"db/data.dump": "x"})
	good, _ := os.ReadFile(filepath.Join(dir, ManifestName))
	validateAgainst(t, r, good)
	bad := map[string]func(m map[string]any){
		"version 2":     func(m map[string]any) { m["schema_version"] = "2" },
		"wrong marker":  func(m map[string]any) { m["_format"] = "other" },
		"extra field":   func(m map[string]any) { m["secret"] = "x" },
		"missing files": func(m map[string]any) { delete(m, "files") },
		"bad sha":       func(m map[string]any) { m["files"].([]any)[0].(map[string]any)["sha256"] = "XYZ" },
		"bad source":    func(m map[string]any) { m["producer"].(map[string]any)["source"] = "mysql" },
	}
	for name, edit := range bad {
		var m map[string]any
		_ = json.Unmarshal(good, &m)
		edit(m)
		raw, _ := json.Marshal(m)
		var v any
		_ = json.Unmarshal(raw, &v)
		if err := r.Validate(v); err == nil {
			t.Errorf("%s: schema accepted an invalid manifest", name)
		}
	}
}

// TestManifestSortCanonical: Sort is idempotent and order-insensitive.
func TestManifestSortCanonical(t *testing.T) {
	m, _ := randManifest(rand.New(rand.NewSource(7)))
	a, b := m, m
	a.Sort()
	rev := append([]Table(nil), b.DB.Tables...)
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	b.DB.Tables = rev
	b.Sort()
	ja, _ := MarshalManifest(a)
	jb, _ := MarshalManifest(b)
	if !bytes.Equal(ja, jb) {
		t.Fatal("Sort is order dependent")
	}
	a.Sort()
	jc, _ := MarshalManifest(a)
	if !bytes.Equal(ja, jc) {
		t.Fatal("Sort is not idempotent")
	}
}

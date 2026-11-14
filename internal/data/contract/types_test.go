// Tests for contract:data.lifecycle.v1: the embedded schema compiles, every Go
// type round-trips through JSON and validates, the schema and the Go json tags
// agree field by field (required = no omitempty), and invalid documents fail.
// Every test name starts with TestSchema so the ticket verify line runs them all.
package contract

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON()))
	if err != nil {
		t.Fatalf("schema.json is not JSON: %v", err)
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("schema.json", doc); err != nil {
		t.Fatal(err)
	}
	s, err := c.Compile("schema.json")
	if err != nil {
		t.Fatalf("schema.json does not compile: %v", err)
	}
	return s
}

func validate(t *testing.T, s *jsonschema.Schema, raw []byte) error {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("not JSON: %v\n%s", err, raw)
	}
	return s.Validate(v)
}

func ts(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC) }

func ptr(t time.Time) *time.Time { return &t }

// sample fills every field, optional ones included, so the round trip and the
// schema check cover the whole contract.
func sample() Document {
	return Document{
		BackupCatalog: &BackupCatalog{
			Stack: "demo-staging", GeneratedAt: ts(9, 12),
			Backups: []BackupEntry{{
				ID: "bk-20261109T020000Z", Kind: BackupFull, CreatedAt: ts(9, 2), ExpiresAt: ptr(ts(30, 2)),
				SizeBytes: 1048576, ChecksumSHA256: strings.Repeat("ab", 32),
				Encryption:   Encryption{Encrypted: true, Algorithm: "age", KeyRef: "keychain:nself/demo-backup"},
				Location:     Location{Destination: "r2", Ref: "backups/demo/bk-20261109T020000Z.tar.age"},
				Verification: Verification{Status: VerifyPassed, Method: VerifyRestore, VerifiedAt: ptr(ts(9, 3)), Detail: "rows and checksums match"},
				Tags:         []string{"pre-upgrade"},
			}, {
				ID: "bk-config", Kind: BackupConfig, CreatedAt: ts(8, 2), SizeBytes: 0, ChecksumSHA256: strings.Repeat("0", 64),
				Encryption: Encryption{Encrypted: false}, Location: Location{Destination: "local", Ref: "_backups/bk-config.tar"},
				Verification: Verification{Status: VerifyUnverified},
			}},
		},
		Retention: &RetentionPolicy{KeepLast: 3, KeepDaily: 7, KeepWeekly: 4, KeepMonthly: 6, MaxAgeDays: 400, KeepTagged: true},
		Purges: []PurgeStatus{
			{Subject: PurgeSubject{Kind: PurgeRow, Ref: "public.users/42"}, RequestedAt: ts(9, 5), TombstoneID: "tomb-1",
				DeletedInPrimary: true, DeletedInIndexes: true, PendingBackupExpiry: ptr(ts(30, 2))},
			{Subject: PurgeSubject{Kind: PurgeObject, Ref: "avatars/42.png"}, RequestedAt: ts(9, 6), Archived: true},
		},
		Pressure: &PressureTelemetry{
			Stack: "demo-staging", Engine: EngineSQLite, ObservedAt: ts(9, 12),
			WriteLockWaitMS: 12.5, QueueAgeSeconds: 3, QueryLatencyMS: Latency{P50: 1.2, P95: 8, P99: 40.5},
			Connections: Connections{InUse: 4, Max: 20}, Storage: StorageUsage{UsedBytes: 5 << 20, CapacityBytes: 10 << 30},
			BackupHealth: BackupHealth{Status: BackupHealthOK, LastSuccessAt: ptr(ts(9, 2))},
		},
	}
}

func TestSchemaIdentity(t *testing.T) {
	var doc map[string]any
	if err := json.Unmarshal(SchemaJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["$id"] != SchemaID || !strings.HasSuffix(SchemaID, ":v1") {
		t.Fatalf("$id = %v, want %s ending in :v1", doc["$id"], SchemaID)
	}
	if !strings.Contains(doc["title"].(string), "data.lifecycle.v1") || !strings.HasSuffix(ContractID, "data.lifecycle.v1") {
		t.Fatalf("title %v / ContractID %s do not name data.lifecycle.v1", doc["title"], ContractID)
	}
	// The version lives in $id only: no object anywhere declares a version field.
	var walk func(v any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if props, ok := x["properties"].(map[string]any); ok {
				for k := range props {
					if strings.Contains(strings.ToLower(k), "version") {
						t.Errorf("schema declares a version field %q", k)
					}
				}
			}
			for _, c := range x {
				walk(c)
			}
		case []any:
			for _, c := range x {
				walk(c)
			}
		}
	}
	walk(doc)
	got := SchemaJSON()
	got[0] = 'x'
	if SchemaJSON()[0] == 'x' {
		t.Fatal("SchemaJSON returns the embedded slice, not a copy")
	}
}

func TestSchemaRoundTrip(t *testing.T) {
	s := compileSchema(t)
	in := sample()
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := validate(t, s, raw); err != nil {
		t.Fatalf("marshalled sample fails the schema: %v\n%s", err, raw)
	}
	var out Document
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("round trip changed the document:\nin  %+v\nout %+v", in, out)
	}
	// Each section also stands alone: an empty document and each single section validate.
	for name, part := range map[string]Document{
		"empty":    {},
		"catalog":  {BackupCatalog: in.BackupCatalog},
		"retain":   {Retention: &RetentionPolicy{}},
		"purges":   {Purges: in.Purges},
		"pressure": {Pressure: in.Pressure},
	} {
		b, _ := json.Marshal(part)
		if err := validate(t, s, b); err != nil {
			t.Errorf("%s: %v\n%s", name, err, b)
		}
	}
}

// TestSchemaMatchesGoTypes: for each object in the schema, the property names
// equal the Go json tags and required equals the tags without omitempty.
func TestSchemaMatchesGoTypes(t *testing.T) {
	var doc struct {
		Properties map[string]any `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]any `json:"properties"`
			Required   []string       `json:"required"`
		} `json:"$defs"`
	}
	if err := json.Unmarshal(SchemaJSON(), &doc); err != nil {
		t.Fatal(err)
	}
	types := map[string]reflect.Type{
		"backup_catalog": reflect.TypeOf(BackupCatalog{}), "backup_entry": reflect.TypeOf(BackupEntry{}),
		"encryption": reflect.TypeOf(Encryption{}), "location": reflect.TypeOf(Location{}),
		"verification": reflect.TypeOf(Verification{}), "retention_policy": reflect.TypeOf(RetentionPolicy{}),
		"purge_status": reflect.TypeOf(PurgeStatus{}), "purge_subject": reflect.TypeOf(PurgeSubject{}),
		"pressure_telemetry": reflect.TypeOf(PressureTelemetry{}), "latency": reflect.TypeOf(Latency{}),
		"connections": reflect.TypeOf(Connections{}), "storage_usage": reflect.TypeOf(StorageUsage{}),
		"backup_health": reflect.TypeOf(BackupHealth{}),
	}
	for name, def := range doc.Defs {
		if def.Properties == nil {
			continue // scalar helper ($defs timestamp, count, stack_ref)
		}
		typ, ok := types[name]
		if !ok {
			t.Errorf("$defs/%s has no Go type in this test", name)
			continue
		}
		props, req := tagSets(typ)
		if got := keys(def.Properties); !reflect.DeepEqual(got, props) {
			t.Errorf("%s: schema properties %v, Go tags %v", name, got, props)
		}
		sort.Strings(def.Required)
		if !reflect.DeepEqual(def.Required, req) {
			t.Errorf("%s: schema required %v, Go non-omitempty %v", name, def.Required, req)
		}
		delete(types, name)
	}
	for name := range types {
		t.Errorf("Go type for %s has no $defs entry", name)
	}
	if props, req := tagSets(reflect.TypeOf(Document{})); !reflect.DeepEqual(keys(doc.Properties), props) || len(req) != 0 {
		t.Errorf("root properties %v, Document tags %v (required %v)", keys(doc.Properties), props, req)
	}
}

func tagSets(typ reflect.Type) (props, required []string) {
	props, required = []string{}, []string{}
	for i := 0; i < typ.NumField(); i++ {
		name, opts, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		props = append(props, name)
		if !strings.Contains(opts, "omitempty") {
			required = append(required, name)
		}
	}
	sort.Strings(props)
	sort.Strings(required)
	return props, required
}

func keys(m map[string]any) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestSchemaRejectsInvalid(t *testing.T) {
	s := compileSchema(t)
	raw, _ := json.Marshal(sample())
	cases := map[string]func(m map[string]any){
		"version field":      func(m map[string]any) { m["version"] = "v1" },
		"unknown section":    func(m map[string]any) { m["sync"] = map[string]any{} },
		"bad checksum":       func(m map[string]any) { backup(m)["checksum_sha256"] = "ABC" },
		"negative size":      func(m map[string]any) { backup(m)["size_bytes"] = -1 },
		"unknown kind":       func(m map[string]any) { backup(m)["kind"] = "snapshot" },
		"bad timestamp":      func(m map[string]any) { backup(m)["created_at"] = "yesterday" },
		"no pending expiry":  func(m map[string]any) { delete(purge(m), "pending_backup_expiry") },
		"no deleted flag":    func(m map[string]any) { delete(purge(m), "deleted_in_indexes") },
		"unknown engine":     func(m map[string]any) { m["pressure"].(map[string]any)["engine"] = "mysql" },
		"retention no daily": func(m map[string]any) { delete(m["retention"].(map[string]any), "keep_daily") },
	}
	for name, mutate := range cases {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		mutate(m)
		b, _ := json.Marshal(m)
		if err := validate(t, s, b); err == nil {
			t.Errorf("%s: schema accepted an invalid document", name)
		}
	}
}

func backup(m map[string]any) map[string]any {
	return m["backup_catalog"].(map[string]any)["backups"].([]any)[0].(map[string]any)
}

func purge(m map[string]any) map[string]any { return m["purges"].([]any)[0].(map[string]any) }

func TestSchemaPurgeComplete(t *testing.T) {
	gone := PurgeStatus{DeletedInPrimary: true, DeletedInIndexes: true}
	if !gone.Complete() {
		t.Error("deleted in primary and indexes with no backup pending must be complete")
	}
	for name, p := range map[string]PurgeStatus{
		"archived only":   {Archived: true},
		"indexes pending": {DeletedInPrimary: true},
		"backup pending":  {DeletedInPrimary: true, DeletedInIndexes: true, PendingBackupExpiry: ptr(ts(30, 0))},
	} {
		if p.Complete() {
			t.Errorf("%s: reported complete", name)
		}
	}
}

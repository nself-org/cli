package portable

// Format is the manifest "_format" marker.
const Format = "nself-portable-export"

// SchemaVersion is the manifest version the Writer emits. The Reader accepts
// any "1" or "1.<n>" and refuses every other major (E515).
const SchemaVersion = "1"

// MajorVersion is the only major this package reads and writes.
const MajorVersion = 1

// ManifestName is the manifest file name at the bundle root.
const ManifestName = "manifest.json"

// Manifest is manifest.json, contract:cli.portable-export v1. The JSON schema
// schemas/portable-export.v1.schema.json is generated from these types by
// tools/schemagen.
//
// Purpose: one definition of a bundle shared by the importer, the exporter and
// every source.
// Constraints: no field may carry a secret value (config lists key names only,
// in config/keys.txt). Arrays are sorted by Sort; maps marshal in key order.
// Only CreatedAt is time-dependent.
type Manifest struct {
	Format        string        `json:"_format"`
	SchemaVersion string        `json:"schema_version"`
	CreatedAt     string        `json:"created_at"`
	Producer      Producer      `json:"producer"`
	Postgres      PostgresInfo  `json:"postgres"`
	DB            DB            `json:"db"`
	Auth          Auth          `json:"auth"`
	Storage       Storage       `json:"storage"`
	Hasura        Hasura        `json:"hasura"`
	Exemptions    []Exemption   `json:"exemptions"`
	SourceCounts  []SourceCount `json:"source_counts"`
	Compat        []string      `json:"compat"`
	Files         []File        `json:"files"`
}

// Producer names the tool that wrote the bundle and the kind of source.
type Producer struct {
	Tool    string `json:"tool"`
	Version string `json:"version"`
	// Source is nself, postgres, nhost, hasura-cloud, supabase, firebase or appwrite.
	Source string `json:"source"`
}

// PostgresInfo describes the server the data was read from.
type PostgresInfo struct {
	ServerVersion string `json:"server_version"`
	Major         int    `json:"major"`
}

// DB lists the dumped schemas and a count and hash per table.
type DB struct {
	Schemas []string `json:"schemas"`
	Tables  []Table  `json:"tables"`
}

// Table is one dumped table. Hash is the decimal string from
// internal/postgres/tablehash; PK keeps the key's column order.
type Table struct {
	Schema string   `json:"schema"`
	Name   string   `json:"name"`
	Rows   int64    `json:"rows"`
	Hash   string   `json:"hash"`
	PK     []string `json:"pk"`
}

// Auth summarises auth/users.jsonl.
type Auth struct {
	Users          int            `json:"users"`
	HashAlgorithms map[string]int `json:"hash_algorithms"`
	ResetRequired  []string       `json:"reset_required"`
}

// Storage lists buckets and objects.
type Storage struct {
	Buckets []Bucket `json:"buckets"`
	Objects []Object `json:"objects"`
}

// Bucket is one storage bucket.
type Bucket struct {
	Name    string `json:"name"`
	Objects int    `json:"objects"`
	Bytes   int64  `json:"bytes"`
}

// Object is one stored object; its bytes live at StorageMember(Bucket, Key).
type Object struct {
	Bucket string `json:"bucket"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Hasura counts the exported metadata and what was skipped.
type Hasura struct {
	Tables        int           `json:"tables"`
	Relationships int           `json:"relationships"`
	Permissions   int           `json:"permissions"`
	Functions     int           `json:"functions"`
	Skipped       HasuraSkipped `json:"skipped"`
}

// HasuraSkipped counts metadata kinds the bundle does not carry.
type HasuraSkipped struct {
	RemoteSchemas int `json:"remote_schemas"`
	Actions       int `json:"actions"`
}

// Exemption records one item deliberately left out, with the reason.
type Exemption struct {
	// Kind is table, user, object or metadata.
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

// SourceCount is the source's own count of one kind, with how it was measured.
type SourceCount struct {
	// Kind is table, users or objects.
	Kind   string `json:"kind"`
	ID     string `json:"id"`
	Count  int64  `json:"count"`
	Method string `json:"method"`
}

// File is one bundle member: path relative to the bundle root, with the
// SHA-256 (lowercase hex) and length of its exact bytes. manifest.json is not
// listed.
type File struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Enumerations of the contract, used by the generated schema.
var (
	// Sources are the producer.source values.
	Sources = []string{"nself", "postgres", "nhost", "hasura-cloud", "supabase", "firebase", "appwrite"}
	// ExemptionKinds are the exemptions[].kind values.
	ExemptionKinds = []string{"table", "user", "object", "metadata"}
	// SourceCountKinds are the source_counts[].kind values.
	SourceCountKinds = []string{"table", "users", "objects"}
)

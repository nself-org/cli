// Package cascade defines the cascade.compat.v1 contract: the compatibility
// matrix between nSelf and the Cascade engines (Cascade and Cascade.app) for the
// claw/v1 contract (outline P9.E12).
//
// The contract is late-binding. It follows Cascade's schedule, not nSelf's, so
// an engine capability may be pending and an entry may be unknown until the
// engine ships. The wire format is the frozen JSON Schema in
// compat.schema.json (draft-07); the types below carry matching json tags. A
// breaking change ships as cascade.compat.v2, never as an edit to v1.
//
// This file holds types and constants only. It has no behaviour.
//
// Layer: L1. This package imports nothing.
package cascade

// SchemaV1 is the value of CompatV1.Schema.
const SchemaV1 = "cascade.compat.v1"

// Engine names a Cascade engine in a MatrixEntry.
type Engine string

// Engines covered by the matrix.
const (
	EngineCascade    Engine = "cascade"     // the Cascade engine
	EngineCascadeApp Engine = "cascade.app" // the Cascade.app desktop application
)

// EntryStatus is the overall result of one MatrixEntry.
type EntryStatus string

// Overall entry results.
const (
	EntryVerified    EntryStatus = "verified"    // every required capability is supported
	EntryPartial     EntryStatus = "partial"     // some capabilities are partial or pending
	EntryUnsupported EntryStatus = "unsupported" // the engine cannot drive the Bundle
	EntryUnknown     EntryStatus = "unknown"     // not yet checked
)

// CapabilityStatus is the state of one Capability.
type CapabilityStatus string

// Capability states.
const (
	CapSupported   CapabilityStatus = "supported"   // works against the engine
	CapPartial     CapabilityStatus = "partial"     // works with the limits in Notes
	CapUnsupported CapabilityStatus = "unsupported" // the engine does not provide it
	CapPending     CapabilityStatus = "pending"     // the engine has not shipped it yet
)

// CompatV1 is the cascade.compat.v1 document. Schema must equal SchemaV1,
// ClawContract names the nSelf contract checked (for example "claw/v1"),
// NselfVersion is a semantic version without a leading v, and GeneratedAt is an
// RFC 3339 UTC time.
type CompatV1 struct {
	Schema       string        `json:"schema"`
	ClawContract string        `json:"claw_contract"`
	NselfVersion string        `json:"nself_version,omitempty"`
	GeneratedAt  string        `json:"generated_at,omitempty"`
	Entries      []MatrixEntry `json:"entries"`
}

// MatrixEntry is the compatibility of one engine version with the claw
// contract. EngineVersion is a semantic version without a leading v.
// VerifiedAt is an RFC 3339 UTC time, absent while Status is EntryUnknown.
type MatrixEntry struct {
	Engine        Engine       `json:"engine"`
	EngineVersion string       `json:"engine_version"`
	Status        EntryStatus  `json:"status"`
	Capabilities  []Capability `json:"capabilities"`
	VerifiedAt    string       `json:"verified_at,omitempty"`
	Notes         string       `json:"notes,omitempty"`
}

// Capability is one late-binding engine capability and its state. Name is
// lower-case dotted, for example "tools.invoke".
type Capability struct {
	Name   string           `json:"name"`
	Status CapabilityStatus `json:"status"`
	Notes  string           `json:"notes,omitempty"`
}

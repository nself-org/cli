package model

// Releases is releases.json: release data written only by the release pipeline
// from published bytes, never by hand (Epic D3, D7). Checksums and signatures
// are release data, not manifest data.
type Releases struct {
	Generated     string             `json:"_generated,omitempty"`
	SchemaVersion int                `json:"schema_version"`
	Releases      map[string]Release `json:"releases"`
}

// Release is one slug's released artifact.
type Release struct {
	Version string `json:"version"`
	// SHA256 is the lowercase hex digest of the served tarball bytes.
	SHA256 string `json:"sha256"`
	// ReleaseSignature is null for an unsigned release.
	ReleaseSignature *Signature `json:"release_signature"`
	TarballURL       string     `json:"tarball_url,omitempty"`
	// ReleaseTag is the release tag the tarball is published under (free).
	ReleaseTag string `json:"release_tag,omitempty"`
	// Tarball is the asset name or URL released CLIs read as `tarball`.
	Tarball string `json:"tarball,omitempty"`
	// PlatformChecksums holds one sha256 per per-platform binary tarball.
	PlatformChecksums map[string]string `json:"platform_checksums,omitempty"`
}

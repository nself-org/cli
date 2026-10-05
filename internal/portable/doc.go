// Package portable reads and writes the vendor-neutral nself portable export
// bundle (contract:cli.portable-export v1, P7-ADOPT-23).
//
// A bundle is a directory: manifest.json plus the member files it lists. The
// Writer produces a deterministic manifest; the Reader treats the directory as
// untrusted input and refuses an unknown major version (E515) or any integrity
// failure (E516): a missing, changed, truncated, oversized, unlisted,
// duplicated or unsafely named member, and any symlink or hard link.
//
// Layering: L1; imports only internal/errs and the standard library.
package portable

// Package portable reads and writes the vendor-neutral nself portable export
// bundle (contract:cli.portable-export v1, P7-ADOPT-23).
//
// A bundle is a directory: manifest.json plus the member files it lists. The
// Writer produces a deterministic manifest; the Reader treats the directory as
// untrusted input and refuses an unknown major version (E515) or any integrity
// failure (E516): a missing, changed, truncated, oversized, unlisted,
// duplicated or unsafely named member, a member that is no longer the file
// that was verified, and any symlink or hard link. Member names are Unicode
// NFC and unique under case folding; storage objects are content-addressed
// (StorageMember).
//
// Layering: L1; imports only internal/errs, the standard library and
// golang.org/x/text (NFC and case folding).
package portable

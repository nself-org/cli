package errs

// Purpose: importer error allocation. Inputs: failures. Outputs: stable codes.
// Constraints: source adapters reuse the adopt block; no credentials in messages.
func init() {
	for _, e := range []struct{ code, summary, fix string }{
		{"E517", "Import source unreachable", "Check the source environment variable and connectivity."},
		{"E518", "Unsupported source Postgres major", "Use POSTGRES_VERSION matching the source, Postgres 12 or later."},
		{"E519", "Import target schema is not empty", "Import into a fresh started project; existing user tables are never overwritten."},
		{"E520", "Import extraction failed", "Check source access and repeat the export."},
		{"E521", "Import load failed", "Read report.json under .nself/import, then reset the target with `nself db reset --yes`, `nself stop` and `nself start`, and import again."},
		{"E522", "Import verification could not run", "Check target service health and read access; the load is complete, so reset with `nself db reset --yes` only to import again."},
		{"E523", "Import verification found mismatches", "Read report.json under .nself/import and resolve every non-exempt mismatch; to import again reset with `nself db reset --yes`, `nself stop` and `nself start`."},
		{"E524", "Import source not registered", "Use a registered db import source."},
		{"E529", "Import requires an object store", "Enable object storage or explicitly use --skip-storage."},
	} {
		Register(CodeEntry{Code: e.code, Category: "adopt", Summary: e.summary, DefaultWhy: e.summary + ".", DefaultFix: e.fix, DocsPath: "reference/error-codes#" + lowerCode(e.code), Exit: 1})
	}
}
func lowerCode(code string) string { return "e" + code[1:] }

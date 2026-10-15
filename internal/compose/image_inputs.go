package compose

// image_inputs.go — the one table of which env variables feed each image the
// generator renders (P7-LIVE-09, fix round 8).
//
// Purpose: a project lock pin yields to an explicit override only when that
// override really changes the rendered image. This table is the single source
// of truth for "really": projectPin (project_pins.go) reads it, nothing else
// decides whether an env variable is an override. A variable not listed here
// never drops a pin and never prints a notice.
// Inputs: none (package-level table).
// Outputs: ImageInputs (a copy), inputFor and the helpers on ImageInput.
// Constraints: one row per (compose service, image lock entry) the generator can
// render through ResolveImage. Vars lists, in the generator's precedence order
// (an *_IMAGE first), every variable the generator reads to build that
// reference; nothing else. Two tests keep the table honest:
//   - TestImageInputsCoverEveryCallSite parses the Go sources and fails on a
//     ResolveImage/ImageRef call site whose (service, entry) pair has no row.
//   - TestImageInputsMatchTheGenerator sets every candidate *_VERSION/*_IMAGE
//     variable, renders through config.Load and the generator, and fails when
//     the set of images that changed differs from the rows that list it.
// Add a row only together with a generator that reads the variable.

import "strings"

// ImageInput is one row of the table.
type ImageInput struct {
	Service            string   // compose service name; also the lock pin key
	Entry              string   // image lock entry the generator renders for it
	Vars               []string // variables that feed the rendered reference, precedence order
	alsoDefaultEntries []string // explicit compatibility defaults that render this entry
	// NotInitDefault marks an entry whose version nself init never writes into .env
	// (Hasura leaves the version blank). Any nonblank value is an override.
	NotInitDefault bool
}

var imageInputs = []ImageInput{
	// postgres: POSTGRES_IMAGE wins outright (ResolvePostgresImage); otherwise
	// POSTGRES_VERSION picks the tag, and POSTGRES_EXTENSIONS=pgvector picks the
	// pgvector repository from the major version.
	{Service: "postgres", Entry: "postgres", Vars: []string{"POSTGRES_IMAGE", "POSTGRES_VERSION"}},
	{Service: "postgres", Entry: "pgvector", Vars: []string{"POSTGRES_IMAGE", "POSTGRES_VERSION"}, alsoDefaultEntries: []string{"postgres"}},
	// Hasura: init leaves the version blank; both releases are explicit choices.
	{Service: "hasura", Entry: "hasura", Vars: []string{"HASURA_VERSION"}, NotInitDefault: true},
	{Service: "hasura", Entry: "hasura-v14", Vars: []string{"HASURA_VERSION"}, NotInitDefault: true},
	{Service: "auth", Entry: "auth", Vars: []string{"AUTH_VERSION"}, alsoDefaultEntries: []string{"auth-v14"}},
	// auth-v14: nhost/auth 0.36.0, what v1.4 runs and v1.5 until AUTH_ENCRYPTION_KEY is
	// set (core_services_auth_nginx.go, P7-LIVE-20); the same variable picks the tag.
	{Service: "auth", Entry: "auth-v14", Vars: []string{"AUTH_VERSION"}, alsoDefaultEntries: []string{"auth"}},
	// nginx renders ImageRef("nginx", ""): NGINX_VERSION (written by nself init) is
	// parsed into the config but never reaches the image.
	{Service: "nginx", Entry: "nginx"},
	{Service: "redis", Entry: "redis", Vars: []string{"REDIS_VERSION"}},
	// valkey: the same service and variable when REDIS_ENGINE=valkey or the lock pins
	// a valkey image (cache_service.go, P7-LIVE-19); the engine decides the entry.
	{Service: "redis", Entry: "valkey", Vars: []string{"REDIS_VERSION"}, alsoDefaultEntries: []string{"redis"}},
	{Service: "minio", Entry: "minio", Vars: []string{"MINIO_VERSION"}},
	{Service: "seaweedfs", Entry: "seaweedfs", Vars: []string{"SEAWEEDFS_VERSION"}},
	{Service: "mailpit", Entry: "mailpit", Vars: []string{"MAILPIT_VERSION"}},
	// admin reads NSELF_ADMIN_VERSION only; ADMIN_VERSION is not a variable.
	{Service: "admin", Entry: "admin", Vars: []string{"NSELF_ADMIN_VERSION"}},
	// functions: only the node runtime renders FUNCTIONS_VERSION; deno and python
	// render their lock entry's default.
	{Service: "functions", Entry: "functions", Vars: []string{"FUNCTIONS_VERSION"}},
	{Service: "functions", Entry: "deno"},
	{Service: "functions", Entry: "python"},
	{Service: "meilisearch", Entry: "meilisearch", Vars: []string{"MEILISEARCH_VERSION"}},
	{Service: "meilisearch-init", Entry: "meilisearch-init"},
	{Service: "typesense", Entry: "typesense", Vars: []string{"TYPESENSE_VERSION"}},
}

// ImageInputs returns a copy of the table.
func ImageInputs() []ImageInput {
	out := make([]ImageInput, len(imageInputs))
	for i, in := range imageInputs {
		in.Vars = append([]string(nil), in.Vars...)
		in.alsoDefaultEntries = append([]string(nil), in.alsoDefaultEntries...)
		out[i] = in
	}
	return out
}

// inputFor finds the row for service whose lock entry has the repository of the
// rendered image. A service with several rows (functions, postgres) is told
// apart by that repository; when none matches (a POSTGRES_IMAGE of another
// repository) the service's first row answers, so a pin of another repository
// is still judged by the variables the service reads. A service without rows
// (not rendered through ResolveImage) has no override variable at all. Rows whose
// entries share a repository (auth, auth-v14) are told apart by the entry's own
// default reference first.
func inputFor(service, image string) ImageInput {
	var first, sameRepo *ImageInput
	for i := range imageInputs {
		in := &imageInputs[i]
		if in.Service != service {
			continue
		}
		if first == nil {
			first = in
		}
		r, ok := LockedRef(in.Entry)
		if ok && (CanonicalRef(image) == CanonicalRef(r.String()) || CanonicalRef(image) == CanonicalRef(r.LegacyRef)) {
			return *in
		}
		if ok && sameRepo == nil && repoOf(r.Repository) == repoOf(image) {
			sameRepo = in
		}
	}
	if sameRepo != nil {
		return *sameRepo
	}
	if first == nil {
		return ImageInput{Service: service}
	}
	return *first
}

// Set returns the first variable of the row that is explicitly set, in
// precedence order, or "". A variable holding nothing, or the default of this
// selected row's lock entry (what nself init writes into .env;
// Ref.IsDefaultVersion, the test ImageRef applies), is not set: it renders the
// CLI default, so it must not drop a pin. alsoDefaultEntries lists the few
// compatibility values the generator maps to this row; every other entry's
// default is an explicit value.
func (in ImageInput) Set(getenv func(string) string) string {
	for _, v := range in.Vars {
		if val := strings.TrimSpace(getenv(v)); val != "" && !in.isDefault(val) {
			return v
		}
	}
	return ""
}

// isDefault reports whether val is a default version of this row's lock entry.
func (in ImageInput) isDefault(val string) bool {
	if !in.NotInitDefault {
		if r, ok := LockedRef(in.Entry); ok && r.IsDefaultVersion(val) {
			return true
		}
	}
	for _, entry := range in.alsoDefaultEntries {
		if r, ok := LockedRef(entry); ok && r.IsDefaultVersion(val) {
			return true
		}
	}
	return false
}

// ImageVars returns the full-reference variables (*_IMAGE) of the row: the only
// ones whose value can carry a digest.
func (in ImageInput) ImageVars() []string {
	var out []string
	for _, v := range in.Vars {
		if strings.HasSuffix(v, "_IMAGE") {
			out = append(out, v)
		}
	}
	return out
}

package compose

import "sort"

// Package compose (service catalog): the declarative source of truth for which
// services make up an nSelf stack.
//
// Purpose: before CLI-R07 the required/optional split existed only as control
// flow — `if p.CoreDB`, `if cfg.Redis.Enabled` — scattered through
// generator.go. Nothing could answer "what does a minimal nSelf stack contain?"
// without reading the generator, so the docs answered it from memory and drifted.
// This catalog states it once; `nself service list --core` and the generated
// wiki/SPORT page both read it, and a test asserts it matches the generator.
//
// Inputs:  none — it is static data.
// Outputs: CoreServices(), OptionalServices(), ServiceCatalog().
// Constraints: adding a service to generator.go without adding it here fails
// TestCatalogCoversGeneratorServices in this package. Image defaults are read
// from DefaultImageVersions so there is exactly one place to bump a version.

// AdminServiceName is the compose SERVICE name for the nSelf Admin UI.
//
// It is deliberately NOT "admin". "admin" is the image pin key (ImageKey
// below) and the container suffix (`<project>_admin`), so all three names for
// the same thing differ by one character. Passing "admin" to
// `docker compose up` fails with "no such service", which is how the E2E
// golden path broke at step 11: the failure surfaced only as `exit status 1`
// because Run discards subprocess stderr in non-TTY contexts.
//
// Use this constant wherever a compose service name is needed, so the string
// exists once rather than at each call site.
const AdminServiceName = "nself-admin"

// ServiceTier says whether a service is part of every stack or opt-in.
type ServiceTier string

const (
	// TierRequired services are present in every app profile. Removing one
	// produces a stack that cannot serve a request.
	TierRequired ServiceTier = "required"

	// TierOptional services are added only when their config gate is enabled.
	TierOptional ServiceTier = "optional"
)

// CatalogEntry describes one service in the stack.
type CatalogEntry struct {
	// Name is the compose service name, matching the key in DockerCompose.Services.
	Name string
	// Tier is required or optional.
	Tier ServiceTier
	// Purpose is a one-line description for docs and `service list`.
	Purpose string
	// EnableEnv is the environment variable that switches an optional service
	// on. Empty for required services, which have no off switch.
	EnableEnv string
	// VersionEnv is the environment variable that overrides the image tag.
	VersionEnv string
	// DefaultImage is the pinned image used when VersionEnv is unset. Resolved
	// from DefaultImageVersions so version bumps have a single home.
	DefaultImage string
	// ImageKey overrides the DefaultImageVersions lookup key when the compose
	// service name differs from it (nself-admin is pinned under "admin").
	ImageKey string
}

// serviceCatalog is the ordered catalog. Required services come first, in boot
// order; optional services follow alphabetically.
var serviceCatalog = []CatalogEntry{
	{
		Name:       "postgres",
		Tier:       TierRequired,
		Purpose:    "PostgreSQL database — the system of record for every nSelf stack",
		VersionEnv: "POSTGRES_VERSION",
	},
	{
		Name:       "hasura",
		Tier:       TierRequired,
		Purpose:    "Hasura GraphQL engine — the only supported data access path for apps",
		VersionEnv: "HASURA_VERSION",
	},
	{
		Name:       "auth",
		Tier:       TierRequired,
		Purpose:    "hasura-auth — issues the JWTs Hasura authorises against",
		VersionEnv: "AUTH_VERSION",
	},
	{
		Name:       "nginx",
		Tier:       TierRequired,
		Purpose:    "Reverse proxy and TLS termination for every published route",
		VersionEnv: "NGINX_VERSION",
	},
	{
		// The compose service is "nself-admin"; the image pin key is "admin".
		Name:       AdminServiceName,
		Tier:       TierOptional,
		Purpose:    "nSelf Admin web UI (localhost only; never deployed)",
		EnableEnv:  "NSELF_ADMIN_ENABLED",
		VersionEnv: "NSELF_ADMIN_VERSION",
		ImageKey:   "admin",
	},
	{
		Name:       "functions",
		Tier:       TierOptional,
		Purpose:    "Serverless function runtime",
		EnableEnv:  "FUNCTIONS_ENABLED",
		VersionEnv: "FUNCTIONS_VERSION",
	},
	{
		Name:       "mailpit",
		Tier:       TierOptional,
		Purpose:    "Development SMTP catcher — captures outbound mail instead of sending it",
		EnableEnv:  "MAILPIT_ENABLED",
		VersionEnv: "MAILPIT_VERSION",
	},
	{
		Name:       "meilisearch",
		Tier:       TierOptional,
		Purpose:    "Full-text search index (SEARCH_ENGINE=meilisearch, the default)",
		EnableEnv:  "SEARCH_ENABLED",
		VersionEnv: "MEILISEARCH_VERSION",
	},
	{
		Name:       "minio",
		Tier:       TierOptional,
		Purpose:    "S3-compatible object storage",
		EnableEnv:  "MINIO_ENABLED",
		VersionEnv: "MINIO_VERSION",
	},
	{
		Name:       "typesense",
		Tier:       TierOptional,
		Purpose:    "Full-text search index (SEARCH_ENGINE=typesense alternative)",
		EnableEnv:  "SEARCH_ENABLED",
		VersionEnv: "TYPESENSE_VERSION",
	},
	{
		Name:       "redis",
		Tier:       TierOptional,
		Purpose:    "Cache and queue backend",
		EnableEnv:  "REDIS_ENABLED",
		VersionEnv: "REDIS_VERSION",
	},
}

// ServiceCatalog returns every catalogued service with its default image
// resolved from DefaultImageVersions.
func ServiceCatalog() []CatalogEntry {
	out := make([]CatalogEntry, len(serviceCatalog))
	copy(out, serviceCatalog)
	for i := range out {
		key := out[i].ImageKey
		if key == "" {
			key = out[i].Name
		}
		out[i].DefaultImage = DefaultImageVersions[key]
	}
	return out
}

// CoreServices returns the services present in every app-profile stack.
func CoreServices() []CatalogEntry {
	return filterTier(TierRequired)
}

// OptionalServices returns the services that require a config gate.
func OptionalServices() []CatalogEntry {
	return filterTier(TierOptional)
}

func filterTier(tier ServiceTier) []CatalogEntry {
	var out []CatalogEntry
	for _, e := range ServiceCatalog() {
		if e.Tier == tier {
			out = append(out, e)
		}
	}
	return out
}

// CatalogEntryFor looks a service up by compose name.
func CatalogEntryFor(name string) (CatalogEntry, bool) {
	for _, e := range ServiceCatalog() {
		if e.Name == name {
			return e, true
		}
	}
	return CatalogEntry{}, false
}

// CatalogNames returns every catalogued service name, sorted.
func CatalogNames() []string {
	names := make([]string, 0, len(serviceCatalog))
	for _, e := range serviceCatalog {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	return names
}

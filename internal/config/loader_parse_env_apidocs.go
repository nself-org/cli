package config

// loader_parse_env_apidocs.go — env var -> Config.ApiDocs mapping (D-0455).
// Inputs:  os.Environ, read into the cfg passed in by parseEnvToConfig.
// Outputs: none — mutates cfg in place.
// Constraints: pure os.Getenv reads only. Path is always set here, so the build
//              step that falls back to the default docs config for an empty
//              section no longer turns API_DOCS_ENABLED=false back on.

// parseEnvAPIDocs fills cfg.ApiDocs from the API_DOCS_* variables. Each field
// takes its default only when its variable is unset or empty. Title has no
// default: the docs renderer derives it from the project name.
func parseEnvAPIDocs(cfg *Config) {
	cfg.ApiDocs.Enabled = getEnvBool("API_DOCS_ENABLED", true)
	cfg.ApiDocs.Path = getEnvOr("API_DOCS_PATH", "/docs")
	cfg.ApiDocs.Title = getEnvOr("API_DOCS_TITLE", "")
	cfg.ApiDocs.Theme = getEnvOr("API_DOCS_THEME", "default")
	cfg.ApiDocs.AuthEnvVar = getEnvOr("API_DOCS_AUTH_ENV_VAR", "")
	cfg.ApiDocs.GraphQLEnabled = getEnvBool("API_DOCS_GRAPHQL_ENABLED", true)
	cfg.ApiDocs.GraphQLEndpoint = getEnvOr("API_DOCS_GRAPHQL_ENDPOINT", "/v1/graphql")
}

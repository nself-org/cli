package config

// loader_parse_env.go — maps every environment variable to its Config struct field.
//
// Purpose: Single canonical mapping from env var name strings to typed Config
//          struct fields. Split into loader_parse_env_{core,storage,ops}.go by
//          domain (T-P6-E2-W1-S1-T3) for 300-line compliance; this file is the
//          entry point that calls each in the same order as the original single
//          function body.
// Inputs:  os.Environ (read via os.Getenv, getEnvOr, getEnvInt, getEnvBool).
// Outputs: *Config — fully populated struct (no defaults yet; ApplyDefaults
//          fills zero values after this function returns).
// Constraints: Must not call ApplyDefaults or touch the filesystem. Pure
//              os.Getenv reads only. Keep in sync with loader_known_vars.go.
// SPORT:   cli/internal/config — decomposed from loader.go (T-E2-06), further
//          split for 300-line compliance (T-P6-E2-W1-S1-T3).

// parseEnvToConfig reads every Config field from os.Getenv, delegating to the
// per-domain helpers below in the same order as the original monolithic
// function body.
func parseEnvToConfig() *Config {
	cfg := &Config{}

	parseEnvCore(cfg)
	parseEnvStorage(cfg)
	parseEnvOps(cfg)
	parseEnvAPIDocs(cfg)

	return cfg
}

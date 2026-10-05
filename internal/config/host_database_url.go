package config

import "fmt"

// HostDatabaseURL returns the PostgreSQL connection string for a client that
// runs on the host, outside the compose network.
//
// Purpose: DatabaseURL is the in-network DSN (host "postgres", port 5432) and
// does not resolve from the host. Compose publishes Postgres on loopback only,
// so host-side tools (importer, exporter, table hash) connect to 127.0.0.1 on
// POSTGRES_PORT.
// Inputs: cfg.Postgres User, Password, Port (0 means 5432) and DB (empty means
// "nself", the default applyDefaultsPostgres sets).
// Outputs: postgresql://<user>:<pass>@127.0.0.1:<port>/<db>, with the
// credentials percent-encoded through URLUserInfo exactly as DatabaseURL does.
// Constraints: the result carries the password; callers never log or print it.
func (cfg *Config) HostDatabaseURL() string {
	port := cfg.Postgres.Port
	if port == 0 {
		port = 5432
	}
	db := cfg.Postgres.DB
	if db == "" {
		db = "nself"
	}
	return fmt.Sprintf("postgresql://%s@127.0.0.1:%d/%s",
		URLUserInfo(cfg.Postgres.User, cfg.Postgres.Password), port, db)
}

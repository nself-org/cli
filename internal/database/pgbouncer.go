package database

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/nself-org/cli/internal/config"
)

// PgBouncerStatus holds runtime status of PgBouncer.
type PgBouncerStatus struct {
	Running       bool
	PoolMode      string
	ActivePools   int
	IdleServers   int
	ActiveClients int
}

// PgBouncerConfigFile generates the pgbouncer.ini content for a project.
// Returns the INI file contents as a string.
func PgBouncerConfigFile(cfg *config.Config) string {
	pgHost := cfg.Postgres.Host
	if pgHost == "" {
		pgHost = "postgres"
	}
	dbName := cfg.Postgres.DB
	if dbName == "" {
		dbName = "nself"
	}
	port := cfg.PgBouncer.Port
	if port == 0 {
		port = 6432
	}
	poolMode := cfg.PgBouncer.PoolMode
	if poolMode == "" {
		poolMode = "session"
	}
	maxClientConn := cfg.PgBouncer.MaxClientConn
	if maxClientConn == 0 {
		maxClientConn = 100
	}
	defaultPoolSize := cfg.PgBouncer.DefaultPoolSize
	if defaultPoolSize == 0 {
		defaultPoolSize = 25
	}
	minPoolSize := cfg.PgBouncer.MinPoolSize
	if minPoolSize == 0 {
		minPoolSize = 5
	}
	reservePoolSize := cfg.PgBouncer.ReservePoolSize
	if reservePoolSize == 0 {
		reservePoolSize = 5
	}
	serverIdleTimeout := cfg.PgBouncer.ServerIdleTimeout
	if serverIdleTimeout == 0 {
		serverIdleTimeout = 600
	}
	adminUsers := cfg.PgBouncer.AdminUsers
	if adminUsers == "" {
		adminUsers = "postgres"
	}
	statsUsers := cfg.PgBouncer.StatsUsers
	if statsUsers == "" {
		statsUsers = "postgres"
	}

	logConns := boolToInt(cfg.PgBouncer.LogConnections)
	logDisconns := boolToInt(cfg.PgBouncer.LogDisconnections)

	var sb strings.Builder
	sb.WriteString("[databases]\n")
	fmt.Fprintf(&sb, "%s = host=%s port=5432 dbname=%s\n", dbName, pgHost, dbName)
	sb.WriteString("\n[pgbouncer]\n")
	sb.WriteString("listen_addr = 0.0.0.0\n")
	fmt.Fprintf(&sb, "listen_port = %d\n", port)
	sb.WriteString("auth_type = md5\n")
	sb.WriteString("auth_file = /etc/pgbouncer/userlist.txt\n")
	fmt.Fprintf(&sb, "pool_mode = %s\n", poolMode)
	fmt.Fprintf(&sb, "max_client_conn = %d\n", maxClientConn)
	fmt.Fprintf(&sb, "default_pool_size = %d\n", defaultPoolSize)
	fmt.Fprintf(&sb, "min_pool_size = %d\n", minPoolSize)
	fmt.Fprintf(&sb, "reserve_pool_size = %d\n", reservePoolSize)
	fmt.Fprintf(&sb, "server_idle_timeout = %d\n", serverIdleTimeout)
	fmt.Fprintf(&sb, "log_connections = %d\n", logConns)
	fmt.Fprintf(&sb, "log_disconnections = %d\n", logDisconns)
	fmt.Fprintf(&sb, "admin_users = %s\n", adminUsers)
	fmt.Fprintf(&sb, "stats_users = %s\n", statsUsers)
	return sb.String()
}

// PgBouncerHBAFile generates the pg_hba.conf-style auth file content for PgBouncer.
func PgBouncerHBAFile(cfg *config.Config) string {
	pgUser := cfg.Postgres.User
	if pgUser == "" {
		pgUser = "postgres"
	}
	dbName := cfg.Postgres.DB
	if dbName == "" {
		dbName = "nself"
	}

	var sb strings.Builder
	sb.WriteString("# TYPE  DATABASE  USER  ADDRESS  METHOD\n")
	fmt.Fprintf(&sb, "local   %s         %s             md5\n", dbName, pgUser)
	sb.WriteString("host    all        all   0.0.0.0/0  md5\n")
	return sb.String()
}

// PgBouncerUserlistFile generates the userlist.txt content for PgBouncer.
// Passwords are stored in plaintext here; PgBouncer supports md5 and plaintext.
func PgBouncerUserlistFile(cfg *config.Config) string {
	pgUser := cfg.Postgres.User
	if pgUser == "" {
		pgUser = "postgres"
	}
	pgPass := cfg.Postgres.Password

	var sb strings.Builder
	fmt.Fprintf(&sb, "%q %q\n", pgUser, pgPass)
	return sb.String()
}

// WritePgBouncerConfig writes the three PgBouncer config files to the project's
// .nself/pgbouncer/ directory.
func WritePgBouncerConfig(ctx context.Context, cfg *config.Config, projectDir string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("writing pgbouncer config: %w", ctx.Err())
	default:
	}

	dir := filepath.Join(projectDir, ".nself", "pgbouncer")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating pgbouncer config directory: %w", err)
	}

	files := map[string]string{
		"pgbouncer.ini": PgBouncerConfigFile(cfg),
		"pg_hba.conf":   PgBouncerHBAFile(cfg),
		"userlist.txt":  PgBouncerUserlistFile(cfg),
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			return fmt.Errorf("writing %s: %w", name, err)
		}
		// Enforce 0600 explicitly: WriteFile respects umask, which can weaken the mode.
		if err := os.Chmod(path, 0600); err != nil {
			return fmt.Errorf("chmod %s: %w", path, err)
		}
	}
	return nil
}

// PgBouncerConnectionURL returns the connection URL that points through
// PgBouncer instead of directly to PostgreSQL.
func PgBouncerConnectionURL(cfg *config.Config) string {
	user := cfg.Postgres.User
	if user == "" {
		user = "postgres"
	}
	password := cfg.Postgres.Password
	host := "localhost"
	port := cfg.PgBouncer.Port
	if port == 0 {
		port = 6432
	}
	dbName := cfg.Postgres.DB
	if dbName == "" {
		dbName = "nself"
	}

	if password != "" {
		return fmt.Sprintf("postgres://%s@%s:%d/%s", config.URLUserInfo(user, password), host, port, dbName)
	}
	return fmt.Sprintf("postgres://%s@%s:%d/%s", config.URLUserInfo(user, ""), host, port, dbName)
}

// GetPgBouncerStatus queries the PgBouncer admin database for runtime status.
// It connects via psql inside the pgbouncer container (named {project}_pgbouncer).
func GetPgBouncerStatus(ctx context.Context, cfg *config.Config) (*PgBouncerStatus, error) {
	port := cfg.PgBouncer.Port
	if port == 0 {
		port = 6432
	}
	poolMode := cfg.PgBouncer.PoolMode
	if poolMode == "" {
		poolMode = "session"
	}

	pgUser := cfg.Postgres.User
	if pgUser == "" {
		pgUser = "postgres"
	}

	// PgBouncer exposes an admin database named "pgbouncer" on its own listen port.
	// We exec into the pgbouncer container and connect via psql pointing to that port.
	pgbouncerContainer := cfg.ProjectName + "_pgbouncer"
	out, err := pgbouncerAdminQuery(ctx, pgbouncerContainer, pgUser, port, "SHOW STATS;")
	if err != nil {
		return nil, fmt.Errorf("querying pgbouncer stats: %w", err)
	}

	status := &PgBouncerStatus{
		Running:  true,
		PoolMode: poolMode,
	}

	// Parse columnar psql output. Each line is a row; we look for known column names.
	// SHOW STATS output is: database | total_xact_count | total_query_count | ...
	// We parse the count columns from subsequent aggregated SHOW POOLS output.
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "database") {
			continue
		}
		// SHOW STATS returns one row per database; count non-header rows as active pools.
		status.ActivePools++
	}

	// Query active client and idle server counts from SHOW POOLS.
	poolsOut, err := pgbouncerAdminQuery(ctx, pgbouncerContainer, pgUser, port, "SHOW POOLS;")
	if err == nil {
		for _, line := range strings.Split(poolsOut, "\n") {
			fields := strings.Split(line, "|")
			if len(fields) < 8 {
				continue
			}
			// SHOW POOLS columns: database | user | cl_active | cl_waiting | sv_active | sv_idle | sv_used | sv_tested | ...
			clActive, _ := strconv.Atoi(strings.TrimSpace(fields[2]))
			svIdle, _ := strconv.Atoi(strings.TrimSpace(fields[5]))
			status.ActiveClients += clActive
			status.IdleServers += svIdle
		}
	}

	return status, nil
}

// pgbouncerAdminQuery runs a SQL command against the PgBouncer admin database
// by exec-ing into the named container.
func pgbouncerAdminQuery(ctx context.Context, container, user string, port int, sql string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "exec", container,
		"psql",
		"-h", "127.0.0.1",
		"-p", strconv.Itoa(port),
		"-U", user,
		"-d", "pgbouncer",
		"-tAc", sql,
	)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("psql: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSpace(stdout.String()), nil
}

// boolToInt converts a bool to 0 or 1 for PgBouncer INI format.
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

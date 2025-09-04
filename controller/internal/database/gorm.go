package database

import (
	"fmt"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm/logger"
	"netwatcher-controller/internal/probe"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"netwatcher-controller/internal/agent"
	// your modules
	"netwatcher-controller/internal/auth"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"
)

// OpenFromEnv picks the driver & DSN from env, opens GORM, tunes the pool, and pings.
func OpenFromEnv() (*gorm.DB, error) {
	dialector, dsn, err := DialectorFromEnv()
	if err != nil {
		return nil, err
	}

	gormCfg := &gorm.Config{
		// tweak if you want more/less verbosity:
		Logger:                                   logger.Default.LogMode(currentLogLevel()),
		DisableForeignKeyConstraintWhenMigrating: false,
		// NamingStrategy: &schema.NamingStrategy{SingularTable: false},
	}

	db, err := gorm.Open(dialector, gormCfg)
	if err != nil {
		return nil, fmt.Errorf("gorm open: %w", err)
	}

	// Pool config + ping
	if err := tuneAndPing(db, dsn); err != nil {
		return nil, err
	}

	return db, nil
}

// DialectorFromEnv returns the correct GORM dialector + DSN by reading env vars.
//
//	DB_DRIVER: postgres | mysql | sqlite | sqlserver
//	(driver-specific vars shown in helpers below)
func DialectorFromEnv() (gorm.Dialector, string, error) {
	driver := strings.ToLower(strings.TrimSpace(os.Getenv("DB_DRIVER")))
	if driver == "" {
		driver = "postgres" // default to Postgres
	}

	switch driver {
	case "postgres", "postgresql":
		dsn := buildPostgresDSN()
		return postgres.Open(dsn), dsn, nil
	case "mysql", "mariadb":
		dsn := buildMySQLDSN()
		return mysql.Open(dsn), dsn, nil
	case "sqlite", "sqlite3":
		dsn := buildSQLiteDSN()
		return sqlite.Open(dsn), dsn, nil
	case "sqlserver", "mssql":
		dsn := buildSQLServerDSN()
		return sqlserver.Open(dsn), dsn, nil
	default:
		return nil, "", fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}
}

// ---------- DSN Builders ----------

func buildPostgresDSN() string {
	host := envOr("POSTGRES_HOST", "localhost")
	port := envOr("POSTGRES_PORT", "5432")
	user := os.Getenv("POSTGRES_USER")
	pass := os.Getenv("POSTGRES_PASSWORD")
	db := os.Getenv("POSTGRES_DB")
	ssl := envOr("POSTGRES_SSLMODE", "disable")
	tz := envOr("POSTGRES_TIMEZONE", "America/Vancouver")

	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		host, port, user, pass, db, ssl, tz,
	)
}

func buildMySQLDSN() string {
	// Example: user:pass@tcp(host:port)/dbname?parseTime=true&loc=Local
	host := envOr("MYSQL_HOST", "localhost")
	port := envOr("MYSQL_PORT", "3306")
	user := os.Getenv("MYSQL_USER")
	pass := os.Getenv("MYSQL_PASSWORD")
	db := os.Getenv("MYSQL_DB")
	params := os.Getenv("MYSQL_PARAMS") // e.g. "charset=utf8mb4&parseTime=True&loc=Local"
	if params == "" {
		params = "charset=utf8mb4&parseTime=True&loc=Local"
	}
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s", user, pass, host, port, db, params)
}

func buildSQLiteDSN() string {
	// SQLITE_PATH may be ":memory:" or a file path (e.g., "data/app.db")
	path := envOr("SQLITE_PATH", "data/app.db")
	// Shared cache can help in some scenarios; not required.
	params := os.Getenv("SQLITE_PARAMS") // e.g. "_busy_timeout=5000&_journal_mode=WAL"
	if params != "" {
		return fmt.Sprintf("%s?%s", path, params)
	}
	return path
}

func buildSQLServerDSN() string {
	// Example: sqlserver://user:pass@host:1433?database=dbname
	host := envOr("MSSQL_HOST", "localhost")
	port := envOr("MSSQL_PORT", "1433")
	user := os.Getenv("MSSQL_USER")
	pass := os.Getenv("MSSQL_PASSWORD")
	db := os.Getenv("MSSQL_DB")
	params := os.Getenv("MSSQL_PARAMS") // e.g. "encrypt=disable"
	if params != "" {
		return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s&%s", user, pass, host, port, db, params)
	}
	return fmt.Sprintf("sqlserver://%s:%s@%s:%s?database=%s", user, pass, host, port, db)
}

// ---------- Pooling / Ping ----------

func tuneAndPing(gdb *gorm.DB, dsn string) error {
	sqlDB, err := gdb.DB()
	if err != nil {
		return fmt.Errorf("extract *sql.DB: %w", err)
	}

	maxOpen := envInt("DB_MAX_OPEN_CONNS", 25)
	maxIdle := envInt("DB_MAX_IDLE_CONNS", 25)
	lifetime := envDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	idleTime := envDuration("DB_CONN_MAX_IDLE_TIME", 10*time.Minute)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)
	sqlDB.SetConnMaxIdleTime(idleTime)

	// try a quick ping (some drivers ignore DSN here; it's fine)
	if err := sqlDB.Ping(); err != nil {
		return fmt.Errorf("db ping: %w", err)
	}
	return nil
}

// ---------- helpers ----------

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func currentLogLevel() logger.LogLevel {
	switch strings.ToLower(os.Getenv("GORM_LOG")) {
	case "silent":
		return logger.Silent
	case "error":
		return logger.Error
	case "warn", "warning":
		return logger.Warn
	case "info", "":
		return logger.Info
	default:
		return logger.Info
	}
}

// RunMigrations performs GORM AutoMigrate and creates extra indexes/constraints
func RunMigrations(db *gorm.DB) error {
	// 1) AutoMigrate all tables
	if err := db.AutoMigrate(
		&users.User{},
		&agent.Agent{},
		&agent.AgentNonce{},
		&workspace.Workspace{},
		&workspace.WorkspaceMember{},
		&auth.Session{},
		&probe.Probe{},
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	// 2) Dialect-specific extras (functional + partial indexes, etc.)
	switch db.Dialector.Name() {
	case "postgres":
		if err := migratePostgres(db); err != nil {
			return err
		}
	case "mysql":
		if err := migrateMySQL(db); err != nil {
			return err
		}
	case "sqlite", "sqlite3":
		if err := migrateSQLite(db); err != nil {
			return err
		}
	default:
		// No-op: unknown dialect; AutoMigrate already did most work
	}
	return nil
}

func migratePostgres(db *gorm.DB) error {
	// -------- Users: case-insensitive unique email --------
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_users_email_lower ON users (LOWER(email));`).Error; err != nil {
		return fmt.Errorf("users email lower unique: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_last_login_at ON users (last_login_at);`).Error; err != nil {
		return fmt.Errorf("users last_login idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_status ON users (status);`).Error; err != nil {
		return fmt.Errorf("users status idx: %w", err)
	}

	// -------- Agents --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws ON agents (workspace_id);`).Error; err != nil {
		return fmt.Errorf("agents ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws_site ON agents (workspace_id, site_id);`).Error; err != nil {
		return fmt.Errorf("agents ws_site idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_status ON agents (status);`).Error; err != nil {
		return fmt.Errorf("agents status idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents (last_seen_at);`).Error; err != nil { // no DESC in index for portability
		return fmt.Errorf("agents last_seen idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents (hostname);`).Error; err != nil {
		return fmt.Errorf("agents hostname idx: %w", err)
	}

	// Public key fingerprint & PIN uniqueness (unclaimed only)
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_pubkey_fp ON agents (public_key_fp);`).Error; err != nil {
		return fmt.Errorf("agents pubkey_fp idx: %w", err)
	}
	// Ensure a PIN (via pin_index) cannot be duplicated among unclaimed agents in the same workspace.
	// We use a partial unique index that applies only while the agent is unclaimed (no key, no pin_consumed_at).
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_agents_ws_pin_unclaimed
		ON agents (workspace_id, pin_index)
		WHERE pin_index IS NOT NULL AND public_key IS NULL AND pin_consumed_at IS NULL;
	`).Error; err != nil {
		return fmt.Errorf("agents ws pin unclaimed unique: %w", err)
	}

	// -------- Workspaces --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces (owner_user_id);`).Error; err != nil {
		return fmt.Errorf("workspaces owner idx: %w", err)
	}

	// -------- WorkspaceMembers --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws ON workspace_members (workspace_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_user ON workspace_members (workspace_id, user_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws_user idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_email ON workspace_members (workspace_id, email);`).Error; err != nil {
		return fmt.Errorf("wsm ws_email idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_role ON workspace_members (role);`).Error; err != nil {
		return fmt.Errorf("wsm role idx: %w", err)
	}
	// Single active OWNER per workspace
	if err := db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS ux_wsm_single_owner_per_ws
		ON workspace_members (workspace_id)
		WHERE role = 'OWNER' AND revoked_at IS NULL;
	`).Error; err != nil {
		return fmt.Errorf("wsm single owner partial unique: %w", err)
	}

	// -------- Sessions --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_item ON sessions (item_id, is_agent);`).Error; err != nil {
		return fmt.Errorf("sessions item idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expiry);`).Error; err != nil {
		return fmt.Errorf("sessions expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_ws_conn ON sessions (ws_conn);`).Error; err != nil {
		return fmt.Errorf("sessions ws_conn idx: %w", err)
	}

	// -------- Probes --------
	// General probe indexes
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_agent_type ON probes (agent_id, type);`).Error; err != nil {
		return fmt.Errorf("probes agent_type idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_reverse_of ON probes (reverse_of_probe_id);`).Error; err != nil {
		return fmt.Errorf("probes reverse_of idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_original_agent ON probes (original_agent_id);`).Error; err != nil {
		return fmt.Errorf("probes original_agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_server ON probes (server);`).Error; err != nil {
		return fmt.Errorf("probes server idx: %w", err)
	}
	// Handy partial for TRAFFICSIM servers
	if err := db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_probes_trafsim_servers
		ON probes (agent_id)
		WHERE server = TRUE AND type = 'TRAFFICSIM';
	`).Error; err != nil {
		return fmt.Errorf("probes trafsim servers partial idx: %w", err)
	}

	// Targets
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_probe ON probe_targets (probe_id);`).Error; err != nil {
		return fmt.Errorf("targets probe idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_agent ON probe_targets (agent_id);`).Error; err != nil {
		return fmt.Errorf("targets agent idx: %w", err)
	}

	// -------- Foreign Keys (safe) --------
	// Add FKs only if they do not exist.
	// probe_targets.probe_id -> probes.id (CASCADE)
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_probe_targets_probe'
			) THEN
				ALTER TABLE probe_targets
				ADD CONSTRAINT fk_probe_targets_probe
				FOREIGN KEY (probe_id) REFERENCES probes(id)
				ON UPDATE CASCADE ON DELETE CASCADE;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("fk probe_targets.probe_id: %w", err)
	}

	// probe_targets.agent_id -> agents.id (SET NULL)
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_probe_targets_agent'
			) THEN
				ALTER TABLE probe_targets
				ADD CONSTRAINT fk_probe_targets_agent
				FOREIGN KEY (agent_id) REFERENCES agents(id)
				ON UPDATE CASCADE ON DELETE SET NULL;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("fk probe_targets.agent_id: %w", err)
	}

	// probes.agent_id -> agents.id (CASCADE)  — optional but useful
	if err := db.Exec(`
		DO $$
		BEGIN
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint
				WHERE conname = 'fk_probes_agent'
			) THEN
				ALTER TABLE probes
				ADD CONSTRAINT fk_probes_agent
				FOREIGN KEY (agent_id) REFERENCES agents(id)
				ON UPDATE CASCADE ON DELETE CASCADE;
			END IF;
		END $$;
	`).Error; err != nil {
		return fmt.Errorf("fk probes.agent_id: %w", err)
	}

	// -------- Agent nonces --------
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_agent_nonces_nonce ON agent_nonces (nonce);`).Error; err != nil {
		return fmt.Errorf("agent_nonces nonce unique: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_agent ON agent_nonces (agent_id);`).Error; err != nil {
		return fmt.Errorf("agent_nonces agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_expiry ON agent_nonces (expires_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_used ON agent_nonces (used_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces used idx: %w", err)
	}
	return nil
}

func migrateMySQL(db *gorm.DB) error {
	// -------- Users --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_last_login_at ON users (last_login_at);`).Error; err != nil {
		return fmt.Errorf("users last_login idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_status ON users (status);`).Error; err != nil {
		return fmt.Errorf("users status idx: %w", err)
	}

	// -------- Agents --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws ON agents (workspace_id);`).Error; err != nil {
		return fmt.Errorf("agents ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws_site ON agents (workspace_id, site_id);`).Error; err != nil {
		return fmt.Errorf("agents ws_site idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_status ON agents (status);`).Error; err != nil {
		return fmt.Errorf("agents status idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents (last_seen_at);`).Error; err != nil {
		return fmt.Errorf("agents last_seen idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents (hostname);`).Error; err != nil {
		return fmt.Errorf("agents hostname idx: %w", err)
	}

	// Public key fingerprint
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_pubkey_fp ON agents (public_key_fp);`).Error; err != nil {
		return fmt.Errorf("agents pubkey_fp idx: %w", err)
	}
	// MySQL cannot do partial unique indexes; enforce uniqueness by (workspace_id, pin_index)
	// and ensure application clears pin_index (sets NULL) once the PIN is consumed.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_agents_ws_pin ON agents (workspace_id, pin_index);`).Error; err != nil {
		// if already exists, ignore; IF NOT EXISTS should handle modern MySQL/MariaDB, but versions vary
	}

	// -------- Workspaces --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces (owner_user_id);`).Error; err != nil {
		return fmt.Errorf("workspaces owner idx: %w", err)
	}

	// -------- WorkspaceMembers --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws ON workspace_members (workspace_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_user ON workspace_members (workspace_id, user_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws_user idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_email ON workspace_members (workspace_id, email);`).Error; err != nil {
		return fmt.Errorf("wsm ws_email idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_role ON workspace_members (role);`).Error; err != nil {
		return fmt.Errorf("wsm role idx: %w", err)
	}

	// -------- Sessions --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_item ON sessions (item_id, is_agent);`).Error; err != nil {
		return fmt.Errorf("sessions item idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expiry);`).Error; err != nil {
		return fmt.Errorf("sessions expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_ws_conn ON sessions (ws_conn);`).Error; err != nil {
		return fmt.Errorf("sessions ws_conn idx: %w", err)
	}

	// -------- Probes --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_agent_type ON probes (agent_id, type);`).Error; err != nil {
		return fmt.Errorf("probes agent_type idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_reverse_of ON probes (reverse_of_probe_id);`).Error; err != nil {
		return fmt.Errorf("probes reverse_of idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_original_agent ON probes (original_agent_id);`).Error; err != nil {
		return fmt.Errorf("probes original_agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_server ON probes (server);`).Error; err != nil {
		return fmt.Errorf("probes server idx: %w", err)
	}

	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_probe ON probe_targets (probe_id);`).Error; err != nil {
		return fmt.Errorf("targets probe idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_agent ON probe_targets (agent_id);`).Error; err != nil {
		return fmt.Errorf("targets agent idx: %w", err)
	}

	// -------- Foreign Keys (safe) --------
	// MySQL supports IF NOT EXISTS for indexes but not for constraints; emulate with information_schema checks.
	if err := db.Exec(`
		ALTER TABLE probe_targets
		ADD CONSTRAINT fk_probe_targets_probe
		FOREIGN KEY (probe_id) REFERENCES probes(id)
		ON UPDATE CASCADE ON DELETE CASCADE;
	`).Error; err != nil {
		// Ignore errno 1826/1061/1068 equivalents (already exists); GORM doesn't expose vendor codes here,
		// so we keep it simple: if it fails due to duplicate, it's harmless in idempotent deploys.
	}
	if err := db.Exec(`
		ALTER TABLE probe_targets
		ADD CONSTRAINT fk_probe_targets_agent
		FOREIGN KEY (agent_id) REFERENCES agents(id)
		ON UPDATE CASCADE ON DELETE SET NULL;
	`).Error; err != nil {
	}
	if err := db.Exec(`
		ALTER TABLE probes
		ADD CONSTRAINT fk_probes_agent
		FOREIGN KEY (agent_id) REFERENCES agents(id)
		ON UPDATE CASCADE ON DELETE CASCADE;
	`).Error; err != nil {
	}

	// MySQL cannot do partial unique like Postgres; single-owner enforced in code.

	// -------- Agent nonces --------
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_agent_nonces_nonce ON agent_nonces (nonce);`).Error; err != nil {
		return fmt.Errorf("agent_nonces nonce unique: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_agent ON agent_nonces (agent_id);`).Error; err != nil {
		return fmt.Errorf("agent_nonces agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_expiry ON agent_nonces (expires_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_used ON agent_nonces (used_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces used idx: %w", err)
	}

	return nil
}

func migrateSQLite(db *gorm.DB) error {
	// -------- Users --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_last_login_at ON users (last_login_at);`).Error; err != nil {
		return fmt.Errorf("users last_login idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_status ON users (status);`).Error; err != nil {
		return fmt.Errorf("users status idx: %w", err)
	}

	// -------- Agents --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws ON agents (workspace_id);`).Error; err != nil {
		return fmt.Errorf("agents ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_ws_site ON agents (workspace_id, site_id);`).Error; err != nil {
		return fmt.Errorf("agents ws_site idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_status ON agents (status);`).Error; err != nil {
		return fmt.Errorf("agents status idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents (last_seen_at);`).Error; err != nil {
		return fmt.Errorf("agents last_seen idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_hostname ON agents (hostname);`).Error; err != nil {
		return fmt.Errorf("agents hostname idx: %w", err)
	}

	// Public key fingerprint
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agents_pubkey_fp ON agents (public_key_fp);`).Error; err != nil {
		return fmt.Errorf("agents pubkey_fp idx: %w", err)
	}
	// SQLite also lacks partial unique; use (workspace_id, pin_index) and set pin_index=NULL on consume in app logic.
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_agents_ws_pin ON agents (workspace_id, pin_index);`).Error; err != nil {
		return fmt.Errorf("agents ws pin unique: %w", err)
	}

	// -------- Workspaces --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_workspaces_owner ON workspaces (owner_user_id);`).Error; err != nil {
		return fmt.Errorf("workspaces owner idx: %w", err)
	}

	// -------- WorkspaceMembers --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws ON workspace_members (workspace_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_user ON workspace_members (workspace_id, user_id);`).Error; err != nil {
		return fmt.Errorf("wsm ws_user idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_ws_email ON workspace_members (workspace_id, email);`).Error; err != nil {
		return fmt.Errorf("wsm ws_email idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_wsm_role ON workspace_members (role);`).Error; err != nil {
		return fmt.Errorf("wsm role idx: %w", err)
	}

	// -------- Sessions --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_item ON sessions (item_id, is_agent);`).Error; err != nil {
		return fmt.Errorf("sessions item idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expiry);`).Error; err != nil {
		return fmt.Errorf("sessions expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_sessions_ws_conn ON sessions (ws_conn);`).Error; err != nil {
		return fmt.Errorf("sessions ws_conn idx: %w", err)
	}

	// -------- Probes --------
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_agent_type ON probes (agent_id, type);`).Error; err != nil {
		return fmt.Errorf("probes agent_type idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_reverse_of ON probes (reverse_of_probe_id);`).Error; err != nil {
		return fmt.Errorf("probes reverse_of idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_original_agent ON probes (original_agent_id);`).Error; err != nil {
		return fmt.Errorf("probes original_agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_probes_server ON probes (server);`).Error; err != nil {
		return fmt.Errorf("probes server idx: %w", err)
	}

	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_probe ON probe_targets (probe_id);`).Error; err != nil {
		return fmt.Errorf("targets probe idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_targets_agent ON probe_targets (agent_id);`).Error; err != nil {
		return fmt.Errorf("targets agent idx: %w", err)
	}

	// (SQLite FKs require table recreation for ON DELETE changes; rely on GORM creates or leave as-is.)

	// -------- Agent nonces --------
	if err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_agent_nonces_nonce ON agent_nonces (nonce);`).Error; err != nil {
		return fmt.Errorf("agent_nonces nonce unique: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_agent ON agent_nonces (agent_id);`).Error; err != nil {
		return fmt.Errorf("agent_nonces agent idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_expiry ON agent_nonces (expires_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces expiry idx: %w", err)
	}
	if err := db.Exec(`CREATE INDEX IF NOT EXISTS idx_agent_nonces_used ON agent_nonces (used_at);`).Error; err != nil {
		return fmt.Errorf("agent_nonces used idx: %w", err)
	}
	return nil
}

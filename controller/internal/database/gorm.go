package database

import (
	"context"
	"errors"
	"fmt"
	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/alert"
	"netwatcher-controller/internal/probe"
	"netwatcher-controller/internal/share"
	"netwatcher-controller/internal/speedtest"
	"netwatcher-controller/internal/users"
	"netwatcher-controller/internal/workspace"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// OpenFromEnv opens Postgres, tunes the pool, verifies connectivity, applies
// automigrations (delegating workspace tables to workspace.Store), then creates
// extra indexes that are not covered by struct tags.
func OpenFromEnv() (*gorm.DB, error) {
	dsn, err := postgresDSNFromEnv()
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(currentLogLevel()),
		DisableForeignKeyConstraintWhenMigrating: false,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("db(): %w", err)
	}
	sqlDB.SetMaxOpenConns(getEnvInt("DB_MAX_OPEN_CONNS", 25))
	sqlDB.SetMaxIdleConns(getEnvInt("DB_MAX_IDLE_CONNS", 25))
	sqlDB.SetConnMaxIdleTime(getEnvDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute))
	sqlDB.SetConnMaxLifetime(getEnvDuration("DB_CONN_MAX_LIFETIME", 60*time.Minute))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	// --- Automigrations ---

	return db, nil
}

// ----- DSN / Config -----

func postgresDSNFromEnv() (string, error) {
	if raw := strings.TrimSpace(os.Getenv("POSTGRES_DSN")); raw != "" {
		return raw, nil
	}
	host := envOr("POSTGRES_HOST", "localhost")
	port := envOr("POSTGRES_PORT", "5432")
	user := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	dbname := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
	pass := os.Getenv("POSTGRES_PASSWORD")
	sslmode := envOr("POSTGRES_SSLMODE", "disable")
	tz := envOr("POSTGRES_TIMEZONE", "UTC")

	if user == "" || dbname == "" {
		return "", errors.New("POSTGRES_USER and POSTGRES_DB are required (or set POSTGRES_DSN)")
	}

	parts := []string{
		"host=" + host,
		"port=" + port,
		"user=" + user,
		"dbname=" + dbname,
		"sslmode=" + sslmode,
		"TimeZone=" + tz,
	}
	if pass != "" {
		parts = append(parts, "password="+pass)
	}
	return strings.Join(parts, " "), nil
}

func DialectorFromEnv() (gorm.Dialector, string, error) {
	dsn, err := postgresDSNFromEnv()
	if err != nil {
		return nil, "", err
	}
	return postgres.Open(dsn), dsn, nil
}

func currentLogLevel() logger.LogLevel {
	if lvl := strings.ToLower(strings.TrimSpace(os.Getenv("GORM_LOG_LEVEL"))); lvl != "" {
		switch lvl {
		case "silent":
			return logger.Silent
		case "error":
			return logger.Error
		case "warn", "warning":
			return logger.Warn
		case "info", "debug":
			return logger.Info
		}
	}
	if asBool(os.Getenv("DEBUG"), true) {
		return logger.Info
	}
	return logger.Warn
}

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func asBool(v string, def bool) bool {
	if v == "" {
		return def
	}
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getEnvInt(key string, def int) int {
	if s := strings.TrimSpace(os.Getenv(key)); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if s := strings.TrimSpace(os.Getenv(key)); s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			return d
		}
	}
	return def
}

// CreateIndexes Creates Indexes inferred from your structs -----
// Workspace indexes are created by workspace.Store.AutoMigrate(ctx).
func CreateIndexes(db *gorm.DB) error {
	if err := workspace.NewStore(db).AutoMigrate(context.TODO()); err != nil {
		return fmt.Errorf("workspace automigrate: %w", err)
	}

	// 2) Remaining models (ordered loosely by dependency)
	if err := db.WithContext(context.TODO()).AutoMigrate(
		&users.User{},
		&users.Session{},   // TableName(): "sessions"
		&users.UserToken{}, // TableName(): "user_tokens" - email verification, password reset

		&agent.Agent{},
		&agent.Auth{}, // TableName(): "agent_pins"

		&probe.Probe{},  // TableName(): "probes"
		&probe.Target{}, // TableName(): "probe_targets"

		&speedtest.QueueItem{},    // TableName(): "speedtest_queue"
		&speedtest.CachedServer{}, // TableName(): "agent_speedtest_servers"

		&alert.AlertRule{},     // TableName(): "alert_rules"
		&alert.Alert{},         // TableName(): "alerts"
		&alert.RouteBaseline{}, // TableName(): "route_baselines"

		&share.ShareLink{}, // TableName(): "share_links"
	); err != nil {
		return fmt.Errorf("automigrate: %w", err)
	}

	stmts := []string{
		// users: guard against case-variant duplicates
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_users_email_lower ON users (LOWER(email));`,

		// sessions (users.Session → "sessions")
		`CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions (user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expiry ON sessions (expiry);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_created ON sessions (created);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_expiry ON sessions (user_id, expiry);`,

		// agents
		`CREATE INDEX IF NOT EXISTS idx_agents_ws_name ON agents (workspace_id, name);`,
		`CREATE INDEX IF NOT EXISTS idx_agents_last_seen ON agents (last_seen_at);`,

		// agent_pins (agent.Auth → "agent_pins")
		`CREATE INDEX IF NOT EXISTS idx_agent_pins_ws ON agent_pins (workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_pins_agent ON agent_pins (agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_pins_expires ON agent_pins (expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_pins_consumed ON agent_pins (consumed);`,

		// probes
		`CREATE INDEX IF NOT EXISTS idx_probes_ws ON probes (workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_probes_agent ON probes (agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_probes_type ON probes (type);`,
		`CREATE INDEX IF NOT EXISTS idx_probes_enabled ON probes (enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_probes_agent_type ON probes (agent_id, type);`,

		// probe_targets
		`CREATE INDEX IF NOT EXISTS idx_probe_targets_probe ON probe_targets (probe_id);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_targets_agent ON probe_targets (agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_targets_group ON probe_targets (group_id);`,
		`CREATE INDEX IF NOT EXISTS idx_probe_targets_probe_agent ON probe_targets (probe_id, agent_id);`,

		// speedtest_queue
		`CREATE INDEX IF NOT EXISTS idx_speedtest_queue_agent ON speedtest_queue (agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_queue_status ON speedtest_queue (status);`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_queue_agent_status ON speedtest_queue (agent_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_queue_expires ON speedtest_queue (expires_at);`,

		// agent_speedtest_servers
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_speedtest_servers_agent_server ON agent_speedtest_servers (agent_id, server_id);`,
		`CREATE INDEX IF NOT EXISTS idx_speedtest_servers_agent ON agent_speedtest_servers (agent_id);`,

		// alert_rules
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_ws ON alert_rules (workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_ws_enabled ON alert_rules (workspace_id, enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_alert_rules_probe ON alert_rules (probe_id);`,

		// alerts
		`CREATE INDEX IF NOT EXISTS idx_alerts_ws ON alerts (workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_status ON alerts (status);`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_ws_status ON alerts (workspace_id, status);`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_rule ON alerts (alert_rule_id);`,

		// share_links
		`CREATE INDEX IF NOT EXISTS idx_share_links_ws ON share_links (workspace_id);`,
		`CREATE INDEX IF NOT EXISTS idx_share_links_agent ON share_links (agent_id);`,
		`CREATE INDEX IF NOT EXISTS idx_share_links_expires ON share_links (expires_at);`,
	}

	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	// Optional JSONB GIN indexes for ad-hoc filters on labels/metadata.
	if asBool(os.Getenv("ENABLE_JSONB_GIN"), false) {
		jsonb := []string{
			`CREATE INDEX IF NOT EXISTS idx_agents_labels_gin ON agents USING gin (labels);`,
			`CREATE INDEX IF NOT EXISTS idx_agents_metadata_gin ON agents USING gin (metadata);`,
			`CREATE INDEX IF NOT EXISTS idx_probes_labels_gin ON probes USING gin (labels);`,
			`CREATE INDEX IF NOT EXISTS idx_probes_metadata_gin ON probes USING gin (metadata);`,
		}
		for _, sql := range jsonb {
			if err := db.Exec(sql).Error; err != nil {
				return fmt.Errorf("create gin index: %w", err)
			}
		}
	}

	return nil
}

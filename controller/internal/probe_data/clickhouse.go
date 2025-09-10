// internal/probe_data/ch.go
package probe_data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"netwatcher-controller/internal/probe"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
)

// OpenClickHouseFromEnv returns a *sql.DB using clickhouse-go v2.
func OpenClickHouseFromEnv() (*sql.DB, error) {
	host := getenv("CLICKHOUSE_HOST", "localhost")
	port := getenv("CLICKHOUSE_PORT", "9000")
	user := getenv("CLICKHOUSE_USER", "default")
	pass := os.Getenv("CLICKHOUSE_PASSWORD")
	db := getenv("CLICKHOUSE_DB", "default")

	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", host, port)},
		Auth: clickhouse.Auth{
			Database: db,
			Username: user,
			Password: pass,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
			// enable if using JSON/Object on older CH versions:
			// "allow_experimental_object_type": 1,
		},
		DialTimeout: 5 * time.Second,
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
	})

	// Verify the connection
	if err := conn.Ping(); err != nil {
		return nil, fmt.Errorf("clickhouse ping failed: %w", err)
	}
	return conn, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// MigrateCH creates the table (idempotent).
func MigrateCH(ctx context.Context, ch *sql.DB) error {
	// If your cluster supports JSON (24.8+), keep payload_json JSON.
	// Otherwise, change it to String or Object('json') with experimental flag.
	const ddl = `
	CREATE TABLE IF NOT EXISTS probe_data (
		id               UInt64           DEFAULT 0,
		created_at       DateTime('UTC')  DEFAULT now('UTC'),
		received_at       DateTime('UTC')  DEFAULT now('UTC'),
	type             LowCardinality(String),
	probe_id         UInt64,
		probe_agent_id   UInt64,
	    agent_id         UInt64,
		triggered        Boolean,
		triggered_reason String,
		target           String,
		target_agent     UInt64,
		payload_raw      String
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(created_at)
	ORDER BY (type, probe_id, created_at)
	TTL created_at + INTERVAL 90 DAY DELETE
	SETTINGS index_granularity = 8192;
`
	_, err := ch.ExecContext(ctx, ddl)
	return err
}

// SaveRecordCH inserts one probe event row.
// SaveRecordCH inserts one probe event row.
func SaveRecordCH(ctx context.Context, ch *sql.DB, data ProbeData, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	const ins = `
INSERT INTO probe_data
(created_at, received_at, type, probe_id, probe_agent_id, agent_id,
 triggered, triggered_reason, target, target_agent, payload_raw)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	created := data.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}

	received := data.ReceivedAt
	if received.IsZero() {
		received = time.Now().UTC()
	}

	_, err = ch.ExecContext(ctx, ins,
		created, received, kind,
		uint64(data.ProbeID), uint64(data.ProbeAgentID), uint64(data.AgentID),
		data.Triggered, /* <— pass bool, not uint8 */
		data.TriggeredReason,
		data.Target, uint64(data.TargetAgent),
		string(raw),
	)
	return err
}

func boolToUInt8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// THESE ARE THE GETTERS

// DataRow matches one row from ClickHouse `probe_data`.

// DecodePayload unmarshals the JSON payload into v (pointer to struct/map).
func (e ProbeData) DecodePayload(v any) error {
	if e.Payload == nil {
		return errors.New("empty payload")
	}
	return json.Unmarshal([]byte(e.Payload), v)
}

// -------------------------------------------
// Simple, focused helpers
// -------------------------------------------

// ADD these helpers near the top of the "GETTERS" section:

// CH string literal (single-quote + escape internal ')
func chQuoteString(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// CH time literal in UTC ('YYYY-MM-DD HH:MM:SS')
func chQuoteTime(t time.Time) string { return "'" + t.UTC().Format("2006-01-02 15:04:05") + "'" }

// GetProbeDataByProbe returns rows for a given probe within a time range.
// If from.IsZero() or to.IsZero(), that bound is ignored.
// If limit <= 0, no limit is applied.
// REWRITE GetProbeDataByProbe: inline literals (no args / ? placeholders)
func GetProbeDataByProbe(
	ctx context.Context,
	db *sql.DB,
	probeID uint64,
	from, to time.Time,
	ascending bool,
	limit int,
) ([]ProbeData, error) {

	var clauses []string
	clauses = append(clauses, fmt.Sprintf("probe_id = %d", probeID))

	if !from.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at >= %s", chQuoteTime(from)))
	}
	if !to.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at <= %s", chQuoteTime(to)))
	}

	order := "DESC"
	if ascending {
		order = "ASC"
	}

	q := `
SELECT
    created_at, received_at, type, probe_id, agent_id, probe_agent_id,
    triggered, triggered_reason, target, target_agent, payload_raw
FROM probe_data
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY created_at ` + order

	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProbeData
	for rows.Next() {
		var r ProbeData
		var trigBool bool
		var typeStr string
		var payloadStr string
		if err := rows.Scan(
			&r.CreatedAt, &r.ReceivedAt, &typeStr, &r.ProbeID, &r.AgentID, &r.ProbeAgentID,
			&trigBool, &r.TriggeredReason, &r.Target, &r.TargetAgent, &payloadStr,
		); err != nil {
			return nil, err
		}
		r.Type = probe.Type(typeStr)
		r.Triggered = trigBool
		r.Payload = json.RawMessage(payloadStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetLatestByTypeAndAgent returns the newest event for a given type and reporting agent.
// If probeID != nil, it also filters by that probe.
// REWRITE GetLatestByTypeAndAgent: inline literals (no args / ? placeholders)
func GetLatestByTypeAndAgent(
	ctx context.Context,
	db *sql.DB,
	typ string,
	agentID uint64,
	probeID *uint64,
) (*ProbeData, error) {

	var clauses []string
	clauses = append(clauses,
		fmt.Sprintf("type = %s", chQuoteString(typ)),
		fmt.Sprintf("agent_id = %d", agentID),
	)
	if probeID != nil {
		clauses = append(clauses, fmt.Sprintf("probe_id = %d", *probeID))
	}

	q := `
SELECT
    created_at, received_at, type, probe_id, agent_id, probe_agent_id,
    triggered, triggered_reason, target, target_agent, payload_raw
FROM probe_data
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY created_at DESC
LIMIT 1
`

	row := db.QueryRowContext(ctx, q)

	var r ProbeData
	var trigBool bool
	var typeStr string
	var payloadStr string
	if err := row.Scan(
		&r.CreatedAt, &r.ReceivedAt, &typeStr, &r.ProbeID, &r.AgentID, &r.ProbeAgentID,
		&trigBool, &r.TriggeredReason, &r.Target, &r.TargetAgent, &payloadStr,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	r.Type = probe.Type(typeStr)
	r.Triggered = trigBool
	r.Payload = json.RawMessage(payloadStr)
	return &r, nil
}

// -------------------------------------------
// Flexible “finder” with optional filters.
// Useful when you need the same WHERE logic in multiple places.
// -------------------------------------------

type FindParams struct {
	Type         *string   // equals
	ProbeID      *uint64   // equals
	AgentID      *uint64   // equals (reporting agent)
	ProbeAgentID *uint64   // equals (owner)
	TargetAgent  *uint64   // equals (reverse probe target)
	TargetPrefix *string   // target LIKE 'prefix%'
	Triggered    *bool     // equals
	From         time.Time // created_at >=
	To           time.Time // created_at <=
	Limit        int       // LIMIT N
	Ascending    bool      // ORDER BY created_at ASC (default DESC)
}

// REWRITE FindProbeData: inline literals (no args / ? placeholders)
func FindProbeData(ctx context.Context, db *sql.DB, p FindParams) ([]ProbeData, error) {
	var clauses []string

	if p.Type != nil {
		clauses = append(clauses, fmt.Sprintf("type = %s", chQuoteString(*p.Type)))
	}
	if p.ProbeID != nil {
		clauses = append(clauses, fmt.Sprintf("probe_id = %d", *p.ProbeID))
	}
	if p.AgentID != nil {
		clauses = append(clauses, fmt.Sprintf("agent_id = %d", *p.AgentID))
	}
	if p.ProbeAgentID != nil {
		clauses = append(clauses, fmt.Sprintf("probe_agent_id = %d", *p.ProbeAgentID))
	}
	if p.TargetAgent != nil {
		clauses = append(clauses, fmt.Sprintf("target_agent = %d", *p.TargetAgent))
	}
	if p.TargetPrefix != nil && *p.TargetPrefix != "" {
		// LIKE 'prefix%'
		clauses = append(clauses, fmt.Sprintf("target LIKE %s", chQuoteString(*p.TargetPrefix+"%")))
	}
	if !p.From.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at >= %s", chQuoteTime(p.From)))
	}
	if !p.To.IsZero() {
		clauses = append(clauses, fmt.Sprintf("created_at <= %s", chQuoteTime(p.To)))
	}
	if p.Triggered != nil {
		v := uint8(0)
		if *p.Triggered {
			v = 1
		}
		clauses = append(clauses, fmt.Sprintf("triggered = %d", v))
	}

	where := "1"
	if len(clauses) > 0 {
		where = strings.Join(clauses, " AND ")
	}

	order := "DESC"
	if p.Ascending {
		order = "ASC"
	}

	q := `
SELECT
    created_at, received_at, type, probe_id, agent_id, probe_agent_id,
    triggered, triggered_reason, target, target_agent, payload_raw
FROM probe_data
WHERE ` + where + `
ORDER BY created_at ` + order

	if p.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", p.Limit)
	}

	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ProbeData
	for rows.Next() {
		var r ProbeData
		var trigBool bool
		var typeStr string
		var payloadStr string
		if err := rows.Scan(
			&r.CreatedAt, &r.ReceivedAt, &typeStr, &r.ProbeID, &r.AgentID, &r.ProbeAgentID,
			&trigBool, &r.TriggeredReason, &r.Target, &r.TargetAgent, &payloadStr,
		); err != nil {
			return nil, err
		}
		r.Type = probe.Type(typeStr)
		r.Triggered = trigBool
		r.Payload = json.RawMessage(payloadStr)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetLatest returns the newest row satisfying the filters in FindParams.
// Only a subset makes sense for a single-row lookup: Type, AgentID, ProbeID, etc.
func GetLatest(ctx context.Context, db *sql.DB, p FindParams) (*ProbeData, error) {
	p.Limit = 1
	p.Ascending = false
	rows, err := FindProbeData(ctx, db, p)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// Convenience wrapper for your stated use-case:
// “ONLY the newest entry for probe with type NETINFO and agent (reporting agent) id = X”
func GetLatestNetInfoForAgent(
	ctx context.Context,
	db *sql.DB,
	agentID uint64,
	probeID *uint64, // pass nil to ignore probe_id
) (*ProbeData, error) {
	typ := string(probe.TypeNetInfo) // or string(probe.TypeNetInfo) if you prefer
	params := FindParams{
		Type:    &typ,
		AgentID: &agentID,
	}
	if probeID != nil {
		params.ProbeID = probeID
	}
	return GetLatest(ctx, db, params)
}

// Convenience wrapper for your stated use-case:
// “ONLY the newest entry for probe with type NETINFO and agent (reporting agent) id = X”
func GetLatestSysInfoForAgent(
	ctx context.Context,
	db *sql.DB,
	agentID uint64,
	probeID *uint64, // pass nil to ignore probe_id
) (*ProbeData, error) {
	typ := string(probe.TypeSysInfo) // or string(probe.TypeNetInfo) if you prefer
	params := FindParams{
		Type:    &typ,
		AgentID: &agentID,
	}
	if probeID != nil {
		params.ProbeID = probeID
	}
	return GetLatest(ctx, db, params)
}

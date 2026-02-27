// internal/probe_data/ch.go
package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	log "github.com/sirupsen/logrus"
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

// MigrateCH creates the tables with configurable retention (idempotent).
func MigrateCH(ctx context.Context, ch *sql.DB, retentionDays int) error {
	if retentionDays <= 0 {
		retentionDays = 90 // default
	}

	// If your cluster supports JSON (24.8+), keep payload_json JSON.
	// Otherwise, change it to String or Object('json') with experimental flag.
	ddl := fmt.Sprintf(`
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
	TTL created_at + INTERVAL %d DAY DELETE
	SETTINGS index_granularity = 8192;
`, retentionDays)
	if _, err := ch.ExecContext(ctx, ddl); err != nil {
		return err
	}

	// Analysis snapshots — stores periodic workspace health analysis results
	// for long-term trend analysis. Top-level metrics are native columns for
	// efficient ClickHouse queries; detail data is stored as JSON strings.
	snapshotDDL := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS analysis_snapshots (
		workspace_id      UInt64,
		generated_at      DateTime('UTC')  DEFAULT now('UTC'),
		overall_health    Float64,
		grade             LowCardinality(String),
		latency_score     Float64,
		packet_loss_score Float64,
		route_stability   Float64,
		mos_score         Float64,
		status            LowCardinality(String),
		status_message    String,
		incident_count    UInt32,
		total_agents      UInt32,
		online_agents     UInt32,
		total_probes      UInt32,
		incidents_json    String,
		agents_json       String,
		llm_summary       String DEFAULT ''
	)
	ENGINE = MergeTree
	PARTITION BY toYYYYMM(generated_at)
	ORDER BY (workspace_id, generated_at)
	TTL generated_at + INTERVAL %d DAY DELETE
	SETTINGS index_granularity = 8192;
`, retentionDays)
	_, err := ch.ExecContext(ctx, snapshotDDL)
	return err
}

// MigrateCHWithDefaults creates the table with default 90-day retention
func MigrateCHWithDefaults(ctx context.Context, ch *sql.DB) error {
	return MigrateCH(ctx, ch, 90)
}

// ---- Batch Writer ----

// chRecord is a pre-serialized row ready for batch insertion.
type chRecord struct {
	CreatedAt       time.Time
	ReceivedAt      time.Time
	Kind            string
	ProbeID         uint64
	ProbeAgentID    uint64
	AgentID         uint64
	Triggered       bool
	TriggeredReason string
	Target          string
	TargetAgent     uint64
	PayloadRaw      string
}

// CHBatchWriter buffers probe data rows and flushes them in batches to
// ClickHouse, dramatically reducing the number of data parts created.
type CHBatchWriter struct {
	ch       *sql.DB
	records  chan chRecord
	done     chan struct{}
	maxBatch int
	interval time.Duration
}

const (
	defaultBatchSize     = 50
	defaultFlushInterval = 2 * time.Second
	batchChanSize        = 2000 // buffer up to 2 000 records before blocking
)

// globalBatchWriter is the package-level singleton initialised at startup.
var globalBatchWriter *CHBatchWriter

// InitBatchWriter creates and starts the global batch writer.
// Call this once at startup, after MigrateCH.
func InitBatchWriter(ch *sql.DB) {
	w := &CHBatchWriter{
		ch:       ch,
		records:  make(chan chRecord, batchChanSize),
		done:     make(chan struct{}),
		maxBatch: defaultBatchSize,
		interval: defaultFlushInterval,
	}
	globalBatchWriter = w
	go w.loop()
	log.Info("ClickHouse batch writer started")
}

// StopBatchWriter signals the writer to flush remaining records and stop.
func StopBatchWriter() {
	if globalBatchWriter == nil {
		return
	}
	close(globalBatchWriter.records)
	<-globalBatchWriter.done // wait for final flush
	log.Info("ClickHouse batch writer stopped")
}

// enqueue adds a record to the batch buffer. Non-blocking as long as the
// channel has capacity; blocks if the buffer is full (back-pressure).
func (w *CHBatchWriter) enqueue(r chRecord) {
	w.records <- r
}

// loop is the background goroutine that reads from the channel and
// flushes in batches.
func (w *CHBatchWriter) loop() {
	defer close(w.done)

	buf := make([]chRecord, 0, w.maxBatch)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case r, ok := <-w.records:
			if !ok {
				// Channel closed — flush remaining and exit
				if len(buf) > 0 {
					w.flush(buf)
				}
				return
			}
			buf = append(buf, r)
			if len(buf) >= w.maxBatch {
				w.flush(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				w.flush(buf)
				buf = buf[:0]
			}
		}
	}
}

// flush writes a batch of records with a single multi-row INSERT.
func (w *CHBatchWriter) flush(batch []chRecord) {
	if len(batch) == 0 {
		return
	}

	// Build multi-row VALUES
	var sb strings.Builder
	sb.WriteString(`INSERT INTO probe_data
(created_at, received_at, type, probe_id, probe_agent_id, agent_id,
 triggered, triggered_reason, target, target_agent, payload_raw) VALUES `)

	args := make([]any, 0, len(batch)*11)
	for i, r := range batch {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
		args = append(args,
			r.CreatedAt, r.ReceivedAt, r.Kind,
			r.ProbeID, r.ProbeAgentID, r.AgentID,
			r.Triggered, r.TriggeredReason,
			r.Target, r.TargetAgent, r.PayloadRaw,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := w.ch.ExecContext(ctx, sb.String(), args...); err != nil {
		log.WithError(err).Errorf("CH batch flush failed (%d records)", len(batch))
		return
	}
	log.Debugf("CH batch flush: %d records", len(batch))
}

// SaveRecordCH inserts one probe event row.
// If the global batch writer is running the record is enqueued for
// batched insertion; otherwise it falls back to a direct INSERT.
func SaveRecordCH(ctx context.Context, ch *sql.DB, data ProbeData, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	created := data.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	received := data.ReceivedAt
	if received.IsZero() {
		received = time.Now().UTC()
	}

	rec := chRecord{
		CreatedAt:       created,
		ReceivedAt:      received,
		Kind:            kind,
		ProbeID:         uint64(data.ProbeID),
		ProbeAgentID:    uint64(data.ProbeAgentID),
		AgentID:         uint64(data.AgentID),
		Triggered:       data.Triggered,
		TriggeredReason: data.TriggeredReason,
		Target:          data.Target,
		TargetAgent:     uint64(data.TargetAgent),
		PayloadRaw:      string(raw),
	}

	// Use batch writer if available, otherwise direct INSERT
	if globalBatchWriter != nil {
		globalBatchWriter.enqueue(rec)
		return nil
	}

	// Fallback: direct single-row INSERT (for tests / one-off scripts)
	const ins = `
INSERT INTO probe_data
(created_at, received_at, type, probe_id, probe_agent_id, agent_id,
 triggered, triggered_reason, target, target_agent, payload_raw)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err = ch.ExecContext(ctx, ins,
		rec.CreatedAt, rec.ReceivedAt, rec.Kind,
		rec.ProbeID, rec.ProbeAgentID, rec.AgentID,
		rec.Triggered, rec.TriggeredReason,
		rec.Target, rec.TargetAgent, rec.PayloadRaw,
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
// If agentID is not nil, filters by the reporting agent (agent_id).
// REWRITE GetProbeDataByProbe: inline literals (no args / ? placeholders)
func GetProbeDataByProbe(
	ctx context.Context,
	db *sql.DB,
	probeID uint64,
	agentID *uint64,
	from, to time.Time,
	ascending bool,
	limit int,
) ([]ProbeData, error) {

	var clauses []string
	clauses = append(clauses, fmt.Sprintf("probe_id = %d", probeID))

	if agentID != nil {
		clauses = append(clauses, fmt.Sprintf("agent_id = %d", *agentID))
	}

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
		r.Type = Type(typeStr)
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
	r.Type = Type(typeStr)
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
		r.Type = Type(typeStr)
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
	log.Infof("[IP-DEBUG] GetLatestNetInfoForAgent: querying NETINFO for agent_id=%d", agentID)
	typ := string(TypeNetInfo) // or string(probe.TypeNetInfo) if you prefer
	params := FindParams{
		Type:    &typ,
		AgentID: &agentID,
	}
	if probeID != nil {
		params.ProbeID = probeID
	}
	result, err := GetLatest(ctx, db, params)
	if err != nil {
		log.Errorf("[IP-DEBUG] GetLatestNetInfoForAgent: query error for agent %d: %v", agentID, err)
		return nil, err
	}
	if result != nil {
		log.Infof("[IP-DEBUG] GetLatestNetInfoForAgent: found record for agent %d - result.AgentID=%d, ProbeID=%d",
			agentID, result.AgentID, result.ProbeID)
	} else {
		log.Warnf("[IP-DEBUG] GetLatestNetInfoForAgent: no record found for agent %d", agentID)
	}
	return result, nil
}

// Convenience wrapper for your stated use-case:
// “ONLY the newest entry for probe with type NETINFO and agent (reporting agent) id = X”
func GetLatestSysInfoForAgent(
	ctx context.Context,
	db *sql.DB,
	agentID uint64,
	probeID *uint64, // pass nil to ignore probe_id
) (*ProbeData, error) {
	typ := string(TypeSysInfo) // or string(probe.TypeNetInfo) if you prefer
	params := FindParams{
		Type:    &typ,
		AgentID: &agentID,
	}
	if probeID != nil {
		params.ProbeID = probeID
	}
	return GetLatest(ctx, db, params)
}

// MaxRawRowsForAggregation limits how many raw rows we fetch before aggregating in Go.
// This prevents memory exhaustion on very large time ranges.
const MaxRawRowsForAggregation = 50000

// GetProbeDataAggregated returns aggregated rows for a given probe using time-bucket averaging.
// aggregateSec specifies the bucket size in seconds (e.g., 60 = 1 minute buckets).
// This fetches raw data and aggregates in Go for robustness with JSON parsing.
// For very large time ranges, it limits raw data to MaxRawRowsForAggregation rows.
// If agentID is not nil, filters by the reporting agent (agent_id).
func GetProbeDataAggregated(
	ctx context.Context,
	db *sql.DB,
	probeID uint64,
	agentID *uint64,
	probeType string, // "PING", "TRAFFICSIM", or "MTR"
	from, to time.Time,
	aggregateSec int,
	limit int,
) ([]ProbeData, error) {
	if aggregateSec <= 0 {
		// Fall back to non-aggregated query
		return GetProbeDataByProbe(ctx, db, probeID, agentID, from, to, false, limit)
	}

	// Fetch raw data from ClickHouse with a sensible limit
	// This prevents memory exhaustion on very large time ranges
	rawData, err := GetProbeDataByProbe(ctx, db, probeID, agentID, from, to, false, MaxRawRowsForAggregation)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch raw probe data: %w", err)
	}

	if len(rawData) == 0 {
		return []ProbeData{}, nil
	}

	// Filter by type before aggregation - AGENT probes store data with actual types (PING, MTR, TRAFFICSIM)
	// When aggregating, we need to ensure we only process data of the requested type
	filteredData := make([]ProbeData, 0, len(rawData))
	for _, d := range rawData {
		if string(d.Type) == probeType {
			filteredData = append(filteredData, d)
		}
	}

	if len(filteredData) == 0 {
		return []ProbeData{}, nil
	}

	// Aggregate in Go based on probe type
	bucketDuration := time.Duration(aggregateSec) * time.Second

	switch probeType {
	case "PING":
		return aggregatePingData(filteredData, bucketDuration, limit), nil
	case "TRAFFICSIM":
		return aggregateTrafficSimData(filteredData, bucketDuration, limit), nil
	case "MTR":
		// For MTR, aggregate with intelligent route grouping + notable trace preservation
		return aggregateMtrData(filteredData, bucketDuration, limit), nil
	default:
		// For other types, just bucket by time without payload aggregation
		return bucketProbeData(filteredData, bucketDuration, limit), nil
	}
}

// pingAggInputPayload represents the JSON structure for PING probe data (used for aggregation input)
// This matches the snake_case format sent by the agent (PingResult struct in agent/probes/ping.go)
// RTT values are time.Duration which serializes as nanoseconds (int64)
type pingAggInputPayload struct {
	StartTimestamp        string  `json:"start_timestamp"`
	StopTimestamp         string  `json:"stop_timestamp"`
	PacketsRecv           int     `json:"packets_recv"`
	PacketsSent           int     `json:"packets_sent"`
	PacketsRecvDuplicates int     `json:"packets_recv_duplicates"`
	PacketLoss            float64 `json:"packet_loss"`
	Addr                  string  `json:"addr"`
	MinRtt                int64   `json:"min_rtt"` // nanoseconds
	MaxRtt                int64   `json:"max_rtt"` // nanoseconds
	AvgRtt                int64   `json:"avg_rtt"` // nanoseconds
	StdDevRtt             int64   `json:"std_dev_rtt"`
}

// AggregatedPingPayload represents aggregated PING data
type AggregatedPingPayload struct {
	Latency     float64 `json:"latency"`
	MinLatency  float64 `json:"minLatency"`
	MaxLatency  float64 `json:"maxLatency"`
	AvgLatency  float64 `json:"avgLatency"`
	PacketLoss  float64 `json:"packetLoss"`
	PacketsSent uint64  `json:"packetsSent"`
	PacketsRecv uint64  `json:"packetsRecv"`
}

// TrafficSimPayload represents the JSON structure for TRAFFICSIM probe data
type TrafficSimPayload struct {
	ReportTime    string  `json:"reportTime"`
	AverageRTT    float64 `json:"averageRTT"`
	MinRTT        float64 `json:"minRTT"`
	MaxRTT        float64 `json:"maxRTT"`
	TotalPackets  uint64  `json:"totalPackets"`
	LostPackets   uint64  `json:"lostPackets"`
	OutOfSequence uint64  `json:"outOfSequence"`
	Duplicates    uint64  `json:"duplicates"`
}

// MTR aggregation types
type MtrHopHost struct {
	IP string `json:"ip"`
}

type MtrHop struct {
	TTL     int          `json:"ttl"`
	Hosts   []MtrHopHost `json:"hosts"`
	LossPct interface{}  `json:"loss_pct"` // Can be string or float
	Sent    int          `json:"sent"`
	Recv    int          `json:"recv"`
	Avg     string       `json:"avg"`
	Best    string       `json:"best"`
	Worst   string       `json:"worst"`
	Last    string       `json:"last"`
	StdDev  string       `json:"stdev"`
	Jitter  string       `json:"jitter"`
	Javg    string       `json:"javg"`
	Jmax    string       `json:"jmax"`
	Jint    string       `json:"jint"`
}

type MtrReport struct {
	Hops []MtrHop `json:"hops"`
}

type MtrPayload struct {
	Report         MtrReport `json:"report"`
	StopTimestamp  string    `json:"stop_timestamp"`
	StartTimestamp string    `json:"start_timestamp"`
}

// AggregatedMtrPayload represents aggregated MTR data for a time bucket
type AggregatedMtrPayload struct {
	Report                 MtrReport `json:"report"` // Aggregated hop data
	StopTimestamp          string    `json:"stop_timestamp"`
	StartTimestamp         string    `json:"start_timestamp"`
	RouteSignature         string    `json:"route_signature"`          // Route signature for grouping
	PreviousRouteSignature string    `json:"previous_route_signature"` // Previous route (for route-change diff)
	TraceCount             int       `json:"trace_count"`              // Number of traces in this bucket
	IsAggregated           bool      `json:"is_aggregated"`            // True if this is aggregated data
	NotableReason          string    `json:"notable_reason"`           // Why this trace is notable (triggered, route-change, high-loss, high-latency)
}

// getMtrRouteSignature generates a signature from hop IPs
func getMtrRouteSignature(hops []MtrHop) string {
	var parts []string
	for _, hop := range hops {
		if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" {
			parts = append(parts, hop.Hosts[0].IP)
		} else {
			parts = append(parts, "*")
		}
	}
	return strings.Join(parts, "->")
}

// isMtrTraceNotable checks if a trace should be preserved individually
func isMtrTraceNotable(payload MtrPayload, prevSignature string, triggered bool) (bool, string) {
	currentSignature := getMtrRouteSignature(payload.Report.Hops)

	// Check for triggered alert
	if triggered {
		return true, "triggered"
	}

	// Check for route change
	if prevSignature != "" && currentSignature != prevSignature {
		return true, "route-change"
	}

	// Check for high packet loss (>10% on responding hops only)
	// Empty hops (no host IP) are NOT packet loss - they're just routers that don't respond to ICMP
	for _, hop := range payload.Report.Hops {
		// Skip empty hops - these are NOT real packet loss
		if len(hop.Hosts) == 0 || hop.Hosts[0].IP == "" || hop.Hosts[0].IP == "*" {
			continue
		}
		loss := parseLossPct(hop.LossPct)
		if loss > 10.0 {
			return true, "high-loss"
		}
	}

	// Check for high latency (>150ms on final RESPONDING hop)
	// Find the last hop that actually has a response
	for i := len(payload.Report.Hops) - 1; i >= 0; i-- {
		hop := payload.Report.Hops[i]
		if len(hop.Hosts) > 0 && hop.Hosts[0].IP != "" && hop.Hosts[0].IP != "*" {
			if latency := parseLatency(hop.Avg); latency > 150.0 {
				return true, "high-latency"
			}
			break // Only check the last responding hop
		}
	}

	return false, ""
}

func parseLossPct(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case string:
		val = strings.TrimSuffix(val, "%")
		f, _ := strconv.ParseFloat(val, 64)
		return f
	}
	return 0
}

func parseLatency(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "ms")
	s = strings.TrimSuffix(s, " ")
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// aggregateMtrData aggregates MTR traces into time buckets, preserving notable traces
func aggregateMtrData(rawData []ProbeData, bucketDuration time.Duration, limit int) []ProbeData {
	if len(rawData) == 0 {
		return []ProbeData{}
	}

	// Sort by time ascending for route change detection
	sortedData := make([]ProbeData, len(rawData))
	copy(sortedData, rawData)
	sort.Slice(sortedData, func(i, j int) bool {
		return sortedData[i].CreatedAt.Before(sortedData[j].CreatedAt)
	})

	type mtrBucket struct {
		traces     []ProbeData
		payloads   []MtrPayload
		signatures map[string]int // signature -> count
		lastData   ProbeData
	}

	buckets := make(map[time.Time]*mtrBucket)
	notableTraces := []ProbeData{}
	var prevSignature string

	for _, d := range sortedData {
		if d.Payload == nil || len(d.Payload) == 0 {
			continue
		}

		var p MtrPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil || len(p.Report.Hops) == 0 {
			continue
		}

		currentSignature := getMtrRouteSignature(p.Report.Hops)
		isNotable, reason := isMtrTraceNotable(p, prevSignature, d.Triggered)

		if isNotable {
			// Wrap notable traces with extended metadata for frontend display
			wrapped := AggregatedMtrPayload{
				Report:                 p.Report,
				StartTimestamp:         p.StartTimestamp,
				StopTimestamp:          p.StopTimestamp,
				RouteSignature:         currentSignature,
				PreviousRouteSignature: prevSignature,
				TraceCount:             1,
				IsAggregated:           false,
				NotableReason:          reason,
			}
			wrappedPayload, _ := json.Marshal(wrapped)
			wrappedTrace := d
			wrappedTrace.Payload = wrappedPayload
			notableTraces = append(notableTraces, wrappedTrace)
		}

		// Also add to bucket for aggregation (if not notable, it will be aggregated)
		key := getBucketKey(d.CreatedAt, bucketDuration)
		b, ok := buckets[key]
		if !ok {
			b = &mtrBucket{signatures: make(map[string]int)}
			buckets[key] = b
		}

		b.traces = append(b.traces, d)
		b.payloads = append(b.payloads, p)
		b.signatures[currentSignature]++
		if d.CreatedAt.After(b.lastData.CreatedAt) {
			b.lastData = d
		}

		prevSignature = currentSignature
	}

	result := []ProbeData{}

	// First add all notable traces (these are preserved individually)
	for _, d := range notableTraces {
		result = append(result, d)
	}

	// Now create aggregated entries for each bucket
	// Only include aggregated data if there are non-notable traces in the bucket
	for bucketTime, b := range buckets {
		if len(b.payloads) == 0 {
			continue
		}

		// Find the most common route signature in this bucket
		var primarySignature string
		var maxCount int
		for sig, count := range b.signatures {
			if count > maxCount {
				primarySignature = sig
				maxCount = count
			}
		}

		// Collect payloads matching the primary route for aggregation
		var matchingPayloads []MtrPayload
		for _, p := range b.payloads {
			if getMtrRouteSignature(p.Report.Hops) == primarySignature {
				matchingPayloads = append(matchingPayloads, p)
			}
		}

		if len(matchingPayloads) == 0 {
			continue
		}

		// Aggregate the matching payloads
		aggPayload := aggregateMtrPayloads(matchingPayloads, bucketTime, primarySignature)

		payload, _ := json.Marshal(aggPayload)
		pd := b.lastData
		pd.CreatedAt = bucketTime
		pd.Payload = payload
		result = append(result, pd)
	}

	// Sort by time descending
	sortProbeDataDesc(result)

	// Deduplicate: if a notable trace is already covered by an aggregated bucket, don't double-count
	// (Notable traces are intentionally kept separate, so no dedup needed)

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

// aggregateMtrPayloads creates an aggregated MTR payload from multiple traces
func aggregateMtrPayloads(payloads []MtrPayload, bucketTime time.Time, signature string) AggregatedMtrPayload {
	if len(payloads) == 0 {
		return AggregatedMtrPayload{IsAggregated: true}
	}

	// Use the first payload as a template
	template := payloads[0]
	maxHops := len(template.Report.Hops)
	for _, p := range payloads {
		if len(p.Report.Hops) > maxHops {
			maxHops = len(p.Report.Hops)
		}
	}

	// Aggregate each hop's metrics
	aggHops := make([]MtrHop, maxHops)
	for hopIdx := 0; hopIdx < maxHops; hopIdx++ {
		var avgLatencies, bestLatencies, worstLatencies []float64
		var totalSent, totalRecv int
		var hosts []MtrHopHost
		var ttl int

		for _, p := range payloads {
			if hopIdx >= len(p.Report.Hops) {
				continue
			}
			hop := p.Report.Hops[hopIdx]
			ttl = hop.TTL
			if len(hop.Hosts) > 0 {
				hosts = hop.Hosts
			}
			totalSent += hop.Sent
			totalRecv += hop.Recv
			if lat := parseLatency(hop.Avg); lat > 0 {
				avgLatencies = append(avgLatencies, lat)
			}
			if lat := parseLatency(hop.Best); lat > 0 {
				bestLatencies = append(bestLatencies, lat)
			}
			if lat := parseLatency(hop.Worst); lat > 0 {
				worstLatencies = append(worstLatencies, lat)
			}
		}

		lossPct := 0.0
		if totalSent > 0 {
			lossPct = float64(totalSent-totalRecv) / float64(totalSent) * 100
		}

		aggHops[hopIdx] = MtrHop{
			TTL:     ttl,
			Hosts:   hosts,
			LossPct: fmt.Sprintf("%.1f%%", lossPct),
			Sent:    totalSent,
			Recv:    totalRecv,
			Avg:     fmt.Sprintf("%.2f", avg(avgLatencies)),
			Best:    fmt.Sprintf("%.2f", minF(bestLatencies)),
			Worst:   fmt.Sprintf("%.2f", maxF(worstLatencies)),
		}
	}

	return AggregatedMtrPayload{
		Report: MtrReport{
			Hops: aggHops,
		},
		StartTimestamp: bucketTime.UTC().Format(time.RFC3339),
		StopTimestamp:  bucketTime.Add(time.Minute).UTC().Format(time.RFC3339),
		RouteSignature: signature,
		TraceCount:     len(payloads),
		IsAggregated:   true,
	}
}

func getBucketKey(t time.Time, duration time.Duration) time.Time {
	return t.Truncate(duration)
}

func aggregatePingData(rawData []ProbeData, bucketDuration time.Duration, limit int) []ProbeData {
	type pingBucket struct {
		latencies    []float64
		minLatencies []float64
		maxLatencies []float64
		packetLoss   []float64
		packetsSent  uint64
		packetsRecv  uint64
		lastData     ProbeData
	}

	buckets := make(map[time.Time]*pingBucket)

	for _, d := range rawData {
		if d.Payload == nil || len(d.Payload) == 0 {
			continue
		}
		// Use pingAggInputPayload which matches the snake_case JSON format from the agent
		var p pingAggInputPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil {
			continue // Skip malformed payloads
		}

		key := getBucketKey(d.CreatedAt, bucketDuration)
		b, ok := buckets[key]
		if !ok {
			b = &pingBucket{}
			buckets[key] = b
		}

		// Convert nanoseconds to milliseconds for latency values
		avgLatencyMs := float64(p.AvgRtt) / 1e6
		minLatencyMs := float64(p.MinRtt) / 1e6
		maxLatencyMs := float64(p.MaxRtt) / 1e6

		b.latencies = append(b.latencies, avgLatencyMs)
		b.minLatencies = append(b.minLatencies, minLatencyMs)
		b.maxLatencies = append(b.maxLatencies, maxLatencyMs)
		b.packetLoss = append(b.packetLoss, p.PacketLoss)
		b.packetsSent += uint64(p.PacketsSent)
		b.packetsRecv += uint64(p.PacketsRecv)

		// Keep the most recent data for metadata
		if d.CreatedAt.After(b.lastData.CreatedAt) {
			b.lastData = d
		}
	}

	// Convert buckets to ProbeData
	result := make([]ProbeData, 0, len(buckets))
	for bucketTime, b := range buckets {
		if len(b.latencies) == 0 {
			continue
		}

		agg := AggregatedPingPayload{
			Latency:     avg(b.latencies),
			MinLatency:  minF(b.minLatencies),
			MaxLatency:  maxF(b.maxLatencies),
			AvgLatency:  avg(b.latencies),
			PacketLoss:  avg(b.packetLoss),
			PacketsSent: b.packetsSent,
			PacketsRecv: b.packetsRecv,
		}

		payload, _ := json.Marshal(agg)
		pd := b.lastData
		pd.CreatedAt = bucketTime
		pd.Payload = payload
		result = append(result, pd)
	}

	// Sort by time descending
	sortProbeDataDesc(result)

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

func aggregateTrafficSimData(rawData []ProbeData, bucketDuration time.Duration, limit int) []ProbeData {
	type tsBucket struct {
		rtts          []float64
		minRTT        float64
		maxRTT        float64
		totalPackets  uint64
		lostPackets   uint64
		outOfSequence uint64
		duplicates    uint64
		lastData      ProbeData
		initialized   bool
	}

	buckets := make(map[time.Time]*tsBucket)

	for _, d := range rawData {
		if d.Payload == nil || len(d.Payload) == 0 {
			continue
		}
		var p TrafficSimPayload
		if err := json.Unmarshal(d.Payload, &p); err != nil {
			continue
		}

		key := getBucketKey(d.CreatedAt, bucketDuration)
		b, ok := buckets[key]
		if !ok {
			b = &tsBucket{minRTT: p.MinRTT, maxRTT: p.MaxRTT}
			buckets[key] = b
		}

		b.rtts = append(b.rtts, p.AverageRTT)
		if !b.initialized || p.MinRTT < b.minRTT {
			b.minRTT = p.MinRTT
		}
		if p.MaxRTT > b.maxRTT {
			b.maxRTT = p.MaxRTT
		}
		b.totalPackets += p.TotalPackets
		b.lostPackets += p.LostPackets
		b.outOfSequence += p.OutOfSequence
		b.duplicates += p.Duplicates
		b.initialized = true

		if d.CreatedAt.After(b.lastData.CreatedAt) {
			b.lastData = d
		}
	}

	result := make([]ProbeData, 0, len(buckets))
	for bucketTime, b := range buckets {
		if len(b.rtts) == 0 {
			continue
		}

		agg := TrafficSimPayload{
			ReportTime:    bucketTime.UTC().Format(time.RFC3339),
			AverageRTT:    avg(b.rtts),
			MinRTT:        b.minRTT,
			MaxRTT:        b.maxRTT,
			TotalPackets:  b.totalPackets,
			LostPackets:   b.lostPackets,
			OutOfSequence: b.outOfSequence,
			Duplicates:    b.duplicates,
		}

		payload, _ := json.Marshal(agg)
		pd := b.lastData
		pd.CreatedAt = bucketTime
		pd.Payload = payload
		result = append(result, pd)
	}

	sortProbeDataDesc(result)

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

func bucketProbeData(rawData []ProbeData, bucketDuration time.Duration, limit int) []ProbeData {
	buckets := make(map[time.Time]ProbeData)

	for _, d := range rawData {
		key := getBucketKey(d.CreatedAt, bucketDuration)
		if existing, ok := buckets[key]; !ok || d.CreatedAt.After(existing.CreatedAt) {
			buckets[key] = d
		}
	}

	result := make([]ProbeData, 0, len(buckets))
	for _, d := range buckets {
		result = append(result, d)
	}

	sortProbeDataDesc(result)

	if limit > 0 && len(result) > limit {
		result = result[:limit]
	}

	return result
}

func avg(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func minF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxF(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func sortProbeDataDesc(data []ProbeData) {
	for i := 0; i < len(data)-1; i++ {
		for j := i + 1; j < len(data); j++ {
			if data[j].CreatedAt.After(data[i].CreatedAt) {
				data[i], data[j] = data[j], data[i]
			}
		}
	}
}

// ── Analysis Snapshot Persistence ──

// AnalysisSnapshot represents a stored workspace analysis result.
type AnalysisSnapshot struct {
	WorkspaceID     uint      `json:"workspace_id"`
	GeneratedAt     time.Time `json:"generated_at"`
	OverallHealth   float64   `json:"overall_health"`
	Grade           string    `json:"grade"`
	LatencyScore    float64   `json:"latency_score"`
	PacketLossScore float64   `json:"packet_loss_score"`
	RouteStability  float64   `json:"route_stability"`
	MosScore        float64   `json:"mos_score"`
	Status          string    `json:"status"`
	StatusMessage   string    `json:"status_message"`
	IncidentCount   int       `json:"incident_count"`
	TotalAgents     int       `json:"total_agents"`
	OnlineAgents    int       `json:"online_agents"`
	TotalProbes     int       `json:"total_probes"`
	IncidentsJSON   string    `json:"incidents_json,omitempty"`
	AgentsJSON      string    `json:"agents_json,omitempty"`
	LLMSummary      string    `json:"llm_summary,omitempty"`
}

// SaveAnalysisSnapshot persists a workspace analysis result to ClickHouse.
// The LLM summary is only stored if it was already generated during analysis
// (no additional LLM calls are made). Errors are non-fatal — callers should
// log and continue.
func SaveAnalysisSnapshot(ctx context.Context, ch *sql.DB, analysis *WorkspaceAnalysis) error {
	if analysis == nil {
		return nil
	}

	// Count online agents
	onlineCount := 0
	for _, a := range analysis.Agents {
		if a.IsOnline {
			onlineCount++
		}
	}

	// Serialize detail data as JSON
	incidentsJSON, _ := json.Marshal(analysis.Incidents)
	agentsJSON, _ := json.Marshal(analysis.Agents)

	// The status message may contain the LLM summary if enrichment was active,
	// otherwise it's the rule-based message. We always store whatever was
	// generated — no extra LLM calls.
	llmSummary := ""
	if analysis.Status.Message != "" {
		llmSummary = analysis.Status.Message
	}

	const ins = `
INSERT INTO analysis_snapshots
(workspace_id, generated_at, overall_health, grade, latency_score,
 packet_loss_score, route_stability, mos_score, status, status_message,
 incident_count, total_agents, online_agents, total_probes,
 incidents_json, agents_json, llm_summary)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`
	_, err := ch.ExecContext(ctx, ins,
		uint64(analysis.WorkspaceID),
		analysis.GeneratedAt,
		analysis.OverallHealth.OverallHealth,
		analysis.OverallHealth.Grade,
		analysis.OverallHealth.LatencyScore,
		analysis.OverallHealth.PacketLossScore,
		analysis.OverallHealth.RouteStability,
		analysis.OverallHealth.MosScore,
		analysis.Status.Status,
		analysis.Status.Message,
		uint32(len(analysis.Incidents)),
		uint32(analysis.TotalAgents),
		uint32(onlineCount),
		uint32(analysis.TotalProbes),
		string(incidentsJSON),
		string(agentsJSON),
		llmSummary,
	)
	return err
}

// GetAnalysisSnapshots returns historical analysis snapshots for a workspace.
// Results are ordered by generated_at descending (newest first).
func GetAnalysisSnapshots(
	ctx context.Context,
	ch *sql.DB,
	workspaceID uint,
	from, to time.Time,
	limit int,
) ([]AnalysisSnapshot, error) {
	var clauses []string
	clauses = append(clauses, fmt.Sprintf("workspace_id = %d", workspaceID))

	if !from.IsZero() {
		clauses = append(clauses, fmt.Sprintf("generated_at >= %s", chQuoteTime(from)))
	}
	if !to.IsZero() {
		clauses = append(clauses, fmt.Sprintf("generated_at <= %s", chQuoteTime(to)))
	}

	if limit <= 0 {
		limit = 288 // 24h of 5-min intervals
	}

	q := `
SELECT
    workspace_id, generated_at, overall_health, grade,
    latency_score, packet_loss_score, route_stability, mos_score,
    status, status_message, incident_count, total_agents,
    online_agents, total_probes, incidents_json, agents_json, llm_summary
FROM analysis_snapshots
WHERE ` + strings.Join(clauses, " AND ") + `
ORDER BY generated_at DESC
` + fmt.Sprintf("LIMIT %d", limit)

	rows, err := ch.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AnalysisSnapshot
	for rows.Next() {
		var s AnalysisSnapshot
		if err := rows.Scan(
			&s.WorkspaceID, &s.GeneratedAt, &s.OverallHealth, &s.Grade,
			&s.LatencyScore, &s.PacketLossScore, &s.RouteStability, &s.MosScore,
			&s.Status, &s.StatusMessage, &s.IncidentCount, &s.TotalAgents,
			&s.OnlineAgents, &s.TotalProbes, &s.IncidentsJSON, &s.AgentsJSON,
			&s.LLMSummary,
		); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

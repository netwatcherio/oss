// Temporary READ-ONLY diagnostic: inspects agents 41/71 and probe 672, then
// runs the real ListForAgent expansion against the live DB to verify the
// bidirectional TrafficSim flow. Delete after use.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"netwatcher-controller/internal/agent"
	"netwatcher-controller/internal/probe"
)

func envOr(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func openPG() (*gorm.DB, error) {
	parts := []string{
		"host=" + envOr("POSTGRES_HOST", "localhost"),
		"port=" + envOr("POSTGRES_PORT", "5432"),
		"user=" + os.Getenv("POSTGRES_USER"),
		"dbname=" + os.Getenv("POSTGRES_DB"),
		"sslmode=" + envOr("POSTGRES_SSLMODE", "disable"),
		"TimeZone=" + envOr("POSTGRES_TIMEZONE", "UTC"),
		"password=" + os.Getenv("POSTGRES_PASSWORD"),
	}
	return gorm.Open(postgres.Open(strings.Join(parts, " ")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func openCH() (*sql.DB, error) {
	conn := clickhouse.OpenDB(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", envOr("CLICKHOUSE_HOST", "localhost"), envOr("CLICKHOUSE_PORT", "9000"))},
		Auth: clickhouse.Auth{
			Database: envOr("CLICKHOUSE_DB", "default"),
			Username: envOr("CLICKHOUSE_USER", "default"),
			Password: os.Getenv("CLICKHOUSE_PASSWORD"),
		},
		DialTimeout: 5 * time.Second,
	})
	return conn, conn.Ping()
}

func dumpAgent(ctx context.Context, db *gorm.DB, id uint) {
	a, err := agent.GetAgentByID(ctx, db, id)
	if err != nil {
		fmt.Printf("AGENT %d: ERROR %v\n", id, err)
		return
	}
	fmt.Printf("AGENT %d %q ws=%d trafficsim_enabled=%v host=%q port=%d public_ip_override=%q is_global=%v bidir_default=%v initialized=%v last_seen=%s\n",
		a.ID, a.Name, a.WorkspaceID, a.TrafficSimEnabled, a.TrafficSimHost, a.TrafficSimPort,
		a.PublicIPOverride, a.IsGlobal, a.BidirectionalDefault, a.Initialized, a.LastSeenAt.Format(time.RFC3339))
}

func dumpProbe(ctx context.Context, db *gorm.DB, id uint) {
	p, err := probe.GetByID(ctx, db, id)
	if err != nil || p == nil {
		fmt.Printf("PROBE %d: ERROR %v\n", id, err)
		return
	}
	fmt.Printf("PROBE %d type=%s agent=%d ws=%d enabled=%v server=%v metadata=%s\n",
		p.ID, p.Type, p.AgentID, p.WorkspaceID, p.Enabled, p.Server, string(p.Metadata))
	for _, t := range p.Targets {
		aid := "nil"
		if t.AgentID != nil {
			aid = fmt.Sprintf("%d", *t.AgentID)
		}
		fmt.Printf("  TARGET id=%d target=%q agent_id=%s\n", t.ID, t.Target, aid)
	}
}

func summarize(list []probe.Probe) {
	for _, p := range list {
		var md map[string]any
		_ = json.Unmarshal(p.Metadata, &md)
		tgts := make([]string, 0, len(p.Targets))
		for _, t := range p.Targets {
			aid := "-"
			if t.AgentID != nil {
				aid = fmt.Sprintf("%d", *t.AgentID)
			}
			tgts = append(tgts, fmt.Sprintf("%q(agent:%s)", t.Target, aid))
		}
		fmt.Printf("  id=%d type=%s owner=%d server=%v targets=[%s] metadata=%s\n",
			p.ID, p.Type, p.AgentID, p.Server, strings.Join(tgts, ", "), string(p.Metadata))
	}
}

func main() {
	ctx := context.Background()

	db, err := openPG()
	if err != nil {
		fmt.Println("postgres:", err)
		os.Exit(1)
	}
	ch, err := openCH()
	if err != nil {
		fmt.Println("clickhouse:", err)
		os.Exit(1)
	}

	fmt.Println("==== AGENTS ====")
	dumpAgent(ctx, db, 41)
	dumpAgent(ctx, db, 71)

	fmt.Println("\n==== PROBE 672 ====")
	dumpProbe(ctx, db, 672)

	fmt.Println("\n==== ALL PROBES OWNED BY 41 / 71 (raw DB) ====")
	for _, id := range []uint{41, 71} {
		list, err := probe.ListByAgent(ctx, db, id)
		if err != nil {
			fmt.Printf("agent %d: %v\n", id, err)
			continue
		}
		fmt.Printf("agent %d owns %d probes:\n", id, len(list))
		summarize(list)
	}

	fmt.Println("\n==== ListForAgent(41) — what agent 41 would receive ====")
	out41, err := probe.ListForAgent(ctx, db, ch, 41)
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	summarize(out41)

	fmt.Println("\n==== ListForAgent(71) — what agent 71 would receive ====")
	out71, err := probe.ListForAgent(ctx, db, ch, 71)
	if err != nil {
		fmt.Println("ERROR:", err)
	}
	summarize(out71)

	fmt.Println("\n==== probe_data rows for probe 672 (last 2h, by reporter/type) ====")
	rows, err := ch.QueryContext(ctx, `
		SELECT agent_id, type, count() AS n, min(created_at) AS first, max(created_at) AS last
		FROM probe_data
		WHERE probe_id = 672 AND created_at > now() - INTERVAL 2 HOUR
		GROUP BY agent_id, type
		ORDER BY type, agent_id`)
	if err != nil {
		fmt.Println("ERROR:", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var agentID uint64
			var typ string
			var n uint64
			var first, last time.Time
			if err := rows.Scan(&agentID, &typ, &n, &first, &last); err != nil {
				fmt.Println("scan:", err)
				break
			}
			fmt.Printf("  reporter=%d type=%-12s rows=%-6d first=%s last=%s\n",
				agentID, typ, n, first.Format("15:04:05"), last.Format("15:04:05"))
		}
	}

	fmt.Println("\n==== ComputeProbeAnalysis(ws=2, probe=672, 60m) ====")
	if pa, err := probe.ComputeProbeAnalysis(ctx, ch, db, 2, 672, 60); err != nil {
		fmt.Println("ERROR:", err)
	} else {
		fmt.Printf("FORWARD  %s → %s: health=%.0f (%s) MOS=%.2f lat=%.1fms loss=%.2f%% jitter=%.1fms samples=%d\n",
			pa.AgentName, pa.Target, pa.Health.OverallHealth, pa.Health.Grade, pa.Health.MosScore,
			pa.Metrics.AvgLatency, pa.Metrics.PacketLoss, pa.Metrics.JitterAvg, pa.Metrics.SampleCount)
		if pa.Reverse != nil {
			fmt.Printf("REVERSE  %s → %s: health=%.0f (%s) MOS=%.2f lat=%.1fms loss=%.2f%% jitter=%.1fms samples=%d\n",
				pa.Reverse.AgentName, pa.Reverse.Target, pa.Reverse.Health.OverallHealth, pa.Reverse.Health.Grade,
				pa.Reverse.Health.MosScore, pa.Reverse.Metrics.AvgLatency, pa.Reverse.Metrics.PacketLoss,
				pa.Reverse.Metrics.JitterAvg, pa.Reverse.Metrics.SampleCount)
		} else {
			fmt.Println("REVERSE  (none)")
		}
		if pa.CombinedHealth != nil {
			fmt.Printf("COMBINED health=%.0f (%s) MOS=%.2f\n",
				pa.CombinedHealth.OverallHealth, pa.CombinedHealth.Grade, pa.CombinedHealth.MosScore)
		}
		for _, s := range pa.Signals {
			fmt.Printf("SIGNAL   [%s] %s — %s\n", s.Severity, s.Title, s.Evidence)
		}
		for _, f := range pa.Findings {
			fmt.Printf("FINDING  [%s/%s] %s\n", f.Severity, f.Category, f.Title)
		}
	}

	fmt.Println("\n==== NETINFO public_address history (last 6h) ====")
	for _, aid := range []uint64{41, 71} {
		nrows, err := ch.QueryContext(ctx, `
			SELECT created_at, JSONExtractString(payload_raw, 'public_address') AS ip
			FROM probe_data
			WHERE type = 'NETINFO' AND agent_id = ? AND created_at > now() - INTERVAL 6 HOUR
			ORDER BY created_at DESC LIMIT 8`, aid)
		if err != nil {
			fmt.Printf("agent %d: %v\n", aid, err)
			continue
		}
		fmt.Printf("agent %d:\n", aid)
		for nrows.Next() {
			var at time.Time
			var ip string
			if err := nrows.Scan(&at, &ip); err != nil {
				break
			}
			fmt.Printf("  %s public_address=%s\n", at.Format("15:04:05"), ip)
		}
		nrows.Close()
	}

	fmt.Println("\n==== last 3 TRAFFICSIM payloads for probe 672, per reporter ====")
	for _, aid := range []uint64{41, 71} {
		prows, err := ch.QueryContext(ctx, `
			SELECT created_at, target, target_agent, payload_raw
			FROM probe_data
			WHERE probe_id = 672 AND type = 'TRAFFICSIM' AND agent_id = ?
			ORDER BY created_at DESC LIMIT 3`, aid)
		if err != nil {
			fmt.Printf("reporter %d: %v\n", aid, err)
			continue
		}
		fmt.Printf("reporter %d:\n", aid)
		for prows.Next() {
			var at time.Time
			var target, payload string
			var targetAgent uint64
			if err := prows.Scan(&at, &target, &targetAgent, &payload); err != nil {
				fmt.Println("scan:", err)
				break
			}
			if len(payload) > 220 {
				payload = payload[:220] + "..."
			}
			fmt.Printf("  %s target=%q target_agent=%d payload=%s\n", at.Format("15:04:05"), target, targetAgent, payload)
		}
		prows.Close()
	}
}

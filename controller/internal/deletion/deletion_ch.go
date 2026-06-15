package deletion

import (
	"context"
	"database/sql"
	"fmt"
)

// CHOps is the abstraction the worker uses to dispatch ClickHouse mutations.
// Production code wires a *CHClient; tests can substitute an in-memory fake.
type CHOps interface {
	DeleteProbeDataByProbeID(ctx context.Context, probeID uint) error
	DeleteProbeDataByAgentID(ctx context.Context, agentID uint) error
}

// CHClient is a *sql.DB-backed implementation of CHOps.
type CHClient struct {
	DB *sql.DB
}

func (c *CHClient) DeleteProbeDataByProbeID(ctx context.Context, probeID uint) error {
	q := fmt.Sprintf("ALTER TABLE probe_data DELETE WHERE probe_id = %d", probeID)
	if _, err := c.DB.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("clickhouse delete by probe_id=%d: %w", probeID, err)
	}
	return nil
}

func (c *CHClient) DeleteProbeDataByAgentID(ctx context.Context, agentID uint) error {
	q := fmt.Sprintf("ALTER TABLE probe_data DELETE WHERE agent_id = %d", agentID)
	if _, err := c.DB.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("clickhouse delete by agent_id=%d: %w", agentID, err)
	}
	return nil
}

// DeleteProbeDataByProbeID is a package-level convenience for non-worker callers.
func DeleteProbeDataByProbeID(ctx context.Context, ch *sql.DB, probeID uint) error {
	return (&CHClient{DB: ch}).DeleteProbeDataByProbeID(ctx, probeID)
}

// DeleteProbeDataByAgentID is a package-level convenience for non-worker callers.
func DeleteProbeDataByAgentID(ctx context.Context, ch *sql.DB, agentID uint) error {
	return (&CHClient{DB: ch}).DeleteProbeDataByAgentID(ctx, agentID)
}

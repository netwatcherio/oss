package probe

import (
	"context"
	"database/sql"
	"encoding/json"

	"netwatcher-controller/internal/alert"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// SaveRecordWithAlertEval saves the probe data to ClickHouse AND evaluates alert rules.
// This should be called instead of SaveRecordCH when alert evaluation is desired.
func SaveRecordWithAlertEval(
	ctx context.Context,
	ch *sql.DB,
	pg *gorm.DB,
	data ProbeData,
	kind string,
	payload any,
) error {
	// First, save to ClickHouse
	if err := SaveRecordCH(ctx, ch, data, kind, payload); err != nil {
		return err
	}

	// Only evaluate alerts for types that have metrics we can check
	if kind != string(TypePing) && kind != string(TypeTrafficSim) {
		return nil
	}

	// Look up the probe to get workspace ID
	var probe Probe
	if err := pg.WithContext(ctx).First(&probe, data.ProbeID).Error; err != nil {
		// Don't fail the save operation if we can't find the probe
		log.Warnf("alert_hook: could not find probe %d for alert evaluation: %v", data.ProbeID, err)
		return nil
	}

	// Look up the agent to get workspace ID
	type agentWS struct {
		WorkspaceID uint
	}
	var agent agentWS
	if err := pg.WithContext(ctx).Table("agents").Select("workspace_id").Where("id = ?", probe.AgentID).First(&agent).Error; err != nil {
		log.Warnf("alert_hook: could not find agent %d for alert evaluation: %v", probe.AgentID, err)
		return nil
	}

	// Convert payload to JSON for the alert evaluator
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Warnf("alert_hook: could not marshal payload for alert evaluation: %v", err)
		return nil
	}

	// Evaluate alerts (non-blocking, log errors)
	if err := alert.EvaluateProbeData(ctx, pg, data.ProbeID, agent.WorkspaceID, kind, payloadJSON); err != nil {
		log.Warnf("alert_hook: alert evaluation failed: %v", err)
	}

	return nil
}

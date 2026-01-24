package probe

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

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
	if kind != string(TypePing) && kind != string(TypeTrafficSim) && kind != string(TypeMTR) && kind != string(TypeSysInfo) {
		return nil
	}

	// Look up the probe to get workspace ID and details
	var probe Probe
	if err := pg.WithContext(ctx).Preload("Targets").First(&probe, data.ProbeID).Error; err != nil {
		// Don't fail the save operation if we can't find the probe
		log.Warnf("alert_hook: could not find probe %d for alert evaluation: %v", data.ProbeID, err)
		return nil
	}

	// Build probe target string
	var targetStr string
	if len(probe.Targets) > 0 {
		targets := make([]string, 0, len(probe.Targets))
		for _, t := range probe.Targets {
			if t.Target != "" {
				targets = append(targets, t.Target)
			}
		}
		targetStr = strings.Join(targets, ", ")
	}

	// Look up the agent to get workspace ID and name
	type agentInfo struct {
		WorkspaceID uint
		Name        string
	}
	var agent agentInfo
	if err := pg.WithContext(ctx).Table("agents").Select("workspace_id, name").Where("id = ?", probe.AgentID).First(&agent).Error; err != nil {
		log.Warnf("alert_hook: could not find agent %d for alert evaluation: %v", probe.AgentID, err)
		return nil
	}

	// Convert payload to JSON for the alert evaluator
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		log.Warnf("alert_hook: could not marshal payload for alert evaluation: %v", err)
		return nil
	}

	// Build probe context with enriched information
	pctx := alert.ProbeContext{
		ProbeID:     data.ProbeID,
		ProbeType:   kind,
		ProbeName:   string(probe.Type), // Use probe type as display name
		ProbeTarget: targetStr,
		AgentID:     probe.AgentID,
		AgentName:   agent.Name,
		WorkspaceID: agent.WorkspaceID,
	}

	// Evaluate alerts (non-blocking, log errors)
	if err := alert.EvaluateProbeData(ctx, pg, pctx, payloadJSON); err != nil {
		log.Warnf("alert_hook: alert evaluation failed: %v", err)
	}

	return nil
}

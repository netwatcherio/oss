package probe

import (
	"context"
	"fmt"
	"strings"

	"netwatcher-controller/internal/alert"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// EvaluateAnalysisIncidents evaluates AI analysis incidents against workspace alert rules
// and creates alerts for any matching rules. This bridges the analysis engine into the
// existing alert pipeline (panel, webhook, email notifications).
func EvaluateAnalysisIncidents(ctx context.Context, pg *gorm.DB, workspaceID uint, analysis *WorkspaceAnalysis) error {
	if analysis == nil {
		return nil
	}

	// Load workspace alert rules that use AI analysis metrics
	rules, err := alert.ListRulesByWorkspace(ctx, pg, workspaceID)
	if err != nil {
		return fmt.Errorf("loading alert rules: %w", err)
	}

	// Filter to only AI analysis metric rules
	var analysisRules []alert.AlertRule
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if isAnalysisMetric(rule.Metric) {
			analysisRules = append(analysisRules, rule)
		}
	}

	if len(analysisRules) == 0 {
		return nil
	}

	for _, rule := range analysisRules {
		results := evaluateAnalysisRule(&rule, analysis)
		for _, result := range results {
			if !result.triggered {
				continue
			}

			// Deduplicate: check if an active alert already exists for this rule
			if hasActiveAlert(ctx, pg, rule.ID) {
				continue
			}

			// Create alert through the existing pipeline
			actx := &alert.AlertContext{
				AgentName: result.agentName,
			}
			if result.agentID != 0 {
				actx.AgentID = result.agentID
			}

			alertInstance, err := alert.CreateAlert(ctx, pg, &rule, result.value, result.message, actx)
			if err != nil {
				log.Warnf("[analysis_alert] failed to create alert for rule %d: %v", rule.ID, err)
				continue
			}

			// Dispatch notifications (webhook, email, panel)
			alert.DispatchNotifications(ctx, pg, &rule, alertInstance)
			log.Infof("[analysis_alert] triggered alert %d for rule '%s' (workspace %d): %s",
				alertInstance.ID, rule.Name, workspaceID, result.message)
		}
	}

	return nil
}

// isAnalysisMetric checks if a metric is an AI analysis type
func isAnalysisMetric(m alert.Metric) bool {
	switch m {
	case alert.MetricHealthScore,
		alert.MetricLatencyBaseline,
		alert.MetricLossBaseline,
		alert.MetricIPChange,
		alert.MetricISPChange,
		alert.MetricIncidentCount:
		return true
	}
	return false
}

type analysisEvalResult struct {
	triggered bool
	value     float64
	message   string
	agentName string
	agentID   uint
}

func evaluateAnalysisRule(rule *alert.AlertRule, analysis *WorkspaceAnalysis) []analysisEvalResult {
	var results []analysisEvalResult

	switch rule.Metric {
	case alert.MetricHealthScore:
		value := analysis.OverallHealth.OverallHealth
		if alert.ShouldTrigger(rule.Operator, value, rule.Threshold) {
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     value,
				message:   fmt.Sprintf("Workspace health score dropped to %.0f (threshold: %.0f)", value, rule.Threshold),
			})
		}

	case alert.MetricIncidentCount:
		// Count incidents matching severity if specified
		count := float64(len(analysis.Incidents))
		if alert.ShouldTrigger(rule.Operator, count, rule.Threshold) {
			severities := countSeverities(analysis.Incidents)
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     count,
				message:   fmt.Sprintf("%d active incidents detected (%s)", int(count), severities),
			})
		}

	case alert.MetricLatencyBaseline:
		for _, inc := range analysis.Incidents {
			if !strings.Contains(inc.ID, "latency_regression") {
				continue
			}
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     1,
				message:   inc.Title + " — " + inc.SuggestedCause,
				agentName: firstOrEmpty(inc.AffectedAgents),
			})
		}

	case alert.MetricLossBaseline:
		for _, inc := range analysis.Incidents {
			if !strings.Contains(inc.ID, "loss_regression") {
				continue
			}
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     1,
				message:   inc.Title + " — " + inc.SuggestedCause,
				agentName: firstOrEmpty(inc.AffectedAgents),
			})
		}

	case alert.MetricIPChange:
		for _, inc := range analysis.Incidents {
			if !strings.Contains(inc.ID, "ip_change") {
				continue
			}
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     1,
				message:   inc.Title + " — " + strings.Join(inc.Evidence, "; "),
				agentName: firstOrEmpty(inc.AffectedAgents),
			})
		}

	case alert.MetricISPChange:
		for _, inc := range analysis.Incidents {
			if !strings.Contains(inc.ID, "isp_change") {
				continue
			}
			results = append(results, analysisEvalResult{
				triggered: true,
				value:     1,
				message:   inc.Title + " — " + inc.SuggestedCause,
				agentName: firstOrEmpty(inc.AffectedAgents),
			})
		}
	}

	return results
}

func hasActiveAlert(ctx context.Context, pg *gorm.DB, ruleID uint) bool {
	var count int64
	pg.WithContext(ctx).Model(&alert.Alert{}).
		Where("alert_rule_id = ? AND status = ?", ruleID, alert.StatusActive).
		Count(&count)
	return count > 0
}

func countSeverities(incidents []DetectedIncident) string {
	critical, warning, info := 0, 0, 0
	for _, inc := range incidents {
		switch inc.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		default:
			info++
		}
	}
	parts := []string{}
	if critical > 0 {
		parts = append(parts, fmt.Sprintf("%d critical", critical))
	}
	if warning > 0 {
		parts = append(parts, fmt.Sprintf("%d warning", warning))
	}
	if info > 0 {
		parts = append(parts, fmt.Sprintf("%d info", info))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func firstOrEmpty(s []string) string {
	if len(s) > 0 {
		return s[0]
	}
	return ""
}

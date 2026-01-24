package alert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// -------------------- Types & Constants --------------------

type Metric string
type Operator string
type Severity string
type Status string
type LogicalOperator string

const (
	MetricPacketLoss Metric = "packet_loss"
	MetricLatency    Metric = "latency"
	MetricJitter     Metric = "jitter"
	MetricOffline    Metric = "offline" // Agent offline (minutes since last seen)
	// MTR-specific metrics
	MetricEndHopLoss    Metric = "end_hop_loss"    // Final destination packet loss %
	MetricEndHopLatency Metric = "end_hop_latency" // Final destination avg latency (ms)
	MetricRouteChange   Metric = "route_change"    // Route differs from baseline
	MetricWorstHopLoss  Metric = "worst_hop_loss"  // Any hop exceeding loss threshold
	// SYSINFO metrics
	MetricCpuUsage    Metric = "cpu_usage"    // CPU usage percentage
	MetricMemoryUsage Metric = "memory_usage" // Memory usage percentage
)

const (
	LogicalAnd LogicalOperator = "AND"
	LogicalOr  LogicalOperator = "OR"
)

const (
	OperatorGT  Operator = "gt"  // greater than
	OperatorLT  Operator = "lt"  // less than
	OperatorGTE Operator = "gte" // greater than or equal
	OperatorLTE Operator = "lte" // less than or equal
	OperatorEQ  Operator = "eq"  // equal
)

const (
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

const (
	StatusActive       Status = "active"
	StatusAcknowledged Status = "acknowledged"
	StatusResolved     Status = "resolved"
)

var (
	ErrNotFound  = errors.New("alert not found")
	ErrBadInput  = errors.New("invalid input")
	ErrForbidden = errors.New("forbidden")
)

// -------------------- Models --------------------

// AlertRule configures when to trigger alerts on a probe or agent
type AlertRule struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	WorkspaceID uint   `gorm:"index;not null" json:"workspace_id"`
	ProbeID     *uint  `gorm:"index" json:"probe_id,omitempty"` // nil = workspace-wide default
	AgentID     *uint  `gorm:"index" json:"agent_id,omitempty"` // nil = workspace-wide default
	Name        string `gorm:"size:128" json:"name"`
	Description string `gorm:"size:512" json:"description,omitempty"`

	// Primary condition
	Metric    Metric   `gorm:"type:VARCHAR(32);index" json:"metric"`
	Operator  Operator `gorm:"type:VARCHAR(8)" json:"operator"`
	Threshold float64  `json:"threshold"`
	Severity  Severity `gorm:"type:VARCHAR(16);default:'warning'" json:"severity"`

	// Optional secondary condition for compound alerts (e.g., latency > 100 OR packet_loss > 5)
	Metric2    *Metric         `gorm:"type:VARCHAR(32)" json:"metric2,omitempty"`
	Operator2  *Operator       `gorm:"type:VARCHAR(8)" json:"operator2,omitempty"`
	Threshold2 *float64        `json:"threshold2,omitempty"`
	LogicalOp  LogicalOperator `gorm:"type:VARCHAR(8);default:'AND'" json:"logical_op"`

	// Notification channels
	NotifyPanel   bool   `gorm:"default:true" json:"notify_panel"`         // Show in panel alerts (always on)
	NotifyEmail   bool   `gorm:"default:false" json:"notify_email"`        // Email workspace members
	NotifyWebhook bool   `gorm:"default:false" json:"notify_webhook"`      // Send to webhook URL
	WebhookURL    string `gorm:"size:512" json:"webhook_url,omitempty"`    // Webhook endpoint
	WebhookSecret string `gorm:"size:128" json:"webhook_secret,omitempty"` // Optional HMAC secret

	Enabled bool `gorm:"default:true;index" json:"enabled"`
}

func (AlertRule) TableName() string { return "alert_rules" }

// Alert stores triggered alert instances
type Alert struct {
	ID        uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	UpdatedAt time.Time      `gorm:"index" json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`

	AlertRuleID uint  `gorm:"index;not null" json:"alert_rule_id"`
	WorkspaceID uint  `gorm:"index;not null" json:"workspace_id"`
	ProbeID     *uint `gorm:"index" json:"probe_id,omitempty"`
	AgentID     *uint `gorm:"index" json:"agent_id,omitempty"`

	// Contextual fields (denormalized for historical accuracy)
	ProbeType   string `gorm:"size:64" json:"probe_type,omitempty"`
	ProbeName   string `gorm:"size:256" json:"probe_name,omitempty"`
	ProbeTarget string `gorm:"size:512" json:"probe_target,omitempty"`
	AgentName   string `gorm:"size:256" json:"agent_name,omitempty"`

	Metric    Metric   `gorm:"type:VARCHAR(32)" json:"metric"`
	Value     float64  `json:"value"`
	Threshold float64  `json:"threshold"`
	Severity  Severity `gorm:"type:VARCHAR(16)" json:"severity"`
	Status    Status   `gorm:"type:VARCHAR(16);default:'active';index" json:"status"`
	Message   string   `gorm:"size:512" json:"message,omitempty"`

	TriggeredAt    time.Time  `gorm:"index" json:"triggered_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
	AcknowledgedAt *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy *uint      `json:"acknowledged_by,omitempty"`
}

func (Alert) TableName() string { return "alerts" }

// -------------------- DTOs --------------------

type CreateRuleInput struct {
	WorkspaceID uint   `json:"workspace_id"`
	ProbeID     *uint  `json:"probe_id,omitempty"`
	AgentID     *uint  `json:"agent_id,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// Primary condition
	Metric    Metric   `json:"metric"`
	Operator  Operator `json:"operator"`
	Threshold float64  `json:"threshold"`
	Severity  Severity `json:"severity,omitempty"`
	// Optional secondary condition (compound alert)
	Metric2    *Metric         `json:"metric2,omitempty"`
	Operator2  *Operator       `json:"operator2,omitempty"`
	Threshold2 *float64        `json:"threshold2,omitempty"`
	LogicalOp  LogicalOperator `json:"logical_op,omitempty"`
	Enabled    *bool           `json:"enabled,omitempty"`
	// Notification channels
	NotifyPanel   *bool  `json:"notify_panel,omitempty"`
	NotifyEmail   *bool  `json:"notify_email,omitempty"`
	NotifyWebhook *bool  `json:"notify_webhook,omitempty"`
	WebhookURL    string `json:"webhook_url,omitempty"`
	WebhookSecret string `json:"webhook_secret,omitempty"`
}

type UpdateRuleInput struct {
	ID          uint
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	// Primary condition
	Metric    *Metric   `json:"metric,omitempty"`
	Operator  *Operator `json:"operator,omitempty"`
	Threshold *float64  `json:"threshold,omitempty"`
	Severity  *Severity `json:"severity,omitempty"`
	// Optional secondary condition (compound alert)
	Metric2    *Metric          `json:"metric2,omitempty"`
	Operator2  *Operator        `json:"operator2,omitempty"`
	Threshold2 *float64         `json:"threshold2,omitempty"`
	LogicalOp  *LogicalOperator `json:"logical_op,omitempty"`
	Enabled    *bool            `json:"enabled,omitempty"`
	// Notification channels
	NotifyPanel   *bool   `json:"notify_panel,omitempty"`
	NotifyEmail   *bool   `json:"notify_email,omitempty"`
	NotifyWebhook *bool   `json:"notify_webhook,omitempty"`
	WebhookURL    *string `json:"webhook_url,omitempty"`
	WebhookSecret *string `json:"webhook_secret,omitempty"`
}

// -------------------- CRUD Operations --------------------

// Migrate creates the tables
func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&AlertRule{}, &Alert{})
}

// CreateRule creates a new alert rule
func CreateRule(ctx context.Context, db *gorm.DB, in CreateRuleInput) (*AlertRule, error) {
	if in.WorkspaceID == 0 {
		return nil, fmt.Errorf("%w: workspace_id required", ErrBadInput)
	}
	if in.Metric == "" || in.Operator == "" {
		return nil, fmt.Errorf("%w: metric and operator required", ErrBadInput)
	}

	severity := in.Severity
	if severity == "" {
		severity = SeverityWarning
	}

	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}

	// Default logical operator to AND if secondary condition is provided
	logicalOp := in.LogicalOp
	if logicalOp == "" && in.Metric2 != nil {
		logicalOp = LogicalAnd
	}

	rule := &AlertRule{
		WorkspaceID: in.WorkspaceID,
		ProbeID:     in.ProbeID,
		AgentID:     in.AgentID,
		Name:        in.Name,
		Description: in.Description,
		Metric:      in.Metric,
		Operator:    in.Operator,
		Threshold:   in.Threshold,
		Severity:    severity,
		Metric2:     in.Metric2,
		Operator2:   in.Operator2,
		Threshold2:  in.Threshold2,
		LogicalOp:   logicalOp,
		Enabled:     enabled,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := db.WithContext(ctx).Create(rule).Error; err != nil {
		return nil, err
	}
	return rule, nil
}

// GetRuleByID retrieves a rule by its ID
func GetRuleByID(ctx context.Context, db *gorm.DB, id uint) (*AlertRule, error) {
	var rule AlertRule
	err := db.WithContext(ctx).First(&rule, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &rule, err
}

// ListRulesByWorkspace returns all rules for a workspace
func ListRulesByWorkspace(ctx context.Context, db *gorm.DB, workspaceID uint) ([]AlertRule, error) {
	var rules []AlertRule
	err := db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("id DESC").
		Find(&rules).Error
	return rules, err
}

// UpdateRule updates an existing alert rule
func UpdateRule(ctx context.Context, db *gorm.DB, in UpdateRuleInput) (*AlertRule, error) {
	if in.ID == 0 {
		return nil, fmt.Errorf("%w: id required", ErrBadInput)
	}

	updates := map[string]any{"updated_at": time.Now()}
	if in.Name != nil {
		updates["name"] = *in.Name
	}
	if in.Description != nil {
		updates["description"] = *in.Description
	}
	if in.Metric != nil {
		updates["metric"] = *in.Metric
	}
	if in.Operator != nil {
		updates["operator"] = *in.Operator
	}
	if in.Threshold != nil {
		updates["threshold"] = *in.Threshold
	}
	if in.Severity != nil {
		updates["severity"] = *in.Severity
	}
	// Secondary condition fields
	if in.Metric2 != nil {
		updates["metric2"] = *in.Metric2
	}
	if in.Operator2 != nil {
		updates["operator2"] = *in.Operator2
	}
	if in.Threshold2 != nil {
		updates["threshold2"] = *in.Threshold2
	}
	if in.LogicalOp != nil {
		updates["logical_op"] = *in.LogicalOp
	}
	if in.Enabled != nil {
		updates["enabled"] = *in.Enabled
	}

	res := db.WithContext(ctx).Model(&AlertRule{}).Where("id = ?", in.ID).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}

	return GetRuleByID(ctx, db, in.ID)
}

// DeleteRule deletes an alert rule
func DeleteRule(ctx context.Context, db *gorm.DB, id uint) error {
	res := db.WithContext(ctx).Delete(&AlertRule{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// -------------------- Alert CRUD --------------------

// AlertContext provides contextual information about the probe/agent for alert display
type AlertContext struct {
	ProbeID     uint
	ProbeType   string
	ProbeName   string
	ProbeTarget string
	AgentID     uint
	AgentName   string
}

// CreateAlert creates a new alert instance with optional contextual information
func CreateAlert(ctx context.Context, db *gorm.DB, rule *AlertRule, value float64, message string, actx *AlertContext) (*Alert, error) {
	alert := &Alert{
		AlertRuleID: rule.ID,
		WorkspaceID: rule.WorkspaceID,
		ProbeID:     rule.ProbeID,
		AgentID:     rule.AgentID,
		Metric:      rule.Metric,
		Value:       value,
		Threshold:   rule.Threshold,
		Severity:    rule.Severity,
		Status:      StatusActive,
		Message:     message,
		TriggeredAt: time.Now(),
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Populate contextual fields if provided
	if actx != nil {
		if actx.ProbeID != 0 {
			alert.ProbeID = &actx.ProbeID
		}
		if actx.AgentID != 0 {
			alert.AgentID = &actx.AgentID
		}
		alert.ProbeType = actx.ProbeType
		alert.ProbeName = actx.ProbeName
		alert.ProbeTarget = actx.ProbeTarget
		alert.AgentName = actx.AgentName
	}

	if err := db.WithContext(ctx).Create(alert).Error; err != nil {
		return nil, err
	}
	return alert, nil
}

// GetAlertByID retrieves an alert by ID
func GetAlertByID(ctx context.Context, db *gorm.DB, id uint) (*Alert, error) {
	var alert Alert
	err := db.WithContext(ctx).First(&alert, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &alert, err
}

// ListAlerts returns alerts with optional filters
func ListAlerts(ctx context.Context, db *gorm.DB, workspaceID *uint, status *Status, limit int) ([]Alert, error) {
	query := db.WithContext(ctx).Model(&Alert{})

	if workspaceID != nil {
		query = query.Where("workspace_id = ?", *workspaceID)
	}
	if status != nil {
		query = query.Where("status = ?", *status)
	}

	if limit > 0 {
		query = query.Limit(limit)
	}

	var alerts []Alert
	err := query.Order("triggered_at DESC").Find(&alerts).Error
	return alerts, err
}

// CountActiveAlerts counts unresolved alerts
func CountActiveAlerts(ctx context.Context, db *gorm.DB, userWorkspaceIDs []uint) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&Alert{}).
		Where("status = ? AND workspace_id IN ?", StatusActive, userWorkspaceIDs).
		Count(&count).Error
	return count, err
}

// AcknowledgeAlert marks an alert as acknowledged
func AcknowledgeAlert(ctx context.Context, db *gorm.DB, id uint, userID uint) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&Alert{}).Where("id = ?", id).Updates(map[string]any{
		"status":          StatusAcknowledged,
		"acknowledged_at": now,
		"acknowledged_by": userID,
		"updated_at":      now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ResolveAlert marks an alert as resolved
func ResolveAlert(ctx context.Context, db *gorm.DB, id uint) error {
	now := time.Now()
	res := db.WithContext(ctx).Model(&Alert{}).Where("id = ?", id).Updates(map[string]any{
		"status":      StatusResolved,
		"resolved_at": now,
		"updated_at":  now,
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

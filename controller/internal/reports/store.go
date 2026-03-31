package reports

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

type ReportType string

const (
	ReportTypeWorkspaceSummary ReportType = "workspace_summary"
	ReportTypeProbeDetail      ReportType = "probe_detail"
	ReportTypeSLA              ReportType = "sla"
)

type ReportConfig struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	WorkspaceID     uint       `gorm:"not null;index" json:"workspace_id"`
	Name            string     `gorm:"size:255" json:"name"`
	Description     string     `gorm:"size:500" json:"description"`
	ReportType      ReportType `gorm:"size:50;not null" json:"report_type"`
	Schedule        string     `gorm:"size:100" json:"schedule,omitempty"`
	EmailEnabled    bool       `gorm:"default:false" json:"email_enabled"`
	EmailRecipients string     `gorm:"type:text" json:"email_recipients"`
	LastRunAt       *time.Time `json:"last_run_at,omitempty"`
	LastError       string     `gorm:"type:text" json:"last_error,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (ReportConfig) TableName() string { return "report_configs" }

type ReportConfigJSON struct {
	TimeRangeDays int64   `json:"time_range_days"`
	AgentIDs      []uint  `json:"agent_ids,omitempty"`
	ProbeIDs      []uint  `json:"probe_ids,omitempty"`
	IncludeSLA    bool    `json:"include_sla"`
	IncludeAlerts bool    `json:"include_alerts"`
	SLATarget     float64 `json:"sla_target,omitempty"`
}

type ReportConfigDetails struct {
	ID              uint             `json:"id"`
	WorkspaceID     uint             `json:"workspace_id"`
	Name            string           `json:"name"`
	Description     string           `json:"description"`
	ReportType      ReportType       `json:"report_type"`
	Schedule        string           `json:"schedule,omitempty"`
	EmailEnabled    bool             `json:"email_enabled"`
	EmailRecipients []string         `json:"email_recipients"`
	LastRunAt       *time.Time       `json:"last_run_at,omitempty"`
	LastError       string           `json:"last_error,omitempty"`
	Config          ReportConfigJSON `json:"config"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) AutoMigrate(ctx context.Context) error {
	return s.db.WithContext(ctx).AutoMigrate(&ReportConfig{})
}

func (s *Store) Create(ctx context.Context, cfg *ReportConfig) error {
	return s.db.WithContext(ctx).Create(cfg).Error
}

func (s *Store) GetByID(ctx context.Context, id uint) (*ReportConfig, error) {
	var cfg ReportConfig
	if err := s.db.WithContext(ctx).First(&cfg, id).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (s *Store) GetByWorkspace(ctx context.Context, workspaceID uint) ([]ReportConfig, error) {
	var configs []ReportConfig
	err := s.db.WithContext(ctx).
		Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Find(&configs).Error
	return configs, err
}

func (s *Store) GetScheduled(ctx context.Context) ([]ReportConfig, error) {
	var configs []ReportConfig
	err := s.db.WithContext(ctx).
		Where("schedule IS NOT NULL AND schedule != ''").
		Find(&configs).Error
	return configs, err
}

func (s *Store) Update(ctx context.Context, cfg *ReportConfig) error {
	return s.db.WithContext(ctx).Save(cfg).Error
}

func (s *Store) Delete(ctx context.Context, id uint) error {
	return s.db.WithContext(ctx).Delete(&ReportConfig{}, id).Error
}

func (s *Store) UpdateLastRun(ctx context.Context, id uint, errMsg string) error {
	now := time.Now()
	updates := map[string]any{
		"last_run_at": now,
		"last_error":  errMsg,
	}
	return s.db.WithContext(ctx).Model(&ReportConfig{}).Where("id = ?", id).Updates(updates).Error
}

func ParseEmailRecipients(s string) []string {
	if s == "" {
		return nil
	}
	var recipients []string
	json.Unmarshal([]byte(s), &recipients)
	return recipients
}

func SerializeEmailRecipients(recipients []string) string {
	if len(recipients) == 0 {
		return ""
	}
	b, _ := json.Marshal(recipients)
	return string(b)
}

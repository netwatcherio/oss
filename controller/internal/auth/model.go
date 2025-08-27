package auth

import (
	"time"

	"gorm.io/gorm"
)

type Session struct {
	// Item being authenticated. For users this is users.User.ID; for agents this is agent.Agent.ID
	ID        uint           `json:"item_id" gorm:"column:item_id;index;not null"`
	IsAgent   bool           `json:"is_agent" gorm:"column:is_agent;index;not null;default:false"`
	SessionID uint           `json:"session_id" gorm:"primaryKey;autoIncrement"`
	Expiry    time.Time      `json:"expiry" gorm:"column:expiry;index;not null"`
	Created   time.Time      `json:"created" gorm:"column:created;index;not null"`
	WSConn    string         `json:"ws_conn" gorm:"column:ws_conn;size:255;index"`
	IP        string         `json:"ip,omitempty" gorm:"column:ip;size:64"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (Session) TableName() string { return "sessions" }

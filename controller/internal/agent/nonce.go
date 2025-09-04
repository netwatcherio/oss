package agent

import "time"

// AgentNonce provides single-use, short-lived nonces for bootstrap/auth.
type AgentNonce struct {
	ID        uint      `gorm:"primaryKey;autoIncrement"`
	AgentID   uint      `gorm:"index"`
	Nonce     string    `gorm:"size:64;uniqueIndex"`
	ExpiresAt time.Time `gorm:"index"`
	UsedAt    *time.Time
	CreatedAt time.Time `gorm:"index"`
}

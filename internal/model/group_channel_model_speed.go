package model

import (
	"time"
)

// GroupChannelModelSpeed stores the latest speed-test measurement for a group member.
// Keyed by (group_id, channel_id, model_name) - only latest result is kept.
type GroupChannelModelSpeed struct {
	ID             int       `json:"id" gorm:"primaryKey"`
	GroupID        int       `json:"group_id" gorm:"not null;index:idx_group_channel_model_speed,unique"`
	ChannelID      int       `json:"channel_id" gorm:"not null;index:idx_group_channel_model_speed,unique"`
	ModelName      string    `json:"model_name" gorm:"not null;size:255;index:idx_group_channel_model_speed,unique"`
	ResponseTimeMs int       `json:"response_time_ms"`                         // Measured tool-call response time in milliseconds
	Status         string    `json:"status" gorm:"size:32"`                    // "success" or "failed"
	LastError      string    `json:"last_error" gorm:"size:512"`               // Error message if failed
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime;not null"`
}

// SpeedTestStatus constants
const (
	SpeedTestStatusSuccess = "success"
	SpeedTestStatusFailed  = "failed"
)
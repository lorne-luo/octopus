package model

import "time"

// ChannelHealthStatus represents the health status of a channel check
type ChannelHealthStatus string

const (
	ChannelHealthStatusUnknown   ChannelHealthStatus = "unknown"   // Not checked yet
	ChannelHealthStatusHealthy   ChannelHealthStatus = "healthy"   // Healthy (green)
	ChannelHealthStatusUnhealthy ChannelHealthStatus = "unhealthy" // Unhealthy (red)
	ChannelHealthStatusChecking  ChannelHealthStatus = "checking"  // Currently checking
)

// ChannelHealth represents a health check configuration for a channel
type ChannelHealth struct {
	ID              int                 `json:"id" gorm:"primaryKey"`
	ChannelID       int                 `json:"channel_id" gorm:"not null;index:idx_channel_model,unique"`
	ModelName       string              `json:"model_name" gorm:"not null;index:idx_channel_model,unique"`
	IntervalMinutes int                 `json:"interval_minutes" gorm:"not null;default:60"`
	Prompt          string              `json:"prompt" gorm:"not null"`
	Status          ChannelHealthStatus `json:"status" gorm:"default:'unknown'"`
	LastCheck       *time.Time          `json:"last_check"`
	NextCheck       *time.Time          `json:"next_check"`
	LastError       *string             `json:"last_error"`
	CreatedAt       time.Time           `json:"created_at"`
	UpdatedAt       time.Time           `json:"updated_at"`
}

// ChannelHealthWithChannel includes the channel name for frontend display
type ChannelHealthWithChannel struct {
	ChannelHealth
	ChannelName string `json:"channel_name"`
}

// TableName specifies the table name for ChannelHealth
func (ChannelHealth) TableName() string {
	return "channel_healths"
}

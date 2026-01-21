package model

import "time"

// HealthCheckStatus represents the health status of a channel check
type HealthCheckStatus string

const (
	HealthCheckStatusUnknown   HealthCheckStatus = "unknown"   // Not checked yet
	HealthCheckStatusHealthy   HealthCheckStatus = "healthy"   // Healthy (green)
	HealthCheckStatusUnhealthy HealthCheckStatus = "unhealthy" // Unhealthy (red)
	HealthCheckStatusChecking  HealthCheckStatus = "checking"  // Currently checking
)

// HealthCheck represents a health check configuration for a channel
type HealthCheck struct {
	ID              int               `json:"id" gorm:"primaryKey"`
	ChannelID       int               `json:"channel_id" gorm:"not null;index:idx_channel_model,unique"`
	ModelName       string            `json:"model_name" gorm:"not null;index:idx_channel_model,unique"`
	IntervalMinutes int               `json:"interval_minutes" gorm:"not null;default:60"`
	Prompt          string            `json:"prompt" gorm:"not null"`
	Status          HealthCheckStatus `json:"status" gorm:"default:'unknown'"`
	LastCheck       *time.Time        `json:"last_check"`
	NextCheck       *time.Time        `json:"next_check"`
	LastError       *string           `json:"last_error"`
	LatencyMs       *int              `json:"latency_ms"` // Latency in milliseconds
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
}

// HealthCheckWithChannel includes the channel name for frontend display
type HealthCheckWithChannel struct {
	HealthCheck
	ChannelName string `json:"channel_name"`
}

// TableName specifies the table name for HealthCheck
func (HealthCheck) TableName() string {
	return "health_checks"
}

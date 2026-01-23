package op

import (
	"context"
	"fmt"
	"time"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
)

// HealthList returns all health checks with their channel names
func HealthList(ctx context.Context) ([]model.ChannelHealthWithChannel, error) {
	var healthChecks []model.ChannelHealthWithChannel
	err := db.GetDB().WithContext(ctx).
		Table("channel_healths").
		Select("channel_healths.*, channels.name as channel_name").
		Joins("LEFT JOIN channels ON channel_healths.channel_id = channels.id").
		Order("channel_healths.id DESC").
		Find(&healthChecks).Error
	return healthChecks, err
}

// HealthGet retrieves a single health check by ID
func HealthGet(ctx context.Context, id int) (*model.ChannelHealth, error) {
	var hc model.ChannelHealth
	err := db.GetDB().WithContext(ctx).First(&hc, id).Error
	if err != nil {
		return nil, err
	}
	return &hc, nil
}

// HealthCreate creates a new health check and registers it as a task
func HealthCreate(ctx context.Context, hc *model.ChannelHealth) error {
	// Check if the channel exists
	var channel model.Channel
	if err := db.GetDB().WithContext(ctx).First(&channel, hc.ChannelID).Error; err != nil {
		return fmt.Errorf("channel not found: %w", err)
	}

	// Check for duplicate channel_id + model_name
	var count int64
	err := db.GetDB().WithContext(ctx).Model(&model.ChannelHealth{}).
		Where("channel_id = ? AND model_name = ?", hc.ChannelID, hc.ModelName).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("health check already exists for this channel and model")
	}

	// Set initial status and next check time
	hc.Status = model.ChannelHealthStatusUnknown
	now := time.Now()
	nextCheck := now
	hc.NextCheck = &nextCheck

	// Create in database
	if err := db.GetDB().WithContext(ctx).Create(hc).Error; err != nil {
		return err
	}

	return nil
}

// HealthUpdate updates an existing health check
func HealthUpdate(ctx context.Context, hc *model.ChannelHealth) error {
	if hc.ID == 0 {
		return fmt.Errorf("health check ID is required")
	}

	// Check if exists
	var existing model.ChannelHealth
	if err := db.GetDB().WithContext(ctx).First(&existing, hc.ID).Error; err != nil {
		return fmt.Errorf("health check not found: %w", err)
	}

	// Check for duplicate if channel_id or model_name changed
	if hc.ChannelID != existing.ChannelID || hc.ModelName != existing.ModelName {
		var count int64
		err := db.GetDB().WithContext(ctx).Model(&model.ChannelHealth{}).
			Where("channel_id = ? AND model_name = ? AND id != ?", hc.ChannelID, hc.ModelName, hc.ID).
			Count(&count).Error
		if err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("health check already exists for this channel and model")
		}
	}

	// Calculate new next_check based on last_check (or now if never checked)
	var nextCheck time.Time
	if existing.LastCheck != nil {
		nextCheck = existing.LastCheck.Add(time.Duration(hc.IntervalMinutes) * time.Minute)
	} else {
		nextCheck = time.Now().Add(time.Duration(hc.IntervalMinutes) * time.Minute)
	}

	// Update only specific fields
	updates := map[string]interface{}{
		"model_name":       hc.ModelName,
		"interval_minutes": hc.IntervalMinutes,
		"prompt":           hc.Prompt,
		"next_check":       nextCheck,
	}

	err := db.GetDB().WithContext(ctx).Model(&model.ChannelHealth{}).
		Where("id = ?", hc.ID).
		Updates(updates).Error

	return err
}

// HealthDelete deletes a health check by ID
func HealthDelete(ctx context.Context, id int) error {
	result := db.GetDB().WithContext(ctx).Delete(&model.ChannelHealth{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("health check not found")
	}
	return nil
}

// HealthUpdateStatus updates the status, error message, and latency of a health check
// This is called by the task runner after each check
func HealthUpdateStatus(ctx context.Context, id int, status model.ChannelHealthStatus, lastError *string, nextCheck *time.Time, latencyMs *int) error {
	now := time.Now()
	updates := map[string]interface{}{
		"status":     status,
		"last_check": now,
		"last_error": lastError,
	}
	if nextCheck != nil {
		updates["next_check"] = nextCheck
	}
	if latencyMs != nil {
		updates["latency_ms"] = latencyMs
	}

	return db.GetDB().WithContext(ctx).Model(&model.ChannelHealth{}).
		Where("id = ?", id).
		Updates(updates).Error
}

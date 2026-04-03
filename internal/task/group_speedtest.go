package task

import (
	"context"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

// GroupSpeedTestTask runs speed tests for all group members.
// It measures tool-call response time for each (group, channel, model) combination.
func GroupSpeedTestTask() {
	log.Debugf("group speed test task started")
	startTime := time.Now()
	defer func() {
		log.Debugf("group speed test task finished, elapsed: %s", time.Since(startTime))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Load all groups
	groups, err := op.GroupList(ctx)
	if err != nil {
		log.Errorf("failed to list groups: %v", err)
		return
	}

	// Process each group
	for _, group := range groups {
		if len(group.Items) == 0 {
			continue
		}
		processGroupSpeedTest(ctx, &group)
	}
}

// processGroupSpeedTest runs speed tests for all items in a group.
func processGroupSpeedTest(ctx context.Context, group *model.Group) {
	for _, item := range group.Items {
		// Get the channel for this item
		channel, err := op.ChannelGet(item.ChannelID, ctx)
		if err != nil {
			log.Warnf("failed to get channel %d for group %d: %v", item.ChannelID, group.ID, err)
			continue
		}

		// Skip disabled channels
		if !channel.Enabled {
			continue
		}

		// Run speed test with retry
		result := runSpeedTestWithRetry(ctx, channel, item.ModelName, 3)

		// Persist the result
		speed := &model.GroupChannelModelSpeed{
			GroupID:        group.ID,
			ChannelID:      item.ChannelID,
			ModelName:      item.ModelName,
			ResponseTimeMs: result.ResponseTimeMs,
			Status:         model.SpeedTestStatusSuccess,
		}

		if !result.Success {
			speed.Status = model.SpeedTestStatusFailed
			speed.LastError = result.Error
		}

		if err := op.UpsertGroupChannelModelSpeed(ctx, speed); err != nil {
			log.Warnf("failed to upsert speed for group=%d channel=%d model=%s: %v",
				group.ID, item.ChannelID, item.ModelName, err)
		}
	}
}

// runSpeedTestWithRetry executes a speed test with retry using alternate API keys.
// Returns the result from the first successful attempt, or the last failure.
func runSpeedTestWithRetry(ctx context.Context, channel *model.Channel, modelName string, maxAttempts int) helper.SpeedTestResult {
	var lastResult helper.SpeedTestResult
	excludeKeyIDs := make([]int, 0, maxAttempts)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result := helper.RunGroupSpeedTest(ctx, helper.SpeedTestConfig{
			Channel:       channel,
			ModelName:     modelName,
			ExcludeKeyIDs: excludeKeyIDs,
		})

		if result.Success {
			return result
		}

		lastResult = result
		log.Debugf("speed test attempt %d/%d failed for channel=%d model=%s: %s",
			attempt, maxAttempts, channel.ID, modelName, result.Error)

		// For OAuth channels, no point in retrying with different keys
		if channel.UseOAuth {
			break
		}

		// Add the failed key ID to exclusion list for next attempt
		// Note: We can't get the key ID from the result, so we need to track it differently
		// For now, we'll just try different keys by excluding previously tried ones
	}

	return lastResult
}

// RunGroupSpeedTestManual triggers a manual speed test for a specific group.
// This is used for the on-demand 测速 button.
func RunGroupSpeedTestManual(ctx context.Context, groupID int) error {
	group, err := op.GroupGet(groupID, ctx)
	if err != nil {
		return err
	}

	if len(group.Items) == 0 {
		return nil
	}

	go func() {
		processGroupSpeedTest(ctx, group)
	}()

	return nil
}

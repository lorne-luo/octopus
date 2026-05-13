package task

import (
	"context"
	"sync"
	"time"

	"github.com/bestruirui/octopus/internal/helper"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/op"
	"github.com/bestruirui/octopus/internal/utils/log"
)

const groupSpeedTestWorkers = 8

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
	workerCount := groupSpeedTestWorkers
	if len(group.Items) < workerCount {
		workerCount = len(group.Items)
	}
	jobs := make(chan model.GroupItem)
	var wg sync.WaitGroup

	for range workerCount {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobs {
				processGroupSpeedTestItem(ctx, group.ID, item)
			}
		}()
	}

	for _, item := range group.Items {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return
		case jobs <- item:
		}
	}
	close(jobs)
	wg.Wait()
}

func processGroupSpeedTestItem(ctx context.Context, groupID int, item model.GroupItem) {
	channel, err := op.ChannelGet(item.ChannelID, ctx)
	if err != nil {
		log.Warnf("failed to get channel %d for group %d: %v", item.ChannelID, groupID, err)
		return
	}
	if !channel.Enabled {
		return
	}

	result := runSpeedTestWithRetry(ctx, channel, item.ModelName, 3)
	speed := &model.GroupChannelModelSpeed{
		GroupID:        groupID,
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
			groupID, item.ChannelID, item.ModelName, err)
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
		log.Debugf("speed test attempt %d/%d failed for channel=%d model=%s: %s", attempt, maxAttempts, channel.ID, modelName, result.Error)

		// For OAuth channels, no point in retrying with different keys
		if channel.UseOAuth {
			break
		}

		// Add the failed key ID to exclusion list for next attempt
		if result.KeyID > 0 {
			excludeKeyIDs = append(excludeKeyIDs, result.KeyID)
		}
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

	// Execute synchronously so the API response returns after latest results are persisted.
	bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	processGroupSpeedTest(bgCtx, group)

	return nil
}

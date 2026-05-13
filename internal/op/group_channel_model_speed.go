package op

import (
	"context"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"gorm.io/gorm/clause"
)

const groupSpeedQueryBatchSize = 500

// UpsertGroupChannelModelSpeed inserts or updates the latest speed-test result for a group member.
// Uses upsert on (group_id, channel_id, model_name) to ensure only the latest result is kept.
func UpsertGroupChannelModelSpeed(ctx context.Context, speed *model.GroupChannelModelSpeed) error {
	return db.GetDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "group_id"},
				{Name: "channel_id"},
				{Name: "model_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"response_time_ms",
				"status",
				"last_error",
				"updated_at",
			}),
		}).
		Create(speed).Error
}

// GetGroupChannelModelSpeed retrieves the latest speed-test result for a specific group member.
func GetGroupChannelModelSpeed(ctx context.Context, groupID, channelID int, modelName string) (*model.GroupChannelModelSpeed, error) {
	var speed model.GroupChannelModelSpeed
	err := db.GetDB().WithContext(ctx).
		Where("group_id = ? AND channel_id = ? AND model_name = ?", groupID, channelID, modelName).
		First(&speed).Error
	if err != nil {
		return nil, err
	}
	return &speed, nil
}

// GetGroupChannelModelSpeedsByGroup retrieves all speed-test results for a group.
func GetGroupChannelModelSpeedsByGroup(ctx context.Context, groupID int) ([]model.GroupChannelModelSpeed, error) {
	var speeds []model.GroupChannelModelSpeed
	err := db.GetDB().WithContext(ctx).
		Where("group_id = ?", groupID).
		Find(&speeds).Error
	return speeds, err
}

// GetAllGroupChannelModelSpeeds retrieves all speed-test results.
func GetAllGroupChannelModelSpeeds(ctx context.Context) ([]model.GroupChannelModelSpeed, error) {
	var speeds []model.GroupChannelModelSpeed
	err := db.GetDB().WithContext(ctx).Find(&speeds).Error
	return speeds, err
}

// DeleteGroupChannelModelSpeedsByGroup deletes all speed-test results for a group.
func DeleteGroupChannelModelSpeedsByGroup(ctx context.Context, groupID int) error {
	return db.GetDB().WithContext(ctx).
		Where("group_id = ?", groupID).
		Delete(&model.GroupChannelModelSpeed{}).Error
}

// DeleteGroupChannelModelSpeedsByChannel deletes all speed-test results for a channel across all groups.
func DeleteGroupChannelModelSpeedsByChannel(ctx context.Context, channelID int) error {
	return db.GetDB().WithContext(ctx).
		Where("channel_id = ?", channelID).
		Delete(&model.GroupChannelModelSpeed{}).Error
}

type GroupChannelModelSpeedKey struct {
	GroupID   int
	ChannelID int
	ModelName string
}

// GetGroupChannelModelSpeedMap returns a map keyed by (channel_id, model_name) for quick lookup.
func GetGroupChannelModelSpeedMap(ctx context.Context, groupID int) (map[string]*model.GroupChannelModelSpeed, error) {
	speeds, err := GetGroupChannelModelSpeedsByGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*model.GroupChannelModelSpeed, len(speeds))
	for i := range speeds {
		key := speeds[i].ModelName // Use model_name as key since channel is implied by group context
		result[key] = &speeds[i]
	}
	return result, nil
}

func GetGroupChannelModelSpeedMapByGroups(ctx context.Context, groupIDs []int) (map[GroupChannelModelSpeedKey]*model.GroupChannelModelSpeed, error) {
	result := make(map[GroupChannelModelSpeedKey]*model.GroupChannelModelSpeed)
	for start := 0; start < len(groupIDs); start += groupSpeedQueryBatchSize {
		end := start + groupSpeedQueryBatchSize
		if end > len(groupIDs) {
			end = len(groupIDs)
		}
		var speeds []model.GroupChannelModelSpeed
		if err := db.GetDB().WithContext(ctx).Where("group_id IN ?", groupIDs[start:end]).Find(&speeds).Error; err != nil {
			return nil, err
		}
		for i := range speeds {
			key := GroupChannelModelSpeedKey{
				GroupID:   speeds[i].GroupID,
				ChannelID: speeds[i].ChannelID,
				ModelName: speeds[i].ModelName,
			}
			result[key] = &speeds[i]
		}
	}
	return result, nil
}

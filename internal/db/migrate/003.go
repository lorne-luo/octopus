package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 3,
		Up:      migrateUseOAuthField,
	})
}

// 003: Set UseOAuth=true for channels that have OAuthProviderID > 0
func migrateUseOAuthField(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Infof("Starting migration 003: Set UseOAuth for existing OAuth channels")

	// Count channels that need migration
	var count int64
	if err := db.Table("channels").
		Where("use_oauth = ? AND oauth_provider_id > ?", false, 0).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count channels to migrate: %w", err)
	}

	if count == 0 {
		log.Infof("Migration 003: No channels to migrate")
		return nil
	}

	log.Infof("Migration 003: Found %d channels to migrate", count)

	// Update channels
	result := db.Table("channels").
		Where("use_oauth = ? AND oauth_provider_id > ?", false, 0).
		Update("use_oauth", true)

	if result.Error != nil {
		return fmt.Errorf("failed to update channels: %w", result.Error)
	}

	log.Infof("Migration 003 completed: Updated %d channels", result.RowsAffected)

	// Verify data consistency
	var inconsistentCount int64
	db.Table("channels").
		Where("(use_oauth = ? AND oauth_provider_id > ?) OR (use_oauth = ? AND oauth_provider_id = ?)",
			false, 0, true, 0).
		Count(&inconsistentCount)

	if inconsistentCount > 0 {
		log.Warnf("Migration 003: Found %d channels with inconsistent OAuth settings after migration",
			inconsistentCount)
	}

	return nil
}

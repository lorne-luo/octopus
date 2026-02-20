package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/transformer/outbound"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterAfterAutoMigration(Migration{
		Version: 3,
		Up:      migrateOAuthProviders,
	})
}

// oauthProviderData holds raw data from oauth_providers table for migration
type oauthProviderData struct {
	ID           int
	Name         string
	ProviderType string
	Status       int
	BaseURL      string
	Model        string // Read from old column for migration
	CustomModel  string // Read from old column for migration
}

// getBaseURL returns the base URL for the provider
func (p *oauthProviderData) getBaseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	switch p.ProviderType {
	case "iflow":
		return "https://apis.iflow.cn/v1"
	default:
		return ""
	}
}

// migrateOAuthProviders handles all OAuth-related migrations:
// 1. Create Channel records for existing OAuth Providers
// 2. Set UseOAuth=true for channels with OAuthProviderID > 0
// 3. Migrate group_items with negative channel_id to positive IDs
// 4. Drop model and custom_model columns from oauth_providers
func migrateOAuthProviders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Infof("Starting migration 003: OAuth providers migration")

	// Step 1: Create channels for OAuth providers
	if err := createChannelsForOAuthProviders(db); err != nil {
		return err
	}

	// Step 2: Set UseOAuth for existing channels
	if err := setUseOAuthField(db); err != nil {
		return err
	}

	// Step 3: Migrate negative channel IDs in group_items
	if err := migrateNegativeChannelIDs(db); err != nil {
		return err
	}

	// Step 4: Drop old columns from oauth_providers
	if err := dropOAuthProviderModelColumns(db); err != nil {
		return err
	}

	log.Infof("Migration 003 completed successfully")
	return nil
}

// createChannelsForOAuthProviders creates Channel records for OAuth providers that don't have one
func createChannelsForOAuthProviders(db *gorm.DB) error {
	// Check if oauth_providers table exists
	if !db.Migrator().HasTable("oauth_providers") {
		log.Infof("Migration 003: oauth_providers table does not exist, skipping channel creation")
		return nil
	}

	// Get all OAuth providers using raw SQL to read model/custom_model columns
	var providers []oauthProviderData
	if err := db.Table("oauth_providers").Find(&providers).Error; err != nil {
		return fmt.Errorf("failed to fetch OAuth providers: %w", err)
	}

	if len(providers) == 0 {
		log.Infof("Migration 003: No OAuth providers found")
		return nil
	}

	log.Infof("Migration 003: Found %d OAuth providers", len(providers))

	created := 0
	skipped := 0

	for _, provider := range providers {
		// Check if a channel already exists for this OAuth provider
		var existingChannel model.Channel
		err := db.Where("use_oauth = ? AND oauth_provider_id = ?", true, provider.ID).
			First(&existingChannel).Error

		if err == nil {
			skipped++
			continue
		}

		if err != gorm.ErrRecordNotFound {
			log.Warnf("Migration 003: Error checking for existing channel for provider %d: %v",
				provider.ID, err)
			continue
		}

		// Create a new channel for this OAuth provider
		channel := model.Channel{
			Name:            fmt.Sprintf("OAuth-%s", provider.Name),
			Type:            outbound.OutboundTypeOpenAIChat,
			Enabled:         provider.Status == 1,
			BaseUrls:        []model.BaseUrl{{URL: provider.getBaseURL(), Delay: 0}},
			Keys:            []model.ChannelKey{},
			Model:           provider.Model,
			CustomModel:     provider.CustomModel,
			Proxy:           false,
			AutoSync:        false,
			AutoGroup:       model.AutoGroupTypeNone,
			CustomHeader:    []model.CustomHeader{},
			UseOAuth:        true,
			OAuthProviderID: provider.ID,
		}

		if err := db.Create(&channel).Error; err != nil {
			log.Warnf("Migration 003: Failed to create channel for OAuth provider %d (%s): %v",
				provider.ID, provider.Name, err)
			continue
		}

		created++
		log.Infof("Migration 003: Created channel %d for OAuth provider %d (%s)",
			channel.ID, provider.ID, provider.Name)
	}

	log.Infof("Migration 003: Created %d channels, skipped %d existing channels", created, skipped)
	return nil
}

// setUseOAuthField sets UseOAuth=true for channels that have OAuthProviderID > 0
func setUseOAuthField(db *gorm.DB) error {
	// Count channels that need migration
	var count int64
	if err := db.Table("channels").
		Where("use_o_auth = ? AND oauth_provider_id > ?", false, 0).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count channels to migrate: %w", err)
	}

	if count == 0 {
		return nil
	}

	log.Infof("Migration 003: Found %d channels to set UseOAuth", count)

	// Update channels
	result := db.Table("channels").
		Where("use_o_auth = ? AND o_auth_provider_id > ?", false, 0).
		Update("use_o_auth", true)

	if result.Error != nil {
		return fmt.Errorf("failed to update channels: %w", result.Error)
	}

	log.Infof("Migration 003: Updated %d channels with UseOAuth=true", result.RowsAffected)
	return nil
}

// migrateNegativeChannelIDs migrates group_items with negative channel_id to positive IDs
func migrateNegativeChannelIDs(db *gorm.DB) error {
	// Check if group_items table exists
	if !db.Migrator().HasTable("group_items") {
		return nil
	}

	// Find all group_items with negative channel_id
	type GroupItem struct {
		ID        int `gorm:"primaryKey"`
		ChannelID int
	}
	var negativeItems []GroupItem
	if err := db.Table("group_items").Where("channel_id < 0").Find(&negativeItems).Error; err != nil {
		log.Warnf("Migration 003: Failed to query negative channel_id items: %v", err)
		return nil // Not a critical error
	}

	if len(negativeItems) == 0 {
		return nil
	}

	log.Infof("Migration 003: Found %d group_items with negative channel_id", len(negativeItems))

	migrated := 0
	deleted := 0

	for _, item := range negativeItems {
		// The old pattern: channel_id = -oauth_provider_id
		oauthProviderID := -item.ChannelID

		// Find the corresponding channel
		var channel struct {
			ID int `gorm:"primaryKey"`
		}
		err := db.Table("channels").
			Where("use_o_auth = ? AND oauth_provider_id = ?", true, oauthProviderID).
			First(&channel).Error

		if err != nil {
			// No corresponding channel found, delete the group_item
			log.Warnf("Migration 003: No channel found for oauth_provider_id %d, deleting group_item %d",
				oauthProviderID, item.ID)
			if err := db.Table("group_items").Where("id = ?", item.ID).Delete(nil).Error; err != nil {
				log.Warnf("Migration 003: Failed to delete group_item %d: %v", item.ID, err)
			} else {
				deleted++
			}
			continue
		}

		// Update the group_item to use the positive channel ID
		if err := db.Table("group_items").Where("id = ?", item.ID).Update("channel_id", channel.ID).Error; err != nil {
			log.Warnf("Migration 003: Failed to update group_item %d: %v", item.ID, err)
			continue
		}
		migrated++
	}

	log.Infof("Migration 003: Migrated %d group_items, deleted %d items", migrated, deleted)
	return nil
}

// dropOAuthProviderModelColumns drops model and custom_model columns from oauth_providers
func dropOAuthProviderModelColumns(db *gorm.DB) error {
	// Check if the table exists
	if !db.Migrator().HasTable("oauth_providers") {
		return nil
	}

	// Drop model column if exists
	if db.Migrator().HasColumn("oauth_providers", "model") {
		if err := db.Migrator().DropColumn("oauth_providers", "model"); err != nil {
			log.Warnf("Migration 003: Failed to drop model column: %v", err)
			return err
		}
		log.Infof("Migration 003: Dropped model column from oauth_providers")
	}

	// Drop custom_model column if exists
	if db.Migrator().HasColumn("oauth_providers", "custom_model") {
		if err := db.Migrator().DropColumn("oauth_providers", "custom_model"); err != nil {
			log.Warnf("Migration 003: Failed to drop custom_model column: %v", err)
			return err
		}
		log.Infof("Migration 003: Dropped custom_model column from oauth_providers")
	}

	return nil
}

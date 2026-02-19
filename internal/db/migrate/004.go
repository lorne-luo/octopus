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
		Version: 4,
		Up:      createChannelsForOAuthProviders,
	})
}

// 004: Create Channel records for existing OAuth Providers that don't have one
func createChannelsForOAuthProviders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Infof("Starting migration 004: Create channels for OAuth providers")

	// Get all OAuth providers
	var providers []model.OAuthProvider
	if err := db.Find(&providers).Error; err != nil {
		return fmt.Errorf("failed to fetch OAuth providers: %w", err)
	}

	if len(providers) == 0 {
		log.Infof("Migration 004: No OAuth providers found")
		return nil
	}

	log.Infof("Migration 004: Found %d OAuth providers", len(providers))

	created := 0
	skipped := 0

	for _, provider := range providers {
		// Check if a channel already exists for this OAuth provider
		var existingChannel model.Channel
		err := db.Where("use_oauth = ? AND oauth_provider_id = ?", true, provider.ID).
			First(&existingChannel).Error

		if err == nil {
			// Channel already exists
			skipped++
			log.Debugf("Migration 004: Channel already exists for OAuth provider %d (%s)",
				provider.ID, provider.Name)
			continue
		}

		if err != gorm.ErrRecordNotFound {
			// Unexpected error
			log.Warnf("Migration 004: Error checking for existing channel for provider %d: %v",
				provider.ID, err)
			continue
		}

		// Create a new channel for this OAuth provider
		channel := model.Channel{
			Name:            fmt.Sprintf("OAuth-%s", provider.Name),
			Type:            outbound.OutboundTypeOpenAIChat,
			Enabled:         provider.Status == 1,
			BaseUrls:        []model.BaseUrl{{URL: provider.GetBaseURL(), Delay: 0}},
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
			log.Warnf("Migration 004: Failed to create channel for OAuth provider %d (%s): %v",
				provider.ID, provider.Name, err)
			continue
		}

		created++
		log.Infof("Migration 004: Created channel %d for OAuth provider %d (%s)",
			channel.ID, provider.ID, provider.Name)
	}

	log.Infof("Migration 004 completed: Created %d channels, skipped %d existing channels",
		created, skipped)

	return nil
}

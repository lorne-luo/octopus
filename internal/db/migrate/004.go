package migrate

import (
	"encoding/json"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 4,
		Up:      migrateOAuthProviderAuthJSON,
	})
}

// oauthProviderMigrateData holds raw data from oauth_providers table for migration
type oauthProviderMigrateData struct {
	ID           int
	ProviderType string
	Cookie       string
}

// migrateOAuthProviderAuthJSON handles migration from cookie to auth_json
// and provider_type from string to int
func migrateOAuthProviderAuthJSON(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	log.Infof("Starting migration 004: OAuth provider auth_json migration")

	// Check if oauth_providers table exists
	if !db.Migrator().HasTable("oauth_providers") {
		log.Infof("Migration 004: oauth_providers table does not exist, skipping")
		return nil
	}

	// Check if auth_json column already exists
	if db.Migrator().HasColumn("oauth_providers", "auth_json") {
		log.Infof("Migration 004: auth_json column already exists, skipping")
		return nil
	}

	// Step 1: Add auth_json column
	if err := db.Exec("ALTER TABLE oauth_providers ADD COLUMN auth_json TEXT").Error; err != nil {
		log.Warnf("Migration 004: Failed to add auth_json column: %v", err)
		return err
	}
	log.Infof("Migration 004: Added auth_json column")

	// Step 2: Migrate cookie to auth_json
	var providers []oauthProviderMigrateData
	if err := db.Table("oauth_providers").Where("cookie IS NOT NULL AND cookie != ''").Find(&providers).Error; err != nil {
		log.Warnf("Migration 004: Failed to query providers with cookie: %v", err)
		// Not critical, continue
	} else {
		for _, p := range providers {
			// Create auth_json from cookie
			authJSON, _ := json.Marshal(map[string]string{"BXAuth": p.Cookie})
			if err := db.Table("oauth_providers").Where("id = ?", p.ID).Update("auth_json", string(authJSON)).Error; err != nil {
				log.Warnf("Migration 004: Failed to migrate cookie for provider %d: %v", p.ID, err)
			}
		}
		log.Infof("Migration 004: Migrated %d cookies to auth_json", len(providers))
	}

	// Step 3: Migrate provider_type from string to int
	var allProviders []oauthProviderMigrateData
	if err := db.Table("oauth_providers").Find(&allProviders).Error; err != nil {
		log.Warnf("Migration 004: Failed to query all providers: %v", err)
	} else {
		for _, p := range allProviders {
			var newType model.OAuthProviderType
			switch p.ProviderType {
			case "iflow":
				newType = model.OAuthProviderTypeIFlow
			case "kiro":
				newType = model.OAuthProviderTypeKiro
			default:
				newType = model.OAuthProviderTypeIFlow // Default to iflow
			}
			if err := db.Table("oauth_providers").Where("id = ?", p.ID).Update("provider_type", int(newType)).Error; err != nil {
				log.Warnf("Migration 004: Failed to update provider_type for provider %d: %v", p.ID, err)
			}
		}
		log.Infof("Migration 004: Migrated %d provider_types to int", len(allProviders))
	}

	// Step 4: Drop cookie column
	if db.Migrator().HasColumn("oauth_providers", "cookie") {
		if err := db.Migrator().DropColumn("oauth_providers", "cookie"); err != nil {
			log.Warnf("Migration 004: Failed to drop cookie column: %v", err)
			// Not critical for functionality
		} else {
			log.Infof("Migration 004: Dropped cookie column")
		}
	}

	log.Infof("Migration 004 completed successfully")
	return nil
}

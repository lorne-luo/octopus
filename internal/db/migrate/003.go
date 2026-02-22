package migrate

import (
	"fmt"

	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/log"
	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 3,
		Up:      migrateAuthJsonToSeparateTable,
	})
}

// oauthProviderAuthData holds data needed for migration
type oauthProviderAuthData struct {
	ID       int
	AuthJSON string
}

// migrateAuthJsonToSeparateTable handles migration from single auth_json to auth_jsons table
// This migration:
// 1. Creates auth_jsons table if it doesn't exist
// 2. Migrates existing auth_json data from oauth_providers to auth_jsons table
// 3. Drops auth_json column from oauth_providers
func migrateAuthJsonToSeparateTable(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	log.Infof("Starting migration 003: AuthJson separate table migration")

	dialect := db.Dialector.Name()

	// Check if oauth_providers table exists
	if !db.Migrator().HasTable("o_auth_providers") && !db.Migrator().HasTable("oauth_providers") {
		log.Infof("Migration 003: oauth_providers table does not exist, skipping")
		return nil
	}

	// Determine the actual table name (GORM converts OAuthProvider to o_auth_providers)
	tableName := "o_auth_providers"
	if !db.Migrator().HasTable(tableName) {
		tableName = "oauth_providers"
	}

	// Check if auth_jsons table already exists
	if db.Migrator().HasTable("auth_jsons") {
		log.Infof("Migration 003: auth_jsons table already exists, skipping")
		return nil
	}

	// Step 1: Create auth_jsons table
	if err := db.AutoMigrate(&model.AuthJson{}); err != nil {
		return fmt.Errorf("failed to create auth_jsons table: %w", err)
	}
	log.Infof("Migration 003: Created auth_jsons table")

	// Step 2: Check if auth_json column exists
	hasAuthJSONCol := hasColumnHelper(db, dialect, tableName, "auth_json")
	if !hasAuthJSONCol {
		log.Infof("Migration 003: auth_json column does not exist, skipping data migration")
		return nil
	}

	// Step 3: Migrate existing auth_json data from oauth_providers
	var providers []oauthProviderAuthData
	if err := db.Table(tableName).Where("auth_json IS NOT NULL AND auth_json != ''").Find(&providers).Error; err != nil {
		log.Warnf("Migration 003: Failed to query providers with auth_json: %v", err)
		// Not critical, continue
	} else {
		migrated := 0
		for _, p := range providers {
			authJson := &model.AuthJson{
				OAuthProviderID: p.ID,
				Content:         p.AuthJSON,
				Enabled:         true,
				StatusCode:      0,
			}
			if err := db.Create(authJson).Error; err != nil {
				log.Warnf("Migration 003: Failed to migrate auth_json for provider %d: %v", p.ID, err)
			} else {
				migrated++
			}
		}
		log.Infof("Migration 003: Migrated %d auth_json records to auth_jsons table", migrated)
	}

	// Step 4: Drop auth_json column from oauth_providers
	if err := dropColumnHelper(db, dialect, tableName, "auth_json"); err != nil {
		log.Warnf("Migration 003: Failed to drop auth_json column: %v", err)
		// Not critical for functionality
	} else {
		log.Infof("Migration 003: Dropped auth_json column from %s", tableName)
	}

	log.Infof("Migration 003 completed successfully")
	return nil
}

// hasColumnHelper checks if a column exists in a table across different databases
func hasColumnHelper(db *gorm.DB, dialect, table, column string) bool {
	switch dialect {
	case "sqlite":
		var name string
		db.Raw("SELECT name FROM pragma_table_info(?) WHERE name = ? LIMIT 1", table, column).Scan(&name)
		return name == column
	case "mysql":
		var count int64
		db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?", table, column).Scan(&count)
		return count > 0
	case "postgres":
		var count int64
		db.Raw("SELECT COUNT(*) FROM information_schema.columns WHERE table_name = ? AND column_name = ?", table, column).Scan(&count)
		return count > 0
	default:
		return db.Migrator().HasColumn(table, column)
	}
}

// dropColumnHelper drops a column from a table across different databases
func dropColumnHelper(db *gorm.DB, dialect, table, column string) error {
	var sql string
	switch dialect {
	case "sqlite":
		// SQLite 3.35.0+
		sql = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)
	case "mysql":
		sql = fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`", table, column)
	case "postgres":
		sql = fmt.Sprintf("ALTER TABLE %s DROP COLUMN IF EXISTS %s", table, column)
	default:
		sql = fmt.Sprintf("ALTER TABLE %s DROP COLUMN %s", table, column)
	}
	return db.Exec(sql).Error
}

package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 3,
		Up:      renameHealthChecksToChannelHealths,
	})
}

// renameHealthChecksToChannelHealths renames the health_checks table to channel_healths
func renameHealthChecksToChannelHealths(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	// Check if old table exists
	if !db.Migrator().HasTable("health_checks") {
		// Table already renamed or doesn't exist
		return nil
	}

	fmt.Println("Migrating: renaming table health_checks to channel_healths...")

	switch dialect {
	case "sqlite", "mysql", "postgres":
		// All three databases support ALTER TABLE ... RENAME TO
		// SQLite 3.25.0+ supports this syntax
		if err := db.Exec("ALTER TABLE health_checks RENAME TO channel_healths").Error; err != nil {
			return fmt.Errorf("failed to rename health_checks table: %w", err)
		}

	default:
		return fmt.Errorf("unsupported database dialect: %s", dialect)
	}

	fmt.Println("Successfully renamed health_checks table to channel_healths")
	return nil
}
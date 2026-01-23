package migrate

import (
	"fmt"

	"gorm.io/gorm"
)

func init() {
	RegisterBeforeAutoMigration(Migration{
		Version: 4,
		Up:      addLatencyMsToChannelHealths,
	})
}

// addLatencyMsToChannelHealths adds the latency_ms column to channel_healths table
func addLatencyMsToChannelHealths(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}

	dialect := db.Dialector.Name()

	// Check if column already exists
	hasColumn := func(table, column string) bool {
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

	if hasColumn("channel_healths", "latency_ms") {
		// Column already exists, skip migration
		return nil
	}

	fmt.Println("Migrating: adding latency_ms column to channel_healths table...")

	var sql string
	switch dialect {
	case "sqlite":
		sql = "ALTER TABLE channel_healths ADD COLUMN latency_ms INTEGER"
	case "mysql":
		sql = "ALTER TABLE `channel_healths` ADD COLUMN `latency_ms` INT"
	case "postgres":
		sql = "ALTER TABLE channel_healths ADD COLUMN latency_ms INTEGER"
	default:
		sql = "ALTER TABLE channel_healths ADD COLUMN latency_ms INTEGER"
	}

	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to add latency_ms column: %w", err)
	}

	fmt.Println("Successfully added latency_ms column to channel_healths table")
	return nil
}

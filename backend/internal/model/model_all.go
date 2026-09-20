package model

import (
	"fmt"

	"gorm.io/gorm"
)

func AllModels() []any {
	return []any{&Category{}, &Product{}, &Supplier{}, &Offer{}, &PriceHistory{}, &Favorite{}, &PriceAlert{}, &Budget{}, &LockOrder{}, &LockOrderItem{}}
}
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return err
	}
	return createLockPartialIndexes(db)
}

// createLockPartialIndexes enforces "at most one active lock per user per
// product" at the database level. The partial predicate is supported by both
// PostgreSQL and SQLite; a racing duplicate submit fails here even when the
// application-level conflict probe is bypassed.
func createLockPartialIndexes(db *gorm.DB) error {
	indexName := "idx_lock_items_user_product_active"
	if db.Migrator().HasIndex(&LockOrderItem{}, indexName) {
		return nil
	}
	if err := db.Exec(`CREATE UNIQUE INDEX ` + indexName + `
		ON lock_order_items (user_id, product_id)
		WHERE active = true`).Error; err != nil {
		return fmt.Errorf("create lock item partial index: %w", err)
	}
	return nil
}

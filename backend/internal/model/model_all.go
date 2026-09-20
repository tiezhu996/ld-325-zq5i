package model

import (
	"fmt"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"gorm.io/gorm"
)

func AllModels() []any {
	return []any{&Category{}, &Product{}, &Supplier{}, &Offer{}, &PriceHistory{}, &Favorite{}, &PriceAlert{}, &Budget{}, &PriceLock{}, &PriceLockItem{}}
}

// Migrate 建表后补建部分唯一索引：同一款材料同一时刻只允许一张有效（active）
// 锁价单的明细行。PostgreSQL 使用 partial unique index；SQLite 语法兼容。
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(AllModels()...); err != nil {
		return fmt.Errorf("auto migrate: %w", err)
	}
	// 状态值来自内部常量，非用户输入，可安全内联到 DDL。
	indexSQL := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS idx_price_lock_items_active_product
		ON price_lock_items (product_id) WHERE status = '%s'`, constants.LockStatusActive)
	if err := db.Exec(indexSQL).Error; err != nil {
		return fmt.Errorf("create active lock unique index: %w", err)
	}
	return nil
}

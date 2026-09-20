package repository

import (
	"errors"
	"fmt"
	"strings"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	// ErrLockConflict 表示同一款材料已存在有效锁价单（重复提交）。
	ErrLockConflict = errors.New("active price lock already exists for product")
	// ErrLockItemInvalid 表示事务复核时报价已不再满足锁价条件。
	ErrLockItemInvalid = errors.New("locked offer no longer satisfies the lock conditions")
)

type PriceLockRepository interface {
	ActiveProductIDs(productIDs []uint) ([]uint, error)
	CreateInTx(lock model.PriceLock) (model.PriceLock, error)
	ListByUser(userID string) ([]model.PriceLock, error)
}

type priceLockRepository struct{ db *gorm.DB }

func NewPriceLockRepository(db *gorm.DB) PriceLockRepository { return &priceLockRepository{db} }

// ActiveProductIDs 返回给定材料中当前仍持有有效锁价单的材料 ID。
func (r *priceLockRepository) ActiveProductIDs(productIDs []uint) ([]uint, error) {
	ids := []uint{}
	if len(productIDs) == 0 {
		return ids, nil
	}
	if err := r.db.Model(&model.PriceLockItem{}).
		Where("product_id IN ? AND status = ?", productIDs, constants.LockStatusActive).
		Distinct().Pluck("product_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("query active lock products: %w", err)
	}
	return ids, nil
}

// CreateInTx 在单个事务中完成"行锁复核报价 → 冲突复查 → 写入主单与明细"。
// 任一环节失败均回滚，不留下任何记录。
func (r *priceLockRepository) CreateInTx(lock model.PriceLock) (model.PriceLock, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		productIDs := make([]uint, 0, len(lock.Items))
		for _, item := range lock.Items {
			productIDs = append(productIDs, item.ProductID)
			if err := revalidateOffer(tx, item); err != nil {
				return err
			}
		}
		var conflictCount int64
		if err := tx.Model(&model.PriceLockItem{}).
			Where("product_id IN ? AND status = ?", productIDs, constants.LockStatusActive).
			Count(&conflictCount).Error; err != nil {
			return fmt.Errorf("count active lock items: %w", err)
		}
		if conflictCount > 0 {
			return ErrLockConflict
		}
		if err := tx.Create(&lock).Error; err != nil {
			if isActiveLockIndexViolation(err) {
				return ErrLockConflict
			}
			return fmt.Errorf("insert price lock: %w", err)
		}
		return nil
	})
	if err != nil {
		return model.PriceLock{}, err
	}
	return lock, nil
}

// revalidateOffer 对选中的报价加行锁后重新核对：存在、属于该材料、仍在售、
// 提交数量达到起订量，且快照价与当前价一致（提交价不可被并发改动偷换）。
func revalidateOffer(tx *gorm.DB, item model.PriceLockItem) error {
	query := tx.Model(&model.Offer{})
	if tx.Dialector.Name() == constants.DialectorPostgres {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var offer model.Offer
	if err := query.Where("id = ?", item.OfferID).First(&offer).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrLockItemInvalid
		}
		return fmt.Errorf("lock offer for update: %w", err)
	}
	valid := offer.ProductID == item.ProductID &&
		offer.StockStatus == constants.StatusInStock &&
		item.QuantitySnapshot >= offer.MOQ &&
		offer.UnitPrice == item.UnitPriceSnapshot
	if !valid {
		return ErrLockItemInvalid
	}
	return nil
}

func (r *priceLockRepository) ListByUser(userID string) ([]model.PriceLock, error) {
	locks := []model.PriceLock{}
	err := r.db.Where("user_id = ?", userID).
		Preload("Items").
		Order("created_at DESC").
		Find(&locks).Error
	if err != nil {
		return nil, fmt.Errorf("list price locks: %w", err)
	}
	return locks, nil
}

// InvalidateLocksByOffer 在调用方提供的事务中，把所有引用该报价且仍有效的
// 锁价单整单失效（主单与明细一起标记），是报价状态变更的级联钩子。
func InvalidateLocksByOffer(tx *gorm.DB, offerID uint, reason string) error {
	var lockIDs []uint
	if err := tx.Model(&model.PriceLockItem{}).
		Where("offer_id = ? AND status = ?", offerID, constants.LockStatusActive).
		Distinct().Pluck("price_lock_id", &lockIDs).Error; err != nil {
		return fmt.Errorf("find locks referencing offer: %w", err)
	}
	if len(lockIDs) == 0 {
		return nil
	}
	if err := tx.Model(&model.PriceLockItem{}).
		Where("price_lock_id IN ? AND status = ?", lockIDs, constants.LockStatusActive).
		Updates(map[string]any{"status": constants.LockStatusInvalid}).Error; err != nil {
		return fmt.Errorf("invalidate lock items: %w", err)
	}
	if err := tx.Model(&model.PriceLock{}).
		Where("id IN ? AND status = ?", lockIDs, constants.LockStatusActive).
		Updates(map[string]any{"status": constants.LockStatusInvalid, "invalid_reason": reason}).Error; err != nil {
		return fmt.Errorf("invalidate lock headers: %w", err)
	}
	return nil
}

// isActiveLockIndexViolation 识别部分唯一索引冲突，兼容 PostgreSQL 与 SQLite 报错文案。
func isActiveLockIndexViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "idx_price_lock_items_active_product") ||
		strings.Contains(message, "23505") ||
		strings.Contains(message, "unique constraint failed")
}

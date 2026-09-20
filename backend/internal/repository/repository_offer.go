package repository

import (
	"errors"
	"fmt"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"gorm.io/gorm"
)

type OfferRepository interface {
	ListByProduct(uint) ([]model.Offer, error)
	Get(uint) (model.Offer, error)
	UpdateStatus(uint, string) (model.Offer, error)
}
type offerRepository struct{ db *gorm.DB }

func NewOfferRepository(db *gorm.DB) OfferRepository { return &offerRepository{db} }
func (r *offerRepository) ListByProduct(id uint) ([]model.Offer, error) {
	var data []model.Offer
	if err := r.db.Preload("Supplier").Where("product_id = ?", id).Order("unit_price ASC").Find(&data).Error; err != nil {
		return nil, fmt.Errorf("list offers: %w", err)
	}
	return data, nil
}

func (r *offerRepository) Get(id uint) (model.Offer, error) {
	var item model.Offer
	if err := r.db.Preload("Supplier").First(&item, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, gorm.ErrRecordNotFound
		}
		return item, fmt.Errorf("find offer: %w", err)
	}
	return item, nil
}

// UpdateStatus 更新报价库存状态；当报价被置为缺货/非在售时，在同一事务内
// 立即级联失效所有引用该报价的有效锁价单，保证刷新后无法回读成已锁定。
func (r *offerRepository) UpdateStatus(id uint, status string) (model.Offer, error) {
	var item model.Offer
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.First(&item, id).Error; err != nil {
			return fmt.Errorf("find offer: %w", err)
		}
		item.StockStatus = status
		if err := tx.Save(&item).Error; err != nil {
			return fmt.Errorf("update offer: %w", err)
		}
		if status == constants.StatusOutOfStock || status == constants.StatusDiscontinued {
			reason := constants.LockInvalidReasonOutOfStock
			if status == constants.StatusDiscontinued {
				reason = constants.LockInvalidReasonDiscontinued
			}
			if err := InvalidateLocksByOffer(tx, id, reason); err != nil {
				return fmt.Errorf("invalidate locks after offer status update: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return model.Offer{}, err
	}
	return item, nil
}

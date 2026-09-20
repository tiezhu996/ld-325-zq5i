package repository

import (
	"fmt"
	"strings"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// UnitOfWork groups every database operation that must participate in the same
// lock-order transaction. Services orchestrate business rules through this
// interface without depending on *gorm.DB directly.
type UnitOfWork interface {
	// FindOfferForUpdate row-locks the offer (SELECT ... FOR UPDATE on
	// PostgreSQL; a no-op lock on SQLite) and loads its product and supplier
	// for snapshotting.
	FindOfferForUpdate(productID, offerID uint) (model.Offer, model.Product, model.Supplier, error)
	HasActiveLock(userID string, productID uint) (bool, error)
	InsertLock(order *model.LockOrder, items []model.LockOrderItem) error
	ListLocksForUser(userID string) ([]model.LockOrder, error)
	GetLockByID(orderID uint) (model.LockOrder, error)
	LockOffer(offerID uint) (model.Offer, error)
	SaveOfferStatus(offer *model.Offer, status string) error
	// InvalidateLocksForOffer immediately invalidates every active lock order
	// containing the offer; returns how many orders were affected.
	InvalidateLocksForOffer(offerID uint, reason string) (int64, error)
	// ReconcileStaleLocks invalidates active orders whose offers are no longer
	// lockable according to the current stock state. It is the read-side guard
	// ensuring an invalidated order can never be read back as locked.
	ReconcileStaleLocks() (int64, error)
}

// TxManager runs a function inside a single database transaction.
type TxManager struct{ db *gorm.DB }

func NewTxManager(db *gorm.DB) *TxManager { return &TxManager{db: db} }

func (m *TxManager) Run(fn func(UnitOfWork) error) error {
	return m.db.Transaction(func(tx *gorm.DB) error {
		return fn(&unitOfWork{tx: tx})
	})
}

type unitOfWork struct{ tx *gorm.DB }

func (u *unitOfWork) FindOfferForUpdate(productID, offerID uint) (model.Offer, model.Product, model.Supplier, error) {
	var offer model.Offer
	if err := u.tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&offer, offerID).Error; err != nil {
		return offer, model.Product{}, model.Supplier{}, fmt.Errorf("lock offer %d: %w", offerID, err)
	}
	if offer.ProductID != productID {
		return offer, model.Product{}, model.Supplier{}, gorm.ErrRecordNotFound
	}
	var product model.Product
	if err := u.tx.First(&product, productID).Error; err != nil {
		return offer, product, model.Supplier{}, fmt.Errorf("load product %d: %w", productID, err)
	}
	var supplier model.Supplier
	if err := u.tx.First(&supplier, offer.SupplierID).Error; err != nil {
		return offer, product, supplier, fmt.Errorf("load supplier %d: %w", offer.SupplierID, err)
	}
	return offer, product, supplier, nil
}

func (u *unitOfWork) HasActiveLock(userID string, productID uint) (bool, error) {
	var count int64
	if err := u.tx.Model(&model.LockOrderItem{}).
		Where("user_id = ? AND product_id = ? AND active = ?", userID, productID, true).
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("probe active lock: %w", err)
	}
	return count > 0, nil
}

func (u *unitOfWork) InsertLock(order *model.LockOrder, items []model.LockOrderItem) error {
	if err := u.tx.Create(order).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("insert lock order: %w", ErrLockDuplicate)
		}
		return fmt.Errorf("insert lock order: %w", err)
	}
	for index := range items {
		items[index].LockOrderID = order.ID
	}
	if err := u.tx.Create(&items).Error; err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("insert lock items: %w", ErrLockDuplicate)
		}
		return fmt.Errorf("insert lock items: %w", err)
	}
	return nil
}

func (u *unitOfWork) ListLocksForUser(userID string) ([]model.LockOrder, error) {
	var orders []model.LockOrder
	if err := u.tx.Preload("Items").
		Where("user_id = ?", userID).Order("id DESC").Find(&orders).Error; err != nil {
		return nil, fmt.Errorf("list lock orders: %w", err)
	}
	return orders, nil
}

func (u *unitOfWork) GetLockByID(orderID uint) (model.LockOrder, error) {
	var order model.LockOrder
	if err := u.tx.Preload("Items").First(&order, orderID).Error; err != nil {
		return order, fmt.Errorf("get lock order %d: %w", orderID, err)
	}
	return order, nil
}

func (u *unitOfWork) LockOffer(offerID uint) (model.Offer, error) {
	var offer model.Offer
	if err := u.tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		First(&offer, offerID).Error; err != nil {
		return offer, fmt.Errorf("lock offer %d: %w", offerID, err)
	}
	return offer, nil
}

func (u *unitOfWork) SaveOfferStatus(offer *model.Offer, status string) error {
	offer.StockStatus = status
	if err := u.tx.Save(offer).Error; err != nil {
		return fmt.Errorf("save offer status: %w", err)
	}
	return nil
}

func (u *unitOfWork) InvalidateLocksForOffer(offerID uint, reason string) (int64, error) {
	var orderIDs []uint
	if err := u.tx.Model(&model.LockOrderItem{}).
		Where("offer_id = ? AND active = ?", offerID, true).
		Distinct().Pluck("lock_order_id", &orderIDs).Error; err != nil {
		return 0, fmt.Errorf("find locks for offer %d: %w", offerID, err)
	}
	return u.invalidateOrders(orderIDs, reason)
}

func (u *unitOfWork) ReconcileStaleLocks() (int64, error) {
	var affected int64
	for _, status := range []string{constants.StatusOutOfStock, constants.StatusDiscontinued} {
		var orderIDs []uint
		if err := u.tx.Model(&model.LockOrderItem{}).
			Joins("JOIN offers ON offers.id = lock_order_items.offer_id").
			Where("lock_order_items.active = ? AND offers.stock_status = ?", true, status).
			Distinct().Pluck("lock_order_items.lock_order_id", &orderIDs).Error; err != nil {
			return affected, fmt.Errorf("scan stale locks: %w", err)
		}
		count, err := u.invalidateOrders(orderIDs, status)
		affected += count
		if err != nil {
			return affected, err
		}
	}
	return affected, nil
}

func (u *unitOfWork) invalidateOrders(orderIDs []uint, reason string) (int64, error) {
	if len(orderIDs) == 0 {
		return 0, nil
	}
	if err := u.tx.Model(&model.LockOrder{}).
		Where("id IN ? AND status = ?", orderIDs, constants.LockOrderStatusActive).
		Updates(map[string]any{"status": constants.LockOrderStatusInvalid, "invalid_reason": reason}).Error; err != nil {
		return 0, fmt.Errorf("invalidate lock orders: %w", err)
	}
	if err := u.tx.Model(&model.LockOrderItem{}).
		Where("lock_order_id IN ?", orderIDs).Update("active", false).Error; err != nil {
		return 0, fmt.Errorf("deactivate lock items: %w", err)
	}
	return int64(len(orderIDs)), nil
}

// isUniqueViolation recognises PostgreSQL (23505) and SQLite (1555/2067)
// unique constraint failures without importing driver-specific packages.
func isUniqueViolation(err error) bool {
	message := err.Error()
	return strings.Contains(message, "23505") ||
		strings.Contains(message, "duplicate key value") ||
		strings.Contains(message, "UNIQUE constraint failed") ||
		strings.Contains(message, "Duplicate entry")
}

package model

import "gorm.io/gorm"

// LockOrder is a closed-loop price lock request covering 2-4 compared products.
// The whole order is rejected unless every selected quote is lockable, so no
// partial lock rows are ever persisted.
type LockOrder struct {
	gorm.Model
	OrderNo string `gorm:"size:32;uniqueIndex;not null"`
	UserID  string `gorm:"size:64;index;not null"`
	Status  string `gorm:"size:16;index;not null"`
	// InvalidReason records why an active lock became invalid, e.g. the offer
	// was switched to out_of_stock or discontinued by its supplier.
	InvalidReason string
	Items         []LockOrderItem
}

// LockOrderItem stores the committed quote snapshot. LockedUnitPrice is the
// price the buyer agreed at submit time; later offer price changes do not
// rewrite it. UserID is denormalized so the "one active lock per product"
// constraint can be enforced by a partial unique index.
type LockOrderItem struct {
	gorm.Model
	LockOrderID     uint   `gorm:"index;not null"`
	UserID          string `gorm:"size:64;not null"`
	ProductID       uint   `gorm:"not null"`
	ProductName     string
	OfferID         uint `gorm:"not null"`
	SupplierID      uint
	SupplierName    string
	LockedUnitPrice float64
	Quantity        int
	MOQ             int
	// Active mirrors the parent order status so the partial unique index
	// (user_id, product_id) WHERE active = true can enforce one active lock
	// per product without a subquery predicate.
	Active bool `gorm:"index;not null;default:false"`
}

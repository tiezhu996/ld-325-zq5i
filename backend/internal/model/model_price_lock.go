package model

import "gorm.io/gorm"

// PriceLock 是锁价单主表。任一明细引用的报价被供应商置为缺货/非在售时，
// 整单立即失效（status=invalid），刷新后也不能再回读成已锁定。
type PriceLock struct {
	gorm.Model
	OrderNo       string `gorm:"uniqueIndex;size:32;not null"`
	UserID        string `gorm:"index;size:64;not null"`
	Status        string `gorm:"size:16;index;not null;default:active"`
	InvalidReason string `gorm:"size:32"`
	Items         []PriceLockItem
}

// PriceLockItem 是锁价单明细，保存提交时刻的报价快照价；
// status 冗余主单状态，用于部分唯一索引保证同一材料至多一张有效锁价单。
type PriceLockItem struct {
	gorm.Model
	PriceLockID       uint `gorm:"index;not null"`
	ProductID         uint `gorm:"index;not null"`
	ProductName       string
	OfferID           uint `gorm:"index;not null"`
	SupplierID        uint
	SupplierName      string
	UnitPriceSnapshot float64
	MOQSnapshot       int
	QuantitySnapshot  int
	Status            string `gorm:"size:16;index;not null;default:active"`
}

package dto

// CreatePriceLockRequest 创建锁价单：一次锁定 2–4 款对比材料，
// 每款必须显式选择一家"有货且达到起订量"的报价；任一不满足则整单拒绝。
type CreatePriceLockRequest struct {
	Items []CreatePriceLockItem `json:"items" validate:"required,min=2,max=4,dive"`
}

type CreatePriceLockItem struct {
	ProductID uint `json:"product_id" validate:"required,gt=0"`
	OfferID   uint `json:"offer_id" validate:"required,gt=0"`
	Quantity  int  `json:"quantity" validate:"required,gt=0"`
}

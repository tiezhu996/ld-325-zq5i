package dto

// CreateLockOrderRequest is the closed-loop lock submission. It must contain
// 2-4 lines (matching the comparison tray), one offer per compared product.
type CreateLockOrderRequest struct {
	Items []CreateLockOrderItemRequest `json:"items" validate:"required,min=2,max=4,dive"`
}

type CreateLockOrderItemRequest struct {
	ProductID uint `json:"product_id" validate:"required,gt=0"`
	OfferID   uint `json:"offer_id" validate:"required,gt=0"`
	Quantity  int  `json:"quantity" validate:"required,gt=0"`
}

// LockOrderItemResponse exposes the committed snapshot: the locked price is
// what was submitted, never recomputed from the live offer.
type LockOrderItemResponse struct {
	ProductID       uint    `json:"product_id"`
	ProductName     string  `json:"product_name"`
	OfferID         uint    `json:"offer_id"`
	SupplierID      uint    `json:"supplier_id"`
	SupplierName    string  `json:"supplier_name"`
	LockedUnitPrice float64 `json:"locked_unit_price"`
	Quantity        int     `json:"quantity"`
	MOQ             int     `json:"moq"`
}

// LockOrderResponse carries the order number, snapshot lines and the current
// validity state so the UI can render expired locks after refresh.
type LockOrderResponse struct {
	ID            uint                    `json:"id"`
	OrderNo       string                  `json:"order_no"`
	Status        string                  `json:"status"`
	InvalidReason string                  `json:"invalid_reason,omitempty"`
	CreatedAt     string                  `json:"created_at"`
	Items         []LockOrderItemResponse `json:"items"`
}

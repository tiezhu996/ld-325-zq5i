package handler

import (
	"time"

	"github.com/blueship581/cybuildprice/backend/internal/dto"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type LockOrderHandler struct {
	service  *service.LockOrderService
	validate *validator.Validate
}

func NewLockOrderHandler(s *service.LockOrderService, v *validator.Validate) *LockOrderHandler {
	return &LockOrderHandler{service: s, validate: v}
}

// Create handles POST /lock-orders. Any business rejection (quote missing,
// out of stock, MOQ not met, existing active lock) aborts the whole order and
// the middleware renders the matching 4xx response.
func (h *LockOrderHandler) Create(c *gin.Context) {
	var req dto.CreateLockOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.Error(err)
		return
	}
	order, err := h.service.Submit(c.GetString("user_id"), req)
	if err != nil {
		c.Error(err)
		return
	}
	success(c, toLockOrderResponse(order))
}

// List handles GET /lock-orders and returns current lock states, including
// orders invalidated by supplier stock changes.
func (h *LockOrderHandler) List(c *gin.Context) {
	orders, err := h.service.List(c.GetString("user_id"))
	if err != nil {
		c.Error(err)
		return
	}
	items := make([]dto.LockOrderResponse, 0, len(orders))
	for _, order := range orders {
		items = append(items, toLockOrderResponse(order))
	}
	success(c, items)
}

func toLockOrderResponse(order model.LockOrder) dto.LockOrderResponse {
	items := make([]dto.LockOrderItemResponse, 0, len(order.Items))
	for _, item := range order.Items {
		items = append(items, dto.LockOrderItemResponse{
			ProductID:       item.ProductID,
			ProductName:     item.ProductName,
			OfferID:         item.OfferID,
			SupplierID:      item.SupplierID,
			SupplierName:    item.SupplierName,
			LockedUnitPrice: item.LockedUnitPrice,
			Quantity:        item.Quantity,
			MOQ:             item.MOQ,
		})
	}
	return dto.LockOrderResponse{
		ID:            order.ID,
		OrderNo:       order.OrderNo,
		Status:        order.Status,
		InvalidReason: order.InvalidReason,
		CreatedAt:     order.CreatedAt.Format(time.RFC3339),
		Items:         items,
	}
}

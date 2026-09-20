package handler

import (
	"net/http"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/dto"
	"github.com/blueship581/cybuildprice/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

type PriceLockHandler struct {
	service  *service.PriceLockService
	validate *validator.Validate
}

func NewPriceLockHandler(s *service.PriceLockService, v *validator.Validate) *PriceLockHandler {
	return &PriceLockHandler{service: s, validate: v}
}

// Create 创建锁价单：任一报价不满足条件都会整单拒绝。
func (h *PriceLockHandler) Create(c *gin.Context) {
	var req dto.CreatePriceLockRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.Error(err)
		return
	}
	if err := h.validate.Struct(req); err != nil {
		c.Error(err)
		return
	}
	lock, err := h.service.Create(c.GetString(constants.UserIDContextKey), req)
	if err != nil {
		c.Error(err)
		return
	}
	c.JSON(http.StatusCreated, dto.Response{Code: constants.SuccessCode, Message: constants.SuccessMessage, Data: lock})
}

// List 回读当前用户的锁价单，失效单据带失效状态返回。
func (h *PriceLockHandler) List(c *gin.Context) {
	locks, err := h.service.List(c.GetString(constants.UserIDContextKey))
	if err != nil {
		c.Error(err)
		return
	}
	success(c, locks)
}

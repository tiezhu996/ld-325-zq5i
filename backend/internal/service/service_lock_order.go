package service

import (
	"errors"
	"fmt"
	"time"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/dto"
	apperrors "github.com/blueship581/cybuildprice/backend/internal/errors"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	httpStatusConflict      = 409
	httpStatusUnprocessable = 422
)

type LockOrderService struct {
	tx *repository.TxManager
}

func NewLockOrderService(tx *repository.TxManager) *LockOrderService {
	return &LockOrderService{tx: tx}
}

// Submit creates a lock order only when every requested quote is lockable.
// When any line is invalid the whole transaction is rolled back, so the
// rejected order leaves no records at all.
func (s *LockOrderService) Submit(userID string, req dto.CreateLockOrderRequest) (model.LockOrder, error) {
	if duplicateProduct(req.Items) {
		return model.LockOrder{}, rejected(constants.LockItemsNotFoundCode, httpStatusUnprocessable, "同一材料只能选择一家报价进行锁价")
	}
	var created model.LockOrder
	err := s.tx.Run(func(uow repository.UnitOfWork) error {
		items := make([]model.LockOrderItem, 0, len(req.Items))
		seen := make(map[uint]struct{}, len(req.Items))
		for _, line := range req.Items {
			if _, ok := seen[line.ProductID]; ok {
				return rejected(constants.LockItemsNotFoundCode, httpStatusUnprocessable, "锁价明细中存在重复材料")
			}
			seen[line.ProductID] = struct{}{}
			offer, product, supplier, err := uow.FindOfferForUpdate(line.ProductID, line.OfferID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return rejected(constants.LockItemsNotFoundCode, httpStatusUnprocessable,
						fmt.Sprintf("材料 %d 的所选报价不存在", line.ProductID))
				}
				return fmt.Errorf("load lockable offer: %w", err)
			}
			if offer.StockStatus != constants.StatusInStock {
				return rejected(constants.LockItemsUnavailableCode, httpStatusUnprocessable,
					fmt.Sprintf("供应商 %s 的报价当前缺货或已非在售，无法锁价", supplier.Name))
			}
			if line.Quantity < offer.MOQ {
				return rejected(constants.LockItemsMOQCode, httpStatusUnprocessable,
					fmt.Sprintf("供应商 %s 的报价起订量为 %d，提交数量 %d 未达到", supplier.Name, offer.MOQ, line.Quantity))
			}
			conflict, err := uow.HasActiveLock(userID, line.ProductID)
			if err != nil {
				return fmt.Errorf("check active lock: %w", err)
			}
			if conflict {
				return conflictForProduct(line.ProductID)
			}
			items = append(items, model.LockOrderItem{
				UserID:          userID,
				ProductID:       line.ProductID,
				ProductName:     product.Name,
				OfferID:         offer.ID,
				SupplierID:      supplier.ID,
				SupplierName:    supplier.Name,
				LockedUnitPrice: offer.UnitPrice,
				Quantity:        line.Quantity,
				MOQ:             offer.MOQ,
				Active:          true,
			})
		}
		order := model.LockOrder{
			OrderNo: generateOrderNo(),
			UserID:  userID,
			Status:  constants.LockOrderStatusActive,
		}
		if err := uow.InsertLock(&order, items); err != nil {
			if errors.Is(err, repository.ErrLockDuplicate) {
				return conflictForProduct(0)
			}
			return fmt.Errorf("persist lock order: %w", err)
		}
		created = order
		return nil
	})
	if err != nil {
		return model.LockOrder{}, err
	}
	if err := s.tx.Run(func(uow repository.UnitOfWork) error {
		full, err := uow.GetLockByID(created.ID)
		if err != nil {
			return fmt.Errorf("reload lock order: %w", err)
		}
		created.Items = full.Items
		return nil
	}); err != nil {
		return model.LockOrder{}, err
	}
	return created, nil
}

// List returns the caller's lock orders. Stale active orders are reconciled
// inside a transaction first, so an offer that went out of stock can never be
// read back as a still-locked order.
func (s *LockOrderService) List(userID string) ([]model.LockOrder, error) {
	var orders []model.LockOrder
	err := s.tx.Run(func(uow repository.UnitOfWork) error {
		if _, err := uow.ReconcileStaleLocks(); err != nil {
			return fmt.Errorf("reconcile stale locks: %w", err)
		}
		rows, err := uow.ListLocksForUser(userID)
		if err != nil {
			return fmt.Errorf("list lock orders: %w", err)
		}
		orders = rows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return orders, nil
}

func duplicateProduct(items []dto.CreateLockOrderItemRequest) bool {
	seen := map[uint]struct{}{}
	for _, item := range items {
		if _, ok := seen[item.ProductID]; ok {
			return true
		}
		seen[item.ProductID] = struct{}{}
	}
	return false
}

func generateOrderNo() string {
	random := uuid.NewString()
	keep := 0
	short := make([]byte, 0, constants.LockNoRandomPartLength)
	for index := 0; index < len(random) && keep < constants.LockNoRandomPartLength; index++ {
		if random[index] != '-' {
			short = append(short, random[index])
			keep++
		}
	}
	return fmt.Sprintf("%s-%s-%s", constants.LockOrderNoPrefix, time.Now().Format(constants.LockNoDateLayout), string(short))
}

func rejected(code, status int, message string) error {
	return &apperrors.BusinessError{Code: code, Status: status, Message: message, Err: apperrors.ErrLockRejected}
}

func conflictForProduct(productID uint) error {
	message := "该材料已存在一张有效锁价单，同一时刻每款材料只允许一张有效锁价单"
	if productID > 0 {
		message = fmt.Sprintf("材料 %d %s", productID, message)
	}
	return &apperrors.BusinessError{
		Code:    constants.LockItemsConflictCode,
		Status:  httpStatusConflict,
		Message: message,
		Err:     apperrors.ErrLockConflict,
	}
}

package service

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/dto"
	apperrors "github.com/blueship581/cybuildprice/backend/internal/errors"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/repository"
	"gorm.io/gorm"
)

// PriceLockService 实现锁价单闭环：选中的每家报价都必须仍有货且达到起订量，
// 任一不满足则整单拒绝（不落任何记录）；同一材料同时刻只允许一张有效锁价单。
type PriceLockService struct {
	locks    repository.PriceLockRepository
	offers   repository.OfferRepository
	products repository.ProductRepository
}

func NewPriceLockService(locks repository.PriceLockRepository, offers repository.OfferRepository, products repository.ProductRepository) *PriceLockService {
	return &PriceLockService{locks: locks, offers: offers, products: products}
}

// Create 校验并创建锁价单。提交价以此刻报价单价为快照写入。
func (s *PriceLockService) Create(userID string, input dto.CreatePriceLockRequest) (model.PriceLock, error) {
	if len(input.Items) < 2 || len(input.Items) > 4 {
		return model.PriceLock{}, invalidLock("一次需锁定 2–4 款对比材料")
	}
	items := make([]model.PriceLockItem, 0, len(input.Items))
	productIDs := make([]uint, 0, len(input.Items))
	seen := make(map[uint]struct{}, len(input.Items))
	for _, line := range input.Items {
		if _, duplicated := seen[line.ProductID]; duplicated {
			return model.PriceLock{}, invalidLock("同一款材料在一张锁价单中只能选择一家报价")
		}
		seen[line.ProductID] = struct{}{}

		product, err := s.products.Get(line.ProductID)
		if err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return model.PriceLock{}, invalidLock("存在已下架或无法识别的材料，请刷新后重试")
			}
			return model.PriceLock{}, fmt.Errorf("load product for lock: %w", err)
		}
		offer, err := s.offers.Get(line.OfferID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return model.PriceLock{}, invalidLock("所选报价不存在或已失效")
			}
			return model.PriceLock{}, fmt.Errorf("load offer for lock: %w", err)
		}
		if offer.ProductID != line.ProductID {
			return model.PriceLock{}, invalidLock("所选报价不属于对应材料")
		}
		if offer.StockStatus != constants.StatusInStock {
			return model.PriceLock{}, invalidLock(fmt.Sprintf("%s 的所选商家当前无货，无法锁价", product.Name))
		}
		if line.Quantity < offer.MOQ {
			return model.PriceLock{}, invalidLock(fmt.Sprintf("%s 的下单数量未达到起订量 %d", product.Name, offer.MOQ))
		}
		items = append(items, model.PriceLockItem{
			ProductID:         line.ProductID,
			ProductName:       product.Name,
			OfferID:           offer.ID,
			SupplierID:        offer.SupplierID,
			SupplierName:      offer.Supplier.Name,
			UnitPriceSnapshot: offer.UnitPrice,
			MOQSnapshot:       offer.MOQ,
			QuantitySnapshot:  line.Quantity,
			Status:            constants.LockStatusActive,
		})
		productIDs = append(productIDs, line.ProductID)
	}

	conflicts, err := s.locks.ActiveProductIDs(productIDs)
	if err != nil {
		return model.PriceLock{}, fmt.Errorf("check active locks: %w", err)
	}
	if len(conflicts) > 0 {
		return model.PriceLock{}, &apperrors.BusinessError{
			Code:    constants.ErrorConflict,
			Err:     apperrors.ErrConflict,
			Message: "其中材料已存在有效锁价单，每款材料同一时刻只能锁定一次",
		}
	}

	lock := model.PriceLock{
		OrderNo: generateLockOrderNo(),
		UserID:  userID,
		Status:  constants.LockStatusActive,
		Items:   items,
	}
	created, err := s.locks.CreateInTx(lock)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrLockConflict):
			return model.PriceLock{}, &apperrors.BusinessError{
				Code:    constants.ErrorConflict,
				Err:     apperrors.ErrConflict,
				Message: "其中材料已存在有效锁价单，每款材料同一时刻只能锁定一次",
			}
		case errors.Is(err, repository.ErrLockItemInvalid):
			return model.PriceLock{}, invalidLock("提交瞬间报价状态发生变化（缺货、调价或不满足起订量），整单未锁定")
		default:
			return model.PriceLock{}, fmt.Errorf("create price lock: %w", err)
		}
	}
	return created, nil
}

// List 返回当前用户的锁价单；已失效单据同样保留并回读失效状态。
func (s *PriceLockService) List(userID string) ([]model.PriceLock, error) {
	rows, err := s.locks.ListByUser(userID)
	if err != nil {
		return nil, fmt.Errorf("list price locks service: %w", err)
	}
	return rows, nil
}

func invalidLock(message string) error {
	return &apperrors.BusinessError{
		Code:    constants.ErrorValidation,
		Err:     apperrors.ErrInvalidInput,
		Message: message,
	}
}

func generateLockOrderNo() string {
	return constants.LockOrderNoPrefix +
		time.Now().Format(constants.LockOrderTimeForm) +
		fmt.Sprintf("%04d", rand.Intn(constants.LockOrderRandomTo))
}

package service

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/dto"
	apperrors "github.com/blueship581/cybuildprice/backend/internal/errors"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newLockTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := model.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedLockFixtures(t *testing.T, db *gorm.DB) ([]model.Product, []model.Offer) {
	t.Helper()
	supplier := model.Supplier{Name: "测试建材店", Status: constants.SupplierApproved}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatalf("create supplier: %v", err)
	}
	products := []model.Product{
		{Name: "岩板"},
		{Name: "地板"},
		{Name: "乳胶漆"},
	}
	for index := range products {
		if err := db.Create(&products[index]).Error; err != nil {
			t.Fatalf("create product %d: %v", index, err)
		}
	}
	offers := []model.Offer{
		{ProductID: products[0].ID, SupplierID: supplier.ID, UnitPrice: 398, MOQ: 10, StockStatus: constants.StatusInStock},
		{ProductID: products[1].ID, SupplierID: supplier.ID, UnitPrice: 188, MOQ: 20, StockStatus: constants.StatusInStock},
		// 同款材料（地板）的缺货报价，用于整单拒绝场景。
		{ProductID: products[1].ID, SupplierID: supplier.ID, UnitPrice: 205, MOQ: 10, StockStatus: constants.StatusOutOfStock},
		// 岩板的另一家在售报价，用于失效后重新锁价。
		{ProductID: products[0].ID, SupplierID: supplier.ID, UnitPrice: 420, MOQ: 10, StockStatus: constants.StatusInStock},
	}
	for index := range offers {
		if err := db.Create(&offers[index]).Error; err != nil {
			t.Fatalf("create offer %d: %v", index, err)
		}
	}
	return products, offers
}

func lockRequest(products []model.Product, offers []model.Offer, quantity ...int) dto.CreateLockOrderRequest {
	items := []dto.CreateLockOrderItemRequest{}
	for index := range products {
		qty := 10
		if index < len(quantity) {
			qty = quantity[index]
		}
		items = append(items, dto.CreateLockOrderItemRequest{
			ProductID: products[index].ID,
			OfferID:   offers[index].ID,
			Quantity:  qty,
		})
	}
	return dto.CreateLockOrderRequest{Items: items}
}

func assertBusinessError(t *testing.T, err error, wantCode, wantStatus int, wantSentinel error) {
	t.Helper()
	var business *apperrors.BusinessError
	if !errors.As(err, &business) {
		t.Fatalf("expected business error, got %v", err)
	}
	if business.Code != wantCode || business.Status != wantStatus {
		t.Fatalf("unexpected code/status: %d/%d, want %d/%d", business.Code, business.Status, wantCode, wantStatus)
	}
	if wantSentinel != nil && !errors.Is(err, wantSentinel) {
		t.Fatalf("error does not wrap %v: %v", wantSentinel, err)
	}
}

func countLockRows(t *testing.T, db *gorm.DB) (int64, int64) {
	t.Helper()
	var orders, items int64
	db.Model(&model.LockOrder{}).Count(&orders)
	db.Model(&model.LockOrderItem{}).Count(&items)
	return orders, items
}

// 闭环 1：2-4 款材料全部可锁时，整单落库并保存提交价快照。
func TestLockOrderSubmitSuccess(t *testing.T) {
	db := newLockTestDB(t)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM lock_order_items").Error
		_ = db.Exec("DELETE FROM lock_orders").Error
	})
	products, offers := seedLockFixtures(t, db)
	svc := NewLockOrderService(repository.NewTxManager(db))

	order, err := svc.Submit("buyer-1", lockRequest(products[:2], offers[:2], 10, 25))
	if err != nil {
		t.Fatalf("submit lock: %v", err)
	}
	if order.OrderNo == "" || order.Status != constants.LockOrderStatusActive || len(order.Items) != 2 {
		t.Fatalf("unexpected created order: %+v", order)
	}
	if order.Items[0].LockedUnitPrice != 398 || order.Items[0].ProductName != "岩板" {
		t.Fatalf("snapshot not stored: %+v", order.Items[0])
	}
	// 快照价不随后续报价变动而改写。
	db.Model(&model.Offer{}).Where("id = ?", offers[0].ID).Update("unit_price", 450)
	list, err := svc.List("buyer-1")
	if err != nil || len(list) != 1 || list[0].Items[0].LockedUnitPrice != 398 {
		t.Fatalf("snapshot price should remain 398, got %+v err=%v", list, err)
	}
}

// 闭环 2：任一报价缺货/非在售或未达起订量，整单拒绝且不落任何记录。
func TestLockOrderRejectedLeavesNoRecords(t *testing.T) {
	cases := []struct {
		name      string
		build     func(products []model.Product, offers []model.Offer) dto.CreateLockOrderRequest
		wantCode  int
		wrapError error
	}{
		{
			name: "one offer out of stock",
			build: func(p []model.Product, o []model.Offer) dto.CreateLockOrderRequest {
				return lockRequest(p[:2], []model.Offer{o[0], o[2]}, 10, 10)
			},
			wantCode:  constants.LockItemsUnavailableCode,
			wrapError: apperrors.ErrLockRejected,
		},
		{
			name: "quantity below MOQ",
			build: func(p []model.Product, o []model.Offer) dto.CreateLockOrderRequest {
				return lockRequest(p[:2], o[:2], 10, 5)
			},
			wantCode:  constants.LockItemsMOQCode,
			wrapError: apperrors.ErrLockRejected,
		},
		{
			name: "offer belongs to another product",
			build: func(p []model.Product, o []model.Offer) dto.CreateLockOrderRequest {
				req := lockRequest(p[:2], o[:2], 10, 20)
				req.Items[1].OfferID = o[0].ID
				return req
			},
			wantCode:  constants.LockItemsNotFoundCode,
			wrapError: apperrors.ErrLockRejected,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := newLockTestDB(t)
			t.Cleanup(func() {
				_ = db.Exec("DELETE FROM lock_order_items").Error
				_ = db.Exec("DELETE FROM lock_orders").Error
			})
			products, offers := seedLockFixtures(t, db)
			svc := NewLockOrderService(repository.NewTxManager(db))

			_, err := svc.Submit("buyer-2", tc.build(products, offers))
			assertBusinessError(t, err, tc.wantCode, httpStatusUnprocessable, tc.wrapError)
			orders, items := countLockRows(t, db)
			if orders != 0 || items != 0 {
				t.Fatalf("rejected order persisted rows: orders=%d items=%d", orders, items)
			}
		})
	}
}

// 闭环 3：每款材料同一时刻只允许一张有效锁价单，重复提交返回 409 冲突。
func TestLockOrderDuplicateProductConflict(t *testing.T) {
	db := newLockTestDB(t)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM lock_order_items").Error
		_ = db.Exec("DELETE FROM lock_orders").Error
	})
	products, offers := seedLockFixtures(t, db)
	svc := NewLockOrderService(repository.NewTxManager(db))

	if _, err := svc.Submit("buyer-3", lockRequest(products[:2], offers[:2], 10, 20)); err != nil {
		t.Fatalf("first submit: %v", err)
	}
	_, err := svc.Submit("buyer-3", lockRequest(products[:2], offers[:2], 12, 22))
	assertBusinessError(t, err, constants.LockItemsConflictCode, httpStatusConflict, apperrors.ErrLockConflict)

	// 不同用户互不影响。
	if _, err := svc.Submit("buyer-other", lockRequest(products[:2], offers[:2], 10, 20)); err != nil {
		t.Fatalf("other user lock should be allowed: %v", err)
	}
}

// 闭环 4：供应商把报价置为缺货/非在售，关联锁价单立即失效，且刷新不能回读成已锁定。
func TestOfferStatusChangeInvalidatesLocks(t *testing.T) {
	db := newLockTestDB(t)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM lock_order_items").Error
		_ = db.Exec("DELETE FROM lock_orders").Error
	})
	products, offers := seedLockFixtures(t, db)
	lockSvc := NewLockOrderService(repository.NewTxManager(db))
	offerSvc := NewOfferService(repository.NewOfferRepository(db), repository.NewTxManager(db))

	order, err := lockSvc.Submit("buyer-4", lockRequest(products[:2], offers[:2], 10, 20))
	if err != nil {
		t.Fatalf("submit lock: %v", err)
	}
	updated, err := offerSvc.UpdateStatus(offers[0].ID, constants.StatusOutOfStock)
	if err != nil || updated.StockStatus != constants.StatusOutOfStock {
		t.Fatalf("update offer status: %+v err=%v", updated, err)
	}
	list, err := lockSvc.List("buyer-4")
	if err != nil || len(list) != 1 {
		t.Fatalf("list locks: %+v err=%v", list, err)
	}
	if list[0].Status != constants.LockOrderStatusInvalid || list[0].InvalidReason != constants.StatusOutOfStock {
		t.Fatalf("lock should be invalid: %+v", list[0])
	}
	for _, item := range list[0].Items {
		if item.LockedUnitPrice == 0 {
			t.Fatalf("snapshot price must remain readable on invalid order")
		}
	}
	// 失效后允许对同款材料用另一家在售报价重新锁价。
	relockReq := dto.CreateLockOrderRequest{Items: []dto.CreateLockOrderItemRequest{
		{ProductID: products[0].ID, OfferID: offers[3].ID, Quantity: 10},
		{ProductID: products[1].ID, OfferID: offers[1].ID, Quantity: 20},
	}}
	relocked, err := lockSvc.Submit("buyer-4", relockReq)
	if err != nil {
		t.Fatalf("relock after invalidation should succeed: %v", err)
	}
	if relocked.ID == order.ID {
		t.Fatalf("expected a new order record")
	}
}

// 边界：明细数量必须是 2-4 款，重复材料在 service 层即被拒。
func TestLockOrderDuplicateProductInRequest(t *testing.T) {
	db := newLockTestDB(t)
	t.Cleanup(func() {
		_ = db.Exec("DELETE FROM lock_order_items").Error
		_ = db.Exec("DELETE FROM lock_orders").Error
	})
	products, offers := seedLockFixtures(t, db)
	svc := NewLockOrderService(repository.NewTxManager(db))

	req := lockRequest(products[:2], offers[:2], 10, 20)
	req.Items = append(req.Items, dto.CreateLockOrderItemRequest{ProductID: products[0].ID, OfferID: offers[0].ID, Quantity: 10})
	_, err := svc.Submit("buyer-5", req)
	assertBusinessError(t, err, constants.LockItemsNotFoundCode, httpStatusUnprocessable, apperrors.ErrLockRejected)
	orders, items := countLockRows(t, db)
	if orders != 0 || items != 0 {
		t.Fatalf("rejected order persisted rows: orders=%d items=%d", orders, items)
	}
}

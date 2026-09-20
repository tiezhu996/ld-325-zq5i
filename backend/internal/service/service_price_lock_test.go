package service

import (
	"errors"
	"testing"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/dto"
	apperrors "github.com/blueship581/cybuildprice/backend/internal/errors"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/repository"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type lockFixture struct {
	db        *gorm.DB
	svc       *PriceLockService
	offerRepo repository.OfferRepository
	lockRepo  repository.PriceLockRepository
}

func newLockFixture(t *testing.T) lockFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Migrate(db); err != nil {
		t.Fatal(err)
	}
	productRepo := repository.NewProductRepository(db)
	offerRepo := repository.NewOfferRepository(db)
	lockRepo := repository.NewPriceLockRepository(db)
	return lockFixture{
		db:        db,
		svc:       NewPriceLockService(lockRepo, offerRepo, productRepo),
		offerRepo: offerRepo,
		lockRepo:  lockRepo,
	}
}

func seedOffer(t *testing.T, db *gorm.DB, name string, price float64, moq int, status string) (uint, uint) {
	t.Helper()
	category := model.Category{Name: name + "类"}
	if err := db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}
	product := model.Product{Name: name, Brand: "测试", CategoryID: category.ID, Unit: "件"}
	if err := db.Create(&product).Error; err != nil {
		t.Fatal(err)
	}
	supplier := model.Supplier{Name: name + "供应商", Status: constants.SupplierApproved}
	if err := db.Create(&supplier).Error; err != nil {
		t.Fatal(err)
	}
	offer := model.Offer{ProductID: product.ID, SupplierID: supplier.ID, UnitPrice: price, MOQ: moq, StockStatus: status}
	if err := db.Create(&offer).Error; err != nil {
		t.Fatal(err)
	}
	return product.ID, offer.ID
}

func lockReq(ids [][2]uint, qty int) dto.CreatePriceLockRequest {
	items := make([]dto.CreatePriceLockItem, 0, len(ids))
	for _, pair := range ids {
		items = append(items, dto.CreatePriceLockItem{ProductID: pair[0], OfferID: pair[1], Quantity: qty})
	}
	return dto.CreatePriceLockRequest{Items: items}
}

func TestCreatePriceLockSucceedsAndSnapshotsPrice(t *testing.T) {
	fx := newLockFixture(t)
	p1, o1 := seedOffer(t, fx.db, "岩板", 398, 10, constants.StatusInStock)
	p2, o2 := seedOffer(t, fx.db, "地板", 188, 5, constants.StatusInStock)

	lock, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p1, o1}, {p2, o2}}, 12))
	if err != nil {
		t.Fatalf("expected lock created, got %v", err)
	}
	if lock.Status != constants.LockStatusActive || lock.OrderNo == "" {
		t.Fatalf("unexpected active lock: %+v", lock)
	}
	if lock.Items[0].UnitPriceSnapshot != 398 || lock.Items[0].QuantitySnapshot != 12 {
		t.Fatalf("snapshot mismatch: %+v", lock.Items[0])
	}
}

func TestCreatePriceLockRejectsInvalidWholeOrderWithoutRecords(t *testing.T) {
	cases := []struct {
		name   string
		status string
		qty    int
		moq    int
	}{
		{name: "缺货报价整单拒绝", status: constants.StatusOutOfStock, qty: 10, moq: 5},
		{name: "停产报价整单拒绝", status: constants.StatusDiscontinued, qty: 10, moq: 5},
		{name: "未达起订量整单拒绝", status: constants.StatusInStock, qty: 3, moq: 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fx := newLockFixture(t)
			goodProduct, goodOffer := seedOffer(t, fx.db, "好材料", 100, 1, constants.StatusInStock)
			badProduct, badOffer := seedOffer(t, fx.db, "问题材料", 200, tc.moq, tc.status)

			_, err := fx.svc.Create("demo-user", lockReq([][2]uint{{goodProduct, goodOffer}, {badProduct, badOffer}}, tc.qty))
			if err == nil {
				t.Fatal("expected rejection")
			}
			var business *apperrors.BusinessError
			if !errors.As(err, &business) || business.Code != constants.ErrorValidation {
				t.Fatalf("expected validation business error, got %T %v", err, err)
			}
			var lockCount, itemCount int64
			fx.db.Model(&model.PriceLock{}).Count(&lockCount)
			fx.db.Model(&model.PriceLockItem{}).Count(&itemCount)
			if lockCount != 0 || itemCount != 0 {
				t.Fatalf("rejected order must not persist records, locks=%d items=%d", lockCount, itemCount)
			}
		})
	}
}

func TestDuplicateProductSubmissionConflicts(t *testing.T) {
	fx := newLockFixture(t)
	p1, o1 := seedOffer(t, fx.db, "瓷砖", 88, 1, constants.StatusInStock)
	p2, o2 := seedOffer(t, fx.db, "涂料", 99, 1, constants.StatusInStock)
	p3, o3 := seedOffer(t, fx.db, "五金", 77, 1, constants.StatusInStock)

	if _, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p1, o1}, {p2, o2}}, 5)); err != nil {
		t.Fatal(err)
	}
	// 重复提交包含已锁材料 p2，应返回冲突且不落第二张单。
	_, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p2, o2}, {p3, o3}}, 5))
	var business *apperrors.BusinessError
	if !errors.As(err, &business) || business.Code != constants.ErrorConflict {
		t.Fatalf("expected conflict error, got %v", err)
	}
	var lockCount int64
	fx.db.Model(&model.PriceLock{}).Count(&lockCount)
	if lockCount != 1 {
		t.Fatalf("conflicting submission must not be saved, got %d locks", lockCount)
	}
}

func TestOfferOutOfStockInvalidatesLockImmediately(t *testing.T) {
	fx := newLockFixture(t)
	p1, o1 := seedOffer(t, fx.db, "岩板", 398, 10, constants.StatusInStock)
	p2, o2 := seedOffer(t, fx.db, "地板", 188, 5, constants.StatusInStock)
	if _, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p1, o1}, {p2, o2}}, 10)); err != nil {
		t.Fatal(err)
	}

	if _, err := fx.offerRepo.UpdateStatus(o1, constants.StatusOutOfStock); err != nil {
		t.Fatal(err)
	}
	locks, err := fx.lockRepo.ListByUser("demo-user")
	if err != nil || len(locks) != 1 {
		t.Fatalf("expected one lock readable, got %d err=%v", len(locks), err)
	}
	if locks[0].Status != constants.LockStatusInvalid || locks[0].InvalidReason != constants.LockInvalidReasonOutOfStock {
		t.Fatalf("lock must be invalidated: %+v", locks[0])
	}
	for _, item := range locks[0].Items {
		if item.Status != constants.LockStatusInvalid {
			t.Fatalf("item %d still reads as locked: %+v", item.ID, item)
		}
		if item.UnitPriceSnapshot <= 0 {
			t.Fatalf("snapshot price must remain for display: %+v", item)
		}
	}
}

func TestInvalidatedLockReleasesProductForNewLock(t *testing.T) {
	fx := newLockFixture(t)
	p1, o1 := seedOffer(t, fx.db, "岩板", 398, 10, constants.StatusInStock)
	p2, o2 := seedOffer(t, fx.db, "地板", 188, 5, constants.StatusInStock)
	if _, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p1, o1}, {p2, o2}}, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := fx.offerRepo.UpdateStatus(o1, constants.StatusOutOfStock); err != nil {
		t.Fatal(err)
	}
	// 旧单失效后，同一款材料可在另一张有效单中重新锁定。
	p3, o3 := seedOffer(t, fx.db, "灯具", 159, 1, constants.StatusInStock)
	if _, err := fx.svc.Create("demo-user", lockReq([][2]uint{{p2, o2}, {p3, o3}}, 8)); err != nil {
		t.Fatalf("relock after invalidation should succeed, got %v", err)
	}
}

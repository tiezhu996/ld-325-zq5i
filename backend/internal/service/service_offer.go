package service

import (
	"fmt"

	"github.com/blueship581/cybuildprice/backend/internal/constants"
	"github.com/blueship581/cybuildprice/backend/internal/model"
	"github.com/blueship581/cybuildprice/backend/internal/repository"
)

type OfferService struct {
	repo repository.OfferRepository
	tx   *repository.TxManager
}

func NewOfferService(repo repository.OfferRepository, tx *repository.TxManager) *OfferService {
	return &OfferService{repo: repo, tx: tx}
}

func (s *OfferService) List(productID uint) ([]model.Offer, error) {
	rows, err := s.repo.ListByProduct(productID)
	if err != nil {
		return nil, fmt.Errorf("list offer service: %w", err)
	}
	return rows, nil
}

// UpdateStatus changes a quote's stock state. When the supplier marks the
// quote out of stock or discontinued, every active lock order relying on it is
// invalidated in the same transaction ("immediately", and atomically with the
// quote state change).
func (s *OfferService) UpdateStatus(id uint, status string) (model.Offer, error) {
	var updated model.Offer
	err := s.tx.Run(func(uow repository.UnitOfWork) error {
		offer, err := uow.LockOffer(id)
		if err != nil {
			return fmt.Errorf("update offer status: %w", err)
		}
		if offer.StockStatus == status {
			updated = offer
			return nil
		}
		if err := uow.SaveOfferStatus(&offer, status); err != nil {
			return fmt.Errorf("update offer status: %w", err)
		}
		updated = offer
		if status != constants.StatusInStock {
			if _, err := uow.InvalidateLocksForOffer(id, status); err != nil {
				return fmt.Errorf("invalidate linked locks: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return model.Offer{}, err
	}
	return updated, nil
}

package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/repository"
)

// WishService 찜하기 서비스 인터페이스
type WishService interface {
	ToggleWish(userID, itemID uint64) (bool, error)
	ListWishes(userID uint64, page, limit int) ([]*domain.WishItemResponse, *common.Meta, error)
}

type wishService struct {
	wishRepo repository.WishRepository
	itemRepo repository.ItemRepository
}

// NewWishService 찜하기 서비스 생성
func NewWishService(
	wishRepo repository.WishRepository,
	itemRepo repository.ItemRepository,
) WishService {
	return &wishService{
		wishRepo: wishRepo,
		itemRepo: itemRepo,
	}
}

func (s *wishService) ToggleWish(userID, itemID uint64) (bool, error) {
	// 상품 존재 확인
	_, err := s.itemRepo.FindByID(itemID)
	if err != nil {
		return false, common.ErrNotFound
	}

	// 이미 찜했는지 확인
	exists, err := s.wishRepo.Exists(userID, itemID)
	if err != nil {
		return false, err
	}

	if exists {
		// 찜 해제
		if err := s.wishRepo.Delete(userID, itemID); err != nil {
			return false, err
		}
		_ = s.itemRepo.IncrementWishCount(itemID, -1)
		return false, nil
	}

	// 찜 추가
	wish := &domain.Wish{
		UserID: userID,
		ItemID: itemID,
	}
	if err := s.wishRepo.Create(wish); err != nil {
		return false, err
	}
	_ = s.itemRepo.IncrementWishCount(itemID, 1)
	return true, nil
}

func (s *wishService) ListWishes(userID uint64, page, limit int) ([]*domain.WishItemResponse, *common.Meta, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	wishes, total, err := s.wishRepo.ListByUser(userID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.WishItemResponse, len(wishes))
	for i, wish := range wishes {
		var itemResp *domain.ItemListResponse
		if wish.Item != nil {
			itemResp = toWishItemListResponse(wish.Item)
		}
		responses[i] = &domain.WishItemResponse{
			ID:        wish.ID,
			Item:      itemResp,
			CreatedAt: wish.CreatedAt,
		}
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	return responses, meta, nil
}

func toWishItemListResponse(item *domain.Item) *domain.ItemListResponse {
	var categoryName string
	if item.Category != nil {
		categoryName = item.Category.Name
	}

	return &domain.ItemListResponse{
		ID:           item.ID,
		SellerID:     item.SellerID,
		CategoryName: categoryName,
		Title:        item.Title,
		Price:        item.Price,
		Currency:     item.Currency,
		Condition:    item.Condition,
		Status:       item.Status,
		StatusText:   domain.GetStatusText(item.Status),
		Location:     item.Location,
		IsNegotiable: item.IsNegotiable,
		ViewCount:    item.ViewCount,
		WishCount:    item.WishCount,
		ChatCount:    item.ChatCount,
		IsWished:     true,
		CreatedAt:    item.CreatedAt,
		TimeAgo:      formatTimeAgo(item.CreatedAt),
	}
}

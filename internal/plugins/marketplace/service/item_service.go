package service

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/repository"
)

// ItemService 상품 서비스 인터페이스
type ItemService interface {
	CreateItem(sellerID uint64, req *domain.CreateItemRequest) (*domain.ItemResponse, error)
	GetItem(id uint64, viewerID *uint64) (*domain.ItemResponse, error)
	UpdateItem(id uint64, sellerID uint64, req *domain.UpdateItemRequest) error
	DeleteItem(id uint64, sellerID uint64) error
	ListItems(params *repository.ItemListParams, viewerID *uint64) ([]*domain.ItemListResponse, *common.Meta, error)
	ListMyItems(sellerID uint64, page, limit int) ([]*domain.ItemListResponse, *common.Meta, error)
	UpdateStatus(id uint64, sellerID uint64, req *domain.UpdateStatusRequest) error
	BumpItem(id uint64, sellerID uint64) error
}

type itemService struct {
	itemRepo     repository.ItemRepository
	categoryRepo repository.CategoryRepository
	wishRepo     repository.WishRepository
}

// NewItemService 상품 서비스 생성
func NewItemService(
	itemRepo repository.ItemRepository,
	categoryRepo repository.CategoryRepository,
	wishRepo repository.WishRepository,
) ItemService {
	return &itemService{
		itemRepo:     itemRepo,
		categoryRepo: categoryRepo,
		wishRepo:     wishRepo,
	}
}

func (s *itemService) CreateItem(sellerID uint64, req *domain.CreateItemRequest) (*domain.ItemResponse, error) {
	// 카테고리 유효성 검사
	if req.CategoryID != nil {
		_, err := s.categoryRepo.FindByID(*req.CategoryID)
		if err != nil {
			return nil, errors.New("invalid category")
		}
	}

	// 이미지 JSON 변환
	imagesJSON, _ := json.Marshal(req.Images)

	item := &domain.Item{
		SellerID:      sellerID,
		CategoryID:    req.CategoryID,
		Title:         req.Title,
		Description:   req.Description,
		Price:         req.Price,
		OriginalPrice: req.OriginalPrice,
		Condition:     req.Condition,
		TradeMethod:   req.TradeMethod,
		Location:      req.Location,
		IsNegotiable:  req.IsNegotiable,
		Images:        string(imagesJSON),
		Status:        domain.ItemStatusSelling,
	}

	if err := s.itemRepo.Create(item); err != nil {
		return nil, err
	}

	// 카테고리 상품 수 증가
	if req.CategoryID != nil {
		_ = s.categoryRepo.IncrementItemCount(*req.CategoryID, 1)
	}

	return s.toItemResponse(item, false), nil
}

func (s *itemService) GetItem(id uint64, viewerID *uint64) (*domain.ItemResponse, error) {
	item, err := s.itemRepo.FindByID(id)
	if err != nil {
		return nil, common.ErrNotFound
	}

	// 조회수 증가 (본인 상품이 아닌 경우)
	if viewerID == nil || *viewerID != item.SellerID {
		_ = s.itemRepo.IncrementViewCount(id)
		item.ViewCount++
	}

	// 찜 여부 확인
	var isWished bool
	if viewerID != nil {
		isWished, _ = s.wishRepo.Exists(*viewerID, id)
	}

	return s.toItemResponse(item, isWished), nil
}

func (s *itemService) UpdateItem(id uint64, sellerID uint64, req *domain.UpdateItemRequest) error {
	item, err := s.itemRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	if item.SellerID != sellerID {
		return common.ErrForbidden
	}

	oldCategoryID := item.CategoryID

	// 필드 업데이트
	if req.CategoryID != nil {
		item.CategoryID = req.CategoryID
	}
	if req.Title != nil {
		item.Title = *req.Title
	}
	if req.Description != nil {
		item.Description = *req.Description
	}
	if req.Price != nil {
		item.Price = *req.Price
	}
	if req.OriginalPrice != nil {
		item.OriginalPrice = req.OriginalPrice
	}
	if req.Condition != nil {
		item.Condition = *req.Condition
	}
	if req.TradeMethod != nil {
		item.TradeMethod = *req.TradeMethod
	}
	if req.Location != nil {
		item.Location = *req.Location
	}
	if req.IsNegotiable != nil {
		item.IsNegotiable = *req.IsNegotiable
	}
	if req.Images != nil {
		imagesJSON, _ := json.Marshal(req.Images)
		item.Images = string(imagesJSON)
	}

	if err := s.itemRepo.Update(item); err != nil {
		return err
	}

	// 카테고리 상품 수 업데이트
	if req.CategoryID != nil && (oldCategoryID == nil || *oldCategoryID != *req.CategoryID) {
		if oldCategoryID != nil {
			_ = s.categoryRepo.IncrementItemCount(*oldCategoryID, -1)
		}
		_ = s.categoryRepo.IncrementItemCount(*req.CategoryID, 1)
	}

	return nil
}

func (s *itemService) DeleteItem(id uint64, sellerID uint64) error {
	item, err := s.itemRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	if item.SellerID != sellerID {
		return common.ErrForbidden
	}

	// 찜 데이터 삭제
	_ = s.wishRepo.DeleteByItem(id)

	// 카테고리 상품 수 감소
	if item.CategoryID != nil {
		_ = s.categoryRepo.IncrementItemCount(*item.CategoryID, -1)
	}

	return s.itemRepo.Delete(id)
}

func (s *itemService) ListItems(params *repository.ItemListParams, viewerID *uint64) ([]*domain.ItemListResponse, *common.Meta, error) {
	if params.Page <= 0 {
		params.Page = 1
	}
	if params.Limit <= 0 {
		params.Limit = 20
	}
	if params.Limit > 100 {
		params.Limit = 100
	}

	items, total, err := s.itemRepo.List(params)
	if err != nil {
		return nil, nil, err
	}

	// 찜 여부 확인
	var wishedMap map[uint64]bool
	if viewerID != nil && len(items) > 0 {
		itemIDs := make([]uint64, len(items))
		for i, item := range items {
			itemIDs[i] = item.ID
		}
		wishedMap, _ = s.wishRepo.GetWishedItemIDs(*viewerID, itemIDs)
	}

	responses := make([]*domain.ItemListResponse, len(items))
	for i, item := range items {
		isWished := wishedMap != nil && wishedMap[item.ID]
		responses[i] = s.toItemListResponse(item, isWished)
	}

	meta := &common.Meta{
		Page:  params.Page,
		Limit: params.Limit,
		Total: total,
	}

	return responses, meta, nil
}

func (s *itemService) ListMyItems(sellerID uint64, page, limit int) ([]*domain.ItemListResponse, *common.Meta, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	items, total, err := s.itemRepo.ListBySeller(sellerID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.ItemListResponse, len(items))
	for i, item := range items {
		responses[i] = s.toItemListResponse(item, false)
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	return responses, meta, nil
}

func (s *itemService) UpdateStatus(id uint64, sellerID uint64, req *domain.UpdateStatusRequest) error {
	item, err := s.itemRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	if item.SellerID != sellerID {
		return common.ErrForbidden
	}

	return s.itemRepo.UpdateStatus(id, req.Status, req.BuyerID)
}

func (s *itemService) BumpItem(id uint64, sellerID uint64) error {
	item, err := s.itemRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	if item.SellerID != sellerID {
		return common.ErrForbidden
	}

	return s.itemRepo.BumpItem(id)
}

// Helper functions

func (s *itemService) toItemResponse(item *domain.Item, isWished bool) *domain.ItemResponse {
	var images []string
	_ = json.Unmarshal([]byte(item.Images), &images)

	var categoryResp *domain.CategoryResponse
	if item.Category != nil {
		categoryResp = item.Category.ToResponse()
	}

	return &domain.ItemResponse{
		ID:            item.ID,
		SellerID:      item.SellerID,
		Category:      categoryResp,
		Title:         item.Title,
		Description:   item.Description,
		Price:         item.Price,
		OriginalPrice: item.OriginalPrice,
		Currency:      item.Currency,
		Condition:     item.Condition,
		ConditionText: domain.GetConditionText(item.Condition),
		Status:        item.Status,
		StatusText:    domain.GetStatusText(item.Status),
		TradeMethod:   item.TradeMethod,
		TradeText:     domain.GetTradeText(item.TradeMethod),
		Location:      item.Location,
		IsNegotiable:  item.IsNegotiable,
		ViewCount:     item.ViewCount,
		WishCount:     item.WishCount,
		ChatCount:     item.ChatCount,
		Images:        images,
		IsWished:      isWished,
		CreatedAt:     item.CreatedAt,
		UpdatedAt:     item.UpdatedAt,
		TimeAgo:       formatTimeAgo(item.CreatedAt),
	}
}

func (s *itemService) toItemListResponse(item *domain.Item, isWished bool) *domain.ItemListResponse {
	var images []string
	_ = json.Unmarshal([]byte(item.Images), &images)

	var thumbnail string
	if len(images) > 0 {
		thumbnail = images[0]
	}

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
		Thumbnail:    thumbnail,
		IsWished:     isWished,
		CreatedAt:    item.CreatedAt,
		TimeAgo:      formatTimeAgo(item.CreatedAt),
	}
}

func formatTimeAgo(t time.Time) string {
	diff := time.Since(t)

	switch {
	case diff < time.Minute:
		return "방금 전"
	case diff < time.Hour:
		return string(rune(int(diff.Minutes()))) + "분 전"
	case diff < 24*time.Hour:
		return string(rune(int(diff.Hours()))) + "시간 전"
	case diff < 7*24*time.Hour:
		return string(rune(int(diff.Hours()/24))) + "일 전"
	case diff < 30*24*time.Hour:
		return string(rune(int(diff.Hours()/(24*7)))) + "주 전"
	case diff < 365*24*time.Hour:
		return string(rune(int(diff.Hours()/(24*30)))) + "개월 전"
	default:
		return string(rune(int(diff.Hours()/(24*365)))) + "년 전"
	}
}

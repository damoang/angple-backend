package service

import (
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"github.com/damoang/angple-backend/internal/plugins/commerce/security"
)

// Commerce 에러 정의
var (
	ErrProductNotFound   = errors.New("product not found")
	ErrProductForbidden  = errors.New("you are not the owner of this product")
	ErrSlugAlreadyExists = errors.New("slug already exists")
)

// ProductService 상품 서비스 인터페이스
type ProductService interface {
	// 판매자용
	CreateProduct(sellerID uint64, req *domain.CreateProductRequest) (*domain.ProductResponse, error)
	GetMyProduct(sellerID uint64, productID uint64) (*domain.ProductResponse, error)
	UpdateProduct(sellerID uint64, productID uint64, req *domain.UpdateProductRequest) (*domain.ProductResponse, error)
	DeleteProduct(sellerID uint64, productID uint64) error
	ListMyProducts(sellerID uint64, req *domain.ProductListRequest) ([]*domain.ProductResponse, *common.Meta, error)

	// 공개용
	GetShopProduct(productID uint64) (*domain.ProductResponse, error)
	GetShopProductBySlug(slug string) (*domain.ProductResponse, error)
	ListShopProducts(req *domain.ProductListRequest) ([]*domain.ProductResponse, *common.Meta, error)
}

// productService 구현체
type productService struct {
	repo      repository.ProductRepository
	validator *security.Validator
	sanitizer *security.Sanitizer
}

// NewProductService 생성자
func NewProductService(repo repository.ProductRepository) ProductService {
	return &productService{
		repo:      repo,
		validator: security.NewValidator(),
		sanitizer: security.NewRichTextSanitizer(),
	}
}

// CreateProduct 상품 생성
func (s *productService) CreateProduct(sellerID uint64, req *domain.CreateProductRequest) (*domain.ProductResponse, error) {
	// 입력 검증 (Phase 7: 보안 강화)
	if errs := s.validator.ValidateProductRequest(req.Name, req.Description, req.Slug, req.Price); errs.HasErrors() {
		return nil, errors.New(errs.Error())
	}

	// 입력 살균화 (Phase 7: XSS 방지)
	req.Name = s.sanitizer.SanitizeString(req.Name)
	req.ShortDesc = s.sanitizer.SanitizeString(req.ShortDesc)
	req.Description = s.sanitizer.SanitizeHTML(req.Description) // 리치 텍스트 허용

	// 슬러그 중복 확인
	if req.Slug != "" {
		available, err := s.repo.IsSlugAvailable(req.Slug, 0)
		if err != nil {
			return nil, err
		}
		if !available {
			return nil, ErrSlugAlreadyExists
		}
	}

	// 기본값 설정
	currency := req.Currency
	if currency == "" {
		currency = "KRW"
	}
	status := domain.ProductStatus(req.Status)
	if status == "" {
		status = domain.ProductStatusDraft
	}
	visibility := req.Visibility
	if visibility == "" {
		visibility = "public"
	}

	product := &domain.Product{
		SellerID:       sellerID,
		Name:           req.Name,
		Slug:           req.Slug,
		Description:    req.Description,
		ShortDesc:      req.ShortDesc,
		ProductType:    domain.ProductType(req.ProductType),
		Price:          req.Price,
		OriginalPrice:  req.OriginalPrice,
		Currency:       currency,
		StockQuantity:  req.StockQuantity,
		StockStatus:    domain.StockStatusInStock,
		DownloadLimit:  req.DownloadLimit,
		DownloadExpiry: req.DownloadExpiry,
		Status:         status,
		Visibility:     visibility,
		Password:       req.Password,
		FeaturedImage:  req.FeaturedImage,
	}

	if err := s.repo.Create(product); err != nil {
		return nil, err
	}

	return product.ToResponse(), nil
}

// GetMyProduct 내 상품 상세 조회
func (s *productService) GetMyProduct(sellerID uint64, productID uint64) (*domain.ProductResponse, error) {
	product, err := s.repo.FindByID(productID)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// 소유자 확인
	if product.SellerID != sellerID {
		return nil, ErrProductForbidden
	}

	return product.ToResponse(), nil
}

// UpdateProduct 상품 수정
func (s *productService) UpdateProduct(sellerID uint64, productID uint64, req *domain.UpdateProductRequest) (*domain.ProductResponse, error) {
	// 상품 조회
	product, err := s.repo.FindByID(productID)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// 소유자 확인
	if product.SellerID != sellerID {
		return nil, ErrProductForbidden
	}

	// 슬러그 변경 시 중복 확인
	if req.Slug != nil && *req.Slug != product.Slug {
		available, err := s.repo.IsSlugAvailable(*req.Slug, productID)
		if err != nil {
			return nil, err
		}
		if !available {
			return nil, ErrSlugAlreadyExists
		}
	}

	// 업데이트할 필드 적용 (Phase 7: 입력 살균화)
	if req.Name != nil {
		if err := s.validator.ValidateProductName(*req.Name); err != nil {
			return nil, err
		}
		product.Name = s.sanitizer.SanitizeString(*req.Name)
	}
	if req.Slug != nil {
		if err := s.validator.ValidateSlug(*req.Slug); err != nil {
			return nil, err
		}
		product.Slug = *req.Slug
	}
	if req.Description != nil {
		if err := s.validator.ValidateProductDescription(*req.Description); err != nil {
			return nil, err
		}
		product.Description = s.sanitizer.SanitizeHTML(*req.Description)
	}
	if req.ShortDesc != nil {
		product.ShortDesc = s.sanitizer.SanitizeString(*req.ShortDesc)
	}
	if req.ProductType != nil {
		product.ProductType = domain.ProductType(*req.ProductType)
	}
	if req.Price != nil {
		if err := s.validator.ValidatePrice(*req.Price); err != nil {
			return nil, err
		}
		product.Price = *req.Price
	}
	if req.OriginalPrice != nil {
		product.OriginalPrice = req.OriginalPrice
	}
	if req.Currency != nil {
		product.Currency = *req.Currency
	}
	if req.StockQuantity != nil {
		product.StockQuantity = req.StockQuantity
	}
	if req.StockStatus != nil {
		product.StockStatus = domain.StockStatus(*req.StockStatus)
	}
	if req.DownloadLimit != nil {
		product.DownloadLimit = req.DownloadLimit
	}
	if req.DownloadExpiry != nil {
		product.DownloadExpiry = req.DownloadExpiry
	}
	if req.Status != nil {
		oldStatus := product.Status
		product.Status = domain.ProductStatus(*req.Status)
		// draft → published 전환 시 published_at 설정
		if oldStatus != domain.ProductStatusPublished && product.Status == domain.ProductStatusPublished {
			now := time.Now()
			product.PublishedAt = &now
		}
	}
	if req.Visibility != nil {
		product.Visibility = *req.Visibility
	}
	if req.Password != nil {
		product.Password = *req.Password
	}
	if req.FeaturedImage != nil {
		product.FeaturedImage = *req.FeaturedImage
	}

	if err := s.repo.Update(productID, product); err != nil {
		return nil, err
	}

	// 업데이트된 상품 다시 조회
	updated, err := s.repo.FindByID(productID)
	if err != nil {
		return nil, err
	}

	return updated.ToResponse(), nil
}

// DeleteProduct 상품 삭제
func (s *productService) DeleteProduct(sellerID uint64, productID uint64) error {
	product, err := s.repo.FindByID(productID)
	if err != nil {
		return ErrProductNotFound
	}

	// 소유자 확인
	if product.SellerID != sellerID {
		return ErrProductForbidden
	}

	return s.repo.Delete(productID)
}

// ListMyProducts 내 상품 목록 조회
func (s *productService) ListMyProducts(sellerID uint64, req *domain.ProductListRequest) ([]*domain.ProductResponse, *common.Meta, error) {
	// 기본값 설정
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 20
	}

	products, total, err := s.repo.ListBySeller(sellerID, req)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.ProductResponse, len(products))
	for i, p := range products {
		responses[i] = p.ToResponse()
	}

	meta := &common.Meta{
		Page:  req.Page,
		Limit: req.Limit,
		Total: total,
	}

	return responses, meta, nil
}

// GetShopProduct 공개 상품 상세 조회
func (s *productService) GetShopProduct(productID uint64) (*domain.ProductResponse, error) {
	product, err := s.repo.FindByID(productID)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// 공개 상품만 조회 가능
	if product.Status != domain.ProductStatusPublished || product.Visibility != "public" {
		return nil, ErrProductNotFound
	}

	// 조회수 증가 (비동기)
	go s.repo.IncrementViewCount(productID) //nolint:errcheck

	return product.ToResponse(), nil
}

// GetShopProductBySlug 슬러그로 공개 상품 조회
func (s *productService) GetShopProductBySlug(slug string) (*domain.ProductResponse, error) {
	product, err := s.repo.FindBySlug(slug)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// 공개 상품만 조회 가능
	if product.Status != domain.ProductStatusPublished || product.Visibility != "public" {
		return nil, ErrProductNotFound
	}

	// 조회수 증가 (비동기)
	go s.repo.IncrementViewCount(product.ID) //nolint:errcheck

	return product.ToResponse(), nil
}

// ListShopProducts 공개 상품 목록 조회
func (s *productService) ListShopProducts(req *domain.ProductListRequest) ([]*domain.ProductResponse, *common.Meta, error) {
	// 기본값 설정
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 20
	}

	products, total, err := s.repo.ListPublished(req)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.ProductResponse, len(products))
	for i, p := range products {
		responses[i] = p.ToResponse()
	}

	meta := &common.Meta{
		Page:  req.Page,
		Limit: req.Limit,
		Total: total,
	}

	return responses, meta, nil
}

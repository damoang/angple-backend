package repository

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// ProductRepository 상품 저장소 인터페이스
type ProductRepository interface {
	// 생성/수정/삭제
	Create(product *domain.Product) error
	Update(id uint64, product *domain.Product) error
	Delete(id uint64) error

	// 조회
	FindByID(id uint64) (*domain.Product, error)
	FindBySlug(slug string) (*domain.Product, error)
	FindByIDWithDeleted(id uint64) (*domain.Product, error)

	// 목록 조회
	ListBySeller(sellerID uint64, req *domain.ProductListRequest) ([]*domain.Product, int64, error)
	ListPublished(req *domain.ProductListRequest) ([]*domain.Product, int64, error)

	// 통계
	IncrementViewCount(id uint64) error
	IncrementSalesCount(id uint64, quantity int) error

	// 슬러그 유효성
	IsSlugAvailable(slug string, excludeID uint64) (bool, error)
}

// productRepository GORM 구현체
type productRepository struct {
	db *gorm.DB
}

// NewProductRepository 생성자
func NewProductRepository(db *gorm.DB) ProductRepository {
	return &productRepository{db: db}
}

// Create 상품 생성
func (r *productRepository) Create(product *domain.Product) error {
	now := time.Now()
	product.CreatedAt = now
	product.UpdatedAt = now

	// 슬러그 자동 생성
	if product.Slug == "" {
		product.Slug = r.generateSlug(product.Name)
	}

	// 슬러그 중복 확인 및 수정
	product.Slug = r.ensureUniqueSlug(product.Slug, 0)

	// 발행 시 published_at 설정
	if product.Status == domain.ProductStatusPublished && product.PublishedAt == nil {
		product.PublishedAt = &now
	}

	return r.db.Create(product).Error
}

// Update 상품 수정
func (r *productRepository) Update(id uint64, product *domain.Product) error {
	product.UpdatedAt = time.Now()

	// 발행 시 published_at 설정
	if product.Status == domain.ProductStatusPublished && product.PublishedAt == nil {
		now := time.Now()
		product.PublishedAt = &now
	}

	return r.db.Model(&domain.Product{}).Where("id = ?", id).Updates(product).Error
}

// Delete 상품 삭제 (소프트 삭제)
func (r *productRepository) Delete(id uint64) error {
	return r.db.Delete(&domain.Product{}, id).Error
}

// FindByID ID로 상품 조회
func (r *productRepository) FindByID(id uint64) (*domain.Product, error) {
	var product domain.Product
	err := r.db.Where("id = ?", id).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// FindBySlug 슬러그로 상품 조회
func (r *productRepository) FindBySlug(slug string) (*domain.Product, error) {
	var product domain.Product
	err := r.db.Where("slug = ?", slug).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// FindByIDWithDeleted 삭제된 상품 포함 조회
func (r *productRepository) FindByIDWithDeleted(id uint64) (*domain.Product, error) {
	var product domain.Product
	err := r.db.Unscoped().Where("id = ?", id).First(&product).Error
	if err != nil {
		return nil, err
	}
	return &product, nil
}

// ListBySeller 판매자별 상품 목록 조회
func (r *productRepository) ListBySeller(sellerID uint64, req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	return r.listProducts(req, func(db *gorm.DB) *gorm.DB {
		return db.Where("seller_id = ?", sellerID)
	})
}

// ListPublished 공개된 상품 목록 조회
func (r *productRepository) ListPublished(req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	return r.listProducts(req, func(db *gorm.DB) *gorm.DB {
		return db.Where("status = ?", domain.ProductStatusPublished).
			Where("visibility = ?", "public")
	})
}

// listProducts 공통 목록 조회 로직
func (r *productRepository) listProducts(req *domain.ProductListRequest, baseQuery func(*gorm.DB) *gorm.DB) ([]*domain.Product, int64, error) {
	var products []*domain.Product
	var total int64

	// 기본값 설정
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// 기본 쿼리
	query := r.db.Model(&domain.Product{})
	if baseQuery != nil {
		query = baseQuery(query)
	}

	// 상품 유형 필터
	if req.ProductType != "" {
		query = query.Where("product_type = ?", req.ProductType)
	}

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 검색
	if req.Search != "" {
		searchPattern := "%" + req.Search + "%"
		query = query.Where("name LIKE ? OR short_desc LIKE ?", searchPattern, searchPattern)
	}

	// 총 개수
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// 페이지네이션
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Find(&products).Error; err != nil {
		return nil, 0, err
	}

	return products, total, nil
}

// IncrementViewCount 조회수 증가
func (r *productRepository) IncrementViewCount(id uint64) error {
	return r.db.Model(&domain.Product{}).
		Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + ?", 1)).
		Error
}

// IncrementSalesCount 판매 수량 증가
func (r *productRepository) IncrementSalesCount(id uint64, quantity int) error {
	return r.db.Model(&domain.Product{}).
		Where("id = ?", id).
		UpdateColumn("sales_count", gorm.Expr("sales_count + ?", quantity)).
		Error
}

// IsSlugAvailable 슬러그 사용 가능 여부 확인
func (r *productRepository) IsSlugAvailable(slug string, excludeID uint64) (bool, error) {
	var count int64
	query := r.db.Model(&domain.Product{}).Where("slug = ?", slug)
	if excludeID > 0 {
		query = query.Where("id != ?", excludeID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

// generateSlug 상품명으로 슬러그 생성
func (r *productRepository) generateSlug(name string) string {
	// 소문자 변환
	slug := strings.ToLower(name)

	// 한글, 영문, 숫자만 허용
	reg := regexp.MustCompile(`[^a-z0-9가-힣]+`)
	slug = reg.ReplaceAllString(slug, "-")

	// 연속된 하이픈 제거
	reg = regexp.MustCompile(`-+`)
	slug = reg.ReplaceAllString(slug, "-")

	// 앞뒤 하이픈 제거
	slug = strings.Trim(slug, "-")

	// 최대 길이 제한
	if len(slug) > 200 {
		slug = slug[:200]
	}

	return slug
}

// ensureUniqueSlug 유니크 슬러그 생성
func (r *productRepository) ensureUniqueSlug(slug string, excludeID uint64) string {
	originalSlug := slug
	counter := 1

	for {
		available, err := r.IsSlugAvailable(slug, excludeID)
		if err != nil || available {
			break
		}
		slug = fmt.Sprintf("%s-%d", originalSlug, counter)
		counter++
	}

	return slug
}

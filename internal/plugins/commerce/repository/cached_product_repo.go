package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/redis/go-redis/v9"
)

// CacheConfig 캐시 설정
type CacheConfig struct {
	ProductDetailTTL time.Duration // 상품 상세 TTL (기본 10분)
	ProductListTTL   time.Duration // 상품 목록 TTL (기본 5분)
	KeyPrefix        string        // 캐시 키 접두사
}

// DefaultCacheConfig 기본 캐시 설정
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		ProductDetailTTL: 10 * time.Minute,
		ProductListTTL:   5 * time.Minute,
		KeyPrefix:        "commerce:product:",
	}
}

// CachedProductRepository 캐시가 적용된 상품 저장소
type CachedProductRepository struct {
	repo   ProductRepository
	redis  *redis.Client
	config *CacheConfig
}

// NewCachedProductRepository 캐시 적용 상품 저장소 생성
func NewCachedProductRepository(repo ProductRepository, redisClient *redis.Client, config *CacheConfig) ProductRepository {
	if config == nil {
		config = DefaultCacheConfig()
	}
	return &CachedProductRepository{
		repo:   repo,
		redis:  redisClient,
		config: config,
	}
}

// ============================================
// 캐시 키 생성 헬퍼
// ============================================

func (r *CachedProductRepository) keyProductByID(id uint64) string {
	return fmt.Sprintf("%sid:%d", r.config.KeyPrefix, id)
}

func (r *CachedProductRepository) keyProductBySlug(slug string) string {
	return fmt.Sprintf("%sslug:%s", r.config.KeyPrefix, slug)
}

func (r *CachedProductRepository) keyProductList(prefix string, req *domain.ProductListRequest) string {
	return fmt.Sprintf("%slist:%s:p%d:l%d:t%s:s%s:so%s:sb%s",
		r.config.KeyPrefix, prefix,
		req.Page, req.Limit, req.ProductType, req.Status, req.SortOrder, req.SortBy)
}

// ============================================
// 캐시 무효화 헬퍼
// ============================================

// InvalidateProduct 특정 상품 캐시 무효화
func (r *CachedProductRepository) InvalidateProduct(ctx context.Context, id uint64, slug string) error {
	keys := []string{
		r.keyProductByID(id),
	}
	if slug != "" {
		keys = append(keys, r.keyProductBySlug(slug))
	}

	// 목록 캐시도 무효화
	if err := r.invalidateListCache(ctx); err != nil {
		return err
	}

	return r.redis.Del(ctx, keys...).Err()
}

// invalidateListCache 목록 캐시 무효화
func (r *CachedProductRepository) invalidateListCache(ctx context.Context) error {
	pattern := fmt.Sprintf("%slist:*", r.config.KeyPrefix)
	return r.invalidatePattern(ctx, pattern)
}

// invalidatePattern 패턴 기반 캐시 무효화
func (r *CachedProductRepository) invalidatePattern(ctx context.Context, pattern string) error {
	var cursor uint64
	for {
		var keys []string
		var err error
		keys, cursor, err = r.redis.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := r.redis.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		if cursor == 0 {
			break
		}
	}
	return nil
}

// ============================================
// ProductRepository 인터페이스 구현
// ============================================

// Create 상품 생성 (캐시 무효화)
func (r *CachedProductRepository) Create(product *domain.Product) error {
	err := r.repo.Create(product)
	if err != nil {
		return err
	}

	// 목록 캐시 무효화
	ctx := context.Background()
	r.invalidateListCache(ctx)

	return nil
}

// Update 상품 수정 (캐시 무효화)
func (r *CachedProductRepository) Update(id uint64, product *domain.Product) error {
	// 기존 상품 조회 (슬러그 확인)
	oldProduct, _ := r.repo.FindByID(id)
	oldSlug := ""
	if oldProduct != nil {
		oldSlug = oldProduct.Slug
	}

	err := r.repo.Update(id, product)
	if err != nil {
		return err
	}

	// 캐시 무효화
	ctx := context.Background()
	r.InvalidateProduct(ctx, id, oldSlug)
	if product.Slug != "" && product.Slug != oldSlug {
		r.InvalidateProduct(ctx, id, product.Slug)
	}

	return nil
}

// Delete 상품 삭제 (캐시 무효화)
func (r *CachedProductRepository) Delete(id uint64) error {
	// 기존 상품 조회 (슬러그 확인)
	oldProduct, _ := r.repo.FindByID(id)
	oldSlug := ""
	if oldProduct != nil {
		oldSlug = oldProduct.Slug
	}

	err := r.repo.Delete(id)
	if err != nil {
		return err
	}

	// 캐시 무효화
	ctx := context.Background()
	r.InvalidateProduct(ctx, id, oldSlug)

	return nil
}

// FindByID ID로 상품 조회 (캐시 적용)
func (r *CachedProductRepository) FindByID(id uint64) (*domain.Product, error) {
	ctx := context.Background()
	cacheKey := r.keyProductByID(id)

	// 캐시 조회
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var product domain.Product
		if json.Unmarshal(cached, &product) == nil {
			return &product, nil
		}
	}

	// 캐시 미스 - DB 조회
	product, err := r.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	// 캐시 저장
	if data, err := json.Marshal(product); err == nil {
		r.redis.Set(ctx, cacheKey, data, r.config.ProductDetailTTL)
	}

	return product, nil
}

// FindBySlug 슬러그로 상품 조회 (캐시 적용)
func (r *CachedProductRepository) FindBySlug(slug string) (*domain.Product, error) {
	ctx := context.Background()
	cacheKey := r.keyProductBySlug(slug)

	// 캐시 조회
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var product domain.Product
		if json.Unmarshal(cached, &product) == nil {
			return &product, nil
		}
	}

	// 캐시 미스 - DB 조회
	product, err := r.repo.FindBySlug(slug)
	if err != nil {
		return nil, err
	}

	// 캐시 저장
	if data, err := json.Marshal(product); err == nil {
		r.redis.Set(ctx, cacheKey, data, r.config.ProductDetailTTL)
	}

	return product, nil
}

// FindByIDWithDeleted 삭제된 상품 포함 조회 (캐시 미적용)
func (r *CachedProductRepository) FindByIDWithDeleted(id uint64) (*domain.Product, error) {
	return r.repo.FindByIDWithDeleted(id)
}

// ListBySeller 판매자별 상품 목록 조회 (캐시 적용)
func (r *CachedProductRepository) ListBySeller(sellerID uint64, req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	ctx := context.Background()
	cacheKey := r.keyProductList(fmt.Sprintf("seller:%d", sellerID), req)

	// 캐시 조회
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var result struct {
			Products []*domain.Product `json:"products"`
			Total    int64             `json:"total"`
		}
		if json.Unmarshal(cached, &result) == nil {
			return result.Products, result.Total, nil
		}
	}

	// 캐시 미스 - DB 조회
	products, total, err := r.repo.ListBySeller(sellerID, req)
	if err != nil {
		return nil, 0, err
	}

	// 캐시 저장
	result := struct {
		Products []*domain.Product `json:"products"`
		Total    int64             `json:"total"`
	}{
		Products: products,
		Total:    total,
	}
	if data, err := json.Marshal(result); err == nil {
		r.redis.Set(ctx, cacheKey, data, r.config.ProductListTTL)
	}

	return products, total, nil
}

// ListPublished 공개 상품 목록 조회 (캐시 적용)
func (r *CachedProductRepository) ListPublished(req *domain.ProductListRequest) ([]*domain.Product, int64, error) {
	ctx := context.Background()
	cacheKey := r.keyProductList("published", req)

	// 캐시 조회
	cached, err := r.redis.Get(ctx, cacheKey).Bytes()
	if err == nil {
		var result struct {
			Products []*domain.Product `json:"products"`
			Total    int64             `json:"total"`
		}
		if json.Unmarshal(cached, &result) == nil {
			return result.Products, result.Total, nil
		}
	}

	// 캐시 미스 - DB 조회
	products, total, err := r.repo.ListPublished(req)
	if err != nil {
		return nil, 0, err
	}

	// 캐시 저장
	result := struct {
		Products []*domain.Product `json:"products"`
		Total    int64             `json:"total"`
	}{
		Products: products,
		Total:    total,
	}
	if data, err := json.Marshal(result); err == nil {
		r.redis.Set(ctx, cacheKey, data, r.config.ProductListTTL)
	}

	return products, total, nil
}

// IncrementViewCount 조회수 증가 (캐시 미적용 - 성능상 캐시 무효화 하지 않음)
func (r *CachedProductRepository) IncrementViewCount(id uint64) error {
	return r.repo.IncrementViewCount(id)
}

// IncrementSalesCount 판매 수량 증가 (캐시 무효화)
func (r *CachedProductRepository) IncrementSalesCount(id uint64, quantity int) error {
	err := r.repo.IncrementSalesCount(id, quantity)
	if err != nil {
		return err
	}

	// 판매량은 중요 지표이므로 캐시 무효화
	ctx := context.Background()
	product, _ := r.repo.FindByID(id)
	if product != nil {
		r.InvalidateProduct(ctx, id, product.Slug)
	}

	return nil
}

// IsSlugAvailable 슬러그 사용 가능 여부 확인 (캐시 미적용)
func (r *CachedProductRepository) IsSlugAvailable(slug string, excludeID uint64) (bool, error) {
	return r.repo.IsSlugAvailable(slug, excludeID)
}

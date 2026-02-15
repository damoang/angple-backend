package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	"github.com/redis/go-redis/v9"
)

const (
	galleryCacheTTL  = 5 * time.Minute  // 갤러리 캐시 5분
	searchCacheTTL   = 3 * time.Minute  // 검색 캐시 3분
	boardIDsCacheTTL = 10 * time.Minute // 게시판 ID 목록 캐시 10분
)

// GalleryService handles gallery and unified search
type GalleryService interface {
	GetGallery(boardID string, page, limit int) ([]*domain.GalleryItem, *common.Meta, error)
	GetGalleryAll(page, limit int) ([]*domain.GalleryItem, *common.Meta, error)
	UnifiedSearch(keyword string, page, limit int) ([]*domain.UnifiedSearchResult, *common.Meta, error)
}

type galleryService struct {
	repo  *repository.GalleryRepository
	redis *redis.Client
}

// NewGalleryService creates a new GalleryService
func NewGalleryService(repo *repository.GalleryRepository, redisClient *redis.Client) GalleryService {
	return &galleryService{repo: repo, redis: redisClient}
}

// GetGallery returns gallery posts for a specific board
func (s *galleryService) GetGallery(boardID string, page, limit int) ([]*domain.GalleryItem, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("gallery:%s:%d:%d", boardID, page, limit)

	// Try Redis cache
	if cached := s.getFromCache(cacheKey); cached != nil {
		var result struct {
			Items []*domain.GalleryItem `json:"items"`
			Meta  *common.Meta          `json:"meta"`
		}
		if err := json.Unmarshal(cached, &result); err == nil {
			return result.Items, result.Meta, nil
		}
	}

	items, total, err := s.repo.FindGalleryPosts(boardID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}

	// Cache result
	s.setToCache(cacheKey, galleryCacheTTL, map[string]interface{}{
		"items": items,
		"meta":  meta,
	})

	return items, meta, nil
}

// GetGalleryAll returns gallery posts across all boards
func (s *galleryService) GetGalleryAll(page, limit int) ([]*domain.GalleryItem, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("gallery:all:%d:%d", page, limit)

	// Try Redis cache
	if cached := s.getFromCache(cacheKey); cached != nil {
		var result struct {
			Items []*domain.GalleryItem `json:"items"`
			Meta  *common.Meta          `json:"meta"`
		}
		if err := json.Unmarshal(cached, &result); err == nil {
			return result.Items, result.Meta, nil
		}
	}

	boardIDs, err := s.getBoardIDs()
	if err != nil {
		return nil, nil, err
	}

	items, total, err := s.repo.FindGalleryAll(boardIDs, page, limit)
	if err != nil {
		return nil, nil, err
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}

	s.setToCache(cacheKey, galleryCacheTTL, map[string]interface{}{
		"items": items,
		"meta":  meta,
	})

	return items, meta, nil
}

// UnifiedSearch searches across all boards with caching
func (s *galleryService) UnifiedSearch(keyword string, page, limit int) ([]*domain.UnifiedSearchResult, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	cacheKey := fmt.Sprintf("search:%s:%d:%d", keyword, page, limit)

	// Try Redis cache (동시접속 1만명 대비, 동일 검색어 캐싱)
	if cached := s.getFromCache(cacheKey); cached != nil {
		var result struct {
			Items []*domain.UnifiedSearchResult `json:"items"`
			Meta  *common.Meta                  `json:"meta"`
		}
		if err := json.Unmarshal(cached, &result); err == nil {
			return result.Items, result.Meta, nil
		}
	}

	boardIDs, err := s.getBoardIDs()
	if err != nil {
		return nil, nil, err
	}

	results, total, err := s.repo.UnifiedSearch(boardIDs, keyword, page, limit)
	if err != nil {
		return nil, nil, err
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}

	s.setToCache(cacheKey, searchCacheTTL, map[string]interface{}{
		"items": results,
		"meta":  meta,
	})

	return results, meta, nil
}

// getBoardIDs returns all board IDs with caching
func (s *galleryService) getBoardIDs() ([]string, error) {
	cacheKey := "board_ids:active"

	if cached := s.getFromCache(cacheKey); cached != nil {
		var ids []string
		if err := json.Unmarshal(cached, &ids); err == nil {
			return ids, nil
		}
	}

	ids, err := s.repo.GetAllBoardIDs()
	if err != nil {
		return nil, err
	}

	s.setToCache(cacheKey, boardIDsCacheTTL, ids)
	return ids, nil
}

// getFromCache reads from Redis (returns nil if not available)
func (s *galleryService) getFromCache(key string) []byte {
	if s.redis == nil {
		return nil
	}
	val, err := s.redis.Get(context.Background(), key).Bytes()
	if err != nil {
		return nil
	}
	return val
}

// setToCache writes to Redis (ignores errors)
func (s *galleryService) setToCache(key string, ttl time.Duration, value interface{}) {
	if s.redis == nil {
		return
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	s.redis.Set(context.Background(), key, data, ttl) //nolint:errcheck
}

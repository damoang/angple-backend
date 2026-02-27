package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// TTL 상수 정의
const (
	TTLBoard   = 10 * time.Minute // 게시판 설정 (변경 빈도 낮음)
	TTLPopular = 5 * time.Minute  // 인기 게시글
	TTLSession = 30 * time.Minute // 세션
	TTLShort   = 1 * time.Minute  // 짧은 캐시 (실시간성 필요)
	TTLDefault = 5 * time.Minute  // 기본값
	TTLPosts   = 30 * time.Second // 게시글 목록 (자주 갱신)
	TTLNotices = 2 * time.Minute  // 공지사항
)

// 캐시 키 접두사
const (
	PrefixBoard   = "board:"
	PrefixPopular = "popular:"
	PrefixSession = "session:"
	PrefixUser    = "user:"
	PrefixPost    = "post:"
	PrefixPosts   = "posts:"
	PrefixNotices = "notices:"
)

// Service Redis 캐시 서비스 인터페이스
type Service interface {
	// 기본 캐시 연산
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
	Exists(ctx context.Context, key string) (bool, error)

	// 게시판 캐시
	GetBoard(ctx context.Context, boardID string) ([]byte, error)
	SetBoard(ctx context.Context, boardID string, data interface{}) error
	InvalidateBoard(ctx context.Context, boardID string) error
	InvalidateAllBoards(ctx context.Context) error

	// 인기 게시글 캐시
	GetPopularPosts(ctx context.Context, boardID string) ([]byte, error)
	SetPopularPosts(ctx context.Context, boardID string, data interface{}) error
	InvalidatePopularPosts(ctx context.Context, boardID string) error

	// 세션 캐시
	GetSession(ctx context.Context, sessionID string) ([]byte, error)
	SetSession(ctx context.Context, sessionID string, data interface{}) error
	DeleteSession(ctx context.Context, sessionID string) error
	ExtendSession(ctx context.Context, sessionID string) error

	// 사용자 캐시
	GetUser(ctx context.Context, userID string) ([]byte, error)
	SetUser(ctx context.Context, userID string, data interface{}) error
	InvalidateUser(ctx context.Context, userID string) error

	// 게시글 목록 캐시
	GetPosts(ctx context.Context, boardID string, page, limit int) ([]byte, error)
	SetPosts(ctx context.Context, boardID string, page, limit int, data interface{}) error
	InvalidatePosts(ctx context.Context, boardID string) error

	// 공지사항 캐시
	GetNotices(ctx context.Context, boardID string) ([]byte, error)
	SetNotices(ctx context.Context, boardID string, data interface{}) error
	InvalidateNotices(ctx context.Context, boardID string) error

	// 유틸리티
	IsAvailable() bool
	Ping(ctx context.Context) error
}

// redisCache Redis 기반 캐시 구현
type redisCache struct {
	client *redis.Client
}

// NewService 새로운 캐시 서비스 생성
func NewService(client *redis.Client) Service {
	return &redisCache{client: client}
}

// IsAvailable Redis 연결 가능 여부
func (c *redisCache) IsAvailable() bool {
	return c.client != nil
}

// Ping Redis 연결 테스트
func (c *redisCache) Ping(ctx context.Context) error {
	if c.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	return c.client.Ping(ctx).Err()
}

// Get 캐시에서 값 조회
func (c *redisCache) Get(ctx context.Context, key string, dest interface{}) error {
	if c.client == nil {
		return fmt.Errorf("redis not available")
	}

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}

	return json.Unmarshal(data, dest)
}

// Set 캐시에 값 저장
func (c *redisCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if c.client == nil {
		return nil // Redis 없으면 무시
	}

	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	return c.client.Set(ctx, key, data, ttl).Err()
}

// Delete 캐시 삭제
func (c *redisCache) Delete(ctx context.Context, keys ...string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

// Exists 캐시 존재 여부 확인
func (c *redisCache) Exists(ctx context.Context, key string) (bool, error) {
	if c.client == nil {
		return false, nil
	}
	n, err := c.client.Exists(ctx, key).Result()
	return n > 0, err
}

// ========================================
// 게시판 캐시
// ========================================

func (c *redisCache) boardKey(boardID string) string {
	return PrefixBoard + boardID
}

func (c *redisCache) GetBoard(ctx context.Context, boardID string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.boardKey(boardID)).Bytes()
}

func (c *redisCache) SetBoard(ctx context.Context, boardID string, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.boardKey(boardID), jsonData, TTLBoard).Err()
}

func (c *redisCache) InvalidateBoard(ctx context.Context, boardID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.boardKey(boardID)).Err()
}

func (c *redisCache) InvalidateAllBoards(ctx context.Context) error {
	if c.client == nil {
		return nil
	}
	return c.deleteByPattern(ctx, PrefixBoard+"*")
}

// ========================================
// 인기 게시글 캐시
// ========================================

func (c *redisCache) popularKey(boardID string) string {
	if boardID == "" {
		return PrefixPopular + "all"
	}
	return PrefixPopular + boardID
}

func (c *redisCache) GetPopularPosts(ctx context.Context, boardID string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.popularKey(boardID)).Bytes()
}

func (c *redisCache) SetPopularPosts(ctx context.Context, boardID string, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.popularKey(boardID), jsonData, TTLPopular).Err()
}

func (c *redisCache) InvalidatePopularPosts(ctx context.Context, boardID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.popularKey(boardID)).Err()
}

// ========================================
// 세션 캐시
// ========================================

func (c *redisCache) sessionKey(sessionID string) string {
	return PrefixSession + sessionID
}

func (c *redisCache) GetSession(ctx context.Context, sessionID string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.sessionKey(sessionID)).Bytes()
}

func (c *redisCache) SetSession(ctx context.Context, sessionID string, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.sessionKey(sessionID), jsonData, TTLSession).Err()
}

func (c *redisCache) DeleteSession(ctx context.Context, sessionID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.sessionKey(sessionID)).Err()
}

func (c *redisCache) ExtendSession(ctx context.Context, sessionID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Expire(ctx, c.sessionKey(sessionID), TTLSession).Err()
}

// ========================================
// 사용자 캐시
// ========================================

func (c *redisCache) userKey(userID string) string {
	return PrefixUser + userID
}

func (c *redisCache) GetUser(ctx context.Context, userID string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.userKey(userID)).Bytes()
}

func (c *redisCache) SetUser(ctx context.Context, userID string, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.userKey(userID), jsonData, TTLDefault).Err()
}

func (c *redisCache) InvalidateUser(ctx context.Context, userID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.userKey(userID)).Err()
}

// ========================================
// 내부 유틸리티
// ========================================

func (c *redisCache) deleteByPattern(ctx context.Context, pattern string) error {
	iter := c.client.Scan(ctx, 0, pattern, 100).Iterator()
	for iter.Next(ctx) {
		if err := c.client.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}
	return iter.Err()
}

// ========================================
// 게시글 목록 캐시
// ========================================

func (c *redisCache) postsKey(boardID string, page, limit int) string {
	return fmt.Sprintf("%s%s:%d:%d", PrefixPosts, boardID, page, limit)
}

func (c *redisCache) GetPosts(ctx context.Context, boardID string, page, limit int) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.postsKey(boardID, page, limit)).Bytes()
}

func (c *redisCache) SetPosts(ctx context.Context, boardID string, page, limit int, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.postsKey(boardID, page, limit), jsonData, TTLPosts).Err()
}

func (c *redisCache) InvalidatePosts(ctx context.Context, boardID string) error {
	if c.client == nil {
		return nil
	}
	return c.deleteByPattern(ctx, PrefixPosts+boardID+":*")
}

// ========================================
// 공지사항 캐시
// ========================================

func (c *redisCache) noticesKey(boardID string) string {
	return PrefixNotices + boardID
}

func (c *redisCache) GetNotices(ctx context.Context, boardID string) ([]byte, error) {
	if c.client == nil {
		return nil, fmt.Errorf("redis not available")
	}
	return c.client.Get(ctx, c.noticesKey(boardID)).Bytes()
}

func (c *redisCache) SetNotices(ctx context.Context, boardID string, data interface{}) error {
	if c.client == nil {
		return nil
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, c.noticesKey(boardID), jsonData, TTLNotices).Err()
}

func (c *redisCache) InvalidateNotices(ctx context.Context, boardID string) error {
	if c.client == nil {
		return nil
	}
	return c.client.Del(ctx, c.noticesKey(boardID)).Err()
}

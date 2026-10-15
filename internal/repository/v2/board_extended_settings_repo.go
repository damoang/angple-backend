package v2

import (
	"errors"
	"sync"
	"time"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// BoardExtendedSettingsRepository handles extended settings data access
type BoardExtendedSettingsRepository interface {
	FindByBoardSlug(slug string) (*v2.V2BoardExtendedSettings, error)
	Upsert(settings *v2.V2BoardExtendedSettings) error
}

type boardExtendedSettingsRepository struct {
	db *gorm.DB
}

// NewBoardExtendedSettingsRepository creates a new BoardExtendedSettingsRepository
func NewBoardExtendedSettingsRepository(db *gorm.DB) BoardExtendedSettingsRepository {
	return &boardExtendedSettingsRepository{db: db}
}

// 게시판 확장 설정 프로세스 캐시.
//
// FindByBoardSlug 결과를 게시판별로 boardExtSettingsTTL 동안 들고 있는다. 「행 없음 → 기본값」 결과도 캐시하고,
// 조회 오류는 캐시하지 않는다. 이 프로세스의 쓰기(Upsert, 행운 관리자 게시판 설정 저장) 뒤에는 그 게시판 키를
// 지운다. 다른 프로세스나 다른 서비스가 같은 테이블에 쓴 값은 최대 boardExtSettingsTTL 뒤에 반영된다.
//
// 캐시 키에는 *gorm.Config 를 함께 넣는다 — 같은 gorm.Open 에서 나온 핸들(세션·트랜잭션 포함)은 같은 Config 를
// 공유하므로 운영에서는 저장소끼리 같은 캐시를 보고, 서로 다른 DB 를 여는 테스트끼리는 섞이지 않는다.

// boardExtSettingsTTL 은 게시판 확장 설정 프로세스 캐시의 수명이다.
const boardExtSettingsTTL = 60 * time.Second

// boardExtSettingsMaxEntries 는 캐시 항목 수 상한이다. 넘으면 통째로 비운다(게시판 수는 이보다 훨씬 적다).
const boardExtSettingsMaxEntries = 2000

// boardExtSettingsNow 는 캐시 만료 판단에 쓰는 시계다(테스트에서 바꾼다).
var boardExtSettingsNow = time.Now

type boardExtSettingsKey struct {
	cfg  *gorm.Config
	slug string
}

type boardExtSettingsEntry struct {
	val     v2.V2BoardExtendedSettings
	expires time.Time
}

// boardExtSettingsCache 는 게시판 확장 설정의 프로세스 캐시다. gen 은 무효화마다 오르는 세대 카운터로,
// 조회 도중 무효화가 끼면 그 조회 결과를 캐시에 넣지 않게 한다.
type boardExtSettingsCache struct {
	mu      sync.Mutex
	gen     uint64
	entries map[boardExtSettingsKey]boardExtSettingsEntry
}

var boardExtSettings = &boardExtSettingsCache{entries: make(map[boardExtSettingsKey]boardExtSettingsEntry)}

// get 은 만료 전 항목의 값 복사본과 현재 세대를 돌려준다.
func (c *boardExtSettingsCache) get(k boardExtSettingsKey, now time.Time) (v2.V2BoardExtendedSettings, bool, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[k]
	if !ok || !now.Before(e.expires) {
		return v2.V2BoardExtendedSettings{}, false, c.gen
	}
	return e.val, true, c.gen
}

// put 은 조회 시작 때 본 세대(gen)가 그대로일 때만 값을 넣는다.
func (c *boardExtSettingsCache) put(k boardExtSettingsKey, val v2.V2BoardExtendedSettings, gen uint64, now time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.gen != gen {
		return
	}
	if len(c.entries) >= boardExtSettingsMaxEntries {
		c.entries = make(map[boardExtSettingsKey]boardExtSettingsEntry)
	}
	c.entries[k] = boardExtSettingsEntry{val: val, expires: now.Add(boardExtSettingsTTL)}
}

// forget 은 게시판 키들을 지우고 세대를 올린다.
func (c *boardExtSettingsCache) forget(cfg *gorm.Config, slugs ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	for _, s := range slugs {
		delete(c.entries, boardExtSettingsKey{cfg: cfg, slug: s})
	}
}

func (c *boardExtSettingsCache) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.gen++
	c.entries = make(map[boardExtSettingsKey]boardExtSettingsEntry)
}

// invalidateBoardExtSettings 는 db 와 같은 연결에서 쓴 게시판들의 확장 설정 캐시를 지운다.
// 쓰기(트랜잭션이면 커밋)가 끝난 뒤 부른다.
func invalidateBoardExtSettings(db *gorm.DB, slugs ...string) {
	if db == nil {
		return
	}
	boardExtSettings.forget(db.Config, slugs...)
}

// FindByBoardSlug returns extended settings by board slug.
// 결과는 프로세스 캐시에서 boardExtSettingsTTL 동안 재사용되며, 호출자는 늘 자기 복사본을 받는다.
func (r *boardExtendedSettingsRepository) FindByBoardSlug(slug string) (*v2.V2BoardExtendedSettings, error) {
	key := boardExtSettingsKey{cfg: r.db.Config, slug: slug}
	now := boardExtSettingsNow()
	cached, ok, gen := boardExtSettings.get(key, now)
	if ok {
		// 필드가 문자열·시각뿐이라 값 복사가 곧 깊은 복사다.
		return &cached, nil
	}

	var settings v2.V2BoardExtendedSettings
	err := r.db.Where("board_id = ?", slug).First(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Return empty settings if not found
		settings = v2.V2BoardExtendedSettings{
			BoardID:  slug,
			Settings: "{}",
		}
	} else if err != nil {
		return &settings, err
	}
	boardExtSettings.put(key, settings, gen, now) // 값으로 담기므로 아래 반환값과 캐시는 서로 독립이다
	return &settings, nil
}

// Upsert creates or updates extended settings for a board
func (r *boardExtendedSettingsRepository) Upsert(settings *v2.V2BoardExtendedSettings) error {
	// 실패해도 지운다 — 일부만 반영됐을 수 있고, 지워서 손해 볼 것은 다음 조회 1회뿐이다.
	defer invalidateBoardExtSettings(r.db, settings.BoardID)

	var existing v2.V2BoardExtendedSettings
	err := r.db.Where("board_id = ?", settings.BoardID).First(&existing).Error

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(settings).Error
	} else if err != nil {
		return err
	}

	return r.db.Save(settings).Error
}

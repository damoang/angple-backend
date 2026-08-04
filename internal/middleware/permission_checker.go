package middleware

import (
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// boardCacheEntry holds a cached board with expiry
type boardCacheEntry struct {
	board  *v2.V2Board
	expiry time.Time
}

// DBBoardPermissionChecker checks board permissions using database
// In-memory cache with 60s TTL eliminates N+1 queries for permission checks
type DBBoardPermissionChecker struct {
	boardRepo v2repo.BoardRepository
	cache     map[string]boardCacheEntry
	mu        sync.RWMutex
}

const boardCacheTTL = 60 * time.Second

// NewDBBoardPermissionChecker creates a new permission checker
func NewDBBoardPermissionChecker(boardRepo v2repo.BoardRepository) *DBBoardPermissionChecker {
	return &DBBoardPermissionChecker{
		boardRepo: boardRepo,
		cache:     make(map[string]boardCacheEntry),
	}
}

// getBoard returns a board from cache or DB
func (c *DBBoardPermissionChecker) getBoard(slug string) (*v2.V2Board, error) {
	now := time.Now()

	// Check cache (read lock)
	c.mu.RLock()
	entry, ok := c.cache[slug]
	c.mu.RUnlock()
	if ok && now.Before(entry.expiry) {
		return entry.board, nil
	}

	// Cache miss — fetch from DB
	board, err := c.boardRepo.FindBySlug(slug)
	if err != nil {
		return nil, err
	}

	// Store in cache (write lock)
	c.mu.Lock()
	c.cache[slug] = boardCacheEntry{board: board, expiry: now.Add(boardCacheTTL)}
	// Evict expired entries if cache grows beyond 200 boards
	if len(c.cache) > 200 {
		for k, v := range c.cache {
			if now.After(v.expiry) {
				delete(c.cache, k)
			}
		}
	}
	c.mu.Unlock()

	return board, nil
}

func (c *DBBoardPermissionChecker) CanList(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.ListLevel), nil
}

func (c *DBBoardPermissionChecker) CanRead(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.ReadLevel), nil
}

func (c *DBBoardPermissionChecker) CanWrite(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	if common.IsBoardWriteBlockedByLevel(memberLevel, boardSlug) {
		return false, nil
	}
	return memberLevel >= int(board.WriteLevel), nil
}

func (c *DBBoardPermissionChecker) CanReply(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	if common.IsBoardWriteBlockedByLevel(memberLevel, boardSlug) {
		return false, nil
	}
	return memberLevel >= int(board.ReplyLevel), nil
}

func (c *DBBoardPermissionChecker) CanComment(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	if common.IsBoardWriteBlockedByLevel(memberLevel, boardSlug) {
		return false, nil
	}
	return memberLevel >= int(board.CommentLevel), nil
}

func (c *DBBoardPermissionChecker) CanUpload(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	// GetAllPermissions 의 CanUpload 와 답이 달라지면 안 된다. 같은 질문에 두 API 가
	// 다른 답을 내면 어느 쪽이 맞는지 아무도 모르게 된다.
	if common.IsBoardWriteBlockedByLevel(memberLevel, boardSlug) {
		return false, nil
	}
	return memberLevel >= int(board.UploadLevel), nil
}

func (c *DBBoardPermissionChecker) CanDownload(boardSlug string, memberLevel int) (bool, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return false, err
	}
	return memberLevel >= int(board.DownloadLevel), nil
}

func (c *DBBoardPermissionChecker) GetRequiredLevel(boardSlug string, action string) int {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return 1
	}
	switch action {
	case "list":
		return int(board.ListLevel)
	case "read":
		return int(board.ReadLevel)
	case "write":
		return int(board.WriteLevel)
	case "reply":
		return int(board.ReplyLevel)
	case "comment":
		return int(board.CommentLevel)
	case "upload":
		return int(board.UploadLevel)
	case "download":
		return int(board.DownloadLevel)
	default:
		return 1
	}
}

func (c *DBBoardPermissionChecker) GetAllPermissions(boardSlug string, memberLevel int) (*BoardPermissions, error) {
	board, err := c.getBoard(boardSlug)
	if err != nil {
		return nil, err
	}
	// 등급별 게시판 제한 (광고앙 = 직접홍보 전용).
	// ⛔ 읽기·목록·다운로드는 막지 않는다. 쓰기 계열(글·답글·댓글·첨부)만 막는다 —
	//    광고앙도 커뮤니티를 볼 수는 있어야 한다.
	blocked := common.IsBoardWriteBlockedByLevel(memberLevel, boardSlug)
	return &BoardPermissions{
		CanList:     memberLevel >= int(board.ListLevel),
		CanRead:     memberLevel >= int(board.ReadLevel),
		CanWrite:    !blocked && memberLevel >= int(board.WriteLevel),
		CanReply:    !blocked && memberLevel >= int(board.ReplyLevel),
		CanComment:  !blocked && memberLevel >= int(board.CommentLevel),
		CanUpload:   !blocked && memberLevel >= int(board.UploadLevel),
		CanDownload: memberLevel >= int(board.DownloadLevel),
	}, nil
}

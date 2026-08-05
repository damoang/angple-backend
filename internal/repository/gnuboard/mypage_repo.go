package gnuboard

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

const maxDBConcurrency = 10

// MyPageRepository provides access to user's posts, comments, and stats across g5_write_* tables
type MyPageRepository interface {
	FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error)
	// 검색 전용 경로 — 구현·근거는 mypage_search.go 참조.
	// ⛔ 검색이 없을 때는 위의 fan-out 을 그대로 쓴다. 기존 목록의 순서·건수를 건드리지 않기 위함.
	SearchPostsByMember(mbID, q string, page, limit int) ([]gnuboard.MyPost, int64, error)
	SearchCommentsByMember(mbID, q string, page, limit int) ([]gnuboard.MyCommentRow, int64, error)
	FindLikedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	GetBoardStats(mbID string) ([]gnuboard.BoardStat, error)
	FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error)
	FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error)
	// FindDisciplinedIDs returns the set of wr_ids (within the given board) that are
	// referenced as discipline evidence in g5_na_singo. 글·댓글 공용 (#12751/#12908).
	FindDisciplinedIDs(boardID string, wrIDs []int) (map[int]bool, error)
	GetSearchableBoards() ([]searchableBoard, error)
	// FindRecentAcrossBoards 는 검색가능 게시판별 최신 perBoard개를 모은 시간순 후보 풀을 반환한다.
	// 보드별 캡·인터리브(다양성)는 핸들러에서 적용. cursor 는 보드slug→wr_id 워터마크. excludeMbIDs 는 차단.
	FindRecentAcrossBoards(perBoard int, cursor map[string]int, excludeMbIDs []string) ([]gnuboard.FeedPost, error)
}

type searchableBoard struct {
	BoTable   string `gorm:"column:bo_table"`
	BoSubject string `gorm:"column:bo_subject"`
}

// searchableBoardsCache caches the searchable boards list (5 min TTL)
var searchableBoardsCache struct {
	sync.RWMutex
	boards    []searchableBoard
	expiresAt time.Time
}

const boardsCacheTTL = 5 * time.Minute

type myPageRepository struct {
	db        *gorm.DB
	boardRepo BoardRepository
}

// NewMyPageRepository creates a new MyPageRepository
func NewMyPageRepository(db *gorm.DB, boardRepo BoardRepository) MyPageRepository {
	return &myPageRepository{db: db, boardRepo: boardRepo}
}

// getActiveBoards returns board IDs that actually have write tables
func (r *myPageRepository) getActiveBoards() []string {
	boards, err := r.boardRepo.FindAll()
	if err != nil {
		return nil
	}
	// Batch check all tables at once (1 query instead of N)
	tableNames := make([]string, len(boards))
	for i, b := range boards {
		tableNames[i] = fmt.Sprintf("g5_write_%s", b.BoTable)
	}
	var existingTables []string
	r.db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ?", tableNames).Scan(&existingTables)

	existSet := make(map[string]bool, len(existingTables))
	for _, t := range existingTables {
		existSet[t] = true
	}
	var ids []string
	for _, b := range boards {
		if existSet[fmt.Sprintf("g5_write_%s", b.BoTable)] {
			ids = append(ids, b.BoTable)
		}
	}
	return ids
}

// largeBoardID is the board whose g5_write_* table is too large for per-board full scans.
// For this board, member_activity_feed (covering index) is used instead.
const largeBoardID = "free"

// FindPostsByMember returns posts written by the member across all boards.
// Uses parallel per-board queries instead of UNION ALL for better DB performance.
// g5_write_free (600K+ rows) is handled via member_activity_feed to avoid 3.6s queries.
func (r *myPageRepository) FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	perTable := page * limit

	// Phase A: parallel COUNT per board
	type boardCount struct {
		boardID string
		count   int64
	}
	var (
		muCounts   sync.Mutex
		counts     []boardCount
		totalCount int64
	)

	g := errgroup.Group{}
	g.SetLimit(maxDBConcurrency)
	for _, boardID := range boards {
		g.Go(func() error {
			var cnt int64
			if boardID == largeBoardID {
				// Use member_activity_feed covering index instead of scanning 600K-row table
				r.db.Raw("SELECT COUNT(*) FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 1", mbID, largeBoardID).Scan(&cnt)
			} else {
				table := fmt.Sprintf("g5_write_%s", boardID)
				r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0", table), mbID).Scan(&cnt)
			}
			if cnt > 0 {
				muCounts.Lock()
				counts = append(counts, boardCount{boardID: boardID, count: cnt})
				totalCount += cnt
				muCounts.Unlock()
			}
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil (errors skipped per board)
	g.Wait()

	if totalCount == 0 {
		return nil, 0, nil
	}

	// Phase B: parallel data fetch from boards that have results
	var (
		mu    sync.Mutex
		posts []gnuboard.MyPost
	)

	g2 := errgroup.Group{}
	g2.SetLimit(maxDBConcurrency)
	for _, bc := range counts {
		g2.Go(func() error {
			var rows []gnuboard.MyPost
			if bc.boardID == largeBoardID {
				// Step 1: Get write_ids from activity_feed (covering index scan, ~1ms)
				var writeIDs []int
				r.db.Raw(
					"SELECT write_id FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 1 ORDER BY source_created_at DESC LIMIT ?",
					mbID, largeBoardID, perTable,
				).Scan(&writeIDs)
				if len(writeIDs) > 0 {
					// Step 2: Batch PK lookup for full post data
					//
					// ⛔ 피드가 준 ID 를 그대로 믿지 않는다. mb_id·wr_is_comment 를 정본에서
					//    다시 확인한다(다른 게시판 경로는 원래 이렇게 한다 — 아래 else 참고).
					//    피드에는 v2_posts 기반 행이 섞여 있는데(write_table='v2_posts'),
					//    그 write_id 는 g5_write_* 의 wr_id 와 무관한 번호다. 검증 없이 조회하면
					//    번호가 우연히 겹치는 남의 글·댓글이 내 글로 나온다(#13109).
					r.db.Raw(
						fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id, wr_deleted_at AS deleted_at FROM `g5_write_%s` WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0 ORDER BY wr_datetime DESC", largeBoardID, largeBoardID),
						writeIDs, mbID,
					).Scan(&rows)
				}
			} else {
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id, wr_deleted_at AS deleted_at FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 ORDER BY wr_datetime DESC LIMIT %d", bc.boardID, table, perTable),
					mbID,
				).Scan(&rows)
			}
			if len(rows) > 0 {
				mu.Lock()
				posts = append(posts, rows...)
				mu.Unlock()
			}
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil
	g2.Wait()

	// Sort and paginate in Go
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].WrDatetime.After(posts[j].WrDatetime)
	})

	offset := (page - 1) * limit
	if offset >= len(posts) {
		return nil, totalCount, nil
	}
	end := offset + limit
	if end > len(posts) {
		end = len(posts)
	}
	return posts[offset:end], totalCount, nil
}

// FindCommentsByMember returns comments written by the member with parent post titles.
// Uses parallel per-board queries instead of UNION ALL.
// g5_write_free (600K+ rows) is handled via member_activity_feed to avoid 3.4s LEFT JOIN queries.
func (r *myPageRepository) FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	perTable := page * limit

	// Phase A: parallel COUNT per board
	type boardCount struct {
		boardID string
		count   int64
	}
	var (
		muCounts   sync.Mutex
		counts     []boardCount
		totalCount int64
	)

	g := errgroup.Group{}
	g.SetLimit(maxDBConcurrency)
	for _, boardID := range boards {
		g.Go(func() error {
			var cnt int64
			if boardID == largeBoardID {
				// Use member_activity_feed covering index instead of scanning 600K-row table
				r.db.Raw("SELECT COUNT(*) FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 2", mbID, largeBoardID).Scan(&cnt)
			} else {
				table := fmt.Sprintf("g5_write_%s", boardID)
				r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1", table), mbID).Scan(&cnt)
			}
			if cnt > 0 {
				muCounts.Lock()
				counts = append(counts, boardCount{boardID: boardID, count: cnt})
				totalCount += cnt
				muCounts.Unlock()
			}
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil
	g.Wait()

	if totalCount == 0 {
		return nil, 0, nil
	}

	// Phase B: parallel data fetch
	var (
		mu       sync.Mutex
		comments []gnuboard.MyCommentRow
	)

	g2 := errgroup.Group{}
	g2.SetLimit(maxDBConcurrency)
	for _, bc := range counts {
		g2.Go(func() error {
			var rows []gnuboard.MyCommentRow
			if bc.boardID == largeBoardID {
				// Step 1: Get write_ids + parent_title from activity_feed (covering index, ~1ms)
				type commentRef struct {
					WriteID     int    `gorm:"column:write_id"`
					ParentTitle string `gorm:"column:parent_title"`
				}
				var refs []commentRef
				r.db.Raw(
					"SELECT write_id, COALESCE(parent_title, '') as parent_title FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 2 ORDER BY source_created_at DESC LIMIT ?",
					mbID, largeBoardID, perTable,
				).Scan(&refs)
				if len(refs) > 0 {
					writeIDs := make([]int, len(refs))
					titleMap := make(map[int]string, len(refs))
					for i, ref := range refs {
						writeIDs[i] = ref.WriteID
						titleMap[ref.WriteID] = ref.ParentTitle
					}
					// Step 2: Batch PK lookup for comment data (no LEFT JOIN needed)
					r.db.Raw(
						fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, '' as post_title, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `g5_write_%s` c LEFT JOIN `g5_write_%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.wr_id IN ? AND c.wr_is_comment = 1 ORDER BY c.wr_datetime DESC", largeBoardID, largeBoardID, largeBoardID),
						writeIDs,
					).Scan(&rows)
					// Fill parent titles from activity_feed (avoids expensive LEFT JOIN)
					for i := range rows {
						if t, ok := titleMap[rows[i].WrID]; ok {
							rows[i].PostTitle = t
						}
					}
				}
			} else {
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, CASE WHEN p.wr_deleted_at IS NOT NULL THEN '[삭제된 글]' ELSE COALESCE(p.wr_subject, '') END as post_title, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 ORDER BY c.wr_datetime DESC LIMIT %d", bc.boardID, table, table, perTable),
					mbID,
				).Scan(&rows)
			}
			if len(rows) > 0 {
				mu.Lock()
				comments = append(comments, rows...)
				mu.Unlock()
			}
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil
	g2.Wait()

	// Sort and paginate in Go
	sort.Slice(comments, func(i, j int) bool {
		return comments[i].WrDatetime.After(comments[j].WrDatetime)
	})

	offset := (page - 1) * limit
	if offset >= len(comments) {
		return nil, totalCount, nil
	}
	end := offset + limit
	if end > len(comments) {
		end = len(comments)
	}
	return comments[offset:end], totalCount, nil
}

// FindLikedPostsByMember returns posts that the member liked (from g5_board_good)
func (r *myPageRepository) FindLikedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	// Count total liked posts
	var total int64
	if err := r.db.Table("g5_board_good").
		Where("mb_id = ? AND bg_flag = 'good'", mbID).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return nil, 0, nil
	}

	// Get liked post references
	offset := (page - 1) * limit
	type likedRef struct {
		BoTable    string `gorm:"column:bo_table"`
		WrID       int    `gorm:"column:wr_id"`
		BgDatetime string `gorm:"column:bg_datetime"`
	}
	var refs []likedRef
	if err := r.db.Table("g5_board_good").
		Select("bo_table, wr_id, bg_datetime").
		Where("mb_id = ? AND bg_flag = 'good'", mbID).
		Order("bg_datetime DESC").
		Offset(offset).
		Limit(limit).
		Scan(&refs).Error; err != nil {
		return nil, 0, err
	}

	// Group refs by board for batch queries
	boardPosts := make(map[string][]int)
	refOrder := make([]string, 0, len(refs)) // preserve order
	for _, ref := range refs {
		key := fmt.Sprintf("%s:%d", ref.BoTable, ref.WrID)
		refOrder = append(refOrder, key)
		boardPosts[ref.BoTable] = append(boardPosts[ref.BoTable], ref.WrID)
	}

	// Fetch post details per board in parallel
	var (
		mu      sync.Mutex
		postMap = make(map[string]gnuboard.MyPost)
	)

	g := errgroup.Group{}
	g.SetLimit(maxDBConcurrency)
	for boardID, wrIDs := range boardPosts {
		g.Go(func() error {
			table := fmt.Sprintf("g5_write_%s", boardID)
			var posts []gnuboard.MyPost
			if err := r.db.Raw(
				fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE wr_id IN ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", boardID, table),
				wrIDs,
			).Scan(&posts).Error; err != nil {
				return nil // skip boards with errors
			}
			mu.Lock()
			for _, p := range posts {
				key := fmt.Sprintf("%s:%d", boardID, p.WrID)
				postMap[key] = p
			}
			mu.Unlock()
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil
	g.Wait()

	// Build result in original order
	var result []gnuboard.MyPost
	for _, key := range refOrder {
		if post, ok := postMap[key]; ok {
			result = append(result, post)
		}
	}

	return result, total, nil
}

// GetBoardStats returns post/comment counts per board for the member.
// Uses parallel per-board queries instead of UNION ALL.
func (r *myPageRepository) GetBoardStats(mbID string) ([]gnuboard.BoardStat, error) {
	boards, err := r.boardRepo.FindAll()
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 {
		return nil, nil
	}

	tableNames := make([]string, len(boards))
	boardMap := make(map[string]string)
	for i, b := range boards {
		tableName := fmt.Sprintf("g5_write_%s", b.BoTable)
		tableNames[i] = tableName
		boardMap[tableName] = b.BoSubject
	}

	var existingTables []string
	r.db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ?", tableNames).Scan(&existingTables)
	if len(existingTables) == 0 {
		return nil, nil
	}

	type boardCount struct {
		BoardID      string
		PostCount    int64
		CommentCount int64
	}

	var (
		mu     sync.Mutex
		counts []boardCount
	)

	g := errgroup.Group{}
	g.SetLimit(maxDBConcurrency)
	for _, tableName := range existingTables {
		boardID := strings.TrimPrefix(tableName, "g5_write_")
		g.Go(func() error {
			var postCount, commentCount int64
			r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", tableName), mbID).Scan(&postCount)
			r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", tableName), mbID).Scan(&commentCount)
			if postCount > 0 || commentCount > 0 {
				mu.Lock()
				counts = append(counts, boardCount{BoardID: boardID, PostCount: postCount, CommentCount: commentCount})
				mu.Unlock()
			}
			return nil
		})
	}
	//nolint:errcheck // all goroutines return nil
	g.Wait()

	var stats []gnuboard.BoardStat
	for _, c := range counts {
		tableName := fmt.Sprintf("g5_write_%s", c.BoardID)
		stats = append(stats, gnuboard.BoardStat{
			BoardID:      c.BoardID,
			BoardName:    boardMap[tableName],
			PostCount:    c.PostCount,
			CommentCount: c.CommentCount,
		})
	}
	return stats, nil
}

// GetSearchableBoards returns boards with bo_use_search=1 that have existing write tables.
// Results are cached in memory for 5 minutes.
func (r *myPageRepository) GetSearchableBoards() ([]searchableBoard, error) {
	// Check memory cache first
	searchableBoardsCache.RLock()
	if time.Now().Before(searchableBoardsCache.expiresAt) && searchableBoardsCache.boards != nil {
		cached := searchableBoardsCache.boards
		searchableBoardsCache.RUnlock()
		return cached, nil
	}
	searchableBoardsCache.RUnlock()

	// Cache miss — query DB
	boards, err := r.boardRepo.FindAll()
	if err != nil {
		return nil, err
	}
	if len(boards) == 0 {
		return nil, nil
	}

	tableNames := make([]string, len(boards))
	boardMap := make(map[string]*gnuboard.G5Board, len(boards))
	for i, b := range boards {
		tableName := fmt.Sprintf("g5_write_%s", b.BoTable)
		tableNames[i] = tableName
		boardMap[tableName] = b
	}

	var existingTables []string
	r.db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ?", tableNames).Scan(&existingTables)

	var result []searchableBoard
	for _, t := range existingTables {
		b, ok := boardMap[t]
		if !ok || b.BoUseSearch != 1 {
			continue
		}
		result = append(result, searchableBoard{
			BoTable:   b.BoTable,
			BoSubject: b.BoSubject,
		})
	}

	// Store in cache
	searchableBoardsCache.Lock()
	searchableBoardsCache.boards = result
	searchableBoardsCache.expiresAt = time.Now().Add(boardsCacheTTL)
	searchableBoardsCache.Unlock()

	return result, nil
}

// FindRecentAcrossBoards returns a chronological cross-board timeline of recent posts.
// UNION ALL per searchable board with a PK (wr_id) range scan — no wr_datetime ORDER BY /
// OFFSET inside subqueries, so the 670만-row free 보드도 안전(PK range). 병합만 wr_datetime DESC.
func (r *myPageRepository) FindRecentAcrossBoards(perBoard int, cursor map[string]int, excludeMbIDs []string) ([]gnuboard.FeedPost, error) {
	if perBoard <= 0 || perBoard > 20 {
		perBoard = 8
	}
	boards, err := r.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		return nil, err
	}

	var unions []string
	var args []interface{}
	for _, b := range boards {
		table := fmt.Sprintf("g5_write_%s", b.BoTable)
		// mypage 의 검증된 WHERE(secret/lock) + 삭제 제외(제로데이트 호환).
		where := "wr_is_comment = 0 AND (wr_option NOT LIKE '%secret%' OR wr_option IS NULL)" +
			" AND (wr_7 IS NULL OR wr_7 != 'lock')" +
			" AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')"
		if wm, ok := cursor[b.BoTable]; ok && wm > 0 {
			where += fmt.Sprintf(" AND wr_id < %d", wm)
		}
		if len(excludeMbIDs) > 0 {
			where += " AND mb_id NOT IN ?"
			args = append(args, excludeMbIDs)
		}
		// 보드별 최신 perBoard개. LEFT(wr_content,1000) 로 전송량 제한(발췌 140자엔 충분).
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_subject, LEFT(wr_content, 1000) AS wr_content, wr_datetime, wr_10, wr_hit, wr_good, wr_comment, mb_id, wr_name, wr_option, '%s' AS board_id FROM `%s` WHERE %s ORDER BY wr_id DESC LIMIT %d)",
			b.BoTable, table, where, perBoard))
	}

	// 각 보드의 최신 perBoard개를 모아 시간순 후보 풀로 반환(안전 상한 600). 보드별 캡·인터리브는 핸들러에서.
	sql := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT 600",
		strings.Join(unions, " UNION ALL "))

	var rows []gnuboard.FeedPost
	if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

// 피드 후보를 몇 배로 뽑을지. 오염 행이 걸러져도 limit 을 채우기 위한 여유분이다.
// write_table 조건으로 v2_posts 행을 후보 단계에서 걸러낸 뒤라 남는 오탈락은
// 삭제·비밀글·잠김 정도로 적다. 여유분 3배면 충분하다.
const activityCandidateFactor = 3

// 테이블명에 끼워 넣을 슬러그 검증. board_id 는 피드 테이블에서 온 값이라
// 정본(g5_board)을 거치지 않았으므로 그대로 SQL 에 넣지 않는다.
var activityBoardSlugRe = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// searchableBoardSet 은 bo_use_search=1 보드의 슬러그 집합을 돌려준다.
// 피드 후보의 board_id 는 정본(g5_board)을 거치지 않은 값이라, 비검색 보드가
// 후보에 섞여도 여기서 걸러진다 (#13174 — is_public 완화로 새로 열린 경로 차단).
// 조회 실패 시 nil 을 돌려주고 호출부는 필터를 생략한다(기존 동작 유지 — fail-open 이
// 아니라 "종전과 동일" 이다. 이 함수 도입 전에는 이 필터 자체가 없었다).
func (r *myPageRepository) searchableBoardSet() map[string]bool {
	boards, err := r.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		return nil
	}
	set := make(map[string]bool, len(boards))
	for _, b := range boards {
		set[b.BoTable] = true
	}
	return set
}

// verifyActivityPosts 는 피드가 준 후보 중 정본에 실재하는 것만 남긴다.
//
// 게시판마다 테이블이 다르므로(g5_write_*) 보드별로 묶어 한 번씩 확인한다.
// 후보가 limit*3 개(기본 15)라 보드 수는 많아야 몇 개이고, 조회는 전부 PK IN 이다.
//
// 확인 조건은 정본 UNION 경로(아래 fallback)와 같게 맞춘다:
//
//	본인 글이고 · 댓글이 아니고 · 비밀글이 아니고 · 신고로 잠기지 않은 것.
//
// 삭제글은 걸러내지 않는다 — 자리표시자([삭제된 게시물])로 표시된다 (#13174).
// 비밀글은 삭제 여부와 무관하게 제외한다(존재 자체 비노출).
//
// 한 보드 조회가 실패해도 그 보드만 건너뛴다(컬럼 구성이 다른 보드가 있다).
func (r *myPageRepository) verifyActivityPosts(
	mbID string, candidates []gnuboard.ActivityPost, limit int,
) []gnuboard.ActivityPost {
	searchable := r.searchableBoardSet()
	byBoard := make(map[string][]int)
	for _, c := range candidates {
		if c.BoardID == "" {
			continue
		}
		if searchable != nil && !searchable[c.BoardID] {
			continue
		}
		byBoard[c.BoardID] = append(byBoard[c.BoardID], c.WrID)
	}

	confirmed := make(map[string]map[int]*time.Time, len(byBoard))
	for boardID, ids := range byBoard {
		if !activityBoardSlugRe.MatchString(boardID) {
			continue
		}
		// 삭제 여부도 정본에서 가져온다. 피드의 is_deleted 는 뒤처질 수 있고,
		// 실제로 뒤처져 있었다: free 만 641건이 "피드는 살아있음 / 정본은 삭제됨" 이다.
		// 제보자(#13109)가 바로 이 경우라, 지운 글이 프로필에 살아있는 것처럼 남아 있었다.
		type verifiedRow struct {
			WrID        int        `gorm:"column:wr_id"`
			WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
		}
		var okRows []verifiedRow
		// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
		q := fmt.Sprintf(
			"SELECT wr_id, wr_deleted_at FROM `g5_write_%s` WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0"+
				" AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL)"+
				" AND (wr_7 IS NULL OR wr_7 != 'lock')", boardID)
		if err := r.db.Raw(q, ids, mbID).Scan(&okRows).Error; err != nil {
			continue
		}
		m := make(map[int]*time.Time, len(okRows))
		for _, row := range okRows {
			m[row.WrID] = row.WrDeletedAt
		}
		confirmed[boardID] = m
	}

	out := make([]gnuboard.ActivityPost, 0, limit)
	for _, c := range candidates {
		if len(out) >= limit {
			break
		}
		m, ok := confirmed[c.BoardID]
		if !ok {
			continue
		}
		deletedAt, found := m[c.WrID]
		if !found {
			continue
		}
		// 피드 캐시가 아니라 정본의 삭제 시각을 싣는다.
		c.DeletedAt = deletedAt
		// #13174: 삭제글 제목은 서버에서 비운다. 피드에 원제가 캐시돼 있어
		// 여기서 지우지 않으면 자리표시자 옆으로 원제가 새어 나간다.
		if deletedAt != nil {
			c.WrSubject = ""
		}
		out = append(out, c)
	}
	return out
}

// verifyActivityComments 는 verifyActivityPosts 의 댓글판이다 (#13174).
//
// 피드의 is_public 은 자체삭제·부모삭제·부모비밀글·비검색보드가 한 비트로 뭉개져
// 있어(멤버 activity sync 의 cascade), 후보 완화 후의 판정은 전적으로 정본이 한다:
//
//	본인 댓글이고 · 부모가 비밀글이 아니고 · 부모가 신고로 잠기지 않은 것.
//
// 삭제된 댓글([삭제된 댓글])과 부모가 삭제된 생존 댓글([삭제된 게시물] 배지)은
// 남긴다 — 13103 확정 정책("글이 삭제돼도 댓글 스레드는 유지")의 활동 피드판.
// 비밀글 부모의 댓글은 삭제 여부와 무관하게 제외된다(존재 자체 비노출).
func (r *myPageRepository) verifyActivityComments(
	mbID string, candidates []gnuboard.ActivityComment, limit int,
) []gnuboard.ActivityComment {
	searchable := r.searchableBoardSet()
	byBoard := make(map[string][]int)
	for _, c := range candidates {
		if c.BoardID == "" {
			continue
		}
		if searchable != nil && !searchable[c.BoardID] {
			continue
		}
		byBoard[c.BoardID] = append(byBoard[c.BoardID], c.WrID)
	}

	type verifiedComment struct {
		WrID            int        `gorm:"column:wr_id"`
		WrDeletedAt     *time.Time `gorm:"column:wr_deleted_at"`
		ParentDeletedAt *time.Time `gorm:"column:parent_deleted_at"`
	}
	confirmed := make(map[string]map[int]verifiedComment, len(byBoard))
	for boardID, ids := range byBoard {
		if !activityBoardSlugRe.MatchString(boardID) {
			continue
		}
		var okRows []verifiedComment
		// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
		q := fmt.Sprintf(
			"SELECT c.wr_id, c.wr_deleted_at, p.wr_deleted_at AS parent_deleted_at"+
				" FROM `g5_write_%s` c INNER JOIN `g5_write_%s` p"+
				" ON p.wr_id = c.wr_parent AND p.wr_is_comment = 0"+
				" WHERE c.wr_id IN ? AND c.mb_id = ? AND c.wr_is_comment = 1"+
				" AND (p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL)"+
				" AND (p.wr_7 IS NULL OR p.wr_7 != 'lock')", boardID, boardID)
		if err := r.db.Raw(q, ids, mbID).Scan(&okRows).Error; err != nil {
			continue
		}
		m := make(map[int]verifiedComment, len(okRows))
		for _, row := range okRows {
			m[row.WrID] = row
		}
		confirmed[boardID] = m
	}

	out := make([]gnuboard.ActivityComment, 0, limit)
	for _, c := range candidates {
		if len(out) >= limit {
			break
		}
		m, ok := confirmed[c.BoardID]
		if !ok {
			continue
		}
		row, found := m[c.WrID]
		if !found {
			continue
		}
		// 피드 캐시가 아니라 정본의 삭제 시각을 싣는다.
		c.DeletedAt = row.WrDeletedAt
		c.ParentDeletedAt = row.ParentDeletedAt
		// #13174: 삭제 댓글 원문은 서버에서 비운다(피드에 미리보기가 캐시돼 있다).
		if row.WrDeletedAt != nil {
			c.WrContent = ""
		}
		out = append(out, c)
	}
	return out
}

// FindPublicPostsByMember returns recent public posts by a member.
// Uses UNION ALL with per-subquery LIMIT for efficiency.
// Each subquery leverages mb_id index + PK ordering.
func (r *myPageRepository) FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error) {
	var posts []gnuboard.ActivityPost
	// 피드는 "후보 목록"이지 정답이 아니다. 제목까지 피드에 캐시돼 있어, 그대로 쓰면
	// 정본을 한 번도 보지 않고 화면을 그리게 된다. 실제로 그래서 남의 댓글이 내 글로
	// 떴고, 댓글 번호를 글 주소로 링크해 404 가 났다(#13109).
	// → 후보를 넉넉히 뽑은 뒤 정본으로 확인된 것만 남긴다.
	// #13174: 삭제된 글도 자리표시자([삭제된 게시물])로 나가야 하므로 후보에 포함한다.
	// 삭제 행은 is_public=0 이라, 공개 분기와 삭제 분기를 UNION ALL 로 나눠 뽑는다.
	// ⛔ 단일 쿼리 `(is_public=1 OR is_deleted=1)` 로 풀지 말 것 — OR 는 인덱스
	//    등호 프리픽스를 깨서 다작 회원(수만 행)에서 filesort 가 난다. 분기별로는
	//    기존 인덱스의 등호 연속이 유지된다.
	// 삭제 시각·비밀글 제외는 후보 단계가 아니라 정본 검증(verifyActivityPosts)이 정답이다.
	var candidates []gnuboard.ActivityPost
	if err := r.db.Raw(
		`SELECT * FROM (
		    (SELECT write_id AS wr_id,
		            COALESCE(title, '') AS wr_subject,
		            source_created_at AS wr_datetime,
		            board_id,
		            id AS feed_id
		       FROM member_activity_feed
		      WHERE member_id = ?
		        AND activity_type = 1
		        AND is_public = 1
		        AND write_table = CONCAT('g5_write_', board_id)
		      ORDER BY source_created_at DESC, id DESC
		      LIMIT ?)
		    UNION ALL
		    (SELECT write_id AS wr_id,
		            COALESCE(title, '') AS wr_subject,
		            source_created_at AS wr_datetime,
		            board_id,
		            id AS feed_id
		       FROM member_activity_feed
		      WHERE member_id = ?
		        AND activity_type = 1
		        AND is_deleted = 1
		        AND write_table = CONCAT('g5_write_', board_id)
		      ORDER BY source_created_at DESC, id DESC
		      LIMIT ?)
		 ) AS t
		 ORDER BY wr_datetime DESC, feed_id DESC
		 LIMIT ?`,
		mbID, limit*activityCandidateFactor,
		mbID, limit*activityCandidateFactor,
		limit*activityCandidateFactor,
	).Scan(&candidates).Error; err == nil {
		// ⛔ 후보가 0건이어도 폴백으로 가지 않는다. 글이 하나도 없는 회원이
		//    61.5%(37,905명)인데, 그들을 121개 서브쿼리 UNION 으로 보내면
		//    게시글 상세마다 그 쿼리가 돈다(작성자 패널은 최고 트래픽 경로다).
		if len(candidates) == 0 {
			return nil, nil
		}
		posts = r.verifyActivityPosts(mbID, candidates, limit)
		if len(posts) > 0 {
			return posts, nil
		}
		// 후보는 있는데 확인된 게 0건일 때만 정본 UNION 으로 떨어진다.
	}

	boards, err := r.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		return nil, err
	}

	var unions []string
	var args []interface{}
	for _, b := range boards {
		table := fmt.Sprintf("g5_write_%s", b.BoTable)
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_subject, wr_datetime, '%s' as board_id, wr_deleted_at AS deleted_at FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL) AND (wr_7 IS NULL OR wr_7 != 'lock') ORDER BY wr_id DESC LIMIT %d)",
			b.BoTable, table, limit))
		args = append(args, mbID)
	}

	// ⛔ 보드가 섞이므로 wr_id 로 정렬하면 안 된다. wr_id 는 보드마다 범위가 달라
	//    (free 최대 681만 vs qa 8만) free 글이 날짜와 무관하게 항상 앞에 온다.
	sql := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ?", strings.Join(unions, " UNION ALL "))
	args = append(args, limit)

	if err := r.db.Raw(sql, args...).Scan(&posts).Error; err != nil {
		return nil, err
	}
	// #13174: 삭제글 원제는 어느 경로에서도 나가면 안 된다 (verify 경로와 동일).
	for i := range posts {
		if posts[i].DeletedAt != nil {
			posts[i].WrSubject = ""
		}
	}
	return posts, nil
}

// FindPublicCommentsByMember returns recent public comments by a member.
// Uses UNION ALL + INNER JOIN to filter out comments on secret/locked/deleted parent posts.
func (r *myPageRepository) FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error) {
	// #13174: 종전엔 is_public=1 로 좁혀 삭제 댓글은 물론, **삭제된 글에 달린 살아있는
	// 댓글**까지 통째로 빠졌다(부모글 sync 가 산하 댓글 is_public 을 cascade 로 내림).
	// 후보를 두 분기로 완화한다:
	//   ① is_deleted=0 — is_public 무조건. 부모삭제 생존 댓글이 여기 들어온다.
	//      부모 비밀글·비검색 보드 댓글도 섞이지만, 판정은 정본 검증이 한다
	//      (verifyActivityComments — 피드의 is_public 은 원인 구분이 안 되는 비트다).
	//   ② is_deleted=1 — 삭제된 댓글([삭제된 댓글] 자리표시자).
	// ⛔ 단일 OR 금지(인덱스 프리픽스 파괴) — 글 후보 쿼리와 같은 이유.
	var comments []gnuboard.ActivityComment
	var candidates []gnuboard.ActivityComment
	if err := r.db.Raw(
		`SELECT * FROM (
		    (SELECT write_id AS wr_id,
		            COALESCE(content_preview, '') AS wr_content,
		            COALESCE(parent_write_id, 0) AS wr_parent,
		            source_created_at AS wr_datetime,
		            board_id,
		            id AS feed_id
		       FROM member_activity_feed
		      WHERE member_id = ?
		        AND activity_type = 2
		        AND is_deleted = 0
		        AND write_table = CONCAT('g5_write_', board_id)
		      ORDER BY source_created_at DESC, id DESC
		      LIMIT ?)
		    UNION ALL
		    (SELECT write_id AS wr_id,
		            COALESCE(content_preview, '') AS wr_content,
		            COALESCE(parent_write_id, 0) AS wr_parent,
		            source_created_at AS wr_datetime,
		            board_id,
		            id AS feed_id
		       FROM member_activity_feed
		      WHERE member_id = ?
		        AND activity_type = 2
		        AND is_deleted = 1
		        AND write_table = CONCAT('g5_write_', board_id)
		      ORDER BY source_created_at DESC, id DESC
		      LIMIT ?)
		 ) AS t
		 ORDER BY wr_datetime DESC, feed_id DESC
		 LIMIT ?`,
		mbID, limit*activityCandidateFactor,
		mbID, limit*activityCandidateFactor,
		limit*activityCandidateFactor,
	).Scan(&candidates).Error; err == nil {
		// 글 경로(:683)와 같은 이유로 후보 0건이면 폴백으로 가지 않는다.
		if len(candidates) == 0 {
			return nil, nil
		}
		comments = r.verifyActivityComments(mbID, candidates, limit)
		if len(comments) > 0 {
			return comments, nil
		}
		// 후보는 있는데 확인된 게 0건일 때만 정본 UNION 으로 떨어진다.
	}

	boards, err := r.GetSearchableBoards()
	if err != nil || len(boards) == 0 {
		return nil, err
	}

	var unions []string
	var args []interface{}
	for _, b := range boards {
		table := fmt.Sprintf("g5_write_%s", b.BoTable)
		unions = append(unions, fmt.Sprintf(
			"(SELECT c.wr_id, c.wr_content, c.wr_parent, c.wr_datetime, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `%s` c INNER JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 AND (p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL) AND (p.wr_7 IS NULL OR p.wr_7 != 'lock') WHERE c.mb_id = ? AND c.wr_is_comment = 1 ORDER BY c.wr_id DESC LIMIT %d)",
			b.BoTable, table, table, limit))
		args = append(args, mbID)
	}

	// ⛔ 보드가 섞이므로 wr_id 로 정렬하면 안 된다. wr_id 는 보드마다 범위가 달라
	//    (free 최대 681만 vs qa 8만) free 글이 날짜와 무관하게 항상 앞에 온다.
	sql := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ?", strings.Join(unions, " UNION ALL "))
	args = append(args, limit)

	if err := r.db.Raw(sql, args...).Scan(&comments).Error; err != nil {
		return nil, err
	}
	// #13174: 삭제 댓글 원문은 어느 경로에서도 나가면 안 된다 (verify 경로와 동일).
	for i := range comments {
		if comments[i].DeletedAt != nil {
			comments[i].WrContent = ""
		}
	}
	return comments, nil
}

// FindDisciplinedIDs returns wr_ids (within boardID) that are referenced as
// discipline evidence (g5_na_singo.discipline_log_id IS NOT NULL). 글/댓글 공용
// (sg_id 는 글·댓글 wr_id 를 동일 컬럼으로 담음). 프로필 최근 글·댓글 마스킹에 사용.
func (r *myPageRepository) FindDisciplinedIDs(boardID string, wrIDs []int) (map[int]bool, error) {
	result := make(map[int]bool)
	if boardID == "" || len(wrIDs) == 0 {
		return result, nil
	}
	var ids []int
	err := r.db.Raw(
		"SELECT DISTINCT sg_id FROM g5_na_singo WHERE sg_table = ? AND sg_id IN ? AND discipline_log_id IS NOT NULL",
		boardID, wrIDs,
	).Scan(&ids).Error
	if err != nil {
		return result, err
	}
	for _, id := range ids {
		result[id] = true
	}
	return result, nil
}

package gnuboard

import (
	"fmt"
	"log"
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
	// 본인이 삭제한 글·댓글 — 작성자에게만 보이는 별도 목록 (bug/13341, mypage_deleted.go)
	FindDeletedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	FindDeletedCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error)
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
	// CountCommentReplies 는 주어진 댓글들(wr_id)에 달린 답글(대댓글) 수를 반환한다 (makeang/88).
	// 그누보드 댓글 트리: 같은 wr_comment 그룹에서 내 wr_comment_reply 로 시작하고 더 긴 코드 = 내 답글.
	CountCommentReplies(boardID string, wrIDs []int) (map[int]int, error)
	GetSearchableBoards() ([]searchableBoard, error)
	// FindRecentAcrossBoards 는 검색가능 게시판별 최신 perBoard개를 모은 시간순 후보 풀을 반환한다.
	// 보드별 캡·인터리브(다양성)는 핸들러에서 적용. cursor 는 보드slug→wr_id 워터마크. excludeMbIDs 는 차단.
	FindRecentAcrossBoards(perBoard int, cursor map[string]int, excludeMbIDs []string) ([]gnuboard.FeedPost, error)
	// FindHotAcrossBoards 는 최근 hours 시간 내 공감(wr_good>=1) 글을 공감순으로 모은
	// 크로스보드 핫 피드다(GET /api/v2/feed/hot). 두 번째 반환값은 has_more.
	// 보드당 최근 2000글(PK 역순)만 후보로 본다 — 구현·근거는 hot_feed_repo.go 참조.
	FindHotAcrossBoards(hours, limit, offset int, excludeMbIDs []string) ([]gnuboard.FeedPost, bool, error)
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

// activeBoardsCache 는 getActiveBoards 결과를 통째로 캐시한다.
//
// ⛔ 이 함수는 호출마다 쿼리를 **두 개** 돌린다(g5_board 전량 + information_schema).
//
//	2026-08-18 실측: 각각 630만 회 / 19,238초 + 22,778초 = 합계 42,016초.
//	information_schema 는 Aurora 에서 특히 비싸다.
//
// ⭐ 결과가 바뀌는 조건은 **게시판 생성·삭제뿐**이다. 글이 아무리 늘어도 안 바뀐다.
//
//	그래서 통째로 캐시해도 안전하다. 같은 파일 계열의 boardByIDCache 와 같은 방식.
var activeBoardsCache struct {
	sync.RWMutex
	ids       []string
	expiresAt time.Time
}

// 게시판 추가 후 최대 이만큼 목록에 안 나타난다. 게시판 생성은 드물어 5분이면 충분하다.
const activeBoardsCacheTTL = 5 * time.Minute

// getActiveBoards returns board IDs that actually have write tables
func (r *myPageRepository) getActiveBoards() []string {
	now := time.Now()

	activeBoardsCache.RLock()
	if activeBoardsCache.ids != nil && now.Before(activeBoardsCache.expiresAt) {
		// ⛔ 캐시된 슬라이스를 그대로 돌려주면 호출자가 append/정렬로 오염시킬 수 있다.
		//    복사본을 준다.
		out := make([]string, len(activeBoardsCache.ids))
		copy(out, activeBoardsCache.ids)
		activeBoardsCache.RUnlock()
		return out
	}
	activeBoardsCache.RUnlock()

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

	// ⛔ 빈 결과는 캐시하지 않는다. DB 일시 장애로 빈 목록이 나왔을 때
	//    그 상태가 TTL 동안 굳어 마이페이지가 통째로 비어 보인다.
	if len(ids) > 0 {
		activeBoardsCache.Lock()
		activeBoardsCache.ids = ids
		activeBoardsCache.expiresAt = time.Now().Add(activeBoardsCacheTTL)
		activeBoardsCache.Unlock()
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
				r.db.Raw("SELECT COUNT(*) FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 1 AND is_deleted = 0", mbID, largeBoardID).Scan(&cnt)
			} else {
				table := fmt.Sprintf("g5_write_%s", boardID)
				r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", table), mbID).Scan(&cnt)
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
					"SELECT write_id FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 1 AND is_deleted = 0 ORDER BY source_created_at DESC LIMIT ?",
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
						fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id, wr_deleted_at AS deleted_at FROM `g5_write_%s` WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL ORDER BY wr_datetime DESC", largeBoardID, largeBoardID),
						writeIDs, mbID,
					).Scan(&rows)
				}
			} else {
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id, wr_deleted_at AS deleted_at FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL ORDER BY wr_datetime DESC LIMIT %d", bc.boardID, table, perTable),
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
// bug/13675 (2026-08-22): idx_mb_comment(mb_id, wr_is_comment) 추가 후 정본 g5_write_free
// LEFT JOIN 이 77~337ms 로 측정돼 피드 우회를 걷어냈다. 피드(activity_type=2)는 최근 삭제분이
// source_created_at 정렬 앞머리를 통째로 차지해, LIMIT perTable 로 긁으면 생존 댓글이 뒤로 밀려
// 뒷페이지가 비는 페이지네이션 버그가 있었다(총계=정본 vs 목록=피드 불일치).
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
				// bug/13518: member_activity_feed.is_deleted 는 댓글 삭제 경로가
				// 동기화하지 않아 낡아 있다(웹/레거시 댓글 삭제는 wr_deleted_at 만
				// UPDATE 하고 피드를 건드리지 않는다). 그래서 삭제된 댓글이 피드에는
				// 살아있는 것처럼 남아 "남은 댓글" 수가 부풀려졌다(제보 회원 기준
				// 피드 2409 vs 정본 920). "남은 댓글" 수는 피드를 믿지 말고 정본
				// g5_write_free 에서 직접 센다 — 작성자 프로필의 생존 수와 일치한다.
				// idx_mb_comment(mb_id, wr_is_comment) 로 회원 댓글 행만 스캔하므로
				// 600K/7M-행 풀스캔이 아니다(바운드된 COUNT). 예측(wr_deleted_at IS NULL)은
				// 같은 함수의 다른 보드 COUNT·free Phase B 조회와 동일해 목록과 수가 맞는다.
				// 게시글 경로(activity_type=1)는 피드 is_deleted 가 동기화돼 있어 손대지 않는다.
				table := fmt.Sprintf("g5_write_%s", largeBoardID)
				r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", table), mbID).Scan(&cnt)
			} else {
				table := fmt.Sprintf("g5_write_%s", boardID)
				r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", table), mbID).Scan(&cnt)
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
				// bug/13675: 피드(activity_type=2)는 최근 삭제분이 source_created_at
				// 정렬 앞머리를 통째로 차지해, LIMIT perTable 로 긁으면 생존 댓글이
				// 뒤로 밀려 뒷페이지가 비었다(대량 삭제 회원일수록 심함). count 와
				// 동일하게 정본 g5_write_free 에서 직접 조회한다(작은 보드 분기와 동일).
				// idx_mb_comment(mb_id, wr_is_comment) 로 바운드돼 77~337ms(2026-08-22 측정).
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, CASE WHEN p.wr_deleted_at IS NOT NULL THEN '[삭제된 글]' ELSE COALESCE(p.wr_subject, '') END as post_title, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_datetime DESC LIMIT %d", bc.boardID, table, table, perTable),
					mbID,
				).Scan(&rows)
			} else {
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, CASE WHEN p.wr_deleted_at IS NOT NULL THEN '[삭제된 글]' ELSE COALESCE(p.wr_subject, '') END as post_title, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_datetime DESC LIMIT %d", bc.boardID, table, table, perTable),
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
	// ⛔ (nil, nil) 을 돌려주지 않는다. 이 함수의 반환값은 **허용 목록**이라
	//    "비었다"가 호출부에서 "제한 없음"으로 읽힌다. 게시판이 정상적으로 0개일 수는 없으므로
	//    빈 목록은 결과가 아니라 조회 이상이다.
	if len(boards) == 0 {
		return nil, fmt.Errorf("searchable boards: board list is empty")
	}

	tableNames := make([]string, len(boards))
	boardMap := make(map[string]*gnuboard.G5Board, len(boards))
	for i, b := range boards {
		tableName := fmt.Sprintf("g5_write_%s", b.BoTable)
		tableNames[i] = tableName
		boardMap[tableName] = b
	}

	// ⛔ 이 조회의 에러를 버리면 안 된다. 실패하면 existingTables 가 비고 result 가 nil 이 되어
	//    조용히 (nil, nil) 로 나갔다 — 「검색 가능한 게시판이 하나도 없다」로 읽히는 값이다.
	//    같은 형태가 getActiveBoards·GetBoardStats 에도 있으나 그쪽은 허용 목록이 아니라
	//    이 변경의 범위 밖으로 둔다.
	var existingTables []string
	if err := r.db.Raw(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ?",
		tableNames,
	).Scan(&existingTables).Error; err != nil {
		return nil, fmt.Errorf("searchable boards: table existence check failed: %w", err)
	}

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

	// ⛔ 빈 결과는 캐시하지도, 반환하지도 않는다. 위와 같은 이유다.
	if len(result) == 0 {
		return nil, fmt.Errorf("searchable boards: no searchable board resolved")
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
	// ⚠️ perBoard 는 보드별 UNION 시절의 파라미터다. 크로스보드 단일 스캔으로 바뀌면서
	//    쓰이지 않는다(후보는 항상 feedCandidatePool). 시그니처는 호출부 호환을 위해 유지한다.
	_ = perBoard

	// 1단계: 후보를 member_activity_feed 에서 뽑는다(인덱스 레인지 스캔 1회).
	type cand struct {
		WriteID int    `gorm:"column:write_id"`
		BoardID string `gorm:"column:board_id"`
	}
	var cands []cand
	q := r.db.Table("member_activity_feed").
		Select("write_id, board_id").
		Where("activity_type = 1 AND is_public = 1 AND is_deleted = 0")
	if len(cursor) > 0 {
		// 기존 커서는 보드별 wr_id 다. 크로스보드 시간순에서는 보드별로 그 이하만 남긴다.
		var ors []string
		var oargs []interface{}
		for b, wm := range cursor {
			if wm <= 0 || !activityBoardSlugRe.MatchString(b) {
				continue
			}
			ors = append(ors, "(board_id = ? AND write_id < ?)")
			oargs = append(oargs, b, wm)
		}
		if len(ors) > 0 {
			q = q.Where("("+strings.Join(ors, " OR ")+") OR board_id NOT IN ?", append(oargs, cursorBoards(cursor))...)
		}
	}
	if err := q.Order("source_created_at DESC").Limit(feedCandidatePool).Scan(&cands).Error; err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, nil
	}

	// 2단계: 등장 보드만 PK-IN 으로 **정본에서** 읽는다.
	//
	// ⛔ feed 테이블을 그대로 내보내지 않는 이유가 둘이다.
	//    ① feed 에는 wr_7(신고잠금) 컬럼 자체가 없다. 잠긴 글을 거를 방법이 없다.
	//    ② feed 의 is_deleted 는 비동기라 뒤처진다 — 2026-08-21 실측으로 free 한 곳에서만
	//       **23,414건**이 「피드는 살아있음 / 정본은 삭제됨」이었다.
	//    최신순 상위 600건에는 마침 0건이었지만 그건 운이지 설계가 아니다. 신고잠금은
	//    글이 올라온 직후에도 걸리므로 상위 구간에 들어올 수 있다.
	//
	// ⭐ 그래서 feed 는 **후보 선정에만** 쓰고 표시 데이터는 전부 정본에서 가져온다.
	//    wr_10 처럼 feed 에 없는 확장 필드가 누락되는 문제도 함께 사라진다.
	// ⛔ fail-closed. 보드 목록을 못 얻으면 **필터 없이 진행하지 않는다.**
	//    feed 에는 검색 대상이 아닌 보드가 29개 섞여 있고
	//    그중 adm(1,670글)·advertiser(755)·temp(753)·archive(432) 는 **읽기 레벨 10(관리자 전용)**,
	//    disciplinelog·truthroom·claim·angreport 는 징계·소명 기록이다.
	//    ⭐ 피드가 잠깐 안 뜨는 것과 관리자 게시판이 노출되는 것 중에는 전자가 낫다.
	searchable, err := r.searchableBoardSet()
	if err != nil {
		return nil, err
	}
	byBoard := make(map[string][]int)
	order := make([]cand, 0, len(cands))
	for _, c := range cands {
		if c.BoardID == "" || !activityBoardSlugRe.MatchString(c.BoardID) {
			continue
		}
		if !searchable[c.BoardID] {
			continue
		}
		byBoard[c.BoardID] = append(byBoard[c.BoardID], c.WriteID)
		order = append(order, c)
	}

	found := make(map[string]map[int]gnuboard.FeedPost, len(byBoard))
	for boardID, ids := range byBoard {
		where := "wr_id IN ? AND wr_is_comment = 0" +
			" AND (wr_option NOT LIKE '%secret%' OR wr_option IS NULL)" +
			" AND (wr_7 IS NULL OR wr_7 != 'lock')" +
			" AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')"
		args := []interface{}{ids}
		if len(excludeMbIDs) > 0 {
			where += " AND mb_id NOT IN ?"
			args = append(args, excludeMbIDs)
		}
		// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
		sql := fmt.Sprintf(
			"SELECT wr_id, wr_subject, LEFT(wr_content, 1000) AS wr_content, wr_datetime, wr_10,"+
				" wr_hit, wr_good, wr_comment, mb_id, wr_name, wr_option, '%s' AS board_id"+
				" FROM `g5_write_%s` WHERE %s", boardID, boardID, where)
		var rows []gnuboard.FeedPost
		if err := r.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
			// 한 보드가 실패해도 피드 전체를 죽이지 않는다(기존 UNION 은 통째로 실패했다).
			// ⛔ 다만 조용히 넘기면 한 보드가 영구히 피드에서 사라져도 아무도 모른다.
			log.Printf("[feed] board %s verify failed, skipped: %v", boardID, err)
			continue
		}
		m := make(map[int]gnuboard.FeedPost, len(rows))
		for _, w := range rows {
			m[w.WrID] = w
		}
		found[boardID] = m
	}

	// 3단계: 후보 순서(= source_created_at DESC)를 그대로 유지해 방출한다.
	out := make([]gnuboard.FeedPost, 0, len(order))
	for _, c := range order {
		if m, ok := found[c.BoardID]; ok {
			if w, ok2 := m[c.WriteID]; ok2 {
				out = append(out, w)
			}
		}
	}
	return out, nil
}

// feedCandidatePool 은 재검증에서 걸러질 것을 감안한 후보 여유분이다.
// 기존 UNION 도 같은 상한(600)을 썼다.
const feedCandidatePool = 600

// cursorBoards 는 커서가 걸린 보드 슬러그 목록. 커서 없는 보드는 제한 없이 통과시킨다.
func cursorBoards(cursor map[string]int) []string {
	out := make([]string, 0, len(cursor))
	for b, wm := range cursor {
		if wm > 0 && activityBoardSlugRe.MatchString(b) {
			out = append(out, b)
		}
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
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
// ⛔ 이 함수는 **절대 (nil, nil) 을 돌려주지 않는다.** 반환값이 「허용 목록」이라
//
//	nil 은 「제한 없음」으로 읽힌다. 호출부가 `if set != nil && !set[b]` 같은 가드를 쓰면
//	조회 실패가 곧 **전면 허용**이 된다. 2026-08-21 에 이 실수를 하루에 세 곳에서 찾았다.
//
// 무엇이 새는가 — member_activity_feed 151개 보드 중 검색 대상이 아닌 것이 **29개**다:
//
//	adm(읽기레벨 10, 1,670글) · advertiser(10, 755) · temp(10, 753) · archive(10, 432)
//	disciplinelog(징계기록, 6,394) · truthroom(신고누적, 2,327) · claim(소명, 644) · angreport(신고, 456)
//
// 필터가 사라지면 **회원 공개 프로필에 소명·징계 게시판 글이 뜬다** — 누가 소명을 냈는지,
// 누가 징계 기록에 있는지가 제3자에게 드러난다. 관리자 게시판 노출보다 나쁜 종류다.
//
// ⭐ 그래서 실패는 반드시 error 로 나간다. 호출부가 "몰라서" 통과시키는 일이 없어야 한다.
func (r *myPageRepository) searchableBoardSet() (map[string]bool, error) {
	// ⭐ 빈 목록·조회 실패 판정은 GetSearchableBoards 가 한다(그쪽이 계약의 정본이다).
	//    같은 지식이 두 곳에 있으면 한쪽만 고쳐질 때 다시 벌어진다.
	boards, err := r.GetSearchableBoards()
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(boards))
	for _, b := range boards {
		set[b.BoTable] = true
	}
	return set, nil
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
	// ⛔ fail-closed. 보드 목록을 못 얻으면 **필터 없이 진행하지 않는다.**
	//    빈 결과를 돌려주면 호출부가 정본 UNION 폴백으로 떨어지고(그 경로도 fail-closed)
	//    사용자에게는 정상 동작한다 — 가용성을 잃지 않으면서 노출도 막는다.
	searchable, err := r.searchableBoardSet()
	if err != nil {
		log.Printf("[activity] %v — 보드 필터 없이 진행하지 않고 정본 조회로 폴백한다", err)
		return nil
	}
	byBoard := make(map[string][]int)
	for _, c := range candidates {
		if c.BoardID == "" {
			continue
		}
		if !searchable[c.BoardID] {
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
		// #13512: 후보에 이용제한 근거 글(비밀 처리)이 들어와도, 아래 비밀글 제외에
		//    다시 걸려 사라지면 마스킹을 태울 행이 없다. 근거로 등록된 wr_id 만 예외적으로
		//    비밀·잠금 제외를 우회시킨다(정본에 실재하면 남긴다). 제목 마스킹은 핸들러가
		//    [이용제한 근거 글]로 덮는다. 일반 비밀글은 근거 집합에 없어 그대로 제외(무회귀).
		//    근거 집합은 게시판 단위로 캐시돼 있어(loadDisciplinedIDs) 추가 DB 부하가 없다.
		var evidenceIDs []int
		// ⛔ 여기서 조회가 실패하면 **우회를 붙이지 않는다.** 그러면 아래 엄격 필터가
		//    그대로 적용돼 비밀·잠금 근거글이 목록에서 빠진다 — 원제목이 나가는 것보다 안전하다.
		//    (마스킹만이 보호막인 일반 근거글은 이 필터를 통과하므로 핸들러 쪽 방어가 따로 필요하다.)
		if disciplined, derr := r.loadDisciplinedIDs(boardID); derr != nil {
			log.Printf("[activity] 근거글 조회 실패 board=%s: %v — 비밀·잠금 근거글 우회를 붙이지 않는다", boardID, derr)
		} else {
			for _, id := range ids {
				if disciplined[id] {
					evidenceIDs = append(evidenceIDs, id)
				}
			}
		}
		// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
		q := fmt.Sprintf(
			"SELECT wr_id, wr_deleted_at FROM `g5_write_%s` WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0"+
				" AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL)"+
				" AND (wr_7 IS NULL OR wr_7 != 'lock')", boardID)
		qArgs := []interface{}{ids, mbID}
		if len(evidenceIDs) > 0 {
			// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
			q = fmt.Sprintf(
				"SELECT wr_id, wr_deleted_at FROM `g5_write_%s` WHERE wr_id IN ? AND mb_id = ? AND wr_is_comment = 0"+
					" AND (((wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL)"+
					" AND (wr_7 IS NULL OR wr_7 != 'lock')) OR wr_id IN ?)", boardID)
			qArgs = []interface{}{ids, mbID, evidenceIDs}
		}
		if err := r.db.Raw(q, qArgs...).Scan(&okRows).Error; err != nil {
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
	// ⛔ fail-closed. 보드 목록을 못 얻으면 **필터 없이 진행하지 않는다.**
	//    빈 결과를 돌려주면 호출부가 정본 UNION 폴백으로 떨어지고(그 경로도 fail-closed)
	//    사용자에게는 정상 동작한다 — 가용성을 잃지 않으면서 노출도 막는다.
	searchable, err := r.searchableBoardSet()
	if err != nil {
		log.Printf("[activity] %v — 보드 필터 없이 진행하지 않고 정본 조회로 폴백한다", err)
		return nil
	}
	byBoard := make(map[string][]int)
	// #13512 후속: 부모 글 id 도 모은다. 근거글 우회 판정은 **댓글이 아니라 부모 글**로 한다.
	parentsByBoard := make(map[string][]int)
	for _, c := range candidates {
		if c.BoardID == "" {
			continue
		}
		if !searchable[c.BoardID] {
			continue
		}
		byBoard[c.BoardID] = append(byBoard[c.BoardID], c.WrID)
		if c.WrParent > 0 {
			parentsByBoard[c.BoardID] = append(parentsByBoard[c.BoardID], c.WrParent)
		}
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

		// #13512 후속 — 근거글에 달린 **제3자 본인 댓글**이 통째로 사라지던 것.
		//
		// ⛔ 부모 글이 비밀·잠금이면 그 아래 댓글이 전부 빠졌다. 그런데 그 댓글들은
		//    제재당한 콘텐츠가 아니라 **각자 소유**다. 2026-08-21 실측(free):
		//      근거글(secret) 아래 댓글 956건 · 작성자 516명 · 그중 제3자 84%
		//    글 경로는 #680 에서 이미 우회를 붙였는데 댓글 경로에는 없었다.
		//
		// ⭐ 우회 판정은 **부모 글**로 한다(댓글 자신이 아니라). 근거글로 등록된 부모만
		//    비밀·잠금 제외를 우회시킨다 — 근거글이 아닌 일반 잠긴 글은 그대로 제외된다(무회귀).
		//
		// ⛔ 제재당한 댓글 자체(2,322건)는 이 우회와 무관하다. 핸들러가 [이용제한 댓글] 로
		//    마스킹하므로 내용은 안 나간다. 여기서 되살리는 것은 **무고한 제3자 댓글**뿐이다.
		//
		// ⛔ 조회가 실패하면 우회를 붙이지 않는다 — 엄격 필터가 그대로 적용된다.
		//    댓글이 덜 보이는 쪽이 원문이 새는 쪽보다 안전하다(글 경로와 같은 원칙).
		var evidenceParents []int
		if parents := parentsByBoard[boardID]; len(parents) > 0 {
			if disciplined, derr := r.loadDisciplinedIDs(boardID); derr != nil {
				log.Printf("[activity] 근거글 조회 실패(댓글) board=%s: %v — 우회를 붙이지 않는다", boardID, derr)
			} else {
				seen := make(map[int]struct{}, len(parents))
				for _, pid := range parents {
					if _, dup := seen[pid]; dup {
						continue
					}
					seen[pid] = struct{}{}
					if disciplined[pid] {
						evidenceParents = append(evidenceParents, pid)
					}
				}
			}
		}

		// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
		q := fmt.Sprintf(
			"SELECT c.wr_id, c.wr_deleted_at, p.wr_deleted_at AS parent_deleted_at"+
				" FROM `g5_write_%s` c INNER JOIN `g5_write_%s` p"+
				" ON p.wr_id = c.wr_parent AND p.wr_is_comment = 0"+
				" WHERE c.wr_id IN ? AND c.mb_id = ? AND c.wr_is_comment = 1"+
				" AND (p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL)"+
				" AND (p.wr_7 IS NULL OR p.wr_7 != 'lock')", boardID, boardID)
		qArgs := []interface{}{ids, mbID}
		if len(evidenceParents) > 0 {
			// #nosec G201 -- boardID 는 activityBoardSlugRe 로 검증된 슬러그다.
			q = fmt.Sprintf(
				"SELECT c.wr_id, c.wr_deleted_at, p.wr_deleted_at AS parent_deleted_at"+
					" FROM `g5_write_%s` c INNER JOIN `g5_write_%s` p"+
					" ON p.wr_id = c.wr_parent AND p.wr_is_comment = 0"+
					" WHERE c.wr_id IN ? AND c.mb_id = ? AND c.wr_is_comment = 1"+
					" AND (((p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL)"+
					" AND (p.wr_7 IS NULL OR p.wr_7 != 'lock')) OR p.wr_id IN ?)", boardID, boardID)
			qArgs = []interface{}{ids, mbID, evidenceParents}
		}
		if err := r.db.Raw(q, qArgs...).Scan(&okRows).Error; err != nil {
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
	// #13512: 이용제한 근거 글은 비밀 처리(wr_option 'secret')로 피드에 is_public=0·
	//    is_deleted=0 으로 실린다. 위 두 분기(is_public=1 / is_deleted=1) 어디에도
	//    걸리지 않아 후보에서 통째로 빠졌고, #12908 근거글 마스킹이 무력화됐다.
	//    → 세 번째 분기로 "근거로 등록된(g5_na_singo.discipline_log_id IS NOT NULL)"
	//    비밀·미삭제 글만 정확히 살려 후보에 넣는다. 일반 비밀글은 EXISTS 에 걸리지
	//    않아 계속 제외된다(무회귀). is_public=0 AND is_deleted=0 로 좁혀 1·2 분기와
	//    중복되지 않게 한다(공개/삭제 근거글은 이미 1·2 분기가 잡는다).
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
		    UNION ALL
		    (SELECT write_id AS wr_id,
		            COALESCE(title, '') AS wr_subject,
		            source_created_at AS wr_datetime,
		            board_id,
		            id AS feed_id
		       FROM member_activity_feed f
		      WHERE f.member_id = ?
		        AND f.activity_type = 1
		        AND f.is_public = 0
		        AND f.is_deleted = 0
		        AND f.write_table = CONCAT('g5_write_', f.board_id)
		        AND EXISTS (SELECT 1 FROM g5_na_singo s
		                     WHERE s.sg_table = f.board_id
		                       AND s.sg_id = f.write_id
		                       AND s.discipline_log_id IS NOT NULL)
		      ORDER BY source_created_at DESC, id DESC
		      LIMIT ?)
		 ) AS t
		 ORDER BY wr_datetime DESC, feed_id DESC
		 LIMIT ?`,
		mbID, limit*activityCandidateFactor,
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

	// ⛔ 예전엔 `err != nil || len(boards) == 0` 이었다. 빈 목록일 때 err 가 nil 이라
	//    (nil, nil) 로 나가 「이 회원은 글이 없다」는 성공 응답이 됐다 — 조회 실패가 조용히
	//    정상 결과로 둔갑했다. 이제 GetSearchableBoards 가 빈 목록도 error 로 준다.
	boards, err := r.GetSearchableBoards()
	if err != nil {
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
		            COALESCE(content_kind, '') AS content_kind,
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
		            COALESCE(content_kind, '') AS content_kind,
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

	// ⛔ 예전엔 `err != nil || len(boards) == 0` 이었다. 빈 목록일 때 err 가 nil 이라
	//    (nil, nil) 로 나가 「이 회원은 글이 없다」는 성공 응답이 됐다 — 조회 실패가 조용히
	//    정상 결과로 둔갑했다. 이제 GetSearchableBoards 가 빈 목록도 error 로 준다.
	boards, err := r.GetSearchableBoards()
	if err != nil {
		return nil, err
	}

	var unions []string
	var args []interface{}
	for _, b := range boards {
		table := fmt.Sprintf("g5_write_%s", b.BoTable)
		// #13512 후속 — 이 폴백에도 같은 우회를 붙인다.
		//
		// ⛔ verify 경로만 고치면 **확인된 게 0건일 때** 여기로 떨어지면서 다시 사라진다.
		//    실제로 근거글만 있는 회원은 verify 가 0건이 되기 쉬워, 폴백이 오히려 주 경로다.
		//
		// ⭐ 우회 조건은 verify 와 같다 — **부모 글이 근거글로 등록된 경우만** 비밀·잠금 제외를 푼다.
		//    여기서는 id 목록을 미리 못 구하므로 EXISTS 서브쿼리로 같은 판정을 한다
		//    (글 경로 FindPublicPostsByMember 의 3번째 분기와 동형).
		//    idx_singo_discipline(sg_table, discipline_log_id, sg_id) 가 있어 비싸지 않다.
		unions = append(unions, fmt.Sprintf(
			"(SELECT c.wr_id, c.wr_content, c.wr_parent, c.wr_datetime, '%s' as board_id, c.wr_deleted_at AS deleted_at, p.wr_deleted_at AS parent_deleted_at FROM `%s` c INNER JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 AND (((p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL) AND (p.wr_7 IS NULL OR p.wr_7 != 'lock')) OR EXISTS (SELECT 1 FROM g5_na_singo s WHERE s.sg_table = '%s' AND s.sg_id = p.wr_id AND s.discipline_log_id IS NOT NULL)) WHERE c.mb_id = ? AND c.wr_is_comment = 1 ORDER BY c.wr_id DESC LIMIT %d)",
			b.BoTable, table, table, b.BoTable, limit))
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
// disciplinedIDsCache 는 게시판별 "이용제한 근거" 대상 ID 집합을 통째로 캐시한다.
//
// ⛔ 예전에는 프로필을 열 때마다 게시판별로 DB 를 쳤다(글·댓글 두 곳에서 각각).
//
//	2026-08-19 실측: 누적 2억 640만회 · 156,912초 = DB 실행시간의 약 5.0%.
//	평균 760us 로 한 번은 빠르지만 **반환이 평균 0.01행** — 100번 중 99번이 빈손이었다.
//
// ⭐ 대상 전체가 작다 — 전 게시판 합쳐 3,407개(약 54KB). free 3,171 · truthroom 120 · car 32 …
//
//	그래서 게시판별로 통째로 캐시하고 메모리에서 판정한다. web 의 댓글 경로도 같은 방식으로
//	고쳤다(PR #2120) — 같은 문제가 두 저장소에 나뉘어 있었다.
//
// ⛔ 갱신 쿼리가 비싸면 문제를 옮기는 것뿐이다. g5_na_singo 에 discipline_log_id 를 포함한
//
//	인덱스가 없어 153,626행을 훑고 0.705초가 걸렸다. idx_table_disc_id(sg_table,
//	discipline_log_id, sg_id) 를 추가해 0.072초(10배)로 줄였다 — 이 인덱스를 지우면 다시 비싸진다.
var disciplinedIDsCache struct {
	sync.RWMutex
	byBoard map[string]map[int]bool
	expires map[string]time.Time
}

// 제재는 하루 몇 건 수준이라 5분이면 충분하다. 늦어도 "근거 표시" 가 5분 늦게 붙을 뿐이다.
const disciplinedIDsCacheTTL = 5 * time.Minute

// 이 수를 넘으면 쓰기 시점에 만료분을 청소한다. 게시판 수(약 167)보다 넉넉히 잡되
// 무한 증가는 막는다.
const disciplinedIDsCacheMaxBoards = 200

// loadDisciplinedIDs 는 게시판의 전체 근거 ID 집합을 가져온다(캐시 우선).
//
// ⛔ 실패를 nil 로 뭉개지 않는다. 반환값이 「가려야 할 글 목록」이라 nil 은
//
//	「가릴 게 없다」로 읽히고, 그러면 원제목이 그대로 프로필에 나간다.
//	2026-08-21 실측: free 한 곳에서만 **692글 · 341명**이 이 마스킹만을 보호막으로 삼는다
//	(나머지는 삭제·잠금·비밀글이라 어차피 안 보인다).
//	근거글은 징계를 부른 글이라 제목 자체가 분란 유도 문구인 경우가 많다.
//
// ⭐ 다만 곧바로 error 를 내지 않는다. 만료된 캐시가 있으면 그걸 쓴다 —
// 「몇 분 전의 근거글 목록」은 「근거글 없음」보다 압도적으로 정확하다.
// 제재는 하루 몇 건 수준이라 그 사이 새로 생긴 근거글은 사실상 없다.
// DisciplinedIDs 는 보드의 근거글 wr_id 집합을 돌려준다(캐시 + 만료 폴백).
//
// ⭐ 패키지 함수로 둔 이유 — 근거글 마스킹이 걸린 표면이 넷이고(회원 프로필 · 게시판 목록 ·
//
//	글 상세 · 댓글) 각자 따로 조회하면 같은 실수가 또 난다. 2026-08-21 에 이 계열 fail-open 을
//	다섯 곳에서 찾았다. **캐시 하나 · 계약 하나**로 모은다.
//
// 부수효과: 목록 경로는 예전에 요청마다 IN 절 쿼리를 쳤다. 이제 보드당 5분에 한 번이다.
func DisciplinedIDs(db *gorm.DB, boardID string) (map[int]bool, error) {
	var stale map[int]bool
	disciplinedIDsCache.RLock()
	if set, ok := disciplinedIDsCache.byBoard[boardID]; ok {
		if exp, ok2 := disciplinedIDsCache.expires[boardID]; ok2 && time.Now().Before(exp) {
			disciplinedIDsCache.RUnlock()
			return set, nil
		}
		// ⭐ 만료됐어도 붙들어 둔다. 조회가 실패하면 이게 유일한 정답 근사치다.
		stale = set
	}
	disciplinedIDsCache.RUnlock()

	var ids []int
	if err := db.Raw(
		"SELECT DISTINCT sg_id FROM g5_na_singo WHERE sg_table = ? AND discipline_log_id IS NOT NULL",
		boardID,
	).Scan(&ids).Error; err != nil {
		// ⛔ 실패를 캐시하지 않는다. DB 일시 장애가 TTL 동안 굳으면
		//    그 사이 근거 표시가 통째로 사라진다.
		if stale != nil {
			log.Printf("[activity] 근거글 조회 실패 board=%s: %v — 만료된 캐시로 대체한다(%d건)",
				boardID, err, len(stale))
			return stale, nil
		}
		return nil, fmt.Errorf("disciplined ids lookup failed (board=%s): %w", boardID, err)
	}

	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}

	disciplinedIDsCache.Lock()
	if disciplinedIDsCache.byBoard == nil {
		disciplinedIDsCache.byBoard = make(map[string]map[int]bool)
		disciplinedIDsCache.expires = make(map[string]time.Time)
	}
	// ⛔ 게시판이 167개다. 만료된 항목을 치우지 않으면 맵이 계속 자란다.
	//    전량이 54KB 수준이라 크지는 않지만, 상한 없는 캐시는 원칙적으로 두지 않는다.
	//    쓰기 시점에 만료분을 함께 정리한다(별도 타이머 불필요).
	if len(disciplinedIDsCache.byBoard) > disciplinedIDsCacheMaxBoards {
		now := time.Now()
		for b, exp := range disciplinedIDsCache.expires {
			if now.After(exp) {
				delete(disciplinedIDsCache.byBoard, b)
				delete(disciplinedIDsCache.expires, b)
			}
		}
	}
	// ⛔ 빈 집합도 캐시한다. 대부분의 게시판이 실제로 0개라, 캐시하지 않으면
	//    그 게시판들은 매 요청마다 DB 를 쳐서 원래 문제가 그대로 남는다.
	disciplinedIDsCache.byBoard[boardID] = set
	disciplinedIDsCache.expires[boardID] = time.Now().Add(disciplinedIDsCacheTTL)
	disciplinedIDsCache.Unlock()

	return set, nil
}

// loadDisciplinedIDs 는 DisciplinedIDs 로 위임한다. 계약의 정본은 그쪽 하나다.
func (r *myPageRepository) loadDisciplinedIDs(boardID string) (map[int]bool, error) {
	return DisciplinedIDs(r.db, boardID)
}

func (r *myPageRepository) FindDisciplinedIDs(boardID string, wrIDs []int) (map[int]bool, error) {
	result := make(map[int]bool)
	if boardID == "" || len(wrIDs) == 0 {
		return result, nil
	}

	// ⛔ 캐시된 map 을 그대로 돌려주지 마라 — 호출자가 쓰면 공유 상태가 오염된다.
	//    요청된 ID 만 골라 새 map 을 만든다(호출자 계약도 그대로 유지된다).
	// ⛔ 실패를 삼키지 않는다. 예전엔 이 함수의 return 경로가 둘 다 (result, nil) 이라
	//    **error 를 낼 수가 없었고**, 소비자의 `if err == nil` 가드가 구조적으로 무의미했다.
	boardSet, err := r.loadDisciplinedIDs(boardID)
	if err != nil {
		return nil, err
	}
	for _, id := range wrIDs {
		if boardSet[id] {
			result[id] = true
		}
	}
	return result, nil
}

// CountCommentReplies 는 댓글별 답글(대댓글) 수를 반환한다 (makeang/88).
//
// 그누보드 댓글 트리: 한 글의 댓글은 wr_comment(그룹번호)로 묶이고, wr_comment_reply
// 문자열(” → 'A' → 'AA' …)이 답글 깊이를 나타낸다. 내 댓글 C(그룹 N, 코드 R)의 답글은
// 같은 wr_parent·wr_comment=N 안에서 wr_comment_reply 가 R 로 시작하고 R 보다 긴 행들이다
// (R=” 이면 그룹의 모든 하위 = 최상위 댓글의 답글 전부). 실데이터로 검증한 규칙.
//
// ⛔ boardID 를 테이블명에 직접 넣으므로 활성 게시판 화이트리스트로 검증한다(주입 방지).
func (r *myPageRepository) CountCommentReplies(boardID string, wrIDs []int) (map[int]int, error) {
	result := make(map[int]int)
	if boardID == "" || len(wrIDs) == 0 {
		return result, nil
	}
	valid := false
	for _, b := range r.getActiveBoards() {
		if b == boardID {
			valid = true
			break
		}
	}
	if !valid {
		return result, nil
	}
	table := fmt.Sprintf("g5_write_%s", boardID)
	var rows []struct {
		WrID int `gorm:"column:wr_id"`
		Cnt  int `gorm:"column:cnt"`
	}
	q := fmt.Sprintf("SELECT c.wr_id AS wr_id, COUNT(r.wr_id) AS cnt "+
		"FROM `%s` c LEFT JOIN `%s` r "+
		"ON r.wr_is_comment = 1 AND r.wr_parent = c.wr_parent AND r.wr_comment = c.wr_comment "+
		"AND r.wr_comment_reply LIKE CONCAT(c.wr_comment_reply, '%%') "+
		"AND r.wr_comment_reply <> c.wr_comment_reply AND r.wr_deleted_at IS NULL "+
		"WHERE c.wr_id IN ? AND c.wr_is_comment = 1 "+
		"GROUP BY c.wr_id", table, table)
	if err := r.db.Raw(q, wrIDs).Scan(&rows).Error; err != nil {
		return result, err
	}
	for _, row := range rows {
		if row.Cnt > 0 {
			result[row.WrID] = row.Cnt
		}
	}
	return result, nil
}

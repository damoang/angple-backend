package gnuboard

import (
	"fmt"
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
	FindLikedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	GetBoardStats(mbID string) ([]gnuboard.BoardStat, error)
	FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error)
	FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error)
	GetSearchableBoards() ([]searchableBoard, error)
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
					r.db.Raw(
						fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `g5_write_%s` WHERE wr_id IN ? ORDER BY wr_datetime DESC", largeBoardID, largeBoardID),
						writeIDs,
					).Scan(&rows)
				}
			} else {
				table := fmt.Sprintf("g5_write_%s", bc.boardID)
				r.db.Raw(
					fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL ORDER BY wr_datetime DESC LIMIT %d", bc.boardID, table, perTable),
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
				r.db.Raw("SELECT COUNT(*) FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 2 AND is_deleted = 0", mbID, largeBoardID).Scan(&cnt)
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
				// Step 1: Get write_ids + parent_title from activity_feed (covering index, ~1ms)
				type commentRef struct {
					WriteID     int    `gorm:"column:write_id"`
					ParentTitle string `gorm:"column:parent_title"`
				}
				var refs []commentRef
				r.db.Raw(
					"SELECT write_id, COALESCE(parent_title, '') as parent_title FROM member_activity_feed WHERE member_id = ? AND board_id = ? AND activity_type = 2 AND is_deleted = 0 ORDER BY source_created_at DESC LIMIT ?",
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
						fmt.Sprintf("SELECT wr_id, wr_content, wr_datetime, mb_id, wr_name, wr_parent, wr_good, wr_nogood, wr_option, '' as post_title, '%s' as board_id FROM `g5_write_%s` WHERE wr_id IN ? AND wr_is_comment = 1 ORDER BY wr_datetime DESC", largeBoardID, largeBoardID),
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
					fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, COALESCE(p.wr_subject, '') as post_title, '%s' as board_id FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_datetime DESC LIMIT %d", bc.boardID, table, table, perTable),
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

// FindPublicPostsByMember returns recent public posts by a member.
// Uses UNION ALL with per-subquery LIMIT for efficiency.
// Each subquery leverages mb_id index + PK ordering.
func (r *myPageRepository) FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error) {
	var posts []gnuboard.ActivityPost
	if err := r.db.Raw(
		`SELECT write_id AS wr_id,
		        COALESCE(title, '') AS wr_subject,
		        source_created_at AS wr_datetime,
		        board_id
		   FROM member_activity_feed
		  WHERE member_id = ?
		    AND activity_type = 1
		    AND is_public = 1
		    AND is_deleted = 0
		  ORDER BY source_created_at DESC, id DESC
		  LIMIT ?`,
		mbID, limit,
	).Scan(&posts).Error; err == nil {
		return posts, nil
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
			"(SELECT wr_id, wr_subject, wr_datetime, '%s' as board_id FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND (wr_option NOT LIKE '%%secret%%' OR wr_option IS NULL) AND (wr_7 IS NULL OR wr_7 != 'lock') AND wr_deleted_at IS NULL ORDER BY wr_id DESC LIMIT %d)",
			b.BoTable, table, limit))
		args = append(args, mbID)
	}

	sql := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_id DESC LIMIT ?", strings.Join(unions, " UNION ALL "))
	args = append(args, limit)

	if err := r.db.Raw(sql, args...).Scan(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// FindPublicCommentsByMember returns recent public comments by a member.
// Uses UNION ALL + INNER JOIN to filter out comments on secret/locked/deleted parent posts.
func (r *myPageRepository) FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error) {
	var comments []gnuboard.ActivityComment
	if err := r.db.Raw(
		`SELECT write_id AS wr_id,
		        COALESCE(content_preview, '') AS wr_content,
		        COALESCE(parent_write_id, 0) AS wr_parent,
		        source_created_at AS wr_datetime,
		        board_id
		   FROM member_activity_feed
		  WHERE member_id = ?
		    AND activity_type = 2
		    AND is_public = 1
		    AND is_deleted = 0
		  ORDER BY source_created_at DESC, id DESC
		  LIMIT ?`,
		mbID, limit,
	).Scan(&comments).Error; err == nil {
		return comments, nil
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
			"(SELECT c.wr_id, c.wr_content, c.wr_parent, c.wr_datetime, '%s' as board_id FROM `%s` c INNER JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 AND (p.wr_option NOT LIKE '%%secret%%' OR p.wr_option IS NULL) AND (p.wr_7 IS NULL OR p.wr_7 != 'lock') AND p.wr_deleted_at IS NULL WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_id DESC LIMIT %d)",
			b.BoTable, table, table, limit))
		args = append(args, mbID)
	}

	sql := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_id DESC LIMIT ?", strings.Join(unions, " UNION ALL "))
	args = append(args, limit)

	if err := r.db.Raw(sql, args...).Scan(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

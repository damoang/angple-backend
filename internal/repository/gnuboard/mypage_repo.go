package gnuboard

import (
	"fmt"
	"sort"
	"strings"
	"sync"

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

// FindPostsByMember returns posts written by the member across all boards.
// Uses parallel per-board queries instead of UNION ALL for better DB performance.
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
			table := fmt.Sprintf("g5_write_%s", boardID)
			var cnt int64
			r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", table), mbID).Scan(&cnt)
			if cnt > 0 {
				muCounts.Lock()
				counts = append(counts, boardCount{boardID: boardID, count: cnt})
				totalCount += cnt
				muCounts.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()

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
			table := fmt.Sprintf("g5_write_%s", bc.boardID)
			var rows []gnuboard.MyPost
			r.db.Raw(
				fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL ORDER BY wr_datetime DESC LIMIT %d", bc.boardID, table, perTable),
				mbID,
			).Scan(&rows)
			if len(rows) > 0 {
				mu.Lock()
				posts = append(posts, rows...)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g2.Wait()

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
			table := fmt.Sprintf("g5_write_%s", boardID)
			var cnt int64
			r.db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE mb_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", table), mbID).Scan(&cnt)
			if cnt > 0 {
				muCounts.Lock()
				counts = append(counts, boardCount{boardID: boardID, count: cnt})
				totalCount += cnt
				muCounts.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()

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
			table := fmt.Sprintf("g5_write_%s", bc.boardID)
			var rows []gnuboard.MyCommentRow
			r.db.Raw(
				fmt.Sprintf("SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, COALESCE(p.wr_subject, '') as post_title, '%s' as board_id FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_datetime DESC LIMIT %d", bc.boardID, table, table, perTable),
				mbID,
			).Scan(&rows)
			if len(rows) > 0 {
				mu.Lock()
				comments = append(comments, rows...)
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g2.Wait()

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
	_ = g.Wait()

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
	_ = g.Wait()

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

// GetSearchableBoards returns boards with bo_use_search=1 that have existing write tables
func (r *myPageRepository) GetSearchableBoards() ([]searchableBoard, error) {
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
	return result, nil
}

// FindPublicPostsByMember returns recent public posts by a member.
// Temporarily disabled: returns empty to prevent slow queries under high concurrency.
func (r *myPageRepository) FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error) {
	return nil, nil
}

// FindPublicCommentsByMember returns recent public comments by a member.
// Temporarily disabled: returns empty to prevent slow queries under high concurrency.
func (r *myPageRepository) FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error) {
	return nil, nil
}

package gnuboard

import (
	"fmt"
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
// Each sub-query uses per-table ORDER BY + LIMIT to cap rows_examined.
// Relies on idx_mb_comment(mb_id, wr_is_comment) index.
func (r *myPageRepository) FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	// Cap per-table fetch: page * limit ensures we can paginate correctly
	perTable := page * limit

	var unions []string
	var args []interface{}
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL ORDER BY wr_datetime DESC LIMIT %d)",
			boardID, table, perTable))
		args = append(args, mbID)
	}
	unionQuery := strings.Join(unions, " UNION ALL ")

	// Count from the capped result (approximate for deep pages, exact for early pages)
	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", unionQuery)
	if err := r.db.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * limit
	dataSQL := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ? OFFSET ?", unionQuery)
	dataArgs := append(args, limit, offset)

	var posts []gnuboard.MyPost
	if err := r.db.Raw(dataSQL, dataArgs...).Scan(&posts).Error; err != nil {
		return nil, 0, err
	}
	return posts, total, nil
}

// FindCommentsByMember returns comments written by the member with parent post titles.
// Each sub-query uses per-table ORDER BY + LIMIT to cap rows_examined.
func (r *myPageRepository) FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	perTable := page * limit

	var unions []string
	var args []interface{}
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)
		unions = append(unions, fmt.Sprintf(
			"(SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, COALESCE(p.wr_subject, '') as post_title, '%s' as board_id FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL ORDER BY c.wr_datetime DESC LIMIT %d)",
			boardID, table, table, perTable))
		args = append(args, mbID)
	}
	unionQuery := strings.Join(unions, " UNION ALL ")

	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", unionQuery)
	if err := r.db.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	offset := (page - 1) * limit
	dataSQL := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ? OFFSET ?", unionQuery)
	dataArgs := append(args, limit, offset)

	var comments []gnuboard.MyCommentRow
	if err := r.db.Raw(dataSQL, dataArgs...).Scan(&comments).Error; err != nil {
		return nil, 0, err
	}
	return comments, total, nil
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
// Uses UNION ALL with per-table aggregation (each sub-query returns 1 row).
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

	var unions []string
	var args []interface{}
	for _, tableName := range existingTables {
		boardID := strings.TrimPrefix(tableName, "g5_write_")
		unions = append(unions, fmt.Sprintf(
			"(SELECT '%s' as board_id, SUM(CASE WHEN wr_is_comment = 0 THEN 1 ELSE 0 END) as post_count, SUM(CASE WHEN wr_is_comment = 1 THEN 1 ELSE 0 END) as comment_count FROM `%s` WHERE mb_id = ? AND wr_deleted_at IS NULL)",
			boardID, tableName))
		args = append(args, mbID)
	}

	type boardCount struct {
		BoardID      string `gorm:"column:board_id"`
		PostCount    int64  `gorm:"column:post_count"`
		CommentCount int64  `gorm:"column:comment_count"`
	}
	var counts []boardCount
	unionSQL := strings.Join(unions, " UNION ALL ")
	if err := r.db.Raw(unionSQL, args...).Scan(&counts).Error; err != nil {
		return nil, err
	}

	var stats []gnuboard.BoardStat
	for _, c := range counts {
		if c.PostCount > 0 || c.CommentCount > 0 {
			tableName := fmt.Sprintf("g5_write_%s", c.BoardID)
			stats = append(stats, gnuboard.BoardStat{
				BoardID:      c.BoardID,
				BoardName:    boardMap[tableName],
				PostCount:    c.PostCount,
				CommentCount: c.CommentCount,
			})
		}
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
// Per-table LIMIT ensures each sub-query scans at most N rows via idx_mb_comment.
func (r *myPageRepository) FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error) {
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

	var posts []gnuboard.ActivityPost
	if err := r.db.Raw(sql, args...).Scan(&posts).Error; err != nil {
		return nil, err
	}
	return posts, nil
}

// FindPublicCommentsByMember returns recent public comments by a member.
// Per-table LIMIT ensures each sub-query scans at most N rows via idx_mb_comment.
func (r *myPageRepository) FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error) {
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

	var comments []gnuboard.ActivityComment
	if err := r.db.Raw(sql, args...).Scan(&comments).Error; err != nil {
		return nil, err
	}
	return comments, nil
}

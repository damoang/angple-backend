package gnuboard

import (
	"fmt"
	"strings"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// MyPageRepository provides access to user's posts, comments, and stats across g5_write_* tables
type MyPageRepository interface {
	FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error)
	FindLikedPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error)
	GetBoardStats(mbID string) ([]gnuboard.BoardStat, error)
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

// FindPostsByMember returns posts written by the member across all boards
func (r *myPageRepository) FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	// Build UNION ALL query for posts
	var unions []string
	var args []interface{}
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)
		unions = append(unions, fmt.Sprintf(
			"(SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE mb_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL)",
			boardID, table))
		args = append(args, mbID)
	}
	unionQuery := strings.Join(unions, " UNION ALL ")

	// Count total
	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", unionQuery)
	if err := r.db.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return nil, 0, nil
	}

	// Fetch page
	offset := (page - 1) * limit
	dataSQL := fmt.Sprintf("SELECT * FROM (%s) AS t ORDER BY wr_datetime DESC LIMIT ? OFFSET ?", unionQuery)
	dataArgs := append(args, limit, offset)

	var posts []gnuboard.MyPost
	if err := r.db.Raw(dataSQL, dataArgs...).Scan(&posts).Error; err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

// FindCommentsByMember returns comments written by the member with parent post titles
func (r *myPageRepository) FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error) {
	boards := r.getActiveBoards()
	if len(boards) == 0 {
		return nil, 0, nil
	}

	// Build UNION ALL query for comments with parent post title
	var unions []string
	var args []interface{}
	for _, boardID := range boards {
		table := fmt.Sprintf("g5_write_%s", boardID)
		unions = append(unions, fmt.Sprintf(
			"(SELECT c.wr_id, c.wr_content, c.wr_datetime, c.mb_id, c.wr_name, c.wr_parent, c.wr_good, c.wr_nogood, c.wr_option, COALESCE(p.wr_subject, '') as post_title, '%s' as board_id FROM `%s` c LEFT JOIN `%s` p ON c.wr_parent = p.wr_id AND p.wr_is_comment = 0 WHERE c.mb_id = ? AND c.wr_is_comment = 1 AND c.wr_deleted_at IS NULL)",
			boardID, table, table))
		args = append(args, mbID)
	}
	unionQuery := strings.Join(unions, " UNION ALL ")

	// Count total
	var total int64
	countSQL := fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS t", unionQuery)
	if err := r.db.Raw(countSQL, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}

	if total == 0 {
		return nil, 0, nil
	}

	// Fetch page
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

	// Fetch post details per board
	postMap := make(map[string]gnuboard.MyPost)
	for boardID, wrIDs := range boardPosts {
		table := fmt.Sprintf("g5_write_%s", boardID)
		var posts []gnuboard.MyPost
		if err := r.db.Raw(
			fmt.Sprintf("SELECT wr_id, wr_subject, wr_content, wr_hit, wr_good, wr_nogood, wr_comment, wr_datetime, mb_id, wr_name, wr_option, wr_file, '%s' as board_id FROM `%s` WHERE wr_id IN ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", boardID, table),
			wrIDs,
		).Scan(&posts).Error; err != nil {
			continue // skip boards with errors
		}
		for _, p := range posts {
			key := fmt.Sprintf("%s:%d", boardID, p.WrID)
			postMap[key] = p
		}
	}

	// Build result in original order
	var result []gnuboard.MyPost
	for _, key := range refOrder {
		if post, ok := postMap[key]; ok {
			result = append(result, post)
		}
	}

	return result, total, nil
}

// GetBoardStats returns post/comment counts per board for the member
func (r *myPageRepository) GetBoardStats(mbID string) ([]gnuboard.BoardStat, error) {
	boards, err := r.boardRepo.FindAll()
	if err != nil {
		return nil, err
	}

	if len(boards) == 0 {
		return nil, nil
	}

	// 1. Batch check which tables exist via information_schema (1 query instead of N)
	tableNames := make([]string, len(boards))
	boardMap := make(map[string]string) // table_name -> bo_subject
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

	// 2. Build UNION ALL for post/comment counts (1 query instead of 2N)
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

package gnuboard

import (
	"fmt"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

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

// FindPostsByMember returns posts written by the member across all boards
// TODO: UNION ALL across ~90 tables causes slow queries (2+ sec). Needs index or denormalized table.
func (r *myPageRepository) FindPostsByMember(mbID string, page, limit int) ([]gnuboard.MyPost, int64, error) {
	return nil, 0, nil
}

// FindCommentsByMember returns comments written by the member with parent post titles
// TODO: UNION ALL across ~90 tables causes slow queries (2+ sec). Needs index or denormalized table.
func (r *myPageRepository) FindCommentsByMember(mbID string, page, limit int) ([]gnuboard.MyCommentRow, int64, error) {
	return nil, 0, nil
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
// TODO: UNION ALL across ~90 tables causes slow queries. Needs denormalized stats table.
func (r *myPageRepository) GetBoardStats(mbID string) ([]gnuboard.BoardStat, error) {
	return nil, nil
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

// FindPublicPostsByMember returns recent public posts by a member using UNION ALL
// TODO: UNION ALL across ~90 tables causes slow queries (2+ sec). Needs index or denormalized table.
func (r *myPageRepository) FindPublicPostsByMember(mbID string, limit int) ([]gnuboard.ActivityPost, error) {
	return nil, nil
}

// FindPublicCommentsByMember returns recent public comments by a member using UNION ALL
// TODO: UNION ALL across ~90 tables causes slow queries (2+ sec). Needs index or denormalized table.
func (r *myPageRepository) FindPublicCommentsByMember(mbID string, limit int) ([]gnuboard.ActivityComment, error) {
	return nil, nil
}

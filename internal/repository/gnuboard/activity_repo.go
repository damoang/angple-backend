package gnuboard

import (
	"sync"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

const (
	activityTypePost    int8 = 1
	activityTypeComment int8 = 2
)

type activityCommentRef struct {
	BoardID       string `gorm:"column:board_id"`
	WriteID       int    `gorm:"column:write_id"`
	ParentWriteID int    `gorm:"column:parent_write_id"`
	ParentTitle   string `gorm:"column:parent_title"`
}

type activityPostRef struct {
	BoardID string `gorm:"column:board_id"`
	WriteID int    `gorm:"column:write_id"`
}

// MemberActivityRepository provides read access to the member_activity_feed read model.
type MemberActivityRepository interface {
	IsEnabled() bool
	CountByMemberBoardType(mbID, boardID string, activityType int8) (int64, error)
	CountByMemberType(mbID string, activityType int8) (int64, error)
	ListWriteIDsByMemberBoardType(mbID, boardID string, activityType int8, limit int) ([]int, error)
	ListPostRefsByMember(mbID string, offset, limit int) ([]activityPostRef, error)
	ListCommentRefsByMember(mbID string, offset, limit int) ([]activityCommentRef, error)
	ListPostsByMember(mbID string, offset, limit int) ([]gnudomain.MyPost, error)
	ListCommentsByMember(mbID string, offset, limit int) ([]gnudomain.MyCommentRow, error)
	ListCommentRefsByMemberBoardType(mbID, boardID string, limit int) ([]activityCommentRef, error)
	ListBoardStatsByMember(mbID string) ([]gnudomain.MemberActivityStatsRow, error)
	ListPublicPostsByMember(mbID string, limit int) ([]gnudomain.ActivityPost, error)
	ListPublicCommentsByMember(mbID string, limit int) ([]gnudomain.ActivityComment, error)
}

type memberActivityRepository struct {
	db *gorm.DB
}

var memberActivityTableCache struct {
	sync.RWMutex
	exists    bool
	checked   bool
	expiresAt time.Time
}

const memberActivityTableCacheTTL = time.Minute

func NewMemberActivityRepository(db *gorm.DB) MemberActivityRepository {
	return &memberActivityRepository{db: db}
}

func (r *memberActivityRepository) IsEnabled() bool {
	return r.hasFeedTable()
}

func (r *memberActivityRepository) CountByMemberBoardType(mbID, boardID string, activityType int8) (int64, error) {
	if !r.hasFeedTable() {
		return 0, gorm.ErrRecordNotFound
	}

	var count int64
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Where("member_id = ? AND board_id = ? AND activity_type = ? AND is_deleted = 0", mbID, boardID, activityType).
		Count(&count).Error
	return count, err
}

func (r *memberActivityRepository) CountByMemberType(mbID string, activityType int8) (int64, error) {
	if !r.hasFeedTable() {
		return 0, gorm.ErrRecordNotFound
	}

	var count int64
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Where("member_id = ? AND activity_type = ? AND is_deleted = 0", mbID, activityType).
		Count(&count).Error
	return count, err
}

func (r *memberActivityRepository) ListWriteIDsByMemberBoardType(mbID, boardID string, activityType int8, limit int) ([]int, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var rows []struct {
		WriteID int `gorm:"column:write_id"`
	}
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("write_id").
		Where("member_id = ? AND board_id = ? AND activity_type = ? AND is_deleted = 0", mbID, boardID, activityType).
		Order("source_created_at DESC, id DESC").
		Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	writeIDs := make([]int, len(rows))
	for i, row := range rows {
		writeIDs[i] = row.WriteID
	}
	return writeIDs, nil
}

func (r *memberActivityRepository) ListCommentRefsByMemberBoardType(mbID, boardID string, limit int) ([]activityCommentRef, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var refs []activityCommentRef
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("write_id, COALESCE(parent_title, '') AS parent_title").
		Where("member_id = ? AND board_id = ? AND activity_type = ? AND is_deleted = 0", mbID, boardID, activityTypeComment).
		Order("source_created_at DESC, id DESC").
		Limit(limit).
		Scan(&refs).Error
	return refs, err
}

func (r *memberActivityRepository) ListPostRefsByMember(mbID string, offset, limit int) ([]activityPostRef, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var refs []activityPostRef
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("board_id, write_id").
		Where("member_id = ? AND activity_type = ? AND is_deleted = 0", mbID, activityTypePost).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Scan(&refs).Error
	return refs, err
}

func (r *memberActivityRepository) ListCommentRefsByMember(mbID string, offset, limit int) ([]activityCommentRef, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var refs []activityCommentRef
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("board_id, write_id, COALESCE(parent_write_id, 0) AS parent_write_id, COALESCE(parent_title, '') AS parent_title").
		Where("member_id = ? AND activity_type = ? AND is_deleted = 0", mbID, activityTypeComment).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Scan(&refs).Error
	return refs, err
}

func (r *memberActivityRepository) ListPostsByMember(mbID string, offset, limit int) ([]gnudomain.MyPost, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var posts []gnudomain.MyPost
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select(`
			write_id AS wr_id,
			COALESCE(title, '') AS wr_subject,
			COALESCE(content_preview, '') AS wr_content,
			view_count AS wr_hit,
			like_count AS wr_good,
			dislike_count AS wr_nogood,
			comment_count AS wr_comment,
			source_created_at AS wr_datetime,
			member_id AS mb_id,
			COALESCE(author_name, '') AS wr_name,
			COALESCE(wr_option, '') AS wr_option,
			CASE WHEN has_file = 1 THEN 1 ELSE 0 END AS wr_file,
			board_id`).
		Where("member_id = ? AND activity_type = ? AND is_deleted = 0", mbID, activityTypePost).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Scan(&posts).Error
	return posts, err
}

func (r *memberActivityRepository) ListCommentsByMember(mbID string, offset, limit int) ([]gnudomain.MyCommentRow, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var comments []gnudomain.MyCommentRow
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select(`
			write_id AS wr_id,
			COALESCE(content_preview, '') AS wr_content,
			source_created_at AS wr_datetime,
			member_id AS mb_id,
			COALESCE(author_name, '') AS wr_name,
			COALESCE(parent_write_id, 0) AS wr_parent,
			like_count AS wr_good,
			dislike_count AS wr_nogood,
			COALESCE(wr_option, '') AS wr_option,
			COALESCE(parent_title, '') AS post_title,
			board_id`).
		Where("member_id = ? AND activity_type = ? AND is_deleted = 0", mbID, activityTypeComment).
		Order("source_created_at DESC, id DESC").
		Offset(offset).
		Limit(limit).
		Scan(&comments).Error
	return comments, err
}

func (r *memberActivityRepository) ListBoardStatsByMember(mbID string) ([]gnudomain.MemberActivityStatsRow, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var stats []gnudomain.MemberActivityStatsRow
	err := r.db.Model(&gnudomain.MemberActivityStatsRow{}).
		Where("member_id = ? AND board_id != '' AND (post_count > 0 OR comment_count > 0)", mbID).
		Order("post_count + comment_count DESC, board_id ASC").
		Find(&stats).Error
	return stats, err
}

func (r *memberActivityRepository) ListPublicPostsByMember(mbID string, limit int) ([]gnudomain.ActivityPost, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var posts []gnudomain.ActivityPost
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("write_id AS wr_id, COALESCE(title, '') AS wr_subject, source_created_at AS wr_datetime, board_id").
		Where("member_id = ? AND activity_type = ? AND is_public = 1 AND is_deleted = 0", mbID, activityTypePost).
		Order("source_created_at DESC, id DESC").
		Limit(limit).
		Scan(&posts).Error
	return posts, err
}

func (r *memberActivityRepository) ListPublicCommentsByMember(mbID string, limit int) ([]gnudomain.ActivityComment, error) {
	if !r.hasFeedTable() {
		return nil, gorm.ErrRecordNotFound
	}

	var comments []gnudomain.ActivityComment
	err := r.db.Model(&gnudomain.MemberActivityFeed{}).
		Select("write_id AS wr_id, COALESCE(content_preview, '') AS wr_content, COALESCE(parent_write_id, 0) AS wr_parent, source_created_at AS wr_datetime, board_id").
		Where("member_id = ? AND activity_type = ? AND is_public = 1 AND is_deleted = 0", mbID, activityTypeComment).
		Order("source_created_at DESC, id DESC").
		Limit(limit).
		Scan(&comments).Error
	return comments, err
}

func (r *memberActivityRepository) hasFeedTable() bool {
	now := time.Now()

	memberActivityTableCache.RLock()
	if memberActivityTableCache.checked && now.Before(memberActivityTableCache.expiresAt) {
		exists := memberActivityTableCache.exists
		memberActivityTableCache.RUnlock()
		return exists
	}
	memberActivityTableCache.RUnlock()

	var count int64
	err := r.db.Raw(
		"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?",
		"member_activity_feed",
	).Scan(&count).Error
	exists := err == nil && count > 0

	memberActivityTableCache.Lock()
	memberActivityTableCache.exists = exists
	memberActivityTableCache.checked = true
	memberActivityTableCache.expiresAt = now.Add(memberActivityTableCacheTTL)
	memberActivityTableCache.Unlock()

	return exists
}

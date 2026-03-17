package service

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	memberActivityHTMLTagRe = regexp.MustCompile(`<[^>]*>`)
	memberActivityEmoRe     = regexp.MustCompile(`\{emo:[^}]+\}`)
	memberActivityWSRe      = regexp.MustCompile(`\s+`)
	memberActivitySlugRe    = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
)

type MemberActivitySyncService struct {
	db *gorm.DB
}

type MemberActivityBackfillReport struct {
	Scope             string
	BoardSlug         string
	PostCount         int64
	CommentCount      int64
	ProcessedPosts    int64
	ProcessedComments int64
}

type MemberActivityVerifyReport struct {
	Scope                 string
	BoardSlug             string
	SourcePosts           int64
	FeedPosts             int64
	SourceComments        int64
	FeedComments          int64
	SourceDeletedPosts    int64
	FeedDeletedPosts      int64
	SourceDeletedComments int64
	FeedDeletedComments   int64
}

func NewMemberActivitySyncService(db *gorm.DB) *MemberActivitySyncService {
	return &MemberActivitySyncService{db: db}
}

func (s *MemberActivitySyncService) SyncPost(postID uint64) error {
	return s.syncPost(postID, true)
}

func (s *MemberActivitySyncService) syncPost(postID uint64, rebuildStats bool) error {
	type postRow struct {
		ID           uint64    `gorm:"column:id"`
		BoardSlug    string    `gorm:"column:board_slug"`
		MemberID     string    `gorm:"column:member_id"`
		Author       string    `gorm:"column:author_name"`
		Title        string    `gorm:"column:title"`
		Content      string    `gorm:"column:content"`
		ViewCount    int       `gorm:"column:view_count"`
		LikeCount    int       `gorm:"column:like_count"`
		DislikeCount int       `gorm:"column:dislike_count"`
		CommentCount int       `gorm:"column:comment_count"`
		IsSecret     bool      `gorm:"column:is_secret"`
		Status       string    `gorm:"column:status"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var row postRow
	if err := s.db.Raw(`
		SELECT p.id, b.slug AS board_slug, COALESCE(m.mb_id, '') AS member_id,
		       COALESCE(m.mb_nick, '') AS author_name, p.title, p.content,
		       p.view_count, p.like_count, p.dislike_count, p.comment_count,
		       p.is_secret, p.status, p.created_at, p.updated_at
		  FROM v2_posts p
		  JOIN v2_boards b ON b.id = p.board_id
		  LEFT JOIN g5_member m ON m.mb_no = p.user_id
		 WHERE p.id = ?
		 LIMIT 1`, postID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == 0 || row.MemberID == "" || row.BoardSlug == "" {
		return nil
	}

	isPublic := !row.IsSecret && row.Status != "deleted" && s.isBoardSearchable(row.BoardSlug)
	updatedAt := row.UpdatedAt
	affectedMembers := []string{row.MemberID}
	feed := &gnudomain.MemberActivityFeed{
		MemberID:        row.MemberID,
		BoardID:         row.BoardSlug,
		WriteTable:      "v2_posts",
		WriteID:         int(row.ID),
		ActivityType:    1,
		IsPublic:        isPublic,
		IsDeleted:       row.Status == "deleted",
		Title:           truncateForFeed(row.Title, 255),
		ContentPreview:  buildContentPreview(row.Content, 200),
		AuthorName:      row.Author,
		WrOption:        wrOptionFromSecret(row.IsSecret),
		ViewCount:       row.ViewCount,
		LikeCount:       row.LikeCount,
		DislikeCount:    row.DislikeCount,
		CommentCount:    row.CommentCount,
		SourceCreatedAt: row.CreatedAt,
		SourceUpdatedAt: &updatedAt,
	}
	if err := s.upsertFeed(feed); err != nil {
		return err
	}

	// Parent title/public visibility changes cascade to comment activity rows.
	if err := s.db.Table("member_activity_feed").
		Where("write_table = ? AND parent_write_id = ? AND activity_type = 2", "v2_comments", postID).
		Updates(map[string]interface{}{
			"parent_title":      truncateForFeed(row.Title, 255),
			"is_public":         boolToInt(isPublic),
			"source_updated_at": updatedAt,
		}).Error; err != nil {
		return err
	}
	if members, err := s.memberIDsForParent("v2_comments", int(postID)); err == nil {
		affectedMembers = append(affectedMembers, members...)
	}

	if !rebuildStats {
		return nil
	}
	return s.rebuildMemberBoardStats(row.BoardSlug, uniqueNonEmptyStrings(affectedMembers))
}

func (s *MemberActivitySyncService) SyncComment(commentID uint64) error {
	return s.syncComment(commentID, true)
}

func (s *MemberActivitySyncService) syncComment(commentID uint64, rebuildStats bool) error {
	type commentRow struct {
		ID           uint64    `gorm:"column:id"`
		PostID       uint64    `gorm:"column:post_id"`
		BoardSlug    string    `gorm:"column:board_slug"`
		MemberID     string    `gorm:"column:member_id"`
		Author       string    `gorm:"column:author_name"`
		Content      string    `gorm:"column:content"`
		PostTitle    string    `gorm:"column:post_title"`
		LikeCount    int       `gorm:"column:like_count"`
		DislikeCount int       `gorm:"column:dislike_count"`
		PostSecret   bool      `gorm:"column:post_secret"`
		PostStatus   string    `gorm:"column:post_status"`
		Status       string    `gorm:"column:status"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var row commentRow
	if err := s.db.Raw(`
		SELECT c.id, c.post_id, b.slug AS board_slug, COALESCE(m.mb_id, '') AS member_id,
		       COALESCE(m.mb_nick, '') AS author_name, c.content,
		       c.like_count, c.dislike_count,
		       COALESCE(p.title, '') AS post_title, p.is_secret AS post_secret,
		       p.status AS post_status, c.status, c.created_at, c.updated_at
		  FROM v2_comments c
		  JOIN v2_posts p ON p.id = c.post_id
		  JOIN v2_boards b ON b.id = p.board_id
		  LEFT JOIN g5_member m ON m.mb_no = c.user_id
		 WHERE c.id = ?
		 LIMIT 1`, commentID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == 0 || row.MemberID == "" || row.BoardSlug == "" {
		return nil
	}

	isPublic := row.Status != "deleted" &&
		row.PostStatus != "deleted" &&
		!row.PostSecret &&
		s.isBoardSearchable(row.BoardSlug)
	updatedAt := row.UpdatedAt
	parentID := int(row.PostID)
	feed := &gnudomain.MemberActivityFeed{
		MemberID:        row.MemberID,
		BoardID:         row.BoardSlug,
		WriteTable:      "v2_comments",
		WriteID:         int(row.ID),
		ParentWriteID:   &parentID,
		ActivityType:    2,
		IsPublic:        isPublic,
		IsDeleted:       row.Status == "deleted",
		ContentPreview:  buildContentPreview(row.Content, 200),
		ParentTitle:     truncateForFeed(row.PostTitle, 255),
		AuthorName:      row.Author,
		LikeCount:       row.LikeCount,
		DislikeCount:    row.DislikeCount,
		SourceCreatedAt: row.CreatedAt,
		SourceUpdatedAt: &updatedAt,
	}
	if err := s.upsertFeed(feed); err != nil {
		return err
	}

	if !rebuildStats {
		return nil
	}
	return s.rebuildMemberBoardStats(row.BoardSlug, []string{row.MemberID})
}

func (s *MemberActivitySyncService) SyncLegacyPost(boardSlug string, wrID int) error {
	return s.syncLegacyPost(boardSlug, wrID, true)
}

func (s *MemberActivitySyncService) syncLegacyPost(boardSlug string, wrID int, rebuildStats bool) error {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return err
	}

	type postRow struct {
		ID           int       `gorm:"column:id"`
		MemberID     string    `gorm:"column:member_id"`
		Author       string    `gorm:"column:author_name"`
		Title        string    `gorm:"column:title"`
		Content      string    `gorm:"column:content"`
		WrOption     string    `gorm:"column:wr_option"`
		ViewCount    int       `gorm:"column:view_count"`
		LikeCount    int       `gorm:"column:like_count"`
		DislikeCount int       `gorm:"column:dislike_count"`
		CommentCount int       `gorm:"column:comment_count"`
		HasFile      bool      `gorm:"column:has_file"`
		IsDeleted    bool      `gorm:"column:is_deleted"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var row postRow
	query := fmt.Sprintf(`
		SELECT p.wr_id AS id,
		       COALESCE(p.mb_id, '') AS member_id,
		       COALESCE(p.wr_name, '') AS author_name,
		       COALESCE(p.wr_subject, '') AS title,
		       COALESCE(p.wr_content, '') AS content,
		       COALESCE(p.wr_option, '') AS wr_option,
		       COALESCE(p.wr_hit, 0) AS view_count,
		       COALESCE(p.wr_good, 0) AS like_count,
		       COALESCE(p.wr_nogood, 0) AS dislike_count,
		       COALESCE(p.wr_comment, 0) AS comment_count,
		       (COALESCE(p.wr_file, 0) > 0) AS has_file,
		       (p.wr_deleted_at IS NOT NULL) AS is_deleted,
		       p.wr_datetime AS created_at,
		       CASE
		           WHEN p.wr_last IS NULL OR p.wr_last = '' OR p.wr_last = '0000-00-00 00:00:00' THEN p.wr_datetime
		           ELSE p.wr_last
		       END AS updated_at
		  FROM %s p
		 WHERE p.wr_id = ?
		   AND p.wr_is_comment = 0
		 LIMIT 1`, tableName)
	if err := s.db.Raw(query, wrID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == 0 || row.MemberID == "" {
		return nil
	}

	isPublic := !row.IsDeleted &&
		!strings.Contains(strings.ToLower(row.WrOption), "secret") &&
		s.isBoardSearchable(boardSlug)
	updatedAt := row.UpdatedAt
	affectedMembers := []string{row.MemberID}
	feed := &gnudomain.MemberActivityFeed{
		MemberID:        row.MemberID,
		BoardID:         boardSlug,
		WriteTable:      tableName,
		WriteID:         row.ID,
		ActivityType:    1,
		IsPublic:        isPublic,
		IsDeleted:       row.IsDeleted,
		Title:           truncateForFeed(row.Title, 255),
		ContentPreview:  buildContentPreview(row.Content, 200),
		AuthorName:      row.Author,
		WrOption:        truncateForFeed(row.WrOption, 255),
		ViewCount:       row.ViewCount,
		LikeCount:       row.LikeCount,
		DislikeCount:    row.DislikeCount,
		CommentCount:    row.CommentCount,
		HasFile:         row.HasFile,
		SourceCreatedAt: row.CreatedAt,
		SourceUpdatedAt: &updatedAt,
	}
	if err := s.upsertFeed(feed); err != nil {
		return err
	}

	if err := s.db.Table("member_activity_feed").
		Where("write_table = ? AND parent_write_id = ? AND activity_type = 2", tableName, wrID).
		Updates(map[string]interface{}{
			"parent_title":      truncateForFeed(row.Title, 255),
			"is_public":         boolToInt(isPublic),
			"source_updated_at": updatedAt,
		}).Error; err != nil {
		return err
	}
	if members, err := s.memberIDsForParent(tableName, wrID); err == nil {
		affectedMembers = append(affectedMembers, members...)
	}

	if !rebuildStats {
		return nil
	}
	return s.rebuildMemberBoardStats(boardSlug, uniqueNonEmptyStrings(affectedMembers))
}

func (s *MemberActivitySyncService) SyncLegacyComment(boardSlug string, wrID int) error {
	return s.syncLegacyComment(boardSlug, wrID, true)
}

func (s *MemberActivitySyncService) syncLegacyComment(boardSlug string, wrID int, rebuildStats bool) error {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return err
	}

	type commentRow struct {
		ID           int       `gorm:"column:id"`
		ParentID     int       `gorm:"column:parent_id"`
		MemberID     string    `gorm:"column:member_id"`
		Author       string    `gorm:"column:author_name"`
		Content      string    `gorm:"column:content"`
		ParentTitle  string    `gorm:"column:parent_title"`
		ParentOpt    string    `gorm:"column:parent_option"`
		LikeCount    int       `gorm:"column:like_count"`
		DislikeCount int       `gorm:"column:dislike_count"`
		IsDeleted    bool      `gorm:"column:is_deleted"`
		ParentGone   bool      `gorm:"column:parent_deleted"`
		CreatedAt    time.Time `gorm:"column:created_at"`
		UpdatedAt    time.Time `gorm:"column:updated_at"`
	}

	var row commentRow
	query := fmt.Sprintf(`
		SELECT c.wr_id AS id,
		       c.wr_parent AS parent_id,
		       COALESCE(c.mb_id, '') AS member_id,
		       COALESCE(c.wr_name, '') AS author_name,
		       COALESCE(c.wr_content, '') AS content,
		       COALESCE(p.wr_subject, '') AS parent_title,
		       COALESCE(p.wr_option, '') AS parent_option,
		       COALESCE(c.wr_good, 0) AS like_count,
		       COALESCE(c.wr_nogood, 0) AS dislike_count,
		       (c.wr_deleted_at IS NOT NULL) AS is_deleted,
		       (p.wr_deleted_at IS NOT NULL) AS parent_deleted,
		       c.wr_datetime AS created_at,
		       CASE
		           WHEN c.wr_last IS NULL OR c.wr_last = '' OR c.wr_last = '0000-00-00 00:00:00' THEN c.wr_datetime
		           ELSE c.wr_last
		       END AS updated_at
		  FROM %s c
		  JOIN %s p ON p.wr_id = c.wr_parent AND p.wr_is_comment = 0
		 WHERE c.wr_id = ?
		   AND c.wr_is_comment = 1
		 LIMIT 1`, tableName, tableName)
	if err := s.db.Raw(query, wrID).Scan(&row).Error; err != nil {
		return err
	}
	if row.ID == 0 || row.MemberID == "" {
		return nil
	}

	isPublic := !row.IsDeleted &&
		!row.ParentGone &&
		!strings.Contains(strings.ToLower(row.ParentOpt), "secret") &&
		s.isBoardSearchable(boardSlug)
	updatedAt := row.UpdatedAt
	parentID := row.ParentID
	feed := &gnudomain.MemberActivityFeed{
		MemberID:        row.MemberID,
		BoardID:         boardSlug,
		WriteTable:      tableName,
		WriteID:         row.ID,
		ParentWriteID:   &parentID,
		ActivityType:    2,
		IsPublic:        isPublic,
		IsDeleted:       row.IsDeleted,
		ContentPreview:  buildContentPreview(row.Content, 200),
		ParentTitle:     truncateForFeed(row.ParentTitle, 255),
		AuthorName:      row.Author,
		LikeCount:       row.LikeCount,
		DislikeCount:    row.DislikeCount,
		SourceCreatedAt: row.CreatedAt,
		SourceUpdatedAt: &updatedAt,
	}
	if err := s.upsertFeed(feed); err != nil {
		return err
	}

	if !rebuildStats {
		return nil
	}
	return s.rebuildMemberBoardStats(boardSlug, []string{row.MemberID})
}

func (s *MemberActivitySyncService) RemoveLegacyPost(boardSlug string, wrID int) error {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return err
	}
	memberIDs, err := s.memberIDsForWriteOrParent(tableName, wrID)
	if err != nil {
		return err
	}
	if err := s.db.Where("write_table = ? AND (write_id = ? OR parent_write_id = ?)", tableName, wrID, wrID).
		Delete(&gnudomain.MemberActivityFeed{}).Error; err != nil {
		return err
	}
	return s.rebuildMemberBoardStats(boardSlug, memberIDs)
}

func (s *MemberActivitySyncService) RemoveLegacyComment(boardSlug string, wrID int) error {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return err
	}
	memberIDs, err := s.memberIDsForWriteOrParent(tableName, wrID)
	if err != nil {
		return err
	}
	if err := s.db.Where("write_table = ? AND write_id = ? AND activity_type = 2", tableName, wrID).
		Delete(&gnudomain.MemberActivityFeed{}).Error; err != nil {
		return err
	}
	return s.rebuildMemberBoardStats(boardSlug, memberIDs)
}

func (s *MemberActivitySyncService) BackfillLegacyBoard(boardSlug string, batchSize int) (*MemberActivityBackfillReport, error) {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return nil, err
	}
	if batchSize <= 0 {
		batchSize = 500
	}

	report := &MemberActivityBackfillReport{Scope: "legacy", BoardSlug: boardSlug}
	if err := s.db.Table(tableName).Where("wr_is_comment = 0 AND mb_id <> ''").Count(&report.PostCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table(tableName).Where("wr_is_comment = 1 AND mb_id <> ''").Count(&report.CommentCount).Error; err != nil {
		return nil, err
	}

	var lastID int
	for {
		var postIDs []int
		if err := s.db.Table(tableName).
			Select("wr_id").
			Where("wr_is_comment = 0 AND mb_id <> '' AND wr_id > ?", lastID).
			Order("wr_id ASC").
			Limit(batchSize).
			Scan(&postIDs).Error; err != nil {
			return nil, err
		}
		if len(postIDs) == 0 {
			break
		}
		for _, id := range postIDs {
			if err := s.syncLegacyPost(boardSlug, id, false); err != nil {
				return nil, err
			}
			report.ProcessedPosts++
			lastID = id
		}
	}

	lastID = 0
	for {
		var commentIDs []int
		if err := s.db.Table(tableName).
			Select("wr_id").
			Where("wr_is_comment = 1 AND mb_id <> '' AND wr_id > ?", lastID).
			Order("wr_id ASC").
			Limit(batchSize).
			Scan(&commentIDs).Error; err != nil {
			return nil, err
		}
		if len(commentIDs) == 0 {
			break
		}
		for _, id := range commentIDs {
			if err := s.syncLegacyComment(boardSlug, id, false); err != nil {
				return nil, err
			}
			report.ProcessedComments++
			lastID = id
		}
	}

	return report, s.rebuildBoardStats(boardSlug)
}

func (s *MemberActivitySyncService) BackfillV2(batchSize int) (*MemberActivityBackfillReport, error) {
	if batchSize <= 0 {
		batchSize = 500
	}
	report := &MemberActivityBackfillReport{Scope: "v2"}

	if err := s.db.Table("v2_posts p").
		Joins("JOIN g5_member m ON m.mb_no = p.user_id").
		Where("COALESCE(m.mb_id, '') <> ''").
		Count(&report.PostCount).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("v2_comments c").
		Joins("JOIN g5_member m ON m.mb_no = c.user_id").
		Where("COALESCE(m.mb_id, '') <> ''").
		Count(&report.CommentCount).Error; err != nil {
		return nil, err
	}

	boardTouched := map[string]struct{}{}
	var lastID uint64
	for {
		var postIDs []uint64
		if err := s.db.Table("v2_posts").
			Select("id").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batchSize).
			Scan(&postIDs).Error; err != nil {
			return nil, err
		}
		if len(postIDs) == 0 {
			break
		}
		for _, id := range postIDs {
			boardSlug, err := s.syncPostWithoutStats(id)
			if err != nil {
				return nil, err
			}
			if boardSlug != "" {
				boardTouched[boardSlug] = struct{}{}
				report.ProcessedPosts++
			}
			lastID = id
		}
	}

	lastID = 0
	for {
		var commentIDs []uint64
		if err := s.db.Table("v2_comments").
			Select("id").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batchSize).
			Scan(&commentIDs).Error; err != nil {
			return nil, err
		}
		if len(commentIDs) == 0 {
			break
		}
		for _, id := range commentIDs {
			boardSlug, err := s.syncCommentWithoutStats(id)
			if err != nil {
				return nil, err
			}
			if boardSlug != "" {
				boardTouched[boardSlug] = struct{}{}
				report.ProcessedComments++
			}
			lastID = id
		}
	}

	for boardSlug := range boardTouched {
		if err := s.rebuildBoardStats(boardSlug); err != nil {
			return nil, err
		}
	}

	return report, nil
}

func (s *MemberActivitySyncService) VerifyLegacyBoard(boardSlug string) (*MemberActivityVerifyReport, error) {
	tableName, err := legacyWriteTableName(boardSlug)
	if err != nil {
		return nil, err
	}
	report := &MemberActivityVerifyReport{Scope: "legacy", BoardSlug: boardSlug}

	if err := s.db.Table(tableName).Where("wr_is_comment = 0 AND mb_id <> ''").Count(&report.SourcePosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table(tableName).Where("wr_is_comment = 1 AND mb_id <> ''").Count(&report.SourceComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table(tableName).Where("wr_is_comment = 0 AND mb_id <> '' AND wr_deleted_at IS NOT NULL").Count(&report.SourceDeletedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table(tableName).Where("wr_is_comment = 1 AND mb_id <> '' AND wr_deleted_at IS NOT NULL").Count(&report.SourceDeletedComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND board_id = ? AND activity_type = 1", tableName, boardSlug).Count(&report.FeedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND board_id = ? AND activity_type = 2", tableName, boardSlug).Count(&report.FeedComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND board_id = ? AND activity_type = 1 AND is_deleted = 1", tableName, boardSlug).Count(&report.FeedDeletedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND board_id = ? AND activity_type = 2 AND is_deleted = 1", tableName, boardSlug).Count(&report.FeedDeletedComments).Error; err != nil {
		return nil, err
	}

	return report, nil
}

func (s *MemberActivitySyncService) VerifyV2() (*MemberActivityVerifyReport, error) {
	report := &MemberActivityVerifyReport{Scope: "v2"}
	if err := s.db.Table("v2_posts p").
		Joins("JOIN g5_member m ON m.mb_no = p.user_id").
		Where("COALESCE(m.mb_id, '') <> ''").
		Count(&report.SourcePosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("v2_comments c").
		Joins("JOIN g5_member m ON m.mb_no = c.user_id").
		Where("COALESCE(m.mb_id, '') <> ''").
		Count(&report.SourceComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("v2_posts p").
		Joins("JOIN g5_member m ON m.mb_no = p.user_id").
		Where("COALESCE(m.mb_id, '') <> '' AND p.status = ?", "deleted").
		Count(&report.SourceDeletedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("v2_comments c").
		Joins("JOIN g5_member m ON m.mb_no = c.user_id").
		Where("COALESCE(m.mb_id, '') <> '' AND c.status = ?", "deleted").
		Count(&report.SourceDeletedComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND activity_type = 1", "v2_posts").Count(&report.FeedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND activity_type = 2", "v2_comments").Count(&report.FeedComments).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND activity_type = 1 AND is_deleted = 1", "v2_posts").Count(&report.FeedDeletedPosts).Error; err != nil {
		return nil, err
	}
	if err := s.db.Table("member_activity_feed").Where("write_table = ? AND activity_type = 2 AND is_deleted = 1", "v2_comments").Count(&report.FeedDeletedComments).Error; err != nil {
		return nil, err
	}
	return report, nil
}

func (s *MemberActivitySyncService) upsertFeed(feed *gnudomain.MemberActivityFeed) error {
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "write_table"}, {Name: "write_id"}, {Name: "activity_type"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"member_id",
			"board_id",
			"parent_write_id",
			"is_public",
			"is_deleted",
			"title",
			"content_preview",
			"parent_title",
			"author_name",
			"wr_option",
			"view_count",
			"like_count",
			"dislike_count",
			"comment_count",
			"has_file",
			"source_created_at",
			"source_updated_at",
			"updated_at",
		}),
	}).Create(feed).Error
}

func (s *MemberActivitySyncService) rebuildBoardStats(boardSlug string) error {
	if err := s.db.Exec("DELETE FROM member_activity_stats WHERE board_id = ?", boardSlug).Error; err != nil {
		return err
	}

	return s.db.Exec(`
		INSERT INTO member_activity_stats
		    (member_id, board_id, post_count, comment_count, public_post_count, public_comment_count)
		SELECT
		    member_id,
		    board_id,
		    SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 THEN 1 ELSE 0 END) AS post_count,
		    SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 THEN 1 ELSE 0 END) AS comment_count,
		    SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END) AS public_post_count,
		    SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END) AS public_comment_count
		  FROM member_activity_feed
		 WHERE board_id = ?
		 GROUP BY member_id, board_id`, boardSlug).Error
}

func (s *MemberActivitySyncService) rebuildMemberBoardStats(boardSlug string, memberIDs []string) error {
	memberIDs = uniqueNonEmptyStrings(memberIDs)
	if len(memberIDs) == 0 {
		return nil
	}
	if err := s.db.Where("board_id = ? AND member_id IN ?", boardSlug, memberIDs).
		Delete(&gnudomain.MemberActivityStatsRow{}).Error; err != nil {
		return err
	}
	return s.db.Exec(`
		INSERT INTO member_activity_stats
		    (member_id, board_id, post_count, comment_count, public_post_count, public_comment_count)
		SELECT
		    member_id,
		    board_id,
		    SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 THEN 1 ELSE 0 END) AS post_count,
		    SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 THEN 1 ELSE 0 END) AS comment_count,
		    SUM(CASE WHEN activity_type = 1 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END) AS public_post_count,
		    SUM(CASE WHEN activity_type = 2 AND is_deleted = 0 AND is_public = 1 THEN 1 ELSE 0 END) AS public_comment_count
		  FROM member_activity_feed
		 WHERE board_id = ?
		   AND member_id IN ?
		 GROUP BY member_id, board_id`, boardSlug, memberIDs).Error
}

func (s *MemberActivitySyncService) isBoardSearchable(boardSlug string) bool {
	var boUseSearch int
	if err := s.db.Table("g5_board").
		Select("bo_use_search").
		Where("bo_table = ?", boardSlug).
		Scan(&boUseSearch).Error; err != nil {
		return false
	}
	return boUseSearch == 1
}

func buildContentPreview(content string, maxLen int) string {
	s := memberActivityHTMLTagRe.ReplaceAllString(content, "")
	s = memberActivityEmoRe.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = memberActivityWSRe.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return truncateForFeed(s, maxLen)
}

func truncateForFeed(s string, maxLen int) string {
	runes := []rune(strings.TrimSpace(s))
	if len(runes) <= maxLen {
		return string(runes)
	}
	return string(runes[:maxLen])
}

func wrOptionFromSecret(secret bool) string {
	if secret {
		return "secret"
	}
	return ""
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *MemberActivitySyncService) syncPostWithoutStats(postID uint64) (string, error) {
	boardSlug, err := s.boardSlugForV2Post(postID)
	if err != nil || boardSlug == "" {
		return boardSlug, err
	}
	return boardSlug, s.syncPost(postID, false)
}

func (s *MemberActivitySyncService) syncCommentWithoutStats(commentID uint64) (string, error) {
	boardSlug, err := s.boardSlugForV2Comment(commentID)
	if err != nil || boardSlug == "" {
		return boardSlug, err
	}
	return boardSlug, s.syncComment(commentID, false)
}

func (s *MemberActivitySyncService) boardSlugForV2Post(postID uint64) (string, error) {
	var boardSlug string
	err := s.db.Raw(`
		SELECT b.slug
		  FROM v2_posts p
		  JOIN v2_boards b ON b.id = p.board_id
		 WHERE p.id = ?
		 LIMIT 1`, postID).Scan(&boardSlug).Error
	return boardSlug, err
}

func (s *MemberActivitySyncService) boardSlugForV2Comment(commentID uint64) (string, error) {
	var boardSlug string
	err := s.db.Raw(`
		SELECT b.slug
		  FROM v2_comments c
		  JOIN v2_posts p ON p.id = c.post_id
		  JOIN v2_boards b ON b.id = p.board_id
		 WHERE c.id = ?
		 LIMIT 1`, commentID).Scan(&boardSlug).Error
	return boardSlug, err
}

func legacyWriteTableName(boardSlug string) (string, error) {
	if !memberActivitySlugRe.MatchString(boardSlug) {
		return "", fmt.Errorf("invalid board slug: %q", boardSlug)
	}
	return "g5_write_" + boardSlug, nil
}

func (s *MemberActivitySyncService) memberIDsForParent(writeTable string, parentWriteID int) ([]string, error) {
	return s.memberIDsByFeedWhere("write_table = ? AND parent_write_id = ? AND activity_type = 2", writeTable, parentWriteID)
}

func (s *MemberActivitySyncService) memberIDsForWriteOrParent(writeTable string, writeID int) ([]string, error) {
	return s.memberIDsByFeedWhere("write_table = ? AND (write_id = ? OR parent_write_id = ?)", writeTable, writeID, writeID)
}

func (s *MemberActivitySyncService) memberIDsByFeedWhere(where string, args ...interface{}) ([]string, error) {
	var memberIDs []string
	err := s.db.Model(&gnudomain.MemberActivityFeed{}).
		Distinct("member_id").
		Where(where, args...).
		Pluck("member_id", &memberIDs).Error
	return uniqueNonEmptyStrings(memberIDs), err
}

func uniqueNonEmptyStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

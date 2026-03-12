package gnuboard

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/pkg/sphinx"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// postCountCache caches COUNT(*) results for large boards to avoid expensive full-index scans.
// TTL: 30 seconds. Invalidated on write operations (create/delete/restore).
var postCountCache sync.Map

type cachedCount struct {
	total     int64
	expiresAt time.Time
}

const countCacheTTL = 30 * time.Second

// sortFieldCache caches bo_sort_field per board (60s TTL)
// Eliminates extra g5_board query on every post list request
var sortFieldCache sync.Map

type cachedSortField struct {
	field     string
	expiresAt time.Time
}

const sortFieldCacheTTL = 60 * time.Second

// coreColumns are the columns that exist in all g5_write_* tables
var coreColumns = []string{
	"wr_id", "wr_num", "wr_reply", "wr_parent", "wr_is_comment",
	"wr_comment", "wr_comment_reply", "ca_name", "wr_option",
	"wr_subject", "wr_content", "wr_link1", "wr_link2",
	"wr_link1_hit", "wr_link2_hit", "wr_hit", "wr_good", "wr_nogood",
	"mb_id", "wr_password", "wr_name", "wr_email", "wr_homepage",
	"wr_datetime", "wr_file", "wr_last", "wr_ip",
	"wr_10",                          // 이미지 URL (갤러리/메시지 썸네일)
	"wr_deleted_at", "wr_deleted_by", // Soft delete columns (마이그레이션된 테이블만)
}

// WriteRepository provides access to g5_write_* dynamic tables
type WriteRepository interface {
	// Posts
	FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsAfter(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error)
	FindPostsFiltered(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error)
	SearchPosts(boardID string, searchField, searchQuery string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	SearchPostsFiltered(boardID string, searchField, searchQuery string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error)
	FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error)
	FindDeletedPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	CreatePost(boardID string, post *gnuboard.G5Write) error
	UpdatePost(boardID string, post *gnuboard.G5Write) error
	DeletePost(boardID string, wrID int) error
	SoftDeletePost(boardID string, wrID int, deletedBy string) error
	RestorePost(boardID string, wrID int) error
	IncrementHit(boardID string, wrID int) error

	// Comments
	FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error)
	FindCommentsFiltered(boardID string, parentID int, excludeMbIDs []string) ([]*gnuboard.G5Write, error)
	FindCommentsIncludeDeleted(boardID string, parentID int) ([]*gnuboard.G5Write, error)
	FindCommentByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	CreateComment(boardID string, comment *gnuboard.G5Write) error
	DeleteComment(boardID string, wrID int) error
	SoftDeleteComment(boardID string, wrID int, deletedBy string) error
	RestoreComment(boardID string, wrID int) error

	// Counting
	CountCommentReplies(boardID string, parentID int, commentID int) (int64, error)

	// Utility
	TableExists(boardID string) bool
	GetNextWrNum(boardID string) (int, error)
}

type writeRepository struct {
	db     *gorm.DB
	sphinx *sphinx.Client
	redis  *redis.Client
}

// NewWriteRepository creates a new Gnuboard WriteRepository
func NewWriteRepository(db *gorm.DB) WriteRepository {
	return &writeRepository{db: db}
}

// NewWriteRepositoryWithSphinx creates a WriteRepository with Sphinx search support.
func NewWriteRepositoryWithSphinx(db *gorm.DB, sphinxClient *sphinx.Client) WriteRepository {
	return &writeRepository{db: db, sphinx: sphinxClient}
}

// NewWriteRepositoryWithRedis creates a WriteRepository with Redis for cross-pod COUNT cache sharing.
func NewWriteRepositoryWithRedis(db *gorm.DB, redisClient *redis.Client) WriteRepository {
	return &writeRepository{db: db, redis: redisClient}
}

// NewWriteRepositoryFull creates a WriteRepository with both Sphinx and Redis.
func NewWriteRepositoryFull(db *gorm.DB, sphinxClient *sphinx.Client, redisClient *redis.Client) WriteRepository {
	return &writeRepository{db: db, sphinx: sphinxClient, redis: redisClient}
}

// tableName generates the dynamic table name for a board
func tableName(boardID string) string {
	return fmt.Sprintf("g5_write_%s", boardID)
}

// getSortField returns the sort clause for a board (with caching)
func (r *writeRepository) getSortField(boardID string) string {
	orderClause := "wr_num, wr_reply"
	now := time.Now()
	if cached, ok := sortFieldCache.Load(boardID); ok {
		if entry, valid := cached.(*cachedSortField); valid && now.Before(entry.expiresAt) {
			if entry.field != "" {
				return entry.field
			}
			return orderClause
		}
		sortFieldCache.Delete(boardID)
	}
	var sortField string
	r.db.Table("g5_board").Select("bo_sort_field").Where("bo_table = ?", boardID).Scan(&sortField)
	sortFieldCache.Store(boardID, &cachedSortField{field: sortField, expiresAt: now.Add(sortFieldCacheTTL)})
	if sortField != "" {
		return sortField
	}
	return orderClause
}

// FindPosts retrieves posts (not comments, not deleted) from a board with pagination
func (r *writeRepository) FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	// Posts count with Redis cache (shared across all pods)
	total = r.getCachedPostCount(boardID)
	if total == 0 {
		countQuery := r.db.Table(table).Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')")
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	// 게시판별 커스텀 정렬 (bo_sort_field) — 캐시 사용
	orderClause := "wr_num, wr_reply"
	now := time.Now()
	if cached, ok := sortFieldCache.Load(boardID); ok {
		if entry, valid := cached.(*cachedSortField); valid && now.Before(entry.expiresAt) {
			if entry.field != "" {
				orderClause = entry.field
			}
		} else {
			sortFieldCache.Delete(boardID)
		}
	}
	if orderClause == "wr_num, wr_reply" {
		var sortField string
		r.db.Table("g5_board").Select("bo_sort_field").Where("bo_table = ?", boardID).Scan(&sortField)
		sortFieldCache.Store(boardID, &cachedSortField{field: sortField, expiresAt: now.Add(sortFieldCacheTTL)})
		if sortField != "" {
			orderClause = sortField
		}
	}

	// Select only core columns to avoid errors with missing columns
	// Use FORCE INDEX for default sort order — MySQL optimizer incorrectly prefers idx_list_order
	// over idx_list_page, causing 1M+ row scans instead of 15K (verified with EXPLAIN)
	if orderClause == "wr_num, wr_reply" {
		selectCols := strings.Join(coreColumns, ", ")
		err := r.db.Raw(
			fmt.Sprintf("SELECT %s FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?", selectCols, table),
			limit, offset,
		).Scan(&posts).Error
		// Fallback if idx_list_page doesn't exist on this table
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(coreColumns).
				Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')").
				Order(orderClause).
				Offset(offset).
				Limit(limit).
				Find(&posts).Error
		}
		return posts, total, err
	}

	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')").
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// FindPostsAfter retrieves posts using cursor pagination for the default gnuboard sort.
// Cursor is the last seen (wr_num, wr_reply) pair from the previous page.
func (r *writeRepository) FindPostsAfter(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	table := tableName(boardID)

	total = r.getCachedPostCount(boardID)
	if total == 0 {
		countQuery := r.db.Table(table).Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')")
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	orderClause := r.getSortField(boardID)
	if orderClause != "wr_num, wr_reply" {
		return r.FindPosts(boardID, 1, limit)
	}

	selectCols := strings.Join(coreColumns, ", ")
	err := r.db.Raw(
		fmt.Sprintf(
			"SELECT %s FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') AND (wr_num > ? OR (wr_num = ? AND wr_reply > ?)) ORDER BY wr_num, wr_reply LIMIT ?",
			selectCols,
			table,
		),
		cursorWrNum, cursorWrNum, cursorWrReply, limit,
	).Scan(&posts).Error
	if err != nil && strings.Contains(err.Error(), "idx_list_page") {
		err = r.db.Table(table).
			Select(coreColumns).
			Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') AND (wr_num > ? OR (wr_num = ? AND wr_reply > ?))", cursorWrNum, cursorWrNum, cursorWrReply).
			Order(orderClause).
			Limit(limit).
			Find(&posts).Error
	}

	return posts, total, err
}

// FindPostsFiltered retrieves posts excluding specified members. Delegates to FindPosts if excludeMbIDs is empty.
// Uses the same cached count as FindPosts (차단 유저 수가 적어 total 차이 무시 가능).
func (r *writeRepository) FindPostsFiltered(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error) {
	if len(excludeMbIDs) == 0 {
		return r.FindPosts(boardID, page, limit)
	}

	var posts []*gnuboard.G5Write
	offset := (page - 1) * limit
	table := tableName(boardID)

	// Reuse cached total count (same as FindPosts — avoids expensive COUNT on large tables)
	total := r.getCachedPostCount(boardID)
	if total == 0 {
		if err := r.db.Table(table).Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')").Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	orderClause := r.getSortField(boardID)

	// Use FORCE INDEX for default sort order (same as FindPosts)
	if orderClause == "wr_num, wr_reply" {
		selectCols := strings.Join(coreColumns, ", ")
		err := r.db.Raw(
			fmt.Sprintf("SELECT %s FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') AND mb_id NOT IN ? ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?", selectCols, table),
			excludeMbIDs, limit, offset,
		).Scan(&posts).Error
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(coreColumns).
				Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') AND mb_id NOT IN ?", excludeMbIDs).
				Order(orderClause).
				Offset(offset).
				Limit(limit).
				Find(&posts).Error
		}
		return posts, total, err
	}

	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00') AND mb_id NOT IN ?", excludeMbIDs).
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// SearchPostsFiltered retrieves posts matching search criteria excluding specified members.
// Uses Sphinx for search, then filters out excluded members from results.
func (r *writeRepository) SearchPostsFiltered(boardID string, searchField, searchQuery string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error) {
	if len(excludeMbIDs) == 0 {
		return r.SearchPosts(boardID, searchField, searchQuery, page, limit)
	}

	// Sphinx로 검색 후 차단 유저 필터링
	if r.sphinx == nil {
		return nil, 0, fmt.Errorf("검색 서비스를 일시적으로 사용할 수 없습니다")
	}

	// 차단 유저 필터를 위해 여유분 조회 (최대 2배)
	result, err := r.sphinx.Search(boardID, searchField, searchQuery, page, limit*2)
	if err != nil {
		return nil, 0, fmt.Errorf("검색 서비스 오류: %w", err)
	}
	if result == nil || len(result.IDs) == 0 {
		var total int64
		if result != nil {
			total = result.TotalFound
		}
		return nil, total, nil
	}

	// Fetch full post data and filter excluded members
	var posts []*gnuboard.G5Write
	table := tableName(boardID)
	if err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_id IN ? AND mb_id NOT IN ? AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", result.IDs, excludeMbIDs).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	// Reorder posts to match Sphinx result order
	postMap := make(map[int]*gnuboard.G5Write, len(posts))
	for _, p := range posts {
		postMap[p.WrID] = p
	}
	ordered := make([]*gnuboard.G5Write, 0, len(result.IDs))
	for _, id := range result.IDs {
		if p, ok := postMap[id]; ok {
			ordered = append(ordered, p)
		}
	}

	// limit 적용
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}

	return ordered, result.TotalFound, nil
}

// SearchPosts retrieves posts matching search criteria (sfl/stx) with pagination.
// Requires Sphinx full-text search. Returns error if Sphinx is unavailable.
func (r *writeRepository) SearchPosts(boardID string, searchField, searchQuery string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	if r.sphinx == nil {
		return nil, 0, fmt.Errorf("검색 서비스를 일시적으로 사용할 수 없습니다")
	}

	result, err := r.sphinx.Search(boardID, searchField, searchQuery, page, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("검색 서비스 오류: %w", err)
	}
	if result == nil || len(result.IDs) == 0 {
		var total int64
		if result != nil {
			total = result.TotalFound
		}
		return nil, total, nil
	}

	// Fetch full post data from MySQL by IDs (preserving Sphinx order)
	var posts []*gnuboard.G5Write
	table := tableName(boardID)
	if err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_id IN ? AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", result.IDs).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}
	// Reorder posts to match Sphinx result order
	postMap := make(map[int]*gnuboard.G5Write, len(posts))
	for _, p := range posts {
		postMap[p.WrID] = p
	}
	ordered := make([]*gnuboard.G5Write, 0, len(result.IDs))
	for _, id := range result.IDs {
		if p, ok := postMap[id]; ok {
			ordered = append(ordered, p)
		}
	}
	return ordered, result.TotalFound, nil
}

// FindPostByID retrieves a single post by ID (excludes soft deleted)
func (r *writeRepository) FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", wrID).
		First(&post).Error
	return &post, err
}

// FindPostByIDIncludeDeleted retrieves a single post by ID including soft deleted posts
func (r *writeRepository) FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 0", wrID).
		First(&post).Error
	return &post, err
}

// FindNotices retrieves notice posts by their IDs (excludes soft deleted)
func (r *writeRepository) FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error) {
	if len(noticeIDs) == 0 {
		return []*gnuboard.G5Write{}, nil
	}

	var notices []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id IN ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", noticeIDs).
		Order("wr_num, wr_reply").
		Find(&notices).Error
	return notices, err
}

// FindDeletedPosts retrieves soft deleted posts from a board with pagination (admin use)
func (r *writeRepository) FindDeletedPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	countQuery := r.db.Table(table).Where("wr_is_comment = 0 AND wr_deleted_at IS NOT NULL")
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Table(table).
		Select(coreColumns).
		Where("wr_is_comment = 0 AND wr_deleted_at IS NOT NULL").
		Order("wr_deleted_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// postCountRedisKey returns the Redis key for post count cache
func postCountRedisKey(boardID string) string {
	return "postcount:" + boardID
}

// getCachedPostCount tries Redis first, then falls back to in-memory cache
func (r *writeRepository) getCachedPostCount(boardID string) int64 {
	// Try Redis first (shared across all pods)
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		val, err := r.redis.Get(ctx, postCountRedisKey(boardID)).Result()
		if err == nil {
			if count, err := strconv.ParseInt(val, 10, 64); err == nil {
				return count
			}
		}
	}

	// Fallback to in-memory cache
	cacheKey := "count:" + boardID
	if cached, ok := postCountCache.Load(cacheKey); ok {
		if cc, ok2 := cached.(*cachedCount); ok2 && time.Now().Before(cc.expiresAt) {
			return cc.total
		}
	}
	return 0
}

// setCachedPostCount stores count in both Redis (shared) and in-memory (local fallback)
func (r *writeRepository) setCachedPostCount(boardID string, total int64) {
	// Store in Redis (shared across pods)
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		r.redis.Set(ctx, postCountRedisKey(boardID), total, countCacheTTL)
	}

	// Also store in local memory as fallback
	postCountCache.Store("count:"+boardID, &cachedCount{total: total, expiresAt: time.Now().Add(countCacheTTL)})
}

// invalidatePostCount clears the cached post count for a board from both Redis and memory
func (r *writeRepository) invalidatePostCount(boardID string) {
	// Invalidate Redis
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		r.redis.Del(ctx, postCountRedisKey(boardID))
	}

	// Invalidate in-memory
	postCountCache.Delete("count:" + boardID)
}

// InvalidatePostCount clears the cached post count from in-memory cache (legacy, no Redis)
func InvalidatePostCount(boardID string) {
	postCountCache.Delete("count:" + boardID)
}

// CreatePost creates a new post
func (r *writeRepository) CreatePost(boardID string, post *gnuboard.G5Write) error {
	r.invalidatePostCount(boardID)
	return r.db.Table(tableName(boardID)).Create(post).Error
}

// UpdatePost updates an existing post
func (r *writeRepository) UpdatePost(boardID string, post *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Save(post).Error
}

// DeletePost permanently deletes a post and its comments from the database
func (r *writeRepository) DeletePost(boardID string, wrID int) error {
	r.invalidatePostCount(boardID)
	table := tableName(boardID)
	// Delete comments first
	if err := r.db.Table(table).Where("wr_parent = ?", wrID).Delete(&gnuboard.G5Write{}).Error; err != nil {
		return err
	}
	// Delete the post
	return r.db.Table(table).Where("wr_id = ?", wrID).Delete(&gnuboard.G5Write{}).Error
}

// SoftDeletePost marks a post and its comments as deleted, and records revision history
func (r *writeRepository) SoftDeletePost(boardID string, wrID int, deletedBy string) error {
	r.invalidatePostCount(boardID)
	table := tableName(boardID)
	now := time.Now()

	// Record revision before deletion (g5_write_revisions)
	var post struct {
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
		WrName    string `gorm:"column:wr_name"`
	}
	if err := r.db.Table(table).Select("wr_subject, wr_content, wr_name").Where("wr_id = ?", wrID).Scan(&post).Error; err == nil {
		var nextVersion int
		r.db.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM g5_write_revisions WHERE board_id = ? AND wr_id = ?", boardID, wrID).Scan(&nextVersion)
		if err := r.db.Exec(`INSERT INTO g5_write_revisions
			(board_id, wr_id, version, change_type, title, content, edited_by, edited_by_name, edited_at)
			VALUES (?, ?, ?, 'soft_delete', ?, ?, ?, ?, ?)`,
			boardID, wrID, nextVersion, post.WrSubject, post.WrContent, deletedBy, post.WrName, now,
		).Error; err != nil {
			log.Printf("[SoftDeletePost] Failed to record revision for %s/%d: %v", boardID, wrID, err)
		}
	}

	// Soft delete the post only (comments are preserved)
	return r.db.Table(table).Where("wr_id = ?", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": now,
		"wr_deleted_by": deletedBy,
	}).Error
}

// RestorePost restores a soft deleted post (comments are not affected)
func (r *writeRepository) RestorePost(boardID string, wrID int) error {
	r.invalidatePostCount(boardID)
	table := tableName(boardID)

	return r.db.Table(table).Where("wr_id = ?", wrID).Updates(map[string]interface{}{
		"wr_deleted_at": nil,
		"wr_deleted_by": nil,
	}).Error
}

// IncrementHit increments the view count for a post
func (r *writeRepository) IncrementHit(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ?", wrID).
		UpdateColumn("wr_hit", gorm.Expr("wr_hit + 1")).Error
}

// FindComments retrieves all non-deleted comments for a post
func (r *writeRepository) FindComments(boardID string, parentID int) ([]*gnuboard.G5Write, error) {
	var comments []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_parent = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", parentID).
		Order("wr_comment, wr_comment_reply").
		Find(&comments).Error
	return comments, err
}

// FindCommentsFiltered retrieves non-deleted comments excluding specified members. Delegates to FindComments if excludeMbIDs is empty.
func (r *writeRepository) FindCommentsFiltered(boardID string, parentID int, excludeMbIDs []string) ([]*gnuboard.G5Write, error) {
	if len(excludeMbIDs) == 0 {
		return r.FindComments(boardID, parentID)
	}

	var comments []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_parent = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL AND mb_id NOT IN ?", parentID, excludeMbIDs).
		Order("wr_comment, wr_comment_reply").
		Find(&comments).Error
	return comments, err
}

// FindCommentsIncludeDeleted retrieves all comments for a post including soft deleted ones
func (r *writeRepository) FindCommentsIncludeDeleted(boardID string, parentID int) ([]*gnuboard.G5Write, error) {
	var comments []*gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_parent = ? AND wr_is_comment = 1", parentID).
		Order("wr_comment, wr_comment_reply").
		Find(&comments).Error
	return comments, err
}

// FindCommentByID retrieves a single comment by ID
func (r *writeRepository) FindCommentByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var comment gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(coreColumns).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		First(&comment).Error
	return &comment, err
}

// CreateComment creates a new comment
func (r *writeRepository) CreateComment(boardID string, comment *gnuboard.G5Write) error {
	return r.db.Table(tableName(boardID)).Create(comment).Error
}

// DeleteComment permanently deletes a comment from the database
func (r *writeRepository) DeleteComment(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Delete(&gnuboard.G5Write{}).Error
}

// SoftDeleteComment marks a comment as deleted
func (r *writeRepository) SoftDeleteComment(boardID string, wrID int, deletedBy string) error {
	now := time.Now()
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Updates(map[string]interface{}{
			"wr_deleted_at": now,
			"wr_deleted_by": deletedBy,
		}).Error
}

// RestoreComment restores a soft deleted comment
func (r *writeRepository) RestoreComment(boardID string, wrID int) error {
	return r.db.Table(tableName(boardID)).
		Where("wr_id = ? AND wr_is_comment = 1", wrID).
		Updates(map[string]interface{}{
			"wr_deleted_at": nil,
			"wr_deleted_by": nil,
		}).Error
}

// CountCommentReplies counts the number of replies to a specific comment.
// For a comment with wr_comment=X and wr_comment_reply=Y, replies are those
// with the same wr_comment and wr_comment_reply starting with Y (but longer).
func (r *writeRepository) CountCommentReplies(boardID string, parentID int, commentID int) (int64, error) {
	// First get the comment to find its wr_comment and wr_comment_reply
	comment, err := r.FindCommentByID(boardID, commentID)
	if err != nil {
		return 0, err
	}

	var count int64
	query := r.db.Table(tableName(boardID)).
		Where("wr_parent = ? AND wr_is_comment = 1 AND wr_id != ? AND wr_deleted_at IS NULL", parentID, commentID).
		Where("wr_comment = ?", comment.WrComment)

	if comment.WrCommentReply == "" {
		// Top-level comment: all replies under this wr_comment are its replies
		query = query.Where("wr_comment_reply != ''")
	} else {
		// Nested reply: count replies with longer wr_comment_reply starting with this prefix
		query = query.Where("wr_comment_reply LIKE ? AND LENGTH(wr_comment_reply) > ?",
			comment.WrCommentReply+"%", len(comment.WrCommentReply))
	}

	err = query.Count(&count).Error
	return count, err
}

// TableExists checks if the write table exists for a board
func (r *writeRepository) TableExists(boardID string) bool {
	table := tableName(boardID)
	var count int64
	// Check if table exists by querying INFORMATION_SCHEMA
	r.db.Raw("SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_NAME = ?", table).Scan(&count)
	return count > 0
}

// GetNextWrNum gets the next wr_num for a new post (negative, as per Gnuboard convention)
func (r *writeRepository) GetNextWrNum(boardID string) (int, error) {
	var minNum int
	err := r.db.Table(tableName(boardID)).
		Select("COALESCE(MIN(wr_num), 0)").
		Scan(&minNum).Error
	if err != nil {
		return 0, err
	}
	return minNum - 1, nil
}

// ParseNoticeIDs parses the bo_notice string into a slice of post IDs
func ParseNoticeIDs(noticeStr string) []int {
	if noticeStr == "" {
		return []int{}
	}

	parts := strings.Split(noticeStr, ",")
	ids := make([]int, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int
		if _, err := fmt.Sscanf(part, "%d", &id); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}

	return ids
}

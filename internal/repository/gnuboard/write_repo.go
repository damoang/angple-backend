package gnuboard

import (
	"context"
	"encoding/json"
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
const hotBoardCountCacheTTL = 3 * time.Minute

func countCacheTTLForBoard(boardID string) time.Duration {
	switch boardID {
	case "free", "hello":
		return hotBoardCountCacheTTL
	default:
		return countCacheTTL
	}
}

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
	"wr_9",                           // 리포트 통계 JSON 등
	"wr_10",                          // 이미지 URL (갤러리/메시지 썸네일)
	"wr_deleted_at", "wr_deleted_by", // Soft delete columns (마이그레이션된 테이블만)
	"wr_edit_count", "wr_last_edited_at", // 수정 추적 비정규화 (마이그레이션된 테이블만)
}

// WriteRepository provides access to g5_write_* dynamic tables
type WriteRepository interface {
	// Posts
	FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsSummary(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsByCategory(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsByCategorySummary(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsHasNext(boardID string, page, limit int) ([]*gnuboard.G5Write, bool, error)
	FindPostsHasNextSummary(boardID string, page, limit int) ([]*gnuboard.G5Write, bool, error)
	FindPostsByCategoryHasNext(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, bool, error)
	FindPostsByCategoryHasNextSummary(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, bool, error)
	FindPostsFilteredHasNext(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error)
	FindPostsFilteredHasNextSummary(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error)
	FindPostsByCategoryFilteredHasNext(boardID string, category string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error)
	FindPostsByCategoryFilteredHasNextSummary(boardID string, category string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error)
	FindMessagePostsByPeriod(period string, today time.Time, page, limit int) ([]*gnuboard.G5Write, int64, error)
	FindPostsAfter(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error)
	FindPostsAfterSummary(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error)
	// FindPostsFromDate(Summary) resolves a date to the archive position and returns the first
	// page from that point (newest-at-or-before the date, then older), for depth-independent
	// date navigation (#12975). Subsequent pages reuse the exclusive cursor (FindPostsAfter).
	FindPostsFromDate(boardID string, limit int, beforeDate string) ([]*gnuboard.G5Write, int64, error)
	FindPostsFromDateSummary(boardID string, limit int, beforeDate string) ([]*gnuboard.G5Write, int64, error)
	FindPostsFiltered(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error)
	FindPostsFilteredSummary(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error)
	SearchPosts(boardID string, searchField, searchQuery string, page, limit int, sortBy ...string) ([]*gnuboard.G5Write, int64, error)
	SearchPostsByCategory(boardID string, searchField, searchQuery, category string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	SearchPostsFiltered(boardID string, searchField, searchQuery string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error)
	FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error)
	FindNotices(boardID string, noticeIDs []int) ([]*gnuboard.G5Write, error)
	FindDeletedPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error)
	CreatePost(boardID string, post *gnuboard.G5Write) error
	UpdatePost(boardID string, post *gnuboard.G5Write) error
	DeletePost(boardID string, wrID int, deletedBy string) error
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

func clampCommentDelta(delta int) interface{} {
	return gorm.Expr(
		"CASE WHEN COALESCE(wr_comment, 0) + ? < 0 THEN 0 ELSE COALESCE(wr_comment, 0) + ? END",
		delta,
		delta,
	)
}

func visibleCommentCountExpr(_ string, alias string) string {
	if alias != "" {
		return alias + ".wr_comment AS wr_comment"
	}
	return "wr_comment"
}

func postSelectColumnsForList(boardID, alias string, includeContent bool) string {
	table := tableName(boardID)
	parts := make([]string, 0, len(coreColumns))
	for _, col := range coreColumns {
		if !includeContent && col == "wr_content" {
			continue
		}
		if col == "wr_comment" {
			parts = append(parts, visibleCommentCountExpr(table, alias))
			continue
		}
		if alias != "" {
			parts = append(parts, alias+"."+col)
			continue
		}
		parts = append(parts, col)
	}
	return strings.Join(parts, ", ")
}

func postSelectColumns(boardID, alias string) string {
	return postSelectColumnsForList(boardID, alias, true)
}

func messageSubjectDateExpr(column string) string {
	trimmed := fmt.Sprintf("TRIM(%s)", column)
	return fmt.Sprintf(
		"COALESCE(STR_TO_DATE(%s, '%%Y.%%m.%%d'), STR_TO_DATE(%s, '%%Y.%%c.%%e'), STR_TO_DATE(%s, '%%Y-%%m-%%d'), STR_TO_DATE(%s, '%%Y-%%c-%%e'))",
		trimmed, trimmed, trimmed, trimmed,
	)
}

// allowedSortColumns is the whitelist for bo_sort_field values
var allowedSortColumns = map[string]bool{
	"wr_num, wr_reply":            true,
	"wr_datetime DESC":            true,
	"wr_hit DESC":                 true,
	"wr_good DESC":                true,
	"wr_good DESC, wr_num":        true,
	"wr_id DESC":                  true,
	"wr_num":                      true,
	"wr_reply":                    true,
	"wr_last DESC":                true,
	"wr_last DESC, wr_num":        true,
	"wr_comment DESC":             true,
	"wr_comment DESC, wr_num":     true,
	"wr_datetime":                 true,
	"wr_num DESC, wr_reply":       true,
	"wr_num ASC, wr_reply":        true,
	"wr_num DESC, wr_reply ASC":   true,
	"wr_subject DESC, wr_id DESC": true,
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
	if sortField != "" && allowedSortColumns[sortField] {
		return sortField
	}
	return orderClause
}

// maxPostOffset caps the OFFSET to prevent extreme tail-latency on deep pages.
// Beyond this offset, the deferred JOIN subquery still scans too many index rows.
const maxPostOffset = 30000

// MaxPostOffset exposes maxPostOffset so handlers can bound the advertised
// pagination range to what is actually reachable. Past this offset the query
// returns the capped (duplicate) page, so callers must stop paginating there
// instead of advertising pages that all show identical content (#12975).
const MaxPostOffset = maxPostOffset

func trimHasNextPosts(posts []*gnuboard.G5Write, limit int) ([]*gnuboard.G5Write, bool) {
	if len(posts) > limit {
		return posts[:limit], true
	}
	return posts, false
}

// FindPosts retrieves posts (not comments) from a board with pagination.
// Soft-deleted posts stay in the list so users can still trace their own activity.
func (r *writeRepository) FindPosts(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	return r.findPosts(boardID, page, limit, true)
}

func (r *writeRepository) FindPostsSummary(boardID string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	return r.findPosts(boardID, page, limit, false)
}

func (r *writeRepository) findPosts(boardID string, page, limit int, includeContent bool) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)

	// Posts count with Redis cache (shared across all pods)
	total = r.getCachedPostCount(boardID)
	if total == 0 {
		countQuery := r.db.Table(table).Where("wr_is_comment = 0")
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	// 게시판별 커스텀 정렬 (bo_sort_field) — 캐시된 단일 조회 사용
	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)

	// Deferred JOIN: subquery scans only wr_id via covering index, then JOIN fetches full rows.
	// This avoids reading wide rows during the OFFSET skip phase (max 119s → ~200ms).
	if orderClause == "wr_num, wr_reply" {
		err := r.db.Raw(
			fmt.Sprintf(
				"SELECT %s FROM `%s` t JOIN (SELECT wr_id FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?) ids ON t.wr_id = ids.wr_id ORDER BY t.wr_num, t.wr_reply",
				postSelectColumnsForList(boardID, "t", includeContent), table, table,
			),
			limit, offset,
		).Scan(&posts).Error
		// Fallback if idx_list_page doesn't exist on this table
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(selectCols).
				Where("wr_is_comment = 0").
				Order(orderClause).
				Offset(offset).
				Limit(limit).
				Find(&posts).Error
		}
		return posts, total, err
	}

	err := r.db.Table(table).
		Select(selectCols).
		Where("wr_is_comment = 0").
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

func (r *writeRepository) FindPostsHasNext(boardID string, page, limit int) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsHasNext(boardID, page, limit, true)
}

func (r *writeRepository) FindPostsHasNextSummary(boardID string, page, limit int) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsHasNext(boardID, page, limit, false)
}

func (r *writeRepository) findPostsHasNext(boardID string, page, limit int, includeContent bool) ([]*gnuboard.G5Write, bool, error) {
	var posts []*gnuboard.G5Write

	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)
	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)
	queryLimit := limit + 1

	if orderClause == "wr_num, wr_reply" {
		err := r.db.Raw(
			fmt.Sprintf(
				"SELECT %s FROM `%s` t JOIN (SELECT wr_id FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?) ids ON t.wr_id = ids.wr_id ORDER BY t.wr_num, t.wr_reply",
				postSelectColumnsForList(boardID, "t", includeContent), table, table,
			),
			queryLimit, offset,
		).Scan(&posts).Error
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(selectCols).
				Where("wr_is_comment = 0").
				Order(orderClause).
				Offset(offset).
				Limit(queryLimit).
				Find(&posts).Error
		}
		trimmed, hasNext := trimHasNextPosts(posts, limit)
		return trimmed, hasNext, err
	}

	err := r.db.Table(table).
		Select(selectCols).
		Where("wr_is_comment = 0").
		Order(orderClause).
		Offset(offset).
		Limit(queryLimit).
		Find(&posts).Error
	trimmed, hasNext := trimHasNextPosts(posts, limit)
	return trimmed, hasNext, err
}

// FindPostsByCategory retrieves posts filtered by ca_name (category) with pagination
func (r *writeRepository) FindPostsByCategory(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsByCategory(boardID, category, page, limit, true)
}

func (r *writeRepository) FindPostsByCategorySummary(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsByCategory(boardID, category, page, limit, false)
}

func (r *writeRepository) findPostsByCategory(boardID string, category string, page, limit int, includeContent bool) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	table := tableName(boardID)

	baseWhere := "wr_is_comment = 0 AND ca_name = ?"

	total = r.getCachedPostCountByCategory(boardID, category)
	if total == 0 {
		if err := r.db.Table(table).Where(baseWhere, category).Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCountByCategory(boardID, category, total)
	}

	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)

	err := r.db.Table(table).
		Select(selectCols).
		Where(baseWhere, category).
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

func (r *writeRepository) FindPostsByCategoryHasNext(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsByCategoryHasNext(boardID, category, page, limit, true)
}

func (r *writeRepository) FindPostsByCategoryHasNextSummary(boardID string, category string, page, limit int) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsByCategoryHasNext(boardID, category, page, limit, false)
}

func (r *writeRepository) findPostsByCategoryHasNext(boardID string, category string, page, limit int, includeContent bool) ([]*gnuboard.G5Write, bool, error) {
	var posts []*gnuboard.G5Write

	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)
	baseWhere := "wr_is_comment = 0 AND ca_name = ?"
	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)
	queryLimit := limit + 1

	err := r.db.Table(table).
		Select(selectCols).
		Where(baseWhere, category).
		Order(orderClause).
		Offset(offset).
		Limit(queryLimit).
		Find(&posts).Error

	trimmed, hasNext := trimHasNextPosts(posts, limit)
	return trimmed, hasNext, err
}

func (r *writeRepository) FindMessagePostsByPeriod(period string, today time.Time, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}

	table := tableName("message")
	selectCols := postSelectColumnsForList("message", "", true)
	dateExpr := messageSubjectDateExpr("wr_subject")
	messageDayExpr := "DATE(wr_datetime)"
	baseWhere := "wr_is_comment = 0"
	args := make([]any, 0, 2)
	orderClause := "wr_id DESC"

	switch period {
	case "month":
		monthStart := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
		nextMonth := monthStart.AddDate(0, 1, 0)
		baseWhere += " AND " + dateExpr + " IS NOT NULL AND " + dateExpr + " >= ? AND " + dateExpr + " < ?"
		args = append(args, monthStart.Format("2006-01-02"), nextMonth.Format("2006-01-02"))
		orderClause = dateExpr + " ASC, wr_id DESC"
	case "upcoming":
		tomorrow := today.AddDate(0, 0, 1)
		baseWhere += " AND " + dateExpr + " IS NOT NULL AND " + dateExpr + " >= ?"
		args = append(args, tomorrow.Format("2006-01-02"))
		orderClause = dateExpr + " ASC, wr_id DESC"
	case "past":
		baseWhere += " AND ((" + dateExpr + " IS NOT NULL AND " + dateExpr + " < ?) OR (" + dateExpr + " IS NULL AND " + messageDayExpr + " < ?))"
		args = append(args, today.Format("2006-01-02"), today.Format("2006-01-02"))
		orderClause = "COALESCE(" + dateExpr + ", " + messageDayExpr + ") DESC, wr_id DESC"
	default:
		baseWhere += " AND " + dateExpr + " IS NOT NULL AND " + dateExpr + " = ?"
		args = append(args, today.Format("2006-01-02"))
	}

	if err := r.db.Table(table).Where(baseWhere, args...).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Table(table).
		Select(selectCols).
		Where(baseWhere, args...).
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// FindPostsAfter retrieves posts using cursor pagination for the default gnuboard sort.
// Cursor is the last seen (wr_num, wr_reply) pair from the previous page.
func (r *writeRepository) FindPostsAfter(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsAfter(boardID, limit, cursorWrNum, cursorWrReply, true)
}

func (r *writeRepository) FindPostsAfterSummary(boardID string, limit int, cursorWrNum int, cursorWrReply string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsAfter(boardID, limit, cursorWrNum, cursorWrReply, false)
}

func (r *writeRepository) findPostsAfter(boardID string, limit int, cursorWrNum int, cursorWrReply string, includeContent bool) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64

	table := tableName(boardID)

	total = r.getCachedPostCount(boardID)
	if total == 0 {
		countQuery := r.db.Table(table).Where("wr_is_comment = 0")
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	orderClause := r.getSortField(boardID)
	if orderClause != "wr_num, wr_reply" {
		return r.findPosts(boardID, 1, limit, includeContent)
	}

	err := r.db.Raw(
		fmt.Sprintf(
			"SELECT %s FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND (wr_num > ? OR (wr_num = ? AND wr_reply > ?)) ORDER BY wr_num, wr_reply LIMIT ?",
			postSelectColumnsForList(boardID, "", includeContent),
			table,
		),
		cursorWrNum, cursorWrNum, cursorWrReply, limit,
	).Scan(&posts).Error
	if err != nil && strings.Contains(err.Error(), "idx_list_page") {
		err = r.db.Table(table).
			Select(postSelectColumnsForList(boardID, "", includeContent)).
			Where("wr_is_comment = 0 AND (wr_num > ? OR (wr_num = ? AND wr_reply > ?))", cursorWrNum, cursorWrNum, cursorWrReply).
			Order(orderClause).
			Limit(limit).
			Find(&posts).Error
	}

	return posts, total, err
}

// FindPostsFromDate returns the first archive page at-or-before beforeDate (newest first, then older).
func (r *writeRepository) FindPostsFromDate(boardID string, limit int, beforeDate string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsFromDate(boardID, limit, beforeDate, true)
}

// FindPostsFromDateSummary is FindPostsFromDate without content columns (list view).
func (r *writeRepository) FindPostsFromDateSummary(boardID string, limit int, beforeDate string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsFromDate(boardID, limit, beforeDate, false)
}

// findPostsFromDate resolves beforeDate to an archive position via the wr_datetime index and returns
// the first page from there. wr_num is chronological-monotonic (newest = smallest), so the boundary
// post (newest at-or-before the date) has the smallest wr_num among older posts; `wr_num >= boundary`
// yields that post and everything older, in list order. Depth-independent (no OFFSET) → fast at any age.
// Subsequent pages use the existing exclusive cursor (FindPostsAfter) via next_cursor. #12975.
// 날짜 경계 탐색 창. ⛔ 이 값을 키우지 말 것 — 30일을 넘기면 옵티마이저가
// ix_wr_datetime 시크를 포기하고 최신부터 역방향 스캔으로 전환해 11초가 걸린다.
// (2026-07-21 g5_write_free 실측: 7일 70ms · 30일 62ms · 180일 11,357ms · 730일 11,112ms)
// 빈 구간은 창을 넓히는 대신 창을 뒤로 옮겨 해결한다.
const (
	dateBoundaryWindowDays = 30
	dateBoundaryMaxSteps   = 24 // 30일 × 24 = 약 2년
)

// resolveDateBoundaryWrNum 는 beforeDate 이전의 가장 최근 원글 wr_num 을 찾는다.
//
// 원래 구현은 `wr_datetime <= ?` 만으로 경계를 잡았는데, g5_write_free 는 댓글이
// 89%(590만/666만)라 하한이 없으면 MySQL 이 최신부터 역방향으로 훑으며 댓글을
// 걸러낸다. 날짜가 과거일수록 느려져(2026-07 919ms → 2025-06 11,060ms) 웹 프록시
// 타임아웃 4초를 넘겼다. 실제로 /free?before_date=2025-06 이 8초에 목록 누락으로
// 깨져 있었다(#12975 ② 회귀).
//
// 30일 창을 뒤로 이동하며 찾고, 끝까지 못 찾으면 게시판 최초 글로 폴백한다.
func (r *writeRepository) resolveDateBoundaryWrNum(table, beforeDate string) (int, bool, error) {
	type boundaryRow struct{ WrNum int }

	query := fmt.Sprintf(
		"SELECT wr_num FROM `%s` WHERE wr_is_comment = 0 "+
			"AND wr_datetime <= DATE_SUB(?, INTERVAL ? DAY) "+
			"AND wr_datetime >= DATE_SUB(?, INTERVAL ? DAY) "+
			"ORDER BY wr_datetime DESC LIMIT 1",
		table,
	)

	for step := 0; step < dateBoundaryMaxSteps; step++ {
		var row boundaryRow
		res := r.db.Raw(
			query,
			beforeDate, step*dateBoundaryWindowDays,
			beforeDate, (step+1)*dateBoundaryWindowDays,
		).Scan(&row)
		if res.Error != nil {
			return 0, false, res.Error
		}
		if res.RowsAffected > 0 {
			return row.WrNum, true, nil
		}
	}

	// 2년을 거슬러도 없으면 게시판 최초 글(정방향 스캔이라 빠르다).
	var oldest boundaryRow
	res := r.db.Raw(
		fmt.Sprintf(
			"SELECT wr_num FROM `%s` WHERE wr_is_comment = 0 ORDER BY wr_datetime ASC LIMIT 1",
			table,
		),
	).Scan(&oldest)
	if res.Error != nil {
		return 0, false, res.Error
	}
	return oldest.WrNum, res.RowsAffected > 0, nil
}

func (r *writeRepository) findPostsFromDate(boardID string, limit int, beforeDate string, includeContent bool) ([]*gnuboard.G5Write, int64, error) {
	var posts []*gnuboard.G5Write
	var total int64
	table := tableName(boardID)

	total = r.getCachedPostCount(boardID)
	if total == 0 {
		countQuery := r.db.Table(table).Where("wr_is_comment = 0")
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	orderClause := r.getSortField(boardID)
	if orderClause != "wr_num, wr_reply" {
		// Non-standard ordering boards fall back to page 1 (date nav is a free/hello feature).
		return r.findPosts(boardID, 1, limit, includeContent)
	}

	// boundary = newest root post at-or-before the date.
	// 상수로 미리 해석한다 — 서브쿼리로 두면 아래 목록 쿼리가 매번 느린 경계를 다시 푼다.
	boundaryWrNum, found, err := r.resolveDateBoundaryWrNum(table, beforeDate)
	if err != nil {
		return nil, 0, err
	}
	if !found {
		// 해당 날짜 이전에 글이 없다(게시판 최초 글보다 과거) → 첫 페이지.
		return r.findPosts(boardID, 1, limit, includeContent)
	}

	err = r.db.Raw(
		fmt.Sprintf(
			"SELECT %s FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND wr_num >= ? ORDER BY wr_num, wr_reply LIMIT ?",
			postSelectColumnsForList(boardID, "", includeContent), table,
		),
		boundaryWrNum, limit,
	).Scan(&posts).Error
	if err != nil && strings.Contains(err.Error(), "idx_list_page") {
		err = r.db.Raw(
			fmt.Sprintf(
				"SELECT %s FROM `%s` WHERE wr_is_comment = 0 AND wr_num >= ? ORDER BY wr_num, wr_reply LIMIT ?",
				postSelectColumnsForList(boardID, "", includeContent), table,
			),
			boundaryWrNum, limit,
		).Scan(&posts).Error
	}

	return posts, total, err
}

// FindPostsFiltered retrieves posts excluding specified members. Delegates to FindPosts if excludeMbIDs is empty.
// Uses the same cached count as FindPosts (차단 유저 수가 적어 total 차이 무시 가능).
func (r *writeRepository) FindPostsFiltered(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsFiltered(boardID, page, limit, excludeMbIDs, true)
}

func (r *writeRepository) FindPostsFilteredSummary(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, int64, error) {
	return r.findPostsFiltered(boardID, page, limit, excludeMbIDs, false)
}

func (r *writeRepository) findPostsFiltered(boardID string, page, limit int, excludeMbIDs []string, includeContent bool) ([]*gnuboard.G5Write, int64, error) {
	if len(excludeMbIDs) == 0 {
		return r.findPosts(boardID, page, limit, includeContent)
	}

	var posts []*gnuboard.G5Write
	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)

	// Reuse cached total count (same as FindPosts — avoids expensive COUNT on large tables)
	total := r.getCachedPostCount(boardID)
	if total == 0 {
		if err := r.db.Table(table).Where("wr_is_comment = 0").Count(&total).Error; err != nil {
			return nil, 0, err
		}
		r.setCachedPostCount(boardID, total)
	}

	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)

	// Deferred JOIN with exclusion filter applied in subquery
	if orderClause == "wr_num, wr_reply" {
		err := r.db.Raw(
			fmt.Sprintf(
				"SELECT %s FROM `%s` t JOIN (SELECT wr_id FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND mb_id NOT IN ? ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?) ids ON t.wr_id = ids.wr_id ORDER BY t.wr_num, t.wr_reply",
				postSelectColumnsForList(boardID, "t", includeContent), table, table,
			),
			excludeMbIDs, limit, offset,
		).Scan(&posts).Error
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(selectCols).
				Where("wr_is_comment = 0 AND mb_id NOT IN ?", excludeMbIDs).
				Order(orderClause).
				Offset(offset).
				Limit(limit).
				Find(&posts).Error
		}
		return posts, total, err
	}

	err := r.db.Table(table).
		Select(selectCols).
		Where("wr_is_comment = 0 AND mb_id NOT IN ?", excludeMbIDs).
		Order(orderClause).
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

func (r *writeRepository) FindPostsFilteredHasNext(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsFilteredHasNext(boardID, page, limit, excludeMbIDs, true)
}

func (r *writeRepository) FindPostsFilteredHasNextSummary(boardID string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsFilteredHasNext(boardID, page, limit, excludeMbIDs, false)
}

func (r *writeRepository) findPostsFilteredHasNext(boardID string, page, limit int, excludeMbIDs []string, includeContent bool) ([]*gnuboard.G5Write, bool, error) {
	if len(excludeMbIDs) == 0 {
		return r.findPostsHasNext(boardID, page, limit, includeContent)
	}

	var posts []*gnuboard.G5Write
	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)
	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)
	queryLimit := limit + 1

	if orderClause == "wr_num, wr_reply" {
		err := r.db.Raw(
			fmt.Sprintf(
				"SELECT %s FROM `%s` t JOIN (SELECT wr_id FROM `%s` FORCE INDEX (idx_list_page) WHERE wr_is_comment = 0 AND mb_id NOT IN ? ORDER BY wr_num, wr_reply LIMIT ? OFFSET ?) ids ON t.wr_id = ids.wr_id ORDER BY t.wr_num, t.wr_reply",
				postSelectColumnsForList(boardID, "t", includeContent), table, table,
			),
			excludeMbIDs, queryLimit, offset,
		).Scan(&posts).Error
		if err != nil && strings.Contains(err.Error(), "idx_list_page") {
			err = r.db.Table(table).
				Select(selectCols).
				Where("wr_is_comment = 0 AND mb_id NOT IN ?", excludeMbIDs).
				Order(orderClause).
				Offset(offset).
				Limit(queryLimit).
				Find(&posts).Error
		}
		trimmed, hasNext := trimHasNextPosts(posts, limit)
		return trimmed, hasNext, err
	}

	err := r.db.Table(table).
		Select(selectCols).
		Where("wr_is_comment = 0 AND mb_id NOT IN ?", excludeMbIDs).
		Order(orderClause).
		Offset(offset).
		Limit(queryLimit).
		Find(&posts).Error
	trimmed, hasNext := trimHasNextPosts(posts, limit)
	return trimmed, hasNext, err
}

func (r *writeRepository) FindPostsByCategoryFilteredHasNext(boardID string, category string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsByCategoryFilteredHasNext(boardID, category, page, limit, excludeMbIDs, true)
}

func (r *writeRepository) FindPostsByCategoryFilteredHasNextSummary(boardID string, category string, page, limit int, excludeMbIDs []string) ([]*gnuboard.G5Write, bool, error) {
	return r.findPostsByCategoryFilteredHasNext(boardID, category, page, limit, excludeMbIDs, false)
}

func (r *writeRepository) findPostsByCategoryFilteredHasNext(boardID string, category string, page, limit int, excludeMbIDs []string, includeContent bool) ([]*gnuboard.G5Write, bool, error) {
	if len(excludeMbIDs) == 0 {
		return r.findPostsByCategoryHasNext(boardID, category, page, limit, includeContent)
	}

	var posts []*gnuboard.G5Write
	offset := (page - 1) * limit
	if offset > maxPostOffset {
		offset = maxPostOffset
	}
	table := tableName(boardID)
	orderClause := r.getSortField(boardID)
	selectCols := postSelectColumnsForList(boardID, "", includeContent)
	queryLimit := limit + 1

	err := r.db.Table(table).
		Select(selectCols).
		Where("wr_is_comment = 0 AND ca_name = ? AND mb_id NOT IN ?", category, excludeMbIDs).
		Order(orderClause).
		Offset(offset).
		Limit(queryLimit).
		Find(&posts).Error

	trimmed, hasNext := trimHasNextPosts(posts, limit)
	return trimmed, hasNext, err
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
		Select(postSelectColumns(boardID, "")).
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
func (r *writeRepository) SearchPosts(boardID string, searchField, searchQuery string, page, limit int, sortBy ...string) ([]*gnuboard.G5Write, int64, error) {
	if r.sphinx == nil {
		return nil, 0, fmt.Errorf("검색 서비스를 일시적으로 사용할 수 없습니다")
	}

	result, err := r.sphinx.Search(boardID, searchField, searchQuery, page, limit, sortBy...)
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
		Select(postSelectColumns(boardID, "")).
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

// SearchPostsByCategory retrieves posts matching search criteria filtered by ca_name (category).
// Uses Sphinx for full-text search, then filters by category in MySQL.
func (r *writeRepository) SearchPostsByCategory(boardID string, searchField, searchQuery, category string, page, limit int) ([]*gnuboard.G5Write, int64, error) {
	if r.sphinx == nil {
		return nil, 0, fmt.Errorf("검색 서비스를 일시적으로 사용할 수 없습니다")
	}

	// Fetch extra results from Sphinx since category filter will reduce count
	result, err := r.sphinx.Search(boardID, searchField, searchQuery, 1, limit*page*3)
	if err != nil {
		return nil, 0, fmt.Errorf("검색 서비스 오류: %w", err)
	}
	if result == nil || len(result.IDs) == 0 {
		return nil, 0, nil
	}

	// Fetch from MySQL with category filter and count
	var total int64
	table := tableName(boardID)
	if err := r.db.Table(table).
		Where("wr_id IN ? AND ca_name = ? AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", result.IDs, category).
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []*gnuboard.G5Write
	offset := (page - 1) * limit
	if err := r.db.Table(table).
		Select(postSelectColumns(boardID, "")).
		Where("wr_id IN ? AND ca_name = ? AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", result.IDs, category).
		Order("wr_id DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

// FindPostByID retrieves a single post by ID (excludes soft deleted)
func (r *writeRepository) FindPostByID(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(postSelectColumns(boardID, "")).
		Where("wr_id = ? AND wr_is_comment = 0 AND wr_deleted_at IS NULL", wrID).
		First(&post).Error
	return &post, err
}

// FindPostByIDIncludeDeleted retrieves a single post by ID including soft deleted posts
func (r *writeRepository) FindPostByIDIncludeDeleted(boardID string, wrID int) (*gnuboard.G5Write, error) {
	var post gnuboard.G5Write
	err := r.db.Table(tableName(boardID)).
		Select(postSelectColumns(boardID, "")).
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
		Select(postSelectColumns(boardID, "")).
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
		Select(postSelectColumns(boardID, "")).
		Where("wr_is_comment = 0 AND wr_deleted_at IS NOT NULL").
		Order("wr_deleted_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	return posts, total, err
}

// postCountRedisKey returns the Redis key for post count cache
func postCountCacheKey(boardID string, category string) string {
	if category == "" {
		return boardID
	}
	return boardID + ":" + category
}

func postCountRedisKey(boardID string, category string) string {
	return "postcount:" + postCountCacheKey(boardID, category)
}

func postCountRedisPrefix(boardID string) string {
	return "postcount:" + boardID + ":"
}

func postCountMemoryKey(boardID string, category string) string {
	return "count:" + postCountCacheKey(boardID, category)
}

// getCachedPostCount tries Redis first, then falls back to in-memory cache
func (r *writeRepository) getCachedPostCount(boardID string) int64 {
	return r.getCachedPostCountByCategory(boardID, "")
}

func (r *writeRepository) getCachedPostCountByCategory(boardID string, category string) int64 {
	// Try Redis first (shared across all pods)
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		val, err := r.redis.Get(ctx, postCountRedisKey(boardID, category)).Result()
		if err == nil {
			if count, err := strconv.ParseInt(val, 10, 64); err == nil {
				return count
			}
		}
	}

	// Fallback to in-memory cache
	cacheKey := postCountMemoryKey(boardID, category)
	if cached, ok := postCountCache.Load(cacheKey); ok {
		if cc, ok2 := cached.(*cachedCount); ok2 && time.Now().Before(cc.expiresAt) {
			return cc.total
		}
	}
	return 0
}

// setCachedPostCount stores count in both Redis (shared) and in-memory (local fallback)
func (r *writeRepository) setCachedPostCount(boardID string, total int64) {
	r.setCachedPostCountByCategory(boardID, "", total)
}

func (r *writeRepository) setCachedPostCountByCategory(boardID string, category string, total int64) {
	ttl := countCacheTTLForBoard(boardID)

	// Store in Redis (shared across pods)
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		r.redis.Set(ctx, postCountRedisKey(boardID, category), total, ttl)
	}

	// Also store in local memory as fallback
	postCountCache.Store(
		postCountMemoryKey(boardID, category),
		&cachedCount{total: total, expiresAt: time.Now().Add(ttl)},
	)
}

// invalidatePostCount clears the cached post count for a board from both Redis and memory
func (r *writeRepository) invalidatePostCount(boardID string) {
	// Invalidate Redis
	if r.redis != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		r.redis.Del(ctx, postCountRedisKey(boardID, ""))

		var cursor uint64
		prefix := postCountRedisPrefix(boardID)
		for {
			keys, nextCursor, err := r.redis.Scan(ctx, cursor, prefix+"*", 100).Result()
			if err != nil {
				break
			}
			if len(keys) > 0 {
				r.redis.Del(ctx, keys...)
			}
			cursor = nextCursor
			if cursor == 0 {
				break
			}
		}
	}

	// Invalidate in-memory
	postCountCache.Delete(postCountMemoryKey(boardID, ""))
	prefix := postCountMemoryKey(boardID, "") + ":"
	postCountCache.Range(func(key, _ any) bool {
		keyStr, ok := key.(string)
		if ok && strings.HasPrefix(keyStr, prefix) {
			postCountCache.Delete(keyStr)
		}
		return true
	})
}

// InvalidatePostCount clears the cached post count from in-memory cache (legacy, no Redis)
func InvalidatePostCount(boardID string) {
	postCountCache.Delete(postCountMemoryKey(boardID, ""))
	prefix := postCountMemoryKey(boardID, "") + ":"
	postCountCache.Range(func(key, _ any) bool {
		keyStr, ok := key.(string)
		if ok && strings.HasPrefix(keyStr, prefix) {
			postCountCache.Delete(keyStr)
		}
		return true
	})
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

// RecordContentHistory 는 g5_da_content_history 에 감사 이력 한 건을 best-effort 로 남긴다.
// 기록 실패가 본 작업(삭제 등)을 막으면 안 되므로 에러는 로그만 남기고 삼킨다.
func RecordContentHistory(db *gorm.DB, boTable string, wrID int, wrIsComment int, mbID, wrName, operation, operatedBy string, prevData map[string]interface{}) {
	payload, err := json.Marshal(prevData)
	if err != nil {
		log.Printf("[ContentHistory] Failed to marshal previous data for %s/%d: %v", boTable, wrID, err)
		return
	}
	if err := db.Exec(`INSERT INTO g5_da_content_history
		(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		boTable, wrID, wrIsComment, mbID, wrName, operation, operatedBy, time.Now(), string(payload),
	).Error; err != nil {
		log.Printf("[ContentHistory] Failed to record %s history for %s/%d: %v", operation, boTable, wrID, err)
	}
}

// DeletePost permanently deletes a post and its comments from the database
func (r *writeRepository) DeletePost(boardID string, wrID int, deletedBy string) error {
	r.invalidatePostCount(boardID)
	table := tableName(boardID)

	// 영구삭제는 DB 에서 원본이 사라지므로 삭제 전 글·댓글 내용을 이력으로 남긴다 (best-effort)
	var post struct {
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
		WrName    string `gorm:"column:wr_name"`
		MbID      string `gorm:"column:mb_id"`
	}
	if err := r.db.Table(table).Select("wr_subject, wr_content, wr_name, mb_id").Where("wr_id = ?", wrID).Scan(&post).Error; err == nil {
		RecordContentHistory(r.db, boardID, wrID, 0, post.MbID, post.WrName, "영구삭제", deletedBy, map[string]interface{}{
			"wr_subject": post.WrSubject,
			"wr_content": post.WrContent,
			"wr_name":    post.WrName,
			"mb_id":      post.MbID,
		})
	}
	// 함께 지워지는 댓글은 개수 미상이라 INSERT...SELECT 로 한 번에 기록
	if err := r.db.Exec(`INSERT INTO g5_da_content_history
		(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
		SELECT ?, wr_id, 1, mb_id, wr_name, '영구삭제', ?, ?,
			JSON_OBJECT('wr_content', wr_content, 'wr_name', wr_name, 'mb_id', mb_id)
		FROM `+table+` WHERE wr_parent = ? AND wr_is_comment = 1`,
		boardID, deletedBy, time.Now(), wrID,
	).Error; err != nil {
		log.Printf("[ContentHistory] Failed to record comment histories for %s/%d: %v", boardID, wrID, err)
	}

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
		MbID      string `gorm:"column:mb_id"`
	}
	if err := r.db.Table(table).Select("wr_subject, wr_content, wr_name, mb_id").Where("wr_id = ?", wrID).Scan(&post).Error; err == nil {
		var nextVersion int
		r.db.Raw("SELECT COALESCE(MAX(version), 0) + 1 FROM g5_write_revisions WHERE board_id = ? AND wr_id = ?", boardID, wrID).Scan(&nextVersion)
		if err := r.db.Exec(`INSERT INTO g5_write_revisions
			(board_id, wr_id, version, change_type, title, content, edited_by, edited_by_name, edited_at)
			VALUES (?, ?, ?, 'soft_delete', ?, ?, ?, ?, ?)`,
			boardID, wrID, nextVersion, post.WrSubject, post.WrContent, deletedBy, post.WrName, now,
		).Error; err != nil {
			log.Printf("[SoftDeletePost] Failed to record revision for %s/%d: %v", boardID, wrID, err)
		}

		// g5_da_content_history에도 이중 기록
		prevData, err := json.Marshal(map[string]interface{}{
			"wr_subject": post.WrSubject,
			"wr_content": post.WrContent,
			"wr_name":    post.WrName,
			"mb_id":      post.MbID,
		})
		if err != nil {
			log.Printf("[SoftDeletePost] Failed to marshal content history for %s/%d: %v", boardID, wrID, err)
		} else {
			r.db.Exec(`INSERT INTO g5_da_content_history
				(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
				VALUES (?, ?, 0, ?, ?, '삭제', ?, ?, ?)`,
				boardID, wrID, post.MbID, post.WrName, deletedBy, now, string(prevData))
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
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(tableName(boardID)).Create(comment).Error; err != nil {
			return err
		}
		if comment.WrParent <= 0 || comment.WrIsComment != 1 || comment.WrDeletedAt != nil {
			return nil
		}
		return tx.Table(tableName(boardID)).
			Where("wr_id = ?", comment.WrParent).
			Update("wr_comment", clampCommentDelta(1)).
			Error
	})
}

// DeleteComment permanently deletes a comment from the database
func (r *writeRepository) DeleteComment(boardID string, wrID int) error {
	table := tableName(boardID)
	return r.db.Transaction(func(tx *gorm.DB) error {
		var comment struct {
			WrParent    int        `gorm:"column:wr_parent"`
			WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
		}
		if err := tx.Table(table).
			Select("wr_parent, wr_deleted_at").
			Where("wr_id = ? AND wr_is_comment = 1", wrID).
			Take(&comment).Error; err != nil {
			return err
		}
		if err := tx.Table(table).
			Where("wr_id = ? AND wr_is_comment = 1", wrID).
			Delete(&gnuboard.G5Write{}).Error; err != nil {
			return err
		}
		if comment.WrParent <= 0 || comment.WrDeletedAt != nil {
			return nil
		}
		return tx.Table(table).
			Where("wr_id = ?", comment.WrParent).
			Update("wr_comment", clampCommentDelta(-1)).
			Error
	})
}

// SoftDeleteComment marks a comment as deleted
func (r *writeRepository) SoftDeleteComment(boardID string, wrID int, deletedBy string) error {
	table := tableName(boardID)
	now := time.Now()

	// 삭제 전 댓글 데이터 읽기 + g5_da_content_history 기록
	var comment struct {
		WrContent string `gorm:"column:wr_content"`
		WrName    string `gorm:"column:wr_name"`
		MbID      string `gorm:"column:mb_id"`
	}
	if err := r.db.Table(table).Select("wr_content, wr_name, mb_id").
		Where("wr_id = ? AND wr_is_comment = 1", wrID).Scan(&comment).Error; err == nil {
		prevData, err := json.Marshal(map[string]interface{}{
			"wr_content": comment.WrContent,
			"wr_name":    comment.WrName,
			"mb_id":      comment.MbID,
		})
		if err != nil {
			log.Printf("[SoftDeleteComment] Failed to marshal content history for %s/%d: %v", boardID, wrID, err)
		} else {
			r.db.Exec(`INSERT INTO g5_da_content_history
				(bo_table, wr_id, wr_is_comment, mb_id, wr_name, operation, operated_by, operated_at, previous_data)
				VALUES (?, ?, 1, ?, ?, '삭제', ?, ?, ?)`,
				boardID, wrID, comment.MbID, comment.WrName, deletedBy, now, string(prevData))
		}
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		var comment struct {
			WrParent    int        `gorm:"column:wr_parent"`
			WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
		}
		if err := tx.Table(table).
			Select("wr_parent, wr_deleted_at").
			Where("wr_id = ? AND wr_is_comment = 1", wrID).
			Take(&comment).Error; err != nil {
			return err
		}
		result := tx.Table(table).
			Where("wr_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", wrID).
			Updates(map[string]interface{}{
				"wr_deleted_at": now,
				"wr_deleted_by": deletedBy,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || comment.WrParent <= 0 || comment.WrDeletedAt != nil {
			return nil
		}
		return tx.Table(table).
			Where("wr_id = ?", comment.WrParent).
			Update("wr_comment", clampCommentDelta(-1)).
			Error
	})
}

// RestoreComment restores a soft deleted comment
func (r *writeRepository) RestoreComment(boardID string, wrID int) error {
	table := tableName(boardID)
	return r.db.Transaction(func(tx *gorm.DB) error {
		var comment struct {
			WrParent    int        `gorm:"column:wr_parent"`
			WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
		}
		if err := tx.Table(table).
			Select("wr_parent, wr_deleted_at").
			Where("wr_id = ? AND wr_is_comment = 1", wrID).
			Take(&comment).Error; err != nil {
			return err
		}
		result := tx.Table(table).
			Where("wr_id = ? AND wr_is_comment = 1 AND wr_deleted_at IS NOT NULL", wrID).
			Updates(map[string]interface{}{
				"wr_deleted_at": nil,
				"wr_deleted_by": nil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 || comment.WrParent <= 0 || comment.WrDeletedAt == nil {
			return nil
		}
		return tx.Table(table).
			Where("wr_id = ?", comment.WrParent).
			Update("wr_comment", clampCommentDelta(1)).
			Error
	})
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

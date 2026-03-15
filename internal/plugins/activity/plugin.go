package activity

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/plugin"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	plugin.RegisterFactory("activity-feed", func() plugin.Plugin {
		return New()
	}, Manifest)
}

// Manifest describes the activity feed plugin
var Manifest = &plugin.PluginManifest{
	Name:        "activity-feed",
	Version:     "1.0.0",
	Title:       "Activity Feed",
	Description: "회원 활동 read model write-through 플러그인 (member_activity_feed, member_activity_stats)",
	Author:      "Angple",
	License:     "MIT",
	Requires: plugin.Requires{
		Angple: ">=1.0.0",
	},
}

// ActivityPlugin writes activity data on post/comment hooks
type ActivityPlugin struct {
	db     *gorm.DB
	logger plugin.Logger
}

// New creates a new ActivityPlugin
func New() *ActivityPlugin {
	return &ActivityPlugin{}
}

// Name returns the plugin name
func (p *ActivityPlugin) Name() string { return "activity-feed" }

// Migrate is a no-op; tables are created by internal/migration
func (p *ActivityPlugin) Migrate(_ *gorm.DB) error { return nil }

// Initialize sets up the plugin context
func (p *ActivityPlugin) Initialize(ctx *plugin.PluginContext) error {
	p.db = ctx.DB
	p.logger = ctx.Logger
	p.logger.Info("activity-feed plugin initialized")
	return nil
}

// RegisterRoutes is a no-op (no HTTP routes)
func (p *ActivityPlugin) RegisterRoutes(_ gin.IRouter) {}

// Shutdown cleans up plugin resources
func (p *ActivityPlugin) Shutdown() error { return nil }

// RegisterHooks registers all post/comment hooks for write-through
func (p *ActivityPlugin) RegisterHooks(hm *plugin.HookManager) {
	hm.Register(plugin.HookPostAfterCreate, "activity-feed", p.onPostCreated, 100)
	hm.Register(plugin.HookPostAfterUpdate, "activity-feed", p.onPostUpdated, 100)
	hm.Register(plugin.HookPostAfterDelete, "activity-feed", p.onPostDeleted, 100)
	hm.Register(plugin.HookPostAfterRestore, "activity-feed", p.onPostRestored, 100)
	hm.Register(plugin.HookCommentAfterCreate, "activity-feed", p.onCommentCreated, 100)
	hm.Register(plugin.HookCommentAfterUpdate, "activity-feed", p.onCommentUpdated, 100)
	hm.Register(plugin.HookCommentAfterDelete, "activity-feed", p.onCommentDeleted, 100)
}

// --- hook handlers ---

func (p *ActivityPlugin) onPostCreated(ctx *plugin.HookContext) error {
	data := ctx.Input
	boardID, _ := data["board_id"].(string)
	postID := toUint64(data["post_id"])
	title, _ := data["title"].(string)
	content, _ := data["content"].(string)
	authorID, _ := data["author_id"].(string)
	authorName, _ := data["author_name"].(string)
	isSecret, _ := data["is_secret"].(bool)
	sourceCreatedAt, _ := data["source_created_at"].(time.Time)

	if boardID == "" || authorID == "" || postID == 0 {
		return nil
	}

	isPublic := !isSecret && p.isBoardSearchable(boardID)
	wrOption := ""
	if isSecret {
		wrOption = "secret"
	}

	feed := activityFeedRow{
		MemberID:        authorID,
		BoardID:         boardID,
		WriteTable:      "v2_posts",
		WriteID:         postID,
		ActivityType:    1, // post
		IsPublic:        boolToInt8(isPublic),
		IsDeleted:       0,
		Title:           truncate(title, 255),
		ContentPreview:  stripHTMLPreview(content),
		AuthorName:      authorName,
		WrOption:        wrOption,
		SourceCreatedAt: sourceCreatedAt,
	}

	if err := p.upsertFeed(&feed); err != nil {
		p.logger.Error("activity-feed: post create upsert failed (board=%s, post=%d): %v", boardID, postID, err)
		return nil
	}

	p.incrementStats(authorID, boardID, 1, 0, isPublic)
	return nil
}

func (p *ActivityPlugin) onPostUpdated(ctx *plugin.HookContext) error {
	data := ctx.Input
	postID := toUint64(data["post_id"])
	title, _ := data["title"].(string)
	content, _ := data["content"].(string)
	isSecret, _ := data["is_secret"].(bool)
	boardID, _ := data["board_id"].(string)

	if postID == 0 {
		return nil
	}

	isPublic := !isSecret && p.isBoardSearchable(boardID)
	wrOption := ""
	if isSecret {
		wrOption = "secret"
	}
	now := time.Now()

	err := p.db.Table("member_activity_feed").
		Where("write_table = ? AND write_id = ? AND activity_type = 1", "v2_posts", postID).
		Updates(map[string]interface{}{
			"title":             truncate(title, 255),
			"content_preview":   stripHTMLPreview(content),
			"wr_option":         wrOption,
			"is_public":         boolToInt8(isPublic),
			"source_updated_at": &now,
		}).Error
	if err != nil {
		p.logger.Error("activity-feed: post update failed (post=%d): %v", postID, err)
	}
	return nil
}

func (p *ActivityPlugin) onPostDeleted(ctx *plugin.HookContext) error {
	data := ctx.Input
	postID := toUint64(data["post_id"])
	authorID, _ := data["author_id"].(string)
	boardID, _ := data["board_id"].(string)

	if postID == 0 {
		return nil
	}

	err := p.db.Table("member_activity_feed").
		Where("write_table = ? AND write_id = ? AND activity_type = 1", "v2_posts", postID).
		Update("is_deleted", 1).Error
	if err != nil {
		p.logger.Error("activity-feed: post delete mark failed (post=%d): %v", postID, err)
		return nil
	}
	p.decrementStats(authorID, boardID, 1, 0)
	return nil
}

func (p *ActivityPlugin) onPostRestored(ctx *plugin.HookContext) error {
	data := ctx.Input
	postID := toUint64(data["post_id"])
	authorID, _ := data["author_id"].(string)
	boardID, _ := data["board_id"].(string)

	if postID == 0 {
		return nil
	}

	err := p.db.Table("member_activity_feed").
		Where("write_table = ? AND write_id = ? AND activity_type = 1", "v2_posts", postID).
		Update("is_deleted", 0).Error
	if err != nil {
		p.logger.Error("activity-feed: post restore failed (post=%d): %v", postID, err)
		return nil
	}
	// Approximate: restored posts assumed non-secret
	isPublic := p.isBoardSearchable(boardID)
	p.incrementStats(authorID, boardID, 1, 0, isPublic)
	return nil
}

func (p *ActivityPlugin) onCommentCreated(ctx *plugin.HookContext) error {
	data := ctx.Input
	boardID, _ := data["board_id"].(string)
	commentID := toUint64(data["comment_id"])
	postID := toUint64(data["post_id"])
	content, _ := data["content"].(string)
	authorID, _ := data["author_id"].(string)
	authorName, _ := data["author_name"].(string)
	parentTitle, _ := data["parent_title"].(string)
	sourceCreatedAt, _ := data["source_created_at"].(time.Time)

	if boardID == "" || authorID == "" || commentID == 0 {
		return nil
	}

	isPublic := p.isBoardSearchable(boardID)

	feed := activityFeedRow{
		MemberID:        authorID,
		BoardID:         boardID,
		WriteTable:      "v2_comments",
		WriteID:         commentID,
		ParentWriteID:   &postID,
		ActivityType:    2, // comment
		IsPublic:        boolToInt8(isPublic),
		IsDeleted:       0,
		ContentPreview:  stripHTMLPreview(content),
		ParentTitle:     truncate(parentTitle, 255),
		AuthorName:      authorName,
		SourceCreatedAt: sourceCreatedAt,
	}

	if err := p.upsertFeed(&feed); err != nil {
		p.logger.Error("activity-feed: comment create upsert failed (board=%s, comment=%d): %v", boardID, commentID, err)
		return nil
	}

	p.incrementStats(authorID, boardID, 0, 1, isPublic)
	return nil
}

func (p *ActivityPlugin) onCommentUpdated(ctx *plugin.HookContext) error {
	data := ctx.Input
	commentID := toUint64(data["comment_id"])
	content, _ := data["content"].(string)

	if commentID == 0 {
		return nil
	}

	now := time.Now()
	err := p.db.Table("member_activity_feed").
		Where("write_table = ? AND write_id = ? AND activity_type = 2", "v2_comments", commentID).
		Updates(map[string]interface{}{
			"content_preview":   stripHTMLPreview(content),
			"source_updated_at": &now,
		}).Error
	if err != nil {
		p.logger.Error("activity-feed: comment update failed (comment=%d): %v", commentID, err)
	}
	return nil
}

func (p *ActivityPlugin) onCommentDeleted(ctx *plugin.HookContext) error {
	data := ctx.Input
	commentID := toUint64(data["comment_id"])
	authorID, _ := data["author_id"].(string)
	boardID, _ := data["board_id"].(string)

	if commentID == 0 {
		return nil
	}

	err := p.db.Table("member_activity_feed").
		Where("write_table = ? AND write_id = ? AND activity_type = 2", "v2_comments", commentID).
		Update("is_deleted", 1).Error
	if err != nil {
		p.logger.Error("activity-feed: comment delete mark failed (comment=%d): %v", commentID, err)
		return nil
	}
	p.decrementStats(authorID, boardID, 0, 1)
	return nil
}

// --- data model ---

type activityFeedRow struct {
	MemberID        string     `gorm:"column:member_id"`
	BoardID         string     `gorm:"column:board_id"`
	WriteTable      string     `gorm:"column:write_table"`
	WriteID         uint64     `gorm:"column:write_id"`
	ParentWriteID   *uint64    `gorm:"column:parent_write_id"`
	ActivityType    int8       `gorm:"column:activity_type"`
	IsPublic        int8       `gorm:"column:is_public"`
	IsDeleted       int8       `gorm:"column:is_deleted"`
	Title           string     `gorm:"column:title"`
	ContentPreview  string     `gorm:"column:content_preview"`
	ParentTitle     string     `gorm:"column:parent_title"`
	AuthorName      string     `gorm:"column:author_name"`
	WrOption        string     `gorm:"column:wr_option"`
	SourceCreatedAt time.Time  `gorm:"column:source_created_at"`
	SourceUpdatedAt *time.Time `gorm:"column:source_updated_at"`
}

func (activityFeedRow) TableName() string { return "member_activity_feed" }

// --- helpers ---

func (p *ActivityPlugin) upsertFeed(feed *activityFeedRow) error {
	return p.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "write_table"}, {Name: "write_id"}, {Name: "activity_type"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"title", "content_preview", "parent_title", "author_name",
			"wr_option", "is_public", "source_updated_at",
		}),
	}).Create(feed).Error
}

func (p *ActivityPlugin) incrementStats(mbID, boardID string, postDelta, commentDelta int, isPublic bool) {
	publicPostDelta := 0
	publicCommentDelta := 0
	if isPublic {
		publicPostDelta = postDelta
		publicCommentDelta = commentDelta
	}

	sql := `INSERT INTO member_activity_stats (member_id, board_id, post_count, comment_count, public_post_count, public_comment_count)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
  post_count = post_count + VALUES(post_count),
  comment_count = comment_count + VALUES(comment_count),
  public_post_count = public_post_count + VALUES(public_post_count),
  public_comment_count = public_comment_count + VALUES(public_comment_count)`

	if err := p.db.Exec(sql, mbID, boardID, postDelta, commentDelta, publicPostDelta, publicCommentDelta).Error; err != nil {
		p.logger.Error("activity-feed: stats increment failed (%s/%s): %v", mbID, boardID, err)
	}
}

func (p *ActivityPlugin) decrementStats(mbID, boardID string, postDelta, commentDelta int) {
	sql := fmt.Sprintf(`UPDATE member_activity_stats SET
  post_count = GREATEST(0, CAST(post_count AS SIGNED) - %d),
  comment_count = GREATEST(0, CAST(comment_count AS SIGNED) - %d),
  public_post_count = GREATEST(0, CAST(public_post_count AS SIGNED) - %d),
  public_comment_count = GREATEST(0, CAST(public_comment_count AS SIGNED) - %d)
WHERE member_id = ? AND board_id = ?`, postDelta, commentDelta, postDelta, commentDelta)

	if err := p.db.Exec(sql, mbID, boardID).Error; err != nil {
		p.logger.Error("activity-feed: stats decrement failed (%s/%s): %v", mbID, boardID, err)
	}
}

func (p *ActivityPlugin) isBoardSearchable(boardSlug string) bool {
	var boUseSearch int
	err := p.db.Table("g5_board").
		Select("bo_use_search").
		Where("bo_table = ?", boardSlug).
		Scan(&boUseSearch).Error
	if err != nil {
		return true // default to searchable
	}
	return boUseSearch == 1
}

var (
	reHTMLTag = regexp.MustCompile(`<[^>]*>`)
	reEmoTag  = regexp.MustCompile(`\{emo:[^}]+\}`)
	reMultiWS = regexp.MustCompile(`\s+`)
)

func stripHTMLPreview(s string) string {
	s = reHTMLTag.ReplaceAllString(s, "")
	s = reEmoTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reMultiWS.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return truncate(s, 200)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen])
}

func boolToInt8(b bool) int8 {
	if b {
		return 1
	}
	return 0
}

func toUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case uint64:
		return val
	case int:
		return uint64(val)
	case int64:
		return uint64(val)
	case float64:
		return uint64(val)
	case uint:
		return uint64(val)
	default:
		return 0
	}
}

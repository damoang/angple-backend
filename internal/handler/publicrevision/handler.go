// Package publicrevision 공개 수정 이력 API 핸들러.
package publicrevision

import (
	"errors"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	v1handler "github.com/damoang/angple-backend/internal/handler/v1"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 공개 수정 이력 API — 설정된 게시판(기본 claim)의 글·댓글 수정 이력을 읽기 권한이 있는 모든 사람에게 보여 준다.
//
// 공개 규칙은 service.BuildPublicRevisions 에 있다. 여기서는 접근 검사(게시판·비밀·삭제·가림)와
// 데이터 적재, 응답 정화만 한다. 기존 관리자용 /revisions API 는 건드리지 않는다.
//
// 게시판 읽기 레벨(bo_read_level)은 v1Boards 그룹 미들웨어가 :id 경로에 이미 강제한다.
//
// 숨김 테이블 g5_write_revision_hidden 은 자동 마이그레이션하지 않는다(수동 DDL).

var publicRevisionKST = time.FixedZone("KST", 9*60*60)

const publicRevisionHiddenTable = "g5_write_revision_hidden"

// publicRevisionTarget 글·댓글 한 행(공개 이력 판단에 필요한 열만).
type publicRevisionTarget struct {
	WrID        int        `gorm:"column:wr_id"`
	WrParent    int        `gorm:"column:wr_parent"`
	WrIsComment int        `gorm:"column:wr_is_comment"`
	MbID        string     `gorm:"column:mb_id"`
	WrOption    string     `gorm:"column:wr_option"`
	WrSubject   string     `gorm:"column:wr_subject"`
	WrContent   string     `gorm:"column:wr_content"`
	WrDatetime  time.Time  `gorm:"column:wr_datetime"`
	WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
}

func (t *publicRevisionTarget) isSecret() bool {
	return strings.Contains(t.WrOption, "secret")
}

type publicRevisionRow struct {
	WrID       int       `gorm:"column:wr_id"`
	Version    int       `gorm:"column:version"`
	ChangeType string    `gorm:"column:change_type"`
	Title      *string   `gorm:"column:title"`
	Content    *string   `gorm:"column:content"`
	EditedBy   string    `gorm:"column:edited_by"`
	EditedAt   time.Time `gorm:"column:edited_at"`
}

// publicRevisionHidden 숨김 테이블 행.
type publicRevisionHidden struct {
	BoardID  string    `gorm:"column:board_id;primaryKey"`
	WrID     int       `gorm:"column:wr_id;primaryKey"`
	Version  int       `gorm:"column:version;primaryKey"`
	HiddenBy string    `gorm:"column:hidden_by"`
	HiddenAt time.Time `gorm:"column:hidden_at"`
}

// TableName 숨김 테이블 이름.
func (publicRevisionHidden) TableName() string { return publicRevisionHiddenTable }

// API 공개 수정 이력 핸들러 묶음.
type API struct {
	DB     *gorm.DB
	Config service.PublicRevisionConfig
	// BoardAdmin 게시판의 bo_admin 을 돌려준다. 게시판이 없으면 ok=false.
	BoardAdmin func(slug string) (admin string, ok bool)
	// BlindedIDs 가려진(신고잠금·이용제한 근거) 글·댓글 ID 집합. 실패는 「모름」이라 에러로 돌려 공개하지 않게 한다.
	BlindedIDs func(slug string, ids []int, isComment bool) (map[int]bool, error)
}

// slugPattern 게시판 테이블 이름에 붙일 수 있는 slug.
var slugPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

var errPublicRevisionNotFound = errors.New("public revision target not found")

func kstWallClock(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	// DB 의 DATETIME 은 KST 벽시계로 저장된다(API 세션 time_zone=+09:00). 드라이버가 붙인
	// 로케이션 라벨과 무관하게 숫자 그대로 KST 로 읽는다.
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), publicRevisionKST)
}

// publicRevisionRespond 모든 응답은 개인화·숨김 반영 때문에 캐시하지 않는다.
func publicRevisionRespond(c *gin.Context, status int, ok bool, body gin.H) {
	if body == nil {
		body = gin.H{}
	}
	body["success"] = ok
	c.Header("Cache-Control", "private, no-store")
	c.JSON(status, body)
}

func publicRevisionFail(c *gin.Context, status int, msg string) {
	publicRevisionRespond(c, status, false, gin.H{"error": msg})
}

func publicRevisionNotFound(c *gin.Context) {
	publicRevisionFail(c, http.StatusNotFound, "Not found")
}

func publicRevisionServerError(c *gin.Context) {
	publicRevisionFail(c, http.StatusInternalServerError, "수정 이력을 불러오지 못했습니다")
}

// loadTarget 글(isComment=false) 또는 댓글 한 행.
func (a *API) loadTarget(slug string, wrID int, isComment bool) (*publicRevisionTarget, error) {
	if !slugPattern.MatchString(slug) || wrID <= 0 {
		return nil, errPublicRevisionNotFound
	}
	commentFlag := 0
	if isComment {
		commentFlag = 1
	}
	var rows []publicRevisionTarget
	if err := a.DB.Table("g5_write_"+slug).
		Select("wr_id, wr_parent, wr_is_comment, mb_id, wr_option, wr_subject, wr_content, wr_datetime, wr_deleted_at").
		Where("wr_id = ? AND wr_is_comment = ?", wrID, commentFlag).
		Limit(1).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errPublicRevisionNotFound
	}
	return &rows[0], nil
}

// checkVisible 비밀·삭제·가림 상태면 errPublicRevisionNotFound.
func (a *API) checkVisible(slug string, t *publicRevisionTarget) error {
	if t.WrDeletedAt != nil || t.isSecret() {
		return errPublicRevisionNotFound
	}
	blinded, err := a.BlindedIDs(slug, []int{t.WrID}, t.WrIsComment == 1)
	if err != nil {
		return err
	}
	if blinded[t.WrID] {
		return errPublicRevisionNotFound
	}
	return nil
}

// resolvePost 공개 대상 게시판의 볼 수 있는 글.
func (a *API) resolvePost(slug string, postID int) (*publicRevisionTarget, error) {
	if !a.Config.BoardEnabled(slug) {
		return nil, errPublicRevisionNotFound
	}
	if _, ok := a.BoardAdmin(slug); !ok {
		return nil, errPublicRevisionNotFound
	}
	post, err := a.loadTarget(slug, postID, false)
	if err != nil {
		return nil, err
	}
	if err := a.checkVisible(slug, post); err != nil {
		return nil, err
	}
	return post, nil
}

// resolveComment 볼 수 있는 글에 달린, 볼 수 있는 댓글.
func (a *API) resolveComment(slug string, postID, commentID int) (*publicRevisionTarget, error) {
	if _, err := a.resolvePost(slug, postID); err != nil {
		return nil, err
	}
	comment, err := a.loadTarget(slug, commentID, true)
	if err != nil {
		return nil, err
	}
	if comment.WrParent != postID {
		return nil, errPublicRevisionNotFound
	}
	if err := a.checkVisible(slug, comment); err != nil {
		return nil, err
	}
	return comment, nil
}

// staffAuthors 작성자 중 운영진 집합(사이트 관리자 레벨·설정 목록·bo_admin).
func (a *API) staffAuthors(slug string, authorIDs []string) (map[string]bool, error) {
	out := map[string]bool{}
	ids := make([]string, 0, len(authorIDs))
	seen := map[string]bool{}
	for _, id := range authorIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	type memberLevel struct {
		MbID    string `gorm:"column:mb_id"`
		MbLevel int    `gorm:"column:mb_level"`
	}
	var members []memberLevel
	if err := a.DB.Table("g5_member").Select("mb_id, mb_level").Where("mb_id IN ?", ids).Find(&members).Error; err != nil {
		return nil, err
	}
	levels := make(map[string]int, len(members))
	for _, m := range members {
		levels[m.MbID] = m.MbLevel
	}
	boardAdmin, _ := a.BoardAdmin(slug)
	for _, id := range ids {
		if a.Config.IsStaffAuthor(id, levels[id], boardAdmin) {
			out[id] = true
		}
	}
	return out, nil
}

// loadRecords 대상 ID 들의 이력 행(create·update)과 숨김 버전.
func (a *API) loadRecords(slug string, ids []int) (map[int][]service.RevisionRecord, map[int]map[int]bool, error) {
	records := map[int][]service.RevisionRecord{}
	hidden := map[int]map[int]bool{}
	if len(ids) == 0 {
		return records, hidden, nil
	}
	var rows []publicRevisionRow
	if err := a.DB.Table("g5_write_revisions").
		Select("wr_id, version, change_type, title, content, edited_by, edited_at").
		Where("board_id = ? AND wr_id IN ? AND change_type IN ?", slug, ids, []string{"create", "update"}).
		Order("wr_id, version").
		Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, r := range rows {
		records[r.WrID] = append(records[r.WrID], service.RevisionRecord{
			Version:    r.Version,
			ChangeType: r.ChangeType,
			Title:      r.Title,
			Content:    r.Content,
			EditedBy:   r.EditedBy,
			EditedAt:   kstWallClock(r.EditedAt),
		})
	}
	var hiddenRows []publicRevisionHidden
	if err := a.DB.Table(publicRevisionHiddenTable).
		Select("board_id, wr_id, version").
		Where("board_id = ? AND wr_id IN ?", slug, ids).
		Find(&hiddenRows).Error; err != nil {
		// ⛔ 숨김 여부를 모르면 공개하지 않는다(fail-closed).
		return nil, nil, err
	}
	for _, h := range hiddenRows {
		if hidden[h.WrID] == nil {
			hidden[h.WrID] = map[int]bool{}
		}
		hidden[h.WrID][h.Version] = true
	}
	return records, hidden, nil
}

func publicRevisionSource(t *publicRevisionTarget, records []service.RevisionRecord, hidden map[int]bool) service.PublicRevisionSource {
	src := service.PublicRevisionSource{
		AuthorID:       t.MbID,
		CreatedAt:      kstWallClock(t.WrDatetime),
		CurrentContent: t.WrContent,
		Records:        records,
		Hidden:         hidden,
	}
	if t.WrIsComment == 0 {
		title := t.WrSubject
		src.CurrentTitle = &title
	}
	return src
}

// buildFor 한 대상의 공개 이력(정화 적용). 작성자가 운영진이면 빈 목록.
func (a *API) buildFor(slug string, t *publicRevisionTarget) ([]service.PublicRevision, error) {
	staff, err := a.staffAuthors(slug, []string{t.MbID})
	if err != nil {
		return nil, err
	}
	if staff[t.MbID] {
		return []service.PublicRevision{}, nil
	}
	records, hidden, err := a.loadRecords(slug, []int{t.WrID})
	if err != nil {
		return nil, err
	}
	revs := service.BuildPublicRevisions(publicRevisionSource(t, records[t.WrID], hidden[t.WrID]))
	isComment := t.WrIsComment == 1
	for i := range revs {
		revs[i].Content = sanitizePublicRevisionContent(revs[i].Content, isComment, slug)
	}
	return revs, nil
}

// sanitizePublicRevisionContent 글 본문과 같은 정화 + 미디어 주소 정규화.
// 이메일 가림은 웹 서버 프록시가 본문·댓글과 같은 방식(표지 치환)으로 한다.
func sanitizePublicRevisionContent(content string, isComment bool, slug string) string {
	if isComment {
		return v1handler.NormalizeMediaContent(common.SanitizeComment(content), slug)
	}
	return v1handler.NormalizeMediaContent(common.SanitizePostContent(content), slug)
}

// canHide 숨기기 권한: 사이트 관리자(레벨 10 이상) 또는 해당 게시판 관리자.
func (a *API) canHide(c *gin.Context, slug string) bool {
	if middleware.GetUserLevel(c) >= 10 {
		return true
	}
	username := middleware.GetUsername(c)
	if username == "" {
		return false
	}
	boardAdmin, ok := a.BoardAdmin(slug)
	if !ok {
		return false
	}
	for _, id := range strings.FieldsFunc(boardAdmin, func(r rune) bool { return r == ',' || r == ' ' }) {
		if id == username {
			return true
		}
	}
	return false
}

func (a *API) respondList(c *gin.Context, slug string, t *publicRevisionTarget, err error) {
	if err != nil {
		a.respondError(c, err)
		return
	}
	revs, err := a.buildFor(slug, t)
	if err != nil {
		log.Printf("[public-revisions] 조회 실패 board=%s wr_id=%d: %v", slug, t.WrID, err)
		publicRevisionServerError(c)
		return
	}
	publicRevisionRespond(c, http.StatusOK, true, gin.H{
		"data":     revs,
		"can_hide": a.canHide(c, slug),
	})
}

func (a *API) respondError(c *gin.Context, err error) {
	if errors.Is(err, errPublicRevisionNotFound) {
		publicRevisionNotFound(c)
		return
	}
	log.Printf("[public-revisions] 대상 확인 실패 path=%s: %v", c.FullPath(), err)
	publicRevisionServerError(c)
}

func parsePositiveIntParam(c *gin.Context, name string) (int, bool) {
	v, err := strconv.Atoi(c.Param(name))
	if err != nil || v <= 0 {
		return 0, false
	}
	return v, true
}

// getPost GET /api/v1/boards/:slug/posts/:id/public-revisions
func (a *API) getPost(c *gin.Context) {
	slug := c.Param("slug")
	postID, ok := parsePositiveIntParam(c, "id")
	if !ok {
		publicRevisionNotFound(c)
		return
	}
	post, err := a.resolvePost(slug, postID)
	a.respondList(c, slug, post, err)
}

// getComment GET /api/v1/boards/:slug/posts/:id/comments/:comment_id/public-revisions
func (a *API) getComment(c *gin.Context) {
	slug := c.Param("slug")
	postID, ok1 := parsePositiveIntParam(c, "id")
	commentID, ok2 := parsePositiveIntParam(c, "comment_id")
	if !ok1 || !ok2 {
		publicRevisionNotFound(c)
		return
	}
	comment, err := a.resolveComment(slug, postID, commentID)
	a.respondList(c, slug, comment, err)
}

// getCounts GET /api/v1/boards/:slug/posts/:id/public-revisions/counts
//
// 글과 그 글의 모든 댓글에 대한 공개 수정 횟수를 한 번에 돌려준다(댓글별 개별 요청 방지).
// 0 인 항목은 싣지 않는다.
func (a *API) getCounts(c *gin.Context) {
	slug := c.Param("slug")
	postID, ok := parsePositiveIntParam(c, "id")
	if !ok {
		publicRevisionNotFound(c)
		return
	}
	post, err := a.resolvePost(slug, postID)
	if err != nil {
		a.respondError(c, err)
		return
	}
	postCount, commentCounts, err := a.countAll(slug, post)
	if err != nil {
		log.Printf("[public-revisions] 횟수 조회 실패 board=%s post=%d: %v", slug, postID, err)
		publicRevisionServerError(c)
		return
	}
	publicRevisionRespond(c, http.StatusOK, true, gin.H{
		"data": gin.H{
			"post":     postCount,
			"comments": commentCounts,
		},
	})
}

// visibleComments 글의 댓글 중 삭제·비밀·가림이 아닌 것.
func (a *API) visibleComments(slug string, postID int) ([]publicRevisionTarget, error) {
	var comments []publicRevisionTarget
	if err := a.DB.Table("g5_write_"+slug).
		Select("wr_id, wr_parent, wr_is_comment, mb_id, wr_option, wr_subject, wr_content, wr_datetime, wr_deleted_at").
		Where("wr_parent = ? AND wr_is_comment = 1 AND wr_deleted_at IS NULL", postID).
		Find(&comments).Error; err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(comments))
	for _, cm := range comments {
		ids = append(ids, cm.WrID)
	}
	blinded, err := a.BlindedIDs(slug, ids, true)
	if err != nil {
		return nil, err
	}
	out := make([]publicRevisionTarget, 0, len(comments))
	for _, cm := range comments {
		if cm.isSecret() || blinded[cm.WrID] {
			continue
		}
		out = append(out, cm)
	}
	return out, nil
}

// countAll 글·댓글 공개 수정 횟수. 쿼리 수는 댓글 수와 무관하다(댓글·이력·숨김·회원·가림 각 1회).
func (a *API) countAll(slug string, post *publicRevisionTarget) (int, map[string]int, error) {
	comments, err := a.visibleComments(slug, post.WrID)
	if err != nil {
		return 0, nil, err
	}
	targets := make([]*publicRevisionTarget, 0, len(comments)+1)
	targets = append(targets, post)
	for i := range comments {
		targets = append(targets, &comments[i])
	}
	ids := make([]int, 0, len(targets))
	authors := make([]string, 0, len(targets))
	for _, t := range targets {
		ids = append(ids, t.WrID)
		authors = append(authors, t.MbID)
	}
	staff, err := a.staffAuthors(slug, authors)
	if err != nil {
		return 0, nil, err
	}
	records, hidden, err := a.loadRecords(slug, ids)
	if err != nil {
		return 0, nil, err
	}
	postCount := 0
	commentCounts := map[string]int{}
	for _, t := range targets {
		if staff[t.MbID] || len(records[t.WrID]) == 0 {
			continue
		}
		n := service.CountPublicEdits(publicRevisionSource(t, records[t.WrID], hidden[t.WrID]))
		if n == 0 {
			continue
		}
		if t.WrIsComment == 1 {
			commentCounts[strconv.Itoa(t.WrID)] = n
		} else {
			postCount = n
		}
	}
	return postCount, commentCounts, nil
}

// hidePost POST /api/v1/boards/:slug/posts/:id/public-revisions/:version/hide
func (a *API) hidePost(c *gin.Context) {
	slug := c.Param("slug")
	postID, ok := parsePositiveIntParam(c, "id")
	if !ok {
		publicRevisionNotFound(c)
		return
	}
	a.hide(c, slug, postID, postID, false)
}

// hideComment POST /api/v1/boards/:slug/posts/:id/comments/:comment_id/public-revisions/:version/hide
func (a *API) hideComment(c *gin.Context) {
	slug := c.Param("slug")
	postID, ok1 := parsePositiveIntParam(c, "id")
	commentID, ok2 := parsePositiveIntParam(c, "comment_id")
	if !ok1 || !ok2 {
		publicRevisionNotFound(c)
		return
	}
	a.hide(c, slug, postID, commentID, true)
}

func (a *API) hide(c *gin.Context, slug string, postID, wrID int, isComment bool) {
	if !a.Config.BoardEnabled(slug) {
		publicRevisionNotFound(c)
		return
	}
	if !a.canHide(c, slug) {
		publicRevisionFail(c, http.StatusForbidden, "운영진만 숨길 수 있습니다")
		return
	}
	version, ok := parsePositiveIntParam(c, "version")
	if !ok {
		publicRevisionNotFound(c)
		return
	}
	target, err := a.loadTarget(slug, wrID, isComment)
	if err != nil {
		a.respondError(c, err)
		return
	}
	if isComment && target.WrParent != postID {
		publicRevisionNotFound(c)
		return
	}
	records, _, err := a.loadRecords(slug, []int{wrID})
	if err != nil {
		a.respondError(c, err)
		return
	}
	if !service.IsHideableVersion(records[wrID], version) {
		publicRevisionNotFound(c)
		return
	}
	row := publicRevisionHidden{
		BoardID:  slug,
		WrID:     wrID,
		Version:  version,
		HiddenBy: middleware.GetUsername(c),
		HiddenAt: time.Now().In(publicRevisionKST),
	}
	if err := a.DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		log.Printf("[public-revisions] 숨김 실패 board=%s wr_id=%d version=%d: %v", slug, wrID, version, err)
		publicRevisionServerError(c)
		return
	}
	publicRevisionRespond(c, http.StatusOK, true, nil)
}

// Register 공개 수정 이력 라우트를 v1 게시판 그룹에 붙인다.
func Register(v1Boards *gin.RouterGroup, a *API, auth gin.HandlerFunc) {
	v1Boards.GET("/:slug/posts/:id/public-revisions", a.getPost)
	v1Boards.GET("/:slug/posts/:id/public-revisions/counts", a.getCounts)
	v1Boards.GET("/:slug/posts/:id/comments/:comment_id/public-revisions", a.getComment)
	v1Boards.POST("/:slug/posts/:id/public-revisions/:version/hide", auth, a.hidePost)
	v1Boards.POST("/:slug/posts/:id/comments/:comment_id/public-revisions/:version/hide", auth, a.hideComment)
}

package v2

// live_bridge.go — 현세대(ang-gnu) 라이브 g5_ 스토어를 v2 API 계약으로 서빙하는 브리지.
//
// 배경: 앱/서드파티가 문서(swagger)대로 /api/v2 를 읽는데, v2_posts 는 죽은 2월 스냅샷이라
// 대부분 게시판이 비어 있음(free/qa/new=0). 라이브 글은 g5_write_* 에 있고 /api/v1 로만 나옴.
// 이 브리지는 v2 읽기 핸들러를 g5_ 라이브 리포로 재배선하되, 응답 JSON(V2Post/V2Comment)은 그대로
// 유지 → 앱 무변경/무재심사. board 메타는 fresh 한 v2_boards 를 유지하고 "글"만 라이브로 교체한다.
//
// parity by construction: 웹사이트가 쓰는 것과 동일한 gnu 리포 메서드 + 검증된 v1handler 변환을
// 재사용하므로, 앱 동작이 damoang.net 사이트와 동일해진다(삭제글/공지/정렬 처리 포함).

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	v1handler "github.com/damoang/angple-backend/internal/handler/v1"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"github.com/gin-gonic/gin"
)

// SetLiveReadRepos 는 현세대 라이브 읽기 리포를 주입한다. 주입되면 게시글/댓글 읽기 핸들러가
// v2_posts 대신 g5_write_* 라이브 데이터를 서빙한다(nil-safe: 미주입 시 기존 v2 경로 유지).
func (h *V2Handler) SetLiveReadRepos(w gnurepo.WriteRepository, b gnurepo.BoardRepository) {
	h.gnuWriteRepo = w
	h.gnuBoardRepo = b
}

// SetLiveFeedRepo 는 크로스보드 통합 피드 리포를 주입한다(GET /api/v2/feed).
func (h *V2Handler) SetLiveFeedRepo(repo gnurepo.MyPageRepository) {
	h.feedRepo = repo
}

var feedTagStrip = regexp.MustCompile(`<[^>]+>`)
var feedWhitespace = regexp.MustCompile(`\s+`)

// makeExcerpt 는 HTML 본문에서 태그를 제거해 카드용 짧은 발췌(최대 rune 140)를 만든다.
func makeExcerpt(html string) string {
	text := feedTagStrip.ReplaceAllString(html, " ")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.TrimSpace(feedWhitespace.ReplaceAllString(text, " "))
	r := []rune(text)
	if len(r) > 140 {
		return string(r[:140]) + "…"
	}
	return text
}

// liveAuthor 는 mb_id → v2_users 요약(정체성) 매핑 결과.
type liveAuthor struct {
	ID        uint64
	Nickname  string
	Level     uint8
	AvatarURL *string
}

// resolveLiveAuthors 는 작성자 mb_id 목록을 v2_users 로 배치 해석한다.
// 앱의 숫자 정체성(user_id, user.id)은 v2_users.id 이므로, 로그인/프로비저닝된 작성자만 매핑된다.
// 미프로비저닝 작성자는 맵에 없고, 호출부에서 user 를 생략하고 mb_nick 으로 폴백한다.
func (h *V2Handler) resolveLiveAuthors(mbIDs []string) map[string]liveAuthor {
	out := make(map[string]liveAuthor)
	if h.gnuDB == nil || len(mbIDs) == 0 {
		return out
	}
	seen := make(map[string]bool, len(mbIDs))
	uniq := make([]string, 0, len(mbIDs))
	for _, id := range mbIDs {
		if id != "" && !seen[id] {
			seen[id] = true
			uniq = append(uniq, id)
		}
	}
	if len(uniq) == 0 {
		return out
	}
	var rows []struct {
		ID        uint64
		Username  string
		Nickname  string
		Level     uint8
		AvatarURL *string
	}
	// v2_users 는 gnu g5_ 와 동일 DB(gnuDB) 에 있음(단일 커넥션).
	if err := h.gnuDB.Table("v2_users").
		Select("id, username, nickname, level, avatar_url").
		Where("username IN ?", uniq).
		Find(&rows).Error; err != nil {
		return out
	}
	for _, r := range rows {
		out[r.Username] = liveAuthor{ID: r.ID, Nickname: r.Nickname, Level: r.Level, AvatarURL: r.AvatarURL}
	}
	return out
}

// getBlockedMbIDs 는 요청자의 콘텐츠 차단 대상 mb_id(문자열) 목록을 반환한다.
// gnu 리포의 excludeMbIDs 파라미터에 그대로 사용(기존 getBlockedUserIDs 와 동일 소스).
func (h *V2Handler) getBlockedMbIDs(mbID string) []string {
	if h.blockRepo == nil || mbID == "" {
		return nil
	}
	ids, err := h.blockRepo.GetContentBlockedUserIDs(mbID)
	if err != nil {
		return nil
	}
	return ids
}

// liveNoticeIDs 는 게시판의 공지 wr_id 집합을 만든다(g5_board.bo_notice 파싱).
func liveNoticeIDs(board *gnuboard.G5Board) map[int]bool {
	if board == nil {
		return nil
	}
	return v1handler.BuildNoticeIDMap(gnurepo.ParseNoticeIDs(board.BoNotice))
}

// attachAuthor 는 해석된 작성자가 있으면 user_id/user 를 채운다(없으면 mb_nick 폴백 유지).
func attachAuthor(out map[string]any, mbID string, authors map[string]liveAuthor) {
	a, ok := authors[mbID]
	if !ok || a.ID == 0 {
		return
	}
	out["user_id"] = a.ID
	user := map[string]any{"id": a.ID, "nickname": a.Nickname, "level": a.Level}
	if a.AvatarURL != nil {
		user["avatar_url"] = *a.AvatarURL
	}
	out["user"] = user
}

// rfc3339Updated 는 v1 변환 맵의 updated_at(*time.Time, nil 가능)을 RFC3339 문자열로 정규화한다.
func rfc3339Updated(m map[string]any, fallback string) string {
	if ua, ok := m["updated_at"].(*time.Time); ok && ua != nil {
		return ua.Format(time.RFC3339)
	}
	return fallback
}

// toV2Post 는 G5Write 를 V2Post JSON 계약(앱이 읽는 필드)으로 변환한다.
// 검증된 v1handler 변환을 재사용(미디어 URL 정규화·썸네일·is_secret 등) 후 v2 필드명으로 re-key 한다.
func (h *V2Handler) toV2Post(w *gnuboard.G5Write, boardID uint64, boardSlug, boardName string, isNotice bool, authors map[string]liveAuthor, withContent bool) map[string]any {
	var m map[string]any
	if withContent {
		m = v1handler.TransformToV1PostDetail(w, isNotice, boardSlug)
	} else {
		m = v1handler.TransformToV1Post(w, isNotice)
	}
	status := "published"
	if w.WrDeletedAt != nil {
		status = postStatusDeleted
	}
	createdAt := w.WrDatetime.Format(time.RFC3339)
	out := map[string]any{
		"id":            w.WrID,
		"board_id":      boardID,
		"user_id":       uint64(0),
		"title":         w.WrSubject,
		"content":       "",
		"status":        status,
		"view_count":    w.WrHit,
		"comment_count": w.WrComment,
		"is_notice":     isNotice,
		"created_at":    createdAt,
		"updated_at":    rfc3339Updated(m, createdAt),
		"mb_nick":       w.WrName,
		"mb_id":         w.MbID,
		"good_count":    w.WrGood,
		"reactions":     map[string]int{"👍": w.WrGood},
		"board":         map[string]any{"slug": boardSlug, "name": boardName},
		"excerpt":       makeExcerpt(w.WrContent),
	}
	if withContent {
		if cv, ok := m["content"].(string); ok {
			out["content"] = cv
		}
	}
	// 썸네일은 계약 외 필드지만 무해(앱은 미지의 필드 무시) — 있으면 전달.
	if th, ok := m["thumbnail"]; ok {
		out["thumbnail"] = th
	}
	// 삭제글은 tombstone: 제목/본문/발췌/썸네일을 비워 내용 유출을 막는다(웹 "[삭제된 게시물입니다]" 동일).
	// 목록·상세 공통 경로라 여기 한 곳에서 처리하면 딥링크 상세 유출까지 차단된다.
	if status == postStatusDeleted {
		out["title"] = "삭제된 게시물입니다."
		out["content"] = ""
		out["excerpt"] = ""
		delete(out, "thumbnail")
	}
	attachAuthor(out, w.MbID, authors)
	return out
}

// toV2Comment 는 G5Write(댓글)를 V2Comment JSON 계약으로 변환한다.
func (h *V2Handler) toV2Comment(w *gnuboard.G5Write, authors map[string]liveAuthor) map[string]any {
	m := v1handler.TransformToV1Comment(w)
	status := "active"
	if w.WrDeletedAt != nil {
		status = postStatusDeleted
	}
	createdAt := w.WrDatetime.Format(time.RFC3339)
	out := map[string]any{
		"id":         w.WrID,
		"post_id":    w.WrParent,
		"user_id":    uint64(0),
		"content":    m["content"],
		"depth":      len(w.WrCommentReply),
		"status":     status,
		"created_at": createdAt,
		"updated_at": rfc3339Updated(m, createdAt),
		"mb_nick":    w.WrName,
		"mb_id":      w.MbID,
	}
	attachAuthor(out, w.MbID, authors)
	return out
}

// listPostsLive 는 GET /api/v2/boards/:slug/posts 를 라이브 g5_ 로 서빙한다.
func (h *V2Handler) listPostsLive(c *gin.Context, slug string) {
	// 가상 보드 "all"(태그 전체검색, /tags/*)은 g5_write_all 테이블이 없어 1146 을
	// 낸다. 크로스보드 태그 검색이 구현될 때까지 빈 목록으로 응답한다(기존 v2
	// 경로도 v2_boards 에 all 이 없어 404 였음 — 여기서 500 을 만들지 않는다).
	if slug == "all" {
		page, perPage := parsePagination(c)
		common.V2SuccessWithMeta(c, []map[string]any{}, common.NewV2Meta(page, perPage, 0))
		return
	}
	// board 메타: fresh 한 v2_boards 우선, 없으면 gnu board 로 이름 보강.
	var boardID uint64
	var boardName string
	if b, err := h.boardRepo.FindBySlug(slug); err == nil {
		boardID = b.ID
		boardName = b.Name
	}

	page, perPage := parsePagination(c)
	searchField := c.Query("sfl")
	searchQuery := c.Query("stx")
	blockedMbIDs := h.getBlockedMbIDs(middleware.GetUserID(c))

	var posts []*gnuboard.G5Write
	var total int64
	var err error
	if searchField != "" && searchQuery != "" {
		posts, total, err = h.gnuWriteRepo.SearchPosts(slug, searchField, searchQuery, page, perPage)
	} else {
		posts, total, err = h.gnuWriteRepo.FindPostsFiltered(slug, page, perPage, blockedMbIDs)
	}
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "게시글 목록 조회 실패", err)
		return
	}

	var noticeIDs map[int]bool
	if h.gnuBoardRepo != nil {
		if gb, e := h.gnuBoardRepo.FindByID(slug); e == nil {
			noticeIDs = liveNoticeIDs(gb)
			if boardName == "" {
				boardName = gb.BoSubject
			}
		}
	}

	mbIDs := make([]string, 0, len(posts))
	for _, p := range posts {
		mbIDs = append(mbIDs, p.MbID)
	}
	authors := h.resolveLiveAuthors(mbIDs)

	items := make([]map[string]any, len(posts))
	for i, p := range posts {
		items[i] = h.toV2Post(p, boardID, slug, boardName, noticeIDs[p.WrID], authors, false)
	}
	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, perPage, total))
}

// getPostLive 는 GET /api/v2/boards/:slug/posts/:id 를 라이브 g5_ 로 서빙한다.
func (h *V2Handler) getPostLive(c *gin.Context, slug string) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	// 비마이그레이션 보드(free 등)는 삭제 안 된 글이 wr_deleted_at='0000-00-00 00:00:00'(NULL 아님)이라
	// FindPostByID 의 `wr_deleted_at IS NULL` 필터가 전부 매칭 실패 → 모든 글이 NOT_FOUND 가 됨.
	// v1 GetPost 와 동일하게 IncludeDeleted 로 조회하고 삭제 여부는 Go 에서 판정한다(제로데이트→nil).
	w, err := h.gnuWriteRepo.FindPostByIDIncludeDeleted(slug, wrID)
	if err != nil || w == nil || w.WrID == 0 {
		common.V2ErrorResponse(c, http.StatusNotFound, "게시글을 찾을 수 없습니다", err)
		return
	}

	var boardID uint64
	var boardName string
	if b, e := h.boardRepo.FindBySlug(slug); e == nil {
		boardID = b.ID
		boardName = b.Name
	}
	var isNotice bool
	if h.gnuBoardRepo != nil {
		if gb, e := h.gnuBoardRepo.FindByID(slug); e == nil {
			isNotice = liveNoticeIDs(gb)[wrID]
			if boardName == "" {
				boardName = gb.BoSubject
			}
		}
	}

	authors := h.resolveLiveAuthors([]string{w.MbID})
	common.V2Success(c, h.toV2Post(w, boardID, slug, boardName, isNotice, authors, true))
}

// listCommentsLive 는 GET /api/v2/boards/:slug/posts/:id/comments 를 라이브 g5_ 로 서빙한다.
// gnu 댓글 리포는 페이지네이션 없이 전체를 반환 → 앱 계약(meta) 을 위해 수동 슬라이스.
func (h *V2Handler) listCommentsLive(c *gin.Context, slug string) {
	postID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "잘못된 게시글 ID", err)
		return
	}
	page, perPage := parsePagination(c)
	blockedMbIDs := h.getBlockedMbIDs(middleware.GetUserID(c))

	comments, err := h.gnuWriteRepo.FindCommentsFiltered(slug, postID, blockedMbIDs)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "댓글 목록 조회 실패", err)
		return
	}

	total := int64(len(comments))
	start := (page - 1) * perPage
	if start < 0 {
		start = 0
	}
	if start > len(comments) {
		start = len(comments)
	}
	end := start + perPage
	if end > len(comments) {
		end = len(comments)
	}
	pageComments := comments[start:end]

	mbIDs := make([]string, 0, len(pageComments))
	for _, cm := range pageComments {
		mbIDs = append(mbIDs, cm.MbID)
	}
	authors := h.resolveLiveAuthors(mbIDs)

	items := make([]map[string]any, len(pageComments))
	for i, cm := range pageComments {
		items[i] = h.toV2Comment(cm, authors)
	}
	common.V2SuccessWithMeta(c, items, common.NewV2Meta(page, perPage, total))
}

// --- 크로스보드 통합 피드 (GET /api/v2/feed) ---

// 커서 = 보드slug→wr_id 워터마크 맵. base64url(JSON) 로 인코딩.
func encodeFeedCursor(m map[string]int) string {
	if len(m) == 0 {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeFeedCursor(s string) map[string]int {
	out := map[string]int{}
	if s == "" {
		return out
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return out
	}
	_ = json.Unmarshal(b, &out)
	return out
}

// ListRecentFeed handles GET /api/v2/feed — 크로스보드 최신 타임라인(무한스크롤, 커서 기반).
// 응답 아이템은 board 목록과 동일 V2Post 형태(+excerpt). 차단 사용자 글은 SQL 에서 제외.
func (h *V2Handler) ListRecentFeed(c *gin.Context) {
	if h.feedRepo == nil {
		common.V2ErrorResponse(c, http.StatusNotFound, "피드를 사용할 수 없습니다", nil)
		return
	}
	limit := 20
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 30 {
		limit = v
	}
	cursor := decodeFeedCursor(c.Query("cursor"))
	blockedMbIDs := h.getBlockedMbIDs(middleware.GetUserID(c))

	// 순수 최신순: 보드별 최신 후보 풀(각 limit개)을 가져온다. 한 보드가 매우 활발하면 그 보드 글이
	// 상위를 차지하도록 board별 후보 수를 페이지 크기만큼 확보(리포가 20으로 상한).
	rows, err := h.feedRepo.FindRecentAcrossBoards(limit, cursor, blockedMbIDs)
	if err != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "피드 조회 실패", err)
		return
	}

	// 인터리브 캡 없이 시간순(wr_datetime DESC) 상위 limit개를 그대로 방출 = 신규 작성글이 무조건 위.
	emitted := make([]*gnuboard.FeedPost, 0, limit)
	for i := range rows {
		if len(emitted) >= limit {
			break
		}
		emitted = append(emitted, &rows[i])
	}

	// 게시판 이름 + 작성자 (emit된 것만)
	boardNames := map[string]string{}
	mbIDs := make([]string, 0, len(emitted))
	for _, fp := range emitted {
		slug := fp.BoardID
		if _, ok := boardNames[slug]; !ok {
			boardNames[slug] = slug
			if h.gnuBoardRepo != nil {
				if gb, e := h.gnuBoardRepo.FindByID(slug); e == nil {
					boardNames[slug] = gb.BoSubject
				}
			}
		}
		mbIDs = append(mbIDs, fp.MbID)
	}
	authors := h.resolveLiveAuthors(mbIDs)

	// 다음 커서 = 보드별 이번 페이지에서 emit된 최소 wr_id (미기여 보드는 이전 워터마크 유지)
	next := make(map[string]int, len(cursor))
	for k, v := range cursor {
		next[k] = v
	}
	items := make([]map[string]any, len(emitted))
	for i, fp := range emitted {
		slug := fp.BoardID
		items[i] = h.toV2Post(&fp.G5Write, 0, slug, boardNames[slug], false, authors, false)
		if cur, ok := next[slug]; !ok || fp.WrID < cur {
			next[slug] = fp.WrID
		}
	}

	hasMore := len(emitted) == limit
	nextCursor := ""
	if hasMore {
		nextCursor = encodeFeedCursor(next)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    items,
		"meta":    gin.H{"next_cursor": nextCursor, "has_more": hasMore},
	})
}

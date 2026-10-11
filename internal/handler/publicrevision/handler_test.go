package publicrevision

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/damoang/angple-backend/internal/service"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 공개 수정 이력 API 계약:
//   - 설정된 게시판(기본 claim) 밖은 404.
//   - 비밀·삭제·가림 글/댓글은 404. 다른 글의 댓글을 섞어 부르면 404.
//   - 운영진(사이트 관리자·설정 목록·bo_admin)이 쓴 글/댓글은 빈 목록.
//   - 응답에 수정한 사람·IP·메타데이터가 없고, 본문은 정화되며, 캐시하지 않는다.
//   - 숨기기는 사이트 관리자·게시판 관리자만.

const (
	prevTestEditorNick = "편집자닉네임"
	testBoardClaim     = "claim"
	testBoardFree      = "free"
)

func newPublicRevisionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	stmts := []string{
		`CREATE TABLE g5_write_claim (wr_id INTEGER PRIMARY KEY, wr_parent INTEGER, wr_is_comment INTEGER,
			mb_id TEXT, wr_option TEXT, wr_subject TEXT, wr_content TEXT, wr_ip TEXT,
			wr_datetime DATETIME, wr_deleted_at DATETIME)`,
		`CREATE TABLE g5_write_free (wr_id INTEGER PRIMARY KEY, wr_parent INTEGER, wr_is_comment INTEGER,
			mb_id TEXT, wr_option TEXT, wr_subject TEXT, wr_content TEXT, wr_ip TEXT,
			wr_datetime DATETIME, wr_deleted_at DATETIME)`,
		`CREATE TABLE g5_write_revisions (id INTEGER PRIMARY KEY AUTOINCREMENT, board_id TEXT, wr_id INTEGER,
			version INTEGER, change_type TEXT, title TEXT, content TEXT, edited_by TEXT, edited_by_name TEXT,
			edited_at DATETIME, metadata TEXT)`,
		`CREATE TABLE g5_write_revision_hidden (board_id TEXT, wr_id INTEGER, version INTEGER, hidden_by TEXT,
			hidden_at DATETIME, PRIMARY KEY (board_id, wr_id, version))`,
		`CREATE TABLE g5_member (mb_id TEXT PRIMARY KEY, mb_level INTEGER)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("테이블 생성 실패: %v", err)
		}
	}
	for _, m := range []struct {
		id    string
		level int
	}{{"u1", 4}, {"u2", 2}, {"adm", 10}, {"role1", 4}, {"boardadm", 4}} {
		db.Exec("INSERT INTO g5_member (mb_id, mb_level) VALUES (?, ?)", m.id, m.level)
	}

	type row struct {
		board, author, option, content string
		id, parent, isComment          int
		deleted                        bool
	}
	writes := []row{
		{board: testBoardClaim, id: 1, parent: 1, author: "u1", content: "<script>alert(1)</script>C2"},
		{board: testBoardClaim, id: 11, parent: 1, isComment: 1, author: "u2", content: "K1"},
		{board: testBoardClaim, id: 12, parent: 1, isComment: 1, author: "u1", option: "secret", content: "S1"},
		{board: testBoardClaim, id: 13, parent: 1, isComment: 1, author: "adm", content: "A1"},
		{board: testBoardClaim, id: 14, parent: 1, isComment: 1, author: "role1", content: "R1"},
		{board: testBoardClaim, id: 15, parent: 1, isComment: 1, author: "u2", content: "D1", deleted: true},
		{board: testBoardClaim, id: 2, parent: 2, author: "u1", option: "secret", content: "P2"},
		{board: testBoardClaim, id: 21, parent: 2, isComment: 1, author: "u2", content: "K21"},
		{board: testBoardClaim, id: 3, parent: 3, author: "u1", content: "P3", deleted: true},
		{board: testBoardClaim, id: 4, parent: 4, author: "u1", content: "P4"},
		{board: testBoardClaim, id: 5, parent: 5, author: "adm", content: "P5"},
		{board: testBoardClaim, id: 6, parent: 6, author: "u1", content: "final"},
		{board: testBoardFree, id: 1, parent: 1, author: "u1", content: "F2"},
	}
	for _, w := range writes {
		var deletedAt any
		if w.deleted {
			deletedAt = "2026-10-03 00:00:00"
		}
		if err := db.Exec("INSERT INTO g5_write_"+w.board+
			" (wr_id, wr_parent, wr_is_comment, mb_id, wr_option, wr_subject, wr_content, wr_ip, wr_datetime, wr_deleted_at)"+
			" VALUES (?,?,?,?,?,?,?,?,?,?)",
			w.id, w.parent, w.isComment, w.author, w.option, "제목", w.content, "10.0.0.1",
			"2026-09-01 09:59:00", deletedAt).Error; err != nil {
			t.Fatalf("글 삽입 실패: %v", err)
		}
	}

	type rev struct {
		board        string
		id, version  int
		kind, by, at string
		content      string
	}
	revs := make([]rev, 0, 32)
	revs = append(revs,
		rev{testBoardClaim, 1, 1, "create", "u1", "2026-09-01 10:00:00", "C0"},
		rev{testBoardClaim, 1, 2, "update", "u1", "2026-09-02 00:00:00", "C0"},
		rev{testBoardClaim, 1, 3, "update", "u1", "2026-10-05 12:00:00", "C1"},
	)
	// 댓글·기타 글: 작성자 본인 1회 수정
	for _, id := range []int{11, 12, 13, 14, 15, 21, 2, 3, 4} {
		author := map[int]string{11: "u2", 12: "u1", 13: "adm", 14: "role1", 15: "u2", 21: "u2"}[id]
		if author == "" {
			author = "u1"
		}
		revs = append(revs, rev{testBoardClaim, id, 1, "update", author, "2026-10-02 10:00:00", "old-" + strconv.Itoa(id)})
	}
	revs = append(revs,
		rev{testBoardClaim, 5, 1, "update", "adm", "2026-10-02 10:00:00", "old-5"},
		// 6: 작성자 → 운영진 정정(개인정보 삭제) → 작성자
		rev{testBoardClaim, 6, 1, "update", "u1", "2026-10-02 10:00:00", "orig"},
		rev{testBoardClaim, 6, 2, "update", "adm", "2026-10-02 11:00:00", "has PII mail"},
		rev{testBoardClaim, 6, 3, "update", "u1", "2026-10-02 12:00:00", "after fix"},
		rev{testBoardFree, 1, 1, "update", "u1", "2026-10-02 10:00:00", "F0"},
	)
	for _, r := range revs {
		if err := db.Exec(`INSERT INTO g5_write_revisions
			(board_id, wr_id, version, change_type, title, content, edited_by, edited_by_name, edited_at, metadata)
			VALUES (?,?,?,?,?,?,?,?,?,?)`,
			r.board, r.id, r.version, r.kind, "제목", r.content, r.by, prevTestEditorNick, r.at, `{"ip":"10.0.0.9"}`).Error; err != nil {
			t.Fatalf("이력 삽입 실패: %v", err)
		}
	}
	return db
}

func newPublicRevisionTestRouter(t *testing.T, cfg service.PublicRevisionConfig) (*gin.Engine, *gorm.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := newPublicRevisionTestDB(t)
	api := &API{
		DB:     db,
		Config: cfg,
		BoardAdmin: func(slug string) (string, bool) {
			switch slug {
			case testBoardClaim:
				return "boardadm", true
			case testBoardFree:
				return "", true
			}
			return "", false
		},
		BlindedIDs: func(_ string, ids []int, _ bool) (map[int]bool, error) {
			out := map[int]bool{}
			for _, id := range ids {
				if id == 4 {
					out[id] = true
				}
			}
			return out, nil
		},
	}
	r := gin.New()
	g := r.Group("/api/v1/boards")
	g.Use(func(c *gin.Context) {
		if u := c.GetHeader("X-Test-User"); u != "" {
			c.Set("userID", u)
			c.Set("username", u)
		}
		if lv := c.GetHeader("X-Test-Level"); lv != "" {
			n, _ := strconv.Atoi(lv)
			c.Set("level", n)
		}
		c.Next()
	})
	auth := func(c *gin.Context) {
		if c.GetString("username") == "" {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}
		c.Next()
	}
	Register(g, api, auth)
	return r, db
}

func defaultPublicRevisionConfig() service.PublicRevisionConfig {
	return service.PublicRevisionConfig{
		Boards:   map[string]bool{testBoardClaim: true},
		StaffIDs: map[string]bool{"role1": true},
	}
}

type publicRevisionTestResp struct {
	Success bool `json:"success"`
	Data    []struct {
		Version  int     `json:"version"`
		Content  string  `json:"content"`
		EditedAt *string `json:"edited_at"`
		Current  bool    `json:"current"`
	} `json:"data"`
	CanHide bool `json:"can_hide"`
}

func doPublicRevisionReq(r *gin.Engine, method, path, user, level string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	if user != "" {
		req.Header.Set("X-Test-User", user)
	}
	if level != "" {
		req.Header.Set("X-Test-Level", level)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodePublicRevisions(t *testing.T, w *httptest.ResponseRecorder) publicRevisionTestResp {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var out publicRevisionTestResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("JSON 해석 실패: %v", err)
	}
	return out
}

func TestPublicRevisions_PostHistoryAndHygiene(t *testing.T) {
	r, _ := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	w := doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/public-revisions", "", "")
	resp := decodePublicRevisions(t, w)
	if len(resp.Data) != 3 || resp.Data[0].Content != "C0" || resp.Data[1].Content != "C1" || !resp.Data[2].Current {
		t.Fatalf("이력 = %+v", resp.Data)
	}
	if resp.CanHide {
		t.Fatal("비로그인은 숨기기 권한이 없다")
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("Cache-Control = %q", cc)
	}
	body := w.Body.String()
	for _, forbidden := range []string{"edited_by", "metadata", "10.0.0", prevTestEditorNick, "wr_ip", "<script"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("응답에 %q 가 들어 있다: %s", forbidden, body)
		}
	}
	if resp.Data[1].EditedAt == nil || !strings.HasPrefix(*resp.Data[1].EditedAt, "2026-09-02T00:00:00+09:00") {
		t.Fatalf("edited_at 은 KST 여야 한다: %v", resp.Data[1].EditedAt)
	}
}

func TestPublicRevisions_NotFoundCases(t *testing.T) {
	r, _ := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	paths := map[string]string{
		"다른 게시판":    "/api/v1/boards/free/posts/1/public-revisions",
		"다른 게시판 횟수": "/api/v1/boards/free/posts/1/public-revisions/counts",
		"비밀글":       "/api/v1/boards/claim/posts/2/public-revisions",
		"비밀글의 댓글":   "/api/v1/boards/claim/posts/2/comments/21/public-revisions",
		"삭제글":       "/api/v1/boards/claim/posts/3/public-revisions",
		"가려진 글":     "/api/v1/boards/claim/posts/4/public-revisions",
		"없는 글":      "/api/v1/boards/claim/posts/999/public-revisions",
		"비밀 댓글":     "/api/v1/boards/claim/posts/1/comments/12/public-revisions",
		"삭제 댓글":     "/api/v1/boards/claim/posts/1/comments/15/public-revisions",
		"다른 글의 댓글":  "/api/v1/boards/claim/posts/1/comments/21/public-revisions",
	}
	for name, p := range paths {
		w := doPublicRevisionReq(r, http.MethodGet, p, "u1", "10")
		if w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404 (%s)", name, w.Code, w.Body.String())
		}
	}
}

func TestPublicRevisions_StaffAuthoredIsEmpty(t *testing.T) {
	r, _ := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	for name, p := range map[string]string{
		"관리자 글":       "/api/v1/boards/claim/posts/5/public-revisions",
		"관리자 댓글":      "/api/v1/boards/claim/posts/1/comments/13/public-revisions",
		"설정 목록 계정 댓글": "/api/v1/boards/claim/posts/1/comments/14/public-revisions",
	} {
		resp := decodePublicRevisions(t, doPublicRevisionReq(r, http.MethodGet, p, "", ""))
		if len(resp.Data) != 0 {
			t.Errorf("%s: 운영진 작성은 빈 목록이어야 한다: %+v", name, resp.Data)
		}
	}
	// 일반 회원 댓글은 공개
	resp := decodePublicRevisions(t, doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/comments/11/public-revisions", "", ""))
	if len(resp.Data) != 2 || resp.Data[0].Content != "old-11" || resp.Data[1].Content != "K1" {
		t.Fatalf("댓글 이력 = %+v", resp.Data)
	}
}

func TestPublicRevisions_NonAuthorCorrectionHidesEarlierVersions(t *testing.T) {
	r, _ := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	w := doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/6/public-revisions", "", "")
	resp := decodePublicRevisions(t, w)
	if len(resp.Data) != 2 || resp.Data[0].Content != "after fix" || resp.Data[1].Content != "final" {
		t.Fatalf("이력 = %+v", resp.Data)
	}
	if strings.Contains(w.Body.String(), "PII") || strings.Contains(w.Body.String(), "orig") {
		t.Fatalf("정정 이전 내용이 노출됐다: %s", w.Body.String())
	}
}

func TestPublicRevisions_Counts(t *testing.T) {
	r, _ := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	w := doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/public-revisions/counts", "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var out struct {
		Data struct {
			Post     int            `json:"post"`
			Comments map[string]int `json:"comments"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Data.Post != 2 {
		t.Fatalf("글 수정 횟수 = %d, want 2", out.Data.Post)
	}
	if len(out.Data.Comments) != 1 || out.Data.Comments["11"] != 1 {
		t.Fatalf("댓글 횟수 = %+v (비밀·삭제·운영진 댓글은 빠져야 한다)", out.Data.Comments)
	}
	if w.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("no-store 여야 한다")
	}
}

func TestPublicRevisions_HidePermissionAndEffect(t *testing.T) {
	r, db := newPublicRevisionTestRouter(t, defaultPublicRevisionConfig())
	hidePath := "/api/v1/boards/claim/posts/1/public-revisions/2/hide"

	if w := doPublicRevisionReq(r, http.MethodPost, hidePath, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("비로그인 숨기기 = %d, want 401", w.Code)
	}
	if w := doPublicRevisionReq(r, http.MethodPost, hidePath, "u2", "4"); w.Code != http.StatusForbidden {
		t.Fatalf("일반 회원 숨기기 = %d, want 403", w.Code)
	}
	if w := doPublicRevisionReq(r, http.MethodPost, hidePath, "role1", "4"); w.Code != http.StatusForbidden {
		t.Fatalf("게시판 관리자가 아닌 역할 계정 숨기기 = %d, want 403", w.Code)
	}
	if w := doPublicRevisionReq(r, http.MethodPost, "/api/v1/boards/free/posts/1/public-revisions/1/hide", "adm", "10"); w.Code != http.StatusNotFound {
		t.Fatalf("다른 게시판 숨기기 = %d, want 404", w.Code)
	}
	if w := doPublicRevisionReq(r, http.MethodPost, "/api/v1/boards/claim/posts/1/public-revisions/1/hide", "adm", "10"); w.Code != http.StatusNotFound {
		t.Fatalf("create 행 숨기기 = %d, want 404", w.Code)
	}

	// 운영진에게는 can_hide 가 켜진다.
	resp := decodePublicRevisions(t, doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/public-revisions", "boardadm", "4"))
	if !resp.CanHide {
		t.Fatal("게시판 관리자는 can_hide=true")
	}

	if w := doPublicRevisionReq(r, http.MethodPost, hidePath, "boardadm", "4"); w.Code != http.StatusOK {
		t.Fatalf("게시판 관리자 숨기기 = %d (%s)", w.Code, w.Body.String())
	}
	// 같은 버전을 다시 숨겨도 성공(중복 무시).
	if w := doPublicRevisionReq(r, http.MethodPost, hidePath, "adm", "10"); w.Code != http.StatusOK {
		t.Fatalf("관리자 재숨기기 = %d (%s)", w.Code, w.Body.String())
	}
	var n int64
	db.Table("g5_write_revision_hidden").Where("board_id = ? AND wr_id = ? AND version = ?", testBoardClaim, 1, 2).Count(&n)
	if n != 1 {
		t.Fatalf("숨김 행 = %d, want 1", n)
	}

	resp = decodePublicRevisions(t, doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/public-revisions", "", ""))
	if len(resp.Data) != 2 || resp.Data[0].Content != "C1" {
		t.Fatalf("숨긴 버전이 빠져야 한다: %+v", resp.Data)
	}

	// 댓글 숨기기 — 다른 글 경로로는 404.
	if w := doPublicRevisionReq(r, http.MethodPost, "/api/v1/boards/claim/posts/2/comments/11/public-revisions/1/hide", "adm", "10"); w.Code != http.StatusNotFound {
		t.Fatalf("다른 글 경로의 댓글 숨기기 = %d, want 404", w.Code)
	}
	if w := doPublicRevisionReq(r, http.MethodPost, "/api/v1/boards/claim/posts/1/comments/11/public-revisions/1/hide", "adm", "10"); w.Code != http.StatusOK {
		t.Fatalf("댓글 숨기기 = %d", w.Code)
	}
	resp = decodePublicRevisions(t, doPublicRevisionReq(r, http.MethodGet, "/api/v1/boards/claim/posts/1/comments/11/public-revisions", "", ""))
	if len(resp.Data) != 0 {
		t.Fatalf("현재 내용만 남으면 빈 목록: %+v", resp.Data)
	}
}

package handler

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PollHandler 는 글 부착형 투표(플러그인 poll)의 API 를 담당한다.
//
// 설계 원칙(/home/damoang/docs/poll-design.html):
//   - 익명 고정: 개별 투표 행(mb_id)은 이중투표 방지용으로만 저장하고
//     어떤 응답에도 누가 무엇을 찍었는지 싣지 않는다(집계 + 내 선택만).
//   - reveal='after_close' 인 열린 투표는 서버가 집계를 가린다(클라 가림 금지).
//   - 시간 비교는 전부 Go 에서 한다 — DSN loc 이 KST 라 SQL NOW()/UTC_TIMESTAMP()
//     와 섞으면 9시간 어긋난다(웹 refresh 토큰 만료 비교에서 실증된 함정).
type PollHandler struct {
	db *gorm.DB
}

// NewPollHandler creates a new PollHandler
func NewPollHandler(db *gorm.DB) *PollHandler {
	return &PollHandler{db: db}
}

var pollBoardRe = regexp.MustCompile(`^[a-z0-9_]{1,20}$`)

const (
	pollMinOptions = 2
	pollMaxOptions = 6
	pollMaxLabel   = 100
	pollMaxQuery   = 200
)

type pollRow struct {
	ID         uint64     `gorm:"column:id"`
	BoTable    string     `gorm:"column:bo_table"`
	WrID       int        `gorm:"column:wr_id"`
	MbID       string     `gorm:"column:mb_id"`
	Question   string     `gorm:"column:question"`
	AllowMulti int        `gorm:"column:allow_multi"`
	Reveal     string     `gorm:"column:reveal"`
	ClosesAt   *time.Time `gorm:"column:closes_at"`
	Status     string     `gorm:"column:status"`
}

type pollOptionRow struct {
	Idx        int    `gorm:"column:idx"`
	Label      string `gorm:"column:label"`
	VotesCount int    `gorm:"column:votes_count"`
}

func pollOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

func pollErr(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "error": gin.H{"message": msg}})
}

// effectiveOpen 은 마감시각을 지난 open 투표를 닫힘으로 취급한다(lazy close).
func (p pollRow) effectiveOpen(now time.Time) bool {
	if p.Status != "open" {
		return false
	}
	if p.ClosesAt != nil && now.After(*p.ClosesAt) {
		return false
	}
	return true
}

// loadPostAuthor 는 대상 글의 작성자를 반환한다. 댓글·삭제글·비밀글은 투표 부착 대상이 아니다.
// (비밀글 검사는 조회 시에도 적용 — be#644 첨부 게이트와 같은 이유)
func (h *PollHandler) loadPostAuthor(boTable string, wrID int) (mbID string, secret bool, err error) {
	var row struct {
		MbID        string     `gorm:"column:mb_id"`
		WrIsComment int        `gorm:"column:wr_is_comment"`
		WrOption    string     `gorm:"column:wr_option"`
		WrDeletedAt *time.Time `gorm:"column:wr_deleted_at"`
	}
	e := h.db.Table("g5_write_"+boTable).
		Select("mb_id, wr_is_comment, wr_option, wr_deleted_at").
		Where("wr_id = ?", wrID).Take(&row).Error
	if e != nil {
		return "", false, e
	}
	if row.WrIsComment != 0 || row.WrDeletedAt != nil {
		return "", false, gorm.ErrRecordNotFound
	}
	return row.MbID, strings.Contains(row.WrOption, "secret"), nil
}

type pollCreateRequest struct {
	BoTable       string   `json:"bo_table"`
	WrID          int      `json:"wr_id"`
	Question      string   `json:"question"`
	Options       []string `json:"options"`
	AllowMulti    bool     `json:"allow_multi"`
	Reveal        string   `json:"reveal"`
	DurationHours int      `json:"duration_hours"` // 0=무기한
}

// Create 는 글 작성자가 자기 글에 투표를 만든다. 글당 1개(UNIQUE).
func (h *PollHandler) Create(c *gin.Context) {
	me := middleware.GetUsername(c)
	if me == "" {
		pollErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	var req pollCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		pollErr(c, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	if !pollBoardRe.MatchString(req.BoTable) || req.WrID <= 0 {
		pollErr(c, http.StatusBadRequest, "잘못된 대상입니다.")
		return
	}
	if len(req.Options) < pollMinOptions || len(req.Options) > pollMaxOptions {
		pollErr(c, http.StatusBadRequest, fmt.Sprintf("선택지는 %d~%d개여야 합니다.", pollMinOptions, pollMaxOptions))
		return
	}
	for i := range req.Options {
		req.Options[i] = strings.TrimSpace(req.Options[i])
		if req.Options[i] == "" || len([]rune(req.Options[i])) > pollMaxLabel {
			pollErr(c, http.StatusBadRequest, "선택지 내용을 확인해 주세요.")
			return
		}
	}
	req.Question = strings.TrimSpace(req.Question)
	if len([]rune(req.Question)) > pollMaxQuery {
		pollErr(c, http.StatusBadRequest, "질문이 너무 깁니다.")
		return
	}
	if req.Reveal != "after_close" {
		req.Reveal = "after_vote"
	}
	if req.DurationHours < 0 || req.DurationHours > 24*30 {
		pollErr(c, http.StatusBadRequest, "기간을 확인해 주세요.")
		return
	}

	author, secret, err := h.loadPostAuthor(req.BoTable, req.WrID)
	if err != nil || secret {
		pollErr(c, http.StatusNotFound, "투표를 붙일 수 없는 글입니다.")
		return
	}
	if author != me {
		pollErr(c, http.StatusForbidden, "글 작성자만 투표를 만들 수 있습니다.")
		return
	}

	var closesAt *time.Time
	if req.DurationHours > 0 {
		t := time.Now().Add(time.Duration(req.DurationHours) * time.Hour)
		closesAt = &t
	}
	allowMulti := 0
	if req.AllowMulti {
		allowMulti = 1
	}

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Exec(
			`INSERT INTO angple_polls (bo_table, wr_id, mb_id, question, allow_multi, reveal, closes_at, status)
			 VALUES (?, ?, ?, ?, ?, ?, ?, 'open')`,
			req.BoTable, req.WrID, me, req.Question, allowMulti, req.Reveal, closesAt,
		)
		if res.Error != nil {
			return res.Error
		}
		var pollID uint64
		if e := tx.Raw(`SELECT LAST_INSERT_ID()`).Scan(&pollID).Error; e != nil {
			return e
		}
		for i, label := range req.Options {
			if e := tx.Exec(
				`INSERT INTO angple_poll_options (poll_id, idx, label, votes_count) VALUES (?, ?, ?, 0)`,
				pollID, i, label,
			).Error; e != nil {
				return e
			}
		}
		return nil
	})
	if txErr != nil {
		// UNIQUE(bo_table, wr_id) 위반 = 이미 투표가 있는 글
		pollErr(c, http.StatusConflict, "이 글에는 이미 투표가 있습니다.")
		return
	}
	h.respondByPost(c, req.BoTable, req.WrID, me)
}

// GetByPost 는 글의 투표를 집계와 함께 반환한다(비로그인 열람 가능).
func (h *PollHandler) GetByPost(c *gin.Context) {
	boTable := c.Param("board")
	wrID, err := strconv.Atoi(c.Param("post"))
	if !pollBoardRe.MatchString(boTable) || err != nil || wrID <= 0 {
		pollErr(c, http.StatusBadRequest, "잘못된 대상입니다.")
		return
	}
	me := middleware.GetUsername(c)
	h.respondByPost(c, boTable, wrID, me)
}

func (h *PollHandler) respondByPost(c *gin.Context, boTable string, wrID int, me string) {
	var poll pollRow
	err := h.db.Table("angple_polls").
		Where("bo_table = ? AND wr_id = ?", boTable, wrID).Take(&poll).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			pollErr(c, http.StatusInternalServerError, "잠시 후 다시 시도해 주세요.")
			return
		}
		canCreate := false
		if me != "" {
			if author, secret, e := h.loadPostAuthor(boTable, wrID); e == nil && !secret && author == me {
				canCreate = true
			}
		}
		pollOK(c, gin.H{"exists": false, "can_create": canCreate})
		return
	}

	// 삭제·비밀 전환된 글의 투표는 함께 숨긴다.
	if _, secret, e := h.loadPostAuthor(boTable, wrID); e != nil || secret {
		pollOK(c, gin.H{"exists": false, "can_create": false})
		return
	}

	now := time.Now()
	open := poll.effectiveOpen(now)

	var opts []pollOptionRow
	if e := h.db.Table("angple_poll_options").
		Where("poll_id = ?", poll.ID).Order("idx").Find(&opts).Error; e != nil {
		pollErr(c, http.StatusInternalServerError, "잠시 후 다시 시도해 주세요.")
		return
	}

	var totalVoters int64
	h.db.Table("angple_poll_votes").
		Where("poll_id = ?", poll.ID).
		Distinct("mb_id").Count(&totalVoters)

	myChoices := []int{}
	if me != "" {
		var idxs []int
		h.db.Table("angple_poll_votes").
			Where("poll_id = ? AND mb_id = ?", poll.ID, me).
			Order("option_idx").Pluck("option_idx", &idxs)
		if idxs != nil {
			myChoices = idxs
		}
	}

	// reveal='after_close' 인 열린 투표는 집계를 가린다 — 작성자 포함 전원(서버 강제).
	hideCounts := poll.Reveal == "after_close" && open
	options := make([]gin.H, 0, len(opts))
	for _, o := range opts {
		item := gin.H{"idx": o.Idx, "label": o.Label}
		if !hideCounts {
			item["votes"] = o.VotesCount
		}
		options = append(options, item)
	}

	resp := gin.H{
		"exists":       true,
		"id":           poll.ID,
		"question":     poll.Question,
		"options":      options,
		"allow_multi":  poll.AllowMulti == 1,
		"reveal":       poll.Reveal,
		"status":       map[bool]string{true: "open", false: "closed"}[open],
		"total_voters": totalVoters,
		"my_choices":   myChoices,
		"is_author":    me != "" && me == poll.MbID,
		"hide_counts":  hideCounts,
	}
	if poll.ClosesAt != nil {
		resp["closes_at"] = poll.ClosesAt.Format(time.RFC3339)
	}
	pollOK(c, resp)
}

type pollVoteRequest struct {
	OptionIdxs []int `json:"option_idxs"`
}

// Vote 는 투표하거나(첫 투표) 선택을 바꾼다(재투표). 마감 전만 가능.
func (h *PollHandler) Vote(c *gin.Context) {
	me := middleware.GetUsername(c)
	if me == "" {
		pollErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	pollID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || pollID == 0 {
		pollErr(c, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	var req pollVoteRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.OptionIdxs) == 0 {
		pollErr(c, http.StatusBadRequest, "선택지를 골라 주세요.")
		return
	}

	var poll pollRow
	if e := h.db.Table("angple_polls").Where("id = ?", pollID).Take(&poll).Error; e != nil {
		pollErr(c, http.StatusNotFound, "투표를 찾을 수 없습니다.")
		return
	}
	if !poll.effectiveOpen(time.Now()) {
		pollErr(c, http.StatusConflict, "마감된 투표입니다.")
		return
	}
	if poll.AllowMulti != 1 && len(req.OptionIdxs) != 1 {
		pollErr(c, http.StatusBadRequest, "하나만 선택할 수 있습니다.")
		return
	}

	var optCount int64
	h.db.Table("angple_poll_options").Where("poll_id = ?", pollID).Count(&optCount)
	seen := map[int]bool{}
	for _, idx := range req.OptionIdxs {
		if idx < 0 || int64(idx) >= optCount || seen[idx] {
			pollErr(c, http.StatusBadRequest, "잘못된 선택지입니다.")
			return
		}
		seen[idx] = true
	}

	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		// 재투표 = 기존 선택 전체 교체. PK(poll_id, mb_id, option_idx)가 이중투표를 막는다.
		if e := tx.Exec(`DELETE FROM angple_poll_votes WHERE poll_id = ? AND mb_id = ?`, pollID, me).Error; e != nil {
			return e
		}
		for _, idx := range req.OptionIdxs {
			if e := tx.Exec(
				`INSERT INTO angple_poll_votes (poll_id, mb_id, option_idx) VALUES (?, ?, ?)`,
				pollID, me, idx,
			).Error; e != nil {
				return e
			}
		}
		return h.recountTx(tx, pollID)
	})
	if txErr != nil {
		pollErr(c, http.StatusInternalServerError, "잠시 후 다시 시도해 주세요.")
		return
	}
	h.respondByPost(c, poll.BoTable, poll.WrID, me)
}

// Unvote 는 내 투표를 취소한다(마감 전).
func (h *PollHandler) Unvote(c *gin.Context) {
	me := middleware.GetUsername(c)
	if me == "" {
		pollErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	pollID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || pollID == 0 {
		pollErr(c, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	var poll pollRow
	if e := h.db.Table("angple_polls").Where("id = ?", pollID).Take(&poll).Error; e != nil {
		pollErr(c, http.StatusNotFound, "투표를 찾을 수 없습니다.")
		return
	}
	if !poll.effectiveOpen(time.Now()) {
		pollErr(c, http.StatusConflict, "마감된 투표입니다.")
		return
	}
	txErr := h.db.Transaction(func(tx *gorm.DB) error {
		if e := tx.Exec(`DELETE FROM angple_poll_votes WHERE poll_id = ? AND mb_id = ?`, pollID, me).Error; e != nil {
			return e
		}
		return h.recountTx(tx, pollID)
	})
	if txErr != nil {
		pollErr(c, http.StatusInternalServerError, "잠시 후 다시 시도해 주세요.")
		return
	}
	h.respondByPost(c, poll.BoTable, poll.WrID, me)
}

// Close 는 작성자·관리자가 투표를 조기 마감한다.
func (h *PollHandler) Close(c *gin.Context) {
	me := middleware.GetUsername(c)
	if me == "" {
		pollErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	pollID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || pollID == 0 {
		pollErr(c, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	var poll pollRow
	if e := h.db.Table("angple_polls").Where("id = ?", pollID).Take(&poll).Error; e != nil {
		pollErr(c, http.StatusNotFound, "투표를 찾을 수 없습니다.")
		return
	}
	if poll.MbID != me && middleware.GetUserLevel(c) < 10 {
		pollErr(c, http.StatusForbidden, "권한이 없습니다.")
		return
	}
	if e := h.db.Exec(`UPDATE angple_polls SET status = 'closed' WHERE id = ?`, pollID).Error; e != nil {
		pollErr(c, http.StatusInternalServerError, "잠시 후 다시 시도해 주세요.")
		return
	}
	h.respondByPost(c, poll.BoTable, poll.WrID, me)
}

// recountTx 는 옵션별 집계 캐시를 실제 행 수로 재계산한다(옵션 ≤6 이라 항상 저렴하고 정합).
func (h *PollHandler) recountTx(tx *gorm.DB, pollID uint64) error {
	return tx.Exec(
		`UPDATE angple_poll_options o
		 SET o.votes_count = (
		   SELECT COUNT(*) FROM angple_poll_votes v
		   WHERE v.poll_id = o.poll_id AND v.option_idx = o.idx
		 )
		 WHERE o.poll_id = ?`, pollID,
	).Error
}

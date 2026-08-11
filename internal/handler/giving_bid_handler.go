package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	givingdomain "github.com/damoang/angple-backend/internal/domain/giving"
	gnuboard "github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// givingHostFeePercent 은 유료(lowest_unique) 응모 시 주최자에게 지급되는 비율.
// 나머지(50%)는 소각된다. 레거시 giving_bid_ajax.php 의 고정 50% 정책 이식.
const givingHostFeePercent = 50

// adminMemberID 는 gnuboard 최고관리자 계정의 mb_id.
// 권한 비교에 문자열 리터럴이 흩어져 있어 상수로 모은다(goconst).
const adminMemberID = "admin"

// givingSeedSecret returns the server secret used to derive commit-reveal seeds
// and whether one is configured. Fail-closed: no hardcoded fallback — if neither
// GIVING_SEED_SECRET nor JWT_SECRET is set, ok is false and predictable-seed
// draws (random/ladder) must be refused rather than run with a guessable seed.
func givingSeedSecret() (string, bool) {
	if s := os.Getenv("GIVING_SEED_SECRET"); s != "" {
		return s, true
	}
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return s, true
	}
	return "", false
}

// givingIntPtrEqual reports whether two optional int settings are equal.
func givingIntPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// givingPostRow holds the giving post columns needed for bid/draw logic.
type givingPostRow struct {
	WrID      int    `gorm:"column:wr_id"`
	MbID      string `gorm:"column:mb_id"`
	WrSubject string `gorm:"column:wr_subject"`
	Wr2       string `gorm:"column:wr_2"` // 번호 단가
	Wr4       string `gorm:"column:wr_4"` // 시작
	Wr5       string `gorm:"column:wr_5"` // 종료
	Wr7       string `gorm:"column:wr_7"` // 상태 0/1/2
}

// givingBidRow is one active participation record.
type givingBidRow struct {
	MbID       string `gorm:"column:mb_id"`
	BidNumbers string `gorm:"column:bid_numbers"`
	BidCount   int    `gorm:"column:bid_count"`
	BidPoints  int    `gorm:"column:bid_points"`
}

// givingMetaRow mirrors g5_giving_meta.
type givingMetaRow struct {
	WrID      int    `gorm:"column:wr_id"`
	Method    string `gorm:"column:method"`
	Capacity  *int   `gorm:"column:capacity"`
	NumberMax *int   `gorm:"column:number_max"`
	SeedHash  string `gorm:"column:seed_hash"`
	Status    string `gorm:"column:status"`
}

// givingDrawRow mirrors g5_giving_draw.
type givingDrawRow struct {
	WrID          int             `gorm:"column:wr_id"`
	Method        string          `gorm:"column:method"`
	Seed          string          `gorm:"column:seed"`
	SeedHash      string          `gorm:"column:seed_hash"`
	WinnerMbID    string          `gorm:"column:winner_mb_id"`
	WinningNumber *int            `gorm:"column:winning_number"`
	ResultJSON    json.RawMessage `gorm:"column:result_json"`
	DrawnBy       string          `gorm:"column:drawn_by"`
	DrawnAt       time.Time       `gorm:"column:drawn_at"`
	// N-3: 수령확인·미수령 재추첨(수동 DDL 선행 4컬럼)
	ClaimedAt      *time.Time      `gorm:"column:claimed_at"`
	ClaimDue       *time.Time      `gorm:"column:claim_due"`
	RedrawCount    int             `gorm:"column:redraw_count"`
	ForfeitedMbIDs json.RawMessage `gorm:"column:forfeited_mb_ids"`
}

// N-3 상수: 수령 확인 창(24h) + 재추첨 상한(원당첨+2회).
const (
	givingClaimWindow = 24 * time.Hour
	givingMaxRedraw   = 2
)

func givingOK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": data})
}

// givingDrawNicknames resolves winner mb_ids to nicknames for display.
// 당첨자(winner_mb_id + result_json.winners)만 대상 — 조회 실패 시 nil 을
// 반환해 프론트가 mb_id 폴백으로 그리게 한다.
func (h *GivingHandler) givingDrawNicknames(draw givingDrawRow) map[string]string {
	idSet := make(map[string]struct{}, 4)
	if draw.WinnerMbID != "" {
		idSet[draw.WinnerMbID] = struct{}{}
	}
	if len(draw.ResultJSON) > 0 {
		var res struct {
			Winners []string `json:"winners"`
		}
		if json.Unmarshal(draw.ResultJSON, &res) == nil {
			for _, w := range res.Winners {
				if w != "" {
					idSet[w] = struct{}{}
				}
			}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	ids := make([]string, 0, len(idSet))
	for id := range idSet {
		ids = append(ids, id)
	}
	type memberNickRow struct {
		MbID string `gorm:"column:mb_id"`
		Nick string `gorm:"column:mb_nick"`
	}
	var rows []memberNickRow
	if err := h.db.Table("g5_member").
		Select("mb_id, mb_nick").
		Where("mb_id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil
	}
	nicknames := make(map[string]string, len(rows))
	for _, r := range rows {
		if r.Nick != "" {
			nicknames[r.MbID] = r.Nick
		}
	}
	return nicknames
}

func givingErr(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"success": false, "error": msg})
}

// loadGivingPost fetches the giving post row (author + schedule + unit price).
func (h *GivingHandler) loadGivingPost(wrID int) (*givingPostRow, error) {
	var row givingPostRow
	err := h.db.Table("g5_write_giving").
		Select("wr_id, mb_id, wr_subject, wr_2, wr_4, wr_5, wr_7").
		Where("wr_id = ? AND wr_is_comment = 0", wrID).
		Take(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// loadGivingMeta returns the meta row and whether it exists.
//
// ⛔ 없는 설정을 기본값으로 메우지 않는다(fail-closed).
//
// 예전에는 행이 없으면 lowest_unique/open 으로 폴백했는데, 나눔 글 작성이
// ①글 생성 → ②설정 저장(별도 호출) 두 단계라서 ②가 실패하면 주최자가 고른 방식과
// 무관하게 **유료 게임(lowest_unique)** 으로 표시되는 문제가 있었다.
// 작성폼이 그 실패를 조용히 삼키고 있어(catch {}) 주최자는 알 방법도 없었다.
//
// 또 ①직후 글이 게시판에 노출되므로 ②가 안착하기 전에 응모가 성립하면
// bidCount>0 이 되어 설정 변경이 409 로 잠기고 방식이 영구히 잘못 고정된다.
//
// 설정이 없으면 "준비 중"으로 두고 참가·개표를 모두 거부하면, 그 틈에 참가가
// 불가능해져 경쟁 조건 자체가 사라진다. 금전이 오가는 경로이므로 거부가 기본값이어야 한다.
func (h *GivingHandler) loadGivingMeta(wrID int) (givingMetaRow, bool) {
	var meta givingMetaRow
	if err := h.db.Table("g5_giving_meta").Where("wr_id = ?", wrID).Take(&meta).Error; err != nil {
		return givingMetaRow{WrID: wrID}, false
	}
	meta.Method = givingdomain.NormalizeMethod(meta.Method)
	return meta, true
}

// activeBids returns all active (bid_status=1) participations for a post.
func (h *GivingHandler) activeBids(wrID int) ([]givingBidRow, error) {
	var bids []givingBidRow
	err := h.db.Table("g5_giving_bid").
		Select("mb_id, bid_numbers, bid_count, bid_points").
		Where("bo_table = ? AND wr_id = ? AND bid_status = 1", givingBoardSlug, wrID).
		Order("bid_id ASC").
		Find(&bids).Error
	return bids, err
}

// isHostOrAdmin reports whether the caller may run host actions on the post.
func isGivingHostOrAdmin(c *gin.Context, authorID string) bool {
	mb := middleware.GetUsername(c)
	if mb == "" {
		return false
	}
	return mb == authorID || mb == adminMemberID || middleware.GetUserLevel(c) >= 10
}

// ---------------------------------------------------------------------------
// Config: 주최자 나눔 방식 설정 (POST /config/:id)
// ---------------------------------------------------------------------------

type givingConfigRequest struct {
	Method    string `json:"method"`
	Capacity  *int   `json:"capacity"`
	NumberMax *int   `json:"number_max"`
}

// Config upserts g5_giving_meta with the host-selected method + settings and
// commits the commit-reveal seed hash. Author (or admin) only.
func (h *GivingHandler) Config(c *gin.Context) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 글 번호입니다.")
		return
	}
	var req givingConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		givingErr(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다.")
		return
	}
	if !givingdomain.IsValidMethod(req.Method) {
		givingErr(c, http.StatusBadRequest, "지원하지 않는 나눔 방식입니다.")
		return
	}
	post, err := h.loadGivingPost(wrID)
	if err != nil {
		givingErr(c, http.StatusNotFound, "나눔 글을 찾을 수 없습니다.")
		return
	}
	if !isGivingHostOrAdmin(c, post.MbID) {
		givingErr(c, http.StatusForbidden, "주최자만 설정할 수 있습니다.")
		return
	}

	// 이미 응모(돈 낸 참가자)가 있는 게임은 규칙을 사후 변경할 수 없다:
	// method/number_max/capacity 변경 거부 + status 강제 리셋(open) 방지.
	statusVal := "open"
	var bidCount int64
	h.db.Table("g5_giving_bid").
		Where("bo_table = ? AND wr_id = ?", givingBoardSlug, wrID).
		Count(&bidCount)
	if bidCount > 0 {
		prev, _ := h.loadGivingMeta(wrID)
		if prev.Method != req.Method {
			givingErr(c, http.StatusConflict, "이미 응모가 있어 나눔 방식을 변경할 수 없습니다.")
			return
		}
		if !givingIntPtrEqual(prev.NumberMax, req.NumberMax) {
			givingErr(c, http.StatusConflict, "이미 응모가 있어 번호 상한을 변경할 수 없습니다.")
			return
		}
		// capacity 는 random/ladder 에서 **당첨자 수**다. 참가자 명단을 본 뒤 인원을
		// 늘리거나 줄일 수 있으면 규칙 사후 변경이다. 시드가 커밋돼 있어 *누가* 뽑힐지는
		// 못 고르지만 *몇 명*은 조정되므로 method/number_max 와 같이 잠근다.
		if !givingIntPtrEqual(prev.Capacity, req.Capacity) {
			givingErr(c, http.StatusConflict, "이미 응모가 있어 인원을 변경할 수 없습니다.")
			return
		}
		if prev.Status != "" {
			statusVal = prev.Status
		}
	}

	secret, _ := givingSeedSecret()
	seedHash := givingdomain.SeedHash(givingdomain.DeriveSeed(secret, givingBoardSlug, wrID))
	now := time.Now()

	// Preserve created_at on update; PK is wr_id.
	err = h.db.Exec(`
		INSERT INTO g5_giving_meta (wr_id, method, capacity, number_max, seed_hash, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE method=VALUES(method), capacity=VALUES(capacity),
			number_max=VALUES(number_max), seed_hash=VALUES(seed_hash), status=VALUES(status), updated_at=VALUES(updated_at)`,
		wrID, req.Method, req.Capacity, req.NumberMax, seedHash, statusVal, now, now).Error
	if err != nil {
		givingErr(c, http.StatusInternalServerError, "설정 저장에 실패했습니다.")
		return
	}
	givingOK(c, gin.H{"wr_id": wrID, "method": req.Method, "seed_hash": seedHash, "status": "open"})
}

// ---------------------------------------------------------------------------
// Detail: 상세 + 통계 + 내 참가 + (종료 시) 당첨 결과 (GET /detail/:id)
// ---------------------------------------------------------------------------

// Detail returns method meta, live stats, the caller's own participation, and
// the persisted draw result when present. Public (optional auth).
func (h *GivingHandler) Detail(c *gin.Context) { //nolint:gocyclo // 응답 조립 로직 응집 — 분해 시 통계/노출 경계 위험
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 글 번호입니다.")
		return
	}
	post, err := h.loadGivingPost(wrID)
	if err != nil {
		givingErr(c, http.StatusNotFound, "나눔 글을 찾을 수 없습니다.")
		return
	}
	meta, configured := h.loadGivingMeta(wrID)
	bids, _ := h.activeBids(wrID)

	participants := map[string]struct{}{}
	participantList := make([]string, 0, len(bids))
	totalNumbers := 0
	me := middleware.GetUsername(c)
	myNumbers := ""
	myCount := 0
	myPoints := 0
	for _, b := range bids {
		if _, ok := participants[b.MbID]; !ok {
			participantList = append(participantList, b.MbID)
		}
		participants[b.MbID] = struct{}{}
		totalNumbers += b.BidCount
		if me != "" && b.MbID == me {
			if myNumbers != "" && b.BidNumbers != "" {
				myNumbers += ","
			}
			myNumbers += b.BidNumbers
			myCount += b.BidCount
			myPoints += b.BidPoints
		}
	}

	norm := givingdomain.Normalize(time.Now(), givingdomain.Meta{
		StartRaw: post.Wr4, EndRaw: post.Wr5, StateRaw: post.Wr7,
		ParticipantCount: len(participants),
	})

	unitPrice, _ := strconv.Atoi(post.Wr2)

	// 설정(g5_giving_meta) 이 없으면 "준비 중" — 참가·개표 모두 거부되는 상태다.
	// UI 가 이를 구분해 주최자에게는 설정 완료를, 회원에게는 준비 중임을 알려야 한다.
	// method 를 임의 기본값으로 채우면 유료/무료가 뒤바뀌어 보이므로 빈 값으로 둔다.
	methodOut := ""
	if configured {
		methodOut = meta.Method
	}

	resp := gin.H{
		"wr_id":             wrID,
		"title":             post.WrSubject,
		"host_mb_id":        post.MbID,
		"configured":        configured,
		"method":            methodOut,
		"capacity":          meta.Capacity,
		"number_max":        meta.NumberMax,
		"seed_hash":         meta.SeedHash,
		"config_status":     meta.Status,
		"unit_price":        unitPrice,
		"status":            string(norm.Status),
		"is_paused":         norm.IsPaused,
		"is_urgent":         norm.IsUrgent,
		"giving_start":      norm.GivingStart,
		"giving_end":        norm.GivingEnd,
		"participant_count": len(participants),
		"participants":      participantList,
		"total_numbers":     totalNumbers,
		"total_bids":        len(bids),
		"is_host":           me != "" && (me == post.MbID || me == adminMemberID || middleware.GetUserLevel(c) >= 10),
		"my_participation": gin.H{
			"joined":  myCount > 0 || (me != "" && hasParticipant(participants, me)),
			"numbers": myNumbers,
			"count":   myCount,
			"points":  myPoints,
		},
	}

	// Persisted draw result (재현·검증용 시드/해시 포함).
	var draw givingDrawRow
	if h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&draw).Error == nil {
		resp["draw"] = gin.H{
			"method":         draw.Method,
			"winner_mb_id":   draw.WinnerMbID,
			"winning_number": draw.WinningNumber,
			"seed":           draw.Seed,
			"seed_hash":      draw.SeedHash,
			"drawn_by":       draw.DrawnBy,
			"drawn_at":       draw.DrawnAt,
			"result":         draw.ResultJSON,
			// N-3: 수령확인 상태(프론트가 당첨자에게 '수령 확인' 버튼/카운트다운 렌더).
			"claim_due":    draw.ClaimDue,
			"claimed_at":   draw.ClaimedAt,
			"redraw_count": draw.RedrawCount,
			// 표시용 닉네임 맵. 저장 데이터는 mb_id 정본 유지(닉변 추적 가능),
			// 프론트는 이 맵으로 그리고 없으면 mb_id 폴백.
			"nicknames": h.givingDrawNicknames(draw),
		}
	} else if norm.Status == givingdomain.StatusEnded && meta.Method == givingdomain.MethodLowestUnique {
		// 종료됐지만 미개표: 전량 공개로 재계산 검증 가능하게 응모 스냅샷 제공.
		reveal := make([]gin.H, 0, len(bids))
		for _, b := range bids {
			reveal = append(reveal, gin.H{"mb_id": b.MbID, "numbers": b.BidNumbers})
		}
		resp["reveal_bids"] = reveal
	}

	givingOK(c, resp)
}

func hasParticipant(set map[string]struct{}, mb string) bool {
	_, ok := set[mb]
	return ok
}

// ---------------------------------------------------------------------------
// Bid: 방식별 참가 (POST /bid/:id)
// ---------------------------------------------------------------------------

type givingBidRequest struct {
	Numbers string `json:"numbers"`
}

// Bid handles participation. lowest_unique consumes points (number×unit price)
// with a 50% host fee, all in one DB transaction; the free methods register a
// single 1-per-member entry. Self-giving and duplicate entries are rejected.
func (h *GivingHandler) Bid(c *gin.Context) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 글 번호입니다.")
		return
	}
	mbID := middleware.GetUsername(c)
	if mbID == "" {
		givingErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	post, err := h.loadGivingPost(wrID)
	if err != nil {
		givingErr(c, http.StatusNotFound, "나눔 글을 찾을 수 없습니다.")
		return
	}
	if mbID == post.MbID {
		givingErr(c, http.StatusForbidden, "주최자는 자신의 나눔에 참가할 수 없습니다.")
		return
	}
	meta, configured := h.loadGivingMeta(wrID)

	// ⛔ 설정 전에는 아무도 참가할 수 없다(fail-closed).
	//
	// 글 생성과 설정 저장이 두 호출로 나뉘어 있어 그 사이에 틈이 있다. 이때 참가를
	// 허용하면 bidCount>0 이 되어 설정이 409 로 잠기고, 주최자가 고른 방식과 다른
	// 게임으로 영구히 고정된다. 참가를 막으면 그 틈이 무해해진다.
	if !configured {
		givingErr(c, http.StatusConflict, "아직 준비 중인 나눔입니다. 잠시 후 다시 시도해주세요.")
		return
	}

	// 진행 상태 확인
	norm := givingdomain.Normalize(time.Now(), givingdomain.Meta{
		StartRaw: post.Wr4, EndRaw: post.Wr5, StateRaw: post.Wr7,
	})
	if norm.Status == givingdomain.StatusEnded {
		givingErr(c, http.StatusConflict, "이미 종료된 나눔입니다.")
		return
	}
	// 일정(시작·마감)이 비면 Normalize 가 no_giving 을 낸다. 프론트는 status=='active'
	// 일 때만 참가 UI를 그리므로 화면상으론 이미 막히지만, API 직접 호출까지 막아둔다.
	// 2026-07-23 첫 나눔(#2397)이 정확히 이 상태로 게시됐다 — 주최자도 회원도 몰랐다.
	if norm.Status == givingdomain.StatusNoGiving {
		givingErr(c, http.StatusConflict, "나눔 일정이 설정되지 않았습니다. 주최자에게 문의해주세요.")
		return
	}
	if norm.IsPaused || meta.Status == "paused" {
		givingErr(c, http.StatusConflict, "일시정지된 나눔입니다.")
		return
	}
	if norm.Status == givingdomain.StatusWaiting {
		givingErr(c, http.StatusConflict, "아직 시작되지 않은 나눔입니다.")
		return
	}

	if givingdomain.IsPaid(meta.Method) {
		h.bidLowestUnique(c, post, meta, mbID)
		return
	}
	h.bidFreeEntry(c, wrID, mbID, meta.Method)
}

// bidFreeEntry registers a free 1-per-member entry (random/ladder/curation/host_pick).
func (h *GivingHandler) bidFreeEntry(c *gin.Context, wrID int, mbID, method string) {
	var existing int64
	h.db.Table("g5_giving_bid").
		Where("bo_table = ? AND wr_id = ? AND mb_id = ? AND bid_status = 1", givingBoardSlug, wrID, mbID).
		Count(&existing)
	if existing > 0 {
		givingErr(c, http.StatusConflict, "이미 참가하셨습니다.")
		return
	}
	err := h.db.Exec(`
		INSERT INTO g5_giving_bid (bo_table, wr_id, mb_id, bid_numbers, bid_count, bid_points, bid_datetime, bid_ip, bid_status)
		VALUES (?, ?, ?, '', 1, 0, ?, ?, 1)`,
		givingBoardSlug, wrID, mbID, time.Now(), givingClientIP(c)).Error
	if err != nil {
		givingErr(c, http.StatusInternalServerError, "참가 처리에 실패했습니다.")
		return
	}
	givingOK(c, gin.H{"joined": true, "method": method})
}

// bidLowestUnique parses numbers, blocks duplicates, and settles points
// atomically: full deduction from the bidder + 50% credit to the host, both in
// one transaction with the bid insert (근본적 원자화 — 레거시 fix_missing_points 재발 방지).
func (h *GivingHandler) bidLowestUnique(c *gin.Context, post *givingPostRow, meta givingMetaRow, mbID string) { //nolint:gocyclo // 응모 정산 로직 응집 — 분해 시 트랜잭션 경계 위험
	var req givingBidRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		givingErr(c, http.StatusBadRequest, "응모 번호를 입력해주세요.")
		return
	}
	// 단가 미설정(<=0) 나눔은 lowest_unique 유료 응모 대상이 아니다.
	// 파싱보다 먼저 거부해 무료 대역응모("1-100000") 로 인한 재파싱 CPU DoS 를 차단.
	unitPrice, _ := strconv.Atoi(post.Wr2)
	if unitPrice <= 0 {
		givingErr(c, http.StatusBadRequest, "단가가 설정되지 않은 나눔입니다.")
		return
	}
	parsed := givingdomain.ParseBidNumbers(req.Numbers)
	if len(parsed) == 0 {
		givingErr(c, http.StatusBadRequest, "올바른 응모 번호를 입력해주세요.")
		return
	}
	// number_max 상한 집행: 규칙에 정한 최대 번호를 초과한 응모 거부.
	if meta.NumberMax != nil && *meta.NumberMax > 0 {
		over := make([]int, 0)
		for _, n := range parsed {
			if n > *meta.NumberMax {
				over = append(over, n)
			}
		}
		if len(over) > 0 {
			givingErr(c, http.StatusBadRequest, fmt.Sprintf("최대 번호(%d)를 초과했습니다: %s", *meta.NumberMax, givingdomain.FormatNumbers(over)))
			return
		}
	}
	cost := len(parsed) * unitPrice
	hostFee := cost * givingHostFeePercent / 100

	// 중복 번호 체크 (본인 기존 응모와 교집합)
	existingBids, _ := h.db.Table("g5_giving_bid").
		Select("bid_numbers").
		Where("bo_table = ? AND wr_id = ? AND mb_id = ? AND bid_status = 1", givingBoardSlug, post.WrID, mbID).
		Rows()
	owned := map[int]struct{}{}
	if existingBids != nil {
		for existingBids.Next() {
			var s string
			_ = existingBids.Scan(&s)
			for _, n := range givingdomain.ParseBidNumbers(s) {
				owned[n] = struct{}{}
			}
		}
		_ = existingBids.Close()
	}
	dups := make([]int, 0)
	for _, n := range parsed {
		if _, ok := owned[n]; ok {
			dups = append(dups, n)
		}
	}
	if len(dups) > 0 {
		givingErr(c, http.StatusConflict, fmt.Sprintf("이미 응모한 번호입니다: %s", givingdomain.FormatNumbers(dups)))
		return
	}

	pointConfig := h.pointConfig()
	relID := strconv.Itoa(post.WrID)
	uniqueTag := time.Now().Format("20060102150405.000000")

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if cost > 0 {
			// 잔액 잠금 + 확인
			var balance int
			if err := tx.Raw("SELECT mb_point FROM g5_member WHERE mb_id = ? FOR UPDATE", mbID).Scan(&balance).Error; err != nil {
				return err
			}
			if balance < cost {
				return errInsufficientPoints
			}
		}
		// 응모 기록
		if err := tx.Exec(`
			INSERT INTO g5_giving_bid (bo_table, wr_id, mb_id, bid_numbers, bid_count, bid_points, bid_datetime, bid_ip, bid_status)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			givingBoardSlug, post.WrID, mbID, req.Numbers, len(parsed), cost, time.Now(), givingClientIP(c)).Error; err != nil {
			return err
		}
		if cost > 0 {
			content := fmt.Sprintf("나눔 게시판 %d번 글 응모 (%d개 번호)", post.WrID, len(parsed))
			if err := givingDeductPointTx(tx, mbID, cost, content, givingBoardSlug, relID, "bid_"+uniqueTag); err != nil {
				return err
			}
		}
		if hostFee > 0 {
			content := fmt.Sprintf("나눔 게시판 %d번 글 응모 수수료 [%s님 응모, %d%%]", post.WrID, mbID, givingHostFeePercent)
			if err := givingCreditPointTx(tx, post.MbID, hostFee, content, givingBoardSlug, relID, "bidfee_"+uniqueTag, pointConfig); err != nil {
				return err
			}
		}
		return nil
	})
	if errors.Is(err, errInsufficientPoints) {
		givingErr(c, http.StatusPaymentRequired, "보유 포인트가 부족합니다.")
		return
	}
	if err != nil {
		givingErr(c, http.StatusInternalServerError, "응모 처리에 실패했습니다.")
		return
	}
	givingOK(c, gin.H{
		"joined":       true,
		"numbers":      givingdomain.FormatNumbers(parsed),
		"count":        len(parsed),
		"points_spent": cost,
		"host_fee":     hostFee,
	})
}

var errInsufficientPoints = fmt.Errorf("insufficient points")

func (h *GivingHandler) pointConfig() *v2repo.PointConfig {
	if h.pointConfigRepo == nil {
		return v2repo.DefaultPointConfig()
	}
	pc, err := h.pointConfigRepo.GetPointConfig()
	if err != nil || pc == nil {
		return v2repo.DefaultPointConfig()
	}
	return pc
}

// givingCreditPointTx adds points (positive) + writes a g5_point credit log,
// mirroring gnuboard_point_write_repo.addPositivePoint but tx-scoped for atomicity.
func givingCreditPointTx(tx *gorm.DB, mbID string, point int, content, relTable, relID, relAction string, pc *v2repo.PointConfig) error {
	expireDate := "9999-12-31"
	if pc != nil && pc.ExpiryEnabled && pc.ExpiryDays > 0 {
		expireDate = time.Now().AddDate(0, 0, pc.ExpiryDays).Format("2006-01-02")
	}
	if err := tx.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", point)).Error; err != nil {
		return err
	}
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}
	return tx.Create(&gnuboard.G5Point{
		MbID: mbID, PoDatetime: time.Now(), PoContent: content, PoPoint: point,
		PoUsePoint: 0, PoExpired: 0, PoExpireDate: expireDate,
		PoRelTable: relTable, PoRelID: relID, PoRelAction: relAction, MbPoint: mbPoint,
	}).Error
}

// givingDeductPointTx consumes points FIFO + writes a g5_point deduction log,
// mirroring gnuboard_point_write_repo.addNegativePoint but tx-scoped.
func givingDeductPointTx(tx *gorm.DB, mbID string, amount int, content, relTable, relID, relAction string) error {
	var credits []gnuboard.G5Point
	if err := tx.Raw(`
		SELECT po_id, po_point, po_use_point FROM g5_point
		WHERE mb_id = ? AND po_expired = 0 AND po_point > 0 AND (po_point - po_use_point) > 0
		ORDER BY po_expire_date ASC, po_id ASC FOR UPDATE`, mbID).Scan(&credits).Error; err != nil {
		return err
	}
	remaining := amount
	for _, credit := range credits {
		if remaining <= 0 {
			break
		}
		available := credit.PoPoint - credit.PoUsePoint
		consume := available
		if consume > remaining {
			consume = remaining
		}
		newUse := credit.PoUsePoint + consume
		updates := map[string]interface{}{"po_use_point": newUse}
		if newUse >= credit.PoPoint {
			updates["po_expired"] = 100
		}
		if err := tx.Table("g5_point").Where("po_id = ?", credit.PoID).Updates(updates).Error; err != nil {
			return err
		}
		remaining -= consume
	}
	if err := tx.Table("g5_member").Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point - ?", amount)).Error; err != nil {
		return err
	}
	var mbPoint int
	if err := tx.Table("g5_member").Select("mb_point").Where("mb_id = ?", mbID).Scan(&mbPoint).Error; err != nil {
		return err
	}
	return tx.Create(&gnuboard.G5Point{
		MbID: mbID, PoDatetime: time.Now(), PoContent: content, PoPoint: -amount,
		PoUsePoint: 0, PoExpired: 0, PoExpireDate: "9999-12-31",
		PoRelTable: relTable, PoRelID: relID, PoRelAction: relAction, MbPoint: mbPoint,
	}).Error
}

// ---------------------------------------------------------------------------
// Draw: 개표 (POST /draw/:id)
// ---------------------------------------------------------------------------

type givingDrawRequest struct {
	WinnerMbID string `json:"winner_mb_id"`
	Reason     string `json:"reason"`
}

// Draw runs (or, for host-designated methods, records) the draw and persists the
// authoritative result to g5_giving_draw. Author (or admin) only; idempotent.
func (h *GivingHandler) Draw(c *gin.Context) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 글 번호입니다.")
		return
	}
	post, err := h.loadGivingPost(wrID)
	if err != nil {
		givingErr(c, http.StatusNotFound, "나눔 글을 찾을 수 없습니다.")
		return
	}
	if !isGivingHostOrAdmin(c, post.MbID) {
		givingErr(c, http.StatusForbidden, "주최자만 개표할 수 있습니다.")
		return
	}
	// 멱등: 이미 개표됐으면 기존 결과 반환
	var existing givingDrawRow
	if h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&existing).Error == nil {
		givingOK(c, gin.H{"already_drawn": true, "winner_mb_id": existing.WinnerMbID, "method": existing.Method})
		return
	}
	var req givingDrawRequest
	_ = c.ShouldBindJSON(&req)

	// 설정 없이 개표하면 어떤 방식으로 뽑았는지 근거가 남지 않는다 — 거부한다.
	meta, configured := h.loadGivingMeta(wrID)
	if !configured {
		givingErr(c, http.StatusConflict, "나눔 설정이 없어 개표할 수 없습니다. 설정을 먼저 완료해주세요.")
		return
	}
	drawnBy := middleware.GetUsername(c)
	if err := h.runDraw(wrID, post, meta, req, drawnBy); err != nil {
		var de givingDrawError
		if errors.As(err, &de) {
			givingErr(c, de.status, de.msg)
			return
		}
		givingErr(c, http.StatusInternalServerError, "개표에 실패했습니다.")
		return
	}
	// 개표 완료 → 설정 상태 종료
	h.db.Exec("UPDATE g5_giving_meta SET status = 'drawn', updated_at = ? WHERE wr_id = ?", time.Now(), wrID)
	h.NotifyDrawResult(wrID)

	var saved givingDrawRow
	h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&saved)
	givingOK(c, gin.H{
		"drawn":          true,
		"method":         saved.Method,
		"winner_mb_id":   saved.WinnerMbID,
		"winning_number": saved.WinningNumber,
		"seed":           saved.Seed,
		"seed_hash":      saved.SeedHash,
		"result":         saved.ResultJSON,
	})
}

type givingDrawError struct {
	status int
	msg    string
}

func (e givingDrawError) Error() string { return e.msg }

// runDraw computes the winner(s) per method and persists one g5_giving_draw row.
func (h *GivingHandler) runDraw(wrID int, _ *givingPostRow, meta givingMetaRow, req givingDrawRequest, drawnBy string) error { //nolint:gocyclo // 개표 로직 응집 — 분해 시 트랜잭션/방식 경계 위험
	bids, err := h.activeBids(wrID)
	if err != nil {
		return err
	}

	// 정렬된 고유 참가자 목록 (검증 재현용 안정 입력)
	seen := map[string]struct{}{}
	participants := make([]string, 0)
	for _, b := range bids {
		if _, ok := seen[b.MbID]; !ok {
			seen[b.MbID] = struct{}{}
			participants = append(participants, b.MbID)
		}
	}
	sortStrings(participants)

	method := meta.Method
	secret, secretOK := givingSeedSecret()
	// random/ladder 는 예측 불가능한 서버 시드가 필수 — 시드 미설정 시 개표 거부(fail-closed).
	if (method == givingdomain.MethodRandom || method == givingdomain.MethodLadder) && !secretOK {
		return givingDrawError{http.StatusInternalServerError, "개표 시드가 설정되지 않았습니다."}
	}
	seed := givingdomain.DeriveSeed(secret, givingBoardSlug, wrID)
	seedHash := givingdomain.SeedHash(seed)
	inputHash := givingdomain.InputHash(participants)
	capacity := 1
	if meta.Capacity != nil && *meta.Capacity > 0 {
		capacity = *meta.Capacity
	}

	var winnerMbID string
	var winningNumber *int
	result := gin.H{
		"method":       method,
		"participants": participants,
		"input_hash":   inputHash,
		"drawn_by":     drawnBy,
	}

	switch method {
	case givingdomain.MethodLowestUnique:
		byNumber := map[int][]string{}
		for _, b := range bids {
			for _, n := range givingdomain.ParseBidNumbers(b.BidNumbers) {
				byNumber[n] = append(byNumber[n], b.MbID)
			}
		}
		if num, mb, ok := givingdomain.LowestUniqueWinner(byNumber); ok {
			winnerMbID = mb
			n := num
			winningNumber = &n
			result["winning_number"] = num
		} else {
			result["no_winner"] = true
		}

	case givingdomain.MethodRandom:
		winners := givingdomain.RandomWinners(seed, participants, capacity)
		result["winners"] = winners
		result["seed"] = seed
		result["seed_hash"] = seedHash
		result["capacity"] = capacity
		if len(winners) > 0 {
			winnerMbID = winners[0]
		}

	case givingdomain.MethodLadder:
		ladder := givingdomain.BuildLadder(seed, participants, capacity)
		result["ladder"] = ladder
		result["winners"] = ladder.Winners
		result["seed"] = seed
		result["seed_hash"] = seedHash
		result["capacity"] = capacity
		if len(ladder.Winners) > 0 {
			winnerMbID = ladder.Winners[0]
		}

	case givingdomain.MethodCuration, givingdomain.MethodHostPick:
		if req.WinnerMbID == "" {
			return givingDrawError{http.StatusBadRequest, "당첨자를 지정해주세요."}
		}
		if givingdomain.RequiresReason(method) && req.Reason == "" {
			return givingDrawError{http.StatusBadRequest, "선정 사유를 입력해주세요."}
		}
		if !h.isGivingParticipant(wrID, req.WinnerMbID, seen) {
			return givingDrawError{http.StatusBadRequest, "참가자 또는 댓글 작성자만 지정할 수 있습니다."}
		}
		winnerMbID = req.WinnerMbID
		result["reason"] = req.Reason
		result["designated"] = true

	default:
		return givingDrawError{http.StatusBadRequest, "지원하지 않는 나눔 방식입니다."}
	}

	resultBytes, _ := json.Marshal(result)
	// commit-reveal 시드는 auto 방식에서만 공개
	storeSeed := ""
	storeSeedHash := ""
	if method == givingdomain.MethodRandom || method == givingdomain.MethodLadder {
		storeSeed = seed
		storeSeedHash = seedHash
	}
	// N-3: 자동방식·정원1명·당첨자 있음이면 24h 수령 창을 연다(재추첨 대상).
	var claimDue *time.Time
	if givingdomain.IsAutoDraw(method) && capacity == 1 && winnerMbID != "" {
		t := time.Now().Add(givingClaimWindow)
		claimDue = &t
	}
	return h.db.Exec(`
		INSERT INTO g5_giving_draw (wr_id, method, seed, seed_hash, winner_mb_id, winning_number, result_json, drawn_by, drawn_at, claim_due)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		wrID, method, nullIfEmpty(storeSeed), nullIfEmpty(storeSeedHash),
		nullIfEmpty(winnerMbID), winningNumber, string(resultBytes), drawnBy, time.Now(), claimDue).Error
}

// isGivingParticipant reports whether mb entered the draw or commented on the post.
func (h *GivingHandler) isGivingParticipant(wrID int, mb string, entrants map[string]struct{}) bool {
	if _, ok := entrants[mb]; ok {
		return true
	}
	var cnt int64
	h.db.Table("g5_write_giving").
		Where("wr_parent = ? AND wr_is_comment = 1 AND mb_id = ?", wrID, mb).
		Count(&cnt)
	return cnt > 0
}

// ---------------------------------------------------------------------------
// Admin: 일시정지 / 재개 / 강제종료 (POST /admin/:id/{pause,resume,force-stop})
// ---------------------------------------------------------------------------

// AdminAction pauses, resumes, or force-stops a giving. force-stop ends the
// giving and, for auto-draw methods, runs the draw immediately.
func (h *GivingHandler) AdminAction(c *gin.Context) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 글 번호입니다.")
		return
	}
	action := c.Param("action")
	post, err := h.loadGivingPost(wrID)
	if err != nil {
		givingErr(c, http.StatusNotFound, "나눔 글을 찾을 수 없습니다.")
		return
	}
	if !isGivingHostOrAdmin(c, post.MbID) {
		givingErr(c, http.StatusForbidden, "주최자만 제어할 수 있습니다.")
		return
	}
	now := time.Now()
	switch action {
	case "pause":
		h.db.Exec("UPDATE g5_write_giving SET wr_7 = '1', wr_8 = ? WHERE wr_id = ?", now.Format("2006-01-02 15:04:05"), wrID)
		h.db.Exec("UPDATE g5_giving_meta SET status = 'paused', updated_at = ? WHERE wr_id = ?", now, wrID)
		givingOK(c, gin.H{"status": "paused"})
	case "resume":
		h.db.Exec("UPDATE g5_write_giving SET wr_7 = '0' WHERE wr_id = ?", wrID)
		h.db.Exec("UPDATE g5_giving_meta SET status = 'open', updated_at = ? WHERE wr_id = ?", now, wrID)
		givingOK(c, gin.H{"status": "open"})
	case "force-stop":
		h.db.Exec("UPDATE g5_write_giving SET wr_7 = '2', wr_8 = ? WHERE wr_id = ?", now.Format("2006-01-02 15:04:05"), wrID)
		// 미설정이면 자동 개표하지 않는다 — 방식이 없으므로 뽑을 근거가 없다.
		meta, configured := h.loadGivingMeta(wrID)
		if configured && givingdomain.IsAutoDraw(meta.Method) {
			var existing int64
			h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Count(&existing)
			if existing == 0 {
				if err := h.runDraw(wrID, post, meta, givingDrawRequest{}, middleware.GetUsername(c)); err == nil {
					h.NotifyDrawResult(wrID)
				}
			}
		}
		h.db.Exec("UPDATE g5_giving_meta SET status = 'ended', updated_at = ? WHERE wr_id = ?", now, wrID)
		givingOK(c, gin.H{"status": "ended"})
	default:
		givingErr(c, http.StatusBadRequest, "알 수 없는 동작입니다.")
	}
}

// givingClientIP returns the caller IP clamped to the bid_ip varchar(15) width.
func givingClientIP(c *gin.Context) string {
	ip := middleware.GetClientIP(c)
	if len(ip) > 15 {
		return ip[:15]
	}
	return ip
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// ===== 마감 자동 개표 스윕 + 개표 알림 (cron: /api/internal/cron/giving-draw-sweep) =====
//
// 배경(2026-08-07 실측): 시간 기반 개표 트리거가 없어 giving/2405 가 마감 12일이
// 지나도록 status=open 미개표로 방치됐고, 개표가 되어도 당첨자에게 아무 알림이
// 가지 않았다(참가 150명 나눔도 당첨자가 직접 들어와 확인해야 했다).

// GivingSweepResult 는 스윕 1회의 처리 결과다.
type GivingSweepResult struct {
	Due       int      `json:"due"`        // 마감 경과 + 미개표
	AutoDrawn int      `json:"auto_drawn"` // 자동 개표 실행
	Reminded  int      `json:"reminded"`   // 지명 방식 주최자 독촉
	Redrawn   int      `json:"redrawn"`    // N-3: 미수령 24h 경과 재추첨
	Errors    []string `json:"errors,omitempty"`
}

// givingKSTNow 는 wr_5('2006-01-02T15:04', KST 문자열)와 같은 포맷의 현재 시각.
// ISO 형태라 사전순 비교가 시간순 비교와 일치한다.
func givingKSTNow() string {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*3600)
	}
	return time.Now().In(loc).Format("2006-01-02T15:04")
}

// RunDueDrawSweep 는 마감이 지난 open 나눔을 찾아
//   - 자동 방식(random/ladder/lowest_unique): runDraw 실행(멱등) + 당첨 알림
//   - 지명 방식(curation/host_pick): 주최자에게 개표 독촉 알림(1회)
//
// runDraw 는 g5_giving_draw 존재 시 기존 결과를 재사용하므로 중복 실행에 안전하다.
func (h *GivingHandler) RunDueDrawSweep() (*GivingSweepResult, error) {
	res := &GivingSweepResult{}
	now := givingKSTNow()

	type dueRow struct {
		WrID    int    `gorm:"column:wr_id"`
		Method  string `gorm:"column:method"`
		Subject string `gorm:"column:wr_subject"`
		HostID  string `gorm:"column:mb_id"`
	}
	var due []dueRow
	err := h.db.Table("g5_giving_meta m").
		Select("m.wr_id, m.method, w.wr_subject, w.mb_id").
		Joins("JOIN g5_write_giving w ON w.wr_id = m.wr_id").
		Where("m.status = 'open'").
		Where("w.wr_5 <> '' AND w.wr_5 <= ?", now).
		Where("NOT EXISTS (SELECT 1 FROM g5_giving_draw d WHERE d.wr_id = m.wr_id)").
		Find(&due).Error
	if err != nil {
		return nil, err
	}
	res.Due = len(due)

	for _, row := range due {
		method := givingdomain.NormalizeMethod(row.Method)
		if !givingdomain.IsAutoDraw(method) {
			if h.notifyOnce(row.HostID, "giving_remind", row.WrID,
				fmt.Sprintf("⏰ 나눔 「%s」 마감! 당첨자 지정(개표)을 진행해 주세요.", givingTrim(row.Subject))) {
				res.Reminded++
			}
			continue
		}

		post, perr := h.loadGivingPost(row.WrID)
		if perr != nil || post == nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%d: 글 조회 실패", row.WrID))
			continue
		}
		meta, ok := h.loadGivingMeta(row.WrID)
		if !ok {
			continue
		}
		if derr := h.runDraw(row.WrID, post, meta, givingDrawRequest{}, "cron"); derr != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%d: %v", row.WrID, derr))
			continue
		}
		h.db.Exec("UPDATE g5_giving_meta SET status = 'drawn', updated_at = ? WHERE wr_id = ?", time.Now(), row.WrID)
		h.NotifyDrawResult(row.WrID)
		res.AutoDrawn++
	}

	// N-3: 미수령 24h 경과 재추첨. status=drawn·미수령·claim_due 경과·상한 미도달만.
	// claim_due 를 미래로 밀어 같은 틱/다음 틱 이중 재추첨을 막는다(멱등).
	type redrawRow struct {
		WrID int `gorm:"column:wr_id"`
	}
	var toRedraw []redrawRow
	h.db.Table("g5_giving_draw d").
		Select("d.wr_id").
		Joins("JOIN g5_giving_meta m ON m.wr_id = d.wr_id").
		Where("m.status = 'drawn'").
		Where("d.claimed_at IS NULL").
		Where("d.claim_due IS NOT NULL AND d.claim_due < ?", time.Now()).
		Where("d.redraw_count < ?", givingMaxRedraw).
		Find(&toRedraw)
	for _, r := range toRedraw {
		if err := h.redrawForfeited(r.WrID); err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%d: redraw %v", r.WrID, err))
			continue
		}
		res.Redrawn++
	}

	return res, nil
}

// redrawForfeited 는 미수령 당첨자를 제외하고 다음 순번을 재추첨한다(자동방식·정원1명 한정).
// forfeited 를 참가자에서 빼고 같은 시드로 재선정 → 결정적·검증 가능(다음 순번이 뽑힌다).
// 대상 소진 시 주최자에게 수동 처리 알림 후 claim_due 를 비워 재시도를 멈춘다.
func (h *GivingHandler) redrawForfeited(wrID int) error { //nolint:gocyclo // 재추첨 로직 응집 — 방식별 재선정/트랜잭션 경계 위험
	var draw givingDrawRow
	if err := h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&draw).Error; err != nil {
		return err
	}
	meta, ok := h.loadGivingMeta(wrID)
	if !ok {
		return fmt.Errorf("meta 없음")
	}
	method := givingdomain.NormalizeMethod(draw.Method)
	capacity := 1
	if meta.Capacity != nil && *meta.Capacity > 0 {
		capacity = *meta.Capacity
	}
	if !givingdomain.IsAutoDraw(method) || capacity != 1 {
		return nil // v1 범위 밖 — 지명형·다수정원은 수동
	}

	// 미수령 당첨자를 forfeited 에 추가
	var forfeited []string
	_ = json.Unmarshal(draw.ForfeitedMbIDs, &forfeited)
	prevWinner := draw.WinnerMbID
	if prevWinner != "" {
		forfeited = append(forfeited, prevWinner)
	}
	forf := map[string]bool{}
	for _, f := range forfeited {
		forf[f] = true
	}

	bids, err := h.activeBids(wrID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	participants := make([]string, 0)
	for _, b := range bids {
		if forf[b.MbID] {
			continue
		}
		if _, ok := seen[b.MbID]; !ok {
			seen[b.MbID] = struct{}{}
			participants = append(participants, b.MbID)
		}
	}
	sortStrings(participants)

	secret, secretOK := givingSeedSecret()
	if (method == givingdomain.MethodRandom || method == givingdomain.MethodLadder) && !secretOK {
		return fmt.Errorf("개표 시드 미설정")
	}
	seed := givingdomain.DeriveSeed(secret, givingBoardSlug, wrID)
	var newWinner string
	switch method {
	case givingdomain.MethodLowestUnique:
		byNumber := map[int][]string{}
		for _, b := range bids {
			if forf[b.MbID] {
				continue
			}
			for _, n := range givingdomain.ParseBidNumbers(b.BidNumbers) {
				byNumber[n] = append(byNumber[n], b.MbID)
			}
		}
		if _, mb, ok := givingdomain.LowestUniqueWinner(byNumber); ok {
			newWinner = mb
		}
	case givingdomain.MethodRandom:
		if w := givingdomain.RandomWinners(seed, participants, 1); len(w) > 0 {
			newWinner = w[0]
		}
	case givingdomain.MethodLadder:
		if l := givingdomain.BuildLadder(seed, participants, 1); len(l.Winners) > 0 {
			newWinner = l.Winners[0]
		}
	}

	forfBytes, _ := json.Marshal(forfeited)
	post, _ := h.loadGivingPost(wrID)
	subject := ""
	if post != nil {
		subject = givingTrim(post.WrSubject)
	}

	if newWinner == "" {
		// 재추첨 대상 소진 — claim_due 를 비워 재시도 중단, 주최자에게 수동 처리 요청
		h.db.Exec("UPDATE g5_giving_draw SET claim_due = NULL, forfeited_mb_ids = ?, redraw_count = redraw_count + 1 WHERE wr_id = ?", string(forfBytes), wrID)
		if post != nil {
			h.notifyOnce(post.MbID, "giving_redraw_exhausted", wrID,
				fmt.Sprintf("⚠️ 나눔 「%s」 재추첨 대상이 없습니다. 직접 확인/처리해 주세요.", subject))
		}
		return nil
	}

	newDue := time.Now().Add(givingClaimWindow)
	if derr := h.db.Exec(`UPDATE g5_giving_draw
		SET winner_mb_id = ?, claimed_at = NULL, claim_due = ?, redraw_count = redraw_count + 1, forfeited_mb_ids = ?, drawn_at = ?
		WHERE wr_id = ?`, newWinner, newDue, string(forfBytes), time.Now(), wrID).Error; derr != nil {
		return derr
	}
	h.notifyOnce(newWinner, "giving_win", wrID,
		fmt.Sprintf("🎉 나눔 「%s」에 (재추첨) 당첨되셨습니다! 24시간 내 '수령 확인'을 눌러주세요.", subject))
	if prevWinner != "" {
		h.notifyOnce(prevWinner, "giving_forfeit", wrID,
			fmt.Sprintf("⏰ 나눔 「%s」 수령 미확인(24시간)으로 재추첨되었습니다.", subject))
	}
	return nil
}

// ClaimGiving — 당첨자가 24h 내 수령을 확인한다. POST /api/plugins/giving/claim/:id (당첨자 본인).
func (h *GivingHandler) ClaimGiving(c *gin.Context) {
	wrID, err := strconv.Atoi(c.Param("id"))
	if err != nil || wrID <= 0 {
		givingErr(c, http.StatusBadRequest, "잘못된 요청입니다.")
		return
	}
	me := middleware.GetUsername(c)
	if me == "" {
		givingErr(c, http.StatusUnauthorized, "로그인이 필요합니다.")
		return
	}
	var draw givingDrawRow
	if e := h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&draw).Error; e != nil {
		givingErr(c, http.StatusNotFound, "아직 개표 전이거나 나눔을 찾을 수 없습니다.")
		return
	}
	if draw.WinnerMbID != me {
		givingErr(c, http.StatusForbidden, "당첨자만 수령을 확인할 수 있습니다.")
		return
	}
	if draw.ClaimedAt != nil {
		givingOK(c, gin.H{"claimed": true, "already": true})
		return
	}
	now := time.Now()
	h.db.Exec("UPDATE g5_giving_draw SET claimed_at = ? WHERE wr_id = ? AND winner_mb_id = ? AND claimed_at IS NULL", now, wrID, me)
	givingOK(c, gin.H{"claimed": true, "claimed_at": now})
}

// NotifyDrawResult 는 개표 결과를 당첨자·주최자에게 알린다(중복 방지 내장).
// cron 스윕과 수동 개표(Draw)·강제종료(AdminAction) 모두에서 호출된다.
// 알림 실패는 개표 결과에 영향을 주면 안 되므로 전부 흡수한다.
func (h *GivingHandler) NotifyDrawResult(wrID int) {
	post, err := h.loadGivingPost(wrID)
	if err != nil || post == nil {
		return
	}
	var draw givingDrawRow
	if err := h.db.Table("g5_giving_draw").Where("wr_id = ?", wrID).Take(&draw).Error; err != nil {
		return
	}

	winners := map[string]bool{}
	if draw.WinnerMbID != "" {
		winners[draw.WinnerMbID] = true
	}
	var parsed struct {
		Winners []string `json:"winners"`
	}
	_ = json.Unmarshal(draw.ResultJSON, &parsed)
	for _, w := range parsed.Winners {
		if w != "" {
			winners[w] = true
		}
	}

	subject := givingTrim(post.WrSubject)
	for w := range winners {
		h.notifyOnce(w, "giving_win", wrID,
			fmt.Sprintf("🎉 나눔 「%s」에 당첨되셨습니다! 축하드립니다.", subject))
	}
	// 주최자에게 개표 완료(본인이 직접 개표한 경우는 이미 알고 있으므로 생략)
	if draw.DrawnBy != post.MbID {
		h.notifyOnce(post.MbID, "giving_drawn", wrID,
			fmt.Sprintf("📦 나눔 「%s」 개표가 완료되었습니다. (참여 감사드립니다)", subject))
	}

	// N-3(요청 B): 응모자 전원에게 종료·결과 알림. 당첨자🎉·주최자📦는 위에서 이미 받았으므로 제외.
	// 나눔당 응모자는 수십~수백 규모라 배치로 순회(방송 fan-out 과 달리 물질화 위험 낮음).
	var applicants []string
	h.db.Table("g5_giving_bid").
		Where("bo_table = ? AND wr_id = ? AND bid_status = 1", givingBoardSlug, wrID).
		Distinct().Pluck("mb_id", &applicants)
	for _, mb := range applicants {
		if mb == "" || winners[mb] || mb == post.MbID {
			continue
		}
		h.notifyOnce(mb, "giving_result", wrID,
			fmt.Sprintf("🎁 나눔 「%s」이 마감되었습니다. 이번엔 아쉽게 미당첨이에요. 참여해 주셔서 감사합니다!", subject))
	}
}

// notifyOnce 는 같은 (수신자, 종류, 글) 조합 알림이 없을 때만 1건 생성한다.
// 미지의 ph_from_case 는 웹 알림함에서 기본 아이콘 + rel_msg 전문으로 렌더된다
// (notification-type.ts 의 default 폴백 실측 확인, 8/7).
func (h *GivingHandler) notifyOnce(mbID, fromCase string, wrID int, msg string) bool {
	if mbID == "" {
		return false
	}
	var cnt int64
	h.db.Table("g5_na_noti").
		Where("mb_id = ? AND bo_table = 'giving' AND wr_id = ? AND ph_from_case = ?", mbID, wrID, fromCase).
		Count(&cnt)
	if cnt > 0 {
		return false
	}
	noti := &gnurepo.Notification{
		PhToCase:      "giving",
		PhFromCase:    fromCase,
		BoTable:       "giving",
		WrID:          wrID,
		MbID:          mbID,
		RelMsg:        msg,
		RelURL:        fmt.Sprintf("/giving/%d", wrID),
		PhReaded:      "N",
		PhDatetime:    time.Now(),
		ParentSubject: msg,
		WrParent:      wrID,
	}
	return h.db.Create(noti).Error == nil
}

// givingTrim 은 알림 문구용으로 제목을 40자 내로 줄인다(rune 안전).
func givingTrim(s string) string {
	r := []rune(s)
	if len(r) <= 40 {
		return s
	}
	return string(r[:40]) + "…"
}

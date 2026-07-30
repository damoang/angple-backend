package handler

import (
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// MemberLeaveHandler 는 본인 계정 탈퇴 신청/취소(숙려기간) 셀프 서비스 엔드포인트를 담당한다.
// 관리자 leave set/clear 로직(admin_member_handler.go)을 본인 세션 전용으로 미러링한다.
type MemberLeaveHandler struct {
	db *gorm.DB
}

// NewMemberLeaveHandler creates a new MemberLeaveHandler.
func NewMemberLeaveHandler(db *gorm.DB) *MemberLeaveHandler {
	return &MemberLeaveHandler{db: db}
}

type memberLeaveRequest struct {
	Reason string `json:"reason"`
}

// withdrawalInfo 는 프론트가 취소 UI 를 렌더링할 때 참조하는 숙려 상태 정보다.
type withdrawalInfo struct {
	Status        string `json:"status"` // "grace" | "confirmed"
	LeaveDate     string `json:"leave_date"`
	Deadline      string `json:"deadline"`
	DaysRemaining int    `json:"days_remaining"`
	Cancelable    bool   `json:"cancelable"`
	// Cancelable=false 인 이유를 프론트가 그대로 보여줄 수 있게 담는다.
	// 취소 가능하면 빈 문자열.
	CancelBlockedReason string `json:"cancel_blocked_reason,omitempty"`
}

// 셀프 서비스 취소 흐름에서 반환하는 도메인 에러.
var (
	errNotWithdrawing   = errors.New("탈퇴 신청 상태가 아닙니다")
	errGraceElapsed     = errors.New("숙려기간(30일)이 경과하여 탈퇴를 취소할 수 없습니다")
	errAlreadyAnonymize = errors.New("이미 익명화되어 복구할 수 없습니다")
	errCancelDisabled   = errors.New("탈퇴 취소 기능이 현재 제공되지 않습니다. 문의가 필요하시면 고객센터로 연락 주세요")
	// 이용제한 중 탈퇴한 계정은 숙려기간이 남아 있어도 복원하지 않는다(2026-07-29 운영 결정).
	// 탈퇴 자체는 막지 않는다 — 나가는 것은 자유이고, 돌아오는 것만 제한한다.
	errDisciplinedLeave = errors.New("이용제한 중 탈퇴한 계정은 복원할 수 없습니다")
)

// leaveHistoryTable 은 탈퇴·복원 이력 테이블이다.
// ⛔ 운영은 AutoMigrate 가 돌지 않는다. DDL 이 선행되어야 한다
//
//	(triage/ddl_member_leave_history_step1.sql).
const leaveHistoryTable = "g5_da_member_leave_history"

// leaveHistoryRow 는 이력 한 행. 이벤트 시점의 g5_member 스냅샷을 박제한다.
type leaveHistoryRow struct {
	MbID           string    `gorm:"column:mb_id"`
	Event          string    `gorm:"column:event"`
	EventAt        time.Time `gorm:"column:event_at"`
	LeaveDate      string    `gorm:"column:leave_date"`
	InterceptDate  string    `gorm:"column:intercept_date"`
	WasDisciplined int       `gorm:"column:was_disciplined"`
	MbLevel        int       `gorm:"column:mb_level"`
	Reason         string    `gorm:"column:reason"`
	Source         string    `gorm:"column:source"`
}

// recordLeaveHistory 는 이력을 남긴다. 실패해도 본 흐름을 막지 않는다(best-effort).
//
// ⛔ 단, `leave` 이벤트 기록 실패는 나중에 "제한 중 탈퇴였다"는 근거를 잃는 것이므로
//
//	호출부에서 로그를 남겨 추적할 수 있게 error 를 그대로 돌려준다.
func recordLeaveHistory(db *gorm.DB, row leaveHistoryRow) error {
	// UNIQUE(mb_id, event, event_at) 로 중복이 막히므로 재시도해도 안전하다.
	//
	// ⛔ 여기에 `INSERT IGNORE` 를 직접 쓰지 말 것 — MySQL 전용 문법이다.
	//    테스트는 SQLite(gorm.io/driver/sqlite)로 돌아서 `INSERT IGNORE` 가 파싱되지 않는다
	//    (SQLite 는 `INSERT OR IGNORE`). GORM 의 OnConflict 절에 맡기면
	//    MySQL 에서는 INSERT IGNORE, SQLite 에서는 ON CONFLICT DO NOTHING 으로 각각 생성된다.
	return db.Table(leaveHistoryTable).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&row).Error
}

// disciplinedAtLastLeave 는 "가장 최근 탈퇴 신청 시점에 이용제한 중이었는가"를 판정한다.
//
// ⛔ 왜 g5_member.mb_intercept_date 를 실시간으로 보면 안 되는가:
//
//	internal/cron/discipline_release.go:79 가 제한 만료 시
//	`UPDATE g5_member SET mb_intercept_date = ''` 를 실행한다(매시간).
//	따라서 철회 시점에 원본을 보면, 숙려 30일 사이에 제한이 만료된 회원은
//	"제한 없었음"으로 판정되어 **기다렸다가 복원**하는 우회로가 열린다.
//	기준은 언제나 **탈퇴를 신청한 그 시점**이고, 그 값은 이력 테이블에만 박제되어 있다.
//
// 이력이 없으면 false(허용) 를 돌려준다 — 2026-07-25 이전 탈퇴자는 소스가 없다.
// 없는 죄를 만들지 않는 방향이며, 백필로 숙려중 대상은 이미 채워두었다.
func disciplinedAtLastLeave(db *gorm.DB, mbID string) (bool, error) {
	var flag int
	err := db.Raw(`
		SELECT was_disciplined FROM `+leaveHistoryTable+`
		 WHERE mb_id = ? AND event = 'leave'
		 ORDER BY event_at DESC LIMIT 1`, mbID).Row().Scan(&flag)
	if err != nil {
		// 행이 없으면 sql.ErrNoRows — 제한 이력 없음으로 본다.
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return flag == 1, nil
}

// Leave handles POST /api/v1/members/me/leave
// 본인 계정의 mb_leave_date=오늘, mb_leave_reason 저장(탈퇴 신청 = 숙려 시작).
// mb_intercept_date 는 절대 건드리지 않는다(제재 세탁 방지). 이미 숙려중이면 신청일을 재설정하지 않는다.
func (h *MemberLeaveHandler) Leave(c *gin.Context) {
	mbID := resolveMbID(h.db, middleware.GetUsername(c), middleware.GetUserID(c))
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "인증이 필요합니다"})
		return
	}

	var req memberLeaveRequest
	_ = c.ShouldBindJSON(&req) // reason 은 선택값

	// 감사용 사전 스냅샷 — 신청 시점의 제재 상태를 남겨야 한다.
	// 이용제한 중 탈퇴는 영구정지 사유이므로, 나중에 제재가 만료·해제돼도
	// "신청 당시 제재 중이었다"는 사실이 보존되어야 판정할 수 있다.
	var before gnuboard.G5Member
	_ = h.db.Table("g5_member").Where("mb_id = ?", mbID).Take(&before).Error

	info, err := applySelfLeave(h.db, mbID, req.Reason, time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "회원을 찾을 수 없습니다"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "탈퇴 신청 처리 실패"})
		return
	}

	wasDisciplined := strings.TrimSpace(before.MbInterceptDate) != ""

	// 이력 테이블에 박제. audit_logs 와 별도로 남기는 이유는, audit 은 로그 성격이라
	// 정리·아카이빙 대상이 되기 쉽고 실패가 조용히 넘어가기 때문이다.
	// 제재 판정의 근거는 로그가 아니라 데이터로 들고 있어야 한다.
	if err := recordLeaveHistory(h.db, leaveHistoryRow{
		MbID:           mbID,
		Event:          "leave",
		EventAt:        time.Now(),
		LeaveDate:      info.LeaveDate,
		InterceptDate:  strings.TrimSpace(before.MbInterceptDate),
		WasDisciplined: boolToInt(wasDisciplined),
		MbLevel:        before.MbLevel,
		Reason:         truncate(strings.TrimSpace(req.Reason), 255),
		Source:         "app",
	}); err != nil {
		// 탈퇴 자체는 이미 성사됐으므로 막지 않는다. 다만 이 실패는
		// "제한 중 탈퇴였다"는 근거를 잃는 것이라 반드시 남긴다.
		log.Printf("[leave-history] record leave failed mb_id=%s disciplined=%v: %v",
			mbID, wasDisciplined, err)
	}

	common.WriteAudit(h.db, c, common.AuditEntry{
		UserID:     mbID,
		Action:     common.AuditLeaveRequest,
		Resource:   "member",
		ResourceID: mbID,
		Details: map[string]any{
			"leave_date":      info.LeaveDate,
			"leave_reason":    strings.TrimSpace(req.Reason),
			"intercept_date":  before.MbInterceptDate,
			"was_disciplined": wasDisciplined,
			"mb_level":        before.MbLevel,
		},
	})
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{
		"message":    "탈퇴가 신청되었습니다. 숙려기간(30일) 내 재로그인하여 취소할 수 있습니다.",
		"withdrawal": info,
	}})
}

// CancelLeave handles DELETE /api/v1/members/me/leave
// 본인 계정의 mb_leave_date=”, mb_leave_reason=” 복구. 단 숙려기간(30일) 이내 + 아직 익명화 미확정일 때만.
// mb_intercept_date 는 절대 건드리지 않는다(제재 유지).
func (h *MemberLeaveHandler) CancelLeave(c *gin.Context) {
	mbID := resolveMbID(h.db, middleware.GetUsername(c), middleware.GetUserID(c))
	if mbID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "인증이 필요합니다"})
		return
	}

	// 취소 전 스냅샷 — 취소되면 mb_leave_date 가 지워져 "언제 신청했었는지"를 잃는다.
	var before gnuboard.G5Member
	_ = h.db.Table("g5_member").Where("mb_id = ?", mbID).Take(&before).Error

	if err := cancelSelfLeave(h.db, mbID, time.Now()); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "회원을 찾을 수 없습니다"})
		case errors.Is(err, errNotWithdrawing):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": errNotWithdrawing.Error()})
		case errors.Is(err, errGraceElapsed), errors.Is(err, errAlreadyAnonymize),
			errors.Is(err, errCancelDisabled), errors.Is(err, errDisciplinedLeave):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "탈퇴 취소 처리 실패"})
		}
		return
	}
	common.WriteAudit(h.db, c, common.AuditEntry{
		UserID:     mbID,
		Action:     common.AuditLeaveCancel,
		Resource:   "member",
		ResourceID: mbID,
		Details: map[string]any{
			"leave_date":      before.MbLeaveDate,
			"leave_reason":    before.MbLeaveReason,
			"intercept_date":  before.MbInterceptDate,
			"was_disciplined": strings.TrimSpace(before.MbInterceptDate) != "",
			"mb_level":        before.MbLevel,
		},
	})

	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "탈퇴가 취소되어 계정이 복구되었습니다."}})
}

// resolveMbID 는 요청 주체를 g5_member.mb_id(username)로 정규화한다.
//
// 숙려중 사용자는 세션 쿠키 없이 grace access_token(Bearer)만 보유하며, 이 경우
// JWT subject(userID)는 v2_users.id(숫자)다. 따라서 GetUserID 만으로 g5_member 를 조회하면
// mb_id 미매치로 404 가 난다. username 클레임을 우선 사용하고, 없거나 숫자면 v2_users 로
// username 을 역해석한다.
func resolveMbID(db *gorm.DB, usernameClaim, userID string) string {
	if u := strings.TrimSpace(usernameClaim); u != "" {
		return u
	}
	id := strings.TrimSpace(userID)
	if id == "" {
		return ""
	}
	// 숫자 subject → v2_users.id 로 username 역해석
	if _, err := strconv.ParseUint(id, 10, 64); err == nil && db != nil {
		var username string
		if scanErr := db.Table("v2_users").Select("username").Where("id = ?", id).Row().Scan(&username); scanErr == nil && username != "" {
			return username
		}
	}
	// 비숫자 subject(내부 경로 등)는 이미 mb_id 로 간주
	return id
}

// applySelfLeave 는 본인(mbID) 계정을 숙려 상태로 전환한다. 이미 숙려중이면 신청일을 유지한다.
// 오직 mb_leave_date, mb_leave_reason 만 갱신한다.
func applySelfLeave(db *gorm.DB, mbID, reason string, now time.Time) (withdrawalInfo, error) {
	var member gnuboard.G5Member
	if err := db.Table("g5_member").Where("mb_id = ?", mbID).Take(&member).Error; err != nil {
		return withdrawalInfo{}, err
	}

	// 이미 숙려중이면 신청일(clock)을 재설정하지 않는다 — 숙려기간 연장 악용 방지.
	if state, _ := common.ClassifyWithdrawal(member.MbLeaveDate, now); state == common.WithdrawalNone {
		reason = strings.TrimSpace(reason)
		if reason == "" {
			reason = "self"
		}
		updates := map[string]any{
			"mb_leave_date":   now.Format("20060102"),
			"mb_leave_reason": reason,
		}
		if err := db.Table("g5_member").Where("mb_id = ?", mbID).Updates(updates).Error; err != nil {
			return withdrawalInfo{}, err
		}
		member.MbLeaveDate = updates["mb_leave_date"].(string)
	}

	info := buildWithdrawalInfo(member.MbLeaveDate, member.MbNick, now)
	// 이용제한 중 탈퇴면 복원 대상이 아니다 — 화면이 버튼을 감출 수 있게 알려준다.
	// (실제 차단은 cancelSelfLeave 가 fail-closed 로 처리한다)
	if info.Cancelable {
		if blocked, err := disciplinedAtLastLeave(db, mbID); err == nil && blocked {
			info.Cancelable = false
			info.CancelBlockedReason = errDisciplinedLeave.Error()
		}
	}
	return info, nil
}

// isLeaveCancelEnabled 는 탈퇴 취소(복원) 기능의 on/off 를 읽는다.
//
// 우선순위: g5_kv_store('system:leave_cancel_enabled') → LEAVE_CANCEL_ENABLED env → 기본 on.
// 값이 "0"/"false"/"off" 면 비활성. 배포 없이 DB 값만 바꿔 즉시 토글할 수 있다
// (report_lock_threshold 와 동일한 관례).
//
// 기본값을 on 으로 두는 이유: 이미 사용자에게 "숙려기간 내 취소 가능"을 안내하고 있어,
// 설정 누락으로 조용히 꺼지면 약속을 어기게 된다. 끄려면 명시적으로 꺼야 한다.
func isLeaveCancelEnabled(db *gorm.DB) bool {
	off := func(s string) bool {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "0", "false", "off", "no":
			return true
		}
		return false
	}

	var rows []struct {
		ValueType string `gorm:"column:value_type"`
		ValueText string `gorm:"column:value_text"`
		ValueInt  int    `gorm:"column:value_int"`
	}
	if err := db.Raw("SELECT value_type, value_text, value_int FROM g5_kv_store WHERE `key` = 'system:leave_cancel_enabled' LIMIT 1").
		Scan(&rows).Error; err == nil && len(rows) > 0 {
		r := rows[0]
		// 행이 존재할 때만 판정한다(미설정이면 기본 on 유지).
		if r.ValueType == "INT" {
			return r.ValueInt != 0
		}
		if r.ValueText != "" {
			return !off(r.ValueText)
		}
	}

	if env := os.Getenv("LEAVE_CANCEL_ENABLED"); env != "" && off(env) {
		return false
	}
	return true
}

// cancelSelfLeave 는 숙려중(그리고 미확정)인 본인 계정의 탈퇴를 취소한다.
// mb_intercept_date 는 결코 갱신 대상에 포함하지 않는다(제재 유지).
func cancelSelfLeave(db *gorm.DB, mbID string, now time.Time) error {
	if !isLeaveCancelEnabled(db) {
		return errCancelDisabled
	}
	var member gnuboard.G5Member
	if err := db.Table("g5_member").Where("mb_id = ?", mbID).Take(&member).Error; err != nil {
		return err
	}

	state, _ := common.ClassifyWithdrawal(member.MbLeaveDate, now)
	switch state {
	case common.WithdrawalNone:
		return errNotWithdrawing
	case common.WithdrawalConfirmed:
		return errGraceElapsed
	}
	// 숙려중이라도 이미 익명화 확정된 계정은 복구 불가.
	if common.IsWithdrawalAnonymized(member.MbNick) {
		return errAlreadyAnonymize
	}

	// 이용제한 중 탈퇴한 계정은 숙려기간이 남아 있어도 복원하지 않는다(2026-07-29 운영 결정).
	// ⛔ 판정 근거는 이력 테이블이다. g5_member.mb_intercept_date 를 보면 안 된다 —
	//    discipline_release 크론이 만료 시 지우므로 "기다렸다가 복원"이 가능해진다.
	disciplined, err := disciplinedAtLastLeave(db, mbID)
	if err != nil {
		// ⛔ 조회 실패 시 통과시키지 않는다. 제재 판정은 fail-closed 여야 한다.
		//    이력 테이블이 없거나(DDL 미적용) DB 가 흔들리는 상황에서
		//    막아야 할 복원을 통과시키는 쪽이 훨씬 나쁘다.
		return err
	}
	if disciplined {
		return errDisciplinedLeave
	}

	// mb_leave_date / mb_leave_reason 만 초기화. mb_intercept_date 는 절대 포함하지 않는다.
	if err := db.Table("g5_member").Where("mb_id = ?", mbID).
		Updates(map[string]any{"mb_leave_date": "", "mb_leave_reason": ""}).Error; err != nil {
		return err
	}

	// 복원 이력 기록 — 반복 탈퇴·복원(bug/13133 ②케이스)을 추적하려면 이 행이 필요하다.
	if err := recordLeaveHistory(db, leaveHistoryRow{
		MbID:           mbID,
		Event:          "cancel",
		EventAt:        now,
		LeaveDate:      member.MbLeaveDate,
		InterceptDate:  strings.TrimSpace(member.MbInterceptDate),
		WasDisciplined: 0, // 여기까지 왔다는 것은 제한 중 탈퇴가 아니었다는 뜻이다
		MbLevel:        member.MbLevel,
		Source:         "app",
	}); err != nil {
		// 복원 자체는 이미 성사됐다. 이력만 실패한 것이므로 막지 않고 남긴다.
		log.Printf("[leave-history] record cancel failed mb_id=%s: %v", mbID, err)
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// truncate 는 룬 기준으로 자른다.
// ⛔ 바이트로 자르면 한글이 중간에서 끊겨 깨진 문자가 저장된다.
//
//	DB 컬럼도 VARCHAR(255) = 255 '문자' 라 룬 기준이 맞다.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func buildWithdrawalInfo(leaveDate, mbNick string, now time.Time) withdrawalInfo {
	state, deadline := common.ClassifyWithdrawal(leaveDate, now)
	info := withdrawalInfo{LeaveDate: leaveDate}
	switch state {
	case common.WithdrawalGrace:
		info.Status = "grace"
		info.Deadline = deadline.Format("2006-01-02")
		days := int(deadline.Sub(now).Hours() / 24)
		if days < 0 {
			days = 0
		}
		info.DaysRemaining = days
		info.Cancelable = !common.IsWithdrawalAnonymized(mbNick)
	case common.WithdrawalConfirmed:
		info.Status = "confirmed"
		info.Deadline = deadline.Format("2006-01-02")
		info.Cancelable = false
	default:
		info.Status = "none"
	}
	return info
}

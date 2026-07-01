package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
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
}

// 셀프 서비스 취소 흐름에서 반환하는 도메인 에러.
var (
	errNotWithdrawing   = errors.New("탈퇴 신청 상태가 아닙니다")
	errGraceElapsed     = errors.New("숙려기간(30일)이 경과하여 탈퇴를 취소할 수 없습니다")
	errAlreadyAnonymize = errors.New("이미 익명화되어 복구할 수 없습니다")
)

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

	info, err := applySelfLeave(h.db, mbID, req.Reason, time.Now())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "회원을 찾을 수 없습니다"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "탈퇴 신청 처리 실패"})
		return
	}
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

	if err := cancelSelfLeave(h.db, mbID, time.Now()); err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "회원을 찾을 수 없습니다"})
		case errors.Is(err, errNotWithdrawing):
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": errNotWithdrawing.Error()})
		case errors.Is(err, errGraceElapsed), errors.Is(err, errAlreadyAnonymize):
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "탈퇴 취소 처리 실패"})
		}
		return
	}
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

	return buildWithdrawalInfo(member.MbLeaveDate, member.MbNick, now), nil
}

// cancelSelfLeave 는 숙려중(그리고 미확정)인 본인 계정의 탈퇴를 취소한다.
// mb_intercept_date 는 결코 갱신 대상에 포함하지 않는다(제재 유지).
func cancelSelfLeave(db *gorm.DB, mbID string, now time.Time) error {
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

	// mb_leave_date / mb_leave_reason 만 초기화. mb_intercept_date 는 절대 포함하지 않는다.
	return db.Table("g5_member").Where("mb_id = ?", mbID).
		Updates(map[string]any{"mb_leave_date": "", "mb_leave_reason": ""}).Error
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

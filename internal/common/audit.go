package common

import (
	"encoding/json"
	"log"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// 감사 로그 action 상수 — 계정 상태 변경 이벤트.
//
// ⛔ 회원 감시·행동 프로파일링 용도가 아니다. 계정 상태가 바뀐 사실만 남긴다
// (분쟁 증명, 제재 중 탈퇴 판정, 관리자 조치 추적).
const (
	AuditLeaveRequest    = "member.leave.request"     // 본인 탈퇴 신청(숙려 진입)
	AuditLeaveCancel     = "member.leave.cancel"      // 본인 탈퇴 취소(숙려 중)
	AuditLeaveAdminSet   = "member.leave.admin_set"   // 관리자 강제 탈퇴
	AuditLeaveAdminClear = "member.leave.admin_clear" // 관리자 탈퇴 해제

	// 수동 실명인증 — 해외 앙님처럼 국내 휴대폰 인증이 불가능한 경우 관리자가 직접 처리한다.
	// ⛔ 수동 인증은 DI(mb_dupinfo)를 만들지 않는다. 즉 명의 중복 검사를 거치지 않은 채
	//    인증 회원 권한(인증필수 게시판·쪽지·자동승급)이 부여된다. 그래서 사유를 반드시 남긴다.
	AuditCertifyManualSet   = "member.certify.manual_set"   // 관리자 수동 인증
	AuditCertifyManualClear = "member.certify.manual_clear" // 관리자 수동 인증 해제
)

// AuditEntry 는 audit_logs 한 행을 표현한다.
type AuditEntry struct {
	UserID     string         // 행위자 mb_id (본인 건이면 대상과 동일)
	Action     string         // 위 상수
	Resource   string         // 예: "member"
	ResourceID string         // 대상 mb_id
	Details    map[string]any // JSON 으로 직렬화되어 저장
}

// WriteAudit 는 감사 로그 1건을 best-effort 로 기록한다.
//
// ⛔ 실패해도 호출자의 본 로직을 막지 않는다. 감사 기록이 안 된다고 탈퇴/취소 자체가
// 실패하면 회원이 계정을 정리하지 못하게 되므로, 오류는 로그만 남기고 삼킨다.
// 같은 이유로 상태 변경 트랜잭션 안에서 호출하지 말 것(감사 실패가 롤백을 유발하면 안 됨).
func WriteAudit(db *gorm.DB, c *gin.Context, e AuditEntry) {
	if db == nil || e.Action == "" {
		return
	}

	var detailsJSON string
	if e.Details != nil {
		if b, err := json.Marshal(e.Details); err == nil {
			detailsJSON = string(b)
		}
	}

	var clientIP, userAgent, requestID string
	if c != nil {
		clientIP = c.ClientIP()
		userAgent = c.Request.UserAgent()
		requestID = c.GetHeader("X-Request-Id")
	}

	// created_at 은 앱 커넥션이 Asia/Seoul 이므로 NOW(3) 가 KST 로 저장된다.
	if err := db.Exec(`
		INSERT INTO audit_logs
		(created_at, user_id, action, resource, resource_id, details, client_ip, user_agent, request_id)
		VALUES (NOW(3), ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Action, e.Resource, e.ResourceID, detailsJSON, clientIP, userAgent, requestID,
	).Error; err != nil {
		log.Printf("[Audit] write failed action=%s target=%s: %v", e.Action, e.ResourceID, err)
	}
}

package handler

import (
	"log"

	"gorm.io/gorm"
)

// purgeAuthArtifacts 는 회원의 인증 산출물(세션·리프레시 토큰)을 파기한다.
//
// 왜 — 분쟁조정위 26R05-00197 로 드러난 문제:
//
//	탈퇴 처리가 기존 세션을 무효화하지 않아, 탈퇴 전에 발급된 세션 쿠키로
//	인증 상태가 유지됐다. 신청인 계정에서 탈퇴 17일 뒤 포인트·XP·mb_today_login
//	이 갱신된 것이 그 결과다(신규 로그인이 아니라 잔존 세션에 의한 접근).
//
// ⛔ revoke 가 아니라 DELETE 다.
//
//	angple_sessions·angple_refresh_tokens 는 IP·User-Agent 를 보유해
//	사실상 접속기록이다. 행을 남기면 개인정보가 그대로 보존된다.
//
// ⛔ 실패해도 에러를 반환하지 않는다.
//
//	회원 UPDATE 가 끝난 뒤 호출되며, 파기 실패로 탈퇴가 되돌아가면 안 된다.
//	회원은 이미 탈퇴를 신청했고, 실패하면 더 곤란해진다. 로그만 남긴다.
//
// ⛔ 트랜잭션에 묶어 호출하지 말 것. 위 원칙이 깨진다.
//
// ⛔ 탈퇴 **취소** 경로에서는 부르지 않는다 — 복귀시키는 동작이다.
//
// 호출처: applySelfLeave(자가 탈퇴) · AdminMemberHandler.UpdateMember(관리자 탈퇴).
// 새 탈퇴 진입점을 만들면 여기도 함께 배선할 것.
func purgeAuthArtifacts(db *gorm.DB, mbID string) {
	if mbID == "" {
		return
	}
	if err := db.Exec("DELETE FROM angple_sessions WHERE mb_id = ?", mbID).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 세션 파기 실패 (%s): %v", mbID, err)
	}
	if err := db.Exec("DELETE FROM angple_refresh_tokens WHERE mb_id = ?", mbID).Error; err != nil {
		log.Printf("[purgeAuthArtifacts] 리프레시 토큰 파기 실패 (%s): %v", mbID, err)
	}
}

package handler

import (
	"context"
	"log"

	"github.com/damoang/angple-backend/internal/middleware"
)

// 회원 활동(GET /api/v1/members/:id/activity)의 탈퇴 회원 가드.
//
// 탈퇴 회원 정보 비노출 정책을 회원 활동 API 에도 적용한다. 탈퇴(숙려 포함) 회원의
// 활동은 「활동 없음」과 같은 응답(빈 목록)으로 돌려주고, 탈퇴 사실은 따로 드러내지 않는다.
// 숙려 중 탈퇴를 취소하면 다시 보인다.
//
// 판정은 탈퇴 게이트와 같은 캐시를 거친다(캐시 우선, 미스 시 DB 1회).
// 판정 실패(DB·캐시 오류) 시에는 빈 응답(비노출 우선)이다.

// memberActivityWithdrawn 은 판정 함수다 — 테스트에서 바꿔 끼울 수 있도록 변수로 둔다.
var memberActivityWithdrawn = middleware.IsWithdrawnMember

// hideMemberActivity 는 mbID 회원의 활동을 응답에서 숨겨야 하면 true 를 돌려준다.
// 탈퇴(숙려 포함) 회원이거나 판정에 실패하면 true 다.
func hideMemberActivity(ctx context.Context, mbID string) bool {
	withdrawn, err := memberActivityWithdrawn(ctx, mbID)
	if err != nil {
		log.Printf("[memberActivityGuard] 탈퇴 판정 실패 — 활동 비노출 (%s): %v", mbID, err)
		return true
	}
	return withdrawn
}

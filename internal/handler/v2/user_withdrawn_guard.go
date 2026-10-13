package v2

import (
	"context"
	"log"

	"github.com/damoang/angple-backend/internal/middleware"
)

// 회원 프로필 조회(GET /api/v2/users/:id, /api/v2/users/username/:username)의
// 탈퇴 회원 가드.
//
// 탈퇴 회원 정보 비노출 정책을 회원 프로필 조회에도 적용한다. 탈퇴(숙려 포함) 회원은
// 「사용자를 찾을 수 없습니다」(없는 회원과 같은 404)로 답하고, 탈퇴 사실은 따로
// 드러내지 않는다. 숙려 중 탈퇴를 취소하면 다시 보인다.
//
// 판정 키는 v2_users.username(= mb_id)이다. 판정은 탈퇴 게이트와 같은 캐시를
// 거친다(캐시 우선, 미스 시 DB 1회). 판정 실패 시에도 없는 회원과 같은 응답이다
// (비노출 우선).

// v2UserWithdrawn 은 판정 함수다 — 테스트에서 바꿔 끼울 수 있도록 변수로 둔다.
var v2UserWithdrawn = middleware.IsWithdrawnMember

// hideV2UserProfile 은 mbID 회원의 프로필을 숨겨야 하면 true 를 돌려준다.
// 탈퇴(숙려 포함) 회원이거나 판정에 실패하면 true 다.
func hideV2UserProfile(ctx context.Context, mbID string) bool {
	withdrawn, err := v2UserWithdrawn(ctx, mbID)
	if err != nil {
		log.Printf("[v2UserGuard] 탈퇴 판정 실패 — 프로필 비노출 (%s): %v", mbID, err)
		return true
	}
	return withdrawn
}

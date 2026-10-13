package handler

import (
	"context"
	"log"
	"net/http"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/gin-gonic/gin"
)

// 회원 메모(g5_member_memo)의 탈퇴 대상 가드.
//
// 탈퇴 회원 정보 비노출 정책을 회원 메모에도 적용한다. 탈퇴(숙려 포함) 회원을
// 대상으로 한 메모는 조회하면 「메모 없음」과 같은 응답을 받고, 새로 저장하거나
// 수정할 수 없다. 본인이 남긴 메모를 지우는 것은 막지 않는다.
//
// 판정 실패(DB·캐시 오류) 시:
//   - 조회 계열은 빈 결과(비노출 우선) — 메모가 잠깐 안 보여도 서비스는 깨지지 않는다.
//   - 저장 계열은 거부.

// MemoWithdrawnTargetMessage 는 탈퇴 회원 대상 메모 저장·수정 거부 문구다.
const MemoWithdrawnTargetMessage = "탈퇴한 회원에게는 메모를 남길 수 없습니다."

// memoGuardLookupFailedMessage 는 판정 실패로 저장을 거부할 때의 문구다.
// 대상이 탈퇴했는지는 알려 주지 않는다.
const memoGuardLookupFailedMessage = "메모를 저장하지 못했습니다. 잠시 후 다시 시도해 주세요."

// 판정 함수 — 테스트에서 바꿔 끼울 수 있도록 변수로 둔다.
var (
	memoTargetWithdrawn    = middleware.IsWithdrawnMember
	memoWithdrawnTargetIDs = middleware.WithdrawnMemberIDs
)

// HideMemoForTarget 는 targetID 대상 메모를 조회 응답에서 숨겨야 하면 true 를 돌려준다.
// 탈퇴(숙려 포함) 대상이거나 판정에 실패하면 true 다.
func HideMemoForTarget(ctx context.Context, targetID string) bool {
	withdrawn, err := memoTargetWithdrawn(ctx, targetID)
	if err != nil {
		log.Printf("[memoGuard] 탈퇴 판정 실패 — 조회 비노출 (%s): %v", targetID, err)
		return true
	}
	return withdrawn
}

// RejectMemoWriteForTarget 는 targetID 대상 메모 저장·수정을 막아야 하면
// 응답을 쓰고 true 를 돌려준다. 호출자는 true 면 바로 return 한다.
//
//   - 탈퇴(숙려 포함) 대상: 403 + MemoWithdrawnTargetMessage
//   - 판정 실패: 503 (저장 거부, 탈퇴 여부는 드러내지 않음)
func RejectMemoWriteForTarget(c *gin.Context, targetID string) bool {
	withdrawn, err := memoTargetWithdrawn(c.Request.Context(), targetID)
	if err != nil {
		log.Printf("[memoGuard] 탈퇴 판정 실패 — 저장 거부 (%s): %v", targetID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": memoGuardLookupFailedMessage})
		return true
	}
	if withdrawn {
		c.JSON(http.StatusForbidden, gin.H{"success": false, "error": MemoWithdrawnTargetMessage})
		return true
	}
	return false
}

// DropWithdrawnMemoTargets 는 대상 ID → 메모 map 에서 탈퇴(숙려 포함) 대상을 뺀다.
// 판정은 IN 쿼리 1회이며, 메모가 없으면(빈 map) 조회하지 않는다.
// 판정에 실패하면 이 요청만 빈 map 을 돌려준다(비노출 우선).
func DropWithdrawnMemoTargets(ctx context.Context, memos map[string]interface{}) map[string]interface{} {
	if len(memos) == 0 {
		return memos
	}
	ids := make([]string, 0, len(memos))
	for id := range memos {
		ids = append(ids, id)
	}
	withdrawn, err := memoWithdrawnTargetIDs(ctx, ids)
	if err != nil {
		log.Printf("[memoGuard] 탈퇴 판정 실패 — 배치 비노출 (%d건): %v", len(ids), err)
		return map[string]interface{}{}
	}
	for id := range withdrawn {
		delete(memos, id)
	}
	return memos
}

// DropWithdrawnFromMemoTargets 는 memo-targets 목록에서 탈퇴(숙려 포함) 대상을 뺀다.
// 남은 대상의 순서는 그대로이고 Count 는 남은 개수로 맞춘다. Truncated 는 원래 값을
// 유지한다(목록이 잘렸으면 클라이언트는 어차피 batch 로 폴백하고, batch 도 같은 가드를 탄다).
//
// 판정은 IN 쿼리 1회. 판정에 실패하면 error 를 돌려준다 — 호출자는 목록 조회 실패와
// 똑같이 500 으로 응답한다. 클라이언트는 이를 「모름」으로 보고 같은 가드를 타는
// batch 조회로 폴백하므로 탈퇴 대상이 드러나지 않는다. 빈 목록으로 답하면
// 「메모 없음」으로 확정돼 세션 동안 일반 회원 메모까지 사라진다.
func DropWithdrawnFromMemoTargets(ctx context.Context, data MemoTargets) (MemoTargets, error) {
	if len(data.Targets) == 0 {
		return data, nil
	}
	withdrawn, err := memoWithdrawnTargetIDs(ctx, data.Targets)
	if err != nil {
		log.Printf("[memoGuard] 탈퇴 판정 실패 — 메모 대상 목록 실패 처리 (%d건): %v", len(data.Targets), err)
		return MemoTargets{Targets: []string{}}, err
	}
	if len(withdrawn) == 0 {
		return data, nil
	}
	kept := make([]string, 0, len(data.Targets))
	for _, id := range data.Targets {
		if !withdrawn[id] {
			kept = append(kept, id)
		}
	}
	data.Targets = kept
	data.Count = len(kept)
	return data, nil
}

package gnuboard

import "time"

// 아래 둘은 **다른 패키지의 테스트**가 근거글 캐시 상태를 만들기 위한 헬퍼다.
// (cmd/api 의 enrichWithDisciplineRelated 테스트가 쓴다)
//
// ⛔ 운영 코드에서 부르지 마라. 캐시는 DisciplinedIDs 가 관리한다.
//    이름에 ForTest 를 붙인 이유가 그것이다.

// ResetDisciplinedIDsCacheForTest 는 근거글 캐시를 비운다.
func ResetDisciplinedIDsCacheForTest() {
	disciplinedIDsCache.Lock()
	disciplinedIDsCache.byBoard = nil
	disciplinedIDsCache.expires = nil
	disciplinedIDsCache.Unlock()
}

// SeedDisciplinedIDsCacheForTest 는 지정한 만료 시각으로 캐시를 심는다.
// 만료 시각을 과거로 주면 "만료됐지만 남아 있는" 상태(폴백 대상)를 만들 수 있다.
func SeedDisciplinedIDsCacheForTest(boardID string, set map[int]bool, expiresAt time.Time) {
	disciplinedIDsCache.Lock()
	if disciplinedIDsCache.byBoard == nil {
		disciplinedIDsCache.byBoard = make(map[string]map[int]bool)
		disciplinedIDsCache.expires = make(map[string]time.Time)
	}
	disciplinedIDsCache.byBoard[boardID] = set
	disciplinedIDsCache.expires[boardID] = expiresAt
	disciplinedIDsCache.Unlock()
}

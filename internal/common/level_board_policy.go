package common

// 등급별 게시판 제한 정책.
//
// # 왜 별도 규칙이 필요한가
//
// 게시판 권한은 `mb_level >= bo_write_level` 형태의 **사다리**다. 사다리는
// "이 등급 이상"만 표현할 수 있고 "이 게시판에서만"은 표현할 수 없다.
//
// 광고앙(등급 5)은 유료 광고 계정이라 직접홍보(promotion)에만 글을 남겨야 하는데,
// 사다리 모델에서는 등급 5가 가입인사(write_level=1)·자유게시판(3)을 전부 통과한다.
// promotion 의 write_level 이 5 라서 "5 = promotion 전용"처럼 보이지만,
// 그건 **하한**이지 전용이 아니다.
//
// # 실명인증은 이 역할을 대신할 수 없다 (2026-08-04 실측)
//
// 원래 안전장치는 실명인증이었다 — 인증 면제 게시판이 promotion·verification·overseas
// 뿐이므로, 광고앙이 미인증이면 자연히 promotion 에만 쓸 수 있다.
// 그런데 광고앙 31명 중 **14명이 실명인증을 마쳤고**, 인증한 광고앙은 cert 게이트를
// 정당하게 통과한다. 실명인증은 인증/미인증을 가르는 장치이지 광고앙/일반회원을
// 가르는 장치가 아니라서, 시간이 지나면 반드시 샌다.
//
// # 적용 지점
//
// ⛔ **실제 차단은 cmd/api/main.go 의 인라인 핸들러에서 일어난다.**
//
//	createPostFn    (main.go, v1 웹 + v2 앱 라우트가 공유)  — 글 차단
//	createCommentFn (main.go, 동일)                        — 댓글 차단
//
// ⛔ handler/v2 의 CreatePost·CreateComment 에도 같은 검사가 있지만 **그 핸들러는
//    어떤 라우트에도 등록돼 있지 않다**(routes/v2/routes.go 가 중복 등록 panic 때문에
//    작성 라우트를 빼두었다). 거기만 고치면 아무것도 안 막힌다 — 2026-08-04 실제로
//    거기만 고쳐 놓고 "막았다"고 판단할 뻔했다. 죽은 핸들러는 회귀 방지용으로 남겨둔다.
//
// middleware/permission_checker 는 **차단이 아니라 표시**를 담당한다.
// 권한 API 응답이 UI 의 글쓰기 버튼 노출을 정하므로 여기도 같이 맞춰야 하지만,
// ⛔ 여기만 고치면 버튼만 사라지고 API 는 계속 받는다 — 차단이 아니라 위장이다.

// AdvertiserLevel 은 광고앙(유료 광고 계정) 등급이다.
//
// ⛔ 비교는 반드시 **정확히 일치(==)** 여야 한다. `>=` 로 쓰면 관리자(10)까지 묶인다.
const AdvertiserLevel = 5

// advertiserAllowedBoards 는 광고앙이 글·댓글을 남길 수 있는 유일한 게시판이다.
//
// 늘릴 때는 정말 광고 목적 지면인지 확인할 것. 여기에 게시판을 더하는 순간
// 그 게시판은 광고 노출 지면이 된다.
var advertiserAllowedBoards = map[string]bool{
	"promotion": true,
}

// IsBoardWriteBlockedByLevel 은 해당 등급이 이 게시판에 글·댓글을 남기지 못하는지 판정한다.
//
// 광고앙이 아닌 등급은 항상 false 를 돌려주므로 기존 동작이 그대로 유지된다.
func IsBoardWriteBlockedByLevel(memberLevel int, boardSlug string) bool {
	if memberLevel != AdvertiserLevel {
		return false
	}
	return !advertiserAllowedBoards[boardSlug]
}

// LevelBoardBlockedMessage 는 차단됐을 때 회원에게 보여줄 안내 문구다.
//
// 등급 이름과 게시판 이름을 그대로 적어, 왜 막혔고 어디에 쓰면 되는지 한 줄로 알 수 있게 한다.
const LevelBoardBlockedMessage = "광고앙 등급은 직접홍보 게시판에서만 글과 댓글을 작성하실 수 있습니다."

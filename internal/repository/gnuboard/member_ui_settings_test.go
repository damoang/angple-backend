package gnuboard

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ⛔ 이 파일이 지키는 계약 (bug/13664 쪽지 수신 거부):
//
//	IsMessageDenied 는 **조회 실패를 「거부 아님」으로 뭉개지 않는다.**
//
// 반환값이 「이 사람이 쪽지를 막았는가」라, 실패가 false 로 나가면 호출부는
// 「안 막았다」로 읽는다. 그 자체는 fail-open 이라 서비스는 돌아가지만,
// 호출부가 **실패와 정상 허용을 구분할 수 없게** 된다 — 로그도 못 남긴다.
// ⭐ 판별 기준은 `!= nil` 이 아니라 **`err == nil` 인데 값이 비어 있을 수 있는가** 다.
//
// ⚠️ 게이트 자체는 fail-**open** 이 맞다(호출부 주석 참조).
//
//	이건 보안 게이트가 아니라 수신자 편의 기능이라, 설정을 못 읽었다고 쪽지를 막으면
//	아무 설정도 안 한 사람까지 못 받는다. 다만 **그 판단은 호출부가** 해야 하고,
//	저장소는 「모른다」를 정직하게 돌려줘야 한다.

func newUISettingsRepo(t *testing.T, withTable bool) MemberUISettingsRepository {
	t.Helper()
	ResetMessageDenyCacheForTest()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	if !withTable {
		// ⭐ 테이블이 없으면 조회가 반드시 실패한다. 여기서는 그게 검증 수단이다.
		return NewMemberUISettingsRepository(db)
	}
	// sqlite 에는 JSON 컬럼·`->>` 가 MySQL 과 다르므로 값 자체를 텍스트로 흉내 낸다.
	// ⛔ 이 테스트는 SQL 방언이 아니라 **계약**(실패/기본값/캐시)을 문다.
	//    `settings->>'$.messageDeny'` 가 MySQL 에서 맞는지는 운영 DB 로 따로 확인한다.
	if err := db.Exec(`CREATE TABLE g5_da_member_ui_settings (
		mb_id TEXT PRIMARY KEY, settings TEXT)`).Error; err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}
	return NewMemberUISettingsRepository(db)
}

func TestIsMessageDenied_PropagatesError(t *testing.T) {
	r := newUISettingsRepo(t, false)

	denied, err := r.IsMessageDenied("someone")

	// ⛔ 여기가 핵심이다. err == nil 이면 호출부가 "안 막았다"로 읽고 로그도 못 남긴다.
	if err == nil {
		t.Fatalf("error 를 기대했다. denied=%v — 실패와 정상 허용을 구분할 수 없게 된다", denied)
	}
	if denied {
		t.Error("실패 시에는 denied=true 를 주면 안 된다 — 조회도 못 했는데 쪽지를 막는다")
	}
}

// TestIsMessageDenied_EmptyIDIsNotAnError 는 「물어볼 것 없음」과 「실패」를 구분한다.
func TestIsMessageDenied_EmptyIDIsNotAnError(t *testing.T) {
	r := newUISettingsRepo(t, false) // 테이블이 없어도 쿼리 자체를 안 쳐야 한다

	denied, err := r.IsMessageDenied("")
	if err != nil {
		t.Fatalf("빈 id 는 error 가 아니어야 한다: %v", err)
	}
	if denied {
		t.Error("빈 id 는 거부가 아니다")
	}
}

// TestIsMessageDenied_DefaultsToAllowed 는 **설정을 한 번도 안 건드린 회원**을 지킨다.
// ⛔ 대다수가 이 경우다(행 자체가 없다). 여기서 true 가 나오면 전 회원의 쪽지가 막힌다.
func TestIsMessageDenied_DefaultsToAllowed(t *testing.T) {
	r := newUISettingsRepo(t, true)

	denied, err := r.IsMessageDenied("no_row_member")
	if err != nil {
		t.Fatalf("행이 없는 것은 error 가 아니다: %v", err)
	}
	if denied {
		t.Error("⛔ 설정이 없으면 **수신 허용**이 기본이다")
	}
}

// TestIsMessageDenied_CachesResult 는 쪽지마다 DB 를 치지 않는지 본다.
// 첫 조회 뒤 테이블을 지워도 캐시가 살아 있으면 같은 답이 나와야 한다.
func TestIsMessageDenied_CachesResult(t *testing.T) {
	ResetMessageDenyCacheForTest()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("sqlite 열기 실패: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_da_member_ui_settings (mb_id TEXT PRIMARY KEY, settings TEXT)`).Error; err != nil {
		t.Fatalf("테이블 생성 실패: %v", err)
	}
	r := NewMemberUISettingsRepository(db)

	if _, err := r.IsMessageDenied("cached_member"); err != nil {
		t.Fatalf("첫 조회 실패: %v", err)
	}
	// 테이블을 없앤다 — 캐시가 없으면 여기서 error 가 난다.
	if err := db.Exec("DROP TABLE g5_da_member_ui_settings").Error; err != nil {
		t.Fatalf("DROP 실패: %v", err)
	}
	if _, err := r.IsMessageDenied("cached_member"); err != nil {
		t.Errorf("캐시가 있어야 한다 — 쪽지마다 DB 를 치면 안 된다: %v", err)
	}
}

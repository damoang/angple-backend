package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// fakeKeywordNotiRepo 는 알림 생성·중복 확인만 흉내 낸다(나머지 메서드는 호출되면 패닉).
type fakeKeywordNotiRepo struct {
	gnurepo.NotiRepository
	created []*gnurepo.Notification
}

func (f *fakeKeywordNotiRepo) Create(n *gnurepo.Notification) error {
	f.created = append(f.created, n)
	return nil
}

func (f *fakeKeywordNotiRepo) Exists(mbID, boTable string, wrID int, fromCase, relMbID string) (bool, error) {
	for _, n := range f.created {
		if n.MbID == mbID && n.BoTable == boTable && n.WrID == wrID && n.PhFromCase == fromCase && n.RelMbID == relMbID {
			return true, nil
		}
	}
	return false, nil
}

// byMember 는 회원별로 받은 알림의 ph_to_case 목록을 돌려준다.
func (f *fakeKeywordNotiRepo) byMember() map[string][]string {
	out := make(map[string][]string)
	for _, n := range f.created {
		out[n.MbID] = append(out[n.MbID], n.PhToCase)
	}
	return out
}

// fakeKeywordPrefRepo 는 모든 알림 설정을 켜 두고, keywordOff 회원만 키워드 알림을 끈다.
type fakeKeywordPrefRepo struct {
	keywordOff map[string]bool
}

func (f *fakeKeywordPrefRepo) Get(mbID string) (*gnurepo.NotiPreference, error) {
	return &gnurepo.NotiPreference{
		MbID:               mbID,
		NotiComment:        true,
		NotiReply:          true,
		NotiMention:        true,
		NotiLike:           true,
		NotiFollow:         true,
		NotiBoardSubscribe: true,
		NotiKeyword:        !f.keywordOff[mbID],
		LikeThreshold:      1,
	}, nil
}

func (f *fakeKeywordPrefRepo) Upsert(_ *gnurepo.NotiPreference) error { return nil }

type fakeKeywordLimiter struct {
	deny  map[string]bool
	calls int
}

func (f *fakeKeywordLimiter) Allow(_ context.Context, mbID string, _ time.Time) bool {
	f.calls++
	return !f.deny[mbID]
}

func newKeywordAlertTestDB(t *testing.T, name, wrOption string) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:kwalert_%s?mode=memory&cache=shared", name)
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	stmts := []string{
		`CREATE TABLE g5_board (bo_table TEXT PRIMARY KEY, bo_list_level INTEGER, bo_read_level INTEGER)`,
		`CREATE TABLE g5_member_block (mb_id TEXT, blocked_mb_id TEXT, block_scope TEXT)`,
		`CREATE TABLE g5_member_follow (mb_id TEXT, target_id TEXT)`,
		`CREATE TABLE g5_board_subscribe (bo_table TEXT, mb_id TEXT, level INTEGER)`,
		`CREATE TABLE g5_write_free (wr_id INTEGER PRIMARY KEY, wr_option TEXT)`,
		`INSERT INTO g5_board VALUES ('free', 1, 1)`,
		`INSERT INTO g5_member_block VALUES ('blocker', 'author', 'all'), ('msgblocker', 'author', 'message')`,
		`INSERT INTO g5_member_follow VALUES ('follower', 'author')`,
		`INSERT INTO g5_board_subscribe VALUES ('free', 'subscriber', 1)`,
	}
	for _, s := range stmts {
		if err := db.Exec(s).Error; err != nil {
			t.Fatalf("setup %q: %v", s, err)
		}
	}
	if err := db.Exec(`INSERT INTO g5_write_free VALUES (100, ?)`, wrOption).Error; err != nil {
		t.Fatalf("insert post: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// 모든 테스트 회원이 제목과 일치하는 키워드를 갖는다(nomatch 만 예외).
var keywordAlertTestRows = []KeywordAlertRow{
	{MbID: "author", Keyword: "아이폰"},
	{MbID: "blocker", Keyword: "아이폰"},
	{MbID: "msgblocker", Keyword: "아이폰"},
	{MbID: "follower", Keyword: "아이폰"},
	{MbID: "subscriber", Keyword: "출시"},
	{MbID: "prefoff", Keyword: "아이폰"},
	{MbID: "normal", Keyword: "아이폰"},
	{MbID: "normal", Keyword: "아이폰 17"},
	{MbID: "nomatch", Keyword: "갤럭시"},
}

func newKeywordAlertTestWorker(db *gorm.DB, rows []KeywordAlertRow, limiter KeywordAlertLimiter) (*WriteAfterWorker, *fakeKeywordNotiRepo) {
	repo := &fakeKeywordNotiRepo{}
	w := &WriteAfterWorker{
		db:           db,
		notiRepo:     repo,
		notiPrefRepo: &fakeKeywordPrefRepo{keywordOff: map[string]bool{"prefoff": true}},
	}
	if rows != nil {
		w.SetKeywordAlert(NewKeywordAlertMatcher(&fakeKeywordLoader{rows: rows}), limiter)
	}
	return w, repo
}

func keywordAlertTestJob() PostCreatedJob {
	return PostCreatedJob{
		BoardSlug: "free",
		WriteID:   100,
		MemberID:  "author",
		Author:    "작성자",
		Subject:   "아이폰 17 출시 소식",
		CreatedAt: time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC),
	}
}

func TestHandlePostCreatedKeywordAlertExclusions(t *testing.T) {
	db := newKeywordAlertTestDB(t, "exclusions", "html1")
	limiter := &fakeKeywordLimiter{}
	w, repo := newKeywordAlertTestWorker(db, keywordAlertTestRows, limiter)

	w.handlePostCreated(keywordAlertTestJob())

	got := repo.byMember()
	want := map[string][]string{
		"follower":   {"follow"},
		"subscriber": {"subscribe"},
		"normal":     {"keyword"},
		"msgblocker": {"keyword"}, // 쪽지 한정 차단은 글 알림과 무관
	}
	for mb, cases := range want {
		if strings.Join(got[mb], ",") != strings.Join(cases, ",") {
			t.Errorf("%s: got %v, want %v", mb, got[mb], cases)
		}
	}
	for _, mb := range []string{"author", "blocker", "prefoff", "nomatch"} {
		if len(got[mb]) != 0 {
			t.Errorf("%s 는 알림을 받으면 안 된다: %v", mb, got[mb])
		}
	}
	if len(repo.created) != 4 {
		t.Errorf("총 4건이어야 한다: got %d", len(repo.created))
	}

	for _, n := range repo.created {
		if n.MbID != "normal" {
			continue
		}
		if n.PhFromCase != "write" || n.RelMbID != "author" || n.RelURL != "/free/100" || n.WrParent != 100 {
			t.Errorf("키워드 알림 필드가 다르다: %+v", n)
		}
		if n.RelMsg != "키워드 '아이폰 17' 새 글: 아이폰 17 출시 소식" {
			t.Errorf("가장 긴 키워드로 문구를 만들어야 한다: %q", n.RelMsg)
		}
	}

	// 이벤트 재시도: 같은 글을 다시 처리해도 새 알림이 없고, 시간당 카운터도 오르지 않는다.
	callsBefore := limiter.calls
	w.handlePostCreated(keywordAlertTestJob())
	if len(repo.created) != 4 {
		t.Errorf("재시도 시 중복 발행: got %d", len(repo.created))
	}
	if limiter.calls != callsBefore {
		t.Errorf("이미 보낸 회원은 카운터를 올리지 않아야 한다: %d → %d", callsBefore, limiter.calls)
	}
}

func TestHandlePostCreatedKeywordAlertDisabledKeepsExistingAlerts(t *testing.T) {
	db := newKeywordAlertTestDB(t, "disabled", "html1")
	w, repo := newKeywordAlertTestWorker(db, nil, nil)

	w.handlePostCreated(keywordAlertTestJob())

	got := repo.byMember()
	if len(repo.created) != 2 || strings.Join(got["follower"], ",") != "follow" || strings.Join(got["subscriber"], ",") != "subscribe" {
		t.Fatalf("키워드 단계가 꺼져 있으면 기존 팔로우·구독 알림만: %v", got)
	}
}

func TestHandlePostCreatedKeywordAlertSecretPost(t *testing.T) {
	db := newKeywordAlertTestDB(t, "secret", "html1,secret")
	w, repo := newKeywordAlertTestWorker(db, keywordAlertTestRows, nil)

	w.handlePostCreated(keywordAlertTestJob())

	for _, n := range repo.created {
		if n.PhToCase == keywordAlertToCase {
			t.Errorf("비밀글은 키워드 알림을 보내지 않는다: %+v", n)
		}
	}
	if len(repo.created) != 2 {
		t.Errorf("팔로우·구독 알림은 그대로여야 한다: got %d", len(repo.created))
	}
}

func TestHandlePostCreatedKeywordAlertMissingPostRow(t *testing.T) {
	db := newKeywordAlertTestDB(t, "missingrow", "html1")
	w, repo := newKeywordAlertTestWorker(db, keywordAlertTestRows, nil)
	job := keywordAlertTestJob()
	job.WriteID = 999 // wr_option 을 확인할 수 없는 글 — 안전하게 보내지 않는다

	w.handlePostCreated(job)

	for _, n := range repo.created {
		if n.PhToCase == keywordAlertToCase {
			t.Errorf("비밀글 여부를 확인 못 하면 보내지 않는다: %+v", n)
		}
	}
}

func TestHandlePostCreatedKeywordAlertPrivateBoard(t *testing.T) {
	db := newKeywordAlertTestDB(t, "private", "html1")
	if err := db.Exec(`UPDATE g5_board SET bo_read_level = 2 WHERE bo_table = 'free'`).Error; err != nil {
		t.Fatalf("update board: %v", err)
	}
	w, repo := newKeywordAlertTestWorker(db, keywordAlertTestRows, nil)

	w.handlePostCreated(keywordAlertTestJob())

	if len(repo.created) != 0 {
		t.Errorf("비공개 보드는 어떤 새 글 알림도 보내지 않는다: %v", repo.byMember())
	}
}

func TestHandlePostCreatedKeywordAlertHourlyLimit(t *testing.T) {
	db := newKeywordAlertTestDB(t, "hourly", "html1")
	limiter := &fakeKeywordLimiter{deny: map[string]bool{"normal": true}}
	w, repo := newKeywordAlertTestWorker(db, keywordAlertTestRows, limiter)

	w.handlePostCreated(keywordAlertTestJob())

	got := repo.byMember()
	if len(got["normal"]) != 0 {
		t.Errorf("시간당 상한을 넘은 회원은 받지 않는다: %v", got["normal"])
	}
	if len(got["msgblocker"]) != 1 {
		t.Errorf("상한 안의 회원은 받는다: %v", got["msgblocker"])
	}
}

func TestHandlePostCreatedKeywordAlertRecipientCap(t *testing.T) {
	db := newKeywordAlertTestDB(t, "cap", "html1")
	rows := []KeywordAlertRow{
		{MbID: "m1", Keyword: "아이폰"},
		{MbID: "m2", Keyword: "아이폰"},
		{MbID: "m3", Keyword: "아이폰"},
	}
	w, repo := newKeywordAlertTestWorker(db, rows, nil)
	w.keywordMaxRecipients = 2

	w.handlePostCreated(keywordAlertTestJob())

	got := repo.byMember()
	if len(got["m1"]) != 1 || len(got["m2"]) != 1 || len(got["m3"]) != 0 {
		t.Errorf("글당 수신자 상한으로 잘라야 한다: %v", got)
	}
}

func TestHandlePostCreatedKeywordAlertUnescapesTitle(t *testing.T) {
	db := newKeywordAlertTestDB(t, "unescape", "html1")
	rows := []KeywordAlertRow{{MbID: "amp", Keyword: "AT&T"}}
	w, repo := newKeywordAlertTestWorker(db, rows, nil)
	job := keywordAlertTestJob()
	job.Subject = "AT&amp;T 요금제 정리"

	w.handlePostCreated(job)

	if got := repo.byMember(); strings.Join(got["amp"], ",") != keywordAlertToCase {
		t.Errorf("HTML 이스케이프된 제목도 화면 글자 기준으로 일치해야 한다: %v", got)
	}
}

func TestRedisKeywordAlertLimiter(t *testing.T) {
	counts := make(map[string]int64)
	var failErr error
	l := &RedisKeywordAlertLimiter{
		limit: keywordAlertHourlyLimit,
		incr: func(_ context.Context, key string) (int64, error) {
			if failErr != nil {
				return 0, failErr
			}
			counts[key]++
			return counts[key], nil
		},
	}
	now := time.Date(2026, 10, 12, 3, 59, 0, 0, time.UTC)
	ctx := context.Background()

	for i := 1; i <= keywordAlertHourlyLimit; i++ {
		if !l.Allow(ctx, "user1", now) {
			t.Fatalf("%d번째는 허용돼야 한다", i)
		}
	}
	if l.Allow(ctx, "user1", now) {
		t.Fatalf("%d건 초과는 막아야 한다", keywordAlertHourlyLimit)
	}
	if !l.Allow(ctx, "user2", now) {
		t.Fatal("다른 회원은 따로 센다")
	}
	if !l.Allow(ctx, "user1", now.Add(time.Minute)) {
		t.Fatal("다음 시각(UTC 시 단위)에는 다시 허용")
	}

	failErr = errors.New("redis down")
	if !l.Allow(ctx, "user1", now) {
		t.Fatal("Redis 오류 시에는 보낸다")
	}

	var nilLimiter *RedisKeywordAlertLimiter
	if !nilLimiter.Allow(ctx, "user1", now) {
		t.Fatal("Redis 가 없으면 상한 없이 보낸다")
	}
	if NewRedisKeywordAlertLimiter(nil) != nil {
		t.Fatal("nil 클라이언트면 nil 리미터")
	}
}

func TestKeywordAlertRateKey(t *testing.T) {
	kst := time.FixedZone("KST", 9*3600)
	at := time.Date(2026, 10, 12, 12, 59, 0, 0, kst)
	if got := keywordAlertRateKey("user1", at); got != "kwalert:user1:2026101203" {
		t.Fatalf("UTC 시각 기준 키여야 한다: %q", got)
	}
}

func TestIsSecretPostOption(t *testing.T) {
	cases := map[string]bool{
		"":             false,
		"html1":        false,
		"secret":       true,
		"html1,secret": true,
		"secret,mail":  true,
		"html1,mail":   false,
	}
	for opt, want := range cases {
		if got := isSecretPostOption(opt); got != want {
			t.Errorf("isSecretPostOption(%q) = %v, want %v", opt, got, want)
		}
	}
}

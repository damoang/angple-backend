package cron

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func agoYMD(days int) string {
	return time.Now().AddDate(0, 0, -days).Format("20060102")
}

func setupWithdrawalDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_member (
		mb_no INTEGER PRIMARY KEY AUTOINCREMENT,
		mb_id TEXT,
		mb_nick TEXT,
		mb_nick_date TEXT,
		mb_leave_date TEXT,
		mb_intercept_date TEXT,
		mb_dupinfo TEXT,
		mb_ip TEXT
	)`).Error; err != nil {
		t.Fatalf("create g5_member: %v", err)
	}
	if err := db.Exec(`CREATE TABLE v2_users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT,
		nickname TEXT
	)`).Error; err != nil {
		t.Fatalf("create v2_users: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_board (bo_table TEXT)`).Error; err != nil {
		t.Fatalf("create g5_board: %v", err)
	}
	if err := db.Exec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY AUTOINCREMENT,
		mb_id TEXT,
		wr_name TEXT,
		wr_subject TEXT,
		wr_content TEXT
	)`).Error; err != nil {
		t.Fatalf("create g5_write_free: %v", err)
	}
	db.Exec(`INSERT INTO g5_board (bo_table) VALUES ('free')`)
	return db
}

func nick(t *testing.T, db *gorm.DB, mbID string) string {
	t.Helper()
	var n string
	if err := db.Table("g5_member").Select("mb_nick").Where("mb_id = ?", mbID).Row().Scan(&n); err != nil {
		t.Fatalf("scan nick: %v", err)
	}
	return n
}

func TestRunWithdrawalGraceAnonymize(t *testing.T) {
	db := setupWithdrawalDB(t)
	// confirmed(40일 경과, 제재 겸함) / grace(5일) / 이미 익명화(멱등)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_intercept_date, mb_dupinfo, mb_ip) VALUES
		('gone', '떠난이', ?, '20260615', 'DI-GONE', '1.2.3.4'),
		('staying', '남는이', ?, '', 'DI-STAY', '5.6.7.8'),
		('done', '탈퇴회원_2', ?, '', 'DI-DONE', '9.9.9.9')`,
		agoYMD(40), agoYMD(5), agoYMD(60))
	db.Exec(`INSERT INTO v2_users (username, nickname) VALUES ('gone', '떠난이'), ('staying', '남는이')`)
	// gone 의 과거 게시물(작성자명 + 본문에 옛 닉 PII), staying 의 게시물(치환되면 안 됨)
	db.Exec(`INSERT INTO g5_write_free (mb_id, wr_name, wr_subject, wr_content) VALUES
		('gone', '떠난이', '떠난이의 글', '본문에 떠난이 언급'),
		('staying', '남는이', '남는이의 글', '남는이 본문')`)

	res, err := runWithdrawalGraceAnonymize(db)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// MEDIUM: 과거 게시물 작성자명/본문 내 닉(PII) 치환 확인
	if res.PostsUpdated < 1 {
		t.Errorf("PostsUpdated = %d, want >= 1", res.PostsUpdated)
	}
	var post struct {
		WrName    string `gorm:"column:wr_name"`
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
	}
	db.Table("g5_write_free").Where("mb_id = ?", "gone").Take(&post)
	if post.WrName == "떠난이" {
		t.Errorf("gone post wr_name should be anonymized, got %q", post.WrName)
	}
	if strings.Contains(post.WrSubject, "떠난이") || strings.Contains(post.WrContent, "떠난이") {
		t.Errorf("gone post PII should be replaced: subject=%q content=%q", post.WrSubject, post.WrContent)
	}
	// staying(숙려중)의 게시물은 절대 건드리면 안 됨
	var sName string
	db.Table("g5_write_free").Select("wr_name").Where("mb_id = ?", "staying").Row().Scan(&sName)
	if sName != "남는이" {
		t.Errorf("staying post must be untouched, got %q", sName)
	}

	// confirmed 1건만 익명화, done 은 이미 익명화되어 skip
	if res.AnonymizedCount != 1 {
		t.Errorf("AnonymizedCount = %d, want 1", res.AnonymizedCount)
	}
	if res.SkippedCount != 1 {
		t.Errorf("SkippedCount = %d, want 1", res.SkippedCount)
	}
	if res.Errors != 0 {
		t.Errorf("Errors = %d, want 0", res.Errors)
	}

	// gone 은 익명화됨 (닉 변경, 접두 탈퇴)
	if n := nick(t, db, "gone"); n == "떠난이" || n[:len("탈퇴")] != "탈퇴" {
		t.Errorf("gone nick should be anonymized, got %q", n)
	}
	// grace 회원은 그대로
	if n := nick(t, db, "staying"); n != "남는이" {
		t.Errorf("staying nick must be untouched, got %q", n)
	}

	// 가드 #2: DI / 가입IP / intercept / leave_date 보존
	var row struct {
		Dup       string `gorm:"column:mb_dupinfo"`
		IP        string `gorm:"column:mb_ip"`
		Intercept string `gorm:"column:mb_intercept_date"`
		Leave     string `gorm:"column:mb_leave_date"`
	}
	db.Table("g5_member").Where("mb_id = ?", "gone").Take(&row)
	if row.Dup != "DI-GONE" {
		t.Errorf("DI must be preserved, got %q", row.Dup)
	}
	if row.IP != "1.2.3.4" {
		t.Errorf("join IP must be preserved, got %q", row.IP)
	}
	if row.Intercept != "20260615" {
		t.Errorf("intercept must be preserved, got %q", row.Intercept)
	}
	if row.Leave == "" {
		t.Error("mb_leave_date must be kept (확정 상태 유지)")
	}

	// v2_users 미러 동기화
	var vnick string
	db.Table("v2_users").Select("nickname").Where("username = ?", "gone").Row().Scan(&vnick)
	if vnick[:len("탈퇴")] != "탈퇴" {
		t.Errorf("v2_users nickname should mirror anonymized nick, got %q", vnick)
	}
}

// 가드 #4: 멱등 — 2회 실행 시 중복 익명화·에러 없음.
func TestRunWithdrawalGraceIdempotent(t *testing.T) {
	db := setupWithdrawalDB(t)
	db.Exec(`INSERT INTO g5_member (mb_id, mb_nick, mb_leave_date, mb_dupinfo) VALUES ('gone', '떠난이', ?, 'DI-X')`, agoYMD(45))
	db.Exec(`INSERT INTO v2_users (username, nickname) VALUES ('gone', '떠난이')`)

	first, err := runWithdrawalGraceAnonymize(db)
	if err != nil || first.AnonymizedCount != 1 {
		t.Fatalf("first run: anonymized=%d err=%v", first.AnonymizedCount, err)
	}
	nickAfterFirst := nick(t, db, "gone")

	second, err := runWithdrawalGraceAnonymize(db)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	if second.AnonymizedCount != 0 {
		t.Errorf("second run should anonymize 0 (idempotent), got %d", second.AnonymizedCount)
	}
	if second.SkippedCount != 1 {
		t.Errorf("second run should skip 1, got %d", second.SkippedCount)
	}
	if n := nick(t, db, "gone"); n != nickAfterFirst {
		t.Errorf("nick changed on second run: %q -> %q", nickAfterFirst, n)
	}
}

package v2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAnonymizeMemberBacksUpAndReplacesTargetRows(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TMPDIR", tmpDir)

	db, err := gorm.Open(sqlite.Open("file:admin_anonymize_test?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	mustExec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}

	mustExec(`CREATE TABLE v2_users (
		id INTEGER PRIMARY KEY,
		username TEXT,
		email TEXT,
		password TEXT,
		nickname TEXT,
		level INTEGER,
		status TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	mustExec(`CREATE TABLE g5_member (
		mb_id TEXT PRIMARY KEY,
		mb_nick TEXT,
		mb_nick_date TEXT
	)`)
	mustExec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY,
		wr_name TEXT,
		wr_subject TEXT,
		wr_content TEXT
	)`)
	mustExec(`CREATE TABLE g5_write_referral (
		wr_id INTEGER PRIMARY KEY,
		wr_name TEXT,
		wr_subject TEXT,
		wr_content TEXT
	)`)
	mustExec(`CREATE TABLE g5_write_disciplinelog (
		wr_id INTEGER PRIMARY KEY,
		wr_name TEXT,
		wr_subject TEXT,
		wr_content TEXT
	)`)

	mustExec(`INSERT INTO v2_users (id, username, email, password, nickname, level, status, created_at, updated_at)
		VALUES (7, 'user7', 'u7@example.com', 'pw', '전동운', 1, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	mustExec(`INSERT INTO g5_member (mb_id, mb_nick, mb_nick_date) VALUES ('user7', '전동운', '2026-03-01')`)
	mustExec(`INSERT INTO g5_write_free (wr_id, wr_name, wr_subject, wr_content)
		VALUES (5936618, '전동운', '전동운 게시글', '본문에 전동운 언급')`)
	mustExec(`INSERT INTO g5_write_free (wr_id, wr_name, wr_subject, wr_content)
		VALUES (5942893, '전동운', '', '댓글 전동운')`)
	mustExec(`INSERT INTO g5_write_disciplinelog (wr_id, wr_name, wr_subject, wr_content)
		VALUES (3348, '관리자', '전동운 (user7) 님에 대한 이용제한 안내', '{"note":"전동운 처리"}')`)

	svc := NewAdminService(
		db,
		v2repo.NewUserRepository(db),
		nil,
		nil,
		nil,
		gnurepo.NewMemberRepository(db),
	)

	result, err := svc.AnonymizeMember(7, AnonymizeMemberRequest{
		ReplacementNickname: "탈퇴_1",
		Targets: []string{
			"https://damoang.net/free/5936618",
			"https://damoang.net/free/5936618#c_5942893",
			"https://damoang.net/disciplinelog/3348",
			"https://damoang.net/free/5936618",
		},
		SearchTexts: []string{"전동운"},
	})
	if err != nil {
		t.Fatalf("AnonymizeMember returned error: %v", err)
	}

	var nickname string
	if err := db.Table("v2_users").Select("nickname").Where("id = ?", 7).Scan(&nickname).Error; err != nil {
		t.Fatalf("query v2 nickname: %v", err)
	}
	if nickname != "탈퇴_1" {
		t.Fatalf("nickname = %q, want 탈퇴_1", nickname)
	}
	if err := db.Table("g5_member").Select("mb_nick").Where("mb_id = ?", "user7").Scan(&nickname).Error; err != nil {
		t.Fatalf("query legacy nickname: %v", err)
	}
	if nickname != "탈퇴_1" {
		t.Fatalf("legacy nickname = %q, want 탈퇴_1", nickname)
	}

	assertRowContains := func(table string, rowID int, want string) {
		t.Helper()
		var row struct {
			Name    string `gorm:"column:wr_name"`
			Subject string `gorm:"column:wr_subject"`
			Content string `gorm:"column:wr_content"`
		}
		if err := db.Table(table).Where("wr_id = ?", rowID).Take(&row).Error; err != nil {
			t.Fatalf("query %s/%d: %v", table, rowID, err)
		}
		joined := row.Name + "|" + row.Subject + "|" + row.Content
		if strings.Contains(joined, "전동운") {
			t.Fatalf("%s/%d still contains original text: %s", table, rowID, joined)
		}
		if !strings.Contains(joined, want) {
			t.Fatalf("%s/%d does not contain %q: %s", table, rowID, want, joined)
		}
	}
	assertRowContains("g5_write_free", 5936618, "탈퇴_1")
	assertRowContains("g5_write_free", 5942893, "탈퇴_1")
	assertRowContains("g5_write_disciplinelog", 3348, "탈퇴_1")

	backupBytes, err := os.ReadFile(result.BackupFile)
	if err != nil {
		t.Fatalf("read backup file: %v", err)
	}
	if !strings.Contains(string(backupBytes), "전동운") {
		t.Fatalf("backup file should contain original text, got: %s", string(backupBytes))
	}
	if _, err := os.Stat(result.ManifestFile); err != nil {
		t.Fatalf("manifest file missing: %v", err)
	}
	if filepath.Dir(result.BackupFile) != filepath.Join(tmpDir, "damoang-anonymize") {
		t.Fatalf("backup file path = %s, want under %s", result.BackupFile, tmpDir)
	}
	if len(result.ResolvedTargets) != 3 {
		t.Fatalf("resolved target count = %d, want 3", len(result.ResolvedTargets))
	}
}

func TestResolveTargetsDeduplicatesCommentLinks(t *testing.T) {
	targets, err := resolveTargets([]string{
		"https://damoang.net/free/1#c_2",
		"https://damoang.net/free/1#c_2",
		"https://damoang.net/free/1",
	})
	if err != nil {
		t.Fatalf("resolveTargets returned error: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("resolved target count = %d, want 2", len(targets))
	}
	if targets[0].CommentID != nil {
		t.Fatalf("expected post-only target to sort first")
	}
	if targets[1].CommentID == nil || *targets[1].CommentID != 2 {
		t.Fatalf("expected second target to contain comment id 2")
	}
}

func TestAnonymizeMemberDefaultsToGenericNickname(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:admin_anonymize_default_test?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	mustExec := func(sql string) {
		t.Helper()
		if err := db.Exec(sql).Error; err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}

	mustExec(`CREATE TABLE v2_users (
		id INTEGER PRIMARY KEY,
		username TEXT,
		email TEXT,
		password TEXT,
		nickname TEXT,
		level INTEGER,
		status TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	mustExec(`CREATE TABLE g5_member (
		mb_id TEXT PRIMARY KEY,
		mb_nick TEXT,
		mb_nick_date TEXT
	)`)
	mustExec(`CREATE TABLE g5_write_free (
		wr_id INTEGER PRIMARY KEY,
		wr_name TEXT,
		wr_subject TEXT,
		wr_content TEXT
	)`)
	mustExec(`INSERT INTO v2_users (id, username, email, password, nickname, level, status, created_at, updated_at)
		VALUES (9, 'user9', 'u9@example.com', 'pw', '전동운', 1, 'active', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`)
	mustExec(`INSERT INTO g5_member (mb_id, mb_nick, mb_nick_date) VALUES ('user9', '전동운', '2026-03-01')`)
	mustExec(`INSERT INTO g5_write_free (wr_id, wr_name, wr_subject, wr_content)
		VALUES (1, '전동운', '전동운 제목', '전동운 본문')`)

	svc := NewAdminService(
		db,
		v2repo.NewUserRepository(db),
		nil,
		nil,
		nil,
		gnurepo.NewMemberRepository(db),
	)

	result, err := svc.AnonymizeMember(9, AnonymizeMemberRequest{
		Targets:     []string{"https://damoang.net/free/1"},
		SearchTexts: []string{"전동운"},
	})
	if err != nil {
		t.Fatalf("AnonymizeMember returned error: %v", err)
	}
	if result.ReplacementNickname != defaultAnonymizedNickname {
		t.Fatalf("replacement nickname = %q, want %q", result.ReplacementNickname, defaultAnonymizedNickname)
	}
	if result.MemberID != 9 || result.Username != "user9" {
		t.Fatalf("unexpected internal identifiers: %+v", result)
	}

	var row struct {
		Name    string `gorm:"column:wr_name"`
		Subject string `gorm:"column:wr_subject"`
		Content string `gorm:"column:wr_content"`
	}
	if err := db.Table("g5_write_free").Where("wr_id = ?", 1).Take(&row).Error; err != nil {
		t.Fatalf("query updated row: %v", err)
	}
	joined := row.Name + "|" + row.Subject + "|" + row.Content
	if strings.Contains(joined, "전동운") {
		t.Fatalf("updated row still contains original nickname: %s", joined)
	}
	if !strings.Contains(joined, defaultAnonymizedNickname) {
		t.Fatalf("updated row does not contain default nickname: %s", joined)
	}
}

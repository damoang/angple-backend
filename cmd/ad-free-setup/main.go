// Ad-Free 멤버십 운영 DB 셋업 — navertest 계정 + 멤버십 부여.
// 일회성 도구 (5/20 네이버페이 재심사용). 다른 환경 재사용 금지.
package main

import (
	"bufio"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

const (
	envPath    = "/home/angple/backend/.env"
	secretPath = "/home/damoang/docs/ad-free-membership/secrets/navertest.txt"
	migPath    = "/home/angple/web/plugins/ad-free/migrations/001_ad_free_membership.up.sql"
)

func loadEnv(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	env := map[string]string{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.Index(line, "=")
		if i < 0 {
			continue
		}
		k := strings.TrimSpace(line[:i])
		v := strings.Trim(strings.TrimSpace(line[i+1:]), `"'`)
		env[k] = v
	}
	return env, s.Err()
}

func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func writeSecret(plain string) error {
	body := fmt.Sprintf("navertest\n%s\nemail: navertest@damoang.net\nupdated: %s\n",
		plain, time.Now().Format(time.RFC3339))
	if err := os.WriteFile(secretPath, []byte(body), 0600); err != nil {
		return err
	}
	return os.Chmod(secretPath, 0600)
}

type colInfo struct {
	name      string
	notNull   bool
	hasDefault bool
	colType   string
}

func showColumns(db *sql.DB, table string) (map[string]colInfo, error) {
	rows, err := db.Query("SHOW COLUMNS FROM " + table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]colInfo{}
	for rows.Next() {
		var field, ctype, nullable, key, extra string
		var defaultVal sql.NullString
		if err := rows.Scan(&field, &ctype, &nullable, &key, &defaultVal, &extra); err != nil {
			return nil, err
		}
		out[field] = colInfo{
			name:       field,
			notNull:    nullable == "NO",
			hasDefault: defaultVal.Valid || strings.Contains(extra, "auto_increment"),
			colType:    ctype,
		}
	}
	return out, rows.Err()
}

func intLikeType(t string) bool {
	t = strings.ToLower(t)
	return strings.Contains(t, "int") || strings.Contains(t, "decimal") || strings.Contains(t, "float") || strings.Contains(t, "double")
}

func main() {
	env, err := loadEnv(envPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "env load fail: %v\n", err)
		os.Exit(2)
	}
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true",
		env["DB_USER"], env["DB_PASSWORD"], env["DB_HOST"], env["DB_PORT"], env["DB_NAME"])
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "db open fail: %v\n", err)
		os.Exit(2)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "db ping fail: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("[1/8] DB connected: %s/%s\n", env["DB_HOST"], env["DB_NAME"])

	var existing int
	if err := db.QueryRow("SELECT COUNT(*) FROM g5_member WHERE mb_id='navertest' OR mb_email='navertest@damoang.net'").Scan(&existing); err != nil {
		fmt.Fprintf(os.Stderr, "precheck fail: %v\n", err)
		os.Exit(3)
	}
	if existing > 0 {
		fmt.Fprintf(os.Stderr, "[ABORT] navertest already exists (%d). Run rollback first.\n", existing)
		os.Exit(3)
	}
	fmt.Println("[2/8] precheck: navertest 없음 OK")

	cols, err := showColumns(db, "g5_member")
	if err != nil {
		fmt.Fprintf(os.Stderr, "show columns fail: %v\n", err)
		os.Exit(3)
	}
	fmt.Printf("[3/8] g5_member columns: %d개\n", len(cols))

	insertVals := map[string]any{
		"mb_id":             "navertest",
		"mb_password":       nil,
		"mb_name":           "네이버페이 심사용",
		"mb_nick":           "navertest",
		"mb_email":          "navertest@damoang.net",
		"mb_level":          1,
		"mb_mailling":       0,
		"mb_sms":            0,
		"mb_open":           0,
		"mb_datetime":       "__NOW__",
		"mb_today_login":    "__NOW__",
		"mb_ip":             "127.0.0.1",
		"mb_leave_date":     "",
		"mb_intercept_date": "",
		"mb_email_certify":  "__NOW__",
		"mb_certify":        "email",
	}

	missing := []string{}
	for name, info := range cols {
		if info.notNull && !info.hasDefault {
			if _, ok := insertVals[name]; !ok {
				missing = append(missing, name)
				if intLikeType(info.colType) {
					insertVals[name] = 0
				} else {
					insertVals[name] = ""
				}
			}
		}
	}
	if len(missing) > 0 {
		fmt.Printf("[3.1] WARN NOT NULL & no-default missing → 빈값 보강: %s\n", strings.Join(missing, ","))
	}

	plain, err := randHex(8)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rand fail: %v\n", err)
		os.Exit(4)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bcrypt fail: %v\n", err)
		os.Exit(4)
	}
	insertVals["mb_password"] = string(hash)
	if err := writeSecret(plain); err != nil {
		fmt.Fprintf(os.Stderr, "secret write fail: %v\n", err)
		os.Exit(4)
	}
	fmt.Printf("[4/8] 비번 16자 생성 + secrets 저장 (%s, chmod 600)\n", secretPath)

	migSQL, err := os.ReadFile(migPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration read fail: %v\n", err)
		os.Exit(4)
	}
	// SQL parser — 라인 단위로 -- comment 제거 후 ';' split.
	// (chunk 첫 줄이 comment 이면 통째로 버려지던 버그 fix.)
	cleanedLines := []string{}
	for _, ln := range strings.Split(string(migSQL), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "--") {
			continue
		}
		cleanedLines = append(cleanedLines, ln)
	}
	cleaned := strings.Join(cleanedLines, "\n")
	execCount := 0
	for _, stmt := range strings.Split(cleaned, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			fmt.Fprintf(os.Stderr, "CREATE fail: %v\nstmt: %s\n", err, stmt)
			os.Exit(4)
		}
		execCount++
	}
	fmt.Printf("[5/8] ad_free_membership 테이블 CREATE OK (executed %d stmt)\n", execCount)

	// 실제 테이블 존재 검증
	var tblName sql.NullString
	_ = db.QueryRow("SHOW TABLES LIKE 'ad_free_membership'").Scan(&tblName)
	if !tblName.Valid {
		fmt.Fprintf(os.Stderr, "[ABORT] CREATE 실행됐으나 테이블 미생성 — SQL parser 또는 권한 이슈\n")
		os.Exit(4)
	}

	tx, err := db.Begin()
	if err != nil {
		fmt.Fprintf(os.Stderr, "begin tx fail: %v\n", err)
		os.Exit(5)
	}
	rollback := func(msg string) {
		_ = tx.Rollback()
		fmt.Fprintf(os.Stderr, "[ROLLBACK] %s\n", msg)
		os.Exit(5)
	}

	colNames := []string{}
	placeholders := []string{}
	args := []any{}
	for k, v := range insertVals {
		colNames = append(colNames, "`"+k+"`")
		if v == "__NOW__" {
			placeholders = append(placeholders, "NOW()")
		} else {
			placeholders = append(placeholders, "?")
			args = append(args, v)
		}
	}
	sqlStmt := "INSERT INTO g5_member (" + strings.Join(colNames, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	res, err := tx.Exec(sqlStmt, args...)
	if err != nil {
		rollback(fmt.Sprintf("g5_member INSERT fail: %v", err))
	}
	n, _ := res.RowsAffected()
	fmt.Printf("[6/8] g5_member INSERT OK (rows=%d)\n", n)

	res2, err := tx.Exec(
		"INSERT INTO ad_free_membership (mb_id, site_id, plan, status, current_period_end, payment_provider, payment_order_id) "+
			"VALUES (?, 'default', 'yearly', 'active', DATE_ADD(NOW(), INTERVAL 90 DAY), 'manual', NULL)",
		"navertest",
	)
	if err != nil {
		rollback(fmt.Sprintf("ad_free_membership INSERT fail: %v", err))
	}
	n2, _ := res2.RowsAffected()
	fmt.Printf("[7/8] ad_free_membership INSERT OK (rows=%d)\n", n2)

	if err := tx.Commit(); err != nil {
		rollback(fmt.Sprintf("commit fail: %v", err))
	}

	verify1 := map[string]any{}
	var mbID, mbEmail string
	var mbLevel int
	var mbDatetime time.Time
	_ = db.QueryRow("SELECT mb_id, mb_email, mb_level, mb_datetime FROM g5_member WHERE mb_id='navertest'").
		Scan(&mbID, &mbEmail, &mbLevel, &mbDatetime)
	verify1["mb_id"] = mbID
	verify1["mb_email"] = mbEmail
	verify1["mb_level"] = mbLevel
	verify1["mb_datetime"] = mbDatetime.Format(time.RFC3339)
	b1, _ := json.Marshal(verify1)
	fmt.Printf("[8/8] verify g5_member: %s\n", b1)

	verify2 := map[string]any{}
	var plan, status, periodEnd string
	_ = db.QueryRow("SELECT mb_id, plan, status, DATE_FORMAT(current_period_end,'%Y-%m-%d %H:%i') FROM ad_free_membership WHERE mb_id='navertest'").
		Scan(&mbID, &plan, &status, &periodEnd)
	verify2["mb_id"] = mbID
	verify2["plan"] = plan
	verify2["status"] = status
	verify2["period_end"] = periodEnd
	b2, _ := json.Marshal(verify2)
	fmt.Printf("[8/8] verify ad_free_membership: %s\n", b2)

	fmt.Printf("\nDONE — 비번 파일: %s\n", secretPath)
}

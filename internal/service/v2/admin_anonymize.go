package v2

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

const defaultAnonymizedNickname = "탈퇴사용자"

var allowedAnonymizeBoards = map[string]bool{
	"free":          true,
	"referral":      true,
	"disciplinelog": true,
}

type AnonymizeMemberRequest struct {
	ReplacementNickname string   `json:"replacement_nickname"`
	Targets             []string `json:"targets"`
	SearchTexts         []string `json:"search_texts"`
}

type AnonymizeMemberResult struct {
	MemberID            uint64                  `json:"member_id"`
	Username            string                  `json:"username"`
	ReplacementNickname string                  `json:"replacement_nickname"`
	SearchTexts         []string                `json:"search_texts"`
	ResolvedTargets     []AnonymizeTargetResult `json:"resolved_targets"`
	UpdatedRows         []UpdatedRowResult      `json:"updated_rows"`
	SkippedRows         []SkippedRowResult      `json:"skipped_rows"`
	BackupFile          string                  `json:"backup_file"`
	ManifestFile        string                  `json:"manifest_file"`
}

type AnonymizeTargetResult struct {
	URL       string `json:"url"`
	BoardID   string `json:"board_id"`
	PostID    int    `json:"post_id"`
	CommentID *int   `json:"comment_id,omitempty"`
}

type UpdatedRowResult struct {
	Table  string `json:"table"`
	RowID  string `json:"row_id"`
	Reason string `json:"reason"`
}

type SkippedRowResult struct {
	Table  string `json:"table"`
	RowID  string `json:"row_id"`
	Reason string `json:"reason"`
}

type manifestFile struct {
	GeneratedAt         string                  `json:"generated_at"`
	MemberID            uint64                  `json:"member_id"`
	Username            string                  `json:"username"`
	ReplacementNickname string                  `json:"replacement_nickname"`
	SearchTexts         []string                `json:"search_texts"`
	Targets             []AnonymizeTargetResult `json:"targets"`
	Rows                []backupRow             `json:"rows"`
	BackupFile          string                  `json:"backup_file"`
}

type backupRow struct {
	Table string `json:"table"`
	RowID string `json:"row_id"`
}

type resolvedTarget struct {
	BoardID   string
	PostID    int
	CommentID *int
	URL       string
}

type targetRow struct {
	Table  string
	RowID  int
	Reason string
}

func (s *AdminService) AnonymizeMember(memberID uint64, req AnonymizeMemberRequest) (*AnonymizeMemberResult, error) {
	if s.db == nil || s.memberRepo == nil {
		return nil, fmt.Errorf("anonymize service dependencies are not configured")
	}
	user, err := s.userRepo.FindByID(memberID)
	if err != nil {
		return nil, err
	}
	return s.anonymizeMemberUser(user, req)
}

func (s *AdminService) AnonymizeMemberByUsername(username string, req AnonymizeMemberRequest) (*AnonymizeMemberResult, error) {
	if s.db == nil || s.memberRepo == nil {
		return nil, fmt.Errorf("anonymize service dependencies are not configured")
	}
	user, err := s.userRepo.FindByUsername(username)
	if err != nil {
		return nil, err
	}
	return s.anonymizeMemberUser(user, req)
}

func (s *AdminService) anonymizeMemberUser(user *v2domain.V2User, req AnonymizeMemberRequest) (*AnonymizeMemberResult, error) {
	replacement := strings.TrimSpace(req.ReplacementNickname)
	if replacement == "" {
		replacement = defaultAnonymizedNickname
	}
	if len(req.Targets) == 0 {
		return nil, fmt.Errorf("at least one target URL is required")
	}

	searchTexts := normalizeSearchTexts(req.SearchTexts, user.Nickname, replacement)
	if len(searchTexts) == 0 {
		return nil, fmt.Errorf("at least one non-empty search text is required")
	}

	targets, err := resolveTargets(req.Targets)
	if err != nil {
		return nil, err
	}
	rows := collectTargetRows(targets)

	backupDir := filepath.Join(os.TempDir(), "damoang-anonymize")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	fileBase := fmt.Sprintf("member-%d-%s", user.ID, ts)
	backupFile := filepath.Join(backupDir, fileBase+".sql")
	manifestFilePath := filepath.Join(backupDir, fileBase+".json")

	result := &AnonymizeMemberResult{
		MemberID:            user.ID,
		Username:            user.Username,
		ReplacementNickname: replacement,
		SearchTexts:         searchTexts,
		BackupFile:          backupFile,
		ManifestFile:        manifestFilePath,
	}
	for _, target := range targets {
		result.ResolvedTargets = append(result.ResolvedTargets, AnonymizeTargetResult{
			URL:       target.URL,
			BoardID:   target.BoardID,
			PostID:    target.PostID,
			CommentID: target.CommentID,
		})
	}

	if err := s.writeBackupFiles(user, replacement, searchTexts, result.ResolvedTargets, rows, backupFile, manifestFilePath); err != nil {
		return nil, err
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("v2_users").Where("id = ?", user.ID).Update("nickname", replacement).Error; err != nil {
			return err
		}
		memberUpdates := map[string]any{"mb_nick": replacement}
		if len(searchTexts) > 0 {
			memberUpdates["mb_nick_date"] = time.Now().Format("2006-01-02")
		}
		memberResult := tx.Table("g5_member").Where("mb_id = ?", user.Username).Updates(memberUpdates)
		if memberResult.Error != nil {
			return memberResult.Error
		}
		if memberResult.RowsAffected > 0 {
			result.UpdatedRows = append(result.UpdatedRows, UpdatedRowResult{
				Table:  "g5_member",
				RowID:  user.Username,
				Reason: "member nickname updated",
			})
		} else {
			result.SkippedRows = append(result.SkippedRows, SkippedRowResult{
				Table:  "g5_member",
				RowID:  user.Username,
				Reason: "member row not found",
			})
		}

		for _, row := range rows {
			changed, err := applyRowReplacement(tx, row.Table, row.RowID, searchTexts, replacement)
			if err != nil {
				return err
			}
			rowID := strconv.Itoa(row.RowID)
			if changed {
				result.UpdatedRows = append(result.UpdatedRows, UpdatedRowResult{
					Table:  row.Table,
					RowID:  rowID,
					Reason: row.Reason,
				})
				continue
			}
			result.SkippedRows = append(result.SkippedRows, SkippedRowResult{
				Table:  row.Table,
				RowID:  rowID,
				Reason: "no matching text found",
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func normalizeSearchTexts(values []string, fallback string, replacement string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values)+1)
	appendText := func(v string) {
		v = strings.TrimSpace(v)
		if v == "" || v == replacement {
			return
		}
		if _, ok := seen[v]; ok {
			return
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	for _, v := range values {
		appendText(v)
	}
	appendText(fallback)
	sort.Strings(out)
	return out
}

func resolveTargets(rawTargets []string) ([]resolvedTarget, error) {
	seen := make(map[string]struct{})
	targets := make([]resolvedTarget, 0, len(rawTargets))
	for _, raw := range rawTargets {
		target, err := parseTarget(raw)
		if err != nil {
			return nil, err
		}
		key := fmt.Sprintf("%s:%d", target.BoardID, target.PostID)
		if target.CommentID != nil {
			key = fmt.Sprintf("%s#%d", key, *target.CommentID)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].BoardID != targets[j].BoardID {
			return targets[i].BoardID < targets[j].BoardID
		}
		if targets[i].PostID != targets[j].PostID {
			return targets[i].PostID < targets[j].PostID
		}
		leftComment := 0
		if targets[i].CommentID != nil {
			leftComment = *targets[i].CommentID
		}
		rightComment := 0
		if targets[j].CommentID != nil {
			rightComment = *targets[j].CommentID
		}
		return leftComment < rightComment
	})
	return targets, nil
}

func parseTarget(raw string) (resolvedTarget, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return resolvedTarget{}, fmt.Errorf("invalid target URL %q: %w", raw, err)
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 {
		return resolvedTarget{}, fmt.Errorf("unsupported target path %q", parsed.Path)
	}
	boardID := parts[0]
	if !allowedAnonymizeBoards[boardID] {
		return resolvedTarget{}, fmt.Errorf("unsupported board %q", boardID)
	}
	postID, err := strconv.Atoi(parts[1])
	if err != nil || postID <= 0 {
		return resolvedTarget{}, fmt.Errorf("invalid post id in %q", raw)
	}
	var commentID *int
	if frag := parsed.Fragment; strings.HasPrefix(frag, "c_") {
		id, err := strconv.Atoi(strings.TrimPrefix(frag, "c_"))
		if err != nil || id <= 0 {
			return resolvedTarget{}, fmt.Errorf("invalid comment id in %q", raw)
		}
		commentID = &id
	}
	return resolvedTarget{
		BoardID:   boardID,
		PostID:    postID,
		CommentID: commentID,
		URL:       raw,
	}, nil
}

func collectTargetRows(targets []resolvedTarget) []targetRow {
	seen := make(map[string]struct{})
	rows := make([]targetRow, 0, len(targets)*2)
	for _, target := range targets {
		table := "g5_write_" + target.BoardID
		postKey := fmt.Sprintf("%s:%d", table, target.PostID)
		if _, ok := seen[postKey]; !ok {
			seen[postKey] = struct{}{}
			rows = append(rows, targetRow{Table: table, RowID: target.PostID, Reason: "post content updated"})
		}
		if target.CommentID != nil {
			commentKey := fmt.Sprintf("%s:%d", table, *target.CommentID)
			if _, ok := seen[commentKey]; ok {
				continue
			}
			seen[commentKey] = struct{}{}
			rows = append(rows, targetRow{Table: table, RowID: *target.CommentID, Reason: "comment content updated"})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Table != rows[j].Table {
			return rows[i].Table < rows[j].Table
		}
		return rows[i].RowID < rows[j].RowID
	})
	return rows
}

func (s *AdminService) writeBackupFiles(user *v2domain.V2User, replacement string, searchTexts []string, targets []AnonymizeTargetResult, rows []targetRow, backupPath string, manifestPath string) error {
	sqlText, manifestRows, err := s.buildBackupSQL(user, rows)
	if err != nil {
		return err
	}
	if err := os.WriteFile(backupPath, []byte(sqlText), 0o600); err != nil {
		return err
	}
	manifest := manifestFile{
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		MemberID:            user.ID,
		Username:            user.Username,
		ReplacementNickname: replacement,
		SearchTexts:         searchTexts,
		Targets:             targets,
		Rows:                manifestRows,
		BackupFile:          backupPath,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(manifestPath, data, 0o600)
}

func (s *AdminService) buildBackupSQL(user *v2domain.V2User, rows []targetRow) (string, []backupRow, error) {
	var builder strings.Builder
	manifestRows := []backupRow{{Table: "v2_users", RowID: strconv.FormatUint(user.ID, 10)}, {Table: "g5_member", RowID: user.Username}}
	builder.WriteString("-- Damoang anonymize backup\n")
	builder.WriteString("START TRANSACTION;\n")

	sqlText, err := backupRowsAsSQL(s.db, "v2_users", "id", []int{int(user.ID)})
	if err != nil {
		return "", nil, err
	}
	builder.WriteString(sqlText)

	memberSQL, err := backupRowsByStringIDAsSQL(s.db, "g5_member", "mb_id", []string{user.Username})
	if err != nil {
		return "", nil, err
	}
	builder.WriteString(memberSQL)

	grouped := make(map[string][]int)
	for _, row := range rows {
		grouped[row.Table] = append(grouped[row.Table], row.RowID)
		manifestRows = append(manifestRows, backupRow{Table: row.Table, RowID: strconv.Itoa(row.RowID)})
	}
	tables := make([]string, 0, len(grouped))
	for table := range grouped {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		sqlText, err := backupRowsAsSQL(s.db, table, "wr_id", grouped[table])
		if err != nil {
			return "", nil, err
		}
		builder.WriteString(sqlText)
	}
	builder.WriteString("COMMIT;\n")
	return builder.String(), manifestRows, nil
}

func backupRowsAsSQL(db *gorm.DB, table string, keyColumn string, ids []int) (string, error) {
	if len(ids) == 0 {
		return "", nil
	}
	sort.Ints(ids)
	rows, err := db.Table(table).Where(keyColumn+" IN ?", ids).Order(keyColumn).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	return rowsToInsertSQL(table, rows)
}

func backupRowsByStringIDAsSQL(db *gorm.DB, table string, keyColumn string, ids []string) (string, error) {
	if len(ids) == 0 {
		return "", nil
	}
	rows, err := db.Table(table).Where(keyColumn+" IN ?", ids).Order(keyColumn).Rows()
	if err != nil {
		return "", err
	}
	defer rows.Close()
	return rowsToInsertSQL(table, rows)
}

func rowsToInsertSQL(table string, rows *sql.Rows) (string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for rows.Next() {
		values := make([]sql.RawBytes, len(columns))
		scanArgs := make([]any, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return "", err
		}
		builder.WriteString("REPLACE INTO `")
		builder.WriteString(table)
		builder.WriteString("` (")
		for i, col := range columns {
			if i > 0 {
				builder.WriteString(", ")
			}
			builder.WriteString("`")
			builder.WriteString(col)
			builder.WriteString("`")
		}
		builder.WriteString(") VALUES (")
		for i, value := range values {
			if i > 0 {
				builder.WriteString(", ")
			}
			if value == nil {
				builder.WriteString("NULL")
				continue
			}
			builder.WriteString("'")
			builder.WriteString(escapeSQLString(string(value)))
			builder.WriteString("'")
		}
		builder.WriteString(");\n")
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return builder.String(), nil
}

func escapeSQLString(value string) string {
	replacer := strings.NewReplacer(
		`\\`, `\\\\`,
		`'`, `''`,
		"\n", `\n`,
		"\r", `\r`,
		"\x00", `\0`,
	)
	return replacer.Replace(value)
}

func applyRowReplacement(tx *gorm.DB, table string, rowID int, searchTexts []string, replacement string) (bool, error) {
	type writeRow struct {
		WrName    string `gorm:"column:wr_name"`
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
	}
	var row writeRow
	if err := tx.Table(table).Select("wr_name, wr_subject, wr_content").Where("wr_id = ?", rowID).Take(&row).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}

	updated := map[string]any{}
	newName := replaceAllTexts(row.WrName, searchTexts, replacement)
	if newName != row.WrName {
		updated["wr_name"] = newName
	}
	newSubject := replaceAllTexts(row.WrSubject, searchTexts, replacement)
	if newSubject != row.WrSubject {
		updated["wr_subject"] = newSubject
	}
	newContent := replaceAllTexts(row.WrContent, searchTexts, replacement)
	if newContent != row.WrContent {
		updated["wr_content"] = newContent
	}
	if len(updated) == 0 {
		return false, nil
	}
	if err := tx.Table(table).Where("wr_id = ?", rowID).Updates(updated).Error; err != nil {
		return false, err
	}
	return true, nil
}

func replaceAllTexts(value string, searchTexts []string, replacement string) string {
	out := value
	for _, search := range searchTexts {
		if search == "" {
			continue
		}
		out = strings.ReplaceAll(out, search, replacement)
	}
	return out
}

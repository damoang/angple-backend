package cron

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"gorm.io/gorm"
)

// ProcessReportsResult contains the result of processing approved reports
type ProcessReportsResult struct {
	TotalGroups int      `json:"total_groups"`
	Processed   int      `json:"processed"`
	Errors      int      `json:"errors"`
	Messages    []string `json:"messages"`
	ExecutedAt  string   `json:"executed_at"`
}

// singoReportGroup represents a grouped report from na_singo
type singoReportGroup struct {
	TargetMbID             string  `gorm:"column:target_mb_id"`
	AllReports             string  `gorm:"column:all_reports"`
	AdminDisciplineReasons *string `gorm:"column:admin_discipline_reasons"`
	AdminDisciplineDays    int     `gorm:"column:admin_discipline_days"`
	AdminDisciplineType    string  `gorm:"column:admin_discipline_type"`
	AdminDisciplineDetail  *string `gorm:"column:admin_discipline_detail"`
	TargetTitle            *string `gorm:"column:target_title"`
	TargetContent          *string `gorm:"column:target_content"`
	SgTable                string  `gorm:"column:sg_table"`
	SgID                   int     `gorm:"column:sg_id"`
	SgParent               int     `gorm:"column:sg_parent"`
	ReportCount            int     `gorm:"column:report_count"`
}

type reportedItem struct {
	Table  string `json:"table"`
	ID     int    `json:"id"`
	Parent int    `json:"parent"`
}

// disciplineData is the JSON structure stored in wr_content of g5_write_disciplinelog
type disciplineData struct {
	PenaltyMbID     string         `json:"penalty_mb_id"`
	PenaltyDateFrom string         `json:"penalty_date_from"`
	PenaltyPeriod   int            `json:"penalty_period"`
	PenaltyType     []string       `json:"penalty_type"`
	SgTypes         []int          `json:"sg_types"`
	Content         string         `json:"content,omitempty"`
	ReportedItems   []reportedItem `json:"reported_items"`
	IsBulk          bool           `json:"is_bulk"`
	ReportedURL     string         `json:"reported_url,omitempty"`
	ReportedTable   string         `json:"reported_table,omitempty"`
	ReportedID      int            `json:"reported_id,omitempty"`
	ReportedParent  int            `json:"reported_parent,omitempty"`
	ReportCount     int            `json:"report_count"`
}

// runProcessApprovedReports processes admin-approved reports
func runProcessApprovedReports(db *gorm.DB) (*ProcessReportsResult, error) {
	now := time.Now()
	result := &ProcessReportsResult{
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}

	// 1. Admin 승인된 신고 조회 (그룹핑)
	var groups []singoReportGroup
	if err := db.Raw(`
		SELECT
			target_mb_id,
			GROUP_CONCAT(DISTINCT CONCAT(sg_table, '/', sg_id, '/', sg_parent) ORDER BY sg_id) as all_reports,
			MAX(admin_discipline_reasons) as admin_discipline_reasons,
			MAX(admin_discipline_days) as admin_discipline_days,
			MAX(admin_discipline_type) as admin_discipline_type,
			MAX(admin_discipline_detail) as admin_discipline_detail,
			MAX(target_title) as target_title,
			MAX(target_content) as target_content,
			MIN(sg_table) as sg_table,
			MIN(sg_id) as sg_id,
			MIN(sg_parent) as sg_parent,
			COUNT(*) as report_count
		FROM g5_na_singo
		WHERE admin_approved = 1 AND processed = 0
		GROUP BY target_mb_id, admin_discipline_days, admin_discipline_type, admin_discipline_reasons, DATE(admin_datetime)
		ORDER BY MAX(admin_datetime) ASC
	`).Scan(&groups).Error; err != nil {
		return nil, fmt.Errorf("승인된 신고 조회 실패: %w", err)
	}

	result.TotalGroups = len(groups)
	if len(groups) == 0 {
		result.Messages = append(result.Messages, "처리할 신고가 없습니다")
		return result, nil
	}

	// 2. 각 신고 그룹 처리
	for _, group := range groups {
		if err := processReportGroup(db, &group, now); err != nil {
			result.Errors++
			result.Messages = append(result.Messages, fmt.Sprintf("실패(%s): %v", group.TargetMbID, err))
			log.Printf("[Cron:process-approved-reports] error for %s: %v", group.TargetMbID, err)
			continue
		}
		result.Processed++
		result.Messages = append(result.Messages, fmt.Sprintf("처리완료: %s (신고 %d건)", group.TargetMbID, group.ReportCount))
	}

	return result, nil
}

// processReportGroup processes a single report group
func processReportGroup(db *gorm.DB, group *singoReportGroup, now time.Time) error {
	targetMbID := group.TargetMbID

	// all_reports 파싱 → reported_items 배열
	items := parseReportedItems(group.AllReports, group.SgTable, group.SgID, group.SgParent)

	// 통합 제재 체크: discipline_log_id가 이미 설정된 경우
	var existingLogID int
	isMergedDiscipline := false
	db.Raw(`
		SELECT discipline_log_id FROM g5_na_singo
		WHERE target_mb_id = ? AND admin_approved = 1 AND processed = 0 AND discipline_log_id > 0
		LIMIT 1
	`, targetMbID).Scan(&existingLogID)

	if existingLogID > 0 {
		isMergedDiscipline = true
	}

	// target_mb_id가 없으면 게시글에서 직접 조회
	if targetMbID == "" {
		targetMbID = lookupTargetMbID(db, group)
	}

	// 회원 닉네임 조회
	var targetNick string
	db.Table("g5_member").Select("mb_nick").Where("mb_id = ?", targetMbID).Scan(&targetNick)
	if targetNick == "" {
		targetNick = targetMbID
	}

	// discipline reasons 파싱
	sgTypesArray := parseDisciplineReasons(group.AdminDisciplineReasons)

	disciplineDetail := ""
	if group.AdminDisciplineDetail != nil {
		disciplineDetail = *group.AdminDisciplineDetail
	}

	isBulk := len(items) > 1

	return db.Transaction(func(tx *gorm.DB) error {
		// 2-1. 징계 로그 게시글 작성
		var wrID int
		if isMergedDiscipline {
			wrID = existingLogID
		} else {
			var err error
			wrID, err = createDisciplineLogPost(tx, targetMbID, targetNick, sgTypesArray,
				group.AdminDisciplineDays, group.AdminDisciplineType, disciplineDetail,
				group.SgTable, group.SgID, group.SgParent, group.ReportCount,
				items, isBulk, now)
			if err != nil {
				return fmt.Errorf("징계 로그 작성 실패: %w", err)
			}
		}

		// 2-2. 사용자 제재 적용
		if err := applyUserRestriction(tx, targetMbID, group.AdminDisciplineType,
			group.AdminDisciplineDays, sgTypesArray, now); err != nil {
			return fmt.Errorf("사용자 제재 적용 실패: %w", err)
		}

		// 2-3. 제재 알림 쪽지 발송
		if err := sendDisciplineMemo(tx, targetMbID, targetNick,
			group.AdminDisciplineDays, group.AdminDisciplineType,
			sgTypesArray, disciplineDetail, wrID, now); err != nil {
			log.Printf("[Cron:process-approved-reports] memo send failed for %s: %v", targetMbID, err)
			// 쪽지 발송 실패는 전체 처리를 중단하지 않음
		}

		// 2-4. 신고 처리 완료 표시
		for _, item := range items {
			tx.Exec(`
				UPDATE g5_na_singo
				SET processed = 1, processed_datetime = NOW(), discipline_log_id = ?, version = version + 1
				WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND admin_approved = 1 AND processed = 0
			`, wrID, item.Table, item.ID, item.Parent)
		}

		// BULK_REPORTS에 포함된 추가 신고도 처리
		if disciplineDetail != "" {
			processBulkReports(tx, disciplineDetail, wrID)
		}

		// 2-5. 게시글/댓글별 자동 잠금 체크
		reportLockThreshold := getReportLockThreshold(tx)
		if reportLockThreshold > 0 {
			processedKeys := make(map[string]bool)
			for _, item := range items {
				// 1) 게시글 잠금 (기존 로직 유지)
				postID := item.ID
				if item.Parent > 0 && item.Parent != item.ID {
					postID = item.Parent
				}
				postKey := fmt.Sprintf("%s/%d/post", item.Table, postID)
				if !processedKeys[postKey] {
					processedKeys[postKey] = true
					autoLockPost(tx, item.Table, postID, reportLockThreshold)
				}

				// 2) 댓글 개별 잠금
				if item.Parent > 0 && item.Parent != item.ID {
					commentKey := fmt.Sprintf("%s/%d/comment", item.Table, item.ID)
					if !processedKeys[commentKey] {
						processedKeys[commentKey] = true
						autoLockComment(tx, item.Table, item.ID, item.Parent, reportLockThreshold)
					}
				}
			}
		}

		return nil
	})
}

// parseReportedItems parses "free/123/0,free/456/400" format
func parseReportedItems(allReports string, fallbackTable string, fallbackID, fallbackParent int) []reportedItem {
	var items []reportedItem
	if allReports != "" {
		for _, entry := range strings.Split(allReports, ",") {
			parts := strings.Split(entry, "/")
			if len(parts) == 3 {
				var id, parent int
				fmt.Sscanf(parts[1], "%d", &id)
				fmt.Sscanf(parts[2], "%d", &parent)
				items = append(items, reportedItem{Table: parts[0], ID: id, Parent: parent})
			}
		}
	}
	if len(items) == 0 {
		items = append(items, reportedItem{Table: fallbackTable, ID: fallbackID, Parent: fallbackParent})
	}
	return items
}

// parseDisciplineReasons parses JSON discipline reasons
func parseDisciplineReasons(reasons *string) []int {
	if reasons == nil || *reasons == "" {
		return nil
	}
	var result []int
	if err := json.Unmarshal([]byte(*reasons), &result); err != nil {
		// Try as string array and convert
		var strReasons []string
		if err := json.Unmarshal([]byte(*reasons), &strReasons); err != nil {
			return nil
		}
		for _, s := range strReasons {
			if code, ok := reasonKeyToInt[s]; ok {
				result = append(result, code)
			}
		}
	}
	return result
}

// lookupTargetMbID tries to find target member from the post
func lookupTargetMbID(db *gorm.DB, group *singoReportGroup) string {
	type postRow struct {
		MbID      string `gorm:"column:mb_id"`
		WrName    string `gorm:"column:wr_name"`
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
	}

	var post postRow
	tableName := fmt.Sprintf("g5_write_%s", group.SgTable)
	err := db.Table(tableName).
		Select("mb_id, wr_name, wr_subject, wr_content").
		Where("wr_id = ?", group.SgID).
		First(&post).Error

	if err != nil {
		// Fallback: truthroom
		err = db.Table("g5_write_truthroom").
			Select("mb_id, wr_name, wr_subject, wr_content").
			Where("wr_id = ?", group.SgID).
			First(&post).Error
		if err != nil {
			return "알수없음"
		}
	}

	if post.MbID != "" {
		if group.TargetTitle != nil && *group.TargetTitle == "" {
			*group.TargetTitle = post.WrSubject
		}
		return post.MbID
	}
	return post.WrName
}

// createDisciplineLogPost creates a discipline log post in g5_write_disciplinelog
func createDisciplineLogPost(
	tx *gorm.DB,
	targetMbID, targetNick string,
	sgTypes []int,
	disciplineDays int,
	disciplineType, disciplineDetail string,
	sgTable string, sgID, sgParent, reportCount int,
	items []reportedItem,
	isBulk bool,
	now time.Time,
) (int, error) {
	// 다음 wr_id 조회
	var maxWrID int
	tx.Raw("SELECT COALESCE(MAX(wr_id), 0) FROM g5_write_disciplinelog").Scan(&maxWrID)
	wrID := maxWrID + 1

	// penalty_type 변환
	penaltyType := convertDisciplineType(disciplineType)

	// 9999일은 영구제재로 변환
	penaltyPeriod := disciplineDays
	if disciplineDays == 9999 {
		penaltyPeriod = -1
	}

	// 신고당한 게시글 URL 생성
	reportedPath := fmt.Sprintf("/%s/", sgTable)
	if sgParent > 0 && sgParent != sgID {
		reportedPath += fmt.Sprintf("%d#c_%d", sgParent, sgID)
	} else {
		reportedPath += fmt.Sprintf("%d", sgID)
	}

	actualReportCount := reportCount
	if isBulk {
		actualReportCount = len(items)
	}

	// JSON 데이터 구성
	data := disciplineData{
		PenaltyMbID:     targetMbID,
		PenaltyDateFrom: now.Format("2006-01-02 15:04:05"),
		PenaltyPeriod:   penaltyPeriod,
		PenaltyType:     penaltyType,
		SgTypes:         sgTypes,
		Content:         stripTags(disciplineDetail),
		ReportedItems:   items,
		IsBulk:          isBulk,
		ReportedURL:     reportedPath,
		ReportedTable:   sgTable,
		ReportedID:      sgID,
		ReportedParent:  sgParent,
		ReportCount:     actualReportCount,
	}

	contentJSON, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}

	// wr_1에 저장할 사유 문자열
	wr1Value := buildReasonLabels(sgTypes)

	nowStr := now.Format("2006-01-02 15:04:05")
	subject := fmt.Sprintf("%s(%s)", targetMbID, targetNick)

	// INSERT
	if err := tx.Exec(`
		INSERT INTO g5_write_disciplinelog
		SET wr_id = ?,
			wr_num = (SELECT IFNULL(MIN(wr_num), 0) - 1 FROM g5_write_disciplinelog tmp),
			wr_reply = '',
			wr_parent = ?,
			wr_is_comment = 0,
			wr_comment = 0,
			wr_comment_reply = '',
			ca_name = '',
			wr_option = 'html1',
			wr_subject = ?,
			wr_content = ?,
			wr_link1 = ?,
			wr_link2 = '',
			wr_link1_hit = 0,
			wr_link2_hit = 0,
			wr_hit = 0,
			wr_good = 0,
			wr_nogood = 0,
			mb_id = 'police',
			wr_password = '',
			wr_name = 'police',
			wr_email = '',
			wr_homepage = '',
			wr_datetime = ?,
			wr_file = 0,
			wr_last = ?,
			wr_ip = '127.0.0.1',
			wr_1 = ?,
			wr_2 = '', wr_3 = '', wr_4 = '', wr_5 = '',
			wr_6 = '', wr_7 = '', wr_8 = '', wr_9 = '', wr_10 = ''
	`, wrID, wrID, subject, string(contentJSON),
		"https://damoang.net"+reportedPath,
		nowStr, nowStr, wr1Value,
	).Error; err != nil {
		return 0, err
	}

	// 게시판 글 수 증가
	tx.Exec("UPDATE g5_board SET bo_count_write = bo_count_write + 1 WHERE bo_table = 'disciplinelog'")

	return wrID, nil
}

// applyUserRestriction applies discipline to the target member
func applyUserRestriction(tx *gorm.DB, targetMbID, disciplineType string, disciplineDays int, sgTypes []int, now time.Time) error {
	// 주의 처분 (0일): 제재 없음
	if disciplineDays == 0 {
		return nil
	}

	// 현재 회원 정보 조회
	var member struct {
		MbLevel int `gorm:"column:mb_level"`
	}
	if err := tx.Table("g5_member").Select("mb_level").Where("mb_id = ?", targetMbID).First(&member).Error; err != nil {
		return fmt.Errorf("회원 조회 실패: %w", err)
	}

	// disciplineType에 따라 적절한 필드만 업데이트
	penaltyTypes := convertDisciplineType(disciplineType)

	// penalty_type 무관하게 disciplineDays > 0이면 항상 mb_intercept_date 설정
	if disciplineDays > 0 || disciplineDays == 9999 {
		var restrictionEndDate string
		if disciplineDays == 9999 {
			restrictionEndDate = "99991231"
		} else {
			restrictionEndDate = now.AddDate(0, 0, disciplineDays).Format("2006-01-02 15:04:05")
		}
		if err := tx.Table("g5_member").Where("mb_id = ?", targetMbID).
			Update("mb_intercept_date", restrictionEndDate).Error; err != nil {
			return err
		}
	}

	// g5_da_member_discipline 테이블에 제재 정보 저장 (올바른 penalty_type 사용)
	penaltyTypeValue := derivePenaltyTypeValue(penaltyTypes)

	penaltyPeriod := disciplineDays
	if disciplineDays == 9999 {
		penaltyPeriod = -1
	}

	sgTypesStr := ""
	for i, t := range sgTypes {
		if i > 0 {
			sgTypesStr += ","
		}
		sgTypesStr += fmt.Sprintf("%d", t)
	}

	// UPSERT: 기존 레코드가 있으면 UPDATE, 없으면 INSERT
	var existingID int
	tx.Raw("SELECT id FROM g5_da_member_discipline WHERE penalty_mb_id = ?", targetMbID).Scan(&existingID)

	if existingID > 0 {
		tx.Exec(`UPDATE g5_da_member_discipline SET
			penalty_date_from = ?, penalty_period = ?, penalty_type = ?, prev_level = ?, restriction_scope = ?
			WHERE penalty_mb_id = ?`,
			now.Format("2006-01-02 15:04:05"), penaltyPeriod, penaltyTypeValue, member.MbLevel, "all", targetMbID)
	} else {
		tx.Exec(`INSERT INTO g5_da_member_discipline
			(penalty_mb_id, penalty_date_from, penalty_period, penalty_type, sg_types, prev_level, restriction_scope)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			targetMbID, now.Format("2006-01-02 15:04:05"), penaltyPeriod, penaltyTypeValue, sgTypesStr, member.MbLevel, "all")
	}

	return nil
}

// sendDisciplineMemo sends a discipline notification memo to the target member
func sendDisciplineMemo(tx *gorm.DB, targetMbID, targetNick string, disciplineDays int, disciplineType string, sgTypes []int, disciplineDetail string, wrID int, now time.Time) error {
	// 템플릿 구성 (PHP 템플릿과 동일한 내용)
	memo := buildMemoContent(targetMbID, targetNick, disciplineDays, disciplineType, sgTypes, disciplineDetail, wrID, now)

	nowStr := now.Format("2006-01-02 15:04:05")

	// 1. 받는 회원 쪽지 INSERT (recv)
	result := tx.Exec(`
		INSERT INTO g5_memo
		(me_recv_mb_id, me_send_mb_id, me_send_datetime, me_memo, me_read_datetime, me_type, me_send_ip)
		VALUES (?, 'police', ?, ?, '0000-00-00 00:00:00', 'recv', '127.0.0.1')
	`, targetMbID, nowStr, memo)
	if result.Error != nil {
		return result.Error
	}

	// 마지막 INSERT ID 조회
	var meID int
	tx.Raw("SELECT LAST_INSERT_ID()").Scan(&meID)
	if meID == 0 {
		return nil
	}

	// 2. 보내는 회원 쪽지 INSERT (send)
	tx.Exec(`
		INSERT INTO g5_memo
		(me_recv_mb_id, me_send_mb_id, me_send_datetime, me_memo, me_read_datetime, me_send_id, me_type, me_send_ip)
		VALUES (?, 'police', ?, ?, '0000-00-00 00:00:00', ?, 'send', '127.0.0.1')
	`, targetMbID, nowStr, memo, meID)

	// 3. 실시간 쪽지 알림 업데이트
	tx.Exec(`
		UPDATE g5_member
		SET mb_memo_call = 'police',
			mb_memo_cnt = (SELECT COUNT(*) FROM g5_memo WHERE me_recv_mb_id = ? AND me_type = 'recv' AND me_read_datetime = '0000-00-00 00:00:00')
		WHERE mb_id = ?
	`, targetMbID, targetMbID)

	return nil
}

// buildMemoContent generates the discipline notification memo content
func buildMemoContent(targetMbID, targetNick string, disciplineDays int, disciplineType string, sgTypes []int, disciplineDetail string, wrID int, now time.Time) string {
	// 기간 텍스트
	var penaltyDay string
	if disciplineDays < 0 || disciplineDays == 9999 {
		penaltyDay = "영구"
	} else if disciplineDays == 0 {
		penaltyDay = "주의(이용제한 없음)"
	} else {
		penaltyDay = fmt.Sprintf("%d일", disciplineDays)
	}

	// 종료일
	var endDateStr string
	if disciplineDays > 0 && disciplineDays < 9999 {
		endDateStr = " ~ " + now.AddDate(0, 0, disciplineDays).Format("2006-01-02 15:04:05")
	} else if disciplineDays < 0 || disciplineDays == 9999 {
		endDateStr = " ~"
	}

	// 사유 목록
	reasonList := ""
	idx := 1
	for _, t := range sgTypes {
		label := getReportTypeLabel(t)
		if label != "알 수 없음" {
			reasonList += fmt.Sprintf("%d. %s\n", idx, label)
			idx++
		}
	}

	disciplineLink := fmt.Sprintf("https://damoang.net/disciplinelog/%d", wrID)
	profileLink := fmt.Sprintf("https://damoang.net/disciplinelog?bo_table=disciplinelog&sca=&sfl=wr_subject%%7C%%7Cwr_content%%2C1&sop=and&stx=%s", targetMbID)

	// 추가정보
	additionalInfo := ""
	detail := stripTags(disciplineDetail)
	if detail != "" {
		additionalInfo = "\n• 추가정보:\n" + detail
	}

	memo := fmt.Sprintf(`💌 [잠시 쉬어가기 안내] 💌


안녕하세요, %s님! 👋

잠깐! 우리 %s님께서
조금 쉬어가실 시간이 필요하신 것 같아요 🍀

다모앙 가족 모두가 행복한 공간을 만들기 위해
잠시만 충전의 시간을 가져보시는 건 어떨까요?

곧 다시 만나요! 🌈

📝 쉬어가기 상세 내용
• 내 기록 확인: %s

━━━━━━━━━━━━━━━━━━━━━━━━━━
📚 도움이 될 만한 페이지
• 이용약관: https://damoang.net/content/provision
• 운영정책: https://damoang.net/content/operation_policy
• 제재사유 안내: https://damoang.net/content/operation_policy_add
• 내 기록 확인: %s
━━━━━━━━━━━━━━━━━━━━━━━━━━
💡 잠시만 기다려주세요!
   이 기간 동안은 글쓰기, 댓글, 쪽지 기능이
   잠시 쉬어갑니다 😊

🌟 함께 더 좋은 커뮤니티를 만들어가요!
   서로를 배려하는 마음, 그것이 다모앙의 힘입니다 💪`,
		targetNick, targetNick, disciplineLink, profileLink)

	// 실제 PHP 템플릿과의 호환성 유지 (사용하지 않는 플레이스홀더도 포함)
	_ = penaltyDay
	_ = endDateStr
	_ = reasonList
	_ = additionalInfo

	return memo
}

// processBulkReports processes additional reports embedded in BULK_REPORTS
func processBulkReports(tx *gorm.DB, disciplineDetail string, wrID int) {
	// [BULK_REPORTS:...] 패턴 찾기
	idx := strings.Index(disciplineDetail, "[BULK_REPORTS:")
	if idx == -1 {
		return
	}
	endIdx := strings.Index(disciplineDetail[idx:], "]")
	if endIdx == -1 {
		return
	}

	jsonStr := disciplineDetail[idx+14 : idx+endIdx]
	var bulkReports []struct {
		SgTable  string `json:"sg_table"`
		SgID     int    `json:"sg_id"`
		SgParent int    `json:"sg_parent"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &bulkReports); err != nil {
		return
	}

	for _, br := range bulkReports {
		if br.SgTable == "" || br.SgID <= 0 {
			continue
		}
		tx.Exec(`
			UPDATE g5_na_singo
			SET processed = 1, processed_datetime = NOW(), discipline_log_id = ?, version = version + 1
			WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND admin_approved = 1 AND processed = 0
		`, wrID, br.SgTable, br.SgID, br.SgParent)
	}
}

// convertDisciplineType converts discipline type string to penalty type array
func convertDisciplineType(disciplineType string) []string {
	switch disciplineType {
	case "level_down", "level":
		return []string{"level"}
	case "access_block", "access":
		return []string{"access"}
	case "both", "demotion_and_block":
		return []string{"level", "access"}
	default:
		return []string{}
	}
}

// derivePenaltyTypeValue converts penalty type slice to DB value string
func derivePenaltyTypeValue(penaltyTypes []string) string {
	hasLevel := false
	hasAccess := false
	for _, pt := range penaltyTypes {
		if pt == "level" {
			hasLevel = true
		}
		if pt == "access" {
			hasAccess = true
		}
	}
	if hasLevel && hasAccess {
		return "both"
	}
	if hasLevel {
		return "level"
	}
	if hasAccess {
		return "intercept"
	}
	return ""
}

// stripTags removes HTML tags from a string (simple implementation)
func stripTags(s string) string {
	result := strings.Builder{}
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}

// buildReasonLabels builds comma-separated reason labels from type codes
func buildReasonLabels(sgTypes []int) string {
	var labels []string
	for _, t := range sgTypes {
		label := getReportTypeLabel(t)
		if label != "알 수 없음" {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ", ")
}

// getReportTypeLabel returns the Korean label for a report type code
func getReportTypeLabel(code int) string {
	if label, ok := reportTypeLabels[code]; ok {
		return label
	}
	return "알 수 없음"
}

// reportTypeLabels maps report type codes to Korean labels
var reportTypeLabels = map[int]string{
	1: "회원비하", 2: "예의없음", 3: "부적절한 표현", 4: "차별행위",
	5: "분란유도/갈등조장", 6: "여론조성", 7: "회원기만", 8: "이용방해",
	9: "용도위반", 10: "거래금지위반", 11: "구걸", 12: "권리침해",
	13: "외설", 14: "위법행위", 15: "광고/홍보", 16: "운영정책부정",
	17: "다중이", 18: "기타사유",
	21: "회원비하", 22: "예의없음", 23: "부적절한 표현", 24: "차별행위",
	25: "분란유도/갈등조장", 26: "여론조성", 27: "회원기만", 28: "이용방해",
	29: "용도위반", 30: "거래금지위반", 31: "구걸", 32: "권리침해",
	33: "외설", 34: "위법행위", 35: "광고/홍보", 36: "운영정책부정",
	37: "다중이", 38: "기타사유", 39: "뉴스펌글누락", 40: "뉴스전문전재",
}

// getReportLockThreshold reads the auto-lock threshold.
// Priority: g5_kv_store → REPORT_LOCK_THRESHOLD env → 0 (disabled)
func getReportLockThreshold(db *gorm.DB) int {
	// 1. g5_kv_store에서 조회 (value_type='INT' → value_int, 'TEXT' → value_text)
	var result struct {
		ValueType string `gorm:"column:value_type"`
		ValueText string `gorm:"column:value_text"`
		ValueInt  int    `gorm:"column:value_int"`
	}
	err := db.Raw("SELECT value_type, value_text, value_int FROM g5_kv_store WHERE `key` = 'system:report_lock_threshold' LIMIT 1").Scan(&result).Error
	if err == nil {
		if result.ValueType == "INT" && result.ValueInt > 0 {
			return result.ValueInt
		}
		if result.ValueText != "" {
			var threshold int
			if _, parseErr := fmt.Sscanf(result.ValueText, "%d", &threshold); parseErr == nil && threshold > 0 {
				return threshold
			}
		}
	}

	// 2. 환경변수
	if envVal := os.Getenv("REPORT_LOCK_THRESHOLD"); envVal != "" {
		var threshold int
		if _, parseErr := fmt.Sscanf(envVal, "%d", &threshold); parseErr == nil && threshold > 0 {
			return threshold
		}
	}

	// 3. 기본값: 0 (비활성)
	return 0
}

// autoLockPost checks approved report count and locks post if threshold reached
func autoLockPost(tx *gorm.DB, boTable string, postID, threshold int) {
	tableName := fmt.Sprintf("g5_write_%s", boTable)

	// 이미 잠금 상태인지 확인
	var currentWr7 string
	if err := tx.Raw(fmt.Sprintf("SELECT IFNULL(wr_7, '') FROM `%s` WHERE wr_id = ? AND wr_is_comment = 0", tableName), postID).Scan(&currentWr7).Error; err != nil {
		return
	}
	if currentWr7 == "lock" {
		return
	}

	// 승인된 고유 신고자 수 카운트 (해당 게시글 대상)
	var approvedReporterCount int64
	tx.Raw(`
		SELECT COUNT(DISTINCT mb_id) FROM g5_na_singo
		WHERE sg_table = ? AND (sg_id = ? OR sg_parent = ?) AND admin_approved = 1
	`, boTable, postID, postID).Scan(&approvedReporterCount)

	if common.SafeInt64ToInt(approvedReporterCount) < threshold {
		return
	}

	// 잠금 적용
	tx.Exec(fmt.Sprintf("UPDATE `%s` SET wr_7 = 'lock' WHERE wr_id = ? AND wr_is_comment = 0", tableName), postID)
	log.Printf("[Cron:auto-lock] locked post %s/%d (approved reporters: %d >= threshold: %d)", boTable, postID, approvedReporterCount, threshold)

	// 진실의방에 참조 글 생성
	createTruthroomPost(tx, boTable, postID)
}

// createTruthroomPost creates a reference post in g5_write_truthroom
func createTruthroomPost(tx *gorm.DB, boTable string, postID int) {
	// 중복 체크: wr_1 = boTable AND wr_2 = postID 기존 글 있으면 skip
	var existingCount int64
	tx.Raw(`
		SELECT COUNT(*) FROM g5_write_truthroom
		WHERE wr_1 = ? AND wr_2 = ? AND wr_is_comment = 0
	`, boTable, fmt.Sprintf("%d", postID)).Scan(&existingCount)
	if existingCount > 0 {
		return
	}

	// 원본 글 제목/작성자 조회
	tableName := fmt.Sprintf("g5_write_%s", boTable)
	var original struct {
		WrSubject string `gorm:"column:wr_subject"`
		MbID      string `gorm:"column:mb_id"`
		WrName    string `gorm:"column:wr_name"`
	}
	if err := tx.Table(tableName).Select("wr_subject, mb_id, wr_name").Where("wr_id = ? AND wr_is_comment = 0", postID).First(&original).Error; err != nil {
		log.Printf("[Cron:auto-lock] failed to fetch original post %s/%d: %v", boTable, postID, err)
		return
	}

	authorID := original.MbID
	authorName := original.WrName
	if authorName == "" {
		authorName = original.MbID
	}

	// 원본 내용 미리보기 (200자)
	var originalContent struct {
		WrContent string `gorm:"column:wr_content"`
	}
	tx.Table(tableName).Select("wr_content").Where("wr_id = ? AND wr_is_comment = 0", postID).First(&originalContent)
	preview := stripTags(originalContent.WrContent)
	if len([]rune(preview)) > 200 {
		preview = string([]rune(preview)[:200]) + "…"
	}

	// 다음 wr_id 조회
	var maxWrID int
	tx.Raw("SELECT COALESCE(MAX(wr_id), 0) FROM g5_write_truthroom").Scan(&maxWrID)
	wrID := maxWrID + 1

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	subject := fmt.Sprintf("[신고잠금] %s", original.WrSubject)
	postLink := fmt.Sprintf("https://damoang.net/%s/%d", boTable, postID)
	content := fmt.Sprintf(`<div class="truthroom-preview"><p class="preview-text">%s</p><p class="preview-source">출처: <a href="/%s/%d">%s #%d</a></p></div>`, preview, boTable, postID, boTable, postID)

	if err := tx.Exec(`
		INSERT INTO g5_write_truthroom
		SET wr_id = ?,
			wr_num = (SELECT IFNULL(MIN(wr_num), 0) - 1 FROM g5_write_truthroom tmp),
			wr_reply = '',
			wr_parent = ?,
			wr_is_comment = 0,
			wr_comment = 0,
			wr_comment_reply = '',
			ca_name = '게시글',
			wr_option = 'html1',
			wr_subject = ?,
			wr_content = ?,
			wr_link1 = ?,
			wr_link2 = '',
			wr_link1_hit = 0,
			wr_link2_hit = 0,
			wr_hit = 0,
			wr_good = 0,
			wr_nogood = 0,
			mb_id = ?,
			wr_password = '',
			wr_name = ?,
			wr_email = '',
			wr_homepage = '',
			wr_datetime = ?,
			wr_file = 0,
			wr_last = ?,
			wr_ip = '127.0.0.1',
			wr_1 = ?,
			wr_2 = ?,
			wr_3 = '', wr_4 = '', wr_5 = '',
			wr_6 = '', wr_7 = '', wr_8 = '', wr_9 = '', wr_10 = ''
	`, wrID, wrID, subject, content, postLink,
		authorID, authorName,
		nowStr, nowStr, boTable, fmt.Sprintf("%d", postID),
	).Error; err != nil {
		log.Printf("[Cron:auto-lock] failed to create truthroom post for %s/%d: %v", boTable, postID, err)
		return
	}

	// 게시판 글 수 증가
	tx.Exec("UPDATE g5_board SET bo_count_write = bo_count_write + 1 WHERE bo_table = 'truthroom'")
	log.Printf("[Cron:auto-lock] created truthroom post #%d for %s/%d", wrID, boTable, postID)
}

// autoLockComment checks approved report count for a comment and locks it if threshold reached
func autoLockComment(tx *gorm.DB, boTable string, commentID, parentID, threshold int) {
	tableName := fmt.Sprintf("g5_write_%s", boTable)

	// 이미 잠금 상태인지 확인
	var currentWr7 string
	if err := tx.Raw(fmt.Sprintf("SELECT IFNULL(wr_7, '') FROM `%s` WHERE wr_id = ? AND wr_is_comment = 1", tableName), commentID).Scan(&currentWr7).Error; err != nil {
		return
	}
	if currentWr7 == "lock" {
		return
	}

	// 승인된 고유 신고자 수 카운트 (해당 댓글 대상)
	var approvedReporterCount int64
	tx.Raw(`
		SELECT COUNT(DISTINCT mb_id) FROM g5_na_singo
		WHERE sg_table = ? AND sg_id = ? AND sg_parent = ? AND admin_approved = 1
	`, boTable, commentID, parentID).Scan(&approvedReporterCount)

	if common.SafeInt64ToInt(approvedReporterCount) < threshold {
		return
	}

	// 잠금 적용
	tx.Exec(fmt.Sprintf("UPDATE `%s` SET wr_7 = 'lock' WHERE wr_id = ? AND wr_is_comment = 1", tableName), commentID)
	log.Printf("[Cron:auto-lock] locked comment %s/%d (parent: %d, approved reporters: %d >= threshold: %d)", boTable, commentID, parentID, approvedReporterCount, threshold)

	// 진실의방에 참조 글 생성
	createTruthroomCommentPost(tx, boTable, commentID, parentID)
}

// createTruthroomCommentPost creates a reference post in g5_write_truthroom for a locked comment
func createTruthroomCommentPost(tx *gorm.DB, boTable string, commentID, parentID int) {
	// 중복 체크: wr_1 = boTable AND wr_2 = parentID AND wr_3 = commentID 기존 글 있으면 skip
	var existingCount int64
	tx.Raw(`
		SELECT COUNT(*) FROM g5_write_truthroom
		WHERE wr_1 = ? AND wr_2 = ? AND wr_3 = ? AND wr_is_comment = 0
	`, boTable, fmt.Sprintf("%d", parentID), fmt.Sprintf("%d", commentID)).Scan(&existingCount)
	if existingCount > 0 {
		return
	}

	// 원본 댓글 내용/작성자 조회
	tableName := fmt.Sprintf("g5_write_%s", boTable)
	var original struct {
		WrContent string `gorm:"column:wr_content"`
		MbID      string `gorm:"column:mb_id"`
		WrName    string `gorm:"column:wr_name"`
	}
	if err := tx.Table(tableName).Select("wr_content, mb_id, wr_name").Where("wr_id = ? AND wr_is_comment = 1", commentID).First(&original).Error; err != nil {
		log.Printf("[Cron:auto-lock] failed to fetch original comment %s/%d: %v", boTable, commentID, err)
		return
	}

	authorID := original.MbID
	authorName := original.WrName
	if authorName == "" {
		authorName = original.MbID
	}

	// 댓글 내용 미리보기 (200자)
	preview := stripTags(original.WrContent)
	if len([]rune(preview)) > 200 {
		preview = string([]rune(preview)[:200]) + "…"
	}

	// 제목용 미리보기 (80자)
	titlePreview := preview
	if len([]rune(titlePreview)) > 80 {
		titlePreview = string([]rune(titlePreview)[:80]) + "…"
	}

	// 다음 wr_id 조회
	var maxWrID int
	tx.Raw("SELECT COALESCE(MAX(wr_id), 0) FROM g5_write_truthroom").Scan(&maxWrID)
	wrID := maxWrID + 1

	nowStr := time.Now().Format("2006-01-02 15:04:05")
	subject := fmt.Sprintf("[신고잠금:댓글] %s", titlePreview)
	commentLink := fmt.Sprintf("https://damoang.net/%s/%d#c_%d", boTable, parentID, commentID)
	content := fmt.Sprintf(`<div class="truthroom-preview"><p class="preview-text">%s</p><p class="preview-source">출처: <a href="/%s/%d#c_%d">%s #%d</a></p></div>`, preview, boTable, parentID, commentID, boTable, parentID)

	if err := tx.Exec(`
		INSERT INTO g5_write_truthroom
		SET wr_id = ?,
			wr_num = (SELECT IFNULL(MIN(wr_num), 0) - 1 FROM g5_write_truthroom tmp),
			wr_reply = '',
			wr_parent = ?,
			wr_is_comment = 0,
			wr_comment = 0,
			wr_comment_reply = '',
			ca_name = '댓글',
			wr_option = 'html1',
			wr_subject = ?,
			wr_content = ?,
			wr_link1 = ?,
			wr_link2 = '',
			wr_link1_hit = 0,
			wr_link2_hit = 0,
			wr_hit = 0,
			wr_good = 0,
			wr_nogood = 0,
			mb_id = ?,
			wr_password = '',
			wr_name = ?,
			wr_email = '',
			wr_homepage = '',
			wr_datetime = ?,
			wr_file = 0,
			wr_last = ?,
			wr_ip = '127.0.0.1',
			wr_1 = ?,
			wr_2 = ?,
			wr_3 = ?, wr_4 = '', wr_5 = '',
			wr_6 = '', wr_7 = '', wr_8 = '', wr_9 = '', wr_10 = ''
	`, wrID, wrID, subject, content, commentLink,
		authorID, authorName,
		nowStr, nowStr, boTable, fmt.Sprintf("%d", parentID), fmt.Sprintf("%d", commentID),
	).Error; err != nil {
		log.Printf("[Cron:auto-lock] failed to create truthroom comment post for %s/%d: %v", boTable, commentID, err)
		return
	}

	// 게시판 글 수 증가
	tx.Exec("UPDATE g5_board SET bo_count_write = bo_count_write + 1 WHERE bo_table = 'truthroom'")
	log.Printf("[Cron:auto-lock] created truthroom comment post #%d for %s/%d (parent: %d)", wrID, boTable, commentID, parentID)
}

// reasonKeyToInt maps string reason keys to integer codes
var reasonKeyToInt = map[string]int{
	"member_disparage":     1,
	"no_manner":            2,
	"inappropriate_expr":   3,
	"discrimination":       4,
	"provocation":          5,
	"opinion_manipulation": 6,
	"member_deception":     7,
	"usage_obstruction":    8,
	"purpose_violation":    9,
	"trade_violation":      10,
	"begging":              11,
	"rights_infringement":  12,
	"obscenity":            13,
	"illegal_activity":     14,
	"advertising":          15,
	"policy_denial":        16,
	"multi_account":        17,
	"other":                18,
}

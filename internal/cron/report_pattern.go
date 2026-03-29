package cron

import (
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ReportPatternResult contains the result of report pattern analysis
type ReportPatternResult struct {
	Subject    string `json:"subject"`
	DateFrom   string `json:"date_from"`
	DateTo     string `json:"date_to"`
	Reports    int    `json:"total_reports"`
	Reporters  int    `json:"unique_reporters"`
	ExecutedAt string `json:"executed_at"`
}

// reportStats holds all collected statistics
type reportStats struct {
	TotalReports     int                    `json:"total_reports"`
	CompletedReports int                    `json:"completed_reports"`
	ClaimReports     int                    `json:"claim_reports"`
	ReportCount      int                    `json:"report_count"`
	ReportMonth      int                    `json:"report_month"`
	ReporterCount    int                    `json:"reporter_count"`
	TotalCases       int                    `json:"total_cases"`
	TotalMonthCases  int                    `json:"total_month_cases"`
	ReportTypes      map[string]int         `json:"report_types"`
	BoardStats       []boardStat            `json:"board_stats"`
	DailyStats       map[string]dailyStat   `json:"daily_stats"`
	WeeklyStats      map[string]weeklyStat  `json:"weekly_stats"`
	TopReporters     []topReporter          `json:"top_reporters"`
	PatternAnalysis  []patternItem          `json:"pattern_analysis"`
	PatternSummary   map[string]interface{} `json:"pattern_summary"`
	DateFrom         string                 `json:"date_from"`
	DateTo           string                 `json:"date_to"`
	PeriodDays       int                    `json:"period_days"`
	GeneratedAt      string                 `json:"generated_at"`
}

type boardStat struct {
	Name  string `json:"name" gorm:"column:board_name"`
	Count int    `json:"count" gorm:"column:board_count"`
}

type dailyStat struct {
	Reports  int `json:"reports"`
	Posts    int `json:"posts"`
	Comments int `json:"comments"`
}

type weeklyStat struct {
	Reports   int `json:"reports"`
	Processed int `json:"processed"`
	Posts     int `json:"posts"`
	Comments  int `json:"comments"`
}

type topReporter struct {
	MbID        string `json:"mb_id" gorm:"column:mb_id"`
	MbNick      string `json:"mb_nick" gorm:"column:mb_nick"`
	MbName      string `json:"mb_name" gorm:"column:mb_name"`
	ReportCount int    `json:"report_count" gorm:"column:report_count"`
}

type patternItem struct {
	SgParent      int    `json:"sg_parent" gorm:"column:sg_parent"`
	SgTable       string `json:"sg_table" gorm:"column:sg_table"`
	BoardName     string `json:"board_name" gorm:"column:board_name"`
	ReportCount   int    `json:"report_count" gorm:"column:report_count"`
	ReporterCount int    `json:"reporter_count" gorm:"column:reporter_count"`
}

const singoTypeCondition = "sg_type IN (1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,17,18,21,22,23,24,25,26,27,28,29,30,31,32,33,34,35,36,37,38,39,40)"

// runUpdateReportPattern generates a weekly report pattern analysis
func runUpdateReportPattern(db *gorm.DB) (*ReportPatternResult, error) {
	return runUpdateReportPatternAt(db, time.Now())
}

// runUpdateReportPatternAt generates a report using the given reference time
func runUpdateReportPatternAt(db *gorm.DB, now time.Time) (*ReportPatternResult, error) {

	// 지난 주 일요일~토요일 계산
	startDate, endDate := getLastWeekRange(now)

	// 중복 체크
	startFormatted := formatKoreanDate(startDate)
	endFormatted := formatKoreanDate(endDate)
	subject := fmt.Sprintf("운영 현황 보고서 (%s ~ %s)", startFormatted, endFormatted)

	var dupCount int64
	db.Raw("SELECT COUNT(*) FROM g5_write_report WHERE wr_subject = ?", subject).Scan(&dupCount)
	if dupCount > 0 {
		return &ReportPatternResult{
			Subject:    subject,
			DateFrom:   startDate,
			DateTo:     endDate,
			ExecutedAt: now.Format("2006-01-02 15:04:05"),
		}, fmt.Errorf("해당 기간의 보고서가 이미 존재합니다")
	}

	// 통계 수집
	stats := collectStats(db, startDate, endDate, now)

	// 보고서 저장
	if err := saveReportPost(db, stats, subject, now); err != nil {
		return nil, fmt.Errorf("보고서 저장 실패: %w", err)
	}

	return &ReportPatternResult{
		Subject:    subject,
		DateFrom:   startDate,
		DateTo:     endDate,
		Reports:    stats.TotalReports,
		Reporters:  stats.ReporterCount,
		ExecutedAt: now.Format("2006-01-02 15:04:05"),
	}, nil
}

// getLastWeekRange calculates last week's Sunday~Saturday range
func getLastWeekRange(now time.Time) (string, string) {
	yesterday := now.AddDate(0, 0, -1)
	daysSinceSunday := int(yesterday.Weekday()) // 0=Sunday
	lastSunday := yesterday.AddDate(0, 0, -daysSinceSunday)
	lastSaturday := lastSunday.AddDate(0, 0, 6)

	return lastSunday.Format("2006-01-02"), lastSaturday.Format("2006-01-02")
}

// formatKoreanDate formats "2006-01-02" to "2006년 01월 02일"
func formatKoreanDate(dateStr string) string {
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return dateStr
	}
	return fmt.Sprintf("%d년 %02d월 %02d일", t.Year(), t.Month(), t.Day())
}

// collectStats gathers all statistics for the report
func collectStats(db *gorm.DB, startDate, endDate string, now time.Time) *reportStats {
	startDT := startDate + " 00:00:00"
	endDT := endDate + " 23:59:59"

	stats := &reportStats{
		DateFrom:    startDate,
		DateTo:      endDate,
		GeneratedAt: now.Format("2006-01-02 15:04:05"),
	}

	// 기간 일수 계산
	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)
	stats.PeriodDays = int(end.Sub(start).Hours()/24) + 1

	// 1. 기본 통계
	var basicStats struct {
		TotalReports    int `gorm:"column:total_reports"`
		UniqueReporters int `gorm:"column:unique_reporters"`
		PendingReports  int `gorm:"column:pending_reports"`
	}
	db.Raw(fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT CONCAT(sg_table, '_', sg_id, '_', mb_id)) as total_reports,
			COUNT(DISTINCT mb_id) as unique_reporters,
			SUM(CASE WHEN sg_flag = 0 THEN 1 ELSE 0 END) as pending_reports
		FROM g5_na_singo
		WHERE (%s)
		AND sg_time >= ? AND sg_time <= ?
	`, singoTypeCondition), startDT, endDT).Scan(&basicStats)

	stats.TotalReports = basicStats.TotalReports
	stats.ReporterCount = basicStats.UniqueReporters

	// 2. 신고된 글/댓글 수
	var uniqueCounts struct {
		UniquePosts    int `gorm:"column:unique_posts"`
		UniqueComments int `gorm:"column:unique_comments"`
	}
	db.Raw(fmt.Sprintf(`
		SELECT
			COUNT(DISTINCT CASE WHEN (sg_parent = 0 OR sg_parent = sg_id) THEN CONCAT(sg_table, '_', sg_id, '_', mb_id) END) as unique_posts,
			COUNT(DISTINCT CASE WHEN (sg_parent != 0 AND sg_parent != sg_id) THEN CONCAT(sg_table, '_', sg_id, '_', mb_id) END) as unique_comments
		FROM g5_na_singo
		WHERE (%s)
		AND sg_time >= ? AND sg_time <= ?
	`, singoTypeCondition), startDT, endDT).Scan(&uniqueCounts)

	stats.ReportCount = uniqueCounts.UniquePosts
	stats.ReportMonth = uniqueCounts.UniqueComments

	// 3. 처리완료/소명 건수
	db.Raw("SELECT COUNT(*) FROM g5_write_disciplinelog WHERE wr_is_comment = 0 AND wr_datetime >= ? AND wr_datetime <= ?",
		startDT, endDT).Scan(&stats.CompletedReports)
	db.Raw("SELECT COUNT(*) FROM g5_write_claim WHERE wr_is_comment = 0 AND wr_datetime >= ? AND wr_datetime <= ?",
		startDT, endDT).Scan(&stats.ClaimReports)

	// 4. 전체 게시글/댓글 수
	var postCounts struct {
		TotalPosts    int `gorm:"column:total_posts"`
		TotalComments int `gorm:"column:total_comments"`
	}
	db.Raw(`
		SELECT
			COALESCE(SUM(IF(wr_id=wr_parent,1,0)), 0) as total_posts,
			COALESCE(SUM(IF(wr_id=wr_parent,0,1)), 0) as total_comments
		FROM g5_board_new
		WHERE bn_datetime >= ? AND bn_datetime <= ?
	`, startDT, endDT).Scan(&postCounts)

	stats.TotalCases = postCounts.TotalPosts
	stats.TotalMonthCases = postCounts.TotalComments

	// 5. 신고 유형별 통계 (상위 10)
	stats.ReportTypes = make(map[string]int)
	type typeRow struct {
		SgType    int `gorm:"column:sg_type"`
		TypeCount int `gorm:"column:type_count"`
	}
	var typeRows []typeRow
	db.Raw(fmt.Sprintf(`
		SELECT sg_type, COUNT(*) as type_count
		FROM (
			SELECT DISTINCT sg_type, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo
			WHERE (%s)
			AND sg_time >= ? AND sg_time <= ?
		) as unique_type_reports
		GROUP BY sg_type
		ORDER BY type_count DESC
		LIMIT 10
	`, singoTypeCondition), startDT, endDT).Scan(&typeRows)

	singoTypes := getSingoTypes()
	for _, row := range typeRows {
		label := "기타"
		if l, ok := singoTypes[row.SgType]; ok {
			label = l
		}
		stats.ReportTypes[label] = row.TypeCount
	}

	// 6. 게시판별 통계 (상위 10)
	db.Raw(fmt.Sprintf(`
		SELECT
			COUNT(*) as board_count,
			COALESCE(b.bo_subject, unique_board_reports.sg_table) as board_name
		FROM (
			SELECT DISTINCT sg_table, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo
			WHERE (%s)
			AND sg_time >= ? AND sg_time <= ?
		) as unique_board_reports
		LEFT JOIN g5_board b ON unique_board_reports.sg_table = b.bo_table
		GROUP BY unique_board_reports.sg_table
		ORDER BY board_count DESC
		LIMIT 10
	`, singoTypeCondition), startDT, endDT).Scan(&stats.BoardStats)

	// 7. 일별 통계
	stats.DailyStats = make(map[string]dailyStat)
	current := start
	for !current.After(end) {
		stats.DailyStats[current.Format("2006-01-02")] = dailyStat{}
		current = current.AddDate(0, 0, 1)
	}

	// 일별 신고 데이터
	type dailyReportRow struct {
		ReportDate   string `gorm:"column:report_date"`
		DailyReports int    `gorm:"column:daily_reports"`
	}
	var dailyReports []dailyReportRow
	db.Raw(fmt.Sprintf(`
		SELECT report_date, COUNT(*) as daily_reports
		FROM (
			SELECT DISTINCT CAST(DATE(sg_time) AS CHAR) as report_date, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo
			WHERE (%s)
			AND sg_time >= ? AND sg_time <= ?
		) as unique_daily_reports
		GROUP BY report_date
	`, singoTypeCondition), startDT, endDT).Scan(&dailyReports)

	for _, row := range dailyReports {
		if ds, ok := stats.DailyStats[row.ReportDate]; ok {
			ds.Reports = row.DailyReports
			stats.DailyStats[row.ReportDate] = ds
		}
	}

	// 일별 게시글/댓글 데이터
	type dailyPostRow struct {
		PostDate      string `gorm:"column:post_date"`
		DailyPosts    int    `gorm:"column:daily_posts"`
		DailyComments int    `gorm:"column:daily_comments"`
	}
	var dailyPosts []dailyPostRow
	db.Raw(`
		SELECT CAST(DATE(bn_datetime) AS CHAR) as post_date,
			SUM(CASE WHEN wr_id=wr_parent THEN 1 ELSE 0 END) as daily_posts,
			SUM(CASE WHEN wr_id!=wr_parent THEN 1 ELSE 0 END) as daily_comments
		FROM g5_board_new
		WHERE bn_datetime >= ? AND bn_datetime <= ?
		GROUP BY DATE(bn_datetime)
	`, startDT, endDT).Scan(&dailyPosts)

	for _, row := range dailyPosts {
		if ds, ok := stats.DailyStats[row.PostDate]; ok {
			ds.Posts = row.DailyPosts
			ds.Comments = row.DailyComments
			stats.DailyStats[row.PostDate] = ds
		}
	}

	// 8. 주간 통계 (요일별)
	dayNames := map[int]string{1: "일", 2: "월", 3: "화", 4: "수", 5: "목", 6: "금", 7: "토"}
	stats.WeeklyStats = make(map[string]weeklyStat)
	for _, name := range dayNames {
		stats.WeeklyStats[name] = weeklyStat{}
	}

	type weeklyRow struct {
		DayOfWeek int `gorm:"column:day_of_week"`
		Count     int `gorm:"column:count"`
	}
	var weeklyReports []weeklyRow
	db.Raw(fmt.Sprintf(`
		SELECT day_of_week, COUNT(*) as count
		FROM (
			SELECT DISTINCT DAYOFWEEK(sg_time) as day_of_week, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo
			WHERE (%s)
			AND sg_time >= ? AND sg_time <= ?
		) as unique_weekly_reports
		GROUP BY day_of_week
	`, singoTypeCondition), startDT, endDT).Scan(&weeklyReports)

	for _, row := range weeklyReports {
		if name, ok := dayNames[row.DayOfWeek]; ok {
			ws := stats.WeeklyStats[name]
			ws.Reports = row.Count
			stats.WeeklyStats[name] = ws
		}
	}

	// 요일별 처리 통계
	var weeklyProcessed []weeklyRow
	db.Raw(`
		SELECT DAYOFWEEK(wr_datetime) as day_of_week, COUNT(*) as count
		FROM g5_write_disciplinelog
		WHERE wr_is_comment = 0 AND wr_datetime >= ? AND wr_datetime <= ?
		GROUP BY DAYOFWEEK(wr_datetime)
	`, startDT, endDT).Scan(&weeklyProcessed)

	for _, row := range weeklyProcessed {
		if name, ok := dayNames[row.DayOfWeek]; ok {
			ws := stats.WeeklyStats[name]
			ws.Processed = row.Count
			stats.WeeklyStats[name] = ws
		}
	}

	// 요일별 글/댓글 통계
	type weeklyPostRow struct {
		DayOfWeek     int `gorm:"column:day_of_week"`
		PostsCount    int `gorm:"column:posts_count"`
		CommentsCount int `gorm:"column:comments_count"`
	}
	var weeklyPosts []weeklyPostRow
	db.Raw(`
		SELECT DAYOFWEEK(bn_datetime) as day_of_week,
			SUM(CASE WHEN wr_id=wr_parent THEN 1 ELSE 0 END) as posts_count,
			SUM(CASE WHEN wr_id!=wr_parent THEN 1 ELSE 0 END) as comments_count
		FROM g5_board_new
		WHERE bn_datetime >= ? AND bn_datetime <= ?
		GROUP BY DAYOFWEEK(bn_datetime)
	`, startDT, endDT).Scan(&weeklyPosts)

	for _, row := range weeklyPosts {
		if name, ok := dayNames[row.DayOfWeek]; ok {
			ws := stats.WeeklyStats[name]
			ws.Posts = row.PostsCount
			ws.Comments = row.CommentsCount
			stats.WeeklyStats[name] = ws
		}
	}

	// 9. 상위 신고자 통계 (상위 50)
	db.Raw(fmt.Sprintf(`
		SELECT unique_reports.mb_id, m.mb_nick, COALESCE(m.mb_name, '') as mb_name, COUNT(*) as report_count
		FROM (
			SELECT DISTINCT mb_id, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo s
			WHERE (%s)
			AND s.sg_time >= ? AND s.sg_time <= ?
			AND s.mb_id IS NOT NULL AND s.mb_id != ''
		) as unique_reports
		LEFT JOIN g5_member m ON unique_reports.mb_id = m.mb_id
		GROUP BY unique_reports.mb_id
		ORDER BY report_count DESC
		LIMIT 50
	`, singoTypeCondition), startDT, endDT).Scan(&stats.TopReporters)

	// top_reporters 상위 10명만 최종 저장
	if len(stats.TopReporters) > 10 {
		stats.TopReporters = stats.TopReporters[:10]
	}

	// 10. 신고 패턴 분석 (집중 신고 게시물)
	db.Raw(fmt.Sprintf(`
		SELECT sg_parent, sg_table,
			COUNT(*) as report_count,
			COUNT(DISTINCT mb_id) as reporter_count,
			COALESCE(b.bo_subject, sg_table) as board_name
		FROM (
			SELECT DISTINCT sg_parent, sg_table, mb_id, CONCAT(sg_table, '_', sg_id, '_', mb_id) as unique_key
			FROM g5_na_singo s
			WHERE (%s)
			AND s.sg_time >= ? AND s.sg_time <= ?
			AND s.sg_parent IS NOT NULL
		) as unique_pattern_reports
		LEFT JOIN g5_board b ON unique_pattern_reports.sg_table = b.bo_table
		GROUP BY sg_parent, sg_table
		HAVING report_count >= 2
		ORDER BY report_count DESC, reporter_count DESC
		LIMIT 20
	`, singoTypeCondition), startDT, endDT).Scan(&stats.PatternAnalysis)

	// 패턴 분석 요약
	totalPatternReports := 0
	for _, p := range stats.PatternAnalysis {
		totalPatternReports += p.ReportCount
	}

	top10Reports := stats.PatternAnalysis
	if len(top10Reports) > 10 {
		top10Reports = top10Reports[:10]
	}
	top10Total := 0
	for _, p := range top10Reports {
		top10Total += p.ReportCount
	}

	concentrationRate := float64(0)
	if stats.TotalReports > 0 {
		concentrationRate = float64(top10Total) / float64(stats.TotalReports) * 100
	}

	stats.PatternSummary = map[string]interface{}{
		"top_10_concentration":     fmt.Sprintf("%.1f", concentrationRate),
		"total_concentrated_posts": len(stats.PatternAnalysis),
		"total_pattern_reports":    totalPatternReports,
	}
	if len(stats.PatternAnalysis) > 0 {
		stats.PatternSummary["most_reported_post"] = stats.PatternAnalysis[0]
	}

	return stats
}

// saveReportPost saves the report as a post in g5_write_report
func saveReportPost(db *gorm.DB, stats *reportStats, subject string, now time.Time) error {
	statsJSON, err := json.Marshal(stats)
	if err != nil {
		return err
	}

	// 보고서 내용 생성
	content := fmt.Sprintf(
		"📊 신고 통계 분석 보고서\n\n"+
			"▣ 분석 개요\n"+
			"• 기간: %s ~ %s (%d일)\n"+
			"• 자동 생성: 매주 일요일 자정 5분\n\n"+
			"▣ 주요 지표\n"+
			"• 신고 현황\n"+
			"  - 총 신고건수: %d건\n"+
			"  - 신고된 글: %d건\n"+
			"  - 신고된 댓글: %d건\n\n"+
			"• 처리 현황\n"+
			"  - 처리완료: %d건\n"+
			"  - 소명처리: %d건\n"+
			"  - 신고자수: %d명\n\n"+
			"▣ 데이터 규모\n"+
			"• 전체 게시글: %d개\n"+
			"• 전체 댓글: %d개\n"+
			"• 데이터 크기: %d bytes\n\n"+
			"※ 상세 통계는 차트를 참고해주세요.",
		stats.DateFrom, stats.DateTo, stats.PeriodDays,
		stats.TotalReports, stats.ReportCount, stats.ReportMonth,
		stats.CompletedReports, stats.ClaimReports, stats.ReporterCount,
		stats.TotalCases, stats.TotalMonthCases, len(statsJSON),
	)

	// wr_num 계산: wr_datetime 기준 올바른 위치에 삽입
	// 이 보고서보다 최신 글 중 가장 큰 wr_num을 찾아 그보다 1 작은 값 사용
	// (wr_num이 작을수록 위에 표시됨)
	nowStr := now.Format("2006-01-02 15:04:05")
	var wrNum int
	db.Raw(`
		SELECT COALESCE(
			(SELECT wr_num - 1 FROM g5_write_report
			 WHERE wr_is_comment = 0 AND wr_datetime > ?
			 ORDER BY wr_datetime ASC LIMIT 1),
			COALESCE(MIN(wr_num), 0) - 1
		) FROM g5_write_report
	`, nowStr).Scan(&wrNum)

	result := db.Exec(`
		INSERT INTO g5_write_report
		SET wr_num = ?,
			wr_reply = '',
			wr_comment = 0,
			ca_name = '통계',
			wr_option = '',
			wr_subject = ?,
			wr_content = ?,
			wr_9 = ?,
			mb_id = 'ai',
			wr_password = '',
			wr_name = '다모앙',
			wr_datetime = ?,
			wr_last = ?,
			wr_ip = '127.0.0.1'
	`, wrNum, subject, content, string(statsJSON), nowStr, nowStr)

	if result.Error != nil {
		return result.Error
	}

	// 게시판 글 수 업데이트
	db.Exec("UPDATE g5_board SET bo_count_write = bo_count_write + 1 WHERE bo_table = 'report'")

	return nil
}

// getSingoTypes returns the report type labels map
func getSingoTypes() map[int]string {
	return reportTypeLabels
}

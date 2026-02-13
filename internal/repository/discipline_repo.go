package repository

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// sgTypeLabels maps sg_type integer codes to Korean labels (PHP SingoHelper 호환)
var sgTypeLabels = map[int]string{
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

// buildReasonLabels converts sg_type integer codes to comma-separated Korean labels
func buildReasonLabels(sgTypes []int) string {
	labels := make([]string, 0, len(sgTypes))
	for _, code := range sgTypes {
		if label, ok := sgTypeLabels[code]; ok {
			labels = append(labels, label)
		}
	}
	return strings.Join(labels, ", ")
}

const disciplineLogTable = "g5_write_disciplinelog"

// DisciplineRepository handles discipline log data operations
type DisciplineRepository struct {
	db *gorm.DB
}

// NewDisciplineRepository creates a new DisciplineRepository
func NewDisciplineRepository(db *gorm.DB) *DisciplineRepository {
	return &DisciplineRepository{db: db}
}

// WithTx returns a new DisciplineRepository with the given transaction
func (r *DisciplineRepository) WithTx(tx *gorm.DB) *DisciplineRepository {
	return &DisciplineRepository{db: tx}
}

// DB returns the underlying database instance
func (r *DisciplineRepository) DB() *gorm.DB {
	return r.db
}

// GetNextWriteNum retrieves the next write number for disciplinelog
func (r *DisciplineRepository) GetNextWriteNum() (int, error) {
	var maxNum int
	err := r.db.Table(disciplineLogTable).
		Select("IFNULL(MIN(wr_num), 0) - 1").
		Scan(&maxNum).Error
	if err != nil {
		return 0, err
	}
	return maxNum, nil
}

// CreateDisciplineLog creates a new discipline log entry
func (r *DisciplineRepository) CreateDisciplineLog(
	adminID string,
	adminName string,
	targetID string,
	targetNickname string,
	content *domain.DisciplineLogContent,
	reportID int,
	reportTable string,
	processType string,
	clientIP string,
) (int, error) {
	// Get next wr_num
	wrNum, err := r.GetNextWriteNum()
	if err != nil {
		return 0, fmt.Errorf("failed to get next write num: %w", err)
	}

	// Serialize content to JSON
	contentJSON, err := json.Marshal(content)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal content: %w", err)
	}

	now := time.Now()
	nowStr := now.Format("2006-01-02 15:04:05")

	// Build subject: "user_id(nickname)"
	subject := targetID
	if targetNickname != "" {
		subject = fmt.Sprintf("%s(%s)", targetID, targetNickname)
	}

	// Build wr_1: 사유 라벨 (PHP 목록 스킨 호환)
	reasonLabels := buildReasonLabels(content.SgTypes)

	// Build wr_link1: 원본 글 URL (PHP 호환)
	wrLink1 := ""
	if reportTable != "" && content.ReportedURL != "" {
		wrLink1 = "https://damoang.net" + content.ReportedURL
	}

	log := &domain.DisciplineLog{
		Num:       wrNum,
		Reply:     "",
		Parent:    0,
		IsComment: 0,
		Comment:   0,
		Option:    "html1",
		Subject:   subject,
		Content:   string(contentJSON),
		MemberID:  "police", // PHP 호환: 시스템 계정
		Name:      "police", // PHP 호환: 시스템 계정
		Password:  "",
		DateTime:  now,
		Last:      nowStr,
		IP:        clientIP,
		Wr1:       reasonLabels,                // 사유 라벨 (목록 표시용)
		Link1:     wrLink1,                     // 원본 글 URL
		Wr4:       "step2_approved",            // 처리 상태
		Wr5:       reportTable,                 // 원본 신고 테이블
		Wr6:       fmt.Sprintf("%d", reportID), // 원본 신고 ID
		Wr7:       processType,                 // 처리 유형
	}

	if err := r.db.Table(disciplineLogTable).Create(log).Error; err != nil {
		return 0, err
	}

	// Update wr_parent to wr_id (그누보드 방식)
	if err := r.db.Table(disciplineLogTable).
		Where("wr_id = ?", log.ID).
		Update("wr_parent", log.ID).Error; err != nil {
		return 0, err
	}

	return log.ID, nil
}

// GetByID retrieves a discipline log by ID
func (r *DisciplineRepository) GetByID(id int) (*domain.DisciplineLog, error) {
	var log domain.DisciplineLog
	if err := r.db.Table(disciplineLogTable).Where("wr_id = ?", id).First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}

// FindByTargetMember returns discipline logs where the subject contains the member ID (parent posts only)
func (r *DisciplineRepository) FindByTargetMember(memberID string, page, limit int) ([]domain.DisciplineLog, int64, error) {
	var logs []domain.DisciplineLog
	var total int64

	// DisciplineLog content JSON에 target_id가 포함된 것을 찾음
	// wr_is_comment = 0 (본글만, 소명 댓글 제외)
	query := r.db.Table(disciplineLogTable).
		Where("wr_is_comment = 0 AND wr_content LIKE ?", fmt.Sprintf("%%\"target_id\":\"%s\"%%", memberID))

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("wr_datetime DESC").
		Offset(offset).Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// CountByTargetMemberIDs returns discipline count for multiple target members (batch query)
// wr_subject 형식: "user_id(nickname)" → SUBSTRING_INDEX로 user_id 추출
func (r *DisciplineRepository) CountByTargetMemberIDs(memberIDs []string) (map[string]int, error) {
	if len(memberIDs) == 0 {
		return map[string]int{}, nil
	}

	type countRow struct {
		MemberID string `gorm:"column:member_id"`
		Count    int    `gorm:"column:count"`
	}

	var rows []countRow
	err := r.db.Table(disciplineLogTable).
		Select("SUBSTRING_INDEX(wr_subject, '(', 1) as member_id, COUNT(*) as count").
		Where("wr_is_comment = 0 AND SUBSTRING_INDEX(wr_subject, '(', 1) IN (?)", memberIDs).
		Group("SUBSTRING_INDEX(wr_subject, '(', 1)").
		Find(&rows).Error

	if err != nil {
		return nil, err
	}

	result := make(map[string]int, len(rows))
	for _, row := range rows {
		result[row.MemberID] = row.Count
	}
	return result, nil
}

// ListAll returns all discipline logs (parent posts only, for admin board view)
func (r *DisciplineRepository) ListAll(page, limit int) ([]domain.DisciplineLog, int64, error) {
	var logs []domain.DisciplineLog
	var total int64

	query := r.db.Table(disciplineLogTable).Where("wr_is_comment = 0")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("wr_datetime DESC").
		Offset(offset).Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

// CreateAppeal creates an appeal comment (소명 글) under a discipline log
func (r *DisciplineRepository) CreateAppeal(parentID int, memberID, memberName, content, ip string) (int, error) {
	// Get parent to derive wr_num
	parent, err := r.GetByID(parentID)
	if err != nil {
		return 0, fmt.Errorf("이용제한 내역을 찾을 수 없습니다")
	}

	// Count existing comments to build reply string
	var commentCount int64
	r.db.Table(disciplineLogTable).
		Where("wr_parent = ? AND wr_is_comment = 1", parentID).
		Count(&commentCount)

	now := time.Now()
	appeal := &domain.DisciplineLog{
		Num:       parent.Num,
		Reply:     fmt.Sprintf("%02d", commentCount+1),
		Parent:    parentID,
		IsComment: 1,
		Option:    "html1",
		Subject:   "",
		Content:   content,
		MemberID:  memberID,
		Name:      memberName,
		DateTime:  now,
		Last:      now.Format("2006-01-02 15:04:05"),
		IP:        ip,
	}

	if err := r.db.Table(disciplineLogTable).Create(appeal).Error; err != nil {
		return 0, err
	}

	// Update parent comment count
	r.db.Table(disciplineLogTable).
		Where("wr_id = ?", parentID).
		Update("wr_comment", gorm.Expr("wr_comment + 1"))

	return appeal.ID, nil
}

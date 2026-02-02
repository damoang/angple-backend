package repository

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

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

	log := &domain.DisciplineLog{
		Num:       wrNum,
		Reply:     "",
		Parent:    0,
		IsComment: 0,
		Comment:   0,
		Option:    "html1",
		Subject:   subject,
		Content:   string(contentJSON),
		MemberID:  adminID,
		Name:      adminName,
		Password:  "",
		DateTime:  now,
		Last:      nowStr,
		IP:        clientIP,
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

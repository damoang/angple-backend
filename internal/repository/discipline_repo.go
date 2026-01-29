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

package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// DisciplineLogRepository v2 discipline log data access
type DisciplineLogRepository interface {
	FindByID(id uint64) (*v2.DisciplineLog, error)
	FindAll(page, limit int) ([]*v2.DisciplineLog, int64, error)
	FindByMemberID(memberID string, page, limit int) ([]*v2.DisciplineLog, int64, error)
	Create(log *v2.DisciplineLog) error
	Update(log *v2.DisciplineLog) error
	Delete(id uint64) error
	UpdateStatus(id uint64, status string) error
}

type disciplineLogRepository struct {
	db *gorm.DB
}

// NewDisciplineLogRepository creates a new DisciplineLogRepository
func NewDisciplineLogRepository(db *gorm.DB) DisciplineLogRepository {
	return &disciplineLogRepository{db: db}
}

func (r *disciplineLogRepository) FindByID(id uint64) (*v2.DisciplineLog, error) {
	var log v2.DisciplineLog
	err := r.db.Where("id = ?", id).First(&log).Error
	return &log, err
}

func (r *disciplineLogRepository) FindAll(page, limit int) ([]*v2.DisciplineLog, int64, error) {
	var logs []*v2.DisciplineLog
	var total int64

	query := r.db.Model(&v2.DisciplineLog{}).Where("status = 'approved'")
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("penalty_date_from DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *disciplineLogRepository) FindByMemberID(memberID string, page, limit int) ([]*v2.DisciplineLog, int64, error) {
	var logs []*v2.DisciplineLog
	var total int64

	query := r.db.Model(&v2.DisciplineLog{}).Where("member_id = ? AND status = 'approved'", memberID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("penalty_date_from DESC").Offset(offset).Limit(limit).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}

func (r *disciplineLogRepository) Create(log *v2.DisciplineLog) error {
	return r.db.Create(log).Error
}

func (r *disciplineLogRepository) Update(log *v2.DisciplineLog) error {
	return r.db.Save(log).Error
}

func (r *disciplineLogRepository) Delete(id uint64) error {
	return r.db.Delete(&v2.DisciplineLog{}, id).Error
}

func (r *disciplineLogRepository) UpdateStatus(id uint64, status string) error {
	return r.db.Model(&v2.DisciplineLog{}).Where("id = ?", id).Update("status", status).Error
}

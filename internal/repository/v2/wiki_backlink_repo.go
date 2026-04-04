package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

type WikiBacklinkRepository struct {
	db *gorm.DB
}

func NewWikiBacklinkRepository(db *gorm.DB) *WikiBacklinkRepository {
	return &WikiBacklinkRepository{db: db}
}

// AutoMigrate wiki_backlinks 테이블 생성
func (r *WikiBacklinkRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&v2.WikiBacklink{})
}

// Create 백링크 생성
func (r *WikiBacklinkRepository) Create(backlink *v2.WikiBacklink) error {
	return r.db.Create(backlink).Error
}

// DeleteBySourceID 특정 문서의 모든 백링크 삭제 (재계산 전)
func (r *WikiBacklinkRepository) DeleteBySourceID(sourcePostID uint64) error {
	return r.db.Where("source_post_id = ?", sourcePostID).Delete(&v2.WikiBacklink{}).Error
}

// FindByTargetID 특정 문서를 참조하는 모든 백링크 조회
func (r *WikiBacklinkRepository) FindByTargetID(targetPostID uint64) ([]*v2.WikiBacklink, error) {
	var backlinks []*v2.WikiBacklink
	err := r.db.Where("target_post_id = ?", targetPostID).
		Order("created_at DESC").
		Find(&backlinks).Error
	return backlinks, err
}

// FindBySourceID 특정 문서에서 참조하는 모든 백링크 조회
func (r *WikiBacklinkRepository) FindBySourceID(sourcePostID uint64) ([]*v2.WikiBacklink, error) {
	var backlinks []*v2.WikiBacklink
	err := r.db.Where("source_post_id = ?", sourcePostID).
		Find(&backlinks).Error
	return backlinks, err
}

// ReplaceBacklinks 특정 문서의 백링크를 전부 교체 (삭제 후 일괄 삽입)
func (r *WikiBacklinkRepository) ReplaceBacklinks(sourcePostID uint64, backlinks []*v2.WikiBacklink) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 기존 백링크 삭제
		if err := tx.Where("source_post_id = ?", sourcePostID).Delete(&v2.WikiBacklink{}).Error; err != nil {
			return err
		}
		// 새 백링크 삽입
		if len(backlinks) > 0 {
			return tx.Create(&backlinks).Error
		}
		return nil
	})
}

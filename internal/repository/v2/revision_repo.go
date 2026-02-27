package v2

import (
	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// RevisionRepository v2 content revision data access
type RevisionRepository interface {
	Create(revision *v2.V2ContentRevision) error
	FindByPostID(postID uint64) ([]*v2.V2ContentRevision, error)
	FindByPostIDAndVersion(postID uint64, version uint) (*v2.V2ContentRevision, error)
	GetNextVersion(postID uint64) (uint, error)
}

type revisionRepository struct {
	db *gorm.DB
}

// NewRevisionRepository creates a new v2 RevisionRepository
func NewRevisionRepository(db *gorm.DB) RevisionRepository {
	return &revisionRepository{db: db}
}

func (r *revisionRepository) Create(revision *v2.V2ContentRevision) error {
	return r.db.Create(revision).Error
}

func (r *revisionRepository) FindByPostID(postID uint64) ([]*v2.V2ContentRevision, error) {
	var revisions []*v2.V2ContentRevision
	err := r.db.Where("post_id = ?", postID).Order("version DESC").Find(&revisions).Error
	return revisions, err
}

func (r *revisionRepository) FindByPostIDAndVersion(postID uint64, version uint) (*v2.V2ContentRevision, error) {
	var revision v2.V2ContentRevision
	err := r.db.Where("post_id = ? AND version = ?", postID, version).First(&revision).Error
	return &revision, err
}

func (r *revisionRepository) GetNextVersion(postID uint64) (uint, error) {
	var maxVersion *uint
	err := r.db.Model(&v2.V2ContentRevision{}).
		Where("post_id = ?", postID).
		Select("MAX(version)").
		Scan(&maxVersion).Error
	if err != nil {
		return 1, err
	}
	if maxVersion == nil {
		return 1, nil
	}
	return *maxVersion + 1, nil
}

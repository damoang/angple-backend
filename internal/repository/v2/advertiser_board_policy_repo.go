package v2

import (
	"errors"

	v2 "github.com/damoang/angple-backend/internal/domain/v2"
	"gorm.io/gorm"
)

// AdvertiserBoardPolicyRepository handles advertiser board policy data access.
type AdvertiserBoardPolicyRepository interface {
	FindByBoardSlug(slug string) (*v2.V2AdvertiserBoardPolicy, error)
	Upsert(policy *v2.V2AdvertiserBoardPolicy) error
}

type advertiserBoardPolicyRepository struct {
	db *gorm.DB
}

func NewAdvertiserBoardPolicyRepository(db *gorm.DB) AdvertiserBoardPolicyRepository {
	return &advertiserBoardPolicyRepository{db: db}
}

func (r *advertiserBoardPolicyRepository) FindByBoardSlug(slug string) (*v2.V2AdvertiserBoardPolicy, error) {
	var policy v2.V2AdvertiserBoardPolicy
	err := r.db.Where("board_id = ?", slug).First(&policy).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &v2.V2AdvertiserBoardPolicy{
			BoardID: slug,
			Mode:    "shadow",
			Enabled: false,
		}, nil
	}
	return &policy, err
}

func (r *advertiserBoardPolicyRepository) Upsert(policy *v2.V2AdvertiserBoardPolicy) error {
	var existing v2.V2AdvertiserBoardPolicy
	err := r.db.Where("board_id = ?", policy.BoardID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.db.Create(policy).Error
	} else if err != nil {
		return err
	}

	return r.db.Save(policy).Error
}

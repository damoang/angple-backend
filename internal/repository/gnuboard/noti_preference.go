package gnuboard

import (
	"time"

	"gorm.io/gorm"
)

// NotiPreference represents a row in g5_noti_preference table
type NotiPreference struct {
	MbID          string    `gorm:"column:mb_id;primaryKey"`
	NotiComment   bool      `gorm:"column:noti_comment;default:1"`
	NotiReply     bool      `gorm:"column:noti_reply;default:1"`
	NotiMention   bool      `gorm:"column:noti_mention;default:1"`
	NotiLike      bool      `gorm:"column:noti_like;default:1"`
	NotiFollow    bool      `gorm:"column:noti_follow;default:1"`
	LikeThreshold int       `gorm:"column:like_threshold;default:1"`
	UpdatedAt     time.Time `gorm:"column:updated_at"`
}

// TableName returns the g5_noti_preference table name
func (NotiPreference) TableName() string { return "g5_noti_preference" }

// NotiPreferenceRepository handles CRUD for notification preferences
type NotiPreferenceRepository interface {
	Get(mbID string) (*NotiPreference, error)
	Upsert(pref *NotiPreference) error
}

type notiPreferenceRepository struct {
	db *gorm.DB
}

// NewNotiPreferenceRepository creates a new NotiPreferenceRepository
func NewNotiPreferenceRepository(db *gorm.DB) NotiPreferenceRepository {
	return &notiPreferenceRepository{db: db}
}

// Get retrieves a user's notification preferences. Returns defaults if not found.
func (r *notiPreferenceRepository) Get(mbID string) (*NotiPreference, error) {
	var pref NotiPreference
	err := r.db.Where("mb_id = ?", mbID).First(&pref).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return &NotiPreference{
				MbID:          mbID,
				NotiComment:   true,
				NotiReply:     true,
				NotiMention:   true,
				NotiLike:      true,
				NotiFollow:    true,
				LikeThreshold: 1,
			}, nil
		}
		return nil, err
	}
	return &pref, nil
}

// Upsert inserts or updates a user's notification preferences
func (r *notiPreferenceRepository) Upsert(pref *NotiPreference) error {
	result := r.db.Where("mb_id = ?", pref.MbID).First(&NotiPreference{})
	if result.Error == gorm.ErrRecordNotFound {
		return r.db.Create(pref).Error
	}
	return r.db.Model(&NotiPreference{}).Where("mb_id = ?", pref.MbID).Updates(map[string]interface{}{
		"noti_comment":   pref.NotiComment,
		"noti_reply":     pref.NotiReply,
		"noti_mention":   pref.NotiMention,
		"noti_like":      pref.NotiLike,
		"noti_follow":    pref.NotiFollow,
		"like_threshold": pref.LikeThreshold,
	}).Error
}

package gnuboard

import (
	"time"

	"gorm.io/gorm"
)

// NotiPreference represents a row in g5_noti_preference table
type NotiPreference struct {
	MbID        string `gorm:"column:mb_id;primaryKey"`
	NotiComment bool   `gorm:"column:noti_comment;default:1"`
	NotiReply   bool   `gorm:"column:noti_reply;default:1"`
	NotiMention bool   `gorm:"column:noti_mention;default:1"`
	NotiLike    bool   `gorm:"column:noti_like;default:1"`
	NotiFollow  bool   `gorm:"column:noti_follow;default:1"`
	// 게시판 구독('write' subscribe) 알림 — 회원 팔로우(NotiFollow)와 분리 (#12607)
	NotiBoardSubscribe bool      `gorm:"column:noti_board_subscribe;default:1"`
	NotiKeyword        bool      `gorm:"column:noti_keyword;default:1"`
	LikeThreshold      int       `gorm:"column:like_threshold;default:1"`
	UpdatedAt          time.Time `gorm:"column:updated_at"`
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
				MbID:               mbID,
				NotiComment:        true,
				NotiReply:          true,
				NotiMention:        true,
				NotiLike:           true,
				NotiFollow:         true,
				NotiBoardSubscribe: true,
				NotiKeyword:        true,
				LikeThreshold:      1,
			}, nil
		}
		return nil, err
	}
	return &pref, nil
}

// Upsert inserts or updates a user's notification preferences
//
// ⛔ 처음 저장(행 없음)에 db.Create(pref) 를 쓰면 안 된다. default:1 태그가 붙은 bool 필드는
// 값이 false(제로값)일 때 GORM 이 컬럼 기본값(1)으로 바꿔 저장한다 — 행이 없는 회원이
// 알림을 끄면 켜진 채로 저장된다. 그래서 모든 컬럼을 명시한 map 으로 넣는다.
func (r *notiPreferenceRepository) Upsert(pref *NotiPreference) error {
	result := r.db.Where("mb_id = ?", pref.MbID).First(&NotiPreference{})
	if result.Error == gorm.ErrRecordNotFound {
		values := notiPreferenceColumns(pref)
		values["mb_id"] = pref.MbID
		values["updated_at"] = time.Now()
		return r.db.Model(&NotiPreference{}).Create(values).Error
	}
	return r.db.Model(&NotiPreference{}).Where("mb_id = ?", pref.MbID).Updates(notiPreferenceColumns(pref)).Error
}

// notiPreferenceColumns 는 저장할 설정 컬럼 전체를 map 으로 돌려준다(false 도 그대로 저장되도록).
func notiPreferenceColumns(pref *NotiPreference) map[string]interface{} {
	return map[string]interface{}{
		"noti_comment":         pref.NotiComment,
		"noti_reply":           pref.NotiReply,
		"noti_mention":         pref.NotiMention,
		"noti_like":            pref.NotiLike,
		"noti_follow":          pref.NotiFollow,
		"noti_board_subscribe": pref.NotiBoardSubscribe,
		"noti_keyword":         pref.NotiKeyword,
		"like_threshold":       pref.LikeThreshold,
	}
}

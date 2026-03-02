package v2

import "time"

// Advertiser represents an advertiser in the advertisers table
type Advertiser struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MemberID  string    `gorm:"column:member_id;type:varchar(50)" json:"member_id"`
	Name      string    `gorm:"column:name;type:varchar(100)" json:"name"`
	PostCount uint      `gorm:"column:post_count;default:1" json:"post_count"`
	StartDate *string   `gorm:"column:start_date;type:date" json:"start_date,omitempty"`
	EndDate   *string   `gorm:"column:end_date;type:date" json:"end_date,omitempty"`
	IsPinned  bool      `gorm:"column:is_pinned;default:false" json:"is_pinned"`
	IsActive  bool      `gorm:"column:is_active;default:true" json:"is_active"`
	Memo      *string   `gorm:"column:memo;type:text" json:"-"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Advertiser) TableName() string { return "advertisers" }

// PromotionPost represents a promotion post
type PromotionPost struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	AdvertiserID uint64    `gorm:"column:advertiser_id" json:"advertiser_id"`
	Title        string    `gorm:"column:title;type:varchar(255)" json:"title"`
	Content      *string   `gorm:"column:content;type:text" json:"content,omitempty"`
	LinkURL      *string   `gorm:"column:link_url;type:varchar(500)" json:"link_url,omitempty"`
	ImageURL     *string   `gorm:"column:image_url;type:varchar(500)" json:"image_url,omitempty"`
	Views        uint      `gorm:"column:views;default:0" json:"views"`
	Likes        uint      `gorm:"column:likes;default:0" json:"likes"`
	CommentCount uint      `gorm:"column:comment_count;default:0" json:"comment_count"`
	IsActive     bool      `gorm:"column:is_active;default:true" json:"is_active"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`

	// Join field
	Advertiser *Advertiser `gorm:"foreignKey:AdvertiserID" json:"advertiser,omitempty"`
}

func (PromotionPost) TableName() string { return "promotion_posts" }

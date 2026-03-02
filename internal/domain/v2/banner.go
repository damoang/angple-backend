package v2

import "time"

// Banner represents a banner in the banners table
type Banner struct {
	ID         uint64     `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Title      string     `gorm:"column:title;type:varchar(100)" json:"title"`
	ImageURL   *string    `gorm:"column:image_url;type:varchar(500)" json:"image_url,omitempty"`
	LinkURL    *string    `gorm:"column:link_url;type:varchar(500)" json:"link_url,omitempty"`
	Position   string     `gorm:"column:position;type:enum('header','sidebar','content','footer');default:'sidebar'" json:"position"`
	StartDate  *string    `gorm:"column:start_date;type:date" json:"start_date,omitempty"`
	EndDate    *string    `gorm:"column:end_date;type:date" json:"end_date,omitempty"`
	Priority   int        `gorm:"column:priority;default:0" json:"priority"`
	IsActive   bool       `gorm:"column:is_active;default:true" json:"is_active"`
	ClickCount uint       `gorm:"column:click_count;default:0" json:"click_count"`
	ViewCount  uint       `gorm:"column:view_count;default:0" json:"view_count"`
	AltText    *string    `gorm:"column:alt_text;type:varchar(255)" json:"alt_text,omitempty"`
	Target     string     `gorm:"column:target;type:enum('_self','_blank');default:'_blank'" json:"target"`
	Memo       *string    `gorm:"column:memo;type:text" json:"-"`
	CreatedAt  time.Time  `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (Banner) TableName() string { return "banners" }

// BannerClickLog represents a click log entry
type BannerClickLog struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BannerID  uint64    `gorm:"column:banner_id" json:"banner_id"`
	MemberID  *string   `gorm:"column:member_id;type:varchar(50)" json:"member_id,omitempty"`
	IPAddress *string   `gorm:"column:ip_address;type:varchar(45)" json:"ip_address,omitempty"`
	UserAgent *string   `gorm:"column:user_agent;type:varchar(500)" json:"user_agent,omitempty"`
	Referer   *string   `gorm:"column:referer;type:varchar(500)" json:"referer,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (BannerClickLog) TableName() string { return "banner_click_logs" }

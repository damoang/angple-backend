package v2

// V2AdvertiserBoardPolicy represents board-specific advertiser policy settings.
type V2AdvertiserBoardPolicy struct {
	ID                           uint    `gorm:"primaryKey" json:"id"`
	BoardID                      string  `gorm:"column:board_id;uniqueIndex" json:"board_id"`
	Mode                         string  `gorm:"column:mode;default:shadow" json:"mode"`
	Enabled                      bool    `gorm:"column:enabled;default:false" json:"enabled"`
	AllowActiveAdvertiserRead    bool    `gorm:"column:allow_active_advertiser_read;default:true" json:"allow_active_advertiser_read"`
	AllowActiveAdvertiserWrite   bool    `gorm:"column:allow_active_advertiser_write;default:true" json:"allow_active_advertiser_write"`
	AllowActiveAdvertiserComment bool    `gorm:"column:allow_active_advertiser_comment;default:true" json:"allow_active_advertiser_comment"`
	RequireCertification         bool    `gorm:"column:require_certification;default:false" json:"require_certification"`
	DailyPostLimit               int     `gorm:"column:daily_post_limit;default:0" json:"daily_post_limit"`
	AllowedRoutesJSON            *string `gorm:"column:allowed_routes_json;type:text" json:"allowed_routes_json"`
}

// TableName returns the table name for GORM.
func (V2AdvertiserBoardPolicy) TableName() string {
	return "v2_advertiser_board_policies"
}

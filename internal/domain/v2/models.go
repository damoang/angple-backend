package v2

import (
	"time"
)

// V2User represents a user in the v2 schema
type V2User struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Username    string    `gorm:"column:username;type:varchar(50);uniqueIndex" json:"username"`
	Email       string    `gorm:"column:email;type:varchar(255);uniqueIndex" json:"email"`
	Password    string    `gorm:"column:password;type:varchar(255)" json:"-"`
	Nickname    string    `gorm:"column:nickname;type:varchar(100)" json:"nickname"`
	Level       uint8     `gorm:"column:level;default:1" json:"level"`
	Point       int       `gorm:"column:point;default:0" json:"point"`
	Exp         int       `gorm:"column:exp;default:0" json:"exp"`
	NariyaLevel uint8     `gorm:"column:nariya_level;default:1" json:"nariya_level"`
	NariyaMax   int       `gorm:"column:nariya_max;default:1000" json:"nariya_max"`
	Status      string    `gorm:"column:status;type:enum('active','inactive','banned');default:'active'" json:"status"`
	AvatarURL   *string   `gorm:"column:avatar_url;type:varchar(500)" json:"avatar_url,omitempty"`
	Bio         *string   `gorm:"column:bio;type:text" json:"bio,omitempty"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2User) TableName() string { return "v2_users" }

// V2Board represents a board in the v2 schema
type V2Board struct {
	ID          uint64  `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Slug        string  `gorm:"column:slug;type:varchar(50);uniqueIndex" json:"slug"`
	Name        string  `gorm:"column:name;type:varchar(100)" json:"name"`
	Description *string `gorm:"column:description;type:text" json:"description,omitempty"`
	CategoryID  *uint64 `gorm:"column:category_id" json:"category_id,omitempty"`
	Settings    *string `gorm:"column:settings;type:json" json:"settings,omitempty"`
	IsActive    bool    `gorm:"column:is_active;default:true" json:"is_active"`
	OrderNum    uint    `gorm:"column:order_num;default:0" json:"order_num"`
	// 레벨 제어 (그누보드 bo_*_level 대응)
	ListLevel     uint8 `gorm:"column:list_level;default:0" json:"list_level"`
	ReadLevel     uint8 `gorm:"column:read_level;default:0" json:"read_level"`
	WriteLevel    uint8 `gorm:"column:write_level;default:1" json:"write_level"`
	ReplyLevel    uint8 `gorm:"column:reply_level;default:1" json:"reply_level"`
	CommentLevel  uint8 `gorm:"column:comment_level;default:1" json:"comment_level"`
	UploadLevel   uint8 `gorm:"column:upload_level;default:1" json:"upload_level"`
	DownloadLevel uint8 `gorm:"column:download_level;default:1" json:"download_level"`
	// 포인트 설정 (양수=지급, 음수=차감)
	WritePoint    int       `gorm:"column:write_point;default:0" json:"write_point"`
	CommentPoint  int       `gorm:"column:comment_point;default:0" json:"comment_point"`
	DownloadPoint int       `gorm:"column:download_point;default:0" json:"download_point"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// V2Point represents a point transaction log
type V2Point struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	Point     int       `gorm:"column:point" json:"point"`
	Balance   int       `gorm:"column:balance" json:"balance"`
	Reason    string    `gorm:"column:reason;type:varchar(100)" json:"reason"`
	RelTable  string    `gorm:"column:rel_table;type:varchar(50)" json:"rel_table"`
	RelID     uint64    `gorm:"column:rel_id" json:"rel_id"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Point) TableName() string { return "v2_points" }

func (V2Board) TableName() string { return "v2_boards" }

// V2Post represents a post in the v2 schema
type V2Post struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BoardID      uint64    `gorm:"column:board_id;index" json:"board_id"`
	UserID       uint64    `gorm:"column:user_id;index" json:"user_id"`
	Title        string    `gorm:"column:title;type:varchar(255)" json:"title"`
	Content      string    `gorm:"column:content;type:mediumtext" json:"content"`
	Status       string    `gorm:"column:status;type:enum('draft','published','deleted');default:'published'" json:"status"`
	ViewCount    uint      `gorm:"column:view_count;default:0" json:"view_count"`
	CommentCount uint      `gorm:"column:comment_count;default:0" json:"comment_count"`
	IsNotice     bool      `gorm:"column:is_notice;default:false" json:"is_notice"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2Post) TableName() string { return "v2_posts" }

// V2Comment represents a comment in the v2 schema
type V2Comment struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PostID    uint64    `gorm:"column:post_id;index" json:"post_id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	ParentID  *uint64   `gorm:"column:parent_id" json:"parent_id,omitempty"`
	Content   string    `gorm:"column:content;type:text" json:"content"`
	Depth     uint8     `gorm:"column:depth;default:0" json:"depth"`
	Status    string    `gorm:"column:status;type:enum('active','deleted');default:'active'" json:"status"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (V2Comment) TableName() string { return "v2_comments" }

// V2Category represents a category
type V2Category struct {
	ID          uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	ParentID    *uint64   `gorm:"column:parent_id" json:"parent_id,omitempty"`
	Name        string    `gorm:"column:name;type:varchar(100)" json:"name"`
	Slug        string    `gorm:"column:slug;type:varchar(50);uniqueIndex" json:"slug"`
	Description *string   `gorm:"column:description;type:text" json:"description,omitempty"`
	OrderNum    uint      `gorm:"column:order_num;default:0" json:"order_num"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Category) TableName() string { return "v2_categories" }

// V2Tag represents a tag
type V2Tag struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Name      string    `gorm:"column:name;type:varchar(50);uniqueIndex" json:"name"`
	Slug      string    `gorm:"column:slug;type:varchar(50);uniqueIndex" json:"slug"`
	PostCount uint      `gorm:"column:post_count;default:0" json:"post_count"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Tag) TableName() string { return "v2_tags" }

// V2PostTag represents a post-tag relationship
type V2PostTag struct {
	PostID uint64 `gorm:"column:post_id;primaryKey" json:"post_id"`
	TagID  uint64 `gorm:"column:tag_id;primaryKey" json:"tag_id"`
}

func (V2PostTag) TableName() string { return "v2_post_tags" }

// V2File represents an uploaded file
type V2File struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PostID        *uint64   `gorm:"column:post_id;index" json:"post_id,omitempty"`
	CommentID     *uint64   `gorm:"column:comment_id" json:"comment_id,omitempty"`
	UserID        uint64    `gorm:"column:user_id;index" json:"user_id"`
	OriginalName  string    `gorm:"column:original_name;type:varchar(255)" json:"original_name"`
	StoredName    string    `gorm:"column:stored_name;type:varchar(255)" json:"stored_name"`
	MimeType      string    `gorm:"column:mime_type;type:varchar(100)" json:"mime_type"`
	FileSize      uint64    `gorm:"column:file_size" json:"file_size"`
	StoragePath   string    `gorm:"column:storage_path;type:varchar(500)" json:"storage_path"`
	DownloadCount uint      `gorm:"column:download_count;default:0" json:"download_count"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2File) TableName() string { return "v2_files" }

// V2Notification represents a notification
type V2Notification struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	Type      string    `gorm:"column:type;type:varchar(50)" json:"type"`
	Title     string    `gorm:"column:title;type:varchar(255)" json:"title"`
	Content   *string   `gorm:"column:content;type:text" json:"content,omitempty"`
	Link      *string   `gorm:"column:link;type:varchar(500)" json:"link,omitempty"`
	IsRead    bool      `gorm:"column:is_read;default:false" json:"is_read"`
	SenderID  *uint64   `gorm:"column:sender_id" json:"sender_id,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Notification) TableName() string { return "v2_notifications" }

// V2Session represents a user session
type V2Session struct {
	ID        string    `gorm:"column:id;type:varchar(128);primaryKey" json:"id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	UserAgent *string   `gorm:"column:user_agent;type:varchar(500)" json:"user_agent,omitempty"`
	IPAddress *string   `gorm:"column:ip_address;type:varchar(45)" json:"ip_address,omitempty"`
	ExpiresAt time.Time `gorm:"column:expires_at;index" json:"expires_at"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
}

func (V2Session) TableName() string { return "v2_sessions" }

// Meta tables for plugin extensibility

// UserMeta represents user metadata for plugins
type UserMeta struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	UserID    uint64    `gorm:"column:user_id;index" json:"user_id"`
	Namespace string    `gorm:"column:namespace;type:varchar(64)" json:"namespace"`
	MetaKey   string    `gorm:"column:meta_key;type:varchar(128)" json:"meta_key"`
	MetaValue *string   `gorm:"column:meta_value;type:json" json:"meta_value,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (UserMeta) TableName() string { return "v2_user_meta" }

// PostMeta represents post metadata for plugins
type PostMeta struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	PostID    uint64    `gorm:"column:post_id;index" json:"post_id"`
	Namespace string    `gorm:"column:namespace;type:varchar(64)" json:"namespace"`
	MetaKey   string    `gorm:"column:meta_key;type:varchar(128)" json:"meta_key"`
	MetaValue *string   `gorm:"column:meta_value;type:json" json:"meta_value,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (PostMeta) TableName() string { return "v2_post_meta" }

// CommentMeta represents comment metadata for plugins
type CommentMeta struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	CommentID uint64    `gorm:"column:comment_id;index" json:"comment_id"`
	Namespace string    `gorm:"column:namespace;type:varchar(64)" json:"namespace"`
	MetaKey   string    `gorm:"column:meta_key;type:varchar(128)" json:"meta_key"`
	MetaValue *string   `gorm:"column:meta_value;type:json" json:"meta_value,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (CommentMeta) TableName() string { return "v2_comment_meta" }

// OptionMeta represents global options for core and plugins
type OptionMeta struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Namespace string    `gorm:"column:namespace;type:varchar(64)" json:"namespace"`
	MetaKey   string    `gorm:"column:meta_key;type:varchar(128)" json:"meta_key"`
	MetaValue *string   `gorm:"column:meta_value;type:json" json:"meta_value,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

func (OptionMeta) TableName() string { return "v2_option_meta" }

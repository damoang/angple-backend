package domain

import (
	"time"
)

// Board represents a bulletin board configuration (g5_board table)
type Board struct {
	InsertTime       time.Time `gorm:"column:bo_insert_time;autoCreateTime" json:"insert_time"`
	BoardID          string    `gorm:"column:bo_table;primaryKey;size:20" json:"board_id"`
	GroupID          string    `gorm:"column:gr_id;size:20" json:"group_id"`
	Subject          string    `gorm:"column:bo_subject;size:255" json:"subject"`
	Admin            string    `gorm:"column:bo_admin;size:255" json:"admin"`
	Device           string    `gorm:"column:bo_device;size:10;default:both" json:"device"`
	CategoryList     string    `gorm:"column:bo_category_list;type:text" json:"category_list"`
	Skin             string    `gorm:"column:bo_skin;size:255" json:"skin"`
	MobileSkin       string    `gorm:"column:bo_mobile_skin;size:255" json:"mobile_skin"`
	Notice           string    `gorm:"column:bo_notice;type:text" json:"notice"`
	Extra1           string    `gorm:"column:bo_1;size:255" json:"extra_1,omitempty"`
	Extra2           string    `gorm:"column:bo_2;size:255" json:"extra_2,omitempty"`
	Extra3           string    `gorm:"column:bo_3;size:255" json:"extra_3,omitempty"`
	Extra4           string    `gorm:"column:bo_4;size:255" json:"extra_4,omitempty"`
	Extra5           string    `gorm:"column:bo_5;size:255" json:"extra_5,omitempty"`
	Extra6           string    `gorm:"column:bo_6;size:255" json:"extra_6,omitempty"`
	Extra7           string    `gorm:"column:bo_7;size:255" json:"extra_7,omitempty"`
	Extra8           string    `gorm:"column:bo_8;size:255" json:"extra_8,omitempty"`
	Extra9           string    `gorm:"column:bo_9;size:255" json:"extra_9,omitempty"`
	Extra10          string    `gorm:"column:bo_10;size:255" json:"extra_10,omitempty"`
	UploadSize       int64     `gorm:"column:bo_upload_size;default:1048576" json:"upload_size"`
	ListLevel        int       `gorm:"column:bo_list_level;default:1" json:"list_level"`
	ReadLevel        int       `gorm:"column:bo_read_level;default:1" json:"read_level"`
	WriteLevel       int       `gorm:"column:bo_write_level;default:1" json:"write_level"`
	ReplyLevel       int       `gorm:"column:bo_reply_level;default:1" json:"reply_level"`
	CommentLevel     int       `gorm:"column:bo_comment_level;default:1" json:"comment_level"`
	HTMLLevel        int       `gorm:"column:bo_html_level;default:1" json:"html_level"`
	LinkLevel        int       `gorm:"column:bo_link_level;default:1" json:"link_level"`
	UploadLevel      int       `gorm:"column:bo_upload_level;default:1" json:"upload_level"`
	DownloadLevel    int       `gorm:"column:bo_download_level;default:1" json:"download_level"`
	UseCategory      int       `gorm:"column:bo_use_category;default:0" json:"use_category"`
	UseSideview      int       `gorm:"column:bo_use_sideview;default:0" json:"use_sideview"`
	UseFileContent   int       `gorm:"column:bo_use_file_content;default:0" json:"use_file_content"`
	UseSecret        int       `gorm:"column:bo_use_secret;default:0" json:"use_secret"`
	UseDhtml         int       `gorm:"column:bo_use_dhtml_editor;default:0" json:"use_dhtml"`
	UseRss           int       `gorm:"column:bo_use_rss;default:0" json:"use_rss"`
	UseGood          int       `gorm:"column:bo_use_good;default:0" json:"use_good"`
	UseNogood        int       `gorm:"column:bo_use_nogood;default:0" json:"use_nogood"`
	UseName          int       `gorm:"column:bo_use_name;default:0" json:"use_name"`
	UseSignature     int       `gorm:"column:bo_use_signature;default:0" json:"use_signature"`
	UseIP            int       `gorm:"column:bo_use_ip_view;default:0" json:"use_ip"`
	UseListView      int       `gorm:"column:bo_use_list_view;default:0" json:"use_list_view"`
	UseListContent   int       `gorm:"column:bo_use_list_content;default:0" json:"use_list_content"`
	UseCaptcha       int       `gorm:"column:bo_use_captcha;default:0" json:"use_captcha"`
	PageRows         int       `gorm:"column:bo_page_rows;default:15" json:"page_rows"`
	MobilePageRows   int       `gorm:"column:bo_mobile_page_rows;default:15" json:"mobile_page_rows"`
	SubjectLen       int       `gorm:"column:bo_subject_len;default:60" json:"subject_len"`
	MobileSubjectLen int       `gorm:"column:bo_mobile_subject_len;default:30" json:"mobile_subject_len"`
	NewRows          int       `gorm:"column:bo_new;default:24" json:"new_rows"`
	HotRows          int       `gorm:"column:bo_hot;default:100" json:"hot_rows"`
	MinWritePoint    int       `gorm:"column:bo_write_min;default:0" json:"min_write_point"`
	MaxWritePoint    int       `gorm:"column:bo_write_max;default:0" json:"max_write_point"`
	CommentMinPoint  int       `gorm:"column:bo_comment_min;default:0" json:"comment_min_point"`
	CommentMaxPoint  int       `gorm:"column:bo_comment_max;default:0" json:"comment_max_point"`
	UploadCount      int       `gorm:"column:bo_upload_count;default:2" json:"upload_count"`
	ImageWidth       int       `gorm:"column:bo_image_width;default:835" json:"image_width"`
	Order            int       `gorm:"column:bo_order;default:0" json:"order"`
	CountWrite       int       `gorm:"column:bo_count_write;default:0" json:"count_write"`
	CountComment     int       `gorm:"column:bo_count_comment;default:0" json:"count_comment"`
}

func (Board) TableName() string {
	return "g5_board"
}

// CreateBoardRequest - 게시판 생성 요청 DTO
type CreateBoardRequest struct {
	// 포인터 필드들 먼저
	ListLevel    *int   `json:"list_level,omitempty"`
	ReadLevel    *int   `json:"read_level,omitempty"`
	WriteLevel   *int   `json:"write_level,omitempty"`
	ReplyLevel   *int   `json:"reply_level,omitempty"`
	CommentLevel *int   `json:"comment_level,omitempty"`
	UseCategory  *int   `json:"use_category,omitempty"`
	PageRows     *int   `json:"page_rows,omitempty"`
	UploadCount  *int   `json:"upload_count,omitempty"`
	UploadSize   *int64 `json:"upload_size,omitempty"`

	// String 필드들
	BoardID      string `json:"board_id" binding:"required,min=2,max=20,alphanum"`
	GroupID      string `json:"group_id" binding:"required"`
	Subject      string `json:"subject" binding:"required,min=1,max=255"`
	Admin        string `json:"admin,omitempty"`
	Device       string `json:"device,omitempty"`        // pc, mobile, both
	CategoryList string `json:"category_list,omitempty"` // 파이프 구분 (예: "공지|자유|질문")
	Skin         string `json:"skin,omitempty"`
	MobileSkin   string `json:"mobile_skin,omitempty"`
}

// UpdateBoardRequest - 게시판 수정 요청 DTO
type UpdateBoardRequest struct {
	// 포인터 필드들 먼저
	Subject      *string `json:"subject,omitempty"`
	Admin        *string `json:"admin,omitempty"`
	Device       *string `json:"device,omitempty"`
	CategoryList *string `json:"category_list,omitempty"`
	Skin         *string `json:"skin,omitempty"`
	MobileSkin   *string `json:"mobile_skin,omitempty"`
	ListLevel    *int    `json:"list_level,omitempty"`
	ReadLevel    *int    `json:"read_level,omitempty"`
	WriteLevel   *int    `json:"write_level,omitempty"`
	ReplyLevel   *int    `json:"reply_level,omitempty"`
	CommentLevel *int    `json:"comment_level,omitempty"`
	UseCategory  *int    `json:"use_category,omitempty"`
	PageRows     *int    `json:"page_rows,omitempty"`
	UploadCount  *int    `json:"upload_count,omitempty"`
	UploadSize   *int64  `json:"upload_size,omitempty"`
}

// BoardResponse - 게시판 응답 DTO
type BoardResponse struct {
	InsertTime   time.Time `json:"insert_time"`
	MobileSkin   string    `json:"mobile_skin"`
	BoardID      string    `json:"board_id"`
	GroupID      string    `json:"group_id"`
	Subject      string    `json:"subject"`
	Admin        string    `json:"admin"`
	Device       string    `json:"device"`
	CategoryList string    `json:"category_list,omitempty"`
	Skin         string    `json:"skin"`
	ListLevel    int       `json:"list_level"`
	UploadSize   int64     `json:"upload_size"`
	ReadLevel    int       `json:"read_level"`
	WriteLevel   int       `json:"write_level"`
	ReplyLevel   int       `json:"reply_level"`
	CommentLevel int       `json:"comment_level"`
	UseCategory  int       `json:"use_category"`
	PageRows     int       `json:"page_rows"`
	UploadCount  int       `json:"upload_count"`
	CountWrite   int       `json:"count_write"`
	CountComment int       `json:"count_comment"`
}

func (b *Board) ToResponse() *BoardResponse {
	return &BoardResponse{
		BoardID:      b.BoardID,
		GroupID:      b.GroupID,
		Subject:      b.Subject,
		Admin:        b.Admin,
		Device:       b.Device,
		ListLevel:    b.ListLevel,
		ReadLevel:    b.ReadLevel,
		WriteLevel:   b.WriteLevel,
		ReplyLevel:   b.ReplyLevel,
		CommentLevel: b.CommentLevel,
		UseCategory:  b.UseCategory,
		CategoryList: b.CategoryList,
		Skin:         b.Skin,
		MobileSkin:   b.MobileSkin,
		PageRows:     b.PageRows,
		UploadCount:  b.UploadCount,
		UploadSize:   b.UploadSize,
		CountWrite:   b.CountWrite,
		CountComment: b.CountComment,
		InsertTime:   b.InsertTime,
	}
}

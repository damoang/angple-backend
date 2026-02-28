package gnuboard

// G5Board represents the g5_board table (Gnuboard board settings)
type G5Board struct {
	BoTable        string `gorm:"column:bo_table;primaryKey" json:"bo_table"`
	GrID           string `gorm:"column:gr_id" json:"gr_id"`
	BoSubject      string `gorm:"column:bo_subject" json:"bo_subject"`
	BoMobile       string `gorm:"column:bo_mobile" json:"bo_mobile"`
	BoDevice       string `gorm:"column:bo_device" json:"bo_device"`
	BoAdmin        string `gorm:"column:bo_admin" json:"bo_admin"`
	BoListLevel    int    `gorm:"column:bo_list_level" json:"bo_list_level"`
	BoReadLevel    int    `gorm:"column:bo_read_level" json:"bo_read_level"`
	BoWriteLevel   int    `gorm:"column:bo_write_level" json:"bo_write_level"`
	BoReplyLevel   int    `gorm:"column:bo_reply_level" json:"bo_reply_level"`
	BoCommentLevel int    `gorm:"column:bo_comment_level" json:"bo_comment_level"`
	BoUploadLevel  int    `gorm:"column:bo_upload_level" json:"bo_upload_level"`
	BoDownloadLevel int   `gorm:"column:bo_download_level" json:"bo_download_level"`
	BoOrder        int    `gorm:"column:bo_order" json:"bo_order"`
	BoCountWrite   int    `gorm:"column:bo_count_write" json:"bo_count_write"`
	BoCountComment int    `gorm:"column:bo_count_comment" json:"bo_count_comment"`
	BoNumListCount int    `gorm:"column:bo_num_list_count" json:"bo_num_list_count"`
	BoPageRows     int    `gorm:"column:bo_page_rows" json:"bo_page_rows"`
	BoUseCategory  int    `gorm:"column:bo_use_category" json:"bo_use_category"`
	BoCategoryList string `gorm:"column:bo_category_list" json:"bo_category_list"`
	BoUseSideview  int    `gorm:"column:bo_use_sideview" json:"bo_use_sideview"`
	BoUseSns       int    `gorm:"column:bo_use_sns" json:"bo_use_sns"`
	BoUseSecret    int    `gorm:"column:bo_use_secret" json:"bo_use_secret"`
	BoWritePoint   int    `gorm:"column:bo_write_point" json:"bo_write_point"`
	BoCommentPoint int    `gorm:"column:bo_comment_point" json:"bo_comment_point"`
	BoReadPoint    int    `gorm:"column:bo_read_point" json:"bo_read_point"`
	BoDownloadPoint int   `gorm:"column:bo_download_point" json:"bo_download_point"`
	BoUseGood      int    `gorm:"column:bo_use_good" json:"bo_use_good"`
	BoUseNogood    int    `gorm:"column:bo_use_nogood" json:"bo_use_nogood"`
	BoUseName      int    `gorm:"column:bo_use_name" json:"bo_use_name"`
	BoUseSkin      int    `gorm:"column:bo_use_skin" json:"bo_use_skin"`
	BoSkin         string `gorm:"column:bo_skin" json:"bo_skin"`
	BoMobileSkin   string `gorm:"column:bo_mobile_skin" json:"bo_mobile_skin"`
	BoNotice       string `gorm:"column:bo_notice" json:"bo_notice"`
	Bo1            string `gorm:"column:bo_1" json:"bo_1"`
	Bo2            string `gorm:"column:bo_2" json:"bo_2"`
	Bo3            string `gorm:"column:bo_3" json:"bo_3"`
	Bo4            string `gorm:"column:bo_4" json:"bo_4"`
	Bo5            string `gorm:"column:bo_5" json:"bo_5"`
	Bo6            string `gorm:"column:bo_6" json:"bo_6"`
	Bo7            string `gorm:"column:bo_7" json:"bo_7"`
	Bo8            string `gorm:"column:bo_8" json:"bo_8"`
	Bo9            string `gorm:"column:bo_9" json:"bo_9"`
	Bo10           string `gorm:"column:bo_10" json:"bo_10"`
}

// TableName returns the table name for GORM
func (G5Board) TableName() string {
	return "g5_board"
}

// BoardResponse is the API response format for board
type BoardResponse struct {
	ID            string `json:"id"`
	Slug          string `json:"slug"`
	Name          string `json:"name"`
	GroupID       string `json:"group_id"`
	ListLevel     int    `json:"list_level"`
	ReadLevel     int    `json:"read_level"`
	WriteLevel    int    `json:"write_level"`
	ReplyLevel    int    `json:"reply_level"`
	CommentLevel  int    `json:"comment_level"`
	UploadLevel   int    `json:"upload_level"`
	DownloadLevel int    `json:"download_level"`
	Order         int    `json:"order"`
	UseCategory   bool   `json:"use_category"`
	CategoryList  string `json:"category_list"`
	WritePoint    int    `json:"write_point"`
	CommentPoint  int    `json:"comment_point"`
	ReadPoint     int    `json:"read_point"`
	DownloadPoint int    `json:"download_point"`
	UseGood       bool   `json:"use_good"`
	UseNogood     bool   `json:"use_nogood"`
	UseSecret     bool   `json:"use_secret"`
	PostCount     int    `json:"post_count"`
	CommentCount  int    `json:"comment_count"`
}

// ToResponse converts G5Board to API response format
func (b *G5Board) ToResponse() BoardResponse {
	return BoardResponse{
		ID:            b.BoTable,
		Slug:          b.BoTable,
		Name:          b.BoSubject,
		GroupID:       b.GrID,
		ListLevel:     b.BoListLevel,
		ReadLevel:     b.BoReadLevel,
		WriteLevel:    b.BoWriteLevel,
		ReplyLevel:    b.BoReplyLevel,
		CommentLevel:  b.BoCommentLevel,
		UploadLevel:   b.BoUploadLevel,
		DownloadLevel: b.BoDownloadLevel,
		Order:         b.BoOrder,
		UseCategory:   b.BoUseCategory == 1,
		CategoryList:  b.BoCategoryList,
		WritePoint:    b.BoWritePoint,
		CommentPoint:  b.BoCommentPoint,
		ReadPoint:     b.BoReadPoint,
		DownloadPoint: b.BoDownloadPoint,
		UseGood:       b.BoUseGood == 1,
		UseNogood:     b.BoUseNogood == 1,
		UseSecret:     b.BoUseSecret == 1,
		PostCount:     b.BoCountWrite,
		CommentCount:  b.BoCountComment,
	}
}

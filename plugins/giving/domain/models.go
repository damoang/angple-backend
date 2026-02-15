// 나눔 플러그인 도메인 모델
package domain

import "time"

// GivingPost 나눔 게시글 (g5_write_giving)
type GivingPost struct {
	WrID        int       `gorm:"column:wr_id;primaryKey" json:"wr_id"`
	WrSubject   string    `gorm:"column:wr_subject" json:"wr_subject"`
	WrName      string    `gorm:"column:wr_name" json:"wr_name"`
	WrDatetime  time.Time `gorm:"column:wr_datetime" json:"wr_datetime"`
	WrHit       int       `gorm:"column:wr_hit" json:"wr_hit"`
	WrGood      int       `gorm:"column:wr_good" json:"wr_good"`
	WrComment   int       `gorm:"column:wr_comment" json:"wr_comment"`
	WrIsComment int       `gorm:"column:wr_is_comment" json:"wr_is_comment"`
	MbID        string    `gorm:"column:mb_id" json:"mb_id"`
	Wr2         string    `gorm:"column:wr_2" json:"wr_2"`   // 번호당 포인트
	Wr3         string    `gorm:"column:wr_3" json:"wr_3"`   // 상품명
	Wr4         string    `gorm:"column:wr_4" json:"wr_4"`   // 시작일시
	Wr5         string    `gorm:"column:wr_5" json:"wr_5"`   // 종료일시
	Wr6         string    `gorm:"column:wr_6" json:"wr_6"`   // 배송유형
	Wr7         string    `gorm:"column:wr_7" json:"wr_7"`   // 상태 (0:진행, 1:일시정지, 2:강제종료)
	Wr8         string    `gorm:"column:wr_8" json:"wr_8"`   // 일시정지 시각
	Wr10        string    `gorm:"column:wr_10" json:"wr_10"` // 썸네일
}

func (GivingPost) TableName() string {
	return "g5_write_giving"
}

// GivingBid 나눔 응모 (g5_giving_bid)
type GivingBid struct {
	BidID       int       `gorm:"column:bid_id;primaryKey;autoIncrement" json:"bid_id"`
	WrID        int       `gorm:"column:wr_id;not null" json:"wr_id"`
	MbID        string    `gorm:"column:mb_id;size:50;not null" json:"mb_id"`
	MbNick      string    `gorm:"column:mb_nick;size:50" json:"mb_nick"`
	BidNumbers  string    `gorm:"column:bid_numbers;type:text" json:"bid_numbers"`
	BidCount    int       `gorm:"column:bid_count" json:"bid_count"`
	BidPoints   int       `gorm:"column:bid_points" json:"bid_points"`
	BidDatetime time.Time `gorm:"column:bid_datetime" json:"bid_datetime"`
	BidStatus   string    `gorm:"column:bid_status;size:20;default:active" json:"bid_status"`
}

func (GivingBid) TableName() string {
	return "g5_giving_bid"
}

// GivingBidNumber 나눔 응모 번호 (g5_giving_bid_numbers)
type GivingBidNumber struct {
	ID        int    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	WrID      int    `gorm:"column:wr_id;not null" json:"wr_id"`
	BidID     int    `gorm:"column:bid_id;not null" json:"bid_id"`
	MbID      string `gorm:"column:mb_id;size:50;not null" json:"mb_id"`
	BidNumber int    `gorm:"column:bid_number;not null" json:"bid_number"`
	BidStatus string `gorm:"column:bid_status;size:20;default:active" json:"bid_status"`
}

func (GivingBidNumber) TableName() string {
	return "g5_giving_bid_numbers"
}

// Member 회원 (g5_member) - 포인트 조회용
type Member struct {
	MbID    string `gorm:"column:mb_id;primaryKey" json:"mb_id"`
	MbNick  string `gorm:"column:mb_nick" json:"mb_nick"`
	MbPoint int    `gorm:"column:mb_point" json:"mb_point"`
}

func (Member) TableName() string {
	return "g5_member"
}

// PointLog 포인트 내역 (g5_point)
type PointLog struct {
	PoID        int       `gorm:"column:po_id;primaryKey;autoIncrement" json:"po_id"`
	MbID        string    `gorm:"column:mb_id;size:50;not null" json:"mb_id"`
	PoContent   string    `gorm:"column:po_content;size:255" json:"po_content"`
	PoPoint     int       `gorm:"column:po_point" json:"po_point"`
	PoDatetime  time.Time `gorm:"column:po_datetime" json:"po_datetime"`
	PoRelTable  string    `gorm:"column:po_rel_table;size:50" json:"po_rel_table"`
	PoRelID     string    `gorm:"column:po_rel_id;size:50" json:"po_rel_id"`
	PoRelAction string    `gorm:"column:po_rel_action;size:50" json:"po_rel_action"`
}

func (PointLog) TableName() string {
	return "g5_point"
}

// BoardFile 게시판 첨부파일 (g5_board_file)
type BoardFile struct {
	BoTable string `gorm:"column:bo_table" json:"bo_table"`
	WrID    int    `gorm:"column:wr_id" json:"wr_id"`
	BfNo    int    `gorm:"column:bf_no" json:"bf_no"`
	BfFile  string `gorm:"column:bf_file" json:"bf_file"`
}

func (BoardFile) TableName() string {
	return "g5_board_file"
}

// --- Response DTOs ---

// GivingListItem 나눔 목록 아이템 응답
type GivingListItem struct {
	ID               int    `json:"id"`
	Title            string `json:"title"`
	Content          string `json:"content"`
	Author           string `json:"author"`
	AuthorID         string `json:"author_id"`
	Views            int    `json:"views"`
	Likes            int    `json:"likes"`
	CommentsCount    int    `json:"comments_count"`
	CreatedAt        string `json:"created_at"`
	Thumbnail        string `json:"thumbnail"`
	Extra2           string `json:"extra_2"`
	Extra3           string `json:"extra_3"`
	Extra4           string `json:"extra_4"`
	Extra5           string `json:"extra_5"`
	Extra6           string `json:"extra_6"`
	Extra7           string `json:"extra_7"`
	Extra10          string `json:"extra_10"`
	ParticipantCount string `json:"participant_count"`
	IsUrgent         bool   `json:"is_urgent"`
}

// GivingDetailResponse 나눔 상세 응답
type GivingDetailResponse struct {
	TotalParticipants int         `json:"totalParticipants"`
	TotalBidCount     int         `json:"totalBidCount"`
	MyBids            []GivingBid `json:"myBids"`
	Winner            *WinnerInfo `json:"winner"`
}

// WinnerInfo 당첨자 정보
type WinnerInfo struct {
	MbID          string `json:"mb_id"`
	MbNick        string `json:"mb_nick"`
	WinningNumber int    `json:"winning_number"`
}

// BidRequest 응모 요청
type BidRequest struct {
	Numbers string `json:"numbers" binding:"required"`
}

// BidResponse 응모 결과 응답
type BidResponse struct {
	BidID      int   `json:"bid_id"`
	Numbers    []int `json:"numbers"`
	PointsUsed int   `json:"points_used"`
}

// NumberCount 번호별 카운트
type NumberCount struct {
	BidNumber int `gorm:"column:bid_number" json:"bid_number"`
	Count     int `gorm:"column:cnt" json:"count"`
}

// ParticipantCount 게시글별 참여자 수
type ParticipantCount struct {
	WrID               int `gorm:"column:wr_id" json:"wr_id"`
	UniqueParticipants int `gorm:"column:unique_participants" json:"unique_participants"`
}

// BidStats 응모 통계
type BidStats struct {
	UniqueParticipants int `gorm:"column:unique_participants" json:"unique_participants"`
	TotalBidCount      int `gorm:"column:total_bid_count" json:"total_bid_count"`
}

// AdminStats 관리자 통계
type AdminStats struct {
	PostID             int           `json:"post_id"`
	UniqueParticipants int           `json:"unique_participants"`
	TotalBidCount      int           `json:"total_bid_count"`
	TotalPointsUsed    int           `json:"total_points_used"`
	NumberDistribution []NumberCount `json:"number_distribution"`
	RecentBids         []GivingBid   `json:"recent_bids"`
}

// VisualizationResponse 번호 분포 시각화 응답
type VisualizationResponse struct {
	Numbers []NumberCount `json:"numbers"`
	Winner  *WinnerInfo   `json:"winner"`
}

// LiveStatusResponse 실시간 현황 응답
type LiveStatusResponse struct {
	ParticipantCount int `json:"participant_count"`
	TotalBidCount    int `json:"total_bid_count"`
}

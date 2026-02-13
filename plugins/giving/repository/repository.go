//go:build ignore

// 나눔 플러그인 Repository
package repository

import (
	"fmt"
	"time"

	"angple-backend/plugins/giving/domain"

	"gorm.io/gorm"
)

// GivingRepository 나눔 데이터 접근 인터페이스
type GivingRepository interface {
	// 목록 조회
	ListActive(sort string, limit, offset int) ([]domain.GivingPost, int64, error)
	ListEnded(limit, offset int) ([]domain.GivingPost, int64, error)

	// 참여자 수 조회
	GetParticipantCounts(wrIDs []int) (map[int]int, error)

	// 첨부파일 조회 (첫 번째만)
	GetFirstFiles(wrIDs []int) (map[int]string, error)

	// 상세 조회
	GetPost(wrID int) (*domain.GivingPost, error)
	GetBidStats(wrID int) (*domain.BidStats, error)
	GetUserBids(wrID int, mbID string) ([]domain.GivingBid, error)

	// 당첨자 계산
	GetNumberCounts(wrID int) ([]domain.NumberCount, error)
	GetWinnerByNumber(wrID int, bidNumber int) (*domain.WinnerInfo, error)

	// 응모
	GetMemberPoints(mbID string) (int, error)
	GetDuplicateNumbers(wrID int, mbID string, numbers []int) ([]int, error)
	CreateBid(tx *gorm.DB, bid *domain.GivingBid) (int, error)
	CreateBidNumbers(tx *gorm.DB, numbers []domain.GivingBidNumber) error
	DeductPoints(tx *gorm.DB, mbID string, points int) error
	AddPoints(tx *gorm.DB, mbID string, points int) error
	InsertPointLog(tx *gorm.DB, log *domain.PointLog) error

	// 관리자
	PausePost(wrID int) error
	ResumePost(wrID int) error
	ForceStopPost(wrID int) error
	GetAdminStats(wrID int) (*domain.AdminStats, error)

	// 라이브 현황
	GetLiveStatus(wrID int) (*domain.LiveStatusResponse, error)

	// 트랜잭션
	Begin() *gorm.DB
}

type givingRepository struct {
	db *gorm.DB
}

// NewGivingRepository Repository 생성자
func NewGivingRepository(db *gorm.DB) GivingRepository {
	return &givingRepository{db: db}
}

func (r *givingRepository) ListActive(sort string, limit, offset int) ([]domain.GivingPost, int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")

	where := "wr_is_comment = 0 AND (wr_5 = '' OR wr_5 > ?) AND (wr_4 = '' OR wr_4 <= ?) AND (wr_7 IS NULL OR wr_7 = '' OR wr_7 = '0')"

	var total int64
	if err := r.db.Model(&domain.GivingPost{}).Where(where, now, now).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var orderClause string
	if sort == "urgent" {
		orderClause = "CASE WHEN wr_5 != '' THEN wr_5 END ASC, wr_datetime DESC"
	} else {
		orderClause = "wr_datetime DESC"
	}

	var posts []domain.GivingPost
	if err := r.db.Where(where, now, now).
		Order(orderClause).
		Limit(limit).
		Offset(offset).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

func (r *givingRepository) ListEnded(limit, offset int) ([]domain.GivingPost, int64, error) {
	now := time.Now().Format("2006-01-02 15:04:05")

	where := "wr_is_comment = 0 AND ((wr_5 != '' AND wr_5 <= ?) OR wr_7 = '2')"

	var total int64
	if err := r.db.Model(&domain.GivingPost{}).Where(where, now).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var posts []domain.GivingPost
	if err := r.db.Where(where, now).
		Order("wr_datetime DESC").
		Limit(limit).
		Offset(offset).
		Find(&posts).Error; err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

func (r *givingRepository) GetParticipantCounts(wrIDs []int) (map[int]int, error) {
	if len(wrIDs) == 0 {
		return map[int]int{}, nil
	}

	var results []domain.ParticipantCount
	if err := r.db.Model(&domain.GivingBid{}).
		Select("wr_id, COUNT(DISTINCT mb_id) as unique_participants").
		Where("wr_id IN ? AND bid_status = 'active'", wrIDs).
		Group("wr_id").
		Scan(&results).Error; err != nil {
		return nil, err
	}

	m := make(map[int]int)
	for _, row := range results {
		m[row.WrID] = row.UniqueParticipants
	}
	return m, nil
}

func (r *givingRepository) GetFirstFiles(wrIDs []int) (map[int]string, error) {
	if len(wrIDs) == 0 {
		return map[int]string{}, nil
	}

	var files []domain.BoardFile
	if err := r.db.Where("bo_table = 'giving' AND wr_id IN ? AND bf_file != ''", wrIDs).
		Order("bf_no").
		Find(&files).Error; err != nil {
		return nil, err
	}

	m := make(map[int]string)
	for _, f := range files {
		if _, exists := m[f.WrID]; !exists {
			m[f.WrID] = f.BfFile
		}
	}
	return m, nil
}

func (r *givingRepository) GetPost(wrID int) (*domain.GivingPost, error) {
	var post domain.GivingPost
	if err := r.db.Where("wr_id = ? AND wr_is_comment = 0", wrID).First(&post).Error; err != nil {
		return nil, err
	}
	return &post, nil
}

func (r *givingRepository) GetBidStats(wrID int) (*domain.BidStats, error) {
	var stats domain.BidStats
	if err := r.db.Model(&domain.GivingBid{}).
		Select("COUNT(DISTINCT mb_id) as unique_participants, COALESCE(SUM(bid_count), 0) as total_bid_count").
		Where("wr_id = ? AND bid_status = 'active'", wrID).
		Scan(&stats).Error; err != nil {
		return nil, err
	}
	return &stats, nil
}

func (r *givingRepository) GetUserBids(wrID int, mbID string) ([]domain.GivingBid, error) {
	var bids []domain.GivingBid
	if err := r.db.
		Where("wr_id = ? AND mb_id = ? AND bid_status = 'active'", wrID, mbID).
		Order("bid_datetime DESC").
		Find(&bids).Error; err != nil {
		return nil, err
	}
	return bids, nil
}

func (r *givingRepository) GetNumberCounts(wrID int) ([]domain.NumberCount, error) {
	var counts []domain.NumberCount
	if err := r.db.Model(&domain.GivingBidNumber{}).
		Select("bid_number, COUNT(*) as cnt").
		Where("wr_id = ? AND bid_status = 'active'", wrID).
		Group("bid_number").
		Order("bid_number ASC").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (r *givingRepository) GetWinnerByNumber(wrID int, bidNumber int) (*domain.WinnerInfo, error) {
	var result struct {
		MbID   string `gorm:"column:mb_id"`
		MbNick string `gorm:"column:mb_nick"`
	}

	if err := r.db.Table("g5_giving_bid_numbers bn").
		Select("b.mb_id, b.mb_nick").
		Joins("JOIN g5_giving_bid b ON bn.bid_id = b.bid_id").
		Where("bn.wr_id = ? AND bn.bid_number = ? AND bn.bid_status = 'active'", wrID, bidNumber).
		Limit(1).
		Scan(&result).Error; err != nil {
		return nil, err
	}

	if result.MbID == "" {
		return nil, nil
	}

	return &domain.WinnerInfo{
		MbID:          result.MbID,
		MbNick:        result.MbNick,
		WinningNumber: bidNumber,
	}, nil
}

func (r *givingRepository) GetMemberPoints(mbID string) (int, error) {
	var member domain.Member
	if err := r.db.Select("mb_point").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return 0, err
	}
	return member.MbPoint, nil
}

func (r *givingRepository) GetDuplicateNumbers(wrID int, mbID string, numbers []int) ([]int, error) {
	if len(numbers) == 0 {
		return nil, nil
	}

	var existing []int
	if err := r.db.Model(&domain.GivingBidNumber{}).
		Select("bid_number").
		Where("wr_id = ? AND mb_id = ? AND bid_status = 'active' AND bid_number IN ?", wrID, mbID, numbers).
		Pluck("bid_number", &existing).Error; err != nil {
		return nil, err
	}
	return existing, nil
}

func (r *givingRepository) CreateBid(tx *gorm.DB, bid *domain.GivingBid) (int, error) {
	bid.BidDatetime = time.Now()
	bid.BidStatus = "active"
	if err := tx.Create(bid).Error; err != nil {
		return 0, err
	}
	return bid.BidID, nil
}

func (r *givingRepository) CreateBidNumbers(tx *gorm.DB, numbers []domain.GivingBidNumber) error {
	if len(numbers) == 0 {
		return nil
	}
	return tx.Create(&numbers).Error
}

func (r *givingRepository) DeductPoints(tx *gorm.DB, mbID string, points int) error {
	return tx.Model(&domain.Member{}).
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point - ?", points)).Error
}

func (r *givingRepository) AddPoints(tx *gorm.DB, mbID string, points int) error {
	return tx.Model(&domain.Member{}).
		Where("mb_id = ?", mbID).
		UpdateColumn("mb_point", gorm.Expr("mb_point + ?", points)).Error
}

func (r *givingRepository) InsertPointLog(tx *gorm.DB, log *domain.PointLog) error {
	log.PoDatetime = time.Now()
	return tx.Create(log).Error
}

func (r *givingRepository) PausePost(wrID int) error {
	now := time.Now().Format("2006-01-02 15:04:05")
	return r.db.Model(&domain.GivingPost{}).
		Where("wr_id = ?", wrID).
		Updates(map[string]interface{}{
			"wr_7": "1",
			"wr_8": now,
		}).Error
}

func (r *givingRepository) ResumePost(wrID int) error {
	// 일시정지 시각 조회
	var post domain.GivingPost
	if err := r.db.Select("wr_5, wr_8").Where("wr_id = ?", wrID).First(&post).Error; err != nil {
		return err
	}

	updates := map[string]interface{}{
		"wr_7": "0",
		"wr_8": "",
	}

	// 정지 기간만큼 종료시간 연장
	if post.Wr5 != "" && post.Wr8 != "" {
		endTime, err1 := time.Parse("2006-01-02 15:04:05", post.Wr5)
		pauseTime, err2 := time.Parse("2006-01-02 15:04:05", post.Wr8)
		if err1 == nil && err2 == nil {
			pauseDuration := time.Since(pauseTime)
			newEndTime := endTime.Add(pauseDuration)
			updates["wr_5"] = newEndTime.Format("2006-01-02 15:04:05")
		}
	}

	return r.db.Model(&domain.GivingPost{}).
		Where("wr_id = ?", wrID).
		Updates(updates).Error
}

func (r *givingRepository) ForceStopPost(wrID int) error {
	return r.db.Model(&domain.GivingPost{}).
		Where("wr_id = ?", wrID).
		UpdateColumn("wr_7", "2").Error
}

func (r *givingRepository) GetAdminStats(wrID int) (*domain.AdminStats, error) {
	stats := &domain.AdminStats{PostID: wrID}

	// 기본 통계
	bidStats, err := r.GetBidStats(wrID)
	if err != nil {
		return nil, err
	}
	stats.UniqueParticipants = bidStats.UniqueParticipants
	stats.TotalBidCount = bidStats.TotalBidCount

	// 총 사용 포인트
	var totalPoints struct {
		Total int `gorm:"column:total"`
	}
	if err := r.db.Model(&domain.GivingBid{}).
		Select("COALESCE(SUM(bid_points), 0) as total").
		Where("wr_id = ? AND bid_status = 'active'", wrID).
		Scan(&totalPoints).Error; err != nil {
		return nil, err
	}
	stats.TotalPointsUsed = totalPoints.Total

	// 번호 분포
	numberCounts, err := r.GetNumberCounts(wrID)
	if err != nil {
		return nil, err
	}
	stats.NumberDistribution = numberCounts

	// 최근 응모 10건
	var recentBids []domain.GivingBid
	if err := r.db.Where("wr_id = ? AND bid_status = 'active'", wrID).
		Order("bid_datetime DESC").
		Limit(10).
		Find(&recentBids).Error; err != nil {
		return nil, err
	}
	stats.RecentBids = recentBids

	return stats, nil
}

func (r *givingRepository) GetLiveStatus(wrID int) (*domain.LiveStatusResponse, error) {
	var result struct {
		ParticipantCount int `gorm:"column:participant_count"`
		TotalBidCount    int `gorm:"column:total_bid_count"`
	}

	if err := r.db.Model(&domain.GivingBid{}).
		Select("COUNT(DISTINCT mb_id) as participant_count, COALESCE(SUM(bid_count), 0) as total_bid_count").
		Where("wr_id = ? AND bid_status = 'active'", wrID).
		Scan(&result).Error; err != nil {
		return nil, fmt.Errorf("live status query failed: %w", err)
	}

	return &domain.LiveStatusResponse{
		ParticipantCount: result.ParticipantCount,
		TotalBidCount:    result.TotalBidCount,
	}, nil
}

func (r *givingRepository) Begin() *gorm.DB {
	return r.db.Begin()
}

// 나눔 플러그인 비즈니스 로직
package service

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/plugins/giving/domain"
	"github.com/damoang/angple-backend/plugins/giving/repository"

	"gorm.io/gorm"
)

// GivingService 나눔 서비스 인터페이스
type GivingService interface {
	// 목록
	ListGivings(tab, sort string, page, limit int) ([]domain.GivingListItem, int64, error)

	// 상세
	GetDetail(wrID int, mbID string) (*domain.GivingDetailResponse, error)

	// 응모
	CreateBid(wrID int, mbID, mbNick, numbersInput string) (*domain.BidResponse, error)
	GetMyBids(wrID int, mbID string) ([]domain.GivingBid, error)

	// 관리자
	PauseGiving(wrID int) error
	ResumeGiving(wrID int) error
	ForceStopGiving(wrID int) error
	GetAdminStats(wrID int) (*domain.AdminStats, error)

	// 공개
	GetVisualization(wrID int) (*domain.VisualizationResponse, error)
	GetLiveStatus(wrID int) (*domain.LiveStatusResponse, error)
}

type givingService struct {
	repo repository.GivingRepository
}

// NewGivingService 서비스 생성자
func NewGivingService(repo repository.GivingRepository) GivingService {
	return &givingService{repo: repo}
}

func (s *givingService) ListGivings(tab, sort string, page, limit int) ([]domain.GivingListItem, int64, error) {
	if limit < 1 {
		limit = 20
	}
	if limit > 40 {
		limit = 40
	}
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	var posts []domain.GivingPost
	var total int64
	var err error

	if tab == "ended" {
		posts, total, err = s.repo.ListEnded(limit, offset)
	} else {
		posts, total, err = s.repo.ListActive(sort, limit, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("목록 조회 실패: %w", err)
	}

	if len(posts) == 0 {
		return []domain.GivingListItem{}, total, nil
	}

	// 게시글 ID 수집
	wrIDs := make([]int, len(posts))
	for i, p := range posts {
		wrIDs[i] = p.WrID
	}

	// 참여자 수 조회
	participantMap, err := s.repo.GetParticipantCounts(wrIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("참여자 수 조회 실패: %w", err)
	}

	// 첨부파일 조회
	fileMap, err := s.repo.GetFirstFiles(wrIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("첨부파일 조회 실패: %w", err)
	}

	now := time.Now()
	items := make([]domain.GivingListItem, len(posts))
	for i, post := range posts {
		// 긴급 여부 계산
		isUrgent := false
		if post.Wr5 != "" {
			if endTime, err := time.Parse("2006-01-02 15:04:05", post.Wr5); err == nil {
				diff := endTime.Sub(now)
				isUrgent = diff > 0 && diff <= 24*time.Hour
			}
		}

		// 썸네일 처리
		thumbnail := post.Wr10
		if thumbnail == "" {
			if fileURL, ok := fileMap[post.WrID]; ok {
				thumbnail = "https://damoang.net/data/file/giving/" + fileURL
			}
		}

		// S3 URL → Lambda 썸네일 변환
		if thumbnail != "" && isS3DamoangURL(thumbnail) {
			thumbnail = convertToThumbnailURL(thumbnail)
		}

		extra10 := post.Wr10
		if extra10 == "" {
			extra10 = thumbnail
		}

		items[i] = domain.GivingListItem{
			ID:               post.WrID,
			Title:            post.WrSubject,
			Content:          "",
			Author:           post.WrName,
			AuthorID:         "",
			Views:            post.WrHit,
			Likes:            post.WrGood,
			CommentsCount:    post.WrComment,
			CreatedAt:        post.WrDatetime.Format("2006-01-02 15:04:05"),
			Thumbnail:        thumbnail,
			Extra2:           post.Wr2,
			Extra3:           post.Wr3,
			Extra4:           post.Wr4,
			Extra5:           post.Wr5,
			Extra6:           post.Wr6,
			Extra7:           post.Wr7,
			Extra10:          extra10,
			ParticipantCount: strconv.Itoa(participantMap[post.WrID]),
			IsUrgent:         isUrgent,
		}
	}

	return items, total, nil
}

func (s *givingService) GetDetail(wrID int, mbID string) (*domain.GivingDetailResponse, error) {
	// 응모 통계
	stats, err := s.repo.GetBidStats(wrID)
	if err != nil {
		return nil, fmt.Errorf("통계 조회 실패: %w", err)
	}

	resp := &domain.GivingDetailResponse{
		TotalParticipants: stats.UniqueParticipants,
		TotalBidCount:     stats.TotalBidCount,
		MyBids:            []domain.GivingBid{},
	}

	// 로그인 사용자의 응모 현황
	if mbID != "" {
		bids, err := s.repo.GetUserBids(wrID, mbID)
		if err != nil {
			return nil, fmt.Errorf("사용자 응모 조회 실패: %w", err)
		}
		if bids != nil {
			resp.MyBids = bids
		}
	}

	// 종료 여부 확인 → 당첨자 계산
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		return nil, fmt.Errorf("게시글 조회 실패: %w", err)
	}

	isEnded := post.Wr7 == "2"
	if !isEnded && post.Wr5 != "" {
		if endTime, err := time.Parse("2006-01-02 15:04:05", post.Wr5); err == nil {
			isEnded = endTime.Before(time.Now()) || endTime.Equal(time.Now())
		}
	}

	if isEnded {
		winner, err := s.calculateWinner(wrID)
		if err != nil {
			return nil, fmt.Errorf("당첨자 계산 실패: %w", err)
		}
		resp.Winner = winner
	}

	return resp, nil
}

func (s *givingService) CreateBid(wrID int, mbID, mbNick, numbersInput string) (*domain.BidResponse, error) {
	// 번호 파싱
	numbers := parseBidNumbers(numbersInput)
	if len(numbers) == 0 {
		return nil, fmt.Errorf("유효한 번호가 없습니다")
	}
	if len(numbers) > 100 {
		return nil, fmt.Errorf("한 번에 최대 100개 번호까지 응모 가능합니다")
	}

	// 게시글 확인
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("나눔을 찾을 수 없습니다")
		}
		return nil, fmt.Errorf("게시글 조회 실패: %w", err)
	}

	// 상태 확인
	if post.Wr7 == "1" || post.Wr7 == "2" {
		return nil, fmt.Errorf("이 나눔은 현재 응모할 수 없습니다")
	}

	now := time.Now()
	if post.Wr5 != "" {
		if endTime, err := time.Parse("2006-01-02 15:04:05", post.Wr5); err == nil {
			if !endTime.After(now) {
				return nil, fmt.Errorf("종료된 나눔입니다")
			}
		}
	}

	if post.Wr4 != "" {
		if startTime, err := time.Parse("2006-01-02 15:04:05", post.Wr4); err == nil {
			if startTime.After(now) {
				return nil, fmt.Errorf("아직 시작되지 않은 나눔입니다")
			}
		}
	}

	// 번호당 포인트
	pointsPerNumber, _ := strconv.Atoi(post.Wr2)
	totalPoints := len(numbers) * pointsPerNumber

	// 사용자 포인트 확인
	currentPoints, err := s.repo.GetMemberPoints(mbID)
	if err != nil {
		return nil, fmt.Errorf("포인트 조회 실패: %w", err)
	}

	if currentPoints < totalPoints {
		return nil, fmt.Errorf("포인트가 부족합니다. (필요: %d, 보유: %d)", totalPoints, currentPoints)
	}

	// 중복 번호 체크
	dupes, err := s.repo.GetDuplicateNumbers(wrID, mbID, numbers)
	if err != nil {
		return nil, fmt.Errorf("중복 확인 실패: %w", err)
	}
	if len(dupes) > 0 {
		dupeStrs := make([]string, len(dupes))
		for i, d := range dupes {
			dupeStrs[i] = strconv.Itoa(d)
		}
		return nil, fmt.Errorf("이미 응모한 번호가 있습니다: %s", strings.Join(dupeStrs, ", "))
	}

	// 트랜잭션으로 응모 처리
	tx := s.repo.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("트랜잭션 시작 실패: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 응모 레코드 생성
	bid := &domain.GivingBid{
		WrID:       wrID,
		MbID:       mbID,
		MbNick:     mbNick,
		BidNumbers: numbersInput,
		BidCount:   len(numbers),
		BidPoints:  totalPoints,
	}
	bidID, err := s.repo.CreateBid(tx, bid)
	if err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("응모 생성 실패: %w", err)
	}

	// 2. 개별 번호 레코드 생성
	bidNumbers := make([]domain.GivingBidNumber, len(numbers))
	for i, n := range numbers {
		bidNumbers[i] = domain.GivingBidNumber{
			WrID:      wrID,
			BidID:     bidID,
			MbID:      mbID,
			BidNumber: n,
			BidStatus: "active",
		}
	}
	if err := s.repo.CreateBidNumbers(tx, bidNumbers); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("응모 번호 생성 실패: %w", err)
	}

	// 3. 포인트 차감 (응모자)
	if err := s.repo.DeductPoints(tx, mbID, totalPoints); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("포인트 차감 실패: %w", err)
	}

	// 4. 포인트 내역 기록 (응모자 차감)
	if err := s.repo.InsertPointLog(tx, &domain.PointLog{
		MbID:        mbID,
		PoContent:   fmt.Sprintf("나눔 응모 (%d개 번호)", len(numbers)),
		PoPoint:     -totalPoints,
		PoRelTable:  "giving",
		PoRelID:     strconv.Itoa(wrID),
		PoRelAction: "bid",
	}); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("포인트 내역 기록 실패: %w", err)
	}

	// 5. 수수료 50%: 글작성자에게 포인트 지급
	if post.MbID != "" && post.MbID != mbID {
		authorPoints := int(math.Floor(float64(totalPoints) * 0.5))
		if authorPoints > 0 {
			if err := s.repo.AddPoints(tx, post.MbID, authorPoints); err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("수수료 지급 실패: %w", err)
			}

			if err := s.repo.InsertPointLog(tx, &domain.PointLog{
				MbID:        post.MbID,
				PoContent:   "나눔 응모 수수료",
				PoPoint:     authorPoints,
				PoRelTable:  "giving",
				PoRelID:     strconv.Itoa(wrID),
				PoRelAction: "bid_commission",
			}); err != nil {
				tx.Rollback()
				return nil, fmt.Errorf("수수료 내역 기록 실패: %w", err)
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("트랜잭션 커밋 실패: %w", err)
	}

	return &domain.BidResponse{
		BidID:      bidID,
		Numbers:    numbers,
		PointsUsed: totalPoints,
	}, nil
}

func (s *givingService) GetMyBids(wrID int, mbID string) ([]domain.GivingBid, error) {
	bids, err := s.repo.GetUserBids(wrID, mbID)
	if err != nil {
		return nil, fmt.Errorf("응모 조회 실패: %w", err)
	}
	if bids == nil {
		return []domain.GivingBid{}, nil
	}
	return bids, nil
}

func (s *givingService) PauseGiving(wrID int) error {
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		return fmt.Errorf("게시글 조회 실패: %w", err)
	}
	if post.Wr7 == "2" {
		return fmt.Errorf("이미 종료된 나눔입니다")
	}
	if post.Wr7 == "1" {
		return fmt.Errorf("이미 일시정지 상태입니다")
	}
	return s.repo.PausePost(wrID)
}

func (s *givingService) ResumeGiving(wrID int) error {
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		return fmt.Errorf("게시글 조회 실패: %w", err)
	}
	if post.Wr7 != "1" {
		return fmt.Errorf("일시정지 상태가 아닙니다")
	}
	return s.repo.ResumePost(wrID)
}

func (s *givingService) ForceStopGiving(wrID int) error {
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		return fmt.Errorf("게시글 조회 실패: %w", err)
	}
	if post.Wr7 == "2" {
		return fmt.Errorf("이미 종료된 나눔입니다")
	}
	return s.repo.ForceStopPost(wrID)
}

func (s *givingService) GetAdminStats(wrID int) (*domain.AdminStats, error) {
	return s.repo.GetAdminStats(wrID)
}

func (s *givingService) GetVisualization(wrID int) (*domain.VisualizationResponse, error) {
	// 종료 여부 확인
	post, err := s.repo.GetPost(wrID)
	if err != nil {
		return nil, fmt.Errorf("게시글 조회 실패: %w", err)
	}

	isEnded := post.Wr7 == "2"
	if !isEnded && post.Wr5 != "" {
		if endTime, err := time.Parse("2006-01-02 15:04:05", post.Wr5); err == nil {
			isEnded = endTime.Before(time.Now()) || endTime.Equal(time.Now())
		}
	}

	if !isEnded {
		return nil, fmt.Errorf("진행중인 나눔의 번호 분포는 조회할 수 없습니다")
	}

	numbers, err := s.repo.GetNumberCounts(wrID)
	if err != nil {
		return nil, fmt.Errorf("번호 분포 조회 실패: %w", err)
	}

	winner, err := s.calculateWinner(wrID)
	if err != nil {
		return nil, fmt.Errorf("당첨자 계산 실패: %w", err)
	}

	return &domain.VisualizationResponse{
		Numbers: numbers,
		Winner:  winner,
	}, nil
}

func (s *givingService) GetLiveStatus(wrID int) (*domain.LiveStatusResponse, error) {
	return s.repo.GetLiveStatus(wrID)
}

// --- 내부 헬퍼 함수 ---

// calculateWinner 최저고유번호 당첨자 계산
func (s *givingService) calculateWinner(wrID int) (*domain.WinnerInfo, error) {
	numberCounts, err := s.repo.GetNumberCounts(wrID)
	if err != nil {
		return nil, err
	}

	// 고유번호 (1번만 선택된 번호) 중 최저 찾기
	for _, nc := range numberCounts {
		if nc.Count == 1 {
			return s.repo.GetWinnerByNumber(wrID, nc.BidNumber)
		}
	}

	return nil, nil
}

// isS3DamoangURL S3 URL 여부 확인
func isS3DamoangURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Hostname() == "s3.damoang.net"
}

// s3ThumbnailRegex S3 URL 파싱용 정규식
var s3ThumbnailRegex = regexp.MustCompile(`^(https?://s3\.damoang\.net/.+/)([^/]+)\.([a-zA-Z0-9]+)$`)

// convertToThumbnailURL S3 URL → Lambda 썸네일 URL 변환
func convertToThumbnailURL(urlStr string) string {
	matches := s3ThumbnailRegex.FindStringSubmatch(urlStr)
	if len(matches) == 4 {
		return matches[1] + matches[2] + "-400x225.webp"
	}
	return urlStr
}

// parseBidNumbers 번호 문자열 파싱: "1,3,5-10,15~20" → [1,3,5,6,7,8,9,10,15,16,17,18,19,20]
func parseBidNumbers(input string) []int {
	numbers := make(map[int]struct{})
	parts := strings.Split(input, ",")

	rangeRegex := regexp.MustCompile(`^\s*(\d+)\s*[-~]\s*(\d+)\s*$`)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if matches := rangeRegex.FindStringSubmatch(part); len(matches) == 3 {
			start, _ := strconv.Atoi(matches[1])
			end, _ := strconv.Atoi(matches[2])
			if start > end {
				start, end = end, start
			}
			for i := start; i <= end; i++ {
				numbers[i] = struct{}{}
			}
		} else {
			num, err := strconv.Atoi(part)
			if err == nil && num > 0 {
				numbers[num] = struct{}{}
			}
		}
	}

	result := make([]int, 0, len(numbers))
	for n := range numbers {
		result = append(result, n)
	}

	// 정렬
	sortInts(result)
	return result
}

// sortInts 정수 배열 정렬 (sort 패키지 import 없이)
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}

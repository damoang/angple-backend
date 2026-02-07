package service

import (
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"github.com/damoang/angple-backend/internal/pluginstore/repository"
)

// MarketplaceService 플러그인 마켓플레이스 비즈니스 로직
type MarketplaceService struct {
	repo *repository.MarketplaceRepository
}

// NewMarketplaceService 생성자
func NewMarketplaceService(repo *repository.MarketplaceRepository) *MarketplaceService {
	return &MarketplaceService{repo: repo}
}

// === Public API ===

// Browse 마켓플레이스 탐색 (승인된 플러그인만)
func (s *MarketplaceService) Browse(page, perPage int, category, keyword string) ([]domain.MarketplaceListItem, int64, error) {
	subs, total, err := s.repo.FindApprovedSubmissions(page, perPage, category, keyword)
	if err != nil {
		return nil, 0, err
	}

	items := make([]domain.MarketplaceListItem, len(subs))
	for i, sub := range subs {
		avgRating, reviewCount, _ := s.repo.GetAverageRating(sub.PluginName) //nolint:errcheck // rating lookup is best-effort
		// developer name lookup
		devName := ""
		if dev, err := s.repo.FindDeveloperByID(sub.DeveloperID); err == nil {
			devName = dev.DisplayName
		}
		items[i] = domain.MarketplaceListItem{
			PluginName:    sub.PluginName,
			Version:       sub.Version,
			Title:         sub.Title,
			Description:   sub.Description,
			Category:      sub.Category,
			DeveloperName: devName,
			DownloadCount: sub.DownloadCount,
			AvgRating:     avgRating,
			ReviewCount:   reviewCount,
		}
	}
	return items, total, nil
}

// GetPlugin 플러그인 상세 조회 (승인된 최신 버전)
func (s *MarketplaceService) GetPlugin(pluginName string) (*domain.PluginSubmission, float64, int64, error) {
	sub, err := s.repo.FindApprovedByName(pluginName)
	if err != nil {
		return nil, 0, 0, err
	}
	avgRating, reviewCount, _ := s.repo.GetAverageRating(pluginName) //nolint:errcheck // rating lookup is best-effort
	return sub, avgRating, reviewCount, nil
}

// GetReviews 플러그인 리뷰 목록
func (s *MarketplaceService) GetReviews(pluginName string, page, perPage int) ([]domain.PluginReview, int64, error) {
	return s.repo.FindReviewsByPlugin(pluginName, page, perPage)
}

// AddReview 리뷰 작성
func (s *MarketplaceService) AddReview(pluginName string, userID uint64, req domain.PluginReviewRequest) error {
	// 중복 리뷰 방지
	reviewed, err := s.repo.HasUserReviewed(pluginName, userID)
	if err != nil {
		return err
	}
	if reviewed {
		return errors.New("이미 리뷰를 작성했습니다")
	}

	review := &domain.PluginReview{
		PluginName: pluginName,
		UserID:     userID,
		Rating:     req.Rating,
		Comment:    req.Comment,
	}
	return s.repo.CreateReview(review)
}

// TrackDownload 다운로드 기록
func (s *MarketplaceService) TrackDownload(pluginName, version, ip string, userID *uint64) error {
	dl := &domain.PluginDownload{
		PluginName: pluginName,
		Version:    version,
		UserID:     userID,
		IP:         ip,
	}
	if err := s.repo.CreateDownload(dl); err != nil {
		return err
	}
	return s.repo.IncrementDownloadCount(pluginName)
}

// === Developer API ===

// RegisterDeveloper 개발자 등록
func (s *MarketplaceService) RegisterDeveloper(userID uint64, req domain.DeveloperRegisterRequest) (*domain.PluginDeveloper, error) {
	// 이미 등록된 개발자 확인
	existing, err := s.repo.FindDeveloperByUserID(userID)
	if err == nil && existing.ID > 0 {
		return nil, errors.New("이미 개발자로 등록되어 있습니다")
	}

	dev := &domain.PluginDeveloper{
		UserID:      userID,
		DisplayName: req.DisplayName,
		Email:       req.Email,
		Website:     req.Website,
		Bio:         req.Bio,
		Status:      "active",
	}
	if err := s.repo.CreateDeveloper(dev); err != nil {
		return nil, err
	}
	return dev, nil
}

// GetDeveloperProfile 개발자 프로필 조회
func (s *MarketplaceService) GetDeveloperProfile(userID uint64) (*domain.PluginDeveloper, error) {
	return s.repo.FindDeveloperByUserID(userID)
}

// SubmitPlugin 플러그인 제출
func (s *MarketplaceService) SubmitPlugin(developerID uint64, req domain.PluginSubmitRequest) (*domain.PluginSubmission, error) {
	sub := &domain.PluginSubmission{
		DeveloperID: developerID,
		PluginName:  req.PluginName,
		Version:     req.Version,
		Title:       req.Title,
		Description: req.Description,
		Category:    req.Category,
		Tags:        req.Tags,
		SourceURL:   req.SourceURL,
		DownloadURL: req.DownloadURL,
		Readme:      req.Readme,
		Status:      "pending",
	}
	if err := s.repo.CreateSubmission(sub); err != nil {
		return nil, err
	}
	return sub, nil
}

// ListMySubmissions 내 제출 목록
func (s *MarketplaceService) ListMySubmissions(developerID uint64, page, perPage int) ([]domain.PluginSubmission, int64, error) {
	return s.repo.FindSubmissionsByDeveloper(developerID, page, perPage)
}

// === Admin API ===

// ListPendingSubmissions 관리자: 대기 중 제출 목록
func (s *MarketplaceService) ListPendingSubmissions(page, perPage int) ([]domain.PluginSubmission, int64, error) {
	return s.repo.FindPendingSubmissions(page, perPage)
}

// ReviewSubmission 관리자: 제출 승인/거절
func (s *MarketplaceService) ReviewSubmission(submissionID uint64, reviewerID uint64, approve bool, note string) error {
	sub, err := s.repo.FindSubmissionByID(submissionID)
	if err != nil {
		return err
	}
	if sub.Status != "pending" {
		return errors.New("이미 처리된 제출입니다")
	}

	now := time.Now()
	sub.ReviewedBy = &reviewerID
	sub.ReviewedAt = &now
	sub.ReviewNote = note

	if approve {
		sub.Status = "approved"
	} else {
		sub.Status = "rejected"
	}
	return s.repo.UpdateSubmission(sub)
}

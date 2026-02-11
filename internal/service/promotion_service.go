package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// PromotionService defines the business logic for promotions
type PromotionService interface {
	// Advertiser methods
	GetAllAdvertisers() ([]domain.AdvertiserResponse, error)
	GetActiveAdvertisers() ([]domain.AdvertiserResponse, error)
	GetAdvertiserByID(id int64) (*domain.AdvertiserResponse, error)
	CreateAdvertiser(req *domain.CreateAdvertiserRequest) (*domain.AdvertiserResponse, error)
	UpdateAdvertiser(id int64, req *domain.UpdateAdvertiserRequest) (*domain.AdvertiserResponse, error)
	DeleteAdvertiser(id int64) error

	// Promotion Post methods
	GetPromotionPosts(page, limit int) (*domain.PromotionListResponse, error)
	GetPromotionPostsForInsert(count int) ([]domain.PromotionPostResponse, error)
	GetPromotionPostByID(id int64) (*domain.PromotionPostResponse, error)
	CreatePromotionPost(memberID string, req *domain.CreatePromotionPostRequest) (*domain.PromotionPostResponse, error)
	UpdatePromotionPost(id int64, memberID string, req *domain.UpdatePromotionPostRequest) (*domain.PromotionPostResponse, error)
	DeletePromotionPost(id int64, memberID string) error
	IncrementViews(id int64) error

	// Check if user is an advertiser
	IsAdvertiser(memberID string) (bool, *domain.Advertiser, error)

	// Advertiser self-service
	GetMyStats(memberID string) (*domain.AdvertiserStatsResponse, error)
	GetMyRemaining(memberID string) (*domain.AdvertiserRemainingResponse, error)

	// Widget JSON sync
	SyncWidgetJSON() error
}

type promotionService struct {
	repo repository.PromotionRepository
}

// NewPromotionService creates a new PromotionService
func NewPromotionService(repo repository.PromotionRepository) PromotionService {
	return &promotionService{repo: repo}
}

// GetAllAdvertisers retrieves all advertisers
func (s *promotionService) GetAllAdvertisers() ([]domain.AdvertiserResponse, error) {
	advertisers, err := s.repo.GetAllAdvertisers()
	if err != nil {
		return nil, err
	}

	responses := make([]domain.AdvertiserResponse, len(advertisers))
	for i, advertiser := range advertisers {
		responses[i] = advertiser.ToResponse()
	}

	return responses, nil
}

// GetActiveAdvertisers retrieves active advertisers
func (s *promotionService) GetActiveAdvertisers() ([]domain.AdvertiserResponse, error) {
	advertisers, err := s.repo.GetActiveAdvertisers()
	if err != nil {
		return nil, err
	}

	responses := make([]domain.AdvertiserResponse, len(advertisers))
	for i, advertiser := range advertisers {
		responses[i] = advertiser.ToResponse()
	}

	return responses, nil
}

// GetAdvertiserByID retrieves an advertiser by ID
func (s *promotionService) GetAdvertiserByID(id int64) (*domain.AdvertiserResponse, error) {
	advertiser, err := s.repo.FindAdvertiserByID(id)
	if err != nil {
		return nil, err
	}

	resp := advertiser.ToResponse()
	return &resp, nil
}

// CreateAdvertiser creates a new advertiser
func (s *promotionService) CreateAdvertiser(req *domain.CreateAdvertiserRequest) (*domain.AdvertiserResponse, error) {
	// Check if member is already an advertiser
	existing, err := s.repo.FindAdvertiserByMemberID(req.MemberID)
	if err == nil && existing != nil {
		return nil, errors.New("member is already an advertiser")
	}

	// Set default post count
	postCount := req.PostCount
	if postCount <= 0 {
		postCount = 1
	}

	advertiser := &domain.Advertiser{
		MemberID:  req.MemberID,
		Name:      req.Name,
		PostCount: postCount,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		IsPinned:  req.IsPinned,
		IsActive:  req.IsActive,
		Memo:      req.Memo,
	}

	if err := s.repo.CreateAdvertiser(advertiser); err != nil {
		return nil, err
	}

	resp := advertiser.ToResponse()
	return &resp, nil
}

// UpdateAdvertiser updates an advertiser
func (s *promotionService) UpdateAdvertiser(id int64, req *domain.UpdateAdvertiserRequest) (*domain.AdvertiserResponse, error) {
	advertiser, err := s.repo.FindAdvertiserByID(id)
	if err != nil {
		return nil, err
	}

	// Update fields
	if req.Name != "" {
		advertiser.Name = req.Name
	}
	if req.PostCount > 0 {
		advertiser.PostCount = req.PostCount
	}
	advertiser.StartDate = req.StartDate
	advertiser.EndDate = req.EndDate
	advertiser.IsPinned = req.IsPinned
	advertiser.IsActive = req.IsActive
	advertiser.Memo = req.Memo

	if err := s.repo.UpdateAdvertiser(advertiser); err != nil {
		return nil, err
	}

	resp := advertiser.ToResponse()
	return &resp, nil
}

// DeleteAdvertiser deletes an advertiser
func (s *promotionService) DeleteAdvertiser(id int64) error {
	return s.repo.DeleteAdvertiser(id)
}

// GetPromotionPosts retrieves promotion posts with pagination
func (s *promotionService) GetPromotionPosts(page, limit int) (*domain.PromotionListResponse, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	posts, total, err := s.repo.GetPromotionPosts(page, limit)
	if err != nil {
		return nil, err
	}

	responses := make([]domain.PromotionPostResponse, len(posts))
	for i, post := range posts {
		responses[i] = post.ToResponse()
	}

	return &domain.PromotionListResponse{
		Posts: responses,
		Total: int(total),
	}, nil
}

// GetPromotionPostsForInsert retrieves promotion posts for inserting into other boards
func (s *promotionService) GetPromotionPostsForInsert(count int) ([]domain.PromotionPostResponse, error) {
	if count <= 0 {
		count = 3
	}

	posts, err := s.repo.GetPromotionPostsForInsert(count)
	if err != nil {
		return nil, err
	}

	responses := make([]domain.PromotionPostResponse, len(posts))
	for i, post := range posts {
		responses[i] = post.ToResponse()
	}

	return responses, nil
}

// GetPromotionPostByID retrieves a promotion post by ID
func (s *promotionService) GetPromotionPostByID(id int64) (*domain.PromotionPostResponse, error) {
	post, err := s.repo.FindPromotionPostByID(id)
	if err != nil {
		return nil, err
	}

	resp := post.ToResponse()
	return &resp, nil
}

// CreatePromotionPost creates a new promotion post
func (s *promotionService) CreatePromotionPost(memberID string, req *domain.CreatePromotionPostRequest) (*domain.PromotionPostResponse, error) {
	// Check if user is an advertiser
	isAdvertiser, advertiser, err := s.IsAdvertiser(memberID)
	if err != nil {
		return nil, err
	}
	if !isAdvertiser {
		return nil, errors.New("only advertisers can create promotion posts")
	}

	post := &domain.PromotionPost{
		AdvertiserID: advertiser.ID,
		Title:        req.Title,
		Content:      req.Content,
		LinkURL:      req.LinkURL,
		ImageURL:     req.ImageURL,
		IsActive:     true,
	}

	if err := s.repo.CreatePromotionPost(post); err != nil {
		return nil, err
	}

	// Set author name from advertiser
	post.AuthorName = advertiser.Name
	post.IsPinned = advertiser.IsPinned

	resp := post.ToResponse()
	return &resp, nil
}

// UpdatePromotionPost updates a promotion post
func (s *promotionService) UpdatePromotionPost(id int64, memberID string, req *domain.UpdatePromotionPostRequest) (*domain.PromotionPostResponse, error) {
	// Check if user is an advertiser
	isAdvertiser, advertiser, err := s.IsAdvertiser(memberID)
	if err != nil {
		return nil, err
	}
	if !isAdvertiser {
		return nil, errors.New("only advertisers can update promotion posts")
	}

	post, err := s.repo.FindPromotionPostByID(id)
	if err != nil {
		return nil, err
	}

	// Check ownership
	if post.AdvertiserID != advertiser.ID {
		return nil, errors.New("you can only update your own posts")
	}

	// Update fields
	if req.Title != "" {
		post.Title = req.Title
	}
	if req.Content != "" {
		post.Content = req.Content
	}
	post.LinkURL = req.LinkURL
	post.ImageURL = req.ImageURL
	post.IsActive = req.IsActive

	if err := s.repo.UpdatePromotionPost(post); err != nil {
		return nil, err
	}

	resp := post.ToResponse()
	return &resp, nil
}

// DeletePromotionPost deletes a promotion post
func (s *promotionService) DeletePromotionPost(id int64, memberID string) error {
	// Check if user is an advertiser
	isAdvertiser, advertiser, err := s.IsAdvertiser(memberID)
	if err != nil {
		return err
	}
	if !isAdvertiser {
		return errors.New("only advertisers can delete promotion posts")
	}

	post, err := s.repo.FindPromotionPostByID(id)
	if err != nil {
		return err
	}

	// Check ownership
	if post.AdvertiserID != advertiser.ID {
		return errors.New("you can only delete your own posts")
	}

	return s.repo.DeletePromotionPost(id)
}

// IncrementViews increments the view count of a promotion post
func (s *promotionService) IncrementViews(id int64) error {
	return s.repo.IncrementViews(id)
}

// GetMyStats returns the advertiser's own stats
func (s *promotionService) GetMyStats(memberID string) (*domain.AdvertiserStatsResponse, error) {
	advertiser, err := s.repo.FindAdvertiserByMemberID(memberID)
	if err != nil {
		return nil, errors.New("광고주 정보를 찾을 수 없습니다")
	}

	totalViews, totalLikes, postCount, err := s.repo.GetPostStatsByAdvertiser(advertiser.ID)
	if err != nil {
		return nil, err
	}

	return &domain.AdvertiserStatsResponse{
		AdvertiserID: advertiser.ID,
		Name:         advertiser.Name,
		TotalViews:   totalViews,
		TotalLikes:   totalLikes,
		PostCount:    postCount,
		IsActive:     advertiser.IsActive,
		IsPinned:     advertiser.IsPinned,
		StartDate:    advertiser.StartDate,
		EndDate:      advertiser.EndDate,
	}, nil
}

// GetMyRemaining returns the advertiser's remaining ad period
func (s *promotionService) GetMyRemaining(memberID string) (*domain.AdvertiserRemainingResponse, error) {
	advertiser, err := s.repo.FindAdvertiserByMemberID(memberID)
	if err != nil {
		return nil, errors.New("광고주 정보를 찾을 수 없습니다")
	}

	remainingDays := -1 // unlimited
	isExpired := false
	if advertiser.EndDate != nil {
		diff := time.Until(*advertiser.EndDate).Hours() / 24
		remainingDays = int(math.Ceil(diff))
		if remainingDays < 0 {
			remainingDays = 0
			isExpired = true
		}
	}

	return &domain.AdvertiserRemainingResponse{
		AdvertiserID:  advertiser.ID,
		Name:          advertiser.Name,
		StartDate:     advertiser.StartDate,
		EndDate:       advertiser.EndDate,
		RemainingDays: remainingDays,
		IsActive:      advertiser.IsActive,
		IsExpired:     isExpired,
	}, nil
}

// IsAdvertiser checks if a member is an advertiser
func (s *promotionService) IsAdvertiser(memberID string) (bool, *domain.Advertiser, error) {
	advertiser, err := s.repo.FindAdvertiserByMemberID(memberID)
	if err != nil {
		return false, nil, nil // Not found is not an error
	}

	if !advertiser.IsActive {
		return false, advertiser, nil
	}

	return true, advertiser, nil
}

// ============= Widget JSON Sync =============

// nariyaWidgetJSON은 nariya 위젯 JSON 파일의 최상위 구조
type nariyaWidgetJSON struct {
	BoardException       string                       `json:"board_exception"`
	InsertIndex          int                          `json:"insert_index"`
	MinCntForInsertIndex int                          `json:"min_cnt_for_insert_index"`
	HowManyToDisplay     int                          `json:"how_many_to_display"`
	Advertisers          map[string]nariyaAdvertiser   `json:"advertisers"`
}

// nariyaAdvertiser는 nariya 위젯 JSON의 광고주 항목
type nariyaAdvertiser struct {
	Name            string `json:"name"`
	MemberID        string `json:"member_id"`
	StartDate       string `json:"start_date"`
	EndDate         string `json:"end_date"`
	PostCount       int    `json:"post_count"`
	PermissionGroup string `json:"permission_group"`
	PinToTop        bool   `json:"pin_to_top"`
	Order           int    `json:"order"`
}

const widgetJSONDir = "/home/damoang/www/data/nariya/widget"

// SyncWidgetJSON은 DB의 광고주 데이터를 nariya 위젯 JSON 파일에 동기화
func (s *promotionService) SyncWidgetJSON() error {
	advertisers, err := s.repo.GetAllAdvertisers()
	if err != nil {
		return fmt.Errorf("광고주 목록 조회 실패: %w", err)
	}

	// 기존 JSON 파일에서 위젯 설정(board_exception 등)을 읽어옴
	existingSettings := s.readExistingWidgetSettings()

	// DB 데이터를 nariya JSON 형식으로 변환
	widgetData := nariyaWidgetJSON{
		BoardException:       existingSettings.BoardException,
		InsertIndex:          existingSettings.InsertIndex,
		MinCntForInsertIndex: existingSettings.MinCntForInsertIndex,
		HowManyToDisplay:     existingSettings.HowManyToDisplay,
		Advertisers:          make(map[string]nariyaAdvertiser),
	}

	// is_active인 광고주만 포함, created_at 순서로 order 부여
	order := 0
	for _, adv := range advertisers {
		if !adv.IsActive {
			continue
		}

		startDate := ""
		if adv.StartDate != nil {
			startDate = adv.StartDate.Format("2006-01-02")
		}
		endDate := ""
		if adv.EndDate != nil {
			endDate = adv.EndDate.Format("2006-01-02")
		}

		permGroup := "one"
		if adv.PostCount >= 2 {
			permGroup = "two"
		}

		widgetData.Advertisers[adv.MemberID] = nariyaAdvertiser{
			Name:            adv.Name,
			MemberID:        adv.MemberID,
			StartDate:       startDate,
			EndDate:         endDate,
			PostCount:       adv.PostCount,
			PermissionGroup: permGroup,
			PinToTop:        adv.IsPinned,
			Order:           order,
		}
		order++
	}

	// PC/MO 양 플랫폼에 동시 저장
	platforms := []string{"pc", "mo"}
	for _, platform := range platforms {
		filename := fmt.Sprintf("w-promotion-ad-insertion-pai-%s.json", platform)
		filePath := filepath.Join(widgetJSONDir, filename)

		if err := s.writeWidgetJSON(filePath, &widgetData); err != nil {
			log.Printf("[Promotion] 위젯 JSON 저장 실패 (%s): %v", platform, err)
			return fmt.Errorf("위젯 JSON 저장 실패 (%s): %w", platform, err)
		}
	}

	log.Printf("[Promotion] 위젯 JSON 동기화 완료 (광고주 %d명)", len(widgetData.Advertisers))
	return nil
}

// readExistingWidgetSettings는 기존 JSON 파일에서 위젯 설정값을 읽어옴
func (s *promotionService) readExistingWidgetSettings() nariyaWidgetJSON {
	defaults := nariyaWidgetJSON{
		BoardException:       "report,promotion,notice,banners,truthroom,advertiser,claim,disciplinelog,verification,angtt,angreport,nope,flag,gallery,flex,main,admin,angmap,giving,hello,keycaps,image,photo,message",
		InsertIndex:          15,
		MinCntForInsertIndex: 20,
		HowManyToDisplay:     2,
	}

	filePath := filepath.Join(widgetJSONDir, "w-promotion-ad-insertion-pai-pc.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return defaults
	}

	var existing nariyaWidgetJSON
	if err := json.Unmarshal(data, &existing); err != nil {
		return defaults
	}

	// 설정값만 가져오고 advertisers는 무시
	defaults.BoardException = existing.BoardException
	defaults.InsertIndex = existing.InsertIndex
	defaults.MinCntForInsertIndex = existing.MinCntForInsertIndex
	defaults.HowManyToDisplay = existing.HowManyToDisplay
	return defaults
}

// writeWidgetJSON은 백업 후 JSON 파일을 쓴다
func (s *promotionService) writeWidgetJSON(filePath string, data *nariyaWidgetJSON) error {
	// 기존 파일 백업
	if _, err := os.Stat(filePath); err == nil {
		backupPath := filePath + ".backup." + time.Now().Format("2006-01-02-15-04-05")
		if err := copyFile(filePath, backupPath); err != nil {
			log.Printf("[Promotion] 백업 실패: %v", err)
		}
	}

	jsonBytes, err := json.MarshalIndent(data, "", "    ")
	if err != nil {
		return fmt.Errorf("JSON 인코딩 실패: %w", err)
	}

	if err := os.WriteFile(filePath, jsonBytes, 0644); err != nil {
		return fmt.Errorf("파일 쓰기 실패: %w", err)
	}

	// 쓰기 후 검증
	verifyData, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("파일 검증 실패 (읽기): %w", err)
	}
	var verify nariyaWidgetJSON
	if err := json.Unmarshal(verifyData, &verify); err != nil {
		return fmt.Errorf("파일 검증 실패 (파싱): %w", err)
	}

	return nil
}

// copyFile은 src를 dst로 복사
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

package service

import (
	"fmt"
	"log"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// ScrapService business logic for scraps
type ScrapService interface {
	AddScrap(mbID string, boardID string, wrID int) (*domain.ScrapResponse, error)
	RemoveScrap(mbID string, boardID string, wrID int) error
	ListScraps(mbID string, page, limit int) ([]*domain.ScrapResponse, *common.Meta, error)
}

type scrapService struct {
	repo repository.ScrapRepository
}

// NewScrapService creates a new ScrapService
func NewScrapService(repo repository.ScrapRepository) ScrapService {
	return &scrapService{repo: repo}
}

// AddScrap adds a post to scraps
func (s *scrapService) AddScrap(mbID string, boardID string, wrID int) (*domain.ScrapResponse, error) {
	exists, err := s.repo.Exists(mbID, boardID, wrID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("이미 스크랩한 게시글입니다")
	}

	scrap, err := s.repo.Create(mbID, boardID, wrID)
	if err != nil {
		return nil, err
	}

	title, author, err := s.repo.GetPostTitle(boardID, wrID)
	if err != nil {
		log.Printf("warning: failed to get post title for board=%s wr=%d: %v", boardID, wrID, err)
	}

	return &domain.ScrapResponse{
		ScrapID:   scrap.ID,
		BoardID:   boardID,
		PostID:    wrID,
		Title:     title,
		Author:    author,
		CreatedAt: scrap.DateTime.Format("2006-01-02 15:04:05"),
	}, nil
}

// RemoveScrap removes a post from scraps
func (s *scrapService) RemoveScrap(mbID string, boardID string, wrID int) error {
	return s.repo.Delete(mbID, boardID, wrID)
}

// ListScraps returns the member's scrap list
func (s *scrapService) ListScraps(mbID string, page, limit int) ([]*domain.ScrapResponse, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	scraps, total, err := s.repo.FindByMember(mbID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.ScrapResponse, len(scraps))
	for i, sc := range scraps {
		title, author, err := s.repo.GetPostTitle(sc.BoTable, sc.WrID)
		if err != nil {
			log.Printf("warning: failed to get post title for board=%s wr=%d: %v", sc.BoTable, sc.WrID, err)
		}
		responses[i] = &domain.ScrapResponse{
			ScrapID:   sc.ID,
			BoardID:   sc.BoTable,
			PostID:    sc.WrID,
			Title:     title,
			Author:    author,
			CreatedAt: sc.DateTime.Format("2006-01-02 15:04:05"),
		}
	}

	meta := &common.Meta{
		Page:  page,
		Limit: limit,
		Total: total,
	}

	return responses, meta, nil
}

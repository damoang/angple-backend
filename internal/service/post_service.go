package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// PostService business logic for posts
type PostService interface {
	ListPosts(boardID string, page, limit int) ([]*domain.PostResponse, *common.Meta, error)
	GetPost(boardID string, id int) (*domain.PostResponse, error)
	CreatePost(boardID string, req *domain.CreatePostRequest, authorID string) (*domain.PostResponse, error)
	UpdatePost(boardID string, id int, req *domain.UpdatePostRequest, authorID string) error
	DeletePost(boardID string, id int, authorID string) error
	SearchPosts(boardID string, keyword string, page, limit int) ([]*domain.PostResponse, *common.Meta, error)
}

type postService struct {
	repo repository.PostRepository
}

// NewPostService creates a new PostService
func NewPostService(repo repository.PostRepository) PostService {
	return &postService{repo: repo}
}

// ListPosts retrieves paginated posts
func (s *postService) ListPosts(boardID string, page, limit int) ([]*domain.PostResponse, *common.Meta, error) {
	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// Call repository
	posts, total, err := s.repo.ListByBoard(boardID, page, limit)
	if err != nil {
		return nil, nil, err
	}

	// Convert to response
	responses := make([]*domain.PostResponse, len(posts))
	for i, post := range posts {
		responses[i] = post.ToResponse()
	}

	// Build metadata
	meta := &common.Meta{
		BoardID: boardID,
		Page:    page,
		Limit:   limit,
		Total:   total,
	}

	return responses, meta, nil
}

// GetPost retrieves a single post by ID
func (s *postService) GetPost(boardID string, id int) (*domain.PostResponse, error) {
	post, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound
	}

	// Increment view count asynchronously
	go s.repo.IncrementHit(boardID, id) //nolint:errcheck // 비동기 조회수 증가, 실패해도 무시

	return post.ToResponse(), nil
}

// CreatePost creates a new post
func (s *postService) CreatePost(boardID string, req *domain.CreatePostRequest, authorID string) (*domain.PostResponse, error) {
	post := &domain.Post{
		Title:    req.Title,
		Content:  req.Content,
		Category: req.Category,
		Author:   req.Author,
		AuthorID: authorID,
		Password: req.Password,
	}

	if err := s.repo.Create(boardID, post); err != nil {
		return nil, err
	}

	return post.ToResponse(), nil
}

// UpdatePost updates an existing post
func (s *postService) UpdatePost(boardID string, id int, req *domain.UpdatePostRequest, authorID string) error {
	// Check if post exists and belongs to author
	existing, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return common.ErrPostNotFound
	}

	// Verify ownership
	if existing.AuthorID != authorID {
		return common.ErrUnauthorized
	}

	post := &domain.Post{
		Title:    req.Title,
		Content:  req.Content,
		Category: req.Category,
	}

	return s.repo.Update(boardID, id, post)
}

// DeletePost deletes a post
func (s *postService) DeletePost(boardID string, id int, authorID string) error {
	// Check if post exists and belongs to author
	existing, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return common.ErrPostNotFound
	}

	// Verify ownership
	if existing.AuthorID != authorID {
		return common.ErrUnauthorized
	}

	return s.repo.Delete(boardID, id)
}

// SearchPosts searches posts by keyword
func (s *postService) SearchPosts(boardID string, keyword string, page, limit int) ([]*domain.PostResponse, *common.Meta, error) {
	// Validate pagination
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// Call repository
	posts, total, err := s.repo.Search(boardID, keyword, page, limit)
	if err != nil {
		return nil, nil, err
	}

	// Convert to response
	responses := make([]*domain.PostResponse, len(posts))
	for i, post := range posts {
		responses[i] = post.ToResponse()
	}

	// Build metadata
	meta := &common.Meta{
		BoardID: boardID,
		Page:    page,
		Limit:   limit,
		Total:   total,
	}

	return responses, meta, nil
}

package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/plugin"
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
	repo  repository.PostRepository
	hooks *plugin.HookManager
}

// NewPostService creates a new PostService
func NewPostService(repo repository.PostRepository, hooks *plugin.HookManager) PostService {
	return &postService{repo: repo, hooks: hooks}
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

	resp := post.ToResponse()

	// post.content Filter (content 렌더링 시)
	if s.hooks != nil {
		data := s.hooks.Apply("post.content", map[string]interface{}{
			"board_id": boardID,
			"post_id":  id,
			"content":  resp.Content,
		})
		if v, ok := data["content"].(string); ok {
			resp.Content = v
		}
	}

	return resp, nil
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

	// before_create Filter
	if s.hooks != nil {
		data := s.hooks.Apply("post.before_create", map[string]interface{}{
			"board_id":  boardID,
			"title":     post.Title,
			"content":   post.Content,
			"author_id": authorID,
		})
		if v, ok := data["title"].(string); ok {
			post.Title = v
		}
		if v, ok := data["content"].(string); ok {
			post.Content = v
		}
	}

	if err := s.repo.Create(boardID, post); err != nil {
		return nil, err
	}

	// after_create Action
	if s.hooks != nil {
		s.hooks.Do("post.after_create", map[string]interface{}{
			"board_id":  boardID,
			"post_id":   post.ID,
			"title":     post.Title,
			"author_id": authorID,
		})
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

	// before_update Filter
	if s.hooks != nil {
		data := s.hooks.Apply("post.before_update", map[string]interface{}{
			"board_id":  boardID,
			"post_id":   id,
			"title":     post.Title,
			"content":   post.Content,
			"author_id": authorID,
		})
		if v, ok := data["title"].(string); ok {
			post.Title = v
		}
		if v, ok := data["content"].(string); ok {
			post.Content = v
		}
	}

	if err := s.repo.Update(boardID, id, post); err != nil {
		return err
	}

	// after_update Action
	if s.hooks != nil {
		s.hooks.Do("post.after_update", map[string]interface{}{
			"board_id":  boardID,
			"post_id":   id,
			"author_id": authorID,
		})
	}

	return nil
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

	// before_delete Filter
	if s.hooks != nil {
		s.hooks.Apply("post.before_delete", map[string]interface{}{
			"board_id":  boardID,
			"post_id":   id,
			"author_id": authorID,
		})
	}

	if err := s.repo.Delete(boardID, id); err != nil {
		return err
	}

	// after_delete Action
	if s.hooks != nil {
		s.hooks.Do("post.after_delete", map[string]interface{}{
			"board_id":  boardID,
			"post_id":   id,
			"author_id": authorID,
		})
	}

	return nil
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

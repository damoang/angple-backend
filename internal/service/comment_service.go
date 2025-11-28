package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

type CommentService interface {
	ListComments(boardID string, postID int) ([]*domain.CommentResponse, error)
	GetComment(boardID string, id int) (*domain.CommentResponse, error)
	CreateComment(boardID string, postID int, req *domain.CreateCommentRequest, authorID string) (*domain.CommentResponse, error)
	UpdateComment(boardID string, id int, req *domain.UpdateCommentRequest, authorID string) error
	DeleteComment(boardID string, id int, authorID string) error
}

type commentService struct {
	repo repository.CommentRepository
}

func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

// ListComments returns all comments for a post
func (s *commentService) ListComments(boardID string, postID int) ([]*domain.CommentResponse, error) {
	comments, err := s.repo.ListByPost(boardID, postID)
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.CommentResponse, len(comments))
	for i, comment := range comments {
		responses[i] = comment.ToResponse()
	}

	return responses, nil
}

// GetComment returns a single comment
func (s *commentService) GetComment(boardID string, id int) (*domain.CommentResponse, error) {
	comment, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound // Reuse post not found error
	}

	return comment.ToResponse(), nil
}

// CreateComment creates a new comment
func (s *commentService) CreateComment(
	boardID string,
	postID int,
	req *domain.CreateCommentRequest,
	authorID string,
) (*domain.CommentResponse, error) {
	comment := &domain.Comment{
		ParentID: postID,
		Content:  req.Content,
		Author:   req.Author,
		AuthorID: authorID,
	}

	if err := s.repo.Create(boardID, comment); err != nil {
		return nil, err
	}

	return comment.ToResponse(), nil
}

// UpdateComment updates a comment
func (s *commentService) UpdateComment(
	boardID string,
	id int,
	req *domain.UpdateCommentRequest,
	authorID string,
) error {
	// Verify ownership
	existing, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return common.ErrPostNotFound
	}

	if existing.AuthorID != authorID {
		return common.ErrUnauthorized
	}

	comment := &domain.Comment{
		Content: req.Content,
	}

	return s.repo.Update(boardID, id, comment)
}

// DeleteComment deletes a comment
func (s *commentService) DeleteComment(boardID string, id int, authorID string) error {
	// Verify ownership
	existing, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return common.ErrPostNotFound
	}

	if existing.AuthorID != authorID {
		return common.ErrUnauthorized
	}

	return s.repo.Delete(boardID, id)
}

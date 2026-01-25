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
	LikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error)
	DislikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error)
}

type commentService struct {
	repo repository.CommentRepository
}

func NewCommentService(repo repository.CommentRepository) CommentService {
	return &commentService{repo: repo}
}

// ListComments returns all comments for a post
// ToResponse()에서 parent_id가 올바르게 설정됨:
// - 원댓글: parent_id = 게시글 ID
// - 대댓글: parent_id = 부모 댓글 ID (Num 필드에 저장)
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
// 대댓글인 경우 ParentCommentID를 통해 부모 댓글을 참조하고 depth, comment_reply를 계산
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

	// 대댓글인 경우 부모 댓글 정보를 기반으로 depth, comment_reply, Num(부모댓글ID) 설정
	if req.ParentCommentID > 0 {
		// 부모 댓글 조회
		parentComment, err := s.repo.FindByID(boardID, req.ParentCommentID)
		if err != nil {
			return nil, err
		}

		// depth = 부모 댓글의 depth + 1
		comment.CommentCount = parentComment.CommentCount + 1

		// Num에 부모 댓글 ID 저장 (API 응답에서 parent_id로 사용)
		comment.Num = req.ParentCommentID

		// comment_reply 계산: 부모의 comment_reply + 2자리 숫자
		nextReply, err := s.repo.GetNextCommentReply(boardID, postID, parentComment.CommentReply)
		if err != nil {
			return nil, err
		}
		comment.CommentReply = nextReply
	} else {
		// 원댓글 (그누보드 호환: depth 0)
		comment.CommentCount = 0 // depth 0
		comment.Num = 0          // 원댓글은 부모 댓글이 없음
		comment.CommentReply = "" // 빈 문자열
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

// LikeComment increments the like count for a comment
// Note: This is a simple implementation without tracking who liked.
// For production, consider using a separate table to track user likes.
func (s *commentService) LikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error) {
	// Check if comment exists
	_, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound
	}

	// Increment likes
	if err := s.repo.IncrementLikes(boardID, id); err != nil {
		return nil, err
	}

	// Get updated comment
	updated, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, err
	}

	return &domain.CommentLikeResponse{
		Likes:       updated.Likes,
		Dislikes:    updated.Dislikes,
		UserLiked:   true,
		UserDisliked: false,
	}, nil
}

// DislikeComment increments the dislike count for a comment
func (s *commentService) DislikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error) {
	// Check if comment exists
	_, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound
	}

	// Increment dislikes
	if err := s.repo.IncrementDislikes(boardID, id); err != nil {
		return nil, err
	}

	// Get updated comment
	updated, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, err
	}

	return &domain.CommentLikeResponse{
		Likes:       updated.Likes,
		Dislikes:    updated.Dislikes,
		UserLiked:   false,
		UserDisliked: true,
	}, nil
}

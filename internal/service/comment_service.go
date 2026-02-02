package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/plugin"
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
	repo     repository.CommentRepository
	goodRepo repository.GoodRepository
	hooks    *plugin.HookManager
}

func NewCommentService(repo repository.CommentRepository, goodRepo repository.GoodRepository, hooks *plugin.HookManager) CommentService {
	return &commentService{repo: repo, goodRepo: goodRepo, hooks: hooks}
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

	resp := comment.ToResponse()

	// comment.content Filter
	if s.hooks != nil {
		data := s.hooks.Apply(plugin.HookCommentContent, map[string]interface{}{
			"board_id":   boardID,
			"comment_id": id,
			"content":    resp.Content,
		})
		if v, ok := data["content"].(string); ok {
			resp.Content = v
		}
	}

	return resp, nil
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
		comment.CommentCount = 0  // depth 0
		comment.Num = 0           // 원댓글은 부모 댓글이 없음
		comment.CommentReply = "" // 빈 문자열
	}

	// before_create Filter
	if s.hooks != nil {
		data := s.hooks.Apply(plugin.HookCommentBeforeCreate, map[string]interface{}{
			"board_id":  boardID,
			"post_id":   postID,
			"content":   comment.Content,
			"author_id": authorID,
		})
		if v, ok := data["content"].(string); ok {
			comment.Content = v
		}
	}

	if err := s.repo.Create(boardID, comment); err != nil {
		return nil, err
	}

	// after_create Action
	if s.hooks != nil {
		s.hooks.Do(plugin.HookCommentAfterCreate, map[string]interface{}{
			"board_id":  boardID,
			"post_id":   postID,
			"author_id": authorID,
		})
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

	// before_update Filter
	if s.hooks != nil {
		data := s.hooks.Apply(plugin.HookCommentBeforeUpdate, map[string]interface{}{
			"board_id":   boardID,
			"comment_id": id,
			"content":    comment.Content,
			"author_id":  authorID,
		})
		if v, ok := data["content"].(string); ok {
			comment.Content = v
		}
	}

	if err := s.repo.Update(boardID, id, comment); err != nil {
		return err
	}

	// after_update Action
	if s.hooks != nil {
		s.hooks.Do(plugin.HookCommentAfterUpdate, map[string]interface{}{
			"board_id":   boardID,
			"comment_id": id,
			"author_id":  authorID,
		})
	}

	return nil
}

// DeleteComment deletes a comment
//
//nolint:dupl // PostService.DeletePost와 구조 유사하나 다른 Hook 이벤트 사용
func (s *commentService) DeleteComment(boardID string, id int, authorID string) error {
	// Verify ownership
	existing, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return common.ErrPostNotFound
	}

	if existing.AuthorID != authorID {
		return common.ErrUnauthorized
	}

	// before_delete Filter
	if s.hooks != nil {
		s.hooks.Apply(plugin.HookCommentBeforeDelete, map[string]interface{}{
			"board_id":   boardID,
			"comment_id": id,
			"author_id":  authorID,
		})
	}

	if err := s.repo.Delete(boardID, id); err != nil {
		return err
	}

	// after_delete Action
	if s.hooks != nil {
		s.hooks.Do(plugin.HookCommentAfterDelete, map[string]interface{}{
			"board_id":   boardID,
			"comment_id": id,
			"author_id":  authorID,
		})
	}

	return nil
}

// LikeComment increments the like count for a comment with duplicate check via g5_board_good
// DB UNIQUE KEY: (bo_table, wr_id, mb_id) — 한 사용자당 하나의 액션만 허용
//
//nolint:dupl // Like와 Dislike는 구조가 유사하나 의미적으로 다른 서비스 메서드
func (s *commentService) LikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error) {
	_, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound
	}

	if s.goodRepo != nil && userID != "" {
		// Check if already liked
		hasGood, err := s.goodRepo.HasGood(boardID, id, userID, "good")
		if err != nil {
			return nil, err
		}
		if hasGood {
			return nil, common.ErrAlreadyRecommended
		}
		// Remove existing dislike if present
		hasNogood, err := s.goodRepo.HasGood(boardID, id, userID, "nogood")
		if err != nil {
			return nil, err
		}
		if hasNogood {
			if err := s.goodRepo.RemoveGood(boardID, id, userID, "nogood"); err != nil {
				return nil, err
			}
		}
		if err := s.goodRepo.AddGood(boardID, id, userID, "good"); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.IncrementLikes(boardID, id); err != nil {
			return nil, err
		}
	}

	updated, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, err
	}

	return &domain.CommentLikeResponse{
		Likes:        updated.Likes,
		Dislikes:     updated.Dislikes,
		UserLiked:    true,
		UserDisliked: false,
	}, nil
}

// DislikeComment increments the dislike count for a comment with duplicate check via g5_board_good
// DB UNIQUE KEY: (bo_table, wr_id, mb_id) — 한 사용자당 하나의 액션만 허용
//
//nolint:dupl // Like와 Dislike는 구조가 유사하나 의미적으로 다른 서비스 메서드
func (s *commentService) DislikeComment(boardID string, id int, userID string) (*domain.CommentLikeResponse, error) {
	_, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, common.ErrPostNotFound
	}

	if s.goodRepo != nil && userID != "" {
		// Check if already disliked
		hasNogood, err := s.goodRepo.HasGood(boardID, id, userID, "nogood")
		if err != nil {
			return nil, err
		}
		if hasNogood {
			return nil, common.ErrAlreadyRecommended
		}
		// Remove existing like if present
		hasGood, err := s.goodRepo.HasGood(boardID, id, userID, "good")
		if err != nil {
			return nil, err
		}
		if hasGood {
			if err := s.goodRepo.RemoveGood(boardID, id, userID, "good"); err != nil {
				return nil, err
			}
		}
		if err := s.goodRepo.AddGood(boardID, id, userID, "nogood"); err != nil {
			return nil, err
		}
	} else {
		if err := s.repo.IncrementDislikes(boardID, id); err != nil {
			return nil, err
		}
	}

	updated, err := s.repo.FindByID(boardID, id)
	if err != nil {
		return nil, err
	}

	return &domain.CommentLikeResponse{
		Likes:        updated.Likes,
		Dislikes:     updated.Dislikes,
		UserLiked:    false,
		UserDisliked: true,
	}, nil
}

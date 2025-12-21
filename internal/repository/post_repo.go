package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// PostRepository 게시글 저장소 인터페이스
type PostRepository interface {
	// 조회
	ListByBoard(boardID string, page, limit int) ([]*domain.Post, int64, error)
	FindByID(boardID string, id int) (*domain.Post, error)
	Search(boardID string, keyword string, page, limit int) ([]*domain.Post, int64, error)

	// 작성/수정/삭제
	Create(boardID string, post *domain.Post) error
	Update(boardID string, id int, post *domain.Post) error
	Delete(boardID string, id int) error

	// 통계
	IncrementHit(boardID string, id int) error
	IncrementLike(boardID string, id int) error
	DecrementLike(boardID string, id int) error
}

// postRepository GORM 구현체
type postRepository struct {
	db *gorm.DB
}

// NewPostRepository 생성자
func NewPostRepository(db *gorm.DB) PostRepository {
	return &postRepository{db: db}
}

// getTableName 게시판 ID로 동적 테이블명 생성
// 소모임 기능 추가 시 단일 테이블 전략으로 변경 가능
func (r *postRepository) getTableName(boardID string) string {
	return fmt.Sprintf("g5_write_%s", boardID)
}

// ListByBoard 게시판별 게시글 목록 조회
func (r *postRepository) ListByBoard(boardID string, page, limit int) ([]*domain.Post, int64, error) {
	var posts []*domain.Post
	var total int64

	tableName := r.getTableName(boardID)

	// Total count
	query := r.db.Table(tableName).
		Where("wr_is_comment = ?", 0). // 댓글 제외
		Where("wr_parent = wr_id")     // 원글만 (답글 제외)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch posts
	offset := (page - 1) * limit
	err := query.
		Order("wr_num, wr_reply"). // 그누보드 정렬 방식
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	if err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

// FindByID 게시글 상세 조회
func (r *postRepository) FindByID(boardID string, id int) (*domain.Post, error) {
	var post domain.Post
	tableName := r.getTableName(boardID)

	err := r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 0). // 댓글 제외
		First(&post).Error

	if err != nil {
		return nil, err
	}

	return &post, nil
}

// Search 게시글 검색 (제목 + 내용)
func (r *postRepository) Search(boardID string, keyword string, page, limit int) ([]*domain.Post, int64, error) {
	var posts []*domain.Post
	var total int64

	tableName := r.getTableName(boardID)

	query := r.db.Table(tableName).
		Where("wr_is_comment = ?", 0).
		Where("wr_parent = wr_id").
		Where("wr_subject LIKE ? OR wr_content LIKE ?", "%"+keyword+"%", "%"+keyword+"%")

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	err := query.
		Order("wr_num, wr_reply").
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	if err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

// Create 게시글 작성
func (r *postRepository) Create(boardID string, post *domain.Post) error {
	tableName := r.getTableName(boardID)

	// 그누보드 기본값 설정
	post.CreatedAt = time.Now()
	post.ParentID = 0  // 원글
	post.IsComment = 0 // 게시글
	post.Views = 0
	post.Likes = 0
	post.CommentCount = 0

	// 필수 문자열 필드 기본값
	if post.Reply == "" {
		post.Reply = ""
	}
	if post.CommentReply == "" {
		post.CommentReply = ""
	}
	if post.Option == "" {
		post.Option = "html1"
	}
	if post.Link1 == "" {
		post.Link1 = ""
	}
	if post.Link2 == "" {
		post.Link2 = ""
	}
	if post.Email == "" {
		post.Email = ""
	}
	if post.Homepage == "" {
		post.Homepage = ""
	}
	if post.LastUpdated == "" {
		post.LastUpdated = ""
	}
	if post.IP == "" {
		post.IP = ""
	}
	if post.FacebookUser == "" {
		post.FacebookUser = ""
	}
	if post.TwitterUser == "" {
		post.TwitterUser = ""
	}

	// Extra fields (wr_1 to wr_10) 기본값
	post.Extra1 = ""
	post.Extra2 = ""
	post.Extra3 = ""
	post.Extra4 = ""
	post.Extra5 = ""
	post.Extra6 = ""
	post.Extra7 = ""
	post.Extra8 = ""
	post.Extra9 = ""
	post.Extra10 = ""

	// wr_num 값 계산 (가장 작은 음수값 - 1)
	var minNum int
	r.db.Table(tableName).
		Select("COALESCE(MIN(wr_num), 0)").
		Scan(&minNum)
	post.Num = minNum - 1

	// GORM이 zero value를 생략하지 않도록 Select로 모든 필드 명시
	return r.db.Table(tableName).
		Select("*").
		Create(post).Error
}

// Update 게시글 수정
func (r *postRepository) Update(boardID string, id int, post *domain.Post) error {
	tableName := r.getTableName(boardID)

	updates := map[string]interface{}{}
	if post.Title != "" {
		updates["wr_subject"] = post.Title
	}
	if post.Content != "" {
		updates["wr_content"] = post.Content
	}
	if post.Category != "" {
		updates["ca_name"] = post.Category
	}

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 0).
		Updates(updates).Error
}

// Delete 게시글 삭제
func (r *postRepository) Delete(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_is_comment = ?", 0).
		Delete(&domain.Post{}).Error
}

// IncrementHit 조회수 증가
func (r *postRepository) IncrementHit(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		UpdateColumn("wr_hit", gorm.Expr("wr_hit + ?", 1)).
		Error
}

// IncrementLike 좋아요 증가
func (r *postRepository) IncrementLike(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		UpdateColumn("wr_good", gorm.Expr("wr_good + ?", 1)).
		Error
}

// DecrementLike 좋아요 감소
func (r *postRepository) DecrementLike(boardID string, id int) error {
	tableName := r.getTableName(boardID)

	return r.db.Table(tableName).
		Where("wr_id = ?", id).
		Where("wr_good > ?", 0). // 0 이하로 내려가지 않도록
		UpdateColumn("wr_good", gorm.Expr("wr_good - ?", 1)).
		Error
}

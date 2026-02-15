package repository

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// PostRepository 게시글 저장소 인터페이스
type PostRepository interface {
	// 조회
	ListByBoard(boardID string, page, limit int) ([]*domain.Post, int64, error)
	ListNotices(boardID string) ([]*domain.Post, error)
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

	tableName := r.getTableName(boardID)

	// g5_board에서 캐시된 게시글 수 조회 (COUNT(*) 대신 사용 - 성능 최적화)
	// 기존 COUNT(*) 쿼리: 60만건 테이블에서 2.3초 소요
	// g5_board.bo_count_write 사용: ~5ms
	var board struct {
		CountWrite int64 `gorm:"column:bo_count_write"`
	}
	if err := r.db.Table("g5_board").
		Select("bo_count_write").
		Where("bo_table = ?", boardID).
		First(&board).Error; err != nil {
		// 캐시 조회 실패 시 0으로 설정 (fallback)
		board.CountWrite = 0
	}

	// Fetch posts
	offset := (page - 1) * limit
	err := r.db.Table(tableName).
		Where("wr_is_comment = ?", 0). // 댓글 제외
		Where("wr_parent = wr_id").    // 원글만 (답글 제외)
		Order("wr_num, wr_reply").     // 그누보드 정렬 방식
		Offset(offset).
		Limit(limit).
		Find(&posts).Error

	if err != nil {
		return nil, 0, err
	}

	return posts, board.CountWrite, nil
}

// ListNotices 공지사항 목록 조회 (g5_board.bo_notice에서 ID 목록 가져옴)
func (r *postRepository) ListNotices(boardID string) ([]*domain.Post, error) {
	// 1. g5_board에서 bo_notice 조회 (쉼표로 구분된 게시글 ID 목록)
	var board struct {
		Notice string `gorm:"column:bo_notice"`
	}
	if err := r.db.Table("g5_board").
		Select("bo_notice").
		Where("bo_table = ?", boardID).
		First(&board).Error; err != nil {
		return nil, err
	}

	// 공지사항이 없는 경우
	if board.Notice == "" {
		return []*domain.Post{}, nil
	}

	// 2. 쉼표로 구분된 ID 파싱
	noticeIDs := strings.Split(board.Notice, ",")
	if len(noticeIDs) == 0 {
		return []*domain.Post{}, nil
	}

	// ID 문자열을 정수로 변환
	ids := make([]int, 0, len(noticeIDs))
	for _, idStr := range noticeIDs {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		id, err := strconv.Atoi(idStr)
		if err != nil {
			continue // 숫자가 아닌 값은 무시
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return []*domain.Post{}, nil
	}

	// 3. 해당 ID의 게시글 조회
	var posts []*domain.Post
	tableName := r.getTableName(boardID)

	err := r.db.Table(tableName).
		Where("wr_id IN ?", ids).
		Where("wr_is_comment = ?", 0).
		Order("FIELD(wr_id, " + strings.Join(noticeIDs, ",") + ")"). // bo_notice 순서 유지
		Find(&posts).Error

	if err != nil {
		return nil, err
	}

	return posts, nil
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

package repository

import (
	"errors"
	"fmt"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

type BoardRepository struct {
	db *gorm.DB
}

func NewBoardRepository(db *gorm.DB) *BoardRepository {
	return &BoardRepository{db: db}
}

// Create - 게시판 생성 및 동적 테이블 생성
func (r *BoardRepository) Create(board *domain.Board) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. g5_board 테이블에 게시판 설정 저장
		if err := tx.Create(board).Error; err != nil {
			return err
		}

		// 2. 동적 write 테이블 생성 (g5_write_{board_id})
		return r.createWriteTable(tx, board.BoardID)
	})
}

// createWriteTable - 동적 게시판 테이블 생성 (그누보드 sql_write.sql 템플릿 기반)
func (r *BoardRepository) createWriteTable(tx *gorm.DB, boardID string) error {
	tableName := fmt.Sprintf("g5_write_%s", boardID)

	// 그누보드 5 표준 write 테이블 구조
	createTableSQL := fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
  wr_id int(11) NOT NULL AUTO_INCREMENT,
  wr_num int(11) NOT NULL DEFAULT 0,
  wr_reply varchar(10) NOT NULL,
  wr_parent int(11) NOT NULL DEFAULT 0,
  wr_is_comment tinyint(4) NOT NULL DEFAULT 0,
  wr_comment int(11) NOT NULL DEFAULT 0,
  ca_name varchar(255) NOT NULL,
  wr_option set('html1','html2','secret','mail') NOT NULL,
  wr_subject varchar(255) NOT NULL,
  wr_content text NOT NULL,
  wr_seo_title varchar(255) NOT NULL DEFAULT '',
  wr_link1 text NOT NULL,
  wr_link2 text NOT NULL,
  wr_link1_hit int(11) NOT NULL DEFAULT 0,
  wr_link2_hit int(11) NOT NULL DEFAULT 0,
  wr_hit int(11) NOT NULL DEFAULT 0,
  wr_good int(11) NOT NULL DEFAULT 0,
  wr_nogood int(11) NOT NULL DEFAULT 0,
  mb_id varchar(20) NOT NULL,
  wr_password varchar(255) NOT NULL,
  wr_name varchar(255) NOT NULL,
  wr_email varchar(255) NOT NULL,
  wr_homepage varchar(255) NOT NULL,
  wr_datetime datetime NOT NULL DEFAULT '0000-00-00 00:00:00',
  wr_file tinyint(4) NOT NULL DEFAULT 0,
  wr_last varchar(19) NOT NULL,
  wr_ip varchar(255) NOT NULL,
  wr_facebook_user varchar(255) NOT NULL,
  wr_twitter_user varchar(255) NOT NULL,
  wr_1 varchar(255) NOT NULL,
  wr_2 varchar(255) NOT NULL,
  wr_3 varchar(255) NOT NULL,
  wr_4 varchar(255) NOT NULL,
  wr_5 varchar(255) NOT NULL,
  wr_6 varchar(255) NOT NULL,
  wr_7 varchar(255) NOT NULL,
  wr_8 varchar(255) NOT NULL,
  wr_9 varchar(255) NOT NULL,
  wr_10 varchar(255) NOT NULL,
  PRIMARY KEY (wr_id),
  KEY wr_num_reply_parent (wr_num,wr_reply,wr_parent),
  KEY wr_is_comment (wr_is_comment,wr_id)
) ENGINE=MyISAM DEFAULT CHARSET=utf8;
`, tableName)

	return tx.Exec(createTableSQL).Error
}

// FindByID - 게시판 ID로 조회
func (r *BoardRepository) FindByID(boardID string) (*domain.Board, error) {
	var board domain.Board
	err := r.db.Where("bo_table = ?", boardID).First(&board).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, common.ErrNotFound
		}
		return nil, err
	}

	return &board, nil
}

// FindAll - 모든 게시판 목록 조회
func (r *BoardRepository) FindAll(offset, limit int) ([]domain.Board, int64, error) {
	var boards []domain.Board
	var total int64

	// 전체 개수 조회
	if err := r.db.Model(&domain.Board{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 페이징 조회
	err := r.db.Order("bo_order ASC, bo_table ASC").
		Offset(offset).
		Limit(limit).
		Find(&boards).Error

	if err != nil {
		return nil, 0, err
	}

	return boards, total, nil
}

// FindByGroupID - 그룹별 게시판 목록 조회
func (r *BoardRepository) FindByGroupID(groupID string) ([]domain.Board, error) {
	var boards []domain.Board
	err := r.db.Where("gr_id = ?", groupID).
		Order("bo_order ASC, bo_table ASC").
		Find(&boards).Error

	return boards, err
}

// Update - 게시판 정보 수정
func (r *BoardRepository) Update(boardID string, updates map[string]interface{}) error {
	result := r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return common.ErrNotFound
	}

	return nil
}

// Delete - 게시판 삭제 (설정 + 동적 테이블)
func (r *BoardRepository) Delete(boardID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. 동적 write 테이블 삭제
		tableName := fmt.Sprintf("g5_write_%s", boardID)
		dropTableSQL := fmt.Sprintf("DROP TABLE IF EXISTS %s", tableName)
		if err := tx.Exec(dropTableSQL).Error; err != nil {
			return err
		}

		// 2. g5_board 테이블에서 설정 삭제
		result := tx.Where("bo_table = ?", boardID).Delete(&domain.Board{})
		if result.Error != nil {
			return result.Error
		}

		if result.RowsAffected == 0 {
			return common.ErrNotFound
		}

		// 3. g5_board_file 테이블에서 관련 파일 정보 삭제
		if err := tx.Exec("DELETE FROM g5_board_file WHERE bo_table = ?", boardID).Error; err != nil {
			return err
		}

		// 4. g5_board_new 테이블에서 관련 신규 글 정보 삭제
		return tx.Exec("DELETE FROM g5_board_new WHERE bo_table = ?", boardID).Error
	})
}

// Exists - 게시판 존재 여부 확인
func (r *BoardRepository) Exists(boardID string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		Count(&count).Error

	return count > 0, err
}

// IncrementWriteCount - 게시글 수 증가
func (r *BoardRepository) IncrementWriteCount(boardID string) error {
	return r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		UpdateColumn("bo_count_write", gorm.Expr("bo_count_write + ?", 1)).Error
}

// DecrementWriteCount - 게시글 수 감소
func (r *BoardRepository) DecrementWriteCount(boardID string) error {
	return r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		UpdateColumn("bo_count_write", gorm.Expr("bo_count_write - ?", 1)).Error
}

// IncrementCommentCount - 댓글 수 증가
func (r *BoardRepository) IncrementCommentCount(boardID string) error {
	return r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		UpdateColumn("bo_count_comment", gorm.Expr("bo_count_comment + ?", 1)).Error
}

// DecrementCommentCount - 댓글 수 감소
func (r *BoardRepository) DecrementCommentCount(boardID string) error {
	return r.db.Model(&domain.Board{}).
		Where("bo_table = ?", boardID).
		UpdateColumn("bo_count_comment", gorm.Expr("bo_count_comment - ?", 1)).Error
}

package service

import (
	"fmt"
	"regexp"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

type BoardService struct {
	repo *repository.BoardRepository
}

func NewBoardService(repo *repository.BoardRepository) *BoardService {
	return &BoardService{repo: repo}
}

// CreateBoard - 게시판 생성 (관리자만 가능)
func (s *BoardService) CreateBoard(req *domain.CreateBoardRequest, adminID string) (*domain.Board, error) {
	// 1. board_id 유효성 검증 (영문+숫자만 허용, 2~20자)
	if !isValidBoardID(req.BoardID) {
		return nil, fmt.Errorf("invalid board_id: must be 2-20 alphanumeric characters")
	}

	// 2. 중복 확인
	exists, err := s.repo.Exists(req.BoardID)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("board_id '%s' already exists", req.BoardID)
	}

	// 3. Board 엔티티 생성
	board := &domain.Board{
		BoardID: req.BoardID,
		GroupID: req.GroupID,
		Subject: req.Subject,
		Admin:   adminID, // 생성자를 관리자로 설정
		Device:  getOrDefault(req.Device, "both"),

		// 기본 권한 레벨 설정
		ListLevel:     getIntOrDefault(req.ListLevel, 1),
		ReadLevel:     getIntOrDefault(req.ReadLevel, 1),
		WriteLevel:    getIntOrDefault(req.WriteLevel, 1),
		ReplyLevel:    getIntOrDefault(req.ReplyLevel, 1),
		CommentLevel:  getIntOrDefault(req.CommentLevel, 1),
		HTMLLevel:     1,
		LinkLevel:     1,
		UploadLevel:   1,
		DownloadLevel: 1,

		// 게시판 옵션
		UseCategory:    getIntOrDefault(req.UseCategory, 0),
		CategoryList:   req.CategoryList,
		UseSideview:    0,
		UseFileContent: 0,
		UseSecret:      0,
		UseDhtml:       0,
		UseRss:         0,
		UseGood:        0,
		UseNogood:      0,
		UseName:        0,
		UseSignature:   0,
		UseIP:          0,
		UseListView:    0,
		UseListContent: 0,
		UseCaptcha:     0,

		// 스킨
		Skin:       getOrDefault(req.Skin, "basic"),
		MobileSkin: getOrDefault(req.MobileSkin, "basic"),

		// 페이징
		PageRows:         getIntOrDefault(req.PageRows, 15),
		MobilePageRows:   15,
		SubjectLen:       60,
		MobileSubjectLen: 30,
		NewRows:          24,
		HotRows:          100,

		// 포인트
		MinWritePoint:   0,
		MaxWritePoint:   0,
		CommentMinPoint: 0,
		CommentMaxPoint: 0,

		// 파일 업로드
		UploadCount: getIntOrDefault(req.UploadCount, 2),
		UploadSize:  getInt64OrDefault(req.UploadSize, 1048576), // 1MB

		// 이미지
		ImageWidth: 835,

		// 통계
		CountWrite:   0,
		CountComment: 0,
	}

	// 4. Repository를 통해 저장
	if err := s.repo.Create(board); err != nil {
		return nil, err
	}

	return board, nil
}

// GetBoard - 게시판 정보 조회
func (s *BoardService) GetBoard(boardID string) (*domain.Board, error) {
	return s.repo.FindByID(boardID)
}

// ListBoards - 게시판 목록 조회
func (s *BoardService) ListBoards(page, pageSize int) ([]domain.Board, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	offset := (page - 1) * pageSize
	return s.repo.FindAll(offset, pageSize)
}

// ListBoardsByGroup - 그룹별 게시판 목록
func (s *BoardService) ListBoardsByGroup(groupID string) ([]domain.Board, error) {
	return s.repo.FindByGroupID(groupID)
}

// UpdateBoard - 게시판 수정 (관리자 또는 게시판 관리자)
func (s *BoardService) UpdateBoard(boardID string, req *domain.UpdateBoardRequest, userID string, isAdmin bool) error {
	// 1. 기존 게시판 조회
	board, err := s.repo.FindByID(boardID)
	if err != nil {
		return err
	}

	// 2. 권한 확인
	if !isAdmin && board.Admin != userID {
		return common.ErrForbidden
	}

	// 3. 업데이트할 필드 준비
	updates := make(map[string]interface{})

	if req.Subject != nil {
		updates["bo_subject"] = *req.Subject
	}
	if req.Admin != nil {
		updates["bo_admin"] = *req.Admin
	}
	if req.Device != nil {
		updates["bo_device"] = *req.Device
	}
	if req.ListLevel != nil {
		updates["bo_list_level"] = *req.ListLevel
	}
	if req.ReadLevel != nil {
		updates["bo_read_level"] = *req.ReadLevel
	}
	if req.WriteLevel != nil {
		updates["bo_write_level"] = *req.WriteLevel
	}
	if req.ReplyLevel != nil {
		updates["bo_reply_level"] = *req.ReplyLevel
	}
	if req.CommentLevel != nil {
		updates["bo_comment_level"] = *req.CommentLevel
	}
	if req.UseCategory != nil {
		updates["bo_use_category"] = *req.UseCategory
	}
	if req.CategoryList != nil {
		updates["bo_category_list"] = *req.CategoryList
	}
	if req.Skin != nil {
		updates["bo_skin"] = *req.Skin
	}
	if req.MobileSkin != nil {
		updates["bo_mobile_skin"] = *req.MobileSkin
	}
	if req.PageRows != nil {
		updates["bo_page_rows"] = *req.PageRows
	}
	if req.UploadCount != nil {
		updates["bo_upload_count"] = *req.UploadCount
	}
	if req.UploadSize != nil {
		updates["bo_upload_size"] = *req.UploadSize
	}

	// 4. 업데이트 실행
	return s.repo.Update(boardID, updates)
}

// DeleteBoard - 게시판 삭제 (관리자만 가능)
func (s *BoardService) DeleteBoard(boardID string) error {
	return s.repo.Delete(boardID)
}

// CanList - 목록 보기 권한 확인
func (s *BoardService) CanList(boardID string, memberLevel int) (bool, error) {
	board, err := s.repo.FindByID(boardID)
	if err != nil {
		return false, err
	}

	return memberLevel >= board.ListLevel, nil
}

// CanRead - 읽기 권한 확인
func (s *BoardService) CanRead(boardID string, memberLevel int) (bool, error) {
	board, err := s.repo.FindByID(boardID)
	if err != nil {
		return false, err
	}

	return memberLevel >= board.ReadLevel, nil
}

// CanWrite - 쓰기 권한 확인
func (s *BoardService) CanWrite(boardID string, memberLevel int) (bool, error) {
	board, err := s.repo.FindByID(boardID)
	if err != nil {
		return false, err
	}

	return memberLevel >= board.WriteLevel, nil
}

// CanComment - 댓글 권한 확인
func (s *BoardService) CanComment(boardID string, memberLevel int) (bool, error) {
	board, err := s.repo.FindByID(boardID)
	if err != nil {
		return false, err
	}

	return memberLevel >= board.CommentLevel, nil
}

// Utility functions

func isValidBoardID(boardID string) bool {
	// 영문+숫자만 허용, 2~20자
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9]{2,20}$`, boardID)
	return matched
}

func getOrDefault(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

func getIntOrDefault(value *int, defaultValue int) int {
	if value == nil {
		return defaultValue
	}
	return *value
}

func getInt64OrDefault(value *int64, defaultValue int64) int64 {
	if value == nil {
		return defaultValue
	}
	return *value
}

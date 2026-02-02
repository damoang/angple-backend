package v2

import (
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
)

// AdminService handles v2 admin business logic
type AdminService struct {
	userRepo    v2repo.UserRepository
	boardRepo   v2repo.BoardRepository
	postRepo    v2repo.PostRepository
	commentRepo v2repo.CommentRepository
}

// NewAdminService creates a new AdminService
func NewAdminService(
	userRepo v2repo.UserRepository,
	boardRepo v2repo.BoardRepository,
	postRepo v2repo.PostRepository,
	commentRepo v2repo.CommentRepository,
) *AdminService {
	return &AdminService{
		userRepo:    userRepo,
		boardRepo:   boardRepo,
		postRepo:    postRepo,
		commentRepo: commentRepo,
	}
}

// DashboardStats represents admin dashboard statistics
type DashboardStats struct {
	TotalUsers    int64 `json:"total_users"`
	TotalPosts    int64 `json:"total_posts"`
	TotalComments int64 `json:"total_comments"`
	TotalBoards   int64 `json:"total_boards"`
	RecentUsers   int64 `json:"recent_users"`
	RecentPosts   int64 `json:"recent_posts"`
}

// ListAllBoards returns all boards including inactive ones
func (s *AdminService) ListAllBoards() ([]*v2domain.V2Board, error) {
	return s.boardRepo.FindAllIncludingInactive()
}

// CreateBoard creates a new board
func (s *AdminService) CreateBoard(board *v2domain.V2Board) error {
	return s.boardRepo.Create(board)
}

// UpdateBoard updates a board
func (s *AdminService) UpdateBoard(board *v2domain.V2Board) error {
	return s.boardRepo.Update(board)
}

// DeleteBoard deletes a board
func (s *AdminService) DeleteBoard(id uint64) error {
	return s.boardRepo.Delete(id)
}

// GetMember returns a user by ID
func (s *AdminService) GetMember(id uint64) (*v2domain.V2User, error) {
	return s.userRepo.FindByID(id)
}

// ListMembers returns paginated user list with optional search
func (s *AdminService) ListMembers(page, perPage int, keyword string) ([]*v2domain.V2User, int64, error) {
	return s.userRepo.FindAll(page, perPage, keyword)
}

// UpdateMember updates a user
func (s *AdminService) UpdateMember(user *v2domain.V2User) error {
	return s.userRepo.Update(user)
}

// BanMember bans or unbans a user
func (s *AdminService) BanMember(id uint64, ban bool) error {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return err
	}
	if ban {
		user.Status = "banned"
	} else {
		user.Status = "active"
	}
	return s.userRepo.Update(user)
}

// GetDashboardStats returns admin dashboard statistics
func (s *AdminService) GetDashboardStats() (*DashboardStats, error) {
	totalUsers, err := s.userRepo.Count()
	if err != nil {
		return nil, err
	}
	totalPosts, err := s.postRepo.Count()
	if err != nil {
		return nil, err
	}
	totalComments, err := s.commentRepo.Count()
	if err != nil {
		return nil, err
	}

	boards, err := s.boardRepo.FindAllIncludingInactive()
	if err != nil {
		return nil, err
	}

	since := time.Now().AddDate(0, 0, -7)
	recentUsers, err := s.userRepo.CountSince(since)
	if err != nil {
		return nil, err
	}
	recentPosts, err := s.postRepo.CountSince(since)
	if err != nil {
		return nil, err
	}

	return &DashboardStats{
		TotalUsers:    totalUsers,
		TotalPosts:    totalPosts,
		TotalComments: totalComments,
		TotalBoards:   int64(len(boards)),
		RecentUsers:   recentUsers,
		RecentPosts:   recentPosts,
	}, nil
}

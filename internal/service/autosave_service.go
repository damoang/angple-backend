package service

import (
	"strings"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// AutosaveService handles autosave business logic
type AutosaveService interface {
	// Save saves or updates an autosave draft
	Save(memberID string, req *domain.AutosaveRequest) (int64, error)
	// List returns all autosaves for a member
	List(memberID string) ([]domain.AutosaveListItem, error)
	// Load returns a specific autosave
	Load(id int, memberID string) (*domain.AutosaveDetail, error)
	// Delete removes an autosave
	Delete(id int, memberID string) (int64, error)
}

type autosaveService struct {
	repo repository.AutosaveRepository
}

// NewAutosaveService creates a new AutosaveService
func NewAutosaveService(repo repository.AutosaveRepository) AutosaveService {
	return &autosaveService{repo: repo}
}

// Save saves or updates an autosave draft
func (s *autosaveService) Save(memberID string, req *domain.AutosaveRequest) (int64, error) {
	// Trim and limit subject/content
	subject := strings.TrimSpace(req.Subject)
	content := strings.TrimSpace(req.Content)

	if len(subject) > 255 {
		subject = subject[:255]
	}
	if len(content) > 65536 {
		content = content[:65536]
	}

	// Check if same content already exists (skip if so)
	exists, err := s.repo.ExistsSameContent(memberID, subject, content)
	if err != nil {
		return 0, err
	}
	if exists {
		// Same content exists, return current count without saving
		return s.repo.Count(memberID)
	}

	// Save the autosave
	autosave := &domain.Autosave{
		MemberID: memberID,
		UID:      req.UID,
		Subject:  subject,
		Content:  content,
	}

	if err := s.repo.Save(autosave); err != nil {
		return 0, err
	}

	return s.repo.Count(memberID)
}

// List returns all autosaves for a member
func (s *autosaveService) List(memberID string) ([]domain.AutosaveListItem, error) {
	autosaves, err := s.repo.FindByMemberID(memberID)
	if err != nil {
		return nil, err
	}

	items := make([]domain.AutosaveListItem, len(autosaves))
	for i, a := range autosaves {
		subject := a.Subject
		// Truncate subject for display (25 chars)
		if len(subject) > 25 {
			subject = subject[:25] + "..."
		}

		items[i] = domain.AutosaveListItem{
			ID:        a.ID,
			UID:       a.UID,
			Subject:   subject,
			CreatedAt: a.CreatedAt.Format("06-01-02 15:04"),
		}
	}

	return items, nil
}

// Load returns a specific autosave
func (s *autosaveService) Load(id int, memberID string) (*domain.AutosaveDetail, error) {
	autosave, err := s.repo.FindByID(id, memberID)
	if err != nil {
		return nil, err
	}

	return &domain.AutosaveDetail{
		ID:        autosave.ID,
		Subject:   autosave.Subject,
		Content:   autosave.Content,
		CreatedAt: autosave.CreatedAt.Format("2006-01-02 15:04:05"),
	}, nil
}

// Delete removes an autosave and returns the remaining count
func (s *autosaveService) Delete(id int, memberID string) (int64, error) {
	if err := s.repo.Delete(id, memberID); err != nil {
		return -1, err
	}

	return s.repo.Count(memberID)
}

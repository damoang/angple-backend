package service

import (
	"errors"
	"math"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// NotificationService handles notification business logic
type NotificationService struct {
	repo *repository.NotificationRepository
}

// NewNotificationService creates a new NotificationService
func NewNotificationService(repo *repository.NotificationRepository) *NotificationService {
	return &NotificationService{repo: repo}
}

// GetUnreadCount returns the unread notification count for a member
func (s *NotificationService) GetUnreadCount(memberID string) (*domain.NotificationSummaryResponse, error) {
	count, err := s.repo.GetUnreadCount(memberID)
	if err != nil {
		return nil, err
	}
	return &domain.NotificationSummaryResponse{TotalUnread: int(count)}, nil
}

// GetList returns paginated notifications for a member
func (s *NotificationService) GetList(memberID string, page, limit int) (*domain.NotificationListResponse, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	offset := (page - 1) * limit
	notifications, total, err := s.repo.GetList(memberID, offset, limit)
	if err != nil {
		return nil, err
	}

	unreadCount, err := s.repo.GetUnreadCount(memberID)
	if err != nil {
		return nil, err
	}

	items := make([]domain.NotificationItem, len(notifications))
	for i, n := range notifications {
		items[i] = domain.NotificationItem{
			ID:         n.ID,
			Type:       n.Type,
			Title:      n.Title,
			Content:    n.Content,
			URL:        n.URL,
			SenderID:   n.SenderID,
			SenderName: n.SenderName,
			IsRead:     n.IsRead,
			CreatedAt:  n.CreatedAt.Format(time.RFC3339),
		}
	}

	totalPages := int(math.Ceil(float64(total) / float64(limit)))

	return &domain.NotificationListResponse{
		Items:       items,
		Total:       total,
		UnreadCount: unreadCount,
		Page:        page,
		Limit:       limit,
		TotalPages:  totalPages,
	}, nil
}

// MarkAsRead marks a notification as read after ownership check
func (s *NotificationService) MarkAsRead(memberID string, notificationID int) error {
	n, err := s.repo.FindByID(notificationID)
	if err != nil {
		return err
	}
	if n == nil {
		return errors.New("notification not found")
	}
	if n.MemberID != memberID {
		return errors.New("forbidden")
	}
	return s.repo.MarkAsRead(notificationID)
}

// MarkAllAsRead marks all notifications as read for a member
func (s *NotificationService) MarkAllAsRead(memberID string) error {
	return s.repo.MarkAllAsRead(memberID)
}

// Delete deletes a notification after ownership check
func (s *NotificationService) Delete(memberID string, notificationID int) error {
	n, err := s.repo.FindByID(notificationID)
	if err != nil {
		return err
	}
	if n == nil {
		return errors.New("notification not found")
	}
	if n.MemberID != memberID {
		return errors.New("forbidden")
	}
	return s.repo.Delete(notificationID)
}

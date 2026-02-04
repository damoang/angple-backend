package service

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
	"gorm.io/gorm"
)

// AdminMemberService handles admin member operations
type AdminMemberService interface {
	ListMembers(page, limit int, keyword string) ([]*domain.AdminMemberListItem, *common.Meta, error)
	GetMember(id int) (*domain.AdminMemberDetail, error)
	UpdateMember(id int, req *domain.AdminMemberUpdateRequest) error
	AdjustPoint(id int, req *domain.AdminPointAdjustRequest) error
	RestrictMember(id int, req *domain.AdminRestrictRequest) error
}

type adminMemberService struct {
	memberRepo repository.MemberRepository
	db         *gorm.DB
}

// NewAdminMemberService creates a new AdminMemberService
func NewAdminMemberService(memberRepo repository.MemberRepository, db *gorm.DB) AdminMemberService {
	return &adminMemberService{memberRepo: memberRepo, db: db}
}

// ListMembers returns paginated member list for admin
func (s *adminMemberService) ListMembers(page, limit int, keyword string) ([]*domain.AdminMemberListItem, *common.Meta, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	members, total, err := s.memberRepo.FindAll(page, limit, keyword)
	if err != nil {
		return nil, nil, err
	}

	items := make([]*domain.AdminMemberListItem, len(members))
	for i, m := range members {
		items[i] = m.ToAdminListItem()
	}

	meta := &common.Meta{Page: page, Limit: limit, Total: total}
	return items, meta, nil
}

// GetMember returns detailed member info for admin
func (s *adminMemberService) GetMember(id int) (*domain.AdminMemberDetail, error) {
	member, err := s.memberRepo.FindByID(id)
	if err != nil {
		return nil, err
	}
	return member.ToAdminDetail(), nil
}

// UpdateMember updates member fields for admin
func (s *adminMemberService) UpdateMember(id int, req *domain.AdminMemberUpdateRequest) error {
	fields := make(map[string]interface{})
	if req.Nickname != nil {
		fields["mb_nick"] = *req.Nickname
	}
	if req.Name != nil {
		fields["mb_name"] = *req.Name
	}
	if req.Email != nil {
		fields["mb_email"] = *req.Email
	}
	if req.Level != nil {
		fields["mb_level"] = *req.Level
	}
	if req.Memo != nil {
		fields["mb_memo"] = *req.Memo
	}
	if len(fields) == 0 {
		return nil
	}
	return s.memberRepo.UpdateFields(id, fields)
}

// AdjustPoint adjusts member point and records in g5_point
func (s *adminMemberService) AdjustPoint(id int, req *domain.AdminPointAdjustRequest) error {
	member, err := s.memberRepo.FindByID(id)
	if err != nil {
		return err
	}

	newPoint := member.Point + req.Point
	if newPoint < 0 {
		newPoint = 0
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		// Update member point
		if err := tx.Model(&domain.Member{}).Where("mb_no = ?", id).Update("mb_point", newPoint).Error; err != nil {
			return err
		}
		// Record in g5_point
		point := &domain.Point{
			MbID:      member.UserID,
			Point:     req.Point,
			Content:   req.Content,
			MbPoint:   newPoint,
			RelTable:  "admin",
			RelAction: "point_adjust",
			RelID:     fmt.Sprintf("%d", id),
			Datetime:  time.Now().Format("2006-01-02 15:04:05"),
		}
		return tx.Create(point).Error
	})
}

// RestrictMember sets or clears the intercept date on a member
func (s *adminMemberService) RestrictMember(id int, req *domain.AdminRestrictRequest) error {
	return s.memberRepo.UpdateFields(id, map[string]interface{}{
		"mb_intercept_date": req.InterceptDate,
	})
}

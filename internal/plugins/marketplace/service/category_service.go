package service

import (
	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/domain"
	"github.com/damoang/angple-backend/internal/plugins/marketplace/repository"
)

// CategoryService 카테고리 서비스 인터페이스
type CategoryService interface {
	CreateCategory(req *domain.CreateCategoryRequest) (*domain.CategoryResponse, error)
	GetCategory(id uint64) (*domain.CategoryResponse, error)
	UpdateCategory(id uint64, req *domain.UpdateCategoryRequest) error
	DeleteCategory(id uint64) error
	ListCategories() ([]*domain.CategoryResponse, error)
	ListCategoryTree() ([]*domain.CategoryResponse, error)
}

type categoryService struct {
	categoryRepo repository.CategoryRepository
}

// NewCategoryService 카테고리 서비스 생성
func NewCategoryService(categoryRepo repository.CategoryRepository) CategoryService {
	return &categoryService{
		categoryRepo: categoryRepo,
	}
}

func (s *categoryService) CreateCategory(req *domain.CreateCategoryRequest) (*domain.CategoryResponse, error) {
	// 부모 카테고리 유효성 검사
	if req.ParentID != nil {
		_, err := s.categoryRepo.FindByID(*req.ParentID)
		if err != nil {
			return nil, common.ErrBadRequest
		}
	}

	// 슬러그 중복 검사
	existing, _ := s.categoryRepo.FindBySlug(req.Slug)
	if existing != nil {
		return nil, common.ErrBadRequest
	}

	category := &domain.Category{
		ParentID: req.ParentID,
		Name:     req.Name,
		Slug:     req.Slug,
		Icon:     req.Icon,
		OrderNum: req.OrderNum,
		IsActive: true,
	}

	if err := s.categoryRepo.Create(category); err != nil {
		return nil, err
	}

	return category.ToResponse(), nil
}

func (s *categoryService) GetCategory(id uint64) (*domain.CategoryResponse, error) {
	category, err := s.categoryRepo.FindByID(id)
	if err != nil {
		return nil, common.ErrNotFound
	}
	return category.ToResponse(), nil
}

func (s *categoryService) UpdateCategory(id uint64, req *domain.UpdateCategoryRequest) error {
	category, err := s.categoryRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	if req.ParentID != nil {
		// 자기 자신을 부모로 설정 방지
		if *req.ParentID == id {
			return common.ErrBadRequest
		}
		category.ParentID = req.ParentID
	}
	if req.Name != nil {
		category.Name = *req.Name
	}
	if req.Slug != nil {
		// 슬러그 중복 검사
		existing, _ := s.categoryRepo.FindBySlug(*req.Slug)
		if existing != nil && existing.ID != id {
			return common.ErrBadRequest
		}
		category.Slug = *req.Slug
	}
	if req.Icon != nil {
		category.Icon = *req.Icon
	}
	if req.OrderNum != nil {
		category.OrderNum = *req.OrderNum
	}
	if req.IsActive != nil {
		category.IsActive = *req.IsActive
	}

	return s.categoryRepo.Update(category)
}

func (s *categoryService) DeleteCategory(id uint64) error {
	category, err := s.categoryRepo.FindByID(id)
	if err != nil {
		return common.ErrNotFound
	}

	// 하위 카테고리가 있으면 삭제 불가
	children, _ := s.categoryRepo.ListByParent(id)
	if len(children) > 0 {
		return common.ErrBadRequest
	}

	// 상품이 있으면 삭제 불가
	if category.ItemCount > 0 {
		return common.ErrBadRequest
	}

	return s.categoryRepo.Delete(id)
}

func (s *categoryService) ListCategories() ([]*domain.CategoryResponse, error) {
	categories, err := s.categoryRepo.ListActive()
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.CategoryResponse, len(categories))
	for i, cat := range categories {
		responses[i] = cat.ToResponse()
	}

	return responses, nil
}

func (s *categoryService) ListCategoryTree() ([]*domain.CategoryResponse, error) {
	roots, err := s.categoryRepo.ListRoots()
	if err != nil {
		return nil, err
	}

	responses := make([]*domain.CategoryResponse, len(roots))
	for i, root := range roots {
		responses[i] = root.ToResponse()
	}

	return responses, nil
}

package service

import (
	"errors"
	"testing"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock PostRepository ---

type mockPostRepo struct {
	mock.Mock
}

func (m *mockPostRepo) ListByBoard(boardID string, page, limit int) ([]*domain.Post, int64, error) {
	args := m.Called(boardID, page, limit)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Post), args.Get(1).(int64), args.Error(2)
}

func (m *mockPostRepo) FindByID(boardID string, id int) (*domain.Post, error) {
	args := m.Called(boardID, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Post), args.Error(1)
}

func (m *mockPostRepo) Search(boardID string, keyword string, page, limit int) ([]*domain.Post, int64, error) {
	args := m.Called(boardID, keyword, page, limit)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Post), args.Get(1).(int64), args.Error(2)
}

func (m *mockPostRepo) Create(boardID string, post *domain.Post) error {
	return m.Called(boardID, post).Error(0)
}

func (m *mockPostRepo) Update(boardID string, id int, post *domain.Post) error {
	return m.Called(boardID, id, post).Error(0)
}

func (m *mockPostRepo) Delete(boardID string, id int) error {
	return m.Called(boardID, id).Error(0)
}

func (m *mockPostRepo) IncrementHit(boardID string, id int) error {
	return m.Called(boardID, id).Error(0)
}

func (m *mockPostRepo) IncrementLike(boardID string, id int) error {
	return m.Called(boardID, id).Error(0)
}

func (m *mockPostRepo) DecrementLike(boardID string, id int) error {
	return m.Called(boardID, id).Error(0)
}

// --- Tests ---

func TestListPosts_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	posts := []*domain.Post{
		{ID: 1, Title: "Test Post", AuthorID: "user1"},
		{ID: 2, Title: "Another Post", AuthorID: "user2"},
	}
	repo.On("ListByBoard", "free", 1, 20).Return(posts, int64(2), nil)

	results, meta, err := svc.ListPosts("free", 1, 20)

	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, int64(2), meta.Total)
	assert.Equal(t, "free", meta.BoardID)
	repo.AssertExpectations(t)
}

func TestListPosts_PaginationDefaults(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("ListByBoard", "free", 1, 20).Return([]*domain.Post{}, int64(0), nil)

	// page < 1 → defaults to 1, limit < 1 → defaults to 20
	_, _, err := svc.ListPosts("free", -1, 0)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestListPosts_LimitCap(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("ListByBoard", "free", 1, 20).Return([]*domain.Post{}, int64(0), nil)

	// limit > 100 → defaults to 20
	_, _, err := svc.ListPosts("free", 1, 200)
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestListPosts_RepoError(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("ListByBoard", "free", 1, 20).Return(nil, int64(0), errors.New("db error"))

	results, meta, err := svc.ListPosts("free", 1, 20)
	assert.Error(t, err)
	assert.Nil(t, results)
	assert.Nil(t, meta)
}

func TestGetPost_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	post := &domain.Post{ID: 1, Title: "Test", AuthorID: "user1"}
	repo.On("FindByID", "free", 1).Return(post, nil)
	repo.On("IncrementHit", "free", 1).Return(nil)

	result, err := svc.GetPost("free", 1)
	assert.NoError(t, err)
	assert.Equal(t, "Test", result.Title)
}

func TestGetPost_NotFound(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("FindByID", "free", 999).Return(nil, errors.New("not found"))

	result, err := svc.GetPost("free", 999)
	assert.ErrorIs(t, err, common.ErrPostNotFound)
	assert.Nil(t, result)
}

func TestCreatePost_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("Create", "free", mock.AnythingOfType("*domain.Post")).Return(nil)

	req := &domain.CreatePostRequest{Title: "New Post", Content: "Content", Author: "tester"}
	result, err := svc.CreatePost("free", req, "user1")

	assert.NoError(t, err)
	assert.Equal(t, "New Post", result.Title)
	repo.AssertExpectations(t)
}

func TestCreatePost_RepoError(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("Create", "free", mock.AnythingOfType("*domain.Post")).Return(errors.New("create failed"))

	req := &domain.CreatePostRequest{Title: "New Post", Content: "Content"}
	result, err := svc.CreatePost("free", req, "user1")

	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestUpdatePost_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	existing := &domain.Post{ID: 1, Title: "Old", AuthorID: "user1"}
	repo.On("FindByID", "free", 1).Return(existing, nil)
	repo.On("Update", "free", 1, mock.AnythingOfType("*domain.Post")).Return(nil)

	req := &domain.UpdatePostRequest{Title: "Updated"}
	err := svc.UpdatePost("free", 1, req, "user1")
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestUpdatePost_NotOwner(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	existing := &domain.Post{ID: 1, AuthorID: "user1"}
	repo.On("FindByID", "free", 1).Return(existing, nil)

	req := &domain.UpdatePostRequest{Title: "Hack"}
	err := svc.UpdatePost("free", 1, req, "hacker")
	assert.ErrorIs(t, err, common.ErrUnauthorized)
}

func TestUpdatePost_NotFound(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	repo.On("FindByID", "free", 999).Return(nil, errors.New("not found"))

	req := &domain.UpdatePostRequest{Title: "Updated"}
	err := svc.UpdatePost("free", 999, req, "user1")
	assert.ErrorIs(t, err, common.ErrPostNotFound)
}

func TestDeletePost_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	existing := &domain.Post{ID: 1, AuthorID: "user1"}
	repo.On("FindByID", "free", 1).Return(existing, nil)
	repo.On("Delete", "free", 1).Return(nil)

	err := svc.DeletePost("free", 1, "user1")
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestDeletePost_NotOwner(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	existing := &domain.Post{ID: 1, AuthorID: "user1"}
	repo.On("FindByID", "free", 1).Return(existing, nil)

	err := svc.DeletePost("free", 1, "hacker")
	assert.ErrorIs(t, err, common.ErrUnauthorized)
}

func TestSearchPosts_Success(t *testing.T) {
	repo := new(mockPostRepo)
	svc := NewPostService(repo, nil)

	posts := []*domain.Post{{ID: 1, Title: "Search Result"}}
	repo.On("Search", "free", "keyword", 1, 20).Return(posts, int64(1), nil)

	results, meta, err := svc.SearchPosts("free", "keyword", 1, 20)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, int64(1), meta.Total)
}

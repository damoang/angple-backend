package service

import (
	"errors"
	"testing"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/pkg/auth"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// --- Mock MemberRepository ---

type mockMemberRepo struct {
	mock.Mock
}

func (m *mockMemberRepo) FindByUserID(userID string) (*domain.Member, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *mockMemberRepo) FindByEmail(email string) (*domain.Member, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *mockMemberRepo) FindByID(id int) (*domain.Member, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *mockMemberRepo) FindByNickname(nickname string) (*domain.Member, error) {
	args := m.Called(nickname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *mockMemberRepo) Create(member *domain.Member) error {
	return m.Called(member).Error(0)
}

func (m *mockMemberRepo) Update(id int, member *domain.Member) error {
	return m.Called(id, member).Error(0)
}

func (m *mockMemberRepo) UpdateLoginTime(userID string) error {
	return m.Called(userID).Error(0)
}

func (m *mockMemberRepo) UpdatePassword(userID string, hashedPassword string) error {
	return m.Called(userID, hashedPassword).Error(0)
}

func (m *mockMemberRepo) ExistsByUserID(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *mockMemberRepo) ExistsByEmail(email string) (bool, error) {
	args := m.Called(email)
	return args.Bool(0), args.Error(1)
}

func (m *mockMemberRepo) ExistsByNickname(nickname string, excludeUserID string) (bool, error) {
	args := m.Called(nickname, excludeUserID)
	return args.Bool(0), args.Error(1)
}

func (m *mockMemberRepo) ExistsByPhone(phone string, excludeUserID string) (bool, error) {
	args := m.Called(phone, excludeUserID)
	return args.Bool(0), args.Error(1)
}

func (m *mockMemberRepo) ExistsByEmailExcluding(email string, excludeUserID string) (bool, error) {
	args := m.Called(email, excludeUserID)
	return args.Bool(0), args.Error(1)
}

func (m *mockMemberRepo) FindAll(page, limit int, keyword string) ([]*domain.Member, int64, error) {
	args := m.Called(page, limit, keyword)
	if args.Get(0) == nil {
		return nil, args.Get(1).(int64), args.Error(2)
	}
	return args.Get(0).([]*domain.Member), args.Get(1).(int64), args.Error(2)
}

func (m *mockMemberRepo) UpdateFields(id int, fields map[string]interface{}) error {
	return m.Called(id, fields).Error(0)
}

// --- Tests ---

func newTestJWTManager() *jwt.Manager {
	return jwt.NewManager("test-secret-key-for-testing-only-32b!", 15, 1440)
}

func TestLogin_Success(t *testing.T) {
	repo := new(mockMemberRepo)
	jwtMgr := newTestJWTManager()
	svc := NewAuthService(repo, jwtMgr, nil)

	hashedPwd := auth.HashPassword("password123")
	member := &domain.Member{
		UserID:   "testuser",
		Password: hashedPwd,
		Nickname: "Tester",
		Level:    2,
	}
	repo.On("FindByUserID", "testuser").Return(member, nil)
	repo.On("UpdateLoginTime", "testuser").Return(nil)

	result, err := svc.Login("testuser", "password123")

	assert.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
	assert.Equal(t, "testuser", result.User.UserID)
}

func TestLogin_UserNotFound(t *testing.T) {
	repo := new(mockMemberRepo)
	jwtMgr := newTestJWTManager()
	svc := NewAuthService(repo, jwtMgr, nil)

	repo.On("FindByUserID", "nobody").Return(nil, errors.New("not found"))

	result, err := svc.Login("nobody", "password")
	assert.ErrorIs(t, err, common.ErrInvalidCredentials)
	assert.Nil(t, result)
}

func TestLogin_WrongPassword(t *testing.T) {
	repo := new(mockMemberRepo)
	jwtMgr := newTestJWTManager()
	svc := NewAuthService(repo, jwtMgr, nil)

	member := &domain.Member{
		UserID:   "testuser",
		Password: auth.HashPassword("correct"),
	}
	repo.On("FindByUserID", "testuser").Return(member, nil)

	result, err := svc.Login("testuser", "wrong")
	assert.ErrorIs(t, err, common.ErrInvalidCredentials)
	assert.Nil(t, result)
}

func TestRegister_Success(t *testing.T) {
	repo := new(mockMemberRepo)
	jwtMgr := newTestJWTManager()
	svc := NewAuthService(repo, jwtMgr, nil)

	repo.On("ExistsByUserID", "newuser").Return(false, nil)
	repo.On("ExistsByEmail", "new@test.com").Return(false, nil)
	repo.On("ExistsByNickname", "NewNick", "").Return(false, nil)
	repo.On("Create", mock.AnythingOfType("*domain.Member")).Return(nil)

	req := &RegisterRequest{
		UserID:   "newuser",
		Password: "pass1234",
		Name:     "New User",
		Nickname: "NewNick",
		Email:    "new@test.com",
	}
	result, err := svc.Register(req)

	assert.NoError(t, err)
	assert.Equal(t, "newuser", result.UserID)
	repo.AssertExpectations(t)
}

func TestRegister_DuplicateUserID(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	repo.On("ExistsByUserID", "existing").Return(true, nil)

	req := &RegisterRequest{UserID: "existing", Password: "p", Name: "N", Nickname: "N", Email: "e@e.com"}
	result, err := svc.Register(req)

	assert.ErrorIs(t, err, common.ErrUserAlreadyExists)
	assert.Nil(t, result)
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	repo.On("ExistsByUserID", "newuser").Return(false, nil)
	repo.On("ExistsByEmail", "dup@test.com").Return(true, nil)

	req := &RegisterRequest{UserID: "newuser", Password: "p", Name: "N", Nickname: "N", Email: "dup@test.com"}
	result, err := svc.Register(req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "이메일")
	assert.Nil(t, result)
}

func TestRegister_DuplicateNickname(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	repo.On("ExistsByUserID", "newuser").Return(false, nil)
	repo.On("ExistsByEmail", "e@e.com").Return(false, nil)
	repo.On("ExistsByNickname", "Taken", "").Return(true, nil)

	req := &RegisterRequest{UserID: "newuser", Password: "p", Name: "N", Nickname: "Taken", Email: "e@e.com"}
	result, err := svc.Register(req)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "닉네임")
	assert.Nil(t, result)
}

func TestWithdraw_Success(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	member := &domain.Member{ID: 1, UserID: "user1", LeaveDate: ""}
	repo.On("FindByUserID", "user1").Return(member, nil)
	repo.On("Update", 1, mock.AnythingOfType("*domain.Member")).Return(nil)

	err := svc.Withdraw("user1")
	assert.NoError(t, err)
	repo.AssertExpectations(t)
}

func TestWithdraw_AlreadyWithdrawn(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	member := &domain.Member{UserID: "user1", LeaveDate: "20260101"}
	repo.On("FindByUserID", "user1").Return(member, nil)

	err := svc.Withdraw("user1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "이미 탈퇴")
}

func TestWithdraw_UserNotFound(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	repo.On("FindByUserID", "nobody").Return(nil, errors.New("not found"))

	err := svc.Withdraw("nobody")
	assert.ErrorIs(t, err, common.ErrUserNotFound)
}

func TestRefreshToken_Success(t *testing.T) {
	repo := new(mockMemberRepo)
	jwtMgr := newTestJWTManager()
	svc := NewAuthService(repo, jwtMgr, nil)

	// Generate a valid refresh token
	refreshToken, _ := jwtMgr.GenerateRefreshToken("user1")

	member := &domain.Member{UserID: "user1", Nickname: "Nick", Level: 2}
	repo.On("FindByUserID", "user1").Return(member, nil)

	result, err := svc.RefreshToken(refreshToken)
	assert.NoError(t, err)
	assert.NotEmpty(t, result.AccessToken)
	assert.NotEmpty(t, result.RefreshToken)
}

func TestRefreshToken_InvalidToken(t *testing.T) {
	repo := new(mockMemberRepo)
	svc := NewAuthService(repo, newTestJWTManager(), nil)

	result, err := svc.RefreshToken("invalid-token")
	assert.ErrorIs(t, err, common.ErrInvalidToken)
	assert.Nil(t, result)
}

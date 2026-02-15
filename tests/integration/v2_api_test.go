package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2handler "github.com/damoang/angple-backend/internal/handler/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"
	v2routes "github.com/damoang/angple-backend/internal/routes/v2"
	v2svc "github.com/damoang/angple-backend/internal/service/v2"
	"github.com/damoang/angple-backend/pkg/jwt"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// V2APISuite is an integration test suite for v2 API endpoints
type V2APISuite struct {
	suite.Suite
	db         *gorm.DB
	router     *gin.Engine
	jwtManager *jwt.Manager
}

func TestV2APISuite(t *testing.T) {
	suite.Run(t, new(V2APISuite))
}

func (s *V2APISuite) SetupSuite() {
	gin.SetMode(gin.TestMode)

	// Use SQLite for tests (no external DB dependency)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	s.Require().NoError(err)
	s.db = db

	// Migrate v2 schema (raw SQL for SQLite compatibility — no enum types)
	for _, ddl := range []string{
		`CREATE TABLE IF NOT EXISTS v2_users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username VARCHAR(50) UNIQUE, email VARCHAR(255) UNIQUE,
			password VARCHAR(255), nickname VARCHAR(100),
			level INTEGER DEFAULT 1, status TEXT DEFAULT 'active',
			avatar_url VARCHAR(500), bio TEXT,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS v2_boards (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			slug VARCHAR(50) UNIQUE, name VARCHAR(100),
			description TEXT, category_id INTEGER,
			settings TEXT, is_active BOOLEAN DEFAULT 1,
			order_num INTEGER DEFAULT 0,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS v2_posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			board_id INTEGER, user_id INTEGER,
			title VARCHAR(255), content TEXT,
			status TEXT DEFAULT 'published',
			view_count INTEGER DEFAULT 0, comment_count INTEGER DEFAULT 0,
			is_notice BOOLEAN DEFAULT 0,
			created_at DATETIME, updated_at DATETIME)`,
		`CREATE TABLE IF NOT EXISTS v2_comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER, user_id INTEGER,
			parent_id INTEGER, content TEXT,
			depth INTEGER DEFAULT 0, status TEXT DEFAULT 'active',
			created_at DATETIME, updated_at DATETIME)`,
	} {
		s.Require().NoError(db.Exec(ddl).Error)
	}

	// JWT manager
	s.jwtManager = jwt.NewManager("test-secret-key-for-integration-tests", 900, 86400)
	damoangManager := jwt.NewDamoangManager("test-damoang-secret-key")

	// Setup repos, handlers, routes
	userRepo := v2repo.NewUserRepository(db)
	postRepo := v2repo.NewPostRepository(db)
	commentRepo := v2repo.NewCommentRepository(db)
	boardRepo := v2repo.NewBoardRepository(db)

	v2Handler := v2handler.NewV2Handler(userRepo, postRepo, commentRepo, boardRepo)
	authSvc := v2svc.NewV2AuthService(userRepo, s.jwtManager, damoangManager)
	authHandler := v2handler.NewV2AuthHandler(authSvc)

	s.router = gin.New()
	v2routes.Setup(s.router, v2Handler, s.jwtManager)
	v2routes.SetupAuth(s.router, authHandler, s.jwtManager)

	// Seed test data
	s.seedTestData()
}

func (s *V2APISuite) seedTestData() {
	hashed, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)

	user := &v2domain.V2User{
		Username: "testuser",
		Email:    "test@example.com",
		Password: string(hashed),
		Nickname: "TestUser",
		Level:    5,
		Status:   "active",
	}
	s.db.Create(user)

	board := &v2domain.V2Board{
		Slug:     "free",
		Name:     "Free Board",
		IsActive: true,
	}
	s.db.Create(board)

	post := &v2domain.V2Post{
		BoardID:      board.ID,
		UserID:       user.ID,
		Title:        "Test Post",
		Content:      "Test content",
		Status:       "published",
		ViewCount:    10,
		CommentCount: 1,
	}
	s.db.Create(post)

	comment := &v2domain.V2Comment{
		PostID:  post.ID,
		UserID:  user.ID,
		Content: "Test comment",
		Status:  "active",
	}
	s.db.Create(comment)
}

// --- Auth Tests ---

func (s *V2APISuite) TestLogin_Success() {
	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)

	var resp map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	assert.NoError(s.T(), err)
	assert.True(s.T(), resp["success"].(bool))

	data := resp["data"].(map[string]interface{})
	assert.NotEmpty(s.T(), data["access_token"])
	assert.NotNil(s.T(), data["user"])
}

func (s *V2APISuite) TestLogin_InvalidPassword() {
	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "wrongpassword",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *V2APISuite) TestLogin_NonexistentUser() {
	body, _ := json.Marshal(map[string]string{
		"username": "nobody",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

// --- Board Tests ---

func (s *V2APISuite) TestListBoards() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *V2APISuite) TestGetBoard() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards/free", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *V2APISuite) TestGetBoard_NotFound() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards/nonexistent", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusNotFound, w.Code)
}

// --- Post Tests ---

func (s *V2APISuite) TestListPosts() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards/free/posts", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *V2APISuite) TestGetPost() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards/free/posts/1", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *V2APISuite) TestCreatePost_Unauthorized() {
	body, _ := json.Marshal(map[string]string{
		"title":   "New Post",
		"content": "New content",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/boards/free/posts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	// Should fail without auth
	assert.Equal(s.T(), http.StatusUnauthorized, w.Code)
}

func (s *V2APISuite) TestCreatePost_Authenticated() {
	// Login first
	token := s.getAuthToken()

	body, _ := json.Marshal(map[string]string{
		"title":   "Authenticated Post",
		"content": "Created with JWT",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/boards/free/posts", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusCreated, w.Code)
}

// --- Comment Tests ---

func (s *V2APISuite) TestListComments() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/boards/free/posts/1/comments", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// --- User Tests ---

func (s *V2APISuite) TestListUsers() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/users", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

func (s *V2APISuite) TestGetUserByUsername() {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/users/username/testuser", nil)
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	assert.Equal(s.T(), http.StatusOK, w.Code)
}

// --- Helper ---

func (s *V2APISuite) getAuthToken() string {
	body, _ := json.Marshal(map[string]string{
		"username": "testuser",
		"password": "password123",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v2/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.router.ServeHTTP(w, req)

	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	return data["access_token"].(string)
}

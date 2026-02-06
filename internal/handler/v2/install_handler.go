package v2

import (
	"database/sql"
	"fmt"
	"net/http"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/pkg/auth"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// InstallHandler handles installation-related endpoints
type InstallHandler struct {
	db *gorm.DB
}

// NewInstallHandler creates a new InstallHandler
func NewInstallHandler(db *gorm.DB) *InstallHandler {
	return &InstallHandler{db: db}
}

// TestDBRequest represents the request body for database connection test
type TestDBRequest struct {
	Host     string `json:"host" binding:"required"`
	Port     int    `json:"port" binding:"required"`
	Database string `json:"database" binding:"required"`
	User     string `json:"user" binding:"required"`
	Password string `json:"password"`
}

// TestDBResponse represents the response for database connection test
type TestDBResponse struct {
	Success         bool     `json:"success"`
	HasExistingData bool     `json:"hasExistingData"`
	Tables          []string `json:"tables"`
	Message         string   `json:"message,omitempty"`
}

// CreateAdminRequest represents the request body for creating admin account
type CreateAdminRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Name     string `json:"name" binding:"required"`
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required,min=8"`
}

// CreateAdminResponse represents the response for creating admin account
type CreateAdminResponse struct {
	Success bool   `json:"success"`
	UserID  string `json:"userId,omitempty"`
	Message string `json:"message"`
}

// TestDB handles POST /api/v2/install/test-db
// @Summary Test database connection
// @Description Tests if the provided database credentials are valid
// @Tags install
// @Accept json
// @Produce json
// @Param request body TestDBRequest true "Database connection details"
// @Success 200 {object} TestDBResponse
// @Failure 400 {object} common.APIResponse
// @Failure 500 {object} common.APIResponse
// @Router /api/v2/install/test-db [post]
func (h *InstallHandler) TestDB(c *gin.Context) {
	var req TestDBRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// Build DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		req.User, req.Password, req.Host, req.Port, req.Database)

	// Test connection
	testDB, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		common.V2Success(c, TestDBResponse{
			Success: false,
			Message: fmt.Sprintf("데이터베이스 연결 실패: %v", err),
		})
		return
	}

	// Get underlying sql.DB to close connection
	sqlDB, err := testDB.DB()
	if err != nil {
		common.V2Success(c, TestDBResponse{
			Success: false,
			Message: fmt.Sprintf("데이터베이스 연결 확인 실패: %v", err),
		})
		return
	}
	defer sqlDB.Close()

	// Test ping
	if err := sqlDB.Ping(); err != nil {
		common.V2Success(c, TestDBResponse{
			Success: false,
			Message: fmt.Sprintf("데이터베이스 응답 없음: %v", err),
		})
		return
	}

	// Check for existing tables
	var tables []string
	rows, err := sqlDB.Query("SHOW TABLES")
	if err != nil {
		common.V2Success(c, TestDBResponse{
			Success: true,
			Message: "연결 성공 (테이블 조회 실패)",
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err == nil {
			tables = append(tables, tableName)
		}
	}

	hasExistingData := len(tables) > 0

	common.V2Success(c, TestDBResponse{
		Success:         true,
		HasExistingData: hasExistingData,
		Tables:          tables,
		Message:         "데이터베이스 연결 성공",
	})
}

// CreateAdmin handles POST /api/v2/install/create-admin
// @Summary Create admin account
// @Description Creates the initial admin account during installation
// @Tags install
// @Accept json
// @Produce json
// @Param request body CreateAdminRequest true "Admin account details"
// @Success 200 {object} CreateAdminResponse
// @Failure 400 {object} common.APIResponse
// @Failure 500 {object} common.APIResponse
// @Router /api/v2/install/create-admin [post]
func (h *InstallHandler) CreateAdmin(c *gin.Context) {
	var req CreateAdminRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.V2ErrorResponse(c, http.StatusBadRequest, "요청 형식이 올바르지 않습니다", err)
		return
	}

	// Check if admin already exists
	var count int64
	h.db.Table("g5_member").Where("mb_level >= 10").Count(&count)
	if count > 0 {
		common.V2Success(c, CreateAdminResponse{
			Success: false,
			Message: "이미 관리자 계정이 존재합니다",
		})
		return
	}

	// Check if username already exists
	var existingUser int64
	h.db.Table("g5_member").Where("mb_id = ?", req.Username).Count(&existingUser)
	if existingUser > 0 {
		common.V2Success(c, CreateAdminResponse{
			Success: false,
			Message: "이미 사용 중인 아이디입니다",
		})
		return
	}

	// Hash password (using Gnuboard-compatible format)
	hashedPassword := auth.HashPassword(req.Password)

	// Create admin member
	now := sql.NullString{String: "now()", Valid: true}
	result := h.db.Exec(`
		INSERT INTO g5_member (
			mb_id, mb_password, mb_name, mb_nick, mb_email,
			mb_level, mb_datetime, mb_ip, mb_intercept_date, mb_leave_date,
			mb_email_certify, mb_memo, mb_adult, mb_dupinfo, mb_profile,
			mb_1, mb_2, mb_3, mb_4, mb_5, mb_6, mb_7, mb_8, mb_9, mb_10
		) VALUES (
			?, ?, ?, ?, ?,
			10, NOW(), ?, '', '',
			NOW(), '', 0, '', '',
			'', '', '', '', '', '', '', '', '', ''
		)
	`, req.Username, hashedPassword, req.Name, req.Name, req.Email, c.ClientIP())

	if result.Error != nil {
		common.V2ErrorResponse(c, http.StatusInternalServerError, "관리자 계정 생성 실패", result.Error)
		return
	}

	_ = now // suppress unused variable warning

	common.V2Success(c, CreateAdminResponse{
		Success: true,
		UserID:  req.Username,
		Message: "관리자 계정이 생성되었습니다",
	})
}

// CheckInstallStatus handles GET /api/v2/install/status
// @Summary Check installation status
// @Description Returns whether the system is already installed
// @Tags install
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v2/install/status [get]
func (h *InstallHandler) CheckInstallStatus(c *gin.Context) {
	// Check if g5_member table exists and has admin
	var adminCount int64
	err := h.db.Table("g5_member").Where("mb_level >= 10").Count(&adminCount).Error

	if err != nil {
		// Table doesn't exist or DB error
		common.V2Success(c, gin.H{
			"installed":    false,
			"hasDatabase":  false,
			"hasAdmin":     false,
			"errorMessage": err.Error(),
		})
		return
	}

	common.V2Success(c, gin.H{
		"installed":   adminCount > 0,
		"hasDatabase": true,
		"hasAdmin":    adminCount > 0,
	})
}

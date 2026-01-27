// 배너 플러그인 HTTP 핸들러
package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var db *gorm.DB

// SetDB DB 인스턴스 설정
func SetDB(database *gorm.DB) {
	db = database
}

// BannerItem 배너 모델
type BannerItem struct {
	ID         int64      `gorm:"primaryKey" json:"id"`
	Title      string     `gorm:"size:100;not null" json:"title"`
	ImageURL   string     `gorm:"size:500" json:"image_url"`
	LinkURL    string     `gorm:"size:500" json:"link_url"`
	Position   string     `gorm:"size:20;not null;default:sidebar" json:"position"`
	StartDate  *time.Time `json:"start_date"`
	EndDate    *time.Time `json:"end_date"`
	Priority   int        `gorm:"default:0" json:"priority"`
	IsActive   bool       `gorm:"default:true" json:"is_active"`
	ClickCount int        `gorm:"default:0" json:"click_count"`
	ViewCount  int        `gorm:"default:0" json:"view_count"`
	AltText    string     `gorm:"size:255" json:"alt_text"`
	Target     string     `gorm:"size:10;default:_blank" json:"target"`
	Memo       string     `gorm:"type:text" json:"memo"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (BannerItem) TableName() string {
	return "banner_items"
}

// BannerClickLog 클릭 로그
type BannerClickLog struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	BannerID  int64     `gorm:"not null" json:"banner_id"`
	MemberID  string    `gorm:"size:50" json:"member_id"`
	IPAddress string    `gorm:"size:45" json:"ip_address"`
	UserAgent string    `gorm:"size:500" json:"user_agent"`
	Referer   string    `gorm:"size:500" json:"referer"`
	CreatedAt time.Time `json:"created_at"`
}

func (BannerClickLog) TableName() string {
	return "banner_click_logs"
}

// CreateBannerRequest 배너 생성 요청
type CreateBannerRequest struct {
	Title     string     `json:"title" binding:"required,max=100"`
	ImageURL  string     `json:"image_url" binding:"max=500"`
	LinkURL   string     `json:"link_url" binding:"max=500"`
	Position  string     `json:"position" binding:"required,oneof=header sidebar content footer"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	Priority  int        `json:"priority"`
	IsActive  bool       `json:"is_active"`
	AltText   string     `json:"alt_text" binding:"max=255"`
	Target    string     `json:"target" binding:"oneof=_self _blank"`
	Memo      string     `json:"memo"`
}

// UpdateBannerRequest 배너 수정 요청
type UpdateBannerRequest struct {
	Title     string     `json:"title" binding:"max=100"`
	ImageURL  string     `json:"image_url" binding:"max=500"`
	LinkURL   string     `json:"link_url" binding:"max=500"`
	Position  string     `json:"position" binding:"oneof=header sidebar content footer"`
	StartDate *time.Time `json:"start_date"`
	EndDate   *time.Time `json:"end_date"`
	Priority  int        `json:"priority"`
	IsActive  *bool      `json:"is_active"`
	AltText   string     `json:"alt_text" binding:"max=255"`
	Target    string     `json:"target" binding:"oneof=_self _blank"`
	Memo      string     `json:"memo"`
}

// ListBanners 공개 배너 목록 (위치별)
// GET /api/plugins/banner/list?position=sidebar
func ListBanners(c *gin.Context) {
	position := c.Query("position")
	now := time.Now()

	query := db.Model(&BannerItem{}).
		Where("is_active = ?", true).
		Where("(start_date IS NULL OR start_date <= ?)", now).
		Where("(end_date IS NULL OR end_date >= ?)", now)

	if position != "" {
		query = query.Where("position = ?", position)
	}

	var banners []BannerItem
	if err := query.Order("priority DESC, created_at DESC").Find(&banners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "배너 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"banners": banners,
		"total":   len(banners),
	})
}

// TrackClick 배너 클릭 트래킹 및 리다이렉트
// GET /api/plugins/banner/:id/click
func TrackClick(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 배너 ID"})
		return
	}

	var banner BannerItem
	if err := db.First(&banner, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "배너를 찾을 수 없습니다"})
		return
	}

	// 클릭 수 증가
	db.Model(&banner).UpdateColumn("click_count", gorm.Expr("click_count + 1"))

	// 클릭 로그 저장
	memberID := ""
	if userID, exists := c.Get("user_id"); exists {
		memberID = userID.(string)
	}

	clickLog := BannerClickLog{
		BannerID:  id,
		MemberID:  memberID,
		IPAddress: c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Referer:   c.GetHeader("Referer"),
	}
	db.Create(&clickLog)

	// 링크로 리다이렉트
	if banner.LinkURL != "" {
		c.Redirect(http.StatusFound, banner.LinkURL)
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// TrackView 배너 노출 트래킹
// POST /api/plugins/banner/:id/view
func TrackView(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 배너 ID"})
		return
	}

	// 노출 수 증가
	if err := db.Model(&BannerItem{}).Where("id = ?", id).
		UpdateColumn("view_count", gorm.Expr("view_count + 1")).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "노출 트래킹 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// AdminListBanners 관리자 배너 목록
// GET /api/plugins/banner/admin/list
func AdminListBanners(c *gin.Context) {
	position := c.Query("position")
	isActive := c.Query("is_active")

	query := db.Model(&BannerItem{})

	if position != "" {
		query = query.Where("position = ?", position)
	}

	if isActive == "true" {
		query = query.Where("is_active = ?", true)
	} else if isActive == "false" {
		query = query.Where("is_active = ?", false)
	}

	var banners []BannerItem
	if err := query.Order("position, priority DESC, created_at DESC").Find(&banners).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "배너 조회 실패"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"banners": banners,
		"total":   len(banners),
	})
}

// CreateBanner 배너 생성
// POST /api/plugins/banner/admin
func CreateBanner(c *gin.Context) {
	var req CreateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	banner := BannerItem{
		Title:     req.Title,
		ImageURL:  req.ImageURL,
		LinkURL:   req.LinkURL,
		Position:  req.Position,
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
		Priority:  req.Priority,
		IsActive:  req.IsActive,
		AltText:   req.AltText,
		Target:    req.Target,
		Memo:      req.Memo,
	}

	if banner.Target == "" {
		banner.Target = "_blank"
	}

	if err := db.Create(&banner).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "배너 생성 실패"})
		return
	}

	c.JSON(http.StatusCreated, banner)
}

// UpdateBanner 배너 수정
// PUT /api/plugins/banner/admin/:id
func UpdateBanner(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 배너 ID"})
		return
	}

	var banner BannerItem
	if err := db.First(&banner, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "배너를 찾을 수 없습니다"})
		return
	}

	var req UpdateBannerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}

	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.ImageURL != "" {
		updates["image_url"] = req.ImageURL
	}
	if req.LinkURL != "" {
		updates["link_url"] = req.LinkURL
	}
	if req.Position != "" {
		updates["position"] = req.Position
	}
	if req.StartDate != nil {
		updates["start_date"] = req.StartDate
	}
	if req.EndDate != nil {
		updates["end_date"] = req.EndDate
	}
	if req.Priority != 0 {
		updates["priority"] = req.Priority
	}
	if req.IsActive != nil {
		updates["is_active"] = *req.IsActive
	}
	if req.AltText != "" {
		updates["alt_text"] = req.AltText
	}
	if req.Target != "" {
		updates["target"] = req.Target
	}
	if req.Memo != "" {
		updates["memo"] = req.Memo
	}

	if err := db.Model(&banner).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "배너 수정 실패"})
		return
	}

	db.First(&banner, id)
	c.JSON(http.StatusOK, banner)
}

// DeleteBanner 배너 삭제
// DELETE /api/plugins/banner/admin/:id
func DeleteBanner(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 배너 ID"})
		return
	}

	result := db.Delete(&BannerItem{}, id)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "배너 삭제 실패"})
		return
	}

	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "배너를 찾을 수 없습니다"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "배너가 삭제되었습니다"})
}

// GetBannerStats 배너 통계
// GET /api/plugins/banner/admin/:id/stats
func GetBannerStats(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "잘못된 배너 ID"})
		return
	}

	var banner BannerItem
	if err := db.First(&banner, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "배너를 찾을 수 없습니다"})
		return
	}

	// CTR (Click Through Rate) 계산
	var ctr float64
	if banner.ViewCount > 0 {
		ctr = float64(banner.ClickCount) / float64(banner.ViewCount) * 100
	}

	// 최근 7일 클릭 통계
	var dailyClicks []struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	}
	db.Model(&BannerClickLog{}).
		Select("DATE(created_at) as date, COUNT(*) as count").
		Where("banner_id = ?", id).
		Where("created_at >= ?", time.Now().AddDate(0, 0, -7)).
		Group("DATE(created_at)").
		Order("date").
		Scan(&dailyClicks)

	c.JSON(http.StatusOK, gin.H{
		"banner_id":    id,
		"title":        banner.Title,
		"view_count":   banner.ViewCount,
		"click_count":  banner.ClickCount,
		"ctr":          ctr,
		"daily_clicks": dailyClicks,
	})
}

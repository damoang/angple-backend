package repository

import (
	"github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
)

// EventRepository 플러그인 이벤트 저장소
type EventRepository struct {
	db *gorm.DB
}

// NewEventRepository 생성자
func NewEventRepository(db *gorm.DB) *EventRepository {
	return &EventRepository{db: db}
}

// Create 이벤트 기록
func (r *EventRepository) Create(event *domain.PluginEvent) error {
	return r.db.Create(event).Error
}

// ListByPlugin 플러그인별 이벤트 조회 (최신순, 기본 50개)
func (r *EventRepository) ListByPlugin(pluginName string, limit int) ([]domain.PluginEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	var list []domain.PluginEvent
	err := r.db.Where("plugin_name = ?", pluginName).
		Order("created_at DESC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

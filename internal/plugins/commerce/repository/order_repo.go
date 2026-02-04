package repository

import (
	"fmt"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"gorm.io/gorm"
)

// OrderRepository 주문 저장소 인터페이스
type OrderRepository interface {
	// 생성/수정
	Create(order *domain.Order) error
	CreateWithItems(order *domain.Order, items []domain.OrderItem) error
	Update(id uint64, order *domain.Order) error
	UpdateStatus(id uint64, status domain.OrderStatus) error

	// 조회
	FindByID(id uint64) (*domain.Order, error)
	FindByIDWithItems(id uint64) (*domain.Order, error)
	FindByOrderNumber(orderNumber string) (*domain.Order, error)
	FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error)

	// 목록 조회
	ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error)
	ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error)
	ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error)

	// 주문 아이템
	FindItemByID(itemID uint64) (*domain.OrderItem, error)
	UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error

	// 주문번호 생성
	GenerateOrderNumber() (string, error)
}

// orderRepository GORM 구현체
type orderRepository struct {
	db *gorm.DB
}

// NewOrderRepository 생성자
func NewOrderRepository(db *gorm.DB) OrderRepository {
	return &orderRepository{db: db}
}

// Create 주문 생성
func (r *orderRepository) Create(order *domain.Order) error {
	now := time.Now()
	order.CreatedAt = now
	order.UpdatedAt = now
	return r.db.Create(order).Error
}

// CreateWithItems 주문과 아이템 함께 생성 (트랜잭션)
func (r *orderRepository) CreateWithItems(order *domain.Order, items []domain.OrderItem) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		order.CreatedAt = now
		order.UpdatedAt = now

		// 주문 생성
		if err := tx.Create(order).Error; err != nil {
			return err
		}

		// 주문 아이템 생성
		for i := range items {
			items[i].OrderID = order.ID
			items[i].CreatedAt = now
			items[i].UpdatedAt = now
		}

		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

// Update 주문 수정
func (r *orderRepository) Update(id uint64, order *domain.Order) error {
	order.UpdatedAt = time.Now()
	return r.db.Model(&domain.Order{}).Where("id = ?", id).Updates(order).Error
}

// UpdateStatus 주문 상태 업데이트
func (r *orderRepository) UpdateStatus(id uint64, status domain.OrderStatus) error {
	updates := map[string]interface{}{
		"status":     status,
		"updated_at": time.Now(),
	}

	// 상태에 따른 타임스탬프 설정
	now := time.Now()
	switch status {
	case domain.OrderStatusPaid:
		updates["paid_at"] = now
	case domain.OrderStatusCompleted:
		updates["completed_at"] = now
	case domain.OrderStatusCancelled:
		updates["cancelled_at"] = now
	case domain.OrderStatusShipped:
		updates["shipped_at"] = now
	case domain.OrderStatusDelivered:
		updates["delivered_at"] = now
	}

	return r.db.Model(&domain.Order{}).Where("id = ?", id).UpdateColumns(updates).Error
}

// FindByID ID로 주문 조회
func (r *orderRepository) FindByID(id uint64) (*domain.Order, error) {
	var order domain.Order
	err := r.db.Where("id = ?", id).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// FindByIDWithItems ID로 주문 조회 (아이템 포함)
func (r *orderRepository) FindByIDWithItems(id uint64) (*domain.Order, error) {
	var order domain.Order
	err := r.db.Preload("Items").Where("id = ?", id).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// FindByOrderNumber 주문번호로 조회
func (r *orderRepository) FindByOrderNumber(orderNumber string) (*domain.Order, error) {
	var order domain.Order
	err := r.db.Where("order_number = ?", orderNumber).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// FindByOrderNumberWithItems 주문번호로 조회 (아이템 포함)
func (r *orderRepository) FindByOrderNumberWithItems(orderNumber string) (*domain.Order, error) {
	var order domain.Order
	err := r.db.Preload("Items").Where("order_number = ?", orderNumber).First(&order).Error
	if err != nil {
		return nil, err
	}
	return &order, nil
}

// ListByUser 사용자의 주문 목록 조회
func (r *orderRepository) ListByUser(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	return r.listOrders(req, func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	})
}

// ListByUserWithItems 사용자의 주문 목록 조회 (아이템 포함)
func (r *orderRepository) ListByUserWithItems(userID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	return r.listOrdersWithItems(req, func(db *gorm.DB) *gorm.DB {
		return db.Where("user_id = ?", userID)
	})
}

// ListBySeller 판매자의 주문 목록 조회 (주문 아이템 기준)
func (r *orderRepository) ListBySeller(sellerID uint64, req *domain.OrderListRequest) ([]*domain.Order, int64, error) {
	var orders []*domain.Order
	var total int64

	// 기본값 설정
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// 서브쿼리: 해당 판매자의 아이템이 포함된 주문 ID
	subQuery := r.db.Table("commerce_order_items").
		Select("DISTINCT order_id").
		Where("seller_id = ?", sellerID)

	// 메인 쿼리
	query := r.db.Model(&domain.Order{}).Where("id IN (?)", subQuery)

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 총 개수
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// 페이지네이션
	offset := (page - 1) * limit
	if err := query.Preload("Items", "seller_id = ?", sellerID).
		Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// listOrders 공통 목록 조회 로직
func (r *orderRepository) listOrders(req *domain.OrderListRequest, baseQuery func(*gorm.DB) *gorm.DB) ([]*domain.Order, int64, error) {
	var orders []*domain.Order
	var total int64

	// 기본값 설정
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// 기본 쿼리
	query := r.db.Model(&domain.Order{})
	if baseQuery != nil {
		query = baseQuery(query)
	}

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 총 개수
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// 페이지네이션
	offset := (page - 1) * limit
	if err := query.Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// listOrdersWithItems 공통 목록 조회 로직 (아이템 포함)
func (r *orderRepository) listOrdersWithItems(req *domain.OrderListRequest, baseQuery func(*gorm.DB) *gorm.DB) ([]*domain.Order, int64, error) {
	var orders []*domain.Order
	var total int64

	// 기본값 설정
	page := req.Page
	if page < 1 {
		page = 1
	}
	limit := req.Limit
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// 기본 쿼리
	query := r.db.Model(&domain.Order{})
	if baseQuery != nil {
		query = baseQuery(query)
	}

	// 상태 필터
	if req.Status != "" {
		query = query.Where("status = ?", req.Status)
	}

	// 총 개수
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 정렬
	sortBy := req.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := req.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}
	query = query.Order(fmt.Sprintf("%s %s", sortBy, sortOrder))

	// 페이지네이션
	offset := (page - 1) * limit
	if err := query.Preload("Items").Offset(offset).Limit(limit).Find(&orders).Error; err != nil {
		return nil, 0, err
	}

	return orders, total, nil
}

// FindItemByID 주문 아이템 ID로 조회
func (r *orderRepository) FindItemByID(itemID uint64) (*domain.OrderItem, error) {
	var item domain.OrderItem
	err := r.db.Where("id = ?", itemID).First(&item).Error
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateItemStatus 주문 아이템 상태 업데이트
func (r *orderRepository) UpdateItemStatus(itemID uint64, status domain.OrderItemStatus) error {
	return r.db.Model(&domain.OrderItem{}).
		Where("id = ?", itemID).
		UpdateColumns(map[string]interface{}{
			"status":     status,
			"updated_at": time.Now(),
		}).Error
}

// GenerateOrderNumber 고유 주문번호 생성
// 형식: yyyyMMddHHmmss + 6자리 랜덤
func (r *orderRepository) GenerateOrderNumber() (string, error) {
	now := time.Now()
	prefix := now.Format("20060102150405")

	// 밀리초 기반 숫자 생성 (6자리)
	suffix := fmt.Sprintf("%06d", now.UnixNano()%1000000)

	orderNumber := prefix + suffix

	// 중복 확인 (매우 드물지만 안전을 위해)
	var count int64
	r.db.Model(&domain.Order{}).Where("order_number = ?", orderNumber).Count(&count)
	if count > 0 {
		// 재시도
		time.Sleep(time.Millisecond)
		return r.GenerateOrderNumber()
	}

	return orderNumber, nil
}

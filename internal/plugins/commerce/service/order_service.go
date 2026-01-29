package service

import (
	"errors"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 주문 에러 정의
var (
	ErrOrderNotFound          = errors.New("order not found")
	ErrOrderForbidden         = errors.New("you are not the owner of this order")
	ErrOrderCannotBeCancelled = errors.New("order cannot be cancelled")
	ErrEmptyCart              = errors.New("cart is empty")
	ErrShippingInfoRequired   = errors.New("shipping info is required for physical products")
	ErrOrderItemNotFound      = errors.New("order item not found")
	ErrInvalidOrderStatus     = errors.New("invalid order status")
)

// 플랫폼 수수료율 (기본값 5%)
const DefaultPlatformFeeRate = 5.0

// OrderService 주문 서비스 인터페이스
type OrderService interface {
	// 주문 생성
	CreateOrder(userID uint64, req *domain.CreateOrderRequest, ipAddress, userAgent string) (*domain.OrderResponse, error)

	// 주문 조회
	GetOrder(userID uint64, orderID uint64) (*domain.OrderResponse, error)
	GetOrderByNumber(userID uint64, orderNumber string) (*domain.OrderResponse, error)
	ListOrders(userID uint64, req *domain.OrderListRequest) ([]*domain.OrderResponse, *common.Meta, error)

	// 주문 상태 변경
	CancelOrder(userID uint64, orderID uint64, req *domain.CancelOrderRequest) error
	UpdateOrderStatus(orderID uint64, status domain.OrderStatus) error

	// 판매자용
	ListSellerOrders(sellerID uint64, req *domain.OrderListRequest) ([]*domain.OrderResponse, *common.Meta, error)
}

// orderService 구현체
type orderService struct {
	orderRepo   repository.OrderRepository
	cartRepo    repository.CartRepository
	productRepo repository.ProductRepository
}

// NewOrderService 생성자
func NewOrderService(
	orderRepo repository.OrderRepository,
	cartRepo repository.CartRepository,
	productRepo repository.ProductRepository,
) OrderService {
	return &orderService{
		orderRepo:   orderRepo,
		cartRepo:    cartRepo,
		productRepo: productRepo,
	}
}

// CreateOrder 주문 생성 (장바구니 → 주문)
func (s *orderService) CreateOrder(userID uint64, req *domain.CreateOrderRequest, ipAddress, userAgent string) (*domain.OrderResponse, error) {
	// 장바구니 조회
	carts, err := s.cartRepo.ListByUserWithProducts(userID)
	if err != nil {
		return nil, err
	}

	if len(carts) == 0 {
		return nil, ErrEmptyCart
	}

	// 유효한 아이템 필터링 및 계산
	var items []domain.OrderItem
	var subtotal float64
	var hasPhysicalProduct bool
	currency := "KRW"

	for _, cart := range carts {
		if cart.Product == nil {
			continue
		}

		// 상품 상태 확인
		if cart.Product.Status != domain.ProductStatusPublished {
			continue
		}

		// 재고 확인 (실물 상품)
		if cart.Product.ProductType == domain.ProductTypePhysical {
			hasPhysicalProduct = true
			if cart.Product.StockQuantity != nil && *cart.Product.StockQuantity < cart.Quantity {
				return nil, ErrInsufficientStock
			}
		}

		// 주문 아이템 생성
		itemSubtotal := cart.Product.Price * float64(cart.Quantity)
		platformFeeRate := DefaultPlatformFeeRate
		platformFee := itemSubtotal * (platformFeeRate / 100)
		sellerAmount := itemSubtotal - platformFee

		item := domain.OrderItem{
			ProductID:       cart.ProductID,
			SellerID:        cart.Product.SellerID,
			ProductName:     cart.Product.Name,
			ProductType:     cart.Product.ProductType,
			Price:           cart.Product.Price,
			Quantity:        cart.Quantity,
			Subtotal:        itemSubtotal,
			PlatformFeeRate: &platformFeeRate,
			PlatformFee:     &platformFee,
			SellerAmount:    &sellerAmount,
			Status:          domain.OrderItemStatusPending,
		}

		items = append(items, item)
		subtotal += itemSubtotal

		if currency == "KRW" && cart.Product.Currency != "" {
			currency = cart.Product.Currency
		}
	}

	if len(items) == 0 {
		return nil, ErrEmptyCart
	}

	// 실물 상품 포함 시 배송 정보 필수
	if hasPhysicalProduct {
		if req.ShippingName == "" || req.ShippingPhone == "" || req.ShippingAddress == "" || req.ShippingPostal == "" {
			return nil, ErrShippingInfoRequired
		}
	}

	// 주문번호 생성
	orderNumber, err := s.orderRepo.GenerateOrderNumber()
	if err != nil {
		return nil, err
	}

	// 주문 생성
	order := &domain.Order{
		OrderNumber:     orderNumber,
		UserID:          userID,
		Subtotal:        subtotal,
		Discount:        0, // 추후 쿠폰 시스템에서 적용
		ShippingFee:     0, // 추후 배송비 계산 로직 추가
		Total:           subtotal,
		Currency:        currency,
		Status:          domain.OrderStatusPending,
		ShippingName:    req.ShippingName,
		ShippingPhone:   req.ShippingPhone,
		ShippingAddress: req.ShippingAddress,
		ShippingPostal:  req.ShippingPostal,
		ShippingMemo:    req.ShippingMemo,
		IPAddress:       ipAddress,
		UserAgent:       userAgent,
	}

	// 주문과 아이템 저장
	if err := s.orderRepo.CreateWithItems(order, items); err != nil {
		return nil, err
	}

	// 재고 차감 (실물 상품)
	for _, item := range items {
		if item.ProductType == domain.ProductTypePhysical {
			product, err := s.productRepo.FindByID(item.ProductID)
			if err == nil && product.StockQuantity != nil {
				newQuantity := *product.StockQuantity - item.Quantity
				product.StockQuantity = &newQuantity

				// 재고가 0이 되면 품절 상태로 변경
				if newQuantity <= 0 {
					product.StockStatus = domain.StockStatusOutOfStock
				}

				s.productRepo.Update(product.ID, product)
			}
		}
	}

	// 장바구니 비우기
	if err := s.cartRepo.DeleteByUserID(userID); err != nil {
		// 장바구니 삭제 실패는 로그만 남기고 진행
		// TODO: 로깅 추가
	}

	// 주문 다시 조회 (아이템 포함)
	createdOrder, err := s.orderRepo.FindByIDWithItems(order.ID)
	if err != nil {
		return nil, err
	}

	return createdOrder.ToResponse(), nil
}

// GetOrder 주문 상세 조회
func (s *orderService) GetOrder(userID uint64, orderID uint64) (*domain.OrderResponse, error) {
	order, err := s.orderRepo.FindByIDWithItems(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if order.UserID != userID {
		return nil, ErrOrderForbidden
	}

	return order.ToResponse(), nil
}

// GetOrderByNumber 주문번호로 조회
func (s *orderService) GetOrderByNumber(userID uint64, orderNumber string) (*domain.OrderResponse, error) {
	order, err := s.orderRepo.FindByOrderNumberWithItems(orderNumber)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if order.UserID != userID {
		return nil, ErrOrderForbidden
	}

	return order.ToResponse(), nil
}

// ListOrders 주문 목록 조회
func (s *orderService) ListOrders(userID uint64, req *domain.OrderListRequest) ([]*domain.OrderResponse, *common.Meta, error) {
	// 기본값 설정
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 20
	}

	orders, total, err := s.orderRepo.ListByUserWithItems(userID, req)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.OrderResponse, len(orders))
	for i, order := range orders {
		responses[i] = order.ToResponse()
	}

	meta := &common.Meta{
		Page:  req.Page,
		Limit: req.Limit,
		Total: total,
	}

	return responses, meta, nil
}

// CancelOrder 주문 취소
func (s *orderService) CancelOrder(userID uint64, orderID uint64, req *domain.CancelOrderRequest) error {
	order, err := s.orderRepo.FindByIDWithItems(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}

	// 소유자 확인
	if order.UserID != userID {
		return ErrOrderForbidden
	}

	// 취소 가능 상태 확인 (pending만 취소 가능)
	if order.Status != domain.OrderStatusPending {
		return ErrOrderCannotBeCancelled
	}

	// 재고 복구 (실물 상품)
	for _, item := range order.Items {
		if item.ProductType == domain.ProductTypePhysical {
			product, err := s.productRepo.FindByID(item.ProductID)
			if err == nil && product.StockQuantity != nil {
				newQuantity := *product.StockQuantity + item.Quantity
				product.StockQuantity = &newQuantity
				product.StockStatus = domain.StockStatusInStock
				s.productRepo.Update(product.ID, product)
			}
		}
	}

	// 주문 상태 변경
	return s.orderRepo.UpdateStatus(orderID, domain.OrderStatusCancelled)
}

// UpdateOrderStatus 주문 상태 변경 (내부용/관리자용)
func (s *orderService) UpdateOrderStatus(orderID uint64, status domain.OrderStatus) error {
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrOrderNotFound
		}
		return err
	}

	// 상태 전이 유효성 검사
	if !s.isValidStatusTransition(order.Status, status) {
		return ErrInvalidOrderStatus
	}

	return s.orderRepo.UpdateStatus(orderID, status)
}

// ListSellerOrders 판매자의 주문 목록 조회
func (s *orderService) ListSellerOrders(sellerID uint64, req *domain.OrderListRequest) ([]*domain.OrderResponse, *common.Meta, error) {
	// 기본값 설정
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Limit < 1 || req.Limit > 100 {
		req.Limit = 20
	}

	orders, total, err := s.orderRepo.ListBySeller(sellerID, req)
	if err != nil {
		return nil, nil, err
	}

	responses := make([]*domain.OrderResponse, len(orders))
	for i, order := range orders {
		responses[i] = order.ToResponse()
	}

	meta := &common.Meta{
		Page:  req.Page,
		Limit: req.Limit,
		Total: total,
	}

	return responses, meta, nil
}

// isValidStatusTransition 상태 전이 유효성 검사
func (s *orderService) isValidStatusTransition(from, to domain.OrderStatus) bool {
	validTransitions := map[domain.OrderStatus][]domain.OrderStatus{
		domain.OrderStatusPending: {
			domain.OrderStatusPaid,
			domain.OrderStatusCancelled,
		},
		domain.OrderStatusPaid: {
			domain.OrderStatusProcessing,
			domain.OrderStatusCancelled,
			domain.OrderStatusRefunded,
		},
		domain.OrderStatusProcessing: {
			domain.OrderStatusShipped,
			domain.OrderStatusCompleted, // 디지털 상품의 경우
			domain.OrderStatusCancelled,
			domain.OrderStatusRefunded,
		},
		domain.OrderStatusShipped: {
			domain.OrderStatusDelivered,
			domain.OrderStatusRefunded,
		},
		domain.OrderStatusDelivered: {
			domain.OrderStatusCompleted,
			domain.OrderStatusRefunded,
		},
		domain.OrderStatusCompleted: {
			domain.OrderStatusRefunded,
		},
	}

	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}

	for _, status := range allowed {
		if status == to {
			return true
		}
	}

	return false
}

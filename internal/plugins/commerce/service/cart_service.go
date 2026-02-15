package service

import (
	"errors"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 장바구니 에러 정의
var (
	ErrCartItemNotFound     = errors.New("cart item not found")
	ErrCartItemForbidden    = errors.New("you are not the owner of this cart item")
	ErrProductNotAvailable  = errors.New("product is not available")
	ErrInsufficientStock    = errors.New("insufficient stock")
	ErrInvalidQuantity      = errors.New("invalid quantity")
	ErrProductAlreadyInCart = errors.New("product already in cart")
)

// CartService 장바구니 서비스 인터페이스
type CartService interface {
	// 장바구니 조회
	GetCart(userID uint64) (*domain.CartResponse, error)

	// 장바구니 아이템 조작
	AddToCart(userID uint64, req *domain.AddToCartRequest) (*domain.CartItemResponse, error)
	UpdateCartItem(userID uint64, cartID uint64, req *domain.UpdateCartRequest) (*domain.CartItemResponse, error)
	RemoveFromCart(userID uint64, cartID uint64) error
	ClearCart(userID uint64) error

	// 유효성 검사
	ValidateCart(userID uint64) (*domain.CartResponse, []error)
}

// cartService 구현체
type cartService struct {
	cartRepo    repository.CartRepository
	productRepo repository.ProductRepository
}

// NewCartService 생성자
func NewCartService(cartRepo repository.CartRepository, productRepo repository.ProductRepository) CartService {
	return &cartService{
		cartRepo:    cartRepo,
		productRepo: productRepo,
	}
}

// GetCart 장바구니 조회
func (s *cartService) GetCart(userID uint64) (*domain.CartResponse, error) {
	carts, err := s.cartRepo.ListByUserWithProducts(userID)
	if err != nil {
		return nil, err
	}

	response := &domain.CartResponse{
		Items:      make([]*domain.CartItemResponse, 0, len(carts)),
		ItemCount:  len(carts),
		TotalCount: 0,
		Subtotal:   0,
		Currency:   "KRW",
	}

	for _, cart := range carts {
		// 삭제되거나 비공개된 상품 건너뛰기
		if cart.Product == nil {
			continue
		}

		item := cart.ToCartItemResponse()
		response.Items = append(response.Items, item)
		response.Subtotal += item.Subtotal
		response.TotalCount += cart.Quantity

		// 통화 설정 (첫 번째 상품 기준)
		if response.Currency == "KRW" && cart.Product.Currency != "" {
			response.Currency = cart.Product.Currency
		}
	}

	return response, nil
}

// AddToCart 장바구니에 상품 추가
func (s *cartService) AddToCart(userID uint64, req *domain.AddToCartRequest) (*domain.CartItemResponse, error) {
	// 수량 검증
	if req.Quantity < 1 {
		return nil, ErrInvalidQuantity
	}

	// 상품 조회 및 유효성 검사
	product, err := s.productRepo.FindByID(req.ProductID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrProductNotFound
		}
		return nil, err
	}

	// 상품 상태 확인
	if product.Status != domain.ProductStatusPublished {
		return nil, ErrProductNotAvailable
	}

	// 재고 확인 (실물 상품인 경우)
	if product.ProductType == domain.ProductTypePhysical && product.StockQuantity != nil {
		if *product.StockQuantity < req.Quantity {
			return nil, ErrInsufficientStock
		}
	}

	// 기존 장바구니 아이템 확인
	existingCart, err := s.cartRepo.FindByUserAndProduct(userID, req.ProductID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	var cart *domain.Cart

	if existingCart != nil {
		// 기존 아이템이 있으면 수량 증가
		newQuantity := existingCart.Quantity + req.Quantity

		// 재고 재확인 (실물 상품인 경우)
		if product.ProductType == domain.ProductTypePhysical && product.StockQuantity != nil {
			if *product.StockQuantity < newQuantity {
				return nil, ErrInsufficientStock
			}
		}

		if err := s.cartRepo.SetQuantity(existingCart.ID, newQuantity); err != nil {
			return nil, err
		}

		existingCart.Quantity = newQuantity
		existingCart.Product = product
		cart = existingCart
	} else {
		// 새 아이템 추가
		cart = &domain.Cart{
			UserID:    userID,
			ProductID: req.ProductID,
			Quantity:  req.Quantity,
		}

		if err := s.cartRepo.Create(cart); err != nil {
			return nil, err
		}

		cart.Product = product
	}

	return cart.ToCartItemResponse(), nil
}

// UpdateCartItem 장바구니 아이템 수량 변경
func (s *cartService) UpdateCartItem(userID uint64, cartID uint64, req *domain.UpdateCartRequest) (*domain.CartItemResponse, error) {
	// 수량 검증
	if req.Quantity < 1 {
		return nil, ErrInvalidQuantity
	}

	// 장바구니 아이템 조회
	cart, err := s.cartRepo.FindByID(cartID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCartItemNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if cart.UserID != userID {
		return nil, ErrCartItemForbidden
	}

	// 상품 조회
	product, err := s.productRepo.FindByID(cart.ProductID)
	if err != nil {
		return nil, ErrProductNotFound
	}

	// 재고 확인 (실물 상품인 경우)
	if product.ProductType == domain.ProductTypePhysical && product.StockQuantity != nil {
		if *product.StockQuantity < req.Quantity {
			return nil, ErrInsufficientStock
		}
	}

	// 수량 업데이트
	if err := s.cartRepo.SetQuantity(cartID, req.Quantity); err != nil {
		return nil, err
	}

	cart.Quantity = req.Quantity
	cart.Product = product

	return cart.ToCartItemResponse(), nil
}

// RemoveFromCart 장바구니에서 아이템 삭제
func (s *cartService) RemoveFromCart(userID uint64, cartID uint64) error {
	// 장바구니 아이템 조회
	cart, err := s.cartRepo.FindByID(cartID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCartItemNotFound
		}
		return err
	}

	// 소유자 확인
	if cart.UserID != userID {
		return ErrCartItemForbidden
	}

	return s.cartRepo.Delete(cartID)
}

// ClearCart 장바구니 비우기
func (s *cartService) ClearCart(userID uint64) error {
	return s.cartRepo.DeleteByUserID(userID)
}

// ValidateCart 장바구니 유효성 검사 (주문 전 검증)
func (s *cartService) ValidateCart(userID uint64) (*domain.CartResponse, []error) {
	carts, err := s.cartRepo.ListByUserWithProducts(userID)
	if err != nil {
		return nil, []error{err}
	}

	if len(carts) == 0 {
		return nil, []error{errors.New("cart is empty")}
	}

	var validationErrors []error
	validItems := make([]*domain.Cart, 0, len(carts))

	for _, cart := range carts {
		// 상품 존재 여부
		if cart.Product == nil {
			validationErrors = append(validationErrors, errors.New("product not found: "+string(rune(cart.ProductID))))
			continue
		}

		// 상품 상태 확인
		if cart.Product.Status != domain.ProductStatusPublished {
			validationErrors = append(validationErrors, errors.New("product not available: "+cart.Product.Name))
			continue
		}

		// 재고 확인 (실물 상품)
		if cart.Product.ProductType == domain.ProductTypePhysical && cart.Product.StockQuantity != nil {
			if *cart.Product.StockQuantity < cart.Quantity {
				validationErrors = append(validationErrors, errors.New("insufficient stock: "+cart.Product.Name))
				continue
			}
		}

		validItems = append(validItems, cart)
	}

	// 유효한 아이템이 없으면 에러
	if len(validItems) == 0 {
		return nil, append(validationErrors, errors.New("no valid items in cart"))
	}

	// 유효한 아이템으로 응답 생성
	response := &domain.CartResponse{
		Items:      make([]*domain.CartItemResponse, 0, len(validItems)),
		ItemCount:  len(validItems),
		TotalCount: 0,
		Subtotal:   0,
		Currency:   "KRW",
	}

	for _, cart := range validItems {
		item := cart.ToCartItemResponse()
		response.Items = append(response.Items, item)
		response.Subtotal += item.Subtotal
		response.TotalCount += cart.Quantity

		if response.Currency == "KRW" && cart.Product.Currency != "" {
			response.Currency = cart.Product.Currency
		}
	}

	return response, validationErrors
}

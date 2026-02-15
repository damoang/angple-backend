package service

import (
	"context"
	"errors"
	"time"

	"github.com/damoang/angple-backend/internal/plugins/commerce/domain"
	"github.com/damoang/angple-backend/internal/plugins/commerce/gateway"
	"github.com/damoang/angple-backend/internal/plugins/commerce/repository"
	"gorm.io/gorm"
)

// 결제 에러 정의
var (
	ErrPaymentNotFound      = errors.New("payment not found")
	ErrPaymentForbidden     = errors.New("you are not the owner of this payment")
	ErrPaymentAlreadyExists = errors.New("payment already exists for this order")
	ErrPaymentAlreadyPaid   = errors.New("payment already completed")
	ErrPaymentNotPending    = errors.New("payment is not in pending status")
	ErrAmountMismatch       = errors.New("payment amount mismatch")
	ErrPGNotSupported       = errors.New("payment gateway not supported")
	ErrPaymentFailed        = errors.New("payment failed")
)

// PaymentService 결제 서비스 인터페이스
type PaymentService interface {
	// 결제 준비
	PreparePayment(ctx context.Context, userID uint64, req *domain.PreparePaymentRequest) (*domain.PreparePaymentResponse, error)

	// 결제 완료
	CompletePayment(ctx context.Context, userID uint64, req *domain.CompletePaymentRequest) (*domain.PaymentResponse, error)

	// 결제 취소
	CancelPayment(ctx context.Context, userID uint64, req *domain.CancelPaymentRequest) error

	// 결제 조회
	GetPayment(ctx context.Context, userID uint64, paymentID uint64) (*domain.PaymentResponse, error)
	GetPaymentByOrder(ctx context.Context, userID uint64, orderID uint64) (*domain.PaymentResponse, error)

	// 웹훅 처리
	HandleWebhook(ctx context.Context, provider domain.PGProvider, payload []byte) error
}

// paymentService 구현체
type paymentService struct {
	paymentRepo repository.PaymentRepository
	orderRepo   repository.OrderRepository
	productRepo repository.ProductRepository
	gatewayMgr  *gateway.GatewayManager
}

// NewPaymentService 생성자
func NewPaymentService(
	paymentRepo repository.PaymentRepository,
	orderRepo repository.OrderRepository,
	productRepo repository.ProductRepository,
	gatewayMgr *gateway.GatewayManager,
) PaymentService {
	return &paymentService{
		paymentRepo: paymentRepo,
		orderRepo:   orderRepo,
		productRepo: productRepo,
		gatewayMgr:  gatewayMgr,
	}
}

// PreparePayment 결제 준비
func (s *paymentService) PreparePayment(ctx context.Context, userID uint64, req *domain.PreparePaymentRequest) (*domain.PreparePaymentResponse, error) {
	// 주문 조회
	order, err := s.orderRepo.FindByIDWithItems(req.OrderID)
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

	// 주문 상태 확인
	if order.Status != domain.OrderStatusPending {
		return nil, ErrPaymentAlreadyPaid
	}

	// 기존 결제 확인
	existingPayment, err := s.paymentRepo.FindByOrderID(req.OrderID)
	if err == nil && existingPayment.Status == domain.PaymentStatusPaid {
		return nil, ErrPaymentAlreadyPaid
	}

	// PG 게이트웨이 조회
	pgProvider := domain.PGProvider(req.PGProvider)
	gw, err := s.gatewayMgr.Get(pgProvider)
	if err != nil {
		return nil, ErrPGNotSupported
	}

	// 결제 준비 요청
	prepareReq := &gateway.PrepareRequest{
		OrderID:       order.ID,
		OrderNumber:   order.OrderNumber,
		Amount:        order.Total,
		Currency:      order.Currency,
		ProductName:   s.getProductNames(order),
		BuyerName:     order.ShippingName,
		BuyerPhone:    order.ShippingPhone,
		PaymentMethod: domain.PaymentMethod(req.PaymentMethod),
		ReturnURL:     req.ReturnURL,
		CancelURL:     req.CancelURL,
	}

	prepareResp, err := gw.Prepare(ctx, prepareReq)
	if err != nil {
		return nil, err
	}

	// 결제 레코드 생성
	payment := &domain.Payment{
		OrderID:       order.ID,
		PGProvider:    pgProvider,
		PGOrderID:     prepareResp.PGOrderID,
		PaymentMethod: domain.PaymentMethod(req.PaymentMethod),
		Amount:        order.Total,
		Currency:      order.Currency,
		Status:        domain.PaymentStatusPending,
	}

	if err := s.paymentRepo.Create(payment); err != nil {
		return nil, err
	}

	return &domain.PreparePaymentResponse{
		PaymentID:   payment.ID,
		OrderNumber: order.OrderNumber,
		Amount:      order.Total,
		Currency:    order.Currency,
		PGProvider:  req.PGProvider,
		PGOrderID:   prepareResp.PGOrderID,
		RedirectURL: prepareResp.RedirectURL,
		MobileURL:   prepareResp.MobileURL,
		AppScheme:   prepareResp.AppScheme,
		MerchantID:  prepareResp.MerchantID,
		Signature:   prepareResp.Signature,
		Timestamp:   prepareResp.Timestamp,
		ExtraData:   prepareResp.ExtraData,
	}, nil
}

// CompletePayment 결제 완료
func (s *paymentService) CompletePayment(ctx context.Context, userID uint64, req *domain.CompletePaymentRequest) (*domain.PaymentResponse, error) {
	// 결제 조회
	payment, err := s.paymentRepo.FindByIDWithOrder(req.PaymentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if payment.Order != nil && payment.Order.UserID != userID {
		return nil, ErrPaymentForbidden
	}

	// 상태 확인
	if payment.Status != domain.PaymentStatusPending {
		if payment.Status == domain.PaymentStatusPaid {
			return nil, ErrPaymentAlreadyPaid
		}
		return nil, ErrPaymentNotPending
	}

	// 금액 확인
	if payment.Amount != req.Amount {
		return nil, ErrAmountMismatch
	}

	// PG 게이트웨이 조회
	pgProvider := domain.PGProvider(req.PGProvider)
	gw, err := s.gatewayMgr.Get(pgProvider)
	if err != nil {
		return nil, ErrPGNotSupported
	}

	// 결제 완료 처리
	completeReq := &gateway.CompleteRequest{
		PGTID:     req.PGTID,
		PGOrderID: req.PGOrderID,
		Amount:    req.Amount,
	}

	completeResp, err := gw.Complete(ctx, completeReq)
	if err != nil {
		// 결제 실패 처리
		s.paymentRepo.UpdateStatus(payment.ID, domain.PaymentStatusFailed)
		return nil, err
	}

	if !completeResp.Success {
		s.paymentRepo.UpdateStatus(payment.ID, domain.PaymentStatusFailed)
		return nil, ErrPaymentFailed
	}

	// 결제 정보 업데이트
	now := time.Now()
	payment.PGTID = completeResp.PGTID
	payment.Status = domain.PaymentStatusPaid
	payment.PaidAt = &now
	payment.CardCompany = completeResp.CardCompany
	payment.CardNumber = completeResp.CardNumber
	payment.CardType = completeResp.CardType
	payment.InstallMonth = completeResp.InstallMonth
	payment.VBankName = completeResp.VBankName
	payment.VBankNumber = completeResp.VBankNumber
	payment.VBankHolder = completeResp.VBankHolder
	payment.RawResponse = completeResp.RawResponse

	if completeResp.PGFee > 0 {
		payment.PGFee = &completeResp.PGFee
	}

	// 가상계좌 입금 대기 상태
	if completeResp.Status == domain.PaymentStatusReady {
		payment.Status = domain.PaymentStatusReady
		payment.PaidAt = nil
		// 가상계좌 입금 기한 파싱
		if completeResp.VBankDue != "" {
			if dueTime, err := time.Parse("2006-01-02T15:04:05", completeResp.VBankDue); err == nil {
				payment.VBankDue = &dueTime
			}
		}
	}

	if err := s.paymentRepo.Update(payment.ID, payment); err != nil {
		return nil, err
	}

	// 주문 상태 업데이트
	if payment.Status == domain.PaymentStatusPaid {
		s.orderRepo.UpdateStatus(payment.OrderID, domain.OrderStatusPaid)

		// 판매 수량 증가
		if payment.Order != nil {
			for _, item := range payment.Order.Items {
				s.productRepo.IncrementSalesCount(item.ProductID, item.Quantity)
			}
		}
	}

	return payment.ToResponse(), nil
}

// CancelPayment 결제 취소
func (s *paymentService) CancelPayment(ctx context.Context, userID uint64, req *domain.CancelPaymentRequest) error {
	// 결제 조회
	payment, err := s.paymentRepo.FindByIDWithOrder(req.PaymentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPaymentNotFound
		}
		return err
	}

	// 소유자 확인
	if payment.Order != nil && payment.Order.UserID != userID {
		return ErrPaymentForbidden
	}

	// 취소 가능 상태 확인
	if payment.Status != domain.PaymentStatusPaid && payment.Status != domain.PaymentStatusPartialCancelled {
		return ErrCancelNotAllowed
	}

	// 취소 금액 확인
	cancelAmount := req.CancelAmount
	if cancelAmount <= 0 {
		cancelAmount = payment.Amount - payment.CancelledAmount
	}

	remainingAmount := payment.Amount - payment.CancelledAmount
	if cancelAmount > remainingAmount {
		return ErrInvalidAmount
	}

	// PG 게이트웨이 조회
	gw, err := s.gatewayMgr.Get(payment.PGProvider)
	if err != nil {
		return ErrPGNotSupported
	}

	// PG 취소 요청
	cancelReq := &gateway.CancelRequest{
		PGTID:        payment.PGTID,
		PGOrderID:    payment.PGOrderID,
		CancelAmount: cancelAmount,
		TotalAmount:  payment.Amount,
		CancelReason: req.CancelReason,
	}

	cancelResp, err := gw.Cancel(ctx, cancelReq)
	if err != nil {
		return err
	}

	if !cancelResp.Success {
		return ErrCancelFailed
	}

	// 결제 취소 정보 업데이트
	if err := s.paymentRepo.UpdateCancelled(payment.ID, cancelResp.CancelledAmount, req.CancelReason); err != nil {
		return err
	}

	// 전체 취소인 경우 주문 상태 변경
	if cancelResp.RemainingAmount == 0 {
		s.orderRepo.UpdateStatus(payment.OrderID, domain.OrderStatusCancelled)

		// 재고 복구 (실물 상품)
		if payment.Order != nil {
			for _, item := range payment.Order.Items {
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
		}
	}

	return nil
}

// GetPayment 결제 조회
func (s *paymentService) GetPayment(ctx context.Context, userID uint64, paymentID uint64) (*domain.PaymentResponse, error) {
	payment, err := s.paymentRepo.FindByIDWithOrder(paymentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	// 소유자 확인
	if payment.Order != nil && payment.Order.UserID != userID {
		return nil, ErrPaymentForbidden
	}

	return payment.ToResponse(), nil
}

// GetPaymentByOrder 주문의 결제 조회
func (s *paymentService) GetPaymentByOrder(ctx context.Context, userID uint64, orderID uint64) (*domain.PaymentResponse, error) {
	// 주문 소유자 확인
	order, err := s.orderRepo.FindByID(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}

	if order.UserID != userID {
		return nil, ErrOrderForbidden
	}

	payment, err := s.paymentRepo.FindByOrderID(orderID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPaymentNotFound
		}
		return nil, err
	}

	return payment.ToResponse(), nil
}

// HandleWebhook 웹훅 처리
func (s *paymentService) HandleWebhook(ctx context.Context, provider domain.PGProvider, payload []byte) error {
	// PG 게이트웨이 조회
	gw, err := s.gatewayMgr.Get(provider)
	if err != nil {
		return ErrPGNotSupported
	}

	// 웹훅 처리
	result, err := gw.HandleWebhook(ctx, payload)
	if err != nil {
		return err
	}

	// 결제 조회
	payment, err := s.paymentRepo.FindByPGTID(provider, result.PGTID)
	if err != nil {
		// PG TID로 찾지 못한 경우 PG Order ID로 재시도
		payment, err = s.paymentRepo.FindByPGOrderID(provider, result.PGOrderID)
		if err != nil {
			return ErrPaymentNotFound
		}
	}

	// 웹훅 이벤트 처리
	switch result.Status {
	case domain.PaymentStatusPaid:
		// 가상계좌 입금 완료 등
		now := time.Now()
		if err := s.paymentRepo.UpdatePaid(payment.ID, now, 0); err != nil {
			return err
		}
		s.orderRepo.UpdateStatus(payment.OrderID, domain.OrderStatusPaid)

	case domain.PaymentStatusReady:
		// 가상계좌 발급
		payment.VBankName = result.VBankName
		payment.VBankNumber = result.VBankNumber
		payment.VBankHolder = result.VBankHolder
		payment.Status = domain.PaymentStatusReady
		if err := s.paymentRepo.Update(payment.ID, payment); err != nil {
			return err
		}

	case domain.PaymentStatusCancelled:
		// 취소됨
		if err := s.paymentRepo.UpdateCancelled(payment.ID, payment.Amount, "웹훅 취소"); err != nil {
			return err
		}
		s.orderRepo.UpdateStatus(payment.OrderID, domain.OrderStatusCancelled)
	}

	return nil
}

// getProductNames 주문 상품명 생성
func (s *paymentService) getProductNames(order *domain.Order) string {
	if len(order.Items) == 0 {
		return "상품"
	}

	if len(order.Items) == 1 {
		return order.Items[0].ProductName
	}

	return order.Items[0].ProductName + " 외 " + string(rune('0'+len(order.Items)-1)) + "건"
}

// 에러 추가
var ErrCancelFailed = errors.New("payment cancel failed")
var ErrCancelNotAllowed = errors.New("payment cancel not allowed")
var ErrInvalidAmount = errors.New("invalid amount")

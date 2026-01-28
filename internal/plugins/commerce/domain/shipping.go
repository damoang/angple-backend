package domain

import (
	"time"
)

// ShippingCarrierCode 배송사 코드
type ShippingCarrierCode string

const (
	CarrierCJ       ShippingCarrierCode = "cj"       // CJ대한통운
	CarrierLotte    ShippingCarrierCode = "lotte"    // 롯데택배
	CarrierHanjin   ShippingCarrierCode = "hanjin"   // 한진택배
	CarrierPost     ShippingCarrierCode = "post"     // 우체국택배
	CarrierLogen    ShippingCarrierCode = "logen"    // 로젠택배
	CarrierEPost    ShippingCarrierCode = "epost"    // 우체국 EMS
	CarrierKD       ShippingCarrierCode = "kd"       // 경동택배
	CarrierGSPostBox ShippingCarrierCode = "gspostbox" // GSPostbox
)

// ShippingStatus 배송 상태
type ShippingStatus string

const (
	ShippingStatusPending     ShippingStatus = "pending"      // 배송 준비 중
	ShippingStatusPickedUp    ShippingStatus = "picked_up"    // 집화 완료
	ShippingStatusInTransit   ShippingStatus = "in_transit"   // 배송 중
	ShippingStatusOutForDelivery ShippingStatus = "out_for_delivery" // 배송 출발
	ShippingStatusDelivered   ShippingStatus = "delivered"    // 배송 완료
	ShippingStatusException   ShippingStatus = "exception"    // 배송 이상
	ShippingStatusReturned    ShippingStatus = "returned"     // 반송
)

// ShippingCarrierInfo 배송사 정보
type ShippingCarrierInfo struct {
	Code       ShippingCarrierCode `json:"code"`
	Name       string              `json:"name"`
	TrackingURL string             `json:"tracking_url"`
}

// 배송사 목록
var ShippingCarriers = map[ShippingCarrierCode]*ShippingCarrierInfo{
	CarrierCJ: {
		Code:       CarrierCJ,
		Name:       "CJ대한통운",
		TrackingURL: "https://www.cjlogistics.com/ko/tool/parcel/tracking?gnbInvcNo=%s",
	},
	CarrierLotte: {
		Code:       CarrierLotte,
		Name:       "롯데택배",
		TrackingURL: "https://www.lotteglogis.com/home/reservation/tracking/linkView?InvNo=%s",
	},
	CarrierHanjin: {
		Code:       CarrierHanjin,
		Name:       "한진택배",
		TrackingURL: "https://www.hanjin.com/kor/CMS/DeliveryMgr/WaybillResult.do?mession=13&wblnum=%s",
	},
	CarrierPost: {
		Code:       CarrierPost,
		Name:       "우체국택배",
		TrackingURL: "https://service.epost.go.kr/trace.RetrieveDomRi498.parcel?sid1=%s",
	},
	CarrierLogen: {
		Code:       CarrierLogen,
		Name:       "로젠택배",
		TrackingURL: "https://www.ilogen.com/web/personal/trace/%s",
	},
	CarrierKD: {
		Code:       CarrierKD,
		Name:       "경동택배",
		TrackingURL: "https://kdexp.com/service/delivery/etc/delivery_result.do?barcode=%s",
	},
}

// TrackingEvent 배송 추적 이벤트
type TrackingEvent struct {
	Time        time.Time `json:"time"`
	Location    string    `json:"location"`
	Status      string    `json:"status"`
	Description string    `json:"description"`
}

// TrackingInfo 배송 추적 정보
type TrackingInfo struct {
	Carrier        *ShippingCarrierInfo `json:"carrier"`
	TrackingNumber string               `json:"tracking_number"`
	Status         ShippingStatus       `json:"status"`
	StatusText     string               `json:"status_text"`
	EstimatedDate  *time.Time           `json:"estimated_date,omitempty"`
	DeliveredAt    *time.Time           `json:"delivered_at,omitempty"`
	RecipientName  string               `json:"recipient_name,omitempty"`
	Events         []*TrackingEvent     `json:"events"`
}

// RegisterShippingRequest 송장번호 등록 요청
type RegisterShippingRequest struct {
	Carrier        string `json:"carrier" binding:"required,oneof=cj lotte hanjin post logen kd gspostbox"`
	TrackingNumber string `json:"tracking_number" binding:"required,min=5,max=30"`
}

// TrackingResponse 배송 추적 응답
type TrackingResponse struct {
	OrderID        uint64        `json:"order_id"`
	OrderNumber    string        `json:"order_number"`
	TrackingInfo   *TrackingInfo `json:"tracking_info"`
	TrackingURL    string        `json:"tracking_url"`
}

// ShippingCarrierListResponse 배송사 목록 응답
type ShippingCarrierListResponse struct {
	Carriers []*ShippingCarrierInfo `json:"carriers"`
}

// GetShippingCarrier 배송사 코드로 배송사 정보 조회
func GetShippingCarrier(code string) *ShippingCarrierInfo {
	carrier, ok := ShippingCarriers[ShippingCarrierCode(code)]
	if !ok {
		return nil
	}
	return carrier
}

// GetAllShippingCarriers 모든 배송사 목록 조회
func GetAllShippingCarriers() []*ShippingCarrierInfo {
	carriers := make([]*ShippingCarrierInfo, 0, len(ShippingCarriers))
	for _, carrier := range ShippingCarriers {
		carriers = append(carriers, carrier)
	}
	return carriers
}

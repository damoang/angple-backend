# Commerce Plugin 구현 진행 상황

## 개요
Angple 플랫폼 커머스(쇼핑몰) 플러그인 구현 진행 상황

---

## Phase 1: 기반 인프라 ✅ 완료

### 구현 완료
- [x] 플러그인 시스템 구현 (`internal/plugin/`)
- [x] Commerce 플러그인 기본 구조 (`internal/plugins/commerce/plugin.go`)
- [x] DB 마이그레이션 (`migration/plugins/commerce/001_init.up.sql`)
- [x] 9개 테이블 스키마 작성

---

## Phase 2: 상품 관리 ✅ 완료

### 구현 파일
- [x] `domain/product.go` - 상품 엔티티 + DTO
- [x] `domain/product_file.go` - 디지털 파일 엔티티
- [x] `repository/product_repo.go` - 상품 CRUD
- [x] `service/product_service.go` - 상품 비즈니스 로직
- [x] `handler/product_handler.go` - 상품 HTTP 핸들러

### API 엔드포인트
| Method | Path | Auth | 상태 |
|--------|------|------|------|
| GET | /products | required | ✅ |
| POST | /products | required | ✅ |
| GET | /products/:id | required | ✅ |
| PUT | /products/:id | required | ✅ |
| DELETE | /products/:id | required | ✅ |
| GET | /shop/products | none | ✅ |
| GET | /shop/products/:id | none | ✅ |
| GET | /shop/products/slug/:slug | none | ✅ |

---

## Phase 3: 장바구니 & 주문 ✅ 완료

### 구현 파일
- [x] `domain/cart.go` - 장바구니 엔티티 + DTO
- [x] `domain/order.go` - 주문/주문아이템 엔티티 + DTO
- [x] `repository/cart_repo.go` - 장바구니 CRUD
- [x] `repository/order_repo.go` - 주문 CRUD (트랜잭션 지원)
- [x] `service/cart_service.go` - 장바구니 비즈니스 로직
- [x] `service/order_service.go` - 주문 비즈니스 로직 (재고 관리)
- [x] `handler/cart_handler.go` - 장바구니 HTTP 핸들러
- [x] `handler/order_handler.go` - 주문 HTTP 핸들러

### API 엔드포인트
| Method | Path | Auth | 상태 |
|--------|------|------|------|
| GET | /cart | required | ✅ |
| POST | /cart | required | ✅ |
| PUT | /cart/:id | required | ✅ |
| DELETE | /cart/:id | required | ✅ |
| DELETE | /cart | required | ✅ |
| GET | /orders | required | ✅ |
| POST | /orders | required | ✅ |
| GET | /orders/:id | required | ✅ |
| POST | /orders/:id/cancel | required | ✅ |

---

## Phase 4: 결제 통합 ✅ 완료

### 구현 파일
- [x] `domain/payment.go` - 결제 엔티티 + DTO
- [x] `gateway/gateway.go` - PaymentGateway 인터페이스, GatewayManager
- [x] `gateway/inicis.go` - KG이니시스 게이트웨이
- [x] `gateway/tosspayments.go` - 토스페이먼츠 게이트웨이
- [x] `repository/payment_repo.go` - 결제 CRUD
- [x] `service/payment_service.go` - 결제 비즈니스 로직
- [x] `handler/payment_handler.go` - 결제 HTTP 핸들러

### 지원 PG사
- [x] KG이니시스 (inicis)
- [x] 토스페이먼츠 (tosspayments)

### API 엔드포인트
| Method | Path | Auth | 상태 |
|--------|------|------|------|
| POST | /payments/prepare | required | ✅ |
| POST | /payments/complete | required | ✅ |
| POST | /payments/:id/cancel | required | ✅ |
| GET | /payments/:id | required | ✅ |
| POST | /webhooks/:provider | none | ✅ |

---

## Phase 5: 상품 배송 (다운로드) ✅ 완료

### 구현 파일
- [x] `domain/download.go` - 다운로드 엔티티 + DTO
- [x] `repository/download_repo.go` - 다운로드 CRUD, ProductFile CRUD
- [x] `service/download_service.go` - 다운로드 서비스 (서명된 URL)
- [x] `handler/download_handler.go` - 다운로드 핸들러 (파일 스트리밍)

### 주요 기능
- [x] 서명된 다운로드 URL 생성 (HMAC-SHA256)
- [x] 다운로드 횟수/기한 제한
- [x] 파일 스트리밍

### API 엔드포인트
| Method | Path | Auth | 상태 |
|--------|------|------|------|
| GET | /downloads | required | ✅ |
| GET | /downloads/:order_item_id/:file_id | required | ✅ |
| GET | /downloads/:token | required | ✅ |
| GET | /orders/:order_item_id/downloads | required | ✅ |

---

## Phase 6: 정산 시스템 ✅ 완료

### 구현 파일
- [x] `domain/settlement.go` - 정산 엔티티 + DTO
- [x] `repository/settlement_repo.go` - 정산 CRUD
- [x] `service/settlement_service.go` - 정산 비즈니스 로직
- [x] `handler/settlement_handler.go` - 정산 HTTP 핸들러

### 수수료 구조
```
결제금액: 100,000원
├── PG 수수료 (3.3%): 3,300원 → PG사
├── 플랫폼 수수료 (5%): 5,000원 → Angple
└── 판매자 정산: 91,700원 → 판매자
```

### API 엔드포인트
| Method | Path | Auth | 상태 |
|--------|------|------|------|
| GET | /settlements | required | ✅ |
| GET | /settlements/summary | required | ✅ |
| GET | /settlements/:id | required | ✅ |
| GET | /admin/settlements | required | ✅ |
| POST | /admin/settlements/:seller_id | required | ✅ |
| POST | /admin/settlements/:id/process | required | ✅ |

---

## 테스트 현황

### 단위 테스트
- 총 테스트: 28개
- PASS: 28개
- FAIL: 0개

### 테스트 실행
```bash
go test ./internal/plugins/commerce/... -v
```

---

## Phase 7: 최적화 & 테스트 (예정)

- [ ] 성능 최적화
- [ ] 보안 감사
- [ ] E2E 테스트
- [ ] API 문서화 (Swagger)

---

## Phase 8: 추가 기능 (예정)

- [ ] 쿠폰/할인 코드 시스템
- [ ] 상품 리뷰/평점
- [ ] 카카오페이 연동
- [ ] 실물 상품 배송 추적
- [ ] 판매자 대시보드 고도화

---

## 파일 구조

```
internal/plugins/commerce/
├── domain/
│   ├── product.go          # 상품 엔티티
│   ├── product_file.go     # 디지털 파일 엔티티
│   ├── cart.go             # 장바구니 엔티티
│   ├── order.go            # 주문 엔티티
│   ├── payment.go          # 결제 엔티티
│   ├── download.go         # 다운로드 엔티티
│   └── settlement.go       # 정산 엔티티
├── repository/
│   ├── product_repo.go     # 상품 저장소
│   ├── cart_repo.go        # 장바구니 저장소
│   ├── order_repo.go       # 주문 저장소
│   ├── payment_repo.go     # 결제 저장소
│   ├── download_repo.go    # 다운로드 저장소
│   └── settlement_repo.go  # 정산 저장소
├── service/
│   ├── product_service.go  # 상품 서비스
│   ├── cart_service.go     # 장바구니 서비스
│   ├── order_service.go    # 주문 서비스
│   ├── payment_service.go  # 결제 서비스
│   ├── download_service.go # 다운로드 서비스
│   └── settlement_service.go # 정산 서비스
├── handler/
│   ├── product_handler.go  # 상품 핸들러
│   ├── cart_handler.go     # 장바구니 핸들러
│   ├── order_handler.go    # 주문 핸들러
│   ├── payment_handler.go  # 결제 핸들러
│   ├── download_handler.go # 다운로드 핸들러
│   └── settlement_handler.go # 정산 핸들러
├── gateway/
│   ├── gateway.go          # PG 인터페이스
│   ├── inicis.go           # KG이니시스
│   └── tosspayments.go     # 토스페이먼츠
├── plugin.go               # 플러그인 진입점
└── progress.md             # 진행 상황 문서
```

---

## 변경 이력

| 날짜 | 변경 내용 |
|------|-----------|
| 2026-01-28 | Phase 4~6 완료: 결제, 다운로드, 정산 시스템 구현 |
| - | Phase 3 완료: 장바구니, 주문 시스템 구현 |
| - | Phase 2 완료: 상품 관리 시스템 구현 |
| - | Phase 1 완료: 플러그인 인프라 구현 |

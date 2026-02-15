# Angple Commerce Plugin - Task Checklist

## 아키텍처: MIT Core vs Optional Plugin

| 구분 | 라이선스 | 위치 | 설명 |
|------|----------|------|------|
| **Core** | MIT | `internal/plugin/` | 플러그인 시스템 인프라 (기본 포함) |
| **Commerce** | Optional | `internal/plugins/commerce/` | 이커머스 플러그인 (별도 설치) |
| **Marketplace** | Optional | `internal/plugins/marketplace/` | 중고거래 플러그인 (별도 설치) |

> 플러그인은 **플러그인 마켓플레이스 시스템**(미구현)을 통해 설치/제거/활성화/비활성화

---

## Phase 1: 기반 인프라 (2주) - ✅ 완료

### Backend 플러그인 시스템
- [x] `internal/plugin/loader.go` - plugin.yaml 파싱
- [x] `internal/plugin/manager.go` - 플러그인 관리 (메뉴 자동 등록/비활성화 포함)
- [x] `internal/plugin/registry.go` - 라우트 등록
- [x] `internal/plugin/types.go` - 타입 정의 (MenuConfig 포함)
- [x] `internal/plugin/logger.go` - 기본 로거
- [x] `cmd/api/main.go` 수정 - 플러그인 로더 통합

### Commerce DB 마이그레이션
- [x] `migration/plugins/commerce/001_init.up.sql` - 초기 테이블 (9개)
- [x] `migration/plugins/commerce/001_init.down.sql` - 롤백

### 테스트
- [x] `internal/plugin/loader_test.go` - 로딩 테스트
- [x] `internal/plugin/manager_test.go` - 매니저 테스트

---

## Phase 2: 상품 관리 (3주) - ✅ 완료

### Backend
- [x] `internal/plugins/commerce/domain/product.go` - 상품 엔티티 + DTO
- [x] `internal/plugins/commerce/domain/product_file.go` - 디지털 파일 엔티티
- [x] `internal/plugins/commerce/repository/product_repo.go` - CRUD + 검색
- [x] `internal/plugins/commerce/service/product_service.go` - 비즈니스 로직
- [x] `internal/plugins/commerce/handler/product_handler.go` - HTTP 핸들러
- [x] `internal/plugins/commerce/service/product_service_test.go` - 단위 테스트

### API Endpoints
- [x] GET `/api/plugins/commerce/products` - 내 상품 목록
- [x] POST `/api/plugins/commerce/products` - 상품 등록
- [x] GET `/api/plugins/commerce/products/:id` - 상품 상세
- [x] PUT `/api/plugins/commerce/products/:id` - 상품 수정
- [x] DELETE `/api/plugins/commerce/products/:id` - 상품 삭제
- [x] GET `/api/plugins/commerce/shop/products` - 공개 상품 목록
- [x] GET `/api/plugins/commerce/shop/products/:id` - 공개 상품 상세
- [x] GET `/api/plugins/commerce/shop/products/slug/:slug` - 슬러그로 조회

---

## Phase 3: 장바구니 & 주문 (2주) - ✅ 완료

### Backend
- [x] `domain/cart.go` - 장바구니 엔티티 + DTO
- [x] `domain/order.go` - 주문 엔티티 + DTO (OrderItem 포함)
- [x] `repository/cart_repo.go` - 장바구니 CRUD
- [x] `repository/order_repo.go` - 주문 CRUD (트랜잭션 포함)
- [x] `service/cart_service.go` - 장바구니 비즈니스 로직
- [x] `service/order_service.go` - 주문 비즈니스 로직
- [x] `handler/cart_handler.go` - 장바구니 HTTP 핸들러
- [x] `handler/order_handler.go` - 주문 HTTP 핸들러
- [x] `service/cart_service_test.go` - 장바구니 테스트
- [x] `service/order_service_test.go` - 주문 테스트

### API Endpoints
- [x] GET/POST/PUT/DELETE `/cart` - 장바구니 CRUD
- [x] POST `/orders` - 주문 생성 (장바구니 → 주문)
- [x] GET `/orders` - 주문 목록
- [x] GET `/orders/:id` - 주문 상세
- [x] POST `/orders/:id/cancel` - 주문 취소

---

## Phase 4: 결제 통합 (4주) - ✅ 완료

### Backend
- [x] `gateway/gateway.go` - PaymentGateway 인터페이스 + GatewayManager
- [x] `gateway/inicis.go` - KG이니시스
- [x] `gateway/tosspayments.go` - 토스페이먼츠
- [x] `domain/payment.go` - 결제 엔티티 + DTO
- [x] `repository/payment_repo.go` - 결제 CRUD
- [x] `service/payment_service.go` - 결제 비즈니스 로직
- [x] `handler/payment_handler.go` - 결제 HTTP 핸들러
- [x] `service/payment_service_test.go` - 결제 테스트

### API Endpoints
- [x] POST `/payments/prepare` - 결제 준비
- [x] POST `/payments/complete` - 결제 완료
- [x] POST `/payments/:id/cancel` - 결제 취소
- [x] GET `/payments/:id` - 결제 조회
- [x] POST `/webhooks/:provider` - PG 웹훅

---

## Phase 5: 다운로드 시스템 (2주) - ✅ 완료

### Backend
- [x] `domain/download.go` - 다운로드 엔티티
- [x] `repository/download_repo.go` - 다운로드 CRUD
- [x] `service/download_service.go` - 서명 URL, 횟수/기한 제한
- [x] `handler/download_handler.go` - 다운로드 HTTP 핸들러
- [x] `service/download_service_test.go` - 다운로드 테스트

### API Endpoints
- [x] GET `/downloads` - 내 다운로드 목록
- [x] GET `/downloads/:order_item_id/:file_id` - 다운로드 URL 생성
- [x] GET `/downloads/:token` - 파일 다운로드
- [x] GET `/orders/:order_item_id/downloads` - 주문별 다운로드

---

## Phase 6: 정산 시스템 (2주) - ✅ 완료

### Backend
- [x] `domain/settlement.go` - 정산 엔티티
- [x] `repository/settlement_repo.go` - 정산 CRUD
- [x] `service/settlement_service.go` - 수수료 계산, 정산 처리
- [x] `handler/settlement_handler.go` - 정산 HTTP 핸들러
- [x] `service/settlement_service_test.go` - 정산 테스트

### API Endpoints
- [x] GET `/settlements` - 내 정산 목록
- [x] GET `/settlements/summary` - 정산 요약
- [x] GET `/settlements/:id` - 정산 상세
- [x] GET `/admin/settlements` - 관리자 전체 정산
- [x] POST `/admin/settlements/:seller_id` - 정산 생성
- [x] POST `/admin/settlements/:id/process` - 정산 처리

---

## Phase 7: 최적화 & 테스트 (2주) - ✅ 완료

### 성능 최적화
- [x] `middleware/cache.go` - Redis 캐싱 미들웨어
- [x] `repository/cached_product_repo.go` - 캐시 적용 상품 저장소

### 보안 강화
- [x] `middleware/rate_limit.go` - Rate Limiting
- [x] `security/validator.go` - 입력 검증
- [x] `security/sanitizer.go` - XSS 방지

### 테스트
- [x] 28개 단위 테스트 PASS
- [x] E2E 테스트 (`e2e/commerce_e2e_test.go`)
- [x] 빌드 성공

---

## Phase 8: 추가 기능 (2주) - ✅ 완료

### 8.1 쿠폰 시스템
- [x] `domain/coupon.go` - 쿠폰 엔티티
- [x] `repository/coupon_repo.go` - 쿠폰 CRUD
- [x] `service/coupon_service.go` - 쿠폰 비즈니스 로직
- [x] `handler/coupon_handler.go` - 쿠폰 HTTP 핸들러
- [x] `service/coupon_service_test.go` - 쿠폰 테스트

### 8.2 리뷰 시스템
- [x] `domain/review.go` - 리뷰 엔티티
- [x] `repository/review_repo.go` - 리뷰 CRUD
- [x] `service/review_service.go` - 리뷰 비즈니스 로직
- [x] `handler/review_handler.go` - 리뷰 HTTP 핸들러
- [x] `service/review_service_test.go` - 리뷰 테스트

### 8.3 카카오페이 연동
- [x] `gateway/kakaopay.go` - 카카오페이 게이트웨이

### 8.4 배송 추적
- [x] `domain/shipping.go` - 배송 엔티티
- [x] `carrier/carrier.go` - 배송사 인터페이스
- [x] `carrier/cj.go` - CJ대한통운
- [x] `carrier/lotte.go` - 롯데택배
- [x] `service/shipping_service.go` - 배송 서비스
- [x] `handler/shipping_handler.go` - 배송 핸들러
- [x] `service/shipping_service_test.go` - 배송 테스트

---

## Phase 9: Admin UI + 플러그인-메뉴 연동 - ✅ 완료

### Backend (메뉴 연동)
- [x] `internal/plugin/types.go` - MenuConfig 구조체 추가
- [x] `internal/plugin/manager.go` - 메뉴 자동 등록/비활성화 로직
- [x] `internal/plugins/commerce/plugin.go` - 메뉴 정의 (6개 메뉴)
- [x] PR #40 CI 전체 통과

### Admin UI (SvelteKit)
- [x] Commerce, Widgets, Menus 라우트 존재

---

## Phase 10: 플러그인 마켓플레이스 시스템 - 🔄 미구현

> Commerce/Marketplace 같은 Optional 플러그인을 설치/제거/관리하는 시스템

### 필요 기능
- [ ] 플러그인 목록 조회 (원격 저장소 or 로컬)
- [ ] 플러그인 설치 (다운로드 + 활성화)
- [ ] 플러그인 제거 (비활성화 + 삭제)
- [ ] 플러그인 업데이트
- [ ] Admin UI에서 플러그인 관리 페이지
- [ ] 플러그인 의존성 해결
- [ ] 플러그인 설정 UI 자동 생성

### 기술 결정 필요
- [ ] 플러그인 배포 형태 (바이너리? Go 소스? 별도 저장소?)
- [ ] 플러그인 저장소 (GitHub Releases? 자체 Registry?)
- [ ] 인증/라이선스 관리

---

## 보안 체크리스트

- [ ] PG 키 암호화 저장
- [x] 다운로드 토큰 서명 검증 (HMAC-SHA256)
- [ ] CSRF 토큰 적용
- [x] Rate Limiting 구현
- [x] SQL Injection 방지 (GORM Prepared Statement)
- [x] XSS 방지 (sanitizer.go)
- [x] 입력 검증 (validator.go)

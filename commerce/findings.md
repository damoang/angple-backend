# Commerce Plugin - Findings & Research

이 문서는 개발 중 발견한 정보, API 문서, 코드 분석 결과 등을 기록합니다.

---

## 프로젝트 분석

### 현재 아키텍처
- Clean Architecture: Handler → Service → Repository → Database
- DI 패턴: main.go에서 순차적으로 생성 및 주입
- 라우트: internal/routes/routes.go에서 설정
- 웹 프레임워크: **Gin** (Fiber v2에서 전환됨)

### 플러그인 시스템 구조
```
internal/plugin/         # Core (MIT) - 플러그인 인프라
├── types.go            # Plugin 인터페이스, PluginManifest, MenuConfig 등
├── loader.go           # plugin.yaml 파싱, 플러그인 발견
├── manager.go          # 라이프사이클 관리, 메뉴 자동 등록
├── registry.go         # 라우트 등록 (/api/plugins/{name}/*)
└── logger.go           # 플러그인용 로거

internal/plugins/        # Optional 플러그인들
├── commerce/           # 이커머스 (쇼핑몰)
└── marketplace/        # 중고거래
```

### Plugin 인터페이스
```go
type Plugin interface {
    Name() string
    Initialize(ctx *PluginContext) error
    RegisterRoutes(router gin.IRouter)
    Shutdown() error
}
```

### PluginContext
```go
type PluginContext struct {
    DB       *gorm.DB
    Redis    *redis.Client
    Config   map[string]interface{}
    Logger   Logger
    BasePath string
}
```

### 플러그인 스펙 (docs/specs/plugin-spec-v1.0.md)

#### 핵심 원칙
- **Core 최소주의**: Core는 필수 기능만, 확장은 플러그인으로
- **비침투적 확장**: Core 코드/DB/API 직접 수정 금지
- **독립적 생명주기**: 설치/업데이트/제거가 다른 플러그인에 영향 없음

#### DB 규칙
- Core 테이블 수정 금지 (users, posts, comments 등)
- 플러그인 테이블: `{plugin_name}_{table_name}` (예: `commerce_products`)
- Meta 테이블로 확장 데이터 저장 가능

#### API 규칙
- 경로: `/api/plugins/{plugin-name}/*`
- 인증: `required` | `optional` | `none`

---

## MIT Core vs Optional Plugin 분리

### 결정 사항 (2026-01-31)
- Commerce 전체가 Optional 플러그인 (MIT Core에 포함하지 않음)
- 필요한 사용자만 플러그인 마켓플레이스에서 설치
- 플러그인 마켓플레이스 시스템은 아직 미구현 (Phase 10)

### Core에 포함되는 것
- `internal/plugin/` - 플러그인 시스템 인프라
- `internal/domain/menu.go` - 메뉴 (PluginName 필드 포함)
- `cmd/api/main.go` - 플러그인 로딩/활성화 코드

### Optional로 분리되는 것
- `internal/plugins/commerce/` - 이커머스 전체
- `internal/plugins/marketplace/` - 중고거래 전체
- `migration/plugins/commerce/` - Commerce DB 마이그레이션
- `migration/plugins/marketplace/` - Marketplace DB 마이그레이션

---

## PG 연동 참조 자료

### KG이니시스
- 문서: https://manual.inicis.com/
- 테스트 환경: https://iniweb.inicis.com/
- 구현: `gateway/inicis.go`

### 토스페이먼츠
- 문서: https://docs.tosspayments.com/
- 테스트 환경: https://dashboard.tosspayments.com/
- 구현: `gateway/tosspayments.go`

### 카카오페이
- 문서: https://developers.kakao.com/docs/latest/ko/kakaopay/
- 구현: `gateway/kakaopay.go`
- 플로우: 준비(ready) → 인증 → 승인(approve) → 완료

---

## 배송사 API 참조

### 스마트택배 (SweetTracker)
- API: https://tracking.sweettracker.co.kr/
- 배송사 코드:
  - CJ대한통운: 04
  - 롯데택배: 08
  - 한진택배: 05
  - 우체국택배: 01
- 구현: `carrier/cj.go`, `carrier/lotte.go`

---

## Commerce 파일 현황 (65+ 파일)

### 도메인 (domain/)
| 파일 | 설명 |
|------|------|
| product.go | 상품 엔티티, DTO |
| product_file.go | 디지털 파일 엔티티 |
| cart.go | 장바구니 엔티티, DTO |
| order.go | 주문/주문아이템 엔티티, DTO |
| payment.go | 결제 엔티티, DTO |
| download.go | 다운로드 엔티티 |
| settlement.go | 정산 엔티티 |
| coupon.go | 쿠폰 엔티티 |
| review.go | 리뷰 엔티티 |
| shipping.go | 배송 엔티티 |

### 리포지토리 (repository/)
| 파일 | 설명 |
|------|------|
| product_repo.go | 상품 CRUD |
| cached_product_repo.go | Redis 캐시 데코레이터 |
| cart_repo.go | 장바구니 CRUD |
| order_repo.go | 주문 CRUD (트랜잭션) |
| payment_repo.go | 결제 CRUD |
| download_repo.go | 다운로드 CRUD |
| settlement_repo.go | 정산 CRUD |
| coupon_repo.go | 쿠폰 CRUD |
| review_repo.go | 리뷰 CRUD |

### 테스트 현황
- product_service_test.go ✅
- cart_service_test.go ✅
- order_service_test.go ✅
- payment_service_test.go ✅
- download_service_test.go ✅
- settlement_service_test.go ✅
- coupon_service_test.go ✅
- review_service_test.go ✅
- shipping_service_test.go ✅
- commerce_e2e_test.go ✅
- **총 28+ 테스트 PASS**

---

## 결정 사항

| 날짜 | 항목 | 결정 | 이유 |
|------|------|------|------|
| 2026-01-28 | PG 연동 | KG이니시스 + 토스페이먼츠 동시 | 점유율 + 개발자 친화성 |
| 2026-01-28 | 상품 유형 | 디지털 + 실물 동시 | 플랫폼 확장성 |
| 2026-01-28 | 일정 | 20주 (여유있게) | 추가 기능 포함 |
| 2026-01-31 | 아키텍처 | MIT Core + Optional Plugin | 이커머스 불필요 사용자 배려 |
| 2026-01-31 | 설치 방식 | 플러그인 마켓플레이스 | 미구현, Phase 10 |

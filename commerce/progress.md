# Commerce Plugin - Progress Log

이 문서는 작업 진행 상황, 시도한 내용, 에러 및 해결 방법을 기록합니다.

---

## 2026-01-28

### Phase 1 완료
- [x] 플러그인 인프라 구축 (loader, manager, registry, types, logger)
- [x] DB 마이그레이션 (9개 테이블)
- [x] 테스트 8개 PASS
- 빌드: `go build ./...` ✅

### Phase 2 완료
- [x] 상품 도메인/리포/서비스/핸들러
- [x] 단위 테스트 5개 (13개 서브테스트)
- 빌드: ✅

### Phase 3 완료
- [x] 장바구니 + 주문 전체 구현
- [x] 장바구니 테스트 4개 (10개 서브테스트)
- [x] 주문 테스트 4개 (10개 서브테스트)
- 빌드: ✅, 테스트 28개 전체 PASS

---

## 2026-01-29 (추정)

### Phase 4 완료
- [x] PG 게이트웨이 인터페이스 + Manager
- [x] KG이니시스, 토스페이먼츠 구현
- [x] 결제 도메인/리포/서비스/핸들러
- [x] payment_service_test.go

### Phase 5 완료
- [x] 디지털 다운로드 시스템 (서명 URL, 횟수/기한 제한)
- [x] download_service_test.go

### Phase 6 완료
- [x] 정산 시스템 (수수료 구조: PG 3.3% + 플랫폼 5%)
- [x] settlement_service_test.go

---

## 2026-01-30 (추정)

### Phase 7 완료
- [x] Redis 캐싱 (상품 목록/상세)
- [x] Rate Limiting (IP당 100req/min, 결제 30req/min)
- [x] 입력 검증 + XSS 방지
- [x] E2E 테스트

### Phase 8 완료
- [x] 쿠폰 시스템 (정액/정률/무료배송/첫구매)
- [x] 리뷰 시스템 (별점, 사진, 판매자 답글, 도움됨)
- [x] 카카오페이 게이트웨이
- [x] 배송 추적 (CJ대한통운, 롯데택배)

---

## 2026-01-31

### Phase 9 완료 (커밋됨)
- [x] MenuConfig 구조체 추가 (types.go)
- [x] 플러그인 메뉴 자동 등록/비활성화 (manager.go)
- [x] Commerce 메뉴 6개 정의 (plugin.go)
- [x] PR #40 CI 전체 통과

### Marketplace 플러그인 (미커밋)
- [x] 중고거래 플러그인 코드 작성
  - domain: item, category, wish
  - repository: item, category, wish
  - service: item, category, wish
  - handler: item, category, wish
  - plugin.go: 매니페스트 + 라우트

### 아키텍처 결정: MIT Core vs Optional Plugin
- Core (MIT): `internal/plugin/` 플러그인 인프라만 기본 포함
- Commerce: Optional 플러그인 (이커머스 - 별도 설치)
- Marketplace: Optional 플러그인 (중고거래 - 별도 설치)
- **플러그인 마켓플레이스 시스템**: 미구현 → Phase 10으로 추가

---

## 에러 로그

| 날짜 | 에러 | 원인 | 해결 방법 |
|------|------|------|-----------|
| - | - | - | - |

---

## 성능 메모

- Redis 캐싱 적용 시 상품 목록 TTL: 5분, 상세: 10분
- Rate Limiting: Redis 기반, IP당 100req/min (결제: 30req/min)

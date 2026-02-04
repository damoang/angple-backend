# Angple Backend - 작업 완료 시 체크리스트

## 필수 실행 명령어
```bash
# 1. 코드 포맷팅
make fmt

# 2. 린트 검사
make lint

# 3. 테스트 실행
make test
```

## PR 생성 전 체크리스트
- [ ] `make test` 통과
- [ ] `make lint` 통과
- [ ] 새 기능에 대한 테스트 코드 작성
- [ ] API 변경 시 `docs/swagger.yaml` 업데이트
- [ ] 파일 크기 1000줄 이하 확인

## 커밋 규칙
- **브랜치**: `feature/작업명` → PR → `main` 머지
- **커밋/푸시 전 승인 필수** (사용자 승인 후 진행)

## 환경변수 변경 시
- `.env.example` 업데이트
- `configs/config.dev.yaml` 업데이트 (필요시)
- README.md 환경 설정 섹션 업데이트

## 데이터베이스 변경 시
- 마이그레이션 스크립트 작성 (`.docker/mysql/init/`)
- DATABASE.md 업데이트
- 기존 Gnuboard 호환성 확인

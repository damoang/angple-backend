.PHONY: help dev dev-docker dev-docker-down dev-docker-logs build build-api build-gateway build-migrate test clean docker-up docker-down migrate migrate-dry-run migrate-verify

# 기본 타겟
help:
	@echo "Angple Backend - Available Commands:"
	@echo ""
	@echo "📦 로컬 개발 (Docker All-in-One - 권장):"
	@echo "  make dev-docker       - Docker로 개발 환경 시작 (MySQL + Redis + API)"
	@echo "  make dev-docker-down  - Docker 개발 환경 중지"
	@echo "  make dev-docker-logs  - Docker 개발 환경 로그 확인"
	@echo ""
	@echo "🔧 로컬 개발 (직접 실행):"
	@echo "  make dev              - 로컬 개발 환경 실행 (go run, MySQL/Redis 필요)"
	@echo ""
	@echo "🏗️  빌드:"
	@echo "  make build            - 전체 빌드 (api + gateway)"
	@echo "  make build-api        - API 서버 빌드"
	@echo "  make build-gateway    - Gateway 빌드"
	@echo ""
	@echo "🧪 테스트:"
	@echo "  make test             - 테스트 실행"
	@echo "  make test-coverage    - 커버리지 포함 테스트"
	@echo ""
	@echo "📚 Swagger 문서:"
	@echo "  make swagger          - Swagger 문서 자동 생성 (docs/)"
	@echo "  make swagger-fmt      - Swagger 주석 포맷팅"
	@echo ""
	@echo "🚀 운영 환경:"
	@echo "  make docker-up        - 운영 Docker Compose 실행 (외부 DB 연결)"
	@echo "  make docker-down      - 운영 Docker Compose 중지"
	@echo ""
	@echo "🔄 마이그레이션 (g5_* → v2_*):"
	@echo "  make migrate          - 전체 데이터 마이그레이션 실행"
	@echo "  make migrate-dry-run  - 마이그레이션 미리보기 (실행 안함)"
	@echo "  make migrate-verify   - 마이그레이션 데이터 검증"
	@echo ""
	@echo "🧹 기타:"
	@echo "  make clean            - 빌드 결과물 삭제"
	@echo "  make fmt              - 코드 포맷팅"
	@echo "  make lint             - 린트 실행"

# 로컬 개발 환경 (Docker All-in-One)
dev-docker:
	@echo "Starting development environment with Docker (MySQL + Redis + API)..."
	@echo "Containers: angple-dev-mysql, angple-dev-redis, angple-dev-api"
	docker compose -f docker-compose.dev.yml up -d
	@echo ""
	@echo "✅ Development environment started!"
	@echo "   API: http://localhost:8081"
	@echo "   MySQL: localhost:3306"
	@echo "   Redis: localhost:6379"
	@echo ""
	@echo "Check logs: make dev-docker-logs"
	@echo "Stop: make dev-docker-down"

dev-docker-down:
	@echo "Stopping development environment..."
	docker compose -f docker-compose.dev.yml down

dev-docker-logs:
	@echo "Showing logs (Ctrl+C to exit)..."
	docker compose -f docker-compose.dev.yml logs -f

dev-docker-rebuild:
	@echo "Rebuilding development environment..."
	docker compose -f docker-compose.dev.yml up -d --build

# 로컬 개발 환경 (직접 실행)
dev:
	@echo "Starting API server in development mode..."
	@echo "⚠️  Requires: MySQL on localhost:3306, Redis on localhost:6379"
	APP_ENV=local go run cmd/api/main.go

dev-gateway:
	@echo "Starting Gateway in development mode..."
	go run cmd/gateway/main.go

# 빌드
build: build-api build-gateway build-migrate

build-api:
	@echo "Building API server..."
	go build -o bin/api cmd/api/main.go

build-gateway:
	@echo "Building Gateway..."
	go build -o bin/gateway cmd/gateway/main.go

build-migrate:
	@echo "Building Migration tool..."
	go build -o bin/migrate cmd/migrate/main.go

# 마이그레이션
migrate:
	@echo "Running data migration (all targets)..."
	go run cmd/migrate/main.go -target=all

migrate-dry-run:
	@echo "Dry-run migration..."
	go run cmd/migrate/main.go -dry-run

migrate-verify:
	@echo "Verifying migration data..."
	go run cmd/migrate/main.go -verify

# 테스트
test:
	@echo "Running tests..."
	go test -v ./...

test-coverage:
	@echo "Running tests with coverage..."
	go test -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Docker
docker-up:
	@echo "Starting Docker containers..."
	docker-compose up -d

docker-down:
	@echo "Stopping Docker containers..."
	docker-compose down

docker-logs:
	docker-compose logs -f

docker-rebuild:
	@echo "Rebuilding Docker containers..."
	docker-compose up -d --build

# 정리
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	rm -f coverage.out coverage.html

# Go 모듈
deps:
	@echo "Downloading dependencies..."
	go mod download

tidy:
	@echo "Tidying go.mod..."
	go mod tidy

# 린트
lint:
	@echo "Running linter..."
	golangci-lint run

# 포맷
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Swagger 문서 생성
swagger:
	@echo "Generating Swagger documentation..."
	swag init -g cmd/api/main.go -o docs
	@echo "✅ Swagger docs generated in docs/"
	@echo "   View at: http://localhost:8082/swagger/index.html"

swagger-fmt:
	@echo "Formatting Swagger comments..."
	swag fmt

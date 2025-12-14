# Angple Backend Setup Guide

차세대 다모앙 백엔드 로컬 개발 환경 설정 가이드

## 🚀 빠른 시작

### 1. 레포지토리 클론 및 환경 설정

```bash
cd /path/to/projects
git clone <angple-backend-repo>
cd angple-backend

# 환경 변수 파일 생성
cp .env.example .env
```

### 2. Docker Compose로 서비스 시작

```bash
# MySQL + Redis 시작
docker-compose up -d mysql redis

# 로그 확인
docker-compose logs -f mysql
```

### 3. API 서버 실행

```bash
# Go 의존성 설치
go mod download

# API 서버 실행
go run cmd/api/main.go

# 또는
make run
```

서버가 시작되면 http://localhost:8081 에서 API에 접근할 수 있습니다.

---

## 📦 포함된 서비스

### MySQL Database
- **컨테이너명**: angple-mysql
- **포트**: 3307 (호스트) → 3306 (컨테이너)
- **데이터베이스**: angple_db
- **사용자**: angple_user
- **비밀번호**: angple_pass_2024 (로컬 개발용)

**왜 3307 포트?**
현세대(ang-gnu)가 3306을 사용하므로 포트 충돌 방지

### Redis Cache
- **컨테이너명**: angple-redis
- **포트**: 6379

### API Server (별도 실행)
- **포트**: 8081
- **프레임워크**: Go Fiber

---

## 🗄️ 데이터베이스

### 초기 스키마

Docker Compose 첫 실행 시 `.docker/mysql/init/` 의 SQL 파일들이 자동으로 실행됩니다:

1. **01-schema.sql**: 테이블 생성
   - `menus` - 메뉴 시스템 (계층 구조)
   - `users` - 사용자
   - `boards` - 게시판

2. **02-seed-menus.sql**: 메뉴 샘플 데이터 (29개 메뉴)

### MySQL 접속

```bash
# Docker exec로 접속
docker exec -it angple-mysql mysql -uangple_user -pangple_pass_2024 angple_db

# 외부 클라이언트 (TablePlus, DBeaver 등)
Host: localhost
Port: 3307
User: angple_user
Password: angple_pass_2024
Database: angple_db
```

### 메뉴 데이터 확인

```sql
-- 전체 메뉴 수
SELECT COUNT(*) FROM menus;

-- 루트 메뉴
SELECT * FROM menus WHERE parent_id IS NULL;

-- 계층 구조 확인
SELECT
    CONCAT(REPEAT('  ', depth - 1), title) AS menu_tree,
    url, icon, shortcut
FROM menus
WHERE is_active = TRUE
ORDER BY COALESCE(parent_id, 0), order_num;
```

---

## 🛠️ Docker Compose 명령어

```bash
# 전체 서비스 시작
docker-compose up -d

# 특정 서비스만 시작
docker-compose up -d mysql
docker-compose up -d redis

# 서비스 중지
docker-compose stop

# 서비스 + 컨테이너 제거 (데이터는 보존)
docker-compose down

# 서비스 + 컨테이너 + 볼륨 제거 (데이터 삭제)
docker-compose down -v

# 로그 확인
docker-compose logs -f mysql
docker-compose logs -f redis

# 재시작
docker-compose restart mysql
```

---

## 🔄 데이터베이스 초기화

### 데이터 완전 초기화가 필요한 경우

```bash
# 1. 모든 컨테이너와 볼륨 제거
docker-compose down -v

# 2. 다시 시작 (초기화 스크립트 자동 실행)
docker-compose up -d mysql

# 3. 로그 확인
docker-compose logs -f mysql
```

---

## 🌍 환경별 설정

### 로컬 개발 (.env)

```bash
# Database (Docker 컨테이너 내부)
DB_HOST=mysql              # Docker 컨테이너명
DB_PORT=3306               # 컨테이너 내부 포트
DB_USER=angple_user
DB_PASSWORD=angple_pass_2024
DB_NAME=angple_db

# Redis
REDIS_HOST=redis
REDIS_PORT=6379
```

### 운영 서버

```bash
# Database (외부 MySQL 서버)
DB_HOST=prod-mysql.example.com
DB_PORT=3306
DB_USER=angple_prod_user
DB_PASSWORD=강력한_비밀번호
DB_NAME=angple_production

# Redis (외부 Redis 서버)
REDIS_HOST=prod-redis.example.com
REDIS_PORT=6379
```

---

## 🔧 트러블슈팅

### MySQL 컨테이너가 시작되지 않음

```bash
# 1. 로그 확인
docker-compose logs mysql

# 2. 포트 충돌 확인
lsof -i :3307

# 3. 기존 컨테이너 확인 및 제거
docker ps -a | grep angple-mysql
docker rm -f angple-mysql

# 4. 다시 시작
docker-compose up -d mysql
```

### "Can't connect to MySQL server" 에러

```bash
# MySQL이 완전히 시작될 때까지 대기 (약 10-15초)
docker-compose logs -f mysql

# "ready for connections" 메시지 확인 후 API 재시작
```

### Go 모듈 관련 에러

```bash
# 모듈 정리
go mod tidy

# 캐시 클리어
go clean -modcache

# 재다운로드
go mod download
```

---

## 📚 관련 문서

- **[DATABASE.md](./DATABASE.md)** - 데이터베이스 상세 가이드
- **[README.md](./README.md)** - 프로젝트 개요 및 API 문서
- **[docs/api-roadmap.csv](./docs/api-roadmap.csv)** - API 개발 로드맵

---

## 🏃 다음 단계

1. ✅ MySQL, Redis 시작 (`docker-compose up -d`)
2. ✅ API 서버 실행 (`go run cmd/api/main.go`)
3. 📝 API 테스트 (Postman, curl 등)
4. 🎨 프론트엔드 연동 (angple 프로젝트)

---

## 🔗 관련 프로젝트

- **angple**: 프론트엔드 (SvelteKit) - API를 호출하는 웹 애플리케이션
- **angple-auth**: 인증 서비스 (Go) - JWT 토큰 발급
- **ang-gnu**: 현세대 시스템 (PHP/Gnuboard) - 레거시 시스템

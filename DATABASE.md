# Angple Database Setup Guide

차세대 다모앙 데이터베이스 구성 가이드

## 환경별 데이터베이스 구성

### 1. Local (로컬 개발)
- **방식**: Docker MySQL 컨테이너
- **포트**: 3307 (현세대 3306과 충돌 방지)
- **자동 초기화**: 메뉴 테이블 + 시드 데이터

### 2. Dev (개발 서버)
- **방식**: 외부 MySQL 서버
- **설정**: `.env.dev` 파일

### 3. Staging (스테이징)
- **방식**: 외부 MySQL 서버
- **설정**: `.env.staging` 파일

### 4. Production (운영)
- **방식**: 외부 MySQL 서버 (RDS 등)
- **설정**: `.env.production` 파일

---

## 로컬 개발 환경 설정

### 1. 초기 설정

```bash
# 1. 환경 변수 파일 생성
cp .env.example .env

# 2. Docker Compose로 MySQL 시작
docker-compose up -d mysql

# 3. MySQL 상태 확인
docker-compose ps
docker-compose logs mysql

# 4. MySQL 접속 테스트
docker exec -it angple-mysql mysql -uangple_user -pangple_pass_2024 angple_db
```

### 2. 데이터베이스 확인

```sql
-- MySQL 접속 후
SHOW DATABASES;
USE angple_db;

-- 테이블 확인
SHOW TABLES;

-- 메뉴 데이터 확인
SELECT * FROM menus ORDER BY depth, order_num;

-- 계층 구조 확인
SELECT
    CONCAT(REPEAT('  ', depth - 1), title) AS menu_tree,
    url,
    icon,
    shortcut
FROM menus
WHERE is_active = TRUE
ORDER BY
    COALESCE(parent_id, 0),
    order_num;
```

### 3. 환경 변수 (.env)

```bash
# Database Configuration (로컬)
DB_HOST=localhost
DB_PORT=3307
DB_NAME=angple_db
DB_USER=angple_user
DB_PASSWORD=angple_pass_2024
```

---

## 원격 서버 환경 설정

### Dev 서버

```bash
# .env.dev 파일 사용
export APP_ENV=dev

# DB 설정 업데이트
DB_HOST=dev-mysql.example.com
DB_PORT=3306
DB_NAME=angple_dev
DB_USER=angple_dev_user
DB_PASSWORD=실제_비밀번호
```

### Staging/Production 서버

```bash
# 서버에서 직접 설정
ln -sf .env.staging .env   # 스테이징
ln -sf .env.production .env  # 운영
```

---

## 데이터베이스 스키마

### 메뉴 테이블 (menus)

| 컬럼 | 타입 | 설명 |
|------|------|------|
| id | BIGINT | 메뉴 ID (Primary Key) |
| parent_id | BIGINT | 부모 메뉴 ID (NULL이면 루트) |
| title | VARCHAR(100) | 메뉴 제목 |
| url | VARCHAR(255) | 메뉴 URL |
| icon | VARCHAR(50) | Lucide 아이콘 이름 |
| shortcut | VARCHAR(10) | 단축키 (F, Q, G 등) |
| depth | TINYINT | 메뉴 깊이 (1, 2, 3) |
| order_num | INT | 정렬 순서 |
| is_active | BOOLEAN | 활성화 여부 |
| show_in_header | BOOLEAN | 헤더 노출 |
| show_in_sidebar | BOOLEAN | 사이드바 노출 |

---

## Docker Compose 명령어

### 기본 명령어

```bash
# MySQL 컨테이너만 시작
docker-compose up -d mysql

# 전체 서비스 시작 (web, admin 포함)
docker-compose up -d

# 로그 확인
docker-compose logs -f mysql

# MySQL 재시작
docker-compose restart mysql

# MySQL 중지
docker-compose stop mysql

# MySQL 완전 삭제 (데이터 포함)
docker-compose down -v
```

### 데이터베이스 관리

```bash
# MySQL 접속
docker exec -it angple-mysql mysql -uroot -pangple_root_2024

# 데이터베이스 덤프
docker exec angple-mysql mysqldump \
  -uangple_user -pangple_pass_2024 \
  angple_db > backup.sql

# 데이터베이스 복원
docker exec -i angple-mysql mysql \
  -uangple_user -pangple_pass_2024 \
  angple_db < backup.sql
```

---

## 초기화 스크립트

`.docker/mysql/init/` 디렉토리의 SQL 파일들이 MySQL 컨테이너 첫 실행 시 자동으로 실행됩니다:

1. **01-schema.sql**: 테이블 스키마 생성
   - menus (메뉴 테이블)
   - users (사용자 테이블)
   - boards (게시판 테이블)

2. **02-seed-menus.sql**: 메뉴 시드 데이터
   - 커뮤니티 메뉴
   - 소모임 메뉴
   - 리뷰 메뉴 등

### 초기화 스크립트 재실행

```bash
# 1. MySQL 컨테이너와 볼륨 완전 삭제
docker-compose down -v

# 2. 다시 시작 (초기화 스크립트 자동 실행)
docker-compose up -d mysql
```

---

## 트러블슈팅

### MySQL 컨테이너가 시작되지 않을 때

```bash
# 로그 확인
docker-compose logs mysql

# 포트 충돌 확인 (3307 포트 사용 중인지)
lsof -i :3307

# 기존 컨테이너 확인 및 삭제
docker ps -a | grep angple-mysql
docker rm -f angple-mysql
```

### 데이터 초기화가 필요할 때

```bash
# 볼륨 포함 완전 삭제
docker-compose down -v

# 다시 시작
docker-compose up -d mysql
```

### 외부에서 MySQL 접속 시

```bash
# 로컬에서 접속
mysql -h 127.0.0.1 -P 3307 -uangple_user -pangple_pass_2024 angple_db

# GUI 툴 (TablePlus, DBeaver 등)
Host: localhost
Port: 3307
User: angple_user
Password: angple_pass_2024
Database: angple_db
```

---

## 현세대(ang-gnu) 연동

현세대 메뉴 데이터를 차세대로 동기화하려면:

```bash
# TODO: 동기화 스크립트 작성 예정
# scripts/sync-menus-from-legacy.sh
```

---

## 보안 주의사항

1. **.env 파일 관리**
   - `.env` (로컬용)는 절대 커밋하지 마세요
   - `.env.dev`, `.env.staging`, `.env.production`은 비밀번호를 제외하고 커밋 가능
   - 실제 비밀번호는 서버에서 직접 설정

2. **운영 환경 비밀번호**
   - 예시 비밀번호(`angple_pass_2024`)는 개발용입니다
   - 운영/스테이징 환경에서는 강력한 비밀번호 사용

3. **포트 노출**
   - 운영 환경에서는 MySQL 포트를 외부에 노출하지 마세요
   - 애플리케이션 서버에서만 접근 가능하도록 설정

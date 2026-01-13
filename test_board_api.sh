#!/bin/bash

# Board Management API 테스트 스크립트

API_URL="http://localhost:8082/api/v2"

echo "=== Board Management API 테스트 ==="
echo ""

# JWT 토큰 생성 (generate_token 프로그램 사용)
echo "1. JWT 토큰 생성 중..."
JWT_TOKEN=$(go run cmd/generate_token/main.go)

echo "JWT Token: ${JWT_TOKEN:0:50}..."
echo ""

# 게시판 생성 테스트
echo "2. 게시판 생성 테스트 (POST /api/v2/boards)"
CREATE_RESPONSE=$(curl -s -X POST "${API_URL}/boards" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d '{
    "board_id": "free",
    "group_id": "community",
    "subject": "자유게시판",
    "device": "both",
    "list_level": 1,
    "read_level": 1,
    "write_level": 2,
    "comment_level": 2,
    "use_category": 0,
    "skin": "basic",
    "page_rows": 20,
    "upload_count": 2
  }')

echo "$CREATE_RESPONSE" | jq .
echo ""

# 게시판 목록 조회
echo "3. 게시판 목록 조회 (GET /api/v2/boards)"
curl -s "${API_URL}/boards" | jq .
echo ""

# 특정 게시판 조회
echo "4. 특정 게시판 조회 (GET /api/v2/boards/free)"
curl -s "${API_URL}/boards/free" | jq .
echo ""

# 게시판 수정 테스트
echo "5. 게시판 수정 테스트 (PUT /api/v2/boards/free)"
curl -s -X PUT "${API_URL}/boards/free" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${JWT_TOKEN}" \
  -d '{
    "subject": "자유게시판 (수정됨)",
    "page_rows": 30
  }' | jq .
echo ""

# 동적 테이블 확인
echo "6. 동적 테이블 생성 확인 (g5_write_free 테이블)"
docker exec angple-dev-mysql mysql -udamoang_user -pdev_pass_2024 damoang -e "SHOW TABLES LIKE 'g5_write_%'" 2>&1 | grep -v "insecure"
echo ""

echo "=== 테스트 완료 ==="

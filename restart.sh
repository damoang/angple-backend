#!/bin/bash

###############################################################################
# angple-backend 재시작 스크립트
# 사용법: ./restart.sh [build]
#   build 옵션: 재시작 전 빌드 수행
###############################################################################

set -e  # 에러 발생 시 스크립트 중단

BACKEND_DIR="/home/damoang/angple-backend"
BINARY="angple-backend"
LOG_FILE="$BACKEND_DIR/api.log"
PID_FILE="$BACKEND_DIR/angple-backend.pid"

cd "$BACKEND_DIR" || exit 1

echo "=========================================="
echo "  angple-backend 재시작"
echo "=========================================="
echo ""

# 1. 기존 프로세스 종료
echo "[1/4] 기존 프로세스 종료 중..."
if pgrep -f "$BINARY" > /dev/null; then
    # Graceful shutdown (SIGTERM)
    pkill -TERM -f "$BINARY"
    echo "  - SIGTERM 전송, 5초 대기..."
    sleep 5

    # 아직 살아있으면 강제 종료 (SIGKILL)
    if pgrep -f "$BINARY" > /dev/null; then
        echo "  - 프로세스가 종료되지 않음, SIGKILL 전송..."
        pkill -9 -f "$BINARY"
        sleep 1
    fi

    echo "  ✓ 기존 프로세스 종료 완료"
else
    echo "  - 실행 중인 프로세스 없음"
fi

# PID 파일 제거
rm -f "$PID_FILE"

# 2. 빌드 (선택)
if [ "$1" = "build" ]; then
    echo ""
    echo "[2/4] 빌드 수행 중..."
    go build -o "$BINARY" cmd/api/main.go
    if [ $? -eq 0 ]; then
        echo "  ✓ 빌드 완료"
    else
        echo "  ✗ 빌드 실패"
        exit 1
    fi
else
    echo ""
    echo "[2/4] 빌드 건너뜀 (빌드하려면 './restart.sh build' 실행)"
fi

# 3. 로그 파일 백업 (옵션)
if [ -f "$LOG_FILE" ] && [ $(stat -f%z "$LOG_FILE" 2>/dev/null || stat -c%s "$LOG_FILE" 2>/dev/null) -gt 10485760 ]; then
    echo ""
    echo "[3/4] 로그 파일 백업 중... (10MB 이상)"
    mv "$LOG_FILE" "$LOG_FILE.$(date +%Y%m%d_%H%M%S).bak"
    echo "  ✓ 로그 백업 완료"
else
    echo ""
    echo "[3/4] 로그 백업 건너뜀"
fi

# 4. 백엔드 시작
echo ""
echo "[4/4] 백엔드 시작 중..."

# nohup으로 백그라운드 실행
nohup ./"$BINARY" > "$LOG_FILE" 2>&1 &
BACKEND_PID=$!

# PID 저장
echo $BACKEND_PID > "$PID_FILE"

echo "  - PID: $BACKEND_PID"
echo "  - 로그: $LOG_FILE"

# 프로세스 시작 확인 (3초 대기)
sleep 3

if ps -p $BACKEND_PID > /dev/null 2>&1; then
    echo ""
    echo "=========================================="
    echo "  ✓ 백엔드 시작 완료"
    echo "=========================================="
    echo ""
    echo "실행 중인 프로세스:"
    ps aux | grep "$BINARY" | grep -v grep
    echo ""
    echo "로그 확인: tail -f $LOG_FILE"
    echo "프로세스 중지: pkill -f $BINARY"
else
    echo ""
    echo "=========================================="
    echo "  ✗ 백엔드 시작 실패"
    echo "=========================================="
    echo ""
    echo "최근 로그 (마지막 20줄):"
    tail -20 "$LOG_FILE"
    exit 1
fi

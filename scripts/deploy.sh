#!/bin/bash
# Backend deployment script
# Usage: ./scripts/deploy.sh <release_tag> <target>
# Example: ./scripts/deploy.sh deploy-20260228-123456 api

set -e

RELEASE_TAG="${1:-latest}"
TARGET="${2:-api}"
REPO="damoang/angple-backend"
BACKEND_DIR="/home/angple/backend"

cd "$BACKEND_DIR"

echo "=== Backend Deploy Script ==="
echo "Release: $RELEASE_TAG"
echo "Target: $TARGET"
echo ""

# Download binaries (as angple user with gh auth)
if [[ "$TARGET" == "api" || "$TARGET" == "all" ]]; then
    echo "Downloading API binary..."
    gh release download "$RELEASE_TAG" --repo "$REPO" --pattern api -D /tmp --clobber
    mv /tmp/api ./bin/api-new
    chmod +x ./bin/api-new
    echo "API binary downloaded."
fi

if [[ "$TARGET" == "gateway" || "$TARGET" == "all" ]]; then
    echo "Downloading Gateway binary..."
    gh release download "$RELEASE_TAG" --repo "$REPO" --pattern gateway -D /tmp --clobber
    mv /tmp/gateway ./bin/gateway-new
    chmod +x ./bin/gateway-new
    echo "Gateway binary downloaded."
fi

# Deploy API
if [[ "$TARGET" == "api" || "$TARGET" == "all" ]]; then
    echo "Deploying API..."
    sudo pkill -f "./bin/api" 2>/dev/null || true
    sleep 2
    mv ./bin/api-new ./bin/api

    # Start API in background
    source .env 2>/dev/null || true
    nohup ./bin/api >> /home/angple/api.log 2>&1 &
    API_PID=$!
    sleep 5

    if pgrep -f "./bin/api" > /dev/null; then
        echo "API deployed successfully (PID: $(pgrep -f './bin/api' | head -1))"
    else
        echo "ERROR: API failed to start"
        tail -20 /home/angple/api.log
        exit 1
    fi
fi

# Deploy Gateway
if [[ "$TARGET" == "gateway" || "$TARGET" == "all" ]]; then
    echo "Deploying Gateway..."
    sudo pkill -f "./bin/gateway" 2>/dev/null || true
    sleep 2
    mv ./bin/gateway-new ./bin/gateway

    nohup ./bin/gateway >> /home/angple/gateway.log 2>&1 &
    sleep 5

    if pgrep -f "./bin/gateway" > /dev/null; then
        echo "Gateway deployed successfully (PID: $(pgrep -f './bin/gateway' | head -1))"
    else
        echo "ERROR: Gateway failed to start"
        tail -20 /home/angple/gateway.log
        exit 1
    fi
fi

echo ""
echo "=== Deploy completed at $(date) ==="

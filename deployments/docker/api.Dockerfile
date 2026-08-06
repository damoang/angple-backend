# Build stage - cross-compile without QEMU emulation
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /app/api ./cmd/api

# 오목 실시간 대전 서버 — 같은 이미지에 넣되 **별도 Deployment 로 띄운다**.
# 장시간 유지되는 WebSocket 연결이 API 롤아웃에 끌려 끊기지 않게 하기 위해서다.
# (실행 진입점은 k8s command 로 지정 — 이 이미지의 기본 CMD 는 api 그대로)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /app/omok-ws ./cmd/omok-ws

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl tzdata

WORKDIR /app

COPY --from=builder /app/api .
COPY --from=builder /app/omok-ws .
COPY --from=builder /app/configs ./configs

EXPOSE 8081 8084

CMD ["./api"]

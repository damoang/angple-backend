# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# 의존성 복사 및 다운로드
COPY go.mod go.sum ./
RUN go mod download

# 소스 코드 복사
COPY . .

# 빌드
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api cmd/api/main.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# 바이너리 복사
COPY --from=builder /app/api .
COPY --from=builder /app/configs ./configs

EXPOSE 8082

CMD ["./api"]

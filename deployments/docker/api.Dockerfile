# Build stage - cross-compile without QEMU emulation
FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETARCH

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /app/api ./cmd/api

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates curl tzdata

WORKDIR /app

COPY --from=builder /app/api .
COPY --from=builder /app/configs ./configs

EXPOSE 8081

CMD ["./api"]

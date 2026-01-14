package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func main() {
	// 환경 변수
	laravelBackendURL := getEnv("LARAVEL_BACKEND_URL", "http://localhost:80")
	goAPIURL := getEnv("GO_API_URL", "http://localhost:8081")
	gatewayPort := getEnv("GATEWAY_PORT", "8080")

	log.Printf("Starting API Gateway on port %s", gatewayPort)
	log.Printf("Laravel Backend: %s", laravelBackendURL)
	log.Printf("Go API Backend: %s", goAPIURL)

	// Gin 라우터 생성
	router := gin.Default()

	// CORS 설정
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"http://localhost:3010", "http://localhost:5173", "https://damoang.dev", "https://api.damoang.dev", "https://web.damoang.net", "https://damoang.net"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowCredentials: true,
	}))

	// Health Check
	router.GET("/health", func(c *gin.Context) {
		c.String(200, "Gateway OK")
	})

	// TODO: Proxy routes to be implemented with httputil.ReverseProxy

	// 서버 시작
	addr := fmt.Sprintf(":%s", gatewayPort)
	log.Printf("Gateway listening on %s", addr)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

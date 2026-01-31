package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
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

	// 프록시 대상 URL 파싱
	goTarget, err := url.Parse(goAPIURL)
	if err != nil {
		log.Fatalf("Invalid GO_API_URL: %v", err)
	}
	laravelTarget, err := url.Parse(laravelBackendURL)
	if err != nil {
		log.Fatalf("Invalid LARAVEL_BACKEND_URL: %v", err)
	}

	// 리버스 프록시 생성
	goProxy := httputil.NewSingleHostReverseProxy(goTarget)
	laravelProxy := httputil.NewSingleHostReverseProxy(laravelTarget)

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
		c.String(http.StatusOK, "Gateway OK")
	})

	// API v2 → Go Backend (신규)
	router.Any("/api/v2/*path", func(c *gin.Context) {
		log.Printf("Proxying to Go API: %s %s", c.Request.Method, c.Request.URL.Path)
		goProxy.ServeHTTP(c.Writer, c.Request)
	})

	// API plugins → Go Backend
	router.Any("/api/plugins/*path", func(c *gin.Context) {
		log.Printf("Proxying to Go API: %s %s", c.Request.Method, c.Request.URL.Path)
		goProxy.ServeHTTP(c.Writer, c.Request)
	})

	// API v1 → Laravel Backend (기존)
	router.Any("/api/v1/*path", func(c *gin.Context) {
		log.Printf("Proxying to Laravel: %s %s", c.Request.Method, c.Request.URL.Path)
		laravelProxy.ServeHTTP(c.Writer, c.Request)
	})

	// 그 외 모든 요청 → Laravel Backend
	router.NoRoute(func(c *gin.Context) {
		log.Printf("Proxying to Laravel (default): %s %s", c.Request.Method, c.Request.URL.Path)
		laravelProxy.ServeHTTP(c.Writer, c.Request)
	})

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

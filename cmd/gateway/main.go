package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/proxy"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	// 환경 변수
	laravelBackendURL := getEnv("LARAVEL_BACKEND_URL", "http://localhost:80")
	goAPIURL := getEnv("GO_API_URL", "http://localhost:8081")
	gatewayPort := getEnv("GATEWAY_PORT", "8080")

	log.Printf("Starting API Gateway on port %s", gatewayPort)
	log.Printf("Laravel Backend: %s", laravelBackendURL)
	log.Printf("Go API Backend: %s", goAPIURL)

	// Fiber 앱 생성
	app := fiber.New(fiber.Config{
		Prefork:      false,
		ServerHeader: "Angple Gateway",
		AppName:      "Angple API Gateway v1.0.0",
	})

	// 미들웨어
	app.Use(recover.New())
	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${method} ${path} (${latency})\n",
	}))

	// Health Check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.SendString("Gateway OK")
	})

	// API v2 → Go Backend (신규)
	app.All("/api/v2/*", func(c *fiber.Ctx) error {
		url := goAPIURL + c.Path()
		log.Printf("Proxying to Go API: %s", url)

		if err := proxy.Do(c, url); err != nil {
			return c.Status(502).JSON(fiber.Map{
				"error": "Failed to proxy request to Go API",
			})
		}

		// 응답 헤더 수정
		c.Response().Header.Set("X-Powered-By", "Angple-Go")
		return nil
	})

	// API v1 → Laravel Backend (기존)
	app.All("/api/v1/*", func(c *fiber.Ctx) error {
		url := laravelBackendURL + c.Path()
		log.Printf("Proxying to Laravel: %s", url)

		if err := proxy.Do(c, url); err != nil {
			return c.Status(502).JSON(fiber.Map{
				"error": "Failed to proxy request to Laravel",
			})
		}

		return nil
	})

	// 기본 라우트 (Laravel로 전달)
	app.All("/*", func(c *fiber.Ctx) error {
		url := laravelBackendURL + c.Path()
		return proxy.Do(c, url)
	})

	// 서버 시작
	addr := fmt.Sprintf(":%s", gatewayPort)
	if err := app.Listen(addr); err != nil {
		log.Fatalf("Failed to start gateway: %v", err)
	}
}

// getEnv 환경 변수 조회 (기본값 지원)
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

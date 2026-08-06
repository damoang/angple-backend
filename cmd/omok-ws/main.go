// Command omok-ws 는 실시간 오목 대전 WebSocket 서버다.
//
// 포트 8084 에서 /omok-ws/ 를 서빙한다(호스트 nginx 가 이미 이 경로를 프록시한다).
// API 서버와 같은 저장소·같은 DB 를 쓰되 프로세스는 분리한다 — 장시간 유지되는
// WebSocket 연결이 API 파드의 롤아웃에 끌려다니지 않게 하기 위해서다.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/damoang/angple-backend/internal/omok"
	"github.com/damoang/angple-backend/pkg/jwt"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	port := envOr("OMOK_PORT", "8084")
	path := envOr("OMOK_PATH", "/omok-ws/")

	db, err := openDB()
	if err != nil {
		log.Fatalf("[omok] DB 연결 실패: %v", err)
	}
	store := omok.NewStore(db)

	// 재시작 시 남아 있던 진행 대국 정리 — 게임 상태는 메모리에만 있어 이어갈 수
	// 없다. 참가비만 돌려주고 대국은 aborted 로 닫는다.
	if n, aerr := store.AbortStalePlayingGames(); aerr != nil {
		log.Printf("[omok] 중단 대국 정리 실패: %v", aerr)
	} else if n > 0 {
		log.Printf("[omok] 중단 대국 정리 — 참가비 %d건 환불", n)
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("[omok] JWT_SECRET 이 없습니다 — 익명 대국은 허용하지 않습니다")
	}
	jwtManager := jwt.NewManager(secret, 900, 604800)
	if next := os.Getenv("JWT_SECRET_NEXT"); next != "" {
		jwtManager.SetNextKey(next) // 키 롤인 중에도 기존 토큰을 받아준다
	}

	verify := func(token string) (string, string, error) {
		claims, verr := jwtManager.VerifyToken(token)
		if verr != nil {
			return "", "", verr
		}
		return claims.UserID, claims.Nickname, nil
	}

	server := omok.NewServer(store, verify)
	go server.StartHeartbeat()
	go server.StartStatusMonitor()

	mux := http.NewServeMux()
	mux.HandleFunc(path, server.HandleWebSocket)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]interface{}{
			"status": "ok", "service": "omok-ws", "snapshot": server.Snapshot(),
		})
		_, _ = w.Write(body)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("[omok] listening on :%s%s", port, path)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[omok] ListenAndServe: %v", err)
	}
}

func openDB() (*gorm.DB, error) {
	cfg := mysqldriver.NewConfig()
	cfg.User = os.Getenv("DB_USER")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = os.Getenv("DB_HOST") + ":" + envOr("DB_PORT", "3306")
	cfg.DBName = os.Getenv("DB_NAME")
	cfg.ParseTime = true
	cfg.InterpolateParams = true
	cfg.Params = map[string]string{"charset": "utf8mb4", "time_zone": "'+09:00'"}

	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	maxOpen, _ := strconv.Atoi(envOr("OMOK_DB_MAX_OPEN", "10"))
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(2)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

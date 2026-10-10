// Command stack-ws 는 앙쌓기 온라인 대전·점수판 서버다.
//
// 기본 포트 8086 에서 /stack-ws/ 를 서빙한다(웹소켓 + 로비·점수판·혼자하기 기록 REST).
// API 서버와 같은 저장소·같은 DB 를 쓰되 프로세스는 분리한다. 장시간 유지되는 웹소켓 연결이
// API 파드의 롤아웃에 끌려다니지 않게 하기 위해서다(오목·장기와 같은 구성).
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	stacksrv "github.com/damoang/angple-backend/internal/stack_srv"
	"github.com/damoang/angple-backend/pkg/jwt"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	port := envOr("STACK_PORT", "8086")
	path := envOr("STACK_PATH", "/stack-ws/")

	db, err := openDBWithRetry()
	if err != nil {
		log.Fatalf("[stack] DB 연결 실패(재시도 소진): %v", err)
	}
	store := stacksrv.NewStore(db)

	// 재시작 시 남아 있던 진행 대전 정리 — 대전 상태는 메모리에만 있어 이어갈 수 없다.
	// 참가비만 돌려주고 대전은 aborted 로 닫는다.
	if n, aerr := store.AbortStalePlayingGames(); aerr != nil {
		log.Printf("[stack] 중단 대전 정리 실패: %v", aerr)
	} else if n > 0 {
		log.Printf("[stack] 중단 대전 정리 — 참가비 %d건 환불", n)
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("[stack] JWT_SECRET 이 없습니다 — 익명 대전은 허용하지 않습니다")
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

	server := stacksrv.NewServer(store, verify, stacksrv.DefaultTiming())
	go server.StartHeartbeat()
	go server.StartStatusMonitor()
	solo := stacksrv.NewSoloService(store, verify)
	board := stacksrv.NewLeaderboard(store)

	mux := http.NewServeMux()
	mux.HandleFunc(path, server.HandleWebSocket)
	// 공개 GET 두 개(로비·점수판)는 인증 불요, 숫자·닉네임만 내보낸다. 60초 메모리 캐시 + max-age=60.
	mux.HandleFunc(path+"lobby", server.HandleLobby)
	mux.Handle(path+"leaderboard", board)
	// 혼자하기 기록 — 로그인 회원만(Authorization: Bearer). 실패하면 웹은 로컬 모드로 둔다.
	mux.HandleFunc(path+"solo/start", solo.HandleStart)
	mux.HandleFunc(path+"solo/finish", solo.HandleFinish)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]interface{}{
			"status": "ok", "service": "stack-ws", "snapshot": server.Snapshot(),
		})
		_, _ = w.Write(body)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("[stack] listening on :%s%s", port, path)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[stack] ListenAndServe: %v", err)
	}
}

// openDBWithRetry 는 기동 시 DB 연결을 지수 백오프로 재시도한다.
// 이름 해석 순간 장애 한 번에 프로세스가 죽으면 진행 중 대전이 전부 aborted 되고 참가비가 환불된다.
// 그렇다고 무한 대기는 하지 않는다. 설정이 틀렸거나 DB 가 정말 죽었으면 빨리 죽어
// CrashLoopBackOff 로 드러나는 편이 낫다(장기 서버와 같은 정책).
func openDBWithRetry() (*gorm.DB, error) {
	const attempts = 6 // 1+2+4+8+16+32 ≈ 63초
	var lastErr error
	for i := 0; i < attempts; i++ {
		db, err := openDB()
		if err == nil {
			if i > 0 {
				log.Printf("[stack] DB 연결 성공 (재시도 %d회)", i)
			}
			return db, nil
		}
		lastErr = err
		if i == attempts-1 {
			break
		}
		wait := time.Duration(1<<uint(i)) * time.Second
		log.Printf("[stack] DB 연결 실패 — %v 후 재시도 (%d/%d): %v", wait, i+1, attempts-1, err)
		time.Sleep(wait)
	}
	return nil, lastErr
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
	maxOpen, _ := strconv.Atoi(envOr("STACK_DB_MAX_OPEN", "10"))
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

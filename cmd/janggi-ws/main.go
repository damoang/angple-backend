// Command omok-ws 는 실시간 오목 대전 WebSocket 서버다.
//
// 포트 8085 에서 /janggi-ws/ 를 서빙한다(호스트 nginx 가 이미 이 경로를 프록시한다).
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

	janggisrv "github.com/damoang/angple-backend/internal/janggi_srv"
	"github.com/damoang/angple-backend/pkg/jwt"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	port := envOr("JANGGI_PORT", "8085")
	path := envOr("JANGGI_PATH", "/janggi-ws/")

	db, err := openDBWithRetry()
	if err != nil {
		log.Fatalf("[janggi] DB 연결 실패(재시도 소진): %v", err)
	}
	store := janggisrv.NewStore(db)

	// 재시작 시 남아 있던 진행 대국 정리 — 게임 상태는 메모리에만 있어 이어갈 수
	// 없다. 참가비만 돌려주고 대국은 aborted 로 닫는다.
	if n, aerr := store.AbortStalePlayingGames(); aerr != nil {
		log.Printf("[janggi] 중단 대국 정리 실패: %v", aerr)
	} else if n > 0 {
		log.Printf("[janggi] 중단 대국 정리 — 참가비 %d건 환불", n)
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("[janggi] JWT_SECRET 이 없습니다 — 익명 대국은 허용하지 않습니다")
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

	server := janggisrv.NewServer(store, verify)
	go server.StartHeartbeat()
	go server.StartStatusMonitor()

	mux := http.NewServeMux()
	mux.HandleFunc(path, server.HandleWebSocket)
	// 대기 현황 (인증 불요·공개). 온라인 대전 탭이 폴링해 「지금 N명 대기 중」을 띄운다.
	// 유동성이 낮을 때는 "누가 기다리고 있다"는 사실 자체가 매칭 확률을 만든다 —
	// 두 번째 사람이 지금 누르면 바로 붙는다는 것을 알아야 누른다. (8/7 첫 대국까지
	// 전원이 각자 혼자 대기하다 이탈한 원인이 이 정보 부재였다.)
	// 개인정보 없음: 숫자만 내보낸다(대기자 닉네임 노출 금지).
	mux.HandleFunc(path+"lobby", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=5")
		body, _ := json.Marshal(server.Snapshot())
		_, _ = w.Write(body)
	})
	// 공개 랭킹 (인증 불요). 레이팅 상위 20 — 닉네임·승·패·무·레이팅만 담긴다.
	// DB 부하 방지: 60초 공유 캐시면 충분하다(전적은 대국 종료 때만 변한다).
	mux.HandleFunc(path+"ranking", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=60")
		body, _ := json.Marshal(map[string]interface{}{"ranking": store.RankingTop(20)})
		_, _ = w.Write(body)
	})
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]interface{}{
			"status": "ok", "service": "janggi-ws", "snapshot": server.Snapshot(),
		})
		_, _ = w.Write(body)
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("[janggi] listening on :%s%s", port, path)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("[janggi] ListenAndServe: %v", err)
	}
}

// openDBWithRetry 는 기동 시 DB 연결을 지수 백오프로 재시도한다.
//
// ⛔ 왜 필요한가 — 예전에는 openDB 가 한 번 실패하면 곧바로 log.Fatalf 로 죽었다.
//
//	2026-08-20 실측: CoreDNS 순간 장애로 이름 해석이 한 번 실패해 프로세스가 종료됐고,
//	8시간 동안 4번 재시작됐다.
//
//	  [janggi] DB 연결 실패: dial tcp: lookup ...rds.amazonaws.com on 10.43.0.10:53:
//	          read udp ...: connection refused
//
//	재시작 비용이 크다. 이 파일 위쪽 주석대로 **게임 상태는 메모리에만 있어 이어갈 수 없고**,
//	재시작할 때마다 진행 중 대국이 전부 aborted 되고 참가비가 환불된다.
//	프로세스를 API 와 분리한 목적(롤아웃에 끌려다니지 않기)도 함께 무력화된다.
//
// ⛔ 무한 대기는 하지 않는다. 설정이 틀렸거나 DB 가 정말 죽었으면 빨리 죽어서
//
//	k8s 가 CrashLoopBackOff 로 드러내는 편이 낫다. 상한을 두고 소진되면 종료한다.
func openDBWithRetry() (*gorm.DB, error) {
	const attempts = 6 // 1+2+4+8+16+32 ≈ 63초. 순간 장애는 이 안에서 풀린다
	var lastErr error
	for i := 0; i < attempts; i++ {
		db, err := openDB()
		if err == nil {
			if i > 0 {
				log.Printf("[janggi] DB 연결 성공 (재시도 %d회)", i)
			}
			return db, nil
		}
		lastErr = err
		if i == attempts-1 {
			break
		}
		wait := time.Duration(1<<uint(i)) * time.Second
		log.Printf("[janggi] DB 연결 실패 — %v 후 재시도 (%d/%d): %v", wait, i+1, attempts-1, err)
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
	maxOpen, _ := strconv.Atoi(envOr("JANGGI_DB_MAX_OPEN", "10"))
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

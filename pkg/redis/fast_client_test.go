package redis

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestNewFastClient_NilBase(t *testing.T) {
	if c := NewFastClient(nil, 100*time.Millisecond, 100*time.Millisecond); c != nil {
		t.Fatalf("base 가 nil 이면 nil 이어야 한다: %v", c)
	}
}

func TestNewFastClient_Options(t *testing.T) {
	// 연결은 첫 명령 때 맺으므로 여기서는 서버가 없어도 된다.
	base := redis.NewClient(&redis.Options{
		Addr:         "127.0.0.1:6399",
		Password:     "pw",
		DB:           3,
		PoolSize:     50,
		MinIdleConns: 10,
		ReadTimeout:  5 * time.Second,
	})
	t.Cleanup(func() { _ = base.Close() })

	fast := NewFastClient(base, 200*time.Millisecond, 300*time.Millisecond)
	if fast == nil {
		t.Fatal("클라이언트가 만들어지지 않았다")
	}
	t.Cleanup(func() { _ = fast.Close() })

	got := fast.Options()
	if got.Addr != "127.0.0.1:6399" || got.Password != "pw" || got.DB != 3 {
		t.Fatalf("서버·인증·DB 는 base 와 같아야 한다: addr=%s db=%d", got.Addr, got.DB)
	}
	if !got.ContextTimeoutEnabled {
		t.Fatal("ContextTimeoutEnabled 가 켜져야 한다")
	}
	if got.ReadTimeout != 200*time.Millisecond || got.WriteTimeout != 300*time.Millisecond {
		t.Fatalf("소켓 상한이 다르다: read=%v write=%v", got.ReadTimeout, got.WriteTimeout)
	}
	if got.DialTimeout != 300*time.Millisecond || got.PoolTimeout != 300*time.Millisecond {
		t.Fatalf("연결·풀 대기 상한이 다르다: dial=%v pool=%v", got.DialTimeout, got.PoolTimeout)
	}
	if got.MaxRetries != 0 {
		t.Fatalf("재시도가 없어야 한다: %d", got.MaxRetries)
	}
	if got.PoolSize != FastClientPoolSize || got.MaxIdleConns != FastClientPoolSize || got.MinIdleConns != 0 {
		t.Fatalf("풀 크기가 다르다: pool=%d maxIdle=%d minIdle=%d", got.PoolSize, got.MaxIdleConns, got.MinIdleConns)
	}

	// base 의 설정은 바뀌지 않는다.
	b := base.Options()
	if b.ContextTimeoutEnabled || b.PoolSize != 50 || b.MinIdleConns != 10 || b.ReadTimeout != 5*time.Second {
		t.Fatalf("base 설정이 바뀌었다: ctxTimeout=%v pool=%d minIdle=%d read=%v",
			b.ContextTimeoutEnabled, b.PoolSize, b.MinIdleConns, b.ReadTimeout)
	}
}

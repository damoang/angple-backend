package redis

import (
	"time"

	"github.com/redis/go-redis/v9"
)

// FastClientPoolSize 는 NewFastClient 가 만드는 클라이언트의 연결 풀 크기다.
const FastClientPoolSize = 4

// NewFastClient 는 base 와 같은 서버·DB·인증으로 붙는, 짧은 조회 전용 클라이언트를 만든다.
// base 가 nil 이면 nil 을 돌려준다.
//
// go-redis v9 는 ContextTimeoutEnabled 가 기본 false 라 호출 컨텍스트의 마감이 소켓 대기에
// 적용되지 않는다(Redis 가 멈추면 ReadTimeout 까지 기다린다). 전역 클라이언트 동작은 그대로 두고
// 이 클라이언트에서만 다음을 바꾼다.
//   - ContextTimeoutEnabled = true: 호출 컨텍스트 마감을 소켓 읽기·쓰기에 적용한다.
//   - ReadTimeout = readTimeout, WriteTimeout = writeTimeout: 컨텍스트가 없어도 소켓 대기 상한.
//   - DialTimeout·PoolTimeout = max(readTimeout, writeTimeout): 연결·풀 대기도 같은 상한.
//   - MaxRetries = -1(재시도 없음): 재시도로 대기가 몇 배가 되지 않게.
//   - PoolSize = MaxIdleConns = FastClientPoolSize, MinIdleConns = 0: 작은 풀.
//
// 호출하는 쪽은 실패를 「값 없음」으로 다루고 대체 경로를 가져야 한다. 다 쓰면 Close 한다.
func NewFastClient(base *redis.Client, readTimeout, writeTimeout time.Duration) *redis.Client {
	if base == nil {
		return nil
	}
	own := *base.Options()
	own.ContextTimeoutEnabled = true
	own.ReadTimeout = readTimeout
	own.WriteTimeout = writeTimeout
	waitTimeout := max(readTimeout, writeTimeout)
	own.DialTimeout = waitTimeout
	own.PoolTimeout = waitTimeout
	own.MaxRetries = -1
	own.PoolSize = FastClientPoolSize
	own.MaxIdleConns = FastClientPoolSize
	own.MinIdleConns = 0
	return redis.NewClient(&own)
}

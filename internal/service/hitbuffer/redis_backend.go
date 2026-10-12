package hitbuffer

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// 버퍼에 값이 있을 수 있는 게시판 ID 집합.
	// hits:{bo_table} 과 겹치지 않도록 접두어를 따로 쓴다(게시판 ID 가 "tables" 여도 안전).
	tablesKey = "hitbuf:tables"

	// 처리 중 키 집합(파드가 죽어 남은 키를 다음 flush 가 찾는 곳).
	pendingKey = "hitbuf:pending"

	// flush 리더 락.
	lockKey = "hitbuf:lock"
)

// claimScript 는 src 를 dst 로 RENAME 하고 처리 중 집합에 dst 를 올린다(원자적).
// src 가 없으면 0 을 돌려주고 목록에서 정리한다. 게시판 목록 정리는 Incr 의
// MULTI(HINCRBY+SADD)와 원자적으로 직렬화되므로, 방금 쌓인 값의 게시판을 빼는 일은 없다.
//
// KEYS[1]=src KEYS[2]=dst KEYS[3]=pending KEYS[4]=tables
// ARGV[1]=boardID ARGV[2]="1" 이면 src 가 처리 중 키
var claimScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then
	if ARGV[2] == '1' then
		redis.call('SREM', KEYS[3], KEYS[1])
	else
		redis.call('SREM', KEYS[4], ARGV[1])
	end
	return 0
end
redis.call('RENAME', KEYS[1], KEYS[2])
redis.call('SADD', KEYS[3], KEYS[2])
if ARGV[2] == '1' then
	redis.call('SREM', KEYS[3], KEYS[1])
end
return 1
`)

// restoreScript 는 처리 중 키에 남은 값을 살아있는 해시로 더해 되돌리고 처리 중 키를 지운다(원자적).
//
// KEYS[1]=flushing KEYS[2]=live KEYS[3]=pending KEYS[4]=tables
// ARGV[1]=boardID
var restoreScript = redis.NewScript(`
local kv = redis.call('HGETALL', KEYS[1])
for i = 1, #kv, 2 do
	redis.call('HINCRBY', KEYS[2], kv[i], kv[i + 1])
end
if #kv > 0 then
	redis.call('SADD', KEYS[4], ARGV[1])
end
redis.call('DEL', KEYS[1])
redis.call('SREM', KEYS[3], KEYS[1])
return #kv / 2
`)

var _ Backend = (*RedisBackend)(nil)

// RedisBackend 는 go-redis 클라이언트로 구현한 Backend 다.
type RedisBackend struct {
	client redis.UniversalClient
}

// NewRedisBackend 는 RedisBackend 를 만든다.
func NewRedisBackend(client redis.UniversalClient) *RedisBackend {
	return &RedisBackend{client: client}
}

// Incr 는 hits:{boardID} 의 wrID 필드를 1 올리고 게시판을 목록에 등록한다(MULTI).
func (r *RedisBackend) Incr(ctx context.Context, boardID string, wrID int) error {
	_, err := r.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.HIncrBy(ctx, LiveKey(boardID), strconv.Itoa(wrID), 1)
		p.SAdd(ctx, tablesKey, boardID)
		return nil
	})
	return err
}

// AcquireLock 은 SET NX EX 로 flush 리더 락을 잡는다.
func (r *RedisBackend) AcquireLock(ctx context.Context, token string, ttl time.Duration) (bool, error) {
	return r.client.SetNX(ctx, lockKey, token, ttl).Result()
}

// Tables 는 버퍼에 값이 있을 수 있는 게시판 ID 목록을 돌려준다.
func (r *RedisBackend) Tables(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, tablesKey).Result()
}

// RemoveTable 은 게시판 ID 를 목록에서 뺀다.
func (r *RedisBackend) RemoveTable(ctx context.Context, boardID string) error {
	return r.client.SRem(ctx, tablesKey, boardID).Err()
}

// Pending 은 처리 중 키 목록을 돌려준다.
func (r *RedisBackend) Pending(ctx context.Context) ([]string, error) {
	return r.client.SMembers(ctx, pendingKey).Result()
}

// Claim 은 src 를 처리 중 키 dst 로 원자적으로 옮긴다.
func (r *RedisBackend) Claim(ctx context.Context, boardID, src, dst string, srcPending bool) (bool, error) {
	flag := "0"
	if srcPending {
		flag = "1"
	}
	n, err := claimScript.Run(ctx, r.client, []string{src, dst, pendingKey, tablesKey}, boardID, flag).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// Read 는 처리 중 키의 필드와 값을 모두 읽는다.
func (r *RedisBackend) Read(ctx context.Context, key string) (map[string]string, error) {
	return r.client.HGetAll(ctx, key).Result()
}

// Ack 는 DB 반영이 끝난 필드를 처리 중 키에서 지운다.
func (r *RedisBackend) Ack(ctx context.Context, key string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	return r.client.HDel(ctx, key, fields...).Err()
}

// Restore 는 처리 중 키에 남은 값을 hits:{boardID} 로 되돌린다.
func (r *RedisBackend) Restore(ctx context.Context, boardID, key string) error {
	return restoreScript.Run(ctx, r.client, []string{key, LiveKey(boardID), pendingKey, tablesKey}, boardID).Err()
}

// Done 은 처리가 끝난 처리 중 키를 지우고 목록에서 뺀다.
func (r *RedisBackend) Done(ctx context.Context, key string) error {
	_, err := r.client.TxPipelined(ctx, func(p redis.Pipeliner) error {
		p.Del(ctx, key)
		p.SRem(ctx, pendingKey, key)
		return nil
	})
	return err
}

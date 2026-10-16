package handler

import (
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/activitycache"
)

// 활동 응답 캐시는 activitycache 의 TTL·키 규칙을 그대로 써야 무효화와 어긋나지 않는다.
func TestMemberActivityCacheUsesSharedTTLAndKey(t *testing.T) {
	if activityCacheTTL != 5*time.Minute {
		t.Fatalf("activityCacheTTL = %v, want 5m", activityCacheTTL)
	}
	if activityCacheTTL != activitycache.TTL {
		t.Fatal("handler TTL must come from activitycache.TTL")
	}
	if got := activitycache.Key("user1", 5); got != "member_activity:user1:5" {
		t.Fatalf("key = %q", got)
	}
}

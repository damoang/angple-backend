package middleware

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// DeprecationConfig configures the deprecation middleware
type DeprecationConfig struct {
	// SunsetDate is the date when the API will be removed (RFC 1123 format)
	SunsetDate string
	// Link to migration guide
	MigrationGuideURL string
}

// Deprecation adds standard deprecation headers to responses.
// See: https://datatracker.ietf.org/doc/html/draft-ietf-httpapi-deprecation-header
func Deprecation(cfg DeprecationConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Deprecation", "true")
		if cfg.SunsetDate != "" {
			c.Header("Sunset", cfg.SunsetDate)
		}
		if cfg.MigrationGuideURL != "" {
			c.Header("Link", fmt.Sprintf(`<%s>; rel="deprecation"`, cfg.MigrationGuideURL))
		}
		c.Next()
	}
}

// APIUsageTracker tracks per-endpoint call counts for monitoring deprecated API usage
type APIUsageTracker struct {
	mu       sync.RWMutex
	counters map[string]*uint64 // endpoint -> count
	started  time.Time
}

// NewAPIUsageTracker creates a new tracker
func NewAPIUsageTracker() *APIUsageTracker {
	return &APIUsageTracker{
		counters: make(map[string]*uint64),
		started:  time.Now(),
	}
}

// Track returns a middleware that increments the counter for each request
func (t *APIUsageTracker) Track() gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.Request.Method + " " + c.FullPath()
		counter := t.getOrCreate(key)
		atomic.AddUint64(counter, 1)
		c.Next()
	}
}

func (t *APIUsageTracker) getOrCreate(key string) *uint64 {
	t.mu.RLock()
	if counter, ok := t.counters[key]; ok {
		t.mu.RUnlock()
		return counter
	}
	t.mu.RUnlock()

	t.mu.Lock()
	defer t.mu.Unlock()
	if counter, ok := t.counters[key]; ok {
		return counter
	}
	var counter uint64
	t.counters[key] = &counter
	return &counter
}

// EndpointUsage represents usage stats for one endpoint
type EndpointUsage struct {
	Endpoint string `json:"endpoint"`
	Count    uint64 `json:"count"`
}

// GetStats returns all tracked endpoint usage
func (t *APIUsageTracker) GetStats() []EndpointUsage {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make([]EndpointUsage, 0, len(t.counters))
	for endpoint, counter := range t.counters {
		count := atomic.LoadUint64(counter)
		if count > 0 {
			stats = append(stats, EndpointUsage{
				Endpoint: endpoint,
				Count:    count,
			})
		}
	}
	return stats
}

// TotalCalls returns the sum of all tracked calls
func (t *APIUsageTracker) TotalCalls() uint64 {
	t.mu.RLock()
	defer t.mu.RUnlock()

	var total uint64
	for _, counter := range t.counters {
		total += atomic.LoadUint64(counter)
	}
	return total
}

// StartedAt returns when tracking started
func (t *APIUsageTracker) StartedAt() time.Time {
	return t.started
}

// Reset clears all counters
func (t *APIUsageTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.counters = make(map[string]*uint64)
	t.started = time.Now()
}

package plugin

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics 플러그인별 API 메트릭 수집기
type Metrics struct {
	plugins map[string]*pluginMetric
	mu      sync.RWMutex
}

type pluginMetric struct {
	TotalRequests  int64
	TotalErrors    int64
	TotalLatencyMs int64
	StatusCodes    map[int]int64
	Endpoints      map[string]*endpointMetric
}

type endpointMetric struct {
	Requests   int64
	Errors     int64
	LatencyMs  int64
	MinLatency int64
	MaxLatency int64
}

// MetricsSummary API 응답용 메트릭 요약
type MetricsSummary struct {
	PluginName    string                      `json:"plugin_name"`
	TotalRequests int64                       `json:"total_requests"`
	TotalErrors   int64                       `json:"total_errors"`
	ErrorRate     float64                     `json:"error_rate"`
	AvgLatencyMs  float64                     `json:"avg_latency_ms"`
	StatusCodes   map[int]int64               `json:"status_codes"`
	Endpoints     map[string]*EndpointSummary `json:"endpoints"`
}

// EndpointSummary 엔드포인트별 요약
type EndpointSummary struct {
	Requests     int64   `json:"requests"`
	Errors       int64   `json:"errors"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	MinLatencyMs int64   `json:"min_latency_ms"`
	MaxLatencyMs int64   `json:"max_latency_ms"`
}

// NewMetrics 생성자
func NewMetrics() *Metrics {
	return &Metrics{
		plugins: make(map[string]*pluginMetric),
	}
}

// Middleware Gin 미들웨어 - 플러그인 API 호출 메트릭 수집
func (pm *Metrics) Middleware(pluginName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start).Milliseconds()
		status := c.Writer.Status()
		endpoint := c.Request.Method + " " + c.FullPath()

		pm.record(pluginName, endpoint, status, latency)
	}
}

// record 메트릭 기록
func (pm *Metrics) record(pluginName, endpoint string, status int, latencyMs int64) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	m, exists := pm.plugins[pluginName]
	if !exists {
		m = &pluginMetric{
			StatusCodes: make(map[int]int64),
			Endpoints:   make(map[string]*endpointMetric),
		}
		pm.plugins[pluginName] = m
	}

	m.TotalRequests++
	m.TotalLatencyMs += latencyMs
	m.StatusCodes[status]++

	if status >= http.StatusBadRequest {
		m.TotalErrors++
	}

	ep, exists := m.Endpoints[endpoint]
	if !exists {
		ep = &endpointMetric{
			MinLatency: latencyMs,
			MaxLatency: latencyMs,
		}
		m.Endpoints[endpoint] = ep
	}

	ep.Requests++
	ep.LatencyMs += latencyMs
	if status >= http.StatusBadRequest {
		ep.Errors++
	}
	if latencyMs < ep.MinLatency {
		ep.MinLatency = latencyMs
	}
	if latencyMs > ep.MaxLatency {
		ep.MaxLatency = latencyMs
	}
}

// GetSummary 특정 플러그인 메트릭 요약
func (pm *Metrics) GetSummary(pluginName string) *MetricsSummary {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	m, exists := pm.plugins[pluginName]
	if !exists {
		return &MetricsSummary{
			PluginName:  pluginName,
			StatusCodes: make(map[int]int64),
			Endpoints:   make(map[string]*EndpointSummary),
		}
	}

	return pm.buildSummary(pluginName, m)
}

// GetAllSummaries 전체 플러그인 메트릭 요약
func (pm *Metrics) GetAllSummaries() []MetricsSummary {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	result := make([]MetricsSummary, 0, len(pm.plugins))
	for name, m := range pm.plugins {
		result = append(result, *pm.buildSummary(name, m))
	}
	return result
}

func (pm *Metrics) buildSummary(name string, m *pluginMetric) *MetricsSummary {
	var avgLatency float64
	var errorRate float64
	if m.TotalRequests > 0 {
		avgLatency = float64(m.TotalLatencyMs) / float64(m.TotalRequests)
		errorRate = float64(m.TotalErrors) / float64(m.TotalRequests)
	}

	codes := make(map[int]int64, len(m.StatusCodes))
	for k, v := range m.StatusCodes {
		codes[k] = v
	}

	endpoints := make(map[string]*EndpointSummary, len(m.Endpoints))
	for ep, em := range m.Endpoints {
		var epAvg float64
		if em.Requests > 0 {
			epAvg = float64(em.LatencyMs) / float64(em.Requests)
		}
		endpoints[ep] = &EndpointSummary{
			Requests:     em.Requests,
			Errors:       em.Errors,
			AvgLatencyMs: epAvg,
			MinLatencyMs: em.MinLatency,
			MaxLatencyMs: em.MaxLatency,
		}
	}

	return &MetricsSummary{
		PluginName:    name,
		TotalRequests: m.TotalRequests,
		TotalErrors:   m.TotalErrors,
		ErrorRate:     errorRate,
		AvgLatencyMs:  avgLatency,
		StatusCodes:   codes,
		Endpoints:     endpoints,
	}
}

// Reset 특정 플러그인 메트릭 초기화
func (pm *Metrics) Reset(pluginName string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.plugins, pluginName)
}

// ResetAll 전체 메트릭 초기화
func (pm *Metrics) ResetAll() {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.plugins = make(map[string]*pluginMetric)
}

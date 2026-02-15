package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestMetrics_Record(t *testing.T) {
	pm := NewMetrics()
	pm.record("test-plugin", "GET /api/test", 200, 50)
	pm.record("test-plugin", "GET /api/test", 200, 30)
	pm.record("test-plugin", "GET /api/test", 500, 100)

	summary := pm.GetSummary("test-plugin")
	if summary.TotalRequests != 3 {
		t.Fatalf("expected 3 requests, got %d", summary.TotalRequests)
	}
	if summary.TotalErrors != 1 {
		t.Fatalf("expected 1 error, got %d", summary.TotalErrors)
	}
	if summary.AvgLatencyMs != 60 {
		t.Fatalf("expected avg 60ms, got %.2f", summary.AvgLatencyMs)
	}
}

func TestMetrics_Middleware(t *testing.T) {
	pm := NewMetrics()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/test", pm.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/api/error", pm.Middleware("test-plugin"), func(c *gin.Context) {
		c.String(http.StatusInternalServerError, "fail")
	})

	// 2 success + 1 error
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		req, _ := http.NewRequestWithContext(context.Background(), "GET", "/api/test", nil)
		r.ServeHTTP(w, req)
	}
	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/api/error", nil)
	r.ServeHTTP(w, req)

	summary := pm.GetSummary("test-plugin")
	if summary.TotalRequests != 3 {
		t.Fatalf("expected 3 total, got %d", summary.TotalRequests)
	}
	if summary.TotalErrors != 1 {
		t.Fatalf("expected 1 error, got %d", summary.TotalErrors)
	}
	if len(summary.Endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(summary.Endpoints))
	}
}

func TestMetrics_ErrorRate(t *testing.T) {
	pm := NewMetrics()
	pm.record("p", "GET /", 200, 10)
	pm.record("p", "GET /", 200, 10)
	pm.record("p", "GET /", 400, 10)
	pm.record("p", "GET /", 500, 10)

	summary := pm.GetSummary("p")
	if summary.ErrorRate != 0.5 {
		t.Fatalf("expected error rate 0.5, got %.2f", summary.ErrorRate)
	}
}

func TestMetrics_EmptyPlugin(t *testing.T) {
	pm := NewMetrics()
	summary := pm.GetSummary("nonexistent")
	if summary.TotalRequests != 0 {
		t.Fatal("expected 0 requests for nonexistent plugin")
	}
}

func TestMetrics_GetAllSummaries(t *testing.T) {
	pm := NewMetrics()
	pm.record("a", "GET /", 200, 10)
	pm.record("b", "GET /", 200, 20)

	summaries := pm.GetAllSummaries()
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
}

func TestMetrics_Reset(t *testing.T) {
	pm := NewMetrics()
	pm.record("test", "GET /", 200, 10)

	pm.Reset("test")
	summary := pm.GetSummary("test")
	if summary.TotalRequests != 0 {
		t.Fatal("expected 0 after reset")
	}
}

func TestMetrics_EndpointMinMax(t *testing.T) {
	pm := NewMetrics()
	pm.record("p", "GET /api", 200, 10)
	pm.record("p", "GET /api", 200, 50)
	pm.record("p", "GET /api", 200, 30)

	summary := pm.GetSummary("p")
	ep := summary.Endpoints["GET /api"]
	if ep == nil {
		t.Fatal("expected endpoint summary")
	}
	if ep.MinLatencyMs != 10 {
		t.Fatalf("expected min 10, got %d", ep.MinLatencyMs)
	}
	if ep.MaxLatencyMs != 50 {
		t.Fatalf("expected max 50, got %d", ep.MaxLatencyMs)
	}
}

func TestMetrics_StatusCodes(t *testing.T) {
	pm := NewMetrics()
	pm.record("p", "GET /", 200, 10)
	pm.record("p", "GET /", 200, 10)
	pm.record("p", "GET /", 404, 10)

	summary := pm.GetSummary("p")
	if summary.StatusCodes[200] != 2 {
		t.Fatalf("expected 2 x 200, got %d", summary.StatusCodes[200])
	}
	if summary.StatusCodes[404] != 1 {
		t.Fatalf("expected 1 x 404, got %d", summary.StatusCodes[404])
	}
}

func TestMetrics_LatencyTracking(t *testing.T) {
	pm := NewMetrics()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/slow", pm.Middleware("slow-plugin"), func(c *gin.Context) {
		time.Sleep(10 * time.Millisecond)
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "/slow", nil)
	r.ServeHTTP(w, req)

	summary := pm.GetSummary("slow-plugin")
	if summary.AvgLatencyMs < 10 {
		t.Fatalf("expected latency >= 10ms, got %.2f", summary.AvgLatencyMs)
	}
}

package cron

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// SetGivingSweep injects the giving due-draw sweep (wired in main.go from the
// giving handler — cron 패키지가 handler 를 import 하지 않도록 클로저로 받는다).
func (h *Handler) SetGivingSweep(fn func() (interface{}, error)) {
	h.givingSweep = fn
}

// GivingDrawSweep handles POST /api/internal/cron/giving-draw-sweep
//
// 마감이 지난 open 나눔을 자동 개표(자동 방식)하거나 주최자에게 개표를
// 독촉(지명 방식)한다. 시간 기반 트리거가 없어 마감 12일 경과 미개표
// (giving/2405)가 방치되던 구멍을 메운다. 멱등 — 개표 완료 건은 건드리지 않는다.
func (h *Handler) GivingDrawSweep(c *gin.Context) {
	if !h.verifySecret(c) {
		return
	}
	if h.givingSweep == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "giving sweep not wired"})
		return
	}

	result, err := h.givingSweep()
	if err != nil {
		log.Printf("[Cron:giving-draw-sweep] error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	log.Printf("[Cron:giving-draw-sweep] %+v", result)
	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/middleware"
	"github.com/stretchr/testify/assert"
)

func TestGetDBStrategyByPlan_TenantService(t *testing.T) {
	svc := &TenantService{}
	assert.Equal(t, "shared", svc.getDBStrategyByPlan("free"))
	assert.Equal(t, "schema", svc.getDBStrategyByPlan("pro"))
	assert.Equal(t, "schema", svc.getDBStrategyByPlan("business"))
	assert.Equal(t, "dedicated", svc.getDBStrategyByPlan("enterprise"))
	assert.Equal(t, "shared", svc.getDBStrategyByPlan("invalid"))
}

func TestPlanLimits(t *testing.T) {
	free := middleware.GetPlanLimits("free")
	assert.Equal(t, int64(500), free.MaxStorage)
	assert.Equal(t, 5, free.MaxBoards)
	assert.False(t, free.CustomDomain)
	assert.False(t, free.PluginsAllowed)

	pro := middleware.GetPlanLimits("pro")
	assert.Equal(t, int64(5000), pro.MaxStorage)
	assert.True(t, pro.CustomDomain)
	assert.True(t, pro.PluginsAllowed)
	assert.Equal(t, 5, pro.MaxPlugins)

	enterprise := middleware.GetPlanLimits("enterprise")
	assert.Equal(t, int64(-1), enterprise.MaxStorage) // unlimited
	assert.Equal(t, -1, enterprise.MaxPlugins)

	// Unknown plan falls back to free
	unknown := middleware.GetPlanLimits("unknown")
	assert.Equal(t, free.MaxStorage, unknown.MaxStorage)
}

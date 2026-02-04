package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetDBStrategyByPlan(t *testing.T) {
	tests := []struct {
		plan     string
		expected string
	}{
		{"free", "shared"},
		{"pro", "schema"},
		{"business", "schema"},
		{"enterprise", "dedicated"},
		{"unknown", "shared"},
		{"", "shared"},
	}

	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			assert.Equal(t, tt.expected, getDBStrategyByPlan(tt.plan))
		})
	}
}

func TestPlanPricing(t *testing.T) {
	// Verify pricing map is populated
	assert.Equal(t, 0, planPricing["free"].MonthlyKRW)
	assert.Equal(t, 29000, planPricing["pro"].MonthlyKRW)
	assert.Equal(t, 99000, planPricing["business"].MonthlyKRW)
	assert.Equal(t, 299000, planPricing["enterprise"].MonthlyKRW)

	// Trial days
	assert.Equal(t, 0, planPricing["free"].TrialDays)
	assert.Equal(t, 14, planPricing["pro"].TrialDays)
	assert.Equal(t, 30, planPricing["enterprise"].TrialDays)

	// Yearly discount
	assert.Equal(t, 290000, planPricing["pro"].YearlyKRW)
	assert.Less(t, planPricing["pro"].YearlyKRW, planPricing["pro"].MonthlyKRW*12)
}

package domain

import "time"

// AIEvaluation represents an AI evaluation result for a report
type AIEvaluation struct {
	ID                int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	Table             string    `gorm:"column:sg_table" json:"sg_table"`
	Parent            int       `gorm:"column:sg_parent" json:"sg_parent"`
	Score             int       `gorm:"column:score" json:"score"`
	Confidence        int       `gorm:"column:confidence" json:"confidence"`
	RecommendedAction string    `gorm:"column:recommended_action" json:"recommended_action"`
	PenaltyDays       int       `gorm:"column:penalty_days" json:"penalty_days"`
	PenaltyType       string    `gorm:"column:penalty_type;type:json" json:"penalty_type"`
	PenaltyReasons    string    `gorm:"column:penalty_reasons;type:json" json:"penalty_reasons"`
	Reasoning         string    `gorm:"column:reasoning" json:"reasoning"`
	Flags             string    `gorm:"column:flags;type:json" json:"flags"`
	RawResponse       string    `gorm:"column:raw_response" json:"raw_response,omitempty"`
	Model             string    `gorm:"column:model" json:"model"`
	EvaluatedAt       time.Time `gorm:"column:evaluated_at" json:"evaluated_at"`
	EvaluatedBy       string    `gorm:"column:evaluated_by" json:"evaluated_by"`
	CreatedAt         time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName returns the table name
func (AIEvaluation) TableName() string {
	return "g5_ai_evaluation"
}

// SaveAIEvaluationRequest represents the request to save an AI evaluation
type SaveAIEvaluationRequest struct {
	Table             string   `json:"sg_table" binding:"required"`
	Parent            int      `json:"sg_parent" binding:"required"`
	Score             int      `json:"score"`
	Confidence        int      `json:"confidence"`
	RecommendedAction string   `json:"recommended_action"`
	PenaltyDays       int      `json:"penalty_days"`
	PenaltyType       []string `json:"penalty_type"`
	PenaltyReasons    []int    `json:"penalty_reasons"`
	Reasoning         string   `json:"reasoning"`
	Flags             []string `json:"flags"`
	RawResponse       string   `json:"raw_response"`
	Model             string   `json:"model"`
	EvaluatedAt       string   `json:"evaluated_at"`
}

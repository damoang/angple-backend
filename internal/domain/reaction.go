package domain

import "time"

// ReactionCount represents reaction count aggregate (g5_da_reaction)
// This table needs to be created - it doesn't exist in the original schema
type ReactionCount struct {
	ID            int    `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	TargetID      string `gorm:"column:target_id;index;size:100" json:"target_id"`
	ParentID      string `gorm:"column:parent_id;size:100" json:"parent_id"`
	Reaction      string `gorm:"column:reaction;size:50" json:"reaction"`
	ReactionCount int    `gorm:"column:reaction_count" json:"reaction_count"`
}

// TableName returns the table name for reaction counts
func (ReactionCount) TableName() string {
	return "g5_da_reaction"
}

// ReactionChoose represents individual user reaction (g5_da_reaction_choose)
// Maps to existing table structure: bo_table, wr_id, mb_id, chosen_type, chosen_ip, chosen_datetime
type ReactionChoose struct {
	ID         int64      `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	BoardTable string     `gorm:"column:bo_table" json:"bo_table"`
	WriteID    int64      `gorm:"column:wr_id" json:"wr_id"`
	MemberID   string     `gorm:"column:mb_id" json:"member_id"`
	Reaction   string     `gorm:"column:chosen_type" json:"reaction"` // emoji:thumbsup, etc.
	ChosenIP   string     `gorm:"column:chosen_ip" json:"chosen_ip,omitempty"`
	CreatedAt  *time.Time `gorm:"column:chosen_datetime" json:"created_at"`
	// Virtual fields for compatibility with existing code
	TargetID string `gorm:"-" json:"target_id,omitempty"` // Computed: comment:{bo_table}:{wr_id}
	ParentID string `gorm:"-" json:"parent_id,omitempty"` // Not stored in this table
}

// TableName returns the table name for reaction choices
func (ReactionChoose) TableName() string {
	return "g5_da_reaction_choose"
}

// GetTargetID returns computed target ID
func (r *ReactionChoose) GetTargetID() string {
	if r.TargetID != "" {
		return r.TargetID
	}
	return "comment:" + r.BoardTable + ":" + string(rune(r.WriteID))
}

// ReactionRequest represents request for adding/removing reaction
type ReactionRequest struct {
	ReactionMode string `json:"reactionMode"` // "add" or "remove"
	Reaction     string `json:"reaction"`     // reaction type (emoji:xxx, image:xxx)
	TargetID     string `json:"targetId"`     // target post/comment ID
	ParentID     string `json:"parentId"`     // parent post ID (for comments)
}

// ReactionItem represents a single reaction with count and choose status
type ReactionItem struct {
	Reaction   string `json:"reaction"`
	Category   string `json:"category"`
	ReactionID string `json:"reactionId"`
	Count      int    `json:"count"`
	Choose     bool   `json:"choose"`
}

// ReactionResponse represents reaction response
type ReactionResponse struct {
	Status  string                    `json:"status"`
	Message string                    `json:"message,omitempty"`
	Result  map[string][]ReactionItem `json:"result,omitempty"`
}

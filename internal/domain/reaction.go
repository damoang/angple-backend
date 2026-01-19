package domain

import "time"

// ReactionCount represents reaction count aggregate (g5_da_reaction)
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
type ReactionChoose struct {
	ID        int       `gorm:"column:id;primaryKey;autoIncrement" json:"id"`
	MemberID  string    `gorm:"column:member_id;index;size:50" json:"member_id"`
	TargetID  string    `gorm:"column:target_id;index;size:100" json:"target_id"`
	ParentID  string    `gorm:"column:parent_id;size:100" json:"parent_id"`
	Reaction  string    `gorm:"column:reaction;size:50" json:"reaction"`
	ChosenIP  string    `gorm:"column:chosen_ip;size:50" json:"chosen_ip,omitempty"`
	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

// TableName returns the table name for reaction choices
func (ReactionChoose) TableName() string {
	return "g5_da_reaction_choose"
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

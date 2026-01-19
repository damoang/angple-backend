package repository

import (
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReactionRepository handles reaction data operations
type ReactionRepository struct {
	db *gorm.DB
}

// NewReactionRepository creates a new ReactionRepository
func NewReactionRepository(db *gorm.DB) *ReactionRepository {
	return &ReactionRepository{db: db}
}

// GetReactions retrieves reactions for target IDs
func (r *ReactionRepository) GetReactions(targetIDs []string, memberID string) (map[string][]domain.ReactionItem, error) {
	result := make(map[string][]domain.ReactionItem)

	// Get reaction counts
	var counts []domain.ReactionCount
	if err := r.db.Where("target_id IN ?", targetIDs).
		Order("id ASC").
		Find(&counts).Error; err != nil {
		return nil, err
	}

	// Get member's choices
	memberChoices := make(map[string]map[string]bool)
	if memberID != "" {
		var choices []domain.ReactionChoose
		if err := r.db.Where("member_id = ? AND target_id IN ?", memberID, targetIDs).
			Find(&choices).Error; err != nil {
			return nil, err
		}

		for _, choice := range choices {
			if memberChoices[choice.TargetID] == nil {
				memberChoices[choice.TargetID] = make(map[string]bool)
			}
			memberChoices[choice.TargetID][choice.Reaction] = true
		}
	}

	// Build result
	for _, count := range counts {
		item := parseReaction(count.Reaction, count.ReactionCount)
		if memberID != "" {
			if choices, ok := memberChoices[count.TargetID]; ok {
				item.Choose = choices[count.Reaction]
			}
		}
		result[count.TargetID] = append(result[count.TargetID], item)
	}

	return result, nil
}

// GetReactionsByParent retrieves reactions by parent ID
func (r *ReactionRepository) GetReactionsByParent(parentID string, memberID string) (map[string][]domain.ReactionItem, error) {
	result := make(map[string][]domain.ReactionItem)

	// Get reaction counts
	var counts []domain.ReactionCount
	if err := r.db.Where("parent_id = ?", parentID).
		Order("id ASC").
		Find(&counts).Error; err != nil {
		return nil, err
	}

	// Get member's choices
	memberChoices := make(map[string]map[string]bool)
	if memberID != "" {
		var choices []domain.ReactionChoose
		if err := r.db.Where("member_id = ? AND parent_id = ?", memberID, parentID).
			Find(&choices).Error; err != nil {
			return nil, err
		}

		for _, choice := range choices {
			if memberChoices[choice.TargetID] == nil {
				memberChoices[choice.TargetID] = make(map[string]bool)
			}
			memberChoices[choice.TargetID][choice.Reaction] = true
		}
	}

	// Build result
	for _, count := range counts {
		item := parseReaction(count.Reaction, count.ReactionCount)
		if memberID != "" {
			if choices, ok := memberChoices[count.TargetID]; ok {
				item.Choose = choices[count.Reaction]
			}
		}
		result[count.TargetID] = append(result[count.TargetID], item)
	}

	return result, nil
}

// HasReaction checks if member already has a reaction
func (r *ReactionRepository) HasReaction(memberID, targetID, reaction string) (bool, error) {
	var count int64
	err := r.db.Model(&domain.ReactionChoose{}).
		Where("member_id = ? AND target_id = ? AND reaction = ?", memberID, targetID, reaction).
		Count(&count).Error
	return count > 0, err
}

// AddReaction adds a new reaction
func (r *ReactionRepository) AddReaction(memberID, reaction, targetID, parentID, ip string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Add to choose table
		choose := &domain.ReactionChoose{
			MemberID:  memberID,
			Reaction:  reaction,
			TargetID:  targetID,
			ParentID:  parentID,
			ChosenIP:  ip,
			CreatedAt: time.Now(),
		}
		if err := tx.Create(choose).Error; err != nil {
			return err
		}

		// Upsert reaction count
		reactionCount := &domain.ReactionCount{
			Reaction:      reaction,
			TargetID:      targetID,
			ParentID:      parentID,
			ReactionCount: 1,
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "target_id"}, {Name: "reaction"}},
			DoUpdates: clause.Assignments(map[string]interface{}{"reaction_count": gorm.Expr("reaction_count + 1")}),
		}).Create(reactionCount).Error
	})
}

// RemoveReaction removes a reaction
func (r *ReactionRepository) RemoveReaction(memberID, reaction, targetID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Remove from choose table
		if err := tx.Where("member_id = ? AND target_id = ? AND reaction = ?", memberID, targetID, reaction).
			Delete(&domain.ReactionChoose{}).Error; err != nil {
			return err
		}

		// Decrement reaction count
		if err := tx.Model(&domain.ReactionCount{}).
			Where("target_id = ? AND reaction = ?", targetID, reaction).
			Update("reaction_count", gorm.Expr("reaction_count - 1")).Error; err != nil {
			return err
		}

		// Delete if count <= 0
		return tx.Where("target_id = ? AND reaction = ? AND reaction_count <= 0", targetID, reaction).
			Delete(&domain.ReactionCount{}).Error
	})
}

// GetReactionCount gets count of reactions for a target
func (r *ReactionRepository) GetReactionCount(targetID string) (int64, error) {
	var count int64
	err := r.db.Model(&domain.ReactionCount{}).
		Where("target_id = ?", targetID).
		Count(&count).Error
	return count, err
}

// parseReaction parses reaction string into ReactionItem
func parseReaction(reaction string, count int) domain.ReactionItem {
	parts := strings.SplitN(reaction, ":", 2)
	category := ""
	reactionID := reaction
	if len(parts) == 2 {
		category = parts[0]
		reactionID = parts[1]
	}

	return domain.ReactionItem{
		Reaction:   reaction,
		Category:   category,
		ReactionID: reactionID,
		Count:      count,
		Choose:     false,
	}
}

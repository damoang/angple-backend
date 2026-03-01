package v2

import (
	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// ExpSummary represents experience point summary statistics
type ExpSummary struct {
	TotalExp     int `json:"total_exp"`
	CurrentLevel int `json:"current_level"`
	NextLevel    int `json:"next_level"`
	NextLevelExp int `json:"next_level_exp"`
	ExpToNext    int `json:"exp_to_next"`
	Progress     int `json:"progress"` // percentage 0-100
}

// ExpRepository handles experience point data access
type ExpRepository interface {
	// GetSummary returns exp summary for a user (by mb_id)
	GetSummary(mbID string) (*ExpSummary, error)
	// GetHistory returns exp history with pagination
	GetHistory(mbID string, page, limit int) ([]gnuboard.ExpHistory, int64, error)
	// AddExp adds experience points to a user
	AddExp(mbID string, point int, content, relTable, relID, action string) error
}

type expRepository struct {
	db *gorm.DB
}

// NewExpRepository creates a new ExpRepository
func NewExpRepository(db *gorm.DB) ExpRepository {
	return &expRepository{db: db}
}

// Level thresholds (cumulative exp required for each level)
var levelThresholds = []int{
	0,      // Level 1
	1000,   // Level 2
	3000,   // Level 3
	6000,   // Level 4
	10000,  // Level 5
	15000,  // Level 6
	21000,  // Level 7
	28000,  // Level 8
	36000,  // Level 9
	45000,  // Level 10
	55000,  // Level 11
	66000,  // Level 12
	78000,  // Level 13
	91000,  // Level 14
	105000, // Level 15
}

func calculateLevelInfo(totalExp int) (currentLevel, nextLevel, nextLevelExp, expToNext, progress int) {
	currentLevel = 1
	for i, threshold := range levelThresholds {
		if totalExp >= threshold {
			currentLevel = i + 1
		} else {
			break
		}
	}

	// Calculate next level info
	if currentLevel >= len(levelThresholds) {
		// Max level reached
		nextLevel = currentLevel
		nextLevelExp = levelThresholds[len(levelThresholds)-1]
		expToNext = 0
		progress = 100
	} else {
		nextLevel = currentLevel + 1
		nextLevelExp = levelThresholds[currentLevel]
		prevLevelExp := 0
		if currentLevel > 1 {
			prevLevelExp = levelThresholds[currentLevel-1]
		}
		expToNext = nextLevelExp - totalExp
		levelRange := nextLevelExp - prevLevelExp
		if levelRange > 0 {
			progress = (totalExp - prevLevelExp) * 100 / levelRange
		}
	}

	return
}

func (r *expRepository) GetSummary(mbID string) (*ExpSummary, error) {
	// Get current exp and level from member
	var member gnuboard.G5Member
	if err := r.db.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
		return nil, err
	}

	totalExp := member.AsExp
	currentLevel, nextLevel, nextLevelExp, expToNext, progress := calculateLevelInfo(totalExp)

	return &ExpSummary{
		TotalExp:     totalExp,
		CurrentLevel: currentLevel,
		NextLevel:    nextLevel,
		NextLevelExp: nextLevelExp,
		ExpToNext:    expToNext,
		Progress:     progress,
	}, nil
}

func (r *expRepository) GetHistory(mbID string, page, limit int) ([]gnuboard.ExpHistory, int64, error) {
	// Count total
	var total int64
	if err := r.db.Model(&gnuboard.G5NaXP{}).Where("mb_id = ?", mbID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	var xpLogs []gnuboard.G5NaXP
	if err := r.db.Where("mb_id = ?", mbID).
		Order("xp_datetime DESC").
		Offset(offset).
		Limit(limit).
		Find(&xpLogs).Error; err != nil {
		return nil, 0, err
	}

	// Convert to ExpHistory
	history := make([]gnuboard.ExpHistory, len(xpLogs))
	for i, xp := range xpLogs {
		history[i] = xp.ToExpHistory()
	}

	return history, total, nil
}

func (r *expRepository) AddExp(mbID string, point int, content, relTable, relID, action string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Update member exp
		if err := tx.Model(&gnuboard.G5Member{}).
			Where("mb_id = ?", mbID).
			UpdateColumn("as_exp", gorm.Expr("as_exp + ?", point)).Error; err != nil {
			return err
		}

		// Get updated exp to check level
		var member gnuboard.G5Member
		if err := tx.Select("as_exp, as_level").Where("mb_id = ?", mbID).First(&member).Error; err != nil {
			return err
		}

		// Check if level up is needed
		newLevel, _, _, _, _ := calculateLevelInfo(member.AsExp)
		if newLevel > member.AsLevel {
			if err := tx.Model(&gnuboard.G5Member{}).
				Where("mb_id = ?", mbID).
				UpdateColumn("as_level", newLevel).Error; err != nil {
				return err
			}
		}

		// Insert exp log
		log := &gnuboard.G5NaXP{
			MbID:        mbID,
			XpPoint:     point,
			XpContent:   content,
			XpRelTable:  relTable,
			XpRelID:     relID,
			XpRelAction: action,
		}
		return tx.Create(log).Error
	})
}

package gnuboard

import (
	"strings"
	"time"
	"unicode/utf8"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
	"gorm.io/gorm"
)

// TagRepository handles g5_na_tag / g5_na_tag_log operations
type TagRepository interface {
	GetPostTags(boTable string, wrID int) ([]string, error)
	GetPostsTagsBatch(boTable string, wrIDs []int) (map[int][]string, error)
	SetPostTags(boTable string, wrID int, tags []string, mbID string) error
	DeletePostTags(boTable string, wrID int) error
}

type tagRepository struct {
	db *gorm.DB
}

// NewTagRepository creates a new TagRepository
func NewTagRepository(db *gorm.DB) TagRepository {
	return &tagRepository{db: db}
}

// GetPostTags returns tag names for a post
func (r *tagRepository) GetPostTags(boTable string, wrID int) ([]string, error) {
	var logs []gnuboard.G5NaTagLog
	err := r.db.Where("bo_table = ? AND wr_id = ?", boTable, wrID).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	tags := make([]string, len(logs))
	for i, l := range logs {
		tags[i] = l.Tag
	}
	return tags, nil
}

// GetPostsTagsBatch returns tags for multiple posts at once
func (r *tagRepository) GetPostsTagsBatch(boTable string, wrIDs []int) (map[int][]string, error) {
	if len(wrIDs) == 0 {
		return nil, nil
	}
	var logs []gnuboard.G5NaTagLog
	err := r.db.Where("bo_table = ? AND wr_id IN ?", boTable, wrIDs).Find(&logs).Error
	if err != nil {
		return nil, err
	}
	tagMap := make(map[int][]string)
	for _, l := range logs {
		tagMap[l.WrID] = append(tagMap[l.WrID], l.Tag)
	}
	return tagMap, nil
}

// SetPostTags replaces all tags for a post
func (r *tagRepository) SetPostTags(boTable string, wrID int, tags []string, mbID string) error {
	// Normalize and deduplicate
	normalized := make([]string, 0, len(tags))
	seen := make(map[string]bool)
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t != "" && !seen[t] {
			normalized = append(normalized, t)
			seen[t] = true
		}
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		// 1. Get existing tag_ids before delete (for cnt update later)
		var oldTagIDs []int
		tx.Model(&gnuboard.G5NaTagLog{}).
			Where("bo_table = ? AND wr_id = ?", boTable, wrID).
			Pluck("tag_id", &oldTagIDs)

		// 2. Delete existing tag logs
		if err := tx.Where("bo_table = ? AND wr_id = ?", boTable, wrID).
			Delete(&gnuboard.G5NaTagLog{}).Error; err != nil {
			return err
		}

		if len(normalized) == 0 {
			// Only need to update old tag counts
			if len(oldTagIDs) > 0 {
				tx.Exec("UPDATE g5_na_tag SET cnt = (SELECT COUNT(*) FROM g5_na_tag_log WHERE tag_id = g5_na_tag.id) WHERE id IN ?", oldTagIDs)
			}
			return nil
		}

		now := time.Now()

		// 3. Batch find existing tags (1 query instead of N)
		var existingTags []gnuboard.G5NaTag
		tx.Where("tag IN ? AND type = 0", normalized).Find(&existingTags)
		tagMap := make(map[string]gnuboard.G5NaTag, len(existingTags))
		for _, t := range existingTags {
			tagMap[t.Tag] = t
		}

		// 4. Create missing tags + build log entries
		var newTagIDs []int
		for _, tagName := range normalized {
			tag, exists := tagMap[tagName]
			if !exists {
				tag = gnuboard.G5NaTag{
					Type: 0, Idx: firstChar(tagName), Tag: tagName,
					Cnt: 0, RegDate: now, LastDate: now,
				}
				if err := tx.Create(&tag).Error; err != nil {
					return err
				}
				tagMap[tagName] = tag
			}

			// Insert tag log
			logEntry := gnuboard.G5NaTagLog{
				BoTable: boTable, WrID: wrID, TagID: tag.ID,
				Tag: tagName, MbID: mbID, RegDate: now,
			}
			if err := tx.Create(&logEntry).Error; err != nil {
				return err
			}
			newTagIDs = append(newTagIDs, tag.ID)
		}

		// 5. Batch update cnt + lastdate for all affected tags (1 query)
		allAffectedIDs := make(map[int]bool)
		for _, id := range oldTagIDs {
			allAffectedIDs[id] = true
		}
		for _, id := range newTagIDs {
			allAffectedIDs[id] = true
		}
		ids := make([]int, 0, len(allAffectedIDs))
		for id := range allAffectedIDs {
			ids = append(ids, id)
		}
		if len(ids) > 0 {
			tx.Exec(`UPDATE g5_na_tag SET
				cnt = (SELECT COUNT(*) FROM g5_na_tag_log WHERE tag_id = g5_na_tag.id),
				lastdate = ? WHERE id IN ?`, now, ids)
		}

		return nil
	})
}

// DeletePostTags removes all tag logs for a post
func (r *tagRepository) DeletePostTags(boTable string, wrID int) error {
	return r.db.Where("bo_table = ? AND wr_id = ?", boTable, wrID).
		Delete(&gnuboard.G5NaTagLog{}).Error
}

// firstChar returns the first character of a string for the idx field
func firstChar(s string) string {
	if s == "" {
		return ""
	}
	r, _ := utf8.DecodeRuneInString(s)
	return string(r)
}

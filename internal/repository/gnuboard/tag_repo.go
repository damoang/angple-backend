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
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Delete existing tag logs for this post
		if err := tx.Where("bo_table = ? AND wr_id = ?", boTable, wrID).
			Delete(&gnuboard.G5NaTagLog{}).Error; err != nil {
			return err
		}

		now := time.Now()

		for _, tagName := range tags {
			tagName = strings.TrimSpace(tagName)
			if tagName == "" {
				continue
			}

			// Find or create the tag in g5_na_tag
			var tag gnuboard.G5NaTag
			err := tx.Where("tag = ? AND type = 0", tagName).First(&tag).Error
			if err != nil {
				// Create new tag
				tag = gnuboard.G5NaTag{
					Type:     0,
					Idx:      firstChar(tagName),
					Tag:      tagName,
					Cnt:      0,
					RegDate:  now,
					LastDate: now,
				}
				if err := tx.Create(&tag).Error; err != nil {
					return err
				}
			}

			// Create tag log entry
			log := gnuboard.G5NaTagLog{
				BoTable: boTable,
				WrID:    wrID,
				TagID:   tag.ID,
				Tag:     tagName,
				MbID:    mbID,
				RegDate: now,
			}
			if err := tx.Create(&log).Error; err != nil {
				return err
			}

			// Update tag count and lastdate
			tx.Model(&gnuboard.G5NaTag{}).Where("id = ?", tag.ID).Updates(map[string]interface{}{
				"cnt":      gorm.Expr("(SELECT COUNT(*) FROM g5_na_tag_log WHERE tag_id = ?)", tag.ID),
				"lastdate": now,
			})
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

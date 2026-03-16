package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// NariyaSettings represents the parsed extended settings JSON for nariya conversion
type NariyaSettings struct {
	Comment      *NariyaComment      `json:"comment,omitempty"`
	Lucky        *NariyaLucky        `json:"lucky,omitempty"`
	XP           *NariyaXP           `json:"xp,omitempty"`
	Features     *NariyaFeatures     `json:"features,omitempty"`
	Notification *NariyaNotification `json:"notification,omitempty"`
	Writing      *NariyaWriting      `json:"writing,omitempty"`
	Skin         *NariyaSkin         `json:"skin,omitempty"`
}

// NariyaComment holds comment settings
type NariyaComment struct {
	UseRecommend     bool   `json:"useRecommend"`
	UseDislike       bool   `json:"useDislike"`
	AuthorOnly       bool   `json:"authorOnly"`
	Paging           string `json:"paging"`
	PageSize         int    `json:"pageSize"`
	ImageSizeLimitMB int    `json:"imageSizeLimitMB"`
	AutoEmbed        bool   `json:"autoEmbed"`
}

// NariyaLucky holds lucky point settings
type NariyaLucky struct {
	Points int `json:"points"`
	Odds   int `json:"odds"`
}

// NariyaXP holds XP settings
type NariyaXP struct {
	Write   int `json:"write"`
	Comment int `json:"comment"`
}

// NariyaFeatures holds feature toggle settings
type NariyaFeatures struct {
	CodeHighlighter     bool   `json:"codeHighlighter"`
	ExternalImageSave   bool   `json:"externalImageSave"`
	TagLevel            int    `json:"tagLevel"`
	Rating              bool   `json:"rating"`
	MobileEditor        string `json:"mobileEditor"`
	CategoryMovePermit  string `json:"categoryMovePermit"`
	CategoryMoveMessage string `json:"categoryMoveMessage"`
	HideNickname        bool   `json:"hideNickname"`
}

// NariyaNotification holds notification settings
type NariyaNotification struct {
	NewPostReceivers string `json:"newPostReceivers"`
	Enabled          bool   `json:"enabled"`
}

// NariyaWriting holds writing restriction settings
type NariyaWriting struct {
	MaxPosts            int    `json:"maxPosts"`
	MaxPostsTotal       int    `json:"maxPostsTotal"`
	AllowedLevels       string `json:"allowedLevels"`
	RestrictedUsers     bool   `json:"restrictedUsers"`
	MemberOnly          bool   `json:"memberOnly"`
	MemberOnlyPermit    string `json:"memberOnlyPermit"`
	AllowedMembersOne   string `json:"allowedMembersOne"`
	AllowedMembersTwo   string `json:"allowedMembersTwo"`
	AllowedMembersThree string `json:"allowedMembersThree"`
}

// NariyaSkin holds skin settings
type NariyaSkin struct {
	Category string `json:"category"`
	List     string `json:"list"`
	View     string `json:"view"`
	Comment  string `json:"comment"`
}

// GenerateNariyaPHP converts extended settings JSON to nariya PHP file content
func GenerateNariyaPHP(settingsJSON string) (string, error) {
	var s NariyaSettings
	if err := json.Unmarshal([]byte(settingsJSON), &s); err != nil {
		return "", fmt.Errorf("unmarshal settings: %w", err)
	}

	pairs := buildPHPPairs(&s)

	var b strings.Builder
	b.WriteString("<?php\nif (!defined('_GNUBOARD_')) exit();\n$data=array (\n")

	for i, p := range pairs {
		b.WriteString(fmt.Sprintf("  '%s' => '%s'", p.key, escapePHPString(p.value)))
		if i < len(pairs)-1 {
			b.WriteString(",\n")
		} else {
			b.WriteString(",\n")
		}
	}

	b.WriteString(");\n")
	return b.String(), nil
}

type phpPair struct {
	key   string
	value string
}

func buildPHPPairs(s *NariyaSettings) []phpPair {
	var pairs []phpPair

	// Skin settings
	cateSkin := "basic"
	listSkin := "list"
	viewSkin := "basic"
	commentSkin := "basic"
	if s.Skin != nil {
		if s.Skin.Category != "" {
			cateSkin = s.Skin.Category
		}
		if s.Skin.List != "" {
			listSkin = s.Skin.List
		}
		if s.Skin.View != "" {
			viewSkin = s.Skin.View
		}
		if s.Skin.Comment != "" {
			commentSkin = s.Skin.Comment
		}
	}
	pairs = append(pairs,
		phpPair{"cate_skin", cateSkin},
		phpPair{"list_skin", listSkin},
		phpPair{"view_skin", viewSkin},
		phpPair{"comment_skin", commentSkin},
	)

	// Notification (noti_mb)
	notiMb := ""
	if s.Notification != nil {
		notiMb = s.Notification.NewPostReceivers
	}
	pairs = append(pairs, phpPair{"noti_mb", notiMb})

	// Editor mobile
	editorMo := ""
	if s.Features != nil {
		editorMo = s.Features.MobileEditor
	}
	pairs = append(pairs, phpPair{"editor_mo", editorMo})

	// Tag level
	tagLevel := "0"
	if s.Features != nil && s.Features.TagLevel > 0 {
		tagLevel = strconv.Itoa(s.Features.TagLevel)
	}
	pairs = append(pairs, phpPair{"tag", tagLevel})

	// XP
	xpWrite := ""
	xpComment := ""
	if s.XP != nil {
		if s.XP.Write > 0 {
			xpWrite = strconv.Itoa(s.XP.Write)
		}
		if s.XP.Comment > 0 {
			xpComment = strconv.Itoa(s.XP.Comment)
		}
	}
	pairs = append(pairs,
		phpPair{"xp_write", xpWrite},
		phpPair{"xp_comment", xpComment},
	)

	// Feature flags (only include if enabled)
	if s.Features != nil {
		if s.Features.CodeHighlighter {
			pairs = append(pairs, phpPair{"code", "1"})
		}
		if s.Features.ExternalImageSave {
			pairs = append(pairs, phpPair{"save_image", "1"})
		}
		if s.Features.Rating {
			pairs = append(pairs, phpPair{"check_star_rating", "1"})
		}
		if s.Features.HideNickname {
			pairs = append(pairs, phpPair{"check_list_hide_profile", "1"})
		}
	}

	// Notification disabled flag
	if s.Notification != nil && !s.Notification.Enabled {
		pairs = append(pairs, phpPair{"noti_no", "1"})
	}

	// Comment author only
	if s.Comment != nil && s.Comment.AuthorOnly {
		pairs = append(pairs, phpPair{"author_only_comment", "1"})
	}

	// Comment auto embed
	if s.Comment != nil && s.Comment.AutoEmbed {
		pairs = append(pairs, phpPair{"comment_convert", "1"})
	}

	// Writing restrictions
	if s.Writing != nil {
		if s.Writing.RestrictedUsers {
			pairs = append(pairs, phpPair{"check_write_permit", "1"})
		}
		if s.Writing.MemberOnly {
			pairs = append(pairs, phpPair{"check_member_only", "1"})
		}
		pairs = append(pairs,
			phpPair{"bo_write_allow_one", s.Writing.AllowedMembersOne},
			phpPair{"bo_write_allow_two", s.Writing.AllowedMembersTwo},
			phpPair{"bo_write_allow_three", s.Writing.AllowedMembersThree},
		)
		memberOnlyPermit := "admin_only"
		if s.Writing.MemberOnlyPermit != "" {
			memberOnlyPermit = s.Writing.MemberOnlyPermit
		}
		pairs = append(pairs, phpPair{"member_only_permit", memberOnlyPermit})
	} else {
		pairs = append(pairs,
			phpPair{"bo_write_allow_one", ""},
			phpPair{"bo_write_allow_two", ""},
			phpPair{"bo_write_allow_three", ""},
			phpPair{"member_only_permit", "admin_only"},
		)
	}

	// Category move
	if s.Features != nil {
		categoryMovePermit := "admin_only"
		if s.Features.CategoryMovePermit != "" {
			categoryMovePermit = s.Features.CategoryMovePermit
		}
		pairs = append(pairs, phpPair{"category_move_permit", categoryMovePermit})
		if s.Features.CategoryMoveMessage != "" {
			pairs = append(pairs, phpPair{"category_move_message", s.Features.CategoryMoveMessage})
		}
	} else {
		pairs = append(pairs, phpPair{"category_move_permit", "admin_only"})
	}

	// Writing limits
	limitMaxWrite := "0"
	writeableLevel := ""
	if s.Writing != nil {
		if s.Writing.MaxPosts > 0 {
			limitMaxWrite = strconv.Itoa(s.Writing.MaxPosts)
		}
		writeableLevel = s.Writing.AllowedLevels
	}
	pairs = append(pairs,
		phpPair{"limit_max_write", limitMaxWrite},
		phpPair{"writeable_level", writeableLevel},
	)

	// Comment settings
	commentImageSize := ""
	if s.Comment != nil && s.Comment.ImageSizeLimitMB > 0 {
		commentImageSize = strconv.Itoa(s.Comment.ImageSizeLimitMB)
	}
	pairs = append(pairs, phpPair{"comment_image_size", commentImageSize})

	commentGood := ""
	if s.Comment != nil && s.Comment.UseRecommend {
		commentGood = "1"
	}
	pairs = append(pairs, phpPair{"comment_good", commentGood})

	commentSort := "old"
	if s.Comment != nil && s.Comment.Paging == "newest" {
		commentSort = "new"
	}
	pairs = append(pairs, phpPair{"comment_sort", commentSort})

	commentRows := "5000"
	if s.Comment != nil && s.Comment.PageSize > 0 {
		commentRows = strconv.Itoa(s.Comment.PageSize)
	}
	pairs = append(pairs, phpPair{"comment_rows", commentRows})

	// Lucky point
	luckyPoint := ""
	luckyDice := ""
	if s.Lucky != nil {
		if s.Lucky.Points > 0 {
			luckyPoint = strconv.Itoa(s.Lucky.Points)
		}
		if s.Lucky.Odds > 0 {
			luckyDice = strconv.Itoa(s.Lucky.Odds)
		}
	}
	pairs = append(pairs,
		phpPair{"lucky_point", luckyPoint},
		phpPair{"lucky_dice", luckyDice},
	)

	return pairs
}

func escapePHPString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "'", "\\'")
	return s
}

// WriteNariyaPHPFiles generates and writes nariya PHP files for a board
func WriteNariyaPHPFiles(nariyaDataPath, boardSlug, settingsJSON string) error {
	if nariyaDataPath == "" {
		return nil // nariya sync disabled
	}

	phpContent, err := GenerateNariyaPHP(settingsJSON)
	if err != nil {
		return fmt.Errorf("generate PHP: %w", err)
	}

	// Write both PC and mobile files (identical content)
	for _, suffix := range []string{"pc", "mo"} {
		filename := fmt.Sprintf("board-%s-%s.php", boardSlug, suffix)
		filePath := filepath.Join(nariyaDataPath, filename)

		if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
			return fmt.Errorf("create directory: %w", err)
		}

		if err := os.WriteFile(filePath, []byte(phpContent), 0644); err != nil {
			return fmt.Errorf("write %s: %w", filename, err)
		}
	}

	return nil
}

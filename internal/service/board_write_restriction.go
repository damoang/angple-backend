package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	v2repo "github.com/damoang/angple-backend/internal/repository/v2"

	"gorm.io/gorm"
)

// WritingSettings mirrors the frontend WritingSettings interface from v2_board_extended_settings JSON.
type WritingSettings struct {
	MaxPosts            int    `json:"maxPosts,omitempty"`
	MaxPostsTotal       int    `json:"maxPostsTotal,omitempty"`
	AllowedLevels       string `json:"allowedLevels,omitempty"`
	RestrictedUsers     bool   `json:"restrictedUsers,omitempty"`
	MemberOnly          bool   `json:"memberOnly,omitempty"`
	MemberOnlyPermit    string `json:"memberOnlyPermit,omitempty"`
	AllowedMembersOne   string `json:"allowedMembersOne,omitempty"`
	AllowedMembersTwo   string `json:"allowedMembersTwo,omitempty"`
	AllowedMembersThree string `json:"allowedMembersThree,omitempty"`
}

// ExtendedSettingsJSON represents the top-level JSON structure of v2_board_extended_settings.settings.
type ExtendedSettingsJSON struct {
	Writing *WritingSettings `json:"writing,omitempty"`
}

// WriteRestrictionResult is returned by Check to indicate whether a member can write.
type WriteRestrictionResult struct {
	CanWrite   bool   `json:"can_write"`
	Remaining  int    `json:"remaining"`   // -1 = unlimited
	DailyLimit int    `json:"daily_limit"` // 0 = unlimited
	TotalLimit int    `json:"total_limit"` // 0 = unlimited
	TotalCount int    `json:"total_count"`
	Reason     string `json:"reason,omitempty"`
}

// BoardWriteRestrictionService enforces per-board writing restrictions
// based on v2_board_extended_settings WritingSettings.
type BoardWriteRestrictionService struct {
	db                   *gorm.DB
	extendedSettingsRepo v2repo.BoardExtendedSettingsRepository
}

// NewBoardWriteRestrictionService creates a new BoardWriteRestrictionService.
func NewBoardWriteRestrictionService(db *gorm.DB, repo v2repo.BoardExtendedSettingsRepository) *BoardWriteRestrictionService {
	return &BoardWriteRestrictionService{
		db:                   db,
		extendedSettingsRepo: repo,
	}
}

// Check verifies whether the given member can write to the specified board.
func (s *BoardWriteRestrictionService) Check(boardSlug, memberID string, memberLevel int) (*WriteRestrictionResult, error) {
	settings, err := s.extendedSettingsRepo.FindByBoardSlug(boardSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to load extended settings for board %s: %w", boardSlug, err)
	}

	writing, err := parseWritingSettings(settings)
	if err != nil {
		return nil, fmt.Errorf("failed to parse writing settings for board %s: %w", boardSlug, err)
	}

	// No writing restrictions configured → allow
	if writing == nil {
		return &WriteRestrictionResult{CanWrite: true, Remaining: -1, DailyLimit: 0}, nil
	}

	// 최고관리자(레벨 10 이상)는 모든 글쓰기 제한 바이패스
	if memberLevel >= 10 {
		return &WriteRestrictionResult{CanWrite: true, Remaining: -1, DailyLimit: 0}, nil
	}

	// 1. allowedLevels check
	if writing.AllowedLevels != "" {
		levels := parseCommaSeparatedInts(writing.AllowedLevels)
		if len(levels) > 0 && !containsInt(levels, memberLevel) {
			return &WriteRestrictionResult{
				CanWrite: false,
				Reason:   fmt.Sprintf("레벨 %s만 글을 작성할 수 있습니다.", writing.AllowedLevels),
			}, nil
		}
	}

	// 2. maxPostsTotal — lifetime total limit (e.g. 가입인사 1회)
	if writing.MaxPostsTotal > 0 {
		totalCount, err := s.countTotalPosts(boardSlug, memberID)
		if err != nil {
			return nil, fmt.Errorf("failed to count total posts: %w", err)
		}

		if totalCount >= writing.MaxPostsTotal {
			return &WriteRestrictionResult{
				CanWrite:   false,
				Remaining:  0,
				TotalLimit: writing.MaxPostsTotal,
				TotalCount: totalCount,
				Reason:     fmt.Sprintf("이 게시판에는 총 %d개까지 작성 가능합니다. (이미 %d개 작성)", writing.MaxPostsTotal, totalCount),
			}, nil
		}

		remaining := writing.MaxPostsTotal - totalCount
		return &WriteRestrictionResult{
			CanWrite:   true,
			Remaining:  remaining,
			TotalLimit: writing.MaxPostsTotal,
			TotalCount: totalCount,
		}, nil
	}

	// 3. restrictedUsers → only allowed members can write
	if writing.RestrictedUsers {
		dailyLimit := findMemberDailyLimit(memberID, writing)
		if dailyLimit == 0 {
			return &WriteRestrictionResult{
				CanWrite: false,
				Reason:   "허용된 회원만 글을 작성할 수 있습니다.",
			}, nil
		}

		todayCount, err := s.countTodayPosts(boardSlug, memberID)
		if err != nil {
			return nil, fmt.Errorf("failed to count today's posts: %w", err)
		}

		if todayCount >= dailyLimit {
			return &WriteRestrictionResult{
				CanWrite:   false,
				Remaining:  0,
				DailyLimit: dailyLimit,
				Reason:     fmt.Sprintf("오늘 %d개까지 작성 가능합니다. (이미 %d개 작성)", dailyLimit, todayCount),
			}, nil
		}

		return &WriteRestrictionResult{
			CanWrite:   true,
			Remaining:  dailyLimit - todayCount,
			DailyLimit: dailyLimit,
		}, nil
	}

	// 4. maxPosts global daily limit (applies regardless of restrictedUsers)
	if writing.MaxPosts > 0 {
		todayCount, err := s.countTodayPosts(boardSlug, memberID)
		if err != nil {
			return nil, fmt.Errorf("failed to count today's posts: %w", err)
		}

		if todayCount >= writing.MaxPosts {
			return &WriteRestrictionResult{
				CanWrite:   false,
				Remaining:  0,
				DailyLimit: writing.MaxPosts,
				Reason:     fmt.Sprintf("하루 %d개까지 작성 가능합니다. (이미 %d개 작성)", writing.MaxPosts, todayCount),
			}, nil
		}

		return &WriteRestrictionResult{
			CanWrite:   true,
			Remaining:  writing.MaxPosts - todayCount,
			DailyLimit: writing.MaxPosts,
		}, nil
	}

	// No restrictions → unlimited
	return &WriteRestrictionResult{CanWrite: true, Remaining: -1, DailyLimit: 0}, nil
}

// countTodayPosts counts the number of non-comment posts written today (KST) by the member.
// wr_datetime is stored in KST by legacy PHP (Gnuboard).
func (s *BoardWriteRestrictionService) countTodayPosts(boardSlug, memberID string) (int, error) {
	tableName := fmt.Sprintf("g5_write_%s", boardSlug)
	kst := time.FixedZone("KST", 9*60*60)
	today := time.Now().In(kst).Format("2006-01-02")

	var count int64
	err := s.db.Table(tableName).
		Where("mb_id = ? AND DATE(wr_datetime) = ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", memberID, today).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// countTotalPosts counts all non-comment posts ever written by the member in the board.
func (s *BoardWriteRestrictionService) countTotalPosts(boardSlug, memberID string) (int, error) {
	tableName := fmt.Sprintf("g5_write_%s", boardSlug)

	var count int64
	err := s.db.Table(tableName).
		Where("mb_id = ? AND wr_is_comment = 0 AND (wr_deleted_at IS NULL OR wr_deleted_at = '0000-00-00 00:00:00')", memberID).
		Count(&count).Error
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

// parseWritingSettings extracts WritingSettings from the extended settings JSON.
func parseWritingSettings(settings *v2domain.V2BoardExtendedSettings) (*WritingSettings, error) {
	if settings == nil || settings.Settings == "" || settings.Settings == "{}" {
		return nil, nil
	}

	var parsed ExtendedSettingsJSON
	if err := json.Unmarshal([]byte(settings.Settings), &parsed); err != nil {
		return nil, err
	}
	return parsed.Writing, nil
}

// findMemberDailyLimit returns how many posts per day the member is allowed.
// Checks Three(3) → Two(2) → One(1) lists. Returns 0 if not found.
func findMemberDailyLimit(memberID string, w *WritingSettings) int {
	if containsMember(w.AllowedMembersThree, memberID) {
		return 3
	}
	if containsMember(w.AllowedMembersTwo, memberID) {
		return 2
	}
	if containsMember(w.AllowedMembersOne, memberID) {
		return 1
	}
	return 0
}

// containsMember checks if memberID is in a comma-separated list.
func containsMember(commaSeparated, memberID string) bool {
	if commaSeparated == "" || memberID == "" {
		return false
	}
	for _, id := range strings.Split(commaSeparated, ",") {
		if strings.TrimSpace(id) == memberID {
			return true
		}
	}
	return false
}

// parseCommaSeparatedInts parses "1,3,5" into []int{1, 3, 5}.
func parseCommaSeparatedInts(s string) []int {
	var result []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if v, err := strconv.Atoi(part); err == nil {
			result = append(result, v)
		}
	}
	return result
}

// containsInt checks if a value exists in an int slice.
func containsInt(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

package common

import (
	"errors"
	"regexp"
	"slices"
	"strings"
)

// ErrBlockedAffiliateLink is returned when blocked affiliate link is detected
var ErrBlockedAffiliateLink = errors.New("다른 사용자의 수익 링크(naver.me 등)는 등록할 수 없습니다")

// Boards that block third-party affiliate links
var blockedAffiliateBoards = []string{"deal", "economy"}

// Blocked affiliate link domains
var blockedAffiliateDomains = []string{
	"naver.me",
	"m.site.naver.com",
	"link.naver.com",
	"smartstore.naver.com",
}

// URL pattern to extract links from content
var urlPattern = regexp.MustCompile(`https?://[^\s<>"']+`)

// containsBlockedDomain checks if content contains any blocked domain links
func containsBlockedDomain(content string, blockedDomains []string) bool {
	// Find all URLs in content
	urls := urlPattern.FindAllString(content, -1)

	for _, url := range urls {
		urlLower := strings.ToLower(url)
		for _, domain := range blockedDomains {
			if strings.Contains(urlLower, domain) {
				return true
			}
		}
	}
	return false
}

// ValidateAffiliateLinks validates content for blocked affiliate links
// Returns error if blocked links are found
//
// Parameters:
//   - content: post or comment content to validate
//   - boardID: board slug (e.g., "deal", "economy")
//   - memberLevel: user's member level (10+ is admin)
//   - isAdsContent: true if content is from ads system (always allowed)
func ValidateAffiliateLinks(content, boardID string, memberLevel int, isAdsContent bool) error {
	// Skip if not a blocked board
	if !slices.Contains(blockedAffiliateBoards, boardID) {
		return nil
	}

	// Admin users (level 10+) are exempt
	if memberLevel >= 10 {
		return nil
	}

	// Ads system content is exempt
	if isAdsContent {
		return nil
	}

	// Check for blocked domains
	if containsBlockedDomain(content, blockedAffiliateDomains) {
		return ErrBlockedAffiliateLink
	}

	return nil
}

// IsBlockedAffiliateBoard checks if a board blocks third-party affiliate links
func IsBlockedAffiliateBoard(boardID string) bool {
	return slices.Contains(blockedAffiliateBoards, boardID)
}

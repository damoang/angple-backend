package common

import (
	"testing"
)

func TestValidateAffiliateLinks(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		boardID     string
		memberLevel int
		isAds       bool
		wantErr     bool
	}{
		{
			name:        "naver.me on deal board - should block",
			content:     "Check this out: https://naver.me/abc123",
			boardID:     "deal",
			memberLevel: 1,
			isAds:       false,
			wantErr:     true,
		},
		{
			name:        "naver.me on economy board - should block",
			content:     "Visit https://naver.me/xyz789 now!",
			boardID:     "economy",
			memberLevel: 5,
			isAds:       false,
			wantErr:     true,
		},
		{
			name:        "naver.me on free board - should allow",
			content:     "https://naver.me/test",
			boardID:     "free",
			memberLevel: 1,
			isAds:       false,
			wantErr:     false,
		},
		{
			name:        "naver.me on deal board - admin should allow",
			content:     "https://naver.me/admin",
			boardID:     "deal",
			memberLevel: 10,
			isAds:       false,
			wantErr:     false,
		},
		{
			name:        "naver.me on deal board - ads should allow",
			content:     "https://naver.me/ads",
			boardID:     "deal",
			memberLevel: 1,
			isAds:       true,
			wantErr:     false,
		},
		{
			name:        "link.naver.com on deal board - should block",
			content:     "https://link.naver.com/abc",
			boardID:     "deal",
			memberLevel: 1,
			isAds:       false,
			wantErr:     true,
		},
		{
			name:        "regular content on deal board - should allow",
			content:     "Just some text without blocked links",
			boardID:     "deal",
			memberLevel: 1,
			isAds:       false,
			wantErr:     false,
		},
		{
			name:        "coupang link on deal board - should allow",
			content:     "https://www.coupang.com/product/123",
			boardID:     "deal",
			memberLevel: 1,
			isAds:       false,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAffiliateLinks(tt.content, tt.boardID, tt.memberLevel, tt.isAds)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAffiliateLinks() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

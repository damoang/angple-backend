package v2

import "testing"

func TestIsEligibleForAutoPromotion(t *testing.T) {
	tests := []struct {
		name      string
		level     int
		loginDays int
		exp       int
		certify   string
		want      bool
	}{
		{
			name:      "eligible certified member",
			level:     2,
			loginDays: 7,
			exp:       3000,
			certify:   "simple",
			want:      true,
		},
		{
			name:      "rejects uncertified member",
			level:     2,
			loginDays: 10,
			exp:       12000,
			certify:   "",
			want:      false,
		},
		{
			name:      "rejects wrong level",
			level:     3,
			loginDays: 10,
			exp:       12000,
			certify:   "simple",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEligibleForAutoPromotion(tt.level, tt.loginDays, tt.exp, tt.certify)
			if got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}

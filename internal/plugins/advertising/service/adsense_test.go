package service

import (
	"testing"
)

func TestGetRotationIndex(t *testing.T) {
	tests := []struct {
		name       string
		sessionKey string
		maxSlots   int
		wantStable bool // 같은 세션 키로 여러 번 호출 시 동일한 결과
	}{
		{
			name:       "empty session key returns random index",
			sessionKey: "",
			maxSlots:   8,
			wantStable: false,
		},
		{
			name:       "session key produces stable index",
			sessionKey: "test-session-123",
			maxSlots:   8,
			wantStable: true,
		},
		{
			name:       "different session keys produce different indices",
			sessionKey: "different-session",
			maxSlots:   8,
			wantStable: true,
		},
		{
			name:       "maxSlots=1 always returns 0",
			sessionKey: "any-key",
			maxSlots:   1,
			wantStable: true,
		},
	}

	svc := &adsenseService{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := svc.GetRotationIndex(tt.sessionKey, tt.maxSlots)

			// 인덱스가 범위 내인지 확인
			if got < 0 || got >= tt.maxSlots {
				t.Errorf("GetRotationIndex() = %v, want index in range [0, %d)", got, tt.maxSlots)
			}

			// 안정성 검증
			if tt.wantStable && tt.sessionKey != "" {
				for i := 0; i < 10; i++ {
					second := svc.GetRotationIndex(tt.sessionKey, tt.maxSlots)
					if second != got {
						t.Errorf("GetRotationIndex() not stable: first=%d, second=%d", got, second)
						break
					}
				}
			}
		})
	}
}

func TestGetRotationIndex_EdgeCases(t *testing.T) {
	svc := &adsenseService{}

	// maxSlots=0 또는 음수
	t.Run("zero maxSlots returns 0", func(t *testing.T) {
		got := svc.GetRotationIndex("any-key", 0)
		if got != 0 {
			t.Errorf("GetRotationIndex() with maxSlots=0 = %v, want 0", got)
		}
	})

	t.Run("negative maxSlots returns 0", func(t *testing.T) {
		got := svc.GetRotationIndex("any-key", -5)
		if got != 0 {
			t.Errorf("GetRotationIndex() with negative maxSlots = %v, want 0", got)
		}
	})
}

func TestGetRotationIndex_Distribution(t *testing.T) {
	svc := &adsenseService{}
	maxSlots := 8
	iterations := 1000

	// 다양한 세션 키로 인덱스 분포 확인
	counts := make([]int, maxSlots)
	for i := 0; i < iterations; i++ {
		sessionKey := "session-" + string(rune(i+'0'))
		index := svc.GetRotationIndex(sessionKey, maxSlots)
		counts[index]++
	}

	// 각 인덱스가 최소 한 번 이상 선택되었는지 확인
	for i, count := range counts {
		if count == 0 {
			t.Errorf("Index %d was never selected in %d iterations", i, iterations)
		}
	}
}

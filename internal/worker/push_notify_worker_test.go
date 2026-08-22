package worker

import (
	"testing"

	gnurepo "github.com/damoang/angple-backend/internal/repository/gnuboard"
)

func TestPushAppURL(t *testing.T) {
	tests := []struct {
		name string
		n    gnurepo.Notification
		want string
	}{
		{
			name: "쪽지는 메시지함으로",
			n:    gnurepo.Notification{PhFromCase: "memo"},
			want: "/messages",
		},
		{
			name: "글 대상(wr_parent 없음)은 글 상세로",
			n:    gnurepo.Notification{BoTable: "free", WrID: 123},
			want: "/post/free/123",
		},
		{
			name: "댓글 대상은 부모 글 + commentId 쿼리",
			n:    gnurepo.Notification{BoTable: "free", WrID: 456, WrParent: 123},
			want: "/post/free/123?commentId=456",
		},
		{
			name: "보드 없으면 빈 문자열",
			n:    gnurepo.Notification{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pushAppURL(tt.n); got != tt.want {
				t.Errorf("pushAppURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

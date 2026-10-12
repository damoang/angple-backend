package handler

import "testing"

func TestKeywordNotificationTypeAndTitle(t *testing.T) {
	const keywordTitle = "등록한 키워드가 들어간 새 글이 올라왔습니다"

	if got := mapNotificationType("write", "keyword", 1, 1); got != "keyword" {
		t.Errorf("write+keyword type = %q", got)
	}
	if got := mapNotificationType("write", "subscribe", 1, 1); got != "subscribe" {
		t.Errorf("write+subscribe type = %q", got)
	}
	if got := mapNotificationType("write", "follow", 1, 1); got != "follow" {
		t.Errorf("write+follow type = %q", got)
	}
	if got := generateTitle("write", "keyword", "작성자", 1, 1); got != keywordTitle {
		t.Errorf("keyword title = %q", got)
	}
	if got := generateGroupTitle("write", "keyword", "작성자", 3, 1, 1); got != keywordTitle {
		t.Errorf("keyword group title = %q", got)
	}
	if got := generateTitle("write", "follow", "작성자", 1, 1); got != "작성자님이 새 글을 작성했습니다" {
		t.Errorf("follow title changed: %q", got)
	}
}

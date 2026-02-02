package plugin

import (
	"testing"
)

func TestHookConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant string
		expected string
	}{
		// Post hooks
		{"HookPostBeforeCreate", HookPostBeforeCreate, "post.before_create"},
		{"HookPostAfterCreate", HookPostAfterCreate, "post.after_create"},
		{"HookPostBeforeUpdate", HookPostBeforeUpdate, "post.before_update"},
		{"HookPostAfterUpdate", HookPostAfterUpdate, "post.after_update"},
		{"HookPostBeforeDelete", HookPostBeforeDelete, "post.before_delete"},
		{"HookPostAfterDelete", HookPostAfterDelete, "post.after_delete"},
		{"HookPostContent", HookPostContent, "post.content"},
		// Comment hooks
		{"HookCommentBeforeCreate", HookCommentBeforeCreate, "comment.before_create"},
		{"HookCommentAfterCreate", HookCommentAfterCreate, "comment.after_create"},
		{"HookCommentBeforeUpdate", HookCommentBeforeUpdate, "comment.before_update"},
		{"HookCommentAfterUpdate", HookCommentAfterUpdate, "comment.after_update"},
		{"HookCommentBeforeDelete", HookCommentBeforeDelete, "comment.before_delete"},
		{"HookCommentAfterDelete", HookCommentAfterDelete, "comment.after_delete"},
		{"HookCommentContent", HookCommentContent, "comment.content"},
		// User hooks
		{"HookUserAfterLogin", HookUserAfterLogin, "user.after_login"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("%s = %q, want %q", tt.name, tt.constant, tt.expected)
			}
		})
	}
}

func TestHookManagerDoUserAfterLogin(t *testing.T) {
	hm := newTestHookManager()

	var captured map[string]interface{}
	hm.Register(HookUserAfterLogin, "test-plugin", func(ctx *HookContext) error {
		captured = ctx.Input
		return nil
	}, 10)

	hm.Do(HookUserAfterLogin, map[string]interface{}{
		"user_id":  "testuser",
		"nickname": "테스트",
		"level":    10,
	})

	if captured == nil {
		t.Fatal("hook handler was not called")
	}
	if captured["user_id"] != "testuser" {
		t.Errorf("user_id = %v, want %q", captured["user_id"], "testuser")
	}
	if captured["nickname"] != "테스트" {
		t.Errorf("nickname = %v, want %q", captured["nickname"], "테스트")
	}
	if captured["level"] != 10 {
		t.Errorf("level = %v, want %d", captured["level"], 10)
	}
}


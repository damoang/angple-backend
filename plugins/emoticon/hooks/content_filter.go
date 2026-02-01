//go:build ignore

// 이모티콘 콘텐츠 필터 Hook
package hooks

import (
	"angple-backend/pkg/plugin"
	"angple-backend/plugins/emoticon/service"
)

var svc *service.Service

// Register Hook 등록 및 서비스 초기화
func Register(ctx *plugin.Context, s *service.Service) {
	svc = s
}

// FilterPostContent 게시글 본문 필터 - {emo:...} → <img> 변환
func FilterPostContent(ctx *plugin.HookContext) error {
	content, ok := ctx.Input["content"].(string)
	if !ok || content == "" {
		return nil
	}

	filtered := svc.ParseContent(content)
	ctx.SetOutput("content", filtered)
	return nil
}

// FilterCommentContent 댓글 본문 필터 - {emo:...} → <img> 변환
func FilterCommentContent(ctx *plugin.HookContext) error {
	content, ok := ctx.Input["content"].(string)
	if !ok || content == "" {
		return nil
	}

	filtered := svc.ParseContent(content)
	ctx.SetOutput("content", filtered)
	return nil
}

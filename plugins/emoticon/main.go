//go:build ignore

// 이모티콘 플러그인
// Emoticon Plugin - {emo:filename:width} 코드를 <img> 태그로 변환
package main

import (
	"angple-backend/plugins/emoticon/handlers"
	"angple-backend/plugins/emoticon/hooks"
	"angple-backend/plugins/emoticon/service"

	"angple-backend/pkg/plugin"
)

func init() {
	plugin.Register("emoticon", &plugin.Plugin{
		OnInit: func(ctx *plugin.Context) error {
			// 설정 로드
			config := service.DefaultConfig()
			if v, ok := ctx.Settings["default_width"].(int); ok {
				config.DefaultWidth = v
			}
			if v, ok := ctx.Settings["max_width"].(int); ok {
				config.MaxWidth = v
			}
			if v, ok := ctx.Settings["cdn_url"].(string); ok && v != "" {
				config.CDNURL = v
			}
			if v, ok := ctx.Settings["fallback_filename"].(string); ok && v != "" {
				config.FallbackFilename = v
			}
			if v, ok := ctx.Settings["assets_path"].(string); ok && v != "" {
				config.AssetsPath = v
			}

			// 서비스 초기화
			svc := service.NewService(ctx.DB, config)

			// 핸들러/훅에 서비스 주입
			handlers.SetService(svc)
			hooks.Register(ctx, svc)

			return nil
		},
		OnShutdown: func(ctx *plugin.Context) error {
			return nil
		},
		Handlers: map[string]plugin.HandlerFunc{
			// 공개 API
			"ListPacks":     handlers.ListPacks,
			"ListPackItems": handlers.ListPackItems,
			"ServeImage":    handlers.ServeImage,
			"ServeThumb":    handlers.ServeThumb,

			// 관리자 API
			"AdminListPacks": handlers.AdminListPacks,
			"CreatePack":     handlers.CreatePack,
			"UpdatePack":     handlers.UpdatePack,
			"DeletePack":     handlers.DeletePack,
			"TogglePack":     handlers.TogglePack,
			"ImportLegacy":   handlers.ImportLegacy,
		},
		HookHandlers: map[string]plugin.HookFunc{
			"FilterPostContent":    hooks.FilterPostContent,
			"FilterCommentContent": hooks.FilterCommentContent,
		},
	})
}

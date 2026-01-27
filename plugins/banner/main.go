// 배너 광고 플러그인
// Banner Advertising Plugin
package main

import (
	"angple-backend/plugins/banner/handlers"
	"angple-backend/plugins/banner/hooks"

	"angple-backend/pkg/plugin"
)

func init() {
	plugin.Register("banner", &plugin.Plugin{
		OnInit: func(ctx *plugin.Context) error {
			// Hook 등록
			hooks.Register(ctx)
			return nil
		},
		OnShutdown: func(ctx *plugin.Context) error {
			return nil
		},
		Handlers: map[string]plugin.HandlerFunc{
			// 공개 API
			"ListBanners": handlers.ListBanners,
			"TrackClick":  handlers.TrackClick,
			"TrackView":   handlers.TrackView,

			// 관리자 API
			"AdminListBanners": handlers.AdminListBanners,
			"CreateBanner":     handlers.CreateBanner,
			"UpdateBanner":     handlers.UpdateBanner,
			"DeleteBanner":     handlers.DeleteBanner,
			"GetBannerStats":   handlers.GetBannerStats,
		},
		HookHandlers: map[string]plugin.HookFunc{
			"AddHeaderBanner":    hooks.AddHeaderBanner,
			"AddSidebarBanner":   hooks.AddSidebarBanner,
			"AddFooterBanner":    hooks.AddFooterBanner,
			"InsertContentBanner": hooks.InsertContentBanner,
			"AddAdminMenu":       hooks.AddAdminMenu,
		},
	})
}

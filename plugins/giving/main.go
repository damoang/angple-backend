//go:build ignore

// 나눔 플러그인
// Giving Plugin
package main

import (
	"angple-backend/plugins/giving/handlers"
	"angple-backend/plugins/giving/hooks"

	"angple-backend/pkg/plugin"
)

func init() {
	plugin.Register("giving", &plugin.Plugin{
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
			"ListGivings":    handlers.ListGivings,
			"GetGivingDetail": handlers.GetGivingDetail,
			"GetVisualization": handlers.GetVisualization,
			"GetLiveStatus":   handlers.GetLiveStatus,

			// 인증 필요 API
			"CreateBid": handlers.CreateBid,
			"GetMyBids": handlers.GetMyBids,

			// 관리자 API
			"PauseGiving":    handlers.PauseGiving,
			"ResumeGiving":   handlers.ResumeGiving,
			"ForceStopGiving": handlers.ForceStopGiving,
			"GetAdminStats":  handlers.GetAdminStats,
		},
		HookHandlers: map[string]plugin.HookFunc{
			"AddAdminMenu": hooks.AddAdminMenu,
		},
	})
}

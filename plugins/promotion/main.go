//go:build ignore

package main

import (
	"github.com/angple/core/plugin"
	"github.com/damoang/angple-backend/plugins/promotion/handlers"
	"github.com/damoang/angple-backend/plugins/promotion/hooks"
)

func init() {
	plugin.Register("promotion", &plugin.Plugin{
		OnInit: func(ctx *plugin.Context) error {
			// Hook 등록
			hooks.Register(ctx)
			return nil
		},
		OnActivate: func(ctx *plugin.Context) error {
			// 플러그인 활성화 시 실행
			return nil
		},
		OnDeactivate: func(ctx *plugin.Context) error {
			// 플러그인 비활성화 시 실행
			return nil
		},
		Handlers: map[string]plugin.HandlerFunc{
			// 공개 API
			"ListPosts":         handlers.ListPosts,
			"GetPostsForInsert": handlers.GetPostsForInsert,
			"GetPost":           handlers.GetPost,

			// 광고주 전용 API
			"CreatePost": handlers.CreatePost,
			"UpdatePost": handlers.UpdatePost,
			"DeletePost": handlers.DeletePost,

			// 관리자 API
			"ListAdvertisers":  handlers.ListAdvertisers,
			"CreateAdvertiser": handlers.CreateAdvertiser,
			"UpdateAdvertiser": handlers.UpdateAdvertiser,
			"DeleteAdvertiser": handlers.DeleteAdvertiser,
		},
	})
}

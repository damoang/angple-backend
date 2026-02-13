//go:build ignore

// 나눔 플러그인 Hook 구현
package hooks

import (
	"angple-backend/pkg/plugin"
)

var settings map[string]interface{}

// Register Hook 등록
func Register(ctx *plugin.Context) {
	settings = ctx.Settings
}

// AddAdminMenu 관리자 메뉴에 나눔 관리 추가
func AddAdminMenu(ctx *plugin.HookContext) error {
	menus, ok := ctx.Input["menus"].([]map[string]interface{})
	if !ok {
		menus = []map[string]interface{}{}
	}

	menus = append(menus, map[string]interface{}{
		"id":       "giving",
		"title":    "나눔 관리",
		"icon":     "gift",
		"path":     "/admin/plugins/giving",
		"priority": 40,
	})

	ctx.SetOutput("menus", menus)
	return nil
}

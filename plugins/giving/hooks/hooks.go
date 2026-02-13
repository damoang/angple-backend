// 나눔 플러그인 Hook 구현
package hooks

import (
	"github.com/damoang/angple-backend/internal/plugin"
)

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

	ctx.SetOutput(map[string]interface{}{
		"menus": menus,
	})
	return nil
}

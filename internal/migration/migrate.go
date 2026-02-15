package migration

import (
	"github.com/damoang/angple-backend/internal/domain"
	pluginstoreDomain "github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
)

// Run executes AutoMigrate for core tables and seeds default data if empty.
func Run(db *gorm.DB) error {
	if err := db.AutoMigrate(&domain.Menu{}, &domain.MemberBlock{}, &pluginstoreDomain.PluginInstallation{}, &pluginstoreDomain.PluginSetting{}, &pluginstoreDomain.PluginEvent{}, &pluginstoreDomain.PluginPermission{}, &pluginstoreDomain.PluginMigration{}); err != nil {
		return err
	}

	var count int64
	db.Model(&domain.Menu{}).Count(&count)
	if count == 0 {
		return seedMenus(db)
	}

	return nil
}

func seedMenus(db *gorm.DB) error {
	int64Ptr := func(v int64) *int64 { return &v }

	menus := []domain.Menu{
		// Community
		{ID: 1, ParentID: nil, Title: "Community", URL: "/community", Icon: "MessageSquare", Shortcut: "", Description: "Community", Depth: 1, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: true, ShowInSidebar: true},
		{ID: 2, ParentID: int64Ptr(1), Title: "Free Board", URL: "/free", Icon: "CircleStar", Shortcut: "F", Description: "Free Board", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 3, ParentID: int64Ptr(1), Title: "Q&A", URL: "/qa", Icon: "CircleHelp", Shortcut: "Q", Description: "Q&A", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 4, ParentID: int64Ptr(1), Title: "Gallery", URL: "/gallery", Icon: "Images", Shortcut: "G", Description: "Gallery", Depth: 2, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
	}

	if err := db.Create(&menus).Error; err != nil {
		return err
	}

	db.Exec("ALTER TABLE `menus` AUTO_INCREMENT = 100")

	return nil
}

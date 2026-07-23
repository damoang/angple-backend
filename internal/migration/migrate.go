package migration

import (
	"fmt"

	"github.com/damoang/angple-backend/internal/domain"
	gnudomain "github.com/damoang/angple-backend/internal/domain/gnuboard"
	v2domain "github.com/damoang/angple-backend/internal/domain/v2"
	pluginstoreDomain "github.com/damoang/angple-backend/internal/pluginstore/domain"
	"gorm.io/gorm"
)

// Run executes AutoMigrate for core tables and seeds default data if empty.
func Run(db *gorm.DB) error {
	if err := db.AutoMigrate(&domain.Menu{}, &domain.MemberBlock{}, &v2domain.SiteLogo{}, &pluginstoreDomain.PluginInstallation{}, &pluginstoreDomain.PluginSetting{}, &pluginstoreDomain.PluginEvent{}, &pluginstoreDomain.PluginPermission{}, &pluginstoreDomain.PluginMigration{}, &gnudomain.MemberLevelHistory{}); err != nil {
		return err
	}

	// #12916: block_scope 컬럼 도입 후 기존 차단 레코드를 전체 차단("all")로 백필(idempotent).
	// AutoMigrate 가 기존 행 default 를 채우지 못하는 MySQL 버전 대비.
	if db.Migrator().HasColumn(&domain.MemberBlock{}, "block_scope") {
		if err := db.Model(&domain.MemberBlock{}).
			Where("block_scope IS NULL OR block_scope = ''").
			Update("block_scope", domain.BlockScopeAll).Error; err != nil {
			return err
		}
	}

	var count int64
	db.Model(&domain.Menu{}).Count(&count)
	if count == 0 {
		if err := seedMenus(db); err != nil {
			return err
		}
	}

	// 이후 단계는 모두 "실행하고 실패하면 중단" 형태라 목록으로 접는다.
	// 같은 모양의 if 문이 늘어서면 순환복잡도만 올라가고(gocyclo), 정작 어느 단계에서
	// 실패했는지는 알 수 없었다. 이름을 함께 들고 다니며 에러에 붙인다.
	//
	// 순서 의존성이 있으므로 배열 순서를 임의로 바꾸지 말 것.
	steps := []struct {
		name string
		fn   func(*gorm.DB) error
	}{
		{"AddSoftDeleteColumnsToAllBoards", AddSoftDeleteColumnsToAllBoards},
		{"CreateWriteRevisionsTable", CreateWriteRevisionsTable},
		{"CreateScheduledDeletesTable", CreateScheduledDeletesTable},
		{"CreateWriteAfterEventsTable", CreateWriteAfterEventsTable},
		{"CreateAffiliateLinksTable", CreateAffiliateLinksTable},
		{"CreateAnniversaryDrawEntriesTable", CreateAnniversaryDrawEntriesTable},
		{"FixListPageIndexes", FixListPageIndexes},
		{"AddListDeletedIndexes", AddListDeletedIndexes},
		{"AddDisciplineLogPenaltyMbIDColumn", AddDisciplineLogPenaltyMbIDColumn},
		{"AddRestrictionScopeColumn", AddRestrictionScopeColumn},
		{"ExpandSiteLogoRecurringDateColumn", ExpandSiteLogoRecurringDateColumn},
		{"WidenCommentReplyColumns", WidenCommentReplyColumns},
	}
	for _, s := range steps {
		if err := s.fn(db); err != nil {
			return fmt.Errorf("migration %s: %w", s.name, err)
		}
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

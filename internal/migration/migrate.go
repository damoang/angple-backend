package migration

import (
	"github.com/damoang/angple-backend/internal/domain"
	"gorm.io/gorm"
)

// Run executes AutoMigrate for the menus table and seeds default data if empty.
func Run(db *gorm.DB) error {
	// 1. AutoMigrate - 테이블 없으면 생성, 있으면 skip
	if err := db.AutoMigrate(&domain.Menu{}); err != nil {
		return err
	}

	// 2. Seed - menus 테이블이 비어있을 때만 기본 메뉴 삽입
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
		// 1. 커뮤니티 (루트)
		{ID: 1, ParentID: nil, Title: "커뮤니티", URL: "/community", Icon: "MessageSquare", Shortcut: "", Description: "커뮤니티", Depth: 1, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: true, ShowInSidebar: true},
		// 1-1. 커뮤니티 하위
		{ID: 2, ParentID: int64Ptr(1), Title: "자유게시판", URL: "/free", Icon: "CircleStar", Shortcut: "F", Description: "자유게시판", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 3, ParentID: int64Ptr(1), Title: "질문과 답변", URL: "/qa", Icon: "CircleHelp", Shortcut: "Q", Description: "질문과 답변", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 4, ParentID: int64Ptr(1), Title: "갤러리", URL: "/gallery", Icon: "Images", Shortcut: "G", Description: "갤러리", Depth: 2, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 2. 소모임 (루트)
		{ID: 10, ParentID: nil, Title: "소모임", URL: "/groups", Icon: "Users", Shortcut: "", Description: "소모임", Depth: 1, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: true, ShowInSidebar: true},
		// 2-1. 소모임 하위
		{ID: 11, ParentID: int64Ptr(10), Title: "소모임 모아보기", URL: "/groups/all", Icon: "", Shortcut: "", Description: "소모임 모아보기", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 12, ParentID: int64Ptr(10), Title: "소모임 목록", URL: "/groups/list", Icon: "Users", Shortcut: "", Description: "소모임 목록", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		// 2-2. 소모임 하위 (주요 소모임)
		{ID: 13, ParentID: int64Ptr(12), Title: "AI당", URL: "/groups/ai", Icon: "Cpu", Shortcut: "", Description: "AI당", Depth: 3, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 14, ParentID: int64Ptr(12), Title: "개발한당", URL: "/groups/development", Icon: "Code", Shortcut: "", Description: "개발한당", Depth: 3, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 15, ParentID: int64Ptr(12), Title: "게임한당", URL: "/groups/game", Icon: "Gamepad2", Shortcut: "", Description: "게임한당", Depth: 3, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 16, ParentID: int64Ptr(12), Title: "공부한당", URL: "/groups/study", Icon: "BookOpen", Shortcut: "", Description: "공부한당", Depth: 3, OrderNum: 4, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 17, ParentID: int64Ptr(12), Title: "애플모앙", URL: "/groups/apple", Icon: "Apple", Shortcut: "", Description: "애플모앙", Depth: 3, OrderNum: 5, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 3. 새로운 소식 (루트)
		{ID: 20, ParentID: nil, Title: "새로운 소식", URL: "/news", Icon: "Newspaper", Shortcut: "N", Description: "새로운 소식", Depth: 1, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: true, ShowInSidebar: true},
		// 3-1. 새로운 소식 하위
		{ID: 21, ParentID: int64Ptr(20), Title: "사용기", URL: "/tutorial", Icon: "PenTool", Shortcut: "T", Description: "사용기", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 22, ParentID: int64Ptr(20), Title: "강좌/팁", URL: "/lecture", Icon: "Lightbulb", Shortcut: "L", Description: "강좌/팁", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 23, ParentID: int64Ptr(20), Title: "자료실", URL: "/pds", Icon: "FolderOpen", Shortcut: "P", Description: "자료실", Depth: 2, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 4. 리뷰 (루트)
		{ID: 30, ParentID: nil, Title: "리뷰", URL: "/reviews", Icon: "Gift", Shortcut: "", Description: "리뷰", Depth: 1, OrderNum: 4, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		// 4-1. 리뷰 하위
		{ID: 31, ParentID: int64Ptr(30), Title: "앙지도", URL: "/reviews/map", Icon: "MapPin", Shortcut: "M", Description: "다모앙 지도", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 32, ParentID: int64Ptr(30), Title: "앙티티", URL: "/reviews/rating", Icon: "Star", Shortcut: "O", Description: "다모앙 평점", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 33, ParentID: int64Ptr(30), Title: "수익링크", URL: "/reviews/referral", Icon: "TrendingUp", Shortcut: "", Description: "수익링크", Depth: 2, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 5. 알뜰구매 (루트)
		{ID: 40, ParentID: nil, Title: "알뜰구매", URL: "/economy", Icon: "ShoppingCart", Shortcut: "E", Description: "알뜰구매", Depth: 1, OrderNum: 5, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		// 5-1. 알뜰구매 하위
		{ID: 41, ParentID: int64Ptr(40), Title: "직접홍보", URL: "/economy/promotion", Icon: "Megaphone", Shortcut: "W", Description: "직접홍보", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 6. BETA (루트)
		{ID: 50, ParentID: nil, Title: "BETA", URL: "/beta", Icon: "Sparkles", Shortcut: "", Description: "BETA 기능", Depth: 1, OrderNum: 6, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		// 6-1. BETA 하위
		{ID: 51, ParentID: int64Ptr(50), Title: "다모앙 레시피", URL: "/beta/recipe", Icon: "Coffee", Shortcut: "", Description: "다모앙 레시피", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 52, ParentID: int64Ptr(50), Title: "다모앙 전문가", URL: "/beta/expert", Icon: "UserCheck", Shortcut: "", Description: "다모앙 전문가", Depth: 2, OrderNum: 2, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 53, ParentID: int64Ptr(50), Title: "다모앙 음악", URL: "/beta/music", Icon: "Music", Shortcut: "D", Description: "다모앙 음악", Depth: 2, OrderNum: 3, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},

		// 7. 안내 (루트)
		{ID: 60, ParentID: nil, Title: "안내", URL: "/info", Icon: "Info", Shortcut: "", Description: "안내", Depth: 1, OrderNum: 7, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		// 7-1. 안내 하위
		{ID: 61, ParentID: int64Ptr(60), Title: "게시판 안내", URL: "/info/board", Icon: "HelpCircle", Shortcut: "", Description: "게시판 안내", Depth: 2, OrderNum: 1, IsActive: true, Target: "_self", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
		{ID: 62, ParentID: int64Ptr(60), Title: "위키앙", URL: "https://wikiang.wiki", Icon: "BookText", Shortcut: "", Description: "위키앙", Depth: 2, OrderNum: 2, IsActive: true, Target: "_blank", ViewLevel: 1, ShowInHeader: false, ShowInSidebar: true},
	}

	if err := db.Create(&menus).Error; err != nil {
		return err
	}

	// AUTO_INCREMENT 재설정
	db.Exec("ALTER TABLE `menus` AUTO_INCREMENT = 100")

	return nil
}

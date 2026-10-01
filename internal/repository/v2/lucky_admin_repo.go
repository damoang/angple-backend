package v2

import (
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// luckyConfigHistoryMax 는 site_settings 에 남기는 lucky_config 변경 이력 개수다.
// 이력은 settings_json 안에 있어 설정을 읽을 때마다 같이 읽히므로 무한히 늘리지 않는다.
const luckyConfigHistoryMax = 50

// luckyBoardExtTable 은 게시판 확장설정 테이블이다.
const luckyBoardExtTable = "v2_board_extended_settings"

// luckyDefaultActiveTheme 는 site_settings 행이 없어 새로 만들 때 쓰는 테마다.
// 컬럼 기본값(damoang-official)에 맡기면 운영 테마가 바뀌므로 명시한다.
const luckyDefaultActiveTheme = "damoang-default"

// LuckyConfigHistoryEntry 는 lucky_config 변경 이력 한 건이다. Before·After 는 저장된 JSON 그대로다.
// By 는 변경한 관리자의 mb_id 다(관리자 API 에만 나간다).
type LuckyConfigHistoryEntry struct {
	At     string          `json:"at"` // KST RFC3339
	By     string          `json:"by"`
	Before json.RawMessage `json:"before"`
	After  json.RawMessage `json:"after"`
}

// LuckyBoardSetting 은 관리자 화면에 보여 줄 게시판 하나의 럭키 설정이다. Lucky 가 nil 이면 설정 키 자체가 없다.
type LuckyBoardSetting struct {
	BoardID string      `json:"board_id"`
	Subject string      `json:"subject"`
	Lucky   *BoardLucky `json:"lucky"`
}

// LuckyGrantDetail 은 통계용 원장 한 건에 내역 문구·경험치·닉네임을 붙인 것이다.
// MbID 는 회원 수를 세는 데만 쓰고 응답에는 싣지 않는다(서비스가 Nick 만 내보낸다).
type LuckyGrantDetail struct {
	ID           int64
	MbID         string
	Nick         string
	SourceTable  string
	SourceID     string
	Kind         string
	Amount       int
	Exp          int
	PointContent string
	ExpContent   string
	CreatedAt    time.Time
}

// LuckyAdminRepository 는 관리자 화면용 읽기·쓰기다. 지급 경로(LuckyRepository)와 분리해 둔 것은
// 관리자 쓰기가 지급 경로의 인터페이스·테스트 가짜를 넓히지 않게 하려는 것이다.
type LuckyAdminRepository interface {
	// GetStoredLuckyConfig 는 캐시를 거치지 않고 저장된 lucky_config(기본값 채움)와 변경 이력(오래된 순),
	// 저장된 lucky_config 원문(없으면 nil)을 읽는다. 원문은 관리자 화면이 관대한 파싱에 가려진 오류를 알아채게 하려는 것이다.
	GetStoredLuckyConfig() (*LuckyConfig, []LuckyConfigHistoryEntry, json.RawMessage, error)
	// SaveLuckyConfig 는 lucky_config 를 통째로 바꾸고 같은 트랜잭션에서 이력을 남긴 뒤 이 파드의 캐시를 비운다.
	SaveLuckyConfig(cfg *LuckyConfig, by string, now time.Time) error
	// ListBoardLucky 는 모든 게시판과 각 게시판의 lucky 설정을 돌려준다.
	ListBoardLucky() ([]LuckyBoardSetting, error)
	// ExistingBoardIDs 는 ids 중 g5_board 에 있는 것만 돌려준다.
	ExistingBoardIDs(ids []string) (map[string]bool, error)
	// SetBoardLucky 는 각 게시판 확장설정의 lucky 키만 바꾸고(행이 없으면 만든다) 바뀐 settings 전체를 돌려준다.
	SetBoardLucky(ids []string, lucky BoardLucky, now time.Time) (map[string]string, error)
	// ListLuckyGrantsBetween 은 [start, end) 원장 행을 내역 문구·경험치·닉네임과 함께 최신순으로 돌려준다.
	ListLuckyGrantsBetween(start, end time.Time) ([]LuckyGrantDetail, error)
}

type luckyAdminRepository struct {
	db *gorm.DB
}

// NewLuckyAdminRepository 는 LuckyAdminRepository 를 만든다.
func NewLuckyAdminRepository(db *gorm.DB) LuckyAdminRepository {
	return &luckyAdminRepository{db: db}
}

// luckyJSONParam 은 바인딩한 JSON 문자열을 문자열이 아닌 JSON 값으로 넣는 SQL 조각이다.
// MySQL 은 CAST(? AS JSON), 테스트용 sqlite 는 json(?) 이다 — 그냥 ? 로 넣으면 JSON 안에 따옴표 문자열로 들어간다.
func luckyJSONParam(db *gorm.DB) string {
	if db.Dialector != nil && db.Name() == "sqlite" {
		return "json(?)"
	}
	return "CAST(? AS JSON)"
}

// luckyJSONObjectOr 는 col 이 JSON 객체면 그대로, NULL·'null'·스칼라면 빈 객체를 쓰는 SQL 조각이다.
// JSON_SET 은 NULL 이나 스칼라에는 키를 만들지 못하고 조용히 넘어가므로(저장이 사라진다) 먼저 객체로 맞춘다.
// JSON_TYPE 은 MySQL 이 'OBJECT', sqlite 가 'object' 를 돌려줘 UPPER 로 맞춘다.
func luckyJSONObjectOr(col string) string {
	return "CASE WHEN UPPER(JSON_TYPE(" + col + ")) = 'OBJECT' THEN " + col + " ELSE JSON_OBJECT() END"
}

// GetStoredLuckyConfig 는 저장된 설정과 이력, lucky_config 원문을 읽는다. 행이 없으면 기본값·빈 이력·nil 원문이다.
// 설정 값은 지급 경로와 같은 관대한 파싱(UnmarshalJSON)을 거친다 — 화면에 보이는 값이 실제 적용 값과 같게 하려는 것이다.
func (r *luckyAdminRepository) GetStoredLuckyConfig() (*LuckyConfig, []LuckyConfigHistoryEntry, json.RawMessage, error) {
	var rows []siteSettingsJSON
	if err := r.db.Select("settings_json").Where("site_id = ?", defaultSiteID).Find(&rows).Error; err != nil {
		return nil, nil, nil, err
	}
	if len(rows) == 0 {
		return DefaultLuckyConfig(), nil, nil, nil
	}
	top := parseSettingsTop(rows[0].SettingsJSON)
	cfg := DefaultLuckyConfig()
	var stored json.RawMessage
	if raw, ok := top["lucky_config"]; ok && len(raw) > 0 && string(raw) != nullJSON {
		stored = raw
		if err := json.Unmarshal(raw, cfg); err != nil {
			// 저장값이 JSON 으로도 깨졌으면 지급 경로(getLuckyConfigFromDB)와 같게 기본값(꺼짐)으로 보여 준다.
			// 에러로 돌려주면 관리자 화면이 열리지 않아 고쳐 저장할 길이 막힌다.
			cfg = DefaultLuckyConfig()
		}
	}
	var hist []LuckyConfigHistoryEntry
	if raw, ok := top["lucky_config_history"]; ok {
		if err := json.Unmarshal(raw, &hist); err != nil {
			hist = nil
		}
	}
	return cfg, hist, stored, nil
}

// parseSettingsTop 은 settings_json 을 최상위 키별 원본 JSON 으로 나눈다. 비었거나 깨졌으면 빈 맵이다.
func parseSettingsTop(s *string) map[string]json.RawMessage {
	top := map[string]json.RawMessage{}
	if s == nil || *s == "" || *s == nullJSON {
		return top
	}
	if err := json.Unmarshal([]byte(*s), &top); err != nil || top == nil {
		return map[string]json.RawMessage{}
	}
	return top
}

// SaveLuckyConfig 는 settings_json 의 lucky_config·lucky_config_history 두 경로만 JSON_SET 으로 바꾼다.
//
// 행을 FOR UPDATE 로 잠그고 이전 값을 읽어 이력을 만든 뒤 같은 트랜잭션에서 쓴다 — 두 관리자가 동시에 저장해도
// 이력의 before 가 실제 직전 값과 어긋나지 않는다. 다른 키(xp_config 등)는 SQL 이 건드리지 않으므로 보존된다.
// 행이 없으면 active_theme 를 명시해 새로 만든다. 커밋 뒤 이 파드의 캐시를 비운다(다른 파드는 TTL 안에 따라온다).
func (r *luckyAdminRepository) SaveLuckyConfig(cfg *LuckyConfig, by string, now time.Time) error {
	if cfg == nil {
		return errors.New("lucky: nil config")
	}
	after, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	now = now.Truncate(time.Second)
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var rows []siteSettingsJSON
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Select("settings_json").Where("site_id = ?", defaultSiteID).
			Find(&rows).Error; err != nil {
			return err
		}
		top := map[string]json.RawMessage{}
		if len(rows) > 0 {
			top = parseSettingsTop(rows[0].SettingsJSON)
		}
		histJSON, err := appendLuckyHistory(top, after, by, now)
		if err != nil {
			return err
		}
		jp := luckyJSONParam(tx)
		if len(rows) == 0 {
			return tx.Exec(
				"INSERT INTO site_settings (site_id, settings_json, active_theme, created_at, updated_at) VALUES (?, JSON_OBJECT('lucky_config', "+jp+", 'lucky_config_history', "+jp+"), ?, ?, ?)",
				defaultSiteID, string(after), string(histJSON), luckyDefaultActiveTheme, now, now,
			).Error
		}
		return tx.Exec(
			"UPDATE site_settings SET settings_json = JSON_SET("+luckyJSONObjectOr("settings_json")+", '$.lucky_config', "+jp+", '$.lucky_config_history', "+jp+"), updated_at = ? WHERE site_id = ?",
			string(after), string(histJSON), now, defaultSiteID,
		).Error
	})
	if err != nil {
		return err
	}
	InvalidateLuckyConfigCache()
	return nil
}

// appendLuckyHistory 는 기존 이력에 {at, by, before, after} 한 건을 붙이고 최근 luckyConfigHistoryMax 개만 남긴 JSON 배열을 만든다.
// before 는 지금 저장돼 있는 lucky_config 원본(없으면 null)이다. 깨진 이력은 버리고 새로 시작한다 — 설정 저장 자체를 막지 않는다.
func appendLuckyHistory(top map[string]json.RawMessage, after json.RawMessage, by string, now time.Time) ([]byte, error) {
	before := top["lucky_config"]
	if len(before) == 0 {
		before = json.RawMessage(nullJSON)
	}
	var hist []json.RawMessage
	if raw, ok := top["lucky_config_history"]; ok {
		if err := json.Unmarshal(raw, &hist); err != nil {
			hist = nil
		}
	}
	entry, err := json.Marshal(LuckyConfigHistoryEntry{
		At:     now.In(luckyKST).Format(time.RFC3339),
		By:     by,
		Before: before,
		After:  after,
	})
	if err != nil {
		return nil, err
	}
	hist = append(hist, entry)
	if len(hist) > luckyConfigHistoryMax {
		hist = hist[len(hist)-luckyConfigHistoryMax:]
	}
	return json.Marshal(hist)
}

// boardLuckyRow 는 게시판 목록 조회 행이다.
type boardLuckyRow struct {
	BoTable   string `gorm:"column:bo_table"`
	BoSubject string `gorm:"column:bo_subject"`
}

// extSettingsRow 는 v2_board_extended_settings 조회 행이다.
type extSettingsRow struct {
	BoardID  string  `gorm:"column:board_id"`
	Settings *string `gorm:"column:settings"`
}

// ListBoardLucky 는 g5_board 전체와 확장설정의 lucky 키를 2쿼리로 읽는다(게시판 수만큼 반복하지 않는다).
// 문자열 비교는 Go 에서 한다 — 두 테이블 콜레이션이 달라 SQL 조인이 실패할 수 있다.
func (r *luckyAdminRepository) ListBoardLucky() ([]LuckyBoardSetting, error) {
	var boards []boardLuckyRow
	if err := r.db.Table("g5_board").Select("bo_table, bo_subject").Order("bo_table").Scan(&boards).Error; err != nil {
		return nil, err
	}
	var ext []extSettingsRow
	if err := r.db.Table(luckyBoardExtTable).Select("board_id, settings").Scan(&ext).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]*BoardLucky, len(ext))
	for _, e := range ext {
		if e.Settings == nil || *e.Settings == "" || *e.Settings == nullJSON {
			continue
		}
		var w boardLuckyWrapper
		if err := json.Unmarshal([]byte(*e.Settings), &w); err != nil || w.Lucky == nil {
			continue
		}
		byID[e.BoardID] = w.Lucky
	}
	out := make([]LuckyBoardSetting, 0, len(boards))
	for _, b := range boards {
		out = append(out, LuckyBoardSetting{BoardID: b.BoTable, Subject: b.BoSubject, Lucky: byID[b.BoTable]})
	}
	return out, nil
}

// ExistingBoardIDs 는 ids 중 실제 게시판(g5_board.bo_table)만 골라 1쿼리로 돌려준다.
func (r *luckyAdminRepository) ExistingBoardIDs(ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var found []string
	if err := r.db.Table("g5_board").Where("bo_table IN ?", ids).Pluck("bo_table", &found).Error; err != nil {
		return nil, err
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

// SetBoardLucky 는 한 트랜잭션에서 각 게시판의 lucky 키만 JSON_SET 으로 바꾼다. 다른 키(write_notice 등)는 보존된다.
// 행이 없는 게시판은 {"lucky": ...} 로 새로 만든다. 끝나면 바뀐 settings 전체를 게시판별로 돌려준다
// (호출부가 게시판 캐시 무효화·레거시 파일 동기화를 기존 게시판 설정 저장과 같게 하려는 것이다).
func (r *luckyAdminRepository) SetBoardLucky(ids []string, lucky BoardLucky, now time.Time) (map[string]string, error) {
	if len(ids) == 0 {
		return map[string]string{}, nil
	}
	if lucky.Prizes == nil {
		lucky.Prizes = []LuckyPrize{} // null 이 아니라 빈 표로 저장 — 읽는 쪽은 둘을 같게 보지만 화면이 헷갈리지 않게
	}
	luckyJSON, err := json.Marshal(lucky)
	if err != nil {
		return nil, err
	}
	now = now.Truncate(time.Second)
	var out map[string]string
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var existing []string
		if err := tx.Table(luckyBoardExtTable).
			Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("board_id IN ?", ids).
			Pluck("board_id", &existing).Error; err != nil {
			return err
		}
		has := make(map[string]bool, len(existing))
		for _, id := range existing {
			has[id] = true
		}
		for _, id := range ids {
			if err := upsertBoardLuckyTx(tx, id, has[id], string(luckyJSON), now); err != nil {
				return err
			}
		}
		var rerr error
		out, rerr = readBoardSettings(tx, ids)
		return rerr
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// upsertBoardLuckyTx 는 게시판 하나의 lucky 키를 바꾸거나(행 있음) {"lucky": ...} 로 행을 만든다(행 없음).
func upsertBoardLuckyTx(tx *gorm.DB, id string, exists bool, luckyJSON string, now time.Time) error {
	jp := luckyJSONParam(tx)
	if exists {
		return tx.Exec(
			"UPDATE "+luckyBoardExtTable+" SET settings = JSON_SET("+luckyJSONObjectOr("settings")+", '$.lucky', "+jp+"), updated_at = ? WHERE board_id = ?",
			luckyJSON, now, id,
		).Error
	}
	return tx.Exec(
		"INSERT INTO "+luckyBoardExtTable+" (board_id, settings, created_at, updated_at) VALUES (?, JSON_OBJECT('lucky', "+jp+"), ?, ?)",
		id, luckyJSON, now, now,
	).Error
}

// readBoardSettings 는 ids 게시판의 settings 전체를 읽는다(바뀐 뒤 후처리에 넘길 값).
func readBoardSettings(tx *gorm.DB, ids []string) (map[string]string, error) {
	var rows []extSettingsRow
	if err := tx.Table(luckyBoardExtTable).Select("board_id, settings").
		Where("board_id IN ?", ids).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, row := range rows {
		if row.Settings != nil {
			out[row.BoardID] = *row.Settings
		}
	}
	return out, nil
}

// luckyStatPointRow·luckyStatExpRow·luckyStatNickRow 는 통계 보조 조회 행이다.
type luckyStatPointRow struct {
	PoID      int64  `gorm:"column:po_id"`
	PoContent string `gorm:"column:po_content"`
}

type luckyStatExpRow struct {
	RelTable string `gorm:"column:xp_rel_table"`
	RelID    string `gorm:"column:xp_rel_id"`
	Point    int    `gorm:"column:xp_point"`
	Content  string `gorm:"column:xp_content"`
}

type luckyStatNickRow struct {
	MbID   string `gorm:"column:mb_id"`
	MbNick string `gorm:"column:mb_nick"`
}

// ListLuckyGrantsBetween 은 [start, end) 원장 행과 보조 정보를 고정 4쿼리(원장·포인트·경험치·닉네임)로 읽는다.
//
// 시각 비교는 지급이 created_at 을 쓸 때와 같은 time.Time 바인딩이다 — DSN 시간대가 무엇이든 하루 경계가 같다.
// 원장(utf8mb4_unicode_ci)과 g5_point·g5_na_xp·g5_member 의 콜레이션이 달라 문자열 조인이 실패할 수 있어
// SQL 조인 대신 IN 조회 후 Go 에서 붙인다. 당첨은 하루 수십 건 수준이라 IN 목록이 작다.
func (r *luckyAdminRepository) ListLuckyGrantsBetween(start, end time.Time) ([]LuckyGrantDetail, error) {
	var ledger []LuckyGrant
	if err := r.db.Where("created_at >= ? AND created_at < ?", start, end).
		Order("created_at DESC, id DESC").Find(&ledger).Error; err != nil {
		return nil, err
	}
	if len(ledger) == 0 {
		return nil, nil
	}
	poIDs := make([]int64, 0, len(ledger))
	tables := map[string]struct{}{}
	relIDs := map[string]struct{}{}
	members := map[string]struct{}{}
	for _, g := range ledger {
		if g.PoID != nil {
			poIDs = append(poIDs, *g.PoID)
		}
		tables[g.SourceTable] = struct{}{}
		relIDs[g.SourceID] = struct{}{}
		members[g.MbID] = struct{}{}
	}
	pointContent, err := r.luckyPointContents(poIDs)
	if err != nil {
		return nil, err
	}
	expByRel, err := r.luckyExpByRel(keysOf(tables), keysOf(relIDs))
	if err != nil {
		return nil, err
	}
	nicks, err := r.memberNicks(keysOf(members))
	if err != nil {
		return nil, err
	}

	out := make([]LuckyGrantDetail, 0, len(ledger))
	for _, g := range ledger {
		d := LuckyGrantDetail{
			ID:          g.ID,
			MbID:        g.MbID,
			Nick:        nicks[g.MbID],
			SourceTable: g.SourceTable,
			SourceID:    g.SourceID,
			Kind:        g.Kind,
			Amount:      g.Amount,
			CreatedAt:   g.CreatedAt,
		}
		if g.PoID != nil {
			d.PointContent = pointContent[*g.PoID]
		}
		if x, ok := expByRel[luckyRelKey{g.SourceTable, g.SourceID}]; ok {
			d.Exp, d.ExpContent = x.Point, x.Content
		}
		out = append(out, d)
	}
	return out, nil
}

// luckyRelKey 는 (게시판, wr_id) 묶음이다.
type luckyRelKey struct{ table, id string }

// luckyPointContents 는 po_id 별 내역 문구를 읽는다(빈 목록이면 0쿼리).
func (r *luckyAdminRepository) luckyPointContents(poIDs []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(poIDs) == 0 {
		return out, nil
	}
	var rows []luckyStatPointRow
	if err := r.db.Table("g5_point").Select("po_id, po_content").Where("po_id IN ?", poIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, p := range rows {
		out[p.PoID] = p.PoContent
	}
	return out, nil
}

// luckyExpByRel 은 @lucky 경험치 행을 (게시판, wr_id) 별로 읽는다. 같은 키가 여럿이면 큰 값(배지와 같은 MAX).
// 게시판·wr_id 를 따로 IN 으로 걸러 교차 조합이 섞여 오지만, 키로 맞춰 붙이므로 다른 게시판 행이 섞이지 않는다.
func (r *luckyAdminRepository) luckyExpByRel(tables, relIDs []string) (map[luckyRelKey]luckyStatExpRow, error) {
	out := map[luckyRelKey]luckyStatExpRow{}
	var rows []luckyStatExpRow
	if err := r.db.Table("g5_na_xp").Select("xp_rel_table, xp_rel_id, xp_point, xp_content").
		Where("xp_rel_action = ? AND xp_rel_table IN ? AND xp_rel_id IN ?", luckyRelAction, tables, relIDs).
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, x := range rows {
		k := luckyRelKey{x.RelTable, x.RelID}
		if prev, ok := out[k]; !ok || x.Point > prev.Point {
			out[k] = x
		}
	}
	return out, nil
}

// memberNicks 는 mb_id 별 닉네임을 읽는다. 통계 응답에 회원 ID 대신 싣기 위한 것이다.
func (r *luckyAdminRepository) memberNicks(mbIDs []string) (map[string]string, error) {
	out := map[string]string{}
	var rows []luckyStatNickRow
	if err := r.db.Table("g5_member").Select("mb_id, mb_nick").Where("mb_id IN ?", mbIDs).Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, m := range rows {
		out[m.MbID] = m.MbNick
	}
	return out, nil
}

// keysOf 는 집합의 키를 슬라이스로 바꾼다(IN 바인딩용).
func keysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

package gnuboard

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// joinOr joins SQL conditions with OR
func joinOr(conditions []string) string {
	return strings.Join(conditions, " OR ")
}

// Notification represents a row in g5_na_noti table
type Notification struct {
	PhID          int       `gorm:"column:ph_id;primaryKey"`
	PhToCase      string    `gorm:"column:ph_to_case"`
	PhFromCase    string    `gorm:"column:ph_from_case"`
	BoTable       string    `gorm:"column:bo_table"`
	WrID          int       `gorm:"column:wr_id"`
	MbID          string    `gorm:"column:mb_id"`
	RelMbID       string    `gorm:"column:rel_mb_id"`
	RelMbNick     string    `gorm:"column:rel_mb_nick"`
	RelMsg        string    `gorm:"column:rel_msg"`
	RelURL        string    `gorm:"column:rel_url"`
	PhReaded      string    `gorm:"column:ph_readed"`
	PhDatetime    time.Time `gorm:"column:ph_datetime"`
	ParentSubject string    `gorm:"column:parent_subject"`
	WrParent      int       `gorm:"column:wr_parent"`
}

// TableName returns the g5_na_noti table name
func (Notification) TableName() string { return "g5_na_noti" }

// GroupedNotification represents a group of notifications for the same post+type
type GroupedNotification struct {
	BoTable    string `gorm:"column:bo_table"`
	WrID       int    `gorm:"column:wr_id"`
	PhFromCase string `gorm:"column:ph_from_case"`
	// PhToCase·WrParent 는 **표시 전용** 세부 종류다(bug#13242).
	//
	// ph_from_case 만으로는 구분이 안 되는 조합이 있다 — 30일 실측:
	//   write + subscribe      914,410건  구독 게시판 새 글
	//   write + follow          97,405건  팔로우한 분의 새 글
	//   comment + comment      131,461건  내 글에 달린 댓글
	//   comment + comment_reply 32,904건  내 댓글에 달린 답글
	//
	// write 계열 101만건(전체의 52%)이 한 덩어리로 묶여 회색 기본 아이콘으로 표시됐다.
	//
	// 공감은 ph_to_case 도 똑같이 'good' 이라 갈리지 않는다. 대신 wr_parent 로 가른다 —
	// 3일 실측에서 완전히 분리된다:
	//   wr_parent == wr_id  106,236건 전부 "회원님의 글을 추천했습니다"
	//   wr_parent != wr_id   68,127건 전부 "회원님의 댓글을 추천했습니다"
	// 같은 집합을 rel_url LIKE '%#c_%' 로 세도 정확히 일치한다(독립 신호 2개 합치).
	//
	// ⛔ **그룹 키에는 넣지 않는다.** 그룹 키는 (bo_table, wr_id, ph_from_case) 이고
	//    읽음 처리·삭제가 같은 키로 동작한다. 여기에 ph_to_case 를 더하면 묶음이
	//    쪼개져 읽음/삭제가 어긋난다. 그래서 그룹 대표 행(latest)의 값을 그대로
	//    실어 보내기만 한다 — 쿼리도 그룹 로직도 바뀌지 않는다.
	//
	// ⚠️ 한 그룹 안에서 ph_to_case 가 섞이는 경우가 실제로 있다 (7일 실측:
	//    comment 1,022 그룹, write 306 그룹). 예컨대 게시판을 구독하면서 글쓴이를
	//    팔로우도 하면 같은 글에 두 알림이 온다. 이때는 **가장 최근 행의 종류**를
	//    보여준다 — 묶음 행의 시각·본문이 이미 최신 행 기준이라 그쪽이 일관된다.
	//    good 의 wr_parent 는 섞이는 그룹이 0건이라 이 문제가 없다.
	PhToCase      string    `gorm:"column:ph_to_case"`
	WrParent      int       `gorm:"column:wr_parent"`
	LatestPhID    int       `gorm:"column:latest_ph_id"`
	LatestAt      time.Time `gorm:"column:latest_at"`
	SenderCount   int       `gorm:"column:sender_count"`
	UnreadCount   int       `gorm:"column:unread_count"`
	LatestSender  string    `gorm:"column:latest_sender"`
	Senders       string    `gorm:"column:senders"`
	RelURL        string    `gorm:"column:rel_url"`
	ParentSubject string    `gorm:"column:parent_subject"`
	RelMsg        string    `gorm:"column:rel_msg"`
}

// NotiRepository provides access to g5_na_noti table
type NotiRepository interface {
	GetNotifications(mbID string, page, limit int) ([]Notification, int64, error)
	GetGroupedNotifications(mbID string, page, limit int, filterType string) ([]GroupedNotification, int64, int64, error)
	CountUnread(mbID string) (int64, error)
	MarkAsRead(mbID string, phID int) error
	MarkAllAsRead(mbID string) error
	MarkGroupAsRead(mbID, boTable string, wrID int, fromCase string) error
	// 대상 항목(글/댓글) 단위 통합 묶음 — 같은 글의 좋아요·댓글이 한 줄이 된다
	GetMergedNotifications(mbID string, page, limit int, filterType string) ([]MergedNotification, int64, int64, error)
	MarkMergedGroupAsRead(mbID, boTable, targetKey string) error
	DeleteMergedGroup(mbID, boTable, targetKey string) error
	Delete(mbID string, phID int) error
	DeleteGroup(mbID, boTable string, wrID int, fromCase string) error
	Create(noti *Notification) error
	Exists(mbID, boTable string, wrID int, fromCase, relMbID string) (bool, error)
}

type notiRepository struct {
	db *gorm.DB
}

// NewNotiRepository creates a new NotiRepository for g5_na_noti
func NewNotiRepository(db *gorm.DB) NotiRepository {
	return &notiRepository{db: db}
}

// GetNotifications returns notifications for the user with pagination
func (r *notiRepository) GetNotifications(mbID string, page, limit int) ([]Notification, int64, error) {
	var notifications []Notification
	var total int64

	query := r.db.Model(&Notification{}).Where("mb_id = ?", mbID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("ph_id DESC").Offset(offset).Limit(limit).Find(&notifications).Error; err != nil {
		return nil, 0, err
	}

	return notifications, total, nil
}

// CountUnread returns the count of unread notifications for the user
func (r *notiRepository) CountUnread(mbID string) (int64, error) {
	var count int64
	err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&count).Error
	return count, err
}

// MarkAsRead marks a single notification as read
func (r *notiRepository) MarkAsRead(mbID string, phID int) error {
	return r.db.Model(&Notification{}).
		Where("ph_id = ? AND mb_id = ?", phID, mbID).
		Update("ph_readed", "Y").Error
}

// MarkAllAsRead marks all unread notifications as read for the user
func (r *notiRepository) MarkAllAsRead(mbID string) error {
	return r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Update("ph_readed", "Y").Error
}

// Delete deletes a notification for the user
func (r *notiRepository) Delete(mbID string, phID int) error {
	return r.db.Where("ph_id = ? AND mb_id = ?", phID, mbID).
		Delete(&Notification{}).Error
}

// Create inserts a new notification
func (r *notiRepository) Create(noti *Notification) error {
	return r.db.Create(noti).Error
}

// Exists checks if a duplicate unread notification already exists
func (r *notiRepository) Exists(mbID, boTable string, wrID int, fromCase, relMbID string) (bool, error) {
	var count int64
	err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ? AND rel_mb_id = ? AND ph_readed = 'N'", mbID, boTable, wrID, fromCase, relMbID).
		Count(&count).Error
	return count > 0, err
}

// GetGroupedNotifications returns notifications grouped by (bo_table, wr_id, ph_from_case)
// Optimized: 2-pass approach — first identify top N groups (lightweight), then enrich only those groups
func (r *notiRepository) GetGroupedNotifications(mbID string, page, limit int, filterType string) ([]GroupedNotification, int64, int64, error) {
	// Build filter condition.
	// 'reaction' 타입은 항상 제외 — 이모지 리액션 알림은 운영 정책상 비활성.
	// ph_from_case='reaction' 행이 DB에 존재하지만 사용자에게 노출하지 않는다.
	fromCaseFilter := "AND ph_from_case != 'reaction'"
	switch filterType {
	case "comment":
		fromCaseFilter = "AND ph_from_case IN ('board', 'comment', 'reply')"
	case "like":
		fromCaseFilter = "AND ph_from_case = 'good'"
	case "mention":
		fromCaseFilter = "AND ph_from_case = 'mention'"
	case "memo":
		fromCaseFilter = "AND ph_from_case = 'memo'"
	case "system":
		fromCaseFilter = "AND ph_from_case IN ('write', 'inquire', 'answer')"
	}

	// Count total unread (fast — uses idx_mb_readed index)
	var unreadCount int64
	if err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&unreadCount).Error; err != nil {
		return nil, 0, 0, err
	}

	// Pass 1: Identify top N groups by MAX(ph_id) — lightweight, no GROUP_CONCAT
	// 후보는 최근 notiGroupWindow 행으로 좁힌다(상수 주석 참조). 인덱스(mb_id, ph_id)
	// 역순 스캔 + LIMIT 라 무거운 계정에서도 상한이 잡힌다.
	offset := (page - 1) * limit
	topGroupsSQL := `SELECT
		bo_table, wr_id, ph_from_case,
		MAX(ph_id) as latest_ph_id,
		COUNT(*) as sender_count
	FROM (
		SELECT ph_id, bo_table, wr_id, ph_from_case
		FROM g5_na_noti FORCE INDEX (idx_mb_phid)
		WHERE mb_id = ? ` + fromCaseFilter + `
		ORDER BY ph_id DESC
		LIMIT ?
	) w
	GROUP BY bo_table, wr_id, ph_from_case
	ORDER BY latest_ph_id DESC
	LIMIT ? OFFSET ?`

	type topGroup struct {
		BoTable     string `gorm:"column:bo_table"`
		WrID        int    `gorm:"column:wr_id"`
		PhFromCase  string `gorm:"column:ph_from_case"`
		LatestPhID  int    `gorm:"column:latest_ph_id"`
		SenderCount int    `gorm:"column:sender_count"`
	}
	var tops []topGroup
	if err := r.db.Raw(topGroupsSQL, mbID, notiGroupWindow, limit, offset).Scan(&tops).Error; err != nil {
		return nil, 0, 0, err
	}

	if len(tops) == 0 {
		return []GroupedNotification{}, 0, unreadCount, nil
	}

	// Estimate total groups from first page (avoid expensive COUNT subquery)
	var totalGroups int64
	if page == 1 && len(tops) < limit {
		totalGroups = int64(len(tops))
	} else {
		// ⛔ 창 없이 세면 이 COUNT 가 다시 전체 행 GROUP BY 가 되어(무거운 계정 수백 ms)
		//    Pass 1 을 좁힌 의미가 없어진다. 같은 창 기준으로 센다.
		countSQL := `SELECT COUNT(*) FROM (
			SELECT 1 FROM (
				SELECT bo_table, wr_id, ph_from_case
				FROM g5_na_noti FORCE INDEX (idx_mb_phid)
				WHERE mb_id = ? ` + fromCaseFilter + `
				ORDER BY ph_id DESC
				LIMIT ?
			) w
			GROUP BY bo_table, wr_id, ph_from_case
		) t`
		if err := r.db.Raw(countSQL, mbID, notiGroupWindow).Scan(&totalGroups).Error; err != nil {
			return nil, 0, 0, err
		}
		// 창이 페이지 요청을 다 못 덮었는데 이번 페이지가 꽉 찼다면, 다음 페이지가
		// 존재하도록 보정한다(정확한 총수보다 "더보기가 끊기지 않는 것"이 중요).
		if len(tops) == limit && totalGroups < int64(offset+len(tops)+1) {
			totalGroups = int64(offset + len(tops) + 1)
		}
	}

	// Pass 2a: Get the latest notification row for each group (PK lookup)
	phIDs := make([]int, len(tops))
	for i, t := range tops {
		phIDs[i] = t.LatestPhID
	}

	var latestRows []Notification
	if err := r.db.Where("ph_id IN ?", phIDs).Find(&latestRows).Error; err != nil {
		return nil, 0, 0, err
	}
	latestMap := make(map[int]Notification, len(latestRows))
	for _, row := range latestRows {
		latestMap[row.PhID] = row
	}

	// Pass 2b: Get unread counts per group (only if there are unread notifications)
	type unreadResult struct {
		BoTable    string `gorm:"column:bo_table"`
		WrID       int    `gorm:"column:wr_id"`
		PhFromCase string `gorm:"column:ph_from_case"`
		Cnt        int    `gorm:"column:cnt"`
	}
	var unreadResults []unreadResult
	if unreadCount > 0 {
		conditions := make([]string, 0, len(tops))
		args := make([]interface{}, 0, len(tops)*3+1)
		args = append(args, mbID)
		for _, t := range tops {
			conditions = append(conditions, "(bo_table = ? AND wr_id = ? AND ph_from_case = ?)")
			args = append(args, t.BoTable, t.WrID, t.PhFromCase)
		}
		unreadSQL := `SELECT bo_table, wr_id, ph_from_case, COUNT(*) as cnt
			FROM g5_na_noti
			WHERE mb_id = ? AND ph_readed = 'N' AND (` + joinOr(conditions) + `)
			GROUP BY bo_table, wr_id, ph_from_case`
		r.db.Raw(unreadSQL, args...).Scan(&unreadResults)
	}
	unreadMap := make(map[string]int, len(unreadResults))
	for _, u := range unreadResults {
		key := u.BoTable + "|" + fmt.Sprintf("%d", u.WrID) + "|" + u.PhFromCase
		unreadMap[key] = u.Cnt
	}

	// Pass 2c: Get senders (skip GROUP_CONCAT for single-sender groups)
	senderMap := make(map[string]string, len(tops))
	var multiSenderTops []topGroup
	for _, t := range tops {
		key := t.BoTable + "|" + fmt.Sprintf("%d", t.WrID) + "|" + t.PhFromCase
		if t.SenderCount <= 1 {
			if latest, ok := latestMap[t.LatestPhID]; ok {
				senderMap[key] = latest.RelMbNick
			}
		} else {
			multiSenderTops = append(multiSenderTops, t)
		}
	}

	if len(multiSenderTops) > 0 {
		type senderResult struct {
			BoTable    string `gorm:"column:bo_table"`
			WrID       int    `gorm:"column:wr_id"`
			PhFromCase string `gorm:"column:ph_from_case"`
			Senders    string `gorm:"column:senders"`
		}
		senderConditions := make([]string, 0, len(multiSenderTops))
		senderArgs := make([]interface{}, 0, len(multiSenderTops)*3+1)
		senderArgs = append(senderArgs, mbID)
		for _, t := range multiSenderTops {
			senderConditions = append(senderConditions, "(bo_table = ? AND wr_id = ? AND ph_from_case = ?)")
			senderArgs = append(senderArgs, t.BoTable, t.WrID, t.PhFromCase)
		}
		var senderResults []senderResult
		sendersSQL := `SELECT bo_table, wr_id, ph_from_case,
			SUBSTRING_INDEX(GROUP_CONCAT(DISTINCT rel_mb_nick ORDER BY ph_datetime DESC SEPARATOR '||'), '||', 5) as senders
			FROM g5_na_noti
			WHERE mb_id = ? AND (` + joinOr(senderConditions) + `)
			GROUP BY bo_table, wr_id, ph_from_case`
		r.db.Raw(sendersSQL, senderArgs...).Scan(&senderResults)
		for _, s := range senderResults {
			key := s.BoTable + "|" + fmt.Sprintf("%d", s.WrID) + "|" + s.PhFromCase
			senderMap[key] = s.Senders
		}
	}

	// Assemble results in the same order as tops
	groups := make([]GroupedNotification, 0, len(tops))
	for _, t := range tops {
		latest, ok := latestMap[t.LatestPhID]
		if !ok {
			continue
		}
		key := t.BoTable + "|" + fmt.Sprintf("%d", t.WrID) + "|" + t.PhFromCase
		uc := unreadMap[key]
		senders := senderMap[key]

		groups = append(groups, GroupedNotification{
			BoTable:    t.BoTable,
			WrID:       t.WrID,
			PhFromCase: t.PhFromCase,
			// 그룹 대표(최신) 행의 표시용 필드를 그대로 싣는다(bug#13242).
			// 섞인 그룹에서는 최신 행 기준 — 위 GroupedNotification 주석 참조.
			PhToCase:      latest.PhToCase,
			WrParent:      latest.WrParent,
			LatestPhID:    t.LatestPhID,
			LatestAt:      latest.PhDatetime,
			SenderCount:   t.SenderCount,
			UnreadCount:   uc,
			LatestSender:  latest.RelMbNick,
			Senders:       senders,
			RelURL:        latest.RelURL,
			ParentSubject: latest.ParentSubject,
			RelMsg:        latest.RelMsg,
		})
	}

	return groups, totalGroups, unreadCount, nil
}

// MarkGroupAsRead marks all notifications in a group as read
func (r *notiRepository) MarkGroupAsRead(mbID, boTable string, wrID int, fromCase string) error {
	return r.db.Model(&Notification{}).
		Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ? AND ph_readed = 'N'", mbID, boTable, wrID, fromCase).
		Update("ph_readed", "Y").Error
}

// DeleteGroup deletes all notifications in a group
func (r *notiRepository) DeleteGroup(mbID, boTable string, wrID int, fromCase string) error {
	return r.db.Where("mb_id = ? AND bo_table = ? AND wr_id = ? AND ph_from_case = ?", mbID, boTable, wrID, fromCase).
		Delete(&Notification{}).Error
}

// targetKeyExpr — 알림을 "대상 항목" 단위로 묶는 파생 키.
//
// 왜 파생인가: 스키마의 wr_id 는 유형마다 의미가 다르다(good=받은 항목, comment=**새 댓글의 id**).
// 같은 글의 댓글 알림들이 wr_id 로는 전부 다른 키가 되어 영영 안 묶인다(30일 실측 압축 1.0×).
// 원글 id 는 rel_url(/{bo}/{post}[#c_{cmt}]) 경로에만 있으므로 거기서 뽑는다.
//
//	p:{post}  내 글 묶음   — good(앵커 없음) + comment(새 댓글)
//	cg:{cmt}  내 댓글 좋아요 — good 인데 rel_url 에 #c_ 앵커(이때 wr_id = 내 댓글)
//	r:{post}  내 댓글 답글  — ph_to_case='comment_reply'. ⚠️글 단위다: 스키마 어디에도
//	          "내 댓글 id"가 없어(wr_id·앵커 모두 새 답글) 댓글 단위 키를 만들 수 없다
//	s:{ph_id} 비묶음(write/memo/mention/digest 등) — 행 그대로 한 줄
//
// notiGroupWindow — 묶음 계산이 훑는 최근 행 수의 상한 (알림 속도 개선, 2026-08-06).
//
// 묶음 쿼리(grouped·merged)는 회원의 **전체** 알림을 GROUP BY 했다. 대부분의 회원은
// 수백 행이라 p95 39ms 였지만, 최다 수신 계정(87,430행·안읽음 87,429)은 **678ms** 다.
// 테이블 전체로도 345만 행 중 안읽음이 239만(69%)이라, 안 읽는 계정의 행은 계속 쌓인다.
//
// 화면은 최신순 한 페이지(20묶음)만 보여주므로, 후보를 **최근 N행**으로 좁혀도
// 표시 결과는 같다 — N이 회원의 전체 행 수보다 크면 문자 그대로 동일하고,
// 초과하는 무거운 계정만 "최근 3,000건 안에서의 묶음"이 된다.
//
// 트레이드오프(의도된 것):
//   - 아주 오래된 행까지 걸친 묶음의 개수 합계("외 N명", 좋아요 n)가 창 안 기준으로
//     집계된다. 표시용 근사치라 화면상 차이는 미미하다.
//   - 전체 묶음 수(totalGroups)도 창 안 기준이 된다. 창이 꽉 찼으면 다음 페이지가
//     항상 있도록 보정한다 — 87k 계정도 창 3,000이면 수백 묶음 = 수십 페이지로,
//     알림함을 그 이상 넘겨 보는 사용은 사실상 없다.
//   - 안읽음 배지(unreadCount)는 별도 인덱스 카운트라 **정확값 그대로**다.
//
// ⛔ 읽음 처리·삭제는 이 창과 무관하다 — 같은 그룹 키로 전체 행에 동작한다.
const notiGroupWindow = 3000

const targetKeyExpr = `CASE
	WHEN ph_to_case = 'comment_reply' THEN CONCAT('r:', SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING_INDEX(rel_url,'#',1),'?',1),'/',-1))
	WHEN ph_from_case = 'good' AND LOCATE('#c_', rel_url) > 0 THEN CONCAT('cg:', wr_id)
	WHEN ph_from_case IN ('good','comment','board','reply') THEN CONCAT('p:', SUBSTRING_INDEX(SUBSTRING_INDEX(SUBSTRING_INDEX(rel_url,'#',1),'?',1),'/',-1))
	ELSE CONCAT('s:', ph_id)
END`

// MergedNotification 은 대상 항목 단위 묶음 한 줄이다.
type MergedNotification struct {
	BoTable      string `gorm:"column:bo_table"`
	TargetKey    string `gorm:"column:tkey"`
	LatestPhID   int    `gorm:"column:latest_ph_id"`
	SenderCount  int    `gorm:"column:sender_count"`
	GoodCount    int    `gorm:"column:good_count"`
	CommentCount int    `gorm:"column:comment_count"`
	ReplyCount   int    `gorm:"column:reply_count"`
	UnreadCount  int    `gorm:"column:unread_count"`
	Senders      string `gorm:"column:senders"`
	// 아래는 최신 행(pass 2)에서 채운다
	LatestAt     time.Time `gorm:"-"`
	LatestSender string    `gorm:"-"`
	// 비묶음(s:) 항목의 세부 종류 판별용 — GroupedNotification 의 PhToCase 주석 참조.
	LatestFromCase string `gorm:"-"`
	LatestToCase   string `gorm:"-"`
	LatestWrParent int    `gorm:"-"`
	RelURL         string `gorm:"-"`
	ParentSubject  string `gorm:"-"`
	RelMsg         string `gorm:"-"`
	WrID           int    `gorm:"-"`
}

// GetMergedNotifications — GetGroupedNotifications 와 같은 2-pass 구조에 키만 대상 단위다.
// 기존 메서드는 손대지 않는다(구 소비자 보존). merge=target 파라미터로만 이 경로에 들어온다.
func (r *notiRepository) GetMergedNotifications(mbID string, page, limit int, filterType string) ([]MergedNotification, int64, int64, error) {
	fromCaseFilter := "AND ph_from_case != 'reaction'"
	switch filterType {
	case "comment":
		fromCaseFilter = "AND ph_from_case IN ('board', 'comment', 'reply')"
	case "like":
		fromCaseFilter = "AND ph_from_case = 'good'"
	case "mention":
		fromCaseFilter = "AND ph_from_case = 'mention'"
	case "memo":
		fromCaseFilter = "AND ph_from_case = 'memo'"
	case "system":
		fromCaseFilter = "AND ph_from_case IN ('write', 'inquire', 'answer')"
	}

	var unreadCount int64
	if err := r.db.Model(&Notification{}).
		Where("mb_id = ? AND ph_readed = 'N'", mbID).
		Count(&unreadCount).Error; err != nil {
		return nil, 0, 0, err
	}

	offset := (page - 1) * limit
	// senders 는 최신순 닉 5명까지만 — GROUP_CONCAT 전량은 좋아요 폭주 글에서 수백 명이 된다
	// 후보는 최근 notiGroupWindow 행으로 좁힌다(상수 주석 참조).
	topSQL := `SELECT
		bo_table, ` + targetKeyExpr + ` AS tkey,
		MAX(ph_id) AS latest_ph_id,
		COUNT(*) AS sender_count,
		SUM(ph_from_case = 'good') AS good_count,
		SUM(ph_from_case IN ('comment','board','reply') AND ph_to_case != 'comment_reply') AS comment_count,
		SUM(ph_to_case = 'comment_reply') AS reply_count,
		SUM(ph_readed = 'N') AS unread_count,
		SUBSTRING_INDEX(GROUP_CONCAT(DISTINCT rel_mb_nick ORDER BY ph_id DESC SEPARATOR ','), ',', 5) AS senders
	FROM (
		SELECT ph_id, bo_table, wr_id, ph_from_case, ph_to_case, ph_readed, rel_mb_nick, rel_url
		FROM g5_na_noti FORCE INDEX (idx_mb_phid)
		WHERE mb_id = ? ` + fromCaseFilter + `
		ORDER BY ph_id DESC
		LIMIT ?
	) w
	GROUP BY bo_table, tkey
	ORDER BY latest_ph_id DESC
	LIMIT ? OFFSET ?`

	var groups []MergedNotification
	if err := r.db.Raw(topSQL, mbID, notiGroupWindow, limit, offset).Scan(&groups).Error; err != nil {
		return nil, 0, 0, err
	}
	if len(groups) == 0 {
		return []MergedNotification{}, 0, unreadCount, nil
	}

	var totalGroups int64
	if page == 1 && len(groups) < limit {
		totalGroups = int64(len(groups))
	} else {
		// ⛔ 창 없이 세면 전체 행 GROUP BY 로 되돌아간다 — grouped 쪽과 같은 창 기준.
		countSQL := `SELECT COUNT(*) FROM (
			SELECT 1 FROM (
				SELECT ph_id, bo_table, wr_id, ph_from_case, ph_to_case, rel_url
				FROM g5_na_noti FORCE INDEX (idx_mb_phid)
				WHERE mb_id = ? ` + fromCaseFilter + `
				ORDER BY ph_id DESC
				LIMIT ?
			) w
			GROUP BY bo_table, ` + targetKeyExpr + `
		) t`
		if err := r.db.Raw(countSQL, mbID, notiGroupWindow).Scan(&totalGroups).Error; err != nil {
			return nil, 0, 0, err
		}
		if len(groups) == limit && totalGroups < int64(offset+len(groups)+1) {
			totalGroups = int64(offset + len(groups) + 1)
		}
	}

	// pass 2: 최신 행에서 표시 재료(URL·제목 스냅샷·최신 닉)를 채운다
	phIDs := make([]int, len(groups))
	for i, g := range groups {
		phIDs[i] = g.LatestPhID
	}
	var latestRows []Notification
	if err := r.db.Where("ph_id IN ?", phIDs).Find(&latestRows).Error; err != nil {
		return nil, 0, 0, err
	}
	latestMap := make(map[int]Notification, len(latestRows))
	for _, row := range latestRows {
		latestMap[row.PhID] = row
	}
	for i := range groups {
		if latest, ok := latestMap[groups[i].LatestPhID]; ok {
			groups[i].LatestAt = latest.PhDatetime
			groups[i].LatestSender = latest.RelMbNick
			groups[i].LatestFromCase = latest.PhFromCase
			groups[i].LatestToCase = latest.PhToCase
			groups[i].LatestWrParent = latest.WrParent
			groups[i].RelURL = latest.RelURL
			groups[i].ParentSubject = latest.ParentSubject
			groups[i].RelMsg = latest.RelMsg
			groups[i].WrID = latest.WrID
		}
	}
	return groups, totalGroups, unreadCount, nil
}

// mergedGroupWhere — 읽음/삭제가 목록과 **같은 키 식**을 쓴다. 식이 갈라지면
// "한 줄을 읽었는데 일부 행이 안읽음으로 남는" 미스매치가 생긴다.
func (r *notiRepository) mergedGroupWhere(mbID, boTable, targetKey string) *gorm.DB {
	return r.db.Model(&Notification{}).
		Where("mb_id = ? AND bo_table = ? AND "+targetKeyExpr+" = ?", mbID, boTable, targetKey)
}

func (r *notiRepository) MarkMergedGroupAsRead(mbID, boTable, targetKey string) error {
	return r.mergedGroupWhere(mbID, boTable, targetKey).
		Where("ph_readed = 'N'").
		Update("ph_readed", "Y").Error
}

func (r *notiRepository) DeleteMergedGroup(mbID, boTable, targetKey string) error {
	return r.mergedGroupWhere(mbID, boTable, targetKey).Delete(&Notification{}).Error
}

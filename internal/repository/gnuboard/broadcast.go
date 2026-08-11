package gnuboard

import "time"

// BroadcastForUser 는 활성 방송 1건과 "이 회원이 읽었는지"를 함께 담는다.
//
// 전 회원 방송은 fan-out-on-read 로 처리한다: na_broadcast 에 1행만 저장하고(발송),
// 회원별 노출/미읽음은 읽을 때 계산한다. 읽은 사람만 na_broadcast_read 에 희소 기록.
// 이렇게 하면 발송이 회원 수만큼의 write 스파이크를 만들지 않고, 취소는 canceled_at
// 1행 UPDATE 로 전원에게 즉시 반영된다.
type BroadcastForUser struct {
	ID        uint      `gorm:"column:id"`
	Title     string    `gorm:"column:title"`
	Body      string    `gorm:"column:body"`
	URL       string    `gorm:"column:url"`
	CreatedAt time.Time `gorm:"column:created_at"`
	Read      bool      `gorm:"column:is_read"`
}

// GetActiveBroadcastsForUser 는 이 회원에게 지금 보여야 할 활성 방송을 최신순으로 준다.
// 활성 = 취소 안 됨(canceled_at IS NULL) + 만료 안 됨(expires_at NULL 또는 미래).
// 활성 방송은 보통 0~수 건이라 매 알림 조회에 붙어도 저렴하다.
func (r *notiRepository) GetActiveBroadcastsForUser(mbID string) ([]BroadcastForUser, error) {
	var rows []BroadcastForUser
	err := r.db.Raw(
		`SELECT b.id, b.title, b.body, b.url, b.created_at,
		        (rd.mb_id IS NOT NULL) AS is_read
		   FROM na_broadcast b
		   LEFT JOIN na_broadcast_read rd
		          ON rd.broadcast_id = b.id AND rd.mb_id = ?
		  WHERE b.canceled_at IS NULL
		    AND (b.expires_at IS NULL OR b.expires_at > NOW())
		  ORDER BY b.id DESC`,
		mbID,
	).Scan(&rows).Error
	return rows, err
}

// CountUnreadBroadcasts 는 이 회원이 아직 안 읽은 활성 방송 수를 준다(뱃지 가산용).
func (r *notiRepository) CountUnreadBroadcasts(mbID string) (int64, error) {
	var n int64
	err := r.db.Raw(
		`SELECT COUNT(*)
		   FROM na_broadcast b
		   LEFT JOIN na_broadcast_read rd
		          ON rd.broadcast_id = b.id AND rd.mb_id = ?
		  WHERE b.canceled_at IS NULL
		    AND (b.expires_at IS NULL OR b.expires_at > NOW())
		    AND rd.mb_id IS NULL`,
		mbID,
	).Scan(&n).Error
	return n, err
}

// MarkBroadcastRead 는 방송 읽음을 멱등 기록한다(INSERT IGNORE). 발송 시엔 행이 없고,
// 회원이 읽거나 닫을 때만 (broadcast_id, mb_id) 1행이 생긴다.
func (r *notiRepository) MarkBroadcastRead(mbID string, broadcastID int) error {
	return r.db.Exec(
		`INSERT IGNORE INTO na_broadcast_read (broadcast_id, mb_id, read_at) VALUES (?, ?, NOW())`,
		broadcastID, mbID,
	).Error
}

// Broadcast 는 관리자 목록/취소용 방송 1건 + 읽음 수.
type Broadcast struct {
	ID         uint       `gorm:"column:id" json:"id"`
	Title      string     `gorm:"column:title" json:"title"`
	Body       string     `gorm:"column:body" json:"body"`
	URL        string     `gorm:"column:url" json:"url"`
	Target     string     `gorm:"column:target" json:"target"`
	CreatedBy  string     `gorm:"column:created_by" json:"created_by"`
	CreatedAt  time.Time  `gorm:"column:created_at" json:"created_at"`
	ExpiresAt  *time.Time `gorm:"column:expires_at" json:"expires_at"`
	CanceledAt *time.Time `gorm:"column:canceled_at" json:"canceled_at"`
	CanceledBy *string    `gorm:"column:canceled_by" json:"canceled_by"`
	ReadCount  int64      `gorm:"column:read_count" json:"read_count"`
}

// CreateBroadcast 는 방송 1행을 넣는다(fan-out-on-read 의 발송 = 이 1행이 전부).
//
// ⛔ 시각은 전부 DB NOW() 로만 만든다(created_at·expires_at). 읽기 머지의 만료 비교도
// DB NOW() 라, Go 시각/타임존을 섞으면 [[feedback-db-utc-now-kst-trap]] 에 걸린다.
// expiresDays<=0 이면 무기한(expires_at NULL).
func (r *notiRepository) CreateBroadcast(title, body, url, target, createdBy string, expiresDays int) error {
	if expiresDays > 0 {
		return r.db.Exec(
			`INSERT INTO na_broadcast (title, body, url, target, created_by, created_at, expires_at)
			 VALUES (?, ?, ?, ?, ?, NOW(), DATE_ADD(NOW(), INTERVAL ? DAY))`,
			title, body, url, target, createdBy, expiresDays,
		).Error
	}
	return r.db.Exec(
		`INSERT INTO na_broadcast (title, body, url, target, created_by, created_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, NOW(), NULL)`,
		title, body, url, target, createdBy,
	).Error
}

// ListBroadcasts 는 최신순 방송 이력 + 각 방송의 읽음 수(na_broadcast_read count)를 준다.
func (r *notiRepository) ListBroadcasts(limit int) ([]Broadcast, error) {
	var rows []Broadcast
	err := r.db.Raw(
		`SELECT b.id, b.title, b.body, b.url, b.target, b.created_by, b.created_at,
		        b.expires_at, b.canceled_at, b.canceled_by,
		        (SELECT COUNT(*) FROM na_broadcast_read rd WHERE rd.broadcast_id = b.id) AS read_count
		   FROM na_broadcast b
		  ORDER BY b.id DESC
		  LIMIT ?`,
		limit,
	).Scan(&rows).Error
	return rows, err
}

// CancelBroadcast 는 canceled_at 1행 UPDATE 로 방송을 즉시 소멸시킨다(전원 반영).
// 이미 취소된 건 건드리지 않는다.
func (r *notiRepository) CancelBroadcast(id uint, canceledBy string) error {
	return r.db.Exec(
		`UPDATE na_broadcast SET canceled_at = NOW(), canceled_by = ?
		  WHERE id = ? AND canceled_at IS NULL`,
		canceledBy, id,
	).Error
}

// CountActiveMembers 는 target='all' 방송의 대상 수(탈퇴·차단 제외 회원)를 준다(도달률 통계용).
func (r *notiRepository) CountActiveMembers() (int64, error) {
	var n int64
	err := r.db.Raw(
		`SELECT COUNT(*) FROM g5_member
		  WHERE (mb_leave_date = '' OR mb_leave_date IS NULL)
		    AND (mb_intercept_date = '' OR mb_intercept_date IS NULL)`,
	).Scan(&n).Error
	return n, err
}

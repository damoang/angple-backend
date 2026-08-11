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

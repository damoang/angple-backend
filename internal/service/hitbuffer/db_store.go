package hitbuffer

import (
	"context"
	"sort"
	"strings"

	"gorm.io/gorm"
)

var _ Store = (*DBStore)(nil)

// DBStore 는 g5_write_{bo_table} 의 wr_hit 를 갱신하는 Store 다.
type DBStore struct {
	db        *gorm.DB
	increment func(boardID string, wrID int) error
}

// NewDBStore 는 DBStore 를 만든다. increment 는 폴백용 즉시 증가 함수
// (기존 WriteRepository.IncrementHit)다.
func NewDBStore(db *gorm.DB, increment func(boardID string, wrID int) error) *DBStore {
	return &DBStore{db: db, increment: increment}
}

// IncrementHit 는 글 하나의 조회수를 즉시 1 올린다.
func (s *DBStore) IncrementHit(boardID string, wrID int) error {
	if !ValidBoardID(boardID) {
		return ErrInvalidBoardID
	}
	return s.increment(boardID, wrID)
}

// AddHits 는 한 게시판의 여러 글 조회수를 UPDATE 한 문장으로 올린다.
//
//	UPDATE `g5_write_x` SET wr_hit = wr_hit + CASE wr_id WHEN ? THEN ? ... ELSE 0 END
//	WHERE wr_id IN (...)
//
// 글 ID 를 오름차순으로 넣어 여러 문장이 겹쳐도 잠금 순서가 같게 한다.
func (s *DBStore) AddHits(ctx context.Context, boardID string, hits map[int]int64) error {
	if !ValidBoardID(boardID) {
		return ErrInvalidBoardID
	}
	if len(hits) == 0 {
		return nil
	}
	ids := make([]int, 0, len(hits))
	for id := range hits {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	var sb strings.Builder
	sb.WriteString("UPDATE `g5_write_")
	sb.WriteString(boardID)
	sb.WriteString("` SET wr_hit = wr_hit + CASE wr_id")
	args := make([]interface{}, 0, len(ids)*2+1)
	for _, id := range ids {
		sb.WriteString(" WHEN ? THEN ?")
		args = append(args, id, hits[id])
	}
	sb.WriteString(" ELSE 0 END WHERE wr_id IN ?")
	args = append(args, ids)

	return s.db.WithContext(ctx).Exec(sb.String(), args...).Error
}

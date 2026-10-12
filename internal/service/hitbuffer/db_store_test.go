package hitbuffer

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newHitTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	require.NoError(t, err)
	require.NoError(t, db.Exec("CREATE TABLE g5_write_free (wr_id INTEGER PRIMARY KEY, wr_hit INTEGER NOT NULL DEFAULT 0)").Error)
	require.NoError(t, db.Exec("INSERT INTO g5_write_free (wr_id, wr_hit) VALUES (1, 10), (2, 0), (3, 7)").Error)
	return db
}

func hitOf(t *testing.T, db *gorm.DB, wrID int) int64 {
	t.Helper()
	var hit int64
	require.NoError(t, db.Raw("SELECT wr_hit FROM g5_write_free WHERE wr_id = ?", wrID).Scan(&hit).Error)
	return hit
}

func TestDBStoreAddHits(t *testing.T) {
	db := newHitTestDB(t)
	s := NewDBStore(db, nil)

	require.NoError(t, s.AddHits(context.Background(), testBoard, map[int]int64{1: 5, 2: 3, 99: 4}))
	assert.Equal(t, int64(15), hitOf(t, db, 1))
	assert.Equal(t, int64(3), hitOf(t, db, 2))
	assert.Equal(t, int64(7), hitOf(t, db, 3), "버퍼에 없는 글은 그대로")

	require.NoError(t, s.AddHits(context.Background(), testBoard, nil))
	assert.ErrorIs(t, s.AddHits(context.Background(), "free` SET wr_hit = 0 --", map[int]int64{1: 1}), ErrInvalidBoardID)
	assert.Equal(t, int64(15), hitOf(t, db, 1))
}

func TestDBStoreIncrementHitDelegates(t *testing.T) {
	var gotBoard string
	var gotID int
	s := NewDBStore(nil, func(boardID string, wrID int) error {
		gotBoard, gotID = boardID, wrID
		return nil
	})
	require.NoError(t, s.IncrementHit(testBoard, 3))
	assert.Equal(t, testBoard, gotBoard)
	assert.Equal(t, 3, gotID)
	assert.ErrorIs(t, s.IncrementHit("bad;name", 3), ErrInvalidBoardID)
}

func TestBufferWithDBStoreEndToEnd(t *testing.T) {
	db := newHitTestDB(t)
	be := newFakeBackend()
	b := New(be, NewDBStore(db, nil), WithChunkSize(1))

	for i := 0; i < 4; i++ {
		require.NoError(t, b.Incr(testBoard, 2))
	}
	require.NoError(t, b.Incr(testBoard, 1))
	assert.Equal(t, int64(0), hitOf(t, db, 2), "flush 전에는 반영되지 않는다")

	require.NoError(t, b.Flush(context.Background()))
	assert.Equal(t, int64(11), hitOf(t, db, 1))
	assert.Equal(t, int64(4), hitOf(t, db, 2))
	assert.Empty(t, be.buffered())
}

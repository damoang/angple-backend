package service

import (
	"os"
	"sort"
	"strings"
	"time"
)

// 공개 수정 이력(public revisions)
//
// 일부 게시판(기본: claim)의 글·댓글 수정 이력을 그 게시판을 읽을 수 있는 모든 사람에게 보여 준다.
// 이력 원본은 g5_write_revisions 이며, 기존 관리자용 이력 API 는 바꾸지 않는다.
//
// g5_write_revisions 의 기록 방식(수정 핸들러 기준):
//   - change_type='create' 행: 최초 작성 내용.
//   - change_type='update' 행: **수정 직전** 내용 + 수정한 사람(edited_by) + 수정 시각(edited_at).
//   - 현재 내용은 이력 테이블이 아니라 게시판 테이블(wr_subject·wr_content)에 있다.
//
// 그래서 「내용 상태(스냅샷)」를 다음처럼 복원한다(update 행 u0..u(n-1), 버전 오름차순):
//
//	스냅샷 k (k < n) = uk.content  — uk 가 기록한 수정 직전의 내용
//	스냅샷 n         = 현재 내용
//	수정 i 는 스냅샷 i → i+1 로 바꾼 것이고, 수정한 사람은 ui.edited_by 다.
//
// 공개 규칙:
//   - 작성자가 운영진이면 아무것도 공개하지 않는다(호출부가 판단해 BuildPublicRevisions 를 부르지 않는다).
//   - 작성자 본인의 수정만 공개한다. 작성자가 아닌 사람의 수정이 하나라도 있으면, 마지막 비작성자
//     수정 **이전**의 스냅샷(그 수정이 지운 내용 포함)은 전부 숨긴다. 비작성자 수정 자체도 드러나지 않는다.
//   - 운영진이 숨긴 버전(hidden)은 뺀다. 현재 내용은 숨길 수 없다(화면에 이미 보인다).
//   - 연속으로 같은 내용(제목·본문 모두 같음)은 하나로 합친다.
//   - 남은 스냅샷이 현재 내용 하나뿐이면 빈 목록이다.

// PublicRevisionCurrentVersion 은 응답에서 「현재 내용」 스냅샷을 가리키는 버전 번호다.
// 실제 이력 행의 버전은 1 이상이라 겹치지 않는다.
const PublicRevisionCurrentVersion = 0

// PublicRevisionConfig 공개 수정 이력 설정
type PublicRevisionConfig struct {
	// Boards 공개 대상 게시판(bo_table). 그 외 게시판은 404.
	Boards map[string]bool
	// StaffIDs 운영진 역할 계정(mb_id). 이 계정이 쓴 글·댓글은 이력을 공개하지 않는다.
	// 사이트 관리자(mb_level>=10)와 해당 게시판 bo_admin 은 설정과 무관하게 운영진으로 본다.
	StaffIDs map[string]bool
}

// LoadPublicRevisionConfig 환경 변수에서 설정을 읽는다.
//
//	PUBLIC_REVISION_BOARDS    쉼표 구분 게시판 목록. 비우면 기본값 "claim".
//	PUBLIC_REVISION_STAFF_IDS 쉼표 구분 운영진 역할 계정(mb_id). 기본값 없음.
func LoadPublicRevisionConfig() PublicRevisionConfig {
	boards := parseIDList(os.Getenv("PUBLIC_REVISION_BOARDS"))
	if len(boards) == 0 {
		boards = map[string]bool{"claim": true}
	}
	return PublicRevisionConfig{
		Boards:   boards,
		StaffIDs: parseIDList(os.Getenv("PUBLIC_REVISION_STAFF_IDS")),
	}
}

func parseIDList(raw string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';'
	}) {
		if p := strings.TrimSpace(part); p != "" {
			out[p] = true
		}
	}
	return out
}

// BoardEnabled 공개 대상 게시판인가.
func (c PublicRevisionConfig) BoardEnabled(slug string) bool {
	return c.Boards[slug]
}

// IsStaffAuthor 작성자가 운영진인가.
// memberLevel 은 작성자의 mb_level, boardAdmin 은 게시판 bo_admin(쉼표 구분 허용)이다.
func (c PublicRevisionConfig) IsStaffAuthor(authorID string, memberLevel int, boardAdmin string) bool {
	if authorID == "" {
		return false
	}
	if memberLevel >= 10 || c.StaffIDs[authorID] {
		return true
	}
	return parseIDList(boardAdmin)[authorID]
}

// RevisionRecord g5_write_revisions 한 행(공개 계산에 필요한 열만).
type RevisionRecord struct {
	Version    int
	ChangeType string
	Title      *string
	Content    *string
	EditedBy   string
	EditedAt   time.Time
}

// PublicRevisionSource 공개 이력 계산 입력.
type PublicRevisionSource struct {
	AuthorID       string
	CreatedAt      time.Time // 글·댓글 작성 시각(wr_datetime)
	CurrentTitle   *string   // 댓글은 nil
	CurrentContent string
	Records        []RevisionRecord
	Hidden         map[int]bool // 운영진이 숨긴 버전
}

// PublicRevision 공개 응답 한 항목. 수정한 사람·IP·메타데이터는 싣지 않는다.
type PublicRevision struct {
	Version int     `json:"version"`
	Title   *string `json:"title,omitempty"`
	Content string  `json:"content"`
	// EditedAt 이 내용이 된 시각(KST). 비작성자 수정 직후의 기준 내용은 시각을 싣지 않는다
	// (그 시각이 곧 비작성자 수정 시각이라 수정 사실이 드러난다).
	EditedAt *time.Time `json:"edited_at,omitempty"`
	Current  bool       `json:"current"`
}

type revisionSnapshot struct {
	version int
	title   *string
	content string
	at      *time.Time
	current bool
}

// BuildPublicRevisions 공개 규칙을 적용한 스냅샷 목록(오래된 것 → 현재)을 만든다.
// 공개할 수정이 없으면 빈 슬라이스를 돌려준다.
func BuildPublicRevisions(src PublicRevisionSource) []PublicRevision {
	empty := []PublicRevision{}
	if src.AuthorID == "" {
		return empty
	}

	var createdAt *time.Time
	if !src.CreatedAt.IsZero() {
		t := src.CreatedAt
		createdAt = &t
	}
	updates := make([]RevisionRecord, 0, len(src.Records))
	for _, r := range src.Records {
		switch r.ChangeType {
		case "update":
			updates = append(updates, r)
		case "create":
			if !r.EditedAt.IsZero() {
				t := r.EditedAt
				createdAt = &t
			}
		}
	}
	if len(updates) == 0 {
		return empty
	}
	sort.SliceStable(updates, func(i, j int) bool { return updates[i].Version < updates[j].Version })

	// 마지막 비작성자 수정의 위치. 그 이전 스냅샷(0..lastForeign)은 전부 숨긴다.
	lastForeign := -1
	for i, u := range updates {
		if u.EditedBy != src.AuthorID {
			lastForeign = i
		}
	}
	n := len(updates)
	if lastForeign >= n-1 {
		// 마지막 수정이 비작성자 수정이면 공개할 작성자 수정이 없다.
		return empty
	}

	snaps := make([]revisionSnapshot, 0, n-lastForeign)
	for k := lastForeign + 1; k <= n; k++ {
		var s revisionSnapshot
		if k < n {
			if src.Hidden[updates[k].Version] {
				continue
			}
			s.version = updates[k].Version
			s.title = updates[k].Title
			if updates[k].Content != nil {
				s.content = *updates[k].Content
			}
		} else {
			s.version = PublicRevisionCurrentVersion
			s.title = src.CurrentTitle
			s.content = src.CurrentContent
			s.current = true
		}
		switch {
		case k == 0:
			s.at = createdAt
		case k-1 == lastForeign:
			s.at = nil // 비작성자 수정 시각을 드러내지 않는다
		default:
			t := updates[k-1].EditedAt
			s.at = &t
		}
		if len(snaps) > 0 && sameSnapshot(snaps[len(snaps)-1], s) {
			// 내용이 그대로인 저장은 하나로 합친다. 현재 내용은 현재 표시를 유지한다.
			if s.current {
				prev := snaps[len(snaps)-1]
				s.at = prev.at
				snaps[len(snaps)-1] = s
			}
			continue
		}
		snaps = append(snaps, s)
	}
	if len(snaps) < 2 {
		return empty
	}

	out := make([]PublicRevision, 0, len(snaps))
	for _, s := range snaps {
		out = append(out, PublicRevision{
			Version:  s.version,
			Title:    s.title,
			Content:  s.content,
			EditedAt: s.at,
			Current:  s.current,
		})
	}
	return out
}

// CountPublicEdits 공개되는 수정 횟수(스냅샷 수 - 1). 공개 이력이 없으면 0.
func CountPublicEdits(src PublicRevisionSource) int {
	revs := BuildPublicRevisions(src)
	if len(revs) < 2 {
		return 0
	}
	return len(revs) - 1
}

// IsHideableVersion 숨김 대상이 될 수 있는 버전인가(작성자 이력에 실제로 있는 update 행).
func IsHideableVersion(records []RevisionRecord, version int) bool {
	if version <= 0 {
		return false
	}
	for _, r := range records {
		if r.ChangeType == "update" && r.Version == version {
			return true
		}
	}
	return false
}

func sameSnapshot(a, b revisionSnapshot) bool {
	if a.content != b.content {
		return false
	}
	at, bt := "", ""
	if a.title != nil {
		at = *a.title
	}
	if b.title != nil {
		bt = *b.title
	}
	return at == bt
}

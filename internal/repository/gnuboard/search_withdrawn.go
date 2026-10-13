package gnuboard

import (
	"strings"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// 작성자 검색 탈퇴 회원 제외
//
// 탈퇴 회원 정보 비노출 정책을 작성자 검색에도 적용한다. 프로필·활동 모아보기는
// 탈퇴 회원을 이미 가리는데, 작성자 조건 검색(sfl=author 등)으로는 같은 활동을
// 다시 모아 볼 수 있었다.
//
//   - 판정 기준: g5_member.mb_leave_date 가 비어 있지 않음(숙려 중 포함) — 웹 가드와 같은 기준.
//   - 작성자 계열 검색에만 적용한다. 제목·본문 검색은 그대로다.
//   - 판정 실패 시 작성자 검색은 빈 결과(비노출 우선). 응답은 일반 「결과 없음」과 같다.

// authorSearchFields 는 작성자를 기준으로 찾는 sfl 값이다(웹 sphinx-search.ts 와 일치).
var authorSearchFields = map[string]bool{
	"author":         true,
	"author_nick":    true,
	"author_id":      true,
	"comment_author": true,
	"comment_nick":   true,
	"comment_id":     true,
}

// isAuthorSearchField 는 작성자 계열 검색 필드인지 알려준다.
func isAuthorSearchField(field string) bool {
	return authorSearchFields[field]
}

// withdrawnAuthorNotExistsSQL 은 작성자가 탈퇴 회원인 행을 빼는 조건이다.
// table 은 바깥 쿼리의 게시판 테이블명(검증된 slug 로 만든 값)이다. 비회원 글(mb_id='')은
// 회원 행이 없으므로 그대로 남는다.
func withdrawnAuthorNotExistsSQL(table string) string {
	return "NOT EXISTS (SELECT 1 FROM g5_member wm WHERE wm.mb_id = " + table +
		".mb_id AND wm.mb_leave_date <> '')"
}

// isWithdrawnSearchTerm 은 검색어가 탈퇴 회원의 아이디 또는 닉네임과 정확히 같은지 본다.
// 같으면 호출부는 검색 없이 빈 결과를 돌려준다. 조회 실패는 error 로 올린다.
func (r *writeRepository) isWithdrawnSearchTerm(term string) (bool, error) {
	term = strings.TrimSpace(term)
	if term == "" {
		return false, nil
	}
	var ids []string
	if err := r.db.Table("g5_member").
		Where("(mb_id = ? OR mb_nick = ?) AND mb_leave_date <> ''", term, term).
		Limit(1).
		Pluck("mb_id", &ids).Error; err != nil {
		return false, err
	}
	return len(ids) > 0, nil
}

// withdrawnMemberSet 은 주어진 아이디 중 탈퇴 회원 아이디 집합을 돌려준다(IN 쿼리 1회).
// DB 비교가 대소문자를 구분하지 않으므로 소문자로 담는다. 조회 실패는 error 로 올린다.
func (r *writeRepository) withdrawnMemberSet(mbIDs []string) (map[string]bool, error) {
	seen := make(map[string]bool, len(mbIDs))
	ids := make([]string, 0, len(mbIDs))
	for _, id := range mbIDs {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	set := make(map[string]bool)
	if len(ids) == 0 {
		return set, nil
	}
	var rows []string
	if err := r.db.Table("g5_member").
		Where("mb_id IN ? AND mb_leave_date <> ''", ids).
		Pluck("mb_id", &rows).Error; err != nil {
		return nil, err
	}
	for _, id := range rows {
		set[strings.ToLower(id)] = true
	}
	return set, nil
}

// excludeWithdrawnAuthors 는 작성자가 탈퇴 회원인 글을 빼고, 뺀 수를 함께 돌려준다.
// 순서는 유지한다. 판정 조회 실패는 error 로 올린다.
func (r *writeRepository) excludeWithdrawnAuthors(posts []*gnuboard.G5Write) ([]*gnuboard.G5Write, int, error) {
	if len(posts) == 0 {
		return posts, 0, nil
	}
	mbIDs := make([]string, 0, len(posts))
	for _, p := range posts {
		if p != nil {
			mbIDs = append(mbIDs, p.MbID)
		}
	}
	set, err := r.withdrawnMemberSet(mbIDs)
	if err != nil {
		return nil, 0, err
	}
	if len(set) == 0 {
		return posts, 0, nil
	}
	kept := make([]*gnuboard.G5Write, 0, len(posts))
	for _, p := range posts {
		if p != nil && p.MbID != "" && set[strings.ToLower(p.MbID)] {
			continue
		}
		kept = append(kept, p)
	}
	return kept, len(posts) - len(kept), nil
}

// blockedAuthorSearch 는 작성자 계열 검색을 검색 없이 빈 결과로 끝내야 하는지 알려준다.
// 검색어가 탈퇴 회원의 아이디·닉네임과 정확히 같거나, 그 판정에 실패하면 true 다.
// 작성자 계열이 아닌 검색은 항상 false(조회도 하지 않는다).
func (r *writeRepository) blockedAuthorSearch(field, query string) bool {
	if !isAuthorSearchField(field) {
		return false
	}
	blocked, err := r.isWithdrawnSearchTerm(query)
	return err != nil || blocked
}

// applyAuthorSearchFilter 는 작성자 계열 검색 결과에서 작성자가 탈퇴 회원인 글을 빼고
// 총 건수를 뺀 만큼 줄인다. 작성자 계열이 아니면 그대로 돌려준다.
// 판정 조회에 실패하면 빈 결과(nil, 0)다 — 비노출 우선.
//
// 총 건수는 이번 페이지에서 뺀 만큼만 보정한다. Sphinx total_found 전체를 정확히
// 맞추려면 탈퇴 회원 글 수를 따로 세야 해서(재질의) 근사로 둔다.
func (r *writeRepository) applyAuthorSearchFilter(field string, posts []*gnuboard.G5Write, total int64) ([]*gnuboard.G5Write, int64) {
	if !isAuthorSearchField(field) {
		return posts, total
	}
	kept, removed, err := r.excludeWithdrawnAuthors(posts)
	if err != nil {
		return nil, 0
	}
	return kept, subtractRemoved(total, removed)
}

// subtractRemoved 는 걸러낸 수만큼 총 건수를 줄인다(0 미만 방지).
func subtractRemoved(total int64, removed int) int64 {
	total -= int64(removed)
	if total < 0 {
		return 0
	}
	return total
}

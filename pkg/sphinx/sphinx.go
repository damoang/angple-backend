package sphinx

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode"

	_ "github.com/go-sql-driver/mysql"
)

// ErrUnknownIndex is wrapped into the error returned by Search when the target
// full-text index does not exist on the Manticore/Sphinx server. Newly-created
// boards (신규 소모미) have no g5_write_<slug>_dist index provisioned yet, so a
// search request fails with an "unknown local index" error. Callers can detect
// this case with IsUnknownIndexErr and fall back to a MySQL LIKE search.
var ErrUnknownIndex = errors.New("sphinx: unknown index")

// isUnknownIndexError reports whether a SphinxQL/Manticore error indicates the
// target index does not exist. Manticore speaks the MySQL wire protocol, so the
// signal is only available as text in err.Error(), e.g.:
//
//	"unknown local index 'g5_write_rockmetal_dist' in search request"
//	"index 'g5_write_rockmetal_dist': not found"
//	"no such index: g5_write_rockmetal_dist"
//
// Only index-absence errors match; connection/syntax errors do not, so the
// caller never falls back on a transient outage.
func isUnknownIndexError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unknown local index"),
		strings.Contains(msg, "unknown index"),
		strings.Contains(msg, "no such index"),
		strings.Contains(msg, "unknown table"),
		strings.Contains(msg, "no such table"):
		return true
	case strings.Contains(msg, "index") && strings.Contains(msg, "not found"):
		return true
	case strings.Contains(msg, "table") && strings.Contains(msg, "absent"):
		return true
	default:
		return false
	}
}

// IsUnknownIndexErr reports whether err (or any error it wraps) is an
// unknown-index error produced by Search — i.e. the board's full-text index is
// not provisioned on the search server.
func IsUnknownIndexErr(err error) bool {
	return errors.Is(err, ErrUnknownIndex)
}

// Client wraps a SphinxQL connection (MySQL protocol on port 9306).
type Client struct {
	db *sql.DB
}

// SearchResult holds Sphinx search results.
type SearchResult struct {
	IDs        []int // matched wr_id list (ordered by relevance or wr_id DESC)
	TotalFound int64 // total matching documents
}

// New creates a new Sphinx client connecting via SphinxQL.
func New(host string, port int) (*Client, error) {
	dsn := fmt.Sprintf("tcp(%s:%d)/", host, port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("sphinx connect: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("sphinx ping: %w", err)
	}
	return &Client{db: db}, nil
}

// containsCJK checks if a string contains any CJK or Korean characters.
func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) {
			return true
		}
	}
	return false
}

// addInfixWildcards wraps each whitespace-separated token with * for partial matching.
// For CJK tokens (ngram_len=1), uses strict phrase search ("token") so that
// 1-character ngrams must be adjacent — prevents "산불" matching "생산 불가"
// where "산" and "불" appear in different words (#12543, #11791 재발 패턴).
// Non-CJK (영문/숫자) 은 기존 infix wildcard 그대로 유지.
func addInfixWildcards(escaped string) string {
	tokens := strings.Fields(escaped)
	for i, t := range tokens {
		if containsCJK(t) && len([]rune(t)) >= 2 {
			// CJK 2자+ phrase: strict ordered match ("산불").
			// 이전엔 `"*산불*"` 였으나 Manticore 가 wildcard expand 시 1글자
			// ngram ("산", "불") 단독 매칭으로 분해되어 false positive 발생.
			tokens[i] = `"` + t + `"`
		} else {
			tokens[i] = "*" + t + "*"
		}
	}
	return strings.Join(tokens, " ")
}

// buildMatchExpr builds a Sphinx MATCH expression from search field and query.
func buildMatchExpr(searchField, searchQuery string) string {
	// Escape special Sphinx characters
	escaped := escapeSphinx(searchQuery)
	// Add infix wildcards for partial matching (author uses exact match)
	wildcarded := addInfixWildcards(escaped)
	switch searchField {
	case "title":
		return fmt.Sprintf("@wr_subject %s", wildcarded)
	case "content":
		return fmt.Sprintf("@wr_content %s", wildcarded)
	case "title_content":
		return fmt.Sprintf("@(wr_subject,wr_content) %s", wildcarded)
	case "author":
		return fmt.Sprintf("@(wr_name,mb_id) %s", wildcarded)
	default:
		return fmt.Sprintf("@(wr_subject,wr_content) %s", wildcarded)
	}
}

// escapeSphinx escapes special characters for SphinxQL MATCH expressions.
func escapeSphinx(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		`"`, `\"`,
		`(`, `\(`,
		`)`, `\)`,
		`|`, `\|`,
		`-`, `\-`,
		`!`, `\!`,
		`~`, `\~`,
		`&`, `\&`,
		`/`, `\/`,
		`^`, `\^`,
		`$`, `\$`,
		`=`, `\=`,
		`<`, `\<`,
		`@`, `\@`,
	)
	return replacer.Replace(s)
}

// Search queries Sphinx for matching post IDs using the distributed index (main + delta).
// sortBy: "relevance" for WEIGHT() DESC, anything else for wr_id DESC (default).
func (c *Client) Search(boardID, searchField, searchQuery string, page, limit int, sortBy ...string) (*SearchResult, error) {
	index := fmt.Sprintf("g5_write_%s_dist", boardID)
	matchExpr := buildMatchExpr(searchField, searchQuery)
	offset := (page - 1) * limit

	orderClause := "ORDER BY wr_id DESC"
	optionClause := "OPTION max_matches=10000, ranker=proximity_bm25, field_weights=(wr_subject=10, wr_content=1), max_query_time=5000"
	if len(sortBy) > 0 && sortBy[0] == "relevance" {
		orderClause = "ORDER BY WEIGHT() DESC, wr_id DESC"
	}

	query := fmt.Sprintf(
		"SELECT wr_id FROM %s WHERE MATCH('%s') AND wr_is_comment=0 %s LIMIT %d, %d %s",
		index, matchExpr, orderClause, offset, limit, optionClause,
	)

	rows, err := c.db.Query(query)
	if err != nil {
		// 인덱스 부재(신규 소모미 등)는 sentinel 로 감싸 호출부가 폴백을 태울 수 있게 한다.
		if isUnknownIndexError(err) {
			return nil, fmt.Errorf("sphinx query (%s): %s: %w", index, err.Error(), ErrUnknownIndex)
		}
		return nil, fmt.Errorf("sphinx query: %w", err)
	}
	defer rows.Close()

	var ids []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("sphinx scan: %w", err)
		}
		ids = append(ids, id)
	}

	// Get total_found from SHOW META
	var totalFound int64
	metaRows, err := c.db.Query("SHOW META")
	if err == nil {
		defer metaRows.Close()
		for metaRows.Next() {
			var name, value string
			if err := metaRows.Scan(&name, &value); err == nil {
				if name == "total_found" {
					fmt.Sscanf(value, "%d", &totalFound)
				}
			}
		}
	}

	return &SearchResult{IDs: ids, TotalFound: totalFound}, nil
}

// Close closes the SphinxQL connection.
func (c *Client) Close() error {
	return c.db.Close()
}

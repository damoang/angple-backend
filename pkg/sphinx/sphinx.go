package sphinx

import (
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	_ "github.com/go-sql-driver/mysql"
)

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
// For CJK tokens (ngram_len=1), uses phrase search ("*token*") to ensure
// adjacent ngram matching instead of scattered individual character matches.
func addInfixWildcards(escaped string) string {
	tokens := strings.Fields(escaped)
	for i, t := range tokens {
		if containsCJK(t) && len([]rune(t)) >= 2 {
			// CJK phrase: "단파" → "*단파*" (adjacent ngram matching)
			tokens[i] = `"*` + t + `*"`
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
	optionClause := "OPTION max_matches=10000"
	if len(sortBy) > 0 && sortBy[0] == "relevance" {
		orderClause = "ORDER BY WEIGHT() DESC, wr_id DESC"
		optionClause = "OPTION max_matches=10000, ranker=sph04"
	}

	query := fmt.Sprintf(
		"SELECT wr_id FROM %s WHERE MATCH('%s') AND wr_is_comment=0 %s LIMIT %d, %d %s",
		index, matchExpr, orderClause, offset, limit, optionClause,
	)

	rows, err := c.db.Query(query)
	if err != nil {
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

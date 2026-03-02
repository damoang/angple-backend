package dantry

import (
	"context"
	"fmt"
	"time"
)

// Repository handles ClickHouse queries for js_errors
type Repository struct {
	ch *ClickHouseClient
}

// NewRepository creates a new Dantry Repository
func NewRepository(ch *ClickHouseClient) *Repository {
	return &Repository{ch: ch}
}

// ListErrors returns paginated individual errors
func (r *Repository) ListErrors(ctx context.Context, dateFrom, dateTo, errorType, search string, excludeScriptError bool, page, limit int) ([]Error, int64, error) {
	where, args := buildWhere(dateFrom, dateTo, errorType, search, excludeScriptError)
	offset := (page - 1) * limit

	countQuery := fmt.Sprintf("SELECT count() FROM error_logs.js_errors WHERE %s", where)
	var total uint64
	row := r.ch.QueryRow(ctx, countQuery, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	query := fmt.Sprintf(`SELECT id, member_id, timestamp, type, message, source, lineno, colno, stack, url, userAgent, is_script_blocked, is_mobile
		FROM error_logs.js_errors WHERE %s
		ORDER BY timestamp DESC LIMIT %d OFFSET %d`, where, limit, offset)

	rows, err := r.ch.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list query failed: %w", err)
	}
	defer rows.Close()

	var errors []Error
	for rows.Next() {
		var e Error
		if err := rows.Scan(&e.ID, &e.MemberID, &e.Timestamp, &e.Type, &e.Message, &e.Source, &e.Lineno, &e.Colno, &e.Stack, &e.URL, &e.UserAgent, &e.IsScriptBlocked, &e.IsMobile); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		errors = append(errors, e)
	}
	return errors, int64(total), nil
}

// ListGrouped returns errors grouped by message
func (r *Repository) ListGrouped(ctx context.Context, dateFrom, dateTo, errorType, search string, excludeScriptError bool, page, limit int) ([]ErrorGroup, int64, error) {
	where, args := buildWhere(dateFrom, dateTo, errorType, search, excludeScriptError)
	offset := (page - 1) * limit

	countQuery := fmt.Sprintf("SELECT uniq(message) FROM error_logs.js_errors WHERE %s", where)
	var total uint64
	row := r.ch.QueryRow(ctx, countQuery, args...)
	if err := row.Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count query failed: %w", err)
	}

	query := fmt.Sprintf(`SELECT message, any(type) as type, count() as count,
		min(timestamp) as first_seen, max(timestamp) as last_seen,
		any(source) as sample_source, any(url) as sample_url, any(stack) as sample_stack,
		uniq(url) as unique_urls, uniq(member_id) as unique_members
		FROM error_logs.js_errors WHERE %s
		GROUP BY message ORDER BY count DESC LIMIT %d OFFSET %d`, where, limit, offset)

	rows, err := r.ch.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("grouped query failed: %w", err)
	}
	defer rows.Close()

	var groups []ErrorGroup
	for rows.Next() {
		var g ErrorGroup
		if err := rows.Scan(&g.Message, &g.Type, &g.Count, &g.FirstSeen, &g.LastSeen, &g.SampleSource, &g.SampleURL, &g.SampleStack, &g.UniqueURLs, &g.UniqueMembers); err != nil {
			return nil, 0, fmt.Errorf("scan failed: %w", err)
		}
		groups = append(groups, g)
	}
	return groups, int64(total), nil
}

// GetByID returns a single error by ID
func (r *Repository) GetByID(ctx context.Context, id string) (*Error, error) {
	query := `SELECT id, member_id, timestamp, type, message, source, lineno, colno, stack, url, userAgent, is_script_blocked, is_mobile
		FROM error_logs.js_errors WHERE id = ? LIMIT 1`
	row := r.ch.QueryRow(ctx, query, id)
	var e Error
	if err := row.Scan(&e.ID, &e.MemberID, &e.Timestamp, &e.Type, &e.Message, &e.Source, &e.Lineno, &e.Colno, &e.Stack, &e.URL, &e.UserAgent, &e.IsScriptBlocked, &e.IsMobile); err != nil {
		return nil, fmt.Errorf("error not found: %w", err)
	}
	return &e, nil
}

// GetStats returns summary statistics
func (r *Repository) GetStats(ctx context.Context) (*Stats, error) {
	today := time.Now().Format("2006-01-02")
	weekAgo := time.Now().AddDate(0, 0, -7).Format("2006-01-02")

	query := `SELECT
		countIf(date_partition = toDate(?)) as today_count,
		countIf(date_partition >= toDate(?)) as week_count,
		uniqIf(message, date_partition >= toDate(?)) as unique_errors,
		if(countIf(date_partition >= toDate(?)) > 0,
			countIf(is_mobile = 1 AND date_partition >= toDate(?)) / countIf(date_partition >= toDate(?)) * 100,
			0) as mobile_ratio
		FROM error_logs.js_errors
		WHERE is_script_blocked = 0`

	var s Stats
	row := r.ch.QueryRow(ctx, query, today, weekAgo, weekAgo, weekAgo, weekAgo, weekAgo)
	if err := row.Scan(&s.TodayCount, &s.WeekCount, &s.UniqueErrors, &s.MobileRatio); err != nil {
		return nil, fmt.Errorf("stats query failed: %w", err)
	}

	topQuery := `SELECT message, count() as cnt FROM error_logs.js_errors
		WHERE date_partition >= toDate(?) AND is_script_blocked = 0
		GROUP BY message ORDER BY cnt DESC LIMIT 1`
	topRow := r.ch.QueryRow(ctx, topQuery, weekAgo)
	_ = topRow.Scan(&s.TopError, &s.TopErrorCount)

	return &s, nil
}

// GetTimeseries returns error counts bucketed by hour
func (r *Repository) GetTimeseries(ctx context.Context, dateFrom, dateTo string, excludeScriptError bool) ([]TimeBucket, error) {
	scriptFilter := ""
	if excludeScriptError {
		scriptFilter = "AND is_script_blocked = 0"
	}

	query := fmt.Sprintf(`SELECT toStartOfHour(timestamp) as bucket, count() as count
		FROM error_logs.js_errors
		WHERE date_partition >= toDate(?) AND date_partition <= toDate(?) %s
		GROUP BY bucket ORDER BY bucket`, scriptFilter)

	rows, err := r.ch.Query(ctx, query, dateFrom, dateTo)
	if err != nil {
		return nil, fmt.Errorf("timeseries query failed: %w", err)
	}
	defer rows.Close()

	var buckets []TimeBucket
	for rows.Next() {
		var b TimeBucket
		if err := rows.Scan(&b.Bucket, &b.Count); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		buckets = append(buckets, b)
	}
	return buckets, nil
}

// GetMembersByMessage returns members who triggered a specific error message
func (r *Repository) GetMembersByMessage(ctx context.Context, message, dateFrom, dateTo string) ([]ErrorMember, error) {
	where := "message = ?"
	args := []interface{}{message}
	if dateFrom != "" {
		where += " AND date_partition >= toDate(?)"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where += " AND date_partition <= toDate(?)"
		args = append(args, dateTo)
	}

	query := fmt.Sprintf(`SELECT member_id, count() as count, max(timestamp) as last_seen, any(url) as sample_url
		FROM error_logs.js_errors WHERE %s AND member_id != ''
		GROUP BY member_id ORDER BY count DESC LIMIT 100`, where)

	rows, err := r.ch.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("members query failed: %w", err)
	}
	defer rows.Close()

	var members []ErrorMember
	for rows.Next() {
		var m ErrorMember
		if err := rows.Scan(&m.MemberID, &m.Count, &m.LastSeen, &m.SampleURL); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}
		members = append(members, m)
	}
	return members, nil
}

func buildWhere(dateFrom, dateTo, errorType, search string, excludeScriptError bool) (string, []interface{}) {
	where := "1=1"
	var args []interface{}

	if dateFrom != "" {
		where += " AND date_partition >= toDate(?)"
		args = append(args, dateFrom)
	}
	if dateTo != "" {
		where += " AND date_partition <= toDate(?)"
		args = append(args, dateTo)
	}
	if excludeScriptError {
		where += " AND is_script_blocked = 0"
	}
	if errorType != "" {
		where += " AND type = ?"
		args = append(args, errorType)
	}
	if search != "" {
		where += " AND (message LIKE ? OR source LIKE ? OR url LIKE ?)"
		pattern := "%" + search + "%"
		args = append(args, pattern, pattern, pattern)
	}
	return where, args
}

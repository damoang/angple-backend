package v1handler

import (
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// imgSrcRegex matches src attribute in <img> tags
var imgSrcRegex = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

// thumbnailRegex matches S3 image URLs for thumbnail conversion
var thumbnailRegex *regexp.Regexp

func init() {
	cdnURL := strings.TrimRight(os.Getenv("CDN_URL"), "/")
	if cdnURL != "" {
		if parsed, err := url.Parse(cdnURL); err == nil {
			host := regexp.QuoteMeta(parsed.Host)
			// cdn.damoang.net 레거시 URL도 매칭 (에디터 업로드 시점에 따라 도메인이 다름)
			hostPattern := "(?:" + host + "|cdn\\.damoang\\.net)"
			// GIF 제외: 애니메이션 손실 방지
			thumbnailRegex = regexp.MustCompile(`(?i)^(https?://` + hostPattern + `/data/(?:file|editor)/.+)\.(jpg|jpeg|png|webp)$`)
		}
	}
}

// thumbnailSuffixRegex detects URLs that are already thumbnail versions
var thumbnailSuffixRegex = regexp.MustCompile(`-\d+x\d+\.webp$`)

// toThumbnailURL converts an S3 image URL to a Lambda-generated thumbnail URL
func toThumbnailURL(rawURL string, size string) string {
	if thumbnailRegex == nil {
		return rawURL
	}
	// 이미 요청한 크기와 동일한 썸네일이면 스킵
	if strings.HasSuffix(rawURL, "-"+size+".webp") {
		return rawURL
	}
	// 다른 크기의 썸네일이면 크기 suffix 제거 후 재생성
	if loc := thumbnailSuffixRegex.FindStringIndex(rawURL); loc != nil {
		base := rawURL[:loc[0]]
		return base + "-" + size + ".webp"
	}
	// 썸네일이 아닌 일반 URL → 기존 로직
	m := thumbnailRegex.FindStringSubmatch(rawURL)
	if m == nil {
		return rawURL
	}
	return m[1] + "-" + size + ".webp"
}

// kst is the Asia/Seoul timezone for parsing gnuboard datetime values
var kst = time.FixedZone("KST", 9*60*60)

// parseWrLast converts DB datetime string to RFC3339 format
// Returns nil if parsing fails or if the time equals created time (no updates)
func parseWrLast(wrLast string, createdAt time.Time) any {
	if wrLast == "" {
		return nil
	}
	// DB stores as "2006-01-02 15:04:05" in KST
	lastTime, err := time.ParseInLocation("2006-01-02 15:04:05", wrLast, kst)
	if err != nil {
		return nil
	}
	// If updated_at equals created_at, no actual update occurred
	createdAtKST := createdAt.In(kst)
	if lastTime.Equal(createdAtKST) || lastTime.Sub(createdAtKST).Abs() < time.Second {
		return nil
	}
	return lastTime.Format(time.RFC3339)
}

// extractFirstImageURL extracts the first <img src="..."> URL from HTML content
func extractFirstImageURL(html string) string {
	m := imgSrcRegex.FindStringSubmatch(html)
	if len(m) >= 2 {
		return normalizeMediaURL(m[1])
	}
	return ""
}

func normalizeMediaURL(raw string) string {
	if raw == "" {
		return ""
	}

	cdnURL := strings.TrimRight(os.Getenv("CDN_URL"), "/")
	if cdnURL == "" {
		return raw
	}

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		return rewriteLegacyCDNHost(raw, cdnURL)
	}
	if cdnURL == "" {
		return raw
	}

	raw = strings.TrimPrefix(raw, "./")
	if strings.HasPrefix(raw, "data/") {
		return cdnURL + "/" + raw
	}
	if strings.HasPrefix(raw, "/data/") {
		return cdnURL + raw
	}

	return raw
}

func normalizeMediaContent(raw string, _ ...string) string {
	cdnURL := strings.TrimRight(os.Getenv("CDN_URL"), "/")
	if raw == "" {
		return raw
	}

	if cdnURL == "" {
		return raw
	}

	replacer := strings.NewReplacer(
		`src="https://s3.damoang.net/data/`, `src="`+cdnURL+`/data/`,
		`src='https://s3.damoang.net/data/`, `src='`+cdnURL+`/data/`,
		`src="http://s3.damoang.net/data/`, `src="`+cdnURL+`/data/`,
		`src='http://s3.damoang.net/data/`, `src='`+cdnURL+`/data/`,
		`src="https://cdn.damoang.net/data/`, `src="`+cdnURL+`/data/`,
		`src='https://cdn.damoang.net/data/`, `src='`+cdnURL+`/data/`,
		`src="http://cdn.damoang.net/data/`, `src="`+cdnURL+`/data/`,
		`src='http://cdn.damoang.net/data/`, `src='`+cdnURL+`/data/`,
		`src="/data/`, `src="`+cdnURL+`/data/`,
		`src='/data/`, `src='`+cdnURL+`/data/`,
		`src="data/`, `src="`+cdnURL+`/data/`,
		`src='data/`, `src='`+cdnURL+`/data/`,
		`href="https://s3.damoang.net/data/`, `href="`+cdnURL+`/data/`,
		`href='https://s3.damoang.net/data/`, `href='`+cdnURL+`/data/`,
		`href="http://s3.damoang.net/data/`, `href="`+cdnURL+`/data/`,
		`href='http://s3.damoang.net/data/`, `href='`+cdnURL+`/data/`,
		`href="https://cdn.damoang.net/data/`, `href="`+cdnURL+`/data/`,
		`href='https://cdn.damoang.net/data/`, `href='`+cdnURL+`/data/`,
		`href="http://cdn.damoang.net/data/`, `href="`+cdnURL+`/data/`,
		`href='http://cdn.damoang.net/data/`, `href='`+cdnURL+`/data/`,
		`href="/data/`, `href="`+cdnURL+`/data/`,
		`href='/data/`, `href='`+cdnURL+`/data/`,
		`href="data/`, `href="`+cdnURL+`/data/`,
		`href='data/`, `href='`+cdnURL+`/data/`,
	)
	return replacer.Replace(raw)
}

func rewriteLegacyCDNHost(raw, cdnURL string) string {
	replacements := [][2]string{
		{"https://s3.damoang.net/data/", cdnURL + "/data/"},
		{"http://s3.damoang.net/data/", cdnURL + "/data/"},
		{"https://cdn.damoang.net/data/", cdnURL + "/data/"},
		{"http://cdn.damoang.net/data/", cdnURL + "/data/"},
	}

	for _, pair := range replacements {
		if strings.HasPrefix(raw, pair[0]) {
			return pair[1] + strings.TrimPrefix(raw, pair[0])
		}
	}

	return raw
}

// MaskIP masks the 2nd octet of an IPv4 address with ♡ (e.g. "1.2.3.4" → "1.♡.3.4")
func MaskIP(ip string) string {
	if ip == "" {
		return ""
	}
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		return parts[0] + ".♡." + parts[2] + "." + parts[3]
	}
	return ip
}

// OverrideIPForAdmin replaces masked author_ip with full IP when requester is admin
func OverrideIPForAdmin(items []map[string]any, posts []*gnuboard.G5Write) {
	for i, item := range items {
		if i < len(posts) {
			item["author_ip"] = posts[i].WrIP
		}
	}
}

// OverrideIPForAdminSingle replaces masked author_ip with full IP for a single item
func OverrideIPForAdminSingle(item map[string]any, w *gnuboard.G5Write) {
	item["author_ip"] = w.WrIP
}

// TransformToV1Post converts G5Write to v1 API response format
func TransformToV1Post(w *gnuboard.G5Write, isNotice bool) map[string]any {
	result := map[string]any{
		"id":             w.WrID,
		"title":          w.WrSubject,
		"author":         w.WrName,
		"author_id":      w.MbID,
		"category":       w.CaName,
		"views":          w.WrHit,
		"likes":          w.WrGood,
		"dislikes":       w.WrNogood,
		"comments_count": w.WrComment,
		"has_file":       w.WrFile > 0,
		"is_notice":      isNotice,
		"is_secret":      strings.Contains(w.WrOption, "secret"),
		"link1":          w.WrLink1,
		"link2":          w.WrLink2,
		"author_ip":      MaskIP(w.WrIP),
		"created_at":     w.WrDatetime.Format(time.RFC3339),
		"updated_at":     parseWrLast(w.WrLast, w.WrDatetime),
	}

	// Add deleted_at if post is soft deleted
	if w.WrDeletedAt != nil {
		result["deleted_at"] = w.WrDeletedAt.Format(time.RFC3339)
	}

	// Add thumbnail: priority is wr_10 > first <img> in content
	if w.Wr10 != "" {
		normalized := normalizeMediaURL(w.Wr10)
		result["thumbnail"] = toThumbnailURL(normalized, "400x225")
		result["thumbnail_raw"] = normalized
		result["extra_10"] = normalized
	} else if img := extractFirstImageURL(w.WrContent); img != "" {
		result["thumbnail"] = toThumbnailURL(img, "400x225")
		result["thumbnail_raw"] = img
	}

	return result
}

// TransformToV1PostDetail converts G5Write to detailed v1 API response format
func TransformToV1PostDetail(w *gnuboard.G5Write, isNotice bool, boardID ...string) map[string]any {
	result := TransformToV1Post(w, isNotice)
	result["content"] = normalizeMediaContent(w.WrContent, boardID...)
	if w.Wr9 != "" {
		result["extra_9"] = w.Wr9
	}
	return result
}

// TransformToV1Posts converts a slice of G5Write to v1 API response format
func TransformToV1Posts(posts []*gnuboard.G5Write, noticeIDs map[int]bool) []map[string]any {
	result := make([]map[string]any, len(posts))
	for i, p := range posts {
		isNotice := noticeIDs[p.WrID]
		result[i] = TransformToV1Post(p, isNotice)
	}
	return result
}

// TransformToV1PostsSummary converts posts to a lighter list payload for hot boards.
func TransformToV1PostsSummary(posts []*gnuboard.G5Write, noticeIDs map[int]bool) []map[string]any {
	result := make([]map[string]any, len(posts))
	for i, p := range posts {
		isNotice := noticeIDs[p.WrID]
		item := TransformToV1Post(p, isNotice)
		item["has_video"] = p.Wr9 != ""
		item["has_image"] = item["has_file"] == true || p.Wr10 != ""
		delete(item, "dislikes")
		delete(item, "link1")
		delete(item, "link2")
		delete(item, "author_ip")
		delete(item, "updated_at")
		delete(item, "thumbnail")
		delete(item, "thumbnail_raw")
		delete(item, "extra_10")
		result[i] = item
	}
	return result
}

// TransformToV1Comment converts G5Write (comment) to v1 API response format
func TransformToV1Comment(w *gnuboard.G5Write) map[string]any {
	depth := len(w.WrCommentReply)
	result := map[string]any{
		"id":         w.WrID,
		"post_id":    w.WrParent,
		"content":    normalizeMediaContent(w.WrContent),
		"author":     w.WrName,
		"author_id":  w.MbID,
		"likes":      w.WrGood,
		"dislikes":   w.WrNogood,
		"author_ip":  MaskIP(w.WrIP),
		"depth":      depth,
		"created_at": w.WrDatetime.Format(time.RFC3339),
		"updated_at": parseWrLast(w.WrLast, w.WrDatetime),
		"is_secret":  strings.Contains(w.WrOption, "secret"),
	}
	if w.WrDeletedAt != nil {
		result["deleted_at"] = w.WrDeletedAt.Format(time.RFC3339)
	}
	if w.WrDeletedBy != nil {
		result["deleted_by"] = *w.WrDeletedBy
	}
	return result
}

// TransformToV1Comments converts a slice of G5Write comments to v1 API response format
func TransformToV1Comments(comments []*gnuboard.G5Write) []map[string]any {
	result := make([]map[string]any, len(comments))
	for i, c := range comments {
		result[i] = TransformToV1Comment(c)
	}
	return result
}

// TransformToV1Board converts G5Board to v1 API response format
func TransformToV1Board(b *gnuboard.G5Board) map[string]any {
	return map[string]any{
		"id":             b.BoTable,
		"slug":           b.BoTable,
		"name":           b.BoSubject,
		"group_id":       b.GrID,
		"list_level":     b.BoListLevel,
		"read_level":     b.BoReadLevel,
		"write_level":    b.BoWriteLevel,
		"reply_level":    b.BoReplyLevel,
		"comment_level":  b.BoCommentLevel,
		"upload_level":   b.BoUploadLevel,
		"download_level": b.BoDownloadLevel,
		"order":          b.BoOrder,
		"use_category":   b.BoUseCategory == 1,
		"category_list":  b.BoCategoryList,
		"write_point":    b.BoWritePoint,
		"comment_point":  b.BoCommentPoint,
		"read_point":     b.BoReadPoint,
		"download_point": b.BoDownloadPoint,
		"use_good":       b.BoUseGood == 1,
		"use_nogood":     b.BoUseNogood == 1,
		"use_secret":     b.BoUseSecret > 0,
		"use_sns":        b.BoUseSns,
		"post_count":     b.BoCountWrite,
		"comment_count":  b.BoCountComment,
	}
}

// TransformToV1Member converts G5Member to v1 API response format
func TransformToV1Member(m *gnuboard.G5Member) map[string]any {
	avatarURL := m.MbImageUrl
	if avatarURL == "" {
		avatarURL = m.MbIconPath
	}
	result := map[string]any{
		"id":         m.MbID,
		"username":   m.MbID,
		"nickname":   m.MbNick,
		"email":      m.MbEmail,
		"level":      m.MbLevel,
		"point":      m.MbPoint,
		"avatar_url": avatarURL,
		"profile":    m.MbProfile,
		"created_at": m.MbDatetime.Format(time.RFC3339),
	}
	if m.MbImageUpdatedAt != nil {
		result["avatar_updated_at"] = m.MbImageUpdatedAt.Unix()
	}
	return result
}

// BuildNoticeIDMap creates a map of notice IDs for quick lookup
func BuildNoticeIDMap(noticeIDs []int) map[int]bool {
	m := make(map[int]bool, len(noticeIDs))
	for _, id := range noticeIDs {
		m[id] = true
	}
	return m
}

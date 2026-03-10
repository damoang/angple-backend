package v1handler

import (
	"regexp"
	"strings"
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// imgSrcRegex matches src attribute in <img> tags
var imgSrcRegex = regexp.MustCompile(`<img[^>]+src=["']([^"']+)["']`)

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
		return m[1]
	}
	return ""
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
		result["thumbnail"] = w.Wr10
		result["extra_10"] = w.Wr10
	} else if img := extractFirstImageURL(w.WrContent); img != "" {
		result["thumbnail"] = img
	}

	return result
}

// TransformToV1PostDetail converts G5Write to detailed v1 API response format
func TransformToV1PostDetail(w *gnuboard.G5Write, isNotice bool) map[string]any {
	result := TransformToV1Post(w, isNotice)
	result["content"] = w.WrContent
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

// TransformToV1Comment converts G5Write (comment) to v1 API response format
func TransformToV1Comment(w *gnuboard.G5Write) map[string]any {
	depth := len(w.WrCommentReply)
	result := map[string]any{
		"id":         w.WrID,
		"post_id":    w.WrParent,
		"content":    w.WrContent,
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
	return map[string]any{
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
}

// BuildNoticeIDMap creates a map of notice IDs for quick lookup
func BuildNoticeIDMap(noticeIDs []int) map[int]bool {
	m := make(map[int]bool, len(noticeIDs))
	for _, id := range noticeIDs {
		m[id] = true
	}
	return m
}

package v1handler

import (
	"time"

	"github.com/damoang/angple-backend/internal/domain/gnuboard"
)

// parseWrLast converts DB datetime string to RFC3339 format
// Returns nil if parsing fails or if the time equals created time (no updates)
func parseWrLast(wrLast string, createdAt time.Time) any {
	if wrLast == "" {
		return nil
	}
	// DB stores as "2006-01-02 15:04:05"
	lastTime, err := time.ParseInLocation("2006-01-02 15:04:05", wrLast, time.Local)
	if err != nil {
		return nil
	}
	// If updated_at equals created_at, no actual update occurred
	if lastTime.Equal(createdAt) || lastTime.Sub(createdAt).Abs() < time.Second {
		return nil
	}
	return lastTime.Format(time.RFC3339)
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
		"link1":          w.WrLink1,
		"link2":          w.WrLink2,
		"created_at":     w.WrDatetime.Format(time.RFC3339),
		"updated_at":     parseWrLast(w.WrLast, w.WrDatetime),
	}

	// Add thumbnail/extra_10 if wr_10 has value (for gallery/message layouts)
	if w.Wr10 != "" {
		result["thumbnail"] = w.Wr10
		result["extra_10"] = w.Wr10
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
	return map[string]any{
		"id":         w.WrID,
		"post_id":    w.WrParent,
		"content":    w.WrContent,
		"author":     w.WrName,
		"author_id":  w.MbID,
		"likes":      w.WrGood,
		"dislikes":   w.WrNogood,
		"depth":      depth,
		"created_at": w.WrDatetime.Format(time.RFC3339),
	}
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
		"post_count":     b.BoCountWrite,
		"comment_count":  b.BoCountComment,
	}
}

// TransformToV1Member converts G5Member to v1 API response format
func TransformToV1Member(m *gnuboard.G5Member) map[string]any {
	return map[string]any{
		"id":         m.MbID,
		"username":   m.MbID,
		"nickname":   m.MbNick,
		"email":      m.MbEmail,
		"level":      m.MbLevel,
		"point":      m.MbPoint,
		"avatar_url": m.MbIconPath,
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

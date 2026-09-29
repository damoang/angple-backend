package gnuboard

import (
	"strings"
	"time"
)

// MyPost represents a post row from UNION ALL across g5_write_* tables
type MyPost struct {
	WrID       int        `gorm:"column:wr_id" json:"wr_id"`
	WrSubject  string     `gorm:"column:wr_subject" json:"wr_subject"`
	WrContent  string     `gorm:"column:wr_content" json:"wr_content"`
	WrHit      int        `gorm:"column:wr_hit" json:"wr_hit"`
	WrGood     int        `gorm:"column:wr_good" json:"wr_good"`
	WrNogood   int        `gorm:"column:wr_nogood" json:"wr_nogood"`
	WrComment  int        `gorm:"column:wr_comment" json:"wr_comment"`
	WrDatetime time.Time  `gorm:"column:wr_datetime" json:"wr_datetime"`
	MbID       string     `gorm:"column:mb_id" json:"mb_id"`
	WrName     string     `gorm:"column:wr_name" json:"wr_name"`
	WrOption   string     `gorm:"column:wr_option" json:"wr_option"`
	WrFile     int        `gorm:"column:wr_file" json:"wr_file"`
	BoardID    string     `gorm:"column:board_id" json:"board_id"`
	DeletedAt  *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
}

// ToPostResponse converts MyPost to the standard API response format
func (p *MyPost) ToPostResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":             p.WrID,
		"title":          p.WrSubject,
		"author":         p.WrName,
		"author_id":      p.MbID,
		"board_id":       p.BoardID,
		"views":          p.WrHit,
		"likes":          p.WrGood,
		"dislikes":       p.WrNogood,
		"comments_count": p.WrComment,
		"has_file":       p.WrFile > 0,
		"is_secret":      strings.Contains(p.WrOption, "secret"),
		"created_at":     p.WrDatetime.Format(time.RFC3339),
		"deleted_at":     formatOptionalTime(p.DeletedAt),
	}
}

// MyCommentRow represents a comment row from UNION ALL with parent post title
type MyCommentRow struct {
	WrID            int        `gorm:"column:wr_id" json:"wr_id"`
	WrContent       string     `gorm:"column:wr_content" json:"wr_content"`
	WrDatetime      time.Time  `gorm:"column:wr_datetime" json:"wr_datetime"`
	MbID            string     `gorm:"column:mb_id" json:"mb_id"`
	WrName          string     `gorm:"column:wr_name" json:"wr_name"`
	WrParent        int        `gorm:"column:wr_parent" json:"wr_parent"`
	WrGood          int        `gorm:"column:wr_good" json:"wr_good"`
	WrNogood        int        `gorm:"column:wr_nogood" json:"wr_nogood"`
	WrOption        string     `gorm:"column:wr_option" json:"wr_option"`
	PostTitle       string     `gorm:"column:post_title" json:"post_title"`
	BoardID         string     `gorm:"column:board_id" json:"board_id"`
	DeletedAt       *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	ParentDeletedAt *time.Time `gorm:"column:parent_deleted_at" json:"parent_deleted_at,omitempty"`
}

// ToCommentResponse converts MyCommentRow to the standard API response format
func (c *MyCommentRow) ToCommentResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":              c.WrID,
		"content":         c.WrContent,
		"author":          c.WrName,
		"author_id":       c.MbID,
		"likes":           c.WrGood,
		"dislikes":        c.WrNogood,
		"parent_id":       c.WrParent,
		"post_id":         c.WrParent,
		"post_title":      c.PostTitle,
		"board_id":        c.BoardID,
		"is_secret":       strings.Contains(c.WrOption, "secret"),
		"created_at":      c.WrDatetime.Format(time.RFC3339),
		"deleted_at":      formatOptionalTime(c.DeletedAt),
		"post_deleted_at": formatOptionalTime(c.ParentDeletedAt),
	}
}

// ActivityPost represents a public post for member activity API
type ActivityPost struct {
	WrID       int        `gorm:"column:wr_id" json:"wr_id"`
	WrSubject  string     `gorm:"column:wr_subject" json:"wr_subject"`
	WrDatetime time.Time  `gorm:"column:wr_datetime" json:"wr_datetime"`
	BoardID    string     `gorm:"column:board_id" json:"board_id"`
	DeletedAt  *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	// IsLocked 는 신고로 잠긴 글(wr_7 = 'lock')인지다. 정본에서 읽는다(피드에는 없다).
	IsLocked bool `gorm:"column:is_locked" json:"is_locked"`
}

// ActivityComment represents a public comment for member activity API
type ActivityComment struct {
	WrID            int        `gorm:"column:wr_id" json:"wr_id"`
	WrContent       string     `gorm:"column:wr_content" json:"wr_content"`
	ContentKind     string     `gorm:"column:content_kind" json:"content_kind"`
	WrParent        int        `gorm:"column:wr_parent" json:"wr_parent"`
	WrDatetime      time.Time  `gorm:"column:wr_datetime" json:"wr_datetime"`
	BoardID         string     `gorm:"column:board_id" json:"board_id"`
	DeletedAt       *time.Time `gorm:"column:deleted_at" json:"deleted_at,omitempty"`
	ParentDeletedAt *time.Time `gorm:"column:parent_deleted_at" json:"parent_deleted_at,omitempty"`
	// ParentLocked 는 부모 글이 신고로 잠겼는지(wr_7 = 'lock')다. 정본에서 읽는다.
	ParentLocked bool `gorm:"column:parent_locked" json:"parent_locked"`
}

// LockedPostSubjectMask 는 신고잠금 글의 제목 대신 활동 목록에 싣는 문구다.
const LockedPostSubjectMask = "[신고잠금 글]"

// LockedParentCommentMask 는 신고잠금 글에 달린 댓글의 내용 대신 활동 목록에 싣는 문구다.
const LockedParentCommentMask = "[신고잠금 글의 댓글]"

// MaskLockedActivityPost 는 신고잠금 글의 제목을 서버에서 가린다(링크는 유지).
//
// 삭제된 글은 제목이 이미 비워져 자리표시자로 나가므로 건드리지 않는다.
// 이용제한 근거 글이기도 하면 핸들러가 뒤에서 근거 글 문구로 다시 덮는다.
func MaskLockedActivityPost(p ActivityPost) ActivityPost {
	if p.IsLocked && p.DeletedAt == nil {
		p.WrSubject = LockedPostSubjectMask
	}
	return p
}

// MaskLockedParentActivityComment 는 부모 글이 신고잠금인 댓글의 내용을 서버에서 가린다.
//
// 부모가 이용제한 근거 글이어도 예외 없이 가린다(잠긴 글에 단 댓글은 가린다).
// 댓글 자신이 근거 댓글이면 핸들러가 뒤에서 [이용제한 댓글]로 다시 덮어 그쪽이 우선한다.
// 삭제된 댓글은 내용이 이미 비워져 자리표시자로 나가므로 건드리지 않는다.
func MaskLockedParentActivityComment(c ActivityComment) ActivityComment {
	if c.ParentLocked && c.DeletedAt == nil {
		c.WrContent = LockedParentCommentMask
		// 피드에 저장된 종류(이미지·이모티콘 등)가 남으면 화면이 문구 대신 종류 표기를 띄울 수 있다.
		// 비워 두면 핸들러가 가린 문구로 다시 판정해 텍스트가 된다.
		c.ContentKind = ""
	}
	return c
}

func formatOptionalTime(value *time.Time) interface{} {
	if value == nil {
		return nil
	}
	return value.Format(time.RFC3339)
}

// BoardStat represents post/comment counts per board for a member
type BoardStat struct {
	BoardID      string `json:"board_id"`
	BoardName    string `json:"board_name"`
	PostCount    int64  `json:"post_count"`
	CommentCount int64  `json:"comment_count"`
}

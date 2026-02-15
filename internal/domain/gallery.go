package domain

// GalleryItem represents a gallery post with thumbnail
type GalleryItem struct {
	BoardID      string `json:"board_id"`
	PostID       int    `json:"post_id"`
	Title        string `json:"title"`
	Author       string `json:"author"`
	AuthorID     string `json:"author_id"`
	ThumbnailURL string `json:"thumbnail_url"`
	Views        int    `json:"views"`
	Likes        int    `json:"likes"`
	CommentCount int    `json:"comment_count"`
	CreatedAt    string `json:"created_at"`
}

// UnifiedSearchResult represents a search result across all boards
type UnifiedSearchResult struct {
	BoardID   string `json:"board_id"`
	BoardName string `json:"board_name,omitempty"`
	PostID    int    `json:"post_id"`
	Title     string `json:"title"`
	Content   string `json:"content"` // snippet
	Author    string `json:"author"`
	AuthorID  string `json:"author_id"`
	Views     int    `json:"views"`
	Likes     int    `json:"likes"`
	CreatedAt string `json:"created_at"`
}

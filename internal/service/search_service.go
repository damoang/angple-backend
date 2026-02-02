package service

import (
	"context"
	"fmt"
	"strings"

	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	es "github.com/damoang/angple-backend/pkg/elasticsearch"
	"gorm.io/gorm"
)

const (
	PostsIndex    = "angple_posts"
	CommentsIndex = "angple_comments"
)

// PostDocument represents a post indexed in Elasticsearch
type PostDocument struct {
	BoardID   string `json:"board_id"`
	PostID    int    `json:"post_id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	AuthorID  string `json:"author_id"`
	Category  string `json:"category"`
	CreatedAt string `json:"created_at"`
	Views     int    `json:"views"`
	Good      int    `json:"good"`
	// For autocomplete
	TitleSuggest map[string]interface{} `json:"title_suggest,omitempty"`
}

// CommentDocument represents a comment indexed in Elasticsearch
type CommentDocument struct {
	BoardID   string `json:"board_id"`
	PostID    int    `json:"post_id"`
	CommentID int    `json:"comment_id"`
	Content   string `json:"content"`
	Author    string `json:"author"`
	AuthorID  string `json:"author_id"`
	CreatedAt string `json:"created_at"`
}

// SearchService provides Elasticsearch-based search
type SearchService struct {
	esClient *es.Client
	db       *gorm.DB
}

// NewSearchService creates a new SearchService
func NewSearchService(esClient *es.Client, db *gorm.DB) *SearchService {
	svc := &SearchService{esClient: esClient, db: db}
	// Ensure indices exist
	ctx := context.Background()
	if err := svc.ensureIndices(ctx); err != nil {
		pkglogger.GetLogger().Error().Err(err).Msg("failed to create ES indices")
	}
	return svc
}

// ensureIndices creates ES indices with Korean nori analyzer mappings
func (s *SearchService) ensureIndices(ctx context.Context) error {
	postMapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"korean": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "nori_tokenizer",
						"filter":    []string{"nori_readingform", "lowercase"},
					},
				},
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"board_id":   map[string]interface{}{"type": "keyword"},
				"post_id":    map[string]interface{}{"type": "integer"},
				"title":      map[string]interface{}{"type": "text", "analyzer": "korean", "search_analyzer": "korean"},
				"content":    map[string]interface{}{"type": "text", "analyzer": "korean", "search_analyzer": "korean"},
				"author":     map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"author_id":  map[string]interface{}{"type": "keyword"},
				"category":   map[string]interface{}{"type": "keyword"},
				"created_at": map[string]interface{}{"type": "date"},
				"views":      map[string]interface{}{"type": "integer"},
				"good":       map[string]interface{}{"type": "integer"},
				"title_suggest": map[string]interface{}{
					"type":            "completion",
					"analyzer":        "korean",
					"search_analyzer": "korean",
				},
			},
		},
	}

	commentMapping := map[string]interface{}{
		"settings": map[string]interface{}{
			"analysis": map[string]interface{}{
				"analyzer": map[string]interface{}{
					"korean": map[string]interface{}{
						"type":      "custom",
						"tokenizer": "nori_tokenizer",
						"filter":    []string{"nori_readingform", "lowercase"},
					},
				},
			},
		},
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"board_id":   map[string]interface{}{"type": "keyword"},
				"post_id":    map[string]interface{}{"type": "integer"},
				"comment_id": map[string]interface{}{"type": "integer"},
				"content":    map[string]interface{}{"type": "text", "analyzer": "korean", "search_analyzer": "korean"},
				"author":     map[string]interface{}{"type": "text", "fields": map[string]interface{}{"keyword": map[string]interface{}{"type": "keyword"}}},
				"author_id":  map[string]interface{}{"type": "keyword"},
				"created_at": map[string]interface{}{"type": "date"},
			},
		},
	}

	if err := s.esClient.CreateIndex(ctx, PostsIndex, postMapping); err != nil {
		return fmt.Errorf("create posts index: %w", err)
	}
	if err := s.esClient.CreateIndex(ctx, CommentsIndex, commentMapping); err != nil {
		return fmt.Errorf("create comments index: %w", err)
	}
	return nil
}

// IndexPost indexes a single post
func (s *SearchService) IndexPost(ctx context.Context, doc *PostDocument) error {
	docID := fmt.Sprintf("%s_%d", doc.BoardID, doc.PostID)
	doc.TitleSuggest = map[string]interface{}{
		"input": strings.Fields(doc.Title),
	}
	return s.esClient.IndexDocument(ctx, PostsIndex, docID, doc)
}

// DeletePost removes a post from the index
func (s *SearchService) DeletePost(ctx context.Context, boardID string, postID int) error {
	docID := fmt.Sprintf("%s_%d", boardID, postID)
	return s.esClient.DeleteDocument(ctx, PostsIndex, docID)
}

// IndexComment indexes a single comment
func (s *SearchService) IndexComment(ctx context.Context, doc *CommentDocument) error {
	docID := fmt.Sprintf("%s_%d_%d", doc.BoardID, doc.PostID, doc.CommentID)
	return s.esClient.IndexDocument(ctx, CommentsIndex, docID, doc)
}

// DeleteComment removes a comment from the index
func (s *SearchService) DeleteComment(ctx context.Context, boardID string, postID, commentID int) error {
	docID := fmt.Sprintf("%s_%d_%d", boardID, postID, commentID)
	return s.esClient.DeleteDocument(ctx, CommentsIndex, docID)
}

// SearchPosts searches posts with highlighting
func (s *SearchService) SearchPosts(ctx context.Context, keyword, boardID string, page, perPage int) (*es.SearchResponse, error) {
	must := []map[string]interface{}{
		{
			"multi_match": map[string]interface{}{
				"query":  keyword,
				"fields": []string{"title^3", "content", "author"},
				"type":   "best_fields",
			},
		},
	}

	var filter []map[string]interface{}
	if boardID != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{"board_id": boardID},
		})
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filter,
			},
		},
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"title":   map[string]interface{}{"number_of_fragments": 0},
				"content": map[string]interface{}{"fragment_size": 150, "number_of_fragments": 3},
			},
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
		},
		"sort": []interface{}{
			"_score",
			map[string]interface{}{"created_at": map[string]interface{}{"order": "desc"}},
		},
	}

	from := (page - 1) * perPage
	return s.esClient.Search(ctx, PostsIndex, query, from, perPage)
}

// SearchComments searches comments with highlighting
func (s *SearchService) SearchComments(ctx context.Context, keyword, boardID string, page, perPage int) (*es.SearchResponse, error) {
	must := []map[string]interface{}{
		{
			"match": map[string]interface{}{
				"content": map[string]interface{}{
					"query":    keyword,
					"analyzer": "korean",
				},
			},
		},
	}

	var filter []map[string]interface{}
	if boardID != "" {
		filter = append(filter, map[string]interface{}{
			"term": map[string]interface{}{"board_id": boardID},
		})
	}

	query := map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"must":   must,
				"filter": filter,
			},
		},
		"highlight": map[string]interface{}{
			"fields": map[string]interface{}{
				"content": map[string]interface{}{"fragment_size": 200, "number_of_fragments": 3},
			},
			"pre_tags":  []string{"<mark>"},
			"post_tags": []string{"</mark>"},
		},
		"sort": []interface{}{
			"_score",
			map[string]interface{}{"created_at": map[string]interface{}{"order": "desc"}},
		},
	}

	from := (page - 1) * perPage
	return s.esClient.Search(ctx, CommentsIndex, query, from, perPage)
}

// UnifiedSearch searches across both posts and comments
func (s *SearchService) UnifiedSearch(ctx context.Context, keyword, boardID, searchType string, page, perPage int) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	switch searchType {
	case "posts":
		posts, err := s.SearchPosts(ctx, keyword, boardID, page, perPage)
		if err != nil {
			return nil, err
		}
		result["posts"] = posts
	case "comments":
		comments, err := s.SearchComments(ctx, keyword, boardID, page, perPage)
		if err != nil {
			return nil, err
		}
		result["comments"] = comments
	default:
		// Search both
		posts, err := s.SearchPosts(ctx, keyword, boardID, page, perPage)
		if err != nil {
			return nil, err
		}
		comments, err := s.SearchComments(ctx, keyword, boardID, 1, 5) // 댓글은 상위 5개만
		if err != nil {
			return nil, err
		}
		result["posts"] = posts
		result["comments"] = comments
	}

	return result, nil
}

// Autocomplete returns title suggestions
func (s *SearchService) Autocomplete(ctx context.Context, prefix string, size int) ([]string, error) {
	if size <= 0 {
		size = 10
	}
	return s.esClient.Suggest(ctx, PostsIndex, "title_suggest", prefix, size)
}

// BulkIndexPosts indexes multiple posts from the database (for initial sync)
func (s *SearchService) BulkIndexPosts(ctx context.Context, boardID string, limit int) (int, error) {
	if s.db == nil {
		return 0, fmt.Errorf("database not available")
	}

	tableName := fmt.Sprintf("g5_write_%s", boardID)

	var rows []struct {
		WrID      int    `gorm:"column:wr_id"`
		WrSubject string `gorm:"column:wr_subject"`
		WrContent string `gorm:"column:wr_content"`
		WrName    string `gorm:"column:wr_name"`
		MbID      string `gorm:"column:mb_id"`
		CaName    string `gorm:"column:ca_name"`
		WrDatetime string `gorm:"column:wr_datetime"`
		WrHit     int    `gorm:"column:wr_hit"`
		WrGood    int    `gorm:"column:wr_good"`
	}

	err := s.db.Table(tableName).
		Where("wr_is_comment = 0").
		Order("wr_id DESC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return 0, err
	}

	docs := make(map[string]interface{})
	for _, row := range rows {
		docID := fmt.Sprintf("%s_%d", boardID, row.WrID)
		docs[docID] = PostDocument{
			BoardID:   boardID,
			PostID:    row.WrID,
			Title:     row.WrSubject,
			Content:   stripHTML(row.WrContent),
			Author:    row.WrName,
			AuthorID:  row.MbID,
			Category:  row.CaName,
			CreatedAt: row.WrDatetime,
			Views:     row.WrHit,
			Good:      row.WrGood,
			TitleSuggest: map[string]interface{}{
				"input": strings.Fields(row.WrSubject),
			},
		}
	}

	if err := s.esClient.BulkIndex(ctx, PostsIndex, docs); err != nil {
		return 0, err
	}

	pkglogger.GetLogger().Info().
		Str("board_id", boardID).
		Int("count", len(docs)).
		Msg("bulk indexed posts")

	return len(docs), nil
}

// stripHTML removes HTML tags (simple version)
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(r)
		}
	}
	return result.String()
}

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	pkglogger "github.com/damoang/angple-backend/pkg/logger"
	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// Client wraps the Elasticsearch client with convenience methods
type Client struct {
	es *elasticsearch.Client
}

// NewClient creates a new Elasticsearch client
func NewClient(addresses []string, username, password string) (*Client, error) {
	cfg := elasticsearch.Config{
		Addresses: addresses,
	}
	if username != "" {
		cfg.Username = username
		cfg.Password = password
	}

	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("elasticsearch client creation failed: %w", err)
	}

	// Ping
	res, err := es.Info()
	if err != nil {
		return nil, fmt.Errorf("elasticsearch connection failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error: %s", res.String())
	}

	pkglogger.GetLogger().Info().Msg("connected to Elasticsearch")
	return &Client{es: es}, nil
}

// IndexDocument indexes a single document
func (c *Client) IndexDocument(ctx context.Context, index, docID string, body interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req := esapi.IndexRequest{
		Index:      index,
		DocumentID: docID,
		Body:       bytes.NewReader(data),
		Refresh:    "false",
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("index error [%s]: failed to read response body: %w", res.Status(), err)
		}
		return fmt.Errorf("index error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// DeleteDocument removes a document from an index
func (c *Client) DeleteDocument(ctx context.Context, index, docID string) error {
	req := esapi.DeleteRequest{
		Index:      index,
		DocumentID: docID,
	}

	res, err := req.Do(ctx, c.es)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// 404 is ok (document already gone)
	if res.IsError() && res.StatusCode != 404 {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("delete error [%s]: failed to read response body: %w", res.Status(), err)
		}
		return fmt.Errorf("delete error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// BulkIndex indexes multiple documents in a single request
func (c *Client) BulkIndex(ctx context.Context, index string, docs map[string]interface{}) error {
	if len(docs) == 0 {
		return nil
	}

	var buf bytes.Buffer
	for id, doc := range docs {
		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": index,
				"_id":    id,
			},
		}
		metaLine, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("failed to marshal bulk meta for id %s: %w", id, err)
		}
		buf.Write(metaLine)
		buf.WriteByte('\n')
		dataLine, err := json.Marshal(doc)
		if err != nil {
			return fmt.Errorf("failed to marshal bulk doc for id %s: %w", id, err)
		}
		buf.Write(dataLine)
		buf.WriteByte('\n')
	}

	res, err := c.es.Bulk(bytes.NewReader(buf.Bytes()), c.es.Bulk.WithContext(ctx), c.es.Bulk.WithRefresh("false"))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("bulk error [%s]: failed to read response body: %w", res.Status(), err)
		}
		return fmt.Errorf("bulk error [%s]: %s", res.Status(), string(body))
	}
	return nil
}

// SearchResult represents a single search hit
type SearchResult struct {
	ID        string                 `json:"id"`
	Score     float64                `json:"score"`
	Source    map[string]interface{} `json:"source"`
	Highlight map[string][]string    `json:"highlight,omitempty"`
}

// SearchResponse holds search results
type SearchResponse struct {
	Total   int64          `json:"total"`
	Results []SearchResult `json:"results"`
	Suggest []string       `json:"suggest,omitempty"`
}

// Search performs a search query and returns results with highlights
func (c *Client) Search(ctx context.Context, index string, query map[string]interface{}, from, size int) (*SearchResponse, error) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, err
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(index),
		c.es.Search.WithBody(&buf),
		c.es.Search.WithFrom(from),
		c.es.Search.WithSize(size),
		c.es.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, fmt.Errorf("search error [%s]: failed to read response body: %w", res.Status(), err)
		}
		return nil, fmt.Errorf("search error [%s]: %s", res.Status(), string(body))
	}

	var raw map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, err
	}

	return parseSearchResponse(raw), nil
}

// Suggest returns autocomplete suggestions
func (c *Client) Suggest(ctx context.Context, index, field, text string, size int) ([]string, error) {
	query := map[string]interface{}{
		"suggest": map[string]interface{}{
			"autocomplete": map[string]interface{}{
				"prefix": text,
				"completion": map[string]interface{}{
					"field":           field,
					"size":            size,
					"skip_duplicates": true,
				},
			},
		},
		"_source": false,
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(query); err != nil {
		return nil, fmt.Errorf("failed to encode suggest query: %w", err)
	}

	res, err := c.es.Search(
		c.es.Search.WithContext(ctx),
		c.es.Search.WithIndex(index),
		c.es.Search.WithBody(&buf),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	var raw map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode suggest response: %w", err)
	}

	var suggestions []string
	if suggest, ok := raw["suggest"].(map[string]interface{}); ok {
		if autocomplete, ok := suggest["autocomplete"].([]interface{}); ok && len(autocomplete) > 0 {
			if first, ok := autocomplete[0].(map[string]interface{}); ok {
				if options, ok := first["options"].([]interface{}); ok {
					for _, opt := range options {
						if optMap, ok := opt.(map[string]interface{}); ok {
							if text, ok := optMap["text"].(string); ok {
								suggestions = append(suggestions, text)
							}
						}
					}
				}
			}
		}
	}

	return suggestions, nil
}

// CreateIndex creates an index with the given mapping
func (c *Client) CreateIndex(ctx context.Context, index string, mapping map[string]interface{}) error {
	// Check if index exists
	res, err := c.es.Indices.Exists([]string{index})
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode == 200 {
		return nil // Already exists
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(mapping); err != nil {
		return fmt.Errorf("failed to encode index mapping: %w", err)
	}

	res, err = c.es.Indices.Create(index, c.es.Indices.Create.WithBody(&buf), c.es.Indices.Create.WithContext(ctx))
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.IsError() {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return fmt.Errorf("create index error [%s]: failed to read response body: %w", res.Status(), err)
		}
		// Ignore "already exists" error
		if !strings.Contains(string(body), "resource_already_exists_exception") {
			return fmt.Errorf("create index error: %s", string(body))
		}
	}
	return nil
}

func parseSearchResponse(raw map[string]interface{}) *SearchResponse {
	resp := &SearchResponse{}

	if hits, ok := raw["hits"].(map[string]interface{}); ok {
		if total, ok := hits["total"].(map[string]interface{}); ok {
			if v, ok := total["value"].(float64); ok {
				resp.Total = int64(v)
			}
		}

		if hitList, ok := hits["hits"].([]interface{}); ok {
			for _, h := range hitList {
				hit, ok := h.(map[string]interface{})
				if !ok {
					continue
				}
				result := SearchResult{
					ID: fmt.Sprintf("%v", hit["_id"]),
				}
				if score, ok := hit["_score"].(float64); ok {
					result.Score = score
				}
				if source, ok := hit["_source"].(map[string]interface{}); ok {
					result.Source = source
				}
				if hl, ok := hit["highlight"].(map[string]interface{}); ok {
					result.Highlight = make(map[string][]string)
					for field, fragments := range hl {
						if fragList, ok := fragments.([]interface{}); ok {
							for _, f := range fragList {
								if s, ok := f.(string); ok {
									result.Highlight[field] = append(result.Highlight[field], s)
								}
							}
						}
					}
				}
				resp.Results = append(resp.Results, result)
			}
		}
	}

	return resp
}

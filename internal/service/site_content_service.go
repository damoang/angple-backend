package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/damoang/angple-backend/internal/common"
	"github.com/damoang/angple-backend/internal/domain"
	"github.com/damoang/angple-backend/internal/repository"
)

// SiteContentService implements the business logic for the Angple Sites builder
// (issue #1288 PoC). All methods are safe to call concurrently.
type SiteContentService struct {
	repo *repository.SiteContentRepository
}

// NewSiteContentService constructs a SiteContentService.
func NewSiteContentService(repo *repository.SiteContentRepository) *SiteContentService {
	return &SiteContentService{repo: repo}
}

// MaxBlocksJSONBytes caps the size of the blocks JSON document to keep DB rows small.
// 100 KB matches the §6 R2 risk mitigation in the PoC Sprint Contract.
const MaxBlocksJSONBytes = 100 * 1024

// blockListEnvelope mirrors the schema documented in the A1 RFC §3-2.
type blockListEnvelope struct {
	Blocks        []blockEntry `json:"blocks"`
	SchemaVersion uint16       `json:"schema_version"`
}

type blockEntry struct {
	Data map[string]any `json:"data"`
	Meta map[string]any `json:"meta,omitempty"`
	ID   string         `json:"id"`
	Type string         `json:"type"`
}

// allowedBlockTypes matches the MVP set agreed in RFC §10 (Phase 1).
var allowedBlockTypes = map[string]struct{}{
	"header":    {},
	"paragraph": {},
	"image":     {},
	"button":    {},
	"list":      {},
	"embed":     {},
	"divider":   {},
}

// validateBlocks runs the content-side schema check before persistence.
// Returns common.ErrBadRequest (wrapped) if the payload is malformed.
func validateBlocks(blocksJSON string) error {
	if blocksJSON == "" {
		return fmt.Errorf("%w: blocks cannot be empty", common.ErrBadRequest)
	}
	if len(blocksJSON) > MaxBlocksJSONBytes {
		return fmt.Errorf("%w: blocks exceed %d bytes", common.ErrBadRequest, MaxBlocksJSONBytes)
	}
	var envelope blockListEnvelope
	if err := json.Unmarshal([]byte(blocksJSON), &envelope); err != nil {
		return fmt.Errorf("%w: invalid blocks JSON: %w", common.ErrBadRequest, err)
	}
	if envelope.SchemaVersion != 1 {
		return fmt.Errorf("%w: schema_version must be 1", common.ErrBadRequest)
	}
	if len(envelope.Blocks) == 0 {
		return fmt.Errorf("%w: blocks array cannot be empty", common.ErrBadRequest)
	}
	for i, b := range envelope.Blocks {
		if b.ID == "" {
			return fmt.Errorf("%w: block[%d].id is required", common.ErrBadRequest, i)
		}
		if _, ok := allowedBlockTypes[b.Type]; !ok {
			return fmt.Errorf("%w: block[%d].type %q not in MVP set", common.ErrBadRequest, i, b.Type)
		}
	}
	return nil
}

// Get returns the content row for (site_id, content_key).
// Returns common.ErrNotFound if the row does not exist.
func (s *SiteContentService) Get(
	ctx context.Context, siteID int64, contentKey string,
) (*domain.AngpleSiteContent, error) {
	if siteID <= 0 {
		return nil, fmt.Errorf("%w: site_id must be positive", common.ErrBadRequest)
	}
	if contentKey == "" {
		return nil, fmt.Errorf("%w: content_key is required", common.ErrBadRequest)
	}
	row, err := s.repo.FindBySiteAndKey(ctx, siteID, contentKey)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, common.ErrNotFound
	}
	return row, nil
}

// Upsert validates and persists the content for (site_id, content_key).
func (s *SiteContentService) Upsert(
	ctx context.Context, siteID int64, contentKey string, req *domain.AngpleSiteContentUpsertRequest,
) (*domain.AngpleSiteContent, error) {
	if siteID <= 0 {
		return nil, fmt.Errorf("%w: site_id must be positive", common.ErrBadRequest)
	}
	if contentKey == "" {
		return nil, fmt.Errorf("%w: content_key is required", common.ErrBadRequest)
	}
	if req == nil {
		return nil, fmt.Errorf("%w: request body required", common.ErrBadRequest)
	}
	if err := validateBlocks(req.Blocks); err != nil {
		return nil, err
	}
	row := &domain.AngpleSiteContent{
		SiteID:        siteID,
		ContentKey:    contentKey,
		SchemaVersion: req.SchemaVersion,
		Blocks:        req.Blocks,
		Meta:          req.Meta,
	}
	if err := s.repo.Upsert(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// Delete removes the content row for (site_id, content_key).
func (s *SiteContentService) Delete(
	ctx context.Context, siteID int64, contentKey string,
) error {
	if siteID <= 0 {
		return fmt.Errorf("%w: site_id must be positive", common.ErrBadRequest)
	}
	if contentKey == "" {
		return fmt.Errorf("%w: content_key is required", common.ErrBadRequest)
	}
	return s.repo.Delete(ctx, siteID, contentKey)
}

// IsNotFound is a convenience helper for handler error mapping.
func IsNotFound(err error) bool {
	return errors.Is(err, common.ErrNotFound)
}

package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/damoang/angple-backend/internal/common"
)

// TestValidateBlocks covers the schema-validation path used by Upsert.
// Repository is not exercised here; integration is covered separately.
func TestValidateBlocks(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "empty payload rejected",
			input:   "",
			wantErr: true,
		},
		{
			name:    "invalid JSON rejected",
			input:   "{not json",
			wantErr: true,
		},
		{
			name:    "wrong schema_version rejected",
			input:   `{"schema_version": 2, "blocks": [{"id":"a","type":"header","data":{}}]}`,
			wantErr: true,
		},
		{
			name:    "empty blocks array rejected",
			input:   `{"schema_version": 1, "blocks": []}`,
			wantErr: true,
		},
		{
			name:    "missing block id rejected",
			input:   `{"schema_version": 1, "blocks": [{"type":"header","data":{}}]}`,
			wantErr: true,
		},
		{
			name:    "unknown block type rejected",
			input:   `{"schema_version": 1, "blocks": [{"id":"a","type":"alien","data":{}}]}`,
			wantErr: true,
		},
		{
			name:    "valid header block accepted",
			input:   `{"schema_version": 1, "blocks": [{"id":"01H8XY","type":"header","data":{"text":"hi","level":1}}]}`,
			wantErr: false,
		},
		{
			name:    "valid mixed blocks accepted",
			input:   `{"schema_version": 1, "blocks": [{"id":"a","type":"header","data":{}},{"id":"b","type":"paragraph","data":{}},{"id":"c","type":"image","data":{}},{"id":"d","type":"button","data":{}},{"id":"e","type":"list","data":{}},{"id":"f","type":"embed","data":{}},{"id":"g","type":"divider","data":{}}]}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateBlocks(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateBlocks(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, common.ErrBadRequest) {
				t.Fatalf("expected error to wrap common.ErrBadRequest; got %v", err)
			}
		})
	}
}

// TestValidateBlocksTooLarge ensures the size guard at MaxBlocksJSONBytes triggers.
func TestValidateBlocksTooLarge(t *testing.T) {
	prefix := `{"schema_version": 1, "blocks": [{"id":"x","type":"paragraph","data":{"text":"`
	suffix := `"}}]}`
	filler := strings.Repeat("a", MaxBlocksJSONBytes+1)
	payload := prefix + filler + suffix

	err := validateBlocks(payload)
	if err == nil {
		t.Fatalf("expected size guard to reject %d-byte payload", len(payload))
	}
	if !errors.Is(err, common.ErrBadRequest) {
		t.Fatalf("expected ErrBadRequest, got %v", err)
	}
}

package gnuboard

import (
	"strings"
	"testing"
)

func TestParseNoticeIDs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []int
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []int{},
		},
		{
			name:     "single ID",
			input:    "123",
			expected: []int{123},
		},
		{
			name:     "multiple IDs",
			input:    "1,2,3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "IDs with spaces",
			input:    "1, 2, 3",
			expected: []int{1, 2, 3},
		},
		{
			name:     "IDs with trailing comma",
			input:    "1,2,3,",
			expected: []int{1, 2, 3},
		},
		{
			name:     "IDs with empty entries",
			input:    "1,,3",
			expected: []int{1, 3},
		},
		{
			name:     "real world example",
			input:    "158,157,156",
			expected: []int{158, 157, 156},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseNoticeIDs(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("ParseNoticeIDs(%q) = %v, want %v", tt.input, result, tt.expected)
				return
			}
			for i, v := range result {
				if v != tt.expected[i] {
					t.Errorf("ParseNoticeIDs(%q)[%d] = %d, want %d", tt.input, i, v, tt.expected[i])
				}
			}
		})
	}
}

func TestTableName(t *testing.T) {
	tests := []struct {
		boardID  string
		expected string
	}{
		{"free", "g5_write_free"},
		{"notice", "g5_write_notice"},
		{"qna", "g5_write_qna"},
	}

	for _, tt := range tests {
		result := tableName(tt.boardID)
		if result != tt.expected {
			t.Errorf("tableName(%q) = %q, want %q", tt.boardID, result, tt.expected)
		}
	}
}

func TestMessageSubjectDateExpr(t *testing.T) {
	expr := messageSubjectDateExpr("wr_subject")
	checks := []string{
		"STR_TO_DATE(TRIM(wr_subject), '%Y.%m.%d')",
		"STR_TO_DATE(TRIM(wr_subject), '%Y.%c.%e')",
		"STR_TO_DATE(TRIM(wr_subject), '%Y-%m-%d')",
		"STR_TO_DATE(TRIM(wr_subject), '%Y-%c-%e')",
	}

	for _, check := range checks {
		if !strings.Contains(expr, check) {
			t.Fatalf("messageSubjectDateExpr() missing %q in %q", check, expr)
		}
	}
}

package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAddAdminApproval_EmptyString(t *testing.T) {
	result, err := AddAdminApproval("", "admin123")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(result), &approvals); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(approvals) != 1 {
		t.Errorf("Expected 1 approval, got %d", len(approvals))
	}
	if approvals[0].MbID != "admin123" {
		t.Errorf("Expected mb_id='admin123', got '%s'", approvals[0].MbID)
	}
	if approvals[0].Datetime == "" {
		t.Error("Expected datetime to be set")
	}
}

func TestAddAdminApproval_AddToExisting(t *testing.T) {
	existing := `[{"mb_id":"admin1","datetime":"2025-01-21 10:00:00"}]`
	result, err := AddAdminApproval(existing, "admin2")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(result), &approvals); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(approvals) != 2 {
		t.Errorf("Expected 2 approvals, got %d", len(approvals))
	}

	// Verify first admin preserved
	if approvals[0].MbID != "admin1" {
		t.Errorf("Expected first mb_id='admin1', got '%s'", approvals[0].MbID)
	}

	// Verify second admin added
	if approvals[1].MbID != "admin2" {
		t.Errorf("Expected second mb_id='admin2', got '%s'", approvals[1].MbID)
	}
}

func TestAddAdminApproval_PreventDuplicate(t *testing.T) {
	existing := `[{"mb_id":"admin123","datetime":"2025-01-21 10:00:00"}]`
	result, err := AddAdminApproval(existing, "admin123")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return existing JSON unchanged
	if result != existing {
		t.Errorf("Expected unchanged JSON for duplicate admin")
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(result), &approvals); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(approvals) != 1 {
		t.Errorf("Expected 1 approval (no duplicate), got %d", len(approvals))
	}
}

func TestAddAdminApproval_RecoverFromPlainText(t *testing.T) {
	// Simulate the bug: admin_users = "admin123" (plain text)
	plainText := "admin123"
	result, err := AddAdminApproval(plainText, "admin456")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(result), &approvals); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Should start fresh with only new admin (old plain text discarded)
	if len(approvals) != 1 {
		t.Errorf("Expected 1 approval (fresh start), got %d", len(approvals))
	}
	if approvals[0].MbID != "admin456" {
		t.Errorf("Expected mb_id='admin456', got '%s'", approvals[0].MbID)
	}
}

func TestAddAdminApproval_RecoverFromBoolean(t *testing.T) {
	// Simulate the bug: admin_users = "1" (boolean true as string)
	boolString := "1"
	result, err := AddAdminApproval(boolString, "admin789")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	var approvals AdminApprovalList
	if err := json.Unmarshal([]byte(result), &approvals); err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(approvals) != 1 {
		t.Errorf("Expected 1 approval (fresh start), got %d", len(approvals))
	}
	if approvals[0].MbID != "admin789" {
		t.Errorf("Expected mb_id='admin789', got '%s'", approvals[0].MbID)
	}
}

func TestParseAdminUsers_ValidJSON(t *testing.T) {
	validJSON := `[{"mb_id":"admin1","datetime":"2025-01-21 10:00:00"},{"mb_id":"admin2","datetime":"2025-01-21 11:00:00"}]`
	approvals, err := ParseAdminUsers(validJSON)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(approvals) != 2 {
		t.Errorf("Expected 2 approvals, got %d", len(approvals))
	}

	if approvals[0].MbID != "admin1" {
		t.Errorf("Expected first mb_id='admin1', got '%s'", approvals[0].MbID)
	}
	if approvals[1].MbID != "admin2" {
		t.Errorf("Expected second mb_id='admin2', got '%s'", approvals[1].MbID)
	}
}

func TestParseAdminUsers_EmptyString(t *testing.T) {
	approvals, err := ParseAdminUsers("")
	if err != nil {
		t.Fatalf("Expected no error for empty string, got: %v", err)
	}

	if len(approvals) != 0 {
		t.Errorf("Expected 0 approvals for empty string, got %d", len(approvals))
	}
}

func TestParseAdminUsers_InvalidJSON(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{"plain text", "admin123"},
		{"boolean string", "1"},
		{"malformed JSON", `{"mb_id":"admin1"`},
		{"not an array", `{"mb_id":"admin1","datetime":"2025-01-21 10:00:00"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseAdminUsers(tc.input)
			if err == nil {
				t.Errorf("Expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), "invalid admin_users JSON format") {
				t.Errorf("Expected 'invalid admin_users JSON format' error, got: %v", err)
			}
		})
	}
}

func TestAdminApproval_JSONMarshaling(t *testing.T) {
	approval := AdminApproval{
		MbID:     "testadmin",
		Datetime: "2025-01-21 12:00:00",
	}

	data, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var unmarshaled AdminApproval
	if err := json.Unmarshal(data, &unmarshaled); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if unmarshaled.MbID != approval.MbID {
		t.Errorf("Expected mb_id='%s', got '%s'", approval.MbID, unmarshaled.MbID)
	}
	if unmarshaled.Datetime != approval.Datetime {
		t.Errorf("Expected datetime='%s', got '%s'", approval.Datetime, unmarshaled.Datetime)
	}
}

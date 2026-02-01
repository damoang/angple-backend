package service

import (
	"testing"

	"github.com/damoang/angple-backend/internal/plugin"
)

func TestValidateSetting_String(t *testing.T) {
	schema := plugin.SettingConfig{Key: "title", Type: "string"}
	if err := ValidateSetting(schema, "anything"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateSetting_Textarea(t *testing.T) {
	schema := plugin.SettingConfig{Key: "desc", Type: "textarea"}
	if err := ValidateSetting(schema, "long text\nwith newlines"); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateSetting_Number_Valid(t *testing.T) {
	min, max := 0, 100
	schema := plugin.SettingConfig{Key: "count", Type: "number", Min: &min, Max: &max}

	tests := []struct {
		value string
		ok    bool
	}{
		{"50", true},
		{"0", true},
		{"100", true},
		{"-1", false},
		{"101", false},
		{"abc", false},
		{"3.14", true},
	}

	for _, tc := range tests {
		err := ValidateSetting(schema, tc.value)
		if tc.ok && err != nil {
			t.Errorf("value %q: expected no error, got %v", tc.value, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("value %q: expected error, got nil", tc.value)
		}
	}
}

func TestValidateSetting_Number_NoMinMax(t *testing.T) {
	schema := plugin.SettingConfig{Key: "count", Type: "number"}
	if err := ValidateSetting(schema, "-999"); err != nil {
		t.Errorf("expected no error without min/max, got %v", err)
	}
}

func TestValidateSetting_Boolean(t *testing.T) {
	schema := plugin.SettingConfig{Key: "enabled", Type: "boolean"}

	if err := ValidateSetting(schema, "true"); err != nil {
		t.Errorf("expected no error for 'true', got %v", err)
	}
	if err := ValidateSetting(schema, "false"); err != nil {
		t.Errorf("expected no error for 'false', got %v", err)
	}
	if err := ValidateSetting(schema, "yes"); err == nil {
		t.Error("expected error for 'yes', got nil")
	}
	if err := ValidateSetting(schema, "1"); err == nil {
		t.Error("expected error for '1', got nil")
	}
}

func TestValidateSetting_Select(t *testing.T) {
	schema := plugin.SettingConfig{
		Key:  "theme",
		Type: "select",
		Options: []plugin.SettingOption{
			{Value: "light", Label: "Light"},
			{Value: "dark", Label: "Dark"},
		},
	}

	if err := ValidateSetting(schema, "light"); err != nil {
		t.Errorf("expected no error for valid option, got %v", err)
	}
	if err := ValidateSetting(schema, "blue"); err == nil {
		t.Error("expected error for invalid option, got nil")
	}
}

func TestConvertSettingValue(t *testing.T) {
	tests := []struct {
		schema plugin.SettingConfig
		raw    string
		want   interface{}
	}{
		{plugin.SettingConfig{Type: "number"}, "42", float64(42)},
		{plugin.SettingConfig{Type: "number"}, "3.14", float64(3.14)},
		{plugin.SettingConfig{Type: "boolean"}, "true", true},
		{plugin.SettingConfig{Type: "boolean"}, "false", false},
		{plugin.SettingConfig{Type: "string"}, "hello", "hello"},
		{plugin.SettingConfig{Type: "select"}, "dark", "dark"},
	}

	for _, tc := range tests {
		got := ConvertSettingValue(tc.schema, tc.raw)
		if got != tc.want {
			t.Errorf("ConvertSettingValue(%s, %q) = %v (%T), want %v (%T)",
				tc.schema.Type, tc.raw, got, got, tc.want, tc.want)
		}
	}
}

func TestApplyDefaults(t *testing.T) {
	schemas := []plugin.SettingConfig{
		{Key: "a", Default: "default_a"},
		{Key: "b", Default: 42},
		{Key: "c", Default: nil},
	}
	current := map[string]string{"a": "custom"}

	result := ApplyDefaults(schemas, current)

	if result["a"] != "custom" {
		t.Errorf("expected 'custom', got %q", result["a"])
	}
	if result["b"] != "42" {
		t.Errorf("expected '42', got %q", result["b"])
	}
	if _, exists := result["c"]; exists {
		t.Error("expected 'c' to not be set (nil default)")
	}
}

package service

import (
	"fmt"
	"strconv"

	"github.com/damoang/angple-backend/internal/plugin"
)

// ValidateSetting 스키마 기반 설정값 검증
func ValidateSetting(schema plugin.SettingConfig, value string) error {
	switch schema.Type {
	case "string", "textarea":
		// 모든 문자열 허용
		return nil

	case "number":
		n, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("setting %s: %q is not a valid number", schema.Key, value)
		}
		if schema.Min != nil && n < float64(*schema.Min) {
			return fmt.Errorf("setting %s: value %v is less than minimum %d", schema.Key, n, *schema.Min)
		}
		if schema.Max != nil && n > float64(*schema.Max) {
			return fmt.Errorf("setting %s: value %v is greater than maximum %d", schema.Key, n, *schema.Max)
		}
		return nil

	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("setting %s: %q is not a valid boolean (must be \"true\" or \"false\")", schema.Key, value)
		}
		return nil

	case "select":
		for _, opt := range schema.Options {
			if opt.Value == value {
				return nil
			}
		}
		return fmt.Errorf("setting %s: %q is not a valid option", schema.Key, value)

	default:
		return nil
	}
}

// ConvertSettingValue string 값을 스키마 타입에 맞게 변환
func ConvertSettingValue(schema plugin.SettingConfig, raw string) interface{} {
	switch schema.Type {
	case "number":
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			return v
		}
		return raw
	case "boolean":
		return raw == "true"
	default:
		return raw
	}
}

// ApplyDefaults 누락된 설정 키에 기본값 적용
func ApplyDefaults(schemas []plugin.SettingConfig, current map[string]string) map[string]string {
	result := make(map[string]string, len(current))
	for k, v := range current {
		result[k] = v
	}
	for _, s := range schemas {
		if _, exists := result[s.Key]; !exists && s.Default != nil {
			result[s.Key] = fmt.Sprintf("%v", s.Default)
		}
	}
	return result
}

package security

import (
	"errors"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// 검증 에러
var (
	ErrRequiredField    = errors.New("필수 필드입니다")
	ErrInvalidLength    = errors.New("길이가 유효하지 않습니다")
	ErrInvalidFormat    = errors.New("형식이 유효하지 않습니다")
	ErrInvalidValue     = errors.New("값이 유효하지 않습니다")
	ErrDangerousContent = errors.New("위험한 콘텐츠가 포함되어 있습니다")
)

// Validator 입력 검증 도구
type Validator struct {
	sanitizer *Sanitizer
}

// NewValidator 새 Validator 생성
func NewValidator() *Validator {
	return &Validator{
		sanitizer: NewSanitizer(),
	}
}

// ValidationError 검증 에러
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Field + ": " + e.Message
}

// ValidationErrors 여러 검증 에러
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return ""
	}
	var msgs []string
	for _, err := range e {
		msgs = append(msgs, err.Field+": "+err.Message)
	}
	return strings.Join(msgs, "; ")
}

// HasErrors 에러 존재 여부
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// ValidateProductName 상품명 검증
func (v *Validator) ValidateProductName(name string) error {
	name = strings.TrimSpace(name)

	if name == "" {
		return ErrRequiredField
	}

	length := utf8.RuneCountInString(name)
	if length < 2 || length > 200 {
		return ErrInvalidLength
	}

	// 위험한 패턴 검사
	if v.containsDangerousPatterns(name) {
		return ErrDangerousContent
	}

	return nil
}

// ValidateProductDescription 상품 설명 검증
func (v *Validator) ValidateProductDescription(desc string) error {
	if desc == "" {
		return nil // 선택 필드
	}

	length := utf8.RuneCountInString(desc)
	if length > 50000 {
		return ErrInvalidLength
	}

	// 위험한 스크립트 패턴 검사
	if v.containsScriptPatterns(desc) {
		return ErrDangerousContent
	}

	return nil
}

// ValidatePrice 가격 검증
func (v *Validator) ValidatePrice(price float64) error {
	if price < 0 {
		return ErrInvalidValue
	}
	if price > 100000000 { // 1억 원 제한
		return ErrInvalidValue
	}
	return nil
}

// ValidateQuantity 수량 검증
func (v *Validator) ValidateQuantity(quantity int) error {
	if quantity < 0 {
		return ErrInvalidValue
	}
	if quantity > 9999 {
		return ErrInvalidValue
	}
	return nil
}

// ValidateEmail 이메일 검증
func (v *Validator) ValidateEmail(email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return ErrRequiredField
	}

	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidFormat
	}

	return nil
}

// ValidatePhone 전화번호 검증
func (v *Validator) ValidatePhone(phone string) error {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return ErrRequiredField
	}

	// 숫자와 하이픈만 허용
	re := regexp.MustCompile(`^[0-9\-]+$`)
	if !re.MatchString(phone) {
		return ErrInvalidFormat
	}

	// 최소/최대 길이
	digits := regexp.MustCompile(`[0-9]`).FindAllString(phone, -1)
	if len(digits) < 9 || len(digits) > 15 {
		return ErrInvalidLength
	}

	return nil
}

// ValidatePostalCode 우편번호 검증 (한국)
func (v *Validator) ValidatePostalCode(code string) error {
	code = strings.TrimSpace(code)
	if code == "" {
		return ErrRequiredField
	}

	// 한국 우편번호: 5자리 숫자
	re := regexp.MustCompile(`^\d{5}$`)
	if !re.MatchString(code) {
		return ErrInvalidFormat
	}

	return nil
}

// ValidateAddress 주소 검증
func (v *Validator) ValidateAddress(address string) error {
	address = strings.TrimSpace(address)
	if address == "" {
		return ErrRequiredField
	}

	length := utf8.RuneCountInString(address)
	if length < 5 || length > 500 {
		return ErrInvalidLength
	}

	// 위험한 패턴 검사
	if v.containsDangerousPatterns(address) {
		return ErrDangerousContent
	}

	return nil
}

// ValidateURL URL 검증
func (v *Validator) ValidateURL(urlStr string) error {
	urlStr = strings.TrimSpace(urlStr)
	if urlStr == "" {
		return nil // 선택 필드
	}

	// URL 파싱
	parsed, err := url.Parse(urlStr)
	if err != nil {
		return ErrInvalidFormat
	}

	// 허용된 스킴만
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrInvalidFormat
	}

	// 호스트 필수
	if parsed.Host == "" {
		return ErrInvalidFormat
	}

	return nil
}

// ValidateSlug 슬러그 검증
func (v *Validator) ValidateSlug(slug string) error {
	if slug == "" {
		return nil // 선택 필드 (자동 생성 가능)
	}

	// 영문, 숫자, 하이픈, 한글만 허용
	re := regexp.MustCompile(`^[a-z0-9가-힣\-]+$`)
	if !re.MatchString(slug) {
		return ErrInvalidFormat
	}

	length := utf8.RuneCountInString(slug)
	if length < 2 || length > 200 {
		return ErrInvalidLength
	}

	return nil
}

// containsDangerousPatterns 위험한 패턴 검사
func (v *Validator) containsDangerousPatterns(input string) bool {
	lower := strings.ToLower(input)

	// 스크립트 관련 패턴
	dangerousPatterns := []string{
		"<script", "</script", "javascript:",
		"onerror=", "onload=", "onclick=", "onmouseover=",
		"onfocus=", "onblur=", "onsubmit=",
		"<iframe", "<frame", "<object", "<embed",
		"expression(", "eval(", "alert(",
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// containsScriptPatterns 스크립트 패턴만 검사
func (v *Validator) containsScriptPatterns(input string) bool {
	lower := strings.ToLower(input)

	// 스크립트 태그 검사
	scriptPatterns := []string{
		"<script", "</script",
		"javascript:",
		"vbscript:",
	}

	for _, pattern := range scriptPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

// SanitizeAndValidate 살균화 후 검증
func (v *Validator) SanitizeAndValidate(input string, maxLength int) (string, error) {
	// 살균화
	sanitized := v.sanitizer.SanitizeString(input)

	// 길이 검증
	if utf8.RuneCountInString(sanitized) > maxLength {
		return "", ErrInvalidLength
	}

	return sanitized, nil
}

// ValidateProductRequest 상품 요청 전체 검증
func (v *Validator) ValidateProductRequest(name, desc, slug string, price float64) ValidationErrors {
	var errs ValidationErrors

	if err := v.ValidateProductName(name); err != nil {
		errs = append(errs, ValidationError{Field: "name", Message: err.Error()})
	}

	if err := v.ValidateProductDescription(desc); err != nil {
		errs = append(errs, ValidationError{Field: "description", Message: err.Error()})
	}

	if err := v.ValidateSlug(slug); err != nil {
		errs = append(errs, ValidationError{Field: "slug", Message: err.Error()})
	}

	if err := v.ValidatePrice(price); err != nil {
		errs = append(errs, ValidationError{Field: "price", Message: err.Error()})
	}

	return errs
}

// ValidateShippingInfo 배송 정보 검증
func (v *Validator) ValidateShippingInfo(name, phone, address, postal string) ValidationErrors {
	var errs ValidationErrors

	if name == "" {
		errs = append(errs, ValidationError{Field: "shipping_name", Message: ErrRequiredField.Error()})
	} else if utf8.RuneCountInString(name) > 100 {
		errs = append(errs, ValidationError{Field: "shipping_name", Message: ErrInvalidLength.Error()})
	}

	if err := v.ValidatePhone(phone); err != nil {
		errs = append(errs, ValidationError{Field: "shipping_phone", Message: err.Error()})
	}

	if err := v.ValidateAddress(address); err != nil {
		errs = append(errs, ValidationError{Field: "shipping_address", Message: err.Error()})
	}

	if err := v.ValidatePostalCode(postal); err != nil {
		errs = append(errs, ValidationError{Field: "shipping_postal", Message: err.Error()})
	}

	return errs
}

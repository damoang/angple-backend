package service

import (
	"regexp"
	"strings"

	"github.com/damoang/angple-backend/internal/repository"
)

// MemberValidationService handles member validation operations
type MemberValidationService interface {
	ValidateUserID(userID string) *ValidationResult
	ValidateNickname(nickname string, excludeUserID string) *ValidationResult
	ValidateEmail(email string, excludeUserID string) *ValidationResult
	ValidatePhone(phone string, excludeUserID string) *ValidationResult
}

// ValidationResult represents the result of a validation
type ValidationResult struct {
	Valid   bool   `json:"valid"`
	Message string `json:"message,omitempty"`
	Field   string `json:"field,omitempty"`
}

type memberValidationService struct {
	repo              repository.MemberRepository
	prohibitedIDs     []string
	prohibitedDomains []string
}

// NewMemberValidationService creates a new MemberValidationService
func NewMemberValidationService(repo repository.MemberRepository) MemberValidationService {
	return &memberValidationService{
		repo: repo,
		// 예약어 목록 (향후 DB에서 로드 가능)
		prohibitedIDs: []string{
			"admin", "administrator", "root", "system", "test",
			"guest", "anonymous", "null", "undefined", "api",
		},
		// 금지 이메일 도메인 (향후 DB에서 로드 가능)
		prohibitedDomains: []string{
			"tempmail.com", "throwaway.com", "mailinator.com",
		},
	}
}

// ValidateUserID validates a user ID
func (s *memberValidationService) ValidateUserID(userID string) *ValidationResult {
	userID = strings.TrimSpace(userID)

	// 빈값 체크
	if userID == "" {
		return &ValidationResult{
			Valid:   false,
			Message: "회원아이디를 입력해 주십시오.",
			Field:   "user_id",
		}
	}

	// 형식 체크 (영문, 숫자, _ 만 허용)
	validPattern := regexp.MustCompile(`^[0-9a-zA-Z_]+$`)
	if !validPattern.MatchString(userID) {
		return &ValidationResult{
			Valid:   false,
			Message: "회원아이디는 영문자, 숫자, _ 만 입력하세요.",
			Field:   "user_id",
		}
	}

	// 최소 길이 체크
	if len(userID) < 3 {
		return &ValidationResult{
			Valid:   false,
			Message: "회원아이디는 최소 3글자 이상 입력하세요.",
			Field:   "user_id",
		}
	}

	// 최대 길이 체크
	if len(userID) > 20 {
		return &ValidationResult{
			Valid:   false,
			Message: "회원아이디는 최대 20글자까지 입력 가능합니다.",
			Field:   "user_id",
		}
	}

	// 예약어 체크
	lowerID := strings.ToLower(userID)
	for _, prohibited := range s.prohibitedIDs {
		if lowerID == prohibited {
			return &ValidationResult{
				Valid:   false,
				Message: "이미 예약된 단어로 사용할 수 없는 회원아이디 입니다.",
				Field:   "user_id",
			}
		}
	}

	// 중복 체크
	exists, err := s.repo.ExistsByUserID(userID)
	if err != nil {
		return &ValidationResult{
			Valid:   false,
			Message: "회원아이디 확인 중 오류가 발생했습니다.",
			Field:   "user_id",
		}
	}
	if exists {
		return &ValidationResult{
			Valid:   false,
			Message: "이미 사용중인 회원아이디 입니다.",
			Field:   "user_id",
		}
	}

	return &ValidationResult{
		Valid:   true,
		Message: "사용 가능한 회원아이디 입니다.",
		Field:   "user_id",
	}
}

// ValidateNickname validates a nickname
func (s *memberValidationService) ValidateNickname(nickname string, excludeUserID string) *ValidationResult {
	nickname = strings.TrimSpace(nickname)

	// 빈값 체크
	if nickname == "" {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임을 입력해 주십시오.",
			Field:   "nickname",
		}
	}

	// 형식 체크 (한글, 영문, 숫자, 연속되지 않는 ._ 허용)
	// 연속된 .. 또는 __ 금지
	if strings.Contains(nickname, "..") || strings.Contains(nickname, "__") {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임은 연속된 점(.)이나 밑줄(_)을 사용할 수 없습니다.",
			Field:   "nickname",
		}
	}

	// 허용 문자 체크 (한글, 영문, 숫자, ._ 만)
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9가-힣ㄱ-ㅎㅏ-ㅣ_.]+$`)
	if !validPattern.MatchString(nickname) {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임은 공백없이 한글, 영문, 숫자, 점(.)과 밑줄(_)만 입력 가능합니다.",
			Field:   "nickname",
		}
	}

	// 최소 길이 체크 (바이트 기준 4 = 영문 4글자 또는 한글 약 1.3글자)
	if len(nickname) < 4 {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임은 한글 2글자, 영문 4글자 이상 입력 가능합니다.",
			Field:   "nickname",
		}
	}

	// 최대 길이 체크
	if len(nickname) > 20 {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임은 최대 20자까지 입력 가능합니다.",
			Field:   "nickname",
		}
	}

	// 예약어 체크
	lowerNick := strings.ToLower(nickname)
	for _, prohibited := range s.prohibitedIDs {
		if lowerNick == prohibited {
			return &ValidationResult{
				Valid:   false,
				Message: "이미 예약된 단어로 사용할 수 없는 닉네임 입니다.",
				Field:   "nickname",
			}
		}
	}

	// 중복 체크
	exists, err := s.repo.ExistsByNickname(nickname, excludeUserID)
	if err != nil {
		return &ValidationResult{
			Valid:   false,
			Message: "닉네임 확인 중 오류가 발생했습니다.",
			Field:   "nickname",
		}
	}
	if exists {
		return &ValidationResult{
			Valid:   false,
			Message: "이미 존재하는 닉네임입니다.",
			Field:   "nickname",
		}
	}

	return &ValidationResult{
		Valid:   true,
		Message: "사용 가능한 닉네임 입니다.",
		Field:   "nickname",
	}
}

// ValidateEmail validates an email address
func (s *memberValidationService) ValidateEmail(email string, excludeUserID string) *ValidationResult {
	email = strings.TrimSpace(email)

	// 빈값 체크
	if email == "" {
		return &ValidationResult{
			Valid:   false,
			Message: "E-mail 주소를 입력해 주십시오.",
			Field:   "email",
		}
	}

	// 형식 체크
	emailPattern := regexp.MustCompile(`^[0-9a-zA-Z_\-\.]+@[0-9a-zA-Z_\-]+\.[0-9a-zA-Z_\-]+$`)
	if !emailPattern.MatchString(email) {
		return &ValidationResult{
			Valid:   false,
			Message: "E-mail 주소가 형식에 맞지 않습니다.",
			Field:   "email",
		}
	}

	// 금지 도메인 체크
	parts := strings.Split(email, "@")
	if len(parts) == 2 {
		domain := strings.ToLower(parts[1])
		for _, prohibited := range s.prohibitedDomains {
			if domain == prohibited {
				return &ValidationResult{
					Valid:   false,
					Message: domain + " 메일은 사용할 수 없습니다.",
					Field:   "email",
				}
			}
		}
	}

	// 중복 체크
	exists, err := s.repo.ExistsByEmailExcluding(email, excludeUserID)
	if err != nil {
		return &ValidationResult{
			Valid:   false,
			Message: "E-mail 확인 중 오류가 발생했습니다.",
			Field:   "email",
		}
	}
	if exists {
		return &ValidationResult{
			Valid:   false,
			Message: "이미 사용중인 E-mail 주소입니다.",
			Field:   "email",
		}
	}

	return &ValidationResult{
		Valid:   true,
		Message: "사용 가능한 E-mail 주소입니다.",
		Field:   "email",
	}
}

// ValidatePhone validates a phone number
func (s *memberValidationService) ValidatePhone(phone string, excludeUserID string) *ValidationResult {
	// 숫자만 추출
	digitsOnly := regexp.MustCompile(`[^0-9]`)
	phone = digitsOnly.ReplaceAllString(phone, "")

	// 빈값 체크
	if phone == "" {
		return &ValidationResult{
			Valid:   false,
			Message: "휴대폰번호를 입력해 주십시오.",
			Field:   "phone",
		}
	}

	// 형식 체크 (01X로 시작, 10-11자리)
	phonePattern := regexp.MustCompile(`^01[0-9]{8,9}$`)
	if !phonePattern.MatchString(phone) {
		return &ValidationResult{
			Valid:   false,
			Message: "휴대폰번호를 올바르게 입력해 주십시오.",
			Field:   "phone",
		}
	}

	// 하이픈 형식으로 변환 (DB 저장 형식)
	formattedPhone := formatPhoneNumber(phone)

	// 중복 체크
	exists, err := s.repo.ExistsByPhone(formattedPhone, excludeUserID)
	if err != nil {
		return &ValidationResult{
			Valid:   false,
			Message: "휴대폰번호 확인 중 오류가 발생했습니다.",
			Field:   "phone",
		}
	}
	if exists {
		return &ValidationResult{
			Valid:   false,
			Message: "이미 사용 중인 휴대폰번호입니다.",
			Field:   "phone",
		}
	}

	return &ValidationResult{
		Valid:   true,
		Message: "사용 가능한 휴대폰번호입니다.",
		Field:   "phone",
	}
}

// formatPhoneNumber converts phone to hyphen format (010-1234-5678)
func formatPhoneNumber(phone string) string {
	if len(phone) == 10 {
		// 010-123-4567
		return phone[:3] + "-" + phone[3:6] + "-" + phone[6:]
	} else if len(phone) == 11 {
		// 010-1234-5678
		return phone[:3] + "-" + phone[3:7] + "-" + phone[7:]
	}
	return phone
}

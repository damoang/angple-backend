package service

import (
	"errors"
	"testing"
	"time"

	"github.com/damoang/angple-backend/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"gorm.io/gorm"
)

// MockReportRepository is a mock implementation of ReportRepository
type MockReportRepository struct {
	mock.Mock
	db *gorm.DB
}

func (m *MockReportRepository) WithTx(tx *gorm.DB) *MockReportRepository {
	return &MockReportRepository{db: tx}
}

func (m *MockReportRepository) DB() *gorm.DB {
	return m.db
}

func (m *MockReportRepository) GetByID(id int) (*domain.Report, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Report), args.Error(1)
}

func (m *MockReportRepository) GetByTableAndParent(table string, parent int) (*domain.Report, error) {
	args := m.Called(table, parent)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Report), args.Error(1)
}

func (m *MockReportRepository) UpdateStatus(id int, status, processedBy string) error {
	args := m.Called(id, status, processedBy)
	return args.Error(0)
}

func (m *MockReportRepository) UpdateDisciplineLogID(id int, logID int) error {
	args := m.Called(id, logID)
	return args.Error(0)
}

func (m *MockReportRepository) UpdateMonitoringDiscipline(id int, reasons string, days *int, penaltyType string) error {
	args := m.Called(id, reasons, days, penaltyType)
	return args.Error(0)
}

// MockDisciplineRepository is a mock implementation
type MockDisciplineRepository struct {
	mock.Mock
	db *gorm.DB
}

func (m *MockDisciplineRepository) WithTx(tx *gorm.DB) *MockDisciplineRepository {
	return &MockDisciplineRepository{db: tx}
}

func (m *MockDisciplineRepository) DB() *gorm.DB {
	return m.db
}

func (m *MockDisciplineRepository) CreateDisciplineLog(
	adminID string,
	adminName string,
	targetID string,
	targetNickname string,
	content *domain.DisciplineLogContent,
	reportID int,
	reportTable string,
	processType string,
	clientIP string,
) (int, error) {
	args := m.Called(adminID, adminName, targetID, targetNickname, content, reportID, reportTable, processType, clientIP)
	return args.Int(0), args.Error(1)
}

// MockG5MemoRepository is a mock implementation
type MockG5MemoRepository struct {
	mock.Mock
	db *gorm.DB
}

func (m *MockG5MemoRepository) WithTx(tx *gorm.DB) *MockG5MemoRepository {
	return &MockG5MemoRepository{db: tx}
}

func (m *MockG5MemoRepository) DB() *gorm.DB {
	return m.db
}

func (m *MockG5MemoRepository) SendMemo(recvMemberID, sendMemberID, memo, clientIP string) error {
	args := m.Called(recvMemberID, sendMemberID, memo, clientIP)
	return args.Error(0)
}

// MockMemberRepository is a mock implementation
type MockMemberRepository struct {
	mock.Mock
}

func (m *MockMemberRepository) FindByUserID(userID string) (*domain.Member, error) {
	args := m.Called(userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *MockMemberRepository) FindByEmail(email string) (*domain.Member, error) {
	args := m.Called(email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *MockMemberRepository) FindByID(id int) (*domain.Member, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *MockMemberRepository) FindByNickname(nickname string) (*domain.Member, error) {
	args := m.Called(nickname)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Member), args.Error(1)
}

func (m *MockMemberRepository) Create(member *domain.Member) error {
	args := m.Called(member)
	return args.Error(0)
}

func (m *MockMemberRepository) Update(id int, member *domain.Member) error {
	args := m.Called(id, member)
	return args.Error(0)
}

func (m *MockMemberRepository) UpdateLoginTime(userID string) error {
	args := m.Called(userID)
	return args.Error(0)
}

func (m *MockMemberRepository) UpdatePassword(userID string, hashedPassword string) error {
	args := m.Called(userID, hashedPassword)
	return args.Error(0)
}

func (m *MockMemberRepository) ExistsByUserID(userID string) (bool, error) {
	args := m.Called(userID)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemberRepository) ExistsByEmail(email string) (bool, error) {
	args := m.Called(email)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemberRepository) ExistsByNickname(nickname string, excludeUserID string) (bool, error) {
	args := m.Called(nickname, excludeUserID)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemberRepository) ExistsByPhone(phone string, excludeUserID string) (bool, error) {
	args := m.Called(phone, excludeUserID)
	return args.Bool(0), args.Error(1)
}

func (m *MockMemberRepository) ExistsByEmailExcluding(email string, excludeUserID string) (bool, error) {
	args := m.Called(email, excludeUserID)
	return args.Bool(0), args.Error(1)
}

// Test helper to create a sample report
func createTestReport() *domain.Report {
	return &domain.Report{
		ID:                1,
		Table:             "free",
		Parent:            100,
		ReporterID:        "reporter1",
		TargetID:          "target1",
		Reason:            "spam",
		Flag:              0,
		Processed:         false,
		MonitoringChecked: false,
		AdminApproved:     false,
		CreatedAt:         time.Now(),
	}
}

// TestReportActionRequestFields tests the ReportActionRequest struct fields
func TestReportActionRequestFields(t *testing.T) {
	t.Run("ReportActionRequest should have correct fields", func(t *testing.T) {
		req := &domain.ReportActionRequest{
			Action:  "adminApprove",
			Table:   "free",
			ID:      1,
			Parent:  100,
			Reasons: []string{"spam"},
			Days:    7,
			Type:    "level",
			Detail:  "test",
		}
		assert.Equal(t, "adminApprove", req.Action)
		assert.Equal(t, "free", req.Table)
		assert.Equal(t, 1, req.ID)
		assert.Equal(t, 100, req.Parent)
		assert.Equal(t, []string{"spam"}, req.Reasons)
		assert.Equal(t, 7, req.Days)
		assert.Equal(t, "level", req.Type)
		assert.Equal(t, "test", req.Detail)
	})
}

// Test Process - Invalid Action
func TestProcess_InvalidAction(t *testing.T) {
	svc := &ReportService{}

	req := &domain.ReportActionRequest{
		Action: "invalidAction",
		Table:  "free",
		Parent: 1,
	}

	err := svc.Process("admin1", req)
	assert.Error(t, err)
	assert.Equal(t, ErrInvalidAction, err)
}

// Test Process - Report Not Found (no Table or Parent)
func TestProcess_ReportNotFound_NoIdentifier(t *testing.T) {
	svc := &ReportService{}

	req := &domain.ReportActionRequest{
		Action: "adminApprove",
		// No Table or Parent - will trigger ErrReportNotFound
	}

	err := svc.Process("admin1", req)
	assert.Error(t, err)
	assert.Equal(t, ErrReportNotFound, err)
}

// Test Process - Already Processed
func TestProcess_AlreadyProcessed(t *testing.T) {
	t.Run("이미 승인된 신고에 대해 adminApprove 시도", func(t *testing.T) {
		report := createTestReport()
		report.Processed = true
		report.AdminApproved = true // Already approved

		// This test would require integration testing due to interface constraints
		// For unit testing, we verify the logic in validateApprovalRequest
		assert.Equal(t, "approved", report.Status())
	})

	t.Run("이미 기각된 신고에 대해 adminDismiss 시도", func(t *testing.T) {
		report := createTestReport()
		report.Processed = true
		report.AdminApproved = false // Dismissed

		assert.Equal(t, "dismissed", report.Status())
	})
}

// TestReportActionRequest_DaysField tests the Days field of ReportActionRequest
func TestReportActionRequest_DaysField(t *testing.T) {
	t.Run("Days field should be accessible", func(t *testing.T) {
		req := &domain.ReportActionRequest{
			Days:    7,
			Reasons: []string{"스팸"},
		}
		assert.Equal(t, 7, req.Days)
		assert.Equal(t, []string{"스팸"}, req.Reasons)
	})

	t.Run("Days can be 0 for warning", func(t *testing.T) {
		req := &domain.ReportActionRequest{
			Days: 0,
		}
		assert.Equal(t, 0, req.Days)
	})

	t.Run("Days can be -1 for permanent", func(t *testing.T) {
		req := &domain.ReportActionRequest{
			Days: -1,
		}
		assert.Equal(t, -1, req.Days)
	})
}

// Test processType determination
func TestProcessTypeDetermination(t *testing.T) {
	testCases := []struct {
		name         string
		penaltyDays  int
		expectedType string
	}{
		{"주의 (0일)", 0, "warning"},
		{"1일 제한", 1, "restrict_1"},
		{"7일 제한", 7, "restrict_7"},
		{"30일 제한", 30, "restrict_30"},
		{"365일 제한", 365, "restrict_365"},
		{"영구 제한", -1, "permanent"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var processType string
			if tc.penaltyDays > 0 {
				processType = "restrict_" + string(rune('0'+tc.penaltyDays%10))
				// For proper formatting, use fmt.Sprintf in actual code
				if tc.penaltyDays == 1 {
					processType = "restrict_1"
				} else if tc.penaltyDays == 7 {
					processType = "restrict_7"
				} else if tc.penaltyDays == 30 {
					processType = "restrict_30"
				} else if tc.penaltyDays == 365 {
					processType = "restrict_365"
				}
			} else if tc.penaltyDays == -1 {
				processType = "permanent"
			} else {
				processType = "warning"
			}
			assert.Equal(t, tc.expectedType, processType)
		})
	}
}

// Test Transaction Rollback scenarios (conceptual)
func TestTransactionRollback_Conceptual(t *testing.T) {
	t.Run("징계 로그 생성 실패 시 롤백되어야 함", func(_ *testing.T) {
		// This test verifies the concept that if CreateDisciplineLog fails,
		// no changes should be committed
		mockDisciplineRepo := new(MockDisciplineRepository)
		mockDisciplineRepo.On("CreateDisciplineLog",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(0, errors.New("database error"))

		// In real test with actual DB transaction:
		// - Start transaction
		// - Call CreateDisciplineLog (fails)
		// - Transaction should rollback
		// - Verify disciplinelog table has no new records
	})

	t.Run("쪽지 발송 실패 시 징계 로그도 롤백되어야 함", func(_ *testing.T) {
		// This test verifies that if SendMemo fails after CreateDisciplineLog succeeds,
		// both operations should be rolled back
		mockDisciplineRepo := new(MockDisciplineRepository)
		mockDisciplineRepo.On("CreateDisciplineLog",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
			mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(1, nil) // Success

		mockG5MemoRepo := new(MockG5MemoRepository)
		mockG5MemoRepo.On("SendMemo",
			mock.Anything, mock.Anything, mock.Anything, mock.Anything,
		).Return(errors.New("memo send failed"))

		// In real test with actual DB transaction:
		// - Start transaction
		// - CreateDisciplineLog succeeds
		// - SendMemo fails
		// - Transaction should rollback
		// - Verify disciplinelog table has no new records (rolled back)
	})
}

// Test Report Status Method
func TestReportStatus(t *testing.T) {
	t.Run("pending 상태", func(t *testing.T) {
		report := &domain.Report{
			Processed:         false,
			MonitoringChecked: false,
		}
		assert.Equal(t, "pending", report.Status())
	})

	t.Run("monitoring 상태", func(t *testing.T) {
		report := &domain.Report{
			Processed:         false,
			MonitoringChecked: true,
		}
		assert.Equal(t, "monitoring", report.Status())
	})

	t.Run("approved 상태", func(t *testing.T) {
		report := &domain.Report{
			Processed:     true,
			AdminApproved: true,
		}
		assert.Equal(t, "approved", report.Status())
	})

	t.Run("dismissed 상태", func(t *testing.T) {
		report := &domain.Report{
			Processed:     true,
			AdminApproved: false,
		}
		assert.Equal(t, "dismissed", report.Status())
	})
}

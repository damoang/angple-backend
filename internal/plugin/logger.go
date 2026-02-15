package plugin

import (
	"fmt"
	"log"
)

// DefaultLogger 기본 로거 구현
type DefaultLogger struct {
	prefix string
}

// NewDefaultLogger 새 기본 로거 생성
func NewDefaultLogger(prefix string) *DefaultLogger {
	return &DefaultLogger{prefix: prefix}
}

// Debug 디버그 로그
func (l *DefaultLogger) Debug(msg string, args ...interface{}) {
	log.Printf("[DEBUG] [%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}

// Info 정보 로그
func (l *DefaultLogger) Info(msg string, args ...interface{}) {
	log.Printf("[INFO] [%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}

// Warn 경고 로그
func (l *DefaultLogger) Warn(msg string, args ...interface{}) {
	log.Printf("[WARN] [%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}

// Error 에러 로그
func (l *DefaultLogger) Error(msg string, args ...interface{}) {
	log.Printf("[ERROR] [%s] %s", l.prefix, fmt.Sprintf(msg, args...))
}

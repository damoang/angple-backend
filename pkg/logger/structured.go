package logger

import (
	"io"
	"os"
	"time"

	"github.com/rs/zerolog"
)

var zlog zerolog.Logger

// InitStructured initializes the structured zerolog logger
func InitStructured(env string) {
	var w io.Writer

	if env == "development" || env == "dev" {
		// Pretty console output for development
		w = zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339}
	} else {
		// JSON output for production (machine-readable)
		w = os.Stdout
	}

	zlog = zerolog.New(w).With().
		Timestamp().
		Str("service", "angple-backend").
		Logger()

	zerolog.TimeFieldFormat = time.RFC3339
}

// GetLogger returns the global zerolog logger
func GetLogger() *zerolog.Logger {
	return &zlog
}

// WithRequestID returns a logger with request_id field
func WithRequestID(requestID string) zerolog.Logger {
	return zlog.With().Str("request_id", requestID).Logger()
}

// WithUserID returns a logger with user_id field
func WithUserID(userID string) zerolog.Logger {
	return zlog.With().Str("user_id", userID).Logger()
}

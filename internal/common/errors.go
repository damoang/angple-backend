package common

import "errors"

// Business logic errors
var (
	// General errors
	ErrNotFound  = errors.New("resource not found")
	ErrForbidden = errors.New("forbidden")

	// Post errors
	ErrPostNotFound  = errors.New("post not found")
	ErrBoardNotFound = errors.New("board not found")

	// Comment errors
	ErrCommentNotFound = errors.New("comment not found")

	// Auth errors
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user already exists")

	// Validation errors
	ErrInvalidInput = errors.New("invalid input")
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("expired token")
)

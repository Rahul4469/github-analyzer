package models

import (
	"errors"
	"fmt"
)

// User related errors
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrPasswordTooShort   = errors.New("password must be at least 8 characters")
)

// Session related errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

// Repository related errors
var (
	ErrRepositoryNotFound      = errors.New("repository not found")
	ErrInvalidRepositoryURL    = errors.New("invalid GitHub repository URL")
	ErrRepositoryAlreadyExists = errors.New("repository already exists for this user")
)

// Analysis related errors
var (
	ErrAnalysisNotFound = errors.New("analysis not found")
)

type FileError struct {
	Issue string
}

func (fe FileError) Error() string {
	return fmt.Sprintf("invalid file: %v", fe.Issue)
}

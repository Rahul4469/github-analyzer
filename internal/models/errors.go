package models

import (
	"errors"
	"fmt"
)

var (
	ErrEmailTaken      = errors.New("models: email address is already in use")
	ErrAccountNotFound = errors.New("models: no rows in result set")
	ErrNotFound        = errors.New("models: resources could not be found")
)

type FileError struct {
	Issue string
}

func (fe FileError) Error() string {
	return fmt.Sprintf("invalid file: %v", fe.Issue)
}

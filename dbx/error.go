package dbx

import "github.com/go-sdk/core/errx"

var (
	ErrDriverRequired      = errx.New("database driver is required")
	ErrDSNRequired         = errx.New("database dsn is required")
	ErrDriverNotRegistered = errx.New("database driver is not registered")
)

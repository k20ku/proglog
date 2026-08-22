package server

import (
	"log/slog"

	api "github.com/k20ku/proglog/gen/go/log/v1"
)

const (
	objectLogs    = "logs"
	actionProduce = "produce"
	actionConsume = "consume"
)

type Config struct {
	Logger     *slog.Logger
	CommitLog  CommitLog
	Authorizer Authorizer
}

type CommitLog interface {
	// returns record offset
	Append(*api.Record) (offset uint64, err error)
	// read offset
	// Read returns ErrOffsetOutOfRange error if corresponding record not found,
	// else returns non-nil error if commit log has an internal error.
	Read(off uint64) (*api.Record, error)
}

type Authorizer interface {
	// authorizes subject to run the action to object.
	// if permission denied, returns ErrPermissionDenied
	Authorize(subject, action, object string) error
}

package server

import (
	"errors"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/log"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ CommitLog = (*walCommitLog)(nil)

// CommitLog implementation by WAL log
type walCommitLog struct {
	l *log.Log
}

func NewWalCommitLog(l *log.Log) CommitLog {
	return &walCommitLog{l: l}
}

func (wl *walCommitLog) Append(record *api.Record) (uint64, error) {
	offset, err := wl.l.Append(record)
	if err != nil {
		return offset, status.Error(codes.Internal, "producing record failed")
	}
	return offset, nil
}

func (wl *walCommitLog) Read(offset uint64) (*api.Record, error) {
	record, err := wl.l.Read(offset)
	if err != nil {
		if erroor, ok := errors.AsType[log.ErrOffsetOutOfRange](err); ok {
			return nil, ErrOffsetOutOfRange{Offset: erroor.Offset}
		}
		return nil, status.Error(codes.Internal, "Internal Server Error")
	}
	return record, nil
}

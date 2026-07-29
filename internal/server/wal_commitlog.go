package server

import (
	"errors"
	"fmt"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/k20ku/proglog/internal/log"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
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
		return offset, err
	}
	return offset, nil
}

func (wl *walCommitLog) Read(offset uint64) (*api.Record, error) {
	record, err := wl.l.Read(offset)
	if err != nil {
		if err, ok := errors.AsType[log.ErrOffsetOutOfRange](err); ok {
			return nil, wrap(err)
		}
		return nil, err
	}
	return record, nil
}

func wrap(e log.ErrOffsetOutOfRange) ErrOffsetOutOfRange {
	return &errOffsetOutOfRange{Err: &e}
}

var _ ErrOffsetOutOfRange = (*errOffsetOutOfRange)(nil)

type errOffsetOutOfRange struct {
	Err *log.ErrOffsetOutOfRange
}

func (e errOffsetOutOfRange) Error() string {
	return fmt.Sprintf("offset %d is out of range", e.Err.Offset)
}

func (e *errOffsetOutOfRange) Offset() uint64 {
	return e.Err.Offset
}
func (e *errOffsetOutOfRange) GRPCStatus() *status.Status {
	st := status.New(
		codes.OutOfRange,
		e.Error(),
	)
	msg := fmt.Sprintf(
		"The requested offset is outside the log's range: %d",
		e.Offset(),
	)
	d := &errdetails.LocalizedMessage{
		Locale:  "en-US",
		Message: msg,
	}
	std, err := st.WithDetails(d)
	if err != nil {
		return st
	}
	return std
}

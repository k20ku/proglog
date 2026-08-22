package server

import (
	"fmt"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ErrOffsetOutOfRange struct {
	Offset uint64
}

func (e ErrOffsetOutOfRange) Error() string {
	return fmt.Sprintf("offset %d is out of range", e.Offset)
}

func (e ErrOffsetOutOfRange) GRPCStatus() *status.Status {
	st := status.New(
		codes.OutOfRange,
		e.Error(),
	)
	msg := fmt.Sprintf(
		"The requested offset is outside the log's range: %d",
		e.Offset,
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

type ErrPermissionDenied struct {
	Subject, Action, Object string
}

func (e ErrPermissionDenied) Error() string {
	return fmt.Sprintf("%q not permitted to run %q to %q",
		e.Subject, e.Action, e.Object,
	)
}

func (e ErrPermissionDenied) GRPCStatus() *status.Status {
	st := status.New(
		codes.PermissionDenied,
		e.Error(),
	)
	msg := fmt.Sprintf(
		"The request that %q run action %q is not permitted to %q",
		e.Subject, e.Action, e.Object,
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

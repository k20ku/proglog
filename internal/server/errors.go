package server

import (
	"google.golang.org/grpc/status"
)

type ErrOffsetOutOfRange interface {
	Offset() uint64
	Error() string
	GRPCStatus() *status.Status
}

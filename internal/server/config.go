package server

import (
	api "github.com/k20ku/proglog/gen/go/log/v1"
)

type Config struct {
	CommitLog CommitLog
}

type CommitLog interface {
	// returns record offset
	Append(*api.Record) (offset uint64, err error)
	// read offset
	Read(off uint64) (*api.Record, error)
}

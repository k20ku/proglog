# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

Proglog is a distributed log service being built by following Travis Jeffery's
*Distributed Services with Go*, with dependencies and tooling modernized for the
current Go ecosystem (see `README.md` for the rationale behind each swap: Chi
instead of Gorilla Mux, Step CLI instead of CFSSL, Buf instead of raw `protoc`).
The book is followed chapter by chapter, so the code is intentionally
incremental. The README is written in Japanese; the working language of the
project is a mix of Japanese and English, including in comments.

## Commands

```bash
make test            # go test --race ./...  (always run with the race detector)
make gen             # buf generate — regenerate protobuf code into gen/go/
make clean           # rm -rf gen/* tools/*
go run main.go 8080  # run the HTTP server on the given port (port arg is required)

# Run a single package / test
go test --race ./internal/log/
go test --race ./internal/log/ -run TestNewLog
```

Proto changes: edit `proto/log/v1/log.proto`, then `make gen`. Generated code
lands in `gen/go/log/v1/` and is imported as `api "github.com/k20ku/proglog/gen/go/log/v1"`.

## Architecture

There are **two independent log implementations** in this repo; do not confuse them.

### `internal/server/` — Chapter 1 HTTP service (currently the one that runs)

`main.go` wires an in-memory log to an HTTP API. `server.Log` is a plain
mutex-guarded slice of `Record` — it has **no relationship** to the persistent
commit log in `internal/log/`. The Chi router exposes `POST /` (produce) and
`GET /` (consume) with JSON bodies. Records carry base64-encoded `value` bytes.

### `internal/log/` — the persistent commit log (the real subject of the book)

This is a layered append-only log, built bottom-up. Each layer wraps the one below:

- **store** (`store.go`) — append-only file of length-prefixed records
  (`lenWidth` = 8-byte big-endian length, then the bytes). Writes go through a
  `bufio.Writer`; reads flush the buffer first. `size` tracks the append cursor.
- **index** (`index.go`) — an `mmap`'d file of fixed-width entries
  (`entWidth` = 4-byte relative offset + 8-byte store position). Offsets are
  stored **relative to the segment's base offset** as `uint32` to save space.
  On open the file is `os.Truncate`'d up to `MaxIndexBytes` (mmap needs the space);
  on `Close` it is truncated back down to the real `size` — this back-truncation
  is required for the service to restart correctly. `Read(-1)` returns the last
  entry. Unix-only (`//go:build unix`).
- **segment** (`segment.go`) — couples one store + one index sharing a
  `baseOffset`. `nextOffset` is the offset the next appended record will get.
  `Append` writes the store first, then the index (a crash between the two leaves
  an orphan store record — see the WARNING comment). `IsMaxed` reports when either
  the store or index is full; a full index surfaces as `errSegmentMaxed`.
  `RebuildIndex` reconstructs the index by scanning the store — used when a
  segment is found with a populated store but an empty index.
- **log** (`log.go`) — orders segments by base offset and tracks the
  `activeSegment`. **The `.store` filename is the source of truth**: `setup`
  discovers segments by listing `.store` files and parsing base offsets from
  their names (`.index` files are ignored during discovery). `Append` writes to
  the active segment and `roll`s to a new segment on `errSegmentMaxed`. `Read`
  uses `binarySearch` (`sort.Search`) to locate the owning segment. `Reader()`
  returns an `io.MultiReader` over all stores for snapshotting the whole log.

`config.go` holds `Config.Segment.{MaxStoreBytes, MaxIndexBytes, InitialOffset}`.
Defaults (1024/1024) are applied in `NewLog` when zero.

**Note:** the two implementations are not yet connected — the HTTP server still
uses the in-memory `server.Log`, not `internal/log.Log`.

## Non-source directories

- `gen/` — generated protobuf code; do not edit by hand, regenerate with `make gen`.
- `sample/`, `tmp/` — scratch / experimental code and throwaway data (e.g.
  `tmp/log/` contains manual index-rebuild experiments). Not part of the build.
- `internal/log/utils.go` — test helpers (`tempFile`, `openFile`, `copyToDir`);
  it imports `testing` and is only used by the `_test.go` files.

## Conventions

- Errors are wrapped with `fmt.Errorf("...: %v"|%w, err)`; sentinel errors are
  package-level `var`s (`ErrOffsetOutOfRange`, `errSegmentMaxed`, `errIndexFulled`,
  etc.) — match them with `errors.Is`.
- All tests run under `-race`; the log types are designed for concurrent use
  (`sync.RWMutex` on `Log`, `sync.Mutex` on `store`).

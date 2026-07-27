package log

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

var (
	enc = binary.BigEndian // network endian

	// errors
	errSyncStoreFailed = errors.New("sync store failed")
	errStoreBroken     = errors.New("store file is broken")
)

const (
	lenWidth = 8 // 64 bit length
)

type RelativePosition uint64

type store struct {
	*os.File
	mu   sync.Mutex
	buf  *bufio.Writer
	size uint64
}

// simple wrapper API around a file,
// which has two major APIs: read and append.
func newStore(f *os.File) (*store, error) {
	fi, err := os.Stat(f.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to init store: cannot get file size: %+v", err)
	}

	size := uint64(fi.Size())
	return &store{
		File: f,
		size: size,
		buf:  bufio.NewWriter(f),
	}, nil
}

// Writes to the buffer instead of writing directly to the file
// so as to reducing syscalls.
// It returns number of bytes written and the beginning of the appended position.
func (s *store) Append(p []byte) (n uint64, pos uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pos = s.size
	if err := binary.Write(s.buf, enc, uint64(len(p))); err != nil {
		return 0, 0, fmt.Errorf("failed to write number of bytes: %+v", err)
	}
	w, err := s.buf.Write(p)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to write bytes: %+v", err)
	}
	w += lenWidth
	s.size += uint64(w)
	return uint64(w), pos, nil
}

// returns the record stored at the given position
// At end of store, error is io.EOF
func (s *store) Read(pos uint64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return nil, fmt.Errorf("flush buffer: %w", err)
	}
	size := make([]byte, lenWidth)
	if _, err := s.File.ReadAt(size, int64(pos)); err != nil {
		return nil, err
	}
	b := make([]byte, enc.Uint64(size))
	if _, err := s.File.ReadAt(b, int64(pos+lenWidth)); err != nil {
		return nil, err
	}
	return b, nil
}

// ReadAt reads len(p) bytes from the File starting at byte offset off.
// It returns the number of bytes read and the error, if any.
// ReadAt always returns a non-nil error when n < len(b). At end of file, that error is io.EOF.
func (s *store) ReadAt(p []byte, off int64) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return 0, errSyncStoreFailed
	}
	return s.File.ReadAt(p, off)
}

// persists any buffered data before closing file.
func (s *store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.sync(); err != nil {
		return err
	}
	return s.File.Close()
}

// Sync this store to stable storage
// if sync is dailed, returns errSyncStoreFailed
func (s *store) Sync() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sync()
}

func (s *store) sync() error {
	if err := s.buf.Flush(); err != nil {
		return errSyncStoreFailed
	}
	if err := s.File.Sync(); err != nil {
		return errSyncStoreFailed
	}
	return nil
}

// error is
//   - errStoreBroken
//   - other I/O error
func (s *store) LastPositionAbove(pos uint64) (lastPos uint64, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.buf.Flush(); err != nil {
		return 0, fmt.Errorf("failed to flush buffer: %+v", err)
	}
	size_buf := make([]byte, lenWidth)
	for {
		n, err := s.File.ReadAt(size_buf, int64(pos))
		if errors.Is(err, io.EOF) {
			if n == 0 {
				break
			}
			return pos, errStoreBroken
		} else if err != nil {
			return 0, fmt.Errorf(
				"store failed to get last position above %d pos: %v", pos, err)
		}
		size := enc.Uint64(size_buf)
		b := make([]byte, size)
		if _, err = s.File.ReadAt(b, int64(pos+lenWidth)); err != nil {
			return pos, errStoreBroken
		}
		lastPos = pos
		pos += lenWidth + size
	}
	return lastPos, nil
}

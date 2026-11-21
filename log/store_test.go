package log

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var (
	write = []byte("hello world")
	width = uint64(len(write)) + lenWidth
)

func TestStoreAppendRead(t *testing.T) {
	f, err := tempFile(t, "store_append_read_test*.store")
	require.NoError(t, err)

	s, err := newStore(f)
	require.NoError(t, err)

	testAppend(t, s)
	testRead(t, s)
	testReadAt(t, s)

	// validate if writtens are persisted
	s, err = newStore(f)
	require.NoError(t, err)
	testRead(t, s)
}

func testAppend(t *testing.T, s *store) {
	t.Helper()
	for i := uint64(1); i < 4; i++ {
		n, pos, err := s.Append(write)
		require.NoErrorf(t, err, "failed to append %+v", write)
		require.Equal(t, pos+n, width*i)
	}
}

func testRead(t *testing.T, s *store) {
	t.Helper()
	var pos uint64
	for i := uint64(1); i < 4; i++ {
		read, err := s.Read(pos)
		require.NoError(t, err, "failed to read from position")
		require.Equal(t, write, read)
		pos += width
	}
}

func testReadAt(t *testing.T, s *store) {
	t.Helper()
	for i, off := uint64(1), int64(0); i < 4; i++ {
		// reading offset
		b := make([]byte, lenWidth)
		n, err := s.ReadAt(b, off)
		require.NoErrorf(t, err, "failed to readAt(%+v, %d)", b, off)
		require.Equal(t, lenWidth, n, "invalid size of offset number")
		off += int64(n)

		// reading data
		size := enc.Uint64(b)
		b = make([]byte, size)
		n, err = s.ReadAt(b, off)
		require.NoErrorf(t, err, "failed to readAt(%+v, %d)", b, off)
		require.Equal(t, int(size), n, "read size is not expected")
		require.Equal(t, write, b)
		off += int64(n)
	}
}

func TestStoreClose(t *testing.T) {
	f, err := tempFile(t, "store_close_test*.store")
	require.NoError(t, err)

	s, err := newStore(f)
	require.NoError(t, err)

	_, _, err = s.Append(write)
	require.NoError(t, err)

	f, beforeSize, err := openFile(f.Name())
	require.NoError(t, err)

	// close the store
	err = s.Close()
	require.NoError(t, err)

	_, afterSize, err := openFile(f.Name())
	require.NoError(t, err)
	require.True(t, afterSize > beforeSize)
}

package log

import (
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndex(t *testing.T) {
	f, err := tempFile(t, "index_test*.index")
	require.NoError(t, err, "failed to temp dir")

	c := Config{}
	c.Segment.MaxIndexBytes = 1024
	idx, err := newIndex(f, c)
	require.NoError(t, err, "failed to newIndex")

	_, _, err = idx.Read(-1)
	require.Error(t, err, "should return EOF")

	require.Equal(t, f.Name(), idx.Name())

	entries := []struct {
		Off uint32
		Pos uint64
	}{
		{Off: 0, Pos: 0},
		{Off: 1, Pos: 15},
		{Off: 2, Pos: 31},
		{Off: 3, Pos: 43},
	}

	for _, expected := range entries {
		err := idx.Write(expected.Off, expected.Pos)
		require.NoErrorf(t, err, "failed to write the entry (%+v)", expected)

		_, pos, err := idx.Read(int64(expected.Off))
		require.NoErrorf(t, err, "failed to Read entry at offset %d", expected.Off)

		require.Equal(t, expected.Pos, pos)
	}

	// index and scanner should error when reading past existing entries
	_, _, err = idx.Read(int64(len(entries)))
	require.Error(t, io.EOF, "should return EOF error")
	err = idx.Close()
	require.NoError(t, err, "failed to close index")

	// index should build its state from the existing file
	f, _ = os.OpenFile(f.Name(), os.O_RDWR, 0600)
	idx, err = newIndex(f, c)
	require.NoError(t, err, "failed to re-newIndex")

	off, pos, err := idx.Read(-1)
	require.NoError(t, err, "re-read failed")
	require.Equal(t, off, entries[len(entries)-1].Off)
	require.Equal(t, pos, entries[len(entries)-1].Pos)
}

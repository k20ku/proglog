package log

import (
	"fmt"
	"os"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/stretchr/testify/require"
)

func TestSegment(t *testing.T) {
	dir := t.TempDir()

	want := &api.Record{Value: []byte("Hello World")}

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = entWidth * 3

	baseOffset := uint64(16)
	s, err := newSegment(dir, baseOffset, c)
	require.NoErrorf(t, err,
		"failed newSegment(%s, %d, %#v)", dir, baseOffset, c,
	)
	require.Equal(t, baseOffset, s.nextOffset)
	require.False(t, s.IsMaxed())

	for i := uint64(0); i < 3; i++ {
		off, err := s.Append(want)
		require.NoErrorf(t, err, "failed s.Append(%v) at %d", want, i)
		require.Equal(t, baseOffset+i, off)

		got, err := s.Read(off)
		require.NoErrorf(t, err, "s.Read(%d) failed at %d", off, i)
		require.Equal(t, want.Value, got.Value)
	}

	_, err = s.Append(want)
	require.ErrorIs(t, err, errSegmentMaxed)

	// maxed index
	require.True(t, s.IsMaxed())

	c.Segment.MaxStoreBytes = uint64(len(want.Value) * 3)
	c.Segment.MaxIndexBytes = 1024

	s, err = newSegment(dir, baseOffset, c)
	require.NoErrorf(t, err, "failed to second new session: %v", err)
	// maxed store
	// TODO: 二回目のnewSegmentがnilになる
	require.NotNil(t, s)
	require.True(t, s.IsMaxed())

	err = s.Remove()
	require.NoError(t, err, "second session remove failed")
	s, err = newSegment(dir, baseOffset, c)
	require.NoError(t, err)
	require.False(t, s.IsMaxed())
}

func TestRebuildIndex(t *testing.T) {
	dir := t.TempDir()
	record := &api.Record{Value: []byte("Hello Proglog!")}
	c := Config{}
	c.Segment.MaxIndexBytes = 5 * entWidth
	c.Segment.MaxStoreBytes = 1024
	baseOffset := uint64(0)
	s, err := newSegment(dir, baseOffset, c)
	require.NoError(t, err)
	for i := uint64(0); i < 3; i++ {
		off, err := s.Append(record)
		require.NoErrorf(t, err, "failed s.Append(%v) at %d", record, i)
		require.Equal(t, baseOffset+i, off)
	}
	err = s.Close()
	require.NoError(t, err)
	err = os.Remove(s.index.Name())
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		fmt.Println(entry.Name())
	}

	s, err = newSegment(dir, baseOffset, c)
	require.NoError(t, err)
	for i := uint64(0); i < 3; i++ {
		read, err := s.Read(i)
		require.NoError(t, err)
		require.Equal(t, uint64(i), read.Offset)
	}
}

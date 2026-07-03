package log

import (
	"fmt"
	"sync"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/stretchr/testify/require"
)

func TestNewLog(t *testing.T) {
	dir := t.TempDir()

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = 1024

	l, err := NewLog(dir, c)
	require.NoError(t, err)

	records := []string{"Hello World", "hello world", "hello world!"}
	for _, record := range records {
		record := &api.Record{Value: []byte(record)}
		_, err := l.Append(record)
		require.NoError(t, err)
	}

	var wg sync.WaitGroup
	for i := range 1000 {
		wg.Go(func() {
			for j := range 10 {
				record := &api.Record{Value: []byte(fmt.Sprintf("%s-%d-%d", "Hello", i, j))}
				_, err := l.Append(record)
				require.NoErrorf(t, err, "writing %+v", record)
			}
		})
	}
	wg.Wait()
}

func TestNewLog1(t *testing.T) {
	dir := t.TempDir()

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = 36

	l, err := NewLog(dir, c)
	require.NoError(t, err)

	for i := range 400 {
		value := fmt.Sprintf("%s%d", "Hello World", i)
		record := &api.Record{Value: []byte(value)}
		_, err := l.Append(record)
		require.NoErrorf(t, err, "writing at %d time", i)
	}
	for i := uint64(0); i < 400; i++ {
		record, err := l.Read(i)
		require.NoError(t, err)
		require.Equal(t, string(record.Value), fmt.Sprintf("%s%d", "Hello World", i))
	}
}

func TestNewLog2(t *testing.T) {
	dir := t.TempDir()

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = 4096

	l, err := NewLog(dir, c)
	require.NoError(t, err)

	for i := range 400 {
		value := fmt.Sprintf("%s%d", "Hello World", i)
		record := &api.Record{Value: []byte(value)}
		_, err := l.Append(record)
		require.NoErrorf(t, err, "writing at %d time", i)
	}
	for i := uint64(0); i < 400; i++ {
		record, err := l.Read(i)
		require.NoError(t, err)
		require.Equal(t, string(record.Value), fmt.Sprintf("%s%d", "Hello World", i))
	}
}

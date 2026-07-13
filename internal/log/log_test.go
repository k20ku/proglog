package log

import (
	"fmt"
	"io"
	"testing"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
)

func TestLogBasic(t *testing.T) {
	for senario, fn := range map[string]func(
		t *testing.T, lg *Log,
	){
		"append and read a record succeeds": testAppendRead,
		"offset out of range error":         testOutRangeErr,
		"init with existing segments":       testInitExisting,
		"reader":                            testReader,
		"truncate":                          testTruncate,
	} {
		t.Run(senario, func(t *testing.T) {
			dir := t.TempDir()

			c := Config{}
			c.Segment.MaxStoreBytes = 32
			lg, err := NewLog(dir, c)
			require.NoError(t, err)

			fn(t, lg)
		})
	}
}

func testAppendRead(t *testing.T, lg *Log) {
	append := &api.Record{
		Value: []byte("Hello Proglog!"),
	}
	off, err := lg.Append(append)
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)

	read, err := lg.Read(off)
	require.NoError(t, err)
	require.Equal(t, append.Value, read.Value)
}

func testOutRangeErr(t *testing.T, lg *Log) {
	read, err := lg.Read(1)
	require.Nil(t, read)
	require.Error(t, err)
}

func testInitExisting(t *testing.T, lg *Log) {
	append := &api.Record{
		Value: []byte("Hello Proglog!"),
	}
	for range 3 {
		_, err := lg.Append(append)
		require.NoError(t, err)
	}
	require.NoError(t, lg.Close())

	off, err := lg.LowestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)
	off, err = lg.HighestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(2), off)

	newLg, err := NewLog(lg.Dir, lg.Config)
	require.NoError(t, err)

	off, err = newLg.LowestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)
	off, err = newLg.HighestOffset()
	require.NoError(t, err)
	require.Equal(t, uint64(2), off)
}

func testReader(t *testing.T, log *Log) {
	append := &api.Record{
		Value: []byte("Hello Proglog!"),
	}
	off, err := log.Append(append)
	require.NoError(t, err)
	require.Equal(t, uint64(0), off)

	reader := log.Reader()
	b, err := io.ReadAll(reader)
	require.NoError(t, err)

	read := &api.Record{}
	err = proto.Unmarshal(b[lenWidth:], read)
	require.NoError(t, err)
	require.Equal(t, append.Value, read.Value)
}

func testTruncate(t *testing.T, log *Log) {
	append := &api.Record{
		Value: []byte("Hello Proglog!"),
	}

	for range 3 {
		_, err := log.Append(append)
		require.NoError(t, err)
	}

	err := log.Truncate(1)
	require.NoError(t, err)

	_, err = log.Read(0)
	require.Error(t, err)
}

func TestLogConcurrecy(t *testing.T) {
	dir := t.TempDir()

	c := Config{}
	c.Segment.MaxStoreBytes = 1024
	c.Segment.MaxIndexBytes = 1024

	l, err := NewLog(dir, c)
	require.NoError(t, err)

	var eg errgroup.Group
	for i := range 5 {
		eg.Go(func() error {
			for t := range 10 {
				record := &api.Record{Value: []byte("Hello Proglog!")}
				_, err := l.Append(record)
				if err != nil {
					return fmt.Errorf("writing at routine %d of %d time: %v", i, t, err)
				}
			}
			return nil
		})
	}
	require.NoError(t, eg.Wait())
}

func TestLogConfig(t *testing.T) {

	tests := map[string]struct {
		MaxStoreBytes uint64
		MaxIndexBytes uint64
	}{
		"MaxIndexBytes is less than MaxStoreBytes": {
			1024,
			3 * entWidth,
		},
		"MaxIndexBytes is greater than MaxStoreBytes": {
			1024,
			4 * 1024,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()

			c := Config{}
			c.Segment.MaxStoreBytes = tt.MaxStoreBytes
			c.Segment.MaxIndexBytes = tt.MaxIndexBytes

			l, err := NewLog(dir, c)
			require.NoError(t, err)

			msg := "Hello Proglog!"
			for i := uint64(0); i < 400; i++ {
				value := fmt.Sprintf("%s%d", msg, i)
				record := &api.Record{Value: []byte(value)}
				_, err := l.Append(record)
				require.NoErrorf(t, err, "writing (%d)", i)
			}
			for i := uint64(0); i < 400; i++ {
				record, err := l.Read(i)
				require.NoErrorf(t, err, "reading (%d): ", i)
				require.Equal(t, string(record.Value), fmt.Sprintf("%s%d", msg, i))
			}
		})
	}
}

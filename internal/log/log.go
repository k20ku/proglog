package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"

	api "github.com/k20ku/proglog/gen/go/log/v1"
)

type Log struct {
	mu sync.RWMutex // Readers > Writers

	Dir    string
	Config Config

	activeSegment *segment // the active segment to append writes to
	segments      []*segment
}

// Returns the new log with the given config.
// If config is not specified, it sets up config with default value (Segment.MaxStoreBytes and MaxStoreBytes to 1024, 1024 respectively).
//   - It sets up for the segments that already on the disk.
//   - Or bootstraps the initial segment if there is no stored segment on the disk.
func NewLog(dir string, c Config) (*Log, error) {
	// mut config is done safely thanks to the call-by-value
	if c.Segment.MaxStoreBytes == 0 {
		c.Segment.MaxStoreBytes = 1024
	}
	if c.Segment.MaxIndexBytes == 0 {
		c.Segment.MaxIndexBytes = 1024
	}
	l := &Log{
		Dir:    dir,
		Config: c,
	}

	return l, l.setup()
}

func (l *Log) setup() error {
	files, err := os.ReadDir(l.Dir)
	if err != nil {
		return fmt.Errorf("setup log failed reading the dir (%s): %v", l.Dir, err)
	}
	// fetch baseOffsets from fileNames
	var baseOffsets []uint64
	for _, file := range files {
		offStr := strings.TrimSuffix(
			file.Name(),
			path.Ext(file.Name()),
		)
		off, _ := strconv.ParseUint(offStr, 10, 0)
		baseOffsets = append(baseOffsets, off)
	}
	// arrange baseOffsets in decreasing order
	sort.Slice(baseOffsets, func(i, j int) bool {
		return baseOffsets[i] > baseOffsets[j]
	})
	// create baseOffsets
	for i := 0; i < len(baseOffsets); i++ {
		if err = l.newSegment(baseOffsets[i]); err != nil {
			return err
		}
		// baseOffset contains dup for index and store so we skip dup
		i++
	}
	// no segment on the disk
	if l.segments == nil {
		if err = l.newSegment(
			l.Config.Segment.InitialOffset,
		); err != nil {
			return err
		}
	}
	return nil
}

func (l *Log) newSegment(off uint64) error {
	s, err := newSegment(l.Dir, off, l.Config)
	if err != nil {
		return fmt.Errorf("log failed to new segment: %v", err)
	}
	l.segments = append(l.segments, s)
	l.activeSegment = s
	return nil
}

// Append is safe with concurrent accesses.
// It returns EOF error with 0 offset if there is no disk space to append record.
// 前回追加したときにEOFになってたのならばその時に新たなセグメントを作ってるはずだが，
// segment関係の永続化に不具合があった場合(セグメント初期化から書き込みの間にindexファイルが消されたなど)があればその限りではない
func (l *Log) Append(record *api.Record) (off uint64, err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	off, err = l.activeSegment.Append(record)
	if err != nil {
		if errors.Is(err, io.EOF) {
			// rollback
			return l.roll(record)
		} else {
			// fatal
			return 0, fmt.Errorf("log failed to append: %v", err)
		}
	}
	if l.activeSegment.IsMaxed() {
		err = l.newSegment(off + 1)
	}
	return off, err
}

func (l *Log) roll(record *api.Record) (uint64, error) {
	if err := l.newSegment(l.activeSegment.nextOffset); err != nil {
		return 0, fmt.Errorf("rolling for append failed: %v", err)
	}
	// append to new segment
	off, err := l.activeSegment.Append(record)
	if err != nil {
		// if there is an EOF error, roll dismisses it
		return 0, fmt.Errorf("append to logSegment failed in rolling: %v", err)
	}
	return off, err
}

func (l *Log) Read(off uint64) (*api.Record, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// search for the segment that has offset off
	var s *segment
	for _, segment := range l.segments {
		if segment.baseOffset <= off && off < segment.nextOffset {
			s = segment
			break
		}
	}
	if s == nil || s.nextOffset <= off {
		return nil, fmt.Errorf("offset out of range: %d", off)
	}
	record, err := s.Read(off)
	if err != nil {
		return nil, fmt.Errorf("Log failed to read offset (%d) failed: %v", off, err)
	}
	return record, nil
}

// Close this Log, iterating over the segment and close them.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, segment := range l.segments {
		if err := segment.Close(); err != nil {
			return fmt.Errorf(
				"Closing log (%v) failed to close the segments at base offset %d: %v",
				l, segment.baseOffset, err,
			)
		}
	}
	return nil
}

// Removes this log and removes this data all
func (l *Log) Remove() error {
	if err := l.Close(); err != nil {
		return err
	}
	entries, err := os.ReadDir(l.Dir)
	if err != nil {
		return fmt.Errorf("Removing log failed: %v", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(entry.Name()); err != nil {
			return fmt.Errorf(
				"Removing log (%v) failed to remove data in %s: %v",
				l, l.Dir, err,
			)
		}
	}
	return nil
}

// Deprecated: Reset is intented only for tests. Prefer creating a new log.
// Reset removes the log and then create a new log to replace it
func (l *Log) Reset() error {
	if err := l.Remove(); err != nil {
		return err
	}
	return l.setup()
}

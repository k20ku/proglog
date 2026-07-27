package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
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

	activeSegment *segment   // the active segment to append writes to
	segments      []*segment // all segments orderd in ascending order based upon their baseOffsets.
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

	if err := l.setup(); err != nil {
		return nil, fmt.Errorf("failed to setup log: %v", err)
	}

	return l, nil
}

func (l *Log) setup() error {
	baseOffsets, err := loadBaseOffsets(l.Dir)
	if err != nil {
		return err
	}

	if err := l.buildSegments(baseOffsets); err != nil {
		return err
	}
	// no segment on the disk
	// bootstrapping
	if l.segments == nil {
		baseOffset := l.Config.Segment.InitialOffset
		if err := l.appendSegment(baseOffset); err != nil {
			return err
		}
	}
	return nil
}

// Returns all existing segment base offsets in ascending order.
func loadBaseOffsets(dir string) ([]uint64, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf(
			"failed to read dir(%s) to load the base offsets: %v",
			dir, err,
		)
	}
	// fetch baseOffsets from fileNames
	var baseOffsets []uint64
	for _, file := range files {
		fileExt := path.Ext(file.Name())
		// skip other than .store ext
		// this is because store is the source of truth
		if fileExt != ".store" {
			continue
		}
		offStr := strings.TrimSuffix(
			file.Name(),
			fileExt,
		)
		off, err := strconv.ParseUint(offStr, 10, 0)
		if err != nil {
			fmt.Printf("file name %s is invalid, we skip this", file)
			continue
		}
		baseOffsets = append(baseOffsets, off)
	}

	// ordering baseOffsets in ascending order
	slices.Sort(baseOffsets)

	return baseOffsets, nil
}

func (l *Log) buildSegments(baseOffsets []uint64) error {
	// log appends the new segment for each baseOffset
	for i, baseOffset := range baseOffsets {
		if i < len(baseOffsets)-1 {
			s, err := newSegment(l.Dir, baseOffset, l.Config)
			if err != nil {
				return fmt.Errorf("log building intermediate segment: %v", err)
			}
			l.segments = append(l.segments, s)
		} else {
			if err := l.appendSegment(baseOffset); err != nil {
				return fmt.Errorf(
					"log building last segments: %v", err,
				)
			}
		}

	}

	return nil
}

// this is not opened method
// The caller MUST hold ths lock of this Log
func (l *Log) appendSegment(baseOffset uint64) error {
	s, err := newSegmentChecked(l.Dir, baseOffset, l.Config)
	if err != nil {
		return fmt.Errorf(
			"log failed to new segment (baseOffset: %d): %v",
			baseOffset, err)
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
		if errors.Is(err, errSegmentMaxed) {
			// rollback
			return l.roll(record)
		} else {
			// fatal
			return 0, fmt.Errorf("log failed to append: %v", err)
		}
	}
	if l.activeSegment.IsMaxed() {
		if err := l.activeSegment.Sync(); err != nil {
			return 0, err
		}
		err = l.appendSegment(off + 1)
	}
	return off, err
}

func (l *Log) roll(record *api.Record) (uint64, error) {
	if err := l.appendSegment(l.activeSegment.nextOffset); err != nil {
		return 0, fmt.Errorf("rolling for append failed: %v", err)
	}
	// append to new segment
	off, err := l.activeSegment.Append(record)
	if err != nil {
		// Even if segment returns errSegmentMaxed, we dismisses it
		return 0, fmt.Errorf("append to logSegment failed in rolling: %v", err)
	}
	return off, err
}

// Read Record at off offfset.
// If offset is not exist, returns ErrOffsetOutOfRange
func (l *Log) Read(off uint64) (*api.Record, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	s, found := l.binarySearch(off)
	if !found || s == nil {
		return nil, ErrOffsetOutOfRange{Offset: off}
	}

	record, err := s.Read(off)
	if err != nil {
		return nil, fmt.Errorf("Log failed to read offset (%d) failed: %v", off, err)
	}
	return record, nil
}

func (l *Log) binarySearch(off uint64) (s *segment, found bool) {
	if l.activeSegment.baseOffset <= off && off < l.activeSegment.nextOffset {
		s = l.activeSegment
	}
	// CAUTION: NOT (>=) !
	// 0   10  20  30  40 (45)
	// |___|___|___|____|__:
	// 0   1   2   3    4
	// len(l.segments) == 5
	// |   0   :|   10    :|    |   :
	// | 0 ~ 9 :| 10 ~ 11 :|    |   :
	// 0   +--> 1   +--->  2    |   :

	// n in [0, len(l.segments))
	n := sort.Search(len(l.segments), func(i int) bool {
		return off < l.segments[i].baseOffset // CAUTION: not <=
	})
	if !(1 <= n) {
		return nil, false
	}
	s = l.segments[n-1]

	return s, true
}

// Close this Log, iterating over the segment and close them.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, s := range l.segments {
		if err := s.Close(); err != nil {
			return fmt.Errorf(
				"log closing losing segment at base offset %d: %v",
				s.baseOffset, err)
		}
	}
	return nil
}

// sync all segments
func (l *Log) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.segments {
		if err := s.Sync(); err != nil {
			return fmt.Errorf("log sync: %v", err)
		}
	}
	return nil
}

// Remove all segments whose highest offset is lower than lowest.
func (l *Log) Truncate(lowest uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if lowest >= l.activeSegment.baseOffset {
		return ErrOffsetOutOfRange{Offset: lowest}
	}
	var segments []*segment
	for _, s := range l.segments {
		if s.nextOffset <= lowest+1 {
			baseOffset := s.baseOffset
			if err := s.Remove(); err != nil {
				return fmt.Errorf(
					"Truncating Log failed at baseOffset %d: %v", baseOffset, err)
			}
		} else {
			segments = append(segments, s)
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
		return fmt.Errorf("remove log: %v", err)
	}
	for _, entry := range entries {
		if err := os.RemoveAll(entry.Name()); err != nil {
			return fmt.Errorf(
				"remove log data at %s: %v",
				l.Dir, err,
			)
		}
	}
	return nil
}

// Reader returns io.Reader to snapshot the wholelog for restoreing the whole log
func (l *Log) Reader() io.Reader {
	l.mu.RLock()
	defer l.mu.RUnlock()
	readers := make([]io.Reader, len(l.segments))
	for i, segment := range l.segments {
		readers[i] = &originStoreReader{segment.store, 0}
	}
	return io.MultiReader(readers...)
}

type originStoreReader struct {
	*store
	off int64
}

func (o *originStoreReader) Read(p []byte) (int, error) {
	n, err := o.ReadAt(p, o.off)
	o.off += int64(n)
	return n, err
}

// Deprecated: Reset is intented only for tests. Prefer creating a new log.
// Reset removes the log and then create a new log to replace it
func (l *Log) Reset() error {
	if err := l.Remove(); err != nil {
		return err
	}
	return l.setup()
}

// we can know the offset range stored in the log.
//
// We'll need this information to know
//   - what node has oldest and newest data
//   - what node are falling behind and need to repliate
func (l *Log) LowestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return l.segments[0].baseOffset, nil
}

func (l *Log) HighestOffset() (uint64, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	off := l.segments[len(l.segments)-1].nextOffset
	if off == 0 {
		return 0, nil
	}
	return off - 1, nil
}

package log

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"google.golang.org/protobuf/proto"
)

var (
	errSegmentMaxed          = errors.New("segment is maxed")
	errSegmentHasBrokenIndex = errors.New("segment has a broken index")
)

// The segment wraps the index and store to coordinate operations across the two.
// When Log appends a record to the active segment, the segment write the data to its store
// and a new entry in the index.
type segment struct {
	store      *store
	index      *index
	baseOffset uint64
	nextOffset uint64 // prepare for the next appended record under
	config     Config
}

type indexEntry struct {
	Offset   uint32
	Position uint64
}

func (s *segment) LastIndex() (*indexEntry, error) {
	off, pos, err := s.index.Read(-1)
	if err != nil {
		return nil, err
	}

	return &indexEntry{
		Offset:   off,
		Position: pos,
	}, nil
}

// The log call newSegment when it needs to add a new segment,
// such as when the current active segment hits its max size.
//   - If the index has no entry, the next record appended to the segment is the first record and its offset is segment's base offset.
//   - If the index has at least one entry, the next record appended under and its offset is at the end of the segment.
func newSegment(dir string, baseOffset uint64, cfg Config) (*segment, error) {
	s := &segment{
		baseOffset: baseOffset,
		config:     cfg,
	}

	if err := s.loadStore(dir); err != nil {
		return nil, err
	}

	if err := s.loadIndex(dir); err != nil {
		return nil, err
	}

	if err := s.initNextOffset(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *segment) loadStore(dir string) error {
	storePath := path.Join(dir, fmt.Sprintf("%d%s", s.baseOffset, ".store"))
	storeFile, err := os.OpenFile(
		storePath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("segment failed to open %s: %w", storePath, err)
	}
	_store, err := newStore(storeFile)
	if err != nil {
		return fmt.Errorf("segment failed to load store: %v", err)
	}

	s.store = _store
	return nil
}

func (s *segment) loadIndex(dir string) error {
	indexPath := path.Join(
		dir, fmt.Sprintf("%d%s", s.baseOffset, ".index"),
	)
	indexFile, err := os.OpenFile(
		indexPath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		return fmt.Errorf("segment failed to open %s: %w", indexPath, err)
	}
	_index, err := newIndex(indexFile, s.config)
	if err != nil {
		return fmt.Errorf("segment failed to loadIndex : %v", err)
	}

	s.index = _index
	return nil
}

func (s *segment) initNextOffset() error {
	if s.store.size == 0 {
		s.nextOffset = s.baseOffset
		return nil
	}

	off, _, err := s.lastIndex()
	if err != nil {
		return fmt.Errorf("read last index: %w", err)
	}

	s.nextOffset = s.baseOffset + uint64(off) + 1

	return nil
}

// returns a following errors.
//
//   - errIndexEmpty if index is empty,
//   - errIndexOutOfRange if given relative offset is not in this index
func (s *segment) lastIndex() (uint32, uint64, error) {
	return s.index.Read(-1)
}

func (s *segment) BuildIndexFromStore() error {
	off := s.baseOffset
	pos := uint64(0)

	// discard any existing (wrong or partially-written) entries so the rebuild
	// starts from offset 0 instead of appending onto a broken index.
	s.index.Clear()
	for {
		// appends an entry to the store file
		p, err := s.store.Read(pos)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("segment encountered: %v", err)
		}

		// write to relative off
		err = s.index.Write(uint32(off-s.baseOffset), pos)
		if errors.Is(err, errIndexFulled) {
			return errSegmentMaxed
		}
		if err != nil {
			return fmt.Errorf("segment writes to the index file: %w", err)
		}

		pos += lenWidth + uint64(len(p))
		off++
	}

	s.nextOffset = off
	return nil
}

// Appends the record to the segment and returns the newly appended record's offset.
// It returns errSegmentMaxed error with offset 0
// if there is no index storage space to write the entry under.
func (s *segment) Append(record *api.Record) (offset uint64, err error) {
	cur := s.nextOffset
	record.Offset = cur

	// if index cannot be writable, we do nothing then returning err
	if s.index.IsFull() {
		return 0, errSegmentMaxed
	}

	p, err := proto.Marshal(record)
	if err != nil {
		return 0, fmt.Errorf("failed: %v", err)
	}
	// appends an entry to the store file
	_, pos, err := s.store.Append(p)
	if err != nil {
		return 0, fmt.Errorf("segement appends to the store file: %v", err)
	}
	// after append to the store, then write the entry to the index file
	// WARNING: If this index write failed while appending store is successful,
	// waste data that does not have entry remains in the store
	if err := s.index.Write(
		// index offsets are relative to the base offset
		uint32(s.nextOffset-uint64(s.baseOffset)),
		pos,
	); err != nil {
		if errors.Is(err, errIndexFulled) {
			return 0, errSegmentMaxed
		}
		return 0, fmt.Errorf("segment writes to the index file: %w", err)
	}

	s.nextOffset++
	return cur, nil
}

// returns the record for the given offset.
// Read and Append satisfy the following invariant
//
//	off, err := s.Append(r1)
//	r2, err := s.Read(off)
//	assert.Equal(r1, r2)
func (s *segment) Read(off uint64) (*api.Record, error) {
	// reads the position from the entry
	// TODO: error handling, this can be throws errSegmentOffsetOutOfRangeErr
	_, pos, err := s.index.Read(int64(off - s.baseOffset))
	if errors.Is(err, errIndexEmpty) {
		err = s.BuildIndexFromStore()
	}
	if err != nil {
		return nil, fmt.Errorf(
			"segment failed to read the position from the entry: %v",
			err,
		)
	}
	// reads the data from the position
	// this returns IO err
	p, err := s.store.Read(pos)
	if err != nil {
		return nil, fmt.Errorf(
			"segment failed to read the data at offset: %v",
			err,
		)
	}
	record := &api.Record{}
	err = proto.Unmarshal(p, record)
	return record, nil
}

// validate if the segment has reached its max size, or writing too much to the store or index
func (s *segment) IsMaxed() bool {
	return s.index.IsFull() ||
		s.store.size >= s.config.Segment.MaxStoreBytes
}

func (s *segment) Remove() error {
	if err := s.Close(); err != nil {
		return fmt.Errorf("segment remove failed: %v", err)
	}
	if err := os.Remove(s.index.Name()); err != nil {
		return fmt.Errorf("segment failed to remove the index: %w", err)
	}
	if err := os.Remove(s.store.Name()); err != nil {
		return fmt.Errorf("segment failed to remove the store: %w", err)
	}
	return nil
}

// close this segment.
func (s *segment) Close() error {
	if err := s.store.Close(); err != nil {
		return fmt.Errorf("segment failed to close the store: %w", err)
	}
	if err := s.index.Close(); err != nil {
		return fmt.Errorf("segment failed to close the index: %w", err)
	}
	return nil
}

func (s *segment) Sync() error {
	if err := s.store.Sync(); err != nil {
		return fmt.Errorf("segment failed to sync store: %v", err)
	}
	if err := s.index.Sync(); err != nil {
		return fmt.Errorf("segment failed to sync store: %v", err)
	}
	return nil
}

// returns max( x in ( x < j && x % k == 0 ) ).
// it is used to make sure we stay under the use's disk capacity.
// For example, nearestMultiples(j=9, k=4) == 8,
// because 8 is multiples of 4 and 8 is the maximum greatest one that is <= 9
func nearestMultiples(j, k uint64) uint64 {
	return (j / k) * k
}

// The log call newSegment when it needs to add a new segment,
// such as when the current active segment hits its max size.
//   - If the index has no entry, the next record appended to the segment is the first record and its offset is segment's base offset.
//   - If the index has at least one entry, the next record appended under and its offset is at the end of the segment.
func newSegmentChecked(dir string, baseOffset uint64, cfg Config) (*segment, error) {
	s := &segment{
		baseOffset: baseOffset,
		config:     cfg,
	}

	if err := s.loadStore(dir); err != nil {
		return nil, err
	}

	if err := s.loadIndex(dir); err != nil {
		return nil, err
	}

	if err := s.Repair(); err != nil {
		return nil, err
	}

	if err := s.initNextOffset(); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *segment) Repair() error {
	if err := s.repairIndex(); err != nil {
		return err
	}

	if err := s.verifyIndex(); err != nil {
		return err
	}

	return nil

}

func (s *segment) repairIndex() error {
	if s.store.size == 0 {
		return nil
	}

	if _, _, err := s.lastIndex(); err == nil {
		return nil
	} else {
		switch {
		case errors.Is(err, errIndexEmpty),
			errors.Is(err, errIndexOutOfRange):
			// recover below
			fmt.Println("last")
		default:
			fmt.Println("last error")
			return fmt.Errorf("read last index: %w", err)
		}
	}

	if err := s.BuildIndexFromStore(); err != nil {
		return fmt.Errorf("rebuild index: %w", err)
	}
	fmt.Println("Rebuild")

	return nil
}

func (s *segment) verifyIndex() error {
	if s.store.size == 0 {
		return nil
	}

	_, pos, err := s.lastIndex()
	if err != nil {
		return fmt.Errorf("read last index: %w", err)
	}

	lastPos, err := s.store.LastPositionAbove(pos)
	if err != nil {
		return fmt.Errorf("verify store: %w", err)
	}

	if pos == lastPos {
		return nil
	}

	if err := s.BuildIndexFromStore(); err != nil {
		return fmt.Errorf("rebuild incomplete index: %w", err)
	}

	return nil
}

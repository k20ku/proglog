package log

import (
	"fmt"
	"os"
	"path"

	api "github.com/k20ku/proglog/gen/go/log/v1"
	"google.golang.org/protobuf/proto"
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

// The log call newSegment when it needs to add a new segment,
// such as when the current active segment hits its max size.
//   - If the index has no entry, the next record appended to the segment is the first record and its offset is segment's base offset.
//   - If the index has at least one entry, the next record appended under and its offset is at the end of the segment.
func newSegment(dir string, baseOffset uint64, c Config) (*segment, error) {
	s := &segment{
		baseOffset: baseOffset,
		config:     c,
	}
	var err error
	// store
	storePath := path.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".store"))
	storeFile, err := os.OpenFile(
		storePath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("segment failed to open %s: %w", storePath, err)
	}
	if s.store, err = newStore(storeFile); err != nil {
		return nil, fmt.Errorf("segment failed to newStore: %v", err)
	}
	// index
	indexPath := path.Join(dir, fmt.Sprintf("%d%s", baseOffset, ".index"))
	indexFile, err := os.OpenFile(
		indexPath,
		os.O_CREATE|os.O_RDWR|os.O_APPEND,
		0644,
	)
	if err != nil {
		return nil, fmt.Errorf("segment failed to open %s: %w", indexPath, err)
	}
	if s.index, err = newIndex(indexFile, c); err != nil {
		return nil, fmt.Errorf("segment failed to newIndex: %v", err)
	}
	// nextOffset
	if off, _, err := s.index.Read(-1); err != nil {
		s.nextOffset = baseOffset
	} else {
		s.nextOffset = baseOffset + uint64(off) + 1
	}
	return s, nil
}

// Appends the record to the segment and returns the newly appended record's offset.
// It returns EOF error with offset 0 if there is no index storage space to write the entry under.
//
// WARNING: This implementation does not grant atomicity of writing index and store
func (s *segment) Append(record *api.Record) (offset uint64, err error) {
	cur := s.nextOffset
	record.Offset = cur
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
	_, pos, err := s.index.Read(int64(off - s.baseOffset))
	if err != nil {
		return nil, fmt.Errorf("segment failed to read the position from the entry: %v", err)
	}
	// reads the data from the position
	p, err := s.store.Read(pos)
	if err != nil {
		return nil, fmt.Errorf("segment failed to read the data at offset: %v", err)
	}
	record := &api.Record{}
	err = proto.Unmarshal(p, record)
	return record, nil
}

// validate if the segment has reached its max size, or writing too much to the store or index
func (s *segment) IsMaxed() bool {
	return s.store.size >= s.config.Segment.MaxStoreBytes ||
		s.index.size >= s.config.Segment.MaxIndexBytes
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
	if err := s.index.Close(); err != nil {
		return fmt.Errorf("segment failed to close the index: %w", err)
	}
	if err := s.store.Close(); err != nil {
		return fmt.Errorf("segment failed to close the store: %w", err)
	}
	return nil
}

// returns max( x in ( x < j && x % k == 0 ) ). it is used to make sure we stay under the use's disk capacity. For example, nearestMultiples(j=9, k=4) == 8 because 8 is multiples of 4 and 8 is the maximum greatest one that is <= 9
func nearestMultiples(j, k uint64) uint64 {
	return (j / k) * k
}

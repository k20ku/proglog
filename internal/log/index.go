//go:build unix

package log

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

var (
	offWidth uint64 = 4
	posWidth uint64 = 8
	entWidth        = offWidth + posWidth // entire width of the one entry

	//write
	errIndexFulled = errors.New("index is fulled")
	// read
	errIndexOutOfRange = errors.New("index offset out of range")
	errIndexEmpty      = errors.New("index is empty")
)

type index struct {
	file *os.File
	mmap []byte
	size uint64
}

// creates the index of the given file.
// TODO: windows or darwin compatibility of mmap
func newIndex(f *os.File, c Config) (*index, error) {
	idx := &index{
		file: f,
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("init index failed to get file info: %+v", err)
	}
	idx.size = uint64(fi.Size())
	maxIndexBytes := c.Segment.MaxIndexBytes
	if err = os.Truncate(f.Name(), int64(maxIndexBytes)); err != nil {
		return nil, fmt.Errorf("Truncate failed: %+v", err)
	}
	if idx.mmap, err = unix.Mmap(
		int(f.Fd()),
		0,
		int(maxIndexBytes),
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_SHARED,
	); err != nil {
		return nil, fmt.Errorf("mmap failed: %+v", err)
	}
	return idx, nil
}

// Close the index persisted file. It makes sure that the memory-mapped file has synced its data to the persisted file
// and that the persisted file has flushed its contents to stable storage.
// It truncates empty spaces in index file in closing, so as to restart service properly.
func (i *index) Close() error {
	if err := i.Sync(); err != nil {
		return err
	}
	// on closing, it truncates persisted file to the amount of data that is actually in it.
	if err := i.file.Truncate(int64(i.size)); err != nil {
		return fmt.Errorf("failed to truncate persisted file to its actual data size: %+v", err)
	}
	if err := i.file.Close(); err != nil {
		return fmt.Errorf("failed to close persisted file: %+v", err)
	}
	return nil
}

func (i *index) Sync() error {
	// sync data to persisted file synchronusly
	if err := unix.Msync(i.mmap, unix.MS_SYNC); err != nil {
		return fmt.Errorf("failed to sync data to index persisted file: %+v", err)
	}
	if err := i.file.Sync(); err != nil {
		return fmt.Errorf("failed to sync file to stable storage: %+v", err)
	}
	return nil
}

// Takes in (an offset) and returns the associated record's position in the store.
//
// The given offset is relative to the segments's base offset;
//
//   - -1 is always the offset of the index's last entry.
//
//   - 0 is always the offset of the index's first entry.
//
//   - 1 is the second entry, and so on.
//
//     Read returns a following errors.
//
//   - errIndexEmpty if index is empty,
//
//   - errIndexOutOfRange if given relative offset is not in this index
//
// It uses relative offsets to reduce the size of the indexes by storing offsets as uint32.
func (i *index) Read(in int64) (out uint32, pos uint64, err error) {
	if i.size == 0 {
		return 0, 0, errIndexEmpty
	}
	if in == -1 {
		out = uint32((i.size / entWidth) - 1)
	} else {
		out = uint32(in)
	}
	pos = uint64(out) * entWidth
	if i.size < pos+entWidth {
		return 0, 0, errIndexOutOfRange
	}
	out = enc.Uint32(i.mmap[pos : pos+offWidth])
	pos = enc.Uint64(i.mmap[pos+offWidth : pos+entWidth])
	return out, pos, nil
}

// Appends the given offset and position to the index.
// If there is no space to write the entry, returns errIndexFulled error,
// else write the encoded offset and position to the memory-mapped file
// and then increment the position where the next write will go.
func (i *index) Write(off uint32, pos uint64) error {
	// overflow
	if uint64(len(i.mmap)) < i.size+entWidth {
		return errIndexFulled
	}
	enc.PutUint32(i.mmap[i.size:i.size+offWidth], off)
	enc.PutUint64(i.mmap[i.size+offWidth:i.size+entWidth], pos)
	i.size += entWidth
	return nil
}

// Clear resets the write cursor to the beginning of the index so that
// subsequent Writes overwrite existing entries from offset 0.
//
// It is used before rebuilding the index from the store, so that a wrong or
// partially-written index is discarded instead of being appended to.
// The underlying mmap keeps its size; stale bytes beyond the new size are never
// read (Read bounds every access by size) and are dropped when the index is
// truncated down to size on Close.
func (i *index) Clear() {
	i.size = 0
}

func (i *index) IsFull() bool {
	return uint64(len(i.mmap)) < i.size+entWidth
}

// Returns the index file's path
func (i *index) Name() string {
	return i.file.Name()
}

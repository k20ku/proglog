package log

import "fmt"

type ErrOffsetOutOfRange struct {
	Offset uint64
}

func (e ErrOffsetOutOfRange) Error() string {
	return fmt.Sprintf("offset out of range: %d", e.Offset)
}

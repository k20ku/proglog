package log

import (
	"fmt"
	"os"
)

func dirSync(dir string) (err error) {
	// fsync parent directory
	// 1. open dir
	var dirfd *os.File
	if dirfd, err = os.Open(dir); err != nil {
		return fmt.Errorf(
			"open dir %s: %w",
			dir,
			err,
		)
	}
	defer func() { _ = dirfd.Close() }()
	// 2. fsync!
	if err = dirfd.Sync(); err != nil {
		return fmt.Errorf("sync dir %s: %w",
			dir,
			err,
		)
	}
	return nil
}

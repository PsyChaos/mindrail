//go:build !unix

package storage

import (
	"fmt"
	"io/fs"
	"os"
)

// accessWritableFile approximates the Unix probe where access(2) does not exist.
//
// Windows has no equivalent read-only question: the authoritative answer lives
// in an ACL the standard library does not expose. The owner write bit os.Stat
// synthesizes still catches the read-only case, and being wrong in the
// permissive direction only costs a later write the error it would have raised
// anyway -- whereas guessing "unwritable" would report a healthy repository as
// broken.
func accessWritableFile(path string) error {
	return accessWritableMode(path)
}

// accessWritableDir is the same approximation for a directory.
func accessWritableDir(dir string) error {
	return accessWritableMode(dir)
}

func accessWritableMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("%q is read-only: %w", path, fs.ErrPermission)
	}
	return nil
}

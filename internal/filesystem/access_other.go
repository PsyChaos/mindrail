//go:build !unix

package filesystem

import (
	"fmt"
	"io/fs"
	"os"
)

// accessWritable approximates the Unix probe where access(2) does not exist.
//
// Windows has no equivalent read-only question: the authoritative answer lives
// in an ACL that the standard library does not expose. The owner write bit
// os.Stat synthesizes still catches the read-only case, and being wrong in the
// permissive direction only costs a later create the error it would have raised
// anyway — whereas guessing "unwritable" would report a healthy repository as
// broken.
func accessWritable(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("%q is read-only: %w", dir, fs.ErrPermission)
	}
	return nil
}

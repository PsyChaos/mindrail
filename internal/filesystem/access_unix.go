//go:build unix

package filesystem

import (
	"fmt"
	"syscall"
)

// accessWritable asks the kernel whether this process could create an entry in
// an existing directory.
//
// access(2) is used rather than a trial create because the caller may be doctor
// or status, which decision D-01 forbids from mutating anything: a probe that
// writes a file to find out whether it can write a file is not a read-only
// probe. Write permission alone is not enough — a directory also has to be
// searchable for the create to reach it — so both bits are asked for at once.
//
// The mode bits are spelled out because the standard syscall package does not
// export them on every Unix; their values are fixed by POSIX.
const (
	writeOK = 0x2 // W_OK
	execOK  = 0x1 // X_OK
)

func accessWritable(dir string) error {
	if err := syscall.Access(dir, writeOK|execOK); err != nil {
		return fmt.Errorf("access %q: %w", dir, err)
	}
	return nil
}

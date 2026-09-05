//go:build unix

package storage

import (
	"fmt"
	"syscall"
)

// The POSIX access(2) mode bits, spelled out because the standard syscall
// package does not export them on every Unix. Their values are fixed by POSIX.
const (
	writeOK = 0x2 // W_OK
	execOK  = 0x1 // X_OK
)

// accessWritableFile asks the kernel whether this process could write to an
// existing file.
//
// access(2) rather than an O_WRONLY open: opening the database for writing is
// what SQLite does, and doing it to find out whether it can be done would create
// the WAL sidecars that decision D-01 forbids doctor from leaving behind.
// Only W_OK is asked for; the execute bit means nothing on a regular file.
func accessWritableFile(path string) error {
	if err := syscall.Access(path, writeOK); err != nil {
		return fmt.Errorf("access %q: %w", path, err)
	}
	return nil
}

// accessWritableDir asks whether this process could create an entry in an
// existing directory. A directory has to be searchable as well as writable for
// the create to reach it, so both bits are asked for at once.
func accessWritableDir(dir string) error {
	if err := syscall.Access(dir, writeOK|execOK); err != nil {
		return fmt.Errorf("access %q: %w", dir, err)
	}
	return nil
}

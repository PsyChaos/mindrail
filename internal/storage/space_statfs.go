//go:build linux || darwin

package storage

import (
	"math"
	"syscall"
)

// statFreeSpace asks the kernel what the filesystem holding dir has left.
//
// statfs(2) is the only question in this package that can see a full disk at
// all. access(2) answers from mode bits, and mode bits are perfectly correct on
// a filesystem with zero bytes free -- which is exactly how `doctor` came to
// print `db_writable: true` over a disk that could not take a byte, and then
// prescribe the `mindrail init` that fails on it every time.
//
// It is a read. Nothing is created, nothing is opened, no lock is taken, so
// decision D-01's "doctor mutates nothing" survives it where the obvious
// alternative -- write a byte and see -- would not.
//
// What it can see: the space an unprivileged process may still consume on the
// filesystem this directory lives on, which is the number that decides whether
// `mindrail init` can write. f_bavail rather than f_bfree, so the reserve most
// filesystems keep for the superuser is not counted as room an ordinary user
// has.
//
// What it cannot see, and the report has to say so: a per-user or per-project
// quota, which fails a write with EDQUOT while statfs reports terabytes free; a
// process already at RLIMIT_FSIZE; an exhausted inode table, which fails the
// creation of a `-wal` while there are plenty of blocks left; and a `-wal` that
// cannot grow past a filesystem's own file-size ceiling. Every one of those is a
// full-disk condition this reading calls writable, and every one of them still
// ends in a driver error that diskFullCode classifies -- so the user gets the
// same sentence, one command later.
//
// It can also be wrong in the other direction, which is the direction that
// matters more, so it is worth being exact about how far. On a filesystem whose
// accounting is an estimate rather than a promise -- btrfs and ZFS with
// compression or copy-on-write in play -- f_bavail can reach zero while a small
// write still succeeds. The verdict here would then be "not writable" about a
// filesystem that is, by any practical reading, out of room: `mindrail init`
// writes a schema and a workspace row, not a byte. That is the whole margin the
// zero-only rule leaves, and it is why the rule is zero-only.
func statFreeSpace(dir string) freeSpace {
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return freeSpace{}
	}

	// Both fields are widened rather than trusted at their platform width:
	// f_bsize is signed on Linux and unsigned on Darwin, and a filesystem that
	// reports a nonsense block size must leave the answer unknown rather than
	// produce a byte count that reads as "full".
	blockSize := int64(st.Bsize)
	blocks := int64(st.Bavail)
	if blockSize <= 0 || blocks < 0 {
		return freeSpace{}
	}
	if blocks != 0 && blockSize > math.MaxInt64/blocks {
		// Larger than any real filesystem, and certainly not full. Saturating
		// rather than wrapping matters: a wrapped product is negative, and a
		// negative byte count is how a probe reports "the disk is full" about a
		// filesystem with exabytes free.
		return freeSpace{Known: true, AvailableBytes: math.MaxInt64}
	}
	return freeSpace{Known: true, AvailableBytes: blocks * blockSize}
}

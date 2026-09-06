//go:build !unix

package filesystem

// platformBarrier has no errno to read where the syscall package does not carry
// the Unix ones, and says nothing rather than guessing.
//
// ClassifyRefusal still recognises the package's own sentinels and the io/fs
// ones on this platform, which covers the obstruction, the permission and the
// missing-parent cases; only the read-only mount and the full filesystem lose
// their name, and both then fall back to the generic sentence they had before
// this classification existed -- no better, and not one repository worse.
func platformBarrier(error) Barrier { return BarrierNone }

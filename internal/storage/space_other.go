//go:build !(linux || darwin)

package storage

// statFreeSpace has no answer on a platform whose free-space call this package
// does not speak, and says so.
//
// Guessing in either direction would be worse than silence. Reporting "full"
// would fail every healthy repository on that platform; reporting "plenty" would
// be the same false `db_writable: true` this probe exists to remove, only harder
// to find. An unknown answer leaves the verdict to the permission probes, which
// is exactly where it was before -- no better, and not one repository worse.
//
// The two platforms with a real implementation are the ones the project builds
// and tests on. Anything else is welcome to grow one; until it does, a full disk
// there is diagnosed at the driver, by diskFullCode, which is portable.
func statFreeSpace(string) freeSpace { return freeSpace{} }

//go:build !linux

package storage_test

// fullDiskChild is never that child anywhere else. The full-filesystem scenario
// needs a mount namespace to build the filesystem it fills, which is a Linux
// facility; on every other platform the deterministic tests in
// space_internal_test.go and diskfull_test.go are what hold the detection and
// the classification in place.
func fullDiskChild() (int, bool) { return 0, false }

// readOnlyChild is never that child anywhere else either, and for the same
// reason: the read-only mount the scenario needs is arranged with a Linux mount
// namespace. The classification it exercises is held in place elsewhere by
// TestConditionBlockerLetsTheConditionOverrideThePartItMet and the
// ClassifyRefusal table in internal/filesystem, both of which are portable.
func readOnlyChild() (int, bool) { return 0, false }

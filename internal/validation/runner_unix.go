//go:build linux || darwin

package validation

import (
	"os/exec"
	"syscall"
)

// newProcessGroup detaches the child into its own process group so a timeout
// kills the whole tree, not just the direct child: orphaned grandchildren
// would otherwise hold the captured pipes open and stall Run past the
// deadline (Breaker B-2). Best effort — a group that already exited is gone,
// and pid reuse inside this window is out of scope.
func newProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

//go:build !(linux || darwin)

package validation

import (
	"os/exec"
)

// newProcessGroup is a no-op outside unix: only the direct child is killed,
// and pipe-holding grandchildren may delay return past the deadline. The
// verdict is still timeout; document the platform when it matters.
func newProcessGroup(command *exec.Cmd) {
}

func killProcessGroup(pid int) {
}

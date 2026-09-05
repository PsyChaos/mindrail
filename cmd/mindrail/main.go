// Command mindrail is the single Mindrail executable.
//
// main owns only the root process context, application bootstrap, CLI
// execution and the exit code (tech-stack §85). It contains no domain logic.
package main

import (
	"fmt"
	"os"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/cli"
)

func main() {
	ctx, stop := app.RootContext()
	defer stop()

	err := cli.Execute(ctx)

	// The command has drained its own application by the time Execute returns:
	// every command registers the shutdown before it starts anything, so the
	// SQLite handle is already closed here. All that is left is to release the
	// signal handler before os.Exit skips the deferred one, so a second
	// interrupt during teardown gets the default behaviour rather than being
	// swallowed by a process that is already leaving.
	stop()

	if err != nil {
		fmt.Fprintln(os.Stderr, "mindrail:", err)
		os.Exit(app.ExitCode(err))
	}
}

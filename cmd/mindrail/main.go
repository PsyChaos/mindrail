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

	if err := cli.Execute(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "mindrail:", err)
		stop()
		os.Exit(app.ExitCode(err))
	}
}

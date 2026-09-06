// Package cli builds the Mindrail command tree. Commands translate flags into
// application service calls and render results; they hold no domain logic.
package cli

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/git"
)

// Options are the process seams the command tree cannot discover for itself.
//
// There is one, and it exists because the git-failure branch was otherwise
// unreachable from a test: every command wired git.NewExecRunner() directly, so
// "git is missing" and "git hangs" could only be exercised by mutating PATH for
// the whole process, which a parallel test suite cannot do. That is why the
// contradiction between the envelope and the exit code survived a green suite
// (findings F11, F19). internal/git ships FakeRunner as a non-test file for
// exactly this.
//
// The zero value is production: the real git binary, found on PATH.
type Options struct {
	// Runner executes git subprocesses. nil means git.NewExecRunner().
	Runner git.CommandRunner
}

const rootLong = `Mindrail is a local engineering gate for AI coding agents.

It discovers the actual code change from Git, binds durable engineering
constraints to it, requires snapshot-bound evidence and denies completion
when the proof is insufficient.`

// Root is the command tree together with the command line it was handed.
//
// The extra field exists for one reason. When cobra rejects a command line — an
// unknown subcommand, an unknown flag, a surplus argument — no command body ever
// runs, so nothing emits the single JSON object decision D-15 promises on
// stdout. Answering "did the caller ask for --json?" needs the raw line, and
// neither cobra nor pflag will give it back: cobra keeps its argument slice
// private, and pflag discards the input at the first flag it does not
// recognise. Reading the flag set instead would make the answer depend on the
// order the user happened to type, which is a worse contract than none
// (findings F04, F08).
type Root struct {
	*cobra.Command
	commandLine []string
}

// NewRoot builds the production command tree.
func NewRoot() *Root {
	return NewRootWith(Options{})
}

// NewRootWith builds the root command against explicit seams. Subcommands are
// registered here so that the tree stays additive as milestones land.
func NewRootWith(o Options) *Root {
	root := &Root{}

	cmd := &cobra.Command{
		Use:   "mindrail",
		Short: "Local engineering gate for AI coding agents",
		Long:  rootLong,

		// Errors are rendered and exit-coded by main, not by cobra.
		SilenceErrors: true,
		SilenceUsage:  true,

		// The root has to be runnable for cobra to reach ValidateArgs at all —
		// a non-runnable command returns ErrHelp first — and running it with no
		// arguments has always printed help, so that is what it does.
		RunE: func(c *cobra.Command, _ []string) error { return c.Help() },

		// Replaces cobra's legacyArgs, which produced the same sentence but as a
		// bare error that app.ExitCode could only read as "operation failed".
		// It is classified by the walk in classifyArgs like every other one.
		Args: unknownSubcommand,
	}

	cmd.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return root.usageError(c, err)
	})

	// Persistent, because every command answers the same four questions: which
	// shape the answer takes, how much of the startup sequence is worth
	// logging, whether colour is welcome, and which directory the question is
	// about.
	flags := cmd.PersistentFlags()
	flags.Bool(flagJSON, false, "emit the result as a single JSON object on stdout")
	flags.Bool(flagVerbose, false, "log the startup sequence to stderr")
	flags.Bool(flagNoColor, false, "never colour human output")
	flags.StringP(flagChdir, "C", "", "run as if mindrail was started in this directory")

	cmd.AddCommand(
		newInitCommand(o),
		newStatusCommand(o),
		newDoctorCommand(o),
		newVersionCommand(),
	)

	// A help command of our own, because cobra's prints the whole root help on
	// stdout and exits 0 for a topic it cannot find. `mindrail help bogus` is a
	// command line this binary does not understand, and it takes the same
	// answer as every other one.
	cmd.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if err != nil || target == nil || (len(args) > 0 && target == c.Root()) {
				return root.usageError(c,
					fmt.Errorf("unknown help topic %q for %q", strings.Join(args, " "), c.Root().Name()))
			}
			return target.Help()
		},
	})

	root.Command = cmd
	return root
}

// SetArgs records the command line before handing it to cobra, so that a
// failure during parsing can still tell whether --json was asked for.
func (r *Root) SetArgs(args []string) {
	r.commandLine = slices.Clone(args)
	r.Command.SetArgs(args)
}

// ExecuteContext classifies the tree and then runs it.
//
// The classification cannot happen at construction. cobra adds commands of its
// own — `help`, `completion` and one per shell — inside ExecuteC, so a loop over
// Commands() in the constructor has already finished by the time they exist and
// they were left with their own unwrapped argument checks: `mindrail completion
// bash extra --json` exited 1 with an empty stdout, which is precisely the
// symptom findings F04 and F08 are about. Asking cobra to add them first, and
// then walking the finished tree, is what makes "every command in this binary"
// mean it.
func (r *Root) ExecuteContext(ctx context.Context) error {
	r.classifyArgs()
	return r.Command.ExecuteContext(ctx)
}

// Execute is ExecuteContext with the background context, kept so that *Root
// still satisfies every use a *cobra.Command was put to.
func (r *Root) Execute() error { return r.ExecuteContext(context.Background()) }

// classifyArgs installs the usage classification on every command in the tree,
// including the ones cobra supplies.
//
// Wrapping is idempotent: a command already carrying a classified check is left
// alone, so a second execution of the same tree does not stack closures.
func (r *Root) classifyArgs() {
	r.Command.InitDefaultHelpCmd()
	r.Command.InitDefaultCompletionCmd()

	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		r.classifyArgsOf(cmd)
		for _, sub := range cmd.Commands() {
			walk(sub)
		}
	}
	walk(r.Command)
}

// unknownSubcommand refuses a positional argument to a command whose whole job
// is to hold other commands.
func unknownSubcommand(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return nil
	}
	return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
}

// classifiedArgs marks an argument check this package has already wrapped.
type classifiedArgs struct{ check cobra.PositionalArgs }

func (c classifiedArgs) call(cmd *cobra.Command, args []string) error {
	if c.check == nil {
		return nil
	}
	return c.check(cmd, args)
}

func (r *Root) classifyArgsOf(cmd *cobra.Command) {
	if _, done := cmd.Annotations[argsClassifiedAnnotation]; done {
		return
	}

	// cobra returns ErrHelp for a command with nothing to run *before* it ever
	// validates arguments, so a command that only groups others never reaches
	// its own check: `mindrail completion bogus` printed a page of help on
	// stdout and exited 0, carrying cobra.NoArgs the whole time, where
	// `mindrail bogus` exits 2. Running it prints help, which is what it did
	// before.
	if cmd.HasSubCommands() && !cmd.Runnable() {
		cmd.RunE = func(c *cobra.Command, _ []string) error { return c.Help() }
	}

	declared := classifiedArgs{check: cmd.Args}
	if declared.check == nil {
		if !cmd.HasSubCommands() {
			// A leaf that declares no check accepts anything, and inventing one
			// here would refuse arguments cobra's own commands rely on — the
			// help command's topic among them.
			return
		}
		declared.check = unknownSubcommand
	}
	cmd.Args = func(c *cobra.Command, args []string) error {
		if err := declared.call(c, args); err != nil {
			return r.usageError(c, err, suggestionRemedies(c, args)...)
		}
		return nil
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[argsClassifiedAnnotation] = "true"
}

// argsClassifiedAnnotation records that a command's argument check has already
// been wrapped, so that classifyArgs can run more than once.
const argsClassifiedAnnotation = "mindrail.args_classified"

// usageError classifies a command line this binary could not understand.
//
// Decision D-03 and tech-stack §13 reserve exit 2 for invalid usage, and every
// other usage mistake this binary knows — a directory that is not a repository,
// a bare repository, a malformed config.toml — already returns it. A typo'd
// subcommand is the same class of mistake; returning cobra's error unwrapped
// left it exiting 1, the code reserved for a gate that ran and failed, so a
// wrapper branching on "was my invocation wrong?" got the wrong answer
// (findings F04, F08).
func (r *Root) usageError(cmd *cobra.Command, err error, extra ...string) error {
	next := append([]string{
		"Run `" + cmd.CommandPath() + " --help` to see the accepted commands and flags.",
	}, extra...)

	verdict := app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		err.Error(),
		"Mindrail did not run: no repository was inspected and nothing was written.",
		next...,
	)

	r.emitUsageEnvelope(cmd, verdict)
	return verdict
}

// suggestionRemedies turns cobra's near-miss suggestions into remedies. They
// belong in next_action rather than glued onto the message, because that is the
// field spec §84 reserves for "what to do next" and the only one a machine
// reads for it.
func suggestionRemedies(cmd *cobra.Command, args []string) []string {
	if len(args) == 0 {
		return nil
	}

	suggestions := cmd.SuggestionsFor(args[0])
	remedies := make([]string, 0, len(suggestions))
	for _, suggestion := range suggestions {
		remedies = append(remedies, "Did you mean `"+cmd.CommandPath()+" "+suggestion+"`?")
	}
	return remedies
}

// emitUsageEnvelope gives a rejected command line the same single JSON object
// every other failure puts on stdout.
//
// It is best-effort: a write failure here has nowhere left to be reported, and
// the exit code already carries the verdict. Nothing else has written to stdout
// at this point — the command body never ran — so the envelope is still the only
// value on it.
func (r *Root) emitUsageEnvelope(cmd *cobra.Command, verdict error) {
	if !r.jsonRequested() {
		return
	}
	_ = app.WriteJSON(cmd.OutOrStdout(), cmd.Name(), nil, nil, verdict)
}

// jsonRequested reads --json off the raw command line. Everything after a bare
// `--` is an argument rather than a flag, so the scan stops there.
//
// The value is parsed with strconv.ParseBool because that is what pflag does
// with it. Matching only the literal `--json=true` meant `--json=1` — which the
// flag package accepts everywhere else in this binary — produced exit 2 with an
// empty stdout, so whether a rejected command line got its envelope depended on
// which spelling of "yes" the caller happened to use.
func (r *Root) jsonRequested() bool {
	prefix := "--" + flagJSON

	for _, arg := range r.commandLine {
		if arg == "--" {
			return false
		}
		if arg == prefix {
			return true
		}
		if value, ok := strings.CutPrefix(arg, prefix+"="); ok {
			if asked, err := strconv.ParseBool(value); err == nil {
				return asked
			}
			// An unparseable value is a command line pflag would reject too. The
			// envelope is withheld rather than guessed at, and the sentence on
			// stderr still names the flag.
			return false
		}
	}
	return false
}

// Execute runs the command tree against ctx and returns the command error
// unchanged so that main can map it to an exit code.
func Execute(ctx context.Context) error {
	root := NewRoot()
	root.SetArgs(os.Args[1:])
	return root.ExecuteContext(ctx)
}

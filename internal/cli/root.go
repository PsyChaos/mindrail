// Package cli builds the Mindrail command tree. Commands translate flags into
// application service calls and render results; they hold no domain logic.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

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

	// BusyTimeout is how long a command waits for the runtime database's
	// write lock before MINDRAIL_BUSY_RETRYABLE; zero means storage's
	// default, five seconds (decision D-08). It is a seam for the tests that
	// hold the lock and drive a command into the refusal, which at the
	// production budget would cost five seconds per row.
	BusyTimeout time.Duration

	// RunMCP starts the SDK-backed MCP server for a resolved working directory.
	// It is injected by cmd/mindrail so ordinary CLI commands remain isolated
	// from the SDK's network-capable optional transports.
	RunMCP func(context.Context, string) error
}

const rootLong = `Mindrail is a local engineering gate for AI coding agents.

It discovers the actual code change from Git, binds durable engineering
constraints to it, requires snapshot-bound evidence and denies completion
when the proof is insufficient.

Start once with mindrail init. Your coding agent handles task coordination
through MCP. Use status, doctor and verify to inspect readiness and changes.
Advanced commands remain available through mindrail help <command>.`

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
		RunE: func(c *cobra.Command, _ []string) error { return root.helpOrEnvelope(c) },

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
		newSessionCommand(o),
		newTaskCommand(o),
		newCheckpointCommand(o),
		newLeaseCommand(o),
		newVerifyCommand(o),
		newKnowledgeCommand(o),
		newHookCommand(o),
		newMCPCommand(o),
	)
	for _, sub := range cmd.Commands() {
		switch sub.Name() {
		case "init", "status", "doctor", "verify", "version":
		default:
			sub.Hidden = true
		}
	}
	cmd.CompletionOptions.HiddenDefaultCmd = true

	// A help command of our own, because cobra's prints the whole root help on
	// stdout and exits 0 for a topic it cannot find. `mindrail help bogus` is a
	// command line this binary does not understand, and it takes the same
	// answer as every other one.
	cmd.SetHelpCommand(&cobra.Command{
		Use:   helpCommandName + " [command]",
		Short: "Help about any command",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if err != nil || target == nil || (len(args) > 0 && target == c.Root()) {
				return root.usageError(c,
					fmt.Errorf("unknown help topic %q for %q", strings.Join(args, " "), c.Root().Name()))
			}
			return target.Help()
		},
		// Carried over from the command this replaces. Without it, `mindrail
		// help <TAB>` stopped offering command names and started offering
		// filenames: cobra's own help command supplies this function, and
		// SetHelpCommand replaces the whole command rather than part of it.
		ValidArgsFunction: helpTopics,
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

// helpCommandName is the name cobra gives its help command, spelled once so the
// replacement and the completion above cannot drift apart.
const helpCommandName = "help"

// helpTopics completes `mindrail help <TAB>` with the commands there is help
// for, which is what cobra's own help command does and what replacing it lost.
//
// NoFileComp rather than the default directive: a shell offering filenames after
// `mindrail help ` is offering something that can never be a help topic.
func helpTopics(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
	target, _, err := cmd.Root().Find(args)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	if target == nil {
		target = cmd.Root()
	}

	var topics []cobra.Completion
	for _, sub := range target.Commands() {
		// `help` itself is a topic — `mindrail help help` is a real thing to
		// ask for — and it does not report itself as an available command.
		if !sub.IsAvailableCommand() && sub.Name() != helpCommandName {
			continue
		}
		if strings.HasPrefix(sub.Name(), toComplete) {
			topics = append(topics, cobra.CompletionWithDesc(sub.Name(), sub.Short))
		}
	}
	return topics, cobra.ShellCompDirectiveNoFileComp
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
		cmd.RunE = func(c *cobra.Command, _ []string) error { return r.helpOrEnvelope(c) }
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
// helpOrEnvelope answers a command that groups others and was named without one
// of them.
//
// Help is the right answer for a person and not an answer at all for `--json`:
// decision D-15 promises one JSON object on stdout and nothing else, and
// `mindrail task --json` put 968 bytes of help there and exited 0 — so
// `mindrail checkpoint --json && echo "handoff recorded"` printed that with
// nothing written (finding F34). A group named without a subcommand is a
// command line this binary cannot act on, which is what exit 2 means.
func (r *Root) helpOrEnvelope(cmd *cobra.Command) error {
	if r.jsonRequested() {
		return r.usageError(cmd, fmt.Errorf(
			"%s names a group of commands rather than a command to run", cmd.CommandPath()))
	}
	return cmd.Help()
}

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
	// MCP owns stdout even when parsing refuses the invocation before RunE;
	// its typed usage error is rendered on stderr by the process entrypoint.
	if r.mcpRequested(cmd) {
		return
	}
	if !r.jsonRequested() {
		return
	}
	_ = app.WriteJSON(cmd.OutOrStdout(), publishedName(cmd), nil, nil, verdict)
}

// mcpRequested inspects only the first positional command candidate, skipping
// values of known root flags. Unknown leading flags can make Cobra resolve the
// wrong command, so a path or help argument named "mcp" must not be mistaken
// for a protocol invocation when deciding whether to emit a usage envelope.
func (r *Root) mcpRequested(cmd *cobra.Command) bool {
	if len(r.commandLine) == 0 {
		return cmd.Name() == "mcp" && cmd.Parent() == r.Command
	}
	for i := 0; i < len(r.commandLine); i++ {
		arg := r.commandLine[i]
		if arg == "--" {
			return false
		}
		if strings.HasPrefix(arg, "--") {
			name, _, hasValue := strings.Cut(arg[2:], "=")
			flag := r.PersistentFlags().Lookup(name)
			if flag != nil && flag.NoOptDefVal == "" && !hasValue {
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			flag := r.PersistentFlags().ShorthandLookup(arg[1:2])
			if flag != nil && flag.NoOptDefVal == "" && len(arg) == 2 {
				i++
			}
			continue
		}
		return arg == "mcp"
	}
	return false
}

// publishedName is the command a reader typed, without the binary in front of
// it: "task open", not "open".
//
// It used to be cmd.Name(), which is the leaf word. That was invisible while
// every command was top level — at `cd74767` the leaf word *was* the command —
// and MR-003 added the first nested ones, at which point one command
// contradicted itself under one error class: `task state <id> --to BOGUS`
// published "task state", because the body raises that error and passes its own
// name, while `task state --to CLAIMED` with the id missing published "state",
// because cobra raises it here. Both exit 2 with COMMAND_LINE_INVALID, and a
// consumer keying on `command` saw two different commands (finding F33).
func publishedName(cmd *cobra.Command) string {
	if _, rest, found := strings.Cut(cmd.CommandPath(), " "); found {
		return rest
	}
	return cmd.CommandPath()
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
	return ExecuteWith(ctx, os.Args[1:], Options{})
}

// ExecuteWith runs an explicit command line against injected process seams.
// Production main uses it to wire the public MCP stdio launcher without making
// the normal CLI package depend on the MCP SDK.
func ExecuteWith(ctx context.Context, args []string, options Options) error {
	root := NewRootWith(options)
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}

// newMCPCommand reserves the public `mindrail mcp` command in the normal
// command tree while leaving SDK construction to the executable's composition
// root. MCP owns stdout, so this command must not emit CLI envelopes itself.
func newMCPCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve Mindrail's MCP tools over stdio",
		Long:  "Serve the Mindrail MCP protocol over stdin/stdout. Diagnostics go to stderr; stdout is protocol-only.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.RunMCP == nil {
				return app.NewError(app.CodeStartupIncomplete, app.KindUnavailable,
					"MCP server launcher is unavailable in this command composition",
					"The MCP protocol did not start.",
					"Run the packaged mindrail binary, which wires the MCP launcher.")
			}
			if jsonRequested, _ := cmd.Flags().GetBool(flagJSON); jsonRequested {
				return app.NewError(app.CodeCommandLineInvalid, app.KindUsage,
					"mcp owns stdout for protocol frames; remove --json",
					"The MCP server did not start and stdout remained protocol-only.",
					"Re-run `mindrail mcp` without --json and connect an MCP stdio client.")
			}
			startDir, _ := cmd.Flags().GetString(flagChdir)
			if startDir == "" {
				var err error
				startDir, err = os.Getwd()
				if err != nil {
					return err
				}
			}
			root, err := filepath.Abs(startDir)
			if err != nil {
				return err
			}
			return o.RunMCP(cmd.Context(), root)
		},
	}
}

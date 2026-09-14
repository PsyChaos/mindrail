package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
)

// The flags MR-004 adds. --operation-id is on every writer and --expect-revision
// on `task state`; --file and --task are `lease acquire`'s two targets.
const (
	flagOperationID    = "operation-id"
	flagExpectRevision = "expect-revision"
	flagFile           = "file"
	flagTask           = "task"
)

// newLeaseCommand builds `mindrail lease`.
func newLeaseCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "lease",
		Short: "Hold, keep and give up the lease on a file or a task",
		Long: `Manage leases.

A lease is one session holding one target — a file, or a task — for twenty
minutes from its last renewal. Two sessions cannot hold one target at once: the
second is told who holds it and until when. A holder's own writes renew its
task lease; ` + "`lease renew`" + ` is for the holder that has nothing to write, and
` + "`lease release`" + ` for the one stepping away without a note. A lease that
runs out is taken over by the next session that needs the target, and the
takeover is reported.`,
	}
	group.AddCommand(
		newLeaseAcquireCommand(o),
		newLeaseRenewCommand(o),
		newLeaseReleaseCommand(o),
		newLeaseListCommand(o),
	)
	return group
}

func newLeaseAcquireCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "acquire",
		Short: "Take the lease on a file, or on a task where it stands",
		Long: `Take a lease.

` + "`--file <path>`" + ` leases a repository-relative path; the file need not exist.
` + "`--task <task-id>`" + ` takes a task that is already in a working state where it
stands — the claim without a move — and makes this session its claimant. An
OPEN task is claimed by moving it: ` + "`mindrail task state <task-id> --to CLAIMED`" + `.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			file, _ := cmd.Flags().GetString(flagFile)
			task, _ := cmd.Flags().GetString(flagTask)
			handle, _ := cmd.Flags().GetString(flagSession)

			target, refusal := leaseTargetFlags(cmd, file, task)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "lease acquire", o, refusal)
			}
			operation, refusal := operationIDFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, "lease acquire", o, refusal)
			}

			return runCoordination(cmd, "lease acquire", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					acquired, write, err := s.store.Idempotent(operation).AcquireLease(ctx, s.projectID(), s.attribution(handle), target)
					if err != nil {
						return nil, nil, err
					}
					result := leaseResult{
						Lease:      acquired.Lease,
						Renewed:    acquired.Renewed,
						Superseded: acquired.Superseded,
						Task:       acquired.Task,
						attributed: attributed{Session: write.Session, SessionMinted: write.Minted, Replayed: write.Replayed, OperationID: write.OperationID},
						verb:       "acquired",
					}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagFile, "", "the repository-relative path to lease")
	cmd.Flags().String(flagTask, "", "the id of a task in a working state to take where it stands")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	cmd.Flags().String(flagOperationID, "", "an id this request carries, so a retry of it is answered from the first delivery")
	return cmd
}

func newLeaseRenewCommand(o Options) *cobra.Command {
	return newLeaseHolderCommand(o, "renew", "Move a held lease's expiry forward",
		"renewed", func(ctx context.Context, s *coordination.Store, id string, by coordination.Attribution) (coordination.Lease, coordination.Write, error) {
			return s.RenewLease(ctx, id, by)
		})
}

func newLeaseReleaseCommand(o Options) *cobra.Command {
	return newLeaseHolderCommand(o, "release", "Give a held lease up",
		"released", func(ctx context.Context, s *coordination.Store, id string, by coordination.Attribution) (coordination.Lease, coordination.Write, error) {
			return s.ReleaseLease(ctx, id, by)
		})
}

// newLeaseHolderCommand is what `lease renew` and `lease release` share: one
// lease id, the holder's session, and a write only the holder may make.
func newLeaseHolderCommand(o Options, verb, short, past string,
	write func(context.Context, *coordination.Store, string, coordination.Attribution) (coordination.Lease, coordination.Write, error)) *cobra.Command {

	name := "lease " + verb
	cmd := &cobra.Command{
		Use:   verb + " <lease-id>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			handle, _ := cmd.Flags().GetString(flagSession)
			operation, refusal := operationIDFlag(cmd)
			if refusal != nil {
				return refuseBeforeStarting(cmd, name, o, refusal)
			}

			return runCoordination(cmd, name, o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					lease, wrote, err := write(ctx, s.store.Idempotent(operation), args[0], s.attribution(handle))
					if err != nil {
						return nil, nil, err
					}
					result := leaseResult{
						Lease:      lease,
						attributed: attributed{Session: wrote.Session, SessionMinted: wrote.Minted, Replayed: wrote.Replayed, OperationID: wrote.OperationID},
						verb:       past,
					}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	cmd.Flags().String(flagOperationID, "", "an id this request carries, so a retry of it is answered from the first delivery")
	return cmd
}

func newLeaseListCommand(o Options) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List this project's active leases, oldest first",
		Long: `List the leases this project holds right now.

Expired and released leases are not listed, and nothing is written: an expired
row is closed by the next session that takes the target, not by a reader.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCoordination(cmd, "lease list", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					leases, err := s.store.ListLeases(ctx, s.projectID())
					if err != nil {
						return nil, nil, err
					}
					result := leaseListResult{Leases: leases}
					return result, result.RenderHuman, nil
				})
		},
	}
}

// leaseTargetFlags reads exactly one of --file and --task into a Target, or
// refuses the command line before anything starts.
func leaseTargetFlags(cmd *cobra.Command, file, task string) (coordination.Target, error) {
	switch {
	case file != "" && task != "":
		return coordination.Target{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"--file and --task were both given; a lease is over one target",
			"Mindrail did not run: nothing was read and nothing was written.",
			"Pass either --file <path> or --task <task-id>.",
			"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
		)
	case file == "" && task == "":
		return coordination.Target{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"a lease needs a target",
			"Mindrail did not run: nothing was read and nothing was written.",
			"Pass --file <path> to lease a file, or --task <task-id> to take a task where it stands.",
			"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
		)
	case task != "":
		return coordination.TaskTarget(task), nil
	}

	// Invalid UTF-8 is refused the way every free-text flag is (finding F28's
	// rule, decision D-77): before anything starts, naming the flag, with the
	// bytes quoted so the refusal itself is representable.
	if !utf8.ValidString(file) {
		return coordination.Target{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"the value given for --"+flagFile+" is not valid UTF-8: "+strconv.Quote(file),
			"Mindrail did not run: nothing was read and nothing was written.",
			"Pass a --"+flagFile+" whose every byte is valid UTF-8.",
		).WithMetadata("flag", flagFile).WithMetadata("unrepresentable_value", strconv.Quote(file))
	}

	target, err := coordination.FileTarget(file)
	if err != nil {
		// The store's own refusal, with the impact a judgment made before the
		// application starts can make: Mindrail never ran.
		payload, _ := app.PayloadOf(err)
		return coordination.Target{}, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			payload.Why,
			"Mindrail did not run: nothing was read and nothing was written.",
			append(payload.NextAction, "Run `"+cmd.CommandPath()+" --help` to see the accepted flags.")...,
		).WithMetadata("flag", flagFile).WithMetadata("target_key", file)
	}
	return target, nil
}

// operationIDFlag reads --operation-id and judges it before the application
// starts (decision D-71): a malformed id is a mistake in the command line.
func operationIDFlag(cmd *cobra.Command) (string, error) {
	id, _ := cmd.Flags().GetString(flagOperationID)
	if id == "" || coordination.ValidOperationID(id) {
		return id, nil
	}
	return "", app.NewError(
		app.CodeCommandLineInvalid,
		app.KindUsage,
		"the value given for --"+flagOperationID+" is not an identifier: "+strconv.Quote(id),
		"Mindrail did not run: nothing was read and nothing was written.",
		"Pass --operation-id as up to 128 letters, digits, dots, underscores, colons or dashes, starting with a letter or digit.",
		"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
	).WithMetadata("flag", flagOperationID).WithMetadata("operation_id", strconv.Quote(id))
}

// expectRevisionFlag reads --expect-revision and judges it before the
// application starts (decision D-72): zero means no expectation, and a
// revision below one names none a task can be at.
func expectRevisionFlag(cmd *cobra.Command) (int64, error) {
	if !cmd.Flags().Changed(flagExpectRevision) {
		return 0, nil
	}
	raw, _ := cmd.Flags().GetString(flagExpectRevision)
	revision, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || revision < 1 {
		return 0, app.NewError(
			app.CodeCommandLineInvalid,
			app.KindUsage,
			"the value given for --"+flagExpectRevision+" is not a revision: "+strconv.Quote(raw),
			"Mindrail did not run: nothing was read and nothing was written.",
			"Pass --expect-revision as the revision `mindrail task show` printed, 1 or above.",
			"Run `"+cmd.CommandPath()+" --help` to see the accepted flags.",
		).WithMetadata("flag", flagExpectRevision).WithMetadata("expected_revision", strconv.Quote(raw))
	}
	return revision, nil
}

// attributed is the part of every writer's result that says which session it
// ran under and whether the operations table answered (decisions D-61, D-71).
type attributed struct {
	Session       coordination.Session `json:"session"`
	SessionMinted bool                 `json:"session_minted"`
	Replayed      bool                 `json:"replayed"`
	OperationID   string               `json:"operation_id,omitempty"`
}

// renderAttribution prints what a person has to notice after a write: that a
// session was minted for them, and that a replay wrote nothing.
func (a attributed) renderAttribution(w io.Writer) error {
	if a.Replayed {
		if _, err := fmt.Fprintf(w,
			"\nThis is the recorded result of operation %s; nothing was written again.\n", a.OperationID); err != nil {
			return err
		}
	}
	if a.SessionMinted {
		if _, err := fmt.Fprintf(w,
			"\nNo --session was given, so session %s was opened for this run.\nPass --session %s to keep later commands in it.\n",
			a.Session.ID, a.Session.ID); err != nil {
			return err
		}
	}
	return nil
}

// leaseResult is what the three lease writers publish.
type leaseResult struct {
	Lease      coordination.Lease  `json:"lease"`
	Renewed    bool                `json:"renewed,omitempty"`
	Superseded *coordination.Lease `json:"superseded,omitempty"`
	Task       *coordination.Task  `json:"task,omitempty"`
	attributed

	verb string
}

func (r leaseResult) RenderHuman(w io.Writer, _ bool) error {
	verb := r.verb
	if r.Renewed {
		verb = "already held; renewed"
	}
	if _, err := fmt.Fprintf(w, "Lease %s on %s %s.\n", r.Lease.ID, r.Lease.Target(), verb); err != nil {
		return err
	}
	if err := renderLeaseLine(w, r.Lease); err != nil {
		return err
	}
	if r.Superseded != nil {
		if _, err := fmt.Fprintf(w, "  Took over from session %s, whose lease expired at %s.\n",
			r.Superseded.Holder, app.FormatTime(r.Superseded.ExpiresAt)); err != nil {
			return err
		}
	}
	if r.Task != nil {
		if _, err := fmt.Fprintf(w, "  Task %s is %s (revision %d), claimed by %s.\n",
			r.Task.ID, r.Task.State, r.Task.Revision, r.Task.ClaimedBy); err != nil {
			return err
		}
	}
	return r.renderAttribution(w)
}

// renderLeaseLine prints one lease's status the way every command prints it,
// so `task show`, `lease list` and the writers say one thing about one row.
func renderLeaseLine(w io.Writer, lease coordination.Lease) error {
	var err error
	switch lease.Status {
	case coordination.LeaseActive:
		_, err = fmt.Fprintf(w, "  Held by session %s until %s.\n", lease.Holder, app.FormatTime(lease.ExpiresAt))
	case coordination.LeaseExpired:
		_, err = fmt.Fprintf(w, "  Was held by session %s; expired at %s.\n", lease.Holder, app.FormatTime(lease.ExpiresAt))
	case coordination.LeaseReleased:
		when := ""
		if lease.ReleasedAt != nil {
			when = " at " + app.FormatTime(*lease.ReleasedAt)
		}
		_, err = fmt.Fprintf(w, "  Was held by session %s; released%s (%s).\n", lease.Holder, when, lease.ReleaseReason)
	}
	return err
}

// leaseListResult is what `lease list` publishes. Leases is never nil so the
// marshalled report says [] rather than null for a project with none.
type leaseListResult struct {
	Leases []coordination.Lease `json:"leases"`
}

func (r leaseListResult) RenderHuman(w io.Writer, _ bool) error {
	if len(r.Leases) == 0 {
		_, err := io.WriteString(w, "This project holds no active leases.\n")
		return err
	}
	for _, lease := range r.Leases {
		if _, err := fmt.Fprintf(w, "%s  %-5s %s\n    held by %s until %s\n",
			lease.ID, lease.TargetKind, lease.TargetKey, lease.Holder, app.FormatTime(lease.ExpiresAt)); err != nil {
			return err
		}
	}
	return nil
}

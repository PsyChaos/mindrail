package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/PsyChaos/mindrail/internal/coordination"
)

// newCheckpointCommand builds `mindrail checkpoint`.
func newCheckpointCommand(o Options) *cobra.Command {
	group := &cobra.Command{
		Use:   "checkpoint",
		Short: "Leave a note on a task for whoever picks it up next",
		Long: `Write the handover note an arriving agent reads.

Checkpoints are append-only: there is no edit and no delete, because a note that
could be rewritten after the fact is one the next agent cannot rely on. Read them
with ` + "`mindrail task show`" + `, which returns the newest.`,
	}
	group.AddCommand(newCheckpointWriteCommand(o))
	return group
}

func newCheckpointWriteCommand(o Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "write <task-id>",
		Short: "Append a note to a task",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			note, _ := cmd.Flags().GetString(flagNote)
			handoff, _ := cmd.Flags().GetBool(flagHandoff)
			handle, _ := cmd.Flags().GetString(flagSession)

			// The note is judged before the application starts, for the same
			// reason `task state` parses --to there: an empty note is a mistake
			// in the command line, not something that went wrong during a write.
			if refusal := requireFlagText(cmd, flagNote, note,
				"a checkpoint needs a note",
				"Re-run with --note saying where the work stands."); refusal != nil {
				return refuseBeforeStarting(cmd, "checkpoint write", o, refusal)
			}

			return runCoordination(cmd, "checkpoint write", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					noted, write, err := s.store.WriteCheckpoint(
						ctx, args[0], s.attribution(handle), s.space.ID, note, handoff)
					if err != nil {
						return nil, nil, err
					}
					result := checkpointResult{
						Checkpoint:    noted.Checkpoint,
						Session:       write.Session,
						SessionMinted: write.Minted,
					}
					return result, result.RenderHuman, nil
				})
		},
	}
	cmd.Flags().String(flagNote, "", "where the work stands, for whoever picks it up")
	cmd.Flags().Bool(flagHandoff, false, "mark this as the note left on the way out")
	cmd.Flags().String(flagSession, "", "the session id this run is attributed to; one is minted if omitted")
	return cmd
}

// checkpointResult is what `checkpoint write` publishes.
type checkpointResult struct {
	Checkpoint    coordination.Checkpoint `json:"checkpoint"`
	Session       coordination.Session    `json:"session"`
	SessionMinted bool                    `json:"session_minted"`
}

func (r checkpointResult) RenderHuman(w io.Writer, _ bool) error {
	marker := ""
	if r.Checkpoint.Handoff {
		marker = " as a handoff"
	}
	if _, err := fmt.Fprintf(w, "Checkpoint %s written on task %s%s.\n  %s\n",
		r.Checkpoint.ID, r.Checkpoint.TaskID, marker, r.Checkpoint.Note); err != nil {
		return err
	}
	if r.SessionMinted {
		_, err := fmt.Fprintf(w,
			"\nNo --session was given, so session %s was opened for this run.\nPass --session %s to keep later commands in it.\n",
			r.Session.ID, r.Session.ID)
		return err
	}
	return nil
}

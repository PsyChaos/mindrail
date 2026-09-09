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

			return runCoordination(cmd, "checkpoint write", o,
				func(ctx context.Context, s scope) (any, humanRenderer, error) {
					session, minted, err := s.resolveSession(ctx, handle)
					if err != nil {
						return nil, nil, err
					}
					checkpoint, err := s.store.WriteCheckpoint(ctx, args[0], session.ID, s.space.ID, note, handoff)
					if err != nil {
						return nil, nil, err
					}
					result := checkpointResult{
						Checkpoint:    checkpoint,
						Session:       session,
						SessionMinted: minted,
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

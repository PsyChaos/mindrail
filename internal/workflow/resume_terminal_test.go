package workflow_test

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/coordination"
	"github.com/PsyChaos/mindrail/internal/workflow"
)

// Terminal replay belongs to the run that completed the work. A new key must
// not adopt that task, even after restarting the workflow service (BRK-001).
func TestNewRunCannotResumeTerminalTask(t *testing.T) {
	for _, terminal := range []coordination.State{coordination.StateCompleted, coordination.StateAbandoned} {
		for _, restart := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/restart=%v", terminal, restart), func(t *testing.T) {
				f := newFixture(t)
				run := start(t, f, "original-run", "a.txt")
				if terminal == coordination.StateCompleted {
					write(t, f.root, "a.txt", "completed work\n")
					out, err := f.service.Finalize(t.Context(), workflow.FinalizeInput{RunKey: run.RunKey})
					if err != nil || !out.Completed {
						t.Fatalf("control completion: %+v %v", out, err)
					}
				} else if _, _, err := f.options.Coordination.TransitionExpecting(t.Context(), run.TaskID,
					coordination.NamedSession(run.SessionID), terminal, "abandon work", run.Revision); err != nil {
					t.Fatal(err)
				}
				if restart {
					service, err := workflow.New(f.options)
					if err != nil {
						t.Fatal(err)
					}
					f.service = service
				}
				if terminal == coordination.StateCompleted {
					replay := start(t, f, run.RunKey, "a.txt")
					if replay.TaskID != run.TaskID || replay.SessionID != run.SessionID || replay.State != terminal {
						t.Fatalf("same-key completed replay changed identity: %+v", replay)
					}
				}
				before, err := f.options.Coordination.FindTask(t.Context(), run.TaskID)
				if err != nil {
					t.Fatal(err)
				}
				counts := map[string]int{}
				for _, table := range []string{"sessions", "tasks", "leases", "operations"} {
					var count int
					if err := f.options.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&count); err != nil {
						t.Fatal(err)
					}
					counts[table] = count
				}
				adopted, err := f.service.Start(t.Context(), workflow.StartInput{
					Goal: "Adopt terminal work", RunKey: "different-run", ResumeTaskID: run.TaskID, Paths: []string{"a.txt"},
				})
				if payload, ok := app.PayloadOf(err); !ok || payload.Code != app.CodeCommandLineInvalid {
					t.Fatalf("new run did not refuse terminal resume: run=%+v err=%v", adopted, err)
				}
				if adopted.SessionID != "" || adopted.TaskID != "" {
					t.Fatalf("terminal refusal created a partial run: %+v", adopted)
				}
				after, err := f.options.Coordination.FindTask(t.Context(), run.TaskID)
				if err != nil || !reflect.DeepEqual(after, before) {
					t.Fatalf("terminal task changed on refusal: before=%+v after=%+v err=%v", before, after, err)
				}
				for table, beforeCount := range counts {
					var afterCount int
					if err := f.options.DB.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM "+table).Scan(&afterCount); err != nil {
						t.Fatal(err)
					}
					if afterCount != beforeCount {
						t.Errorf("terminal refusal wrote %s: %d -> %d", table, beforeCount, afterCount)
					}
				}
			})
		}
	}
}

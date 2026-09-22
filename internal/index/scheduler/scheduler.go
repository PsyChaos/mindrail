// Package scheduler orders structural index work without owning its durable
// state. The queue's durable state is the file_index_state table itself
// (decision D-84): this package holds only an in-memory scheduling window that
// is refilled from the store, so a process that dies mid-index loses only the
// in-flight file.
//
// One parse worker drains the queue front to back. Priority applies to queue
// order only: work is never preempted mid-parse (tech-stack §48). The worker
// checks cancellation between files, never inside one — IndexFile leaves a
// cancelled attempt as a pending target hash for resume.
package scheduler

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/index/inventory"
	"github.com/PsyChaos/mindrail/internal/index/parser"
)

// Priority is a scheduling class. Only P0 and P4 have machinery behind them
// (decision D-84): P1–P3 exist as named constants with their tech-stack §47
// meanings so a later milestone inherits the ladder, not a renumbering. An
// empty scheduling ladder is honest documentation; a fake scheduler is a
// defect.
type Priority int

const (
	// P0Targeted is a request aimed at an unindexed unit's file: it reorders
	// that unit's files ahead of the cold remainder.
	P0Targeted Priority = 0
	// P1DependencyClosure is the relevant dependency/test closure of a change.
	// No machinery in 0.1.
	P1DependencyClosure Priority = 1
	// P2ActiveTaskUnit is the active task's ProjectUnit. No machinery in 0.1.
	P2ActiveTaskUnit Priority = 2
	// P3ActiveWorkspace is the active workspace. No machinery in 0.1.
	P3ActiveWorkspace Priority = 3
	// P4ColdRemainder is the cold remainder in deterministic path order.
	P4ColdRemainder Priority = 4
)

// String names the class for logs and test failures.
func (p Priority) String() string {
	switch p {
	case P0Targeted:
		return "P0-targeted"
	case P1DependencyClosure:
		return "P1-dependency-closure"
	case P2ActiveTaskUnit:
		return "P2-active-task-unit"
	case P3ActiveWorkspace:
		return "P3-active-workspace"
	case P4ColdRemainder:
		return "P4-cold-remainder"
	default:
		return fmt.Sprintf("P%d-unknown", int(p))
	}
}

// MaxQueueJobs bounds the in-memory window. The durable resume set is
// unbounded — file_index_state holds every pending file — and FillCold refills
// from it, so a large repository is drained in windows, never held whole in
// memory (tech-stack §49: no unbounded memory queues).
const MaxQueueJobs = 1024

// ErrQueueFull reports a manual enqueue past the in-memory bound. FillCold
// never returns it: it stops at the bound and reports more work outstanding.
var ErrQueueFull = fmt.Errorf("scheduler: in-memory queue is full")

// Job is one file waiting for IndexFile.
type Job struct {
	Unit     index.ProjectUnit
	Path     string
	Priority Priority
}

// Scheduler orders index work. The zero value is unusable; construct with New.
// It is safe for concurrent Enqueue/Prioritize calls, but Run is
// single-worker: concurrent Run calls would parse the same files twice.
type Scheduler struct {
	mu      sync.Mutex
	store   *index.Store
	indexer *index.Indexer
	maxJobs int
	jobs    []Job
	queued  map[string]int
}

// New builds a scheduler over the durable store with the given in-memory
// bound. A non-positive bound selects MaxQueueJobs.
func New(store *index.Store, indexer *index.Indexer, maxJobs int) (*Scheduler, error) {
	if store == nil || indexer == nil {
		return nil, fmt.Errorf("scheduler: needs a store and an indexer")
	}
	if maxJobs <= 0 {
		maxJobs = MaxQueueJobs
	}
	return &Scheduler{store: store, indexer: indexer, maxJobs: maxJobs, queued: map[string]int{}}, nil
}

// Len reports the in-memory queue depth. Tests assert bound and coalescing
// through it; production refills from the durable resume set.
func (s *Scheduler) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

// Paths reports the queued paths in drain order, for order assertions.
func (s *Scheduler) Paths() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	paths := make([]string, len(s.jobs))
	for i, job := range s.jobs {
		paths[i] = job.Path
	}
	return paths
}

// Enqueue adds one file, coalescing duplicates by path: a repository of N
// pending files never holds N queued jobs for one path. A P0 re-enqueue of an
// already-queued path promotes it to the front; any other duplicate keeps its
// position. It reports whether the path is newly queued. Past the bound it
// returns ErrQueueFull and queues nothing.
func (s *Scheduler) Enqueue(unit index.ProjectUnit, path string, priority Priority) (bool, error) {
	if unit.ID == "" || path == "" {
		return false, fmt.Errorf("scheduler: enqueue needs a unit and a path")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if at, ok := s.queued[path]; ok {
		if priority == P0Targeted && s.jobs[at].Priority != P0Targeted {
			job := s.jobs[at]
			job.Priority = P0Targeted
			copy(s.jobs[1:at+1], s.jobs[0:at])
			s.jobs[0] = job
			s.reindex()
		}
		return false, nil
	}
	if len(s.jobs) >= s.maxJobs {
		return false, ErrQueueFull
	}
	s.jobs = append(s.jobs, Job{Unit: unit, Path: path, Priority: priority})
	s.queued[path] = len(s.jobs) - 1
	return true, nil
}

func (s *Scheduler) reindex() {
	for i, job := range s.jobs {
		s.queued[job.Path] = i
	}
}

// FillCold enqueues the durable resume set in deterministic path order at P4,
// up to the in-memory bound. It returns how many it queued and whether the
// store holds more: the remainder stays durable and a later FillCold picks it
// up. Unknown-unit rows fail the fill rather than scheduling work no unit
// owns.
func (s *Scheduler) FillCold(ctx context.Context, units []index.ProjectUnit) (enqueued int, more bool, err error) {
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	byID := make(map[string]index.ProjectUnit, len(units))
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	pending, err := s.store.ListPending(ctx, "")
	if err != nil {
		return 0, false, err
	}
	for _, row := range pending {
		if err := ctx.Err(); err != nil {
			return enqueued, true, err
		}
		unit, ok := byID[row.UnitID]
		if !ok {
			return enqueued, true, fmt.Errorf("scheduler: pending file %s names unknown unit %s", row.Path, row.UnitID)
		}
		s.mu.Lock()
		full := len(s.jobs) >= s.maxJobs
		s.mu.Unlock()
		if full {
			return enqueued, true, nil
		}
		added, err := s.Enqueue(unit, row.Path, P4ColdRemainder)
		if err != nil {
			return enqueued, true, err
		}
		if added {
			enqueued++
		}
	}
	return enqueued, false, nil
}

// Prioritize moves an unindexed unit's queued files ahead of the cold
// remainder in path order, at P0. Files of the unit that are pending in the
// store but not yet queued are enqueued first (up to the bound); the in-flight
// parse, if any, is never interrupted — reordering touches queue order only.
// It returns how many of the unit's files now head the queue.
func (s *Scheduler) Prioritize(ctx context.Context, units []index.ProjectUnit, unitID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if unitID == "" {
		return 0, fmt.Errorf("scheduler: prioritize needs a unit ID")
	}
	byID := make(map[string]index.ProjectUnit, len(units))
	for _, unit := range units {
		byID[unit.ID] = unit
	}
	if _, ok := byID[unitID]; !ok {
		return 0, fmt.Errorf("scheduler: unknown unit %s", unitID)
	}
	pending, err := s.store.ListPending(ctx, unitID)
	if err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	wanted := make(map[string]index.ProjectUnit, len(pending))
	for _, row := range pending {
		wanted[row.Path] = byID[unitID]
	}
	var head, tail []Job
	for _, job := range s.jobs {
		if _, ok := wanted[job.Path]; ok {
			job.Priority = P0Targeted
			head = append(head, job)
			delete(wanted, job.Path)
		} else {
			tail = append(tail, job)
		}
	}
	var missing []string
	for path := range wanted {
		missing = append(missing, path)
	}
	sort.Strings(missing)
	for _, path := range missing {
		if len(head)+len(tail) >= s.maxJobs {
			break
		}
		head = append(head, Job{Unit: byID[unitID], Path: path, Priority: P0Targeted})
	}
	sort.SliceStable(head, func(i, j int) bool { return head[i].Path < head[j].Path })
	s.jobs = append(head, tail...)
	s.reindex()
	return len(head), nil
}

// Register records walked files as durable index work: a file whose language
// has an adapter becomes pending, one without becomes terminally unsupported
// — never pending, so it can never sit in the resume set erroring on every
// attempt (the F3 drain failure). Registration hashes nothing; the indexer
// hashes at parse time.
func (s *Scheduler) Register(ctx context.Context, registry *parser.Registry, files []inventory.File) (registered, unsupported int, err error) {
	if registry == nil {
		return 0, 0, fmt.Errorf("scheduler: register needs a parser registry")
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return registered, unsupported, err
		}
		adapter, ok := registry.Lookup(file.Path)
		if !ok {
			if err := s.store.UpsertFileState(ctx, index.FileIndexState{
				UnitID: file.Unit.ID, Path: file.Path, Language: "", State: index.StateUnsupported,
			}); err != nil {
				return registered, unsupported, err
			}
			unsupported++
			continue
		}
		if err := s.store.UpsertFileState(ctx, index.FileIndexState{
			UnitID: file.Unit.ID, Path: file.Path, Language: adapter.Info().Language, State: index.StatePending,
		}); err != nil {
			return registered, unsupported, err
		}
		registered++
	}
	return registered, unsupported, nil
}

// Run drains the in-memory queue front to back through the indexer and returns
// how many files completed. Cancellation is honored between files; the first
// per-file error stops the run with what completed so far — a failed file is
// never silently skipped, and the remainder stays queued and durable. A file
// aborted by cancellation leaves the in-memory window but stays
// durable-pending: refill with FillCold to resume it.
func (s *Scheduler) Run(ctx context.Context) (int, error) {
	completed := 0
	for {
		s.mu.Lock()
		if len(s.jobs) == 0 {
			s.mu.Unlock()
			return completed, nil
		}
		if err := ctx.Err(); err != nil {
			s.mu.Unlock()
			return completed, err
		}
		job := s.jobs[0]
		copy(s.jobs, s.jobs[1:])
		s.jobs = s.jobs[:len(s.jobs)-1]
		delete(s.queued, job.Path)
		s.reindex()
		s.mu.Unlock()
		if _, err := s.indexer.IndexFile(ctx, job.Unit, job.Path); err != nil {
			return completed, err
		}
		completed++
	}
}

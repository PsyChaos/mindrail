package workflow

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/PsyChaos/mindrail/internal/coordination"
)

// Renew checks every expected lease before renewing by lease ID. Missing or
// expired ownership is a failure, never an implicit takeover or task revision.
func (s *Service) Renew(ctx context.Context, key string) error {
	if err := s.health(ctx, key); err != nil {
		return err
	}
	r, err := s.Resolve(ctx, key)
	if err != nil {
		return err
	}
	if r.State == coordination.StateCompleted {
		return nil
	}
	if r.State.Terminal() {
		return invalid("automatic task is terminal")
	}
	leases, err := s.options.Coordination.ListLeases(ctx, s.options.ProjectID)
	if err != nil {
		return s.failRenew(ctx, key, err)
	}
	expected := map[coordination.Target]bool{coordination.TaskTarget(r.TaskID): true}
	for _, p := range r.Paths {
		rel, err := filepath.Rel(s.options.Root, p)
		if err != nil {
			return err
		}
		target, err := coordination.FileTarget(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		expected[target] = true
	}
	owned := map[coordination.Target]coordination.Lease{}
	for _, l := range leases {
		if l.Holder == r.SessionID {
			owned[l.Target()] = l
		}
	}
	for target := range expected {
		if _, ok := owned[target]; !ok {
			return s.failRenew(ctx, key, invalid("automatic run lost lease for "+target.String()))
		}
	}
	// Stable ordering keeps lock acquisition and error selection reproducible.
	for _, l := range leases {
		if l.Holder != r.SessionID || !expected[l.Target()] {
			continue
		}
		if _, _, err := s.options.Coordination.RenewLease(ctx, l.ID, coordination.NamedSession(r.SessionID)); err != nil {
			return s.failRenew(ctx, key, err)
		}
	}
	return nil
}

func (s *Service) failRenew(ctx context.Context, key string, err error) error {
	if ctx.Err() != nil {
		return err
	}
	// Completion releases leases atomically after our earlier active-state
	// read. That planned release is not ownership loss and must never poison
	// the durable completed run or its lost-response retries.
	current, resolveErr := s.Resolve(ctx, key)
	if resolveErr != nil {
		return fmt.Errorf("lease renewal failed: %w; checking current run: %v", err, resolveErr)
	}
	if current.State.Terminal() {
		return nil
	}
	if recordErr := s.recordFailure(ctx, key, err); recordErr != nil {
		return fmt.Errorf("lease renewal failed: %w; recording failure: %v", err, recordErr)
	}
	return err
}

type Heartbeat struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	err    error
}

func (s *Service) StartHeartbeat(ctx context.Context, key string, interval time.Duration) (*Heartbeat, error) {
	if interval == 0 {
		interval = coordination.LeaseTTL / 3
	}
	if interval <= 0 || interval >= coordination.LeaseTTL {
		return nil, invalid("heartbeat interval must be positive and shorter than the lease TTL")
	}
	if err := s.Renew(ctx, key); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	h := &Heartbeat{cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(h.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := s.Renew(ctx, key); err != nil {
					if ctx.Err() == nil {
						h.mu.Lock()
						h.err = err
						h.mu.Unlock()
					}
					return
				}
				// A successful no-op renewal after finalization also ends this
				// worker; callers need not race to Stop before leases disappear.
				if run, err := s.Resolve(ctx, key); err == nil && run.State.Terminal() {
					return
				}
			}
		}
	}()
	return h, nil
}

// Stop waits until no renewal is in flight, so callers can then close the DB.
func (h *Heartbeat) Stop() {
	if h == nil {
		return
	}
	h.cancel()
	<-h.done
}
func (h *Heartbeat) Health() error {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.err
}

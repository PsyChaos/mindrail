// Package symbol owns durable symbol identity orchestration: allocation,
// rename/move migration matching, target resolution and binding refresh.
//
// It holds policy and order; the SQL lives in internal/index beside the
// migration that creates it (decision D-93). It reads repository-owned
// knowledge but never writes .mindrail/ (the schema stays v1, decision
// D-98). Matching is STRUCTURAL only — body, kind, container, path and Git
// corroboration — never semantic (decision D-106).
package symbol

import (
	"context"
	"fmt"

	"github.com/PsyChaos/mindrail/internal/index"
)

// Service orchestrates identity over an index Store.
type Service struct {
	store *index.Store
}

// New builds a Service over the store that holds the identities.
func New(store *index.Store) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("symbol: needs an index store")
	}
	return &Service{store: store}, nil
}

// EnsureIdentity returns the lineage uid for an allocation key, minting it on
// first sight (spec §24 insert-then-read-back; decision D-94). The migration
// attempt between the lookup miss and the mint arrives in TASK-04; this task
// only looks up or mints, so a genuinely new key always gets exactly one uid
// no matter how many processes race it.
func (s *Service) EnsureIdentity(ctx context.Context, projectID string, unit index.ProjectUnit, language, key string) (string, error) {
	if projectID == "" || unit.ID == "" || language == "" || key == "" {
		return "", fmt.Errorf("symbol: identity needs a project, unit, language and key")
	}
	if existing, found, err := s.store.LookupIdentity(ctx, projectID, unit.ID, language, key); err != nil {
		return "", err
	} else if found {
		return existing.UID, nil
	}
	minted, err := s.store.MintIdentity(ctx, projectID, unit.ID, language, key)
	if err != nil {
		return "", err
	}
	return minted.UID, nil
}

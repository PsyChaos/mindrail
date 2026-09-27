package validation

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"sort"
)

// Freshness statuses. Current means the scope re-hashes equal; anything
// else — changed, added, removed, unreadable, unparseable — is stale with
// the reason naming the gap (decision D-166).
const (
	FreshCurrent = "current"
	FreshStale   = "stale"
)

// Verdict is one evidence row's usability: scope freshness, command success,
// complete-run membership, the stored/recomputed hashes, and a plain reason.
// Verdicts are computed on every check and stored nowhere (decision D-165).
type Verdict struct {
	EvidenceID   string
	Profile      string
	Status       string
	Reason       string
	SnapshotHash string
	CurrentHash  string
}

// ReRunItem is one profile to run again with its reason. One per profile,
// first reason wins, sorted by profile.
type ReRunItem struct {
	Profile string
	Reason  string
}

// Coverage answers the required-evidence question: which required profiles'
// latest runs are complete, wholly successful, and current, and what to re-run.
// The re-run list unions stale rows' profiles with uncovered required profiles
// (decision D-167).
type Coverage struct {
	Required  []string
	Satisfied map[string]bool
	ReRun     []ReRunItem
}

// Check evaluates freshness and complete-run success for stored rows against
// the live tree under root, then required coverage. Rows arrive as plain
// values; no database is touched, so read-only is structural. Declarative
// scope paths come from provenance and are re-enumerated through SnapshotScope;
// malformed/legacy provenance, incomplete runs, failed commands, scope escapes
// and unreadable files all fail safe to stale.
func Check(root string, rows []Evidence, required []string) (verdicts []Verdict, coverage Coverage, err error) {
	root = filepath.Clean(root)
	if root == "" || !filepath.IsAbs(root) {
		return nil, Coverage{}, invalidInput("freshness check needs an absolute root")
	}
	coverage = Coverage{Satisfied: map[string]bool{}}
	rerun := map[string]string{}
	remember := func(profile, reason string) {
		if _, ok := rerun[profile]; !ok {
			rerun[profile] = reason
		}
	}
	type runGroup struct {
		expected      int
		scopePaths    []string
		snapshotHash  string
		positions     []int
		commandSeen   map[int]bool
		commandRows   map[int]string
		metadataValid bool
		recency       string
	}
	groups := map[string]*runGroup{}
	latest := map[string]*runGroup{}
	for _, row := range rows {
		verdict := Verdict{EvidenceID: row.ID, Profile: row.Profile, SnapshotHash: row.SnapshotHash}
		provenance, ok := readProvenance(row)
		if !ok {
			verdict.Status = FreshStale
			verdict.Reason = "provenance cannot prove run completeness and scope membership"
		} else if current, hashErr := resnapshotScope(root, provenance.ScopePaths); hashErr != "" {
			verdict.Status = FreshStale
			verdict.Reason = hashErr
			verdict.CurrentHash = current
		} else {
			verdict.CurrentHash = current
			if row.Status != StatusPass || row.ExitCode != 0 {
				verdict.Status = FreshStale
				verdict.Reason = "validation command did not pass: " + row.Status
			} else if current == row.SnapshotHash {
				verdict.Status = FreshCurrent
				verdict.Reason = "declared scope re-hashes equal"
			} else {
				verdict.Status = FreshStale
				verdict.Reason = "scope content differs from snapshot " + shortHash(row.SnapshotHash)
			}
		}
		verdicts = append(verdicts, verdict)
		position := len(verdicts) - 1
		groupKey := row.Profile + "\x00invalid\x00" + row.ID
		group := &runGroup{positions: []int{position}, metadataValid: false, recency: row.ID}
		if ok {
			groupKey = row.Profile + "\x00" + provenance.RunID
			group = groups[groupKey]
			if group == nil {
				group = &runGroup{
					expected:   provenance.CommandCount,
					scopePaths: append([]string(nil), provenance.ScopePaths...), snapshotHash: row.SnapshotHash,
					commandSeen: map[int]bool{}, commandRows: map[int]string{}, metadataValid: true,
				}
			}
			if group.expected != provenance.CommandCount || group.snapshotHash != row.SnapshotHash ||
				!slices.Equal(group.scopePaths, provenance.ScopePaths) ||
				(group.commandSeen[provenance.CommandIndex] && group.commandRows[provenance.CommandIndex] != row.ID) {
				group.metadataValid = false
			}
			group.commandSeen[provenance.CommandIndex] = true
			group.commandRows[provenance.CommandIndex] = row.ID
			group.positions = append(group.positions, position)
			if row.ID > group.recency {
				group.recency = row.ID
			}
		}
		groups[groupKey] = group
		if current := latest[row.Profile]; current == nil || group.recency > current.recency {
			latest[row.Profile] = group
		}
		if verdict.Status == FreshStale {
			remember(row.Profile, verdict.Reason)
		}
	}
	currentByProfile := map[string]bool{}
	for profile, group := range latest {
		complete := group.metadataValid && len(group.commandSeen) == group.expected
		for commandIndex := 0; complete && commandIndex < group.expected; commandIndex++ {
			complete = group.commandSeen[commandIndex]
		}
		for _, position := range group.positions {
			complete = complete && verdicts[position].Status == FreshCurrent
		}
		if complete {
			currentByProfile[profile] = true
			continue
		}
		for _, position := range group.positions {
			if verdicts[position].Status == FreshCurrent {
				verdicts[position].Status = FreshStale
				verdicts[position].Reason = "validation run is incomplete"
				remember(profile, verdicts[position].Reason)
			}
		}
	}
	for _, profile := range required {
		if currentByProfile[profile] {
			coverage.Satisfied[profile] = true
			continue
		}
		coverage.Satisfied[profile] = false
		remember(profile, "no current evidence")
	}
	coverage.Required = required
	var profiles []string
	for profile := range rerun {
		profiles = append(profiles, profile)
	}
	sort.Strings(profiles)
	for _, profile := range profiles {
		coverage.ReRun = append(coverage.ReRun, ReRunItem{Profile: profile, Reason: rerun[profile]})
	}
	return verdicts, coverage, nil
}

type evidenceProvenance struct {
	Profile      string   `json:"profile"`
	RunID        string   `json:"run_id"`
	CommandIndex int      `json:"command_index"`
	CommandCount int      `json:"command_count"`
	ScopePaths   []string `json:"scope_paths"`
	Scope        []string `json:"scope"`
}

// readProvenance requires both dimensions needed to trust an evidence row:
// complete-run grouping and the declarative scope that can be re-enumerated.
// Legacy rows do not carry them and therefore fail closed.
func readProvenance(row Evidence) (evidenceProvenance, bool) {
	var decoded evidenceProvenance
	if err := json.Unmarshal([]byte(row.Provenance), &decoded); err != nil {
		return evidenceProvenance{}, false
	}
	if decoded.Profile == "" || decoded.Profile != row.Profile || decoded.RunID == "" ||
		decoded.CommandCount <= 0 || decoded.CommandIndex < 0 || decoded.CommandIndex >= decoded.CommandCount ||
		len(decoded.ScopePaths) == 0 || decoded.Scope == nil {
		return evidenceProvenance{}, false
	}
	return decoded, true
}

// resnapshotScope re-enumerates the declaration rather than trusting the
// historical concrete file list. This is what makes additions detectable.
func resnapshotScope(root string, scopePaths []string) (hash, reason string) {
	snapshot, err := SnapshotScope(root, scopePaths)
	if err != nil {
		return "", "declared scope unreadable"
	}
	return snapshot.Hash, ""
}

// shortHash names a hash without printing all 64 hexits into a reason.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

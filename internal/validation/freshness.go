package validation

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Freshness statuses. Current means the scope re-hashes equal; anything
// else — changed, added, removed, unreadable, unparseable — is stale with
// the reason naming the gap (decision D-166).
const (
	FreshCurrent = "current"
	FreshStale   = "stale"
)

// Verdict is one evidence row's freshness: the stored hash, the recomputed
// hash (empty when the scope could not hash), and the reason in plain
// words. Verdicts are computed on every check and stored nowhere
// (decision D-165).
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

// Coverage answers the required-evidence question: which required profiles
// hold ≥1 current row, and what to re-run. The re-run list unions stale
// rows' profiles with uncovered required profiles (decision D-167).
type Coverage struct {
	Required  []string
	Satisfied map[string]bool
	ReRun     []ReRunItem
}

// Check evaluates freshness for stored rows against the live tree under
// root, then required coverage. Rows arrive as plain values; no database
// is touched, so read-only is structural — there is no write path to
// misuse. Scope comes from each row's provenance JSON, hashed with the
// same core as SnapshotScope; scope escaping root, missing or unreadable
// files, and unparseable provenance all fail safe to stale.
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
	for _, row := range rows {
		verdict := Verdict{EvidenceID: row.ID, Profile: row.Profile, SnapshotHash: row.SnapshotHash}
		scope, ok := provenanceScope(row.Provenance)
		if !ok {
			verdict.Status = FreshStale
			verdict.Reason = "provenance scope unreadable"
		} else if current, hashErr := rehashScope(root, scope); hashErr != "" {
			verdict.Status = FreshStale
			verdict.Reason = hashErr
			verdict.CurrentHash = current
		} else {
			verdict.CurrentHash = current
			if current == row.SnapshotHash {
				verdict.Status = FreshCurrent
				verdict.Reason = "scope re-hashes equal over " + strconv.Itoa(len(scope)) + " files"
			} else {
				verdict.Status = FreshStale
				verdict.Reason = "scope content differs from snapshot " + shortHash(row.SnapshotHash)
			}
		}
		verdicts = append(verdicts, verdict)
		if verdict.Status == FreshStale {
			remember(row.Profile, verdict.Reason)
		}
	}
	currentByProfile := map[string]bool{}
	for _, verdict := range verdicts {
		if verdict.Status == FreshCurrent {
			currentByProfile[verdict.Profile] = true
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

// provenanceScope reads the scope array Record stored. Unknown keys are
// ignored; scope must be a string array.
func provenanceScope(provenance string) ([]string, bool) {
	var decoded struct {
		Scope []string `json:"scope"`
	}
	if err := json.Unmarshal([]byte(provenance), &decoded); err != nil {
		return nil, false
	}
	if decoded.Scope == nil {
		return nil, false
	}
	return decoded.Scope, true
}

// rehashScope re-hashes stored absolute scope paths under root with the
// SnapshotScope core. It returns the hash and the stale reason (empty when
// hashable). Escapes, missing files and unreadable content fail safe:
// partial reads never report current. Duplicated paths dedupe here exactly
// as SnapshotScope dedupes at record time.
func rehashScope(root string, scope []string) (hash, reason string) {
	var files []string
	seen := map[string]bool{}
	for _, path := range scope {
		clean := filepath.Clean(path)
		if clean != root && !strings.HasPrefix(clean, root+string(filepath.Separator)) {
			return "", "scope escapes the root: " + path
		}
		info, err := os.Lstat(clean)
		if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return "", "scope unreadable: " + path
		}
		if !seen[clean] {
			seen[clean] = true
			files = append(files, clean)
		}
	}
	sort.Strings(files)
	hash, err := hashFiles(root, files)
	if err != nil {
		return "", "scope unreadable: " + err.Error()
	}
	// An empty scope hashes to the sha256 of nothing: equal only with a row
	// recorded empty, so malformed rows (empty or garbage stored hash)
	// still stale while legitimately empty scopes stay current. The
	// equality in Check decides; no special case needed.
	return hash, ""
}

// shortHash names a hash without printing all 64 hexits into a reason.
func shortHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}

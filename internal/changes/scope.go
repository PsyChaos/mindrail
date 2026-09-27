package changes

import (
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

// Finding is one attribution answer that is not quiet: a drifted file, an
// unregistered symbol, or an ambiguous one. Findings travel as structs with
// reason codes; rendering belongs to MR-013 and later. Blocking answers
// whether completion must stop on it — always true in 0.1 (decision D-138),
// with resolution paths named in NextAction.
type Finding struct {
	Code       app.Code
	Detail     string
	Provenance Provenance
	NextAction []string
	Blocking   bool
}

// Provenance is what the evaluation read to produce a finding: the task it
// evaluated, the baseline scope it read, and the discovery rows it judged —
// never invented paths (AC-02.5).
type Provenance struct {
	TaskID        string
	BaselineScope []string
	ChangeID      string
	ChangeKey     string
	Via           string
}

// ScopeDrift names a changed file outside the task's declared scope.
func ScopeDrift(taskID, changeID, path, kind, reason string, baselineScope []string) Finding {
	return Finding{
		Code:       app.CodeScopeDrift,
		Provenance: Provenance{TaskID: taskID, BaselineScope: baselineScope, ChangeID: changeID, ChangeKey: path, Via: ""},
		NextAction: []string{"Extend the task baseline with `before_change` over " + path + ", or move the edit into a task whose scope covers it."},
		Blocking:   true,
		Detail:     "file " + path + " changed outside baseline scope (" + kind + "): " + reason,
	}
}

// UnregisteredFile names a staged path no active task scope owns. File-level
// attribution is required even when the language is unsupported and no symbol
// rows can exist.
func UnregisteredFile(changeID, path, kind string) Finding {
	return Finding{
		Code:       app.CodeUnregisteredChange,
		Provenance: Provenance{ChangeID: changeID, ChangeKey: path, Via: ViaReconcile},
		NextAction: []string{"Declare the concrete file with `before_change` over " + path + "."},
		Blocking:   true,
		Detail:     "file " + path + " (" + kind + ") matches no active task scope",
	}
}

// AmbiguousFile names a staged path claimed by multiple active task scopes.
// No task is selected implicitly.
func AmbiguousFile(changeID, path string, taskIDs []string) Finding {
	return Finding{
		Code:       app.CodeReconcileAmbiguous,
		Provenance: Provenance{ChangeID: changeID, ChangeKey: path, Via: ViaReconcile},
		NextAction: []string{"Keep " + path + " in exactly one active task scope before committing."},
		Blocking:   true,
		Detail:     "file " + path + " matches several active task scopes: " + strings.Join(taskIDs, ", "),
	}
}

// UnregisteredChange names a symbol no open change claims.
func UnregisteredChange(taskID, changeID, key, via string, baselineScope []string) Finding {
	return Finding{
		Code:       app.CodeUnregisteredChange,
		Provenance: Provenance{TaskID: taskID, BaselineScope: baselineScope, ChangeID: changeID, ChangeKey: key, Via: via},
		NextAction: []string{"Declare the scope with `before_change`, or record an explicit attribution for " + key + "."},
		Blocking:   true,
		Detail:     "symbol " + key + " matches no open change",
	}
}

// AmbiguousAttribution names a symbol two or more open changes claim. No
// candidate is chosen — the candidates are all named instead.
func AmbiguousAttribution(taskID, changeID, key string, candidates []string, baselineScope []string) Finding {
	return Finding{
		Code:       app.CodeReconcileAmbiguous,
		Provenance: Provenance{TaskID: taskID, BaselineScope: baselineScope, ChangeID: changeID, ChangeKey: key},
		NextAction: []string{"Assign " + key + " explicitly to one change, or narrow the overlapping baselines."},
		Blocking:   true,
		Detail:     "symbol " + key + " matches several open changes: " + strings.Join(candidates, ", "),
	}
}

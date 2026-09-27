// Package setup installs repository-local agent guidance and Git integration.
// It preserves user content and refuses ambiguous or unsafe managed targets.
package setup

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/app"
)

const Begin = "<!-- BEGIN MINDRAIL MANAGED SECTION -->"
const End = "<!-- END MINDRAIL MANAGED SECTION -->"
const hookBegin = "# mindrail:begin verify-staged"
const hookEnd = "# mindrail:end verify-staged"
const chainMarker = "# mindrail:chain v1"
const backupSuffix = ".mindrail-original"

const instructions = `## MINDRAIL PROTOCOL

Mindrail is the repository's local engineering gate. The MCP server command is
` + "`mindrail mcp`" + `, started in this repository. Use the existing 13 MCP tools.

At the start of a task, call mindrail_bootstrap with goal, an agent-generated
stable run_key, and paths when known. Keep the same run_key when retrying or
reconnecting. To continue an existing handoff, also pass resume_task_id explicitly.
Do not choose another agent's active task. Sessions, task revisions, operation
identities and leases are managed automatically; do not ask a person to copy them.

Read context through mindrail_status, mindrail_search and mindrail_context.
If bootstrap omitted paths, call mindrail_before_change with paths before the first edit.
Call it again before editing any file outside the declared scope. It may be skipped
only when bootstrap paths already cover every file being edited. Relative
repository file paths are accepted in automatic mode. The task and session may
be omitted when this connection has bootstrapped automatic work. Use
mindrail_after_change or mindrail_reconcile to inspect changes when useful.
Record durable choices through mindrail_decide and mindrail_invariant.

To finish, call mindrail_complete with finalize: true. It reconciles changes,
runs configured validation profiles, applies the shared completion gate, and
completes the task only when allowed. A denial is work to resolve, never success.
No configured profiles means no test profiles ran; do not claim tests passed.
For handoff, call mindrail_checkpoint with a note and handoff: true. The next
run uses a fresh run_key and explicitly resumes that task.

People normally run mindrail init once, and mindrail status, mindrail doctor or
mindrail verify when needed. CI runs mindrail verify --ci. Advanced commands and
explicit-ID MCP payloads remain available for diagnostics and legacy clients.
`

type Result struct {
	AgentsPath, HookPath       string
	AgentsChanged, HookChanged bool
}

type target struct {
	root      *os.Root
	rel, path string
	before    []byte
	mode      os.FileMode
	exists    bool
}

// Install preflights both managed targets before changing either one. HookPath
// must be Git's effective pre-commit path, confined to the worktree/common dir.
func Install(worktree, commonDir, hookPath string) (Result, error) {
	result := Result{AgentsPath: filepath.Join(worktree, "AGENTS.md"), HookPath: hookPath}
	work, err := os.OpenRoot(worktree)
	if err != nil {
		return result, refuse(worktree, err)
	}
	defer work.Close()
	common, err := os.OpenRoot(commonDir)
	if err != nil {
		return result, refuse(commonDir, err)
	}
	defer common.Close()
	agents, err := readTarget(work, "AGENTS.md", result.AgentsPath)
	if err != nil {
		return result, err
	}
	merged, err := mergeInstructions(agents.before)
	if err != nil {
		return result, refuse(result.AgentsPath, err)
	}
	hookRoot, hookRel, err := hookBoundary(work, common, worktree, commonDir, hookPath)
	if err != nil {
		return result, err
	}
	hook, err := readTarget(hookRoot, hookRel, hookPath)
	if err != nil {
		return result, err
	}
	backup, err := readTarget(hookRoot, hookRel+backupSuffix, hookPath+backupSuffix)
	if err != nil {
		return result, err
	}
	wrapper := hookWrapper()
	var foreign []byte
	if !bytes.Equal(hook.before, wrapper) {
		if bytes.Contains(hook.before, []byte(chainMarker)) {
			return result, refuse(hookPath, fmt.Errorf("managed hook wrapper was edited; preserve those edits before reinstalling"))
		}
		foreign, err = foreignHook(hook.before)
		if err != nil {
			return result, refuse(hookPath, err)
		}
		if len(foreign) > 0 && backup.exists && !bytes.Equal(foreign, backup.before) {
			return result, refuse(backup.path, fmt.Errorf("foreign-hook backup is already occupied by different content"))
		}
		if len(foreign) == 0 && backup.exists {
			return result, refuse(backup.path, fmt.Errorf("orphaned foreign-hook backup needs review before installation"))
		}
	}
	if len(foreign) > 0 && !backup.exists {
		if err := writeNew(backup, foreign, hook.mode); err != nil {
			return result, err
		}
	}
	if !bytes.Equal(hook.before, wrapper) || hook.mode.Perm()&0o111 == 0 {
		if err := replace(hook, wrapper, 0o755); err != nil {
			return result, err
		}
		result.HookChanged = true
	}
	if !bytes.Equal(agents.before, merged) {
		mode := agents.mode
		if !agents.exists {
			mode = 0o644
		}
		if err := replace(agents, merged, mode); err != nil {
			return result, err
		}
		result.AgentsChanged = true
	}
	return result, nil
}

func hookBoundary(work, common *os.Root, worktree, commonDir, path string) (*os.Root, string, error) {
	for _, candidate := range []struct {
		root *os.Root
		path string
	}{{common, commonDir}, {work, worktree}} {
		rel, err := filepath.Rel(candidate.path, path)
		if err != nil {
			continue
		}
		if rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return candidate.root, rel, nil
		}
	}
	return nil, "", app.NewError(app.CodePathEscapesRoot, app.KindFailed, "effective pre-commit hook is outside this repository: "+path, "Repository-local setup will not modify an external hooks directory.", "Choose a repository-local core.hooksPath, or install the Mindrail guard in the shared hooks directory yourself.").WithMetadata("path", path)
}

func readTarget(root *os.Root, rel, path string) (target, error) {
	t := target{root: root, rel: rel, path: path, mode: 0o644}
	// Reject links even within the boundary: a managed name must not overwrite
	// another user-owned file. os.Root enforces containment during every I/O.
	for p := rel; p != "."; p = filepath.Dir(p) {
		info, err := root.Lstat(p)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return t, refuse(path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return t, refuse(path, fmt.Errorf("managed path traverses a symbolic link"))
		}
		if p == rel {
			if !info.Mode().IsRegular() {
				return t, refuse(path, fmt.Errorf("managed target is not a regular file"))
			}
			t.mode = info.Mode().Perm()
			t.exists = true
		}
	}
	if t.exists {
		body, err := root.ReadFile(rel)
		if err != nil {
			return t, refuse(path, err)
		}
		t.before = body
	}
	return t, nil
}

func mergeInstructions(content []byte) ([]byte, error) {
	text := string(content)
	begins, ends := strings.Count(text, Begin), strings.Count(text, End)
	block := Begin + "\n" + instructions + End
	if begins == 0 && ends == 0 {
		separator := ""
		if text != "" {
			if !strings.HasSuffix(text, "\n") {
				separator = "\n"
			}
			separator += "\n"
		}
		return []byte(text + separator + block + "\n"), nil
	}
	if begins != 1 || ends != 1 {
		return nil, fmt.Errorf("AGENTS.md needs exactly one matching pair of Mindrail managed markers")
	}
	start, end := strings.Index(text, Begin), strings.Index(text, End)
	if end < start || !wholeLine(text, start, len(Begin)) || !wholeLine(text, end, len(End)) {
		return nil, fmt.Errorf("Mindrail managed markers must be ordered and occupy their own lines")
	}
	return []byte(text[:start] + block + text[end+len(End):]), nil
}

func wholeLine(text string, start, length int) bool {
	return (start == 0 || text[start-1] == '\n') && (start+length == len(text) || text[start+length] == '\n' || (text[start+length] == '\r' && start+length+1 < len(text) && text[start+length+1] == '\n'))
}

func hookWrapper() []byte {
	return []byte("#!/bin/sh\n" + chainMarker + "\nmindrail_original=\"$(dirname -- \"$0\")/pre-commit" + backupSuffix + "\"\nif [ -x \"$mindrail_original\" ]; then\n  \"$mindrail_original\" \"$@\" || exit $?\nfi\n" + hookBegin + "\nmindrail verify --staged\n" + hookEnd + "\n")
}

func foreignHook(content []byte) ([]byte, error) {
	text := string(content)
	if strings.Contains(text, hookBegin) || strings.Contains(text, hookEnd) {
		legacy := hookBegin + "\nmindrail verify --staged\n" + hookEnd + "\n"
		if strings.Count(text, hookBegin) != 1 || strings.Count(text, hookEnd) != 1 || !strings.Contains(text, legacy) {
			return nil, fmt.Errorf("existing Mindrail hook markers are malformed or modified")
		}
		text = strings.Replace(text, legacy, "", 1)
	}
	if text == "" || text == "#!/bin/sh\n" {
		return nil, nil
	}
	return []byte(text), nil
}

func writeNew(t target, data []byte, mode os.FileMode) error {
	if err := t.root.MkdirAll(filepath.Dir(t.rel), 0o755); err != nil {
		return refuse(t.path, err)
	}
	f, err := t.root.OpenFile(t.rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return refuse(t.path, err)
	}
	_, writeErr := f.Write(data)
	syncErr := f.Sync()
	closeErr := f.Close()
	for _, err := range []error{writeErr, syncErr, closeErr} {
		if err != nil {
			return refuse(t.path, err)
		}
	}
	return nil
}

func replace(t target, data []byte, mode os.FileMode) error {
	if err := t.root.MkdirAll(filepath.Dir(t.rel), 0o755); err != nil {
		return refuse(t.path, err)
	}
	temp := filepath.Join(filepath.Dir(t.rel), ".mindrail-setup-"+rand.Text())
	f, err := t.root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return refuse(t.path, err)
	}
	defer t.root.Remove(temp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return refuse(t.path, err)
	}
	if err := f.Close(); err != nil {
		return refuse(t.path, err)
	}
	current, err := readTarget(t.root, t.rel, t.path)
	if err != nil {
		return err
	}
	if current.exists != t.exists || !bytes.Equal(current.before, t.before) {
		return refuse(t.path, fmt.Errorf("target changed during setup; retry after reviewing the concurrent edit"))
	}
	if err := t.root.Rename(temp, t.rel); err != nil {
		return refuse(t.path, err)
	}
	return nil
}

func refuse(path string, cause error) error {
	return app.NewError(app.CodeConfigInvalid, app.KindFailed, "repository setup could not update "+path+": "+cause.Error(), "Setup is incomplete; existing user content was preserved.", "Resolve this target and rerun mindrail init.").WithMetadata("path", path).WithCause(cause)
}

func (r Result) RenderHuman(w io.Writer) error {
	_, err := fmt.Fprintf(w, "Agent instructions: %s\nPre-commit guard: %s\n", r.AgentsPath, r.HookPath)
	return err
}

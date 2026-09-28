package validation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	mindrailgit "github.com/PsyChaos/mindrail/internal/git"
)

// Snapshot is a content address over a profile scope: the sorted absolute
// scope plus the hex sha256 binding it. Evidence staples this hash; MR-011
// compares it to detect staleness.
type Snapshot struct {
	Scope []string
	Hash  string
}

// SnapshotScope hashes every source file selected by root-relative literal
// files, directories, or glob declarations. In a Git worktree, source means
// tracked files plus non-ignored untracked files; this excludes Git metadata
// and ignored runtime/build outputs while retaining new source files. Explicit
// literal files remain selectable even when ignored. Non-Git roots preserve
// filesystem enumeration. Symlinks are never followed, and content is
// addressed. Scope escapes and missing/unreadable literal paths fail before
// execution; an empty glob match remains a valid declaration whose later
// membership can go stale.
func SnapshotScope(root string, paths []string) (Snapshot, error) {
	root = filepath.Clean(root)
	if root == "" || !filepath.IsAbs(root) {
		return Snapshot{}, invalidInput("snapshot needs an absolute root")
	}
	if len(paths) == 0 {
		return Snapshot{}, invalidInput("snapshot needs a scope")
	}
	files, err := enumerateScope(root, paths)
	if err != nil {
		return Snapshot{}, err
	}
	hash, err := hashFiles(root, files)
	if err != nil {
		return Snapshot{}, invalidInput("snapshot scope unreadable: " + err.Error())
	}
	return Snapshot{Scope: files, Hash: hash}, nil
}

// enumerateScope resolves literal files/directories and root-relative glob
// declarations into one sorted regular-file set. A whole-segment ** matches
// zero or more directories, which is the recursive form used by validation
// profile configuration. Symlinks are never followed.
func enumerateScope(root string, paths []string) ([]string, error) {
	gitWorktree, err := isGitWorktree(root)
	if err != nil {
		return nil, invalidInput("snapshot scope unreadable")
	}
	if gitWorktree {
		return enumerateGitScope(root, paths)
	}
	return enumerateFilesystemScope(root, paths)
}

func isGitWorktree(root string) (bool, error) {
	_, err := os.Lstat(filepath.Join(root, ".git"))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, err
}

type scopeDeclaration struct {
	pattern     string
	literal     string
	directory   bool
	explicitAbs string
}

func enumerateGitScope(root string, paths []string) ([]string, error) {
	declarations := make([]scopeDeclaration, 0, len(paths))
	for _, rel := range paths {
		fromSlash := filepath.FromSlash(rel)
		cleanRel := filepath.Clean(fromSlash)
		if rel == "" || filepath.IsAbs(fromSlash) || cleanRel == ".." ||
			strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
			return nil, invalidInput("snapshot scope escapes the root: " + rel)
		}
		if strings.ContainsAny(rel, "*?[") {
			pattern := filepath.ToSlash(cleanRel)
			if _, err := matchScopeGlob(pattern, ""); err != nil {
				return nil, invalidInput("snapshot scope has an invalid glob: " + rel)
			}
			declarations = append(declarations, scopeDeclaration{pattern: pattern})
			continue
		}

		abs := filepath.Join(root, cleanRel)
		if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return nil, invalidInput("snapshot scope escapes the root: " + rel)
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
		declaration := scopeDeclaration{literal: filepath.ToSlash(cleanRel)}
		if info.Mode()&fs.ModeSymlink != 0 {
			declarations = append(declarations, declaration)
			continue
		}
		if info.IsDir() {
			declaration.directory = true
		} else if info.Mode().IsRegular() {
			declaration.explicitAbs = abs
		}
		declarations = append(declarations, declaration)
	}

	runner := mindrailgit.NewExecRunner()
	tracked, trackedStderr, err := runner.Run(context.Background(), root, "ls-files", "-z", "--cached", "--stage", "--")
	if err != nil || len(bytes.TrimSpace(trackedStderr)) != 0 {
		return nil, invalidInput("snapshot Git inventory unavailable")
	}
	untrackedArgs := []string{"ls-files", "-z", "--others", "--exclude-standard", "--"}
	untrackedArgs = append(untrackedArgs, gitInventoryPathspecs(declarations)...)
	untracked, untrackedStderr, err := runner.Run(context.Background(), root, untrackedArgs...)
	if err != nil || len(bytes.TrimSpace(untrackedStderr)) != 0 {
		return nil, invalidInput("snapshot Git inventory unavailable")
	}
	embeddedCandidates, _, err := runner.Run(
		context.Background(), root, "ls-files", "-z", "--others", "--exclude-standard", "--", ".",
	)
	if err != nil {
		return nil, invalidInput("snapshot Git inventory unavailable")
	}
	for _, record := range strings.Split(string(embeddedCandidates), "\x00") {
		if !strings.HasSuffix(filepath.ToSlash(record), "/") {
			continue
		}
		rel := strings.TrimSuffix(filepath.ToSlash(record), "/")
		if rel == "" {
			continue
		}
		selected, matchErr := selectedByScopeDeclarations(declarations, rel)
		if matchErr != nil {
			return nil, invalidInput("snapshot scope has an invalid glob")
		}
		if !selected && !scopeMaySelectDescendant(declarations, rel) {
			continue
		}
		directory := filepath.Join(root, filepath.FromSlash(rel))
		info, safe, statErr := lstatWithoutSymlinkParents(root, directory)
		if statErr != nil {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
		if !safe || !info.IsDir() {
			continue
		}
		marker := filepath.Join(directory, ".git")
		if _, markerErr := os.Lstat(marker); markerErr == nil {
			return nil, invalidInput("snapshot scope contains an unsupported embedded Git repository: " + rel)
		} else if !errors.Is(markerErr, fs.ErrNotExist) {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
	}
	type inventoryEntry struct {
		rel       string
		gitlink   bool
		untracked bool
	}
	entries := make([]inventoryEntry, 0)
	for _, record := range strings.Split(string(tracked), "\x00") {
		if record == "" {
			continue
		}
		header, rel, ok := strings.Cut(record, "\t")
		if !ok {
			return nil, invalidInput("snapshot Git inventory unavailable")
		}
		entries = append(entries, inventoryEntry{rel: rel, gitlink: strings.HasPrefix(header, "160000 ")})
	}
	for _, rel := range strings.Split(string(untracked), "\x00") {
		if rel != "" {
			entries = append(entries, inventoryEntry{rel: rel, untracked: true})
		}
	}

	files := make([]string, 0)
	seen := map[string]bool{}
	add := func(abs string) {
		if !seen[abs] {
			seen[abs] = true
			files = append(files, abs)
		}
	}
	for _, declaration := range declarations {
		if declaration.explicitAbs != "" {
			info, safe, err := lstatWithoutSymlinkParents(root, declaration.explicitAbs)
			if err != nil {
				return nil, invalidInput("snapshot scope unreadable: " + declaration.literal)
			}
			if safe && info.Mode().IsRegular() {
				add(declaration.explicitAbs)
			}
		}
	}
	for _, entry := range entries {
		rel := entry.rel
		cleanRel := filepath.Clean(filepath.FromSlash(rel))
		if filepath.IsAbs(cleanRel) || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
			return nil, invalidInput("snapshot Git inventory escapes the root")
		}
		rel = filepath.ToSlash(cleanRel)
		selected, err := selectedByScopeDeclarations(declarations, rel)
		if err != nil {
			return nil, invalidInput("snapshot scope has an invalid glob")
		}
		intersectsDirectory := (entry.gitlink || entry.untracked) && scopeMaySelectDescendant(declarations, rel)
		if entry.gitlink {
			if selected || intersectsDirectory {
				return nil, invalidInput("snapshot scope contains an unsupported Git submodule: " + rel)
			}
			continue
		}
		if !selected && !intersectsDirectory {
			continue
		}
		abs := filepath.Join(root, cleanRel)
		info, safe, err := lstatWithoutSymlinkParents(root, abs)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
		if !safe {
			continue
		}
		if info.IsDir() {
			if entry.untracked && (selected || intersectsDirectory) {
				return nil, invalidInput("snapshot scope contains an unsupported embedded Git repository: " + rel)
			}
			continue
		}
		if !selected {
			continue
		}
		if !info.Mode().IsRegular() {
			continue
		}
		add(abs)
	}
	sort.Strings(files)
	return files, nil
}

func gitInventoryPathspecs(declarations []scopeDeclaration) []string {
	pathspecs := make([]string, 0, len(declarations))
	seen := map[string]bool{}
	for _, declaration := range declarations {
		pathspec := ":(literal)" + declaration.literal
		if declaration.pattern != "" {
			pathspec = ":(glob)" + declaration.pattern
		}
		if !seen[pathspec] {
			seen[pathspec] = true
			pathspecs = append(pathspecs, pathspec)
		}
	}
	return pathspecs
}

func scopeMaySelectDescendant(declarations []scopeDeclaration, directory string) bool {
	for _, declaration := range declarations {
		if declaration.pattern != "" {
			if globMayMatchDescendant(declaration.pattern, directory) {
				return true
			}
			continue
		}
		literal := declaration.literal
		if declaration.directory && (literal == "." || strings.HasPrefix(directory, literal+"/")) ||
			strings.HasPrefix(literal, directory+"/") {
			return true
		}
	}
	return false
}

func globMayMatchDescendant(pattern, directory string) bool {
	patternParts := strings.Split(pattern, "/")
	directoryParts := strings.Split(directory, "/")
	type state struct{ pattern, directory int }
	seen := map[state]bool{}
	var intersects func(int, int) bool
	intersects = func(patternIndex, directoryIndex int) bool {
		key := state{pattern: patternIndex, directory: directoryIndex}
		if seen[key] {
			return false
		}
		seen[key] = true
		if directoryIndex == len(directoryParts) {
			return patternIndex < len(patternParts)
		}
		if patternIndex == len(patternParts) {
			return false
		}
		if patternParts[patternIndex] == "**" {
			return intersects(patternIndex+1, directoryIndex) || intersects(patternIndex, directoryIndex+1)
		}
		matched, _ := path.Match(patternParts[patternIndex], directoryParts[directoryIndex])
		return matched && intersects(patternIndex+1, directoryIndex+1)
	}
	return intersects(0, 0)
}

// lstatWithoutSymlinkParents returns the final entry only when every path
// component below root is a real directory rather than a symlink. Git's index
// can keep src/file after src is replaced by a symlink; a final-component
// Lstat alone would then hash content outside the worktree.
func lstatWithoutSymlinkParents(root, abs string) (fs.FileInfo, bool, error) {
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, false, fs.ErrInvalid
	}
	current := root
	parts := strings.Split(rel, string(filepath.Separator))
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, false, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return info, false, nil
		}
		if index < len(parts)-1 && !info.IsDir() {
			return info, false, nil
		}
		if index == len(parts)-1 {
			return info, true, nil
		}
	}
	info, err := os.Lstat(root)
	return info, err == nil && info.Mode()&fs.ModeSymlink == 0, err
}

func selectedByScopeDeclarations(declarations []scopeDeclaration, rel string) (bool, error) {
	for _, declaration := range declarations {
		switch {
		case declaration.pattern != "":
			matched, err := matchScopeGlob(declaration.pattern, rel)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		case declaration.directory:
			if declaration.literal == "." || rel == declaration.literal || strings.HasPrefix(rel, declaration.literal+"/") {
				return true, nil
			}
		case rel == declaration.literal:
			return true, nil
		}
	}
	return false, nil
}

func enumerateFilesystemScope(root string, paths []string) ([]string, error) {
	files := make([]string, 0)
	seen := map[string]bool{}
	for _, rel := range paths {
		fromSlash := filepath.FromSlash(rel)
		cleanRel := filepath.Clean(fromSlash)
		if rel == "" || filepath.IsAbs(fromSlash) || cleanRel == ".." ||
			strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
			return nil, invalidInput("snapshot scope escapes the root: " + rel)
		}
		if strings.ContainsAny(rel, "*?[") {
			pattern := filepath.ToSlash(cleanRel)
			if _, err := matchScopeGlob(pattern, ""); err != nil {
				return nil, invalidInput("snapshot scope has an invalid glob: " + rel)
			}
			err := enumerateScopeGlob(root, pattern, func(file string) {
				if !seen[file] {
					seen[file] = true
					files = append(files, file)
				}
			})
			if err != nil {
				return nil, invalidInput("snapshot scope unreadable: " + rel)
			}
			continue
		}
		abs := filepath.Join(root, cleanRel)
		if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return nil, invalidInput("snapshot scope escapes the root: " + rel)
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		if !info.IsDir() {
			if !info.Mode().IsRegular() {
				continue
			}
			if !seen[abs] {
				seen[abs] = true
				files = append(files, abs)
			}
			continue
		}
		err = filepath.WalkDir(abs, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.Type()&fs.ModeSymlink != 0 {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !entry.Type().IsRegular() {
				return nil
			}
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, invalidInput("snapshot scope unreadable: " + rel)
		}
	}
	sort.Strings(files)
	return files, nil
}

// enumerateScopeGlob walks only path segments that can still produce a match.
// Ordinary wildcards consume exactly one segment; ** consumes zero or more
// directory segments. This keeps an unreadable, unmatched sibling outside the
// declaration while still failing closed when an unreadable directory could
// contain a selected file.
func enumerateScopeGlob(root, pattern string, add func(string)) error {
	parts := strings.Split(pattern, "/")
	type state struct {
		dir   string
		index int
	}
	visited := map[state]bool{}
	var walk func(string, int) error
	walk = func(dir string, index int) error {
		if index >= len(parts) {
			return nil
		}
		key := state{dir: dir, index: index}
		if visited[key] {
			return nil
		}
		visited[key] = true
		segment := parts[index]
		last := index == len(parts)-1

		if segment == "**" {
			if !last {
				if err := walk(dir, index+1); err != nil {
					return err
				}
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.Type()&fs.ModeSymlink != 0 {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				file := filepath.Join(dir, entry.Name())
				if info.IsDir() {
					if err := walk(file, index); err != nil {
						return err
					}
					continue
				}
				if last && info.Mode().IsRegular() {
					add(file)
				}
			}
			return nil
		}

		if strings.ContainsAny(segment, "*?[") {
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				matched, err := path.Match(segment, entry.Name())
				if err != nil {
					return err
				}
				if !matched || entry.Type()&fs.ModeSymlink != 0 {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					return err
				}
				file := filepath.Join(dir, entry.Name())
				if last {
					if info.Mode().IsRegular() {
						add(file)
					}
					continue
				}
				if info.IsDir() {
					if err := walk(file, index+1); err != nil {
						return err
					}
				}
			}
			return nil
		}

		file := filepath.Join(dir, filepath.FromSlash(segment))
		info, err := os.Lstat(file)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil
		}
		if last {
			if info.Mode().IsRegular() {
				add(file)
			}
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		return walk(file, index+1)
	}
	return walk(root, 0)
}

func matchScopeGlob(pattern, name string) (bool, error) {
	patternParts := strings.Split(pattern, "/")
	nameParts := []string{}
	if name != "" {
		nameParts = strings.Split(name, "/")
	}
	for _, part := range patternParts {
		if part == "**" {
			continue
		}
		if _, err := path.Match(part, ""); err != nil {
			return false, err
		}
	}
	type state struct{ pattern, name int }
	memo := map[state]bool{}
	seen := map[state]bool{}
	var match func(int, int) bool
	match = func(pi, ni int) bool {
		key := state{pattern: pi, name: ni}
		if seen[key] {
			return memo[key]
		}
		seen[key] = true
		var matched bool
		switch {
		case pi == len(patternParts):
			matched = ni == len(nameParts)
		case patternParts[pi] == "**":
			matched = match(pi+1, ni) || ni < len(nameParts) && match(pi, ni+1)
		case ni < len(nameParts):
			segment, _ := path.Match(patternParts[pi], nameParts[ni])
			matched = segment && match(pi+1, ni+1)
		}
		memo[key] = matched
		return matched
	}
	return match(0, 0), nil
}

// hashFiles content-addresses sorted absolute files as rel + NUL + content
// sha256, with rel against root. Check (MR-011) reuses it verbatim: the
// same bytes must hash the same from either entry point, or staleness
// would hallucinate on identical trees.
func hashFiles(root string, files []string) (string, error) {
	digest := sha256.New()
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return "", errors.New(filepath.ToSlash(file))
		}
		rel, err := filepath.Rel(root, file)
		if err != nil {
			return "", errors.New(filepath.ToSlash(file))
		}
		digest.Write([]byte(filepath.ToSlash(rel)))
		digest.Write([]byte{0})
		digest.Write(content)
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

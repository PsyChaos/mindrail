package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Snapshot is a content address over a profile scope: the sorted absolute
// scope plus the hex sha256 binding it. Evidence staples this hash; MR-011
// compares it to detect staleness.
type Snapshot struct {
	Scope []string
	Hash  string
}

// SnapshotScope hashes every regular file selected by root-relative literal
// files, directories, or glob declarations. Directories are walked, ** is
// recursive, symlinks are never followed, and content is addressed. Scope
// escapes and missing/unreadable literal paths fail before execution; an empty
// glob match remains a valid declaration whose later membership can go stale.
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

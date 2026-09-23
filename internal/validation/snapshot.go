package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
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

// SnapshotScope hashes every regular file under root-relative paths: dirs
// walked, symlinks never followed, content addressed. Scope escapes
// (anything resolving outside root, including through ..) and missing or
// unreadable files fail before any execution: evidence never binds a
// partial scope (decision D-158).
func SnapshotScope(root string, paths []string) (Snapshot, error) {
	if root == "" || !filepath.IsAbs(root) {
		return Snapshot{}, invalidInput("snapshot needs an absolute root")
	}
	if len(paths) == 0 {
		return Snapshot{}, invalidInput("snapshot needs a scope")
	}
	var files []string
	seen := map[string]bool{}
	for _, rel := range paths {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		if abs != root && !strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return Snapshot{}, invalidInput("snapshot scope escapes the root: " + rel)
		}
		info, err := os.Lstat(abs)
		if err != nil {
			return Snapshot{}, invalidInput("snapshot scope unreadable: " + rel)
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
			return Snapshot{}, invalidInput("snapshot scope unreadable: " + rel)
		}
	}
	sort.Strings(files)
	hash, err := hashFiles(root, files)
	if err != nil {
		return Snapshot{}, invalidInput("snapshot scope unreadable: " + err.Error())
	}
	return Snapshot{Scope: files, Hash: hash}, nil
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

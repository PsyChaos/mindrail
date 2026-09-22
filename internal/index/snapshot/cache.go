// Package snapshot stores re-derivable, path-independent parser facts under
// the Git common-dir's runtime cache. It never owns a native Tree-sitter tree
// or durable SQLite facts.
package snapshot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PsyChaos/mindrail/internal/filesystem"
	"github.com/PsyChaos/mindrail/internal/index/parser"
)

// Facts is the complete native-free parser output. No filesystem path,
// project-unit identity, or SQLite row ID belongs in cached facts.
type Facts = parser.Facts

// Computer re-derives facts from source bytes. It owns and closes any native
// parser snapshot it creates. A non-nil error (including a partial parse
// error) is returned unchanged and is never cached.
type Computer func(context.Context, []byte) (Facts, error)

var (
	ErrIdentity     = errors.New("snapshot identity requires language and grammar version")
	ErrNoComputer   = errors.New("snapshot computer is required")
	ErrInvalidFacts = errors.New("snapshot computer returned malformed facts")
)

type Cache struct {
	dir           string
	schemaVersion int
}

// New uses the shared cache directory already resolved by filesystem. A
// missing or unwritable cache directory does not block computation.
func New(paths filesystem.RuntimePaths) *Cache {
	return &Cache{dir: paths.CacheDir, schemaVersion: parser.ParseSchemaVersion}
}

// GetOrCompute returns verified cached facts or recomputes them. Cache read,
// corruption and write failures are correctness-neutral; only the computer's
// error, invalid input, or context cancellation reaches the caller.
func (c *Cache) GetOrCompute(ctx context.Context, info parser.LanguageInfo, content []byte, compute Computer) (Facts, error) {
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	if strings.TrimSpace(info.Language) == "" || strings.TrimSpace(info.GrammarVersion) == "" {
		return Facts{}, ErrIdentity
	}
	if compute == nil {
		return Facts{}, ErrNoComputer
	}
	owned := bytes.Clone(content)
	identity := keyIdentity{
		Language: info.Language, GrammarVersion: info.GrammarVersion,
		SchemaVersion: c.schemaVersion, ContentHash: sha256Hex(owned),
	}
	path := c.entryPath(identity)
	if facts, ok := readEntry(path, identity, len(owned)); ok {
		if err := ctx.Err(); err != nil {
			return Facts{}, err
		}
		return facts, nil
	}
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	facts, computeErr := compute(ctx, bytes.Clone(owned))
	if err := ctx.Err(); err != nil {
		if computeErr != nil {
			return facts, errors.Join(computeErr, err)
		}
		return Facts{}, err
	}
	if computeErr != nil {
		return facts, computeErr
	}
	if !validFacts(facts, len(owned)) {
		return Facts{}, ErrInvalidFacts
	}
	writeEntry(path, identity, facts)
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	return facts, nil
}

type keyIdentity struct {
	Language       string `json:"language"`
	GrammarVersion string `json:"grammar_version"`
	SchemaVersion  int    `json:"schema_version"`
	ContentHash    string `json:"content_hash"`
}

type diskEntry struct {
	Identity keyIdentity     `json:"identity"`
	Facts    json.RawMessage `json:"facts"`
	Digest   string          `json:"digest"`
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c *Cache) entryPath(identity keyIdentity) string {
	encoded, _ := json.Marshal(identity) // fixed string/integer fields cannot fail
	return filepath.Join(c.dir, sha256Hex(encoded)+".json")
}

const maxEntryBytes = 32 << 20

func readEntry(path string, identity keyIdentity, contentLen int) (Facts, bool) {
	// Opening a FIFO would block before GetOrCompute can recheck cancellation.
	// Lstat also rejects symlinks rather than following one to a FIFO. This is
	// a best-effort guard: replacement between Lstat and Open remains possible.
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return Facts{}, false
	}
	file, err := os.Open(path)
	if err != nil {
		return Facts{}, false
	}
	defer file.Close()
	encoded, err := io.ReadAll(io.LimitReader(file, maxEntryBytes+1))
	if err != nil || len(encoded) > maxEntryBytes {
		return Facts{}, false
	}
	var entry diskEntry
	if err := decodeExact(encoded, &entry); err != nil || entry.Identity != identity {
		return Facts{}, false
	}
	if entry.Digest != sha256Hex(entry.Facts) {
		return Facts{}, false
	}
	var facts Facts
	if err := decodeExact(entry.Facts, &facts); err != nil || !validFacts(facts, contentLen) {
		return Facts{}, false
	}
	canonical, err := json.Marshal(facts)
	if err != nil || !bytes.Equal(canonical, entry.Facts) {
		return Facts{}, false
	}
	return facts, true
}

func decodeExact(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	return nil
}

func validFacts(facts Facts, contentLen int) bool {
	if facts.HasSyntaxErrors {
		return false
	}
	keys := make(map[string]bool, len(facts.Symbols))
	for _, symbol := range facts.Symbols {
		if symbol.Name == "" || symbol.Kind == "" || !validRange(symbol.Range, contentLen) ||
			symbol.LocalKey != parser.LocalKey(symbol.ContainerLocalKey, symbol.Kind, symbol.Name) ||
			!validRange(symbol.SignatureRange, contentLen) || !within(symbol.SignatureRange, symbol.Range) ||
			!validHash(symbol.SignatureHash) || !validHash(symbol.BodyHash) || !validHash(symbol.StructureHash) {
			return false
		}
		if symbol.BodyRange != nil && (!validRange(*symbol.BodyRange, contentLen) || !within(*symbol.BodyRange, symbol.Range) || symbol.SignatureRange.EndByte > symbol.BodyRange.StartByte) {
			return false
		}
		keys[symbol.LocalKey] = true
	}
	validKey := func(key string) bool { return key == "" || keys[key] }
	for _, symbol := range facts.Symbols {
		if !validKey(symbol.ContainerLocalKey) {
			return false
		}
	}
	for _, imp := range facts.Imports {
		if imp.Module == "" || !validKey(imp.ImporterLocalKey) || !validRange(imp.Range, contentLen) {
			return false
		}
		for _, name := range imp.Names {
			if name == "" {
				return false
			}
		}
	}
	for _, reference := range facts.References {
		if reference.Name == "" || reference.Kind == "" || !validRange(reference.Range, contentLen) ||
			!validKey(reference.ReferrerLocalKey) || !validKey(reference.ScopeLocalKey) || !validKey(reference.TargetLocalKey) {
			return false
		}
	}
	return true
}

func within(inner, outer parser.Range) bool {
	return inner.StartByte >= outer.StartByte && inner.EndByte <= outer.EndByte
}
func validHash(value string) bool {
	if len(value) != sha256.Size*2 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validRange(span parser.Range, contentLen int) bool {
	if span.StartByte > span.EndByte || span.EndByte > uint(contentLen) || span.StartRow > span.EndRow {
		return false
	}
	return span.StartRow != span.EndRow || span.StartColumn <= span.EndColumn
}

func writeEntry(path string, identity keyIdentity, facts Facts) {
	payload, err := json.Marshal(facts)
	if err != nil {
		return
	}
	encoded, err := json.Marshal(diskEntry{Identity: identity, Facts: payload, Digest: sha256Hex(payload)})
	if err != nil || len(encoded) > maxEntryBytes {
		return
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	temp, err := os.CreateTemp(dir, ".snapshot-*")
	if err != nil {
		return
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(encoded); err != nil {
		temp.Close()
		return
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return
	}
	if err := temp.Close(); err != nil {
		return
	}
	_ = os.Rename(temp.Name(), path)
}

package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/PsyChaos/mindrail/internal/app"
	"github.com/PsyChaos/mindrail/internal/index"
	"github.com/PsyChaos/mindrail/internal/status"
)

// MaxSearchResults bounds one search (decision D-190): above refuses,
// never silently truncates.
const MaxSearchResults = 50

// SearchIn narrows nothing in 0.1 — query plus limit. Limit absent means
// the cap; above the cap refuses.
type SearchIn struct {
	Query string `json:"query"`
	Limit *int   `json:"limit,omitempty"`
}

// SearchHit is one match with its source, handle and snippet.
type SearchHit struct {
	Source  string `json:"source"`
	ID      string `json:"id"`
	Snippet string `json:"snippet"`
}

// SearchOut is hits beside an optional refusal. Results always present:
// an empty list, never a missing key.
type SearchOut struct {
	Results []SearchHit `json:"results"`
	Refusal *Refusal    `json:"refusal,omitempty"`
}

func (s *Server) search(ctx context.Context, _ *sdk.CallToolRequest, in SearchIn) (*sdk.CallToolResult, SearchOut, error) {
	if strings.TrimSpace(in.Query) == "" {
		return nil, SearchOut{}, Invalid("search needs a query")
	}
	limit := MaxSearchResults
	if in.Limit != nil {
		limit = *in.Limit
	}
	if limit <= 0 || limit > MaxSearchResults {
		return nil, SearchOut{Results: []SearchHit{}, Refusal: NotImplemented(
			"search limit above 50",
			"limit 1..50",
			"Retry search with a limit of at most 50.")}, nil
	}
	var hits []SearchHit
	hits = append(hits, s.searchKnowledge(in.Query)...)
	hits = append(hits, s.searchSymbols(ctx, in.Query)...)
	if len(hits) > limit {
		hits = hits[:limit]
	}
	if hits == nil {
		hits = []SearchHit{}
	}
	return nil, SearchOut{Results: hits}, nil
}

type recordText struct {
	Title     string `json:"title"`
	Decision  string `json:"decision"`
	Statement string `json:"statement"`
}

// searchKnowledge substrings the loaded records' human text. Bodies come
// from the loader store the application already holds — no second load.
func (s *Server) searchKnowledge(query string) []SearchHit {
	store := s.app.Subject().Knowledge
	var hits []SearchHit
	lowered := strings.ToLower(query)
	consider := func(id string, body []byte) {
		var text recordText
		if err := json.Unmarshal(body, &text); err != nil {
			return
		}
		joined := id + "\n" + text.Title + "\n" + text.Decision + "\n" + text.Statement
		if !strings.Contains(strings.ToLower(joined), lowered) {
			return
		}
		snippet := firstNonEmpty(text.Title, text.Statement, text.Decision, id)
		hits = append(hits, SearchHit{Source: "knowledge", ID: id, Snippet: snippet})
	}
	for _, ref := range store.Decisions {
		consider(ref.ID, ref.Body)
	}
	for _, ref := range store.Invariants {
		consider(ref.ID, ref.Body)
	}
	return hits
}

// searchSymbols resolves exact declaration names through the index the
// application already opened. Substring symbol search does not exist in
// 0.1: exact names answer, anything else is an empty list, never a guess.
func (s *Server) searchSymbols(ctx context.Context, query string) []SearchHit {
	if s.app.DB() == nil {
		return nil
	}
	indexes := index.NewStore(s.app.DB(), app.SystemClock{})
	named, err := indexes.SymbolsNamed(ctx, query)
	if err != nil {
		return nil
	}
	var hits []SearchHit
	for _, symbol := range named {
		hits = append(hits, SearchHit{
			Source:  "symbol",
			ID:      symbol.UID,
			Snippet: symbol.Name + " in " + symbol.Path,
		})
	}
	return hits
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			if len(value) > 300 {
				return value[:300] + "…(truncated)"
			}
			return value
		}
	}
	return ""
}

// ContextIn carries the 0.1 detail levels plus the deferred drill-down
// vocabulary. Deferred fields are pointers: absence means default, presence
// of anything refuses (decision D-188).
type ContextIn struct {
	DetailLevel *string `json:"detail_level,omitempty"`
	TargetType  *string `json:"target_type,omitempty"`
	TargetID    *string `json:"target_id,omitempty"`
	Cursor      *string `json:"cursor,omitempty"`
	PageSize    *int    `json:"page_size,omitempty"`
}

// ContextOut is the aggregate at the requested level.
type ContextOut struct {
	Readiness  string            `json:"readiness"`
	Decisions  int               `json:"decisions"`
	Invariants int               `json:"invariants"`
	Paths      map[string]string `json:"paths"`
	Excerpts   []SearchHit       `json:"excerpts,omitempty"`
	Refusal    *Refusal          `json:"refusal,omitempty"`
}

func (s *Server) context(ctx context.Context, _ *sdk.CallToolRequest, in ContextIn) (*sdk.CallToolResult, ContextOut, error) {
	if in.TargetType != nil {
		return nil, ContextOut{Paths: map[string]string{}, Refusal: NotImplemented(
			"target_type "+*in.TargetType,
			"no target selection in 0.1",
			"Call context without target_type, or wait for drill-down support.")}, nil
	}
	if in.TargetID != nil {
		return nil, ContextOut{Paths: map[string]string{}, Refusal: NotImplemented(
			"target_id selection",
			"no target selection in 0.1",
			"Call context without target_id, or wait for drill-down support.")}, nil
	}
	if in.Cursor != nil {
		return nil, ContextOut{Paths: map[string]string{}, Refusal: NotImplemented(
			"cursor pagination",
			"unpaged context in 0.1",
			"Call context without cursor, or wait for pagination support.")}, nil
	}
	if in.PageSize != nil {
		return nil, ContextOut{Paths: map[string]string{}, Refusal: NotImplemented(
			"page_size pagination",
			"unbounded excerpts within the focused cap in 0.1",
			"Call context without page_size, or wait for pagination support.")}, nil
	}
	level := "summary"
	if in.DetailLevel != nil {
		level = *in.DetailLevel
	}
	switch level {
	case "summary":
		return nil, s.summarize(), nil
	case "focused":
		return nil, s.focus(), nil
	default:
		return nil, ContextOut{Paths: map[string]string{}, Refusal: NotImplemented(
			"detail_level "+level,
			"summary|focused",
			"Call context with detail_level summary or focused.")}, nil
	}
}

func (s *Server) summarize() ContextOut {
	paths := s.app.Paths()
	store := s.app.Subject().Knowledge
	return ContextOut{
		Readiness:  string(status.Build(s.app.Subject(), time.Since(s.started)).Readiness),
		Decisions:  len(store.Decisions),
		Invariants: len(store.Invariants),
		Paths: map[string]string{
			"worktree_root":   paths.WorktreeRoot,
			"runtime_root":    paths.RuntimeRoot,
			"repo_config_dir": paths.RepoConfigDir,
		},
	}
}

func (s *Server) focus() ContextOut {
	out := s.summarize()
	store := s.app.Subject().Knowledge
	for _, ref := range store.Decisions {
		if len(out.Excerpts) >= 5 {
			break
		}
		out.Excerpts = append(out.Excerpts, excerptOf(ref.ID, ref.Body))
	}
	for _, ref := range store.Invariants {
		if len(out.Excerpts) >= 5 {
			break
		}
		out.Excerpts = append(out.Excerpts, excerptOf(ref.ID, ref.Body))
	}
	if out.Excerpts == nil {
		out.Excerpts = []SearchHit{}
	}
	return out
}

func excerptOf(id string, body []byte) SearchHit {
	var text recordText
	if err := json.Unmarshal(body, &text); err != nil {
		return SearchHit{Source: "knowledge", ID: id, Snippet: id}
	}
	return SearchHit{Source: "knowledge", ID: id,
		Snippet: firstNonEmpty(text.Title, text.Statement, text.Decision, id)}
}

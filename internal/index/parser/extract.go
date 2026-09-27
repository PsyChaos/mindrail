package parser

import (
	"context"
	"encoding/json"
	"errors"
)

// LocalKey is an unambiguous, path-independent declaration description. It is
// deliberately not unique: overload declarations share one logical key.
func LocalKey(container, kind, name string) string {
	encoded, _ := json.Marshal([3]string{container, kind, name})
	// Hashing the tuple keeps a nested container's representation bounded;
	// recursively JSON-escaping the previous tuple grows exponentially.
	return hashBytes(encoded)
}

// Extract returns native-free facts and closes every native snapshot, including
// partial parses. Syntax/extraction errors preserve partial facts; cancellation
// returns no completion. The caller must not cache facts returned with an error.
func Extract(ctx context.Context, a SyntaxAdapter, src SourceFile) (Facts, error) {
	if err := ctx.Err(); err != nil {
		return Facts{}, err
	}
	s, parseErr := a.Parse(ctx, src)
	if s == nil {
		return Facts{}, parseErr
	}
	defer s.Close()
	facts := Facts{HasSyntaxErrors: s.HasErrors()}
	var symbolErr, importErr, referenceErr error
	facts.Symbols, symbolErr = a.Symbols(s)
	facts.Imports, importErr = a.Imports(s)
	facts.References, referenceErr = a.References(s)
	err := errors.Join(parseErr, symbolErr, importErr, referenceErr)
	if contextErr := ctx.Err(); contextErr != nil {
		return Facts{}, errors.Join(err, contextErr)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return Facts{}, err
	}
	return facts, err
}

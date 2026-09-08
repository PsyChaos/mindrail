package schema

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
)

// RecordKind names the two kinds of knowledge record (spec §53, §54).
//
// It is declared here rather than reused from internal/knowledge/loader
// because loader already imports this package: taking loader.RecordKind as a
// parameter would close an import cycle. The two spellings are the same
// strings, and TestSchemaKindsAreSpeltTheSameWayAsTheLoaders holds them
// together, so a caller converts with schema.RecordKind(ref.Kind) and loses
// nothing.
type RecordKind string

const (
	KindDecision  RecordKind = "decision"
	KindInvariant RecordKind = "invariant"
)

// validatedKinds is every record kind this binary compiles a schema for, in a
// fixed order so that a construction failure names the same document on every
// run.
var validatedKinds = []RecordKind{KindDecision, KindInvariant}

// schemaIDBase is the "$id" prefix every shipped knowledge schema document
// declares.
//
// It is pinned in Go rather than read back out of whatever document happens to
// be in the registry, because the compile target has to be independent of the
// file for a renamed "$id" to be caught. A document that quietly renames
// itself breaks every "$ref" that names it; compiling it under whatever name it
// now claims would hide that, while compiling the pinned name fails loudly
// because nothing registered it (decision D-37).
const schemaIDBase = "https://mindrail.dev/schemas/knowledge/"

// ErrSchemaNotShipped reports that compiling the knowledge schemas had to
// resolve a document this binary does not carry.
//
// It is a sentinel because control flow must never turn on an error message
// (tech-stack §72), and because it is the only way to recognise the condition:
// the library wraps a loader failure in a *LoadURLError that implements no
// Unwrap, so the refusal below cannot be reached with errors.Is through it.
var ErrSchemaNotShipped = errors.New("knowledge schema document is not shipped in this binary and will not be fetched")

// Keywords that no JSON Schema vocabulary can define, used for the two findings
// that are about the document as a whole rather than about a keyword in it. The
// parentheses are what makes the collision impossible: a JSON Schema keyword is
// a member name in a schema object, and none of the drafts define one with
// punctuation in it.
const (
	// KeywordDocumentSyntax marks bytes that are not a single JSON value.
	KeywordDocumentSyntax = "(json syntax)"
	// KeywordNoSchema marks a (kind, version) pair this binary compiled no
	// schema for. Returning nothing for one would be a fabricated pass:
	// "nobody looked" would be published as "we looked and it is clean".
	KeywordNoSchema = "(no schema)"
)

// Finding is one violation of one schema document by one record.
//
// It carries no path and no step: which file the bytes came from and which
// pipeline step asked is the caller's knowledge, and internal/knowledge/validate
// adds both when it turns these into its own findings. The type lives here
// rather than in validate because validate imports schema and the reverse
// direction would be a cycle.
type Finding struct {
	// InstanceLocation is the RFC 6901 JSON Pointer of the offending value
	// inside the record; "" is the record itself. It is a structured field
	// rather than only a phrase inside Message so that a caller — or a test —
	// can name the field that was wrong without parsing prose.
	InstanceLocation string `json:"instance_location"`
	// Keyword is the JSON Schema keyword that rejected the value, spelled as
	// its keyword path joined by "/" ("type", "additionalProperties",
	// "dependentRequired/foo"), or one of the two Keyword* constants above.
	Keyword string `json:"keyword"`
	// Message states what is wrong in the library's own wording, so this
	// package does not become a second, drifting implementation of every
	// keyword's diagnosis.
	Message string `json:"message"`
}

// Validator compiles the embedded documents once. Compilation is where a
// malformed shipped schema is found, and finding it per record instead would
// report a defect in the binary as a defect in the repository.
//
// A Validator is safe for concurrent use: every compiled *jsonschema.Schema is
// read-only once Compile has returned, and Validate keeps no state.
type Validator struct {
	// schemas is keyed by the pair that decides which document governs a
	// record. Keying on kind alone would be wrong the day the reader window
	// widens past [1].
	schemas map[schemaKey]*jsonschema.Schema
	// loader is kept after construction only so a test can assert that the
	// happy path attempted no fetch at all. An error-free compilation cannot
	// prove that on its own — a loader that is never consulted and a loader
	// that is consulted and succeeds both leave it green.
	loader *deniedLoader
}

type schemaKey struct {
	kind    RecordKind
	version int
}

// deniedLoader is the compiler's URLLoader.
//
// Every document is handed to the compiler through AddResource before anything
// is compiled, and the Draft 2020-12 metaschemas are embedded inside the
// library itself, so a resolvable reference never reaches a loader. Anything
// that does reach one names a document this binary does not ship, and fetching
// it would make validation depend on the network (tech-stack §39).
//
// The library's default is a FileLoader, which would read a file:// URL off
// whichever machine ran the validation. That is a weaker promise than "no fetch
// on any path", so it is replaced rather than wrapped.
type deniedLoader struct {
	// denied records every URL a fetch was attempted for. Recording rather
	// than counting keeps the failure legible: the test that finds a non-empty
	// slice can say which document went looking for the network.
	denied []string
}

func (l *deniedLoader) Load(url string) (any, error) {
	l.denied = append(l.denied, url)
	return nil, fmt.Errorf("refusing to fetch %s: %w", url, ErrSchemaNotShipped)
}

// NewValidator compiles every document the reader window says this binary can
// read, and fails if it cannot compile all of them.
//
// Failing is the point (decision D-37). A validator that quietly skipped a
// document it could not find would report a record as clean when nothing had
// looked at it, and "nobody looked" published as "we looked and it is clean" is
// the defect class this project keeps paying for.
func NewValidator(reg *Registry) (*Validator, error) {
	if reg == nil {
		return nil, errors.New("knowledge schema validator: nil registry")
	}

	compiler := jsonschema.NewCompiler()
	// Named for what it does, not for the package: this file must stay free of
	// any dependency on internal/knowledge/loader, which imports it.
	urlLoader := &deniedLoader{}
	compiler.UseLoader(urlLoader)

	// Decision D-48. Under Draft 2020-12 this library treats "format" as an
	// annotation unless the metaschema declares the format-assertion
	// vocabulary, and the shipped documents do not. Without this call the
	// "format": "date-time" constraint on created_at enforces nothing at all —
	// verified: created_at "yesterday" produced no error whatsoever until the
	// option was set.
	compiler.AssertFormat()

	for _, name := range reg.Names() {
		raw, ok := reg.Document(name)
		if !ok {
			return nil, fmt.Errorf("knowledge schema validator: %s vanished from the registry between listing and reading", name)
		}

		// jsonschema.UnmarshalJSON rather than encoding/json, twice over.
		// AddResource accepts []byte and json.RawMessage without complaint and
		// only fails much later at Compile with "invalid jsonType []uint8", and
		// encoding/json into any loses integer precision — 9007199254740993
		// decodes as 9.007199254740992e+15, which would make a schema's numeric
		// constraints lie.
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("knowledge schema validator: decoding %s: %w", name, err)
		}

		id, err := documentID(doc)
		if err != nil {
			return nil, fmt.Errorf("knowledge schema validator: %s %w", name, err)
		}

		// Registered under the document's own $id and compiled under the
		// pinned one. When the two disagree the compile below is what notices.
		if err := compiler.AddResource(id, doc); err != nil {
			return nil, fmt.Errorf("knowledge schema validator: registering %s as %s: %w", name, id, err)
		}
	}

	schemas := make(map[schemaKey]*jsonschema.Schema, len(validatedKinds)*len(reg.ReadableVersions()))
	for _, recordKind := range validatedKinds {
		for _, version := range reg.ReadableVersions() {
			name := SchemaNameFor(recordKind, version)
			if _, ok := reg.Document(name); !ok {
				return nil, fmt.Errorf(
					"knowledge schema validator: this binary reads %s schema_version %d but ships no %s",
					recordKind, version, name)
			}

			compiled, err := compiler.Compile(schemaIDBase + name)
			if err != nil {
				// A compile that consulted the loader went looking for a
				// document nothing registered — the pinned $id and the one the
				// document declares have drifted apart. Saying so beats
				// forwarding "invalid file url", which reads like a filesystem
				// problem and sends the reader to the wrong place.
				if len(urlLoader.denied) > 0 {
					return nil, fmt.Errorf(
						"knowledge schema validator: compiling %s: %w: %s (underlying: %v)",
						name, ErrSchemaNotShipped, strings.Join(urlLoader.denied, ", "), err)
				}
				return nil, fmt.Errorf("knowledge schema validator: compiling %s: %w", name, err)
			}
			schemas[schemaKey{kind: recordKind, version: version}] = compiled
		}
	}

	// An empty reader window would leave nothing compiled and every record
	// unvalidated, which is worse than refusing to start.
	if len(schemas) == 0 {
		return nil, errors.New("knowledge schema validator: the reader window is empty, so no record could be validated")
	}

	return &Validator{schemas: schemas, loader: urlLoader}, nil
}

// SchemaNameFor returns the registry document name that governs one record kind
// at one schema version, following the tech-stack §7 "<kind>.v<n>.schema.json"
// layout the shipped files already use.
//
// It is derived rather than looked up in a table so that widening the reader
// window cannot leave a version with no document silently unvalidated: an
// absent name fails NewValidator. TestSchemaDocumentNamingConventionAgreesWithTheRegistryConstants
// keeps the convention and the two exported constants from drifting apart.
func SchemaNameFor(recordKind RecordKind, version int) string {
	return fmt.Sprintf("%s.v%d.schema.json", recordKind, version)
}

// Validate reports every way document breaks the schema for its kind and
// version. It returns an empty, non-nil slice when the record is clean, so a
// caller can append to the result and a marshalled report never says null.
//
// The result is a value, not an error: a record this binary read and found
// wrong is data the repository owns, and turning it into an error would stop
// the caller from reporting the records beside it (decision D-38, D-40).
func (v *Validator) Validate(recordKind RecordKind, version int, document []byte) []Finding {
	findings := make([]Finding, 0, 4)

	compiled, ok := v.schemas[schemaKey{kind: recordKind, version: version}]
	if !ok {
		return append(findings, Finding{
			Keyword: KeywordNoSchema,
			Message: fmt.Sprintf(
				"no shipped schema document governs kind %q at schema_version %d, so this record was not validated",
				recordKind, version),
		})
	}

	// The instance goes through the library's decoder for the same precision
	// reason as the documents: encoding/json would round 1.0000000000000001
	// to exactly 1 and let it satisfy "type": "integer".
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(document))
	if err != nil {
		return append(findings, Finding{
			Keyword: KeywordDocumentSyntax,
			Message: "record is not a single JSON value: " + err.Error(),
		})
	}

	err = compiled.Validate(instance)
	if err == nil {
		return findings
	}

	var invalid *jsonschema.ValidationError
	if !errors.As(err, &invalid) {
		// The library documents *ValidationError as the only failure Validate
		// returns. Reporting an unexpected one is still better than dropping
		// it, which would publish a rejected record as clean.
		return append(findings, Finding{
			Keyword: KeywordNoSchema,
			Message: "schema validation failed in an unexpected way: " + err.Error(),
		})
	}

	findings = collectFindings(invalid, findings)

	// Decision D-47. Causes come back in Go map-iteration order: five
	// consecutive Validate calls on the same schema and the same instance in
	// one process were observed to produce five different leaf orders. A
	// deterministic core (tech-stack §2.2) cannot publish that.
	slices.SortFunc(findings, func(a, b Finding) int {
		return cmp.Or(
			cmp.Compare(a.InstanceLocation, b.InstanceLocation),
			cmp.Compare(a.Keyword, b.Keyword),
			cmp.Compare(a.Message, b.Message),
		)
	})
	return findings
}

// collectFindings walks a ValidationError tree to its leaves and appends one
// finding per leaf.
//
// Causes is a tree, not a list. The root is always a *kind.Schema saying only
// that validation failed, and a "$ref" produces a *kind.Reference wrapper whose
// child carries the keyword that actually rejected the value — reporting either
// would name the mechanism instead of the mistake. Descending to leaves drops
// both without having to recognise them, because neither is ever a leaf.
func collectFindings(node *jsonschema.ValidationError, into []Finding) []Finding {
	if len(node.Causes) > 0 {
		for _, cause := range node.Causes {
			into = collectFindings(cause, into)
		}
		return into
	}

	location := pointerOf(node.InstanceLocation)

	// kind.AdditionalProperties reports the containing object as its instance
	// location and names the offending members only in Properties, so a finding
	// copied straight off it would point at the whole record and leave the
	// reader to guess which key to delete. One finding per offending property,
	// located on the property itself, is what makes the name reachable without
	// reading prose.
	if extra, ok := node.ErrorKind.(*kind.AdditionalProperties); ok && len(extra.Properties) > 0 {
		for _, property := range extra.Properties {
			at := location + "/" + escapePointerToken(property)
			into = append(into, Finding{
				InstanceLocation: at,
				Keyword:          keywordOf(node),
				Message:          fmt.Sprintf("at '%s': property '%s' is not allowed by the schema", at, property),
			})
		}
		return into
	}

	return append(into, Finding{
		InstanceLocation: location,
		Keyword:          keywordOf(node),
		Message:          leafMessage(node),
	})
}

// keywordOf renders a leaf's keyword path. Most kinds report a single keyword;
// dependentRequired and friends report the keyword and the property it hangs
// off, and joining them keeps both.
func keywordOf(node *jsonschema.ValidationError) string {
	return strings.Join(node.ErrorKind.KeywordPath(), "/")
}

// leafMessage renders one leaf in the library's own wording.
//
// It re-wraps the node with its causes stripped because the library's renderer
// walks causes and would otherwise fold a whole subtree into one message; a
// leaf has none, so the copy renders exactly one line. Rendering it through the
// library rather than by hand is deliberate: a second implementation of every
// keyword's diagnosis would drift from the one that decided the verdict.
func leafMessage(node *jsonschema.ValidationError) string {
	solo := &jsonschema.ValidationError{
		SchemaURL:        node.SchemaURL,
		InstanceLocation: node.InstanceLocation,
		ErrorKind:        node.ErrorKind,
	}
	return solo.Error()
}

// pointerTokenEscape is RFC 6901 §3, and the order matters: "~" has to become
// "~0" before "/" becomes "~1", or the tilde introduced by the second rule
// would be escaped by the first. strings.Replacer makes one pass and never
// rescans what it wrote, so a single Replacer is correct where two sequential
// ReplaceAll calls would not be.
var pointerTokenEscape = strings.NewReplacer("~", "~0", "/", "~1")

func escapePointerToken(token string) string {
	return pointerTokenEscape.Replace(token)
}

// pointerOf renders the library's instance location as an RFC 6901 JSON
// Pointer. The library hands back raw, unescaped tokens, so a property actually
// named "a/b" would otherwise be indistinguishable from a nested one.
func pointerOf(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	var out strings.Builder
	for _, token := range tokens {
		out.WriteByte('/')
		out.WriteString(escapePointerToken(token))
	}
	return out.String()
}

// documentID reads the "$id" a schema document declares.
//
// A document with no "$id" is refused rather than registered under some
// invented URL: nothing could reference it, so registering it would only make
// the compiler's view disagree with the file's own claim about its identity.
func documentID(doc any) (string, error) {
	object, ok := doc.(map[string]any)
	if !ok {
		return "", errors.New("is not a JSON object")
	}
	raw, ok := object["$id"]
	if !ok {
		return "", errors.New("declares no $id, so nothing can reference it")
	}
	id, ok := raw.(string)
	if !ok || id == "" {
		return "", fmt.Errorf("declares an $id that is not a non-empty string: %v", raw)
	}
	return id, nil
}

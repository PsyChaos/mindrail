# MR-001 — Implementation design

- **Task:** MR-001 — Yerel repository bootstrap ve tanılama yolu (`.tasks/mindrail-0.1-task-list.md` L35–L51)
- **Deliverable:** `mindrail init`, `mindrail status`, `mindrail doctor` (+ minimal `mindrail version`) in one binary
- **Authority:** `.docs/mindrail-tech-stack.md` (sequencing, package names), `.docs/mindrail-technical-specification-1.0.md` (scope wins on conflict, §168), `.docs/mindrail-0.1-kernel-scope.md` (0.1 delivery scope)
- **Status:** contract for four parallel work units + one wiring unit

## 0. Citation audit

Spot-checked 20 of the citations handed over by the readers against the source
documents. Results:

| Citation | Verdict |
|---|---|
| tech-stack §7 source layout (`internal/{app,bootstrap,git,filesystem,config,storage,migration,doctor,cli,knowledge/…}`, root `migrations/`, `schemas/knowledge/`) | HOLDS (L258–L349) |
| tech-stack §12 "JSON never contains ANSI escape codes." / "`NO_COLOR` is respected." | HOLDS (L479–L480) |
| tech-stack §13 exit codes 0/1/2/3/4 | HOLDS (L490–L496) — no mapping of setup failures onto the set, as the readers said |
| tech-stack §20 "storage code MUST remain behind `database/sql` and MUST NOT depend on driver-specific APIs outside a tiny adapter/bootstrap boundary" | HOLDS verbatim (L688) |
| tech-stack §24 "embedded numbered SQL files + schema_migrations table", "ordered / forward-only by default / transactional where possible / fail-fast / no implicit downgrade" | HOLDS (L771–L794); also "Do not add a migration framework initially." (L767) |
| tech-stack §34/§35 system `git` via `os/exec`, no `go-git`, argv not shell, `--porcelain=v2`/`-z`/`--name-status`/`--format` | HOLDS (L1032–L1079) |
| tech-stack §37 five-layer precedence, `MINDRAIL_` prefix, unknown env var warns | HOLDS (L1137–L1153) |
| tech-stack §39 "Supported schemas are embedded in the executable." / "Normal knowledge validation must not fetch schemas from the network." | HOLDS (L1189–L1191) |
| tech-stack §42 "All persisted timestamps: UTC", RFC 3339, inject `Clock` where determinism matters | HOLDS (L1238–L1253) |
| tech-stack §72 five error carrier fields + `errors.Is`/`errors.As` + "Never use string matching as primary control flow." | HOLDS (L1874–L1892) |
| tech-stack §74/§75 repo-relative `/` stored paths; clean/contain/symlink/traversal | HOLDS (L1908–L1923) |
| tech-stack §83 `Check{Name();Run(context.Context) Result}` + nine check names | HOLDS (L2057–L2076) |
| tech-stack §84 five health states + "Optional missing capabilities must not all be shown as fatal errors." | HOLDS (L2093–L2103) |
| tech-stack §85/§86/§87/§88 wiring, Start/Run/Shutdown, the ten-step sequence, shutdown sequence | HOLDS (L2109–L2189) |
| tech-stack §111/§112/§113 embed targets, `<GIT_COMMON_DIR>/mindrail/{mindrail.db,cache/}`, temp file rules | HOLDS (L2600–L2648) |
| tech-stack §131/§132/§136 injectable runtime/cache roots, `fixture-git-worktree`, "binary smoke tests pass" | HOLDS (L2925–L2953, L3040) |
| tech-stack §159 first binary = init/status/doctor proving Git discovery, runtime paths, SQLite creation, embedded migrations, knowledge loading, workspace registration, structured CLI output | HOLDS (L3464–L3482) |
| spec §8 `.mindrail/` tree labelled "Önerilen 1.0 yapı"; runtime DB/cache under `<GIT_COMMON_DIR>/mindrail/` | HOLDS (L317–L354) |
| spec §10 four pragmas + "Uzun süren hiçbir iş SQLite write transaction açıkken yapılmaz." | HOLDS (L400–L411) — **but the pragmas are introduced as "Önerilen ayarlar" (recommended), not MUST.** Reader 4 flagged this correctly. |
| spec §82 thirteen init steps, "Existing config sessizce overwrite edilmez.", `READY FOR TARGETED WORK` / `BLOCKED: <reason>` | HOLDS (L3484–L3516) |
| spec §83 doctor report example, `mindrail doctor --explain <error-code>` under "gibi scoped diagnostics **destekleyebilir**", "Doctor project config'i kendi kendine değiştirmez" | HOLDS (L3524–L3583) — the `--explain` verb is permissive, as reader 2 said |
| spec §84 `code / why / impact / next_action` + the `EVIDENCE_STALE` / `Why:` / `Impact:` / `Next:` block | HOLDS (L3589–L3614) |
| spec §107 six components + four readiness values | HOLDS (L4297–L4334) |
| spec §113 "repo path containment, symlink escape deny, parameterized SQL, … environment isolation, … no arbitrary shell interpolation" | HOLDS (L4437–L4451) |
| kernel-scope §3 `write_schema_version = 1`, `readable_schema_versions = [1]`, unknown newer fails closed; §4 out-of-scope list; §5 SLO table; §6 AC-01 | HOLDS (L77–L84, L255–L280, L283–L299, L305) |

Corrections applied to the handed-over contract:

1. **`spec §129 AC-13` re-verified and kept.** Reader 1 cited it for "init does not
   wait for the cold index"; the section really does say *"100k+ file repository'de
   `mindrail init` inventory/knowledge sonrası PARTIAL_READY dönebilir; full cold
   index'i beklemek zorunda değildir."* Note the wording: PARTIAL_READY is reached
   **after inventory**, which MR-001 does not perform. This drives OQ-02.
2. **Spec §10 pragmas are "recommended", not mandated.** Promoted to MUST for MR-001
   by decision D-22 below, not by citation.
3. **Reader 1's obligation `stored-paths-repo-relative-slash` is narrowed.**
   tech-stack §74 governs *repository content paths*; a worktree root and a Git
   common-dir are inherently absolute machine paths. MR-001 stores no repository
   content paths at all. Decision D-26 records how this is honoured.
4. **Reader 1's `embedded-migrations-idempotent` "byte-identical applied-at"
   assertion is kept but re-stated**: the assertion is that the row set including
   `applied_at` is unchanged after a second run, which follows from applying nothing.
5. **Reader 2's claim that spec §19 forbids dumping secrets is confirmed**
   ("Secret/environment değerleri dump edilmez.", spec L1141).
6. No obligation was dropped as unsupported.

---

## 1. Package plan

`.docs/mindrail-tech-stack.md` §7 is the naming authority. Every package below
except one is named there.

| Package | Directory | Responsibility (one line) | Unit |
|---|---|---|---|
| `app` | `internal/app/` | Process-level leaf: exit-code classification, structured domain error + code registry, `Clock`, JSON envelope, colour policy. **Imports no other internal package.** | D |
| `filesystem` | `internal/filesystem/` | Path cleaning/containment/symlink policy, repo-relative slash normalization, runtime path resolution under the Git common-dir. | A |
| `git` | `internal/git/` | argv-only `os/exec` adapter over the system `git`; repository / common-dir / worktree resolution. | A |
| `storage` | `internal/storage/` | `database/sql` handle for the runtime DB: open, pragmas, integrity, tx helper; the only package importing a SQLite driver. | B |
| `migration` | `internal/migration/` | Ordered, forward-only, checksum-tracked embedded SQL migrations over `schema_migrations`. | B |
| `migrations` | `migrations/` | `embed.FS` of the numbered `.sql` files (§7 puts them at repo root; `go:embed` cannot reach up, so the FS declaration lives beside them). | B |
| `workspace` | `internal/workspace/` | Project + workspace rows: idempotent registration of the active worktree. | B |
| `config` | `internal/config/` | Strict TOML decode, five-layer precedence, `MINDRAIL_*` env mapping, provenance, `.mindrail/config.toml` scaffolding from an embedded template. | C |
| `schema` | `internal/knowledge/schema/` | Embedded knowledge JSON Schema registry + the `write`/`readable` version window. | C |
| `schemas` | `schemas/` | `embed.FS` of `schemas/knowledge/*.json` (same `go:embed` reach constraint). | C |
| `loader` | `internal/knowledge/loader/` | Read `.mindrail/knowledge`, count records, report per-record problems and the effective schema window. | C |
| `doctor` | `internal/doctor/` | Health-state enum, `Check`/`Result`/`Runner`/`Report`, the concrete MR-001 checks, human + JSON rendering. | D |
| `status` | `internal/status/` | Readiness enum, six-component health map, `status` and `init` report shapes and rendering. **The one package not named in §7** — see D-17. | D |
| `bootstrap` | `internal/bootstrap/` | The §87 ten-step startup sequence, the §86 `App` lifecycle (Start/Run/Shutdown), lazy managers. | E |
| `cli` | `internal/cli/` | cobra command tree; flags → application services → rendered output. No domain logic. | E |

Not created by MR-001 (reserved for their owning MR): `internal/project`,
`internal/session`, `internal/task`, `internal/change`, `internal/lease`,
`internal/checkpoint`, `internal/index/**`, `internal/semantic/**`,
`internal/coverage/**`, `internal/impact/**`, `internal/validation/**`,
`internal/evidence`, `internal/approval`, `internal/reconcile`,
`internal/telemetry`, `internal/mcp`, `internal/knowledge/{decision,invariant}`.

### 1.1 Why `internal/status` is added to §7

§7 is introduced as a *"Recommended structure"*, and §8 tells us to prefer domain
packages over technical buckets. The readiness model (spec §107) is shared by
`status` and `init`, is not a doctor check, and cannot go under `internal/index/`
because MR-005 owns that tree and MR-001 builds no index. `internal/status` is the
smallest honest home. It is the only name invented here.

---

## 2. Public API per package

Signatures below are the compile contract. A unit may add unexported helpers
freely; it may not change an exported signature without amending this document.

### 2.1 `internal/app` — Unit D

`internal/app/exit.go` keeps `ExitSuccess…ExitUnavailable`, `Kind`, `Error`,
`Failed/Usage/Denied/Unavailable`. `ExitCode` gains a `*DomainError` branch **before**
the `*Error` branch.

```go
// internal/app/code.go
type Code string

const (
	CodeNotAGitRepository    Code = "NOT_A_GIT_REPOSITORY"
	CodeBareRepository       Code = "BARE_REPOSITORY_UNSUPPORTED"
	CodeGitUnavailable       Code = "GIT_UNAVAILABLE"
	CodeGitTimeout           Code = "GIT_TIMEOUT"
	CodePathEscapesRoot      Code = "PATH_ESCAPES_ROOT"
	CodeRuntimePathUnwritable Code = "RUNTIME_PATH_UNWRITABLE"
	CodeRuntimeDBUnavailable Code = "RUNTIME_DB_UNAVAILABLE"
	CodeRuntimeDBCorrupt     Code = "RUNTIME_DB_CORRUPT"
	CodeRuntimeDBSchemaTooNew Code = "RUNTIME_DB_SCHEMA_TOO_NEW"
	CodeMigrationFailed      Code = "MIGRATION_FAILED"
	CodeMigrationChecksumMismatch Code = "MIGRATION_CHECKSUM_MISMATCH"
	CodeWorkspaceNotInitialized Code = "WORKSPACE_NOT_INITIALIZED"
	CodeWorkspaceRegistrationFailed Code = "WORKSPACE_REGISTRATION_FAILED"
	CodeConfigInvalid        Code = "CONFIG_INVALID"
	CodeConfigUnknownEnvVar  Code = "CONFIG_UNKNOWN_ENV_VAR"
	CodeKnowledgeUnreadable  Code = "KNOWLEDGE_UNREADABLE"
	CodeKnowledgeSchemaUnsupported Code = "KNOWLEDGE_SCHEMA_UNSUPPORTED"
)

// RegisteredCodes returns every code this binary may emit, sorted.
func RegisteredCodes() []Code
func IsRegistered(c Code) bool
```

```go
// internal/app/errors.go
type DomainError struct {
	Code       Code
	Kind       Kind
	Why        string
	Impact     string
	NextAction []string
	Metadata   map[string]string
	Cause      error
}

func NewError(code Code, kind Kind, why, impact string, next ...string) *DomainError
func (e *DomainError) Error() string   // "CODE: why"
func (e *DomainError) Unwrap() error
func (e *DomainError) WithCause(err error) *DomainError
func (e *DomainError) WithMetadata(key, value string) *DomainError
func (e *DomainError) Payload() ErrorPayload

type ErrorPayload struct {
	Code       Code              `json:"code"`
	Why        string            `json:"why"`
	Impact     string            `json:"impact"`
	NextAction []string          `json:"next_action"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

// PayloadOf recovers the payload from anywhere in the wrap chain (errors.As).
func PayloadOf(err error) (ErrorPayload, bool)

// RenderError writes the spec §84 human block: bare code line, "Why:",
// "Impact:", "Next:" in that order.
func RenderError(w io.Writer, err error) error
```

```go
// internal/app/output.go
type Envelope struct {
	Command  string        `json:"command"`
	OK       bool          `json:"ok"`
	Data     any           `json:"data,omitempty"`
	Error    *ErrorPayload `json:"error,omitempty"`
	Warnings []Warning     `json:"warnings,omitempty"`
}

type Warning struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

// WriteJSON emits exactly one Envelope followed by "\n". Never emits ANSI.
func WriteJSON(w io.Writer, command string, data any, warnings []Warning, err error) error

// ColorEnabled implements the §12 rule: false when jsonMode, when NO_COLOR is
// present in environ, when colorPref == "never", or when w is not a terminal.
func ColorEnabled(w io.Writer, environ []string, colorPref string, jsonMode bool) bool
```

```go
// internal/app/clock.go
type Clock interface{ Now() time.Time }

type SystemClock struct{}
func (SystemClock) Now() time.Time            // time.Now().UTC()

type FixedClock struct{ Instant time.Time }
func (c FixedClock) Now() time.Time           // c.Instant.UTC()

func FormatTime(t time.Time) string           // RFC3339Nano, forced UTC
func ParseTime(s string) (time.Time, error)

const ShutdownTimeout = 5 * time.Second       // tech-stack §88 "bounded time"
```

### 2.2 `internal/filesystem` — Unit A

```go
const (
	ProductName = "mindrail"
	DirMode  os.FileMode = 0o700
	FileMode os.FileMode = 0o600
)

// Root is a canonicalized (EvalSymlinks + Abs + Clean) containment boundary.
type Root struct{ /* unexported canonical string */ }

func NewRoot(dir string) (Root, error)
func (r Root) Path() string
func (r Root) Resolve(rel string) (string, error)   // containment-checked absolute
func (r Root) Contains(abs string) (bool, error)
func (r Root) RelSlash(abs string) (string, error)  // repo-relative, "/" separators
func (r Root) EnsureDir(rel string) (string, error)
func (r Root) WriteFileIfAbsent(rel string, data []byte) (written bool, path string, err error)

// Normalize converts an OS-native relative path to the stored slash form.
func Normalize(p string) string

var (
	ErrEscapesRoot  = errors.New("path escapes the allowed root")
	ErrNotDirectory = errors.New("path is not a directory")
)

// RuntimePaths are derived from the Git common-dir (tech-stack §112).
type RuntimePaths struct {
	CommonDir     string `json:"common_dir"`
	WorktreeRoot  string `json:"worktree_root"`
	RuntimeRoot   string `json:"runtime_root"`    // <CommonDir>/mindrail
	DBPath        string `json:"db_path"`         // <RuntimeRoot>/mindrail.db
	CacheDir      string `json:"cache_dir"`       // <RuntimeRoot>/cache
	RepoConfigDir string `json:"repo_config_dir"` // <WorktreeRoot>/.mindrail
}

type PathOptions struct {
	CommonDir          string
	WorktreeRoot       string
	RuntimeDirOverride string // MINDRAIL_RUNTIME_DIR / test injection
	CacheDirOverride   string // MINDRAIL_CACHE_DIR / test injection
}

func ResolveRuntimePaths(o PathOptions) (RuntimePaths, error)
func (p RuntimePaths) EnsureDirs() error  // 0700, no-op when present
func (p RuntimePaths) Exists() bool       // DBPath stat
```

### 2.3 `internal/git` — Unit A

```go
const DefaultTimeout = 10 * time.Second

type CommandRunner interface {
	Run(ctx context.Context, dir string, args ...string) (stdout, stderr []byte, err error)
}

type ExecRunner struct {
	Bin     string        // "" => "git"
	Timeout time.Duration // 0 => DefaultTimeout
}

func NewExecRunner() *ExecRunner
func (r *ExecRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error)

// SanitizedEnv strips inherited GIT_DIR, GIT_WORK_TREE, GIT_COMMON_DIR,
// GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY, GIT_ALTERNATE_OBJECT_DIRECTORIES and
// GIT_NAMESPACE, then sets GIT_OPTIONAL_LOCKS=0, GIT_TERMINAL_PROMPT=0, LC_ALL=C.
func SanitizedEnv(parent []string) []string

type Invocation struct {
	Dir  string
	Args []string
}

// FakeRunner is the shared test double; Unit A ships it in the non-test build so
// every other unit can drive the adapter deterministically.
type FakeRunner struct {
	Responses map[string]FakeResponse // key = strings.Join(args, " ")
	Default   FakeResponse
	Calls     []Invocation
}
type FakeResponse struct {
	Stdout string
	Stderr string
	Err    error
}
func (f *FakeRunner) Run(ctx context.Context, dir string, args ...string) ([]byte, []byte, error)

type Repository struct {
	CommonDir        string `json:"common_dir"`         // absolute
	GitDir           string `json:"git_dir"`            // absolute, per-worktree
	WorktreeRoot     string `json:"worktree_root"`      // absolute
	IsLinkedWorktree bool   `json:"is_linked_worktree"` // GitDir != CommonDir
	IsBare           bool   `json:"is_bare"`
}

type Adapter struct{ /* runner */ }
func NewAdapter(runner CommandRunner) *Adapter

// Resolve runs, in order and via argv only:
//   rev-parse --is-inside-work-tree
//   rev-parse --path-format=absolute --git-common-dir
//   rev-parse --path-format=absolute --git-dir
//   rev-parse --path-format=absolute --show-toplevel
//   rev-parse --is-bare-repository
func (a *Adapter) Resolve(ctx context.Context, startDir string) (Repository, error)
func (a *Adapter) Version(ctx context.Context) (string, error) // "git --version"

var (
	ErrNotARepository = errors.New("not inside a git repository")
	ErrBareRepository = errors.New("bare repository has no worktree")
	ErrGitUnavailable = errors.New("git executable not available")
	ErrGitTimeout     = errors.New("git command timed out")
)
```

`Resolve` wraps its sentinels in `*app.DomainError` (`NOT_A_GIT_REPOSITORY` /
`BARE_REPOSITORY_UNSUPPORTED` / `GIT_UNAVAILABLE` / `GIT_TIMEOUT`) so that both
`errors.Is(err, git.ErrNotARepository)` and `app.PayloadOf(err)` work.

### 2.4 `internal/storage` — Unit B

```go
type Options struct {
	Path        string        // absolute mindrail.db path
	BusyTimeout time.Duration // 0 => 5s
	ReadOnly    bool          // status/doctor
	MaxOpenConns int          // 0 => 4
}

type DB struct{ *sql.DB }

func Open(ctx context.Context, opts Options) (*DB, error)
func (db *DB) Path() string
func (db *DB) Close() error

type Pragmas struct {
	JournalMode string `json:"journal_mode"` // "wal"
	ForeignKeys int    `json:"foreign_keys"` // 1
	BusyTimeout int    `json:"busy_timeout"` // 5000
	Synchronous int    `json:"synchronous"`  // 1
}

type Querier interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}

func ReadPragmas(ctx context.Context, q Querier) (Pragmas, error)
func ExpectedPragmas(busyTimeout time.Duration) Pragmas
func IntegrityCheck(ctx context.Context, q Querier) (string, error) // "ok" when healthy

// InTx opens a short write transaction. TxActive reports whether the calling
// goroutine currently holds one; the git/filesystem probes assert it is false.
func InTx(ctx context.Context, db *sql.DB, fn func(context.Context, *sql.Tx) error) error
func TxActive(ctx context.Context) bool

var (
	ErrOpenFailed = errors.New("runtime database could not be opened")
	ErrCorrupt    = errors.New("runtime database is corrupt")
	ErrPragma     = errors.New("runtime database pragma not in effect")
)
```

`internal/storage/driver.go` is the **only** file in the repository importing
`modernc.org/sqlite`. It exports nothing but `func dsn(Options) string` (package
private) and the blank driver import.

### 2.5 `migrations/` + `internal/migration` — Unit B

```go
// migrations/embed.go
package migrations

//go:embed *.sql
var FS embed.FS
```

Files: `migrations/000001_initial.sql` only, in MR-001.

```sql
-- migrations/000001_initial.sql
CREATE TABLE projects (
    project_id    TEXT PRIMARY KEY,
    common_dir    TEXT NOT NULL UNIQUE,
    registered_at TEXT NOT NULL
) STRICT;

CREATE TABLE workspaces (
    workspace_id       TEXT PRIMARY KEY,
    project_id         TEXT NOT NULL REFERENCES projects(project_id),
    root_path          TEXT NOT NULL UNIQUE,
    git_dir            TEXT NOT NULL,
    is_linked_worktree INTEGER NOT NULL,
    registered_at      TEXT NOT NULL,
    last_seen_at       TEXT NOT NULL
) STRICT;

CREATE INDEX idx_workspaces_project ON workspaces(project_id);
```

`schema_migrations` is bootstrap DDL owned by the migrator, not by a migration file:

```sql
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    name       TEXT NOT NULL,
    checksum   TEXT NOT NULL,
    applied_at TEXT NOT NULL
);
```

```go
// internal/migration
type Migration struct {
	Version  int64
	Name     string
	SQL      string
	Checksum string // hex sha256 of SQL
}

// Load parses `^([0-9]{6})_([a-z0-9_]+)\.sql$`, sorts ascending, rejects
// duplicate versions and unparseable names.
func Load(fsys fs.FS) ([]Migration, error)

type Applied struct {
	Version   int64     `json:"version"`
	Name      string    `json:"name"`
	Checksum  string    `json:"checksum"`
	AppliedAt time.Time `json:"applied_at"`
}

type Result struct {
	Applied        []Applied `json:"applied"`         // this run only; empty on re-run
	CurrentVersion int64     `json:"current_version"`
	Total          int       `json:"total"`
}

type Migrator struct{ /* db, set, clock */ }
func New(db *sql.DB, set []Migration, clock app.Clock) *Migrator

// Up applies each pending migration inside its own BEGIN IMMEDIATE transaction
// together with its schema_migrations row. Forward-only. No downgrade entry point.
func (m *Migrator) Up(ctx context.Context) (Result, error)
func (m *Migrator) Status(ctx context.Context) ([]Applied, error)
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error)

var (
	ErrApplyFailed      = errors.New("migration failed")
	ErrChecksumMismatch = errors.New("applied migration content changed")
	ErrSchemaAhead      = errors.New("database schema is newer than this binary")
)
```

### 2.6 `internal/workspace` — Unit B

```go
type Project struct {
	ID           string    `json:"project_id"`  // "PRJ-<26>"
	CommonDir    string    `json:"common_dir"`
	RegisteredAt time.Time `json:"registered_at"`
}

type Workspace struct {
	ID               string    `json:"workspace_id"` // "WS-<26>"
	ProjectID        string    `json:"project_id"`
	RootPath         string    `json:"root_path"`
	GitDir           string    `json:"git_dir"`
	IsLinkedWorktree bool      `json:"is_linked_worktree"`
	RegisteredAt     time.Time `json:"registered_at"`
	LastSeenAt       time.Time `json:"last_seen_at"`
}

type Registration struct {
	CommonDir        string
	WorktreeRoot     string
	GitDir           string
	IsLinkedWorktree bool
}

type Store struct{ /* db, clock */ }
func NewStore(db *sql.DB, clock app.Clock) *Store

// Register is idempotent: same worktree root => same workspace row, LastSeenAt bumped.
func (s *Store) Register(ctx context.Context, r Registration) (Project, Workspace, error)
func (s *Store) FindByRoot(ctx context.Context, root string) (Workspace, error)
func (s *Store) List(ctx context.Context) ([]Workspace, error)
func (s *Store) Count(ctx context.Context) (int, error)

// NewID returns an opaque, time-sortable identifier: prefix + "-" + 26 chars.
func NewID(prefix string) string

var ErrNotRegistered = errors.New("workspace is not registered")
```

### 2.7 `internal/config` — Unit C

```go
const (
	RepoDir        = ".mindrail"
	ConfigFileName = "config.toml"
	EnvPrefix      = "MINDRAIL_"
)

type Config struct {
	Project ProjectConfig `toml:"project" json:"project"`
	Output  OutputConfig  `toml:"output"  json:"output"`
	Runtime RuntimeConfig `toml:"-"       json:"runtime"` // env/flag only, never from TOML
}
type ProjectConfig struct {
	Name string `toml:"name" json:"name"`
}
type OutputConfig struct {
	Color string `toml:"color" json:"color"` // auto | always | never
}
type RuntimeConfig struct {
	Dir      string `json:"dir"`       // MINDRAIL_RUNTIME_DIR
	CacheDir string `json:"cache_dir"` // MINDRAIL_CACHE_DIR
}

type Source string
const (
	SourceDefault Source = "default"
	SourceUser    Source = "user"
	SourceRepo    Source = "repo"
	SourceEnv     Source = "env"
	SourceFlag    Source = "flag"
)

type Provenance map[string]Source // key: dotted path, e.g. "output.color"

type Loaded struct {
	Config     Config        `json:"config"`
	Provenance Provenance    `json:"provenance"`
	Warnings   []app.Warning `json:"warnings,omitempty"`
	RepoFile   string        `json:"repo_file,omitempty"`
	UserFile   string        `json:"user_file,omitempty"`
}

type LoaderOptions struct {
	WorktreeRoot  string
	UserConfigDir string            // injectable; "" => os.UserConfigDir()
	Environ       []string          // injectable; nil => os.Environ()
	Flags         map[string]string // dotted key => raw value
}

type Loader struct{ /* opts */ }
func NewLoader(o LoaderOptions) *Loader
func (l *Loader) Load() (Loaded, error)

func Defaults() Config
func (c Config) Validate() error

// DefaultTemplate is the embedded .mindrail/config.toml scaffold (tech-stack §111).
func DefaultTemplate() []byte

// WriteIfAbsent creates <worktreeRoot>/.mindrail/config.toml from the template.
// It never overwrites (spec §82) and reports which happened.
func WriteIfAbsent(worktreeRoot string) (path string, written bool, err error)

// EnsureKnowledgeDirs creates .mindrail/knowledge/{decisions,invariants}/ each
// with a .gitkeep. Returns the repo-relative dirs it actually created.
func EnsureKnowledgeDirs(worktreeRoot string) (created []string, err error)

var ErrUnknownKey = errors.New("unknown configuration key")
```

### 2.8 `schemas/` + `internal/knowledge/schema` — Unit C

```go
// schemas/embed.go
package schemas

//go:embed knowledge/*.json
var KnowledgeFS embed.FS
```

Files: `schemas/knowledge/decision.v1.schema.json`,
`schemas/knowledge/invariant.v1.schema.json` (Draft 2020-12; MR-001 ships the
documents and the version window, MR-002 compiles and enforces them).

```go
// internal/knowledge/schema
const WriteVersion = 1

func ReadableVersions() []int // {1}

type Registry struct{ /* fsys, docs */ }
func NewRegistry(fsys fs.FS) (*Registry, error) // errors when a document is missing/unparseable
func (r *Registry) Names() []string
func (r *Registry) Document(name string) ([]byte, bool)
func (r *Registry) WriteVersion() int
func (r *Registry) ReadableVersions() []int
func (r *Registry) Supports(version int) bool

var ErrUnsupportedVersion = errors.New("knowledge schema_version not readable by this binary")
```

### 2.9 `internal/knowledge/loader` — Unit C

```go
type RecordKind string
const (
	KindDecision  RecordKind = "decision"
	KindInvariant RecordKind = "invariant"
)

type RecordRef struct {
	Kind          RecordKind `json:"kind"`
	ID            string     `json:"id"`
	Path          string     `json:"path"`           // repo-relative, slash
	SchemaVersion int        `json:"schema_version"`
}

type Problem struct {
	Path    string   `json:"path"` // repo-relative, slash
	Code    app.Code `json:"code"`
	Message string   `json:"message"`
	Fatal   bool     `json:"fatal"` // true => unreadable schema_version
}

type Store struct {
	Present                bool        `json:"present"`
	Root                   string      `json:"root"` // ".mindrail/knowledge"
	Decisions              []RecordRef `json:"decisions"`
	Invariants             []RecordRef `json:"invariants"`
	Problems               []Problem   `json:"problems"`
	WriteSchemaVersion     int         `json:"write_schema_version"`
	ReadableSchemaVersions []int       `json:"readable_schema_versions"`
}

func (s Store) Count() int
func (s Store) HasFatalProblem() bool

type Loader struct{ /* root, registry */ }
func New(root filesystem.Root, reg *schema.Registry) *Loader

// Load performs spec §95 steps 1-4 only: read file, parse JSON syntax, read
// schema_version, check the reader window. Steps 5-14 belong to MR-002.
func (l *Loader) Load(ctx context.Context) (Store, error)
```

Behaviour: absent tree ⇒ `Present=false`, zero records, `err == nil`.
Malformed JSON ⇒ non-fatal `Problem` (`KNOWLEDGE_UNREADABLE`).
Unknown newer `schema_version` ⇒ **fatal** `Problem`
(`KNOWLEDGE_SCHEMA_UNSUPPORTED`), which drives readiness to `BLOCKED`.

### 2.10 `internal/doctor` — Unit D

```go
type State string
const (
	StateOK            State = "OK"
	StateDegraded      State = "DEGRADED"
	StateError         State = "ERROR"
	StateUnavailable   State = "UNAVAILABLE"
	StateNotApplicable State = "NOT_APPLICABLE"
)
func States() []State
func (s State) Valid() bool
func (s State) Marker() string                 // ✓ △ ✗ – ·
func (s *State) UnmarshalJSON(b []byte) error  // rejects anything outside States()

type Result struct {
	Name       string            `json:"name"`
	Section    string            `json:"section"`
	State      State             `json:"state"`
	Summary    string            `json:"summary"`
	Diagnostic string            `json:"diagnostic,omitempty"`
	Impact     string            `json:"impact,omitempty"`
	NextAction []string          `json:"next_action,omitempty"`
	Code       app.Code          `json:"code,omitempty"`
	Details    map[string]string `json:"details,omitempty"`
	DurationMS int64             `json:"duration_ms"`
}

type Check interface {
	Name() string
	Run(ctx context.Context) Result
}

// CheckFunc adapts a function to Check (used by tests and by DefaultChecks).
type CheckFunc struct {
	CheckName    string
	CheckSection string
	Fn           func(context.Context) Result
}
func (c CheckFunc) Name() string
func (c CheckFunc) Run(ctx context.Context) Result

// Subject is the already-resolved bootstrap state that checks report on.
// Checks are pure functions of Subject: they open nothing and mutate nothing.
type Subject struct {
	StartDir     string
	GitVersion   string
	Repo         git.Repository
	RepoErr      error
	Paths        filesystem.RuntimePaths
	PathsErr     error
	Config       config.Loaded
	ConfigErr    error
	DBPresent    bool
	DBErr        error
	Pragmas      storage.Pragmas
	IntegrityErr error
	Migrations   []migration.Applied
	PendingCount int
	MigrateErr   error
	Knowledge    loader.Store
	KnowledgeErr error
	Workspace    workspace.Workspace
	WorkspaceErr error
}

// DefaultChecks returns the MR-001 check set, in report order:
// GitCheck, RuntimePathCheck, ConfigCheck, SQLiteCheck, MigrationCheck,
// KnowledgeCheck, WorkspaceCheck.
func DefaultChecks(s Subject) []Check

type Runner struct{ /* checks */ }
func NewRunner(checks ...Check) *Runner
func (r *Runner) Register(c Check)
func (r *Runner) Names() []string
func (r *Runner) Run(ctx context.Context) Report

type Report struct {
	Command    string   `json:"command"` // "doctor"
	Checks     []Result `json:"checks"`
	WorstState State    `json:"worst_state"`
	DurationMS int64    `json:"duration_ms"`
}

// Err returns the app.DomainError of the first ERROR check, or nil.
// DEGRADED / UNAVAILABLE / NOT_APPLICABLE never produce an error.
func (r Report) Err() error
func (r Report) RenderHuman(w io.Writer, color bool) error
```

### 2.11 `internal/status` — Unit D

```go
type Readiness string
const (
	ReadinessPartialReady Readiness = "PARTIAL_READY"
	ReadinessReady        Readiness = "READY"
	ReadinessDegraded     Readiness = "DEGRADED"
	ReadinessBlocked      Readiness = "BLOCKED"
)
func Readinesses() []Readiness
func (r Readiness) Valid() bool
func (r *Readiness) UnmarshalJSON(b []byte) error

type ComponentName string
const (
	ComponentKnowledge   ComponentName = "knowledge"
	ComponentInventory   ComponentName = "inventory"
	ComponentSyntax      ComponentName = "syntax"
	ComponentSemantic    ComponentName = "semantic"
	ComponentCoverageMap ComponentName = "coverage_map"
	ComponentRuntimeDB   ComponentName = "runtime_db"
)
func Components() []ComponentName // exactly the six of spec §107, in that order

type Component struct {
	State      doctor.State `json:"state"`
	Summary    string       `json:"summary"`
	Code       app.Code     `json:"code,omitempty"`
	NextAction []string     `json:"next_action,omitempty"`
}

type RepositoryInfo struct {
	CommonDir        string `json:"common_dir"`
	WorktreeRoot     string `json:"worktree_root"`
	GitDir           string `json:"git_dir"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
	GitVersion       string `json:"git_version,omitempty"`
}
type RuntimeInfo struct {
	DBPath        string `json:"db_path"`
	CacheDir      string `json:"cache_dir"`
	SchemaVersion int64  `json:"schema_version"`
	JournalMode   string `json:"journal_mode,omitempty"`
	Initialized   bool   `json:"initialized"`
}
type KnowledgeInfo struct {
	Present                bool  `json:"present"`
	Decisions              int   `json:"decisions"`
	Invariants             int   `json:"invariants"`
	Problems               int   `json:"problems"`
	WriteSchemaVersion     int   `json:"write_schema_version"`
	ReadableSchemaVersions []int `json:"readable_schema_versions"`
}
type WorkspaceInfo struct {
	Registered bool   `json:"registered"`
	ID         string `json:"workspace_id,omitempty"`
	ProjectID  string `json:"project_id,omitempty"`
}

type Report struct {
	Command           string                      `json:"command"` // "status"
	Readiness         Readiness                   `json:"readiness"`
	BlockingComponent ComponentName               `json:"blocking_component,omitempty"`
	NextAction        []string                    `json:"next_action,omitempty"`
	Components        map[ComponentName]Component `json:"components"`
	Repository        RepositoryInfo              `json:"repository"`
	Runtime           RuntimeInfo                 `json:"runtime"`
	Knowledge         KnowledgeInfo               `json:"knowledge"`
	Workspace         WorkspaceInfo               `json:"workspace"`
	DurationMS        int64                       `json:"duration_ms"`
}

func Build(s doctor.Subject, elapsed time.Duration) Report
func (r Report) RenderHuman(w io.Writer, color bool) error

type TerminalState string
const (
	TerminalReady   TerminalState = "READY FOR TARGETED WORK"
	TerminalBlocked TerminalState = "BLOCKED"
)

type InitReport struct {
	Command              string              `json:"command"` // "init"
	TerminalState        TerminalState       `json:"terminal_state"`
	Reason               string              `json:"reason,omitempty"`
	ConfigPath           string              `json:"config_path"`
	ConfigCreated        bool                `json:"config_created"`
	KnowledgeDirsCreated []string            `json:"knowledge_dirs_created"`
	MigrationsApplied    []migration.Applied `json:"migrations_applied"`
	Status               Report              `json:"status"`
	DurationMS           int64               `json:"duration_ms"`
}

func (r InitReport) RenderHuman(w io.Writer, color bool) error
```

Human `init` output ends with the literal `READY FOR TARGETED WORK` or
`BLOCKED: <reason>` (spec §82).

### 2.12 `internal/bootstrap` — Unit E

```go
type Step string
const (
	StepResolveRepository   Step = "resolve_repository"
	StepLoadConfig          Step = "load_config"
	StepResolveRuntimePaths Step = "resolve_runtime_paths"
	StepOpenSQLite          Step = "open_sqlite"
	StepMigrateDB           Step = "migrate_db"
	StepValidateKnowledge   Step = "validate_knowledge"
	StepRegisterWorkspace   Step = "register_workspace"
	StepLoadIndexState      Step = "load_index_state"
	StepInitManagers        Step = "init_managers"
	StepExecuteCommand      Step = "execute_command"
)
func Steps() []Step // the ten of tech-stack §87, in order

type Recorder interface{ Step(s Step) }

type Mode int
const (
	ModeReadOnly Mode = iota // status, doctor, version: never creates or migrates
	ModeInit                 // init: the only writer
)

type Options struct {
	StartDir           string
	Mode               Mode
	Runner             git.CommandRunner // nil => git.NewExecRunner()
	Clock              app.Clock         // nil => app.SystemClock{}
	Logger             *slog.Logger      // nil => slog.New(discard)
	Environ            []string          // nil => os.Environ()
	Flags              map[string]string
	UserConfigDir      string
	RuntimeDirOverride string
	CacheDirOverride   string
	MigrationFS        fs.FS // nil => migrations.FS
	SchemaFS           fs.FS // nil => schemas.KnowledgeFS
	Recorder           Recorder
}

type App struct{ /* unexported */ }

func New(o Options) *App

// Start executes steps 1-9. It stops at the first hard failure and returns a
// *app.DomainError; partial state is still reachable through Subject().
func (a *App) Start(ctx context.Context) error
func (a *App) Run(ctx context.Context, fn func(context.Context, *App) error) error
func (a *App) Shutdown(ctx context.Context) error // idempotent, bounded by app.ShutdownTimeout

func (a *App) Subject() doctor.Subject
func (a *App) Doctor() *doctor.Runner  // lazily constructed, at most once
func (a *App) DB() *sql.DB             // nil in ModeReadOnly when uninitialized
func (a *App) Repo() git.Repository
func (a *App) Paths() filesystem.RuntimePaths
func (a *App) Config() config.Loaded
func (a *App) Warnings() []app.Warning
func (a *App) ManagerInitCount() int   // test seam for the laziness assertion

// InitResult carries what only ModeInit produces.
type InitResult struct {
	ConfigPath           string
	ConfigCreated        bool
	KnowledgeDirsCreated []string
	MigrationsApplied    []migration.Applied
}
func (a *App) InitResult() InitResult
```

### 2.13 `internal/cli` — Unit E

```go
func NewRootCommand() *cobra.Command      // existing, gains subcommands + persistent flags
func Execute(ctx context.Context) error   // existing

func newInitCommand() *cobra.Command
func newStatusCommand() *cobra.Command
func newDoctorCommand() *cobra.Command
func newVersionCommand() *cobra.Command

// Persistent flags on root: --json, --verbose, --no-color, -C <dir>.
type globalFlags struct {
	json    bool
	verbose bool
	noColor bool
	chdir   string
}
```

`cmd/mindrail/main.go` is unchanged except that it now also drains the app on the
error path; its import set stays `{fmt, os, internal/app, internal/cli}`.

---

## 3. Dependency direction

```
                       cmd/mindrail (main)
                          │        │
                          ▼        ▼
                   internal/cli   internal/app
                          │
                          ▼
                  internal/bootstrap
             ┌────────┬───────┴────┬──────────┬─────────────┐
             ▼        ▼            ▼          ▼             ▼
        internal/  internal/  internal/   internal/    internal/
         status     doctor    workspace   migration     config
             │        │            │          │             │
             └───┬────┘            │          │             │
                 ▼                 │          │             │
        ┌────────┴────────┬────────┴───┐      │             │
        ▼                 ▼            ▼      ▼             ▼
  internal/git   internal/knowledge/  internal/storage   (schemas/, migrations/)
        │             loader                │
        │                │                  │
        ▼                ▼                  ▼
        └──────►  internal/filesystem ◄──────┘
                          │
                          ▼
                    internal/app          (leaf: imports only stdlib)
```

Rules enforced by architecture tests:

| Rule | Enforcement |
|---|---|
| `internal/app` imports no other `internal/…` package | `TestAppPackageIsALeaf` |
| package `main` imports only stdlib + `internal/app` + `internal/cli` | `TestMainImportsAreMinimal` |
| `modernc.org/sqlite` reachable only from `internal/storage` | `TestDriverImportConfinedToStorage` |
| no ORM (`gorm`, `ent`, `sqlboiler`) and no DI framework (`wire`, `fx`, `dig`) in `go.mod` | `TestForbiddenDependencies` |
| no `go-git` anywhere | `TestNoGoGitDependency` |
| `net/http` unreachable from the init path | `TestInitPathDoesNotImportNetHTTP` |
| `internal/doctor` and `internal/status` never imported by `internal/git`, `internal/storage`, `internal/filesystem` | `TestNoUpwardImports` |

The graph is acyclic: `app` and `filesystem` are the sinks, `bootstrap` is the only
package that knows all of them, `cli` knows only `bootstrap`, `status`, `doctor`
and `app`.

---

## 4. Work units

Five units. **A, B, C, D run in parallel; E lands last.** No file appears in two
units. `go.mod` / `go.sum` / `Makefile` are owned exclusively by Unit E.

### Unit-D T0 handshake

Unit D's *first* commit is `internal/app/{code.go,errors.go,output.go,clock.go}`
plus the `ExitCode` amendment in `exit.go`. Units A, B and C import `app` from
minute one; their code compiles against §2.1 as written here, so they need not
wait — but nothing merges before D's T0 commit.

### Unit A — Repository & filesystem

Owns `internal/filesystem/**` and `internal/git/**`.

| File | Contents |
|---|---|
| `internal/filesystem/root.go` | `Root`, `NewRoot`, `Resolve`, `Contains`, `RelSlash`, `EnsureDir`, `WriteFileIfAbsent`, `Normalize`, sentinels, modes |
| `internal/filesystem/runtime.go` | `RuntimePaths`, `PathOptions`, `ResolveRuntimePaths`, `EnsureDirs`, `Exists`, `ProductName` |
| `internal/filesystem/root_test.go` | containment / symlink / normalization tables |
| `internal/filesystem/runtime_test.go` | runtime path derivation + override traversal rejection |
| `internal/git/runner.go` | `CommandRunner`, `ExecRunner`, `SanitizedEnv`, `DefaultTimeout` |
| `internal/git/fake.go` | `FakeRunner`, `FakeResponse`, `Invocation` (non-test build; shared with D and E) |
| `internal/git/adapter.go` | `Adapter`, `Repository`, `Resolve`, `Version`, sentinels, `DomainError` mapping |
| `internal/git/runner_test.go` | argv/shell, env sanitation, cancellation, timeout |
| `internal/git/adapter_test.go` | golden argv, not-a-repo, bare repo (FakeRunner) |
| `internal/git/fixture_test.go` | real `git init` / `git worktree add` fixtures; `t.Skip` when `git` is absent |
| `internal/git/arch_test.go` | `TestNoGoGitDependency`, no `"sh"`/`"-c"` literal in the package |

### Unit B — Runtime store

Owns `internal/storage/**`, `internal/migration/**`, `migrations/**`,
`internal/workspace/**`.

| File | Contents |
|---|---|
| `internal/storage/db.go` | `Options`, `DB`, `Open`, `Close`, `Path`, sentinels |
| `internal/storage/driver.go` | **only** file importing `modernc.org/sqlite`; `dsn(Options) string` |
| `internal/storage/pragma.go` | `Pragmas`, `ReadPragmas`, `ExpectedPragmas`, `IntegrityCheck`, `Querier` |
| `internal/storage/tx.go` | `InTx`, `TxActive` (context-scoped flag) |
| `internal/storage/db_test.go` | pragmas on every pooled conn, WAL sidecar, corrupt DB, reopen |
| `internal/storage/tx_test.go` | commit/rollback, injection literal round-trip |
| `internal/storage/arch_test.go` | driver-confinement + no-ORM assertions |
| `migrations/embed.go` | `package migrations`, `//go:embed *.sql`, `var FS` |
| `migrations/000001_initial.sql` | `projects`, `workspaces` |
| `internal/migration/load.go` | `Migration`, `Load`, filename grammar, checksum |
| `internal/migration/migrator.go` | `Migrator`, `Up`, `Status`, `Pending`, `Applied`, `Result`, bootstrap DDL, sentinels |
| `internal/migration/load_test.go` | ordering, uniqueness, no `.down.sql` |
| `internal/migration/migrator_test.go` | fresh apply, idempotency, fail-fast, checksum, schema-ahead, UTC, concurrency |
| `internal/workspace/store.go` | `Project`, `Workspace`, `Registration`, `Store`, `Register`, `FindByRoot`, `List`, `Count` |
| `internal/workspace/id.go` | `NewID` |
| `internal/workspace/store_test.go` | idempotency, two worktrees, UTC, opaque ids |

### Unit C — Config & knowledge

Owns `internal/config/**`, `internal/knowledge/schema/**`,
`internal/knowledge/loader/**`, `schemas/**`.

| File | Contents |
|---|---|
| `internal/config/config.go` | `Config` and nested structs, `Defaults`, `Validate`, constants |
| `internal/config/loader.go` | `Loader`, `LoaderOptions`, `Load`, precedence, strict decode, provenance |
| `internal/config/env.go` | `MINDRAIL_*` mapping + unknown-var warning |
| `internal/config/scaffold.go` | `DefaultTemplate` (`//go:embed templates/config.toml`), `WriteIfAbsent`, `EnsureKnowledgeDirs` |
| `internal/config/templates/config.toml` | default scaffold |
| `internal/config/loader_test.go` | five-layer precedence table, strict decode, provenance |
| `internal/config/env_test.go` | unknown `MINDRAIL_*` warns, does not fail |
| `internal/config/scaffold_test.go` | no-overwrite, knowledge dir creation, embedded template |
| `schemas/embed.go` | `package schemas`, `//go:embed knowledge/*.json`, `var KnowledgeFS` |
| `schemas/knowledge/decision.v1.schema.json` | Draft 2020-12 document |
| `schemas/knowledge/invariant.v1.schema.json` | Draft 2020-12 document |
| `internal/knowledge/schema/registry.go` | `Registry`, version window, sentinel |
| `internal/knowledge/schema/registry_test.go` | embedding, window, rejection |
| `internal/knowledge/loader/loader.go` | `RecordKind`, `RecordRef`, `Problem`, `Store`, `Loader`, `Load` |
| `internal/knowledge/loader/loader_test.go` | absent tree, counts, corrupt JSON, forward schema, slash paths, offline |

### Unit D — Error model & diagnostics

Owns `internal/app/**`, `internal/doctor/**`, `internal/status/**`.

| File | Contents |
|---|---|
| `internal/app/exit.go` | **edit**: `ExitCode` gains the `*DomainError` branch |
| `internal/app/exit_test.go` | **edit**: add the `DomainError` cases |
| `internal/app/code.go` | `Code`, the code constants, `RegisteredCodes`, `IsRegistered` |
| `internal/app/errors.go` | `DomainError`, `ErrorPayload`, `NewError`, `PayloadOf`, `RenderError` |
| `internal/app/output.go` | `Envelope`, `Warning`, `WriteJSON`, `ColorEnabled` |
| `internal/app/clock.go` | `Clock`, `SystemClock`, `FixedClock`, `FormatTime`, `ParseTime`, `ShutdownTimeout` |
| `internal/app/errors_test.go` | carrier fields, wrapping, payload keys, render golden |
| `internal/app/code_test.go` | registry uniqueness/exhaustiveness |
| `internal/app/output_test.go` | no ANSI in JSON, `NO_COLOR` |
| `internal/app/clock_test.go` | UTC RFC 3339 round-trip |
| `internal/app/arch_test.go` | `TestAppPackageIsALeaf` |
| `internal/doctor/state.go` | `State`, `States`, `Marker`, strict `UnmarshalJSON` |
| `internal/doctor/check.go` | `Check`, `CheckFunc`, `Result`, `Runner`, `Report`, `Err` |
| `internal/doctor/subject.go` | `Subject` |
| `internal/doctor/checks.go` | `DefaultChecks` and the seven MR-001 checks |
| `internal/doctor/render.go` | `Report.RenderHuman` (spec §83 layout) |
| `internal/doctor/state_test.go`, `check_test.go`, `checks_test.go`, `render_test.go` | see §5 |
| `internal/status/readiness.go` | `Readiness`, `ComponentName`, `Components`, `Component` |
| `internal/status/report.go` | `Report` + info structs, `Build` |
| `internal/status/init.go` | `TerminalState`, `InitReport` |
| `internal/status/render.go` | human rendering for both reports |
| `internal/status/readiness_test.go`, `report_test.go`, `render_test.go` | see §5 |

`internal/doctor/testdata/doctor_*.golden`, `internal/status/testdata/*.golden`
are Unit D's.

### Unit E — Wiring (final, sequential)

Owns `internal/bootstrap/**`, `internal/cli/**`, `cmd/mindrail/**`, `go.mod`,
`go.sum`, `Makefile`, `internal/cli/testdata/**`.

| File | Contents |
|---|---|
| `go.mod`, `go.sum` | **edit**: add `modernc.org/sqlite`, `github.com/pelletier/go-toml/v2` |
| `Makefile` | **edit**: add a `smoke` target |
| `internal/bootstrap/app.go` | `App`, `Options`, `Mode`, `New`, `Start`, `Run`, `Shutdown`, accessors |
| `internal/bootstrap/steps.go` | `Step`, `Steps`, `Recorder`, the §87 ordered pipeline |
| `internal/bootstrap/lazy.go` | lazy `Doctor()` behind `sync.Once`, `ManagerInitCount` |
| `internal/bootstrap/app_test.go` | order, fail-fast, read-only mode, shutdown, laziness, no-tx-during-work, offline |
| `internal/cli/root.go` | **edit**: register subcommands, persistent flags, logger→stderr |
| `internal/cli/init.go`, `status.go`, `doctor.go`, `version.go` | the four commands |
| `internal/cli/output.go` | shared render/exit helper |
| `internal/cli/contract_test.go` | the CLI contract table (§5) |
| `internal/cli/arch_test.go` | `TestMainImportsAreMinimal`, `TestForbiddenDependencies`, `TestInitPathDoesNotImportNetHTTP`, `TestNoUpwardImports` |
| `internal/cli/testdata/*.golden` | human-output goldens |
| `cmd/mindrail/main.go` | **edit**: drain the app on the error path |
| `cmd/mindrail/smoke_test.go` | `//go:build smoke`; clean-binary smoke test |
| `cmd/mindrail/signal_test.go` | `//go:build smoke`; SIGINT graceful shutdown |

### File-ownership summary

| Unit | Path globs (exclusive) | Files |
|---|---|---|
| A — Repository & filesystem | `internal/filesystem/**`, `internal/git/**` | 11 new |
| B — Runtime store | `internal/storage/**`, `internal/migration/**`, `migrations/**`, `internal/workspace/**` | 16 new |
| C — Config & knowledge | `internal/config/**`, `internal/knowledge/**`, `schemas/**` | 15 new |
| D — Error model & diagnostics | `internal/app/**`, `internal/doctor/**`, `internal/status/**` | 25 new + 2 edits (`exit.go`, `exit_test.go`) |
| E — Wiring (last) | `internal/bootstrap/**`, `internal/cli/**`, `cmd/**`, `go.mod`, `go.sum`, `Makefile` | 11 new + 4 edits (`root.go`, `main.go`, `go.mod`, `Makefile`) + goldens |

No path glob appears twice. `go.mod`, `go.sum` and `Makefile` are Unit E's alone.

---

## 5. Test plan

Standard `testing` only (tech-stack §89), table-driven, `t.TempDir()` everywhere,
real temporary Git repositories for Git-dependent tests (§92, §132). 112 named
tests.

### 5.1 `internal/app` (Unit D) — 10

| Test | Proves |
|---|---|
| `TestExitCodeFromDomainError` | `DomainError.Kind` maps to 1/2/3/4 ahead of `*Error` |
| `TestDomainErrorCarriesFiveFields` | §72 code/message/cause/metadata/next actions are fields, not a string |
| `TestDomainErrorWrappingSurvivesErrorsIsAndAs` | three `%w` levels, `errors.As` still extracts code + metadata |
| `TestErrorPayloadJSONHasAllFourKeys` | table over every registered code: `code`/`why`/`impact`/`next_action` non-empty |
| `TestRenderErrorBlockLayout` | golden: bare code line, then `Why:`, `Impact:`, `Next:` in order |
| `TestCodeRegistryIsUniqueAndExhaustive` | every constant registered exactly once; `SCREAMING_SNAKE` shape |
| `TestWriteJSONContainsNoANSI` | no `0x1b` byte, output is a single valid JSON object + `\n` |
| `TestColorEnabledRespectsNoColor` | `NO_COLOR` present ⇒ false; `--json` ⇒ false |
| `TestFormatTimeIsUTCRFC3339` | `FixedClock` in a non-UTC zone round-trips with `Z` offset |
| `TestAppPackageIsALeaf` | `go/packages`: no `internal/…` import |

### 5.2 `internal/filesystem` (Unit A) — 8

`TestNormalizeSlashPaths`, `TestRootResolveContainment` (table: `../../etc/passwd`,
`a/../../outside`, absolute-outside, valid nested), `TestRootHandlesSymlinkedRoot`
(root itself is a symlink — the macOS `/tmp` case), `TestRootRejectsSymlinkEscape`
(and asserts the target file is untouched), `TestRelSlashRejectsEscape`,
`TestResolveRuntimePathsUnderCommonDir`,
`TestResolveRuntimePathsRejectsTraversalOverride`,
`TestEnsureDirsAndWriteFileUseRestrictiveModes` (0700 / 0600, Unix-gated).

### 5.3 `internal/git` (Unit A) — 11

| Test | Proves |
|---|---|
| `TestExecRunnerUsesArgvNeverShell` | table of hostile inputs (`; rm -rf /`, `$(id)`, `--upload-pack=evil`, leading `-`); `Cmd.Path` is git, each input is exactly one `Args` element |
| `TestExecRunnerSanitizesGitEnv` | `GIT_DIR`/`GIT_WORK_TREE`/`GIT_INDEX_FILE` stripped; `GIT_OPTIONAL_LOCKS=0`, `GIT_TERMINAL_PROMPT=0`, `LC_ALL=C` set |
| `TestExecRunnerCancellationKillsChild` | stub `git` sleeps; ctx cancelled ⇒ returns fast, `errors.Is(err, context.Canceled)`, child gone |
| `TestExecRunnerTimeout` | `GIT_TIMEOUT` domain error, exit kind unavailable |
| `TestAdapterResolveGoldenArgv` | recorded invocations equal the five `rev-parse` argv lines |
| `TestAdapterResolveNotARepository` | `errors.Is(err, ErrNotARepository)` **and** payload code `NOT_A_GIT_REPOSITORY` |
| `TestAdapterResolveBareRepository` | `BARE_REPOSITORY_UNSUPPORTED`, exit kind usage |
| `TestResolveRealRepository` | git fixture; absolute common-dir/worktree/git-dir |
| `TestResolveFromNestedSubdirectoryIsStable` | `repo/a/b/c` ⇒ byte-identical absolute results |
| `TestResolveLinkedWorktreeSharesCommonDir` | `git worktree add`; common-dir identical from both roots, not under `.git/worktrees/`, `IsLinkedWorktree` differs |
| `TestNoGoGitDependency` | `go list -deps` has no go-git; package source has no `"sh"`/`"-c"` literal |

### 5.4 `internal/storage` (Unit B) — 8

`TestOpenAppliesPragmasOnEveryConnection` (N concurrent `*sql.Conn`, each asserts
`journal_mode=wal`, `foreign_keys=1`, `busy_timeout=5000`, `synchronous=1`),
`TestOpenCreatesWALSidecar`, `TestOpenRejectsCorruptDatabase` (random bytes ⇒
`ErrCorrupt` + `RUNTIME_DB_CORRUPT`), `TestOpenReadOnlyDoesNotCreateFile`,
`TestInTxCommitsAndRollsBack`, `TestParameterizedSQLSurvivesInjectionLiteral`
(`'; DROP TABLE schema_migrations; --` round-trips; table intact),
`TestReopenAfterCloseIsClean` (`PRAGMA integrity_check == ok`),
`TestDriverImportConfinedToStorage` (+ no-ORM assertion).

### 5.5 `internal/migration` (Unit B) — 9

| Test | Proves |
|---|---|
| `TestLoadEmbeddedMigrationsAreOrderedAndUnique` | filename grammar `^[0-9]{6}_[a-z0-9_]+\.sql$`, strictly increasing versions |
| `TestNoDownMigrationsExist` | no `.down.sql`; migrator exports no downgrade entry point |
| `TestUpAppliesAllOnFreshDatabase` | one `schema_migrations` row per embedded file |
| `TestUpIsIdempotentOnSecondRun` | second `Up` applies 0; row set incl. `applied_at` unchanged; sentinel row survives |
| `TestUpFailsFastAndRecordsNothing` | injected FS with bad SQL in the last file ⇒ error; that version absent from `schema_migrations`; its DDL objects absent from `sqlite_master` |
| `TestChecksumMismatchIsRejected` | edited applied migration ⇒ `MIGRATION_CHECKSUM_MISMATCH` |
| `TestSchemaAheadFailsClosed` | DB row version > known ⇒ `RUNTIME_DB_SCHEMA_TOO_NEW` |
| `TestAppliedAtIsUTCRFC3339` | `FixedClock` value read back verbatim |
| `TestConcurrentMigrateSerializes` | two goroutines, `-race`; exactly one applies, both succeed |

### 5.6 `internal/workspace` (Unit B) — 4

`TestRegisterIsIdempotent` (second call ⇒ same `workspace_id`, bumped
`LastSeenAt`), `TestTwoWorktreesRegisterTwoWorkspacesOneProject`,
`TestRegisterStoresUTCTimestamps`, `TestIDsAreOpaquePrefixedAndSortable`.

### 5.7 `internal/config` (Unit C) — 6

`TestPrecedenceAcrossFiveLayers` (table setting `output.color` at each layer and
each combination), `TestStrictDecodeRejectsUnknownKey` (`CONFIG_INVALID`, exit kind
usage), `TestUnknownMindrailEnvVarWarnsNotFails`
(`MINDRAIL_NOT_A_KEY=1` ⇒ one `app.Warning`, `err == nil`),
`TestProvenanceReportsSourceLayer`, `TestWriteIfAbsentPreservesExistingConfig`
(sentinel key survives byte-for-byte; `written == false`),
`TestEnsureKnowledgeDirsCreatesGitkeep`.

### 5.8 `internal/knowledge/schema` (Unit C) — 3

`TestEmbeddedSchemasPresent`, `TestVersionWindowIsWriteOneReadableOne`,
`TestSupportsRejectsNewerVersion`.

### 5.9 `internal/knowledge/loader` (Unit C) — 6

`TestLoadAbsentKnowledgeTreeIsHealthyZero` (`Present=false`, `err==nil`),
`TestLoadCountsDecisionsAndInvariants`,
`TestLoadRecordsCorruptJSONAsNonFatalProblem`,
`TestLoadFailsClosedOnUnknownNewerSchemaVersion` (`Fatal=true`,
`KNOWLEDGE_SCHEMA_UNSUPPORTED`), `TestLoadPathsAreRepoRelativeSlash`,
`TestLoadPerformsNoNetworkCalls` (recording dialer).

### 5.10 `internal/doctor` (Unit D) — 11

| Test | Proves |
|---|---|
| `TestStateEnumIsExactlyFiveValues` | §84 vocabulary, uppercase on the wire |
| `TestStateUnmarshalRejectsUnknown` | anything else is an error |
| `TestDefaultChecksNamesAndSections` | the seven MR-001 checks, stable order |
| `TestGitCheckBranches` | table: healthy ⇒ OK + common-dir detail; non-repo ⇒ ERROR + code + next_action |
| `TestSQLiteCheckDegradesOnlyItself` | broken DB ⇒ only SQLiteCheck non-OK |
| `TestKnowledgeCheckAbsentTreeIsOK` | bare install is not an error |
| `TestOptionalCapabilitiesAreNotErrors` | no resolver/hook/CI ⇒ no `ERROR`, `Report.Err() == nil` |
| `TestEveryNonOKResultCarriesDiagnosticImpactNextAction` | drives every check into its non-OK branch |
| `TestRunnerRespectsContextCancellation` | cancelled ctx ⇒ checks return, no block |
| `TestReportRenderHumanGolden` | `Mindrail Doctor` title, `Repository` section, `✓ Git common-dir detected`, `Diagnostic:`/`Impact:`/`Next:` |
| `TestReportJSONHasNoANSIAndStableKeys` | golden JSON key set |

### 5.11 `internal/status` (Unit D) — 8

`TestReadinessEnumIsExactlyFourValues`, `TestComponentsMapHasExactlySixKeys`
(names equal spec §107), `TestBuildReadyOnHealthySubject`,
`TestBuildBlockedOnUnopenableDB` (`blocking_component == runtime_db`, non-empty
`next_action`), `TestBuildBlockedOnUnsupportedKnowledgeSchema`,
`TestBuildBlockedOnUninitializedRuntime` (readiness `BLOCKED`,
`blocking_component == runtime_db`, code `WORKSPACE_NOT_INITIALIZED`,
`next_action == ["mindrail init"]`), `TestStatusHumanNamesCommonDirAndWorktree`,
`TestStatusJSONParityWithHuman` (every human field has a JSON counterpart).

### 5.12 `internal/bootstrap` (Unit E) — 8

| Test | Proves |
|---|---|
| `TestStartupStepOrderMatchesSpec` | recorder output equals `Steps()` for init, status, doctor |
| `TestNonRepoFailsAtStepOneAndWritesNothing` | only `resolve_repository` recorded; `WalkDir` finds no `mindrail.db`, `-wal`, `-shm`, no `mindrail/` dir |
| `TestReadOnlyModeDoesNotCreateDatabase` | `status`/`doctor` before `init` create nothing |
| `TestShutdownClosesDBAndIsIdempotent` | subsequent `Ping` ⇒ `sql.ErrConnDone`; second `Shutdown` is a no-op |
| `TestLazyManagersNotConstructedForStatus` | `ManagerInitCount() == 0`; `-race` concurrent accessors construct at most once |
| `TestNoTransactionOpenDuringGitOrFilesystemWork` | `storage.TxActive` probe never true inside the git/knowledge adapters |
| `TestStartUsesInjectableRoots` | `RuntimeDirOverride`/`CacheDirOverride` honoured; `os.UserCacheDir()/mindrail` and `os.UserConfigDir()/mindrail` untouched after the suite |
| `TestNoNetworkDuringStart` | recording dialer + failing transport; zero dials, `Start` succeeds |

### 5.13 `internal/cli` — CLI contract (Unit E) — 14

| Test | Proves |
|---|---|
| `TestInitPrintsReadyForTargetedWork` | stdout contains the literal, not `BLOCKED:` |
| `TestInitBlockedPrintsReason` | unwritable common-dir ⇒ `BLOCKED: <non-empty>`, no READY line, exit 4 |
| `TestInitSecondRunPreservesConfigAndSaysSo` | config bytes unchanged **and** stdout names the preserved file |
| `TestInitJSONContract` | golden envelope shape, exit 0 |
| `TestStatusJSONContract` | golden envelope shape incl. six components + readiness |
| `TestDoctorJSONContract` | golden check list, states from the five-value enum |
| `TestVersionJSONContract` | version/commit/go/platform/knowledge schema fields |
| `TestStatusOutsideGitRepoIsStructuredUsageError` | exit 2, `NOT_A_GIT_REPOSITORY`, all four error keys |
| `TestStatusUninitializedIsBlockedExitZero` | readiness `BLOCKED`, `next_action == ["mindrail init"]`, exit 0 |
| `TestBrokenSetupMatrix` | table (no repo / unwritable DB path / corrupt DB / unreadable `.mindrail`): status + doctor each emit code+why+impact+next_action; `errors.As` recovers the same code |
| `TestNoColorProducesNoANSI` | `NO_COLOR=1` over init/status/doctor: no `0x1b` |
| `TestJSONGoesToStdoutLogsToStderr` | stdout is exactly one JSON object; stderr holds the slog records |
| `TestExitCodeMatrix` | scenario → exit code table (D-03) |
| `TestNoPanicOnHostileInput` | corrupt DB, malformed TOML, unreadable `.git`, zero-byte migration ⇒ ordinary errors, no recovered panic |

### 5.14 Architecture + smoke (Unit E) — 6

| Test | Proves |
|---|---|
| `TestMainImportsAreMinimal` | package `main` direct imports ⊆ stdlib + `internal/app` + `internal/cli`; `main.go` declares only `main()` |
| `TestForbiddenDependencies` | no ORM, no DI framework, no go-git in `go.mod` |
| `TestInitPathDoesNotImportNetHTTP` | `net/http` unreachable from `internal/bootstrap` |
| `TestNoUpwardImports` | `git`/`storage`/`filesystem` never import `doctor`/`status`/`bootstrap`/`cli` |
| `TestSmokeCleanBinary` | `go build -o <tmp>/mindrail ./cmd/mindrail`; binary copied alone into an empty dir; `init`, `status --json`, `doctor --json`, `version --json` in a fresh temp Git repo with CWD outside the source tree; all exit 0; `<commonDir>/mindrail/mindrail.db` exists |
| `TestGracefulShutdownOnSIGINT` | subprocess + injected slow step; exits within `app.ShutdownTimeout`; no orphan `git` in the process group; reopened DB passes `integrity_check` |

Smoke tests carry `//go:build smoke` and run via `make smoke`; everything else runs
under plain `go test ./...` and `go test -race ./...`.

### 5.15 Acceptance-criteria mapping

| MR-001 acceptance criterion | Proving tests |
|---|---|
| **1.** `init` succeeds in a Git repo with no network/cloud dependency | `TestNoNetworkDuringStart`, `TestLoadPerformsNoNetworkCalls`, `TestInitPathDoesNotImportNetHTTP`, `TestInitPrintsReadyForTargetedWork`, `TestInitJSONContract`, `TestSmokeCleanBinary` |
| **2.** First run creates the SQLite DB and embedded migrations idempotently | `TestUpAppliesAllOnFreshDatabase`, `TestUpIsIdempotentOnSecondRun`, `TestUpFailsFastAndRecordsNothing`, `TestChecksumMismatchIsRejected`, `TestSchemaAheadFailsClosed`, `TestNoDownMigrationsExist`, `TestAppliedAtIsUTCRFC3339`, `TestConcurrentMigrateSerializes`, `TestOpenAppliesPragmasOnEveryConnection`, `TestOpenCreatesWALSidecar`, `TestInitSecondRunPreservesConfigAndSaysSo` |
| **3.** Git common-dir and active worktree reported correctly and explainably | `TestResolveRealRepository`, `TestResolveFromNestedSubdirectoryIsStable`, `TestResolveLinkedWorktreeSharesCommonDir`, `TestResolveRuntimePathsUnderCommonDir`, `TestTwoWorktreesRegisterTwoWorkspacesOneProject`, `TestStatusHumanNamesCommonDirAndWorktree`, `TestStatusJSONContract`, `TestDoctorJSONContract`, `TestReportRenderHumanGolden` |
| **4.** `status`/`doctor` report missing or broken setups with a structured error and a `next_action` | `TestBrokenSetupMatrix`, `TestStatusOutsideGitRepoIsStructuredUsageError`, `TestStatusUninitializedIsBlockedExitZero`, `TestBuildBlockedOnUnopenableDB`, `TestBuildBlockedOnUninitializedRuntime`, `TestBuildBlockedOnUnsupportedKnowledgeSchema`, `TestEveryNonOKResultCarriesDiagnosticImpactNextAction`, `TestErrorPayloadJSONHasAllFourKeys`, `TestRenderErrorBlockLayout`, `TestDomainErrorWrappingSurvivesErrorsIsAndAs`, `TestCodeRegistryIsUniqueAndExhaustive`, `TestNoPanicOnHostileInput` |
| **5.** CLI contract tests and a clean-binary smoke test exist | all of §5.13 and §5.14 |

---

## 6. Explicit non-goals

MR-001 must **not** implement any of the following. Owner in brackets.

| Not in MR-001 | Owner |
|---|---|
| ProjectUnit discovery, file inventory, ignore-rule resolution, Tree-sitter, content-addressed parse cache, `workspace_files`, cold-index worker, `mindrail index --foreground`, fsnotify | MR-005 |
| Decision/Invariant create/read/supersede lifecycle; spec §95 validation steps 5–14 (filename↔id, unique id, supersede DAG, duplicate lineage, scope syntax/resolution, profile reference, evidence mapping); `mindrail knowledge validate` deep output; `doctor --knowledge` | MR-002 (`knowledge validate` command: MR-017) |
| `agent_sessions`, `tasks`, `checkpoints` tables and their lifecycle; status reporting session/claimed task | MR-003 |
| Leases, `operation_id` idempotency storage, optimistic revision columns, bounded `SQLITE_BUSY` backoff (25/50/100/200/400 ms), `MINDRAIL_BUSY_RETRYABLE`, multi-process contention tests | MR-004 |
| `symbols` / `symbol_identity` tables, `symbol_uid` allocation, rename/move migration | MR-006 |
| `changes` table, reconcile, staged/worktree diff reading, `BASELINE_DIVERGED` | MR-007 (divergence also MR-011) |
| Impact graph, traversal, scoring, path-pattern risk policy | MR-009 |
| Validation profiles, argv-only validation runner, timeouts, bounded output, evidence tables, `.mindrail/policies/*.toml`; §37's rule that the user layer must not weaken repository safety settings | MR-010 (enforcement policy MR-017) |
| Evidence staleness, test-weakening guard, completion gate | MR-011, MR-012, MR-013 |
| MCP server, `mindrail mcp`, the 13 tool schemas, `NOT_IMPLEMENTED_IN_THIS_VERSION`, FTS5 tables and `mindrail search` | MR-014, MR-015, MR-016 |
| Git hook installation at init, `HookCheck`, `mindrail verify --staged` | MR-017 |
| `CIConfigCheck`, `mindrail verify --ci`, `merge-base(base, head)` | MR-018 |
| Warm-path SLO benchmarks, p95 reporting, latency telemetry harness, release matrix / GoReleaser, the ADR-0002 driver benchmark matrix | MR-019 |
| `ResolverCheck`, `doctor --resolver`, resolver diagnostics, managed resolver cache under `os.UserCacheDir()`, `CoverageCheck`, coverage mapping | Out of 0.1 (kernel-scope §4) |
| `ParserCheck`, `IndexCheck`, `doctor --index`, `doctor --unit`, the Project Units and Index sections of the report | MR-005 |
| `mindrail gc`, cache garbage collection, large-file caps, binary detection, generated/vendor defaults | Post-0.1 / MR-005 |
| `doctor --explain <error-code>` and a full error-code prose catalogue | Deferred (D-19) |
| The `AGENTS.md` managed section | **BLOCKING OQ-01** |
| Freezing the status/doctor JSON schemas as a published compatibility guarantee | Post-0.1 (1.0) |

---

## 7. Open questions and decisions

The 40+ questions from the four readers, deduplicated. One is BLOCKING; the rest
are decided here.

### BLOCKING

> **OQ-01 — Who owns the `AGENTS.md` managed section, and what does it contain in 0.1?**
>
> `AGENTS.md` at the repository root already carries hand-written
> `<!-- BEGIN MINDRAIL MANAGED SECTION -->` (L1) and
> `<!-- END MINDRAIL MANAGED SECTION -->` (L54) markers. Spec §82 step 12 lists
> "agent integration instructions" as an `init` step and spec §117 defines the
> managed block as covering **all thirteen** MCP tools — none of which exist until
> MR-014…MR-016. MR-001's "Ne yapılacak" does not mention it.
>
> Candidates: **(a)** MR-001 owns the marker contract (idempotent rewrite strictly
> between the markers, content outside untouched) but writes only the tools that
> exist; **(b)** MR-014 owns it once the MCP surface is real, and MR-001 writes
> nothing; **(c)** MR-001 writes the full §117 block verbatim because §118
> classifies it as soft enforcement anyway.
>
> Recommendation: **(b)** — writing a protocol block that advertises thirteen
> non-existent tools is a correctness hazard for every agent that reads it, and
> §117's own warning ("bir tool'un düşmesi, o tool'un pratikte hiç çağrılmaması
> demektir") argues against a partial block too. This needs a human ruling because
> it changes an existing repository file whose current content asserts the
> opposite, and because it is a product-surface commitment, not an engineering
> detail.
>
> Until this is answered, MR-001 writes nothing to `AGENTS.md`.

### Decisions

| # | Question | Decision | Justification |
|---|---|---|---|
| D-01 | Do `status`/`doctor` auto-create and migrate the DB (§87 steps 4–5)? | **No.** `bootstrap.ModeReadOnly` opens the DB if present and otherwise reports `WORKSPACE_NOT_INITIALIZED`; only `init` creates and migrates. | Acceptance criterion 4 requires a *reportable* missing setup, and spec §83 forbids doctor mutating anything — auto-creation makes both unreachable. |
| D-02 | What readiness does `init`/`status` report with no inventory or index? | `READY` when `knowledge` and `runtime_db` are OK; `inventory`/`syntax`/`semantic`/`coverage_map` report `NOT_APPLICABLE`. `init` prints `READY FOR TARGETED WORK`. | §84 says absent optional capabilities are not failures; a permanently `PARTIAL_READY` healthy install trains users to ignore the field. MR-005 will legitimately introduce `PARTIAL_READY` when the components become applicable. |
| D-03 | Exit code per failure class | not a Git repo → **2**; bare repo → **2**; malformed `config.toml` / unknown config key → **2**; unwritable runtime path → **4**; DB locked / open failure → **4**; corrupt DB → **4**; migration failure → **1**; schema-ahead → **1**; not initialised → **0** with `BLOCKED` readiness | §13's "invalid usage/config" covers deterministic user-correctable placement; "temporary/runtime unavailable" covers environment conditions; migration failure is a genuine operation failure. |
| D-04 | Does `init` exiting `BLOCKED: <reason>` return non-zero? | **Yes**, with the D-03 code for the cause. | `BLOCKED` means the tool is unusable; a zero exit would make the smoke test and CI blind to it. Acceptance criterion 1 constrains only the healthy path. |
| D-05 | What does `init` write under `.mindrail/`? | `config.toml` from the embedded template, plus `knowledge/decisions/.gitkeep` and `knowledge/invariants/.gitkeep`. No `project.md`, no `policies/`. | MR-002 needs stable write targets; `.gitkeep` survives clone (Git tracks no empty dirs). `policies/` belongs to MR-010/MR-017. |
| D-06 | Loader behaviour when `.mindrail/knowledge` is absent | `Present=false`, zero records, `err == nil`, component `OK`. | A clean clone of a repo with no records must not look broken (MR-002 AC2). D-05 and this are complementary, not alternatives. |
| D-07 | Runtime/cache root override name | `MINDRAIL_RUNTIME_DIR` and `MINDRAIL_CACHE_DIR`, plus constructor parameters. Hidden from `--help`, documented as isolation-only. | §131 mandates injectable roots and the clean-binary smoke test drives the compiled binary, which cannot use constructor injection. Adding an env var later is compatible; removing one is not. |
| D-08 | Bounded shutdown deadline (§88) | `app.ShutdownTimeout = 5s`, matching the SQLite `busy_timeout` of 5000 ms. | One number, already justified by the DB contention window. |
| D-09 | Does MR-001 ship `mindrail version`? | **Yes**, minimal: version, commit, Go version, platform, `write_schema_version`, `readable_schema_versions`, MCP compatibility `"none"`. | §104 requires it, no MR owns it, and the smoke test conventionally invokes it. Fields fill in additively. |
| D-10 | Which doctor checks exist in MR-001? | Only the seven whose subsystems exist: `GitCheck`, `RuntimePathCheck`, `ConfigCheck`, `SQLiteCheck`, `MigrationCheck`, `KnowledgeCheck`, `WorkspaceCheck`. The list grows additively. | §84's `NOT_APPLICABLE` means "this project does not use it", not "this binary cannot do it"; registering nine checks would lie about the second case. |
| D-11 | JSON envelope shape | `app.Envelope{command, ok, data, error, warnings}`. Command payloads are the `data` value. | One serializer for CLI now and MCP later (kernel-scope AC-14), one place to guarantee "no ANSI". |
| D-12 | `next_action` singular or list? | JSON key `next_action`, Go type `[]string`. | Keeps spec §84's field name and §72's plural cardinality; the §84 example itself prints two lines. |
| D-13 | `next_action` content format | Free-text strings, one action per element, in 0.1. No `{kind,command,args}` structure. | Spec §83/§84 examples mix prose, CLI commands and config assignments; structuring them now would be guesswork. Revisit at MR-014 when MCP consumers exist. |
| D-14 | `doctor` exit code | `WorstState` of `OK`/`DEGRADED`/`UNAVAILABLE`/`NOT_APPLICABLE` → 0. `ERROR` → the D-03 code of that check's `app.Code`. | §84 forbids treating optional absences as fatal; genuine `ERROR` still has to be visible to CI. |
| D-15 | Which stream carries JSON vs slog? | stdout = human or JSON result, nothing else. stderr = all `slog` records. Default level `WARN`; `--verbose` ⇒ `DEBUG`. | `--json` must be parseable without filtering (§12). |
| D-16 | Which health vocabulary is on the wire? | Three distinct enums: `doctor.State` (§84's five) for checks and components; `status.Readiness` (§107's four) overall; `status.TerminalState` (§82's two strings) for `init` only. Spec §83's `HEALTHY`/`MISCONFIGURED` are per-ProjectUnit and out of 0.1. | The four vocabularies address different objects; collapsing them loses information the spec asserts separately. |
| D-17 | Where does the readiness model live? | `internal/status` — the only package name added to §7. | §7 is "recommended", §8 prefers domain packages, and `internal/index/` belongs to MR-005. |
| D-18 | Error code naming | Unprefixed `SCREAMING_SNAKE`, registered exactly once in `internal/app/code.go`, asserted by `TestCodeRegistryIsUniqueAndExhaustive`. `MINDRAIL_` stays reserved for process/infrastructure conditions (`MINDRAIL_BUSY_RETRYABLE`). | Matches every domain code the docs already name (`EVIDENCE_STALE`, `KNOWLEDGE_SCHEMA_UNSUPPORTED`, `STATE_REVISION_CONFLICT`). |
| D-19 | Is `doctor --explain <code>` required in 0.1? | **No.** Deferred. | Spec §83 says "destekleyebilir" (may support) — permissive, not mandatory. The registry that would back it exists anyway (D-18). |
| D-20 | Does status emit a duration in MR-001? | **Yes**: `duration_ms` in the JSON and the §70 `duration` slog attribute. No benchmark harness. | kernel-scope §5 says the targets "must be measured from the first implementation"; the harness and p95 reporting stay MR-019's. |
| D-21 | Human markers for the five states | `✓ OK`, `△ DEGRADED`, `✗ ERROR`, `– UNAVAILABLE`, `· NOT_APPLICABLE`, always followed by the state word for non-OK results. Golden tests assert on the **word**, not the glyph. | Keeps the spec §83 look while making the tests terminal-independent. |
| D-22 | Are spec §10's four pragmas MUST, and how are they guaranteed per connection? | All four are MUST. Applied via DSN pragma parameters inside `internal/storage/driver.go`, then verified by `ReadPragmas` on a fresh connection during `Open`; a mismatch is `ErrPragma`. | "Recommended" in the spec, but `foreign_keys=OFF` would silently weaken every later MR's schema. Verification makes the guarantee observable and driver-swap-safe. |
| D-23 | `schema_migrations` shape; checksum or not? | `(version INTEGER PRIMARY KEY, name TEXT, checksum TEXT, applied_at TEXT)`, with the checksum verified on every start. | One column buys detection of the realistic MR-002…MR-020 mistake of editing `000001` instead of adding `000002`; it is the only thing that makes "forward-only" enforceable. |
| D-24 | Concurrent first-run migration across processes | Each migration's check-then-apply runs in one `BEGIN IMMEDIATE` transaction; the loser blocks on `busy_timeout=5000` and then observes the applied set. Bounded exponential retry and `MINDRAIL_BUSY_RETRYABLE` stay MR-004. | No new machinery; matches §21's "short write transaction" pattern. |
| D-25 | DB records a migration version this binary does not know | Fail closed: `RUNTIME_DB_SCHEMA_TOO_NEW`, exit 1, `next_action` = upgrade. | Mirrors the `MINDRAIL_VERSION_TOO_OLD` posture of spec §95 and §24's "no implicit downgrade". |
| D-26 | Workspace identity, and the §74 "no absolute paths" tension | Identity is the opaque `WS-…` / `PRJ-…` id. `workspaces.root_path` and `projects.common_dir` are machine-local **location** columns (unique lookup keys), explicitly not identity. MR-001 persists no repository *content* paths at all. | §41 requires opaque external identity; §74 governs repository content paths, which MR-001 does not store. A moved clone re-registers rather than silently orphaning. |
| D-27 | How much of spec §95 runs at MR-001? | Steps 1–4 only: read file, parse JSON syntax, read `schema_version`, check the reader window. Steps 5–14 are MR-002's. | kernel-scope §3 says the fail-closed branch must not be retrofitted; the rest is MR-002's stated lifecycle. |
| D-28 | Does MR-001 compile the JSON Schemas? | **No.** The documents are embedded and the version window is enforced; `santhosh-tekuri/jsonschema/v6` is added by MR-002. | Keeps MR-001's dependency delta to two modules and keeps schema enforcement with the MR that has the acceptance criteria for it. |
| D-29 | `git` subprocess environment isolation (spec §113 "environment isolation") | Strip `GIT_DIR`, `GIT_WORK_TREE`, `GIT_COMMON_DIR`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_NAMESPACE`; set `GIT_OPTIONAL_LOCKS=0`, `GIT_TERMINAL_PROMPT=0`, `LC_ALL=C`; pass the directory via `Cmd.Dir` only. | Git hooks run with `GIT_DIR`/`GIT_INDEX_FILE` preset; MR-017 will call this same adapter from `pre-commit`, so inheriting them would silently redirect discovery. |
| D-30 | `git` subprocess timeout | `git.DefaultTimeout = 10s`, surfaced as `GIT_TIMEOUT` / exit 4, and as `UNAVAILABLE` (not `ERROR`) in doctor. | Prevents an indefinite hang on a network filesystem or a stale `index.lock` from blowing the §5 warm-path budget with no defined failure mode. |
| D-31 | Bare repository handling | Reject with `BARE_REPOSITORY_UNSUPPORTED`, exit 2. | Acceptance criterion 3 requires reporting an *active worktree*; a bare repo has none. |
| D-32 | Symlink policy (§75) | Canonicalize the root once with `filepath.EvalSymlinks`, then require every operated path to canonicalize to a descendant of that canonical root. Inside-root symlinks allowed; escapes denied. | Denying all symlinks breaks the macOS `/tmp` → `/private/tmp` fixture path and common developer layouts. |
| D-33 | Where do `migrations/` and `schemas/` live, given `go:embed` cannot reach up? | Keep §7's repo-root `migrations/` and `schemas/knowledge/`, and add a two-line `package migrations` / `package schemas` file beside them declaring the `embed.FS`. | Honours §7's layout and §111's embedding without a `pkg/` API or a duplicated asset tree. |
| D-34 | Does MR-001 use a DI framework? | No. Explicit constructor wiring only, all seams (`Clock`, `CommandRunner`, runtime/cache roots, migration FS, schema FS) as constructor parameters. | tech-stack §9, asserted by `TestForbiddenDependencies`. |
| D-35 | Dependency delta | `modernc.org/sqlite` (ADR-0002 provisional) and `github.com/pelletier/go-toml/v2` (§37/§137). Nothing else. Unit E owns the `go.mod` edit. | Smallest set that satisfies MR-001; `jsonschema/v6` deferred to MR-002 per D-28. |

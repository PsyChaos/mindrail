# MR-005 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-005-requirements.md) TASK-01 ve
TASK-02 kapılarını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki
kanıtları korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `40c42d0` idi. Dondurulmuş gereksinimlerin §0 tablosunda
  kaydedilen `make verify` ve `make tidy-check` yeşildi.
- Bu turdaki ilk WIP migration/store hali kırmızıydı; kapı, bu kırmızıyı
  gizlemeden Reader ve Breaker geri bildirimleriyle tamamlandı.
- TASK-01, `4548f38` commit'iyle kapatıldı. Son `make verify` ve
  `make tidy-check` yeşildi; `go test -list '.*' ./... | grep -c '^Test'`
  sonucu 906 idi. `.claude/**` ve `graphify-out/**` altındaki kirli dosyalar
  kullanıcıya ait/ilgisiz kabul edildi; TASK-01 değerlendirmesine veya bu
  kayda taşınmadı.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `migrations/000004_index.sql`: yalnız `project_units`, `file_index_state`, `symbols`, `symbol_imports`, `symbol_references`; state CHECK'i D-81 değerlerini sınırlar. |
| AC-01.2 | Karşılandı | `internal/index/index.go`: `TableSchemaVersion = 4`; `internal/index/index_test.go` ledger'ın en yüksek sürümüne bağlar. |
| AC-01.3 | Karşılandı | `internal/app/code.go` üç index kodunu ve ayrı remedy'lerini kaydeder; registry/allCodes ve envelope sınıflandırma testleri güncellendi. |
| AC-01.4 | Karşılandı | `migrations/shipped_test.go` 000004 byte checksum'ını pinler; migration ledger/shape testleri beş tablo ve beklenen sütunları kapsar. |
| AC-01.5 | Karşılandı | `internal/cli/upgrade_test.go` schema-3 fixture'ını 000004'e yükseltir ve koordinasyon satırlarının korunduğunu doğrular. |

Raporlanan doğrulama komutları: `make check`, `make verify`, `make tidy-check`
ve `go test -list '.*' ./... | grep -c '^Test'`. Son kabulde bağımsız delta
değerlendirmesi Reader **PASS**, Breaker **VERIFIED** sonucuna ulaştı.

## Reader / Breaker bulguları ve giderim

| Bulgu | Sonuç ve giderim |
|---|---|
| B1 — typed-nil | Store sınırlarında typed-nil hata/sonuç yüzeyleri denetlendi; ilgili guard ve test kanıtları eklendi. |
| B2 — schema gate | Store'un migration-4 öncesi şemayı boş veri gibi okumaması için schema gate doğrulandı. |
| B3 — pointer ambiguity | `resolved_symbol_id` nullable kaldı; belirsiz/çözümsüz referansın NULL olması hata veya sahte çözüm sayılmadı. |
| B4 — guard gaps | Mutasyon envanteri genişletildi; kaçan guard'lar kontrol/mutant kanıtıyla kapatıldı. |

### D-89 — referans hedefi silinince `ON DELETE SET NULL`

`symbol_references.resolved_symbol_id`, yapısal eşleşmenin opsiyonel bir
ipuçudur; reference gerçeğinin kendisi değildir. Hedef sembol silinince
reference satırını silmek veya işlemi FK hatasıyla durdurmak, D-87'nin
"çözümsüz/ambiguous referans da bir olgudur" kuralını bozar. Bu nedenle FK
`ON DELETE SET NULL` kullanır: hedef pointer'ı kalkar, reference satırı ve
STRUCTURAL bağlamı kalır. Kontrol ve mutant çalışmaları bu davranışın hem
korunduğunu hem de kaldırıldığında testin kırmızıya döndüğünü gösterdi.

## Mutasyon kapsamı

- Store envanteri: 70 adayın 65'i kırmızıya döndü. Üç Go istisnası gerekçeli:
  - `store.go:248` ve `store.go:414`: `json.Marshal([]string)` mevcut tip
    sözleşmesinde erişilemez hata yoludur.
  - `store.go:279`: unique-ID uyuşmazlığı PK ve aynı transaction invarianti
    altında erişilemezdir.
- Bu üç istisna, ilgili type/schema/transaction sınırı değişirse yeniden
  mutasyon gerektirir; özellikle slice tipi, ID üretimi, PK/UNIQUE kuralı veya
  transaction ayrımı değiştiğinde istisna geçersiz sayılır.
- State CHECK, migration düzeyinde kapsanmıştır. Error envanterinde 35 adayın
  30'u kırmızıya döndü; 268 numaralı yol tam suite ile kapsandı. SQL/schema
  envanterinde 31 mutant kırmızıya döndü. Son store turunda 449 mutant
  kırmızıya döndü.
- Kapsam sınırı: her boolean yaprak ve her `NOT NULL` sütunu tek tek mutant
  yapılmadı; yukarıdaki envanter ve migration kontrol/mutantları seçilmiş
  guard'ları kanıtlar, evrensel mutasyon iddiası değildir.

## Denetim artefaktları

Geçici artefaktlar bu çalışma ortamında saklanmıştır:

- `/tmp/mr005-breaker-CsHEvo/BREAKER_REPORT.md`
- `/tmp/mr005-breaker-CsHEvo/mutation-results.json`
- `/tmp/mr005-breaker-delta-9QNGkx/DELTA_BREAKER_REPORT.md`
- `/tmp/mr005-breaker-delta-9QNGkx/mutation-inventory.json`
- `/tmp/mr005-breaker-final-C4PhcQ/FINAL_BREAKER_REPORT.md`
- `/tmp/mr005-breaker-final-C4PhcQ/{mutation-inventory,error-mutation-results,sql-mutation-results}.json`
- `/tmp/mr005-error-evidence-TIiZX1/{empty-cancellation-mutation,error-mutation-results}.json`

Bu dosyalar `/tmp` altında olduğundan kalıcı proje kaydı değildir; bu özet
onların sonuçlarını, ölçüm uydurmadan, TASK-01 için kalıcılaştırır.

## TASK-02 kabul kanıtı

Bu bölüm yalnız TASK-02 uygulama/validation kanıtıdır; bağımsız Reader/Breaker
kapısının sonucunu iddia etmez.

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | `internal/index/inventory.Discover`, `pyproject.toml` ve `package.json` marker'larını, aynı dizindeki `tsconfig.json` ile TypeScript'e rafine eder; `TestDiscoverFindsDeterministicPythonTypeScriptAndJavaScriptUnits` üç birimi path sırasıyla doğrular. `Owner` sınır-güvenli `filepath.Rel` ile en uzun kökü seçer; nested-root testi bunu doğrular. |
| AC-02.2 | Karşılandı | Sıfır birim kalıcı inventory sonucu `inventory: OK, "0 project units discovered"`, `syntax.phase: "INVENTORY"` ve genel `PARTIAL_READY` üretir. Bu, boş envanter için READY iddiasını engeller; `TestBuildReportsInventoryPhaseWithoutRescanning` kanıtıdır. |
| AC-02.3 | Karşılandı | `Discover` canonical repository root altında `WalkDir` ile yalnızca okur; symlink dizinlerini izlemez ve `.git`, `.mindrail`, `node_modules` altını atlar. `TestDiscoverIsIdempotentAndDoesNotTouchTheRepository`, tekrarlı pass'te aynı ID/zamanı, tek unit satırını, sıfır `file_index_state` satırını ve değişmeyen marker mtime'ını doğrular; ayrı symlink-escape testi dış kökü reddeder. |
| AC-02.4 | Karşılandı | Bootstrap mevcut `load_index_state` (§87 step 8) içinde non-blocking discovery yapar; step listesine ek bloklayıcı adım eklenmemiştir. Read-only startup yalnız `Store.ListUnits` ile persisted sonuçları okur. `TestStartupDiscoversInventoryAtTheExistingIndexStateStep` ve marker silindikten sonraki `TestReadOnlyStartupReportsPersistedInventoryWithoutWalkingSource` status'un filesystem rescan yapmadığını doğrular. |

### TASK-02 karar kaydı — boş envanter lifecycle uyuşmazlığı

Dondurulmuş tasarım §9, sıfır unit için `UNINITIALIZED` der; dondurulmuş
gereksinim AC-02.2 ise açıkça `INVENTORY` ve "no READY" ister. TASK-02,
ayrıntı kabul kriterini izler: `syntax.phase = INVENTORY` ve genel
`PARTIAL_READY`. Tasarım dosyası dondurulmuş olduğu için değiştirilmedi; bu
kayıt uyuşmazlığı görünür kılar.

### TASK-02 mutasyon ve doğrulama

- Longest-root guard'ın gerçek olduğunu göstermek için `Owner` içindeki
  `len(unit.Path) > len(owner.Path)` karşılaştırması geçici olarak `<` yapıldı.
  `GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory -run
  '^TestOwnerUsesTheLongestContainingUnitRoot$' -count=1` beklenen kırmızıyı
  verdi: nested path `UNT-child` yerine `UNT-parent` seçildi. Doğru guard geri
  yüklendi.
- Geri yükleme sonrası focused doğrulama:
  `GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory
  ./internal/index ./internal/status ./internal/bootstrap` yeşildi.
- Tam doğrulama: `GOCACHE=/tmp/mindrail-go-build go test ./...` yeşildi.
- Kod grafiği değişiklikten sonra `graphify update .` ile yenilendi. Bu
  çalışma ağacında önceden var olan `graphify-out/**` ve
  `internal/cli/testdata/doctor_human.golden` değişiklikleri TASK-02 kanıtı
  değildir; sonuncusu TASK-01'in schema-v4 golden farkıdır ve bu turda
  değiştirilmedi.

## TASK-02 Breaker giderimi

Breaker'ın doğruladığı dört eksik aşağıdaki şekilde giderildi; bu bölüm önceki
TASK-02 kanıtını geçersiz kılmaz, remediation delta'sını kaydeder.

- **Worktree kapsamı:** `Store.ListUnits(ctx, root)` artık canonical ve temiz
  mutlak root ile sınırlandırılır. SQL predicate'i `LIKE` kullanmaz; `%`/`_`
  path byte'ları ile `/web`–`/website` prefix çakışmasını separator kontrolüyle
  kapatır. Bootstrap her iki read/write yolunda `inventory.CanonicalRoot`u
  geçirir; Store filesystem erişimi yapmaz. Gerçek sibling linked-worktree ve
  nested registered-worktree testleri, yabancı unit'in status'a sızmadığını
  doğrular.
- **Stale reconcile:** Tam `WalkDir` ve bütün upsert'ler başarılı olduktan
  sonra `ReconcileUnits` tek SQLite transaction'ında stale root'un
  `symbol_references → symbol_imports → symbols → file_index_state →
  project_units` olgularını siler. Cancel, walk veya upsert hatasında bu çağrı
  hiç yapılmaz. `workspaces.root_path` içindeki yabancı registered worktree
  altı korunur; migration-4 schema gate'i bu MR-001 tablosunun zaten mevcut
  olmasını garanti eder. Workspace satırları bu Store tarafından değiştirilmez.
- **Context ve symlink:** Discovery girişte ve walk/upsert/reconcile sınırında
  context'i denetler; pre-cancelled boş discovery `context.Canceled` döner.
  File-symlink marker'ı kalıcı testle dışlanır (yalnız directory-symlink
  davranışına güvenilmez).
- **Rollback:** `BEFORE DELETE project_units` injected failure'ı, stale prune
  transaction'ının hem unit hem file-state satırını koruduğunu doğrular.

Giderim guard mutasyonları ve beklenen kırmızıları:

1. Symlink predicate devre dışı bırakıldı →
   `TestDiscoverDoesNotTreatASymlinkedMarkerFileAsInventory` symlinked
   `pyproject.toml` için Python unit üreterek kırmızı oldu.
2. Pre-cancel check devre dışı bırakıldı →
   `TestDiscoverPrioritizesCanceledContextOverAConfigurationError` nil-store
   hatası yerine `context.Canceled` beklerken kırmızı oldu.
3. SQL separator predicate'i kaldırıldı →
   `TestDiscoverReconcilesRemovedRootWithoutTouchingSiblingRoot` `/web`
   reconcile'ının `/website` unit'ini de sildiğini göstererek kırmızı oldu.
4. `file_index_state` delete adımı çıkarıldı → aynı reconcile testi FK
   constraint hatasıyla kırmızı oldu.
5. Registered-foreign-worktree filtresi devre dışı bırakıldı →
   `TestNestedRegisteredWorktreeIsExcludedFromParentInventoryAndPruning`
   nested unit'in prune edildiğini göstererek kırmızı oldu.

Her mutant geri alındı. Giderim sonrası focused komut
`GOCACHE=/tmp/mindrail-go-build go test ./internal/index/inventory
./internal/index ./internal/bootstrap ./internal/status` yeşildi. Host'un
30-saniyelik command penceresi `go test ./...` çıktısını CLI sonrası kesmeye
başladığından tam package kümesi iki bounded çalışmada doğrulandı:
`go test ./internal/cli` ve CLI dışındaki `go list ./...` package'larının
eksiksiz explicit listesi; ikisi de yeşildi. `git diff --check` de yeşildi.

### TASK-02 final delta — filesystem root kapsamı

Delta Reader/Breaker'ın bulduğu root `"/"` hatası kapatıldı. Normal SQL
predicate'i child separator'ını ayrıca aradığından `/` için `//` prefix'i
üretiyor, bu da descendant unit'leri hem `ListUnits` hem `ReconcileUnits`
tarafından görünmez kılıyordu. `pathInRootSQL`, filesystem/volume root için
root'un zaten taşıdığı separator'ı boundary kabul eden ayrı predicate kullanır;
normal `/web`–`/website` koruması değişmeden kalır.

`TestStoreScopesAndReconcilesAtFilesystemRoot`, temp altında persist edilmiş
unit'in `ListUnits(ctx, "/")` ile göründüğünü ve
`ReconcileUnits(ctx, "/", nil)` ile silindiğini doğrular. Root branch'i
geçici olarak devre dışı bırakıldığında aynı test beklenen kırmızıyı verdi
(0 unit); guard geri yüklendi. Geri yükleme sonrası focused
`go test ./internal/index/inventory ./internal/index ./internal/bootstrap
./internal/status` ve `git diff --check` yeşildi.

### TASK-02 final kapı

Son bağımsız değerlendirme Reader **PASS**, Breaker **VERIFIED** sonucunu
verdi (root kapsamı gideriminden sonra). `GOCACHE=/tmp/mindrail-go-build make
verify`; `go vet`, normal test, race test ve smoke aşamalarında yeşildi.
`make tidy-check` de yeşildi. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile 921 olarak kaydedildi.

## TASK-03 kabul ve kapı kaydı

TASK-03 bağımsız değerlendirmesinde Reader **PASS**, Breaker **VERIFIED**
sonucuna ulaştı. Bu kayıt yalnız AC-03.1…03.5'in parser/registry sınırını
belgeler; bu turda tam `make verify` hâlâ çalıştığından onun nihai sonucunu
iddia etmez.

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | Sabit registry `.py`, JS uzantıları, TS uzantıları ve `.tsx` için dört ayrı language entry taşır; extension eşlemesi ve deterministik tie-break testlidir. |
| AC-03.2 | Karşılandı | Grammar'a özgü node ayrıntıları `internal/index/parser` içinde kalır; adapter sınırı `SyntaxAdapter` üzerinden parse, sembol ve reference olgularını normalize eder. |
| AC-03.3 | Karşılandı | Dil başına gömülü query dosyaları registry kurulurken derlenir; geçersiz query kullanıcı komutuna değil teste düşer. |
| AC-03.4 | Karşılandı | Breaker'ın bağımsız native 1.000-cycle/error ve 100×16 eşzamanlı close probeleri kaynak yaşam döngüsünü doğruladı. |
| AC-03.5 | Karşılandı | Geçerli UTF-8 ancak sözdizimsel bozuk girdi partial tree ile birlikte parse error olarak raporlanır; panic veya sessiz kesme yoktur. |

Mutasyon örnekleri: partial parse error'unun bastırılması ilgili fixture'ı
kırmızıya döndürdü; `QueryCursor.Close` çağrısının çıkarılması native leak
guard'ını kırmızıya döndürdü. Her mutant geri alındı.

### TASK-05 için kaydedilen önkoşullar

Bunlar TASK-03 blocker değildir; extraction/fingerprint işi başlamadan önce
TASK-05'in açıkça ele alacağı davranışlardır:

- Python method'ları yanlış etiketlenmemeli.
- TypeScript overload signature'ları tek sembole indirgenmemeli.
- JavaScript generator ve arrow declaration'ları atlanmamalı.

TASK-03 kapısının tamamlanan genel doğrulaması:
`GOCACHE=/tmp/mindrail-go-build make verify` exit 0 ile `go vet`, normal,
race ve smoke aşamalarını geçti; `make tidy-check` exit 0 verdi.
`go test -list '.*' ./... | grep -c '^Test'` test sayısını 931 olarak
raporladı. `git diff --check` temizdi.

## TASK-03 — parser registry and native lifecycle (implementation evidence)

Scope is `internal/index/parser/**`, its embedded queries, and Go dependency
metadata. TASK-04 caching and TASK-05 extraction/fingerprints are not claimed.
The normalized `Symbol`, `Reference` and `Range` values own their data and are
path-independent; `SyntaxSnapshot` is a separate native-tree owner with an
explicit, idempotent `Close`. The interface follows tech-stack §29, including
`context.Context`. `ParseSchemaVersion = 1` follows D-85.

### Official dependency evidence

The read-the-damn-docs skill was applied. Before adding imports, the official
Go module registry was queried with `go list -m -json <module>@latest` for all
four modules, and the pinned upstream source was downloaded and inspected:

| Module | Pinned/latest registry version | Grammar ABI |
|---|---|---|
| `github.com/tree-sitter/go-tree-sitter` | `v0.25.0` | runtime accepts 13–15 |
| `github.com/tree-sitter/tree-sitter-python` | `v0.25.0` | 15 |
| `github.com/tree-sitter/tree-sitter-javascript` | `v0.25.0` | 15 |
| `github.com/tree-sitter/tree-sitter-typescript` | `v0.23.2` | TypeScript 14; TSX 14 |

References: [official binding README](https://github.com/tree-sitter/go-tree-sitter),
[pinned API](https://pkg.go.dev/github.com/tree-sitter/go-tree-sitter@v0.25.0),
[parser implementation](https://github.com/tree-sitter/go-tree-sitter/blob/v0.25.0/parser.go),
[allocator implementation](https://github.com/tree-sitter/go-tree-sitter/blob/v0.25.0/allocator.go),
[TypeScript/TSX Go bindings](https://github.com/tree-sitter/tree-sitter-typescript/tree/v0.23.2/bindings/go).
Module release numbers need not match: grammar ABI compatibility is what
`SetLanguage` checks, and the default test checks all four ABIs and compiles
every shipped query against its actual grammar. TSX calls `LanguageTSX`,
separately from TypeScript's `LanguageTypescript`.

Upstream v0.25.0 `ParseWithOptions` saves a non-nil options payload without
releasing its pointer handle. This implementation uses basic `Parse` (nil
options), whose source frees input callback C strings and its input handle.
Context cancellation is checked before validation/allocation and after the
non-preemptive parse, disposing any completed tree on cancellation. This
choice matches §48 and avoids inheriting that upstream callback leak.

The binding and grammars require CGO and a native C compiler. The current
environment reports `CGO_ENABLED=1`, `CC=gcc`. This work makes no claim of
CGO-disabled or cross-platform release support; tech-stack §26 still requires
native platform CI. Extra grammar checksums added by `go mod tidy` belong to
upstream module tests, not extra built-in language registrations.

### AC evidence and RED/GREEN

- AC-03.1: the exact four-entry ordered table, all eight extensions, unknown
  extensions, defensive metadata copying and first-match conflict are tested.
- AC-03.2: all adapters satisfy `SyntaxAdapter`; real declarations/calls are
  extracted in four languages and TSX's fixture contains JSX. Grammar node
  names remain inside the parser package. No native nodes are exported.
- AC-03.3: eight embedded `symbols.scm`/`references.scm` files compile in the
  default test suite. Injected missing and malformed query files also verify
  constructor failure cleanup, including earlier successfully built queries.
- AC-03.4: 1,000 parse/symbol/reference/dispose cycles **per language** run in
  the default suite. The official `SetAllocator` callbacks forward to libc and
  observe actual live allocation pointers and requested bytes. After warmup,
  every language remained at **91 blocks / 7,978 bytes**, the held registry
  queries; registry disposal returned to **0 blocks / 0 bytes**. The helper is
  imported only by tests, the tests are serial, and deferred allocator restore
  runs after deferred native disposal. No Go finalizer, heap-size threshold,
  process RSS threshold or garbage collection is needed. No TreeCursor or
  LookaheadIterator is allocated by the implementation; Parser, Tree, Query
  and QueryCursor allocations all have explicit Close ownership.
- AC-03.5: valid UTF-8 broken fixtures return both a partial snapshot and
  `ErrSyntax`, and intact declarations before the malformed text remain usable.

The first test run was RED with undefined Registry/SourceFile APIs before
production source existed. A second RED preceded the native allocator helper.
The focused normal and race suites are GREEN; statement coverage is 96.5%.

### Deliberate guard mutations (all restored)

Each named mutation ran the focused default test and failed as intended:

| Mutation | Observed failure |
|---|---|
| Suppress partial parse error | broken-source tests receive nil error in all four languages |
| Disable UTF-8 validation | invalid bytes produce a partial parse instead of `ErrInvalidUTF8` |
| Disable entry cancellation | canceled invalid input returns UTF-8 error instead of cancellation |
| Disable post-parse cancellation | canceled-after-entry context returns success |
| Omit canceled tree cleanup | 16 native blocks / 1,360 bytes remain |
| Remove source copy | extracted name becomes `xxxxx` after caller edits input |
| Remove foreign snapshot check | wrong-language adapter accepts snapshot |
| Remove nil snapshot check | nil-dereference failure in the focused test |
| Remove closed snapshot check | nil native-tree access failure in the focused test |
| Disable closed parser/query guards | closed adapter returns success or wrong error |
| Omit Parser.Close | 1,000 Python cycles add 21,000 native blocks |
| Omit Tree.Close | 1,000 Python cycles add 16,000 native blocks |
| Omit QueryCursor.Close | 1,000 Python cycles add 13,000 native blocks |
| Omit symbol Query.Close | registry disposal retains 47 blocks / 4,282 bytes |
| Omit query read/compile failure cleanup | failed constructor retains 80 blocks / 7,054 bytes |
| Ship nonexistent query node | embedded-query compilation test fails before user use |
| Substitute TypeScript grammar for TSX | JSX fixture produces a syntax error |
| Reverse extension resolution order | conflict test selects JavaScript instead of Python |

The entry-cancellation mutant initially survived because the post-parse check
still returned cancellation. The test was strengthened to assert cancellation
precedence over invalid UTF-8, and the same mutant then failed. The mutation
was restored and the full focused suite passed again.

Final implementation checks: `GOCACHE=/tmp/mindrail-go-build make check build`,
`go test -race ./internal/index/parser/...`, `make tidy-check`, and
`git diff --check` all passed. `graphify update .` completed its AST-only
refresh (it reported the pre-existing optional SQL parser absence). This
entry records implementation evidence only; subsequent TASK-03 Reader/Breaker
acceptance is recorded above. No TASK-03 commit was created by the
implementation worker.

## TASK-04 — content-addressed snapshot cache (implementation evidence)

Scope is `internal/index/snapshot/**`. `Cache.New` consumes
`filesystem.RuntimePaths.CacheDir`, so the existing common-dir and override
rules determine its location. The disk DTO contains only path-independent
`parser.Symbol`/`Reference` values and a syntax-error flag. Native Tree-sitter
trees/nodes, file paths, unit IDs, SQLite IDs and durable facts are never
serialized. TASK-05 must extend this DTO when imports/fingerprints exist and
bump `parser.ParseSchemaVersion` when snapshot semantics change.

| Criterion | TASK-04 result and evidence |
|---|---|
| AC-04.1 | Met at cache level. The filename hashes the JSON identity tuple (language, grammar version, `parser.ParseSchemaVersion`, SHA-256 content hash); the record repeats and verifies it. `TestIdenticalContentSharesOneValidatedSnapshot` reads two differently named files with equal bytes and observes one compute/entry. `TestEveryIdentityComponentSeparatesCacheEntries` checks language, grammar and content; the version test checks schema. |
| AC-04.2 | Met. `TestRealLinkedWorktreesShareOneParseSnapshot` creates an actual Git repository and linked worktree, resolves each through `git.Adapter` and `filesystem.ResolveRuntimePaths`, and observes one parse across both cache instances. |
| AC-04.3 | **Partial, not end-to-end met yet.** `TestCacheLossKeepsStoreFactsStableThroughBridgeFixture` parses real Python, writes through the real `index.Store`, then compares persisted symbol/reference facts after warm, deleted-cache and corrupt-cache runs; parse counts are 1/2/3. This is a test bridge, not the production `IndexFile` path: TASK-05 owns that path and must recheck full extracted facts (including imports/fingerprints) with the cache removed. Read/write-unavailable and tampered entries also recompute without changing returned facts. |
| AC-04.4 | Met at cache level. `TestVersionBumpInvalidatesWithoutDatabaseMutation` increments the cache's internal version seam (simulating a future parser constant bump), observes a miss and second entry, and verifies unchanged bytes at the database path. The cache package imports no database driver/store and never opens the DB. TASK-05 should recheck against a real indexed database path. |

The first focused run was compile-time RED on the missing cache API. A later
runtime RED exposed that JSON `null` could decode as empty facts; canonical
payload validation closed it. The final focused normal and race suites are
GREEN at **84.7% statement coverage** after the FIFO delta below. `make check` (with
`GOCACHE=/tmp/mindrail-go-build`) and `make tidy-check` passed. Cache
read/corruption/write failures are
misses; computation errors and cancellation are returned, never replaced with
a cache error. Writes use an owner-only temporary file, sync/close, rename and
deferred temp cleanup.

### TASK-04 guard mutation ledger (all restored)

Each listed mutation ran its focused test and failed for the intended behavior:

| Guard class | Mutant RED |
|---|---|
| Common-dir placement | A per-worktree root made the linked-worktree test parse twice. |
| Key identity | Omitting grammar collapsed four variants to three entries; fixing the schema version instead of using the version seam collapsed the bump test to one compute/entry. |
| Input validation | Suppressing language/grammar validation computed with an empty language; suppressing the nil-computer check caused the expected nil-call panic. |
| Cancellation | Suppressing the entry check returned identity error instead of cancellation; suppressing the post-compute check wrote an entry after cancellation. |
| Cache read fallback | Treating an absent entry as a hit returned empty facts; suppressing the size cap accepted an oversized valid entry. |
| Record integrity | Suppressing serialized-identity comparison accepted a mismatched schema; suppressing the digest returned tampered `omega` instead of `alpha`; the pre-fix `null` runtime RED proved canonical-shape validation. |
| Fact validation | Suppressing cached-facts validation accepted an empty name; suppressing range bounds accepted an end byte beyond source; suppressing computed-facts validation wrote an invalid range. |
| Compute errors | Suppressing propagation returned nil error for a partial failed computation. |
| Write failure/cleanup | Turning an obstructed-directory return into a panic broke fallback; removing temp cleanup leaked `.snapshot-*` after a forced rename failure. |

The fault-injection scope is bounded. `json.Marshal` of this concrete
DTO/record cannot fail under its present field types (strings, unsigned
integers, booleans and slices; no functions, channels, cyclic pointers or
non-finite floats). Individual `CreateTemp`, `Write`, `Sync` and `Close`
failures were not separately injected; their branches all return from the
void best-effort writer, while blocked-directory and rename-failure tests
exercise the distinct observable outcomes (facts still returned; no temp
leak). A future filesystem abstraction or changed DTO types invalidates those
specific exceptions and needs new guards/tests.

No checkpoint commits were made by the implementation worker, as directed by
the TASK-04 owner. This records implementation evidence, not an independent
Reader/Breaker gate or a claim that TASK-05 is already complete.

### TASK-04 Breaker delta — nonregular cache entries

Breaker found a MEDIUM cache-read robustness gap: replacing the deterministic
entry with a FIFO (directly or through a symlink) made `os.Open` block before
the caller could recheck cancellation. The permanent
`TestNonRegularCacheEntryDoesNotBlock` uses a bounded child process so this
failure cannot hang the package run. Before the fix, the direct-FIFO case was
RED after its four-second child timeout. `readEntry` now uses `os.Lstat` and
requires a regular file before opening, rejecting both the FIFO and its
symlink as cache misses. The direct and symlink cases are GREEN. Disabling
that nonregular guard reproduced the four-second RED, and the guard was
restored.

`TestConcurrentSameKeyWritersLeaveOneReadableEntry` launched 16 same-key
calls together; each returned the same facts, the final entry was readable,
and no temporary files remained. This verifies concurrent-call outcome but
does not force all 16 calls to overlap inside `writeEntry`; simultaneous write
syscalls were not independently proven. The focused normal and race suites
pass after the delta, with 84.7% statement coverage.

The `Lstat`→`Open` sequence has a TOCTOU window: a concurrent actor could
replace a checked regular file with a FIFO between those calls. This patch
closes the deterministic corrupted-entry case, not that hostile concurrent
swap. Eliminating the race would need a platform-specific nonblocking/open
strategy and is outside this narrow delta; no stronger claim is made.

### TASK-04 final gate

The independent Reader gave a **scoped PASS**. The Breaker gave **VERIFIED**
after the FIFO delta; the same independent reviewer performed that follow-up
in a separate adversarial turn. The accepted delta includes the nonregular
FIFO/symlink guard and the 16-call same-key concurrency test. The disclosed
`Lstat`→`Open` TOCTOU limitation remains outside this narrow fix.

The final `GOCACHE=/tmp/mindrail-go-build make verify` exited 0 across vet,
normal tests, race tests and smoke tests. `make tidy-check` exited 0,
`git diff --check` was clean, and `go test -list '.*' ./... | grep -c '^Test'`
reported exactly **946** tests.

This gate accepts TASK-04's cache scope only. **AC-04.3 remains partial**
until TASK-05 exercises warm and deleted-cache equality through the production
`IndexFile` path, including imports and fingerprints; no end-to-end indexing
acceptance is claimed here.

## TASK-05 — structural extraction and the incremental path (implementation evidence)

Scope is `internal/index/indexer.go`, `internal/index/store_cas.go`, the
`internal/index/parser` extraction files (`symbols.go`, `imports.go`,
`references.go`, `extract.go`), the four `imports.scm` query files, the
`internal/index/inventory/files.go` walk, the snapshot DTO/validation update
and the `ParseSchemaVersion` 1→2 bump (D-85: extraction semantics changed
shape, so every stored snapshot invalidates without touching the database).
No scheduler, no readiness wiring, no new CLI surface — those are TASK-06's.

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-05.1 | Karşılandı | `TestCompletePackageExtraction` (parser) + `TestIndexFilePersistsFourLanguagePackageFacts` (store): 4 dilde module function, class method (`method`, container bağlı), import formları (plain/aliased/from — Python; default/named/namespace — JS/TS), aynı-dosya çağrısı; satırlar store üzerinden doğrulanır. TASK-03'ün bıraktığı 3 önkoşul kapandı: Python method etiketi (`function`→`method` ebeveyn class ise), TS overload'lar tek sembole indirgenmez (3 ayrı `Symbol`, tek `LocalKey`), JS generator + arrow declaration atlanmaz. |
| AC-05.2 | Karşılandı | `TestFingerprintSeparatesBodyAndSignature` + `TestIndexFileStoredFingerprintsSeparateBodyAndSignature`: body değişimi yalnız `body_hash`'i değiştirir (`signature`/`structure` sabit); signature değişimi `signature_hash` + `structure_hash`'i değiştirir (`body` sabit). Structure = declaration/signature projeksiyonu + signature baytları; eşit şekilli rename structure'ı değiştirir. `TestMethodBodyDoesNotChangeEnclosingClassStructure` + Python body-leading-comment testi izolasyonu kapatır. |
| AC-05.3 | Karşılandı | `TestIndexFileUniqueAndAmbiguousCalls` + `TestCASCompletionResolvesOnlyUniqueSameKey`: tek aynı-`logical_key` satırı `resolved_symbol_id` doldurur; sıfır/çok satır NULL bırakır, edge `STRUCTURAL_NAME_MATCH` + confidence ≤ 0.5 taşır. `Store`, `TargetLogicalKey` ile çözümü completion transaction'ı içinde yapar; receiver'lı (`obj.helper`) ve shadow'lu çağrılar hedef üretmez. |
| AC-05.4 | Karşılandı | `TestIndexFileUnchangedIndexedSkipsParseAndAllSQLWrites`: değişmemiş hash'te parse çağrısı sıfır, dört tabloya INSERT/UPDATE/DELETE trigger'ıyla sıfır yazı. Sayaç/gözlemle, zamanlamayla değil. |
| AC-05.5 | Karşılandı | `TestIndexFileChangedReplacesOnlyOwnFacts`: değişen dosyanın eski satırları replace edilir (append değil), kardeş dosyanın `symbols.id`'si oynamaz. |
| AC-05.6 | Karşılandı (birim ölçek) | `TestIndexFileCancellationLeavesTargetPendingAndRetries` + `TestIndexFileResumeSkipsCompletedAndProcessesOnlyRemainder`: cancel ortası pending hedef hash'i bırakır, resume yalnız kaydedilmemiş kalanı parse eder (2 çağrı). Süreç ölçeği (kill/restart) AC-07.2'ye aittir. |
| AC-04.3 borcu | Kapatıldı (TASK-04'ten devir) | `TestIndexFileWarmDeletedAndCorruptCacheProduceSameDurableFacts`: production `IndexFile` yoluyla warm/deleted/corrupt cache aynı durable fact'leri üretir (sembol + import + fingerprint + resolved reference dahil); parse sayıları 1/sıcak-0/silinmiş-1/bozuk-1. TASK-04'ün "test köprüsü değil üretim yolu" şartı karşılandı. |

Tasarım-doğruluk notları:

- `Indexer.IndexFile`, parse'u SQLite yazı transaction'ı dışında yapar
  (`TestIndexFileNeverParsesInsideWriteTransaction`); kayıt (`RegisterFileCAS`)
  ve tamamlama (`ReplaceFileFactsCAS`) aynı hedef-hash + generation token'ını
  transaction içinde recheck eder; stale tamamlama no-op'tur. Kaynak
  okuma-yazma-yeniden-okuma (`readSource` + `sourceStillMatches`) üç noktada
  koşar; commit sonrası değişim yeni pending hedef kuyruklar, tamamlanmış eski
  fact'leri silmez; silinmiş/okunamaz kaynak completion'ı invalidate eder —
  daha yeni registration'a dokunmadan.
- Snapshot DTO artık `parser.Facts`'in tamamıdır (import + fingerprint +
  aralıklar); `validFacts` partial-error'lı, hash'siz, anahtarsız, kapsam-dışı
  fact'i cache'e yazmaz. `ParseSchemaVersion = 2` bump'ı D-85'in öngördüğü
  seam üzerinden eski snapshot'ları geçersiz kılar.
- `LocalKey` hash'li tuple'dır (derinlikte 64 bayt sınırında,
  `TestLocalKeysRemainBoundedAndUnambiguousAtDepth`); overload'lar anahtarı
  paylaşır, store tek-satır kuralıyla çözer (D-87). Kalıcı `logical_key`
  path-qualified JSON çiftidir (`qualifiedKey`); D-83 uyarınca UNIQUE değildir.
- `inventory.Files` yalnızca fiziki repo root'u yürür; symlink izlemez,
  `.git/.mindrail/node_modules` ve linked-worktree root'larını atlar,
  deterministic path sırası döner (TASK-06'nın schedule girdisi).

### TASK-05 guard mutasyon defteri (tamamı geri alındı)

Her mutant focused testi kırmızıya döndürdü; dosyalar backup'tan restore
edilip md5 ile doğrulandı:

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | `symbols.go`: signature/body hash girdileri takas edildi | `TestFingerprintSeparatesBodyAndSignature`: "body contaminated signature/structure" FAIL |
| M2 | `store.go` `resolveUniqueSymbol`: `len(ids) != 1` → `len(ids) == 0` (ambiguous da çözülür) | `TestCASCompletionResolvesOnlyUniqueSameKey`: ambiguous pointer dolu FAIL |
| M3 | `indexer.go`: unchanged-hash skip devre dışı | `TestIndexFileUnchangedIndexedSkipsParseAndAllSQLWrites`: `Skipped:false`, parse/yazı FAIL |
| M4 | `snapshot/cache.go` `validFacts`: `HasSyntaxErrors` reddi devre dışı | `TestCompleteFactsValidation/partial_success`: "malformed facts accepted" FAIL |
| M5 | `store_cas.go` `ReplaceFileFactsCAS`: CAS token eşitliği devre dışı | `TestCompletionCASRejectsStaleH1BeforeDeletingH2Facts`: stale H1 `applied=true` FAIL |

(Yanlış kurulan ilk M1 — `bodyBytes` kullanılmayıp derleme hatası verdi —
geçersiz sayılıp derlenen takas mutantıyla tekrarlandı; yukarıdaki M1
doğru mutanttır.)

### TASK-05 kapı (PENDING — bağımsız değerlendirme çalışmadı)

Tam doğrulama: `GOCACHE=/tmp/mindrail-go-build make check` yeşil (exit 0),
`git diff --check` temiz, test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **996** (TASK-04 kapısında
946 idi, +50).

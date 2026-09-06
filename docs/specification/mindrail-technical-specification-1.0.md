# Mindrail 1.0 — Teknik Spesifikasyon

> **Durum:** Technical Specification 1.0 — uygulanabilir hedef mimari  
> **Tarih:** 2026-08-31  
> **Ürün:** Mindrail  
> **Amaç:** AI coding agent'ları için kalıcı engineering context, multi-agent koordinasyonu, semantic destekli kod etki analizi ve kanıt tabanlı tamamlama katmanı.

---

# 1. Bu spesifikasyonun amacı ve versiyonlama modeli

Mindrail'in hedef mimarisi; coding agent'ların geçici context'ine güvenmek yerine engineering state'i kalıcı, doğrulanabilir ve agent-vendor bağımsız şekilde yönetmektir.

Bu dokümandaki **1.0**, ürün binary'sinin ilk public/stable release numarası olmak zorunda değildir. Bu değer bu dokümanın **target technical specification version**'ıdır.

Versiyonlar ayrı tutulur:

```text
TECHNICAL SPECIFICATION
Mindrail Technical Specification 1.0
        │
        │ hedef davranış ve mimari kontrat
        ▼

PRODUCT DELIVERY
0.1 Kernel
0.2 Impact hardening
0.3 Semantic expansion
...
1.0 Stable Product
```

Bu ayrım önemlidir. Technical Specification 1.0 nihai hedef mimariyi tanımlayabilir; ilk kullanılabilir ürün bütün capability'leri aynı release'te teslim etmek zorunda değildir.

Mindrail'in ürün değeri bir code indexer veya memory database olmak değildir.

> **Mindrail is an engineering gate for coding agents.**

Index, semantic resolver, task memory ve coverage bilgisi bu gate'in altyapısıdır. Ürünün çekirdek çıktısı şudur:

```text
Agent Change
    ↓
Discover Actual Change
    ↓
Understand Impact
    ↓
Protect Invariants
    ↓
Require Current Evidence
    ↓
ALLOW / DENY
```

Bu doküman hedef mimariyi tanımlar. İlk implementation scope'u ayrıca `mindrail-0.1-kernel-scope.md` içinde sınırlandırılır.

---

# 2. Mindrail'in temel problemi

AI coding agent'ları uzun süren projelerde dört ana problemi tekrar tekrar yaşar.

## 2.1 Context kaybı

Session kapandığında veya context büyüdüğünde:

- ne yapılmıştı,
- neden yapılmıştı,
- hangi davranışlar korunmalıydı,
- hangi task devam ediyordu,
- başka agent ne üzerinde çalışıyordu

bilgileri kaybolur veya karışır.

## 2.2 Semantic conflict

Git textual conflict'i yakalayabilir.

Ancak şu problemi çözemez:

```text
Agent A:
abc() metodunu oluşturdu.

Agent B:
abc() metodunu daha sonra değiştirdi.

abc() compile oluyor.
abc() unit testi geçiyor.

Ancak abc()'yi kullanan başka bir davranış bozuldu.
```

Bu bir merge conflict değildir.

Bu bir **semantic impact** problemidir.

## 2.3 Intent kaybı

Kodun neden bu şekilde olduğu source'tan anlaşılamayabilir.

Örnek:

```text
"Bu lock neden var?"
"Bu None check neden kaldırılmamalı?"
"Neden timeout negatif olamıyor?"
```

Bunların cevabı yalnız eski chat context'inde kalmamalıdır.

## 2.4 Kanıtsız tamamlanma

Agent'ın:

```text
"Task tamamlandı."
```

demesi proof değildir.

Mindrail'in temel prensibi devam eder:

> **No completion without evidence.**

Ancak 1.0'de evidence yalnız otomatik test anlamına gelmez.

---

# 3. Mindrail ne değildir?

Mindrail:

- LLM memory database'i değildir.
- Chat geçmişi arşivi değildir.
- Vector database değildir.
- AI code reviewer değildir.
- Compiler değildir.
- Full static analyzer değildir.
- Test generator değildir.
- CI sistemi değildir.
- Git'in yerine geçmez.
- IDE'nin yerine geçmez.

Mindrail şudur:

> **Coding agent'ları ile repository arasında çalışan deterministic engineering coordination kernel.**

## 3.1 Mevcut araçlarla ilişki

Mindrail'in ayırt edici tarafı index veya arama değil, **gate**'tir.

| Kategori | Sağladığı | Mindrail'in farkı |
|---|---|---|
| LSP/MCP tabanlı kod gezginleri | symbol arama, referans bulma | Mindrail bu sinyali tüketir; değer bulguyu üretmek değil, bulguyu completion kararına bağlamaktır |
| Memory/context sunucuları | serbest metin hatırlama | Mindrail versiyonlanmış, şemalı, review edilebilir Decision/Invariant tutar |
| Static analysis / code scanning | ihlal listesi | Mindrail ihlali lease ve snapshot'a bağlı evidence ile birleştirip completion'ı bloke eder |
| CI | bağımsız doğrulama | Mindrail CI'ın yerine geçmez; CI sonucunu snapshot'a bağlı `CI_VERIFICATION` evidence'ı olarak tüketir |

Bu nedenle mevcut bir indexer veya language server Mindrail'in yerine geçmez; en fazla Mindrail'in code intelligence katmanını besler.

---

# 4. Mindrail'in beş temel sorusu

Her agent için Mindrail şu soruları cevaplamalıdır:

| Soru | Kaynak |
|---|---|
| Ben neredeyim? | Task + Checkpoint |
| Bu kod neden böyle? | Decision |
| Ne doğru kalmak zorunda? | Invariant |
| Bu değişiklik nereye yayılabilir? | Syntax + Semantic Graph + Runtime/Test Signals |
| Güvenli olduğunu ne kanıtlıyor? | Evidence |

Kısa form:

> **Remember intent. Protect invariants. Validate impact.**

---

# 5. 1.0'in ana mimari değişikliği

İlk tasarım:

```text
Tree-sitter
    ↓
Symbol Graph
    ↓
Impact
```

1.0:

```text
                   ┌────────────────┐
                   │   Tree-sitter  │
                   │ Syntax Layer   │
                   └───────┬────────┘
                           │
                   ┌───────▼────────┐
                   │ Semantic Layer │
                   │ Pyright / TS   │
                   │ Resolver       │
                   └───────┬────────┘
                           │
             ┌─────────────▼────────────┐
             │ Normalized Code Graph    │
             │ symbols + typed edges    │
             └─────────────┬────────────┘
                           │
         ┌─────────────────┼─────────────────┐
         │                 │                 │
    Static Impact      Test/Coverage     Explicit
       Graph              Graph          Invariants
         │                 │                 │
         └─────────────────┼─────────────────┘
                           │
                    Impact Engine
                           │
                  Validation Planner
                           │
                    Evidence Gate
```

Tree-sitter artık **tek analysis engine değil**, syntax foundation'dır.

---

# 6. Agent independence

Mindrail hiçbir vendor'a bağımlı olmayacaktır.

Desteklenen agent entegrasyon modeli:

```text
Claude Code
Codex
Cursor
VS Code agents
JetBrains agents
custom agents
future agents
```

Mindrail core bunlardan hiçbirini özel olarak tanımak zorunda değildir.

Primary interface:

```text
MCP
```

Fallback:

```text
CLI
```

Vendor-specific config yalnız integration adapter seviyesindedir.

---

# 7. Data plane ayrımı

Mindrail 1.0 veriyi üç ayrı doğruluk alanına ayırır.

## 7.1 Engineering Knowledge

Uzun ömürlü ve repository ile taşınması gereken bilgi:

```text
Project Facts
Decisions
Invariants
Validation Policies
Risk Policies
```

Bunlar **version-controlled** olmalıdır.

## 7.2 Runtime Coordination

Geçici/operasyonel state:

```text
Agent Sessions
Task Leases
Change Leases
Open Changes
Workspace State
Temporary Events
Runtime Index Mapping
```

Bunlar local SQLite'ta tutulur.

## 7.3 Proof

Belirli bir source snapshot'a bağlı doğrulama:

```text
Test Evidence
Build Evidence
Typecheck Evidence
Manual Evidence
CI Evidence
```

Proof local DB'de tutulabilir ancak gerektiğinde portable manifest üretilebilir.

---

# 8. Repository yapısı

Önerilen 1.0 yapı:

```text
repo/
├── .mindrail/
│   ├── config.toml
│   ├── project.md
│   │
│   ├── knowledge/
│   │   ├── decisions/
│   │   │   ├── DEC-0001.json
│   │   │   └── DEC-0002.json
│   │   │
│   │   └── invariants/
│   │       ├── INV-0001.json
│   │       └── INV-0002.json
│   │
│   └── policies/
│       ├── validation.toml
│       └── risk.toml
│
├── AGENTS.md
└── ...
```

Agent protokolü `.mindrail/` altında ayrı bir dosyada tutulmaz; §117'de tanımlanan managed section olarak `AGENTS.md` içinde yaşar. Böylece protokol, agent'ın zaten okuduğu dosyada bulunur ve tek kaynağı olur.

Runtime DB:

```text
<GIT_COMMON_DIR>/mindrail/mindrail.db
```

Runtime cache:

```text
<GIT_COMMON_DIR>/mindrail/cache/
```

---

# 9. Neden Decision ve Invariant dosya tabanlı?

Sadece SQLite'ta tutulurlarsa:

```text
Developer A laptop
    ↓
mindrail.db

CI clone
    ↓
Mindrail knowledge yok
```

Bu kabul edilemez.

Decision ve Invariant:

- Git ile versionlanabilir,
- code review görebilir,
- branch ile taşınabilir,
- CI tarafından okunabilir,
- başka makinedeki Mindrail tarafından yeniden indexlenebilir.

Her kayıt ayrı dosya olmalıdır.

Tek büyük JSONL dosyası önerilmez.

Sebep:

```text
multi-agent Git conflict
```

riskini azaltmak.

---

# 10. Runtime SQLite'ın görevi

SQLite yalnız hızlı operational state/index için kullanılır.

Önerilen ayarlar:

```text
journal_mode = WAL
foreign_keys = ON
busy_timeout = 5000
synchronous = NORMAL
```

Ancak daha önemli kural:

> **Uzun süren hiçbir iş SQLite write transaction açıkken yapılmaz.**

Şunlar DB transaction dışında çalışmalıdır:

- Tree-sitter parse,
- semantic resolver query,
- test çalıştırma,
- build,
- typecheck,
- coverage import,
- Git diff hesaplama.

DB transaction yalnız sonucu atomic commit etmek için kullanılır.

---

# 11. SQLite concurrency ve interactive write fairness

Normal flow:

```text
READ STATE
   ↓
RELEASE DB
   ↓
EXPENSIVE WORK
(parse / resolver / validation)
   ↓
SHORT WRITE TRANSACTION
   ↓
COMMIT RESULT
```

Birden fazla Mindrail process aynı SQLite DB'yi kullanabilir.

## WAL + bounded busy retry

`SQLITE_BUSY` durumunda bounded exponential backoff + jitter uygulanır.

Örnek:

```text
25ms
50ms
100ms
200ms
400ms
```

Sonsuz retry yoktur. Budget sonunda:

```text
MINDRAIL_BUSY_RETRYABLE
```

üretilir.

## Idempotency key

Mutating application request'leri `operation_id` taşır. Aynı operation retry edildiğinde duplicate Task/Change/Evidence oluşturulmaz.

## Optimistic revision

Mutable entity'lerde `revision` kullanılır.

```text
WHERE id = ? AND revision = expected
```

eşleşmezse:

```text
STATE_REVISION_CONFLICT
```

üretilir.

## Immutable/content-addressed rows

```text
code_blobs
parse_snapshots
evidence artifacts
```

mümkün olduğunca immutable tutulur ve hash bazlı duplicate insert engellenir.

## Cold-index write fairness

Cold indexer SQLite'ın tek-yazar davranışını uzun bulk transaction ile monopolize etmemelidir.

Cold index persistence küçük chunk'lara bölünür.

Başlangıç default'u:

```text
max_rows_per_index_transaction = 200
```

Bu sayı config/benchmark ile ayarlanabilir.

Akış:

```text
compute parse batch outside DB
      ↓
write <= configured chunk
      ↓
commit
      ↓
yield / allow competing interactive writer
      ↓
next chunk
```

Binlerce symbol/edge tek transaction içinde yazılmaz.

Interactive operations (`claim`, lease, `after_change` result commit, evidence state) küçük transaction olarak kalmalıdır.

## Process-local writer scheduling

Mindrail process'i kendi içinde cold-index batch'lerini interactive write request'lerinden daha düşük öncelikte schedule edebilir. Bu mekanizma cross-process global broker değildir ve SQLite'ın correctness modelinin yerine geçmez.

## Neden global write broker yok?

Target birkaç local concurrent agent'tır. Global broker/daemon eklemek IPC, lifecycle ve recovery complexity getirir.

Önce şu metrikler ölçülür:

```text
busy retry count
p95 write wait
revision conflict rate
write transaction duration
cold-index rows/transaction
```

Gerçek contention kabul edilemez seviyeye çıkarsa repository-local single-writer coordinator ayrı bir mimari karar olarak değerlendirilir.

---

# 12. Content-addressed index cache

Workspace başına bütün AST ve symbol verisini kopyalamak yerine 1.0 content-addressed cache kullanır.

## File Blob

```text
file_content_hash
```

aynıysa parse sonucu tekrar saklanmaz.

Örnek:

```text
Workspace A / src/auth.py
hash = ABC

Workspace B / src/auth.py
hash = ABC
```

İki ayrı symbol tree üretmek yerine:

```text
CODE_SNAPSHOT ABC
```

ortak kullanılır.

Workspace yalnız:

```text
workspace
path
→ content_hash
```

mapping'i tutar.

---

# 13. Content cache modeli

Mantıksal tablolar:

```text
code_blobs
parse_snapshots
snapshot_symbols
snapshot_edges
workspace_files
```

Örnek:

```text
workspace_files
-------------------------
workspace_a
src/auth.py
hash_123

workspace_b
src/auth.py
hash_123
```

İki workspace aynı parse snapshot'ını kullanır.

Bu:

- DB boyutunu azaltır,
- re-index maliyetini düşürür,
- worktree çoğalmasını daha ucuz hale getirir.

---

# 14. Code intelligence tiers

1.0 dil desteği "Tree-sitter parser var mı?" şeklinde tanımlanmaz.

Destek seviyesi semantic capability'ye göre belirlenir.

## FULL

Şart:

```text
Tree-sitter
+
Semantic Resolver
+
Import resolution
+
Reference resolution
+
Type-aware symbol identity
```

1.0 hedef:

```text
Python
TypeScript
JavaScript
```

## STRUCTURAL

Şart:

```text
Tree-sitter
+
symbols
+
imports
+
obvious references
```

Caller/callee doğruluğu sınırlıdır.

## FILE

Yalnız:

```text
Git diff
file risk policy
validation profile
```

kullanılır.

---

# 15. Hedef language scope

## Full support

### Python

Zorunlu stack:

```text
Tree-sitter Python
+
Pyright semantic/type service
```

Pyright bulunamıyorsa:

```text
analysis_mode = STRUCTURAL
```

olur.

Mindrail bunu FULL olarak raporlamaz.

### TypeScript / JavaScript

Zorunlu stack:

```text
Tree-sitter TS/JS
+
TypeScript Language Service
```

TS project context (`tsconfig`) mümkün olduğunca kullanılmalıdır.

---

# 16. C#, Java, Go, PHP

1.0'de:

```text
LIMITED / EXPERIMENTAL
```

olarak tanımlanır.

Bu dillerde parser eklenebilir.

Ancak semantic resolver tamamlanmadan Mindrail:

```text
FULL impact analysis supported
```

iddiasında bulunmaz.

İleride:

```text
C#   → Roslyn
Java → JDT LS
Go   → gopls
PHP  → language-server/static-analysis adapter
```

eklenebilir.

---

# 17. Tree-sitter'ın rolü

Tree-sitter şu işler için kullanılır:

- syntax tree,
- symbol boundaries,
- source range,
- structural fingerprints,
- syntax-level declarations,
- import syntax,
- basic call syntax,
- incremental changed ranges.

Tree-sitter'ın görevi:

```text
"Bu kodun syntax yapısı nedir?"
```

sorusunu cevaplamaktır.

Şu sorunun kesin cevabı değildir:

```text
"Bu call runtime'da hangi implementation'a gider?"
```

---

# 18. Semantic Resolver'ın rolü

Semantic Resolver:

- symbol binding,
- type information,
- import resolution,
- find references,
- implementation resolution,
- overload/reference resolution

sağlar.

Normalized semantic adapter contract:

```text
resolve_symbol(location)
find_references(symbol)
find_definition(reference)
find_implementations(symbol)
get_type(location)
resolve_import(location)
project_health()
```

Core resolver'ın vendor API'sini bilmez.

Adapter normalize eder.

---

# 19. Semantic resolver kurulum, lifecycle ve resource policy

FULL semantic analysis yalnız parser bulunmasıyla değil, ilgili semantic resolver'ın **doğru ProjectUnit için doğru configuration ile sağlıklı çalışmasıyla** mümkündür. Resolver lifecycle ve kaynak yönetimi core capability'dir.

## Resolver Manager sorumlulukları

```text
ProjectUnit discovery
resolver discovery
version compatibility
configuration selection
process supervision
health probing
resource budgeting
restart policy
diagnostic normalization
cache invalidation
managed resolver integrity
shutdown
```

Mindrail resolver'ın işini yeniden implement etmez; resolver'ları supervised external analysis service olarak kullanır.

## Resolver çalışma kaynakları

```text
PROJECT_LOCAL
EXPLICIT
MANAGED
UNAVAILABLE
```

Resolution precedence:

```text
1. Project-local compatible resolver
2. Explicitly configured resolver
3. Mindrail-managed pinned resolver
4. UNAVAILABLE
```

Managed resolver project'in dependency manifest'lerini değiştirmez.

## Managed resolver integrity

Managed resolver executable/package indirilmeden önce release metadata'sında expected version ve SHA-256 bulunmalıdır.

Akış:

```text
download temporary artifact
        ↓
verify expected version
        ↓
verify SHA-256
        ↓
verify signature/provenance when provider supports it
        ↓
atomic install into Mindrail cache
        ↓
execute
```

Checksum uyuşmazsa artifact execute edilmez ve:

```text
RESOLVER_INTEGRITY_FAILED
```

üretilir.

## ProjectUnit discovery

Monorepo tek semantic project sayılmaz. Mindrail repository inventory sırasında `ProjectUnit` üretir.

Python candidate roots:

```text
pyproject.toml
pyrightconfig.json
setup.cfg
setup.py
configured source roots
```

TypeScript/JavaScript candidate roots:

```text
tsconfig.json
jsconfig.json
package.json workspace boundaries
configured source roots
```

Bir dosya birden fazla ProjectUnit'e aday oluyorsa seçim deterministic olmalıdır:

```text
1. explicit config mapping
2. nearest valid semantic config
3. workspace/package boundary
4. repository default unit
```

Ambiguous seçim:

```text
PROJECT_UNIT_AMBIGUOUS
```

olarak raporlanır; Mindrail sessizce rastgele config seçmez.

## Resolver instance modeli

Her resolver instance en az şunları taşır:

```text
project_unit_id
workspace_id
semantic_snapshot_key
language
root
config_file
resolver_kind
resolver_version
resolver_source
process_id
health
generation
restart_count
last_probe_at
last_success_at
last_used_at
estimated_memory_mb
diagnostics
```

Resolver reuse yalnız semantic olarak aynı state için yapılabilir. Farklı worktree'ler aynı ProjectUnit adına sahip olsa bile source/config snapshot farklıysa aynı mutable resolver process'i körlemesine paylaşılmaz.

Pool key minimum:

```text
workspace/snapshot identity
+
ProjectUnit
+
config hash
+
resolver version
```

## Resolver health state machine

```text
DISCOVERED
   ↓
BOOTING
   ↓
HEALTHY
   ├──→ DEGRADED
   ├──→ STALE
   └──→ CRASHED

MISCONFIGURED
UNAVAILABLE
EVICTED
```

`semantic: FULL` yalnız resolver `HEALTHY` ise ilan edilir.

## Resolver resource budget

Resource budget açıkça tanımlanmalıdır.

Kritik kısıt: pool key workspace/snapshot identity içerdiği için resolver process sayısı yaklaşık

```text
aktif workspace sayısı × aktif ProjectUnit sayısı
```

ile büyür. Her kuruluma sabit küçük bir tavan (örn. `4`) dayatmak, çok worktree'li veya monorepo kurulumlarda sürekli evict/restart döngüsü üretir. Pyright/TypeScript servisi soğuk başlatma maliyeti saniyeler mertebesinde olduğu için bu döngü §28 interactive SLO'larını doğrudan ihlal eder.

## Default budget türetme

`max_processes` sabit bir sayı olarak dayatılmaz; discovery sonrası türetilir:

```text
derived_max_processes =
    clamp(
        active_project_units,
        min  = 2,
        max  = min(configured_ceiling, cpu_based_cap, memory_based_cap)
    )
```

Explicit config her zaman türetilen değeri override eder.

```toml
[resolver.resources]
max_processes = "auto"        # veya explicit sayı
max_memory_mb = 4096
idle_ttl_minutes = 10
min_residency_seconds = 120
query_timeout_ms = 3000
```

## Eviction ve thrash koruması

Eviction serbest değildir. Şunlar evict edilemez:

```text
aktif task'ın target ProjectUnit resolver'ı
devam eden bir request'e servis veren instance
min_residency_seconds dolmamış instance
```

`idle_ttl_minutes` yalnız bu korumaların dışındaki instance'lara uygulanır.

Budget davranışı:

```text
requested resolver
      ↓
capacity available? ── yes → start/use
      │
      no
      ↓
evict edilebilir idle instance var mı?
      │                          │
     yes                         no
      ↓                          ↓
    evict                RESOLVER_RESOURCE_LIMIT
      ↓                          ↓
   start/use            policy: degrade → STRUCTURAL
                                veya block
```

Kapasite yoksa **sıcak bir resolver'ı düşürmek yerine yeni talebi reddetmek** tercih edilir.

`on_resolver_unavailable = degrade` ise ilgili ProjectUnit için efektif capability `STRUCTURAL` olarak raporlanır. Bu sessiz bir düşüş değildir; `mindrail doctor` nedeni ve önerilen aksiyonu gösterir.

## Thrash ölçümü

En az şu metrikler tutulur:

```text
resolver start count
resolver eviction count
min_residency içinde evict→restart (thrash count)
resolver cold start duration
RESOLVER_RESOURCE_LIMIT count
```

Thrash oranının yükselmesi performans hatası olarak raporlanır; sessizce tolere edilmez.

Memory ölçümü platforma göre best-effort olabilir; `max_processes` deterministic hard guard olarak uygulanır.

Her resolver query timeout-bound'dur. Uzun `find_references` request'i interactive path'i sınırsız bloke edemez.

## Restart budget

Default kavramsal policy:

```text
restart_window = 5 minutes
max_restarts = 3
backoff = bounded exponential
```

Budget aşılırsa state `CRASHED` olur. Policy `degrade` veya `block` uygular.

## Generation

Her resolver restart/reconfiguration sonrası:

```text
generation += 1
```

Eski generation'a bağlı semantic cache current sayılmaz.

## `mindrail doctor` resolver teşhisi

`mindrail doctor` yalnız `Pyright unavailable` dememelidir. En az şunları göstermelidir:

```text
ProjectUnit
workspace/snapshot
requested capability
resolver kind/source/path
resolver version
selected config
config discovery reason
health
generation
resource state
query timeout
blocking diagnostics
effective capability
why FULL is unavailable
suggested next actions
```

Örnek:

```text
ProjectUnit: apps/admin
Resolver: TypeScript Language Service
Source: PROJECT_LOCAL
Configuration: apps/admin/tsconfig.json
Health: DEGRADED
Effective semantic capability: STRUCTURAL

Why FULL is unavailable:
- referenced project ../shared missing

Suggested action:
- fix tsconfig reference
- run mindrail doctor --unit apps/admin --verbose
```

Secret/environment değerleri dump edilmez.

## Capability vector

```text
syntax: FULL
semantic: FULL
test_impact: RUNTIME_SYMBOL
framework_edges: LIMITED
```

Tek bir `FULL/LIMITED` etiketi kullanılmaz.

## Config

```toml
[analysis.python]
semantic = "required"
resolver = "auto"
on_resolver_unavailable = "degrade"

[analysis.typescript]
semantic = "required"
resolver = "auto"
on_resolver_unavailable = "degrade"
```

Security-sensitive repository `block` seçebilir.

## Resolver acceptance

Bir ProjectUnit `semantic: FULL` sayılmadan önce:

```text
resolver starts
project config loads
representative symbol resolves
cross-file reference query succeeds
health probe succeeds
resource policy satisfied
```

kontrollerini geçmelidir.

---

# 20. Confidence artık global tek sayı değildir

İlk tasarımdaki:

```text
analysis_confidence = LOW
```

tek başına yetersizdir.

1.0 confidence üç seviyede tutulur:

```text
edge confidence
finding confidence
analysis coverage
```

## Edge confidence

Belirli dependency için:

```text
abc → foo
0.98
```

## Finding confidence

Örneğin:

```text
"RadiusReplyBuilder impacted"
confidence = 0.91
```

## Analysis coverage

Mindrail'in repository'nin ilgili alanını ne kadar güvenilir çözebildiğini gösterir.

Örnek:

```text
semantic_resolver = healthy
dynamic_edges = 3 unresolved
parse_errors = 0
coverage = 0.87
```

---

# 21. Critical invariant + düşük confidence politikası

Düşük confidence otomatik:

```text
ALLOW
```

veya:

```text
BLOCK
```

üretmez.

Escalation policy çalışır.

Örnek:

```text
Critical invariant
+
semantic coverage low
```

ise Mindrail:

```text
targeted static tests
        ↓
broader module validation
        ↓
integration validation
        ↓
manual/human evidence if policy allows
```

şeklinde daha geniş proof ister.

Prensip:

> **Uncertainty increases required evidence.**

---

# 22. Framework magic

Mindrail aşağıdaki implicit ilişkilerin static analysis'ten kaçabileceğini kabul eder:

- dependency injection,
- decorators/attributes,
- routing,
- reflection,
- ORM hooks,
- signals/events,
- registries,
- plugin loading,
- magic string dispatch.

Bunları çözmek için framework adapter extension point bulunur.

Örnek normalized edges:

```text
ROUTES_TO
HANDLES
INJECTS
EMITS
SUBSCRIBES
ORM_CALLBACK
REGISTERS
```

1.0 core bunları bilmek zorunda değildir.

Adapter edge registry extensible olur.

---

# 23. Dynamic reference handling

Örnek:

```python
handler = getattr(service, action)
handler()
```

Mindrail:

```text
target_not_resolved
```

der.

Asla:

```text
no_dependency
```

demez.

Finding:

```text
DYNAMIC_DISPATCH
scope: service
confidence: unresolved
```

olarak kaydedilir.

Risk planner gerekirse daha geniş validation scope seçer.

---

# 24. Durable symbol identity, rename ve orphan protection

Line number veya mutable qualified name tek başına durable symbol identity değildir.

Mindrail iki ayrı identity taşır:

```text
symbol_uid     → durable internal identity
logical_key    → current human/query identity
```

Örnek:

```text
symbol_uid:
SYM-01J...

logical_key:
python:src/auth/session.py:SessionService.calculate_timeout:method
```

Invariant, lease history, evidence relation ve test-impact relation mümkün olduğunda `symbol_uid` ile bağlanır.

## symbol_uid tahsisi ve determinizm

`symbol_uid` opaque bir değerdir; bu nedenle normatif olan **nasıl üretildiği** değil, **nasıl tam olarak bir kez tahsis edildiği**dir.

Birden fazla Mindrail process'i (farklı worktree'ler, paralel cold indexer, eşzamanlı `reconcile`) aynı symbol'ü aynı anda ilk kez görebilir. Bu durumda iki farklı `symbol_uid` üretilmesi kabul edilemez; aksi halde aynı symbol'ün invariant, evidence ve lease ilişkileri ikiye bölünür.

Kural:

> **`symbol_uid` proje başına, identity lineage başına tam olarak bir kez tahsis edilir.**

First-observation allocation key minimum:

```text
project_id
+ language
+ owner lineage (parent symbol_uid veya module path)
+ kind
+ declaration name
+ overload discriminator (varsa)
```

Tahsis akışı:

```text
compute allocation key
        ↓
atomic insert-if-absent
(allocation key üzerinde unique constraint)
        ↓
read back existing/inserted symbol_uid
        ↓
use returned uid
```

Kurallar:

- Tahsis **insert-then-read-back** ile yapılır. `read → yoksa insert` sırası yarışa açıktır ve kullanılmaz.
- Content-addressed parse snapshot'ın paylaşılması `symbol_uid`'in paylaşıldığı anlamına gelmez; uid tahsisi ayrı ve atomik bir işlemdir.
- Unique constraint ihlali hata değildir; mevcut uid'in okunmasıyla sonuçlanır.
- Allocation key **mutable**'dır (rename/move ile değişir). Bu nedenle `allocation key → symbol_uid` eşlemesi bir **index**tir, kimliğin kendisi değildir.
- Identity migration bu index kaydını yeniden yazar; yeni uid üretmez.
- Yeni uid tahsisi yalnız identity migration denendikten ve başarısız olduktan sonra yapılabilir; aksi halde her rename sessizce yeni bir symbol yaratır.

## Rename/move identity migration

Örnek:

```text
SessionService.calculate_timeout
            ↓ rename
SessionService.compute_timeout
```

Bu durumda yeni symbol yaratıp eski invariant'ı orphan bırakmak kabul edilmez.

Identity migration sinyalleri:

```text
Git rename/move signal
semantic resolver declaration/reference identity
parent symbol_uid
body_hash
structure_hash
semantic_signature_hash excluding mutable name
source-range neighborhood
```

Yeterli confidence varsa:

```text
SYMBOL_IDENTITY_MIGRATED
SYM-01J...
old logical_key → new logical_key
```

üretilir.

## Ambiguous identity

Bir removed symbol birden fazla added symbol'e eşleşebiliyorsa Mindrail tahminle migration yapmaz.

```text
SYMBOL_IDENTITY_AMBIGUOUS
```

üretilir.

Eğer ambiguous symbol üzerinde active HIGH/CRITICAL invariant veya blocking evidence relation varsa completion block edilir.

## Orphan protection

Tracked invariant scope'lanan symbol kaybolduğunda üç sonuçtan biri zorunludur:

```text
1. identity migrated
2. invariant explicitly superseded/retired
3. SYMBOL_IDENTITY_AMBIGUOUS / ORPHANED_PROTECTED_SYMBOL block
```

Invariant sessizce etkisiz hale gelemez.

## Overload ve nested symbol

Identity key aşağıdakileri gerektiğinde içerir:

```text
owner symbol_uid
kind
overload discriminator / semantic signature
lexical nesting
```

Decorator/wrapper nedeniyle declaration identity değişiyorsa semantic resolver ve structure fingerprint birlikte değerlendirilir.

Rename detection best-effort'tur; güvenli eşleştirme yapılamadığında fail-open değil explicit ambiguity kullanılır.

---

# 25. Symbol fingerprint

Minimum:

```text
signature_hash
body_hash
structure_hash
```

Ek:

```text
semantic_signature_hash
```

Semantic signature:

- resolved parameter types,
- return type,
- generic constraints,
- visibility/export,
- interface/base relation

üzerinden türetilebilir.

---

# 26. Change detection

Bir change için baseline:

```text
before_change snapshot
```

olacaktır.

Yalnız Git HEAD kullanılmaz.

Baseline:

- current HEAD,
- dirty state,
- relevant file hashes,
- symbol fingerprints,
- semantic project snapshot/version

içerir.

---

# 27. after_change parse maliyeti

`after_change` bütün repository'yi parse etmez.

Sıra:

```text
Git/file watcher
   ↓
changed files
   ↓
changed byte ranges
   ↓
Tree-sitter incremental reparse
   ↓
changed symbols
   ↓
semantic resolver targeted refresh
```

Tree-sitter changed ranges kullanılabiliyorsa yalnız ilgili syntactic bölgeler yeniden değerlendirilir.

Fallback olarak whole changed-file parse kabul edilebilir.

Bir dosyayı yeniden parse etmek genellikle repository'yi yeniden parse etmekten çok daha ucuzdur.

---


# 28. Interactive performance contract

Mindrail agent'ın inner loop'unda bulunduğu için doğruluk kadar latency de product contract'tır.

Warm repository için başlangıç SLO hedefleri:

| Operation | STRUCTURAL warm p95 | FULL semantic warm p95 |
|---|---:|---:|
| `mindrail_status` | < 150 ms | < 150 ms |
| `mindrail_context` | < 250 ms | < 250 ms |
| `mindrail_before_change` | < 300 ms | < 500 ms |
| `mindrail_after_change`, ≤10 changed files | < 1.5 s | < 2.5 s |
| `mindrail_reconcile`, ≤10 changed files | < 2 s | < 3 s |

SLO capability mode'a bağlıdır. `semantic: FULL` modda resolver refresh ölçülebilir ek maliyet getirir; bu maliyet tek bir hedefin altında varsayılarak yok sayılmaz.

Kritik kural:

> **SLO analizi kısarak değil, erteleyerek karşılanır.**

Gereken closure bütçe içinde tamamlanamıyorsa Mindrail cevabı bloke etmez; kısmi sonucu açık bir pending state ile döndürür ve eksik kısmı yüksek öncelikle schedule eder.

Kabul edilmeyen iki davranış:

```text
❌ bütçeyi aşıp interactive path'i bloke etmek
❌ analizi sessizce atlayıp sonucu "temiz" göstermek
```

Bu değerler ilk benchmark'larla kalibre edilebilir; kaldırılmamalı, ölçülmelidir.

## Interactive vs deep path

Senkron interactive path yalnız completion/coordination için gereken minimum closure'ı hesaplar.

Aşağıdakiler interactive request'i gereksiz yere bloke etmemelidir:

```text
full cold index
repository-wide coverage import
large low-priority fan-out materialization
calibration report
non-blocking cache enrichment
```

Blocking policy daha fazla proof gerektiriyorsa Mindrail bunu açık state olarak döndürür; sessizce 30 saniyelik analiz yapmaz.

Örnek:

```text
status: ANALYSIS_PENDING
blocking_component: semantic_closure
next_action: mindrail_status / mindrail_validate
```

## Latency telemetry

En az şu metrikler local olarak ölçülür:

```text
operation duration p50/p95
parse duration
resolver query duration
SQLite wait duration
impact traversal duration
reconcile duration
```

Performance regression release acceptance'ın parçasıdır.

---

# 29. Large repository cold-index lifecycle

`mindrail init` FULL repository index'i beklemez.

State:

```text
UNINITIALIZED
    ↓
INVENTORY
    ↓
PARTIAL_READY
    ↓
INDEXING
    ↓
READY
```

`PARTIAL_READY`, Mindrail'in knowledge/task/lease işlevlerinin kullanılabildiği fakat bütün ProjectUnit'lerin analysis capability'sinin hazır olmadığı anlamına gelir.

## Progress

Mindrail sahte ETA üretmez.

Gösterilen ölçüler:

```text
files discovered
files hashed
files parsed
bytes processed
project units ready
project units pending
semantic jobs pending
coverage imports pending
errors
```

## Resumability

Index job kayıtları idempotent olmalıdır:

```text
job_id
item_key
phase
content_hash
state
attempt
last_error
updated_at
```

Completed content hash yeniden parse edilmez.

Crash/SIGINT sonrası worker kaldığı yerden devam eder.

## Cooperative scheduler

Cold-index scheduler priority queue kullanır fakat çalışan işi agresif biçimde kill/preempt etmez.

Priority sınıfları:

```text
P0 current reconcile/before_change target
P1 target dependency closure / relevant tests
P2 active task ProjectUnit
P3 active workspace
P4 repository cold remainder
```

Scheduler:

- bounded worker pool kullanır,
- yeni yüksek öncelikli işi bir sonraki available slot'a alır,
- in-flight parse işini normalde yarıda kesmez,
- resolver request'lerinde bounded concurrency uygular,
- starvation önlemek için priority aging uygular.

## Priority aging

Uzun süredir P4'te kalan job zamanla effective priority kazanır.

Amaç:

```text
aktif task hızlı
+
cold index sonunda ilerliyor
```

dengesidir.

## ProjectUnit reservation

Bir ProjectUnit üzerinde semantic resolver initialization pahalıysa scheduler aynı unit için duplicate startup job oluşturmaz.

State:

```text
NOT_STARTED
STARTING
READY
FAILED
```

CAS ile korunur.

## Target-first davranış

Agent unindexed symbol için reconcile veya `mindrail_before_change` ile aktif hedef belirlenirse:

```text
1. target file hash/parse P0
2. containing ProjectUnit semantic readiness P0/P1
3. direct dependency closure P1
4. relevant test map P1
```

queue'ya yükseltilir.

Mindrail bu request için gereken minimum analysis closure hazır olduğunda before_change devam eder; bütün repository'nin READY olması gerekmez.

## Foreground ve local worker

Kullanıcı:

```bash
mindrail index --foreground
```

ile index'i ön planda çalıştırabilir.

Normal kullanımda local worker process kullanılabilir.

Bu worker remote/cloud background service değildir; repository'nin local Mindrail çalışma modelinin parçasıdır.

## Kesilme

Worker yok olursa Mindrail state bozulmuş sayılmaz.

Sonraki:

```text
mindrail bootstrap
mindrail index --resume
before_change
```

gereken job'ları tekrar schedule eder.

---

# 30. Large repository optimizasyonları

Minimum:

- content-addressed parse snapshots,
- changed-file indexing,
- Tree-sitter incremental ranges,
- ProjectUnit partitioning,
- cooperative priority scheduler,
- target-first scheduling,
- lazy semantic reference resolution,
- bounded parser workers,
- bounded resolver workers,
- duplicate job suppression,
- resumable jobs,
- priority aging.

`mindrail_status`:

```text
index state
worker state
queue depth by priority
current ProjectUnits
failed jobs
resolver bottlenecks
```

gösterir.

`mindrail doctor --index` scheduler starvation, repeated failure, resolver bottleneck ve cache hit-rate gibi teşhisleri gösterir.

---

# 31. Impact Engine'in yeni rolü

Risk score safety decision değildir.

Impact Engine iki çıktı üretir:

```text
1. Impact Set
2. Validation Requirements
```

Risk score yalnız:

- sıralama,
- UX,
- validation breadth başlangıç noktası

için kullanılır.

Completion yalnız score'a göre engellenmez.

---

# 32. Impact kaynakları

Impact Engine tek graph'a güvenmez.

Kaynaklar:

```text
Semantic references
Syntax references
Inheritance/implementation
Framework edges
Coverage/test execution edges
Explicit invariants
Path policies
Public API rules
Recent change history
```

Her kaynak provenance taşır.

---

# 33. Impact graph

Edge örnekleri:

```text
CALLS
REFERENCES
IMPORTS
EXTENDS
IMPLEMENTS
ROUTES_TO
HANDLES
INJECTS
TEST_COVERS
```

Her edge:

```text
origin
confidence
resolver
snapshot
```

taşır.

---

# 34. Risk score kalibrasyonu

İlk tasarımdaki:

```text
100 × weights × depth
```

gibi formül yalnız heuristic'tir.

1.0'de:

> **Score policy kararının yerine geçmez.**

Default score korunabilir ancak config ile tamamen değiştirilebilir.

```toml
[risk.thresholds]
critical = 80
high = 60
medium = 35
```

Edge weight'ler de config'tedir.

Ancak hard rules score'dan bağımsızdır.

---

# 35. Hard policy rules

Örnek:

```text
public API signature change
→ minimum HIGH

critical invariant scope
→ critical validation policy

known caller bulunan symbol removal
→ minimum HIGH

security path
→ project configured minimum

database migration
→ migration validation policy
```

Bu kurallar risk score'dan daha yüksek önceliklidir.

---

# 36. Impact explosion ve drill-down modeli

Base class, yaygın interface veya utility değişikliği binlerce symbol'e yayılabilir.

Mindrail iki problemi ayrı çözer:

```text
1. Analiz bilgisi kaybolmamalı.
2. Agent context'i patlamamalı.
```

Bu nedenle impact sonucu DB'de tam tutulur, MCP context'te varsayılan olarak aggregate edilir.

## Fan-out boundary

Örnek:

```text
direct dependents > 100
```

ise finding:

```text
IMPACT_FANOUT
```

oluşur.

Default summary:

```text
affected packages: 5
affected modules: 18
public implementations: 34
direct symbols: 142
transitive symbols: 811
```

Ancak bu bir bilgi kaybı değildir.

Her aggregate group bir stable `impact_group_id` taşır.

Örnek:

```text
IG-42 auth package
IG-43 billing package
```

Agent gerektiğinde drill-down isteyebilir.

`mindrail_context`:

```text
detail_level = summary | focused | full
```

destekler.

Ayrıca:

```text
target_type = impact_group
target_id = IG-42
cursor = ...
page_size = ...
```

ile belirli grubun exact symbol'leri sayfalı şekilde alınabilir.

`full` mode yine tek response'a sınırsız symbol basmaz; pagination zorunludur.

Prensip:

> **Aggregation is a presentation boundary, not an analysis boundary.**

---

# 37. Traversal boundary ve ayrıntı erişimi

Traversal şu sınırlarda aggregate edilebilir:

- package boundary,
- module boundary,
- service boundary,
- configured architecture boundary,
- max graph cost,
- max fan-out.

Traversal'ın aggregation yapması edge'lerin DB'den silinmesi anlamına gelmez.

Validation planner gerektiğinde:

```text
symbol-level
→ module-level
→ package-level
→ service-level
```

scope'a yükselir.

Agent belirli bir aggregate grubun detayını `mindrail_context` drill-down ile açabilir.

Böylece iki uçtan kaçınılır:

```text
❌ bütün repository'yi context'e basmak
❌ yalnız "5 package etkilendi" deyip ayrıntıyı kaybetmek
```

---

# 38. Test mapping ve coverage-provider modeli

Test mapping tek bir language-specific özellik değildir.

Mindrail normalized bir **Coverage Provider Contract** tanımlar.

Test selection kaynakları confidence sırasıyla:

```text
1. Runtime coverage mapping
2. Explicit invariant ↔ test mapping
3. Semantic symbol reference
4. Static import/reference
5. Path/module convention
6. Naming heuristic
```

Her test selection provenance taşımalıdır.

1.0'de production-ready provider hedefleri:

```text
Python
TypeScript / JavaScript
```

Diğer diller LIMITED olabilir; ancak core coverage modeli baştan language-neutral olmalıdır.

---

# 39. Runtime test impact map ve generic coverage contract

Coverage Provider normalize olarak şunu üretir:

```text
CoverageRun
TestIdentity
ExecutedLocation
SourceSnapshot
```

Minimum contract:

```text
discover() -> ProviderHealth

collect(command/profile) -> CoverageRun

normalize(run) -> [
    {
      test_id,
      source_file,
      start_line,
      end_line,
      optional_symbol,
      snapshot_hash
    }
]
```

Mindrail provider-specific formatı Impact Engine'e taşımaz.

Normalize edilen relation:

```text
TEST_COVERS
```

edge'idir.

Örnek:

```text
test_otp_replay
    ↓ TEST_COVERS
OtpService.consume
```

Coverage relation source snapshot'a bağlıdır ve source/test yapısı değişirse stale olabilir.

---


# 40. Coverage capability modeli

Coverage/test-impact desteği semantic analysis seviyesinden ayrı raporlanır.

Capability değerleri:

```text
NONE
STATIC
RUNTIME_FILE
RUNTIME_RANGE
RUNTIME_SYMBOL
```

Bir dilin semantic resolver'ının FULL olması coverage'ın da FULL olduğu anlamına gelmez.

Örnek:

```text
C#:
syntax = FULL
semantic = FULL
test_impact = STATIC
```

Bu destek modeli gelecekte C#, Java ve Go provider'ları eklenirken aynı kalite kontratını korur.

Her Coverage Provider normalize şu contract'ı sağlamalıdır:

```text
provider_health()
list_tests()
import_run(run_artifact)
map_test_to_ranges(test_id)
map_ranges_to_symbols(ranges)
provenance()
```

Provider capability'si açıkça raporlanır. Mindrail `RUNTIME_SYMBOL` sağlamayan provider için symbol-exact test mapping iddiasında bulunmaz.

Yeni bir dil `test_impact = RUNTIME_SYMBOL` olarak ilan edilmeden önce adapter fixture + integration acceptance testlerini geçmelidir.


---

# 41. Python coverage mapping

Python FULL support için coverage adapter ayrı capability olarak raporlanır.

Örnek health:

```text
semantic: FULL
coverage: AVAILABLE
```

veya:

```text
semantic: FULL
coverage: UNAVAILABLE
```

Pytest/coverage integration kullanılıyorsa test context → executed lines ilişkisinden symbol mapping üretilir.

Coverage provider yoksa Python FULL semantic analysis çalışmaya devam eder; yalnız test-selection confidence düşer ve validation breadth gerektiğinde yükselir.

---

# 42. TypeScript/JavaScript coverage mapping

TypeScript/JavaScript provider aşağıdaki runner/coverage formatlarına adapter ile bağlanabilir:

```text
Vitest
Jest
Node test runner
generic LCOV/coverage JSON importer
```

Mindrail runner'a gömülü olmaz.

Provider'ın görevi yalnız normalize edilmiş:

```text
test_id
source_file
covered_range
snapshot
```

üretmektir.

Range daha sonra current code snapshot içindeki symbol'e resolve edilir.

---

# 43. Coverage yoksa ve yeni dillere genişleme

Coverage yoksa Mindrail precise test mapping uydurmaz.

Fallback:

```text
semantic direct test relations
        ↓
static relations
        ↓
module validation profile
        ↓
package/service validation profile
```

Prensip:

> **Low mapping confidence → broader validation, not guessed precision.**

C#, Java, Go veya başka dil FULL desteğe yükseltildiğinde acceptance kriterlerinden biri yalnız semantic resolver değil, uygun coverage/test-impact provider veya açıkça tanımlanmış güçlü fallback strategy olacaktır.

---

# 44. Test mapping provenance

Her seçilen test veya suite açıklanabilir olmalıdır.

Örnek:

```text
test_radius_reply

selected because:
runtime coverage → RadiusReplyBuilder.build

provider:
python-coverage

confidence:
0.98
```

Fallback örneği:

```text
tests/auth

selected because:
no reliable per-test mapping

fallback:
module validation profile

mapping confidence:
LOW
```

Mindrail "bu test ilgili" demekle yetinmez; selection provenance döndürür.

---

# 45. Validation evidence türleri

Evidence türleri:

```text
AUTOMATED_TEST
TYPECHECK
BUILD
LINT
INTEGRATION_TEST
RUNTIME_PROBE
MANUAL_VERIFICATION
HUMAN_APPROVAL
CI_VERIFICATION
EXTERNAL_SYSTEM
```

Her evidence:

```text
id
type
actor
actor_assurance
source
procedure
reason
timestamp
source_snapshot
result
trust_level
attachments/references
```

taşır.

`MANUAL_VERIFICATION`, `HUMAN_APPROVAL` ve administrative `OVERRIDE` aynı şey değildir.

---

# 46. Manual verification

Manual verification:

```text
"Bu davranış insan tarafından şu prosedürle kontrol edildi."
```

demektir.

Örnek:

```text
physical device üzerinde login flow doğrulandı
```

Manual evidence için zorunlu alanlar:

```text
procedure
expected_result
observed_result
reason
actor
snapshot_hash
```

`"manuel test ettim"` tek başına geçerli evidence değildir.

Default policy:

| Severity | Manual-only evidence |
|---|---|
| LOW | allowed |
| MEDIUM | configurable |
| HIGH | normally insufficient |
| CRITICAL | signed human approval veya stronger evidence gerekir |

---

# 47. Human approval kimlik ve yetkilendirme modeli

`HUMAN_APPROVAL`, normal automated evidence'dan ayrı bir **approval provider** üzerinden üretilir. Agent'ın serbestçe üretebildiği bir string approval sayılmaz.

## Öncelik: imza değil bağımsız güven sınırı

CRITICAL değişiklikler için 1.0 default yaklaşımı:

```text
1. Reproducible automated evidence
2. Independent CI verification
3. Gerekliyse managed/external human approval
4. Local signed approval yalnız project policy gerektiriyorsa
```

Bu nedenle her geliştiricinin SSH/GPG approval sistemi kurması zorunlu değildir.

En rahat ve önerilen ekip modeli:

> **Critical change için independent CI identity + protected merge policy.**

Mindrail CI provider'dan gelen sonucu `CI_VERIFICATION` veya `EXTERNAL_VERIFIED` evidence olarak kabul edebilir.

## Approval assurance seviyeleri

```text
LOCAL_INTERACTIVE
SIGNED_LOCAL
MANAGED_IDENTITY
EXTERNAL_VERIFIED
```

### LOCAL_INTERACTIVE

- interactive TTY,
- OS user/uid,
- host,
- `--reason`,
- exact snapshot

kaydeder.

Güçlü identity değildir.

CRITICAL için default olarak yeterli değildir.

### SIGNED_LOCAL

Canonical approval payload local SSH/GPG/hardware-backed key ile imzalanır.

Private key Mindrail'e girmez.

Bu yol opt-in'dir; her developer'a zorunlu onboarding gereksinimi değildir.

### MANAGED_IDENTITY

CI runner, enterprise identity broker veya repository platformunun doğrulanmış workload/user identity'si kullanılır.

Özellikle ekip kullanımında tercih edilen modeldir.

### EXTERNAL_VERIFIED

Mindrail dışı approval provider'ın cryptographically veya API-level doğruladığı approval'dır.

## Approval payload

```text
project_fingerprint
change_id
snapshot_hash
invariant_ids
policy_id
reason
approver_identity
provider
issued_at
expires_at?
nonce
```

Approval exact snapshot'a bağlıdır.

Source değişirse approval stale olur.

## Authorization

Project policy kimlerin hangi seviyede approval verebildiğini tanımlar.

Örnek:

```toml
[approval.critical]
accepted = ["CI_VERIFICATION", "MANAGED_IDENTITY"]
require_independent_actor = true
```

`require_independent_actor=true` ise change'i oluşturan AgentSession ile aynı actor identity approval şartını karşılamaz.

## Audit

Her approval:

```text
actor
provider
assurance
reason
snapshot
policy
verification result
timestamp
```

ile append-only evidence kaydı oluşturur.

## Önemli ayrım

`HUMAN_APPROVAL` normal evidence workflow'udur.

`OVERRIDE` ise policy dışına çıkma işlemidir.

Bir approval policy'yi karşılıyorsa override değildir.

---

# 48. Evidence stale kuralı

Evidence belirli snapshot'a bağlıdır.

```text
source snapshot S1
↓
test passed
↓
source changed to S2
```

S1 evidence artık:

```text
STALE
```

dir.

Bu kural bütün evidence türleri için geçerlidir.

---

# 49. Validation profiles

Validation command'ları whitelist edilir.

Agent arbitrary shell command'i Mindrail validation olarak çalıştıramaz.

Config örneği:

```toml
[validation.auth_unit]
paths = ["src/auth/**"]
commands = ["pytest -q tests/auth"]

[validation.typecheck]
commands = ["pyright"]

[validation.web]
paths = ["web/**"]
commands = ["pnpm typecheck", "pnpm test"]
```

---

# 50. Validation planner

Planner şu input'ları kullanır:

```text
change classification
impact set
invariants
analysis coverage
test mapping confidence
path policy
public API status
```

Sonuç:

```text
required
recommended
informational
```

olarak ayrılır.

---

# 51. Validation kararı örneği

```text
Change:
OtpService.consume BODY_CHANGED

Invariant:
INV-17 CRITICAL

Semantic coverage:
HIGH

Runtime test mapping:
HIGH

Required:
- test_otp_replay
- test_concurrent_consume
- auth integration profile

Recommended:
- auth package tests
```

---

# 52. "No completion without evidence" nasıl esnek kalır?

Prensip değişmez.

Ancak:

```text
evidence != automated tests only
```

dir.

Mindrail:

```text
"Proof olmadan done diyemezsin."
```

der.

Fakat proof projenin doğasına göre değişebilir.

Bu yaklaşım hem güvenli hem gerçekçidir.

---

# 53. Decision

Decision şu sorunun cevabıdır:

```text
"Neden bu yolu seçtik?"
```

Immutable record.

Yeni karar eskisini overwrite etmez.

```text
DEC-20
supersedes DEC-12
```

---

# 54. Invariant

Invariant:

```text
"Değişiklikten sonra ne doğru kalmak zorunda?"
```

Örnek:

```text
OTP once consumed cannot be consumed again.
```

Scope:

```text
PROJECT
PACKAGE
MODULE
FILE
SYMBOL
```

---

# 55. Decision/Invariant manuel yük problemi ve candidate modeli

Decision ve Invariant tamamen otomatik üretilmez.

Ancak Mindrail deterministic **candidate** kayıtları üretebilir.

Candidate invariant kaynakları 1.0'de açıkça sınırlıdır:

```text
ASSERTION_CONTRACT
REGRESSION_TEST
EXPLICIT_AGENT_CANDIDATE
PUBLIC_CONTRACT_CHANGE
REPEATED_FAILURE_SIGNAL
```

Candidate hiçbir zaman active invariant değildir.

Alanlar:

```text
candidate_id
trigger_type
scope
raw_predicate
source_locations
related_tests
related_change
confidence
created_at
status
```

Status:

```text
OPEN
PROMOTED
DISMISSED
STALE
```

---

# 56. Candidate invariant tetikleme ve promotion kuralı

Mindrail candidate'dan kendi başına natural-language engineering truth üretmez.

Örneğin yeni test:

```python
assert timeout >= 0
```

eklendiyse Mindrail:

```text
ASSERTION_CONTRACT candidate

raw_predicate:
timeout >= 0

related symbol:
SessionService.calculate_timeout

source:
test_session_timeout.py
```

oluşturabilir.

Ancak otomatik olarak:

```text
INV-42: Session timeout must never be negative.
```

persist etmez.

Promotion sırasında agent/human:

- statement,
- rationale,
- scope,
- severity,
- verification mapping

sağlar veya doğrular.

---

# 57. Candidate invariant trigger'ları

## Trigger 1 — ASSERTION_CONTRACT

Yeni/değişmiş assertion semantic/coverage relation ile changed production symbol'e bağlanabiliyorsa candidate oluşturulur.

## Trigger 2 — REGRESSION_TEST

Task içinde production change ile beraber yeni regression test eklendiyse:

```text
test
related changed symbols
previous failing evidence varsa reference
```

ile candidate oluşturulur.

Mindrail invariant cümlesini uydurmaz.

## Trigger 3 — EXPLICIT_AGENT_CANDIDATE

Agent önemli bir contract keşfeder fakat hemen active invariant yapmak istemez:

```text
mindrail_invariant(mode="candidate")
```

kullanabilir.

## Trigger 4 — PUBLIC_CONTRACT_CHANGE

Public schema/type/API precondition veya postcondition değişikliğinde candidate önerilebilir.

## Trigger 5 — REPEATED_FAILURE_SIGNAL

Aynı symbol/behavior için tekrarlanan validation regression görülürse candidate önerisi oluşturulabilir.

Bu trigger yalnız suggestion üretir.

Candidate spam engeli:

- aynı scope + normalized predicate duplicate edilmez,
- dismissed candidate aynı snapshot'ta tekrar önerilmez,
- stale source kaldırılırsa candidate STALE olur.

---

# 58. Task modeli

Task:

```text
OPEN
CLAIMED
IN_PROGRESS
BLOCKED
READY_TO_COMPLETE
COMPLETED
ABANDONED
```

Task ownership lease ile korunur.

---

# 59. Task lease

Default:

```text
20 minutes
```

Her anlamlı Mindrail call lease'i renew edebilir.

Agent crash olursa lease expire olur.

Lease history event olarak kalır.

---

# 60. Symbol/change lease

`before_change` target için lease oluşturur.

Tercih:

```text
logical symbol
```

Fallback:

```text
file
```

Same file different symbol:

```text
allowed with warning
```

özellikle ayrı Git worktree kullanılıyorsa.

---

# 61. Multi-agent çalışma modları

Mindrail birden fazla çalışma modelini destekler.

## Mode A — Same workspace, sequential agents

Bugünkü Claude Code/Codex kullanımında dominant basit senaryodur.

```text
same repository directory
Agent A finishes/releases
Agent B continues
```

Mindrail:

- session identity,
- task lease,
- checkpoint,
- reconcile,
- current workspace snapshot

ile continuity sağlar.

Bu modda worktree zorunlu değildir.

## Mode B — Separate worktrees, concurrent agents

Concurrent mutation için **önerilen isolation mode**:

```text
Agent A → worktree A / branch A
Agent B → worktree B / branch B
```

Task/symbol lease project seviyesinde paylaşılır; source/index workspace seviyesinde ayrılır.

## Mode C — Same workspace, concurrent agents

Desteklenebilir fakat reduced isolation olarak raporlanır.

Riskler:

```text
filesystem overwrite
attribution ambiguity
baseline drift
```

Bu modda reconcile ve lease kontrolleri daha konservatif davranabilir.

> **1 agent = 1 worktree bir requirement değil; concurrent development için recommended isolation pattern'dır.**

Git textual integration'ı, Mindrail semantic coordination'ı yönetir.

---

# 62. Scope drift

Agent target dışında symbol değiştirdiyse:

```text
SCOPE_DRIFT
```

oluşur.

Mindrail yeni changed symbol'ü otomatik analiz scope'una ekler.

Sonra:

```text
lease conflict?
impact expansion?
validation expansion?
```

kontrol edilir.

---

# 63. Scope drift maliyeti

Mindrail tüm repo'yu reparse etmez.

Değişmiş dosyalar:

```text
Git diff
+
file watcher
```

ile bulunur.

Yalnız o dosyalar yeniden analiz edilir.

Bu nedenle scope drift detection'ın maliyeti repository boyutuyla doğrusal olmak zorunda değildir.

---

# 64. Change discovery agent protokolünden bağımsızdır

Agent'ın `mindrail_before_change` çağırmış olması correctness için zorunlu varsayım değildir.

Source-of-truth değişiklik keşfi:

```text
Git/worktree/staged diff
+
content snapshot
+
syntax/semantic delta
```

üzerinden yapılır.

`mindrail_before_change` şu faydaları sağlar:

```text
lease acquisition
pre-change invariant warning
known impact preview
clean baseline attribution
```

Ancak agent bu çağrıyı atlarsa Mindrail değişikliği görmezden gelmez.

Pre-commit, completion veya explicit reconcile sırasında actual diff keşfedilir.

Açık Change'e güvenli şekilde bağlanamayan change:

```text
UNREGISTERED_CHANGE
```

olarak sınıflanır ve canonical reconcile akışına girer.

Bu bir olağanüstü recovery path değil, desteklenen normal correctness path'idir.

---

# 65. Reconcile canonical change-discovery workflow

`mindrail_reconcile` Mindrail'in canonical **actual change discovery and attribution** mekanizmasıdır.

MCP:

```text
mindrail_reconcile
```

CLI:

```bash
mindrail reconcile
```

Ayrıca completion/pre-commit/verify tarafından gerektiğinde otomatik tetiklenebilir.

Input:

```text
workspace
task?
staged/worktree diff
open changes
optional ownership hints
```

Akış:

```text
1. actual changed files çıkarılır
2. changed symbols/structural deltas çıkarılır
3. durable symbol identity migration uygulanır
4. mevcut open Change'lerle exact attribution denenir
5. lease/scope information yardımcı sinyal olarak kullanılır
6. ambiguous ownership ayrılır
7. unowned delta için retroactive Change oluşturulabilir
8. impact analysis yapılır
9. required evidence planı oluşturulur
```

Prensip:

> **before_change improves coordination; reconcile provides correctness.**

Bu sayede Mindrail agent'ın protokol adımlarına %100 uymasına bağımlı değildir.

---

# 66. Ambiguous reconcile

Örnek:

```text
file.py:
symbol A → Change X
symbol B → Change Y
symbol C → unknown
```

Mindrail:

```text
C → otomatik X
```

demez.

Sonuç:

```text
RECONCILE_AMBIGUOUS
```

ve agent/human explicit assignment yapmalıdır.

---

# 67. MCP tool surface

Public protocol henüz legacy compatibility taşımadığı için bütün MCP tool adları ürün adıyla başlar.

```text
mindrail_bootstrap
mindrail_status
mindrail_search
mindrail_context

mindrail_claim

mindrail_before_change
mindrail_after_change
mindrail_reconcile

mindrail_decide
mindrail_invariant

mindrail_checkpoint

mindrail_validate
mindrail_complete
```

Toplam 13 tool.

`mindrail_context` fan-out drill-down için:

```text
detail_level
impact_group target
cursor
page_size
```

alanlarını destekler.

Hiçbir legacy tool alias'ı tanımlanmaz. Henüz yayınlanmış public compatibility yüzeyi olmadığı için geriye dönük alias taşıma yükümlülüğü yoktur; isim değiştirmenin en ucuz olduğu aşama public release öncesidir.

---

# 68. mindrail_status

Amaç:

```text
şu anda Mindrail'in durumu ne?
```

Döndürür:

- session,
- claimed task,
- open changes,
- leases,
- index health,
- semantic resolver health,
- validation status,
- blocking issues.

---

# 69. mindrail_search

Amaç:

- decision bulmak,
- invariant bulmak,
- task bulmak,
- symbol bulmak.

Search:

```text
exact
FTS5
qualified-name
path
```

Vector search yoktur.

---

# 70. mindrail_context ve drill-down

`mindrail_context` yalnız relevance-selected context packaging yapar.

Input örnekleri:

```text
target_type = task | change | symbol | impact_group
target_id
detail_level = summary | focused | full
cursor?
page_size?
```

Default:

```text
summary
```

`impact_group` veya `full` taleplerinde pagination uygulanır.

Mindrail hiçbir zaman tool response size sınırını aşacak şekilde sınırsız graph döndürmez.

Context output'ta:

```text
has_more
next_cursor
```

bulunabilir.

---

# 71. Context budget

Default:

```text
~2500 token equivalent
```

Asla düşürülmemesi gerekenler:

- task goal,
- blocking invariant,
- hard conflict,
- current change,
- required next action.

---

# 72. Git enforcement

Local:

```text
pre-commit
```

kontrolü vardır.

Ancak 1.0'de bu tek hard gate değildir.

---

# 73. CI verification 1.0 kapsamındadır

CI en az:

```bash
mindrail verify --ci
```

çalıştırabilmelidir.

CI yeni clone'da:

```text
.mindrail/config
.mindrail/knowledge
.mindrail/policies
```

üzerinden Mindrail knowledge'ı yeniden kurar.

Runtime task lease gerekmez.

---

# 74. CI ne doğrular?

Örnek:

```text
base commit
→ PR HEAD
```

diff'i çıkarır.

Sonra:

- changed symbols,
- applicable invariants,
- public API rules,
- required validation profiles,
- Mindrail knowledge consistency

kontrol edilir.

CI gerekli validations'ı yeniden çalıştırabilir.

---

# 75. Local evidence ile CI evidence farkı

Local:

```text
developer evidence
```

CI:

```text
independent environment evidence
```

olarak tutulur.

Policy:

```text
HIGH/CRITICAL
```

değişikliklerde CI evidence zorunlu kılınabilir.

---

# 76. Portable change manifest

Task runtime DB CI'a taşınmaz.

Gerekirse local Mindrail:

```text
.mindrail-manifest
```

benzeri temporary/CI artifact üretir.

İçeriği:

- change classification,
- source snapshot,
- relevant invariant IDs,
- validation requirements.

Bu manifest Git'e commit edilmek zorunda değildir.

CI aynı sonucu yeniden hesaplayabiliyorsa manifest yalnız optimization'dır.

---

# 77. Secret redaction

Validation output'u DB'ye yazmadan önce redaction pipeline çalışır.

Sıra:

```text
raw process output
↓
size limit
↓
exact known-secret redaction
↓
pattern redaction
↓
structured sanitizer
↓
bounded excerpt
↓
storage
```

---

# 78. Known-secret redaction

Config ile secret-bearing env isimleri tanımlanabilir:

```text
TOKEN
SECRET
PASSWORD
API_KEY
PRIVATE_KEY
AUTH
```

Mindrail value'ların kendisini loglamaz.

Process output'ta exact value görülürse:

```text
[REDACTED]
```

yapılır.

Mindrail bütün environment'ı evidence olarak saklamaz.

---

# 79. Pattern redaction

Default pattern set örnek kategorileri:

- bearer tokens,
- private key blocks,
- common API token formats,
- database URLs with credentials,
- password-like key/value output.

Regex sistemi configurable olur.

False positive nedeniyle orijinal output ayrı bir logda tutulmaz.

Redaction sonrası output canonical evidence text olur.

---

# 80. Validation log saklama

Default:

```text
exit code
duration
command id
output hash
bounded sanitized excerpt
snapshot hash
```

Tam raw output saklanmaz.

Config ile explicit debugging açılabilir ancak secret policy yine uygulanır.

---

# 81. Developer UX

Mindrail adoption agent workflow kadar önemlidir.

Minimum human UX:

```bash
mindrail init
mindrail doctor
mindrail status
mindrail explain
mindrail search
mindrail index
mindrail knowledge validate
mindrail verify
```

`mindrail doctor` yalnız "healthy/unhealthy" yazan komut değildir.

Özellikle resolver ve monorepo sorunlarında diagnostic rehberdir.

---

# 82. mindrail init UX

`mindrail init`:

```text
1. repository/Git common-dir keşfi
2. .mindrail config initialization
3. knowledge schema validation
4. ProjectUnit discovery
5. resolver discovery
6. test/build tool discovery
7. validation candidate üretme
8. file inventory
9. runtime DB/cache initialization
10. PARTIAL_READY index oluşturma
11. optional Git hook setup
12. agent integration instructions
13. cold index worker başlatma/foreground seçeneği
```

Init bütün cold semantic index'i beklemek zorunda değildir.

Existing config sessizce overwrite edilmez.

Init sonunda kullanıcıya:

```text
READY FOR TARGETED WORK
```

veya:

```text
BLOCKED: <reason>
```

durumu açık gösterilir.

---

# 83. mindrail doctor ve setup health report

Örnek:

```text
Mindrail Doctor

Repository
✓ Git common-dir detected
✓ Knowledge schema v2 valid

Project Units
✓ services/api
  Language: Python
  Config: services/api/pyproject.toml
  Resolver: Pyright 1.x
  Source: managed
  Health: HEALTHY

△ apps/admin
  Language: TypeScript
  Config: apps/admin/tsconfig.json
  Resolver: project-local TypeScript
  Health: MISCONFIGURED

  Diagnostic:
  tsconfig references missing project:
  packages/shared/tsconfig.json

  Impact:
  FULL semantic analysis unavailable for this unit.

  Next:
  fix referenced tsconfig or set analysis.typescript.on_resolver_unavailable=degrade

Index
△ PARTIAL_READY
  Files: 21,442 / 103,882
  Active target units: READY
  Cold queue: 9 units

Validation
✓ pytest detected
✓ pyright profile valid

Git
✓ pre-commit installed

CI
△ Mindrail verify workflow not detected
```

`mindrail doctor` ayrıca:

```bash
mindrail doctor --unit apps/admin
mindrail doctor --resolver
mindrail doctor --knowledge
mindrail doctor --explain <error-code>
```

gibi scoped diagnostics destekleyebilir.

Doctor project config'i kendi kendine değiştirmez; uygulanabilir next action verir.

---

# 84. Error mesajları

Her error:

```text
code
why
impact
next_action
```

taşır.

Örnek:

```text
EVIDENCE_STALE

Why:
Source changed after validation.

Impact:
Current test result no longer proves this snapshot.

Next:
mindrail_after_change CHG-42
mindrail_validate CHG-42
```

---

# 85. Feedback loop

Mindrail kendi impact kalitesini ölçebilmelidir.

1.0 minimum local metrics:

```text
predicted impacted symbols
selected tests
tests actually failed
fallback validation triggered
dynamic unresolved count
reconcile count
false-positive feedback
false-negative feedback
```

---

# 86. User feedback

CLI:

```bash
mindrail feedback CHG-42 --false-positive <finding>
mindrail feedback CHG-42 --missed-impact <symbol>
```

MCP'de 1.0 için zorunlu değildir.

Feedback:

- risk weight'i otomatik değiştirmez,
- local analytics'te saklanır,
- future calibration için kullanılır.

---

# 87. Neden otomatik risk öğrenme yok?

1.0 deterministic kalmalıdır.

Mindrail kendi scoring policy'sini sessizce değiştirmez.

Metrics yalnız:

```text
calibration report
```

üretir.

İnsan config'i değiştirir.

---

# 88. Calibration report

Örnek:

```text
Last 50 changes

HIGH predictions: 18
actual validation failures: 3

IMPORT edge false positives: high
depth-3 usefulness: low

Suggestion:
consider reducing IMPORT weight
```

Bu 1.0 nice-to-have olabilir.

Veri modeli baştan desteklemelidir.

---

# 89. Completion Gate

Completion yalnız risk score'a bakmaz.

Minimum:

```text
task ownership valid
change analyzed
no hard conflict
knowledge index current
code index current
blocking invariants resolved
required evidence current
snapshot matches
unregistered change yok
```

---

# 90. Analysis uncertainty gate

Completion sırasında:

```text
analysis coverage low
```

ise policy kontrol edilir.

Örnek:

```text
LOW risk
→ warning olabilir

HIGH
→ broader validation required

CRITICAL
→ resolver recovery veya stronger evidence required
```

Bu davranış config ile ayarlanır.

---

# 91. Evidence trust

Evidence trust levels:

```text
LOW
MEDIUM
HIGH
INDEPENDENT
```

Örnek:

```text
agent-written note → LOW
Mindrail-run unit test → HIGH
CI rerun → INDEPENDENT
human approval → policy-defined
```

---

# 92. Data truth precedence

Kod konusunda:

```text
current code + semantic index
>
agent note
```

Validation konusunda:

```text
current Mindrail/CI evidence
>
agent statement
```

Intent konusunda:

```text
active Decision
>
old checkpoint
```

Invariant konusunda:

```text
active Invariant
>
superseded Invariant
```

---

# 93. Decision supersede

Records immutable.

```text
DEC-31
status=active

DEC-42
supersedes=DEC-31
```

Mindrail knowledge loader:

```text
DEC-31 → superseded
DEC-42 → active
```

olarak indexler.

---

# 94. Invariant supersede

Aynı model.

Silmek yerine lineage korunur.

Bu stale memory problemini azaltır.

---

# 95. Knowledge schema ve CI doğrulama pipeline'ı

Knowledge store startup ve CI sırasında deterministik loader pipeline'dan geçer.

Her record:

```text
schema_version
kind
id
status
created_at
```

taşır.

Validation sırası:

```text
1. File read
2. JSON syntax
3. schema_version parse
4. reader compatibility
5. JSON Schema validation
6. filename ↔ kind/id consistency
7. unique ID
8. supersede target
9. supersede DAG/cycle
10. duplicate active lineage
11. scope syntax
12. scope resolution when index available
13. referenced validation profile
14. referenced evidence/test mapping syntax
```

## Reader-compatible schema window

Mindrail sürümü iki ayrı kavram ilan eder:

```text
readable_schema_versions
write_schema_version
```

Örnek:

```text
readable = [2,3]
write = 3
```

Repository aynı anda schema 2 ve schema 3 record içerebilir.

Yeni Mindrail:

- eski readable record'u okuyabilir,
- mevcut haliyle kullanabilir,
- yeni/modified record'u current write schema ile yazabilir.

Bu model:

> **read old, write current**

olarak tanımlanır.

Böylece ekip bütün knowledge store'u aynı anda migrate etmek zorunda değildir.

## Lazy per-record upgrade

Schema N-1 record yalnız değiştirildiğinde N formatında yeniden yazılabilir.

Bu normal source edit gibi Git diff üretir.

Bulk migration zorunlu değildir.

## Bulk migration

İstenirse:

```bash
mindrail knowledge migrate
```

bütün readable eski record'ları current write schema'ya dönüştürür.

Bu explicit Git diff'tir ve review edilir.

CI sessiz bulk migration yapmaz.

## Minimum Mindrail version

Knowledge schema/feature eski Mindrail'in anlayamayacağı yeni semantic içerik kullanıyorsa repository config:

```text
min_mindrail_version
```

artırabilir.

Eski client:

```text
MINDRAIL_VERSION_TOO_OLD
```

ile fail eder ve upgrade action gösterir.

## Unknown newer schema

Reader'ın bilmediği daha yeni schema:

```text
FAIL CLOSED
KNOWLEDGE_SCHEMA_UNSUPPORTED
```

olur.

## Feature capability

Sadece schema numarası değil, gerekirse record:

```text
required_capabilities
```

taşıyabilir.

Reader schema'yı okuyabilse bile required capability yoksa record'u sessizce ignore etmez.

## Knowledge doctor

```bash
mindrail doctor --knowledge
```

şunları gösterir:

```text
record count by schema
current writer schema
oldest readable schema
records requiring newer Mindrail
invalid records
supersede problems
unresolved scopes
optional migration count
```

## CI order

`mindrail verify --ci` önce knowledge validation yapar.

Invalid/unsupported blocking knowledge varsa source impact analysis başlamaz.

## Scope resolution

Cold CI clone'da schema doğrulaması semantic index'ten önce yapılabilir.

Ancak blocking symbol-scoped invariant final verification öncesi resolve edilmelidir.

---

# 96. Project facts

`.mindrail/project.md` kısa kalmalıdır.

Şunlar için:

- architecture facts,
- environment rules,
- project-wide constraints.

Task history burada tutulmaz.

---

# 97. Task/checkpoint persistence

Runtime tasks local SQLite'ta kalabilir.

1.0 primary scope:

```text
aynı local Git repository / worktree seti
```

dir.

Cross-machine live task coordination V2 konusudur.

Ancak Decisions/Invariants repository ile taşınır.

---

# 98. MCP state

Mindrail kendi explicit application handles'ını kullanır:

```text
mindrail_session_id
task_id
change_id
```

State transport connection'a bağlı değildir.

Bu agent/client değişimlerini kolaylaştırır.

---

# 99. Session lifecycle

Mutlu yol (agent protokole uyduğunda):

```text
mindrail_bootstrap
        ↓
mindrail_status / mindrail_context
        ↓
mindrail_claim
        ↓
mindrail_before_change      ← koordinasyon optimizasyonu, zorunlu değil
        ↓
EDIT
        ↓
mindrail_after_change
        ↓
mindrail_validate
        ↓
mindrail_complete
```

Correctness yolu (agent protokol adımını atladığında):

```text
EDIT (before_change çağrılmadı)
        ↓
mindrail_reconcile          ← gerçek diff Git'ten keşfedilir
        ↓
mindrail_validate
        ↓
mindrail_complete
```

Bu iki yol ayrı implementasyon değildir. §65 gereği `after_change`, `complete` ve `verify` aynı reconcile engine'ini çağırır; `before_change` yalnız daha iyi baseline ve lease bağlamı sağlar.

`mindrail_complete` her durumda önce reconcile eder. Yukarıdaki mutlu yol, gate'in atlanabileceği anlamına gelmez.

Handoff:

```text
mindrail_checkpoint(handoff=true)
```

---

# 100. before_change

Mindrail:

1. session kontrol eder,
2. task ownership kontrol eder,
3. target resolve eder,
4. lease conflict kontrol eder,
5. Decision/Invariant yükler,
6. dependency snapshot yükler,
7. baseline alır,
8. change oluşturur,
9. compact context döndürür.

---

# 101. after_change

Mindrail:

1. changed files bulur,
2. incremental syntax reparse yapar,
3. changed symbols bulur,
4. semantic resolver refresh yapar,
5. graph delta çıkarır,
6. scope drift kontrol eder,
7. impact set üretir,
8. test mapping üretir,
9. analysis coverage hesaplar,
10. validation plan oluşturur.

---

# 102. Change classifications

Minimum:

```text
SYMBOL_ADDED
SYMBOL_REMOVED
BODY_CHANGED
SIGNATURE_CHANGED
SEMANTIC_SIGNATURE_CHANGED
VISIBILITY_CHANGED
BASE_TYPE_CHANGED
IMPORT_CHANGED
DECORATOR_CHANGED
REFERENCE_CHANGED
FILE_ADDED
FILE_REMOVED
TEST_REMOVED
TEST_CHANGED
UNKNOWN_STRUCTURAL_CHANGE
```

---

# 103. Test weakening ve false-green guard

Bir test zayıfladığında Mindrail yalnız `test passed` sonucuna güvenmez; ucuz structural guard'lar uygular.

## Tetikleme kapsamı

Guard **change-scoped değil, invariant-anchored**'dır.

Tetikleme yalnız "production kodu ve testi aynı Change içinde değişti" koşuluna bağlanırsa şu kaçış önemsiz hale gelir:

```text
Change 1: kritik testi skip et / assertion'ı kaldır
Change 2: production symbol'ü değiştir
```

Bu nedenle guard birbirinden bağımsız üç tetikleyiciden herhangi biriyle çalışır:

```text
1. Aynı Change içinde production + test değişikliği
2. Aktif bir invariant'a mapped test'in zayıflaması
   (o Change hiçbir production symbol'e dokunmasa bile)
3. Bir Change'in required evidence'ında kullanılan test'in zayıflaması
```

Aktif CRITICAL invariant'ın verification test'i remove/skip/disable edildiğinde blocking finding üretilir; ilgili production symbol'ün değişmiş olması şart değildir.

Guard yalnız `after_change` içinde değil, şu yolların hepsinde değerlendirilir:

```text
after_change
reconcile
verify --staged
verify --ci
```

Böylece local gate'ten geçen bir test zayıflatması CI tarafında yeniden yakalanır.

## Minimum heuristics

```text
ASSERTION_REMOVED
ASSERTION_COUNT_DECREASED
TEST_SKIPPED
TEST_XFAILED
TEST_DISABLED
EXPECTATION_REMOVED
ASSERT_TO_NOOP
CRITICAL_TEST_REMOVED
```

Framework adapter örnekleri:

```text
pytest.mark.skip
pytest.mark.xfail
unittest.skip
it.skip
test.skip
describe.skip
```

Tree-sitter ile test function/body delta'sı karşılaştırılarak assertion/expectation sayısındaki belirgin zayıflama bulunabilir.

Bu tam assertion-quality veya mutation testing değildir.

Finding:

```text
TEST_GUARD_WEAKENED
```

şu bilgileri taşır:

```text
test symbol
related production symbol
before guard summary
after guard summary
reason
confidence
```

Policy:

- LOW/MEDIUM: warning veya broader validation,
- HIGH: stronger evidence,
- CRITICAL invariant verification test'i removed/disabled ise hard block.

Amaç agent'ın failing testi kaldırıp/skip edip yeşil sonuç üretmesini ucuz bir guard ile yakalamaktır.

---

# 104. Public API

Public signature change:

```text
minimum HIGH
```

Public removal:

```text
minimum HIGH
```

Known external/package boundary consumer varsa policy CRITICAL yapabilir.

---

# 105. Database/schema changes

Migration ve schema dosyaları symbol graph ile iyi modellenemeyebilir.

Path policy kullanılır:

```text
migrations/**
schema/**
openapi/**
```

Minimum HIGH/CRITICAL olabilir.

Validation profile:

- migration check,
- schema validation,
- integration tests.

---

# 106. Security path

Config:

```toml
[[risk.path]]
pattern = "src/auth/**"
minimum = "HIGH"

[[risk.path]]
pattern = "src/authorization/**"
minimum = "CRITICAL"
```

---

# 107. Index/readiness health modeli

Index tek boolean health taşımaz.

Component health:

```text
knowledge
inventory
syntax
semantic
coverage_map
runtime_db
```

Genel readiness:

```text
PARTIAL_READY
READY
DEGRADED
BLOCKED
```

Örnek:

```text
Knowledge: HEALTHY
Syntax: INDEXING 42%
Semantic:
  api-python: HEALTHY
  admin-ts: MISCONFIGURED
Coverage Map: PARTIAL
```

Agent'ın target ProjectUnit'i healthy ise cold repo'nun kalan kısmı indexlenirken çalışabilir.

`mindrail_status` exact progress ve blocking component'i döndürür.

---

# 108. Parse error

Tree-sitter partial tree üretirse:

```text
syntax_health = DEGRADED
```

Semantic resolver da hata veriyorsa:

```text
analysis coverage
```

daha da düşer.

Critical path ise broader evidence gerekir.

---

# 109. Resolver failure

Pyright/TS service crash:

```text
SEMANTIC_RESOLVER_UNAVAILABLE
```

Mindrail config'e göre:

```text
degrade
or
block
```

yapar.

Degrade FULL → STRUCTURAL olur.

UI bunu açıkça gösterir.

---

# 110. Index freshness

File content hash:

```text
current != indexed
```

ise impacted file stale'dir.

Completion öncesi stale index otomatik refresh edilir.

Stale index ile definitive completion yapılamaz.

---

# 111. Git branch/rebase divergence

Open change sırasında:

```text
HEAD/base changed
```

ise:

```text
BASELINE_DIVERGED
```

olur.

Evidence stale edilir.

Impact yeniden analiz edilir.

---

# 112. CI ve branch awareness

CI:

```text
merge-base(base, head)
→ head
```

diff'i üzerinden code impact hesaplayabilir.

Knowledge files de diff'e dahildir.

Invariant değişikliği de reviewable change'tir.

---

# 113. Security

Minimum:

- repo path containment,
- symlink escape deny,
- parameterized SQL,
- strict MCP schemas,
- command whitelist,
- environment isolation,
- output redaction,
- no arbitrary shell interpolation,
- bounded logs.

---

# 114. Validation runner process model

Command:

```text
shell=True
```

ile serbest string çalıştırılmamalıdır.

Config command mümkünse argv olarak normalize edilmelidir.

Örnek:

```toml
command = ["pytest", "-q", "tests/auth"]
```

Bu shell injection yüzeyini azaltır.

---

# 115. Timeouts

Her validation profile timeout taşımalıdır.

Örnek:

```toml
timeout_seconds = 300
```

Timeout:

```text
VALIDATION_TIMEOUT
```

evidence üretir.

Başarılı sayılmaz.

---

# 116. Resource limits

İleride:

- CPU,
- memory,
- output

limitleri eklenebilir.

1.0 en az:

```text
timeout
output size cap
```

uygulamalıdır.

---

# 117. Agent instructions

`AGENTS.md` managed section:

Managed section 13 tool'un tamamını kapsar; agent'a sunulan protokolden bir tool'un düşmesi, o tool'un pratikte hiç çağrılmaması demektir.

```text
MINDRAIL PROTOCOL

Session start:
mindrail_bootstrap

Orientation:
mindrail_status
mindrail_search

Before task work:
mindrail_claim
mindrail_context

Before source mutation:
mindrail_before_change

After mutation:
mindrail_after_change

If a change was made without before_change,
or the actual diff must be re-derived:
mindrail_reconcile

When a durable design choice is made:
mindrail_decide

When a protected contract is discovered:
mindrail_invariant

Validation:
mindrail_validate

Completion:
mindrail_complete

Handoff:
mindrail_checkpoint
```

---

# 118. Soft vs hard enforcement

Soft:

```text
AGENTS.md
MCP next_actions
warnings
```

Hard:

```text
leases
completion gate
pre-commit
CI verify
critical evidence policy
```

---

# 119. Human approval ve administrative override ayrımı

`HUMAN_APPROVAL` ile `OVERRIDE` ayrıdır.

Human approval:

```text
normal evidence workflow
```

Override:

```text
administrative recovery
```

içindir.

Örnek override use-case:

- yanlış/stale lease'i bırakmak,
- corrupted runtime session'ı kapatmak,
- abandoned change ownership'ını recover etmek.

Agent kendi başına generic:

```text
force=true
```

kullanamaz.

Override MCP surface'e default olarak expose edilmez.

Override command:

```bash
mindrail override <operation> --reason "<required>"
```

interactive/operator channel'da çalışır.

Her override:

```text
override_id
operation
actor
actor_assurance
reason
timestamp
before_state
after_state
affected_ids
```

ile append-only event olarak saklanır.

---

# 120. Approval/override authorization policy

Override authorization project policy'sine bağlıdır.

Minimum local model:

```text
operator allowlist
+
interactive terminal
+
mandatory reason
```

Daha güçlü model:

```text
SIGNED actor identity
```

kullanabilir.

Örnek `.mindrail/approvers.toml`:

```toml
[[approver]]
id = "lead-1"
roles = ["critical-approval", "admin-recovery"]
key = "..."
```

Role örnekleri:

```text
manual-verifier
critical-approver
admin-recovery
```

Policy:

```text
CRITICAL evidence approval
→ critical-approver

lease recovery
→ admin-recovery
```

Agent process'in yalnız shell erişimi olması approver yetkisi anlamına gelmez.

Local private signing credential Mindrail tarafından tutulmaz.

Eğer strong identity configure edilmemişse Mindrail bunu açıkça:

```text
actor_assurance = LOCAL_ONLY
```

olarak işaretler ve CRITICAL policy'nin bunu kabul edip etmeyeceği config ile belirlenir.

---

# 121. Failure modes

## SQLite unavailable

Coordination write işlemleri durur.

## Syntax index unavailable

File-level fallback.

## Semantic resolver unavailable

FULL → STRUCTURAL veya policy block.

## Coverage map unavailable

Static mapping + broader validation fallback.

## MCP unavailable

CLI kullanılabilir.

## Git hook bypass edildi

CI tekrar verify eder.

## CI unavailable

Repository policy'ye göre local completion yapılabilir fakat release/merge gate dış sistem sorumluluğudur.

---


# 122. Dynamic dispatch ve validation budget

Dynamic/reflection uncertainty doğrudan:

```text
module → package → repository
```

şeklinde sınırsız test büyümesine yol açmamalıdır.

Her validation profile opsiyonel maliyet metadata'sı taşır:

```text
estimated_duration_class
scope
confidence_gain
resource_class
```

Örnek duration class:

```text
FAST
NORMAL
EXPENSIVE
VERY_EXPENSIVE
```

Mindrail precise süre tahmini yapmak zorunda değildir.

## Validation budget policy

Project:

```toml
[validation_budget.interactive]
max_class = "EXPENSIVE"

[validation_budget.ci]
max_class = "VERY_EXPENSIVE"
```

tanımlayabilir.

## Escalation

Belirsizlikte planner:

```text
1. runtime/semantic signals ile daralt
2. smallest sufficient validation profile seç
3. budget içinde broader profile ara
4. budget yetmiyorsa unresolved uncertainty olarak raporla
```

CRITICAL uncertainty budget yüzünden sessizce ignore edilmez.

Sonuç:

```text
BLOCKED_BY_VALIDATION_BUDGET
```

olabilir ve seçenekler gösterilir:

```text
- run package suite
- defer independent CI suite
- provide policy-accepted external/manual evidence
- improve resolver/framework mapping
```

## User control

Mindrail agent'a tek seçenek dayatmak yerine deterministic alternatives döndürebilir.

Agent/human daha geniş profile seçebilir ancak required minimum proof'u düşüremez.

Bu sayede reflection kullanılan projeler Mindrail yüzünden her local edit'te tüm repository testlerini çalıştırmak zorunda kalmaz.

---

# 123. Full repository tests

Default olarak her change'de zorunlu değildir.

Çalıştırılırsa sebepler:

- project policy,
- impact fan-out,
- critical shared abstraction,
- low analysis coverage,
- release/CI gate.

Amaç gereksiz binlerce test üretmek veya çalıştırmak değildir.

---

# 124. Validation breadth escalation

```text
TARGETED
MODULE
PACKAGE
SERVICE
REPOSITORY
```

Mindrail belirsizlik arttıkça bir üst seviyeye çıkabilir.

Örnek:

```text
test mapping poor
→ MODULE

interface fan-out huge
→ PACKAGE/SERVICE
```

---

# 125. Impact set representation

Mindrail context'e binlerce symbol basmaz.

Özet:

```text
direct: 4
transitive: 28
fanout groups: 2

affected packages:
auth
radius

highest-risk:
RadiusReplyBuilder.build
GuestSession.authorize
```

Full result DB'de tutulur.

---

# 126. Explainability

Her finding:

```text
why
path
confidence
source
```

taşır.

Örnek:

```text
RadiusReplyBuilder.build

why:
calls SessionService.calculate_timeout

path:
calculate_timeout
← CALLS
RadiusReplyBuilder.build

origin:
semantic reference resolution

confidence:
0.98
```

---

# 127. Risk explainability

Mindrail:

```text
Risk HIGH
```

demekle kalmaz.

Örnek:

```text
Reasons:
+ public signature change
+ 4 known direct consumers
+ HIGH invariant
- semantic resolver healthy
- direct coverage mapping available
```

---

# 128. Hedef implementation workstream'leri

## Milestone 1 — Durable Knowledge + Runtime Core

- `.mindrail/knowledge`
- JSON schemas + schema versions
- knowledge loader/validator
- Decision
- Invariant
- SQLite runtime
- WAL/busy retry
- operation idempotency
- optimistic revisions
- Session
- Task
- Lease
- Checkpoint

Başarı:

Yeni clone knowledge'ı doğrulayabilir; paralel local writers duplicate state üretmez.

## Milestone 2 — Project Units + Syntax Index

- monorepo ProjectUnit discovery
- Tree-sitter registry
- Python
- TS/JS
- content-addressed snapshots
- resumable cold index
- progress model
- target-first scheduler

Başarı:

100k+ file inventory üzerinde init FULL index'i beklemeden PARTIAL_READY olabilir.

## Milestone 3 — Resolver Manager

- project-local resolver discovery
- managed resolver fallback
- health state machine
- Python/Pyright adapter
- TS Language Service adapter
- bounded restart
- `mindrail doctor --resolver`

Başarı:

Her ProjectUnit için FULL/STRUCTURAL mode nedenleri açıklanabilir.

## Milestone 4 — Change Engine

- before_change baseline
- after_change delta
- scope drift
- reconcile
- baseline divergence

Başarı:

Gerçek changed symbols task/change'e bağlanır.

## Milestone 5 — Impact + Fan-out

- semantic graph traversal
- aggregation
- impact groups
- drill-down pagination
- policy rules
- confidence/coverage
- fan-out boundaries
- minimal impact/latency telemetry starts here

Başarı:

Shared interface değişikliği aggregate edilir ancak exact impacted symbols sorgulanabilir kalır.

## Milestone 6 — Coverage/Test Impact

- generic Coverage Provider contract
- Python provider
- TS/JS provider/importer
- TEST_COVERS edges
- fallback validation breadth
- selection provenance

Başarı:

Mindrail bir testin neden seçildiğini açıklar; coverage yoksa tahmin yerine broader suite seçer.

## Milestone 7 — Evidence + Human Trust

- safe validation runner
- stale detection
- manual verification schema
- human approval actor assurance
- signed approval support
- approver roles
- secret redaction

Başarı:

CRITICAL human approval agent'ın basit beyanıyla üretilemez.

## Milestone 8 — MCP + CLI UX

- 13 MCP tools
- status/search/context drill-down
- doctor
- structured errors
- next actions

Başarı:

Claude ve Codex aynı contract'ı kullanabilir.

## Milestone 9 — Git + CI Enforcement

- pre-commit
- unregistered change
- `mindrail knowledge validate`
- `mindrail verify --ci`
- base/head impact
- schema incompatibility fail-closed

Başarı:

Bozuk knowledge veya local hook bypass CI'da yakalanır.

## Milestone 10 — Candidate + Calibration

- deterministic candidate triggers
- promote/dismiss/stale lifecycle
- impact telemetry
- false-positive/missed-impact feedback
- calibration report

Başarı:

Invariant yakalama yükü azaltılır ancak Mindrail otomatik engineering truth üretmez.

---

# 129. Acceptance criteria

## AC-01 Resolver diagnostics

Monorepo'da her ProjectUnit için kullanılan config/resolver path/version ve health açıklanabilir.

## AC-02 Resolver failure

Resolver crash/misconfiguration FULL mode'u sessizce devam ettirmez.

## AC-03 Managed fallback

Project-local resolver yoksa policy izin veriyorsa managed resolver project dependency dosyalarını değiştirmeden kullanılabilir.

## AC-04 Coverage provider abstraction

Core Python/TS-specific coverage formatı bilmeden normalized `TEST_COVERS` relation tüketir.

## AC-05 Future language capability

Yeni FULL language adapter semantic resolver ve test-impact strategy capability matrix'i beyan etmek zorundadır.

## AC-06 Signed human approval

CRITICAL policy signed approval istediğinde unsigned/local agent beyanı kabul edilmez.

## AC-07 Approval provenance

Approval reason, actor, exact snapshot ve covered invariants ile loglanır.

## AC-08 Fan-out drill-down

Aggregate impact result exact symbols'e paginated drill-down sağlar.

## AC-09 SQLite retry safety

Concurrent write retry aynı `operation_id` ile duplicate Change/Evidence üretmez.

## AC-10 Revision conflict

Stale mutable entity update sessiz last-write-wins yapmaz.

## AC-11 Candidate assertion

Yeni assertion candidate invariant üretebilir ancak otomatik active invariant oluşturmaz.

## AC-12 Candidate dedup

Aynı source/predicate candidate spam oluşturmaz.

## AC-13 Cold init

100k+ file repository'de `mindrail init` inventory/knowledge sonrası PARTIAL_READY dönebilir; full cold index'i beklemek zorunda değildir.

## AC-14 Index resume

Index process kesilirse tekrar başlatıldığında completed content hashes yeniden parse edilmez.

## AC-15 Target-first

Unindexed project unit içinde before_change geldiğinde o unit cold queue önüne alınır.

## AC-16 Knowledge JSON corruption

Bozuk knowledge JSON ile `mindrail verify --ci` source verification'a geçmeden fail eder.

## AC-17 Schema forward incompatibility

Mindrail'in bilmediği daha yeni knowledge schema fail-closed olur.

## AC-18 Supersede cycle

Decision/Invariant supersede cycle CI'da reddedilir.

## AC-19 Scope resolution

Blocking symbol-scoped invariant final verification öncesi resolvable olmalıdır.

## AC-20 Agent independence

Farklı MCP clients aynı Mindrail contract'ını kullanabilir.

## AC-21 Symbol lease

Aynı logical symbol conflicting change alamaz.

## AC-22 Evidence stale

Validation sonrası source edit proof'u invalidate eder.

## AC-23 Reconcile ambiguity

Birden fazla Change'e ait olabilecek unregistered symbol otomatik sahiplenilmez.

## AC-24 Secret redaction

Configured known secret plaintext evidence storage'a girmez.

---


## AC-25 Interactive latency SLO

Warm-path benchmark suite `status/context/before_change/after_change/reconcile` p95 hedeflerini STRUCTURAL ve FULL mode için ayrı ölçer ve regression raporlar.

## AC-26 SLO deferral

Semantic closure bütçe içinde tamamlanamadığında operasyon bloke olmaz; pending state döner ve eksik iş yüksek öncelikle schedule edilir. Bütçeyi aşan blocking analiz kabul edilmez.

## AC-27 Reconcile-first correctness

Agent `mindrail_before_change` çağırmadan source edit yaptığında completion/verify actual diff'i discover eder ve impact/evidence gate'i bypass edilmez.

## AC-28 Symbol rename preservation

Protected symbol rename olduğunda invariant/evidence relation ya aynı `symbol_uid`'ye migrate edilir ya da ambiguity explicit block üretir; sessiz orphan oluşmaz.

## AC-29 symbol_uid allocation

Aynı symbol'ü eşzamanlı ilk kez gören iki process tek bir `symbol_uid` üzerinde uzlaşır. Concurrent allocation testi duplicate uid üretmediğini kanıtlar.

## AC-30 Resolver resource budget

`max_processes` ve query timeout uygulanır; budget aşıldığında uncontrolled process growth oluşmaz ve kapasite yokluğu explicit `RESOLVER_RESOURCE_LIMIT` + policy davranışı üretir.

## AC-31 Resolver eviction thrash

Aktif target unit'in resolver'ı ve `min_residency_seconds` dolmamış instance evict edilmez; thrash metriği ölçülür ve raporlanır.

## AC-32 Managed resolver integrity

Checksum doğrulanmayan managed resolver execute edilmez.

## AC-33 Cold-index fairness

Cold index persistence configured chunk sınırını aşan uzun write transaction oluşturmaz ve interactive write starvation metriği ölçülür.

## AC-34 Test weakening

Aktif CRITICAL invariant'ın verification test'inin skip/remove edilmesi, aynı Change production kodu değiştirmese bile `TEST_GUARD_WEAKENED` blocking finding üretir; guard `verify --staged` ve `verify --ci` yollarında da çalışır.

## AC-35 Same-workspace sequential agents

İki farklı AgentSession aynı workspace'i sıralı kullanabilir; worktree zorunluluğu yoktur.

---

# 130. 1.0'de bilinçli olarak çözülmeyenler

Hâlâ garanti edilmez:

- reflection target'larının tamamı,
- runtime plugin discovery'nin tamamı,
- metaprogramming,
- generated runtime code,
- external repository consumer'ları,
- %100 doğru test selection,
- %100 false-negative-free impact analysis,
- tam program verification.

Mindrail bunu gizlememelidir.

---

# 131. Güvenlik modeli özeti

Mindrail güveni üç sinyalin birleşiminden üretir:

```text
CODE UNDERSTANDING
syntax + semantic

BEHAVIORAL CONTRACT
decision + invariant

PROOF
tests + build + runtime + human/CI evidence
```

Tek bir katman mutlak doğru kabul edilmez.

---

# 132. Örnek: Agent A / Agent B

Başlangıç:

```text
A:
OtpService.consume() yazdı.

INV-17:
OTP başarılı consume sonrası tekrar consume edilemez.
```

Semantic graph:

```text
OtpService.consume
← OtpController.verify
← RadiusAuthFlow.authenticate
```

Runtime map:

```text
test_otp_replay
→ OtpService.consume

test_radius_auth
→ RadiusAuthFlow.authenticate
```

B daha sonra `consume()` değiştirir.

---

# 133. B agent flow

```text
mindrail_bootstrap
mindrail_claim TASK-B
mindrail_before_change OtpService.consume
```

Mindrail:

```text
Known invariant:
INV-17 CRITICAL

Direct semantic consumers:
OtpController.verify

Transitive:
RadiusAuthFlow.authenticate

Mapped tests:
test_otp_replay
test_radius_auth

Analysis:
FULL
semantic resolver healthy
```

B edit yapar.

```text
mindrail_after_change
```

Mindrail:

```text
BODY_CHANGED

Risk policy:
CRITICAL invariant affected

Required evidence:
test_otp_replay
test_radius_auth
auth integration profile
```

---

# 134. Resolver düşük confidence olursa

Örneğin reflection nedeniyle Radius path kesin çözülemiyor.

Mindrail:

```text
analysis coverage degraded
dynamic dispatch unresolved
```

der.

Sonra validation breadth:

```text
targeted
→ auth module
```

olarak yükseltilir.

Yani:

```text
"emin değilim, o halde hiçbir şey bilmiyorum"
```

yerine:

```text
"emin değilim, o halde daha geniş proof istiyorum"
```

politikası uygulanır.

---

# 135. Mindrail Technical Specification 1.0 ürün tanımı

> **Mindrail Technical Specification 1.0; farklı AI coding agent'larının aynı Git projesinde portable engineering knowledge paylaşmasını, gerçek source değişikliklerini reconcile ile agent protokolünden bağımsız keşfetmesini, durable symbol identity ile invariant'ları rename/refactor sırasında korumasını, Tree-sitter ve bounded semantic resolver'larla explainable impact üretmesini ve yalnız current evidence bulunduğunda completion'a izin vermesini tanımlayan local-first engineering gate mimarisidir.**

---

# 136. 1.0'ın temel kontratı

```text
No task without context.

No concurrent ownership without lease.

No durable engineering decision without versioned record.

No protected behavior without invariant.

No code change without reconcile-backed impact analysis.

No protected symbol rename without identity migration or explicit ambiguity.

No silent FULL mode without healthy semantic resolver.

No unbounded resolver growth without resource policy.

No low-confidence critical change without stronger evidence.

No aggregate impact without drill-down availability.

No critical human approval without policy-defined actor assurance.

No completion without current evidence.

No false-green critical test weakening without explicit policy handling.

No CI verification on invalid knowledge.

No local-only safety assumption where CI policy requires independent proof.
```

---


# 137. Implementation baseline

Mindrail core implementation language for the architecture defined by this specification is **Go**.

This is an implementation constraint, not a change to protocol semantics.

Go implements:

```text
CLI
MCP server
workflow/state machines
SQLite persistence
knowledge loader/validator
Git integration
scheduler
Tree-sitter integration
impact engine
validation/evidence gate
resolver supervision
```

Language semantic analyzers remain external supervised processes where appropriate.

Examples:

```text
Python → Pyright
TypeScript/JavaScript → TypeScript semantic service
future C# → Roslyn adapter
future Go → gopls adapter
future Java → JDT adapter
```

Exact package/library/build decisions belong to `mindrail-tech-stack.md`.

---

# 138. Nihai mimari özeti

```text
                            AI AGENTS
          Claude Code / Codex / Cursor / Other
                               │
                            MCP / CLI
                               │
                    ┌──────────▼──────────┐
                    │     Mindrail Core     │
                    └──────────┬──────────┘
                               │
         ┌─────────────────────┼─────────────────────┐
         │                     │                     │
  Engineering Knowledge   Runtime State         Code Reality
         │                     │                     │
 .mindrail/knowledge      SQLite/WAL       Tree-sitter Syntax
 Decisions                 Sessions               +
 Invariants                Leases          Semantic Resolver
 Policies                   Changes                +
         │                  Evidence        Coverage Signals
         └─────────────────────┼─────────────────────┘
                               │
                         Impact Engine
                               │
                     Validation Planner
                               │
                         Evidence Gate
                               │
                    Local Git + CI Verify
```

# Installed update and JEV distribution — independent Reader

Date: 2026-09-27
Base: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`
Mode: parallel audit içinde bağımsız Reader conformance pass. Bu rapor yazılmadan
önce Breaker sonucu okunmadı ve beklenmedi.

## Reader sonucu

**COMPLIANT_WITH_MINOR_ISSUES**

Yeni binary'nin kurulması, initialize edilmiş worktree'nin `mindrail update` ile
yenilenmesi ve opsiyonel JEV adapter'ının kurulu binary içinden çalıştırılması
pozitif testlerde gerçekleşti. Bir aktif agent skill'i hâlâ silinmiş eski adapter
yolunu gösteriyor; ayrıca zorunlu Graphify çıktısı yeni mimariye göre
yenilenmemiş. Bu iki uyumsuzluk ana update hattını bozmuyor, fakat JEV kullanan
Claude skill akışını ve repository bilgi grafiğini güncel sözleşmeyle çeliştiriyor.

## R1 — görev normalizasyonu

| ID | Gereksinim | Kaynak | Tür | Doğrulama |
|---|---|---|---|---|
| REQ-001 | Initialize edilmiş worktree için görünür `mindrail update`; uninitialized repository'de yazmadan `init`e yönlendirme; tekrar çalıştırmada byte-stability. | “bu update olayini yapman lazim. hali hazirda proje de zaten `mindrail init` yapildi.” | FUNCTIONAL / COMPATIBILITY | CLI testleri ve compiled-binary scratch repository |
| REQ-002 | Config, knowledge, workspace/project kimliği, coordination verisi, user AGENTS metni ve foreign hook korunmalı. | Mevcut init/upgrade sözleşmesi; frozen requirement | NON_REGRESSION | Update/upgrade ve setup testleri |
| REQ-003 | Source-built binary için açık, configurable ve atomik `make install`; repository update ile binary replacement ayrılmalı. | Update talebi ve mevcut yayın kanalının olmaması | DEPLOYMENT / DOCUMENTATION | Disposable `DESTDIR` kurulumu ve docs↔Makefile karşılaştırması |
| REQ-004 | Canonical JEV adapter mutable project kodu yerine kurulu binary içinde olmalı; hidden `mindrail agent route` köprüsü kullanılmalı. | Frozen threat analysis | SECURITY / DEPLOYMENT | Embedded asset eşitliği, bridge testleri ve compiled binary |
| REQ-005 | Key yok/blank ise normal akış; Python/deadline/provider/response hataları fail-open typed sonuç. | “jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin” | FUNCTIONAL / COMPATIBILITY | 45 Python testi ve Go bridge testleri |
| REQ-006 | Key yalnız agent process environment'ından alınmalı; repo/config/stdin/argv/log/commit içinde olmamalı. | “ayrica jev key'yi nereye girecegim?” | SECURITY | Veri akışı okuması, secret/non-disclosure testleri, docs |
| REQ-007 | TypeSafe'e yalnız minimal, non-sensitive, closed-candidate veri gönderilmeli. | External routing'in güvenlik sonucu | SECURITY / DOCUMENTATION | Managed guidance ve adapter input-contract testleri |
| REQ-008 | Fresh init ve existing update aynı managed JEV guidance'ını üretmeli; user metni korunmalı; eski dış JEV bloğu kaldırılmalı. | Frozen compatibility contract | COMPATIBILITY / DOCUMENTATION | Setup testi, generated AGENTS byte karşılaştırması, aktif skill taraması |
| REQ-009 | Update, bridge, secret ve upgrade branch'leri test edilmeli; full gates geçmeli. | Completion contract | TESTING | Targeted suites, `make tidy-check`, `make verify`, `make gate` |
| TASK_AMBIGUITY-01 | “Update” binary self-update veya repository refresh anlamına gelebilir. | Original request | AMBIGUITY | Frozen contract iki adımı açıkça ayırıyor: `make install` sonra `mindrail update`; network self-update yok. Her iki okuma da dürüstçe karşılanıyor. |

## R2 — gereksinim kararları

| Gereksinim | Karar | Üretilmiş kanıt | Not |
|---|---|---|---|
| REQ-001 | PASS | `TestUpdateRefusesUninitializedRepositoryWithoutWrites`, `TestUpdateUsesInitPipelinePreservesStateAndIsIdempotent`, `TestUpdateAppliesPendingMigrationsWithoutLosingCoordination` geçti. Disposable compiled binary'de iki `update` sonrası AGENTS ve hook SHA-256 değerleri aynı kaldı; JSON envelope ve data command alanları `update` oldu. | `runRepositorySetup` tek ModeInit hattını paylaşırken `requireInitializedRepository` read-only preflight yapıyor. |
| REQ-002 | PASS | Update testi config/knowledge/AGENTS/hook/identity ve schema-3 coordination task'ını korudu; full eski upgrade suite'i `make verify` içinde geçti. | Yeni ikinci migration implementation yok. |
| REQ-003 | PASS | `GOCACHE=/tmp/mindrail-update-reader-go make install DESTDIR=<tmp> BINDIR=/usr/bin` iki kez başarılı; mode `0755`, kurulu binary çalıştı, kalan `.mindrail.install.*` sayısı `0`. Makefile aynı dizinde temp build + `mv` yapıyor. README ve Türkçe kılavuz binary replacement ile repository update'i ayırıyor. | Network self-update iddiası yok. |
| REQ-004 | PASS | Eski ve yeni adapter byte karşılaştırması `cmp=0`; `TestJevRouteSourceIsCanonicalEmbeddedAsset` full suite içinde geçti; hidden agent/route testi ve compiled binary `agent route` başarılı. | Project'e executable adapter kopyalanmıyor. |
| REQ-005 | PASS | Python adapter suite `45/45`; Go bridge focused suite PASS. Compiled binary'de key unset + empty stdin sonucu exit `0`, `disabled/api_key_missing`. Missing/blank key testinde stdin reader ve Python launcher çağrılmadı. | Provider ağına gerçek çağrı gerekmeyen deterministic conformance kanıtı kullanıldı. |
| REQ-006 | PASS | Adapter secret/header testleri ve bridge sanitized-output testleri geçti. `runAgentRoute` key'i yalnız `os.Getenv` ile alıyor; Python argv yalnız embedded source içeriyor; managed ve user docs secret manager/keyring injection diyor. | Repo config veya `.env` loader eklenmedi. |
| REQ-007 | PASS | `internal/setup/setup.go:55-67` ile generated AGENTS raw secret/log/source'u yasaklıyor; adapter closed candidates ve input/output bounds testleri geçti. | Guidance hem fresh hem update sonucuna gömülü. |
| REQ-008 | PARTIAL | Source `AGENTS.md` managed bölümü ile scratch fresh-init managed bölümü byte-identical (`cmp=0`); old block upgrade/idempotence testi geçti; obsolete outer block kaldırıldı. Ancak `.claude/skills/engineering-orchestrator/SKILL.md:85` silinmiş eski adapter'ı çağırıyor (RDR-001). | Managed contract doğru, aktif Claude skill contract'ı stale. |
| REQ-009 | PASS | Targeted Python/Go/setup/update testleri PASS; `make tidy-check` PASS; `make verify` PASS (vet, full suite, full race, smoke); `make gate` dokuz kategorinin tamamında PASS. | Graphify'nin standing refresh kuralı RDR-002'de ayrıca kaydedildi. |

## R3/R4 — findings

| ID | Severity | Requirement | Dosya/satır | Problem | Kanıt | Confidence |
|---|---|---|---|---|---|---|
| RDR-001 | MEDIUM | REQ-008; prior-spec supersession | `.claude/skills/engineering-orchestrator/SKILL.md:72-105`, özellikle `:85` | Aktif engineering-orchestrator skill'i silinmiş repository-local adapter'ı çağırıyor; yeni `mindrail agent route` sözleşmesiyle çelişiyor. | Eski dosya yok; skill'deki tam komut dummy key ile exit `2`, stdout `0` byte ve Python `ENOENT` üretti. Aynı binary'nin yeni komutu key yokken exit `0` typed disabled JSON üretti. | CONFIRMED |
| RDR-002 | LOW | REQ-004/REQ-009 repository integration contract | `graphify-out/graph.json` | Standing `AGENTS.md` kuralı ve TASK-005'e rağmen Graphify yeni bridge/adapter konumuna göre yenilenmemiş; graph silinmiş `.claude/.../jev_route.py` düğümlerini aktif gösteriyor. | `graphify query "mindrail agent route"` sonucu `route()` ve `jev_route.py` kaynaklarını silinmiş `.claude/...` yolunda verdi; changeset'te hiçbir `graphify-out/**` güncellemesi yok. | CONFIRMED |

### RDR-001 ayrıntısı

Beklenen aktif invocation:

```text
mindrail agent route
```

Skill'in halen verdiği invocation:

```text
python .claude/skills/engineering-orchestrator/scripts/jev_route.py < request.json
```

Üretilmiş karşılaştırma:

| Ölçüm | Yeni installed bridge (key unset) | Skill'deki stale komut (dummy key) |
|---|---:|---:|
| Exit code | 0 | 2 |
| stdout JSON nesnesi | 1 | 0 |
| Typed fail-open status | `disabled` | yok |
| Referenced executable asset exists | embedded asset: 1 | old path: 0 |

Beklenen davranış, superseding spec ve managed guidance'ın tek installed bridge'i
göstermesidir. Gerçek davranışta Claude engineering-orchestrator skill'i opt-in key
varsa doğrudan ENOENT alır; adapter'ın typed fallback sözleşmesine ulaşamaz.

Önerilen düzeltme: skill'in Optional JEV bölümündeki komutu ve dağıtım dilini
`mindrail agent route` olarak güncellemek; eski repository-local path'i kaldırmak.
Bu Reader production dosyayı değiştirmedi ve düzeltme ölçümü yapmadı.

Class sweep:

```bash
rg -n "jev_route|agent route|TYPESAFE_API_KEY" .claude .agents .codex AGENTS.md README.md docs internal
```

Aktif yanlış invocation yalnız `.claude/skills/engineering-orchestrator/SKILL.md`
içinde bulundu. Eski audit raporlarındaki yollar tarihsel kanıt olduğu için finding
değildir. Superseded engineering doc aktif yolları doğru gösteriyor.

### RDR-002 ayrıntısı

`AGENTS.md`, kod değişikliğinden sonra `graphify update .` çalıştırılmasını emrediyor.
Current graph query ise yeni `internal/agent/jev_route.py` ve CLI bridge yerine
silinmiş `.claude/skills/engineering-orchestrator/scripts/jev_route.py` düğümünü ve
edge'lerini döndürdü. Önerilen düzeltme production değişikliklerden sonra
`graphify update .` çalıştırıp generated delta'yı doğrulamaktır. Bu Reader Graphify
çıktısını değiştirmedi ve düzeltme ölçümü yapmadı.

## Scope compliance

- **UNDER_IMPLEMENTATION:** RDR-001 ile sınırlı documentation/instruction
  integration eksikliği var.
- **OVER_IMPLEMENTATION / SCOPE_CREEP:** Üretilmedi. Hidden `agent` command,
  shared update pipeline, setup metni, install target ve docs frozen WBS ile aynı
  sınırda.
- **BEHAVIORAL_DRIFT:** Üretilmedi. `init` mevcut pipeline'ı koruyor; `update`
  preflight dışında aynı yolu kullanıyor; MCP tool sayısı değişmiyor.
- **BACKWARD COMPATIBILITY:** Existing schema-3 database, coordination row,
  config, knowledge, workspace/project identity, foreign hook ve user AGENTS metni
  update sonrasında korundu. Uninitialized path yazmadan reddedildi.
- Original talep ana davranış bakımından birebir karşılandı; iki repository
  integration artifact'ı güncel contract'a taşınmadığı için sonuç tam compliant
  değildir.

## Contract ve documentation conformance

- README, `docs/usage-tr.md`, frozen installed-update spec, Makefile, managed
  AGENTS ve CLI help update/install/key sözleşmesinde aynı şeyi söylüyor.
- Historical optional-JEV spec açıkça `SUPERSEDED` ve installed bridge'e link
  veriyor; eski audit kayıtları tarihsel olduğundan değiştirilmemesi doğru.
- RDR-001 bir `DOC_DRIFT`: aktif skill silinmiş executable path'i tarif ediyor.
- RDR-002 generated architecture index drift'idir; runtime contract'ı değiştirmez.

## Pozitif doğrulama kanıtı

| Komut / deney | Sonuç |
|---|---|
| `python -B -m unittest discover -s internal/agent -p 'test_jev_route.py' -v` | PASS, 45 test |
| `GOCACHE=/tmp/mindrail-update-reader-go go test ./internal/agent ./internal/cli -run 'TestAgent' -count=1` | PASS |
| `GOCACHE=/tmp/mindrail-update-reader-go go test ./internal/cli ./internal/status -run 'Test(Update|CommandSurface|RootHelpShows|Init)' -count=1` | PASS |
| `GOCACHE=/tmp/mindrail-update-reader-go go test ./internal/setup -count=1` | PASS |
| Disposable `make install` twice + mode/version/temp residue probes | PASS; 2/2 installs, mode 755, 0 residue |
| Compiled binary fresh init → update → update → no-key agent route | PASS; managed bytes stable, typed disabled output |
| `GOCACHE=/tmp/mindrail-update-reader-go make tidy-check` | PASS |
| `GOCACHE=/tmp/mindrail-update-reader-go make verify` | PASS; vet, full test, full race, smoke |
| `GOCACHE=/tmp/mindrail-update-reader-go make gate` | PASS; unit 1435, domain 421, integration 12, race 9, knowledge-schema 216, MCP-contract 58, Git worktree 77 + named 8, SQLite concurrency 17, E2E 12, smoke 7 |
| `git diff --check 64e8f8a...` | PASS |

## Open items

Yok. Live TypeSafe credential/provider çağrısı güvenlik ve determinism nedeniyle bu
Reader pass'inin gerekli kanıtı değildir; HTTP contract ve failure branch'leri
offline fake transport ile 45 testte yürütüldü.

## Independent conclusion

`mindrail update`, source install ve embedded JEV bridge ana işlevleri current tree'de
pozitif kanıtla çalışıyor. RDR-001 düzeltilmeden repository'nin aktif agent
talimatları tek ve tutarlı invocation sunmuyor; RDR-002 düzeltilmeden knowledge graph
yeni mimariyi temsil etmiyor. Bu nedenle Reader kararı
**COMPLIANT_WITH_MINOR_ISSUES**; production implementation için ret gerektiren veri
kaybı, secret leak, update refusal veya gate regression üretilmedi.

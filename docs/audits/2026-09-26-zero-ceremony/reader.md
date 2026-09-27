# Zero-ceremony UX — bağımsız Reader denetimi

Tarih: 2026-09-26
Rol: Reader / gereksinim uygunluğu
Mod: Paralel bağımsız alt-agent; bu rapor yazılmadan önce Breaker raporu veya Breaker sonucu okunmadı.
Baseline: dirty worktree, `HEAD 2814acb`; untracked task dosyaları dahil mevcut çalışma ağacı.
Karar: **APPROVE — implementasyon Reader açısından `FULLY_COMPLIANT`**. REQ-011'in nihai “dual-agent audit pass” cümlesi koordinatörün Breaker ve reconciliation çıktısına bağlıdır; bu bekleme ürün uygunsuzluğu değildir.

## 1. Özgün görev ve denetim sınırı

Özgün kullanıcı talebi:

> “ben hayatimda bu kadar karisik kullanim gormedim, surekli cli da birseyler yapmak lazim. ne alaka yani. kullanacak kisi neden bui kadar seyle ugrassin”
>
> “yapin o zaman”

Bu kısa talep tek başına teknik şekli belirlemiyor (`TASK_AMBIGUITY-A`). Audit package, `docs/engineering/zero-ceremony-ux-2026-09-26.md` içindeki dondurulmuş `REQ-001..REQ-011` matrisini açıkça yetkili kıldığı için değerlendirme bu kabul kriterlerine göre yapıldı. Kısa talebin dar okuması “insandan lifecycle seremonisini kaldır”, geniş okuması “CLI/MCP/setup/docs bütününde sıfır-seremoni akışını teslim et”tir; dondurulmuş program ikinci okumayı somutlaştırır ve implementasyon iki okumayı da karşılar.

`TASK_AMBIGUITY-B`: REQ-011 hem ürün testlerini hem de bu Reader ile Breaker'ın nihai denetim geçişini ister. Reader ürün/test bölümünü doğrulayabilir; Breaker/reconciliation kapanışını tek başına ilan edemez. Bu nedenle aşağıdaki REQ-011 satırında ürün hükmü `PASS`, nihai audit kapanışı “koordinatör bekliyor” olarak ayrılmıştır.

## 2. Normalize edilmiş gereksinim matrisi

| ID | Gereksinim | Kaynak | Tür | Reader doğrulaması |
| --- | --- | --- | --- | --- |
| REQ-001 | Normal insan yüzeyi `init`, `status`, `doctor`, `verify`, `version`; expert komutlar callable fakat root help'te gizli. | Dondurulmuş tablo: “default human workflow exposes only setup and health/verification commands” | FUNCTIONAL / UX | Çalışan binary help'i + CLI testleri |
| REQ-002 | Tek, idempotent ve güvenli `init`; config/knowledge, managed AGENTS bölümü, foreign hook zinciri; kullanıcı içeriği korunur, unsafe hedef görünür reddedilir. | “performs all safe repository-local setup idempotently” | FUNCTIONAL / CONSTRAINT | Canlı iki init + hash karşılaştırması + setup/CLI testleri |
| REQ-003 | Yeni tool adı olmadan `mindrail_bootstrap {goal,run_key,paths?,resume_task_id?}` automatic start; fresh logical run, explicit resume, no steal; legacy `{}` read-only. | “high-level automatic start operation without a new tool name” | FUNCTIONAL / PUBLIC CONTRACT | Şema, kod yolu, persistent MCP testleri |
| REQ-004 | `finalize:true` gerçek diff reconcile, sıralı configured profiles, shared gate, guarded completion/release; denial actionable; mechanical ID/override yok; legacy complete evaluation-only. | “high-level automatic finish operation without changing legacy completion semantics” | FUNCTIONAL / PUBLIC CONTRACT | Finalize kod yolu + denial/replay/legacy testleri |
| REQ-005 | MCP bağlantısı başına izolasyon; disjoint işler ilerler, overlap conflict; heartbeat revision churn yapmaz; disconnect durdurur; renewal failure completion'ı engeller. | “isolated per MCP connection and safe under concurrency” | SECURITY / CORRECTNESS | Race testleri + connection/heartbeat testleri |
| REQ-006 | Stable opaque `run_key` ile duplicate/restart idempotence; duplicate session/task/double completion yok; partial failure recoverable; denial/stale CAS completion raporlamaz. | “idempotent and recoverable across process restart” | CORRECTNESS | Journal/phase kodu + restart/conflict/replay testleri |
| REQ-007 | Eski dirty work yeni task'a absorbe edilmez; sequential same-file task'ları ayrışır; history kalır, terminal task aktif ownership adayı olmaz. | “scope attribution remains correct across old dirty work and sequential tasks” | DATA INTEGRITY / NON_REGRESSION | Automatic scope/restoration kodu + dirty/sequential testleri |
| REQ-008 | Tam 13 MCP adı; legacy payload/explicit-ID semantics; expert CLI callable; stdout frame-only. | “existing low-level clients and scripts continue to work” | COMPATIBILITY | Gerçek stdio listesi/E2E + legacy ve stdout testleri |
| REQ-009 | Bare `verify` staged/local; `--staged`, `--ci` sürer; conflict reddedilir; insan çıktısı modu söyler. | “local verification needs no mode flag” | FUNCTIONAL / UX | Canlı binary + eşdeğerlik testi |
| REQ-010 | README ve Türkçe kılavuz basit dört komut + automatic lifecycle ile başlar; happy path manual ID/jq/lease istemez; mechanics advanced bölümde kalır. | “documentation leads with the simple path” | DOCUMENTATION | Doküman↔kod karşılaştırması |
| REQ-011 | Lifecycle/retry/dirty/sequential/concurrency/lease/gate/help/init/verify/stdio kapsamı; full gates; bağımsız dual audit. | “delivery is behaviorally tested and independently audited” | TESTING / ACCEPTANCE | Test envanteri + yeniden çalıştırılan kapılar; final dual closure koordinatörde |

## 3. Gereksinim hükümleri

| Gereksinim | Hüküm | Pozitif kanıt | Not |
| --- | --- | --- | --- |
| REQ-001 | PASS | `internal/cli/root.go:119-140`; canlı `mindrail --help` yalnız beş normal komut ile meta `help`i gösterdi; `TestRootHelpShowsHumanCommandsAndExpertCommandsRemainCallable` geçti. | Hidden expert komutların `--help` yolları callable kaldı. |
| REQ-002 | PASS | `internal/setup/setup.go:69-180`; `TestManagedSectionReplacementPreservesSurroundingBytes`, `TestLegacyHookMigrationPreservesForeignBytes`, unsafe/symlink/occupied-backup testleri ve CLI init testleri geçti. Canlı disposable repoda iki init sonrası `AGENTS.md`, config ve hook SHA-256 değerlerinin 3/3'ü değişmedi. | Schema 10'a init ile yükseltme de canlı çıktıda gözlendi. |
| REQ-003 | PASS | `internal/mcp/server.go:219-230`, `internal/mcp/automatic.go:51-134`, `internal/workflow/service.go:151-259`; `TestLegacyBootstrapRemainsReadOnly`, `TestAutomaticConnectionsAreIsolatedAndRetryStable` geçti. | Resume yalnız explicit ID ile; aktif lease conflict'i steal'i engeller. |
| REQ-004 | PASS | `internal/mcp/complete.go:50-112`, `internal/workflow/finalize.go:15-125`; sorted profiles, reconcile, gate, guarded transitions ve lease cleanup tek composition'da. Denial/replay/legacy testleri geçti. | `no_profiles_configured` açıkça ayrı alan. |
| REQ-005 | PASS | Context key'i `*sdk.ServerSession`: `internal/mcp/server.go:43-58`; cleanup `server.go:161-170`; renewal `internal/workflow/heartbeat.go:13-149`. Full race suite ve heartbeat/finalize stress geçti. | Renewal başarısızlığı durable failure olarak completion'ı kesiyor. |
| REQ-006 | PASS | Immutable start intent ve deterministic phase ID'leri `internal/workflow/journal.go:20-162`; `StartRetryRestartAndFreshRuns`, independent-service duplicate start, restart stdio finalize ve partial-conflict recovery geçti. | Lost-response retry terminal sonucu tekrar üretiyor, ikinci transition üretmiyor. |
| REQ-007 | PASS | `internal/changes/automatic.go:62-195`, `internal/changes/automatic_restore.go:16-163`; pre-existing dirt, sequential same-symbol, concurrent completion, exact restore A→B→A ve historical row testleri geçti. | Terminal task'lar yalnız ownership yarışından çıkarılıyor; history silinmiyor. |
| REQ-008 | PASS | Gerçek subprocess `TestMCPSubprocessServesThirteenTools` tam ad listesini doğruladı; `TestZeroCeremonyEndToEndOverStdio` ve subdirectory varyantı geçti; legacy bootstrap/complete testleri side effect'i doğruladı; usage-refusal stdout 0 byte testi geçti. | Public isim veya tool sayısı değişmedi. |
| REQ-009 | PASS | `internal/cli/verify.go:22-100`; canlı bare verify çıktısı `Verification mode: staged changes (local)` ve exit 0; `TestVerifyDefaultMatchesStagedAndNamesMode` geçti. | `--staged && --ci` ve base/head-without-ci kodda explicit usage error. |
| REQ-010 | PASS | `README.md:9-65`, `docs/usage-tr.md:8-164`; advanced compatibility `docs/usage-tr.md:326-431`. | `jq`, manual task/revision/lease yalnız açıkça “İleri seviye” bölümünde. |
| REQ-011 | PASS (ürün/test); koordinatör kapanışı bekliyor | `make tidy-check`, `make verify`, `make gate`, `make bench`, `make release` mevcut dirty tree'nin scratch kopyasında yeniden geçti; belirtilen targeted testler ayrıca `-v` ile geçti. | Breaker + reconciliation sonucu bu Reader tarafından önceden görülmedi ve ilan edilmedi. |

## 4. FIX-001…FIX-012 ve schema-10 izlenebilirliği

| İddia | Reader kanıtı | Hüküm |
| --- | --- | --- |
| FIX-001 — ID-free finalize replay | `internal/mcp/automatic_contract_test.go:221-305`, `internal/workflow/service_test.go:132-161`; duplicate same-connection ve restart replay hedefli koşuda geçti. | PASS |
| FIX-002 — historical reconcile | `internal/workflow/journal.go:165-232`; `TestReconcileUsesDurableSnapshotPhases` ve `TestReconcileReappliesEarlierBytesAfterInterveningSnapshot` full/targeted koşularda geçti. | PASS |
| FIX-003 — context races | `internal/mcp/automatic.go:88-195`; `TestAutomaticConcurrentLifecycleUpdatesAreRaceFree` ve full `-race ./...` geçti. | PASS |
| FIX-004 — guard baseline ordering | Baseline startta index/discovery öncesi `internal/workflow/service.go:218-220`, reconcile'da `journal.go:212-223`; `TestAutomaticGuardBaselinePreservesProtectedTestRemoval` geçti. | PASS |
| FIX-005 — stale reply monotonicity | `internal/mcp/automatic.go:94-115,166-195,235-258`; delayed finalize/bootstrap/scope/handoff testleri `internal/mcp/automatic_internal_test.go:146-330` ve handoff internal testlerinde geçti. | PASS |
| FIX-006 — subdirectory launch | Worktree root canonicalization `internal/mcp/server.go:78-80`; in-memory ve gerçek subprocess subdirectory testleri geçti. | PASS |
| FIX-007 — protocol stdout isolation | `cmd/mindrail/main.go:24-31`; `TestMCPJSONIsTypedUsageRefusalWithProtocolCleanStdout` bütün varyantlarda stdout 0 byte doğruladı; gerçek stdio E2E geçti. | PASS |
| FIX-008 — historical profile replay | `internal/workflow/finalize.go:28-46`; `TestFinalizationReplayPreservesHistoricallyExecutedProfiles` config değişimiyle iki varyantta geçti. | PASS |
| FIX-009 — heartbeat/finalize race | `internal/workflow/heartbeat.go:68-85`; `TestHeartbeatConcurrentFinalizationStressPreservesReplay` ve full race geçti. | PASS |
| FIX-010 — handoff cleanup | `internal/workflow/finalize.go:128-168`; delayed handoff/new-run ve same-revision cleanup testleri geçti. | PASS |
| FIX-011 — exact-baseline restoration | `internal/changes/automatic_restore.go:16-163`; modified/deleted/renamed/added × restart false/true sekiz kolun tümü `TestReconcileRestoresExactBaseline` altında geçti. | PASS |
| FIX-012 — file-index CAS ABA / schema 10 | `migrations/000010_file_index_generations.sql:1-8`, `internal/index/store_cas.go:133-239,253-297`; v9 upgrade, legacy/new registration, restart/no-restart eski remove ve completion token testleri geçti. | PASS |

Schema-10 özel kontrolünde `TestFileGenerationMutationRequiresUpgradeFromVersionNine` v9'da write'ı fail-closed durdurdu, migration 10 sonrası eski live `attempts=7` ve symbol satırını korudu. `TestRemoveFileCASRejectsObserverFromBeforeRecreation` 4 kombinasyonda (`legacy` true/false × `restart` true/false) eski remove ve completion tokenlarını reddetti ve yeni generation'ın büyüdüğünü doğruladı.

## 5. Doğrulanmış Reader bulguları

Yok. `CRITICAL=0`, `HIGH=0`, `MEDIUM=0`, `LOW=0`.

Bu tablo “şüpheli görünen kod yok” anlamına gelmez; yukarıdaki hükümler çalıştırılmış test, canlı binary veya açık veri akışı kanıtına dayanır. Breaker'ın bağımsız falsification bulguları bu rapora önceden dahil edilmemiştir.

## 6. Scope uygunluğu

- **UNDER_IMPLEMENTATION:** bulunmadı.
- **OVER_IMPLEMENTATION / BEHAVIORAL_DRIFT:** task-owned cluster içinde bulunmadı. Expert CLI kaldırılmadı; yalnız hidden yapıldı. Legacy MCP mutasyon/evaluation ayrımı korundu.
- **SCOPE_CREEP:** audit planın açıkça dışladığı pre-existing `.claude/**`, `.wrongstack/**`, `.zcode/**` ve generated `graphify-out/**` Reader ürün hükmüne dahil edilmedi. Task-owned CLI/MCP/workflow/setup/index/schema/docs değişiklikleri REQ-001..011'e izlenebilir.
- **Birebirlik:** Reader açısından implementasyon dondurulmuş programla birebir. Normal insan yoluna lifecycle ID/lease/revision seremonisi sızmıyor; advanced uyumluluk yüzeyi silinmiyor.

## 7. Contract ve dokümantasyon uygunluğu

- **MCP sözleşmesi:** çalışan subprocess tam 13 tool adını yayımladı; automatic alanlar mevcut tool input schema'larına additive. Empty bootstrap read-only; explicit task complete evaluation-only kaldı.
- **CLI sözleşmesi:** root help beş normal komutu gösterdi; expert komutlar doğrudan ve `mindrail help <command>` ile erişilebilir. Bare verify staged davranışı ve insan mod satırı gözlendi.
- **Setup sözleşmesi:** AGENTS marker sınırı, foreign hook backup/chain, repository-local hook boundary, symlink/non-regular/malformed marker fail-closed davranışları test edildi.
- **Storage/migration sözleşmesi:** migration 10 additive `STRICT` table; eski live index/symbol verisini dönüştürmüyor veya silmiyor; mutation API schema floor 10'u açıkça dayatıyor.
- **DOC_DRIFT / SPEC_IMPLEMENTATION_CONFLICT:** bulunmadı. README ve Türkçe kılavuz `before_change` koşulunu doğru anlatıyor: bootstrap paths bütün editleri kapsıyorsa optional; paths atlandıysa veya scope büyüdüyse ilk editten önce zorunlu.

## 8. Off-spec kontroller

| Başlık | Sonuç | Kanıt |
| --- | --- | --- |
| Cost / performans | PASS | `make bench`: MCP p95 status `0.810 ms`, context `0.653 ms`, before_change `1.199 ms`, after_change `10.362 ms`, reconcile `16.004 ms`; tümü kendi hedefinin altında. Service p95 after_change `2.139 ms`, reconcile `13.822 ms`. |
| Mechanical rule = automated test | PASS | Hidden help, init idempotence/safety, default verify, exact 13 tools, stdout isolation, restart replay, heartbeat/race, dirty-tree/sequential attribution, restoration ve schema-10 ABA kuralları adlandırılmış otomatik testlere bağlı. |
| Backward compatibility | PASS | Legacy bootstrap/complete/lifecycle testleri; expert CLI callable testi; v9→v10 data-preservation ve old/new registration ABA testleri; subprocess wire-list testi. |

## 9. Çalıştırılan doğrulamalar

Tüm komutlar `/tmp/mindrail-reader-20260926-tPCMMS/repo` izole kopyasında ve Go komutları `GOCACHE=/tmp/mindrail-gocache` ile çalıştırıldı.

| Komut / prob | Sonuç |
| --- | --- |
| `go test -race ./internal/workflow ./internal/changes ./internal/coordination ./internal/completion ./internal/mcp ./cmd/mindrail` | PASS |
| `go test -race ./internal/setup ./internal/cli ./internal/index/... ./internal/migration ./migrations` | PASS |
| Gereksinim/FIX testlerinin iki ayrı `go test -v ... -run ...` seçimi | PASS; seçilen tüm adlandırılmış testler çalıştı |
| `make tidy-check` | PASS |
| `make verify` | PASS: vet, full normal, full race, compiled-binary smoke |
| `make gate` | PASS: unit 1417, domain 421, integration 12, race 9, knowledge-schema 216, MCP contract 57, git-worktree 77, named-worktree 8, sqlite-concurrency 17, end-to-end 12, smoke 7 |
| `make bench` | PASS; yukarıdaki p95 değerleri |
| `make release` | PASS: native linux/amd64 build + checksum + smoke; linux/arm64, Darwin ve Windows Tree-sitter build constraints nedeniyle açıkça `not-built` (mevcut release sınırı) |
| Canlı disposable repo: `init` iki kez, `status`, `doctor`, bare `verify` | Hepsi exit 0; schema 10; `Overall: OK`; bare verify `staged changes (local)`; tekrar initte 3/3 managed target hash'i aynı |
| Canlı `mindrail --help` | Normal yüzeyde init/status/doctor/verify/version; expert komut yok; meta help var |

Test kapsamı ile requirement kapsamı ayrıldı: full suite pass tek başına hüküm değildir; matris satırları ilgili davranış testi ve kod/sözleşme kanıtına ayrıca bağlandı.

## 10. Açık maddeler

| Madde | Neden bu Reader turunda üretilmedi | Tam kapatma deneyi |
| --- | --- | --- |
| REQ-011 nihai dual-agent audit geçişi | Bağımsızlık gereği Reader, raporunu yazmadan Breaker sonucunu okuyamaz. | Breaker kendi izole raporunu yazdıktan sonra `reconciliation.md` admission/refutation kuralıyla iki raporu birleştir; bütün mandatory Breaker hareketleri çalıştıysa ve surviving CRITICAL/HIGH/open yoksa final REQ-011'i kapat. |
| Mindrail yerel task completion kaydı | Bu alt-agent oturumunda AGENTS.md'deki `mindrail_*` MCP araçları tool envanterinde sunulmadı; Reader ürün dosyalarında workaround CLI lifecycle mutasyonu yapmadı. | Koordinatörün bootstrapped Mindrail oturumunda bu raporu reconcile edip `mindrail_complete {"finalize":true}` çağırması; denial varsa çözmesi. Bu ürün davranışı bulgusu değildir. |

## 11. İzolasyon ve bütünlük

- Scratch ürün kopyası: `/tmp/mindrail-reader-20260926-tPCMMS/repo`
- Disposable canlı quickstart repo: `/tmp/mindrail-reader-live-GDfL40`
- Prob öncesi ve prob sonrası manifest: `internal/**`, `cmd/**`, `migrations/**`, `README.md`, `AGENTS.md`, `docs/usage-tr.md`, frozen requirements dahil 475 SHA-256 satırı.
- Karşılaştırma sonucu: `NO_DIFF`. Orijinal ürün ağacı hiçbir prob tarafından değiştirilmedi; bu Reader yalnız bu rapor dosyasına yazdı.

## 12. Reader sonucu

### Reader — `APPROVE`

Gerekçe: Reader'ın doğrulayabildiği bütün `REQ-001..REQ-011` ürün kriterleri için pozitif kanıt üretildi; FIX-001..012 ve schema-10 yolları kod, contract, migration, focused test, full race/gate ve gerçek subprocess kanıtıyla tutarlı; task-owned scope'ta doğrulanmış bulgu, contract drift veya doküman drift yok.

Bağımsız sonuç: **`FULLY_COMPLIANT`** (Reader lensi). Bu ifade birleşik final verdict değildir; Breaker ve reconciliation tamamlanmadan `VERIFIED` ilan edilmez.

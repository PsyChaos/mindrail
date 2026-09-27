# Mindrail 0.1 bağımsız Reader denetimi

**Denetlenen revision:** `2814acb22d8aa94d48ade1242b58e1fdb14239fc`
**Rol:** Reader / conformance; uygulama veya düzeltme yapılmadı.
**Mod:** Paralel çift-ajan denetiminin bağımsız Reader kolu. Bu rapor Breaker
sonuçları görülmeden yazıldı.
**Reader kararı:** `REJECT` — `NON_COMPLIANT`.

## 1. Orijinal görev ve normalizasyon

> “Proje durumunu kontrol et. Tüm görevlerin bittiği bildirildi. Herşey
> sıkıntısız çalışıyor mu, bir sorun var mı takım olarak denetlet. Takımlara
> farklı modeller görev zorluğuna göre verebilirsin. Daha sonra bana detaylı bir
> usage dökümanı hazırla.”

| ID | Açık gereksinim | Kaynak | Tür | Reader doğrulaması |
|---|---|---|---|---|
| REQ-U1 | Mevcut proje durumunu kanıtla | “Proje durumunu kontrol et” | ACCEPTANCE | HEAD, görev matrisi, build/CLI ve odaklı testler |
| REQ-U2 | MR-001…MR-020 kapanış iddialarını bağımsız değerlendir | “Tüm görevlerin bittiği bildirildi” | NON_REGRESSION | Her MR için `PASS/PARTIAL/FAIL`; yeşil kutular tek başına kanıt sayılmadı |
| REQ-U3 | Gerçek kullanıcı/ajan akışlarında sorun olup olmadığını denetle | “Herşey sıkıntısız çalışıyor mu, bir sorun var mı” | BEHAVIORAL | completion, reconcile, evidence, MCP, CI ve latency/release akışları |
| REQ-U4 | Ayrıntılı kullanım belgesi üret | “detaylı bir usage dökümanı hazırla” | DELIVERABLE | Reader kapsamı değil; ana denetçi/ayrı dokümantasyon kolunun çıktısı |

## 2. Kısa sonuç

Hayır, 20/20 kutunun işaretli olması ürünün sıkıntısız çalıştığını göstermiyor.
Temel completion hipotezini bozan iki üretilmiş fail-open vardır:

1. `mindrail_complete` completion öncesinde canonical `Reconcile` çalıştırmaz.
   Açık task üzerindeki hiç kaydedilmemiş gerçek edit `ALLOW` kalır; aynı
   bytes explicit reconcile edildikten sonra iki denial ile `DENY` olur.
2. Gerekli evidence minimumu server/policy tarafından bulunmaz; MCP çağrıcısı
   `required` listesini çıkararak aynı stale evidence durumunu `DENY`dan
   `ALLOW`a çevirebilir.

Bunlara ek olarak 13 MCP aracı üretim binary'sinden erişilemez, MCP contract
testi SDK'nın ortak framed-I/O çekirdeğini `net.Pipe` üzerinden sınasa da gerçek
stdin/stdout launcher'ını sınamaz, CI doğrulaması required validation profile
adımını hiç çalıştırmaz ve MR-019'un timeout/partial mekanizması canlı
operasyonlara bağlanmamıştır. Breaker reconciliation turunda üç yeni HIGH
doğrulandı: fail/error validation kanıt sayılıyor, scope'a eklenen yeni dosya
kanıtı bayatlatmıyor ve fatal knowledge problemi MCP completion'da düşüyor. Bu
nedenle MR-020'deki “Ship 0.1” kararı mevcut HEAD için doğrulanamaz.

## 3. MR-001…MR-020 gereksinim matrisi

`PASS`, ilgili dilimin çekirdek davranışı için pozitif kod/test kanıtı bulunduğu
anlamına gelir; ürünün tamamı için onay değildir. `PARTIAL` ve `FAIL` satırları
aşağıdaki findings'e bağlıdır.

| MR | Reader | Kanıt ve boşluk |
|---|---|---|
| MR-001 | PASS | Yerel binary build edildi; `version --json` exit 0. Bootstrap smoke sözleşmesi `cmd/mindrail/smoke_test.go:25`; görev `mindrail-0.1-task-list.md:35`. |
| MR-002 | PASS (parser/CLI scope) | Knowledge schema/lineage katmanı ve CI/staged knowledge-first yolları çalışır (`internal/verify/verify.go:67-77`, `internal/verify/ci.go:25-35`). B-03 bu parser'ı değil, MR-016 MCP completion tüketicisinin `Problems` listesini atmasını bozar. |
| MR-003 | PASS | Restart sonrası task/checkpoint testi odaklı koşuda geçti: `internal/coordination/store_test.go:177`. |
| MR-004 | PASS | Lease/idempotency/revision için concurrency ve operation test yüzeyi var; odaklı coordination paketi yeşil. |
| MR-005 | PASS | Python/TS/JS registry/index/scheduler paketleri mevcut; odaklı `internal/index/...` koşusu yeşil. |
| MR-006 | PASS | Cross-process UID ve rename/ambiguity E2E testleri geçti: `internal/index/identity_test.go:838`, `internal/index/symbol/refresh_test.go:595`. |
| MR-007 | PASS (servis) | `Reconcile` gerçek Git diff'ini işler (`internal/changes/reconcile.go:27-69`); odaklı convergence testi geçti. Completion entegrasyonu MR-013 altında başarısızdır. |
| MR-008 | PASS (servis) | Attribution/drift E2E odaklı testi geçti: `internal/changes/e2e_attribution_test.go:21`. |
| MR-009 | PASS | Bounded impact E2E odaklı testi geçti: `internal/impact/e2e_impact_test.go:21`. |
| MR-010 | PASS (runner) | Snapshot-bound runner akışı odaklı testte geçti: `internal/validation/service_test.go:17`. |
| MR-011 | PARTIAL | Mevcut dosyanın içeriğini değiştirme staleness yolu çalışır; fakat başarısız/error evidence current coverage sayılır (B-01) ve directory scope'a yeni dosya eklemek stale üretmez (B-02). Bu ikinci durum frozen AC-01.2'de açıkça gereklidir (`mr-011-requirements.md:107-116`). |
| MR-012 | PASS (guard) | Test weakening E2E odaklı testte geçti: `internal/testguard/e2e_test.go:15`. |
| MR-013 | **FAIL** | Görev completion'ın önce reconcile etmesini ve stale/missing evidence'ı reddetmesini ister (`mindrail-0.1-task-list.md:641-653`). F-01/B-04, B-01 ve B-02 bunun tersini canlı üretir. |
| MR-014 | PARTIAL | 6 handler/şema içeride var, fakat binary MCP transport sunmuyor; F-03. |
| MR-015 | PARTIAL | 5 lifecycle handler'ı ve lifecycle testi var (`internal/mcp/discovery_test.go:114`), fakat gerçek kullanıcı MCP üzerinden bunlara erişemez; F-03. |
| MR-016 | **FAIL** | Senaryo “kanıt kurallarını atlayamaz” (`mindrail-0.1-task-list.md:757-772`); F-01/B-04, B-01/B-02/B-03 canlı bypass'lardır. F-03 production reachability boşluğudur. |
| MR-017 | PASS (frozen scope) | Staged diff/guard/hook çekirdeği testlidir (`internal/verify/verify_test.go:142-328`). Evidence coverage frozen MR-017 kapsamına alınmamıştır; üst pipeline boşluğu F-05/MR-018 altında tutulur. |
| MR-018 | PARTIAL | Fresh-clone/no-verify akışı odaklı testte geçti (`internal/cli/verify_ci_clone_test.go:67`), fakat görevde yazan required profiles adımı (`mindrail-0.1-task-list.md:841-843`) bilinçli olarak yoktur; F-05. |
| MR-019 | PARTIAL | Benchmark/release/gate altyapısı var; canlı timeout → partial/pending kablosu yoktur. Görev kaydı da bunu “canlı-op kablosuz erteleme” diye kabul eder (`mindrail-0.1-task-list.md:910-918`); F-04. |
| MR-020 | **FAIL** | “Ship 0.1” kabulü primary bypass denemelerini kapsadığını iddia eder; F-01/B-04 ve B-01…B-03 bu iddiayı çürütür, F-03 ürünün birincil agent arayüzünü erişilemez bırakır. |

Özet: **12 PASS, 5 PARTIAL, 3 FAIL**. PASS satırlarının çoğu alt-servis
düzeyindedir; release kararı, public workflow zincirinde FAIL bulunduğu için
`APPROVE` değildir.

## 4. Bulgular

| ID | Şiddet | Etkilenen MR | Bulgu | Güven |
|---|---|---|---|---|
| F-01 / B-04 | 🟠 HIGH | MR-013, MR-016, MR-020 | Completion canonical reconcile çalıştırmadan gerçek yeni edit'e ALLOW verir | CONFIRMED ×2 |
| F-02 | 🟡 MEDIUM (contract gap) | MR-013, MR-016, MR-020 | Çağrıcı `required` listesini çıkararak stale/missing evidence DENY'ını kaldırabilir; frozen D-213 bunu kasıtlı tanımlar, üst kernel sözleşmesiyle çatışır | CONFIRMED |
| B-01 | 🟠 HIGH | MR-011, MR-013, MR-016, MR-020 | `fail`/`error` evidence ve kısmen başarısız multi-command profile completion kanıtı sayılır | CONFIRMED ×2 |
| B-02 | 🟠 HIGH | MR-011, MR-013, MR-016, MR-020 | Directory scope'a yeni dosya eklemek eski evidence'ı stale yapmaz | CONFIRMED ×2 |
| B-03 | 🟠 HIGH | MR-016, MR-020 | Fatal unsupported knowledge schema CLI'da DENY iken MCP completion'da ALLOW | CONFIRMED ×2 |
| F-03 | 🟠 HIGH | MR-014…MR-016, MR-020 | Binary'de production MCP stdio transport/komutu yok; test shared framed-I/O core'u sınar fakat launcher/subprocess'i sınamaz | CONFIRMED; gerekçe kısmen düzeltildi |
| F-04 | 🟡 MEDIUM (known narrowing) | MR-019 | Timeout/partial/pending mekanizması canlı operasyonlara bağlanmamış, sadece çağrıcısız impact servisi ve testlerde var | HIGH confidence |
| F-05 | 🟡 MEDIUM (known narrowing) | MR-018, MR-020 | Staged/CI gate required validation profiles/evidence coverage değerlendirmiyor; D-226 bunu açıkça 0.1 dışına iter | CONFIRMED |
| F-06 | 🟢 LOW | release/docs | README, AGENTS ve version çıktısı 20/20 “done/Ship” kaydıyla çelişiyor | CONFIRMED |

### F-01 — Completion gerçek diff'i reconcile etmeden ALLOW veriyor

**Sözleşme.** MR-013 acceptance açıkça “`before_change` çağrılmamış olsa da
completion önce reconcile eder” der (`docs/engineering/mindrail-0.1-task-list.md:650`).
Kernel scope da protokol çağrısını atlamanın gerçek diff'i gizleyemeyeceğini
söyler (`docs/specification/mindrail-technical-specification-1.0.md:3022-3054`).

**Kod yolu.** `internal/mcp/complete.go:66` doğrudan `compose` çağırır;
`compose` `internal/mcp/complete.go:94-160` içinde yalnızca daha önce yazılmış
attribution/evidence/binding/guard satırlarını okur. `changes.Reconcile` çağrısı
yoktur. Okuma tarafı `EvaluateTask`, açık Change yoksa boş sonucu temiz kabul
eder (`internal/changes/evaluation.go:90-100`).

**Üretilmiş ölçüm.** HEAD'in disposable `/tmp` kopyasında aynı server/task ve
aynı source bytes üç ardışık durumda ölçüldü.

| Ölçü | Temiz kontrol | Edit, discovery atlandı | Aynı edit, explicit reconcile |
|---|---:|---:|---:|
| `allow=true` | 1 | 1 | 0 |
| denial sayısı | 0 | 0 | 2 |

Explicit reconcile `SCOPE_DRIFT` + `UNREGISTERED_CHANGE` üretir; aradaki tek
fark discovery çağrısıdır. Daha önce kayıtlı, sahipli aynı dosyada sonraki bir
editin sırf değiştiği için DENY olması gerektiği ayrıca iddia edilmez; o kol
reconcile'ın çağrıldığına dair gözlenebilir bir denial garantisi taşımaz.

**Beklenen:** completion'ın explicit reconcile ile aynı actual-diff girdisini
kullanması ve bu senaryoda aynı iki denial'ı üretmesi.
**Gerçek:** `allow:true`, 0 denial.
**Etkisi:** primary gate, ajan protokolü atladığında fail-open.
**Öneri (ölçülmedi):** completion kompozisyonunun ilk adımında canonical
worktree reconcile çalıştırmak ve elde edilen gerçek snapshot/change kimliğini
bütün gate girdilerine taşımak.

### F-02 — Evidence minimumu caller-controlled ve çıkarılabilir

**Sözleşme.** Ürün “agent ... require current evidence” hipotezini taşır
(`docs/specification/mindrail-0.1-kernel-scope.md:13`); agent/human gerekli
minimum proof'u düşürememelidir
(`docs/specification/mindrail-technical-specification-1.0.md:4816-4821`).

**Kod yolu.** `CompleteIn.Required` public input'tur
(`internal/mcp/complete.go:22-30`). Server yalnız caller'ın verdiği profiller
için satır okur ve `validation.Check` çağırır
(`internal/mcp/complete.go:102-114`). Liste yoksa coverage boş ve gate temizdir.
Bu davranış testte özellikle “nothing required — ALLOW” diye pinlenmiştir
(`internal/mcp/complete_test.go:43-58`). Frozen MR-016 D-213 de caller-controlled
empty listeyi kabul eder (`docs/engineering/mr-016-requirements.md:109-112`),
ancak bu üst teknik sözleşmeyle çatışır.

**Üretilmiş ölçüm.** Aynı task, aynı stale evidence, aynı source snapshot:

| Ölçü | Kontrol `required:["test"]` | Senaryo `required` yok | Delta |
|---|---:|---:|---:|
| `allow=true` | 0 | 1 | +1 |
| denial sayısı | 1 | 0 | -1 |

Kontrol `REQUIRED_EVIDENCE_NOT_CURRENT` verir; tek input farkı `required`
alanının çıkarılmasıdır.
**Öneri (ölçülmedi):** minimum profilleri repository policy + impact/invariant
planından server-side türetmek; caller yalnız daha geniş profil ekleyebilmeli.

### F-03 — 13 MCP aracı public binary'den erişilemiyor

**Sözleşme çatışması.** Tech stack aynı executable'ın MCP stdio server sunmasını
ve `mindrail mcp` komutunu tanımlar
(`docs/specification/mindrail-tech-stack.md:219-252`, `:502-524`) ve altıncı
milestone “MCP stdio server + 13 tool contracts” der (`:3562-3575`). Buna
karşılık frozen D-191 production wiring'i sonraya bırakır
(`docs/engineering/mr-014-requirements.md:103-106`).

**Üretilmiş ölçüm:**

```text
$ go build -o /tmp/mindrail-reader-bin ./cmd/mindrail
$ /tmp/mindrail-reader-bin version --json
exit 0, "mcp_compatibility":"none"

$ /tmp/mindrail-reader-bin mcp --json
exit 2, COMMAND_LINE_INVALID: unknown command "mcp"
```

Version sabiti de açıkça `none`dır (`internal/cli/version.go:27-30`). Server
yalnız `internal/mcp.Server.SDK()` olarak açılır ve yorum transport'u “later
process wiring”e bırakır (`internal/mcp/server.go:141-144`). Üretim binary'sine
bağlı MCP endpoint sayısı **0**, internal server'a kayıtlı handler sayısı **13**.

`TestStdioDiscoversThirteenTools` satır 25'te
`sdk.NewInMemoryTransports()` kullanır (`internal/mcp/stdio_test.go:13-32`).
Reconciliation kontrolünde SDK v1.8.0 kaynak kodu bunun `net.Pipe` kullandığını
ve `StdioTransport` ile aynı framed-I/O çekirdeğine girdiğini gösterdi. Bu
nedenle ilk Reader gerekçesindeki “byte stream sınanmamıştır” bölümü
**refute edildi**. Kalan ve üretilmiş bulgu daha dardır: binary'de launcher
yoktur (`mindrail mcp` exit 2), gerçek process stdin/stdout boundary'si ve
subprocess yaşam döngüsü test edilmemiştir.

**Öneri (ölçülmedi):** aynı binary içinde official SDK stdio transport'u açan
production command/wiring, gerçek stdio subprocess smoke ve compatibility
alanının buna göre damgalanması.

### F-04 — MR-019 deferral canlı operasyonlarda yok (known narrowing)

Görev, bütçe içinde bitmeyen işin explicit partial/pending dönmesini ve kalan
işi schedule etmesini ister (`mindrail-0.1-task-list.md:886-898`). `Budget`
mekanizması vardır (`internal/perf/perf.go:113-173`) ve `impact.Analyze`
tarafından tüketilir (`internal/impact/impact.go:153-175`). Fakat production
Go dosyalarında `perf.NewBudget` veya `perf.WithBudget` çağrısı yoktur; arama:

```bash
rg -n 'NewBudget\(|WithBudget\(' --glob '*.go' --glob '!**/*_test.go' internal
# sonuç: 0 çağrı (yalnız tanımlar)
```

`impact.Service.Analyze` için production caller da yoktur; yalnız test/benchmark
çağrıları vardır. Milestone kaydı bunu “canlı-op kablosuz erteleme” olarak
itiraf eder (`mindrail-0.1-task-list.md:913-915`) ve findings belgesi
`Analyze`ın 0.1'de production çağrıcısı olmadığını yazar
(`docs/engineering/mr-019-findings.md:73-80`).

Bu yüzden bu bulgu gizli bir runtime regresyonu değil, üst task metni ile
frozen daraltma arasındaki ölçülmüş teslim boşluğudur; reconciliation'da
şiddeti HIGH'dan MEDIUM'a indirildi.

**Tam doğrulama için gereken deney:** production MCP stdio wiring'i geldikten
sonra, her 5 warm-path operasyona hedefin hemen altı/üstü injected budget verip
elapsed, partial/pending ve scheduler queue position ölçülmeli. Bugünkü HEAD'de
bu deney public operasyon üzerinde kurulamaz.

### F-05 — Staged/CI required profile/evidence adımını atlıyor (known narrowing)

MR-018 görev tanımı pipeline'a “required validation profiles” koyar
(`mindrail-0.1-task-list.md:841-843`). `verify.Service.compose` ise attribution,
bindings, ambiguity, guard ve drift'i `gate.Input`a ekler; `Coverage` hiç
atanmaz (`internal/verify/ci.go:103-159`). Validation store/runner service'e
enjekte edilmez (`internal/verify/verify.go:45-57`). Frozen D-226 açığı açıkça
“no required validation profiles are executed in 0.1 CI” diye kabul eder
(`docs/engineering/mr-018-requirements.md:97-104`); görev durumu da
“profil-çalıştırma eklenmedi” der (`mindrail-0.1-task-list.md:865-872`).

Bu nedenle no-verify/guard ve knowledge-first yolları çalışsa bile CI “required
current evidence” kuralını bağımsız olarak uygulamaz. D-226 bunun kasıtlı 0.1
daraltması olduğunu açıkça kaydettiği için frozen MR-018 implementasyon hatası
olarak değil, üst task/spec ile teslim arasındaki MEDIUM contract gap olarak
notlandırıldı.
**Tam doğrulama için gereken deney:** config/policy ile belirli bir changed
scope'a `test` profilini required yap, stale/missing evidence içeren commit'i
fresh clone'da `mindrail verify --ci` ile çalıştır; doğru uygulama
`REQUIRED_EVIDENCE_NOT_CURRENT` vermeli, mevcut compose şekli coverage üretmez.

### F-06 — Release ve kullanım belgeleri birbiriyle çelişiyor

- Task list 20/20 `[x]` ve MR-020 “Ship 0.1” der.
- `README.md:11-12` hâlâ “0.1 kernel under construction / command surface ...
  not complete” der.
- `AGENTS.md:4-7` Mindrail 0.1'in implement edilmediğini ve MCP çağrılarının
  fail edeceğini söyler.
- Binary `mcp_compatibility:none` raporlar (`internal/cli/version.go:27-30`).

Bu yalnız kozmetik değildir: F-03 ile birlikte kullanıcıya hangi yüzeyin gerçek
olduğunu belirsiz bırakır.

## 5. Reprodüksiyon eki (durable)

Aşağıdaki test, HEAD'in disposable kopyasında
`internal/mcp/reader_evidence_bypass_test.go` olarak kaydedilip çalıştırıldı.
Repository'nin mevcut package-local fixture'larını kullanır; production kaynak
değişikliği gerektirmez.

```go
package mcp_test

import (
    "os"
    "os/exec"
    "path/filepath"
    "testing"
    "github.com/PsyChaos/mindrail/internal/mcp"
)

func TestReaderRequiredEvidenceCanBeOmitted(t *testing.T) {
    root := newProfileRepo(t)
    server := newTestServer(t, root)
    coord, db := coordinationStore(t, root)
    workspaceID, projectID := workspaceOf(t, db)
    taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)

    callTool(t, server, "probe", mcp.ToolValidate, map[string]any{"profile": "test"})
    if err := os.WriteFile(filepath.Join(root, "tests", "t.py"), []byte("print(2)\n"), 0o644); err != nil { t.Fatal(err) }

    required := callTool(t, server, "probe", mcp.ToolComplete,
        map[string]any{"task_id": taskID, "required": []string{"test"}})
    omitted := callTool(t, server, "probe", mcp.ToolComplete,
        map[string]any{"task_id": taskID})
    t.Logf("required=test: allow=%v denials=%d", required["allow"], len(required["denials"].([]any)))
    t.Logf("required omitted: allow=%v denials=%d", omitted["allow"], len(omitted["denials"].([]any)))
}

func TestReaderCompleteMissesCanonicalReconcile(t *testing.T) {
    root := newTestRepo(t)
    for _, args := range [][]string{
        {"add", "."},
        {"-c", "user.email=reader@example.invalid", "-c", "user.name=Reader", "commit", "-qm", "baseline"},
    } {
        cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
        if out, err := cmd.CombinedOutput(); err != nil { t.Fatalf("%v: %s", err, out) }
    }
    server := newTestServer(t, root)
    coord, db := coordinationStore(t, root)
    workspaceID, projectID := workspaceOf(t, db)
    taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
    args := map[string]any{"task_id": taskID}
    clean := callTool(t, server, "probe", mcp.ToolComplete, args)
    if err := os.WriteFile(filepath.Join(root, "pkg", "a.py"), []byte("def helper():\n    return 2\n"), 0o644); err != nil { t.Fatal(err) }
    skipped := callTool(t, server, "probe", mcp.ToolComplete, args)
    callTool(t, server, "probe", mcp.ToolReconcile, args)
    reconciled := callTool(t, server, "probe", mcp.ToolComplete, args)
    t.Logf("clean=%+v skipped=%+v reconciled=%+v", clean, skipped, reconciled)
}
```

Koşum:

```bash
git archive 2814acb | tar -x -C /tmp/mindrail-reader-2814acb
# Yukarıdaki dosyayı internal/mcp/ altına kaydet.
GOCACHE=/tmp/mindrail-reader-go-build go test -count=1 -v \
  -run '^TestReader(RequiredEvidenceCanBeOmitted|CompleteMissesCanonicalReconcile)$' \
  ./internal/mcp/
```

Gözlenen çıktı:

```text
required=test: allow=false denials=1
required omitted: allow=true denials=0
clean: allow=true denials=0
edit without discovery: allow=true denials=0
same edit after reconcile: allow=false denials=2
PASS
```

## 6. Test ve doğrulama değerlendirmesi

Reader'ın çalıştırdığı read-only/focused kontroller:

```text
git diff -- '*.go'                 -> boş
go vet ./...                       -> PASS
staticcheck ./...                  -> NOT INSTALLED

Odaklı MR test seçimi              -> PASS
coordination/index/symbol/changes/impact/validation/testguard/gate/
mcp/verify/cli paketlerinde seçilen 18 kritik journey

Scratch fail-open probes           -> PASS (kusurlar üretildi)
go build ./cmd/mindrail            -> PASS
mindrail version --json            -> exit 0, mcp_compatibility=none
mindrail mcp --json                -> exit 2, COMMAND_LINE_INVALID
```

Ana denetçi full suite'i merkezi olarak çalıştırdığı için Reader ikinci bir tam
suite başlatmadı. Merkezi koşumun Reader raporu yazılırken paylaşılan sonucu:
`make verify` exit 0 (normal/race 33 package; smoke 2.764 s), `make tidy-check`
exit 0 ve `make release` exit 0 (stamped `0.1.0`, native
`linux/amd64`). Bu güçlü bir build/regression sinyalidir, fakat F-01/F-02'yi
çürütmez: mevcut testler her denial
ailesini önceden hazırlanmış gate girdisiyle sınar, fakat completion'ın o
girdileri gerçek Git/policy'den bağımsız üretip üretmediğini sınamaz.

## 7. Scope ve contract uyumu

- **UNDER_IMPLEMENTATION:** completion→reconcile, policy-derived evidence
  minimum, production MCP stdio wiring, CI evidence step, canlı budget wiring.
- **SPEC_IMPLEMENTATION_CONFLICT:** tech-stack `mindrail mcp`/stdio isterken
  MR-014 D-191 bunu 0.1 dışına iter; teknik şartname minimum evidence'ı caller'ın
  düşüremeyeceğini söylerken MR-016 D-213 boş caller listesine ALLOW verir.
- **DOC_DRIFT:** task list/MR-020 `Ship` ile README/AGENTS/version `under
  construction/not implemented/none` aynı anda doğrudur denemez.
- **OVER_IMPLEMENTATION/SCOPE_CREEP:** Reader'ın doğrulayabildiği ayrı bir
  üretim kapsam taşması yok.

## 8. Açık maddeler ve kesin deneyler

| Madde | Neden bu turda üretilmedi | Sonuçlandıracak kesin deney |
|---|---|---|
| Gerçek stdio cancellation/duplicate request | Production stdio entrypoint yok | `mindrail mcp` wiring sonrası SDK client subprocess ile list/call/cancel/duplicate; process exit, tool count, error envelope ölç |
| 5 public operasyonda budget aşımı | Production budget attachment yok | Her operation için injected deadline'ın hemen altı/üstü; elapsed + partial + pending + queue rank kontrol/scenario tablosu |
| CI required evidence denial | Profile→scope/policy mapping yok | Required `test` policy fixture + stale evidence commit + fresh-clone `verify --ci`; expected denial code `REQUIRED_EVIDENCE_NOT_CURRENT` |
| Çoklu platform release | Bu host yalnız linux/amd64 native smoke yapabilir | linux/arm64, darwin amd64/arm64, windows/amd64 native CI runner'larında build + `version` + smoke; yalnız geçen hedefi advertise et |

## 9. Breaker reconciliation / refutation turu

Breaker raporu artifact olarak yazıldıktan sonra B-01…B-04, Reader'ın ayrı
scratch HEAD kopyasında üretim MCP handler'larıyla yeniden koşturuldu:

```bash
GOCACHE=/tmp/mindrail-go-build go test -run '^TestReaderRefuteB0[1-4]' \
  -count=1 -v ./internal/mcp/
```

Koşu 1.857 saniyede PASS oldu; burada PASS, gözlenen kontrol/scenario
sayılarının aşağıdaki kusurları birebir ürettiği anlamına gelir. Breaker
bulgularının yokluğu üretilemedi.

### B-01 refutation sonucu — CONFIRMED

`validation.Check` coverage hesabında yalnız scope hash freshness'ına bakar
(`internal/validation/freshness.go:68-106`); evidence `Status`/`ExitCode` ve
multi-command run bütünlüğü bu karara girmez. Aynı başlangıç durumu için:

| Komut/profil | Validate öncesi | Evidence sonucu | Complete sonrası |
|---|---:|---:|---:|
| `true` kontrol | allow 0 / denial 1 | 1 pass | allow 1 / denial 0 |
| `false` scenario | allow 0 / denial 1 | 1 fail | allow 1 / denial 0 |
| nonexistent executable scenario | allow 0 / denial 1 | 1 error | allow 1 / denial 0 |
| `true` + `false` scenario | — | 1 pass + 1 fail | allow 1 / denial 0 |

Tek-satır status filtresi mixed profile'ı çözmeyeceğinden, önerilen tam fix
run identity + expected command set + bütün komutların pass/current olmasıdır;
bu öneri Reader tarafından uygulanıp ölçülmedi.

### B-02 refutation sonucu — CONFIRMED

`RunProfile` provenance'a directory kökünü değil, o anda enumerate edilmiş
somut dosya listesini yazar (`internal/validation/service.go:40-58`);
`rehashScope` yalnız bu listeyi yeniden hash'ler
(`internal/validation/freshness.go:119-165`). Frozen AC-01.2 “added file in
scope” için stale ister (`docs/engineering/mr-011-requirements.md:107-116`).

| Operasyon | Önce | Sonra |
|---|---:|---:|
| unchanged kontrol | allow 1 / denial 0 | allow 1 / denial 0 |
| mevcut `t.py` edit kontrolü | allow 1 / denial 0 | allow 0 / denial 1 |
| `tests/added.py` ekleme scenario | allow 1 / denial 0 | allow 1 / denial 0 |

Kök neden tüm directory-scope profile'larda ortaktır. Fix önerisi scope
köklerini provenance'a almak ve check anında yeniden enumerate etmektir;
backward compatibility için eski provenance satırları ayrıca fail-safe policy
ister. Fix ölçülmedi.

### B-03 refutation sonucu — CONFIRMED

MCP `loadKnowledge`, loader'ın `Problems` listesini kullanmadan yalnız geçerli
`store.Invariants` üzerinde döner (`internal/mcp/complete.go:175-200`). Uzun
yaşayan MCP status da startup `Subject` snapshot'ını tekrar render eder
(`internal/mcp/server.go:178-182`). Aynı repository için:

| Kol | Completion | Knowledge/status |
|---|---:|---:|
| schema v1 kontrol | allow 1 / denial 0 | READY, problem 0 |
| canlı server'a schema v999 yaz | allow 1 / denial 0 | READY, problem 0 |
| aynı bytes, CLI `verify --staged` | allow 0 | 1 fatal `KNOWLEDGE_SCHEMA_UNSUPPORTED` |
| aynı bytes, MCP restart | allow 1 / denial 0 | BLOCKED, problem 1 |

Restart status staleness savunmasını refute eder: subject yenilendiğinde BLOCKED
görülür, fakat completion yine Problems'ı düşürüp ALLOW verir. MR-002 parser ve
CLI fail-closed davranışı çalışmaktadır; hata MR-016 tüketicisindedir.

### B-04 refutation sonucu — CONFIRMED, F-01 ile birleştirildi

| Durum | Changed file | Completion |
|---|---:|---:|
| clean kontrol | 0 | allow 1 / denial 0 |
| edit, discovery atlandı | 1 | allow 1 / denial 0 |
| aynı edit, explicit reconcile | 1 | allow 0 / denial 2 |

B-04, Reader F-01 ile aynı kök nedendir ve ayrı sayılmadı. `VerifyStaged` ve
`VerifyCI` sibling yolları kendi reconcile çağrılarını yapar; kusurlu sibling
MCP complete composition'dır.

### Refute edilen gerekçe

| İddia | Refuting evidence | Nihai durum |
|---|---|---|
| F-03: in-memory MCP testi byte-stream/framing'i sınamaz | SDK v1.8.0 `NewInMemoryTransports` `net.Pipe` kullanır ve `StdioTransport` ile aynı `newIOConn` framed-I/O yoluna girer | Bu gerekçe refute edildi; production launcher/stdio command yokluğu canlı `mindrail mcp` exit 2 ile ayakta |

Hiçbir B-01…B-04 bulgusu tamamen refute edilmedi. Yeşil full suite ve release
koşusu regression/build kanıtıdır; yukarıdaki kontrol/scenario farklarını
ortadan kaldırmaz.

### Şiddet reconciliation

- B-01/B-02/B-03 ve F-01/B-04, production MCP command henüz dışarıya bağlı
  olmadığı için bugün uzaktan sömürülebilir güvenlik açıkları değildir; yine de
  0.1 diye ilan edilen handler davranışını ve release acceptance'ı bozdukları
  için **HIGH release blocker** olarak tutuldu.
- F-02, frozen D-213'ün bilinçli davranışıdır; implementation bug değil, üst
  kernel minimum-proof sözleşmesiyle **MEDIUM contract gap**tir.
- F-04 ve F-05 frozen D-231/D-226 daraltmalarıyla açıkça kaydedilmiş teslim
  boşluklarıdır; gizli regresyon değil **MEDIUM known narrowing**dir.

## 10. Bağımsız sonuç

`NON_COMPLIANT` / `REJECT`.

Alt servislerin önemli bölümü iyi test edilmiş ve odaklı/full testler yeşildir;
ancak completion'ın gerçek diff'i kendisinin keşfetmemesi, başarısız validation'ı
kanıt sayması, yeni scope dosyasını görmemesi ve fatal knowledge problemını
düşürmesi ürünün ana correctness iddiasını bozar. Bu HIGH release blocker'lar
kapanmadan ve public MCP/CI/budget boşlukları teslim sözleşmesiyle
uzlaştırılmadan “20/20 tamamlandı” veya “Ship 0.1” sonucu korunamaz.

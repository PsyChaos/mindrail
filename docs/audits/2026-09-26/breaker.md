# Bağımsız Breaker raporu — Mindrail 0.1

Tarih: 2026-09-26. Denetlenen kaynak: `2814acb`. Rol: bağımsız adversarial Breaker; Reader sonuçları görülmeden yazıldı.

## Görev ve sınır

Kullanıcının özgün isteği: “Proje durumunu kontrol et. Tüm görevlerin bittiği bildirildi. Herşey sıkıntısız çalışıyor mu, bir sorun var mı takım olarak denetlet. Takımlara farklı modeller görev zorluğuna göre verebilirsin. Daha sonra bana detaylı bir usage dökümanı hazırla.”

Kapsam son dokümantasyon commit'inin diff'iyle sınırlanmadı: tamamlandığı bildirilen 0.1 ürün davranışı, kernel kapsamı ve MR-019/020 kapanış iddiaları denetlendi. Kullanıcı ağacındaki önceden var olan `.claude/`, `.gitignore`, `graphify-out/`, `.wrongstack/`, `.zcode/` değişiklikleri ürüne mal edilmedi. Ürün kaynakları değiştirilmedi.

`dual-agent-task-audit` Breaker protokolü kullanıldı. Önce `graphify query "gate verify reconcile complete scope kernel" --budget 1600` çalıştırıldı; graf 349 ilişkili düğüm buldu, bütçe nedeniyle 52'sini gösterdi. Graf yalnız gezinme için kullanıldı, doğruluk kaynak ve çalışan servislerden ölçüldü. Graf/model servisine yeni API çağrısı yapılmadı.

İzolasyon: `mktemp -d /tmp/mindrail-breaker-XXXXXX` → `/tmp/mindrail-breaker-tZ4Oqy`; `git archive HEAD` bu kopyaya açıldı, yerel baseline commit oluşturuldu. Deneme repoları yalnız Go `t.TempDir()` içinde oluşturuldu. Go derleme önbelleği `/tmp/mindrail-breaker-tZ4Oqy-gocache`. İlk koşu salt okunur kullanıcı Go cache'i yüzünden kurulamadı; cache /tmp'ye alınarak tekrarlandı. Bu altyapı hatası ürün bulgusu sayılmadı.

## Sonuç

Dört HIGH davranış hatası canlı üretildi. MCP completion'ın “ALLOW” demesi şu an yeterli mühendislik kanıtı anlamına gelmiyor. Mevcut pozitif testler bu boşlukları yakalamıyor.

Test sürücüsü gerçek SDK in-memory transport üzerinden üretim MCP handler'larını, gerçek Git repolarını, gerçek SQLite kayıtlarını ve gerçek `true` / `false` süreçlerini kullanır. Verdict mock'lanmaz. CLI karşılaştırması üretim Cobra command tree'sini çalıştırır. Bu denemeler ayrı bir MCP executable dağıtımı bulunduğunu iddia etmez.

### [HIGH] B-01 — Başarısız veya hiç başlayamayan doğrulama completion kanıtı sayılıyor

Dosya: [freshness.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/validation/freshness.go:94), [service.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/validation/service.go:49).

`Check`, profil kapsamı hash'i eşleşen herhangi bir evidence satırını yeterli sayıyor; satırın `Status` ve `ExitCode` alanlarını kontrol etmiyor. Gerçek `false` komutu status=fail, bulunamayan executable status=error üretmesine rağmen sonraki `mindrail_complete(required=["test"])` ALLOW dönüyor. Çok komutlu profilde de tek başarılı satır bütün profil için yeterli kabul ediliyor.

| Aynı kapsam üzerindeki kol | Evidence satırı | Başarılı komut | Completion ALLOW | Denial sayısı |
|---|---:|---:|---:|---:|
| Kontrol: henüz validate yok | 0 | 0 | false | 1 |
| Kontrol: `true` | 1 | 1 | true | 0 |
| Senaryo: `false` | 1 | 0 | true | 0 |
| Senaryo: olmayan executable | 1 | 0 | true | 0 |
| Senaryo: `true`, sonra `false` | 2 | 1 | true | 0 |

Verbatim anlamlı çıktılar: `status:fail ... AFTER=map[allow:true denials:[]]`; `status:error ... AFTER=map[allow:true denials:[]]`; karma profil `status:pass ... status:fail ... COMPLETE=map[allow:true denials:[]]`.

Komut: `GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp -run '^TestBreaker(FailedEvidence|MixedProfile)$' -count=1 -v`.

Çözüm ölçümü (yalnız scratch): coverage koşuluna `rows[i].Status == StatusPass && rows[i].ExitCode == 0` eklendi, aynı üç tek-komutlu kol yeniden koşuldu. true ALLOW kaldı; false/error DENY oldu (1 denial). Üç alt test PASS. Ardından karma profil yeniden koşuldu: 2 satırın biri fail iken yine ALLOW, test FAIL. Bu nedenle tek satırlık düzeltme **yeterli çözüm değildir**. Tam çözüm profil yürütmesini tek run kimliğiyle gruplamalı, güncel profile ait bütün gerekli komutların başarılı ve güncel olduğunu doğrulamalı; timeout/error/fail ve eksik komutları reddetmeli. Tam çözüm uygulanmadı/ölçülmedi. Scratch değişikliği geri alındı.

Sınıf taraması: üretimde `validation.Check` çağrısı MCP compose yolunda tek; `gate.evidenceDenials` coverage bool'una güveniyor. CLI verify compose evidence coverage oluşturmuyor; aynı eksikliğin “CLI da failed test kanıtını reddediyor” diye örtülemeyeceği not edildi. timeout durumu aynı satır modelini kullanır ancak 10 dakikalık MCP timeout denemesi bu turda çalıştırılmadı.

Güven: yüksek, tekrar üretildi.

### [HIGH] B-02 — Kapsama yeni dosya eklemek eski kanıtı bayatlatmıyor

Dosya: [service.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/validation/service.go:54), [freshness.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/validation/freshness.go:69).

`paths=["tests"]` ile doğrulama yapılınca provenance yalnız o anda bulunan somut dosyaları saklıyor. Sonraki freshness okuması dizini yeniden keşfetmiyor. Bu nedenle `tests/added.py` eklenmesi mevcut test kanıtını geçersiz kılmıyor. Aynı profil ve dosya setine dair iddia eksik: “source changes again → stale” yeni dosyalar için sağlanmıyor.

| Kol | Başlangıç dosya | Son dosya | Completion önce → sonra | Denial sonra |
|---|---:|---:|---|---:|
| Kontrol: değişiklik yok | 1 | 1 | true → true | 0 |
| Kontrol: t.py içeriğini değiştir | 1 | 1 | true → false | 1 |
| Senaryo: added.py ekle | 1 | 2 | true → true | 0 |

Ekleme payload'u: `assert False\n`. Mevcut dosyayı aynı payload ile değiştiren kontrol düzgün biçimde `REQUIRED_EVIDENCE_NOT_CURRENT` alıyor; ekleyen kol almıyor.

Komut: `GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp -run '^TestBreakerNewScopeFile$' -count=1 -v`.

Çözüm: evidence provenance'ında orijinal kapsam köklerini ve yürütülen profilin kimliğini de sakla; freshness sırasında o kapsamı yeniden enumerate edip ekleme/silme/içerik değişikliğini birlikte karşılaştır. Eski evidence için güvenli uyumluluk politikası gerekli. Çözüm bu turda uygulanmadı; kesin kabul deneyi yukarıdaki üç kol + kapsam dışı eklemenin ALLOW kalması + eski provenance okuma koludur.

Sınıf taraması: `SnapshotScope` tek enumerate yazarı, `rehashScope` tek coverage okuru. Aynı tasarım bütün dizin tabanlı profilleri etkiler. Dosya silme/içerik değişikliği mevcut testlerde korunuyor; eksik durum dosya ekleme.

Güven: yüksek, tekrar üretildi.

### [HIGH] B-03 — Bilinmeyen knowledge şeması CLI'da reddedilirken MCP completion tarafından kabul ediliyor

Dosya: [complete.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/mcp/complete.go:184), [server.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/mcp/server.go:180).

MCP `loadKnowledge` loader sonucunun `Problems` listesini değerlendirmiyor. Yeni şemalı invariant geçerli invariant listesine girmeyince hiçbir kısıt yokmuş gibi davranılıyor. Aynı repoda CLI `verify --staged` fatal knowledge problem'ını reddediyor. Kalıcı MCP server'ın `status` yanıtı da başlangıç Subject snapshot'ını kullandığı için değişikliği görmüyor. Sunucu yeniden başlatıldığında status BLOCKED oluyor, fakat completion hâlâ ALLOW: kusur yalnız cache staleness değil.

| Aynı repository | Knowledge problem sayısı | Completion ALLOW | Durum/CLI |
|---|---:|---:|---|
| Kontrol: schema_version=1 | 0 | true | READY |
| Senaryo: mevcut server, schema_version=999 | status'ta 0 | true | MCP status READY |
| Aynı bozuk dosya, yeni CLI çağrısı | 1 fatal | false | KNOWLEDGE_SCHEMA_UNSUPPORTED |
| Aynı bozuk dosya, yeni MCP server | status'ta 1 | true | MCP status BLOCKED |

Payload: `{"schema_version":999,"kind":"invariant","id":"INV-0001"}`, yol `.mindrail/knowledge/invariants/INV-0001.json`.

Verbatim CLI: `"allow":false,"knowledge_problems":[{"code":"KNOWLEDGE_SCHEMA_UNSUPPORTED",...,"fatal":true}]`.
Verbatim restart: `RESTART_COMPLETE=map[allow:true denials:[]]`, `readiness:BLOCKED`.

Komut: `GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp -run '^TestBreakerMalformedKnowledge$' -count=1 -v`.

Çözüm: completion'da CLI ile aynı fatal knowledge kontrolünü uygula; uzun yaşayan MCP okuma yüzeylerinin canlı subject/knowledge durumunu yenile. Uygulanmadı/ölçülmedi. Kabul deneyi geçerli kontrolü ALLOW tutmalı; v999 satırını mevcut server, yeni server ve CLI üçlüsünde aynı fail-closed koduyla reddetmeli.

Sınıf taraması: CLI `verify.loadProblems` sağlam kontrolü içeriyor; MCP `loadKnowledge` Problems'ı düşürüyor; MCP status doğrudan `s.app.Subject()` snapshot'ı kullanıyor. Startup sonrası config değişikliği ve diğer knowledge bozuklukları ayrıca çalıştırılmadı; bu bulgunun kapsamı üretilen unsupported schema girdisidir.

Güven: yüksek, restart dahil tekrar üretildi.

### [HIGH] B-04 — Reconcile unutulduğunda completion gerçek source diff'ini keşfetmiyor

Dosya: [complete.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/mcp/complete.go:96), [evaluation.go](/home/heisenberg/Desktop/Projects/Mindrail/internal/changes/evaluation.go:100).

Temiz committed Python repository + açık task üzerinde `pkg/a.py` değiştiriliyor. before/after/reconcile çağrılmadan doğrudan complete ALLOW veriyor. Yalnız `mindrail_reconcile` eklenince aynı kaynak bytes'ı `SCOPE_DRIFT` ve `UNREGISTERED_CHANGE` ile DENY oluyor. Completion salt önceden kayıt edilmiş change satırlarını okuyor; actual diff keşfi yapmıyor.

| Kol / sıra | Kaynakta değiştirilmiş dosya | Completion denial | ALLOW |
|---|---:|---:|---:|
| Kontrol: temiz commit | 0 | 0 | true |
| Senaryo: edit, discovery çağrısı atlandı | 1 | 0 | true |
| Aynı edit, reconcile çağrıldı | 1 | 2 | false |

Komut: `GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp -run '^TestBreakerForgetReconcile$' -count=1 -v`.

Bu, kernel scope §8'in “forgetting a protocol call” güvencesine ve teknik şartname satır 5203'teki completion/verify actual diff keşfine aykırı. CLI staged/CI yollarının reconciliation yapması bu MCP completion sonucunu doğru yapmaz.

Çözüm: completion kararı öncesinde gerçek diff'i reconcile et ve keşfin başarılı/tam olduğundan emin ol. Uygulanmadı/ölçülmedi; tam çözüm task scope, başka task'ın gerçek değişikliği ve temiz task için aşırı reddetme testlerini de geçmeli.

Sınıf taraması: `verify.VerifyStaged` ReconcileStaged, `verify.VerifyCI` ReconcileRange çağırıyor; MCP complete yalnız EvaluateTask çağırıyor. MCP after_change/reconcile bağımsız araçları çalışıyor. Bu hata o araçların çağrılacağını varsayan composition'da.

Güven: yüksek, aynı dosya üzerinde kontrol/senaryo tekrar üretildi.

## Altı zorunlu hareket

| Hareket | Yapılan ölçüm | Sonuç / sınır |
|---|---|---|
| B1 Mutation | HIGH/CRITICAL severity koruması, snapshot eşitliği, worktree containment tek tek kaldırıldı | 3/3 seçilmiş guard kırmızı; her biri geri yüklendi. Tüm-repo exhaustive mutation **yapılmadı**. |
| B2 Silinen davranış A/B | `git diff 9f9fbb6..HEAD --name-only -- internal/ cmd/ migrations/ go.mod go.sum Makefile` | Çıktı boş: son release kapanışı runtime silmemiş. İlk 0.1 release için daha önce yayımlanmış kullanıcı sürümü yok. Runtime silme A/B uygulanabilir örnek bulunmadı; tüm tarih için davranış eşdeğerliği iddia edilmiyor. |
| B3 Tehdit modeli | Unutulan reconcile; failed/error evidence; unsupported knowledge; mevcut CI unregistered/guard/parity/worktree testleri | 4 canlı bulgu; CI örnek korumaları mevcut testlerle çalışıyor. |
| B4 Upgrade | Gerçek embedded migration 7→8; schema1 registered workspace; eski task revision; schema7→8 change korunması | Odaklı mevcut upgrade testleri yeşil. Eski kanıt satırlarının kapsam kökü taşımaması B-02 çözümünün ayrıca migration/uyumluluk konusu. |
| B5 En zayıf yeterli girdi | `FreshCurrent` + status=fail/error; `required=[]`; yeni dizin dosyası | status hatası B-01; ekleme B-02. required boşluğu aşağıda açık tasarım sınırı olarak kaydedildi. |
| B6 Çift okuyucu | Aynı v999 dosyasıyla MCP complete, kalıcı status, restart status, CLI verify | B-03: ALLOW/READY karşısında CLI DENY ve restart BLOCKED. |

### Seçilmiş mutasyon defteri

Kontroller mutasyondan önce PASS idi. Her mutasyon tek başına geçici kaynak kopyasında uygulandı.

| Mutasyon | Test komutu (önünde aynı GOCACHE) | Kontrol | Mutant |
|---|---|---|---|
| `hardSeverity` daima false | `go test ./internal/gate -run '^TestGateDeniesPerFamily$' -count=1 -v` | 5 denial ailesi, PASS | 3 aile, FAIL; orphan + ambiguity kayıp |
| `if current == row.SnapshotHash` → `if true` | `go test ./internal/validation -run '^TestCheckStalesRelevantEdits$' -count=1 -v` | değişen içerik stale, PASS | değişen içerik current, FAIL |
| `if !insideRoot(...)` → `if false && !insideRoot(...)` | `go test ./internal/mcp -run '^TestBeforeChangeDeclaresScope$' -count=1 -v` | kaçan scope reddedilir, PASS | `out-of-repo scope accepted`, FAIL |

3 seçilmiş guard öldürüldü; yaşayan mutant yok. Guard sayısının bütün repo envanteri çıkarılmadı. Bu örnek 0.1'in bütün korumalarının test altında olduğunu kanıtlamaz.

### Maliyet

Core denemelerinde hosted model/API çağrısı: 0; token/model seçimi yükselten bir ürün akışı bulunmadı.

Ürün sınırlarının çarpımı: `ValidationTimeout=10 dakika` gerçekte **her komut** için Runner içinde yeniden başlar. Config commands uzunluğu için üst sınır yok. N komutlu bir profil için timeout üst toplamı `N × 600 saniye`; 2 komut 1200 saniye (20 dakika), 100 komut 60.000 saniye (16 saat 40 dakika). “Bir profil 10 dakikada biter” garantisi verilemez. Aynı profile ait bütün evidence satırları limit/pagination olmadan okunur; freshness her satırın dosyalarını yeniden hash'ler: E historical run × F scope file × B average bytes = E×F×B okuma işi. Tek komut capture limiti stream başına 65.536 byte: iki stream ve iki truncation marker ile 131.096 byte; 100 komut 13.109.600 byte raw capture üst toplamı, redaction/JSON overhead hariç.

İki komutlu karma profile gerçekten 2 evidence satırı üretildi. Büyük N ve 10 dakikalık timeoutlar bu turda koşturulmadı; yukarıdakiler kodun kendi sabitlerinin kesin çarpımıdır, gecikme benchmark'ı değildir. Repository sahibi config'i kontrol ettiği için bu başlık tek başına yeni güvenlik bulgusu sayılmadı. Profil toplam timeout'u amaçlanıyorsa dış context deadline'ı ve komut sayısı sınırı gerekir; henüz ölçülmedi.

### Mekanik kural ve otomatik denetim

Schema reader window fail-closed, successful/current proof ve unutulan protocol çağrısı sonrası actual diff keşfi mekanik kurallardır. Bu turdaki yeni testler ihlallerini FAIL ile gösterdi; mevcut suite'in yeşil olması bu kuralları uçtan uca kanıtlamıyor. Ayrı “test eksik” bulguları açılmadı; B-01…B-04'ün aynı kök neden/kanıt kapsamına alındı.

Var olan mekanik kontroller: `scripts/gate.sh` her kategori için en az bir gerçek top-level PASS ister; `make fmt-check`, arch testleri, knowledge schema testleri, lease uniqueness DB testi mevcuttur. Bu denetim bunların hepsine exhaustive mutation uygulamadı.

### Açık tasarım sınırı: required profilini çağıran seçiyor

Kontrol `complete(required=["test"])`, evidence yok → DENY/1. Aynı durumda `required=[]` → ALLOW/0. Bu canlı üretildi. MR-016 D-213 bunu açıkça tanımladığı için **ayrı bir implementation bug olarak sayılmadı**. Ancak kernel'in bağımsız required validation profile seçimi vaadiyle arasındaki boşluğu ürün sahibinin çözmesi gerekir. “Zorunlu testleri kendiliğinden bulup dayatır” kullanım iddiası mevcut kodla doğru değildir.

### Backward compatibility / açık deneyler

Ölçülen eski state kombinasyonları: schema1 workspace → init → aynı workspace; schema2 task → migration3 → revision1; schema7 changes → init8 → kayıt korunur; fresh schema0 → tüm 8 migration. Odaklı suite bu kolları geçti.

Üretilmeyen ve kesin deneyi gerekenler: profil config değişikliğinden sonra eski evidence'ın yeniden kullanımı (aynı profile yeni argv/paths yaz, MCP'yi restart et, complete required profili çağır); aynı profile farklı worktree evidence satırlarının taranma maliyeti; timeout evidence'ın MCP completion kararı (kısaltılabilir timeout seam ya da gerçek 10 dakika gerekir); tüm release guard'ları için exhaustive mutation. Bu alanlarda bug veya güvence iddia edilmez.

## Tekrar üretim

Aşağıdaki test dosyası repository'nin mevcut fixture yardımcılarını kullanır; kaynak fixture'lar denetlenen commit'te zaten bulunur. Sadece **scratch** kopyaya `internal/mcp/breaker_audit_test.go` olarak ekleyin. Gerçek kullanıcı repository'sine eklemeyin.

```sh
audit_dir=$(mktemp -d /tmp/mindrail-breaker-repro-XXXXXX)
git archive 2814acb | tar -x -C "$audit_dir"
cd "$audit_dir"
# Aşağıdaki Go bloğunu internal/mcp/breaker_audit_test.go olarak kaydedin.
GOCACHE="$audit_dir/go-cache" go test ./internal/mcp -run '^TestBreaker' -count=1 -v
```

Beklenen özgün sonuç: 6 top-level TestBreaker testi; 5'i FAIL, required-omission gözlem testi PASS. İç alt kollarla toplam 10 senaryo: 6 yanlış ALLOW gösteren başarısız assertion, 4 kontrol/gözlem PASS. Çıktı PID/path/id değerleri değişir, kararlar değişmemelidir.

```go
package mcp_test

import (
    "bytes"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"

    "github.com/PsyChaos/mindrail/internal/mcp"
    "github.com/PsyChaos/mindrail/internal/cli"
)

func breakerComplete(t *testing.T, root string, server *mcp.Server, required []string) map[string]any {
    t.Helper()
    coord, db := coordinationStore(t, root)
    workspaceID, projectID := workspaceOf(t, db)
    taskID, _ := openTaskAndSession(t, coord, workspaceID, projectID)
    return callTool(t, server, "breaker", mcp.ToolComplete, map[string]any{"task_id": taskID, "required":required})
}

func TestBreakerFailedEvidence(t *testing.T) {
    for _, command := range []string{"true", "false", "breaker-command-does-not-exist"} {
        t.Run(command, func(t *testing.T) {
            root := newProfileRepo(t)
            path := filepath.Join(root, ".mindrail", "config.toml")
            data, err := os.ReadFile(path); if err != nil { t.Fatal(err) }
            data = []byte(strings.ReplaceAll(string(data), `[["echo", "hi"]]`, `[["`+command+`"]]`))
            if err := os.WriteFile(path, data, 0644); err != nil {t.Fatal(err)}
            server := newTestServer(t, root)
            before := breakerComplete(t,root,server,[]string{"test"})
            ran := callTool(t,server,"breaker",mcp.ToolValidate,map[string]any{"profile":"test"})
            after := breakerComplete(t,root,server,[]string{"test"})
            t.Logf("command=%s BEFORE=%+v VALIDATE=%+v AFTER=%+v",command,before,ran,after)
            if command != "true" && after["allow"] == true {t.Error("FAILED OR UNEXECUTED VALIDATION SATISFIED COMPLETION")}
        })
    }
}

func TestBreakerNewScopeFile(t *testing.T) {
    for _, operation := range []string{"unchanged", "edit", "add"} {
        t.Run(operation,func(t *testing.T) {
            root := newProfileRepo(t)
            server := newTestServer(t,root)
            callTool(t,server,"breaker",mcp.ToolValidate,map[string]any{"profile":"test"})
            before := breakerComplete(t,root,server,[]string{"test"})
            if operation != "unchanged" {
                name := "t.py"; if operation == "add" {name = "added.py"}
                if err := os.WriteFile(filepath.Join(root,"tests",name),[]byte("assert False\n"),0644); err != nil {t.Fatal(err)}
            }
            after := breakerComplete(t,root,server,[]string{"test"})
            t.Logf("operation=%s BEFORE=%+v AFTER=%+v",operation,before,after)
            if operation != "unchanged" && after["allow"] == true {t.Error("CHANGED SCOPE SATISFIED STALE PROOF")}
        })
    }
}

func TestBreakerOmitRequired(t *testing.T) {
    root := newProfileRepo(t)
    server := newTestServer(t,root)
    before := breakerComplete(t,root,server,[]string{"test"})
    after := breakerComplete(t,root,server,[]string{})
    t.Logf("REQUIRED=%+v OMITTED=%+v",before,after)
}

func TestBreakerMalformedKnowledge(t *testing.T) {
    root := newProfileRepo(t)
    server := newTestServer(t,root)
    before := breakerComplete(t,root,server,[]string{})
    path := filepath.Join(root,".mindrail","knowledge","invariants","INV-0001.json")
    if err := os.WriteFile(path,[]byte(`{"schema_version":999,"kind":"invariant","id":"INV-0001"}`),0644); err != nil {t.Fatal(err)}
    after := breakerComplete(t,root,server,[]string{})
    status := callTool(t,server,"breaker",mcp.ToolStatus,map[string]any{})
    t.Logf("BEFORE=%+v MALFORMED=%+v STATUS=%+v",before,after,status)
    cmd := cli.NewRoot()
    var buf bytes.Buffer
    cmd.SetOut(&buf)
    cmd.SetErr(&buf)
    t.Chdir(root)
    cmd.SetArgs([]string{"verify","--staged","--json"})
    err := cmd.ExecuteContext(t.Context())
    t.Logf("CLI_ERROR=%v CLI=%s",err,buf.String())
    fresh,freshErr := mcp.New(t.Context(),root)
    if freshErr != nil { t.Logf("RESTART_ERROR=%v",freshErr) } else {
        defer fresh.Close(t.Context())
        t.Logf("RESTART_COMPLETE=%+v RESTART_STATUS=%+v",breakerComplete(t,root,fresh,[]string{}),callTool(t,fresh,"breaker",mcp.ToolStatus,map[string]any{}))
    }
    if after["allow"] == true {t.Error("UNKNOWN KNOWLEDGE SCHEMA ACCEPTED COMPLETION")}
}

func TestBreakerMixedProfile(t *testing.T) {
    root := newProfileRepo(t)
    path := filepath.Join(root,".mindrail","config.toml")
    data,err := os.ReadFile(path); if err != nil {t.Fatal(err)}
    data = []byte(strings.ReplaceAll(string(data),`[["echo", "hi"]]`,`[["true"], ["false"]]`))
    if err := os.WriteFile(path,data,0644); err != nil {t.Fatal(err)}
    server := newTestServer(t,root)
    ran := callTool(t,server,"breaker",mcp.ToolValidate,map[string]any{"profile":"test"})
    after := breakerComplete(t,root,server,[]string{"test"})
    t.Logf("VALIDATE=%+v COMPLETE=%+v",ran,after)
    if after["allow"] == true {t.Error("ONE FAILED PROFILE COMMAND ALLOWED COMPLETION")}
}

func TestBreakerForgetReconcile(t *testing.T) {
    root := newTestRepo(t)
    for _, args := range [][]string{{"add","."},{"-c","user.email=audit@example.invalid","-c","user.name=Audit","commit","-qm","baseline"}} {
        cmd := exec.Command("git",append([]string{"-C",root},args...)...)
        if out,err := cmd.CombinedOutput(); err != nil {t.Fatalf("%v: %s",err,out)}
    }
    server := newTestServer(t,root)
    coord, db := coordinationStore(t,root)
    workspaceID,projectID := workspaceOf(t,db)
    taskID,_ := openTaskAndSession(t,coord,workspaceID,projectID)
    args := map[string]any{"task_id":taskID}
    before := callTool(t,server,"breaker",mcp.ToolComplete,args)
    if err := os.WriteFile(filepath.Join(root,"pkg","a.py"),[]byte("def helper():\n    return 2\n"),0644); err != nil {t.Fatal(err)}
    skipped := callTool(t,server,"breaker",mcp.ToolComplete,args)
    reconciled := callTool(t,server,"breaker",mcp.ToolReconcile,args)
    after := callTool(t,server,"breaker",mcp.ToolComplete,args)
    t.Logf("BEFORE=%+v SKIPPED=%+v RECONCILE=%+v AFTER=%+v",before,skipped,reconciled,after)
    if skipped["allow"] == true {t.Error("UNRECONCILED SOURCE CHANGE ALLOWED COMPLETION")}
}

```

## Review Summary

| Severity | Count | Status |
|----------|-------|--------|
| CRITICAL | 0 | pass |
| HIGH | 4 | warn |
| MEDIUM | 0 | info |
| LOW | 0 | note |

Verdict: WARNING — 4 HIGH bulgu giderilmeden “her şey sıkıntısız çalışıyor” sonucu verilemez. Dar ölçülmüş statü filtresi tam çözüm değildir. Ürün kaynaklarına düzeltme uygulanmadı; bütün adversarial değişiklikler /tmp kopyasında kaldı.

## Reconciliation — Reader sonuçlarına karşı kanıt turu

Bu bölüm bağımsız rapor tamamlandıktan sonra eklendi. Reader F-01…F-05 okundu; aynı şeyi söyleyen iki yorumu “oy birliği” saymak yerine gerekçeler ayrı ayrı sınandı. Yeni üretim düzeltmesi yoktur.

| Reader maddesi | Breaker değerlendirmesi | Nihai kanıt sınırı |
|---|---|---|
| F-01 canonical reconcile | B-04 ile doğrulandı; Reader'ın ikinci örneğinin zorunlu DENY yorumu daraltıldı | Kayıtsız edit: complete ALLOW/0, aynı edit + reconcile DENY/2. Kayıtlı/sahipli aynı dosyadaki yeni edit ise aşağıdaki temiz fixture'da reconcile sonrası da ALLOW/0; salt “edit oldu” DENY nedeni değildir. |
| F-02 caller-controlled required | Davranış doğrulandı; üst sözleşme boşluğu | D-213 explicitly empty listeyi kabul eder. Bu frozen implementation'a aykırı bir kod kazası değildir; bağımsız minimum kanıt seçme vaadi karşılanmıyor. B-01'den ayrıdır: B-01 required açık verilmişken bile failed evidence'ı kabul eder. |
| F-03 production MCP yok | Launcher yokluğu doğrulandı; “byte framing test edilmedi” gerekçesi çürütüldü | 13 internal handler JSON byte akışı üstünde testlidir, ancak binary `mcp` komutunu sunmaz. D-191 bunu bilinçli ertelemiştir; üst kullanılabilir MCP ürün kapsamıyla çatışır. |
| F-04 canlı bütçe entegrasyonu | Eksik bağlantının varlık/yokluk iddiası doğrulandı | Production NewBudget/WithBudget çağrısı 0; production Analyze çağrısı 0. Deferral zero/generous iki fixture testi PASS. Gerçek latency ihlali üretilmiş gibi raporlanamaz; canlı operasyonun timeout/pending vaadi uygulanmamıştır. |
| F-05 staged/CI evidence | Eksik entegrasyon doğrulandı; F-02 ile ortak policy açığı altında birleştirilmeli | Verify compose Coverage atamaz; D-226 açıkça required-set empty seçer. Config'te bir profil bulunması onu “required” yapmaz; mevcut olmayan policy eşlemesini varsayan canlı CI fail-open örneği iddia edilmiyor. |

### F-01 için gerçekten çürütülen yardımcı iddia

Reader'ın ikinci kolu task/baseline/after_change sonrasında aynı dosyayı tekrar düzenleyip ALLOW gözlüyor. Bu, discovery çağrısının eksikliğini kod üzerinden gösterse de o editin zorunlu DENY alacağını tek başına kanıtlamaz: dosya hâlâ aynı task'ın declared scope'undadır; required evidence ve başka blocking kısıt yoktur.

İlk ek koşuda Reader fixture'ının committed baseline'ı olmadığından explicit reconcile ayrıca untracked `pkg/` dizinini buldu ve SCOPE_DRIFT üretti. Bunu ikinci editin hatası saymak yanlış olurdu. Deneyi başlangıç dosyalarını Git'e commit ederek tekrarladım:

| Kol | allow | denials | Reconcile finding |
|---|---:|---:|---:|
| İlk kayıtlı edit | true | 0 | — |
| Aynı scope içinde ikinci edit, reconciliation atlandı | true | 0 | — |
| Aynı ikinci edit, explicit reconcile tamamlandı | true | 0 | 0 |

Verbatim: `RECONCILE=map[... findings:[] pending:false ...] AFTER=map[allow:true denials:[]]`.

Bu yardımcı örnekte “ikinci edit mutlaka UNREGISTERED_CHANGE/SCOPE_DRIFT almalıydı” iddiası **çürütüldü**. B-04'ün temiz commit + hiç kayıtlanmamış değişiklik kontrol/senaryosu ise hâlâ doğrulanmış bulgudur; ona dokunulmadı.

Komut: `GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp -run '^TestBreakerRefuteRegisteredEditExpectation$' -count=1 -v`.

Yukarıdaki tekrar üretim dosyasına bu fonksiyon eklenebilir:

```go
func TestBreakerRefuteRegisteredEditExpectation(t *testing.T) {
    root := newTestRepo(t)
    for _, args := range [][]string{{"add","."},{"-c","user.email=audit@example.invalid","-c","user.name=Audit","commit","-qm","baseline"}} {
        cmd := exec.Command("git",append([]string{"-C",root},args...)...)
        if out,err := cmd.CombinedOutput(); err != nil {t.Fatalf("%v: %s",err,out)}
    }
    server := newTestServer(t, root)
    coord,db := coordinationStore(t,root)
    workspaceID,projectID := workspaceOf(t,db)
    taskID,session := openTaskAndSession(t,coord,workspaceID,projectID)
    path := filepath.Join(root,"pkg","a.py")
    callTool(t,server,"breaker",mcp.ToolClaim,map[string]any{"task_id":taskID,"session":session})
    callTool(t,server,"breaker",mcp.ToolBeforeChange,map[string]any{"task_id":taskID,"paths":[]string{path}})
    if err := os.WriteFile(path,[]byte("def helper():\n    return 2\n"),0644); err != nil {t.Fatal(err)}
    callTool(t,server,"breaker",mcp.ToolAfterChange,map[string]any{"task_id":taskID})
    before := callTool(t,server,"breaker",mcp.ToolComplete,map[string]any{"task_id":taskID})
    if err := os.WriteFile(path,[]byte("def helper():\n    return 3\n"),0644); err != nil {t.Fatal(err)}
    skipped := callTool(t,server,"breaker",mcp.ToolComplete,map[string]any{"task_id":taskID})
    reconciled := callTool(t,server,"breaker",mcp.ToolReconcile,map[string]any{"task_id":taskID})
    after := callTool(t,server,"breaker",mcp.ToolComplete,map[string]any{"task_id":taskID})
    t.Logf("REGISTERED=%+v SKIPPED=%+v RECONCILE=%+v AFTER=%+v",before,skipped,reconciled,after)
}

```

### F-03 framing ayrımı: kesin karşı kanıt

Kurulu SDK kaynağı: `/home/heisenberg/go/1.27.0/pkg/mod/github.com/modelcontextprotocol/go-sdk@v1.8.0/mcp/transport.go`.

- Satır 183–185: `NewInMemoryTransports` → `net.Pipe()`.
- Satır 173–174: `InMemoryTransport.Connect` → `newIOConn(t.rwc)`.
- Satır 163'ten başlayan yorum newline-delimited JSON kullanımını açıkça söyler.
- Satır 136–137: `StdioTransport.Connect` → aynı IO connection ailesi, fakat `os.Stdin/os.Stdout` üstünde.

Dolayısıyla codec/framing test edilmiştir. OS stdin/stdout, gerçek executable lifecycle'ı ve process boundary aynı şey değildir; bunlar mevcut launcher olmadığından test edilmemiştir.

Şu odaklı koşu bu turda PASS:

```sh
GOCACHE=/tmp/mindrail-breaker-tZ4Oqy-gocache go test ./internal/mcp ./internal/impact \
  -run '^(TestStdioDiscoversThirteenTools|TestDeferralZeroBudgetPartial|TestDeferralGenerousBudgetCompletes)$' \
  -count=1 -v
```

Ölçülen: tools discovery/smoke testi 1 PASS; budget fixture testleri 2 PASS. Ardından scratch binary ayrı derlendi:

```text
go build -o /tmp/mindrail-breaker-tZ4Oqy/mindrail ./cmd/mindrail
mindrail version --json → mcp_compatibility:"none"
mindrail mcp --json → exit 2, COMMAND_LINE_INVALID: unknown command "mcp"
```

Böylece F-03'ün doğru biçimi: “Internal MCP tool/JSON framing testi var; kullanıcı binary'sinde production stdio launcher yok.” “Hiç byte stream sınanmamış” biçimi kullanılmamalı.

### Birleştirme ve şiddet önerisi

B-01…B-04: dört ayrı, tekrar üretilmiş HIGH ürün davranış kusuru olarak tutulmalı; F-01, B-04'le mükerrerdir. Yerel mühendislik kapısındaki correctness kusurlarını uzaktan auth/RCE güvenlik açığıymış gibi CRITICAL etiketlememek daha ölçülüdür.

F-02 ve F-05 tek HIGH “bağımsız required evidence policy/CI entegrasyonu eksik” kapsam boşluğunda birleştirilebilir; iki ayrı fail-open sayısına şişirilmemeli. Frozen D-213/D-226 bunları bilinçli daraltmıştır; bu daraltma üst kernel vaatlerini kendiliğinden yerine getirmiş sayılmaz.

F-03 ayrıca HIGH kullanılabilir ürün arayüzü boşluğudur. F-04 bu turda **MEDIUM entegrasyon/acceptance gap** olarak sunulmalı; timeout aşımı, kuyruk kaybı veya SLO ihlali canlı ölçülmüş gibi yazılmamalı. Eksik bağlantı kod ve milestone kaydıyla kesindir; kusurlu runtime latency sonucu henüz yoktur.

Böyle bir konsolidasyonda 6 HIGH grup (4 davranış + policy/CI + launcher), 1 MEDIUM bütçe entegrasyonu, isteğe bağlı F-06 LOW dokümantasyon çelişkisi çıkar. Bu öneri Reader'ın bağımsız raporunu sessizce değiştirmez; konsolide rapor kaynak ve kanıt sınıfını açık tutmalıdır.

Tüm remedy'ler **öneri**dir. Yalnız B-01'in dar status filtresi scratch'te ölçüldü ve karma profili düzeltmediği kanıtlandı; hiçbir tam çözüm uygulanmadı veya doğrulandı diye sunulmamalıdır.

## Reconciliation Review Summary

| Severity | Bağımsız Breaker davranış bulgusu | Birleştirme önerisi |
|----------|-------------------------------:|--------------------|
| CRITICAL | 0 | 0 |
| HIGH | 4 | 6 grup, eksik policy/CI ve launcher dahil |
| MEDIUM | 0 | 1 canlı budget entegrasyonu boşluğu |
| LOW | 0 | Dokümantasyon drift'i varsa ayrı 1 |

Verdict: WARNING — mevcut kanıt “sıkıntısız 0.1” sonucunu desteklemiyor; JSON framing ve sahipli ikinci edit hakkında aşırı iddialar karşı kanıtla daraltıldı.

**Release kabul kararı: REJECT / NON_COMPLIANT.** Dört doğrulanmış HIGH davranış kusuru kernel'in bağımsız kanıt kapısı iddiasını bozuyor. Buradaki WARNING kod-review şiddet tablosunun HIGH-only etiketidir; release onayı değildir. Policy/CI ve launcher için iki ek HIGH kapsam boşluğu; bütçe entegrasyonu için bir MEDIUM kabul boşluğu önerilir. Gerçek timeout/SLO ihlali bu turda ölçülmemiştir.

## Son sınırlı public CLI deneyi — B-04'ün ek kapanış yüzeyi

Ana denetçinin isteğiyle gerçek binary üzerinde `task state` yolunu da çalıştırdım. Deneme repo: `/tmp/mindrail-public-complete-I30hHN`; binary: `/tmp/mindrail-breaker-tZ4Oqy/mindrail`. Bu bir MCP handler testi değildir: her komut ayrı OS process'idir.

Başlangıç Git commit'i iki dosya içerir: `pyproject.toml` (`[project]` ve `name = "public-completion-audit"`), `a.py` (`def helper():\n    return 1\n`). `mindrail init` ve `session open` sonrası aynı session altında iki ayrı task açıldı. Kontrol task'ı temiz source ile, senaryo task'ı `return 1` → `return 2` değişikliğiyle kapatıldı. Senaryoda before_change, after_change, reconcile veya validate çağrılmadı.

| Ölçüm | Temiz kontrol | Source edit, discovery/evidence yok |
|---|---|---|
| `OPEN→CLAIMED→IN_PROGRESS→READY_TO_COMPLETE→COMPLETED` | tüm komutlar exit 0 | tüm komutlar exit 0 |
| Kalıcı state / revision | COMPLETED / 5 | COMPLETED / 5 |
| Terminal geçiş sonrası lease | released | released |
| Verification öncesi DB `changes` / `evidence` | 0 / 0 | 0 / 0 |

İki task kapandıktan sonra bağımsız SQLite read çıktısı:

```text
SELECT state,revision FROM tasks ORDER BY created_at;
COMPLETED|5
COMPLETED|5
SELECT count(*) FROM changes;
0
SELECT count(*) FROM evidence;
0
```

Ardından aynı source edit `git add a.py` ile stage edildi. Gerçek `verify --staged --json` **exit 1**, `allow:false`, **1 adet UNREGISTERED_CHANGE** üretti. Sonraki ayrı `task show` hâlâ `COMPLETED`, revision 5 gösterdi. `task state --to IN_PROGRESS` geri dönüşü exit 1 `TASK_STATE_INVALID` ile reddedildi: “COMPLETED, which is final; open a new task instead.”

Gerçek kullanılan çağrı biçimi (SID/TID önceki JSON cevaplarından alınır):

```sh
mindrail -C "$audit_repo" init --json
mindrail -C "$audit_repo" session open --json
mindrail -C "$audit_repo" task open --session "$audit_sid" --title unreconciled-scenario --json
mindrail -C "$audit_repo" task state "$audit_tid" --session "$audit_sid" --to CLAIMED --json
mindrail -C "$audit_repo" task state "$audit_tid" --session "$audit_sid" --to IN_PROGRESS --json
# Bu noktada a.py içindeki return 1, return 2 yapılır; başka protokol çağrısı yoktur.
mindrail -C "$audit_repo" task state "$audit_tid" --session "$audit_sid" --to READY_TO_COMPLETE --json
mindrail -C "$audit_repo" task state "$audit_tid" --session "$audit_sid" --to COMPLETED --json
mindrail -C "$audit_repo" task show "$audit_tid" --json
sqlite3 "$audit_repo/.git/mindrail/mindrail.db" 'SELECT state,revision FROM tasks; SELECT count(*) FROM changes; SELECT count(*) FROM evidence;'
git -C "$audit_repo" add a.py
mindrail -C "$audit_repo" verify --staged --json
mindrail -C "$audit_repo" task show "$audit_tid" --json
mindrail -C "$audit_repo" task state "$audit_tid" --session "$audit_sid" --to IN_PROGRESS --json
```

**Doğru yorum:** CLI coordination task durumunu gate'i çağırmadan terminal COMPLETED yapabilir; lease bırakılır ve bu durum süreçler arasında kalıcıdır. Bu komut **mühendislik ALLOW kararı üretmez**. `verify --staged` bağımsız olarak aynı değişikliği reddeder; bu deney commit/CI kapısının bypass edildiğini iddia etmez. Task listesinde “completed” görmek doğrulama kanıtı değildir.

Kaynak: `internal/cli/task.go:119` doğrudan `TransitionExpecting` çağırır. `internal/coordination/store.go:362` yorum/kod sınırı revision, lease ve lifecycle tablosudur; gate kompozisyonu yoktur. Frozen MR-003 D-55 `READY_TO_COMPLETE→COMPLETED` geçişini açıkça tanımlar. Daha sonraki MR-013 D-179 completion flow için reconcile-first vaat eder, fakat bu mevcut CLI transition yoluna bağlanmamıştır. Dolayısıyla MR-003'ün dar coordination sözleşmesi çalışıyor; ürünün “task completion mühendislik kapısından geçer” şeklinde okunması karşılanmıyor.

**Konsolidasyon önerisi:** HIGH completion-enforcement entegrasyon grubuna (B-04) ek public yüzey/etki olarak alın; aynı ürün açığını ayrı sayıyla şişirmeyin. Kullanım belgesinde “task tamamlamak için CLI yok” yazılmamalı. Doğru ifade: “`task state ... --to COMPLETED` coordination durumunu kapatır; kaynak/kanıt doğrulayan completion CLI komutu yoktur ve bu geçiş gate onayı sayılmaz.”

Önerilen remedy, terminal geçiş öncesi ortak gate değerlendirmesi veya açıkça ayrılmış doğrulanmamış durum semantiğidir; **uygulanmadı/ölçülmedi**. Release kararı **REJECT / NON_COMPLIANT** olarak kalır.

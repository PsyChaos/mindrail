# Breaker delta denetimi — tur 2

**Karar: NOT_VERIFIED.** Önceki BR-02…05 ve RDR-001…003 deneyleri artık beklenen
sonucu veriyor. BR-01'in özgün örneği de düzeldi; ancak aynı glob-kapsam sınıfının
tek bir kardeş örneğinde kapsam dışı erişim hatası kanıtı hâlâ bayatlatıyor.
Kalan bulgu **MEDIUM**, yeni HIGH/CRITICAL yok.

## Bağımsızlık ve ortam

Bu tur yalnız önceki bulguların düzeltme deltasını denetledi; değişmemiş kapsamın
tam denetimi tekrarlanmadı. Reader'ın ilk tur raporu, RDR-001…003 girdilerini
almak için okundu; **Reader tur-2 sonuçları bu rapor yazılmadan okunmadı**.

Güncel ortak ağaçtan yeni kopya `/tmp/mindrail-breaker-round2-Mmj4ME` oluşturuldu
(yalnız scratch snapshot commit'i `4543abf`; gerçek repoda commit yok). Önceki
turun üretim kaynakları kullanılmadı. Kontrol/scenario test kaynakları aynı
girdileri korumak için yeni kopyaya bağımsız test adıyla taşındı. Mutasyonlar yeni
snapshot'ın `-cas` ve `-codec` kopyalarında yapıldı.

Ortak üretim kaynaklarına yazılmadı. Denetim sonunda scratch ile ortak ağacın
`internal/`, `cmd/`, `migrations/` dosyalarının `rsync -rcni` karşılaştırması,
yalnız ek `round2*` probe dosyaları hariç, **0 fark satırı** verdi.

## Önceki bulguların tekrar ölçümü

| Bulgu / deney | Kontrol | Tur-2 scenario | Sonuç |
| --- | --- | --- | --- |
| BR-01 özgün erişim örneği: `tests/**/*.py`, `outside/private` mode 000 | current=1 | current=1 | Özgün örnek kapandı |
| BR-01 sıfır eşleşme: boş literal dizin ve `empty/**/*.py`, komut `true` | Bir dosyada exit=0/current=1 | Sıfır dosyada exit=0/current=1, provenance `scope:[]` | Kapandı |
| BR-02 yarım operation tekrarında iki `true` komutu | PASS satırı=2/current=1 | PASS satırı=2/current=1 | Kapandı |
| BR-03 / RDR-001 aynı operation ve aynı required replay | exit=0/replayed=true | Değişmiş required: exit=1, `OPERATION_ID_CONFLICT`, command=`task complete` | Kapandı |
| BR-04 var olan `İ` profili: validation/MCP/CLI | 1 / 1 / 1 | Boş required: 0 / 0 / 0 | Kapandı |
| BR-04 ad normalizasyonu | Gerçek `İ` kanıtı kabul | `i`, `ı`, baş/son boşluk, NBSP, NFD `próof`: üç okuyucuda da 0 | Ayrışma yok |
| BR-05A evaluation sırasında revision artışı | 4→5, exit=0, COMPLETED | Trigger ile 4→5, exit=1, READY_TO_COMPLETE | Aday koruması doğru |
| BR-05B doğrudan saklanan minimum mapping | `[]`: kabul=1, mapping=0 | Dört alanı boş canonical mapping: kabul=0, mapping=0 | Aday koruması doğru |
| RDR-002 canlı `task` ve `task state` help | Önceki iki yanlış sahiplik iddiası | Yanlış iddia sayısı=0; non-terminal/state ve evaluated COMPLETED/complete ayrımı açık | Kapandı |
| RDR-003 gerçek binary `mcp --json` | Normal SDK subprocess: 13 araç ve bootstrap başarılı | exit=2, stdout=0 byte, stderr=83 byte, `COMMAND_LINE_INVALID` | Kapandı |

Boş scope sonrasında üyelik eklenmesi ile bayatlama,
`TestZeroMatchDeclaredScopeIsCurrentUntilMembershipChanges` üzerinden ayrıca
çalıştırıldı. Shipped `RunProfileRejectsReplayedProvenanceMismatch` kontrolleri,
recovery düzeltmesinin uyumsuz provenance'i kabul etmediğini denetledi.

Bağımsız kontroller:

```sh
GOCACHE=/tmp/mindrail-audit-gocache go test -count=1 -v \
  ./internal/validation ./internal/completion ./internal/cli ./internal/mcp \
  -run '^TestIndependentR2'
```

İlk çalışmada validation, completion, CLI kolları PASS; MCP test binary link'i
`No space left on device` yüzünden kurulamadı. Bu sonuç ürün kusuru veya negatif
test kanıtı sayılmadı. Etkilenen MCP parity kontrolü tek başına yeniden çalıştı:
`go test -count=1 -v ./internal/mcp -run '^TestIndependentR2SameProfileReaders$'`
**PASS, 3.067 s; sekiz kolun tamamı**.

İlgili yeni shipped testlerin seçimi validation 0.451 s, changes 0.187 s,
CLI 2.042 s ve cmd/mindrail 4.359 s ile PASS. Aynı regex'in sıfır test seçtiği
paketler coverage kanıtı sayılmadı. Native binary ayrıca yeni snapshot'tan
`go build -o mindrail-r2 ./cmd/mindrail` ile derlenip üç help sayfası ve
`mcp --json` doğrudan çalıştırıldı.

## İki önceki survivor artık öldürülüyor

Mutasyon kopyaları **bağımsız ek testler eklenmeden** alındı; aşağıdaki başarısızlıklar
uygulamaya eklenmiş kalıcı regression testlerinden geliyor.

| Semantic mutasyon | Önceki tur | Tur 2 | Ayırt edici üretilen değer |
| --- | --- | --- | --- |
| İki completion çağrısında son CAS expected revision'ı `0` yapmak | Shipped CLI suite PASS | **KILLED** — `TestTaskCompleteFinalCASRejectsRevisionChangedDuringGate` | Mutant exit=0, COMPLETED revision=6; test exit=1 bekliyor |
| Durable mapping'in boş zorunlu alan kontrolünü kaldırmak | Shipped changes/completion/MCP PASS | **KILLED** — `TestGuardBaselineRejectsIncompletePersistedMappings` | `path`, `test`, `production_uid`, `invariant_id` için dört ayrı canonical girdi kabul ediliyor; test ret bekliyor |

CAS mutasyonu, eski `TransitionExpecting` yerine yeni completion API'sinin iki
çağrısında `CompleteExpecting(..., expect, required)` →
`CompleteExpecting(..., 0, required)` yapıldı. Böylece aynı semantic koruma
kaldırıldı; idempotency hash uyuşmazlığının yanlışlıkla mutantı öldürmesi önlendi.

İlk paralel mutant derlemelerinde de disk doluluğu görüldü; bunlar kill sayılmadı.
CAS testi tek başına yeniden çalıştırıldığında 0.407 s içinde yukarıdaki gerçek
yanlış COMPLETED sonucuyla kırıldı. Codec paketleri `go test -p 1 -count=1
./internal/changes ./internal/completion ./internal/mcp` ile sırayla yeniden
çalıştırıldı; dört minimum-field input'u kalıcı changes testini kırıyor.

## Kalan BR-01 kardeşi — MEDIUM, güven %100

**REQ-002. Dosya: `internal/validation/snapshot.go:73`.**
Yeni `globWalkRoot` literal öneğin dışındaki dizinleri taramayı önlüyor, fakat
`WalkDir(walkRoot)` altında glob'un hiç seçemeyeceği alt dizinlere inmeye devam
ediyor. `tests/*.py`, `tests/private/deep.txt` dosyasını seçemez; buna rağmen
`tests/private` okunamazsa mevcut kanıt stale oluyor.

| Ölçüm | Kontrol | Scenario | Fark |
| --- | ---: | ---: | ---: |
| Glob'a uyan dosya sayısı | 1 | 1 | 0 |
| Glob'a uyan dosyanın içerik değişikliği | 0 | 0 | 0 |
| `tests/private` mode | 0755 | 000 | Tek değişiklik |
| Required satisfied | 1 | **0** | -1 |
| Sebep | scope re-hashes equal | declared scope unreadable | Yanlış kapsam etkisi |

Deney: `TestIndependentR2GlobUnrelatedSubtreeBelowLiteralPrefix`; exit=1,
validation paket süresi 0.011 s. Minimal yeniden üretim (mevcut validation test
yardımcılarıyla ayrı `_test.go` dosyasına eklenebilir):

```go
func TestGlobUnrelatedSubtreeBelowPrefix(t *testing.T) {
    root := t.TempDir()
    writeScopeFile(t, root, "tests/a.py", "assert True\n")
    writeScopeFile(t, root, "tests/private/deep.txt", "outside glob\n")
    row := evidenceRow(t, "EVD-1", "proof", root, []string{"tests/*.py"})
    _, before, err := validation.Check(root, []validation.Evidence{row}, []string{"proof"})
    if err != nil { t.Fatal(err) }
    private := filepath.Join(root, "tests", "private")
    if err := os.Chmod(private, 0); err != nil { t.Fatal(err) }
    t.Cleanup(func() { _ = os.Chmod(private, 0755) })
    verdicts, after, err := validation.Check(root, []validation.Evidence{row}, []string{"proof"})
    if err != nil { t.Fatal(err) }
    t.Logf("control=%v scenario=%v reason=%s",
        before.Satisfied["proof"], after.Satisfied["proof"], verdicts[0].Reason)
    if !after.Satisfied["proof"] { t.Fatal("out-of-scope subtree staled evidence") }
}
```

Öneri: glob'a göre hiçbir alt dosyası eşleşemeyecek dizine girilmeden
`filepath.SkipDir` kullanmak; örneğin `tests/*.py` için nested dizinleri gezmemek.
Potansiyel eşleşme içeren unreadable dizinlerin reddi korunmalı. Yeni bir üretim
düzeltmesi uygulanmadı ve öneri uygulanmış çözüm gibi sunulmuyor; kalan kusurun
ölçümünü değiştirmeden ana ajana teslim edildi.

Sınıf taraması yalnız bu fix'in `globWalkRoot` → `WalkDir` → `matchScopeGlob`
zinciri ile ilgili mevcut boundary testlerine daraltıldı. Mevcut testler literal
öneğin dışındaki private subtree ve **potansiyel eşleşen** unreadable subtree'yi
kapsıyor; öneğin altındaki **eşleşemeyen** subtree'yi kapsamıyor. Yeni alan,
varsayımsal saldırı veya önceki denetimin tekrarı açılmadı.

## Kapanış

BR-02, BR-03/RDR-001, BR-04, BR-05, RDR-002 ve RDR-003 ölçülmüş kontrol/scenario
çiftleriyle kapandı. BR-01'in sıfır-match ve özgün dış-dizin örnekleri kapandı;
aynı sınıfta üretilen yukarıdaki tek MEDIUM sibling açık kaldı.

**Delta verdict: NOT_VERIFIED.** Yeni HIGH/CRITICAL yok; doğrulanmış tek kalan
eksik, glob literal öneği altında eşleşemeyen unreadable dizinin kapsamı bozması.
Üç yeni tur-2 scratch kopyası ve bu denetçinin `/tmp/mindrail-audit-gocache`
önbelleği rapor sonrası, ana ajanın talimatıyla temizlendi. Bunlar yeniden
üretilebilir geçici test/derleme çıktılarıydı; gerekli ölçümler ve kalan kusurun
repro kaynağı bu raporda korundu. Üretim dosyaları ve eski tur kanıtları korunuyor.

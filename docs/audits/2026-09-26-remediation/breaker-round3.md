# Breaker delta denetimi — tur 3

**Karar: VERIFIED.** Bu turdaki iki önceki MEDIUM konu kapandı: eşleşemeyen
unreadable alt dizinin glob kanıtını bozması ve raw COMPLETED reddinin task
durumuna uygun olmayan ilk next_action vermesi. Kontrol/scenario çiftleri,
kalıcı testi öldüren pruning mutasyonu ve gerçek binary ile yedi durum taraması
beklenen sonuçları üretti. Bu karar yalnız bu iki düzeltmenin deltasına aittir;
önceki tam denetim veya tüm release kontrollerinin tekrarı olduğu iddia edilmez.

## Bağımsızlık ve izolasyon

Güncel ortak ağaçtan yeni `/tmp/mindrail-breaker-round3-xnZgop/repo` snapshot'ı
alındı; eski turların üretim kodu kullanılmadı. Yalnız scratch içindeki snapshot
commit'i `c6d59f4`; ortak repoda commit oluşturulmadı. Mutasyon ayrı `mutant/`
kopyasında, derleme cache'i aynı özel scratch kökü altındaki `cache/` dizininde.
Reader sonuçları rapor yazılmadan okunmadı.

Ortak üretim kaynaklarına dokunulmadı. Sonunda scratch `internal/` ve `cmd/`
dosyaları ortak ağaçla checksum tabanlı `rsync -rcni` karşılaştırıldı; sadece ek
`round3*` testleri hariç tutuldu; fark çıktısı **0 satır**.

## 1. Glob sınırı: aynı veri, tek izin değişikliği

Her kol yeni bir disposable dizinde şu dosyaları oluşturdu:
`tests/a.py`, `tests/unit/b.py`, `tests/private/deep.txt`. Declaration'a bağlı
PASS evidence oluşturuldu ve önce `validation.Check` ile current kontrolü
yapıldı. Sonra yalnız tabloda belirtilen path mode'u `000` yapıldı. Test sonunda
mode geri kondu. Scope'a uygun dosyaların içeriği hiç değişmedi.

| Pattern | Mode 000 yapılan path | Kontrol current | Scenario current | Beklenti |
| --- | --- | ---: | ---: | --- |
| `tests/*.py` | `tests/private` | 1 | **1** | Eşleşemeyen alt dizin kapsam dışı |
| `tests/unit*/*.py` | `tests/private` | 1 | **1** | Segment'e uymayan kardeş kapsam dışı |
| `tests/*/*.py` | `tests/private` | 1 | **0** | İçinde eşleşme bulunabilecek dizin okunamıyor |
| `tests/**/*.py` | `tests/private` | 1 | **0** | Recursive declaration için potansiyel eşleşme |
| `*/**/*.py` | `tests/private` | 1 | **0** | Wildcard-first recursive declaration |
| `tests/*.py` | `tests/a.py` | 1 | **0** | Seçilen dosya okunamıyor |

Altı kolun tamamı **PASS**. Önceki tam repro olan `tests/*.py` +
`tests/private/deep.txt` artık current=1 sonucunu koruyor; gerçekten okunması
gereken yerlerde fail-closed davranış korunuyor.

Bağımsız deney: `TestIndependentR3UnreadableScopeArms`. Ayrıca kalıcı
`TestGlobIgnoresUnmatchedUnreadableSiblingBelowPrefix`,
`TestSnapshotScopeUnreadableGlobBoundaries` ve
`TestSnapshotScopeWildcardSegmentsStayAtTheirLevel` çalıştırıldı. Sonuncusu
tek-segment, iki-segment, recursive ve wildcard-first seçimlerinin beklenen
dosya listelerini karşılaştırır.

## 2. Pruning guard mutasyonu

Yeni snapshot'ın bağımsız kopyasında `internal/validation/snapshot.go` içindeki
segment guard'ı tek başına etkisizleştirildi:

```go
// Aday
if !matched || entry.Type()&fs.ModeSymlink != 0 { continue }
// Mutant: eşleşmeyen segmenti eleme kaldırıldı, symlink kuralı korundu
if false && !matched || entry.Type()&fs.ModeSymlink != 0 { continue }
```

Bağımsız ek testler mutant kopyasına taşınmadı. Kalıcı test komutu:

```sh
GOCACHE=<scratch>/cache go test -count=1 -v ./internal/validation \
  -run 'Test(SnapshotScopeUnreadableGlobBoundaries|SnapshotScopeSelectionBoundaries|CheckIgnoresUnreadableUnmatched)'
```

| Ölçüm | Aday | Mutant |
| --- | ---: | ---: |
| `unmatched_directory_is_pruned` PASS | 1 | **0** |
| Paket exit code | 0 | **1** |
| Potansiyel eşleşen unreadable dizin/file/recursive negatif kolları PASS | 3 | 3 |

**KILLED, 0.013 s.** Belirleyici çıktı:
`snapshot_test.go:217: COMMAND_LINE_INVALID: snapshot scope unreadable: tests/unit*/*.py`.
Derleme hatası veya başka koşulun bozduğu test değil; doğrudan segment prune
korumasının regression testi kırıldı.

## 3. Gerçek binary: raw COMPLETED ve ilk remedy, bütün domain durumları

Yeni snapshot'tan native executable üretildi:

```sh
GOCACHE=<scratch>/cache go build -o <scratch>/mindrail ./cmd/mindrail
```

`TestIndependentR3RealBinaryRawCompletionAllStates` durum listesini
`coordination.States()` üzerinden aldı; lifecycle'a ikinci sabit liste yazmadı.
Her durum için ayrı Git repo initialize/commit edildi; session/task açılışı,
duruma ulaşan bütün transition'lar, show/lease okumaları, raw ret ve remedy
çağrıları **gerçek derlenmiş binary subprocess'leriyle** yapıldı. COMPLETED
fixture'ı, hazır task'ın public `task complete` ile tamamlanmasıyla kuruldu.

Her kolda sırayla:

1. `task show --json` ve `lease list --json` çıktıları saklandı.
2. `task state <id> --to COMPLETED --session <id> --json` çalıştırıldı.
3. Tekrar okunan task ve lease çıktıları byte-for-byte karşılaştırıldı.
4. İlk `next_action` içindeki gerçek komut çıkarıldı; task/session/revision
   placeholder'ları yerleştirildi ve bu komut aynen çalıştırıldı.
5. Terminal durumlarda first-action okumalarının da task/lease'i değiştirmediği
   tekrar kontrol edildi.

| Başlangıç durumu | Raw exit | Task/lease değişiklik sayısı | Çıkan ilk komutun özü | İlk komut exit | Son durum |
| --- | ---: | ---: | --- | ---: | --- |
| OPEN | 1 | 0 / 0 | `task state --to CLAIMED` | 0 | CLAIMED |
| CLAIMED | 1 | 0 / 0 | `task state --to IN_PROGRESS` | 0 | IN_PROGRESS |
| IN_PROGRESS | 1 | 0 / 0 | `task state --to READY_TO_COMPLETE` | 0 | READY_TO_COMPLETE |
| BLOCKED | 1 | 0 / 0 | `task state --to IN_PROGRESS` | 0 | IN_PROGRESS |
| READY_TO_COMPLETE | 1 | 0 / 0 | `task complete --expect-revision <current>` | 0 | COMPLETED |
| COMPLETED | 1 | 0 / 0 | `task show` | 0 | COMPLETED |
| ABANDONED | 1 | 0 / 0 | `task show` | 0 | ABANDONED |

Bütün raw retler `TASK_STATE_INVALID` verdi. READY dışındaki altı durumda
`task complete` önerisi sayısı **0**. İki terminal durumda `task state` veya
`task complete` önerisi sayısı **0**; yalnız inspection/new-task guidance var.
İlk remedy'nin başarı sayısı **7/7**. Bağımsız gerçek-binary testi **PASS,
3.29 s**; kalıcı `TestRawCompletionRefusalIsStateSpecific` de yedi kolda
**PASS, 1.69 s**.

## 4. D-55 ve daraltılmış sınıf kontrolü

`TestNoOtherPackageStatesTheLifecycle` **PASS** (0.01 s; coordination paketi
0.019 s). Test root'u gerçekten tarayan, yedi state literal'ının başka dosyada
birlikte tanımlanmasını arayan repository architecture kontrolüdür; yalnız bir
yorum kontrolü değildir. CLI'nin genel nonterminal önerileri
`coordination.TransitionsFrom` üzerinden üretiliyor; yeni bağımsız probe da
durumları ve hazırlık yollarını aynı domain API'sinden alıyor.

Sınıf taraması yalnız yeni glob segment traversal'ı, ilgili permanent scope
testleri ve raw completion refusal'ın domain-state caller'larıyla sınırlı kaldı.
Yeni alanlara genişleme yapılmadı; başka bir kusur lead'i üretilmedi.

Ana kontrol komutu:

```sh
GOCACHE=<scratch>/cache go test -count=1 -v \
  ./internal/validation ./internal/cli ./internal/coordination \
  -run 'Test(IndependentR3|NoOtherPackageStatesTheLifecycle|SnapshotScopeUnreadableGlobBoundaries|RawCompletionRefusalIsStateSpecific)'
```

**PASS.** CLI paketi 4.996 s, coordination 0.019 s. Ek kalıcı glob kontrolleri
ayrıca PASS. Environmental failure veya atlanmış blocker yok.

## Delta verdict

**VERIFIED — bu turun iki önceki MEDIUM konusu ölçülmüş olarak kapandı.**
Eşleşemeyen unreadable dizinler kapsamı bozmuyor; potansiyel eşleşen okunamayan
path'ler reddediliyor. Segment korumasının kaldırılması permanent testi
kırıyor. Raw COMPLETED bütün domain durumlarında state/lease'i koruyor ve
verdiği ilk next_action gerçekten uygulanabiliyor; terminal task guidance'ı
terminal kalıyor. Yeni açık bulgu yok.

Özel scratch repo, mutant, binary ve cache rapor sonrası temizlendi. Bunlar
yeniden üretilebilir geçici denetim çıktılarıydı; ölçümler ve komutlar bu raporda
korundu. Üretim kaynakları ve ortak kullanıcı verisi korunuyor.

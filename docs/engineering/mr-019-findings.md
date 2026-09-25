# MR-019 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-019-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `6c5742c` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-018 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `make bench`: 5 transport benchmark'ı (mcp) + 2 servis benchmark'ı (changes) + 1 traversal benchmark'ı (impact); hepsi yeşil. |
| AC-01.2 | Karşılandı | p95'ler hedeflerin çok altında: status ~1ms/150ms, context ~0.8ms/250ms, before ~1.2ms/300ms, after ~2ms/1.5s, reconcile ~6–12ms/2s. Aşım `b.Fatalf` ile hedefi düşürür (M5). |
| AC-01.3 | Karşılandı | Breakdown'lar servis benchmark'larında yayınlanır (`sqlite_wait`, `parse`); traversal impact benchmark'ında. Kablolama testleri: `TestAfterChangeObservesBreakdowns`, `TestAnalyzeObservesTraversal`, `TestObserveWithoutTimerIsSilent`. |
| AC-01.4 | Karşılandı | Duvar-saati yalnız benchmark'larda ve `perf.Span` gözleminde; unit testlerde injected clock (`manualClock`, `FixedClock`) + varlık-değil-değer denetimleri. |

**Tasarım notları:** telemetri ctx-plumbing ile çalışır — imza
değişikliği yok, nil-timer sessizce yutar (D-230). Kritik bulgu:
MCP teli (in-memory dahil) context value geçirmez — SDK sunucu
tarafında kendi ctx'iyle dispatch eder. Bu yüzden transport
benchmark'ları toplamları, servis benchmark'ları breakdown'ları ölçer;
`perf.Bench` doc'u bu sınırı açık yazar. `TestTimerReachesHandlers`
bu yüzden yazılmadı — transport-garantisi yok, servis-kablolama
testleri var.

**Ölçüm notları:** `benchRepo` standart `newTestRepo`'yu genişletir
(bootstrap init + 10 dosya); after_change/reconcile her iterasyonda
taze edit yer (sıcak ajan döngüsü + cache-dışı parse/write).
before_change baseline declare eder (yoksa keşif boş olurdu —
yakalanıp düzeltildi). Yardımcı imzalar `*testing.T` → `testing.TB`
oldu (dokunulan test dosyalarında davranış değişikliği yok).

### TASK-01 guard mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | Grade bilinmeyen op'u geçirir | `TestGradeFailsClosed` FAIL |
| M2 | percentile rank kayık (floor) | `TestPercentileNearestRank` FAIL |
| M2' | kapı notuyla ceil-rank'e geçildi; Floor-mutantı tekrar kırmızı | `TestPercentileNearestRank` FAIL |
| M3 | SyncFileSymbols fold'u düşürüldü | `TestAfterChangeObservesBreakdowns` FAIL |
| M4 | traversal span düşürüldü | `TestAnalyzeObservesTraversal` FAIL |
| M5 | Targets'tan status silindi | `BenchmarkStatus` FAIL (fail-closed grading) |

Her mutant sonrası dosyalar benzersiz-isimli backup'tan restore edilip
md5 ile doğrulandı (`mr019-mutant-m{1,3,4}-*.bak`).

`make check` **yeşil** (exit 0). Test sayısı **1300** (TASK-01 başında 1293).

### TASK-01 kapı — Reader: PASS; Breaker: PASS

İki kapı da PASS verdi; iki düşük not aynı turda kapatıldı:
vacuous wall-clock dalı (`TestTimerUsesAppClock` artık Total-varlığını
dener) ve floor-rank percentile (ceil nearest-rank + M2' kırmızı).
Taşınabilir bulgu: MCP teli context value geçirmez — toplamlar
transportta, breakdown'lar servis katmanında ölçülür.

**Karar:** TASK-01 KAPANDI.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Sıfır-bütçe ve öğe-bütçe testleri kısmi girdi + açık pending frontier döndürür (`TestDeferralZeroBudgetPartial`, `TestDeferralItemBudgetFrontier`); bloklama ve sessiz-skip yolu yok (Budget'ta bekleme ve düşürme temsil edilemez). |
| AC-02.2 | Karşılandı | Pending frontier unit'e çözümlenir, `Scheduler.Prioritize` ile kuyruk başına taşınır (kuyruk konumu denetlenir), cömert tekrar tam kapanır (`TestDeferralFrontierRequeuesPrioritized`). |
| AC-02.3 | Karşılandı | Sıfır-bütçe deterministik kısmi, cömert-bütçe tam (`TestDeferralGenerousBudgetCompletes`); öğe-bütçeleri sayımla kesin sınır çizer — duvar-saati yok. |
| AC-02.4 | Karşılandı | `Complete`/`Pending` sonuç şeklidir: kod ve göç yok (grepler TASK-04'te). |

**Tasarım notları:** bütçe ctx-plumbing ile taşınır (imza
değişikliği yok; nil bütçe sonsuza dek sürdürür — bütçesiz çağrılar
eskisi gibi davranır). Sonuç genişletmesi (`Complete`, `Pending`)
DeepEqual karşılaştırması olmadığı için mevcut testleri kırmaz.
Canlı-op kablolaması yok: `Analyze`'ın 0.1'de üretim çağrıcısı
bulunmaz; erteleme, FULL-traversal geldiğinde hazır mekanizmadır
(D-231 uygulaması, dürüst kayıt). Soğuk-indeks PARTIAL_READY yolu
(MR-005) canlı kısmi davranıştır, değişmedi.

**Fixture notu:** erteleme zinciri gerçek unit yolunda tohumlanır
(pending file-state unit-dışı yolu reddeder; paylaşılan
`seedReference` `/r/a.py` sabitler — `seedReferenceAt` ile
çözüldü).

### TASK-02 guard mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M6 | Budget hiç tükenmez | `TestDeferralZeroBudgetPartial` + `TestDeferralItemBudgetFrontier` FAIL |
| M7 | pending frontier düşürüldü | `TestDeferralItemBudgetFrontier` + `TestDeferralFrontierRequeuesPrioritized` FAIL |
| M8 | kısmi sonuç complete raporlar | `TestDeferralZeroBudgetPartial` FAIL |

### TASK-02 kapı — Reader: PASS; Breaker round-1: dar BLOCKED (S1/S2)

**Round-1 Reader:** AC-02.1…02.4 CONFIRMED + 2 düşük kapsam-notu
(aşağıda).
**Round-1 Breaker:** kaynak doğru, kanıt eksik — S1 (`next`'siz
Pending yaşaması) ve S2 (seviye-başına Yield yaşaması) mutantları
tüm suite'te yeşil kalıyordu. Çözüm: dallanan-sınır testi
(`[B, D]` + 1 öğe → pending `[D, C]`); S1/S2 kırmızı koşuldu.
Error-dönüşlerindeki `Complete=false` + boş-Pending için doc
düzeltildi ("on nil-error").

Kapsam-notları (engelsiz, dürüst kayıt): Yield tane-başınadır —
öğe-içi fan-in/fallback bütçesizdir (0.1 structural fan-in küçüktür);
kısmi dönüş breadth-yeniden-hesabını atlar (testlerin tamamı
`DirectOnly`).

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| S1 | Pending'den `next` düşürüldü | `TestDeferralBranchingFrontier` FAIL |
| S2 | Yield seviye-başına alındı | `TestDeferralBranchingFrontier` FAIL |

**Karar:** TASK-02 KAPANDI (re-gate tur 2 aşağıda).

### TASK-02 re-gate tur 2 — PASS

Dallanan-sınır pini zincir topolojisine karşı izlendi (B→C kenarı,
pending `[D, C]`), S1/S2 `/tmp` kopyasında koşularak kırmızı
doğrulandı, süit yeşil, doc-düzeltmesi doğru, diff 3 dosya.

**Karar:** TASK-02 KAPANDI.

Her mutant sonrası dosyalar benzersiz-isimli backup'tan restore edilip
md5 ile doğrulandı (`mr019-mutant-m{6,7}-*.bak`).

`make check` **yeşil** (exit 0). Test sayısı **1306** (TASK-02 başında 1300).

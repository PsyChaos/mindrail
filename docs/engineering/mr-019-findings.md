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
| M2 | percentile rank +1 kayık | `TestPercentileNearestRank` FAIL |
| M3 | SyncFileSymbols fold'u düşürüldü | `TestAfterChangeObservesBreakdowns` FAIL |
| M4 | traversal span düşürüldü | `TestAnalyzeObservesTraversal` FAIL |
| M5 | Targets'tan status silindi | `BenchmarkStatus` FAIL (fail-closed grading) |

Her mutant sonrası dosyalar benzersiz-isimli backup'tan restore edilip
md5 ile doğrulandı (`mr019-mutant-m{1,3,4}-*.bak`).

`make check` **yeşil** (exit 0). Test sayısı **1300** (TASK-01 başında 1293).

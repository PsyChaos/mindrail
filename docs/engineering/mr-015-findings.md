# MR-015 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-015-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `5078038` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-014 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Claim→CLAIMED + revizyon; bilinmeyenler yazmadan reddediliyor; stale revizyon conflict'i revizyon-kelimesiyle pinli (`TestClaimLifecycle`). |
| AC-01.2 | Karşılandı | Baseline özeti + kapsam; claimsiz declare (`TestBeforeChangeDeclaresScope`). |
| AC-01.3 | Karşılandı | Change + bulgular (kod/anahtar/remedy) + pending; op-replay kimlikle (`TestAfterChangeRecordsFindings`, twin-ambiguity dahil). |
| AC-01.4 | Karşılandı | Boş-id'ler tool-katmanında, mesaj-pinli; değer yankısı yok (aynı testler). |

**Tasarım notları:** ModeWrite (tek server); D-199 daraldı — replayed
bayrağı yalnızca yayan serviste (claim), after_change kimlikle kanıtlıyor
(kayıtlı sapma); twin-fixture tüketim dersi MR-007/013 deseninde.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | revision-kontrolü kapatıldı | claim testi FAIL |
| M2 | pending sabit-false | ilk varyant pinsizdi (yeşil) → twin-ambiguity beati eklendi, FAIL |
| M3 | claim validasyonu kapatıldı | ilk varyant maskeli (store da reddediyor) → mesaj-pini eklendi, FAIL |
| M4 | ModeWrite → ReadOnly | claim/after testleri FAIL |
| M5 | containment kaldırıldı (kapı bulgusu) | escape testi FAIL |
| M6 | replayed bayrağı düşürüldü (kapı bulgusu) | replay testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1226**.

### TASK-01 kapı — Reader: 4/4 CONFIRMED + gaps; Breaker: 5 REFUTED + 1 GAP→KAPANDI

**Reader gaps (kapatıldı):** claim-replay, scope-içerik, with-claim,
after-empty, conflict-kod, mesaj-pinleri — tamamı testlere eklendi.

**Breaker B-1 (containment, KAPATILDI):** `/etc/passwd` yakalanıyordu.
Düzeltme: tool-katmanı worktree-containment (`insideRoot`); pin eklendi
(M5 kırmızı).

**Karar:** TASK-01 KAPANDI.

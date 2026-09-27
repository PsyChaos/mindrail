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

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Beyansız reconcile keşfi + replay; mesaj-pinli refuse (`TestReconcileDiscoversUndeclared`). |
| AC-02.2 | Karşılandı | Session'lar-arası handover; note-var/yok iki yön; bilinmeyen reddi (`TestCheckpointCrossSessionRead`). |
| AC-02.3 | Karşılandı | İki kimlikli tam yaşam döngüsü + 11-tool kaydı (`TestLifecycleAcrossIdentities`). |
| AC-02.4 | Karşılandı | Kayıt 11 tool; kod 46; tablo/komut yok (aşağıda). |

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M5 | reconcile boş-görev kontrolü kapatıldı | mesaj-pini eklendi, FAIL |
| M6 | readable_by boşaltıldı | tip-assertion piniyle FAIL (nil/"" tuzağı atlatıldı) |
| M7 | reconcile AfterChange'e bağlandı | keşif testi FAIL (kanonik-yol pini) |
| M8 | checkpoint kaydı silindi | lifecycle/checkpoint testleri FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1229**.

### TASK-02 kapı — Reader: 3 CONFIRMED + 1 UNCONFIRMED→KAPANDI; Breaker: 6 REFUTED

**Reader:** AC-02.1/02.2/02.4 CONFIRMED. AC-02.3 state-görünürlük şerhi
kapatıldı: lifecycle'a twin-overlap pending-beati eklendi.

**Breaker:** 6/6 REFUTED (op-id görev-kapsamlı, boş-read refuse, note-hash
idempotency, silinmiş-dosya, handoff-sıralama, yakınsama hepsi tutarlı).

**Karar:** TASK-02 KAPANDI.

---

## MR-015 kapanış

Üç görev kapandı (TASK-01…02 + TASK-03 kanıt), 10 mutant kırmızı, `make
verify` + `make tidy-check` yeşil, 1229 test. MR-016'ya devir (Durum
bloğundaki gibi): onbir-tool server → validation/completion; handover'lar
→ CI; op-id'ler → retry.

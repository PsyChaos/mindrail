# MR-011 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-011-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `8d62503` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-010 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Dokunulmamış kapsam eşit re-hash → current (`TestCheckKeepsCurrentOnUntouchedScope`). |
| AC-01.2 | Kısmi-karşılandı (şerhli) | İçerik/silinen/okunamaz stale; ancak *eklenen dosya* stale yapmıyor (dosya-taneli kapsam) ve içerik-gerekçesi boştu. İkisi de kapıda kapatıldı: gerekçe snapshot'ı adlandırıyor; eklenen-dosya alt-davranışı DoD-1 gereği *karşılanmadı* olarak kayda geçti (aşağıda). |
| AC-01.3 | Karşılandı | Kapsam-dışı edit (eklenen dosya dahil) current bırakıyor — dosya-taneli kapsam yorumu, kapıda (`TestCheckIgnoresOutOfScopeEdits`). |
| AC-01.4 | Karşılandı | Satisfied/stale-only/missing + sıralı union re-run, gerekçeli (`TestCheckRequiredCoverage`). |
| AC-01.5 | Karşılandı (kapıda) | Harf pinlendi: satır-sayısı + içerik karşılaştırmalı test eklendi (`TestCheckReadOnlyPinsNoWrites`); determinizm korunuyor. |

**Tasarım notları:** hash çekirdeği paylaşımlı (`hashFiles` — MR-010
refaktörü, davranış aynı, mevcut testler yeşil); `Check` root'u parametre
alıyor (MR-010 provenance'ı değişmedi); helper sahte-scope kuruyordu
(genişletilmemiş dizin) → gerçek `snapshot.Scope` kullanılıyor.

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | eşitlik `==` → `!=` | current testi FAIL |
| M2 | kaçış-kontrolü kapatıldı | escape testi FAIL |
| M3 | coverage hep-satisfied | coverage testi FAIL |
| M4 | stale-remember kapatıldı | ilk varyant maskeli (required döngüsü geri ekliyor) → required-dışı assertion eklendi, FAIL |
| M5 | eşitlik `!= ""`a gevşetildi | stale testleri FAIL |
| M6 | scope-dedupe kaldırıldı | dup testi FAIL |
| M7 | root-clean kaldırıldı | trailing-slash testi FAIL |
| M8 | gerekçe snapshot'sız bırakıldı | reason testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` **yeşil** (exit 0). Test sayısı **1180**.

### TASK-01 kapı — Reader: 2 CONFIRMED + 2 REFUTED(harf) + 1 tension; Breaker: 3 BROKEN→KAPANDI + 3 REFUTED

**Reader bulguları (tamamı kapatıldı):** boş gap-gerekçesi → snapshot
adlandıran gerekçe + pin; AC-01.5 harfi → satır/içerik pinli test.

**Eklenen-dosya şerhi (DoD-1 kaydı):** AC-01.2'nin "added file in scope"
alt-davranışı karşılanmadı — kapsam dosya-taneli olduğundan dizine eklenen
dosya eski kanıtı eskitmiyor (kanıtın tanımladığı şey değişmiyor; yeni
dosyanın kapsanması MR-013 breadth'inin işi). Bu benim detaylandırmamın
aşımıydı; task-list AC'si ("ilgili edit") etkilenmiyor. Gerekçe burada,
kodda değil.

**Breaker bulguları (tamamı kapatıldı):** dup-scope divergence → `seen`
dedupe + pin; trailing-slash escape → root-clean + pin; boş-kapsam heutig
→ eşitlik zaten yakalıyor (pin eklendi:
`TestCheckEmptyScopeNeedsEmptyHash`).

**Karar:** TASK-01 KAPANDI (AC-01.2 şerhli).

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | `TestFreshnessLifecycleEndToEnd`: run→current; ilgili edit→stale + re-run + required-unsatisfied; ilgisiz edit current'liği bozmuyor (stale kalıyor); ikinci run→current, 2 satır tabloda birlikte. |
| AC-02.2 | Karşılandı | Grep kanıtı: evidence'de UPDATE/DELETE yok; kod 43; migration yok; config dokunulmadı. |
| AC-02.3 | Karşılandı | M1…M8 defterde, tamamı kırmızı koşuldu. TASK-02 yeni guard eklemiyor (yalnız E2E kanıt testi) — mutasyon borcu yok. |

`make verify` **yeşil** (exit 0: check + race + smoke), `make tidy-check`
**yeşil**. Test sayısı **1185**.

### TASK-02 kapı (commit sonrası bağımsız değerlendirme bekleniyor)

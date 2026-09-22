# MR-007 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-007-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `cb65502` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | `migrations/000006_changes.sql`: 5 tablo + 2 index, CHECK'ler ve natural PK'ler; başka nesne yok. |
| AC-01.2 | Karşılandı | `changes.TableSchemaVersion = 6`; 3 upgrade testi yeşil (schema-3/4/5 → 6, satırlar korunur). |
| AC-01.3 | Karşılandı | `shipped_test.go` 000006 baytını pinler; shape/load testleri 5 tabloyu kapsar. |
| AC-01.4 | Karşılandı | Knowledge dosyaları değişmedi; yeni kod/komut/anahtar yok — yalnız `schema_version` 5→6 sayısı golden'ları sürdü. |

`make check` yeşil (EXIT=0). Test sayısı `go test -list '.*' ./... | grep -c '^Test'` ile **1076**.

## TASK-01 etkileşim bulguları (check'i düşüren 2 gerçek vaka)

- **Ledger kirliliği:** `downgradeToSchemaThree` v6 kalıntıları bırakıyordu
  (edit yanlış fonksiyona gitmişti) → koordinasyon kapısı sustu. Kök neden
  bulundu (ledger `{1,2,6}` + tablolar duruyordu), helper düzeltildi,
  4 upgrade testi yeşil.
- **Disk bandı:** 6. migration DB'yi 512 KiB bandının üstüne taşıdı →
  wide end 1024'e (MR-004 precedentindeki gibi, ölçüm notuyla).

## Reader / Breaker bulguları ve giderim — TASK-01 kapısı

(TASK-01 kapısı aşağıda.)

### TASK-01 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | `idx_changes_task_unique` kaldırıldı | `TestOneOpenChangePerTask` FAIL (ikinci satır kabul edildi) |
| M2 | `TableSchemaVersion` 6 → 5 | gate testi FAIL |
| M3 | shipped pin son hanesi çevrildi | pin testi FAIL |
| M4 | `change_files` kind CHECK kaldırıldı | vocab testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı;
shipped pin'i mutant-öncesi baytlardan hesaplandığı için geçerliliğini korur.

### TASK-01 kapı (PENDING — bağımsız değerlendirme bekleniyor)

## Reader bulguları ve giderim — TASK-01 kapısı (Reader PASS)

Bağımsız Reader **PASS** verdi (tümü doc-seviyesi bulgu):

- **MEDIUM/F1 (ifade):** AC-01.1 "discovered summary" diye varolmayan kolonu
  anıyordu. Giderim: D-130 erratum + AC cümlesi düzeltildi.
- **LOW/F2:** upgrade yorumunda "bring it to 5" kalmıştı. Düzeltildi.
- **LOW/F3:** `downgradeToSchemaFour` v6 bırakıyordu (sadakatsiz fixture).
  Giderim: Four da v6 düşürür; mesaj "000005 and 000006" oldu.
- **LOW/F4:** hostilefs yorumu "three indexes" diyordu; ikisi var. Düzeltildi.
- **LOW/F5:** bulgu prose'u ölçülen-süpürülen ayrımını abartıyordu. Aşağıda
  yumuşatıldı: wide end marjla seçildi, süpürülmedi.

## Çevrimdışı Breaker (bu tur subagent yok — yazar koştu)

Bulgu kaydı: B1 (PK indirgeme) önce SURVIVED kaldı — tane-test aynı
`change_id` altında ikinci anahtarı denemiyordu; test güçlendirildi, mutant
kırmızı. B2/B3/B4 kırmızı. B5 (band 512) kırmızı. FK probları (3 adet) temiz.

### Ek pinler (bu tur)

- `TestChangeSymbolGrainIsPerKey` güçlendirildi (farklı anahtar aynı
  change altında kabul edilir) + `TestBaselineGrainIsPerTaskPath` +
  `TestOperationLogColumnsAreMandatory` + `TestNewStoreRefusesNilHandle` +
  `TestChangeRowsEnforceForeignKeys` (FK duruşu).

### TASK-01 kapı — Reader PASS, Breaker VERIFIED (ikinci tur re-grade PASS)

Tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1083**.

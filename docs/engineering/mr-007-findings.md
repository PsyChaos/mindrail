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

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | `TestCaptureBaselineReplacesWithoutStacking` (değiştir, gerçek hash'ler) + `TestCaptureBaselineEmptyHashForMissingAndNonRegular` (kayıp/dizin boş hash, fail-closed validasyonlar). |
| AC-02.2 | Karşılandı | `TestEnsureOpenChangeStable` (tek task tek satır) + `TestEnsureOpenChangeRetroactive` (NULL id'siz taze satırlar, id'li replay). |
| AC-02.3 | Karşılandı | `TestOperationReplayAndConflict` (aynı id+hash replay, farklı hash `OPERATION_ID_CONFLICT` — Ensure ve CaptureBaseline'de). |
| AC-02.4 | Karşılandı | `TestBaselineClearingDiscipline` (başka çağrılar dokunmaz, clear + recapture çalışır). |

Ek: `TestConcurrentEnsureAgreesOnOneRow` (8 yarışçı, partial UNIQUE hakem) + `TestNewStoreRefusesNilHandle`.

## Reader / Breaker bulguları ve giderim — TASK-02 kapısı

(TASK-02 kapısı aşağıda.)

### TASK-02 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | baseline DELETE kaldırıldı (istiflenir) | replace testi FAIL |
| M2 | task pre-lookup kaldırıldı | SURVIVED (benign) — ON CONFLICT + readback aynı cevabı verir; fazladan yazı yok, sadece tur. Perf-only, guard açığı değil. |
| M3 | replay kontrolü kaldırıldı (hep yeniden yaz) | replay testi FAIL (dup op satırı) |
| M4 | conflict dalı etkisiz (`&& false`) | conflict testi FAIL (laundering kabul edilirdi) |
| M5 | hash hep boş | capture testi FAIL |
| M6 | open-change ON CONFLICT kaldırıldı | concurrent + stable testleri FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` yeşil (EXIT=0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1090**.

## Reader / Breaker bulguları ve giderim — TASK-02 kapısı

Bağımsız Reader **PASS** (AC-02.1…02.4 MET), Breaker **VERIFIED**
(5 öz mutant: 4 kırmızı + 1 benign-perf; 32×3 yarış temiz).

- **Reader F1 + Breaker bulgu 1 (LOW→düzeltildi):** op log ayrı transaction'da
  commitleniyordu (crash penceresi + yarışta ham constraint hatası).
  Giderim: log yazıyla AYNI transaction'da (`recordOperationTx`);
  commit-yarışını kaybeden log'u yeniden okur (aynı hash → replay, diğer →
  conflict). `TestConcurrentSameOperationConverges` (16 yarışçı, race
  dedektörlü) pinler; N2 mutantı (ayrı-tx'e dönüş) FAIL.
- **Breaker bulgu 2 (LOW→düzeltildi):** yetim baseline satırları (task FK'siz).
  Giderim: `CaptureBaseline` task varlığını denetler
  + `TestCaptureBaselineRefusesUnknownTask`; N1 mutantı FAIL.
- **Breaker bulgu 3 (gözlem):** clear-sonrası replay eski özeti döner —
  idempotency semantiği gereği doğru (log clear'dan etkilenmez); kayıtta,
  işlem yok.
- **Reader F2 (doküman):** op-id kapsamı notu eklendi — tasked Ensure'da
  mevcut satır kısa-devre yapar, op log'a değmez (AC-02.2 testiyle kutsanmış
  davranış).
- **Reader F3 (test gücü):** replay testine op-satır-sayısı + `captured_at`
  stabilitesi eklendi.
- **Reader F4 (ifade):** M2-benign cümlesi düzeltildi — lookup'suz varyant
  op-satırı da yazardı; sınıflandırma (perf-only) aynen durur.

### TASK-02 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| N1 | task varlık denetimi kaldırıldı | unknown-task testi FAIL |
| N2 | log ayrı transaction'a alındı | race testi FAIL (ham constraint) |

### TASK-02 kapı — Reader PASS, Breaker VERIFIED (remediasyon sonrası)

Giderim sonrası focused + race süitleri yeşil; tam `make check` aşağıda.
Test sayısı `go test -list '.*' ./... | grep -c '^Test'` ile **1092**.

## TASK-03 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-03.1 | Karşılandı | `TestDiscoverFilesGitKinds` (6 porcelain şekli) + git `TestStatusEntries*` (fake/hatalı/gerçek). |
| AC-03.2 | Karşılandı | `TestDiscoverFilesGitExclusions` (satır yok, stat yok) + escape testi (fail-closed). |
| AC-03.3 | Karşılandı | `TestBaselineFileDeltaConverges` (değişen + kayıtsız kapsam satır olur). |
| AC-03.4 | Karşılandı | `TestDiscoverFilesGitKinds` + `TestUpsertFileRowsConverges` (tür/hash/via). |
| AC-03.5 | Karşılandı | `TestGitAndBaselineFileConvergence` (içerik eşit, provenance farklı). |

## Reader / Breaker bulguları ve giderim — TASK-03 kapısı

(TASK-03 kapısı aşağıda.)

### TASK-03 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | `??` → modified eşlemesi | kinds testi FAIL |
| M2 | exclusion kaldırıldı | exclusions testi FAIL |
| M3 | root-escape kontrolü kaldırıldı | escape testi FAIL |
| M4 | upsert kind validasyonu kaldırıldı | Önce SURVIVED — DB CHECK arkadan yakalıyordu. Test usage-kodu denetleyecek şekilde güçlendirildi; mutant FAIL. |
| M5 | kayıtsız kapsam atlandı | delta + convergence testleri FAIL |
| M6 | git hatası boş kümeye indi | git paketinin kendi testi FAIL (bu pakette değil, kayıtta) |
| M7 | baseline via'sı reconcile yazıldı | convergence provenance assertion FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` yeşil (EXIT=0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1101**.

## Reader / Breaker bulguları ve giderim — TASK-03 kapısı

Bağımsız Reader **FAIL** (dar: 2 MEDIUM test-boşluğu + 3 LOW), Breaker
**VERIFIED** (gözlemler + 5 öz mutant: 3 kırmızı, 1 benign, 1 muafiyet).

- **MEDIUM/F1:** non-regular satırları pinsizdi. Giderim:
  `TestDiscoverFilesGitNonRegularRows` (dizin+fifo, boş hash); N1 kırmızı.
- **MEDIUM/F2:** C/T/U eşlemeleri fixturesızdı. Giderim: iki katmanda
  C/T/U satırları; N2 kırmızı.
- **LOW/F3:** baseline silinme şekli yoktu. Giderim:
  `TestBaselineFileDeltaReportsDeletion`; N3 kırmızı.
- **LOW/F4:** discover→upsert→read kompozisyonu yoktu. Giderim:
  `TestDiscoverUpsertReadComposes`; N4 kırmızı.
- **LOW/F5:** staged-only rename notu koda işlendi.
- Breaker gözlemleri: kök-"/" kaçağı (spekülatif, işlem yok), satır-sınırsız
  upsert (benign), TOCTOU (bilinen sınıf, kayıtta), deleted-early-return
  kaldırma benign (yokluk-toleranslı hash aynı satırı üretir).

### TASK-03 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| N1 | non-regular hata verir | non-regular testi FAIL |
| N2 | `C` → modified eşlemesi | kinds testi FAIL |
| N3 | silinmiş dosya added sayılır | deletion testi FAIL |
| N4 | upsert via'yı ezer | composition + convergence testleri FAIL |

### TASK-03 kapı — Reader PASS, Breaker VERIFIED (ikinci tur)

Remediasyon bağımsız ikinci turda notlandırıldı: 5 maddenin 5'i doğrulandı,
remediasyonun getirdiği yeni sorun yok. `make check` yeşil, 1104 test.

`make check` yeşil (EXIT=0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1104**.

Remediasyon testleri yeşil; tam `make check` aşağıda.

## TASK-04 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-04.1 | Karşılandı | `TestSymbolFlagsPerLanguage` (body/sig/added/removed + hash'ler) + `TestSymbolFlagsTypeScriptJavaScript` (temsilci akış). |
| AC-04.2 | Karşılandı | `TestDiscoveryIndexesWhatItReads` (stored == fresh parse, rename uid takibi). |
| AC-04.3 | Karşılandı | `TestAfterChangeConverges` (tek id, latest-wins flag flip). |
| AC-04.4 | Karşılandı | `TestStagedMoveHintIsLive` (hintli migrate, hintsiz mint; DiffRenames→hint dönüşümü dahil). |

Ara notlar: ilk-temas her zaman `added` olur (test akışları buna göre);
porcelain-sıra tuzağı belgelendi (`status` new-first, `diff` old-first).

## Reader / Breaker bulguları ve giderim — TASK-04 kapısı

(TASK-04 kapısı aşağıda.)

### TASK-04 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | çıkarım hatası yutuldu | broken-file testi FAIL |
| M2 | body bayrağı ters çevrildi | flags testi FAIL |
| M3 | uid iliştirme atlandı | uid assertion'ları FAIL |
| M4 | silinen dosya satır üretmez | deletion testi FAIL |
| M5 | upsert yerine plain INSERT | convergence testi FAIL. İki geçersiz deneme: `OR IGNORE` upsert ile aynı davranır (SQLite önceliği) + string-cerrahi dosyayı bozdu (restore edildi). |
| M6 | hint'ler IndexFile'a geçmez | hint testi FAIL |
| M7 | birimsiz dosya hata verir | out-of-unit testi FAIL |

Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` yeşil (EXIT=0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1112**.

## Reader / Breaker bulguları ve giderim — TASK-04 kapısı

Bağımsız Reader **PASS** (AC-04.1…04.4 MET), Breaker **VERIFIED**
(6 öz mutant + problar temiz).

- **MEDIUM/F1:** TS/JS tam bayrak matrisi eksikti. Giderim: body/structure
  assertion'ları eklendi; N1 kırmızı.
- **MEDIUM/F2:** hash değerleri assert edilmiyordu. Giderim: delta satır
  hash'leri stored facts ile eşitlenir; N2 kırmızı.
- **LOW/F3:** hint yolu delta-satır uid assert etmiyordu. Giderim: eklendi
  (M3 de kapsar).
- **LOW/F4:** TS/JS added/removed tek Python'daydı. Giderim: JS removal
  turu eklendi.
- **LOW-8 (Breaker):** boş-delta AfterChange pinsizdi. Giderim:
  `TestAfterChangeEmptyDeltaReturnsOpenChange` eklendi (Breaker mutant D
  kırmızı olduğunu kanıtladı).
- **F5:** işlem yok (kabul).

### TASK-04 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| N1 | structure bayrağı hep false | flags testi FAIL |
| N2 | change satır hash'leri boş | discovery testi FAIL |

### TASK-04 kapı — Reader PASS, Breaker VERIFIED (remediasyon sonrası)

Giderim sonrası focused süitler yeşil; tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1113**.

## TASK-05 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-05.1 | Karşılandı | `TestReconcileConvergesWithBaselinePath` (ikiz fixture, sıra-bağımsız final içerik; provenance farklı, içerik aynı). NOT: paylaşılan DB'de ardışık akışlar yakınsamaz (keşif tüketir) — yakınsama aynı editin iki yoldan bağımsız keşfidir; bulgu D-131 olarak kayıtta. |
| AC-05.2 | Karşılandı | `TestReconcileDivergenceListsOutOfScopeEdits` (yol+tür+sebep, bloklamaz, satırlar durur). |
| AC-05.3 | Karşılandı | `TestReconcileWithoutTaskCreatesRetroactiveChange` (NULL-task satırları). |
| AC-05.4 (SLO) | Karşılandı | `TestWarmPathBudgetsReading`: after_change 10 dosya ~9ms (bütçe 1.5s), reconcile ~3ms (bütçe 2s). |
| AC-05.4 (non-goals) | Karşılandı | `TestNoLaterMilestoneMachinery` (import taraması), `TestNoNewCodesIn01` (40 pin), `TestCommandSurfaceUnchangedIn01` (mevcut), `TestKnowledgeSchemaStaysV1` (mevcut). |
| Git fixture | Karşılandı | `TestReconcileDiscoversStagedAndWorktreeChanges` (staged + linked worktree, gerçek git) + `TestReconcileStagedMoveMigrates` (staged R → uid taşınır). |

### D-131 — yakınsama bağımsız keşifler üzerinedir (AC-05.1 açıklaması)

Paylaşılan veritabanında ardışık akışlar yakınsayamaz: ilk keşif indexler,
ikincisi indexlenmiş gerçekleri bulur ve sembol satırı üretmez. Yakınsama
garantisi şudur: aynı edit, aynı başlangıç durumundan, iki yoldan bağımsız
keşfedildiğinde aynı dosya+sembol içeriğini üretir. Test ikiz fixture ile
bunu kanıtlar (uid'ler DB-başına ayrı lineage olduğundan varlıkça
karşılaştırılır).

## Reader / Breaker bulguları ve giderim — TASK-05 kapısı

(TASK-05 kapısı aşağıda.)

### TASK-05 guard mutasyon defteri (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | divergence döndürülmez | divergence testi FAIL |
| M2 | task zorunlu tutulur | retroactive testi FAIL |
| M3 | hint'ler düşürülür | staged-move testi FAIL (önce SURVIVED kaldı — test uid karşılaştırmıyordu; hint-öncesi uid kurulumuyla güçlendirildi) |
| M4 | yasak import (`net/http`) belirir | tarama testi FAIL |
| M5 | 41. kod eklenir | sayı testi FAIL |
| M6 | SLO 1ns yapılır | bütçe testi FAIL |

Porcelain-sıra notu (status=new-first, diff=old-first): `rename.go`
yorumunda belgeli; gerçek-git unicode testi `-z` davranışını kanıtlar.
Her mutant sonrası dosyalar backup'tan restore edilip md5 ile doğrulandı.

`make check` yeşil (EXIT=0). Test sayısı `go test -list .\* ./... | grep -c ^Test` ile **1121**.

## Reader / Breaker bulguları ve giderim — TASK-05 kapısı

Bağımsız Reader **PASS** (AC-05.1…05.5 MET), Breaker **VERIFIED** (5 öz
mutant + problar temiz).

- **MEDIUM-1 (hint partition):** `fileHintsFor` iki eşzamanlı rename ile
  pinsizdi. Giderim: `TestReconcileHintsPartitionByTarget` (her heir kendi
  eski uid'sini taşır); P1 mutantı (bölümleme yok) FAIL.
- **MEDIUM-2 (tasarım erratum):** §6 "git failure → empty set" fail-closed
  gerçekle çelişiyordu. Giderim: erratum işlendi (failure → error).
- **LOW-3 (ölü helper):** `contentKey` silindi.
- **LOW-4 (retroactive sembol):** def'li fixture + added/uid assertion eklendi.
- **LOW-5 (SLO):** FakeRunner marjı not edildi (~500×; MR-017 gerçek-bütçe
  işi).
- **LOW-6 (yasaklı listesi):** knowledge/cli genişletmesi geri alındı —
  mevcut mimariyi yasaklardı; gerekçesi koda işlendi.
- Breaker INFO'lar: joinRoot containment yok (git çıktısı güvenilir girdi;
  işlem yok), diff-hatası sessiz hints (D-101 gereği; sürücü-logu gelecek
  işi).

### TASK-05 guard mutasyon defteri — ek (tamamı geri alındı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| P1 | hint bölümleme yok (hepsi her dosyaya) | partition testi FAIL |

### TASK-05 kapı — Reader PASS, Breaker VERIFIED (ikinci tur)

Remediasyon bağımsız ikinci turda notlandırıldı: 6 maddenin 6'sı doğrulandı,
yeni sorun yok. `make check` yeşil, 1122 test.

Giderim sonrası focused süitler yeşil; tam `make check` aşağıda. Test sayısı
`go test -list '.*' ./... | grep -c '^Test'` ile **1122**.

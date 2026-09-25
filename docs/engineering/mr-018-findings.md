# MR-018 — Bulgular ve kapı kaydı

Bu kayıt, dondurulmuş [gereksinimlerin](mr-018-requirements.md) TASK-01
kapısını kaydeder. Reader/Breaker değerlendirmelerini ve bu turdaki kanıtları
korur; gereksinim veya tasarımın yerine geçmez.

## Başlangıç ve çalışma ağacı

- Başlangıç HEAD'i `a2ae21c` idi (dondurulmuş gereksinimler + tasarım).
  §0 tablosundaki `make verify` ve `make tidy-check` MR-017 kapanışında
  yeşildi.

## TASK-01 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-01.1 | Karşılandı | Temiz aralık yeşili (`TestVerifyCICleanGreen`), unregistered DENY (`TestVerifyCIUnregisteredDenies`), guard-denial (`TestVerifyCIGuardDenies`), hepsi paylaşılan kapıdan. |
| AC-01.2 | Karşılandı | Bozuk base (`TestVerifyCIBadBaseRefuses`, COMMAND_LINE_INVALID + 0 satır), unborn HEAD (`TestVerifyCIUnbornHeadRefuses`), default-zincir olumlu/olumsuz (`TestVerifyCIDefaultBase`, `...NamesCandidates`). |
| AC-01.3 | Karşılandı | Çift mod ve rev-flags kapısı (`TestVerifyModesAreExclusive`); yalın `verify` reddi MR-017'den devralındı (`TestVerifyBareRefusesMode`). |
| AC-01.4 | Karşılandı | Fatal knowledge + bozuk base yan yana: knowledge hatası kazanır, 0 satır (`TestVerifyCIKnowledgeFirst`). |
| AC-01.5 | Karşılandı | Aynı içerik staged ve CI'da birebir aynı code+key kümesini deny ediyor (`TestVerifyCIParityWithStaged`). |

**Tasarım notları:** `compose`/`guardMappings` çıkarımları staged
davranışını satır-satır korur (`make check` yeşil, staged testleri
dokunulmadan geçiyor). Guard trigger'ı CI yolunda `TriggerCI` — staged
bulgusu "staged", CI bulgusu "ci" der; parite karşılaştırması bu yüzden
code+key üzerinden yapılır (D-225 okuması, aşağıda). `M5` adayı
davranış-koruyucu çıktı (isim dönmek de çalışıyor — git downstream'da
kendisi çözümlüyor); pin `M5'` ile yazıldı (zincir hatası → olumlu test
kırmızı). `M1`/`M6` ilk yazımda kırmızı vermedi (test kurgusu reddi başka
yoldan üretiyordu); testler güçlendirildi, sonra kırmızı koşuldu.

**D-225 okuması (uygulama notu):** paritedeki "aynı provenance" guard
trigger kelimesini kapsamaz — trigger yolu adlandırır, bulgunun
kaynağını değil. CI'da "staged" yazmak provenance yalanı olurdu.

### TASK-01 guard mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M1 | mod-dışlayıcılık kapatıldı (`staged && ci`) | `TestVerifyModesAreExclusive` FAIL |
| M2 | knowledge-fatal erken-dönüşü kapatıldı | `TestVerifyCIKnowledgeFirst` FAIL |
| M3 | guardRange before=head (zayıflama görünmez) | `TestVerifyCIGuardDenies` FAIL |
| M4 | RangeEntries head..head (boş aralık) | `TestVerifyCIUnregisteredDenies` FAIL (yeşile döndü) |
| M5' | DefaultBase hep-hata | `TestVerifyCIDefaultBase` FAIL |
| M6 | rev-flags kapısı kapatıldı | `TestVerifyModesAreExclusive` FAIL |
| M7 | yalın-verify reddi kapatıldı | `TestVerifyBareRefusesMode` FAIL |

Her mutant sonrası dosyalar benzersiz-isimli backup'tan restore edilip
md5 ile doğrulandı (`mr018-mutant-m{1,2,3}-*.bak`). M5 (çözülmemiş isim
dönmek) kırmızı vermedi — davranış-koruyucu mutant olarak kayıtlı,
yerine M5' koşuldu.

`make check` **yeşil** (exit 0). Test sayısı **1276** (TASK-01 başında 1260).

## TASK-01 kapı remediasyonu (Breaker 5 bulgu → 5 düzeltme)

Reader 5/5 CONFIRMED verdi (F1/F2 INFO: boş-olmayan temiz aralık ve
merge-base-ci testi TASK-02'ye bırakıldı). Breaker 5 GERÇEK buldu;
tamamı aynı turda kapatıldı:

| # | Bulgu | Düzeltme | Pin |
|---|---|---|---|
| B1 | Ters aralık (`base` head'in torunu) boş diff'le yeşil onaylıyordu | `mergeBase == head && base != head` usage-reddi (SHA'lı mesaj) | `TestVerifyCIInvertedRangeRefuses` (M8) |
| B2 | Default-base sessizce boş aralığa çözümleniyordu (HEAD base'in gerisinde) | B1 reddi + çözümlenen SHA'nın mesajda adlandırılması; sessiz-yeşil kapandı | B1 pini + `TestVerifyCIWorktreeMismatchRefuses` mesaj denetimi |
| B3 | CI yolu worktree baytlarına dokunuyordu (`IndexFile` diskten okur); head-gerisi checkout sınıflanmamış çöküyordu (boş kod) | Önkoşul: worktree HEAD == judged head, aksi hâlde usage-reddi | `TestVerifyCIWorktreeMismatchRefuses` (M9) |
| B4 | `compose` tüm NULL-task satırlarını union'lıyordu: CI commitlenmemiş içeriği, staged commitli içeriği yargılıyordu | `AttributeChanges(ctx, [changeID])`: her mod yalnız kendi satırlarını attribute eder | `TestVerifyCIRangeScoping`, `TestVerifyStagedIgnoresCIRows` (M11) |
| B5 | Knowledge worktree'den okunuyordu; commitli-bozuk + düzeltilmiş-desk yeşil geçiyordu | Önkoşul: `.mindrail/`/`.git/` dışı kirli desk usage-reddi (dosyalar adlandırılır) | `TestVerifyCIDirtyDeskRefuses` (M10), dışlama pini (M12) |

**Önkoşul kaydı (D-222 uygulaması):** `VerifyCI` temiz checkout ister —
worktree HEAD judged head'e eşit, desk `.mindrail/`/`.git/` dışında temiz.
Fresh clone bunu yapısal olarak sağlar (TASK-02); kirli desk yalan
söylemek yerine reddedilir.

**Bilinen kısıt (kapsam-dışı, dürüst kayıt):** sembol satırları index
durumuna göre delta'dır; aynı veritabanında staged-yargıdan sonra
koşulan yerel `--ci` satır göremeyip eksik-deny verebilir (union
kalkmadan önce tam tersi kirlenme vardı — B4). Otoriter kapı fresh-clone
CI'dır (ayrı DB); parite testi bu yüzden faz-arasında DB sıfırlar.
Staged yolunun modlar-arası aynı delta-görünürlüğü MR-017'den devralındı,
değiştirilmedi.

**Gözlem (kapsam-dışı):** init'siz dizinde `verify` (staged dahil, önceden
var) boş kodla düşüyor — bootstrap davranışı, burada değiştirilmedi.

### Remediasyon mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M8 | inversiyon reddi kapatıldı | `TestVerifyCIInvertedRangeRefuses` FAIL |
| M9 | worktree-eşleşme reddi kapatıldı | `TestVerifyCIWorktreeMismatchRefuses` FAIL |
| M10 | kir-desk reddi kapatıldı | `TestVerifyCIDirtyDeskRefuses` FAIL |
| M11 | attribution union'a döndürüldü | `TestVerifyCIRangeScoping` + `TestVerifyStagedIgnoresCIRows` FAIL |
| M12 | kir-dışlama (`.mindrail/`) düşürüldü | `TestVerifyCICleanGreen` FAIL |

`M1`/`M6` ilk yazımda kırmızı vermedi (test kurgusu reddi başka yoldan
üretiyordu); testler güçlendirildi (`--staged --base` reddi, commitli
çift-mod), sonra kırmızı koşuldu. `M5` davranış-koruyucu çıktı, `M5'`
koşuldu.

Remediasyon sonrası `make check` **yeşil** (exit 0). Test sayısı **1281**.
Re-gate: Reader + Breaker ikinci tur (aşağıda).

## Re-gate tur 2 (BLOCKED → N1/F1 düzeltmesi)

İki kapı da B1–B4'ü kapattı, B5'i kısmi buldu: `.mindrail/` toptan
dışlama, knowledge kirini gizliyordu (N1/F1 HIGH — committed-fatal +
desk-silme yeşile dönüyordu, M10 yalnızca source kirini pinliyordu).

Düzeltme: dışlama daraltıldı — `.mindrail/knowledge/**/*.json`
(loader'ın okuduğu suffix) kirliyse reddeder; `.gitkeep`/config/`.git/`
hâlâ görmezden gelinir (fresh `init` üç untracked scaffolding bırakır,
kanıtı kapıda). Ters yön (desk-eklenen fatal) knowledge-first kapısında
fail-closed olur (exit 1, fatal raporlu) — menzil hiç adlandırılmaz.

| Pin | Kapsadığı yön |
|---|---|
| `TestVerifyCIKnowledgeDirtRefuses` (M13) | committed-fatal + desk-silme → usage-reddi, dosya adlandırılır |
| `TestVerifyCIKnowledgeAddedDirtRefuses` | committed-clean + desk-eklenen fatal → knowledge-first fatal-reddi |
| `TestDeskDirtKnowledgeRecords` (git birimi) | `.json` listelenir, `.gitkeep`/config/`.git/` listelenmez |

Yan etki: guard iki-geçişli testi artık seed'i commitliyor (CI temiz
desk ister; fresh-clone'a daha sadık). M13 kırmızı koşuldu (carve-out
düşürülünce desk-temiz knowledge ile yeşile döndü).

Tur-2 sonrası `make check` **yeşil**. Test sayısı **1284**.
Re-gate tur 3 (N1/F1 odağı, aşağıda).

### TASK-01 kapı — Reader: 5/5 CONFIRMED; Breaker round-1: 5 BULGU→KAPANDI; round-2: N1/F1→KAPANDI; round-3: PASS

**Round-1 Reader:** AC-01.1…01.5 CONFIRMED ( +2 INFO: F1 boş-olmayan
temiz aralık, F2 merge-base CLI testi — ikisi de TASK-02'ye).
**Round-1 Breaker:** B1…B5 GERÇEK, tamamı kapatıldı (yukarıdaki tablo).
**Round-2:** B1–B4 kapandı; B5 kısmi → N1/F1 HIGH (aşağıda kapatıldı).
**Round-3 (N1/F1 odağı):** PASS — carve-out loader yüzeyiyle eşleşiyor
(`recordSuffix .json`), iki yön pinli (M13 eski ağaçta kırmızı
koşuldu: exit 0 + boş `knowledge_problems`), F1 repro'su HEAD binary'de
iki yönde de reddediyor, süit yeşil. Tek not doc-only stale comment —
aynı turda düzeltildi.

**Karar:** TASK-01 KAPANDI.

## TASK-02 kabul kanıtı

| Kriter | Sonuç | Kanıt |
|---|---|---|
| AC-02.1 | Karşılandı | Fresh-clone temiz yeşil (`TestVerifyCIFreshCloneCleanGreen`, boş-olmayan docs-aralığı — Reader F1'i de kapatır) + `--no-verify` reprodüksiyonu (`TestVerifyCINoVerifyReproduced`, staged↔clone code-kümesi birebir). |
| AC-02.2 | Karşılandı | Clone'da ranged guard blocking (`TestVerifyCIFreshCloneGuardBlocks`, iki-geçiş + commitli seed). |
| AC-02.3 | Karşılandı | Clone'da commitli-fatal knowledge fail-closed (`TestVerifyCIFreshCloneKnowledgeFailsClosed`, 0 satır). |
| AC-02.4 | Karşılandı | Denial şekli (`TestVerifyCIDenialShape`: code+provenance+remedy, bilinen kodlar, envelope exit). |

**Tasarım notları:** clone'lar gerçek `git clone` ile kurulur (mock
yok, tech-stack §92); `origin/main` zinciri clone'da çözümlenir.
Guard testindeki seed clone'da commitlenir (temiz-desk önkoşulu).
Knowledge testinde `init` de fail-closed çıkar (exit 1) ama DB'yi
yazar — sonraki `verify --ci` knowledge-first reddeder; iki yarı da
pinli. Merge-base CLI testi (`TestVerifyCIUnrelatedBasesRefuse`,
Reader F2) TASK-02'ye alındı.

**Üretim-kodu notu (REQ-07):** TASK-02 başlangıçta üretim kodu
eklemiyordu; kapı remediasyonu üç üretim değişikliği getirdi
(aşağıda) — üçü de kırmızı koşuldu (M14…M16).

`make check` **yeşil** (exit 0). Test sayısı **1290** (TASK-02 başında 1284).

## TASK-02 kapı remediasyonu (Breaker BLOCKED → 3 düzeltme)

**Reader:** 4/4 CONFIRMED, PASS. **Breaker:** 2 reprodüksiyon + 1
insidental:

| # | Bulgu | Düzeltme | Pin |
|---|---|---|---|
| T2-1 | Clean-clone fixture boş aralığı yargılıyordu (default base == HEAD) | Explicit `--base` + `change_files == 1` denetimi | `TestVerifyCIFreshCloneCleanGreen` (M16) |
| T2-2 | Bare `--ci` main-tip bypass'ı sessiz-yeşil onaylıyordu | Default'tan gelen boş aralık usage-reddi (explicit eşit revler yeşil kalır) | `TestVerifyCIDefaultEmptyRefuses`, `TestVerifyCIDefaultOverBypassRefuses`, `TestVerifyCIDefaultBaseBehind` (M14) |
| T2-3 (insidental) | `verify-ci` change'i daralmıyordu: geniş-aralık satırları dar aralıkta yeniden yargılanıyordu | Menzil-anahtarlı op-id (`verify-ci-<base12>-<head12>`): hüküm (base, head)'in fonksiyonu | `TestVerifyCINarrowsAcrossRuns` (M15) |

**Tasarım okumaları:** D-223'e ek — default'tan gelen boş aralık
reddedilir, explicit boş aralık yeşildir (çağıranın seçimi). D-227'ye
ek — CI recompute eder: her menzil kendi change'inde birleşir, aynı
menzilin tekrarı yakınsar. Staged yolundaki aynı-birikim davranışı
MR-017'den devralındı, değiştirilmedi.

**Guard-iki-geçiş düzeltmesi (T2-3'ün bedeli):** menzil-anahtarlama,
geçiş-arası head taşıyan guard testlerini kırdı (ikinci geçiş yeni
change'de satır göremedi — delta-görünürlüğü). Çözüm: binding
knowledge'ı geçişlerden ÖNCE commitlenir (base pininden önce);
head iki geçişte de sabittir, aynı change birikir. Staged guard testi
dokunulmadan yeşil.

### Remediasyon mutasyon defteri (tamamı geri alındı, md5-doğrulamalı)

| # | Mutant | Kırmızı kanıt |
|---|---|---|
| M14 | default-boş reddi kapatıldı | `TestVerifyCIDefaultOverBypassRefuses` + `TestVerifyCIDefaultEmptyRefuses` FAIL |
| M15 | menzil-anahtarlı op-id sabit id'ye döndürüldü | `TestVerifyCINarrowsAcrossRuns` FAIL (+ guard/parity de düşer) |
| M16 | RangeEntries head..head | `TestVerifyCIFreshCloneCleanGreen` FAIL (dosya-sayım pini) |

Remediasyon sonrası `make check` **yeşil**. Test sayısı **1293**.
Re-gate tur 2 (aşağıda).

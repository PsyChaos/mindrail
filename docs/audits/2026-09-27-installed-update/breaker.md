# Installed update and JEV distribution — Breaker audit

## Sonuç

**REJECT**

İki bulgu canlı olarak üretildi:

- `BRK-001` — `make install`, hedef bir dizin olduğunda başarı bildirip binary'yi
  yanlış yere bırakıyor; `BINDIR` bir symlink olduğunda `DESTDIR` dışına yazıyor.
- `BRK-002` — yeni Go JEV bridge'indeki sekiz anlamlı koruma mevcut kalıcı test
  kümesinden sağ çıkıyor. Özellikle secret-output testi, secret guard'a ulaşmadan
  `DisallowUnknownFields` tarafından reddedildiği için ismen var ama ayırt edici
  değil.

Adapter'ın kendisi, eski repository yükseltmesi, veri/kimlik/hook koruması ve
`init`/`update` eşliği kırılmadı. Ret kararı bu iki üretilmiş bulguya dayanıyor.

## Kimlik, kapsam ve güvenlik sınırı

- Base: `64e8f8a533dc6bbf34b60edd8455f5d5120ddace`
- Donmuş gereksinimler:
  `docs/engineering/installed-update-and-jev-distribution-2026-09-27.md`
- Orijinal istekler:

  > yap onerinede uyuyorum. yanliz jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin

  > bu update olayini yapman lazim. hali hazirda proje de zaten "mindrail init" yapildi.
  >
  > ayrica jev key'yi nereye girecegim?

- Mod: paralel bağımsız Reader/Breaker; bu rapor Reader sonucu okunmadan yazıldı.
- Scratch root: `/tmp/mindrail-breaker-ftJJ4O`
- Kullanıcının working tree'sinde yalnız bu rapor oluşturuldu. Mutasyonlar, eski
  binary, geçici repository'ler ve `make install` hedefleri scratch altında kaldı.
- Bu audit deneylerinde gerçek TypeSafe key/session log'u yeniden okunmadı; gerçek
  proje runtime'ı değiştirilmedi ve teste verilmedi. Secret deneylerinde yalnız
  `BREAKER_SENTINEL_SECRET` gibi sentineller kullanıldı.

Başlangıç ve rapor öncesi working-tree diff/status parmak izleri aynıydı:

| Ölçüm | Başlangıç | Breaker yazımından hemen önce |
| --- | --- | --- |
| `git diff --binary \| sha256sum` | `cb9066…d33` | `cb9066…d33` |
| `git status --porcelain=v1 -z \| sha256sum` | `289ca8…bfd2` | `289ca8…bfd2` |

## Bulgu BRK-001 — install hedef şekli doğrulanmadığı için yanlış başarı ve staging escape

**Severity:** HIGH
**Confidence:** CONFIRMED
**Gereksinimler:** REQ-003, audit planındaki install-path/replacement threat deneyi

### Üretilen davranış

Kontrol kolu gerçek bir `DESTDIR/usr/bin` dizisiydi:

```sh
GOCACHE=/tmp/mindrail-breaker-ftJJ4O/install-gocache \
  make install DESTDIR=/tmp/mindrail-breaker-ftJJ4O/pkgroot BINDIR=/usr/bin
```

Binary `0755` modunda doğru hedefe kuruldu ve geçici dosya kalmadı.

Senaryo A'da nihai hedef önceden dizin yapıldı:

```sh
mkdir -p /tmp/mindrail-breaker-ftJJ4O/pkg-obstruct/usr/bin/mindrail
GOCACHE=/tmp/mindrail-breaker-ftJJ4O/install-gocache \
  make install DESTDIR=/tmp/mindrail-breaker-ftJJ4O/pkg-obstruct BINDIR=/usr/bin
```

`mv -f "$tmp" "$dest"`, dosyayı hedef dizinin **içine** geçici adıyla taşıdı.
Make exit 0 verdi ve var olmayan kurulum için
`installed .../usr/bin/mindrail` yazdı.

Senaryo B'de `DESTDIR/usr/bin`, scratch dış hedefe symlink idi:

```text
pkg-link/usr/bin -> /tmp/mindrail-breaker-ftJJ4O/outside-bin
```

Aynı `make install` exit 0 verdi ve binary'yi symlink hedefindeki
`outside-bin/mindrail` yoluna yazdı. Böylece paket staging sınırı sessizce aşıldı.
Bu, install ayrıcalıklı çalıştırılırsa daha yüksek etkiye sahiptir.

### Kontrol/senaryo sayıları

| Kol | Make exit | Söylenen hedefte regular binary | Hedefin içinde temp adlı binary | `DESTDIR` dışı yeni binary |
| --- | ---: | ---: | ---: | ---: |
| Kontrol: normal dizin | 0 | 1 | 0 | 0 |
| Senaryo A: `dest` dizin | 0 | 0 | 1 (`27,712,040` byte) | 0 |
| Senaryo B: `BINDIR` symlink | 0 | 0 (staging içinde) | 0 | 1 (`27,712,040` byte) |

Bu bir salt dokümantasyon problemi değildir: başarı kodu ve çıktı gerçekte kurulmamış
bir hedefi kuruldu diye ilan ediyor; symlink kolunda ise caller'ın verdiği staging
sınırının dışı değişiyor.

### Önerilen düzeltmenin ölçümü

Scratch Makefile'a, temp oluşturmadan önce iki preflight eklendi:

```sh
test ! -L "$dir"
test ! -e "$dest" || test -f "$dest"
```

Bu prototip üretim tree'sine alınmadı. Aynı dört kolda ölçüm:

| Kol | Düzeltme öncesi exit | Prototip sonrası exit | Beklenen |
| --- | ---: | ---: | --- |
| Normal gerçek dizin | 0 | 0 | kabul |
| Nihai hedef dizin | 0 | 2 | reddet |
| Install dizini symlink | 0 | 2 | reddet |
| Nihai hedef regular dosyaya symlink | denenmedi | 0 | güvenli atomik replacement; symlink'in dış hedefi değişmedi |

Prototip sonrası kontrol binary'si regular `0755`, temp kalıntısı sayısı `0` idi.
Nihai hedef symlink kolunda dış dosyanın SHA-256'sı değişmedi; symlink kurulu binary
ile atomik olarak değiştirildi. Böylece ölçülen düzeltme, normal kurulumu veya güvenli
nihai-hedef replacement'ını gereksiz yere reddetmedi.

Üretim düzeltmesi TOCTOU ve daha yukarı parent-component symlink'lerini de açıkça
ele almalıdır; yalnız yukarıdaki iki shell testi nihai sağlamlaştırma olarak sunulmuyor.

### Class sweep

`mktemp ...mindrail.install`, `INSTALL_DIR` ve `mv -f "$tmp" "$dest"` şekli
repository'de yalnız Makefile'daki bu install recipe'sinde bulundu. Repository-local
`AGENTS.md`/hook kurulumu aynı sınıfta değil; `os.Root`, symlink reddi, containment ve
atomik replace kullanıyor. Başka eş install recipe bulunmadı.

## Bulgu BRK-002 — sekiz bridge guard mevcut suite'ten sağ çıkıyor

**Severity:** MEDIUM
**Confidence:** CONFIRMED
**Gereksinim:** REQ-009; Breaker move 1

### Üretilen davranış

Python adapter mutation turu güçlüydü: `82/82` guard öldü. Yeni Go bridge için
otomatik harness yalnız tek satırlık status-consistency guard'ını yakaladı; o guard
kalıcı `TestAgent` kümesinden sağ çıktı. Çok satırlı ve expression biçimli diğer
anlamlı guard'lar elle, her seferinde yalnız bir koruma etkisizleştirilerek sınandı.

Kalıcı test komutu:

```sh
env GOCACHE=/tmp/mindrail-breaker-ftJJ4O/manual-mut-cache \
  go test ./internal/cli -run TestAgent -count=1
```

| Etkisizleştirilen koruma | Kalıcı suite | Ayırt edici input olmadan sonuç |
| --- | --- | --- |
| output JSON içinde exact key'i reddet | GREEN | secret guard test edilmedi |
| `status` ↔ `enabled` tutarlılığı | GREEN | fallback/disabled tutarsızlığı test edilmedi |
| stdin `LimitReader` sınırı | GREEN | unbounded read testi yok |
| raw adapter output üst sınırı | GREEN | üst sınır testi yok |
| `boundedBuffer` üst sınırı | GREEN | writer sınırı testi yok |
| stdin read-error erken fallback | GREEN | hata-reader testi yok |
| eksik newline ekleme | GREEN | record terminator assertion'ı yok |
| lookup/run arasındaki interpreter kaybolmasını `python_unavailable` sınıflandırma | GREEN | TOCTOU hata sınıfı testi yok |
| unknown candidate reddi | RED | korunuyor |
| başarılı JSON + başarısız child process reddi | RED | korunuyor |

Toplam: **10 anlamlı bridge mutasyonu; 2 killed, 8 survived**.

Secret survivor masked-guard örneğidir. Mevcut
`TestAgentRouteRejectsEscapedSecretAndInconsistentStatus` çıktıya `detail` alanı
ekliyor; `DisallowUnknownFields` önce reddediyor. `TestAgentRouteInvalidOutputIsSanitized`
ise JSON olmayan metin kullanıyor. Secret taraması kaldırıldığında iki test de yeşil
kalıyor.

### Ayırt edici kontrol/senaryo ölçümü

Scratch'a beş temel survivor için geçici, otherwise-valid testler; diğer üçü için
read-error/newline/interpreter-race testleri eklendi. Üretim tree'sine eklenmediler.
Kontrol toplamı `8/8 PASS`; her guard tek tek kaldırıldığında ilgili test `8/8` ayrı
senaryoda kırmızı oldu.

| Guard | Kontrol | Mutant senaryo |
| --- | --- | --- |
| secret-output | sentinelin output occurrence sayısı `0` | sentinelli raw JSON geçti; output `132` byte ve sentinel içeriyor |
| status consistency | sanitized fallback | `enabled:false/status:fallback` raw sonucu aynen geçti |
| output bound | çıktı `<1,048,576` byte | `1,048,685` byte geçti |
| stdin bound | okunan byte `65,537` | okunan byte `131,074` |
| bounded buffer | `n=1,048,576`, error non-nil | `n=1,048,593`, error nil |
| read error | Python launch sayısı `0` | geçici testte launch çağrısı tetiklendi |
| newline | son byte `\n` | son byte `\n` değil |
| interpreter race | reason `python_unavailable` | reason `adapter_failure` |

Bu deney guard'ların ölü olmadığını, kalıcı suite'in ayırt edici input taşımadığını
gösteriyor. Özellikle secret ve memory-bound korumaları gelecekte sessizce silinebilir.

### Önerilen düzeltmenin ölçümü

Geçici sekiz test değişikliksiz production kodunda `8/8 PASS` verdi. Her testin
hedeflediği tek guard kaldırıldığında karşılık gelen test `1/1 RED` verdi. Test ekleme
ürün davranışını değiştirmediği için over-refusal sayısı `0`; mevcut valid
`status:ok` pass-through testi de yeşil kaldı.

Kalıcı düzeltme bu ayırt edici testleri `internal/cli/agent_test.go` içine taşımalı;
secret testi unknown-field gibi daha erken bir guard tarafından maskelenmeyen geçerli
fallback shape kullanmalıdır.

### Class sweep

`jsonValueContainsSecret`, `maxAgentRouteInput`, `maxAgentRouteOutput`,
`boundedBuffer` ve `acceptableAdapterExit` yalnız `internal/cli/agent.go` içinde
bulundu. Aynı bridge doğrulamasının başka kopyası yok. Python adapter'ın eş guard
sınıfı ayrıca mutasyona tabi tutuldu ve `82/82` killed verdi; sorun adapter suite'ine
yayılmıyor.

## Altı zorunlu Breaker hareketi

### B1 — Mutation

- Embedded Python adapter: **82 killed / 0 survived**.
- Go bridge targeted mutations: **2 killed / 8 survived**; survivor'lar `BRK-002`.
- Harness mutasyonlarının elle doğrulaması: secret guard mutantı gerçekten guard'ı
  kaldırıp otherwise-valid raw JSON'u geçirdi; status mutantı inconsistent sonucu
  geçirdi; candidate ve failed-process mutantları derlenebilir biçimde tekrarlandı ve
  ilgili mevcut testleri kırdı.
- Makefile'da guard yoktu; en zayıf hedef şekilleri canlı recipe üzerinde B5 altında
  çalıştırıldı ve `BRK-001` üretildi.

### B2 — Silinen davranışın A/B'si

Base adapter:

```sh
git show 64e8f8a:.claude/skills/engineering-orchestrator/scripts/jev_route.py
```

ile embedded `internal/agent/jev_route.py` karşılaştırıldı.

| Ölçüm | Eski adapter | Embedded adapter |
| --- | --- | --- |
| SHA-256 | `41f50d…371` | `41f50d…371` |
| Byte comparison | — | eşit (`cmp` exit 0) |
| Offline suite | `45/45 PASS` | `45/45 PASS` |

Adapter taşıma sırasında davranış silinmedi. Repository-local executable dosyasının
silinmesi istenen güvenlik sonucudur; yerine binary-embedded byte-identical kaynak
gelmiştir.

### B3 — Yazılı threat model'i oynama

| Tehdit | Deney | Sonuç |
| --- | --- | --- |
| Repository-controlled `PATH/python3` | Fake interpreter PATH'in başına kondu; key-enabled bridge çağrıldı | fake execution marker `0`; trusted absolute Python kullanıldı |
| Key stdout/error sızıntısı | sentinel içeren invalid child output/error | bridge sanitized fallback; sentinel occurrence `0` |
| Masked secret-output guard | valid fallback reason içine sentinel; guard mutation A/B | kontrol `0`, mutant `1` sızıntı; `BRK-002` |
| Key request body | adapter offline opener capture | key body occurrence `0`, Authorization header occurrence `1` |
| Hostile/unknown/oversized provider output | adapter'ın malformed, redirect, oversized ve timeout testleri | ilgili fallback; 45/45 suite PASS |
| Hostile candidate choice | Go bridge unknown candidate mutant/control | kontrol reddetti; guard mutantı mevcut testi kırdı |
| Hassas context | managed metin ve docs | raw secret/log/source yasağı iki fresh/updated managed blokta aynı; otomatik arbitrary-secret sınıflandırması iddia edilmiyor |
| Install replacement/containment | dest directory ve parent symlink | iki kol da exit 0; `BRK-001` |

Bridge child'a yalnız process environment miras bırakıyor; sentinel key argv'de veya
stdin JSON'unda kullanılmadı. Python `-I -c <embedded source>` argv'sinde adapter
kaynağı var, key yok. Child stderr discard edildi; child hata metni kullanıcıya
yansıtılmadı.

### B4 — Gerçek upgrade path

Base binary ile scratch Git repository'sinde:

1. user-owned AGENTS prefix/suffix ve executable foreign hook yazıldı;
2. base `mindrail init` çalıştırıldı;
3. session ve `audit-survivor` task oluşturuldu;
4. knowledge dosyası eklendi;
5. yeni binary ile ilk `mindrail update`, ikinci `update` ve ardından `init`
   çalıştırıldı.

| Ölçüm | Önce | İlk update sonrası | İkinci update/init sonrası |
| --- | ---: | ---: | ---: |
| update exit | — | 0 | 0 / init 0 |
| workspace ID | `WS-01M3…GAES` | aynı | aynı |
| project ID | `PRJ-01M3…THAR` | aynı | aynı |
| açık task | 1 | 1 | 1 |
| config/knowledge/foreign-hook hash farkı | — | 0 | 0 |
| user AGENTS marker occurrence | 2 | 2 | 2 |
| managed `mindrail agent route` occurrence | 0 | 1 | 1 |
| duplicate outer `Optional Jev routing` | 0 | 0 | 0 |
| tekrar update byte farkı | — | — | 0 |
| tekrar update mtime/mode farkı | — | — | 0 |

Sonradan custom `project.name = "breaker-custom-project"` verilen config de update
sonrasında byte-identical kaldı. Uninitialized repo kontrolünde `update` exit 1,
`COORDINATION_UNAVAILABLE` ve filesystem diff `0` verdi; ardından aynı repo'da fresh
`init` başarıyla kuruldu.

### B5 — En zayıf satisfying input

- Make hedefi için en zayıf “var ama regular file değil” değer olan boş dizin,
  `mv` predicate'ini başarıyla geçti ve yanlış-success üretti (`BRK-001`).
- Install directory için en zayıf yönlendirme olan tek symlink, `DESTDIR` dışı yazı
  üretti (`BRK-001`).
- Key okuyucuları `""`, ASCII space, tab, NBSP, EM SPACE ve ZERO WIDTH SPACE ile
  dual çalıştırıldı. İlk beş hem Python hem Go'da `disabled/api_key_missing`; ZWSP
  her ikisinde `fallback/invalid_api_key` oldu. Bridge fail-open sözleşmesi gereği
  Python direct exit 2'yi kullanıcıya exit 0 fallback olarak çevirdi.
- `status:fallback/enabled:false` en zayıf inconsistent typed output'tu; kontrol
  reddetti fakat guard mutasyonu kalıcı suite'ten sağ çıktı (`BRK-002`).
- Input/output sınırlarının tam bir üstü scratch testlerinde reddedildi; guard
  mutasyonlarında sırasıyla `131,074` byte read ve `1,048,685` byte output görüldü.

### B6 — Dual readers

- Base adapter ve embedded adapter: aynı SHA ve aynı 45 test sonucu.
- Go/Python key reader matrisi: altı whitespace/Unicode girdisinde status/reason
  aynı.
- `init` ve `update`: aynı initialized disk üzerinde terminal state,
  `config_created`, migration sayısı ve managed bytes aynı; yalnız command identity
  doğru biçimde `init`/`update` ayrıldı.
- Source `AGENTS.md`, old-repo update ile üretilen managed blok ve fresh-init managed
  blok: her biri `2,973` byte; iki diff de `0`.
- `status` before/after workspace ve coordination cevapları aynıydı.

Dual-reader ayrışması üretilmedi.

## Cost

### JEV route üst sınırı

| Boyut | Üst sınır |
| --- | ---: |
| Go stdin read | `65,537` byte |
| Adapter raw input | `65,537` byte read; `65,536` kabul sınırı |
| Request body | `65,536` byte |
| Provider response | `262,144` byte |
| Go child stdout | `1,048,576` byte |
| Candidate fan-out | en çok 4 dimension × 32 = 128 candidate; toplam request bound ayrıca geçerli |
| Provider attempts | 1; retry yok |
| Whole-provider deadline | 10 saniye |

Eksik key kolu stdin okumuyor, Python başlatmıyor ve ağ isteği yapmıyor. En kötü
network maliyeti tek 64 KiB upload + 256 KiB response + 10 saniye. Adapter output
Go sınırından küçük olduğundan normal yolda 1 MiB bridge bound ikinci savunmadır.

### Update/install maliyeti

- Ölçülen update süreleri: ilk old-repo update `44 ms`, repeat update `25 ms`, aynı
  disk üzerinde init `15 ms`.
- `update` bir read-only preflight ve ardından tek ModeInit hattı çalıştırır; ağ yok.
- Kurulu binary `27,712,040` byte. Atomik replacement sırasında eski+yeni birlikte
  yaklaşık `55,424,080` byte disk gerektirir.
- Repeat install binary SHA'sı build timestamp nedeniyle değişti; bu REQ-003'teki
  tekrarlanabilir prosedürü bozmadı, fakat bit-for-bit reproducible build iddiası
  yapılmamalıdır.

Limit çarpımlarında ayrıca bir cost bulgusu üretilmedi.

## Mechanical rule = automated test

| Yazılı mekanik kural | Otomatik kanıt | Karar |
| --- | --- | --- |
| Missing key stdin/Python/network kullanmaz | panic-reader + no-launch test | var |
| Adapter closed candidates/secret/body/timeout bounds | 45 test, 82/82 mutation kill | var |
| PATH-controlled Python kullanılmaz | fake PATH testi + canlı marker deneyi | var |
| Update uninitialized repo'ya yazmaz | CLI test + canlı filesystem diff | var |
| Old state/hook/user AGENTS korunur | update/setup testleri + canlı old-repo A/B | var |
| Repeat update byte stable | CLI/setup testleri + byte/mtime probe | var |
| Go bridge key/output/input guards korunur | sekiz guard için kalıcı discriminating test yok | **BRK-002** |
| `make install` atomik/doğru hedefe kurar | Make recipe için otomatik hostile-target testi yok | **BRK-001** |

Raw source/log/context içermeme kuralı içerik-sınıflandırma bakımından insana ait bir
privacy kuralıdır; sistem yalnız exact TypeSafe key'i mekanik olarak engelliyor ve
docs daha geniş bir otomatik secret detector iddia etmiyor.

## Backward compatibility

Üretilen uyum matrisi:

| Existing kurulum | Sonuç |
| --- | --- |
| Base binary schema-10 DB + workspace identity | update kabul, identity aynı |
| Existing session/task | 1 task önce/sonra |
| Default config | byte-identical |
| Custom `project.name` config | byte-identical |
| Existing knowledge file | byte-identical |
| User AGENTS prefix/suffix + old managed block | user bytes korundu, managed blok yenilendi |
| Foreign hook + `.mindrail-original` | byte-identical |
| Uninitialized repo | update reddi, 0 filesystem diff, init remedy |
| Missing/blank key | disabled, normal flow |
| Missing interpreter (injected bridge seam) | `python_unavailable` fallback |

Bu kombinasyonlarda compatibility kırığı üretilmedi. `make install` hedefinin önceden
dizin olması veya staging altında symlink bulunması ise eski kurulum verisi değil,
deployment-target compatibility/security kırığı olarak `BRK-001`'de yer alıyor.

## Çalıştırılan doğrulama özeti

```text
python -B -m unittest discover -s internal/agent -p test_jev_route.py -v
Ran 45 tests — OK

go test ./internal/agent ./internal/cli -run TestAgent -count=1
PASS

go test ./internal/cli ./internal/status -run 'Test(Update|CommandSurface|RootHelpShows|Init)' -count=1
PASS

go test ./internal/setup -count=1
PASS

Python adapter mutation: 82 killed / 0 survived
Go bridge targeted mutation: 2 killed / 8 survived
```

İlk `make install` denemesi sandbox'ın read-only default Go cache'i nedeniyle ürün
koduna ulaşmadan başarısız oldu; bütün raporlanan install ölçümleri explicit scratch
`GOCACHE` ile yeniden çalıştırılan kollardır.

## Açık maddeler

- Gerçek TypeSafe endpoint'i disposable credential ile çağrılmadı. Bunu sonuçlandırmak
  için revoke edilebilir ayrı bir audit key, dış ağ izni ve request-capture proxy'si
  gerekir. Offline HTTP contract ve failure yolları tamamen çalıştırıldı; bu eksik,
  iki bulgunun varlığını değiştirmiyor.
- Parent-component symlink zincirinin bütün derinlikleri için nihai install düzeltmesi
  uygulanmadığından yalnız doğrudan `BINDIR` symlink prototipi ölçüldü. Nihai çözümden
  sonra her path component'i ve concurrent target replacement ayrı scratch testinde
  tekrar denenmelidir.

## Nihai Breaker kararı

`BRK-001` gerçek runtime/deployment davranış hatasıdır. `BRK-002`, secret ve resource
guard'larını sessiz regresyona açık bırakan üretilmiş test boşluğudur. İkisi
giderilmeden sonuç **REJECT** kalır.

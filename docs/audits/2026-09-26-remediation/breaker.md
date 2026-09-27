# Bağımsız Breaker raporu — 2026-09-26

## Görev, bağımsızlık ve sınır

Özgün görev: **“o zaman bu sorunların hepsini düzeltmeni istiyorum”**.
Kapsam `audit-plan.md` ve `docs/engineering/remediation-2026-09-26.md` içindeki
REQ-001…008; base `2814acb22d8aa94d48ade1242b58e1fdb14239fc`.

Paralel bağımsız alt ajan modu kullanıldı. Bu denetçi uygulamayı yazmadı ve bu
rapor yazılmadan Reader sonuçlarını okumadı. B1–B6'nın tamamı çalıştırıldı.
Üretim kaynaklarına veya gerçek kullanıcı verisine deneysel değişiklik yapılmadı.
`RTK.md` bu çalışma ağacında yoktu; AGENTS.md, CLAUDE.md, audit planı, dondurulmuş
gereksinimler ve dual-agent-task-audit Breaker protokolü okundu.

Deney ortamları:

- Adayın bağımsız kopyası: `/tmp/mindrail-breaker-Gi42Mm`.
- Gerçek base commit arşivi: `/tmp/mindrail-breaker-Gi42Mm-old`.
- Tekil mutasyon kopyaları: `/tmp/mindrail-breaker-Gi42Mm-mutants/<ad>`.
- Sadece öneri ölçümü için kopya: `/tmp/mindrail-breaker-Gi42Mm-fix`.
- Ayrı derleme önbelleği: `GOCACHE=/tmp/mindrail-audit-gocache`.
- Tekrarlanabilir ek deneyler: aday kopyadaki `internal/{validation,completion,cli,mcp}/breaker*_test.go`.
- Toplu deney çıktısı: `/tmp/mindrail-breaker-Gi42Mm/probe-results.log`.

Son kontrolde aday kopya ile gerçek ağacın `internal/`, `cmd/`, `migrations/`
dizinleri `rsync -rcni` ile karşılaştırıldı; yalnız scratch deney dosyaları hariç
tutuldu, fark çıktısı **0 satırdı**. Bu rapor dışında ortak ağaca yazılmadı.
Hiç commit oluşturulmadı; paylaşılan servis kullanılmadı.

## Sonuç

Ana dört kapanış açığı için başarısızlık yeniden üretilemedi: başarısız/karma
validation artık kanıt sayılmıyor, dosya ekleme kanıtı bayatlatıyor, bozuk knowledge
kapanışı reddediyor ve protokol atlanınca Git değişiklikleri bulunuyor. Gerçek
binary 13 MCP aracı sunuyor; ham COMPLETED reddediliyor; güncel uygulama kapanış
sırasında değişen task revision'ını reddediyor.

Buna karşılık **beş MEDIUM bulgu** üretildi: glob kapsamının dışındaki erişim
hatası, yarım validation işleminin tekrarında run kimliği, değişmiş required
listesinin başarılı replay sayılması, boş required girdisinin CLI'de yok olması
ve iki güvenlik koşulunun mevcut testlerden sağ çıkması. Üretilmiş HIGH veya
CRITICAL çalışma zamanı kusuru yok. Tam ve birebir görev-tamamlama iddiası için
Breaker kararı **REJECT**; bu karar aşağıdaki sınırlı ve ölçülmüş eksiklere dayanır,
ana düzeltmelerin başarısız olduğu iddiasına dayanmaz.

## Çalıştırılan kontroller

Başlangıçtaki, ek Breaker testleri henüz eklenmemiş aday:

```sh
GOCACHE=/tmp/mindrail-audit-gocache go test -count=1 \
  ./internal/validation ./internal/gate ./internal/completion ./internal/changes \
  ./internal/migration ./migrations ./internal/mcp ./internal/cli ./cmd/mindrail
```

**PASS — 9 paket.** validation 0.863 s, gate 0.329 s, completion 1.744 s,
changes 3.625 s, migration 0.988 s, migrations 0.176 s, MCP 15.624 s,
CLI 48.141 s, gerçek binary test paketi 1.995 s. İlk varsayılan GOCACHE denemesi
salt-okunur dizin nedeniyle derleme öncesinde reddedildi; yukarıdaki çalışma bunu
gideren, gerçek test çalışmasıdır.

Son ek deney komutu:

```sh
cd /tmp/mindrail-breaker-Gi42Mm
GOCACHE=/tmp/mindrail-audit-gocache go test -count=1 -v \
  ./internal/validation ./internal/completion ./internal/cli ./internal/mcp \
  -run '^TestBreaker'
```

Bu komut beklenen davranışı isteyen yeni deneyler nedeniyle **FAIL**; aşağıdaki
bulguların yeniden üretimidir. Adayın başlangıç test paketinin başarısız olduğu
şeklinde sunulmamalıdır. `make verify`, `make gate`, benchmark ve release sonuçları
audit planındaki ana doğrulama kanıtına aittir; bu Breaker bunları bağımsız olarak
yeniden çalıştırdığını iddia etmez.

## B1 — Koruyucu koşul mutasyonları

Semantic guard sınıfı başına örnekleme, audit planının izin verdiği şekilde yapıldı.
Bir kopyada yalnız ilgili koşul etkisizleştirildi. Shipped testler, ek Breaker
testleri eklenmeden çalıştırıldı. Her iki survivor'a da ayırt edici girdi verildi.

| Mutasyon | Çalıştırılan paketler | Sonuç / belirleyici test |
| --- | --- | --- |
| `row.Status != StatusPass || row.ExitCode != 0` kaldırıldı | validation | KILLED — `TestProfileRunMustBeWhollySuccessful` |
| Beklenen komut sayısı/indeks tamlığı kaldırıldı | validation | KILLED — eksik komut satırı |
| Deklaratif yeniden tarama yerine eski somut dosya listesi hash'lendi | validation | KILLED — dizin ve recursive glob içine eklenen dosya |
| Knowledge loader problem reddi kaldırıldı | completion, MCP | KILLED — unsupported, invalid JSON, missing version, unreadable record |
| Knowledge schema finding reddi kaldırıldı | completion, MCP | KILLED — `invalid_schema` |
| Completion'ın canonical reconcile çağrısı etkisizleştirildi | completion, MCP | KILLED — skipped protocol, declared edit, başka task diff'i |
| Baseline transaction'daki eski mapping union'ı kaldırıldı | completion, MCP | KILLED — disjoint concurrency ve tekrarlanan guard değerlendirmesi |
| `head != b.HeadOID` reddi kaldırıldı | completion | KILLED — HEAD hareketi |
| İlk capture öncesi indeks/HEAD hash karşılaştırması kaldırıldı | completion | KILLED — önceden değişmiş indeksin capture edilmesi |
| Ham `task state --to COMPLETED` reddi kaldırıldı | CLI | KILLED — task/lease koruma testi |
| Gate DENY dalı kaldırıldı | CLI | KILLED — eksik required kanıtı |
| Stdio server `Run` çağrısı kaldırıldı | cmd/mindrail | KILLED — gerçek SDK subprocess initialize çağrısında EOF |
| Güncel knowledge policy yerine kaydedilmiş mapping'lere sürekli CRITICAL/active verildi | completion, MCP | KILLED — CRITICAL→LOW güncel policy kontrolü |
| Son transition ve replay'de `expect` yerine `0` verildi | CLI | **SURVIVED** — bütün mevcut CLI paketi PASS; BR-05A |
| Kaydedilmiş mapping'in boş zorunlu alan reddi kaldırıldı | completion, changes, MCP | **SURVIVED** — üç paket PASS; BR-05B |

Toplam: **15 semantic mutasyon; 13 killed, 2 survivor.** CAS'ın yalnız son çağrısı
ilk başta değiştirildiğinde replay request hash'i farklılaştığı için test kırıldı;
bu, revision korumasının testi değildi. Her iki completion çağrısı tutarlı biçimde
değiştirildiğinde paket yeşil kaldı ve gerçek ayırt edici revision deneyi yapıldı.
Stdio mutantının ilk çalışması Git metadata olmayan kopyada Go VCS stamping
hatasına takıldı; bu sonuç sayılmadı. `GOFLAGS=-buildvcs=false` ile yeniden
çalıştırıldığında binary gerçekten başlatıldı ve initialize EOF'si üretildi.

## B2 — Silinen davranışın gerçek base ile A/B ölçümü

Base kaynakları `git archive <base>` ile ayrı dizine açıldı. Aynı
`TestBreakerABDeletedBehavior` / `TestBreakerRawAB` test kaynakları ve aynı
fixture verisi iki sürümde çalıştırıldı; uygulama içi mock sonuç kullanılmadı.

| Girdi / ölçüm | Base | Aday |
| --- | ---: | ---: |
| Başarılı tek komut — required satisfied | 1 | 1 |
| Başarısız tek komut — required satisfied | 1 | 0 |
| PASS+FAIL iki komut — required satisfied | 1 | 0 |
| Çalıştırmadan sonra scope içine dosya ekleme — satisfied | 1 | 0 |
| Boş literal dizin, başarılı komut — satisfied | 0 | 0 |
| Ham COMPLETED komutunun exit code'u | 0 | 1 |
| Ham komuttan sonra revision | 5 | 4 |
| Ham komuttan sonra terminal task sayısı | 1 | 0 |

İstenen eski davranış kaldırılmıştır. Boş literal dizin problemi bu nedenle yeni
regresyon olarak raporlanmadı; yeni glob desteğinin aynı uyumsuzluğu taşıması BR-01
içinde ayrı sınırlamayla belirtilmiştir.

## B3 — Yazılı tehditlerin denenmesi

Başlangıç paketlerinde şu canlı testler çalıştı: failed/error/mixed/missing-row
validation; directory/glob add/edit/remove ve dışarıdaki dosya kontrolü;
unsupported/invalid/unreadable knowledge ve sunucu yeniden başlatma; before/after
çağrılarını atlama; declared change + after-change atlama; başarısız Git erişimi;
korunan testi kaldırma; explicit after-change/reconcile sonrası tekrar; boş
baseline'dan sonra binding ekleme; knowledge CRITICAL→LOW; testin geri konması.
Guard denials aynı sunucuda tekrar ve yeni server instance'ında korundu. Genel
kontrol, eski kanıtı koruyan baseline ile ALLOW üretmeye çalışan bu senaryolarda
başarılı oldu. Mutasyonların bunları kırması B1 tablosunda ayrı ölçüldü.

Gerçek compiled binary testinde 13 tool listelendi ve `mindrail_bootstrap`
çağrısı başarılı oldu. SDK stdio parsing başarılı olduğundan normal startup
stdout'unu bozan bir log üretilmedi; kaldırılmış `Run` mutantı EOF verdi.

## B4 — Eski veri ile upgrade

`TestBreakerEightToNineWithOldRows`: mevcut çalışma alanı/task/session ile
schema-8 şekli oluşturuldu; evidence satırı yeni provenance writer'ından
geçmeden, eski `{"scope":[]}` biçiminde doğrudan saklandı. Yalnız migration-9
nesnesi/ledger kaydı yoktu. İlk `mindrail init` migration 9'u uyguladı.

| Ölçüm | Önce | Sonra |
| --- | ---: | ---: |
| Schema version | 8 | 9 |
| Aynı ID ve output taşıyan eski evidence satırı | 1 | 1 |
| Önceden açılmış task görünür | 1 | 1 |
| Guard baseline satırı | tablo yok | 0 |

Legacy evidence `Check` tarafından conservative stale olarak reddediliyor;
tablo veya geçmiş silinmiyor. Güncel writer'la yeni run çözüm yolu. Eski indeks
HEAD'den sonra değiştirilmiş ve baseline yoksa güvenli capture reddi mevcut
testte üretildi. Unborn HEAD, boş baseline, sekiz eşzamanlı capture, araya giren
iki ayrık mapping setinin monoton union'ı ve sonraki HEAD için ayrı snapshot
başlangıç suite'inde çalıştı. Migration sırasında veri kaybı üretilemedi.

## B5 — En zayıf kabul edilen girdiler

| Koşul / minimum girdi | Üretilen sonuç |
| --- | --- |
| PASS/exit-0, ama `command_count=2` ve yalnız bir indeks | Satisfied=0 |
| Eksik run metadata / legacy provenance | Satisfied=0 |
| `scope_paths` dolu, glob eşleşmesi 0, writer'ın `scope:null` değeri | Başarılı run bile satisfied=0; BR-01 |
| Dolu glob declaration, ama erişilemeyen dizin glob'un dışında | Satisfied 1→0; BR-01 |
| İlk komut eski operation-id replay'i, ikinci komut yeni kayıt | Her ikisi PASS; satisfied 1→0; BR-02 |
| `operation != ""`, task COMPLETED, required listesine tek yeni profil | `allow:true`, replayed=true; BR-03 |
| CLI `--required ""` | MCP reddederken CLI COMPLETED; BR-04 |
| Baseline `[]` | Geçerli boş snapshot; kabul |
| Canonical JSON ama mapping'in dört alanı da `""` | Aday ret, boş-alan mutantı kabul; BR-05B |
| Task revision evaluation sırasında 4→5 | Aday ret, CAS mutantı COMPLETED revision 6; BR-05A |

## B6 — Aynı girdiye birden fazla okuyucu

`TestBreakerSameProfileReaders`, gerçek PASS evidence'i **`İ` adlı profile**
bağladı; yani olumlu kontrolün gerçekten mevcut bir mapping'i var. Aynı required
değerleri `validation.Check`, MCP completion ve CLI completion'a verildi.

| Required | validation | MCP | CLI |
| --- | ---: | ---: | ---: |
| `İ` | 1 | 1 | 1 |
| `i`, `ı`, ` İ`, `İ `, `İ`+NBSP, NFD `próof` | 0 | 0 | 0 |
| Boş string | 0 | 0 (COMMAND_LINE_INVALID) | **1** |

Case/whitespace/NFD negatif kontrollerinde ayrışma yok. Boş string ayrışması
BR-04'tür. Ayrıca canonical reconcile'ı implicit/explicit kullanan iki completion
yolunun aynı denial döndürdüğünü mevcut MCP regression testi; index capture ve
evaluation'ın aynı `changes.IsVerificationTest` sınıflandırmasını kullandığını
call-site taraması doğruladı.

## Bulgular

### BR-01 — MEDIUM: glob yeniden taraması kapsam dışındaki erişim hatasından etkileniyor

**REQ-002; güven %100.** Dosya: `internal/validation/snapshot.go:66`.
`tests/**/*.py` için bir PASS satırı oluşturuldu. Kontrolde bütün dosyalar
okunabilir. Senaryoda yalnız `outside/private` dizininin modu `000` yapıldı;
scope içindeki dosya sayısı ve içerik aynı kaldı. `WalkDir(root)` match kararına
gelmeden ilgisiz alt dizin hatasını döndürüyor.

| Ölçüm | Kontrol | Senaryo | Scratch öneri sonrası kontrol / senaryo |
| --- | ---: | ---: | ---: |
| Scope içi dosya sayısı | 1 | 1 | 1 / 1 |
| Başarılı komut exit code | 0 | 0 | 0 / 0 |
| Required satisfied | 1 | **0** | 1 / 1 |

Komut: `go test -count=1 -v ./internal/validation -run '^TestBreakerGlobOutsideUnreadable$'`.
Yanlış refusal completion'ın gereksiz yere kapanmasını engelliyor; REQ-002'nin
kapsam dışı değişiklik sözleşmesi bozuluyor. Öneri: yalnız glob'a ulaşabilecek
dizinlere inmek, ilgisiz dizinleri baştan budamak. Scratch'te literal glob önekiyle
walk root'unu sınırlamak bu iki kolu düzeltti; genel glob budamasının üretim için
tüm pattern şekilleriyle ayrıca tasarlanması gerekir.

Ek en-zayıf-girdi ölçümü: yeni `empty/**/*.py` declaration'ı geçerli kabul
ediliyor, `true` exit 0 dönüyor, fakat writer `scope:null` yazdığı için kanıt
anında stale oluyor. Bir dosyalı kontrol satisfied=1, sıfır dosyalı scenario=0.
Scope slice'ını `[]` olarak oluşturmak scratch'te 1/1 sonucunu verdi; sonraki
dosya eklemenin hâlâ stale ürettiği bütün validation paketiyle doğrulandı.
Boş literal dizin aynı sorunu base'de de taşıdığından o eski kusur ayrıca sayılmadı.

**Class sweep:** `WalkDir(root` ve `SnapshotScope` kullanımları tarandı. Bu
freshness/glob yolu tek üretim implementation'ı; MCP ve gate aynı checker'ı
paylaşıyor. Başka subsistemlerdeki inventory walk'ları farklı kapsam taşır ve
bu bulgunun kopyası olarak sayılmadı.

### BR-02 — MEDIUM: yarım kalan operation tekrarında başarılı komutlar farklı run'lara ayrılıyor

**REQ-001/008; güven %99.** Dosya: `internal/validation/service.go:49`.
İki `true` komutlu profile `operationID=resume` ile kayıt üretildi. Kontrolde iki
satır mevcut. Senaryoda ikinci satır doğrudan silinerek ilk komut kaydedildikten
sonra kesilen operation'ın kalıcı şekli kuruldu; aynı operation yeniden çalıştı.
`resume#0` eski run ID ile replay oluyor, `resume#1` yeni run ID taşıyor. Her run
kendi `command_count=2` değerini tamamlayamıyor.

| Ölçüm | Kesintisiz kontrol | Yarım operation tekrarı | Scratch öneri sonrası tekrar |
| --- | ---: | ---: | ---: |
| PASS/exit-0 komut satırı | 2 | 2 | 2 |
| Logical run ID sayısı | 1 | **2** | 1 |
| Required satisfied | 1 | **0** | 1 |

Komut: `go test -count=1 -v ./internal/validation -run '^TestBreakerPartialOperationReplay$'`.
Bu bir fail-open değil; idempotent recovery bozulmasıdır. MCP bugün boş operation
ID kullandığı için doğrudan MCP kullanıcısının olağan akışı bu yoldan etkilenmez;
`RunProfile`'ın mevcut idempotency sözleşmesi etkilenir. Yeni operation ID ile tam
rerun bir geçici çözüm.

Öneri: replay edilen ilk evidence satırının run kimliğini kalan komutlarda korumak
ve metadata uyumunu doğrulamak. Scratch'te ilk kaydın provenance RunID'sini
devam ettirmek iki kolu düzeltti; bütün validation paketi PASS, fail/mixed/add
olumsuz kontrolleri hâlâ satisfied=0.

**Class sweep:** `RunProfile`, `commandOp`, `identity.NewID("VRN")` tarandı.
Tek run-ID writer bulundu; `Store.Record` satır replay'i doğru yapıyor, kopukluk
onu yeni run grouping ile birleştiren service katmanında.

### BR-03 — MEDIUM: required listesi değişse de completion operation replay ALLOW döndürüyor

**REQ-006; güven %100.** Dosya: `internal/cli/task.go:92` ve `:123`.
Temiz READY task, `--operation-id once` ve revision 4 ile required'sız tamamlandı.
Aynı istek düzgün replay oldu. Sonra aynı isteğe yalnız
`--required missing-proof` eklendi. Beklenen, farklı request için
`OPERATION_ID_CONFLICT`; üretilen sonuç `allow:true`, `replayed:true`, exit 0.
Transition request hash'ine required listesi hiç dahil edilmiyor.

| Ölçüm | Aynı istek replay | Yeni required eklenmiş replay | Scratch öneri sonrası aynı / değişmiş |
| --- | ---: | ---: | ---: |
| Exit code | 0 | **0** | 0 / 1 |
| `allow:true` cevap sayısı | 1 | **1** | 1 / 0 |
| Yeni required için evidence satırı | 0 | 0 | 0 / 0 |

Komut: `go test -count=1 -v ./internal/cli -run '^TestBreakerCompletionReplayRequiredContract$'`.
Bu, ilk tamamlamanın geriye dönük geçersiz olduğu iddiası değildir: yeni isteğin
eskisiymiş gibi onaylanmasıdır. Öneri: task completion'a özel idempotency payload'ı
required profilleri de taşımalı. Scratch ölçümünde serialized required listeyi
transition hash girdisine katmak identical replay'i korudu, değiştirilmiş isteği
conflict yaptı. Ölçüm amacıyla reason alanı kullanıldı; bu bir üretim tasarım
önerisi değildir, ayrı completion request sözleşmesi tercih edilmelidir.

**Class sweep:** iki completion TransitionExpecting çağrısı aynı boş reason'ı
geçiyor. Başka CLI completion writer'ı yok. Coordination'ın kendi by/task/to/reason/
revision alanlarını hash etmesi bu yeni parametreyi kapsamaz.

### BR-04 — MEDIUM: açıkça verilen boş required profil CLI'de sessizce yok oluyor

**REQ-006/008; güven %100.** Dosya: `internal/cli/task.go:73` ve `:138`.
Örneğin `--required "$PROFILE"` ve boş PROFILE, MCP'nin `required:[""]`
girdisiyle aynı niyeti taşır. CLI flag okuması bunu sıfır profile indiriyor ve
temiz task'ı tamamlıyor; MCP `COMMAND_LINE_INVALID` döndürüyor. Boş olmayan
gerçek `İ` profili üç okuyucuda da olumlu kontrol olarak çalıştı.

| Ölçüm | Required=`İ` | Required=boş | Scratch öneri sonrası boş |
| --- | ---: | ---: | ---: |
| MCP allow | 1 | 0 | 0 |
| CLI allow | 1 | **1** | 0 |
| CLI terminal task | 1 | **1** | 0 |

Komut: `go test -count=1 -v ./internal/mcp -run '^TestBreakerSameProfileReaders$'`.
Öneri: flag açıkça verildiğinde boş listeyi/boş üyeyi komut başlamadan reddetmek.
Yalnız StringSlice→StringArray değiştirme denemesi ayrışmayı gidermedi; bu da
ölçüldü ve başarılı çözüm diye sunulmadı. `Flags().Changed("required")` ile boş
girdiyi ayrıca reddeden scratch denemesi sekiz parity kolunun tamamını geçirdi;
geçerli `İ` kontrolü ALLOW kaldı.

**Class sweep:** required parametresini CLI'de dönüştüren tek yol bu komut.
MCP doğrudan string slice alıyor; ortak `EvidenceForProfile` zaten boş adı
reddediyor. Kaybolan girdi ortak service'e hiç ulaşmıyor.

### BR-05 — MEDIUM: iki safety guard shipped testlerde etkisizleştirilebiliyor

**REQ-008; güven %100.** Aday kodun bu iki koşulu bugün doğru çalışıyor; bulgu
çalışma zamanı bypass'ı değil, üretilmiş regresyon-test açığıdır.

**A — Son revision CAS.** Dosya `internal/cli/task.go:93,123`.
İki completion TransitionExpecting çağrısında `expect`→`0` mutasyonu bütün mevcut
CLI paketini **35.340 s PASS** bıraktı. Ayırt edici girdi, gate'in ilk baseline
insert'inde task revision'ını yükselten disposable SQLite trigger'ıdır:

```sql
CREATE TRIGGER breaker_concurrent_revision AFTER INSERT ON guard_baselines
BEGIN UPDATE tasks SET revision = revision + 1; END;
```

| Ölçüm | Aday, race yok | Aday, race var | CAS mutantı, race var |
| --- | ---: | ---: | ---: |
| İlk revision | 4 | 4 | 4 |
| Son revision | 5 | 5 | **6** |
| Exit code | 0 | 1 | **0** |
| COMPLETED sayısı | 1 | 0 | **1** |

`TestBreakerConcurrentRevisionDuringCompletion` adayda PASS, mutantta FAIL.
Trigger bir üretim fixture'ı değil; iki aşama arasındaki dış revision yazısını
deterministik biçimde modelleyen disposable girdidir. Maskelenmiş guard değildir:
ilk precheck geçiyor ve yalnız final CAS'ın reddedebileceği veri oluşuyor.

**B — Durable mapping completeness.** Dosya
`internal/changes/guard_baseline.go:156`.
Boş `Path/Test/ProductionUID/InvariantID` reddi kaldırılınca mevcut completion,
changes, MCP paketleri **2.315 / 5.681 / 15.734 s PASS** kaldı. Doğrudan saklanan
canonical `[ {"path":"","test":"","production_uid":"","invariant_id":""} ]`
(testte boşluksuz) satırı yeni writer'ın üretemeyeceği minimum girdidir.

| Ölçüm | Geçerli `[]`, aday | Boş alanlı satır, aday | Boş alanlı satır, mutant |
| --- | ---: | ---: | ---: |
| Kabul | 1 | 0 | **1** |
| Dönen mapping sayısı | 0 | 0 | **1** |

`TestBreakerIncompletePersistedMapping` adayda PASS, mutantta FAIL. Öneri ve
ölçülmüş test çözümü, bu iki ayırt edici regresyon testini kalıcı suite'e eklemek.
Üretim guard'ını değiştirmeye gerek yok.

**Class sweep:** `TransitionExpecting`, baseline decoder, tüm task discovery
call-site'ları tarandı. Raw transition, DENY, loader/schema, HEAD, eski indeks,
monoton union ve live policy korumaları örneklenerek kırıldığında suite kırıldı.
Bu iki survivor birleştirilmiş test-coverage bulgusudur; diğer guard'ların da
testsiz olduğu ileri sürülmüyor.

## Reddedilen iddialar ve açık işler

| İddia | Çürüten üretilmiş kanıt |
| --- | --- |
| Boş literal dizin yeni regresyondur | Gerçek base ve adayda satisfied=0 |
| Aday final task CAS'ını atlıyor | Gate sırasında revision 4→5 deneyinde exit 1, READY korunuyor |
| Migration 9 eski evidence/task verisini kaybediyor | 8→9, evidence 1→1, task görünür 1→1 |
| Bütün glob dışı değişiklikler evidence'i bozuyor | Normal outside-file ve glob'a uymayan dosya kontrolü mevcut suite'de current; hata erişilemeyen subtree'ye özgü |
| Başlangıç stdio mutation failure'ı launcher guard testidir | İlk çıktı yalnız Go VCS build hatasıydı; sayılmadı, gerçek subprocess EOF ile yeniden üretildi |

Reader'ın refutation turu bu bağımsız raporun **ardından** yapılmalıdır.
Raporu yazarken Reader bulguları görülmedi; onun adına onay/refutation yazılmadı.
Üretilmemiş bir kusur lead'i devredilmiyor. Parent'ın tam `make verify/gate/bench/
release` yeniden çalıştırmasının sonucu final rapora ayrıca bağlanmalıdır.

## Zorunlu ek başlıklar

| Başlık | Sonuç / hesap |
| --- | --- |
| Cost | MCP validation için komut başına timeout 600 s. N komut için üst sınır `N × 600 s`; iki stream için `N × 2 × 65,536 = 131,072N` yakalanan ham byte (+ truncation marker). N'ye konfigürasyon limiti yok. Freshness E evidence satırı × P glob × F repo entry taraması; hashing her row için tekrarlanıyor. Sonlu global run bütçesi yok. Bunlar audit planında açıkça dışarıda bırakılmış gelecekteki runtime-budget işi; yeni in-scope performans bulgusu diye genişletilmedi. Dış model çağrısı veya model tier escalation eklenmemiş. |
| Mechanical rule = automated test | 13-tool, schema shape/version, format/vet, migration, stdout, lease/state, discovery-call inventory ve temel proof kuralları testlerle korunuyor. Final CAS ve saved mapping'in minimum içerik kuralında iki gerçek mutation survivor: BR-05. |
| Class sweep | Her bulgunun altında grep şekli, paylaşılan okuyucular ve aynı sınıfın diğer örnekleri belirtildi. |
| Backward compatibility | Schema-8 evidence ve task korunuyor; legacy proof'un stale olması dondurulmuş karar. Raw COMPLETED'ın reddi istenmiş breaking behavior. Normal nonterminal lifecycle iki sürümde geçti. Partial operation replay BR-02; yeni completion request replay kapsamı BR-03. |

## Review Summary

| Severity | Count | Status |
| --- | ---: | --- |
| CRITICAL | 0 | pass |
| HIGH | 0 | pass |
| MEDIUM | 5 | info |
| LOW | 0 | note |

**Breaker: REJECT** — ana remediation kontrolleri çalışıyor, fakat bu bağımsız
turda üretilmiş beş MEDIUM eksik nedeniyle “tam ve birebir tamamlandı” sonucu
verilemiyor. Bu, kritik güvenlik blokajı sınıflandırması değildir. Parent,
bulguları refutation/fix ölçümüyle kapatmalı veya açık minor issue olarak nihai
teslimde açıkça sınıflandırmalıdır.

# Mindrail 0.1 — Uygulama Görev Listesi

Bu liste aşağıdaki belgelerden türetilmiştir:

- `docs/specification/mindrail-0.1-kernel-scope.md`
- `docs/specification/mindrail-technical-specification-1.0.md`
- `docs/specification/mindrail-tech-stack.md`

Liste, **0.1 Kernel** kapsamını esas alır. Her görev tek başına alınabilir, uçtan uca doğrulanabilir bir tracer-bullet dilimidir. Teknik Şartname 1.0'daki fakat 0.1 dışında kalan özellikler listenin sonunda açıkça ertelenmiştir.

## Kullanım

- Durum kutusu: `[ ]` bekliyor, `[x]` tamamlandı.
- `AFK`: Belgelerde yeterli karar bulunduğu için insan müdahalesi olmadan uygulanabilir.
- `HITL`: İnsan tarafından ürün/kabul değerlendirmesi gerekir.
- Görevler bağımlılık sırasına göre numaralandırılmıştır; bağımsız işler aynı dalga içinde paralel yürütülebilir.

## Bağımlılık Dalgaları

| Dalga | Görevler | Çıktı |
|---|---|---|
| 1 | MR-001, MR-002 | Çalışan yerel binary ve kalıcı bilgi temeli |
| 2 | MR-003, MR-004, MR-005 | Koordinasyon, eşzamanlılık ve yapısal indeks |
| 3 | MR-006, MR-007, MR-008 | Kalıcı sembol kimliği ve gerçek değişiklik keşfi |
| 4 | MR-009, MR-010 | Açıklanabilir etki ve güncel kanıt üretimi |
| 5 | MR-011, MR-012, MR-013 | Kanıt geçersizleştirme, test guard ve completion gate |
| 6 | MR-014, MR-015, MR-016 | 13 MCP aracının ortak servisler üzerinden sunulması |
| 7 | MR-017, MR-018, MR-019 | Git/CI enforcement ve performans kalite kapısı |
| 8 | MR-020 | 0.1 çıkış koşulunun insan onayı |

---

## Görevler

### [x] MR-001 — Yerel repository bootstrap ve tanılama yolu

- **Tür:** AFK
- **Blocked by:** None — hemen başlanabilir.
- **Kapsadığı senaryo:** Kullanıcı bir Git deposunda bulut servisi olmadan Mindrail'i başlatır ve çalışma durumunu görebilir.

#### Ne yapılacak

Tek bir Go binary içinde `mindrail init`, `mindrail status` ve `mindrail doctor` akışlarını uçtan uca sun. Akış; Git repository/common-dir/worktree keşfini, platforma uygun runtime path seçimini, SQLite veritabanı oluşturmayı, gömülü migration çalıştırmayı, `.mindrail/knowledge` yüklemeyi ve hem insan-okunur hem yapılandırılmış hata/JSON çıktısını kapsamalıdır.

#### Kabul kriterleri

- [x] `mindrail init` bir Git deposunda ağ veya bulut bağımlılığı olmadan başarılı olur.
- [x] İlk çalıştırma SQLite veritabanını ve gömülü migration'ları idempotent biçimde oluşturur.
- [x] Git common-dir ile aktif worktree doğru ve açıklanabilir biçimde raporlanır.
- [x] `status` ve `doctor` eksik/bozuk kurulumları yapılandırılmış hata ve önerilen `next_action` ile bildirir.
- [x] CLI sözleşme testleri ve temiz binary smoke testi bulunur.

#### Durum

Tamamlandı. `make verify` yeşil (gofmt, `go vet`, 1605 test, race detector,
3 temiz-binary smoke testi). Tasarım [mr-001-design.md](mr-001-design.md).

Dördüncü kriter, yedi denetim turundan artakalan 17 bulgunun tamamı kapatıldıktan
sonra işaretlendi. Üç HIGH'ın üçü de yapay ortamda birebir reprodüksiyonlarıyla
doğrulandı: `init` artık kendi son yazma işleminden *sonra* okuma yapıyor, bir
boyut sınırı dolu diskle karıştırılmıyor, okunamayan `config.toml` uygulanabilir
bir çare basıyor.

Kapatma turu da bağımsız olarak denetlendi, o denetimin remediasyonu da. Beş
denetçi, 45 bulgu daha: yedisi düzeltmelerin kendi getirdiği kusurlar, üçü bulgu
belgesindeki yanlış "kapandı" iddiaları, ve ikinci turun tamamı "hiçbir testin
düşemediği" için yazılan korumaların kendilerinin de düşemez olması. Biri açık
bırakıldı ve belgede adıyla kayıtlı. Her bulgunun ne olduğu, neyle kapandığı ve
geri gelirse hangi testin düşeceği — düşecek bir test yoksa bunun neden böyle
olduğu — [mr-001-findings.md](mr-001-findings.md) içinde.

---

### [x] MR-002 — Sürümlü Decision/Invariant bilgi yaşam döngüsü

- **Tür:** AFK
- **Blocked by:** MR-001
- **Kapsadığı senaryo:** Bir Decision veya Invariant process/session yeniden başlatıldıktan ve yeni clone alındıktan sonra doğrulanabilir biçimde yaşamaya devam eder.

#### Ne yapılacak

`.mindrail/knowledge` altında sürüm kontrollü JSON kayıtları için Decision ve Invariant oluşturma, okuma, doğrulama ve supersede yaşam döngüsünü tamamla. Her kayıt `schema_version: 1` taşımalı; bozuk JSON, bilinmeyen daha yeni şema ve supersede cycle fail-closed davranmalıdır.

#### Kabul kriterleri

- [x] Decision ve Invariant kayıtları yeniden başlatma sonrası kaybolmaz.
- [x] Temiz clone yalnızca repository içeriğiyle knowledge kayıtlarını doğrular.
- [x] Bozuk JSON kaynak doğrulamasına geçmeden hata üretir.
- [x] Desteklenmeyen ileri şema ve supersede cycle CI dahil tüm doğrulama yollarında reddedilir.
- [x] Bilgi şeması fixture/contract testleri bulunur.

#### Durum

Tamamlandı. `make verify` yeşil (gofmt, `go vet`, 732 test, race detector,
3 temiz-binary smoke testi), `make tidy-check` yeşil. Tasarım
[mr-002-design.md](mr-002-design.md); koda başlamadan dondurulan sözleşme
[mr-002-requirements.md](mr-002-requirements.md).

Dördüncü kriterin CI satırı **MR-018'e borçlu olarak kayıtlıdır, üstlenilmiş
değildir** (AC-11.6). Bu sürümde bir CI giriş noktası yok; reddetmenin `status`,
`doctor` ve `init` yollarında birebir aynı verdiği test edilir. Kriterin geri
kalanı — ileri şema ve supersede cycle — her üç yolda da karşılanır.

Milestone'un tek ölümcül kontrolü olan supersede cycle, dört denetim turu boyunca
üç kez yanlış yerde durdu; her seferinde soru "bu kaydın o kimlik üzerindeki
iddiası inandırıcı mı?" idi ve her seferinde belgenin *içeriğine* bakılarak
yanıtlandı. Karar **D-52** soruyu kaydın *nerede durduğuna* bağladı: bir kimliğin
adını taşıyan dosyada duran kayıt o kimlik adına konuşur, şemasını geçse de
geçmese de. Kural, bağımsız yazılmış bir hakemle her iki kayıt türü üzerinde
4.864'er mağaza; üç kimlikli geniş uzayda 110.592'şer mağaza; ve her iki türün
karıştığı 4.864 mağaza üzerinde sınandı — toplam 221.184, sıfır uyuşmazlık.

Dördüncü tur (yedi denetçi, altmış yedi ajan) davranışta hata bulamadı:
onaylanan sekiz bulgunun tamamı LOW ve metindeydi. Ardından gelen kapsam turu,
dördüncü turun "bakılmadı" listesini kapattı — karışık türlü mağazalar, geniş
sayım, dosya adı baytları ve aynı anda tek bir deponun üzerinde çalışan iki
komut.

Kapanmayan tek şey bir denetim borcu: **son iki geçiş bağımsız olarak
notlandırılmadı**, ve `internal/knowledge/record`, `internal/knowledge/schema`,
şema belgeleri, migration/storage/workspace paketleri ile doctor'ın altı
bilgi-dışı kontrolü hiç denetlenmedi. Her bulgunun ne olduğu, neyle kapandığı ve
geri gelirse hangi testin düşeceği [mr-002-findings.md](mr-002-findings.md)
içinde; kapsam turunun ölçümleri Ek H'de.

---

### [x] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri

- **Tür:** AFK
- **Blocked by:** MR-001
- **Kapsadığı senaryo:** Aynı workspace'i sıralı kullanan iki farklı AgentSession, aynı Task üzerinde checkpoint/context üzerinden devam eder.

#### Ne yapılacak

AgentSession, Task ve Checkpoint yaşam döngüsünü SQLite üzerinde kur. İlk ajan görevi açıp checkpoint bırakabilmeli; sonraki ajan task bağlamını, son checkpoint'i ve geçerli koordinasyon durumunu geri yükleyebilmelidir. Bu akış worktree zorunluluğu getirmemelidir.

#### Kabul kriterleri

- [x] Task oluşturma, devam ettirme ve kapatma durumları açıkça modellenir.
- [x] Checkpoint yeniden başlatma ve farklı AgentSession sonrasında okunabilir.
- [x] Aynı workspace'teki sıralı ajan devri veri kaybı veya gereksiz conflict üretmez.
- [x] Domain, persistence ve CLI akışı birlikte integration test ile doğrulanır.

#### Durum

**Tamamlandı.** İki denetim turu, iki remediasyon geçişi ve ikincisinin üzerinde
iki ajanlık bir doğrulama geçişinden sonra. Kutular ikinci remediasyona kadar
işaretsiz kaldı, çünkü bu depoda bir remediasyon geçişi denetlenir: MR-002'nin
ilk remediasyonu 22 bulguyu kapatıp 11 yenisini getirmişti, on birinin tamamı
düzeltme geçişinden. Üçüncü tur önerilmiyor; bir sonraki kapı MR-004'te görev
başına.

Sözleşme koda başlanmadan donduruldu ([mr-003-requirements.md](mr-003-requirements.md),
`cd74767`), tasarım [mr-003-design.md](mr-003-design.md). Milestone'un senaryosu
bir test olarak çalışıyor: iki ayrı AgentSession, aynı workspace'te, art arda,
aynı Task'ı sürdürüyor — aralarında Go tarafında hiçbir şey taşınmadan, her adım
veritabanını açıp kapatan ayrı bir komut çağrısı olarak. `make check`,
`make verify` ve `make tidy-check` yeşil; 807 test (ilk remediasyondan sonra
799, denetimde 768, temel 732).

**Birinci denetim turu** (`cd74767..f51478e`, yedi denetçi, 107 ajan) 50 bulgu
önerdi, 46'sı çürütmeden sağ çıktı, birleştirilince **27 farklı kusur**: 5 HIGH,
15 MEDIUM, 7 LOW. Özeti [mr-003-findings.md](mr-003-findings.md) §5'te, kanıtı
[mr-003-audit-round-1.md](mr-003-audit-round-1.md) içinde.

Turun bir cümlelik kararı: **devir çalışıyor, açıklamalar çalışmıyor.** Durum
makinesi beş ayrı saldırıya dayandı — reddedilen bir geçişi yazdıramadılar, iki
talep sahibini birden kazandıramadılar, bir projenin görevlerini ötekinin
sayımına sızdıramadılar; eşzamanlılık da yazma tarafında sağlam çıktı. Kırık
olan, depoda *başka* bir şey bozukken yeni komutların ne söylediği: altı ayrı
depo koşulu iki cümleye indirgeniyor ve hiçbirini gideremeyecek bir çare
basılıyor. Yanında üç daha keskin kusur var — geçersiz UTF-8 taşıyan bir başlık
satırı **yazıp sonra başarısız olduğunu bildiriyor** (ve tekrar denemek ikinci bir
görev açıyor), "en yeni checkpoint" karşılaştırması yalnızca tek süreç içinde
güvenilir, ve `mindrail init` tamamlanmış bir başlatma üzerine "başlatma bu
adımdan önce durdu" yazıyor.

Remediasyon brifingi on beş kalem hâlinde denetim belgesinin sonunda; her kalem
neyi bozmaması gerektiğini ve düzeltmeyi hangi mutasyonun tuttuğunu adıyla
söylüyor.

**Remediasyon** (`c29efa3..e7e946c`, on beş kalem, on üç commit — 8, 9 ve
10. kalemler `79533ac`'yi paylaşıyor; *ikinci tur §4.16 düzeltti, "on beş
commit" yazıyordu*) her kalemin
kendi mutasyonunu uygulayıp **kırmızı** olduğunu doğruladıktan sonra düzeltmeyi
yazdı — çünkü bu depoda en sık üretilen yanlış iddia, bir düzeltmenin üzerinden
geçen yeşil bir test paketidir. Beş HIGH bulgunun beşi de kapandı: koordinasyon
komutları artık başlangıç yargısını `status` ile aynı kodla yayımlıyor, geçersiz
UTF-8 taşıyan bayrak değerleri hiçbir şey açılmadan reddediliyor, saklanmış bir
değer artık "yeniden adlandırılamayan bir yol" diye anlatılmıyor, D-55 muhafızı
iç içe checkout'ta yeşil kalıyor, ve "en yeni checkpoint" veritabanının atadığı
sıra oldu. Ayrıntısı ve bu geçişin brifingden ayrıldığı üç yer
[mr-003-findings.md](mr-003-findings.md) §6'da.

**İkinci denetim** remediasyonun kendisini notladı: 9 denetçi, 35 öneri, 30
ayrık iddia, **17 doğrulanan — 0 HIGH, 4 MEDIUM, 13 LOW**. Yirmi yedi kusurun
yirmi yedisi kodda kapanmış; on yedinin on ikisi geçişin kendi düzeltmesini
olduğundan fazla anlatması — altısı kayıtta düpedüz yanlış cümle — ve
kullanıcıya ulaşan tek bir gerileme: eski sürümün açtığı bir veritabanında
`task list`, "şema geride, `init` çalıştır" yerine `doctor`'a gönderiyordu. Kanıt [mr-003-audit-round-2.md](mr-003-audit-round-2.md); tur
oranı ölçmek için vardı — MR-002'nin ilk remediasyonu 22 kapatıp 11 açmıştı,
bununki 27 kapatıp 16 açtı, oran düzelmedi ama şiddet çöktü.

**İkinci remediasyon** (`0f94e23`'ten `0ea96d5`'e, dokuz kalem, on iki commit —
biri denetim kaydının kendisi, 9. kalem iki commit) dokuz kalemin dokuzunu
kapattı ve brifingin backlog'a bıraktığı bir test onarımını öne aldı; her
mutasyon ana ağaçta yeniden çalıştırıldıktan sonra commit alındı. Üçüncü tur
önerilmiyor: sıfır HIGH. Sonunda iki ajanlık bir doğrulama geçişi — kaydı koda
karşı okuyan biri, üretim değişikliklerine saldıran biri — bir gerileme buldu
(bir hamlenin zaman damgası kilidin dışında okunuyordu, `updated_at` geri
gidebiliyordu) ve düzeltildi; sembolik bağlantı üzerinden girilen bir checkout'ta
dört mimari muhafızın hiçbir dosya görmemesi de aynı geçişte kapandı. Kaydı,
brifingten ayrıldığı altı yer ve doğrulama geçişinin dört bulgusu
[mr-003-findings.md](mr-003-findings.md) §7'de. 807 test.

---

### [x] MR-004 — Güvenli lease, idempotency ve optimistic revision

- **Tür:** AFK
- **Blocked by:** MR-001, MR-003
- **Kapsadığı senaryo:** Eşzamanlı iki ajan aynı korunan hedefte sessizce çakışan aktif lease veya duplicate kayıt üretemez.

#### Ne yapılacak

Task/symbol/file lease edinme-yenileme-bırakma akışını; SQLite WAL, kısa transaction, sınırlı busy retry, `operation_id` idempotency ve optimistic revision kontrolüyle uçtan uca tamamla.

#### Kabul kriterleri

- [x] Aynı logical target için iki process aynı anda çakışan aktif lease alamaz.
- [x] Aynı `operation_id` ile tekrar edilen mutation duplicate Task, Change veya Evidence üretmez.
- [x] Eski revision ile update sessiz last-write-wins yerine açık revision conflict verir.
- [x] SQLite concurrency/race testleri bounded retry ve conflict davranışını kanıtlar.
- [x] Transaction'lar interactive write starvation yaratmayacak kadar kısa tutulur.

#### Durum

**Tamamlandı.** Bu milestone, MR-003'ün sonunda kararlaştırılan süreç
değişikliğini test etmek için koşuldu: denetimi milestone sonuna değil her
görevin sonuna koymak. Sonuç, sürecin lehine tek cümlelik kanıt — MR-003 iki
denetim turu ve iki remediasyon geçişi istedi, MR-004 hiçbirini istemedi.
Sekiz görevin sekizi de kendi Reader/Breaker kapısıyla kapanmadan bir
sonrakine başlanmadı; kapılar iki düzeltme çıkardı (`1431d9f`, `556e88d`),
ikisi de kapı içinde kapatıldı, milestone sonrası tek bir denetim turu
açılmadı ve remediasyon brifi yazılmadı.

Sözleşme koda başlanmadan donduruldu
([mr-004-requirements.md](mr-004-requirements.md), `e4bad83`), tasarım
[mr-004-design.md](mr-004-design.md); üçüncü göç
`migrations/000003_lease_idempotency.sql`, tablo sürümü 3. Milestone'un
senaryosu bir test olarak çalışıyor: iki ayrı işletim sistemi süreci, tek
veritabanı, tek korunan hedef — biri kazanırken öteki kazananın oturumunu ve
bitişini taşıyan `LEASE_CONFLICT` duyuyor, tekrar edilen `operation_id`
ikinci kez yazmıyor, eski revision sessiz last-write-wins yerine
`STATE_REVISION_CONFLICT` veriyor, kilit bekleyeni bütçesini bir merdiven
adımı içinde `MINDRAIL_BUSY_RETRYABLE` ile bırakıyor. İki sürecin
kanıtladığı yedi mutasyon (P1–P7), sekiz lane'in yüz yazısının kilit
süreleriyle birlikte [mr-004-findings.md](mr-004-findings.md) §8'de; her
görevin kaydı ve kapısı aynı belgenin kendi bölümünde.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 872
test (MR-003'ün kapanışında 807). Kapıların backlog'a bıraktıkları, kendi
bölümlerinde kayıtlı: `init`'in kilitli veritabanında ikinci open bütçesi,
`isBusyError`'ın `SQLITE_LOCKED`'ı, `doctor`'un hasarlı satır denetimi
(lease zaman damgaları dahil) ve MR-015'in alan yüzeyi için
bilinmeyen-proje muhafızı.

---

### [x] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme

- **Tür:** AFK
- **Blocked by:** MR-001
- **Kapsadığı senaryo:** Mindrail değişen Python, TypeScript ve JavaScript fonksiyon/metot gövdelerini FULL resolver olmadan STRUCTURAL modda tanır.

#### Ne yapılacak

ProjectUnit keşfi, Tree-sitter language registry, content-addressed snapshot/cache ve incremental changed-file reindex yolunu kur. Sembol, import, açık reference/call ile body/signature/structure fingerprint üret. Büyük repository cold index'i tamamlama beklenmeden PARTIAL_READY dönebilmelidir.

#### Kabul kriterleri

- [x] Python, TS ve JS için fixture'lar declaration, method/function body, import ve açık call/reference çıkarımını doğrular.
- [x] Değişmemiş content hash yeniden parse edilmez.
- [x] Kesilen indeksleme tamamlanmış hash'leri tekrar işlemeden devam eder.
- [x] Unindexed aktif ProjectUnit, hedefe yönelik istek geldiğinde cold queue önüne alınır.
- [x] Büyük inventory sonrasında durum PARTIAL_READY/pending olarak açıkça raporlanabilir.

#### Durum

**Tamamlandı.** Yedi görev (TASK-01…07) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-05 kapısı 1 MEDIUM buldu (parent-dir
symlink guard'ının kırmızı kanıtı yoktu — test eklendi); TASK-06 kapıları
temizdi (2 LOW remediasyon: nil-registry testi, clean-absolute kuralı);
TASK-07 kapıları 2 MEDIUM buldu (kanıtta sembol-satır denetimi ve 60s
sınırının zayıflığı — ikisi de assertion ile kapatıldı) ve kanıt koşusu
**gerçek bir bug** yakaladı: `Prioritize` eviction'ı `queued` map'inde bayat
girdi bırakıyor, tahliye edilen dosyalar refill'de coalesce ile sessizce
atlanıyordu (soğuk index hiç bitmeyecekti). Düzeltme + pin testi
`TestEvictedPathsRefillAfterPrioritize` ile kayıtta.

Sözleşme koda başlanmadan donduruldu
([mr-005-requirements.md](mr-005-requirements.md), `40c42d0`), tasarım
[mr-005-design.md](mr-005-design.md); dördüncü göç
`migrations/000004_index.sql`, tablo sürümü 4. Kararlar D-80…D-92:
D-90 dosya-kapsamlı çözümlemeyi dondurur (MR-009 ya anahtarı genişletir ya
graph'ı bu kısıtla tasarlar), D-91 scheduler-sürücüsüz readiness'i
(TASK-07 kanıtı sürer), D-92 süreç-ölçeği kanıtını yeniden-açılan
handle'larla tanımlar.

Kanıt sayıları (`TestLargeInventoryColdIndexProof`): 2700 dosya, init
~16ms (satırsız, beklemez), PARTIAL_READY + pending=2700, window 1024,
900 TS öne, drain 2700/2700 + `max(attempts)=1` + `symbols=2700`, final
READY, status worst-of-20 ~25–200µs (150ms bütçe).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1028
test (TASK-05 başında 946 idi — MR-004 kapanışında 872). Kapıların MR-006'ya
bıraktıkları: D-90 anahtar-genişletme kararı ve unsupported-only census
faz notu (ikisi de `mr-005-findings.md` §TASK-06/07'de).

---

### [x] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması

- **Tür:** AFK
- **Blocked by:** MR-004, MR-005
- **Kapsadığı senaryo:** Korunan bir sembol rename/move edildiğinde bağlı invariant sessizce orphan olmaz.

#### Ne yapılacak

Yapısal fingerprint ve Git sinyallerini kullanarak dayanıklı `symbol_uid` tahsisi, eşzamanlı ilk-görüş uzlaşması ve rename/move migration akışını tamamla. Kimlik güvenle korunamıyorsa tahmin etmek yerine explicit ambiguity üret.

#### Kabul kriterleri

- [x] İki process aynı yeni sembol için tek bir `symbol_uid` üzerinde uzlaşır.
- [x] Güvenilir rename/move, invariant ilişkisini aynı kimliğe taşır.
- [x] Belirsiz rename korunan invariant'ı düşürmez; blocking ambiguity üretir.
- [x] Concurrent allocation, rename, move ve ambiguity fixture testleri bulunur.

#### Durum

**Tamamlandı.** Beş görev (TASK-01…05) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01 kapısı 1 HIGH buldu (güncellenmemiş
`required_version` pini — tek satır) + 1 MEDIUM (container_uid FK guard'ı).
TASK-02 kapıları temizdi (1 LOW remediasyon: yetim-satır fail-closed).
TASK-03 ilk turda FAIL/BLOCKED verdi (stored-row flip, SYMBOL-ambiguous
refresh, tarama genişliği, PROJECT) — remediasyon ikinci turda PASS aldı.
TASK-04 ilk turda FAIL/BLOCKED verdi ve **iki gerçek kusur** çıkardı:
hayalet ıraksama (ata kümesi staged anahtarları dışlamıyordu) ve
takeover-sessizliği; çekirdek sayım kuralı yeniden yazıldı, ikinci tur PASS.
TASK-05 ilk turda FAIL verdi (perde-2 AMBIGUOUS kodu assert edilmiyordu) —
refresh artık ölü-lineage'da ambiguity satırı varken orphan yerine ambiguous
raporluyor; ikinci turda dar bir eksik (perde-3 stored-row assertion) kapandı,
üçüncü tur PASS.

Sözleşme koda başlanmadan donduruldu
([mr-006-requirements.md](mr-006-requirements.md), `b902237`), tasarım
[mr-006-design.md](mr-006-design.md); beşinci göç
`migrations/000005_symbol_identity.sql`, tablo sürümü 5. Kararlar D-93…D-113:
D-94 tahsis anahtarı, D-95 atomik commit, D-96 bar + sayım kuralı, D-98
severity-blok, D-99 orphan-kanıtı, D-100 mint-only backfill, D-101 best-effort
Git, D-109 süreç-ölçeği uzlaşma, D-110 sticky-binding, D-111 bulgu-düzeyi
orphan, D-112 transaction-içi eşleştirme yerleşimi, D-113 ata-kapsamı ve
fail-safe sıralama.

Kanıt (`TestEndToEndProtectRenameAmbiguousDelete`): CRITICAL binding taşınır
(aynı uid), ikizler ambiguity satırı + AMBIGUOUS bloklar, silinen semboller
orphaned bloklar — iki kod da uçtan uca.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1074
test (MR-006 başında 1028 idi). Kapıların MR-007'ye bıraktıkları: completion
tüketimi için bulgu kodları (`SYMBOL_IDENTITY_AMBIGUOUS`,
`ORPHANED_PROTECTED_SYMBOL`), D-113 sıralama-bağımlılığı notu ve
stale-satır budama ihtiyacı (reconcile alanı).

---

### [x] MR-007 — Reconcile-first gerçek değişiklik keşfi

- **Tür:** AFK
- **Blocked by:** MR-003, MR-005, MR-006
- **Kapsadığı senaryo:** Ajan `mindrail_before_change` çağırmayı unutsa bile gerçek diff bulunur ve gate bypass edilmez.

#### Ne yapılacak

`before_change` baseline'ını bir koordinasyon optimizasyonu, `reconcile` yolunu ise doğruluk kaynağı olarak uygula. Worktree ve staged Git diff'lerini okuyup gerçek changed file/symbol setini Task/Change ile ilişkilendir; `after_change` delta'sını aynı servis üzerinden üret.

#### Kabul kriterleri

- [x] `before_change` çağrılmış ve çağrılmamış düzenlemeler aynı gerçek diff'e uzlaşır.
- [x] Function/method body, signature ve structure değişiklikleri Change kaydına bağlanır.
- [x] Baseline divergence görünür ve açıklanabilir sonuç üretir.
- [x] Git fixture testleri unstaged, staged ve ayrı worktree senaryolarını kapsar.

---

#### Durum

**Tamamlandı.** Beş görev (TASK-01…05) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de check'i düşüren 2 gerçek vaka
(ledger kirliliği, disk bandı) + kapıda 5 doc-bulgusu kapatıldı. TASK-02
kapıları temizdi (same-txn log + task-varlığı remediasyonları). TASK-03 ilk
turda FAIL verdi (4 test-boşluğu) — ikinci tur PASS. TASK-04 kapıları
temizdi (7 mutant + 2 ek pin). TASK-05 ilk turda PASS + INFO'larla kapandı;
hint-partition ve tasarım erratum'u aynı turda eklendi.

Sözleşme koda başlanmadan donduruldu
([mr-007-requirements.md](mr-007-requirements.md), `cb65502`), tasarım
[mr-007-design.md](mr-007-design.md); altıncı göç
`migrations/000006_changes.sql`, tablo sürümü 6. Kararlar D-114…D-131:
D-115 tek-açık-change, D-116 wholesale baseline, D-118 reconcile-indexler,
D-121 op-id replay, D-122 divergence-raporlanır, D-124 yeni-kod-yok, D-130
erratum, D-131 bağımsız-keşif-yakınsaması.

Kanıt sayıları: after_change 10 dosya ~9ms (bütçe 1.5s), reconcile ~3ms
(bütçe 2s); init ~16ms (MR-005 kanıtı korunur).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1122
test (MR-007 başında 1074 idi). Kapıların MR-008'e bıraktıkları: divergence
listesi şekli, unattributed delta listesi, `symbol.Service` + uid'li Change
satırları.

### [ ] MR-008 — Scope drift ve unregistered change ambiguity

- **Tür:** AFK
- **Blocked by:** MR-004, MR-007
- **Kapsadığı senaryo:** Beyan edilen kapsam dışındaki veya birden fazla Change'e ait olabilecek düzenleme sessizce sahiplenilmez.

#### Ne yapılacak

Reconcile sonucu ile claim/before_change kapsamını karşılaştırarak scope drift ve unregistered change bulguları üret. Tek bir Change'e güvenle bağlanamayan sembol için otomatik atama yerine explicit ambiguity ve çözüm yönlendirmesi ver.

#### Kabul kriterleri

- [x] Beyan edilen scope dışındaki değişiklik yapılandırılmış drift bulgusu üretir.
- [x] Birden fazla Change adayı bulunan sembol otomatik sahiplenilmez.
- [x] Unregistered veya ambiguous değişiklik completion/verify sırasında bloklanabilir.
- [x] Sonuç provenance ve uygulanabilir `next_action` içerir.

#### Durum

**Tamamlandı.** Dört görev (TASK-01…04) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de 2 check-düşürücü vaka (shipped
pin newline'ı, kod sayacı 40→43) + kapıda FK pini kapatıldı. TASK-02
kapıları temizdi (D-145 yorumu + tasarım sapması notu). TASK-03 B-1'i
kapattı (fail-closed `RecordAttribution`); override'un anahtar-global
olduğu testle pinlendi. TASK-04 ilk turda 2 kapı-bulgusu verdi (Durum bloğu
yokluğu, E2E'de `c::h` override maskelemesi) — ikisi de aynı turda
kapatıldı: ara-beat assertion'ı + iki-change-adlandırma pini eklendi.

Sözleşme koda başlanmadan donduruldu
([mr-008-requirements.md](mr-008-requirements.md),
[mr-008-design.md](mr-008-design.md), `bf3cc05`), kararlar D-132…D-145:
D-132 beyan-kapsam-bazelin, D-133 dosya-üyeliği-adaylık, D-134 lease-tavsiye,
D-136 bulgu-saklanmaz, D-137 override-öncelikli, D-138 hepsi-bloklar. Yedinci
göç `migrations/000007_scope_attribution.sql`, tablo sürümü 7, 3 yeni kod
(43 toplam).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1138
test (MR-008 başında 1122 idi). 15 guard mutasyonu kırmızı koşuldu
(M1…M15). Kapıların MR-009'a bıraktıkları: blocking set (code + provenance
+ remedy) MR-013'ün kapı girdisi; override satırları MR-015'in atama girdisi;
attributed (change, symbol) çiftleri MR-009'un traversal girdisi.

---

### [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi

- **Tür:** AFK
- **Blocked by:** MR-002, MR-005, MR-007
- **Kapsadığı senaryo:** Değişen semboller için doğrudan yapısal etki ve ilgili invariant'lar bulunur; zayıf sinyal gereksiz tüm-suite çalıştırmaya dönüşmez.

#### Ne yapılacak

Direct structural reference, bounded reverse traversal, file/module fallback ve invariant scope eşlemesini uygula. Yapısal isim eşleşmelerini en fazla `0.5` confidence ve `STRUCTURAL_NAME_MATCH` provenance ile etiketle; varsayılan reverse depth `1` olsun.

#### Kabul kriterleri

- [x] Her impact sonucu kaynak edge, confidence, traversal depth ve fallback nedenini açıklar.
- [x] `depth > 1` yalnızca explicit istekte çalışır.
- [x] Salt structural edge validation breadth'i MODULE üstüne çıkaramaz.
- [x] Path policy, public API kuralı veya explicit invariant scope daha geniş breadth'i gerekçelendirebilir.
- [x] Aynı isimli declaration'lar için belirsizlik kaybolmadan raporlanır.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de justification'sız override
deliği kapıda kapatıldı (M5). TASK-02'de unit-blind `seen` bug'ı (B-2) +
override-clobber kapatıldı (M11, M12). TASK-03'te E2E, AC-02.1 boşluğunu
açığa çıkardı (çözümsüz çağıranlar görünmüyordu) — `UnresolvedReferringTo`
katmanı eklendi (M13), TASK-02 kapısının AC-02.1 onayı bu kayıtla
nitelendi.

Sözleşme koda başlanmadan donduruldu
([mr-009-requirements.md](mr-009-requirements.md),
[mr-009-design.md](mr-009-design.md), `1694d59`), kararlar D-146…D-154:
D-146 D-90-kısıtlı-graph, D-147 saf-hesap, D-148 yeni-paket, D-149
breadth-cap + justification, D-150 depth-1-varsayılan. Göç yok, kod 43,
knowledge v1. Yeni paket `internal/impact`; index'e 7 nötr okuma metodu.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1154
test (MR-009 başında 1138 idi). 13 guard mutasyonu kırmızı koşuldu
(M1…M13). Kapıların MR-010'a bıraktıkları: entry + breadth (runner girdisi),
invariant listeleri (invalidation girdisi), justification slotu (politika
girdisi).

---

### [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence

- **Tür:** AFK
- **Blocked by:** MR-002, MR-007, MR-009
- **Kapsadığı senaryo:** Proje tarafından adlandırılmış doğrulama profili güvenle çalışır ve sonucu tam kaynak snapshot'ına bağlanır.

#### Ne yapılacak

Project-defined validation profile seçimi, argv-only process execution, timeout, bounded stdout/stderr, exit/result normalizasyonu, source snapshot hash ve append-only Evidence kaydını tek uçtan uca akışta sun. Bilinen secret'lar depolama öncesinde redact edilmelidir.

#### Kabul kriterleri

- [x] Shell string yerine yalnızca argv tabanlı komut çalıştırılır.
- [x] Timeout ve çıktı sınırı deterministic, yapılandırılmış sonuç üretir.
- [x] Evidence; profile, command sonucu, snapshot hash, provenance ve timestamp taşır.
- [x] Configured known secret plaintext olarak Evidence içine girmez.
- [x] Runner unit testleri ile gerçek process integration testleri bulunur.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de PATH-hijack tespiti (ResolvedExe
+ LookPath) ve torun-sarkması (process-group kill) kapıda kapatıldı (M6,
M7). TASK-02'de B-3 gerçek sızıntısı çıktı (JSON-escape argv'de yaşıyordu)
— element-bazlı redaksiyona geçildi (M13). TASK-03 kapıları temizdi
(M-etiket çakışması aynı turda giderildi: M1…M7 TASK-01, M8…M13 TASK-02).

Sözleşme koda başlanmadan donduruldu
([mr-010-requirements.md](mr-010-requirements.md),
[mr-010-design.md](mr-010-design.md), `2444309`), kararlar D-155…D-164:
D-155 runtime-DB-evidence, D-156 göç-000008/sürüm-8, D-157 argv-only +
64KB, D-158 fail-closed-snapshot, D-160 op-id-replay, D-164 tipsiz-default
yok. Kod 43, knowledge decision/invariant, göç 8. Yeni paket
`internal/validation`; config'e profil + secret-isimleri.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1174
test (MR-010 başında 1154 idi; +1 B-3 pin testi). 13 guard mutasyonu kırmızı
koşuldu (M1…M13). Kapıların MR-011'e bıraktıkları: evidence satırları
(invalidation girdisi), snapshot hash'leri (staleness girdisi), op-id
replay (retry girdisi).

---

### [ ] MR-011 — Kaynak değişince evidence geçersizleştirme

- **Tür:** AFK
- **Blocked by:** MR-010
- **Kapsadığı senaryo:** Başarılı validation sonrasında kaynak değişirse eski sonuç güncel kanıt gibi kullanılamaz.

#### Ne yapılacak

Evidence snapshot'ını reconcile edilen mevcut source snapshot ile karşılaştır. İlgili kaynak değişikliğinde evidence'ı stale olarak işaretle ve validation/completion bağlamında yeniden çalıştırılacak profilleri açıkla.

#### Kabul kriterleri

- [x] Değişmemiş snapshot için kanıt current kalır.
- [x] Validation sonrasındaki ilgili source edit kanıtı stale yapar.
- [x] Stale kanıt completion için required evidence şartını karşılamaz.
- [x] Stale nedeni ve yeniden çalıştırılması gereken profil kullanıcıya açıklanır.

#### Durum

**Tamamlandı.** İki görev (TASK-01…02) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01 kapısı sıkıydı: boş gap-gerekçesi,
AC-01.5 harf-pini, dup-scope/trailing-slash/boş-kapsam divergence'ları
kapatıldı (M6…M8); AC-01.2 eklenen-dosya alt-davranışı DoD-1 şerhiyle
kayıtlı. TASK-02'de E2E'nin ilgisiz-edit beati zayıf çıktı — hash-eşitliği
assertion'ıyla güçlendirildi (kapı-sonrası, kendi koşumla doğrulandı).

Sözleşme koda başlanmadan donduruldu
([mr-011-requirements.md](mr-011-requirements.md),
[mr-011-design.md](mr-011-design.md), `5aaaae3`), kararlar D-165…D-169:
D-165 hesaplanan-staleness, D-166 kapsam-hash-uyumu, D-167 required-caller
+ union-rerun. Göç/kod/config yok (43 kod, 8 göç). Tek dosya
(`internal/validation/freshness.go`); DB erişimi yok (yapısal salt-okunur).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1185
test (MR-011 başında 1174 idi). 8 guard mutasyonu kırmızı koşuldu (M1…M8).
Kapıların MR-012'ye bıraktıkları: verdict + coverage MR-013'ün kapı
girdisi; re-run listeleri MR-015'in girdisi. Not: bu milestone gate
uygulamaz, zafiyet profili tanımlamaz — MR-012'nin girdisi verdict
şeklidir.

---

### [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı

- **Tür:** AFK
- **Blocked by:** MR-002, MR-005, MR-007
- **Kapsadığı senaryo:** Aktif CRITICAL invariant'ın testini kaldırmak, skip etmek veya assertion azaltmak production sembolü değişmese bile bloklanır.

#### Ne yapılacak

Assertion/expect kaldırma, assertion sayısı azalması, skip/xfail/disabled marker ekleme ve kritik verification test kaldırma sinyallerini düşük maliyetli yapısal analizle yakala. Bulguyu `TEST_GUARD_WEAKENED` olarak invariant ve diff provenance'ıyla üret.

#### Kabul kriterleri

- [x] Assertion kaldırma/azaltma ile skip/xfail/disabled ekleme fixture'ları yakalanır.
- [x] Aktif CRITICAL invariant verification testinin kaldırılması blocking bulgu üretir.
- [x] Yalnız test dosyasına dokunan değişiklik de guard tarafından değerlendirilir.
- [x] Aynı guard reconcile, staged verify ve CI verify yollarında ortak servis olarak kullanılır.
- [x] Açıklanabilir false-positive escape hatch/policy davranışı belgelenir ve test edilir.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01 kapıları temizdi (`test` prefix
D-171 şerhiyle). TASK-02'de B-4 (kaçış-tırnak) + B-5 (duplicate-maskeleme)
kapatıldı; M6 ölü-kodu suite-disable ile canlandı. TASK-03'te E2E'nin
mapping-kontrol körlüğü için unmapped-warn pini + deterministik sıralama
eklendi (M13).

Sözleşme koda başlanmadan donduruldu
([mr-012-requirements.md](mr-012-requirements.md),
[mr-012-design.md](mr-012-design.md), `36c2af5`), kararlar D-170…D-178:
D-170 saf-servis, D-171 dar-test-kimliği, D-172 çağrıcı-eşlemesi, D-173
dar-blocking, D-175 kendini-açıklayan-hatch. Kod 44 (tek yeni), göç yok.
Yeni paket `internal/testguard` + tree-sitter sorguları.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1205
test (MR-012 başında 1185 idi). 13 guard mutasyonu kırmızı koşuldu
(M1…M13). Kapıların MR-013'e bıraktıkları: bulgu + blocking (gate girdisi),
suppression listesi (denetim girdisi), politika tablosu (yapılandırma
girdisi).

---

### [ ] MR-013 — Yerel completion evidence gate

- **Tür:** AFK
- **Blocked by:** MR-006, MR-008, MR-009, MR-011, MR-012
- **Kapsadığı senaryo:** Ajan yalnızca protokol beyanıyla işi bitiremez; gate gerçek diff, invariant ve güncel kanıtı bağımsız değerlendirir.

#### Ne yapılacak

Completion kararını tek bir ALLOW/DENY modeli altında birleştir. Unresolved blocking invariant, missing/stale evidence, protected-symbol ambiguity ve unreconciled change ayrı reason code ve `next_action` ile DENY üretmelidir.

#### Kabul kriterleri

- [x] Tüm denial nedenleri deterministic reason code, provenance ve çözüm önerisi taşır.
- [x] `before_change` çağrılmamış olsa da completion önce reconcile eder.
- [x] Gerekli current evidence ve çözümlenmiş invariant olduğunda ALLOW üretir.
- [x] Aynı snapshot ve state için karar tekrarlanabilir/idempotent olur.
- [x] Birincil uçtan uca kernel senaryosu ALLOW ve her DENY dalı için test edilir.

#### Durum

**Tamamlandı.** İki görev (TASK-01…02) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de boş-bulgu fail-closed'a
çevrildi (Evaluate error dönüyor), dup-denial tekleniyor, sabitler
kullanılıyor (M6…M8). TASK-02'de kernel E2E'nin ALLOW kolu evidence'a
kör çıktı — stale-DENY ara-beati eklendi, warn-scope binding ALLOW
girdisinde.

Sözleşme koda başlanmadan donduruldu
([mr-013-requirements.md](mr-013-requirements.md),
[mr-013-design.md](mr-013-design.md), `eb55e21`), kararlar D-179…D-184:
D-179 saf-composer, D-180 kod-tekrar-kullanımı + tek evidence kodu, D-181
guard-ailesi, D-182 damgasız-karar. Kod 45 (tek yeni), göç yok. Yeni paket
`internal/gate` (depo erişimi yok, yapısal).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1215
test (MR-013 başında 1205 idi). 8 guard mutasyonu kırmızı koşuldu (M1…M8).
Kapıların MR-014'e bıraktıkları: karar modeli (tool çıktısı), denial
kodları (CI çıktısı), coverage-boşlukları (koşul listesi).

---

### [ ] MR-014 — MCP bilgi ve bağlam araçları

- **Tür:** AFK
- **Blocked by:** MR-002, MR-003, MR-009
- **Kapsadığı senaryo:** Farklı MCP client'ları aynı repository/knowledge/context sözleşmesini kullanır.

#### Ne yapılacak

Resmî MCP Go SDK üzerinden `mindrail_bootstrap`, `mindrail_status`, `mindrail_search`, `mindrail_context`, `mindrail_decide` ve `mindrail_invariant` araçlarını mevcut application/domain servislerine bağla. `context.detail_level` yalnızca `summary|focused`, invariant mode yalnızca `active` desteklemelidir.

#### Kabul kriterleri

- [x] CLI ve MCP aynı application servislerini kullanır; davranış fork'u oluşmaz.
- [x] Tool input/output şemaları contract testleriyle sabitlenir.
- [x] Ertelenmiş `full`, `impact_group`, cursor/page size veya `candidate` değerleri sessizce düşürülmez.
- [x] Desteklenmeyen değer `NOT_IMPLEMENTED_IN_THIS_VERSION` ve `next_action` döndürür.
- [x] En az iki farklı MCP client uyumunu simüle eden contract fixture bulunur.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de altı-tool kaydı + rapor
eşdeğerliği kapıda kapatıldı. TASK-02'de write araçları + deferred matris
+ iki-client fixture'ı kapandı (M9'da pin güçlendirme). TASK-03 kapıları
temizdi (wire-isimleri + description'lar Breaker-probe'uyla doğrulandı).

Sözleşme koda başlanmadan donduruldu
([mr-014-requirements.md](mr-014-requirements.md),
[mr-014-design.md](mr-014-design.md), `273090a`), kararlar D-185…D-194:
D-185 resmi-SDK, D-186 bağla-çatallama, D-188 yüksek-sesli-ret, D-190
sınırlı-arama, D-191 komutsuz-sunucusuz. Kod 46 (tek yeni), göç yok. Yeni
paket `internal/mcp` (6 tool, tipli handler'lar).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1223
test (MR-014 başında 1215 idi). 10 guard mutasyonu kırmızı koşuldu
(M1…M10). Kapıların MR-015'e bıraktıkları: tool yüzeyi (koordinasyon/
değişiklik araçları aynı server'a), kayıt dosyaları (CI okur),
refusal-kodları (ajanlara).

---

### [ ] MR-015 — MCP koordinasyon ve değişiklik araçları

- **Tür:** AFK
- **Blocked by:** MR-004, MR-007, MR-008, MR-014
- **Kapsadığı senaryo:** Ajan claim, change ve checkpoint yaşam döngüsünü MCP üzerinden yürütebilir; gerçek diff yine reconcile ile doğrulanır.

#### Ne yapılacak

`mindrail_claim`, `mindrail_before_change`, `mindrail_after_change`, `mindrail_reconcile` ve `mindrail_checkpoint` araçlarını ortak koordinasyon/değişiklik servislerine bağla; operation idempotency, revision conflict ve yapılandırılmış hata sözleşmelerini koru.

#### Kabul kriterleri

- [x] Beş aracın başarı, idempotent retry, conflict ve ambiguity contract testleri bulunur.
- [x] `before_change` opsiyonel optimizasyon, `reconcile` kanonik doğruluk yolu olarak kalır.
- [x] Checkpoint farklı AgentSession tarafından okunabilir.
- [x] Tool sonuçları pending/partial durumları ve `next_action` taşır.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de containment + replay + mesaj
pinleri kapatıldı (M5, M6). TASK-02'de reconcile/checkpoint/lifecycle
kapandı; Reader'ın state-görünürlük şerhi twin-overlap beatiyle kapatıldı.

Sözleşme koda başlanmadan donduruldu
([mr-015-requirements.md](mr-015-requirements.md),
[mr-015-design.md](mr-015-design.md), `5b42d40`), kararlar D-195…D-204:
D-195 tek-server-ModeWrite, D-196 servis-bağlama, D-197 opsiyonel-declare,
D-200 note-var/yok-iki-yön. Kod 46, göç yok. 11 tool tek server'da.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1229
test (MR-015 başında 1223 idi). 10 guard mutasyonu kırmızı koşuldu
(M1…M10). Kapıların MR-016'ya bıraktıkları: onbir-tool server
(validation/completion araçları genişletir), handover'lar (CI okur),
op-id'ler (retry protokolü).

---

### [ ] MR-016 — MCP validation ve completion araçları

- **Tür:** AFK
- **Blocked by:** MR-010, MR-011, MR-013, MR-014
- **Kapsadığı senaryo:** Ajan doğrulama çalıştırıp completion isteyebilir fakat kanıt kurallarını atlayamaz.

#### Ne yapılacak

`mindrail_validate` ve `mindrail_complete` araçlarını validation/evidence/completion servislerine bağla. 0.1'de validate yalnız named project profile, complete yalnız local evidence gate desteklemelidir.

#### Kabul kriterleri

- [x] Named profile sonucu mevcut snapshot'a bağlı Evidence üretir.
- [x] Missing/stale evidence MCP completion'da CLI ile aynı DENY sonucunu verir.
- [x] Budget class/escalation alternative veya managed/signed approval isteği structured version error döndürür.
- [x] MCP stdio end-to-end testi 13 aracın tamamını discover edip temel contract'larını doğrular.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de validate + kompozisyon
read'leri kapandı. TASK-02'de complete kompozisyonu (ambiguity-beat,
13/13 smoke, bilinmeyen-görev reddi) kapı bulgularıyla kapatıldı (M9,
M10). TASK-03 kapıları temizdi (wire-isimleri + SDK-direct Breaker
doğrulamalı).

Sözleşme koda başlanmadan donduruldu
([mr-016-requirements.md](mr-016-requirements.md),
[mr-016-design.md](mr-016-design.md), `c60d0af`), kararlar D-205…D-214:
D-205 isimli-profil, D-206 beş-aile-kompozisyon, D-208 version-error,
D-209 subprocess'siz-stdio. Kod 46, göç yok. 13 tool tek server'da.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1241
test (MR-016 başında 1229 idi). 10 guard mutasyonu kırmızı koşuldu
(M1…M10). Kapıların MR-017'ye bıraktıkları: onüç-tool server (CI sürer),
kompozisyon girdileri (staged/CI verify), version-error'lar (politika).

---

### [ ] MR-017 — Staged değişiklik için yerel Git enforcement

- **Tür:** AFK
- **Blocked by:** MR-002, MR-008, MR-012, MR-013
- **Kapsadığı senaryo:** Commit öncesinde bozuk knowledge, unreconciled değişiklik veya zayıflatılmış kritik test reddedilir.

#### Ne yapılacak

`mindrail verify --staged`, `mindrail knowledge validate` ve mevcut hook'u ezmeden pre-commit entegrasyonunu ekle. Staged diff, aynı reconcile/invariant/test-guard/completion servisleriyle değerlendirilmelidir.

#### Kabul kriterleri

- [x] `verify --staged` unreconciled staged değişikliği yakalar.
- [x] Knowledge corruption/ileri şema kaynak doğrulamasından önce fail-closed olur.
- [x] Test weakening staged yolda blocking bulgu üretir.
- [x] Kurulum mevcut pre-commit hook içeriğini overwrite etmez.
- [x] Git fixture testleri clean, allow ve denial senaryolarını kapsar.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01'de AC-01.3 şerhli kapandı
(malformed-JSON loader-tasarımı) + B-3 fail-closed + drift/unborn/args
pinleri (M6…M9). TASK-02'de hook + fixture'lar kapandı (M10…M13).
TASK-03 kapıları temizdi (M9-etiket çakışması giderildi: M1…M9 / M10…M13).

Sözleşme koda başlanmadan donduruldu
([mr-017-requirements.md](mr-017-requirements.md),
[mr-017-design.md](mr-017-design.md), `8551a1b`), kararlar D-215…D-220:
D-215 staged-satır-yazımı, D-216 index-hükmü, D-217 fatal-önceliği, D-218
eklemeli-kurulum. Kod 46, göç yok. 3 komut (verify/knowledge/hook).

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil; 1260
test (MR-017 başında 1241 idi). 13 guard mutasyonu kırmızı koşuldu
(M1…M13). Kapıların MR-018'e bıraktıkları: verify --staged (CI çağırır),
hook kurulumu (CI eşdeğeri), staged-kaynak (aralıklar).

---

### [ ] MR-018 — Hook bypass'a dayanıklı CI verification

- **Tür:** AFK
- **Blocked by:** MR-017
- **Kapsadığı senaryo:** `git commit --no-verify` ile oluşturulan commit bağımsız CI kontrolünü geçemez.

#### Ne yapılacak

`mindrail verify --ci` akışını fresh clone üzerinde knowledge validation, merge-base(base, head) diff, reconcile, changed symbols, invariant/test guard, required validation profiles ve structured ALLOW/DENY sırasıyla uygula. Runtime task lease, portable manifest ve cross-run impact cache ekleme.

#### Kabul kriterleri

- [x] Base/head seçimi ve merge-base hataları yapılandırılmış biçimde raporlanır.
- [x] `--no-verify` ile yerelde atlanan denial CI'da yeniden üretilir.
- [x] CI, staged/local yol ile aynı policy ve domain servislerini kullanır.
- [x] Bozuk/incompatible knowledge en erken aşamada fail-closed olur.
- [x] Fresh-clone Git fixture testi tüm akışı doğrular.

#### Durum

**Tamamlandı.** Üç görev (TASK-01…03) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01 ilk turda 5 kapı-bulgusu verdi
(B1 ters-aralık yeşili, B2 default-boş yeşili, B3 worktree-bağımlılık
çökmesi, B4 modlar-arası union kirlenmesi, B5 knowledge-desk bypass'ı)
— remediasyon ikinci turda N1/F1 artığıyla BLOCKED verdi (knowledge-kir
carve-out'u), üçüncü tur PASS aldı. TASK-02 ilk turda 2 reprodüksiyon +
1 insidental ile BLOCKED verdi (boş-aralıklı temiz fixture, bare
main-tip yeşili, daralmayan change) — remediasyon ikinci turda PASS
aldı. TASK-03 kapıları temizdi.

Sözleşme koda başlanmadan donduruldu
([mr-018-requirements.md](mr-018-requirements.md),
[mr-018-design.md](mr-018-design.md), `a2ae21c`), kararlar D-221…D-228
+ kapı-okumaları: default-boş reddi (explicit-eşit yeşil), menzil
başına change (hüküm (base, head)'in fonksiyonu), temiz-desk önkoşulu
(worktree HEAD == head, knowledge-JSON kir reddi). Kod 46, göç yok.
`verify` ikinci modu (`--ci` + `--base/--head`) aldı; manifest, cache,
lease, profil-çalıştırma eklenmedi.

`make check`, `make verify` (race + smoke) ve `make tidy-check` yeşil;
1293 test (MR-018 başında 1260 idi). 16 guard mutasyonu kırmızı koşuldu
(M1…M7 TASK-01, M8…M13 remediasyon, M14…M16 TASK-02). Kapıların
MR-019'a bıraktıkları: `verify --ci` (SLO ölçümü), menzil-op-id'leri
(tekrar-protokolü), desk-önkoşulu (temiz-klon varsayımı).

---

### [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı

- **Tür:** AFK
- **Blocked by:** MR-015, MR-016, MR-018
- **Kapsadığı senaryo:** Mindrail zaman bütçesini sessizce aşmaz veya analizi atlamaz; performans ve release kalitesi ölçülebilir.

#### Ne yapılacak

`status`, `context`, `before_change`, `after_change` (≤10 dosya) ve `reconcile` (≤10 dosya) için benchmark/telemetry kur. Bütçe içinde bitmeyen iş explicit partial/pending dönmeli ve kalan işi öncelikli schedule etmelidir. Platform-native binary build ve release kalite kapısını otomatikleştir.

#### Kabul kriterleri

- [x] STRUCTURAL p95 hedefleri sırasıyla `<150 ms`, `<250 ms`, `<300 ms`, `<1.5 s`, `<2 s` olarak raporlanır.
- [x] SQLite wait, parse, impact traversal ve operasyon süreleri ölçülür.
- [x] Bütçe aşımı blocking bekleme veya sessiz skip yerine partial/pending sonuç üretir.
- [x] Unit, domain, integration, race, knowledge schema, MCP contract, Git worktree, SQLite concurrency ve end-to-end test kapıları çalışır.
- [x] Desteklenen platform binary'leri build edilir ve smoke testten geçer.

#### Durum

**Tamamlandı.** Dört görev (TASK-01…04) seri koşuldu; her biri bağımsız
Reader/Breaker kapısından geçti. TASK-01 kapıları temizdi (2 düşük not
aynı turda kapatıldı: vacuous dal, floor-rank). TASK-02 ilk turda dar
BLOCKED verdi (S1/S2 yaşayan mutantlar — dallanan-sınır piniyle
kapatıldı). TASK-03 ilk turda dar BLOCKED verdi (untracked-kör kir
damgası — porcelain ifadesiyle kapatıldı). TASK-04 kapanış kapısı
temizdi.

Sözleşme koda başlanmadan donduruldu
([mr-019-requirements.md](mr-019-requirements.md),
[mr-019-design.md](mr-019-design.md), `6c5742c`), kararlar D-229…D-236
+ kapı-okumaları: transport-toplam/servis-breakdown ayrımı (tel
context geçirmez), canlı-op kablosuz erteleme (mekanizma-hazır,
dürüst kayıt), §106 kuralıyla yalnız linux/amd64 duyurusu. Kod 46,
göç yok. Yeni paket `internal/perf` (timer, recorder, targets,
budget); `verify` 13 tool, `make` 4 yeni hedef (`bench`, `release`,
`gate`, `vuln`).

`make check`, `make verify` (race + smoke), `make bench`, `make gate`
ve `make tidy-check` yeşil; 1308 test (MR-019 başında 1293 idi; 1307
PASS + 1 SKIP). 17 guard mutasyonu kırmızı koşuldu (M1…M8, S1/S2,
M9…M13'). Kapıların MR-020'ye bıraktıkları: bench raporu (demo
ölçümleri), gate matrisi (çıkış-kanıtı koşucusu), release damgası
(sürüm kimliği).

---

### [x] MR-020 — 0.1 çıkış koşulu kabul incelemesi

- **Tür:** HITL
- **Blocked by:** MR-019
- **Kapsadığı senaryo:** Ürün sahibi, kernel hipotezinin yalnız bileşen testleriyle değil gerçek bypass denemeleriyle kanıtlandığını onaylar.

#### Ne yapılacak

Temiz bir örnek repository ve iki ajan/worktree senaryosu üzerinde 0.1 exit condition demonstrasyonu hazırla. Unutulan protocol çağrısı, local hook bypass, protected-symbol rename, kritik test zayıflatma ve stale test sonucu iddiasını ayrı ayrı dene; her birinin bağımsız gate tarafından reddedildiğini kaydet.

#### Kabul kriterleri

- [x] Birincil end-to-end senaryonun başarılı ALLOW yolu demo edilir.
- [x] `before_change` unutma reconcile tarafından yakalanır.
- [x] `--no-verify`, CI verification tarafından yakalanır.
- [x] Protected-symbol rename kimliği korur veya explicit ambiguity ile bloklanır.
- [x] Kritik test zayıflatma production code değişmese bile bloklanır.
- [x] Stale evidence completion proof'u olarak reddedilir.
- [x] Ürün sahibi sonuçları ve 0.1 release kararını kayda geçirir.

#### Durum

**Tamamlandı — ürün sahibi kararı: Ship 0.1.** Dört görev (TASK-01…04)
seri koşuldu; TASK-01…03'ün her biri bağımsız Reader/Breaker
kapısından geçti (round-1 BLOCKED → remediasyon → round-2/3 PASS).
Altı demo scratch repo + gerçek binary/üretim servisleriyle koşuldu:
docs-only değişiklik ALLOW (`allow:true`, sıfır denial); beyan
edilmemiş diff `UNREGISTERED_CHANGE` (gömülü `ReconcileStaged` yolu,
`CHG-…` keşif artığı); hook DENY → `commit --no-verify` → fresh
klonda `verify --ci` birebir aynı kod kümesi; rename aynı
`symbol_uid`'yi taşır, ikizlenirsen `SYMBOL_IDENTITY_AMBIGUOUS` ile
bloklar (satır uid'yi korur), silinirse `ORPHANED_PROTECTED_SYMBOL`;
yalnız-test zayıflatma `TEST_GUARD_WEAKENED` (üretim bayt-aynı pini);
bayat kanıt `REQUIRED_EVIDENCE_NOT_CURRENT` + yeniden-koş listesi,
taze koşu ALLOW'a döndürür.

Sözleşme koda başlanmadan donduruldu
([mr-020-requirements.md](mr-020-requirements.md),
[mr-020-design.md](mr-020-design.md), `e24d1e7`), kararlar D-237…D-244
+ kapı-okumaları: iki-kopya senaryosu CLI worktree'lerinde ve stale
demosunda (kopya A/B); rename'de sıralı-ajan (iki-checkout deneyi
günlüğe geçti: aynı baytlar bağımsız uid basar — lifecycle-başına
dayanıklılık, checkout'lar-arası değil); cross-copy kanıt
`scope escapes the root` ile fail-safe. Freeze-sonrası tek kelimelik
Türkçe düzeltme DoD-1 notuyla kayıtlı. Kod 46, göç yok (v8), MCP
yüzeyi değişmedi — demo sürücüleri geçiciydi, kapanışta silindi
(D-241).

`make verify` (check + race + smoke) ve `make tidy-check` yeşil; 1308
test (MR-019 baseline — geçici 7 demo testi silindi). Kapıların
sonraya bıraktıkları: yok — 0.1 kernel kapandı; bulgular
`mr-020-findings.md`'de, ölçüm girdileri MR-019 kanıtında (bench
raporu, gate matrisi, release damgası linux/amd64).

---

## 0.1 Kabul Kriteri İzlenebilirlik Matrisi

| Kernel kriteri | Karşılayan görev(ler) |
|---|---|
| AC-01 Yerel, cloudsuz init | MR-001 |
| AC-02 Kalıcı Decision/Invariant | MR-002 |
| AC-03 Aynı workspace sıralı ajanlar | MR-003, MR-015 |
| AC-04 Çakışan lease engeli | MR-004 |
| AC-05 Reconcile-first discovery | MR-007, MR-013 |
| AC-06 Python/TS/JS structural change | MR-005, MR-007 |
| AC-07 Rename ve `symbol_uid` koruması | MR-006 |
| AC-08 Snapshot'a bağlı validation evidence | MR-010 |
| AC-09 Stale evidence | MR-011 |
| AC-10 Test weakening | MR-012 |
| AC-11 Completion current evidence zorunluluğu | MR-013 |
| AC-12 `verify --staged` | MR-017 |
| AC-13 Warm-path SLO ölçümü | MR-019 |
| AC-14 CLI/MCP ortak servisleri | MR-014, MR-015, MR-016 |
| AC-15 Operation idempotency | MR-004 |
| AC-16 Optimistic revision conflict | MR-004 |
| AC-17 Secret redaction | MR-010 |
| AC-18 Concurrent `symbol_uid` uzlaşması | MR-006 |
| AC-19 `verify --ci` hook bypass engeli | MR-018 |
| AC-20 Kritik verification test koruması | MR-012, MR-017, MR-018 |
| AC-21 Deferred MCP value rejection | MR-014, MR-016 |
| AC-22 Structural breadth ≤ MODULE | MR-009 |

## 0.1 Dışında Tutulacak Backlog

Aşağıdakiler Technical Specification 1.0 hedefinde olsa da bu listenin teslim kapsamına **alınmamalıdır**:

- Pyright ve TypeScript Language Service FULL semantic resolver'ları
- Managed resolver download, checksum, pool/LRU ve resource-budget yönetimi
- Runtime coverage provider'ları ve `TEST_COVERS` tabanlı gelişmiş test seçimi
- C#, Java, Go, PHP ve framework-magic adapter'ları
- Managed identity, signed approval ve gelişmiş approval provider'ları
- Candidate invariant üretimi ve calibration/report UI
- Gelişmiş fan-out aggregation/pagination
- Portable CI change manifest ve cross-run impact cache
- Remote MCP/HTTP, takım servisi, vector search, embedding ve graph database

Bu backlog, 0.1 çıkış koşulu sağlandıktan sonra Teknik Şartname 1.0 milestone sırasıyla ayrıca dilimlenmelidir.

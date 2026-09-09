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

### [ ] MR-003 — Session, Task ve Checkpoint ile sıralı ajan devri

- **Tür:** AFK
- **Blocked by:** MR-001
- **Kapsadığı senaryo:** Aynı workspace'i sıralı kullanan iki farklı AgentSession, aynı Task üzerinde checkpoint/context üzerinden devam eder.

#### Ne yapılacak

AgentSession, Task ve Checkpoint yaşam döngüsünü SQLite üzerinde kur. İlk ajan görevi açıp checkpoint bırakabilmeli; sonraki ajan task bağlamını, son checkpoint'i ve geçerli koordinasyon durumunu geri yükleyebilmelidir. Bu akış worktree zorunluluğu getirmemelidir.

#### Kabul kriterleri

- [ ] Task oluşturma, devam ettirme ve kapatma durumları açıkça modellenir.
- [ ] Checkpoint yeniden başlatma ve farklı AgentSession sonrasında okunabilir.
- [ ] Aynı workspace'teki sıralı ajan devri veri kaybı veya gereksiz conflict üretmez.
- [ ] Domain, persistence ve CLI akışı birlikte integration test ile doğrulanır.

#### Durum

**Uygulandı, denetlenmedi.** Kutular bu yüzden işaretsiz: dört kriterin dördü de
kod tarafından karşılanıyor ve testleri var, ama bu kodu bağımsız kimse
notlandırmadı. MR-002'nin bulgu belgesinin birinci bölümü bu deponun ölçülmüş
temel oranıdır — taze kodun kaç kusur taşıdığını yazan sayı odur — ve bir
uygulayıcının kendi işini yeniden okuması o ölçüme cevap değildir.

Sözleşme koda başlanmadan donduruldu ([mr-003-requirements.md](mr-003-requirements.md),
`cd74767`), tasarım [mr-003-design.md](mr-003-design.md), uygulamanın kendi kaydı
ise [mr-003-findings.md](mr-003-findings.md). `make verify` ve `make tidy-check`
yeşil; 768 test (temel 732).

Milestone'un senaryosu bir test olarak çalışıyor: iki ayrı AgentSession, aynı
workspace'te, art arda, aynı Task'ı sürdürüyor — aralarında Go tarafında hiçbir
şey taşınmadan, her adım veritabanını açıp kapatan ayrı bir komut çağrısı olarak.

Uygulama sırasında iki kusur kendi kendini gösterdi ve ikisi de belgede adıyla
kayıtlı: komutlar veritabanını salt-okunur açtığı için hiçbir şey yazamıyordu, ve
mevcut "başlatılmamış depo" hata kodu bir başarısızlık nesnesinde kullanılamadığı
için altıncı bir kod eklendi.

---

### [ ] MR-004 — Güvenli lease, idempotency ve optimistic revision

- **Tür:** AFK
- **Blocked by:** MR-001, MR-003
- **Kapsadığı senaryo:** Eşzamanlı iki ajan aynı korunan hedefte sessizce çakışan aktif lease veya duplicate kayıt üretemez.

#### Ne yapılacak

Task/symbol/file lease edinme-yenileme-bırakma akışını; SQLite WAL, kısa transaction, sınırlı busy retry, `operation_id` idempotency ve optimistic revision kontrolüyle uçtan uca tamamla.

#### Kabul kriterleri

- [ ] Aynı logical target için iki process aynı anda çakışan aktif lease alamaz.
- [ ] Aynı `operation_id` ile tekrar edilen mutation duplicate Task, Change veya Evidence üretmez.
- [ ] Eski revision ile update sessiz last-write-wins yerine açık revision conflict verir.
- [ ] SQLite concurrency/race testleri bounded retry ve conflict davranışını kanıtlar.
- [ ] Transaction'lar interactive write starvation yaratmayacak kadar kısa tutulur.

---

### [ ] MR-005 — Python/TypeScript/JavaScript yapısal indeksleme

- **Tür:** AFK
- **Blocked by:** MR-001
- **Kapsadığı senaryo:** Mindrail değişen Python, TypeScript ve JavaScript fonksiyon/metot gövdelerini FULL resolver olmadan STRUCTURAL modda tanır.

#### Ne yapılacak

ProjectUnit keşfi, Tree-sitter language registry, content-addressed snapshot/cache ve incremental changed-file reindex yolunu kur. Sembol, import, açık reference/call ile body/signature/structure fingerprint üret. Büyük repository cold index'i tamamlama beklenmeden PARTIAL_READY dönebilmelidir.

#### Kabul kriterleri

- [ ] Python, TS ve JS için fixture'lar declaration, method/function body, import ve açık call/reference çıkarımını doğrular.
- [ ] Değişmemiş content hash yeniden parse edilmez.
- [ ] Kesilen indeksleme tamamlanmış hash'leri tekrar işlemeden devam eder.
- [ ] Unindexed aktif ProjectUnit, hedefe yönelik istek geldiğinde cold queue önüne alınır.
- [ ] Büyük inventory sonrasında durum PARTIAL_READY/pending olarak açıkça raporlanabilir.

---

### [ ] MR-006 — Dayanıklı `symbol_uid` tahsisi ve rename/move koruması

- **Tür:** AFK
- **Blocked by:** MR-004, MR-005
- **Kapsadığı senaryo:** Korunan bir sembol rename/move edildiğinde bağlı invariant sessizce orphan olmaz.

#### Ne yapılacak

Yapısal fingerprint ve Git sinyallerini kullanarak dayanıklı `symbol_uid` tahsisi, eşzamanlı ilk-görüş uzlaşması ve rename/move migration akışını tamamla. Kimlik güvenle korunamıyorsa tahmin etmek yerine explicit ambiguity üret.

#### Kabul kriterleri

- [ ] İki process aynı yeni sembol için tek bir `symbol_uid` üzerinde uzlaşır.
- [ ] Güvenilir rename/move, invariant ilişkisini aynı kimliğe taşır.
- [ ] Belirsiz rename korunan invariant'ı düşürmez; blocking ambiguity üretir.
- [ ] Concurrent allocation, rename, move ve ambiguity fixture testleri bulunur.

---

### [ ] MR-007 — Reconcile-first gerçek değişiklik keşfi

- **Tür:** AFK
- **Blocked by:** MR-003, MR-005, MR-006
- **Kapsadığı senaryo:** Ajan `mindrail_before_change` çağırmayı unutsa bile gerçek diff bulunur ve gate bypass edilmez.

#### Ne yapılacak

`before_change` baseline'ını bir koordinasyon optimizasyonu, `reconcile` yolunu ise doğruluk kaynağı olarak uygula. Worktree ve staged Git diff'lerini okuyup gerçek changed file/symbol setini Task/Change ile ilişkilendir; `after_change` delta'sını aynı servis üzerinden üret.

#### Kabul kriterleri

- [ ] `before_change` çağrılmış ve çağrılmamış düzenlemeler aynı gerçek diff'e uzlaşır.
- [ ] Function/method body, signature ve structure değişiklikleri Change kaydına bağlanır.
- [ ] Baseline divergence görünür ve açıklanabilir sonuç üretir.
- [ ] Git fixture testleri unstaged, staged ve ayrı worktree senaryolarını kapsar.

---

### [ ] MR-008 — Scope drift ve unregistered change ambiguity

- **Tür:** AFK
- **Blocked by:** MR-004, MR-007
- **Kapsadığı senaryo:** Beyan edilen kapsam dışındaki veya birden fazla Change'e ait olabilecek düzenleme sessizce sahiplenilmez.

#### Ne yapılacak

Reconcile sonucu ile claim/before_change kapsamını karşılaştırarak scope drift ve unregistered change bulguları üret. Tek bir Change'e güvenle bağlanamayan sembol için otomatik atama yerine explicit ambiguity ve çözüm yönlendirmesi ver.

#### Kabul kriterleri

- [ ] Beyan edilen scope dışındaki değişiklik yapılandırılmış drift bulgusu üretir.
- [ ] Birden fazla Change adayı bulunan sembol otomatik sahiplenilmez.
- [ ] Unregistered veya ambiguous değişiklik completion/verify sırasında bloklanabilir.
- [ ] Sonuç provenance ve uygulanabilir `next_action` içerir.

---

### [ ] MR-009 — Sınırlı ve açıklanabilir STRUCTURAL etki analizi

- **Tür:** AFK
- **Blocked by:** MR-002, MR-005, MR-007
- **Kapsadığı senaryo:** Değişen semboller için doğrudan yapısal etki ve ilgili invariant'lar bulunur; zayıf sinyal gereksiz tüm-suite çalıştırmaya dönüşmez.

#### Ne yapılacak

Direct structural reference, bounded reverse traversal, file/module fallback ve invariant scope eşlemesini uygula. Yapısal isim eşleşmelerini en fazla `0.5` confidence ve `STRUCTURAL_NAME_MATCH` provenance ile etiketle; varsayılan reverse depth `1` olsun.

#### Kabul kriterleri

- [ ] Her impact sonucu kaynak edge, confidence, traversal depth ve fallback nedenini açıklar.
- [ ] `depth > 1` yalnızca explicit istekte çalışır.
- [ ] Salt structural edge validation breadth'i MODULE üstüne çıkaramaz.
- [ ] Path policy, public API kuralı veya explicit invariant scope daha geniş breadth'i gerekçelendirebilir.
- [ ] Aynı isimli declaration'lar için belirsizlik kaybolmadan raporlanır.

---

### [ ] MR-010 — Güvenli validation runner ve snapshot'a bağlı evidence

- **Tür:** AFK
- **Blocked by:** MR-002, MR-007, MR-009
- **Kapsadığı senaryo:** Proje tarafından adlandırılmış doğrulama profili güvenle çalışır ve sonucu tam kaynak snapshot'ına bağlanır.

#### Ne yapılacak

Project-defined validation profile seçimi, argv-only process execution, timeout, bounded stdout/stderr, exit/result normalizasyonu, source snapshot hash ve append-only Evidence kaydını tek uçtan uca akışta sun. Bilinen secret'lar depolama öncesinde redact edilmelidir.

#### Kabul kriterleri

- [ ] Shell string yerine yalnızca argv tabanlı komut çalıştırılır.
- [ ] Timeout ve çıktı sınırı deterministic, yapılandırılmış sonuç üretir.
- [ ] Evidence; profile, command sonucu, snapshot hash, provenance ve timestamp taşır.
- [ ] Configured known secret plaintext olarak Evidence içine girmez.
- [ ] Runner unit testleri ile gerçek process integration testleri bulunur.

---

### [ ] MR-011 — Kaynak değişince evidence geçersizleştirme

- **Tür:** AFK
- **Blocked by:** MR-010
- **Kapsadığı senaryo:** Başarılı validation sonrasında kaynak değişirse eski sonuç güncel kanıt gibi kullanılamaz.

#### Ne yapılacak

Evidence snapshot'ını reconcile edilen mevcut source snapshot ile karşılaştır. İlgili kaynak değişikliğinde evidence'ı stale olarak işaretle ve validation/completion bağlamında yeniden çalıştırılacak profilleri açıkla.

#### Kabul kriterleri

- [ ] Değişmemiş snapshot için kanıt current kalır.
- [ ] Validation sonrasındaki ilgili source edit kanıtı stale yapar.
- [ ] Stale kanıt completion için required evidence şartını karşılamaz.
- [ ] Stale nedeni ve yeniden çalıştırılması gereken profil kullanıcıya açıklanır.

---

### [ ] MR-012 — Kritik doğrulama testini zayıflatma guard'ı

- **Tür:** AFK
- **Blocked by:** MR-002, MR-005, MR-007
- **Kapsadığı senaryo:** Aktif CRITICAL invariant'ın testini kaldırmak, skip etmek veya assertion azaltmak production sembolü değişmese bile bloklanır.

#### Ne yapılacak

Assertion/expect kaldırma, assertion sayısı azalması, skip/xfail/disabled marker ekleme ve kritik verification test kaldırma sinyallerini düşük maliyetli yapısal analizle yakala. Bulguyu `TEST_GUARD_WEAKENED` olarak invariant ve diff provenance'ıyla üret.

#### Kabul kriterleri

- [ ] Assertion kaldırma/azaltma ile skip/xfail/disabled ekleme fixture'ları yakalanır.
- [ ] Aktif CRITICAL invariant verification testinin kaldırılması blocking bulgu üretir.
- [ ] Yalnız test dosyasına dokunan değişiklik de guard tarafından değerlendirilir.
- [ ] Aynı guard reconcile, staged verify ve CI verify yollarında ortak servis olarak kullanılır.
- [ ] Açıklanabilir false-positive escape hatch/policy davranışı belgelenir ve test edilir.

---

### [ ] MR-013 — Yerel completion evidence gate

- **Tür:** AFK
- **Blocked by:** MR-006, MR-008, MR-009, MR-011, MR-012
- **Kapsadığı senaryo:** Ajan yalnızca protokol beyanıyla işi bitiremez; gate gerçek diff, invariant ve güncel kanıtı bağımsız değerlendirir.

#### Ne yapılacak

Completion kararını tek bir ALLOW/DENY modeli altında birleştir. Unresolved blocking invariant, missing/stale evidence, protected-symbol ambiguity ve unreconciled change ayrı reason code ve `next_action` ile DENY üretmelidir.

#### Kabul kriterleri

- [ ] Tüm denial nedenleri deterministic reason code, provenance ve çözüm önerisi taşır.
- [ ] `before_change` çağrılmamış olsa da completion önce reconcile eder.
- [ ] Gerekli current evidence ve çözümlenmiş invariant olduğunda ALLOW üretir.
- [ ] Aynı snapshot ve state için karar tekrarlanabilir/idempotent olur.
- [ ] Birincil uçtan uca kernel senaryosu ALLOW ve her DENY dalı için test edilir.

---

### [ ] MR-014 — MCP bilgi ve bağlam araçları

- **Tür:** AFK
- **Blocked by:** MR-002, MR-003, MR-009
- **Kapsadığı senaryo:** Farklı MCP client'ları aynı repository/knowledge/context sözleşmesini kullanır.

#### Ne yapılacak

Resmî MCP Go SDK üzerinden `mindrail_bootstrap`, `mindrail_status`, `mindrail_search`, `mindrail_context`, `mindrail_decide` ve `mindrail_invariant` araçlarını mevcut application/domain servislerine bağla. `context.detail_level` yalnızca `summary|focused`, invariant mode yalnızca `active` desteklemelidir.

#### Kabul kriterleri

- [ ] CLI ve MCP aynı application servislerini kullanır; davranış fork'u oluşmaz.
- [ ] Tool input/output şemaları contract testleriyle sabitlenir.
- [ ] Ertelenmiş `full`, `impact_group`, cursor/page size veya `candidate` değerleri sessizce düşürülmez.
- [ ] Desteklenmeyen değer `NOT_IMPLEMENTED_IN_THIS_VERSION` ve `next_action` döndürür.
- [ ] En az iki farklı MCP client uyumunu simüle eden contract fixture bulunur.

---

### [ ] MR-015 — MCP koordinasyon ve değişiklik araçları

- **Tür:** AFK
- **Blocked by:** MR-004, MR-007, MR-008, MR-014
- **Kapsadığı senaryo:** Ajan claim, change ve checkpoint yaşam döngüsünü MCP üzerinden yürütebilir; gerçek diff yine reconcile ile doğrulanır.

#### Ne yapılacak

`mindrail_claim`, `mindrail_before_change`, `mindrail_after_change`, `mindrail_reconcile` ve `mindrail_checkpoint` araçlarını ortak koordinasyon/değişiklik servislerine bağla; operation idempotency, revision conflict ve yapılandırılmış hata sözleşmelerini koru.

#### Kabul kriterleri

- [ ] Beş aracın başarı, idempotent retry, conflict ve ambiguity contract testleri bulunur.
- [ ] `before_change` opsiyonel optimizasyon, `reconcile` kanonik doğruluk yolu olarak kalır.
- [ ] Checkpoint farklı AgentSession tarafından okunabilir.
- [ ] Tool sonuçları pending/partial durumları ve `next_action` taşır.

---

### [ ] MR-016 — MCP validation ve completion araçları

- **Tür:** AFK
- **Blocked by:** MR-010, MR-011, MR-013, MR-014
- **Kapsadığı senaryo:** Ajan doğrulama çalıştırıp completion isteyebilir fakat kanıt kurallarını atlayamaz.

#### Ne yapılacak

`mindrail_validate` ve `mindrail_complete` araçlarını validation/evidence/completion servislerine bağla. 0.1'de validate yalnız named project profile, complete yalnız local evidence gate desteklemelidir.

#### Kabul kriterleri

- [ ] Named profile sonucu mevcut snapshot'a bağlı Evidence üretir.
- [ ] Missing/stale evidence MCP completion'da CLI ile aynı DENY sonucunu verir.
- [ ] Budget class/escalation alternative veya managed/signed approval isteği structured version error döndürür.
- [ ] MCP stdio end-to-end testi 13 aracın tamamını discover edip temel contract'larını doğrular.

---

### [ ] MR-017 — Staged değişiklik için yerel Git enforcement

- **Tür:** AFK
- **Blocked by:** MR-002, MR-008, MR-012, MR-013
- **Kapsadığı senaryo:** Commit öncesinde bozuk knowledge, unreconciled değişiklik veya zayıflatılmış kritik test reddedilir.

#### Ne yapılacak

`mindrail verify --staged`, `mindrail knowledge validate` ve mevcut hook'u ezmeden pre-commit entegrasyonunu ekle. Staged diff, aynı reconcile/invariant/test-guard/completion servisleriyle değerlendirilmelidir.

#### Kabul kriterleri

- [ ] `verify --staged` unreconciled staged değişikliği yakalar.
- [ ] Knowledge corruption/ileri şema kaynak doğrulamasından önce fail-closed olur.
- [ ] Test weakening staged yolda blocking bulgu üretir.
- [ ] Kurulum mevcut pre-commit hook içeriğini overwrite etmez.
- [ ] Git fixture testleri clean, allow ve denial senaryolarını kapsar.

---

### [ ] MR-018 — Hook bypass'a dayanıklı CI verification

- **Tür:** AFK
- **Blocked by:** MR-017
- **Kapsadığı senaryo:** `git commit --no-verify` ile oluşturulan commit bağımsız CI kontrolünü geçemez.

#### Ne yapılacak

`mindrail verify --ci` akışını fresh clone üzerinde knowledge validation, merge-base(base, head) diff, reconcile, changed symbols, invariant/test guard, required validation profiles ve structured ALLOW/DENY sırasıyla uygula. Runtime task lease, portable manifest ve cross-run impact cache ekleme.

#### Kabul kriterleri

- [ ] Base/head seçimi ve merge-base hataları yapılandırılmış biçimde raporlanır.
- [ ] `--no-verify` ile yerelde atlanan denial CI'da yeniden üretilir.
- [ ] CI, staged/local yol ile aynı policy ve domain servislerini kullanır.
- [ ] Bozuk/incompatible knowledge en erken aşamada fail-closed olur.
- [ ] Fresh-clone Git fixture testi tüm akışı doğrular.

---

### [ ] MR-019 — Warm-path SLO, pending state ve release kalite kapısı

- **Tür:** AFK
- **Blocked by:** MR-015, MR-016, MR-018
- **Kapsadığı senaryo:** Mindrail zaman bütçesini sessizce aşmaz veya analizi atlamaz; performans ve release kalitesi ölçülebilir.

#### Ne yapılacak

`status`, `context`, `before_change`, `after_change` (≤10 dosya) ve `reconcile` (≤10 dosya) için benchmark/telemetry kur. Bütçe içinde bitmeyen iş explicit partial/pending dönmeli ve kalan işi öncelikli schedule etmelidir. Platform-native binary build ve release kalite kapısını otomatikleştir.

#### Kabul kriterleri

- [ ] STRUCTURAL p95 hedefleri sırasıyla `<150 ms`, `<250 ms`, `<300 ms`, `<1.5 s`, `<2 s` olarak raporlanır.
- [ ] SQLite wait, parse, impact traversal ve operasyon süreleri ölçülür.
- [ ] Bütçe aşımı blocking bekleme veya sessiz skip yerine partial/pending sonuç üretir.
- [ ] Unit, domain, integration, race, knowledge schema, MCP contract, Git worktree, SQLite concurrency ve end-to-end test kapıları çalışır.
- [ ] Desteklenen platform binary'leri build edilir ve smoke testten geçer.

---

### [ ] MR-020 — 0.1 çıkış koşulu kabul incelemesi

- **Tür:** HITL
- **Blocked by:** MR-019
- **Kapsadığı senaryo:** Ürün sahibi, kernel hipotezinin yalnız bileşen testleriyle değil gerçek bypass denemeleriyle kanıtlandığını onaylar.

#### Ne yapılacak

Temiz bir örnek repository ve iki ajan/worktree senaryosu üzerinde 0.1 exit condition demonstrasyonu hazırla. Unutulan protocol çağrısı, local hook bypass, protected-symbol rename, kritik test zayıflatma ve stale test sonucu iddiasını ayrı ayrı dene; her birinin bağımsız gate tarafından reddedildiğini kaydet.

#### Kabul kriterleri

- [ ] Birincil end-to-end senaryonun başarılı ALLOW yolu demo edilir.
- [ ] `before_change` unutma reconcile tarafından yakalanır.
- [ ] `--no-verify`, CI verification tarafından yakalanır.
- [ ] Protected-symbol rename kimliği korur veya explicit ambiguity ile bloklanır.
- [ ] Kritik test zayıflatma production code değişmese bile bloklanır.
- [ ] Stale evidence completion proof'u olarak reddedilir.
- [ ] Ürün sahibi sonuçları ve 0.1 release kararını kayda geçirir.

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

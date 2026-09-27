# Mindrail 0.1 — birleşik takım denetimi

**Tarih:** 2026-09-26 · **Kaynak:** `2814acb22d8aa94d48ade1242b58e1fdb14239fc` · **Karar:** `NOT_VERIFIED`

Proje derleniyor; mevcut normal/race/smoke testleri, dokuz kategorili kalite kontrolü, release üretimi ve benchmark'lar geçiyor. Ancak tamamlamanın güvenilir bir mühendislik kapısı olduğu iddiasını bozan dört davranış hatası izole ortamlarda üretildi. Ayrıca yayımlanan binary'de kullanıcı tarafından başlatılabilir MCP sunucusu bulunmuyor. “20 görev kapandı” kaydı doğru; “ürünün bütün vaatleri eksiksiz ve sorunsuz çalışıyor” sonucu bu kayıtla desteklenmiyor.

Kullanım için [Türkçe kılavuz](../../usage-tr.md); ölçümler için [merkezi doğrulama](verification.md); ayrıntılı bağımsız kanıtlar için [Reader](reader.md), [Breaker](breaker.md) ve [dokümantasyon raporu](documentation.md).

> **Tarihsel snapshot notu (remediation sonrası eklendi):** Bu rapordaki karar, yalnız yukarıdaki `2814acb` kaynak ağacı içindir ve silinmez/değişmez. Sonraki remediation çalışma ağacı schema 9 guard baseline'ı, `mindrail mcp` stdio launcher'ı, fail-closed knowledge/canonical reconcile ve `task complete` evaluator'ünü ekler. Bu nedenle aşağıdaki AUD-01…AUD-04 açıklamaları bugünkü davranış iddiası değil, tarihsel karşı örnek kanıtıdır. Güncel kullanım semantiği [kılavuzda](../../usage-tr.md); sonraki doğrulama ayrı bir hedefli test koşusu gerektirir.

## 1. Original Task

> Proje durumunu kontrol et. Tüm görevlerin bittiği bildirildi. Herşey sıkıntısız çalışıyor mu, bir sorun var mı takım olarak denetlet. Takımlara farklı modeller görev zorluğuna göre verebilirsin. Daha sonra bana detaylı bir usage dökümanı hazırla.

Paralel bağımsız ajanlar kullanıldı: GPT-5.6 Sol / yüksek muhakeme Reader ve Go sözleşme incelemesi; GPT-6 Astra / çok yüksek muhakeme Breaker; GPT-5.6 Terra / yüksek muhakeme dokümantasyon ve kullanım örnekleri. Ana ajan test/release kontrollerini ve kanıtların uzlaştırılmasını yönetti. Reader ile Breaker kendi ilk raporlarını yazmadan birbirlerinin sonuçlarını görmedi.

Denetim yalnız son dokümantasyon commit'inin diff'iyle sınırlı değil: tamamlandığı bildirilen 0.1 akışları ve MR-001…MR-020 kayıtları esas alındı. Üretim kaynaklarına düzeltme uygulanmadı; tehlikeli/mutasyon denemeleri ayrı geçici kopyalarda yapıldı.

Altı Breaker ekseni ele alındı: seçilmiş guard mutasyonları; silinen davranış A/B uygulanabilirlik kontrolü; yazılı tehdit modeli; upgrade; en zayıf kabul edilen girdiler; aynı verinin farklı okuyucularla karşılaştırılması. **Bütün repository'deki her guard için exhaustive mutation yapılmadı.** Son release kapanışı runtime kod silmediğinden o diff için çalıştırılabilir silme A/B örneği bulunmadı; tüm Git geçmişinin davranış eşdeğerliği doğrulanmış sayılmaz.

## 2. Requirement Matrix

| İstek / ürün vaadi | Kanıt | Sonuç |
|---|---|---|
| Proje durumunu doğrula | HEAD, 20/20 işaretli MR başlığı, kapanış kayıtları | Kayıtlar kapalı; davranış uygunluğu ayrı değerlendirildi. |
| Çalışabilir build ve test akışı | `make verify`, `tidy-check`, `gate`, `bench`, `release` | PASS; sınırlar verification.md'de. |
| Başarılı ve güncel kanıtla completion | AUD-01, AUD-02 | FAIL |
| Bilinmeyen knowledge şemasında fail-closed davranış | AUD-03 | FAIL; CLI ve MCP ayrışıyor. |
| Protokol çağrısı atlandığında gerçek diff'i bulma | AUD-04 | FAIL; MCP completion keşfi kendisi başlatmıyor. |
| Kullanılabilir MCP agent entegrasyonu | 13 internal handler, public launcher 0; `mcp` exit 2 | PARTIAL; bilinçli daraltma üst ürün vaadini tamamlamıyor. |
| Minimum proof/policy'yi ajanın düşürememesi | `required` atlama; CLI/CI'da profil çalıştırma yok | PARTIAL / sözleşme çatışması |
| Detaylı gerçek kullanım belgesi | `docs/usage-tr.md`, disposable repo örnekleri | Teslim edildi; mevcut erişim ve audit sınırları açık. |

MR bazındaki ayrıntılı matris Reader raporundadır. Alt-servis düzeyindeki PASS ile bütün kullanıcı akışının PASS olması aynı şey değildir.

## 3. Confirmed Findings

### AUD-01 — HIGH: başarısız doğrulama ve eksik profil başarısı completion için yeterli sayılıyor

**Kaynak:** Breaker B-01. **Dosyalar:** `internal/validation/freshness.go:94`, `internal/validation/service.go:49`.

Gerçek subprocess çıktılarıyla aynı kapsam üzerinde ölçüm:

| Girdi | Evidence | Başarılı komut | ALLOW | Denial |
|---|---:|---:|---|---:|
| Henüz validate yok | 0 | 0 | false | 1 |
| `true` | 1 | 1 | true | 0 |
| `false` | 1 | 0 | true | 0 |
| Bulunamayan executable | 1 | 0 | true | 0 |
| `true`, sonra `false` | 2 | 1 | true | 0 |

Freshness hesabı satırın durum/exit code başarısını zorunlu tutmuyor; tek başarılı satırın bütün çok-komutlu profile yetmesi de ayrı bir eksik. Kullanıcı başarısız testin tamamlamayı durdurduğunu varsayamaz.

**Öneri ve ölçüm:** Scratch'te `StatusPass && ExitCode == 0` eklemek tek-komutlu fail/error senaryolarını DENY'a çevirdi ve `true` kontrolünü korudu. Karma profil hâlâ ALLOW kaldı. Bu tek satırlık değişiklik yeterli çözüm değildir. Profil çalıştırması/run kimliği ve bütün gerekli komutların sonucu birlikte değerlendirilmelidir; tam çözüm henüz ölçülmedi.

**Sınıf taraması:** `validation.Check` → MCP compose → `gate.evidenceDenials`; CLI verify bu coverage yolunu hiç oluşturmuyor. Tam test sürücüsü ve karşı inceleme Breaker/Reader eklerindedir.

### AUD-02 — HIGH: doğrulama kapsamına yeni dosya eklemek eski kanıtı bayatlatmıyor

**Kaynak:** Breaker B-02. **Dosyalar:** `internal/validation/service.go:54`, `internal/validation/freshness.go:69`.

| Aynı dizin profili | Dosya sayısı önce → sonra | ALLOW önce → sonra | Son denial |
|---|---|---|---:|
| Değişiklik yok | 1 → 1 | true → true | 0 |
| Var olan dosyayı değiştir | 1 → 1 | true → false | 1 |
| Kapsama `assert False` içeren yeni dosya ekle | 1 → 2 | true → true | 0 |

Provenance yalnız ilk anda bulunan somut dosyaları saklıyor; freshness dizin kapsamını yeniden keşfetmiyor. **Öneri:** orijinal kapsam/policy kimliğini saklayıp dosya setini yeniden enumerate etmek; eski evidence için fail-closed uyumluluk yaklaşımı. Tam çözüm ölçülmedi. Kabul testi ekleme/silme/içerik değişikliği yanında kapsam dışı eklemenin reddedilmemesini de doğrulamalı.

**Sınıf taraması:** tek writer `SnapshotScope`, tek rehash okuru; dizin tabanlı profiller aynı tasarımı paylaşır. Reprodüksiyon Breaker B-02 ekinde.

### AUD-03 — HIGH: fatal knowledge problemi MCP completion tarafından göz ardı ediliyor

**Kaynak:** Breaker B-03. **Dosyalar:** `internal/mcp/complete.go:184`, `internal/mcp/server.go:180`.

| Aynı repository | Bildirilen knowledge problem | Completion | Durum / verify |
|---|---:|---|---|
| Geçerli kontrol | 0 | ALLOW | READY |
| `schema_version=999`, mevcut MCP server | status'ta 0 | ALLOW | READY |
| Aynı bozuk knowledge, yeni CLI verify | 1 fatal | — | DENY / `KNOWLEDGE_SCHEMA_UNSUPPORTED` |
| Aynı bozuk knowledge, yeni MCP server | 1 | ALLOW | BLOCKED |

MCP knowledge loader sonucu içindeki `Problems` completion tarafından düşürülüyor. Server restart da completion sonucunu düzeltmiyor; yalnız status cache'i sorunu değil. Uzun yaşayan status'un eski READY bilgisi ayrı bir görünürlük kusuru.

**Öneri:** completion'a CLI ile aynı fatal knowledge engelini taşımak ve uzun yaşayan okuyucuları güncel gözlemle yenilemek. Uygulanmadı/ölçülmedi. Geçerli kayıt ALLOW kalmalı; mevcut/yeni MCP server ve CLI aynı bozuk kayıt için fail-closed davranmalı. Sınıf taraması ve payload Breaker B-03 ekinde.

### AUD-04 — HIGH: completion öncesi canonical reconcile yok

**Kaynak:** Reader F-01 + Breaker B-04. **Dosyalar:** `internal/mcp/complete.go:66`, `internal/changes/evaluation.go:100`.

| Aynı task ve kaynak | Değişmiş dosya | Denial | ALLOW |
|---|---:|---:|---|
| Temiz kontrol | 0 | 0 | true |
| Kaynak değişti; before/after/reconcile atlandı | 1 | 0 | true |
| Aynı byte'lar, yalnız explicit reconcile eklendi | 1 | 2 | false |

Son kol `SCOPE_DRIFT` ve `UNREGISTERED_CHANGE` üretir. Bu, task-list MR-013'teki “completion önce reconcile eder” kriteri ve kernel'in unutulan çağrı güvencesiyle çatışır. CLI staged/CI'nin reconcile yapması MCP completion'ın eksiğini kapatmıyor.

**Ek public CLI kontrolü:** gerçek binary'de temiz task da kaydedilmemiş kaynak değişikliği bulunan task da `READY_TO_COMPLETE → COMPLETED` geçişini exit 0 ile yaptı; revision 5 kalıcılaştı ve lease bırakıldı. Verification öncesi SQLite kontrolünde change/evidence sayıları sıfırdı. Ardından aynı değişiklik için `verify --staged`, `UNREGISTERED_CHANGE` ile exit 1 verdi; task `COMPLETED` kaldı ve terminal durumdan tekrar açılmadı. MR-003/D-55 ham koordinasyon geçişine izin verir: bu sonuç bir gate `ALLOW` kararı veya `verify` bypass kanıtı değildir. Ancak “task bitti” durumu kanıt değerlendirmesiyle bağlanmamıştır. Ayrı bir beşinci hata olarak sayılmadı; tekrar üretim Breaker B-04 ekindedir.

**Öneri:** karardan önce gerçek diff'in tam ve başarılı keşfini zorunlu kılmak; elde edilen snapshot'ı sonraki kontrollerde kullanmak. Uygulanmadı/ölçülmedi. Temiz task, kapsam içi meşru değişiklik ve başka task'ın değişikliği için aşırı reddetme testleri de gerekir. Aynı ilan edilmiş dosyadaki her yeni edit'in sırf edit olduğu için DENY olması gerektiği iddia edilmez.

### Entegrasyon ve kapsam kararları

Bu maddeler yukarıdaki dört canlı hata ile aynı türden değildir. Bazıları alt milestone kararlarına uygundur; yine de üst düzey ürün vaadinin tamamlandığını kanıtlamaz:

| Konu | Üretilen/okunan kanıt | Etki ve sınıflandırma |
|---|---|---|
| Production MCP launcher yok | Binary `mcp` exit 2; version `none`; internal handler 13, public başlatma girişi 0. D-191/D-209 bunu erteler. | HIGH kullanılabilirlik/kapsam açığı: agent istemcisine çalışan bağlantı ayarı verilemiyor. |
| Required evidence listesi caller-controlled | Aynı stale evidence: `required:["test"]` → DENY/1; alan yok → ALLOW/0. D-213 boş listeyi açıkça kabul eder. | HIGH politika/sözleşme açığı: frozen alt sözleşmeye göre kasıtlı, üst “minimum proof düşürülemez” vaadine göre eksik. Kazara regresyon olarak sunulmuyor. |
| CLI staged/CI validation profile çalıştırmıyor | `verify.Service.compose` Coverage üretmiyor; D-226 bunu açıkça dışlar. | HIGH kapsam açığı: `verify --ci` başarılı test kanıtı garantisi vermez. Kullanıcı test/lint/build'i ayrıca çalıştırmalı. Bu tur bunun için ayrıca config-to-CI canlı fail-open ölçümü üretilmedi. |
| Canlı budget/partial kablosu yok | Production'da `NewBudget`/`WithBudget` çağrısı yok; yalnız tanımlar. Task kaydı canlı-op ertelemesini açıkça söylüyor. | MEDIUM entegrasyon açığı: benchmark PASS, canlı timeout/partial davranışının kanıtı değildir. Ölçülmüş gecikme ihlali iddia edilmiyor. |
| Kullanım ve release belgeleri farklı durum anlatıyor | README “under construction”, kapanış “Ship 0.1”, managed AGENTS eski. | Doküman drift'i; detay ve severity [documentation.md](documentation.md). Yeni rehber eklendi, tarihsel kaynaklar yeniden yazılmadı. |

## 4. Refuted Findings

| İddia | Çürüten/daraltan kanıt | Sonuç |
|---|---|---|
| “Stdio adlı test byte stream/framing sınamıyor” | SDK v1.8.0 `NewInMemoryTransports` `net.Pipe()` kullanıyor; InMemory ve Stdio aynı `newIOConnLimited` çerçeveleme yoluna gidiyor. Test geçti. | Bu gerekçe geri çekildi. Gerçek OS stdin/stdout subprocess launcher testi ve production komutu eksikliği geçerli. |
| “Daha önce kaydedilmiş dosyadaki her yeni edit otomatik DENY olmalı” | Breaker'ın temiz committed fixture'ında aynı sahipli dosyanın sonraki edit'i, explicit reconcile sonrasında da ALLOW / 0 denial verdi. | Aşırı geniş yorum kullanılmadı. AUD-04, kaydedilmemiş değişiklik için explicit reconcile'ın aynı kaynağı iki denial'a çevirdiği bağımsız ölçüme dayanır. |

Reader, Breaker'ın B-01…B-04 sürücülerini kendi ayrı scratch kopyasında çalıştırdı (1.857 s) ve bütün kontrol/senaryo farklarını yeniden üretti. Dört bulgunun hiçbiri çürütülmedi. Birebir karşı denemeler ve nihai görüşler bağımsız raporların reconciliation eklerindedir.

Şiddet uzlaştırması: dört runtime hatası her iki ajan tarafından HIGH kabul edildi. Reader, bilinen D-213/D-226 daraltmalarını MEDIUM; Breaker bağımsız minimum-policy eksikliğini bir HIGH ürün-kapsam grubu olarak değerlendirdi. Ana rapor, üst düzey minimum-proof güvencesini vermeyi engellediği için bu birleşik politika/CI boşluğunu HIGH tutuyor; bunu iki ayrı rastlantısal kod hatası diye saymıyor. Canlı budget bağlantısı MEDIUM: eksik bağlantı kanıtlandı, gecikme ihlali ölçülmedi.

## 5. Open Items

| Sınır | Neden bu sonuçla kapatılmadı | Kesin sonraki deney |
|---|---|---|
| Tam düzeltmeler | Görev denetim + kullanım belgesiydi; üretim kodu değiştirilmedi. | AUD-01…04 sürücülerini düzeltme branch'inde önce RED, sonra GREEN göster; bütün kontrolleri ve aşırı reddetme kollarını koru; tam verify tekrar koş. |
| Supply-chain güvenliği | `govulncheck` yok, `make vuln` SKIPPED. | Uygun tool kurulu ortamda `govulncheck ./...`; exit/output ve DB tarihi kaydedilsin. |
| Gerçek MCP istemci süreci | Public launcher yok. | Launcher geldiğinde built executable'ı stdin/stdout ile başlat; gerçek SDK istemcisi 13 tool'u listeleyip lifecycle'ı tamamlasın ve adversarial kollar DENY olsun. |
| CI minimum validation profilleri | D-226 kapsam daraltması; canlı minimum policy yok. | Named required profile ve changed scope içeren fresh clone'da başarılı, başarısız, eksik ve stale proof kollarını `verify --ci` ile karşılaştır. |
| Canlı budget/partial | Entegrasyon yok; fixture benchmark'ı bunu sınamaz. | Beş operasyon için hedefin altı/üstü budget ver; elapsed, partial/pending ve scheduler sırasını ölç. |
| Exhaustive guard/test kapsamı | Mutasyon örneklemesi 3 guard; geniş ownership enumeration opt-in kapalı. | Önce guard envanteri çıkar; izole mutation planını risk dilimlerine böl. Opt-in testin explicit ortamını açarak ayrı çalıştır; sonuçlarını örnekleme ile karıştırma. |

## 6. Off-Spec Headings

| Başlık | Sonuç / kanıt |
|---|---|
| Maliyet / worst case | Validation timeout her komutta 600 s: N komut için N×600 s; 100 komut 60.000 s (16 sa 40 dk). Tarihsel E evidence × F dosya × B byte, E×F×B hash-okuma işi. Stream capture üst toplamı komut başına 131.096 byte; 100 komut 13.109.600 byte, redaction/JSON ek yükü hariç. Bunlar kod sabitlerinin çarpımıdır; büyük-N gecikme ölçümü değildir. |
| Mekanik kurallar | Schema fail-closed, başarılı/current proof ve unutulan çağrı keşfi uçtan uca eksik. Mevcut test/gate altyapısı var; 3 seçilmiş mutant öldürüldü, surviving mutant 0. Bu, tüm guard'ların kapsandığını kanıtlamaz. |
| Backward compatibility | Odaklı migration/upgrade testleri geçti. AUD-02 düzeltmesi eski provenance için ayrıca uyumluluk politikası gerektiriyor; önerilen tam düzeltmelerin upgrade güvenliği henüz ölçülmedi. |
| Class sweep | Her AUD kaydı Breaker'ın aynı biçimli writer/reader/caller taramasına bağlanıyor. Ayrı yeni “test eksik” bulguları aynı kök nedenleri şişirmek için açılmadı. |

## 7. Scope Compliance

İstenen takım denetimi ve kullanım belgesi üretildi. Denetimde kaynak kodu, migration veya kullanıcı verisi düzeltilmedi; commit/push/release yayını yapılmadı. Eski “Ship 0.1” kararı tarihsel kayıt olarak korunuyor; bu denetimin mevcut revizyona dair teknik hükmü farklı.

Alt görevlere yazılmış bilinçli daraltmalar üst kernel acceptance kriterlerini kendiliğinden karşılamaz. Bu yüzden “birebir tamamlandı” veya “bypass edilemez” denemez. Öncelik sırası: dört davranış hatası; minimum proof/CI policy entegrasyonu; production MCP; canlı bütçe ve doküman uyumu. Yeni MCP launcher eklenmesi, completion kusurlarının da giderildiği anlamına gelmeyecek.

## 8. Test & Verification Assessment

Tam komutlar, cache ve skip bilgileri, platform matrisi ve p50/p95 değerleri [verification.md](verification.md)'de. 1308 test fonksiyonu sayıldı; verbose koşu 1306 top-level PASS ve iki opt-in/ortam SKIP gösterdi. Bağımsız yeni karşı örnekler mevcut suite'e eklenmedi; audit raporlarında tekrar üretilebilir sürücüler olarak korundu.

Test başarısı ile gereksinim kapsamı farklıdır: başarısız proof, yeni scope dosyası ve knowledge reader farkları mevcut suite yeşilken üretildi. `make verify`, `gate`, `bench`, `release` başarılı; ürünün completion doğruluğu başarısız.

## 9. Final Consensus

**Reader — REJECT.** Üst görev/kernel kriterleri ile daraltılmış uygulama arasındaki farklar ve canlı completion bypass kanıtları release onayını desteklemiyor.

**Breaker — REJECT.** Dört HIGH davranış kusuru gerçek Git/SQLite/SDK/subprocess katmanlarıyla üretildi. Yeşil mevcut testler bunları çürütmüyor. Byte-framing gerekçesi düzeltilerek launcher bulgusu daraltıldı.

Mod: farklı modellerle bağımsız paralel ilk raporlar, ardından karşı kanıt/refutation turu. Hiçbir modelin tek başına “görmedim” demesi bir bulguyu elemek için kullanılmadı.

# FINAL VERDICT

**NOT_VERIFIED.** Mevcut CLI altyapısı, test ve release araçları çalışıyor. Ancak completion kararının başarılı/güncel/tam kanıta ve gerçek diff'e bağlı olduğu güvencesi sağlanmıyor; public MCP erişimi de eksik. [Kullanım kılavuzu](../../usage-tr.md) mevcut işleyen yüzeyi ve bu sınırları anlatır, ürünü güvenilir bir bypass engeli olarak onaylamaz.

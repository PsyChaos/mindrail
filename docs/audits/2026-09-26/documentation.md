# Mindrail 0.1 dokümantasyon tutarlılık denetimi

**Denetim tarihi:** 2026-09-26
**İncelenen revizyon:** `2814acb`
**Kapsam:** 0.1 release beyanı, kullanıcıya dönük CLI/MCP/kural yapılandırması ve release işlemleri. Mühendislik tasarım/finding dosyaları tarihsel kanıt olarak incelendi; her eski görev kaydı güncel kullanım kılavuzu gibi değerlendirilmedi.

## 1. Executive Summary

> **Tarihsel snapshot notu (remediation sonrası eklendi):** Bu denetim `2814acb` kaynak ağacı için doğrudur. Sonraki çalışma ağacında `mindrail mcp`, schema 9 guard baseline, canonical reconcile/fail-closed knowledge ve değerlendirilen `task complete` geldi. Bu rapordaki DOC-002, DOC-006 ve ilgili “launcher yok” ifadeleri tarihsel bulgudur; kanıt korunur, güncel kullanıcı davranışı için [Türkçe kılavuz](../../usage-tr.md) otoritedir.

Dokümantasyonun tasarım ve kabul-kanıtı katmanı ayrıntılı; fakat kullanıcının bugün çalıştırabileceği yüzey için güvenilir bir yayın kılavuzu değildir. En tehlikeli sorun, `mindrail mcp`/stdio sunucusu ve 13 araçlık wire sözleşmesinin kodda bulunduğu halde dağıtılan CLI tarafından başlatılamamasıdır. `mindrail version --json` bunu dürüstçe `"mcp_compatibility":"none"` diye bildirir; buna rağmen teknik yığın belgesi `mindrail mcp` komutunu birincil taşıma olarak gösterir.

İkinci önemli drift, README'nin hâlâ “0.1 kernel under construction” demesi, görev listesinin ise “Ship 0.1” kapanışını kaydetmesidir. Bu denetim, release beyanını tam kullanıcı akışının mevcut olduğu şeklinde onaylamaz: yerel CLI koordinasyonu, hook ve `verify` kullanılabilir; MCP üzerinden değişiklik keşfi, doğrulama kanıtı ve tamamlatma ise dış bir MCP istemcisinden erişilebilir değildir.

## 2. Documentation Inventory

| File | Purpose | Status | Authority | Notes |
| --- | --- | --- | --- | --- |
| `README.md` | Başlangıç, build ve depo konumu | PARTIALLY_STALE | Açıklama | Kullanıma dönük tek kök belge; release durumu eski. |
| `docs/specification/mindrail-technical-specification-1.0.md` | Ürün sözleşmesi/hedef mimari | ACTIVE | Bağlayıcı spesifikasyon | 1.0 hedefini tanımlar; tek başına mevcut binary kılavuzu değildir. |
| `docs/specification/mindrail-0.1-kernel-scope.md` | 0.1 kapsamı ve AC'ler | ACTIVE | Bağlayıcı spesifikasyon | MCP wire yüzeyini tanımlar, process launcher'ı tanımlamaz. |
| `docs/specification/mindrail-tech-stack.md` | Teknoloji ve hedef çalışma modeli | PARTIALLY_STALE | Tasarım spesifikasyonu | `mindrail mcp` anlatımı 0.1 D-191 ile çatışır. |
| `docs/engineering/mindrail-0.1-task-list.md` | Milestone kapanış kaydı | HISTORICAL | Release/uygulama kaydı | MR-020 “Ship 0.1” der; kullanıcı kılavuzu değildir. |
| `docs/engineering/mr-014-requirements.md` | MCP ilk altı araç sözleşmesi | HISTORICAL | Dondurulmuş gereksinim | D-191, CLI launcher'ının 0.1'de olmadığını açıkça söyler. |
| `docs/engineering/mr-016-requirements.md` | 13 araç/evidence sözleşmesi | HISTORICAL | Dondurulmuş gereksinim | stdio kanıtı process değil, in-process pipe'tır. |
| `AGENTS.md` | Ajan çalışma protokolü | PARTIALLY_STALE | Ajan yönergesi | Yönetilen blok “0.1 uygulanmadı/araçlar yok” der; kodla çelişir. |

## 3. Documentation ↔ Documentation Findings

| # | Document A | Document B | Topic | Conflict | Severity | Confidence | Action |
| --- | --- | --- | --- | --- | --- | --- | --- |
| DOC-001 | `README.md:11-12` | `mindrail-0.1-task-list.md:949-951` | 0.1 durumu | README “under construction”; kapanış kaydı “Ship 0.1” der. | MEDIUM | VERY_HIGH | FIX_DOC |
| DOC-002 | `mindrail-tech-stack.md:512-521` | `mr-014-requirements.md:103-106`, `mr-016-requirements.md:83-87` | MCP başlatma | Teknik yığın `mindrail mcp` stdio komutunu anlatır; dondurulmuş 0.1 gereksinimleri bu komutu kasıtlı olarak dışlar. | HIGH | VERY_HIGH | SYNC_BOTH |
| DOC-003 | `AGENTS.md:5-7` | `mindrail-0.1-task-list.md:686-778` | MCP araç varlığı | Yönetilen yönerge araçların uygulanmadığını söyler; görev listesi araçların landing'ini kaydeder. | MEDIUM | HIGH | MANUAL_VERIFY |

## 4. Documentation ↔ Code Findings

| # | Document | Section | Code Reference | Category | Severity | Confidence | Action |
| --- | --- | --- | --- | --- | --- | --- | --- |
| DOC-001 | `README.md:11-12` | Status | `internal/cli/root.go:109-120` | DOC_OUTDATED | MEDIUM | VERY_HIGH | FIX_DOC |
| DOC-002 | `mindrail-tech-stack.md:512-521` | MCP SDK / transport | `internal/cli/root.go:101-112`; `internal/cli/version.go:27-31` | SPEC_IMPLEMENTATION_CONFLICT | HIGH | VERY_HIGH | SYNC_BOTH |
| DOC-003 | `AGENTS.md:5-7` | Managed protocol status | `internal/mcp/server.go:22-35`, `internal/mcp/server.go:124-127` | DOC_OUTDATED | MEDIUM | VERY_HIGH | MANUAL_VERIFY |
| DOC-004 | kullanıcı kılavuzu yok | CLI install/işletim | `Makefile:18-43`, `internal/cli/root.go:101-112` | CODE_UNDOCUMENTED | HIGH | VERY_HIGH | ADD_DOC |
| DOC-005 | kullanıcı kılavuzu yok | config/profile sınırları | `internal/config/config.go:52-106`, `internal/config/env.go:16-23` | CONFIG_MISSING_IN_DOC | MEDIUM | VERY_HIGH | ADD_DOC |
| DOC-006 | kullanıcı kılavuzu yok | completion güvenlik sınırı | `docs/audits/2026-09-26/report.md:55-137`; `internal/mcp/complete.go:25-92` | CODE_UNDOCUMENTED | HIGH | VERY_HIGH | ADD_DOC / FIX_CODE |
| DOC-007 | kullanıcı kılavuzu yok | CI validation profile sınırı | `internal/verify/verify.go:45-57`; `docs/engineering/mr-018-requirements.md:97-104` | CODE_UNDOCUMENTED | HIGH | VERY_HIGH | ADD_DOC |

## 5. Detailed Findings

### DOC-001

**Documents:** `README.md:11-12`; `docs/engineering/mindrail-0.1-task-list.md:949-951`
**Code references:** `internal/cli/root.go:109-120`; `Makefile:18-43`
**Current documentation:** README, 0.1'in hâlâ yapım aşamasında olduğunu söyler. Görev listesi MR-020'yi tamamlanmış ve “Ship 0.1” olarak kaydeder.
**Actual implementation:** Binary 11 kayıtlı CLI komutu (ayrıca Cobra'nın `help`/`completion` komutları) ve koordinasyon/hook/verify akışlarını sağlar.
**Cross-document state:** Kapanış kaydı release kararı olarak daha yenidir; README güncellenmemiştir.
**Root cause:** Milestone kapanışından sonra kullanıcı yüzlü README yenilemesi yapılmamış görünür.
**Impact:** Kullanıcı mevcut CLI yüzeyinin olgunluk durumunu yanlış anlar; doğrudan çalışma zamanı hatası üretmez. MCP launcher belirsizliği ayrı ve daha yüksek etkili DOC-002'dir.
**Authority decision:** Çalışabilir yüzey için binary/help; release kararı için MR-020 kaydı.
**Required fix:** README durumunu, kullanılabilir CLI ve MCP launcher eksikliğiyle birlikte güncelle; bu denetimde kapsam dışı olduğundan yazılmadı.

### DOC-002

**Documents:** `docs/specification/mindrail-tech-stack.md:512-521`; `docs/engineering/mr-014-requirements.md:103-106`; `docs/engineering/mr-016-requirements.md:83-87`
**Code references:** `internal/cli/root.go:101-112`; `internal/cli/version.go:27-31`; `internal/mcp/server.go:108-127`
**Current documentation:** Teknik yığın stdio taşımasını `mindrail mcp` olarak gösterir. MR-014/MR-016 aynı sürümde process komutunun bulunmadığını söyler.
**Actual implementation:** `internal/mcp.Server` 13 tool kaydeder, ancak kök Cobra komutuna MCP/serve komutu eklenmemiştir; version çıktısı `none`dır.
**Cross-document state:** D-191 ve D-209 bu farkın bilerek ertelendiğini kanıtlar; bu nedenle kod hatası değil, kullanıcı yüzlü hedef/mevcut durum ayrımıdır.
**Root cause:** Hedef teknik-yığın metni 0.1 ertelenmiş taşıma kararından sonra uyarlanmadı.
**Impact:** İstemci yapılandırması yapan kişi var olmayan executable subcommand'u çalıştırır; agent lifecycle'ın erişilebilir olduğunu sanır.
**Authority decision:** Mevcut dağıtım davranışı CLI/help ve version'dır.
**Required fix:** Teknik yığındaki örneği “gelecek taşıma” diye işaretle veya launcher gelene kadar kaldır; kullanım belgesi bağlantı örneği vermemeli.

### DOC-003

**Documents:** `AGENTS.md:5-7`; managed protocol bloğu
**Code references:** `internal/mcp/server.go:22-35`, `internal/mcp/server.go:124-127`
**Current documentation:** “Mindrail 0.1 uygulanmadı” ve MCP tools yok der.
**Actual implementation:** Server paketi 13 tool tanımlar.
**Cross-document state:** Task list de araçların tamamlandığını kaydeder; fakat executable transport yoktur.
**Root cause:** Yönetilen blok init tarafından sahiplenilmek üzere şablon olarak bırakılmış olabilir.
**Impact:** Ajanlar gerçek servis paketini tamamen yok sayabilir veya tersine kullanılabilir sanabilir.
**Authority decision:** Blok yönetilen olduğundan el ile düzeltilemez. Disposable Git repo probunda `mindrail init` sonrasında `AGENTS.md` oluşmadı; dolayısıyla bu binary blok rewrite işlevini henüz sağlamaz.
**Required fix:** Bu intentional deferred davranışı release/user dokümanında açık tutun; init AGENTS dosyasını yönetmeye başladığında şablonu güncelleyin. El ile değişiklik yapmayın.

### DOC-004

**Documents:** Kullanıcı odaklı kurulum/kullanım belgesi yok.
**Code references:** `Makefile:18-43`; `internal/cli/root.go:101-112`; `internal/cli/coordination.go:91-110`
**Current documentation:** README yalnız build hedeflerini verir.
**Actual implementation:** Init, session/task/lease/checkpoint, hook ve verify komutları çalışır; JSON envelope döner.
**Cross-document state:** Task/finding kayıtlarında kanıt var, fakat operatöre yönlendiren tek belge yok.
**Root cause:** Release kapanışı mühendislik kanıtını kullanıcı rehberine dönüştürmemiştir.
**Impact:** Kullanıcı yanlışlıkla MCP komutu arar, oturum kimliğini uydurur veya `verify` kapsamını yanlış anlar.
**Authority decision:** CLI `--help` ve scratch-repo çalıştırması.
**Required fix:** `docs/usage-tr.md` eklendi.

### DOC-005

**Documents:** Kullanıcı odaklı config referansı yok.
**Code references:** `internal/config/config.go:52-106`; `internal/config/env.go:16-23`; `internal/config/templates/config.toml:1-19`
**Current documentation:** README config katmanlarını, geçerli validation profile türlerini veya env anahtarlarını açıklamaz.
**Actual implementation:** Repo/user/env/flag katmanları; strict TOML; yalnız dört konfigürasyon env anahtarı vardır.
**Cross-document state:** Teknik spesifikasyon vardır fakat pratik örnek yoktur.
**Root cause:** Configuration API'si kullanıcı belgesine taşınmamış.
**Impact:** Profil/yol/argv yanlış yapılandırması, sessiz olmadığından hızlı ret; yine de gereksiz kurulum maliyeti.
**Authority decision:** `Config.Validate` ve embedded template.
**Required fix:** `docs/usage-tr.md` geçerli minimal örneği ve precedence'i verir.

### DOC-006

**Documents:** Kullanıcı odaklı completion güvenlik sınırı yok.
**Code references:** `docs/audits/2026-09-26/report.md:55-137` (AUD-01…04); `internal/mcp/complete.go:25-92`; `docs/engineering/mr-016-requirements.md:109-111`
**Current documentation:** Release kapanışı bypass denemelerini reddeden bir 0.1 gate'i anlatır; kullanıcı belgesi, `required` boş bırakıldığında evidence talebinin boş olduğunu açıklamıyordu.
**Actual implementation:** `CompleteIn.Required` opsiyoneldir ve `complete` bunu doğrudan `compose`a verir; dondurulmuş D-213, boş listenin “no evidence requirement” olduğunu açıkça seçer. Birleşik denetim, AUD-01…04 olarak başarısız/error/mixed validation kabulünü, dizine yeni dosya eklendiğinde freshness kaybını, fatal knowledge'ın MCP completion tarafından kabulünü ve completion öncesi canonical reconcile yokluğunu doğruladı. Reader ve Breaker RELEASE REJECT, birleşik karar `NOT_VERIFIED`/non-compliant'tır.
**Cross-document state:** MR-016 tasarımı boş required davranışını belgeler; MR-020 kapanışının “bypass denemeleri reddedildi” dili, caller'ın zorunlu profile vermediği bu sınırı anlatmaz.
**Root cause:** Evidence requirement caller input'u olarak bırakılmış, release anlatısı varsayılan/boş çağrı sınırını görünür kılmamıştır.
**Impact:** Gelecekte erişilebilir bir MCP launcher eklendiğinde istemci `required`ı atlayıp completion'ı yeterli kanıt varmış gibi yorumlayabilir; reconcile atlanan source edit task keşfinin dışında kalabilir; ayrıca başarısız/mixed kanıt, dizin genişlemesi veya fatal knowledge durumu güvenli completion anlamına gelmeyebilir.
**Authority decision:** Çalışma davranışı ve typed contract; release kapatma iddiası bu spesifik çağrı şekli için yeterli otorite değildir.
**Required fix:** Completion'da project policy ile required profile zorunluluğunu, unregistered diff için canonical reconcile'i, başarısız/mixed evidence reddini, yeniden-enumerated directory freshness'i ve fatal knowledge fail-close davranışını tasarla/test et. MCP launcher bunu yerine geçmez. O zamana kadar usage belgesi completion'ı bypass-proof diye sunmamalıdır.

### DOC-007

**Documents:** Kullanıcı odaklı CI validation/evidence sınırı yok.
**Code references:** `internal/verify/verify.go:45-57`; `docs/engineering/mr-018-requirements.md:97-104`; `docs/audits/2026-09-26/reader.md:205-214`
**Current documentation:** Release kaydı `verify --ci`yi hook bypass savunması olarak anlatır; README ve önceki kullanım belgeleri config'teki validation profile'larının CI gate tarafından koşturulmadığını söylemez.
**Actual implementation:** Staged/CI verify yolu required profile setini boş bırakır; D-226 bunu 0.1 sınırı olarak açıkça kaydeder. Reader denetimi F-05'i `CONFIRMED` olarak sınıflandırır.
**Cross-document state:** MR-018, profile→scope mapping olmadığı için bu adımın 0.1'de olmadığını kabul eder; bu nedenle implementation/spec çelişkisi değil, release kullanım beklentisi drift'idir.
**Root cause:** Hook/CI gate anlatısı, programatik guard'ı test/evidence orchestration ile ayrıştırmamıştır.
**Impact:** Operatör `verify --ci`nin config profile testlerini yürüttüğünü sanıp test/lint/build adımını CI'dan çıkarabilir.
**Authority decision:** CLI verify implementation, D-226 ve confirmed Reader denetimi.
**Required fix:** Usage ve CI belgeleri ayrı test adımını zorunlu gösterir; profil-policy/coverage otomasyonu gelene kadar verify başarı sonucunu test başarısı olarak yorumlamaz.

## 6. Canonical Documentation Map

| Subject | Canonical Document | Duplicate Locations | Recommended Action |
| --- | --- | --- | --- |
| Mevcut CLI davranışı | `docs/usage-tr.md` | README build bölümü | README'yi usage rehberine bağla. |
| CLI söz dizimi | compiled `mindrail --help` | kaynak yorumları | Help çıktısını çalıştırılabilir otorite kabul et. |
| MCP wire şeması | `internal/mcp/*.go` typed input/output | MR-014/015/016 | Kullanım rehberi sadece erişilemez sınırı açıklar. |
| 0.1 hedef mimarisi | `mindrail-technical-specification-1.0.md` | tech-stack | Hedef ile shipped yüzeyi ayrı etiketle. |
| Release karar kanıtı | task list + MR-020 findings | README status | Kapanış kaydı tarihsel kanıt olarak tutulur. |
| Build/release komutları | `Makefile` | README | Makefile hedeflerini referans al. |

## 7. Fix Roadmap

- **Batch 1 — release/MCP doğruluğu:** `README.md`, `mindrail-tech-stack.md`; mevcut–hedef ayrımını ve `mcp_compatibility:none` sınırını düzelt. Doğrulama: built binary `--help`, `version --json`, `mcp --json` ret testi.
- **Batch 2 — kullanıcı operasyonu:** `docs/usage-tr.md`; kurulum, config, task/lease/checkpoint/verify ekle. Doğrulama: boş geçici Git reposunda komutları koş.
- **Batch 2a — completion güvenlik sınırı:** AUD-01…04 için RED→GREEN düzeltme sürücüleri; required policy, canonical reconcile, failed/mixed evidence, directory freshness ve fatal knowledge fail-close. Doğrulama: mevcut dört reprodüksiyon DENY olur, temiz/kapsam içi senaryolar aşırı reddedilmez.
- **Batch 2b — CI evidence sınırı:** CI pipeline dokümanları ve template'leri; test/lint/build'i `verify --ci`dan önce veya yanında explicit çalıştır. Doğrulama: boş profile seti ile verify'nin green olması tek başına profile koşturulmuş anlamına gelmez.
- **Batch 3 — yönetilen protokol:** Future init AGENTS managed bloğunu gerçekten sahiplenirse bu yazıyı güncelle. Mevcut disposable repo probu init sonrası AGENTS dosyası oluşmadığını doğruladı.
- **Batch 4 — tarihsel netlik:** Engineering dosyalarına dokunmadan başlık/indeks seviyesinde “historical evidence” yönlendirmesi ekle. Doğrulama: link kontrolü ve release revizyonu.

## 8. Applied Changes

### Sonraki remediation uzlaştırması

Bu bölüm, tarihsel bulguları silmeden current çalışma ağacının kullanıcı yüzündeki farkını kaydeder. `DOC-002`deki public launcher eksikliği `mindrail mcp` stdio command'ı ve `version --json`daki `mcp_compatibility:"stdio"` ile giderildi. `DOC-006`daki doğrudan `COMPLETED` yazımı artık reddedilir; `task complete` canonical reconcile, güncel fail-closed knowledge, guard baseline ve required evidence değerlendirdikten sonra terminal yazıyı revision korumasıyla yapar. `AUD-01`/`AUD-02`nin validation run/scope kuralları, `AUD-03`ün knowledge fail-close'u ve `AUD-04`ün canonical reconcile'i hedefli remediation kapsamındadır.

Bu, D-213'ün boş `required` policy seçimini veya D-226'nın `verify --ci` validation command çalıştırmama sınırını kaldırmaz. Paylaşılan kirli worktree'de Git task aidiyeti çıkaramaz; ayrı worktree/lease koordinasyonu hâlâ gereklidir. Bu ek, 2814acb'ye verilen tarihsel release kararını yeniden yazmaz.

| File | Section | Previous state | Updated state | Reason |
| --- | --- | --- | --- |
| `docs/audits/2026-09-26/documentation.md` | tamamı | Yok | Bu kanıta dayalı denetim eklendi | Release ve kullanıcı dokümantasyon drift'ini kaydetmek. |
| `docs/usage-tr.md` | tamamı | Yok | Türkçe pratik kılavuz eklendi | Gerçek CLI sınırını ve MCP erişilemezliğini belgelemek. |

## 9. Remaining Manual Review

- 0.1 için dağıtım kanalı (release asset, paket yöneticisi veya imzalama) yok. Yayın sahibinin desteklenen indirme/güncelleme politikasını belirlemesi gerekir.
- `mindrail mcp` launcher'ının 0.1 sonrası mı, yoksa release öncesi mi olması gerektiği ürün kararıdır. Kararı çözmek için D-191'in yerini alan onaylı roadmap gerekir.
- AGENTS managed bloğu current binary tarafından yaratılmaz veya güncellenmez; bunu ancak future init davranışı değişirse tekrar değerlendirin.
- Merkezi release doğrulaması [verification.md](verification.md) içinde 2026-09-26'da `make verify`, `make tidy-check`, `make release` (linux/amd64 build/checksum; dört cross target not-built), `make gate`in dokuz kategorisi ve 1308 testi başarılı raporladı. Bu yeşil kontroller F-05'i veya completion karşı örneklerini çürütmez.
- AUD-01…04 doğrulandı; düzeltmeleri bu denetimin kapsamı dışındadır. D-213/D-226 politika daraltmalarının üst ürün vaadiyle uyumu ayrı ürün kararıdır; doğrulanmış dört davranış hatasını ortadan kaldırmaz.

## 10. Final Integrity Matrix

| Area | Docs Internal Consistency | Code Alignment | Status |
| --- | --- | --- | --- |
| Architecture | Hedef/mevcut sınırı bulanık | Kısmi | MAJOR_DRIFT |
| API | HTTP API yok | Uygulanabilir değil | CONSISTENT |
| Configuration | Pratik referans eksikti | Kullanım rehberiyle kapsandı | MINOR_DRIFT |
| Data Model | Engineering kayıtlarında ayrıntılı | Örneklenmedi | MINOR_DRIFT |
| Business Rules | Specs ve testler baskın | Kapsam dışı örnekleme | MINOR_DRIFT |
| Setup | README dar | Makefile ile uyumlu | MAJOR_DRIFT |
| Deployment | İndirme/paketleme anlatılmamış | Yerel make release var | MAJOR_DRIFT |
| Testing | Verify'nin profile koşmadığı belirtilmemişti | Merkezi kontroller green, F-05 confirmed | MAJOR_DRIFT |
| Integrations | MCP process entegrasyonu iddialı | Launcher yok | MAJOR_DRIFT |

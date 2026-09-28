# Mindrail 0.1 kullanım kılavuzu

Mindrail, AI coding agent'larının yaptığı gerçek Git değişikliğini repository
kararları, invariant'lar ve snapshot'a bağlı doğrulama kanıtıyla birleştiren
yerel bir engineering gate'tir. Normal kullanımda insanın session/task/lease
kimliği taşıması veya uzun bir CLI lifecycle'ı yönetmesi gerekmez.

## İki kullanım yolu: sıfırdan ve mevcut repository

Mindrail'i yalnız bir Git worktree'sinde başlatın. `mcp` sunucusu o process'in
çalışma dizinini repository olarak kabul eder; bu nedenle tek bir global server
tanımıyla farklı repository'ler arasında rastgele dizinden çalışmayın.

### 1. Sıfırdan proje

Önce Git repository'sini ve Mindrail binary'sini hazırlayın. Kaynak tree bu
repository ise desteklenen build yolu şöyledir:

```bash
git init my-project
cd my-project

# Mindrail kaynak tree'sinde:
make install
export PATH="$HOME/.local/bin:$PATH"
mindrail version --json
```

`make install` için Go 1.27.x, Git ve C toolchain gerekir. Binary'yi başka bir
yolla edinmişseniz son iki satırın yerine yalnız binary'nin PATH'te olduğundan
emin olun. Ardından proje kökünde kurulum ve ilk sağlık kontrolünü yapın:

```bash
mindrail init
mindrail status
mindrail doctor
```

`status` kısa hazır olma özetidir; initialize edilmemiş bir repo için
`BLOCKED` raporlayıp yine exit 0 verebilir. `doctor` read-only teşhistir; bir
sorunu onarmak için `init` veya gösterdiği düzeltmeyi tekrar çalıştırın.

İlk değişikliği commit etmeden önce normal yerel gate'i de deneyin:

```bash
git add .
mindrail verify
```

Bu komut yalnız staged index'i denetler. CI'da committed aralık için
`mindrail verify --ci` kullanılır; test, lint ve build'i ayrıca CI'da çalıştırın.

### 2. Var olan projeye güvenli entegrasyon

Önce repository ve hook durumunu görünür kılın. Bunlar yazmaz; ilk iki komut
geri dönüş/inceleme noktasıdır:

```bash
git status --short
git config --get core.hooksPath || true
test -f AGENTS.md && sed -n '1,220p' AGENTS.md
hook_path=$(git rev-parse --git-path hooks/pre-commit)
test -f "$hook_path" && sed -n '1,220p' "$hook_path"
```

Çalışan değişiklikler varsa önce commit, stash veya ayrı worktree ile onları
ayırın. `AGENTS.md` ve pre-commit hook'u kurumunuz için kritikse, içeriğini
kurulum öncesi VCS dışı güvenilir bir yere de yedekleyin. Sonra proje kökünde:

```bash
mindrail init
mindrail status
mindrail doctor
git diff -- AGENTS.md .mindrail
```

`init`, `.mindrail/config.toml` ve knowledge dizinlerini oluşturur, runtime
SQLite verisini Git common directory altında hazırlar ve worktree'yi kaydeder.
`AGENTS.md` içinde yalnız şu marker'lar arasındaki bölümü ekler/günceller;
dosyanın kalanına dokunmaz:

```text
<!-- BEGIN MINDRAIL MANAGED SECTION -->
...
<!-- END MINDRAIL MANAGED SECTION -->
```

Mevcut pre-commit hook varsa Mindrail onu `pre-commit.mindrail-original`
olarak saklar ve yeni wrapper önce bu foreign hook'u, sonra
`mindrail verify --staged` kapısını çalıştırır. Böylece formatter veya staged
dosya üreten eski hook'un çıktısı Mindrail tarafından görülür. Tekrar
`mindrail init` çalıştırmak idempotenttir.

Binary'yi yeni bir kaynak sürümünden değiştirdikten sonra daha önce initialize
edilmiş worktree'de `mindrail update` çalıştırın. Bu komut aynı güvenli managed
setup/migration hattını kullanır; user-owned AGENTS metnini ve foreign hook'u
korur. Initialize edilmemiş repository için önce `mindrail init` gerekir.

Güvenli biçimde birleştirilemeyen durumda kurulum reddedilir: symlink,
normal-dosya olmayan managed hedef, bozuk/çift AGENTS marker'ı, değiştirilmiş
Mindrail wrapper'ı ya da repository dışına yönelen `core.hooksPath` örnektir.
Bu durumda dosyaları elle overwrite etmeyin; `mindrail doctor` ve hata
mesajındaki `next_action`ı inceleyin. Geri alma gerekiyorsa, önce `init`in
oluşturduğu wrapper ve `.mindrail-original` yedeğini karşılaştırın; kendi
orijinal hook'unuzu geri koyun, Mindrail marker bölümünü yalnız onaylı bir
backup'a göre kaldırın ve ardından `mindrail status`/`doctor` ile sonucu
doğrulayın. Runtime verisi Git common directory'dedir; yalnız dosya geri almak
runtime kayıtlarını silmez.

## 60 saniyede günlük kullanım

`mindrail` PATH üzerindeyken hedef Git repository'sinde bir kez çalıştırın:

```bash
mindrail init
```

Sonrasında insan yüzeyi şudur:

```bash
mindrail status     # kısa hazırlık özeti
mindrail doctor     # ayrıntılı ve read-only teşhis
mindrail verify     # local: staged değişiklikleri doğrular
```

Canlı yerel operasyon görünümü için:

```bash
mindrail dashboard
```

Komut yalnız `127.0.0.1` üzerinde rastgele bir porta bağlanır ve terminale
token içeren yerel URL'yi yazar. Factory görünümü yedi gerçek task state'ini,
revision ve `claimed_by` sahipliğini; Agents görünümü MCP connection presence ile
aktif, expired ve released lease geçmişini; Events görünümü checkpoint/handoff,
JEV route-event ve validation evidence metadata'sını canlı SSE akışında gösterir.
Frontend dosyaları binary içine gömülüdür; ayrı Node kurulumu gerekmez.

Dashboard read-only'dir. Evidence output/argv/provenance alanlarını, API key veya
başka credential değerlerini ve sınırsız checkpoint metnini yayınlamaz. Mevcut
şema completion/testguard bulgularını kalıcı tutmadığı için bunlar `on_demand`,
GitHub/merge provider'ı yoksa ilgili bölümler `unavailable` görünür; sahte CI veya
merge sonucu üretilmez. Sabit port gerekirse `mindrail dashboard --port 43187`
kullanılabilir. `Ctrl-C` sunucuyu güvenli biçimde kapatır.

Üst bölümdeki SSE durumu, son snapshot yaşı/sequence değeri ile dashboard başlama
zamanı ve uptime yalnız dashboard sunucusunun ve tarayıcı akışının durumunu
gösterir; bir agent prosesinin çalıştığını kanıtlamaz. Agents kartları, MCP
istemcisinin kendi bildirdiği `ClientInfo.name` değerinden server'ın ürettiği
canonical client family bilgisini, bağlantı süresini, son presence
heartbeat'ini, son Mindrail MCP tool etkinliğini ve `CONNECTED`, `IDLE`, `STALE`
veya `ENDED` durumunu gösterir. Heartbeat yalnız MCP bağlantısının presence
kaydını yenilediğini; activity yalnız bir Mindrail tool'unun çağrıldığını kanıtlar.
Bunların hiçbiri model/proses/token çalışmasını veya private reasoning etkinliğini
kanıtlamaz. Model ve reasoning-effort kimliği istemci açıkça sunmadıkça bilinmez.
`CONNECTED`, heartbeat yaşı en fazla 15 saniye ve MCP tool activity yaşı en fazla
30 saniyeyken; `IDLE`, heartbeat güncel ama activity penceresi aşılmışken;
`STALE`, heartbeat 15 saniyeyi aşmışken gösterilir. `ENDED` bağlantı süresini
kayıtlı bitiş anında dondurur.
Bilinen alias'lar sabit bir canonical client family değerine dönüşür; arbitrary
veya custom adlar `unknown-client` olur, runtime ID ise bağlantıları ayırt etmeye
devam eder. Raw title/version hiçbir zaman kalıcı yazılmaz veya gösterilmez.
Bu tercih, credential biçimli caller input'un durable telemetry'ye girmesini
engellemek için arbitrary client adlarını bilinçli olarak korumaz.

Repository readiness ve JEV credential yapılandırması dashboard başlatılırken
alınan snapshot'tır; gerçek route denemeleri ve kabul edilmiş JEV tavsiyesi ise
ayrı, canlı metadata olarak gösterilir. Yapılandırılmış olmak kullanılmış olmak
değildir. Bu sinyallerde prompt, candidate açıklaması/kimliği veya credential yoktur.
`mindrail jev connect` veya
`mindrail jev disconnect` sonrasında ya da readiness'i yeniden hesaplatmak için
dashboard'u durdurup yeniden başlatın. JEV keyring okunamıyorsa `unavailable`, hiç
credential yoksa `not configured` görünür; key değeri dashboard'a aktarılmaz.

DevTools'ta kaynağı `chrome-extension://.../contentscript.js` olan
`MaxListenersExceededWarning` veya `ObjectMultiplex` mesajları Mindrail asset'i
değil, tarayıcı eklentisinin enjekte ettiği content script'tir. Kaynağa tıklayıp
extension ID'sini `chrome://extensions` ile eşleştirebilir veya dashboard'u Guest
profilde/eklentiler kapalıyken açarak doğrulayabilirsiniz. Mindrail'in kendi istemci
hataları token'lı yerel URL altındaki `assets/app.js` kaynağını gösterir.

CI'da committed merge-base aralığını doğrulamak için:

```bash
mindrail verify --ci
```

`mindrail init` tek seferde:

- eksik `.mindrail/config.toml` ile knowledge dizinlerini oluşturur;
- runtime SQLite database'ini Git common directory altında oluşturur/günceller
  ve worktree'yi kaydeder;
- `AGENTS.md` içinde yalnız Mindrail'in marker'larla sınırlı yönetilen bölümünü
  oluşturur veya günceller;
- repository-local pre-commit hook'una `mindrail verify --staged` kapısını
  kurar ve var olan foreign hook'u önce çalışacak biçimde zincirler.

Var olan kullanıcı metni, config ve hook içeriği korunur; aynı `init` çağrısını
tekrarlamak güvenlidir. Bozuk marker, symlink veya repository dışına çıkan
`core.hooksPath` gibi güvenli biçimde birleştirilemeyen hedefler sessizce
overwrite edilmez, görünür bir hata ile reddedilir.

Normal akışta insanın yapmadığı işler de önemlidir: `session open`, `task open`,
revision kopyalama, lease acquire/renew/release, operation ID üretme ve
`task complete` çağırma agent'ın MCP lifecycle'ının iç işidir.

## MCP'yi bir araca bağlama

Mindrail yalnız **stdio** MCP server'ıdır: HTTP/TCP endpoint'i yoktur. Her MCP
uyumlu istemcide gereken gerçek sözleşme aynıdır: binary'yi hedef repository
çalışma dizininde başlatın; komut `mindrail mcp`, argümanlar `['mcp']` ve stdout
yalnız protocol frame'leri içindir. `--json` eklemeyin.

İstemcinizin stdio tanımına karşılık gelen genel şablon:

```json
{
  "type": "stdio",
  "command": "/absolute/path/to/mindrail",
  "args": ["mcp"]
}
```

`command` için PATH'e güvenmek yerine mutlak binary yolu kullanmak, GUI veya
CI istemcilerindeki farklı PATH'lerden doğan hataları önler. İstemci `cwd`
alanını destekliyorsa hedef repository kökünü verin; desteklemiyorsa istemciyi
o kökten açın/çalıştırın. Mindrail'e repository yolunu argüman olarak vermeyin.

### Codex

Yerel Codex yapılandırması TOML'da `mcp_servers` tablosunu kullanır. Örneğin
`~/.codex/config.toml` içine aşağıdaki server kaydı eklenebilir:

```toml
[mcp_servers.mindrail]
command = "/absolute/path/to/mindrail"
args = ["mcp"]
```

Bu kayıt stdio komutunu tanımlar; Mindrail'in doğru repository'yi görmesi için
Codex görevini hedef repository/worktree bağlamında başlatın. Codex sürümünüz
proje-özel MCP çalışma dizini için ayrı bir alan sunmuyorsa, bu alanı tahmin
ederek eklemeyin; istemciyi proje kökünden başlatmak güvenli sözleşmedir.

### Claude Code

Claude Code'un güncel CLI'sinde proje kapsamına stdio tanımı eklemek için hedef
repository kökünde şunu çalıştırın:

```bash
claude mcp add-json mindrail \
  '{"type":"stdio","command":"/absolute/path/to/mindrail","args":["mcp"]}' \
  --scope project
claude mcp get mindrail
```

Eşdeğer, ekiple paylaşılabilir `.mcp.json` kaydı şudur:

```json
{
  "mcpServers": {
    "mindrail": {
      "type": "stdio",
      "command": "/absolute/path/to/mindrail",
      "args": ["mcp"]
    }
  }
}
```

Project-scope server'lar ilk interaktif Claude Code oturumunda onay isteyebilir.
`claude mcp get mindrail`, yapılandırmanın yazıldığını ve bağlantı durumunu
gösterir. Binary yolu makineye özelse, paylaşılmış `.mcp.json` yerine team'in
kabul ettiği environment-variable mekanizmasını veya kurulum standardını
kullanın.

### VS Code

Hedef workspace'te `.vscode/mcp.json` oluşturun ya da Command Palette'ten
**MCP: Open Workspace Folder MCP Configuration** komutunu açın ve şu kaydı
ekleyin:

```json
{
  "servers": {
    "mindrail": {
      "type": "stdio",
      "command": "/absolute/path/to/mindrail",
      "args": ["mcp"],
      "cwd": "${workspaceFolder}"
    }
  }
}
```

Sonra **MCP: List Servers** ile server'ı başlatın/çıktısını görüntüleyin.
VS Code workspace config'ini güven ilişkisi içinde çalıştırır; repository'nin
MCP tanımını incelemeden workspace'i trusted yapmayın. VS Code Agent Host ile
taşınabilir biçim gerekirse workspace kökündeki `.mcp.json` içinde bir önceki
Claude örneğindeki `mcpServers` şablonunu kullanın.

Bu üç örnek istemci yapılandırmasıdır; Mindrail config'i değildir. Herhangi
başka bir MCP istemcisi için yalnız stdio `command`, `args` ve (varsa) hedef
repository `cwd` eşlemesi yapılır. İstemcinin kendi config dosyası, alan adı
veya onay UI'ı farklıysa, o istemcinin MCP dokümantasyonundaki stdio şemasını
kullanın; Mindrail'e özel ek alan yoktur.

## Agent'ın otomatik MCP akışı

MCP istemcisini hedef repository çalışma dizininde şu komutla başlatın:

```json
{"command":"/absolute/path/to/mindrail","args":["mcp"]}
```

`mindrail mcp` stdio transport kullanır. `--json` eklemeyin: stdout yalnız MCP
protocol frame'lerine ayrılmıştır; diagnostics stderr'e gider. TCP/HTTP `serve`
endpoint'i yoktur.

### Minimum lifecycle

Agent için mutlu yol üç küçük adımdır:

| Aşama | Tool | Input |
| --- | --- | --- |
| Başlat/geri al | `mindrail_bootstrap` | `{"goal":"Parser hatasını düzelt","run_key":"<agent-opaque-stable-key>","paths":["internal/parser.go"]}` |
| Kapsam belirtilmediyse veya büyüdüyse | `mindrail_before_change` | `{"paths":["internal/parser_test.go"]}` |
| Bitir | `mindrail_complete` | `{"finalize":true}` |

`paths` bootstrap'ta isteğe bağlıdır; verilmediyse **ilk değişiklikten önce**
`mindrail_before_change` çağrısı zorunludur. Bildirilen kapsam dışındaki bir dosya
değiştirilmeden önce de bu çağrı yapılmalıdır. Çağrı yalnız bootstrap yolları
değiştirilecek bütün dosyaları zaten kapsıyorsa atlanabilir. Otomatik modda
relative repository yolları kabul edilir. İnsan session ID, task ID, revision,
lease veya operation ID vermez.

`run_key`, agent'ın oluşturduğu en fazla 1024 byte'lık opaque ve kararlı retry
anahtarıdır. Aynı mantıksal çalışma yeniden teslim edildiğinde veya MCP process'i
yeniden başladığında aynı anahtar kullanılmalıdır; kullanıcıya kopyalatılmaz.
Başka bir agent'ın açık task'ı otomatik seçilmez. Bilinçli handoff devamı için
yeni run key ile önceki task açıkça `resume_task_id` olarak verilir:

```json
{"goal":"Handoff'tan devam et","run_key":"<new-agent-key>","resume_task_id":"TSK-…"}
```

Aynı MCP bağlantısında `mindrail_complete` için `run_key` tekrar verilmez.
Process restart sonrası terminal sonucu veya yarım kalmış çalışmayı çözmek için
aynı key açıkça verilebilir:

```json
{"finalize":true,"run_key":"<same-agent-key>"}
```

Duplicate bootstrap/finalize teslimi aynı mantıksal session/task/sonucu replay
eder; yeni kayıt veya ikinci terminal geçiş üretmez. Otomatik çalışma boyunca
lease heartbeat'i yenilenir; bağlantı kapanınca veya server durunca heartbeat
temizlenir. Heartbeat hatası finalization'ı bloke eder.

### Finalization ne yapar?

`mindrail_complete {"finalize":true}` sırasıyla gerçek scoped Git diff'ini
yeniden keşfeder, `.mindrail/config.toml` içindeki bütün validation
profile'larını ada göre deterministik sırada çalıştırır, shared completion gate'i
değerlendirir ve yalnız ALLOW halinde task'ı atomik olarak tamamlayıp lease'leri
serbest bırakır. Denial halinde `completed:false` ve düzeltilebilir nedenler
döner; açık çalışma korunur.

Hiç validation profile yoksa cevapta
`"no_profiles_configured":true` bulunur. Bu, **testlerin geçtiği anlamına
gelmez**; hiçbir profile çalışmamıştır. Test/lint/build profile'larını config'e
ekleyin ve CI işlerini ayrıca çalıştırın. Agent, otomatik complete isteğine
`required`, `task_id`, revision veya operation ID ekleyerek kanıt politikasını
zayıflatamaz.

### Aynı yaşam döngüsü, 14 tool

Server yaşam döngüsünü korur ve görünür JEV yönlendirmesiyle şu 14 tool'u yayımlar:

1. `mindrail_bootstrap`
2. `mindrail_status`
3. `mindrail_search`
4. `mindrail_context`
5. `mindrail_decide`
6. `mindrail_invariant`
7. `mindrail_claim`
8. `mindrail_before_change`
9. `mindrail_after_change`
10. `mindrail_reconcile`
11. `mindrail_checkpoint`
12. `mindrail_validate`
13. `mindrail_complete`
14. `mindrail_route`

Agent gerektiğinde `mindrail_status`, `mindrail_search` ve `mindrail_context`
ile okuma yapabilir; durable seçimleri `mindrail_decide` ve
`mindrail_invariant` ile kaydedebilir. Otomatik bootstrap'tan sonra
`mindrail_after_change {}` veya `mindrail_reconcile {}` gerçek değişimi erkenden
incelemek için kullanılabilir, fakat mutlu yolda zorunlu değildir: finalize
zaten reconcile eder.

Checkpoint yazmak için ID gerekmez:

```json
{"note":"Parser testi eklendi; dış servis doğrulaması bekliyor."}
```

Gerçek handoff için `{"note":"…","handoff":true}` kullanılır. Bu çağrı
otomatik run'ı bırakır; devam edecek agent fresh `run_key` ve açık
`resume_task_id` kullanır.

## Yerel verify ve CI

`mindrail verify`, flagsizken `mindrail verify --staged` ile aynı sonucu üretir.
Yani working tree'nin tamamını değil Git index'e alınmış değişiklikleri shared
gate üzerinden değerlendirir. `init` tarafından kurulan pre-commit hook da
explicit `mindrail verify --staged` çağırır.

```bash
git add path/to/changed-file
mindrail verify
```

CI modu committed merge-base range'ini değerlendirir:

```bash
mindrail verify --ci
```

`--base` verilmezse komut sırasıyla `origin/main`, `main`, `master` adaylarını;
`--head` verilmezse `HEAD` kullanır. CI ortamında ref'leri belirsizliğe
bırakmamak için full history checkout yapıp doğrulanmış commit'leri açıkça
geçebilirsiniz:

```bash
BASE_REF="${BASE_REF:?CI base branch/ref gerekli; örn. main}"
HEAD_REF="${HEAD_REF:-HEAD}"
git fetch --no-tags origin "$BASE_REF"
base_commit=$(git rev-parse --verify FETCH_HEAD)
head_commit=$(git rev-parse --verify "$HEAD_REF^{commit}")
git merge-base "$base_commit" "$head_commit" >/dev/null
mindrail verify --ci --base "$base_commit" --head "$head_commit" --json
```

`mindrail verify --ci` validation profile command'larını çalıştırmaz ve agent
evidence coverage'i üretmez; committed range üzerinde aynı repository gate'ini
uygular. Test, lint ve build job'larını CI'da ayrıca çalıştırın. Local hook
`--no-verify` ile aşılabildiği için CI gate'i bağımsız tutulmalıdır.

## Kurulum ve build

Önkoşullar:

- Git repository (bare repository değil)
- Kaynaktan build için Go `1.27.x`, Git ve C toolchain
- Doğrulanmış native hedef: `linux/amd64`

Çalışma zamanı cloud, hesap veya ağ bağlantısı gerektirmez. Go module cache boşsa
ilk kaynak build'i bağımlılık indirmek için ağ isteyebilir. Bu repoda release
asset, paket yöneticisi, imza veya otomatik güncelleme endpoint'i tanımlı
olmadığından güvenilir kurulum yolu bugün kaynaktan build etmektir:

```bash
make check
make install
mindrail version --json
```

`make build` binary'yi `bin/mindrail` içine yazar. `make check` format denetimi,
`go vet` ve testleri; `make verify` bunlara race detector ve clean-binary smoke
testlerini ekler. `make bench` benchmark, `make release` platform
matrix/stamp/checksum üretir. `make install` binary'yi hedef dizinde geçici bir
dosyaya build edip aynı dizinde rename ederek atomik biçimde
`$HOME/.local/bin/mindrail` üzerine kurar.

Kurulum kökü ve paket staging alanı değiştirilebilir:

```bash
make install PREFIX=/opt/mindrail
make install BINDIR=/custom/bin
make install DESTDIR=/tmp/package-root BINDIR=/usr/bin
export PATH="$HOME/.local/bin:$PATH"
```

Doğrulanmış Linux hedefinde install, final hedef bir symlink/dizin ise veya
çözümlenmiş yol `DESTDIR` dışına taşıyorsa yazmadan reddeder. Var olmayan hedef
ve existing regular binary ise aynı hedef dizindeki geçici dosyadan atomik rename
ile kurulur.

Son `export` yalnız mevcut shell içindir. Mindrail'in network self-update kanalı
yoktur. Kaynaktan güncelleme iki ayrı adımdır: önce güncel source checkout'ta
`make check && make install` ile binary'yi değiştirin; sonra her initialize
edilmiş worktree'de `mindrail update` çalıştırın. Binary kurulumu programı,
`mindrail update` ise o repository'nin managed talimatlarını ve migration'larını
günceller.

### Opsiyonel JEV yönlendirmesi ve anahtar yönetimi

JEV tamamen opsiyonel ve coding-agent bağımsızdır. Herhangi bir yerel agent'tan
`mindrail jev connect` çalıştırmasını isteyin veya komutu doğrudan başlatın. Mindrail
yalnız loopback üzerinde geçici bir tarayıcı formu açar ve girilen anahtarı işletim
sisteminin credential store/keyring alanına kaydeder. Anahtarı agent sohbetine,
Codex/Claude/Cursor ayarına, stdin'e, repository config'e, `.env` dosyasına, argv'ye,
log'a veya commit'e yazmayın. `TYPESAFE_API_KEY` yalnız CI/container otomasyonu için
daha yüksek öncelikli opsiyonel override olarak kalır. İki kaynak da yoksa normal
yönlendirme akışı değişmez.

Güvenli kalıcı saklama şu anda Linux'ta Secret Service, Windows'ta Credential
Manager üzerinden desteklenir. Mindrail, uygulamaya bağlı native Keychain backend'i
hazır olana kadar macOS'ta kalıcı saklamayı bilinçli olarak reddeder; macOS'ta
opsiyonel environment override kullanılabilir.

Initialize/update ile kurulan managed talimatlar, belirsiz ve kapalı bir tool,
agent, model, reasoning-effort veya tool şemasının desteklediği başka bir kapalı
seçim öncesinde agent'ın görünür
`mindrail_route` MCP tool'unu bir kez çağırmasını ister. Kullanıcının görev
prompt'unda Mindrail veya JEV demesi gerekmez. Güvenli bounded istek örneği
şöyledir; anahtar payload'a eklenmez:

```text
mindrail_route {"goal":"arama aracı seç","tools":[{"id":"rg","description":"repository metninde ara"}]}
```

Bounded payload zorunlu string `goal`, opsiyonel JSON `context` ve en az bir
`tools`, `agents`, `models` veya `efforts` array'i içerir. Her candidate tam olarak
`id` ve `description` alanlarını taşır. Kurulu binary canonical adapter'ı
`mindrail_route` üzerinden sunar; Python 3 yalnız bu opsiyon etkinse gerekir. Agent yalnız
top-level durum `ok` ve bütün seçimler accepted olduğunda tavsiyeyi kullanır;
disabled, fallback, error veya reddedilmiş sonuçta normal akışa devam eder.
TypeSafe'e yalnız gerekli en küçük hassas-olmayan goal/context özetini ve kapalı
candidate açıklamalarını gönderin; ham prompt, secret, log, kaynak kod, diff,
path veya environment değeri göndermeyin. JEV hiçbir zaman işi bloke etmez,
izin vermez veya Mindrail gate'ini zayıflatmaz. Gizli `mindrail agent route` stdin
köprüsü yalnız `mindrail_route` tool'unu keşfedemeyen eski istemciler için uyumluluk
fallback'idir.

Kalıcı route metadata'sı bilinçli olarak bounded ve ordinal'dır: attribution,
status/reason, candidate sayıları, seçilen ordinal, quantized confidence,
credential source ve süre. Goal/context, candidate ID/açıklamaları, provider
body'si, kaynak kod, diff, path, log, environment değeri ve credential kalıcı
yazılmaz ve dashboard'a gönderilmez.

MCP agent'ın private chain-of-thought akışını göremez ve bir seçimin gerçekten
belirsiz olduğunu kendisi algılayamaz; “otomatik” kullanım managed talimatı
uygulayan istemciye bağlıdır. Mindrail, istemci açıkça sunup uygulamadıkça zaten
çalışan host modelini veya reasoning effort'u bilemez/değiştiremez. Binary veya
managed yapılandırma güncellemesinden sonra yeni tool/talimatların keşfi için MCP
istemcisini yeniden başlatın ya da reconnect edin.
`mindrail jev status` yalnız bağlantı durumunu ve aktif kaynağın adını gösterir;
anahtarı göstermez. `mindrail jev disconnect` keyring'deki credential'ı kaldırır.

## Konfigürasyon ve validation profile'ları

Config önceliği düşükten yükseğe şöyledir: built-in defaults → kullanıcı
config'i → repository `.mindrail/config.toml` → `MINDRAIL_*` environment → CLI
flag. Repository config'i strict TOML'dur; bilinmeyen anahtar hata verir.
Bilinmeyen `MINDRAIL_*` environment anahtarı değerini yazmadan warning üretir.

Geçerli örnek:

```toml
[project]
name = "ornek"

[output]
color = "never" # auto | always | never

[validation.test]
type = "AUTOMATED_TEST"
paths = ["."]
commands = [["go", "test", "./..."]]

[secrets]
env = ["EXAMPLE_TOKEN"]
```

Profile türleri `AUTOMATED_TEST`, `TYPECHECK`, `BUILD`, `LINT`,
`INTEGRATION_TEST`, `RUNTIME_PROBE`, `MANUAL_VERIFICATION`, `HUMAN_APPROVAL`,
`CI_VERIFICATION` ve `EXTERNAL_SYSTEM` olabilir. `paths` boş olamaz;
`commands` shell string'i değil argv array'leri listesidir.

Tanımlı environment anahtarları `MINDRAIL_PROJECT_NAME`,
`MINDRAIL_OUTPUT_COLOR`, `MINDRAIL_RUNTIME_DIR` ve `MINDRAIL_CACHE_DIR`dır.
Runtime/cache override'ları test ve izolasyon içindir; TOML'dan ayarlanamaz.

## Repository-owned knowledge

Decision ve invariant kayıtları `.mindrail/knowledge` altında version-controlled
JSON dosyalarıdır. Agent normalde `mindrail_decide` ve `mindrail_invariant`
tool'larını kullanır. İnsan Git diff ile inceleyebilir ve aşağıdaki komutla
fail-closed schema kontrolü yapabilir:

```bash
mindrail knowledge validate --json
```

Bu komut normal root yardımında görünmeyen ileri seviye yüzeydedir. Örnek yollar:

```text
.mindrail/knowledge/decisions/DEC-0001.json
.mindrail/knowledge/invariants/INV-0001.json
```

Ortak zorunlu alanlar `schema_version`, `kind`, `id`, `status`, `created_at`tır.
Decision ayrıca `title` ve `decision`; invariant `statement`, `severity` ve
`scope` ister. ID ile dosya adı eşleşmeli; zaman UTC RFC 3339 olmalıdır. Eski
kaydı rewrite etmek yerine yeni kayıtla `supersedes` kullanın.

## Status, doctor ve çoklu worktree

`status` kısa readiness özeti verir. Initialize edilmemiş repository için
`BLOCKED` ve `mindrail init` next action'ı raporlayabilir; durumu başarılı
biçimde raporladığı için process exit'i yine 0 olabilir. `doctor` config,
runtime DB, knowledge, Git/worktree ve optional capability bulgularını ayrıntılı
raporlar fakat hiçbir şeyi düzeltmez.

Runtime database Git common directory altında olduğu için linked worktree'ler
project task/lease verisini paylaşır; `.mindrail` config ve knowledge ise
repository içeriğidir. Her yeni worktree'de `mindrail init` çalıştırarak o
worktree'yi kaydedin ve yönetilen kurulumu doğrulayın. Farklı agent'lar için ayrı
worktree kullanmak, aynı fiziksel dirty tree'deki değişiklik aidiyeti
belirsizliğini önler.

## İleri seviye ve uyumluluk referansı

Bu bölüm normal kullanım için gerekli değildir. Eski script'ler, manuel teşhis
ve protocol uyumluluğu için düşük seviyeli yüzeyi belgeler. Root yardımında
yalnız `init`, `status`, `doctor`, `verify` ve `version` listelenir; aşağıdaki
komutlar silinmemiştir ve `mindrail help <command>` ile belgelenir.

### Global seçenekler ve exit code'lar

| Seçenek | Anlamı |
| --- | --- |
| `-C DIR`, `--chdir DIR` | Komutu hedef repository'de çalıştırır. |
| `--json` | Stdout'a tek JSON envelope yazar; `mcp` ile kullanılamaz. |
| `--no-color` | İnsan okunur çıktıda rengi kapatır. |
| `--verbose` | Startup ayrıntılarını stderr'e yazar. |

Başarılı envelope `{"command":"…","ok":true,"data":{…}}`; hata envelope'u
`{"command":"…","ok":false,"error":{"code":"…","why":"…","next_action":[…]}}`
biçimindedir.

| Exit | Anlamı |
| --- | --- |
| `0` | Başarı veya durumu başarıyla raporlama |
| `1` | Operasyon başarısız |
| `2` | Kullanım/configuration hatası |
| `3` | Gate/verification ret kararı |
| `4` | Runtime geçici olarak kullanılamıyor |

### Tam CLI yüzeyi

| Komut | Kullanım |
| --- | --- |
| `init` | Repository setup, managed AGENTS bölümü ve hook kurulumu |
| `status`, `doctor` | Readiness ve read-only sağlık raporu |
| `verify` | Flagsiz/`--staged` local index; `--ci` committed range |
| `session open` | Yeni low-level session; isteğe bağlı `--label`, `--operation-id` |
| `task open` | `--title` zorunlu; `--session`, `--operation-id` isteğe bağlı |
| `task list` / `show` | Task listeleme ve checkpoint dahil ayrıntı |
| `task state` | State transition; `--to`, isteğe bağlı revision/session/operation ID |
| `task complete` | Guarded terminal transition; session ve revision zorunlu |
| `lease acquire/list/renew/release` | File/task lease'lerini elle yönetme |
| `checkpoint write` | Task note/handoff yazma |
| `hook install` | `init`in otomatik yaptığı managed hook kurulumunu ayrıca çalıştırma |
| `knowledge validate` | Repository knowledge'ını fail-closed doğrulama |
| `mcp` | Stdio MCP server; stdout protocol-only |
| `version` | Binary/compatibility bilgisi; repository gerektirmez |
| `help [command]` / `completion <shell>` | Ayrıntılı yardım ve shell completion |

Kesin flag sözleşmesi için çalışan binary otoritedir:
`mindrail help <command>`.

### Manuel session/task/lease akışı

Aşağıdaki eski akış yalnız low-level teşhis ve uyumlu script'ler içindir. `jq`
Mindrail önkoşulu değildir; örnekte JSON'dan server-generated ID çıkarmak için
kullanılır:

```bash
session=$(mindrail session open --label manual --operation-id session-1 --json | jq -r '.data.session.session_id')
opened=$(mindrail task open --title 'Parser düzeltmesi' --session "$session" --operation-id task-open-1 --json)
task=$(jq -r '.data.task.task_id' <<<"$opened")

claimed=$(mindrail task state "$task" --to CLAIMED --session "$session" \
  --expect-revision 1 --operation-id task-claim-1 --json)
revision=$(jq -r '.data.task.revision' <<<"$claimed")
mindrail task state "$task" --to IN_PROGRESS --session "$session" \
  --expect-revision "$revision" --operation-id task-progress-1 --json
mindrail checkpoint write "$task" --session "$session" \
  --note 'Parser testi eklendi.' --handoff --operation-id checkpoint-1 --json
```

Server-generated ID önekleri session `SES-`, task `TSK-`, lease `LSE-`,
checkpoint `CKP-`, project/workspace `PRJ-`/`WS-` şeklindedir. Tahmin etmeyin;
cevaptan alın. Kayıp write yanıtını aynı argümanlar ve aynı operation ID ile
tekrar etmek replay üretir; farklı argümanlar için aynı ID conflict'tir.

State geçişleri `OPEN → CLAIMED|ABANDONED`, `CLAIMED →
IN_PROGRESS|OPEN|ABANDONED`, `IN_PROGRESS →
BLOCKED|READY_TO_COMPLETE|ABANDONED`, `BLOCKED →
IN_PROGRESS|ABANDONED`, `READY_TO_COMPLETE → IN_PROGRESS|ABANDONED`
şeklindedir. `COMPLETED` ve `ABANDONED` terminaldir. `task state --to
COMPLETED` reddedilir; low-level terminal geçiş `task complete` ile yapılır.

`OPEN → CLAIMED` task lease'i alır. Working state'te `lease acquire --task`;
file ownership için `lease acquire --file` kullanılabilir. Lease yaklaşık 20
dakika geçerlidir; holder session yeniler/serbest bırakır. `checkpoint write
--handoff` task lease'ini bırakır.

Low-level completion örneği, task `READY_TO_COMPLETE` durumundayken ve `test`
profile'ı için güncel evidence varken şöyledir:

```bash
ready=$(mindrail task show "$task" --json)
revision=$(jq -r '.data.task.revision' <<<"$ready")
mindrail task complete "$task" --session "$session" \
  --expect-revision "$revision" --required test \
  --operation-id complete-1 --json
```

ALLOW yoksa task/lease değişmez. ALLOW halinde terminal state ve lease release
tek revision-guarded yazıdır. Açıkça boş/whitespace `--required` kullanım
hatasıdır. `BLOCKED` geçişi `--reason` ister.

### Legacy MCP semantiği

Otomatik alanlar additive'dir; yeni route tool'unu kullanmayan eski client'lar kırılmaz:

- `mindrail_bootstrap {}` read-only'dir; session, task veya lease oluşturmaz.
- `mindrail_complete {"task_id":"TSK-…","required":["test"]}`
  **evaluation-only** çalışır: yalnız gate değerlendirmesi yapar. Task'ı
  tamamlamaz, revision ilerletmez ve lease bırakmaz. Low-level terminal
  mutation CLI `task complete` sözleşmesindedir.
- Explicit `task_id`, `session`, `expected_revision` ve `operation_id` kullanan
  eski lifecycle payload'ları geçerliliğini korur.
- Otomatik `mindrail_complete {"finalize":true}` farklıdır: shared evaluator
  sonrasında guarded terminal transition ve lease cleanup yapar.

Explicit-ID tool örnekleri:

| Tool | Legacy input |
| --- | --- |
| `mindrail_claim` | `{"task_id":"TSK-…","session":"SES-…","expected_revision":1,"operation_id":"claim-1"}` |
| `mindrail_before_change` | `{"task_id":"TSK-…","paths":["/absolute/repo/internal/parser.go"],"operation_id":"scope-1"}` |
| `mindrail_after_change` | `{"task_id":"TSK-…","operation_id":"after-1"}` |
| `mindrail_reconcile` | `{"task_id":"TSK-…","operation_id":"reconcile-1"}` |
| `mindrail_checkpoint` | `{"task_id":"TSK-…","session":"SES-…","note":"…","handoff":true,"operation_id":"checkpoint-1"}` |
| `mindrail_validate` | `{"profile":"test"}` |
| `mindrail_complete` | `{"task_id":"TSK-…","required":["test"]}` |

Legacy `before_change.paths` absolute ve repository root içinde olmalıdır.
`context.detail_level` yalnız `summary|focused`; search limiti `1..50`;
invariant `mode` yalnız `active`dır. `validate` config'te tanımlı profile adını
ister. `validate`/`complete` için `budget`, `escalation` veya `approval` alanını
göndermek 0.1'de `NOT_IMPLEMENTED_IN_THIS_VERSION` refusal'ı üretir.

### Evidence ve completion ayrıntıları

`mindrail_validate {"profile":"test"}`, profile komutlarını çalıştırıp
snapshot'a bağlı evidence rows üretir. Freshness, en yeni run'da bütün command
indekslerinin var, `PASS` ve exit code 0 olmasını; kapsam yeniden enumerate
edildiğinde aynı kalmasını ister. Kapsama dosya ekleme/silme/değiştirme önceki
evidence'i stale yapar; malformed/legacy metadata fail-closed reddedilir.

Validation snapshot'ı Git çalışma ağacında Git-duyarlıdır: profile `paths`
seçiminin içindeki tracked dosyalar ile ignore edilmeyen untracked dosyalar
hash'e girer. `.git` metadata'sı ve `.gitignore` ile dışlanan `.next`, geçici
hook çıktıları veya `*.tsbuildinfo` gibi runtime/build çıktıları kanıtı kendi
başına stale yapmaz. Profile'da literal olarak açıkça adlandırılan ignored bir
dosya yine kapsamdadır. Seçilen bir submodule içeriği güvenli biçimde
bağlanamadığı için sessizce atlanmaz; snapshot fail-closed reddedilir.

Canonical reconcile gerçek Git diff'ini bulur ancak aynı fiziksel worktree'de
birden çok agent'ın bıraktığı kirli editin sahibini güvenilir biçimde çıkaramaz.
Ayrı worktree kullanın veya scope/lease koordinasyonunu sıkı tutun. İlgisiz
dirty değişiklik ALLOW kanıtı değil denial/ambiguity nedenidir.

## Sorun giderme

| Belirti / code | Anlamı ve çözüm |
| --- | --- |
| `COMMAND_LINE_INVALID` ile `mindrail mcp --json` | `--json` kaldırın. MCP stdout'u protocol-only kalır; typed hata stderr'dedir. |
| Automatic işlem “bootstrap on this connection” ister | Önce goal+stable run key ile bootstrap yapın; restart sonrası aynı `run_key`i açıkça kullanın. |
| `no_profiles_configured:true` | Hiç validation profile çalışmadı. Testlerin geçtiğini varsaymayın; config/CI kontrollerini ekleyin. |
| `TASK_STATE_INVALID` ile `task state --to COMPLETED` | Low-level terminal transition `task complete`; normal agent akışı `mindrail_complete {"finalize":true}` kullanır. |
| `SESSION_NOT_FOUND` / `TASK_NOT_FOUND` | ID yanlış veya başka repository runtime'ına ait. Normal akışta ID taşımayın; legacy akışta list/open sonucunu kullanın. |
| `LEASE_CONFLICT` | Başka session aktif lease tutuyor. Ayrı scope/worktree kullanın veya holder release/expiry bilgisini izleyin. |
| `CONFIG_INVALID` | Strict TOML anahtarlarını, `output.color`, profile type/paths/argv alanlarını düzeltin. |
| `NOT_IMPLEMENTED_IN_THIS_VERSION` | Error `next_action`daki daha dar isteği kullanın; ertelenmiş alanı kaldırın. |
| `verify` denial | `denials[].code` ve `next_action`ı çözün; hook bypass yerine CI `verify --ci` kapısını koruyun. |

## 2026-09-26 tarihsel denetim ve güncel durum

Bu kılavuz güncel çalışma ağacındaki sıfır-seremoni sözleşmesini anlatır.
`2814acb` snapshot'ındaki tarihsel denetim AUD-01…AUD-04'ü HIGH olarak
doğrulamış ve o release'i `REJECT / NOT_VERIFIED` ilan etmişti: failed/mixed
validation kabulü, dizin kapsamındaki yeni dosya freshness boşluğu, fatal
knowledge'ın completion'da atlanması ve completion öncesi canonical reconcile
eksikliği.

Güncel remediation validation run/scope semantiğini ve knowledge yüklemeyi
fail-closed yapar; completion canonical reconcile ve pre/post HEAD kontrollü
guard baseline kullanır. Tarihsel kanıt silinmemiştir:

- [Tarihsel rapor](audits/2026-09-26/report.md)
- [Nihai remediation raporu](audits/2026-09-26-remediation/report.md)
- [Üçüncü doğrulama turu](audits/2026-09-26-remediation/verification-round3.md)

`guard_baselines` migration schema 9'da workspace+HEAD mapping'ini task
discovery index'i değişmeden önce saklar; eksik/bozuk baseline veya Git okuma
hatası deny/error üretir.

## Güncelleme ve release

Release notes/asset kanalı henüz tanımlı olmadığından yeni kaynak revizyonunu
kontrollü çekin, temiz tree üzerinde `make check` ve gerekirse `make verify`
çalıştırın, sonra `make build` ile yeniden derleyin. Binary kimliğini `mindrail
version --json` ile kaydedin. `make release` platform matrix, checksum ve Linux
native smoke üretir; yayımlama/dağıtma yapmaz.

Bu kılavuzdaki normal CLI sözleşmesi çalışan binary help'i ve CLI contract
testleriyle; automatic bootstrap/finalize, restart replay, aynı bağlantıda
duplicate finalize ve protocol-only stdout davranışı gerçek persistent stdio
subprocess E2E'siyle doğrulanmıştır.

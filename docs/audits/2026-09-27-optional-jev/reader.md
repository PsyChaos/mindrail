# Optional Jev Routing — Reader Audit

## Denetim bağlamı

- Mod: paralel alt ajanlar; bu dosya Breaker sonucunu görmeden yazılan bağımsız Reader
  değerlendirmesidir.
- Orijinal istek: “yap onerinede uyuyorum. yanliz jev opsiyonel bir secenek olmali. key girilirse jev calissin girilmezse normal akis devam etsin”
- Yetkili donmuş şartname: `docs/engineering/optional-jev-routing-2026-09-27.md`
- Base revizyon: `ba35fbe`
- İncelenen yüzey: audit planındaki adapter/test kümesi ile skill, `AGENTS.md` ve
  şartname dokümantasyonu.

## 1. Görev normalizasyonu

| ID | Gereksinim | Görevden kaynak | Tür | Doğrulama yöntemi |
| --- | --- | --- | --- | --- |
| USER-001 | Önceden önerilen Jev yönlendirmesi uygulansın. | “yap onerinede uyuyorum” | FUNCTIONAL | Donmuş şartnamedeki REQ-001–008 ile diff ve testleri izlemek. |
| USER-002 | Jev zorunlu değil, açıkça opt-in olsun. | “jev opsiyonel bir secenek olmali” | CONSTRAINT / COMPATIBILITY | Key yok/boş dallarında ağ çağrısı yapılmadığını ve normal akış talimatını doğrulamak. |
| USER-003 | Key verildiğinde Jev yolu etkinleşsin. | “key girilirse jev calissin” | FUNCTIONAL | Ortam anahtarıyla aktivasyon, HTTP sözleşmesi ve başarılı çok-boyutlu seçim testi. |
| USER-004 | Key verilmezse mevcut akış değişmeden sürsün. | “girilmezse normal akis devam etsin” | NON_REGRESSION | Typed disabled sonuç, sıfır HTTP/STDIN okuması ve agent dokümanlarındaki fallback kuralı. |

`TASK_AMBIGUITY`: “key girilirse jev calissin” ifadesi, her kararda koşulsuz çağrı veya
yalnızca belirsiz yönlendirme kararlarında çağrı olarak okunabilir. Donmuş şartname ve
önceki konuşmanın konu alanı ikinci yorumu bağlayıcı hale getiriyor: Jev kapalı aday
kümeleri için shadow/advisory karar desteğidir; deterministik politika ve Mindrail
kapıları üzerinde yetki sahibi değildir. Kod ve iki agent dokümanı bu yorumla
tutarlıdır.

## 2. Gereksinim kararları

| Gereksinim | Karar | Pozitif kanıt | Not |
| --- | --- | --- | --- |
| REQ-001 | PASS | `jev_route.py:101-108,358-368,409-421`; `test_missing_or_blank_key_is_disabled_without_network`, `test_cli_without_key_does_not_read_stdin`; ayrıca `env -u TYPESAFE_API_KEY .../jev_route.py </dev/null` exit 0 ile typed `disabled/api_key_missing` üretti. | Eksik ve whitespace-only key aynı opt-out yoluna gider; ağ çağrısı input doğrulamasından önce engellenir. |
| REQ-002 | PASS | `jev_route.py:136-215,226-288`; `test_successful_multi_dimension_request_and_mapping`, `test_empty_dimensions_are_not_asked`, `test_invalid_choice_answers_are_rejected`, `test_missing_or_extra_answers_are_rejected`. | Provider yalnızca `candidate_N` değerlerini görür; dönüş, caller ID listesine sınır kontrollü eşlenir. |
| REQ-003 | PASS | `jev_route.py:81-99,290-312`; `.claude/skills/engineering-orchestrator/SKILL.md:94-110`; `AGENTS.md:44-55`; başarılı test `advisory: true`, `mode: shadow` doğrular. | Seçim icra eden veya Mindrail kararlarını değiştiren production çağrı yolu eklenmemiştir. |
| REQ-004 | PASS | `jev_route.py:315-355,391-406`; provider, timeout, malformed/oversized response, invalid choice, low-confidence/no-match ve atomic fallback testleri geçti. | Provider/yanıt hataları sanitized typed fallback üretir; CLI provider fallback için exit 0 döndürür. |
| REQ-005 | PASS | `jev_route.py:101-133,207-215,374-387`; key yalnız Bearer header'a konur. Key non-disclosure, semantic input secret engeli, input/output bound ve invalid-key testleri geçti. | Request gövdesi sadece caller tarafından verilen goal/context ve aday açıklamalarından üretilir. |
| REQ-006 | PASS | `jev_route.py:22-38,61-66,183-215,218-263,379-398`; HTTP sözleşmesi, Choice payload/yanıtı, redirect reddi, timeout ve bounded read testleri geçti. | Yalnız Python standart kütüphanesi kullanılmıştır; üçüncü taraf runtime bağımlılığı eklenmemiştir. |
| REQ-007 | PASS | `.claude/skills/engineering-orchestrator/SKILL.md:72-110` ve `AGENTS.md:34-55` aktivasyon, stdin sözleşmesi, advisory/atomic kabul, no-key, provider fallback ve local caller-error davranışını anlatır. | Managed Mindrail bölümü base ile byte-level `diff -u` karşılaştırmasında farksızdır. |
| REQ-008 | PARTIAL | `python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p 'test_jev_route.py' -v` komutu 29/29 PASS verdi. | Audit planındaki `make tidy-check`, `make verify`, `make gate` tam kapıları bu Reader turunda tek-suite politikasına uygun olarak tekrar çalıştırılmadı; final entegrasyon kanıtı bekleniyor. |

## 3. Bulgular

Reader tarafından severity verilecek, üretilebilir bir şartname veya davranış
uyumsuzluğu bulunmadı.

| ID | Severity | Gereksinim | Dosya/sembol | Problem | Kanıt | Güven |
| --- | --- | --- | --- | --- | --- | --- |
| — | — | — | — | Bulgusuz. | 29 odaklı test PASS; gereksinim–kod–doküman izi tamamlandı. | CONFIRMED |

## 4. Kapsam uyumu

- `UNDER_IMPLEMENTATION`: bulunmadı. Adapter, offline testler ve agent çağrı
  talimatları birlikte teslim edilmiş.
- `OVER_IMPLEMENTATION`: bulunmadı. Jev production Go core'a, MCP araç listesine veya
  icra/yetki katmanına eklenmemiş.
- `SCOPE_CREEP`: bulunmadı. Değişiklikler audit planında tanımlanan iki kümeyle
  sınırlı; audit artefaktları dışında ilgisiz dosya yok.
- `BEHAVIORAL_DRIFT`: bulunmadı. No-key dalı stdin okumadan ve ağ kurmadan exit 0 ile
  mevcut akışa döner.
- `REQUIREMENT_MISINTERPRETATION`: bulunmadı. Uygulama donmuş shadow/advisory yoruma
  uyuyor.

Reader değerlendirmesine göre uygulama, kullanıcının “opsiyonel; key varsa çalış,
yoksa normal akış” isteğiyle birebir uyumludur.

## 5. Sözleşme ve dokümantasyon uyumu

- `SPEC_IMPLEMENTATION_CONFLICT`: bulunmadı. Key aktivasyonu, closed candidate set,
  `jev-latest`, Choice soruları, confidence eşiği, atomic fallback, hata exit kodları
  ve sınırlar şartnameyle uyumlu.
- `DOC_DRIFT`: bulunmadı. Repository-level ve skill-level talimatlar aynı kabul ve
  fallback semantiğini anlatıyor.
- `AGENTS.md` içindeki `BEGIN/END MINDRAIL MANAGED SECTION` bloğu `ba35fbe` ile
  karşılaştırıldığında değişmemiştir; Jev rehberi managed bölümün dışına eklenmiştir.

## 6. Açık maddeler

| Madde | Neden bu turda üretilmedi | Sonuçlandıracak kesin deney |
| --- | --- | --- |
| Tam repository regresyon kapıları | Audit planı tam kapıları entegrasyon aşamasında bir kez çalıştırmayı ister; Reader yalnız cluster-targeted testi çalıştırdı. | Repository kökünde sırasıyla `make tidy-check`, `GOCACHE=/tmp/mindrail-jev-gocache make verify`, `GOCACHE=/tmp/mindrail-jev-gocache make gate` çalıştır; üç komutun exit 0 olması REQ-008'i PASS'e yükseltir, herhangi bir non-zero sonuç ilgili logla bulguya dönüşür. |

## 7. Çalıştırılan doğrulama

```text
python -m unittest discover -s .claude/skills/engineering-orchestrator/scripts -p 'test_jev_route.py' -v
Ran 29 tests in 0.033s
OK
```

```text
env -u TYPESAFE_API_KEY python .claude/skills/engineering-orchestrator/scripts/jev_route.py </dev/null
exit 0
{"advisory":true,"enabled":false,"mode":"shadow","reason":"api_key_missing","selections":{},"status":"disabled","version":1}
```

Managed-section kontrolü:

```text
diff -u <(git show ba35fbe:AGENTS.md | sed -n '/BEGIN MINDRAIL MANAGED SECTION/,/END MINDRAIL MANAGED SECTION/p') <(sed -n '/BEGIN MINDRAIL MANAGED SECTION/,/END MINDRAIL MANAGED SECTION/p' AGENTS.md)
exit 0; output yok
```

## 8. Bağımsız sonuç

`COMPLIANT_WITH_MINOR_ISSUES`

Kod, odaklı testler ve dokümantasyon USER-001–004 ile REQ-001–007'yi pozitif
kanıtla karşılıyor. Ürün bulgusu yoktur. Tek eksik, REQ-008'in repository-wide gate
yarısının final entegrasyon aşamasında henüz üretilmemiş olmasıdır; üç tam kapı exit 0
verdiğinde Reader sonucu `FULLY_COMPLIANT` olur.

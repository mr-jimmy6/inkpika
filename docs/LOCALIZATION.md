# Localization

English and Chinese share one device-owned preference in `config.json`:

```json
{"language": "auto"}
```

`auto` (also the default when omitted) reads `/var/local/system/locale`, accepting
Kindle `LANG=en_US.utf8` / `LANG=zh_CN.utf8` assignments and bare locale strings.
Chinese regional variants use the Chinese catalog. Missing, oversized,
unreadable or unsupported locale data falls back to English. `en` and `zh`
override the device. No host environment, HTTP `Accept-Language`, browser locale,
IP address or visitor query parameter participates in device language selection.
Local fixtures cover this file-based integration. Executed checks and physical-device
language coverage are recorded in [validation](DEVICE_VALIDATION.md).

The settings portal supports a revision-checked, atomic language-only save before
cloud credentials exist. It uses the same backup, locking and service lifecycle
hooks as other saves, without changing credentials, source IDs, state paths or
book files. A missing shared config follows the device; a malformed shared config
uses English until repaired. Cloud config loading rejects invalid preferences.

The running cloud library uses its loaded preference. Settings saves safely restart
it only if it was already running. Transfer and configuration services read the
shared language preference per request, independently of cloud validation. They
return `Content-Language`, and their ES5 clients update the interface without
re-pairing. Refresh other open pages after changing language. Hand-editing the
config requires restarting the cloud library and refreshing KUAL.

KUAL labels are translated before line splitting/truncation. Book keys and fixed
shell actions remain unchanged; KUAL internal status commands stay ASCII because
of its awk/Java byte-count mismatch. The packaged bootstrap menu is English until
a service action or menu refresh regenerates it on the device. The WAF bootstrap's
short loading message is English before it connects to the localized local page.

## Catalogs and boundaries

- `internal/i18n/zh.json`: English message keys and Chinese translations for
  device UI, backend application messages, and the interactive sample.
- `internal/i18n/language.go`: bounded language reads and deterministic fallback.
- `internal/i18n/messages.go`: static-asset rendering, message templates and an
  explicit JSON human-message field allowlist. Filenames, object keys, paths,
  credentials and machine status codes are never localization inputs.
- `internal/i18n/runtime.js`: ES5 text, attribute and dynamic UI localization.
  It captures static nodes before application rendering and never walks new
  book rows during subsequent language changes.
- `site/locales/zh.json`: public-page Chinese copy. Public HTML and README.md are
  English; README_ZH.md is the complete Chinese user guide.

Application errors/logs originate in English. Known templates render in Chinese
at API/KUAL presentation boundaries; reverse templates render older persisted
Chinese messages in English without rewriting saved records. Unknown OS/library
messages retain their original text. Template arguments are preserved; only
explicit wrapped-error slots recurse into another application message.

The website uses explicit `?lang=en` / `?lang=zh` links and defaults to English.
It never guesses the visitor language. Demo frames follow the simulated device's
language preference, initially the chosen page language; settings can override it
without changing the public page language or pairing state. All demo operations
stay local to the browser.

Regenerate checked-in native sample assets, both KUAL menu locales and the
combined public catalog with:

```sh
go run ./scripts/site-demo
node --test scripts/site-demo/model.test.cjs
```

Keep all device scripts ES5-compatible and use the same catalogs for the real
interfaces and generated preview. Verify both languages, unsupported-locale
fallback, conflicting visitor headers, pre-credential language saves, pairing
retention, user data preservation and narrow layouts. Local browser tests and
ARM compilation are not physical Kindle validation.

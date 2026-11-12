# Repository guidance

## Purpose and naming

InkPika connects Kindle users to their books
through read-only object storage and local transfer, retaining the native Kindle reader.
The product targets Kindle devices across generations; current installation requires
jailbreak and KUAL. The build baseline is a Japanese Kindle Paperwhite 2,
firmware 5.12.2.2 and Linux 3.0.35. Record the exact tested build and scope in `docs/DEVICE_VALIDATION.md`.
It supports Alibaba Cloud OSS, QingStor, Azure Blob and S3-compatible object storage, optional SOCKS5 proxy / additional CA configuration, local wireless transfer, and paired remote configuration. Use `inkpika` for the installed executable
and extension, `InkPika` for its default documents directory, and
`github.com/mr-jimmy6/inkpika` for Go imports. Use InkPika with this exact capitalization in all languages.
The repository name is `inkpika`; use its origin, public URLs and Go module consistently.
No prior-brand aliases or migration entry points are part of this development build.
QingStor names belong to its provider, protocol fixtures and configuration, never shared product branding.

## Layout and boundaries

- `cmd/inkpika`: production CLI, process lifecycle and local sync commands.
- `cmd/preview`: desktop preview with synthetic data; excluded from releases.
- `internal/config`: loading and common validation; provider rules in
  `providers.go`, source IDs in `config.go`.
- `internal/providers`: factory plus `aliyunoss`, `qingstor`, `s3` and `azureblob` adapters.
- `internal/storage`: shared read-only `Store` contract. Consumers close every
  returned HTTP response body. Pagination markers are opaque.
- `internal/cloudhttp`: shared TLS, CA roots, connection and redirect policy.
- `internal/download`: resumable queue, filenames and shared transfer lock.
- `internal/syncer`: recursive `sync/` reconciliation and KUAL menu generation.
- `internal/statefile`: atomic local-state replacement.
- `internal/web`: loopback HTTP API and embedded HTML/CSS/ES5 assets.
- `internal/transfer`: LAN ebook transfer and shared paired connection/QR portal; cloud credentials never enter the transfer handler.
- `internal/settings`: separately paired LAN configuration editor with masked prefill and atomic validation/save.
- `internal/bookformat`: shared configurable ebook extension allowlist.
- `internal/i18n`: device language resolution and shared English/Chinese messages; see `docs/LOCALIZATION.md`.
- `internal/buildinfo/VERSION`: single release-version source, embedded in Go
  and read by the packager to render extension XML versions.
- `extensions/inkpika`: KUAL scripts, configuration examples and WAF UI.
- `scripts`: validation, cross-compilation and allowlisted release packaging.

Keep provider selection in `providers.New`; callers supply a normalized config.
Every cloud configuration must explicitly select a provider.
Use the standard library unless a new dependency has a clear device benefit.
Read `docs/DEVELOPMENT.md` before changing package boundaries or API behavior.

## Device constraints

- `.go-version` pins Go 1.23.12. Go 1.24+ requires a newer Linux kernel.
- Release target: `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5`.
- Device scripts must work with POSIX `sh` and BusyBox. No Bash-only syntax,
  Python, Node.js or external runtime on the Kindle.
- UI code must remain ES5-compatible with old Mesquite/WebKit. No modern JS
  syntax, framework dependency, online assets, animations or aggressive polling.
- KUAL submenu navigation must not set `refresh: true`; it resets the menu path.
  Reserve refresh for explicit status/sync and service actions.
- The WAF app ID is `com.inkpika.app`; only its own registration may change.

## Data and security invariants

- Cloud APIs and device connection details listen only on loopback. Only explicitly enabled transfer and configuration listeners accept LAN requests. Preserve Host, token and POST Origin validation; never expose cloud APIs/configuration through the transfer listener.
- Use HTTPS with certificate/hostname validation. Never forward credentials
  through redirects, expose them in the API, or log session URLs/tokens.
- Remote storage operations are read-only. Do not add uploads/deletes as part of
  unrelated work. Remote deletion must not delete downloaded books.
- ETags are opaque version identifiers, not assumed MD5 checksums.
- Browser downloads must not overwrite existing books. Sync replaces only
  verified, registered copies after integrity and remote-version checks.
- Preserve source identity, job IDs, target filenames and state compatibility.
  Changes need a migration strategy and regression coverage; see `docs/COMPATIBILITY.md`.
- Keep `download.Manager.WithTransfer` shared between browser and sync writes.
  State and downloaded books must stay on the same filesystem for rename.
- Cloud object names must never become shell code or KUAL action parameters.

## Verification

From the repository root:

```sh
sh scripts/check.sh
sh scripts/build.sh
```

`check.sh` runs formatting, Go race tests/vet, shell syntax and packaging tests.
It intentionally targets `cmd/` and `internal/`; ignored `local/` may contain
private scratch programs that are not part of the project.
Add regression tests for behavior changes and compatibility risks. Do not add
mirror tests for prose, cosmetic edits or simple moves. Tests use local fixtures
and require no cloud account or device. For provider changes, cover signatures,
encoding, pagination, errors, redirects, Range and conditional requests.

For preview: `GOTOOLCHAIN=go1.23.12 go run ./cmd/preview -provider aliyun_oss`.
It uses synthetic files. Do not claim preview/automated tests prove device
compatibility. Record validation of the current release in `docs/DEVICE_VALIDATION.md`;
never claim another build establishes validation of this binary.

## Device compatibility records

Maintain `docs/DEVICE_COMPATIBILITY.md` as the device/firmware compatibility ledger,
separate from configuration/state compatibility in `docs/COMPATIBILITY.md`.
Keep the device validation status table at the top, with model, firmware, kernel, KUAL,
tested build/date, feature coverage and links to `docs/DEVICE_VALIDATION.md`.
For every device/firmware category, record the compatibility work actually done:
shared implementation measures and category-specific changes, source references,
validation performed, known limitations and remaining adaptation/verification work.
Update the affected category whenever compatibility-related code or evidence changes.
Do not describe a planned change, successful build, desktop preview or another
model's test as completed device adaptation or validation. State explicitly when
a category has no dedicated adaptation or physical-device evidence.
Keep the broad product audience distinct from verified support in README editions
and the project website; avoid positioning the project only for old Kindles.

## Repository hygiene and documentation

This is a public open-source project. Never commit sensitive or private information,
including credentials, personal filesystem paths, actual device/network addresses,
account or device identifiers, personal documents, or unredacted logs and screenshots.
Use synthetic or redacted examples in code, documentation and tests; keep private
operational data only in ignored local files.

Never commit real `config.json`, state, credentials, session files, device
captures, generated binaries or `dist/`. Preserve ignored local artifacts.
`testdata/books/` contains original, redistributable fixtures. Keep release
packaging allowlisted; new runtime files must not be picked up by globbing.
Project code is MIT; retain third-party notices and license texts.

Before every `git push` (including tag pushes and force-pushes), run Gitleaks
after the final commit and require a successful scan with no unresolved findings:

```sh
gitleaks git . --log-opts="--all" --redact=100 --max-decode-depth=3
```

Scan all refs that will be pushed; rerun if their commits change. If Gitleaks is
unavailable, fails, or reports a finding, resolve the problem before pushing.
Do not bypass a failing scan with a success exit code or broad allowlists.
Keep scan output redacted and any reports under ignored `local/`. Also review
personal paths, account/device identifiers and image metadata; secret scanning
does not replace that privacy review.

Keep README installation paths, compatibility guidance, architecture and examples
consistent with the code. InkPika is in development: keep the package version fixed at
`0.0.1.dev` in `internal/buildinfo/VERSION` until a release version is explicitly authorized.
The package is `inkpika-0.0.1.dev-armv5.zip` plus its checksum. Derive package metadata
from that version source; do not bump it for ordinary development changes.
Embed the same UTC package time in the executable and `build.json`. On the device's
configuration information page, show version and package time as the final two rows.
Keep configuration information and manual connection tests in Settings; the cloud
library page contains browsing and downloads. Pairing and editing stay in their
separate authenticated settings service; transfer must never expose configuration.
Maintain `RELEASE_NOTES.md` as the current development distribution notes; do not
claim an initial stable release, add renamed-product history or add migration-only tools.
Keep current technical requirements, data formats, licenses and compatibility limits.
Keep READMEs concise: installation, everyday usage, prerequisites and documentation links.
Do not put per-version test reports or feature/device test status in README editions.
Keep actual results, tested binary/package identity and remaining checks in
`docs/DEVICE_VALIDATION.md`, with the device-category ledger in
`docs/DEVICE_COMPATIBILITY.md`; link to them from READMEs and the website.
Keep provider details, permission policies, NAS instructions, proxy/CA setup and cost
estimates in `docs/OBJECT_STORAGE.md` and `docs/OBJECT_STORAGE_ZH.md`. Update both
editions together and include them in the explicit package allowlist.
Build success does not establish physical-device or cloud-account validation.
Whenever a user-facing README changes, update every language edition in the
same change (currently `README.md` (English) and `README_ZH.md` (Chinese)). Keep feature descriptions,
installation instructions, links and illustrations in sync, using localized
wording and assets. `README_DEV.md` is the developer guide, not a language edition.
Keep user-facing setup in `README.md` and developer workflows in `README_DEV.md`. English is the default language for README.md and the public website.
Write identifiers, comments, developer-facing messages and commit messages in English.
Keep Chinese interface translations in localization resources. Support English and
Chinese with a shared device language: `language=auto` follows the Kindle system
locale, explicit `en`/`zh` overrides it, and unrecognized locales fall back to English.
LAN services must never select a language from visitor headers or browser settings.
Do not translate filenames, object keys, credentials or persistent machine codes. State precisely what was checked and
what still requires a real PW2 or cloud account. Do not publish a release or
operate a connected Kindle merely because a local build succeeded.

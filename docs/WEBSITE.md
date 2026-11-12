# InkPika website

`site/` is the public English website with an explicit Chinese language switch. It describes InkPika 0.0.1.dev, uses the same features and installation instructions as both READMEs, and links to the central feature/device validation record. The website is separate from the on-device UI and is not included in the installation ZIP.

- Name: **InkPika**, with the same capitalization in both languages.
- English slogan: **Transfer books over Wi-Fi. Download from your own cloud.**
- Chinese slogan: **无线直传，云端取书。**
- English introduction: Transfer books between your phone, computer and Kindle over Wi-Fi, or download ebooks from your own object storage and open them in the native Kindle reader.
- Chinese introduction: 通过 Wi-Fi 在手机、电脑与 Kindle 之间传书，也可从自己的对象存储下载电子书，在 Kindle 原生阅读器中打开。

The actual repository remains [github.com/mr-jimmy6/inkpika](https://github.com/mr-jimmy6/inkpika). Keep its working URLs until a separate repository rename is authorized; `inkpika` is the suggested repository name. The [Pages URL](https://mr-jimmy6.github.io/inkpika/) and [interactive demo URL](https://mr-jimmy6.github.io/inkpika/preview/index.html) use that repository path. Local file changes do not publish either page.

## Files

- `index.html`: English copy, metadata, feature comparison, installation prompt and FAQ.
- `locales/zh.json`: Chinese translations; the combined runtime catalog is generated into `assets/i18n.js`.
- `assets/style.css`, `main.js`, `language.js`: responsive layout, accessible navigation and explicit language selection. No browser-language guessing.
- `assets/favicon.svg`: original book icon. `assets/previews/` contains original localized SVG illustrations and public sample data, not device screenshots.
- `preview/index.html`, `demo.*`, `model.js`, `bridge.js`: browser-only demo navigation, state and request handling.
- `preview/native/`, `sample-data.js`: generated production interfaces and public book fixtures; regenerate after changing source assets or menu labels.

The mobile header uses CSS sticky positioning. Device pages remain ES5 and use system fonts, with no framework or online assets. The website's illustration is labeled as such; the interactive demo reuses source interfaces but simulates service, pairing, sync and transfer operations. It does not contact real devices or cloud accounts. Selected files stay in browser memory; reset or close the page to clear them.

## Local maintenance

From the repository root, regenerate public assets without running tests:

```sh
GOTOOLCHAIN=go1.23.12 go run ./scripts/site-demo
```

For a later, separately authorized browser review, serve `site/` on loopback:

```sh
python3 -m http.server 8000 --bind 127.0.0.1 --directory site
```

Review English/Chinese content, narrow layouts, keyboard navigation, all demo entries and Pages subdirectory URLs. Record executed checks in DEVICE_VALIDATION.md; browser checks and automated tests do not substitute for device validation.

## Installation and publication

The installation entry names `inkpika-0.0.1.dev-armv5.zip` and its SHA-256 file. Local builds are available through the developer workflow; the Releases link is for packages after publication. Explain jailbreak, KUAL and the unverified device scope before installation. The agent prompt uses the prepared binary and requires a user-authorized target; SSH private keys stay on the user's computer.

`.github/workflows/pages.yml` is a manually triggered Pages deployment that uploads only `site/`. Publishing requires a separate authorized push and workflow run. Follow the repository's Gitleaks requirement before any push. This local release preparation performs neither action.

Keep [device compatibility](DEVICE_COMPATIBILITY.md), [validation scope](DEVICE_VALIDATION.md), [release notes](../RELEASE_NOTES.md) and both READMEs consistent. PW2 is the engineering baseline; describe only the coverage established for the exact tested build.

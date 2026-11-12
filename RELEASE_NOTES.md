# InkPika development build · 0.0.1.dev

InkPika is in development. The version remains `0.0.1.dev`; the UTC package time identifies each build.

Transfer ebooks over Wi-Fi or download them from read-only object storage, then open them in the native Kindle reader. Features include paired wireless upload/export, cloud browsing and downloads, manual KUAL sync, English/Chinese interfaces, and paired configuration editing. Settings contains masked configuration information and manual connection tests; brand, version and UTC build time appear in a compact footer on every web page.

- Package: `inkpika-0.0.1.dev-armv5.zip` and its `.zip.sha256`.
- Installation: [user guide](README.md#installation). Requires jailbreak, KUAL and compatible Mesquite/WAF; the build targets Linux ARMv5 with Go 1.23.12.
- Provider setup: [object storage guide](docs/OBJECT_STORAGE.md).
- Actual test results and build identity: [validation record](docs/DEVICE_VALIDATION.md). Device requirements and remaining adaptation: [compatibility ledger](docs/DEVICE_COMPATIBILITY.md).

No format conversion, DRM handling, cloud uploads or remote deletion is provided. Native reading depends on the device's formats. Code is MIT; third-party [notices](THIRD_PARTY.md) are retained. A source push does not publish a GitHub Release.

## 简体中文

InkPika 当前处于开发期，版本固定为 `0.0.1.dev`，不同构建以 UTC 打包时间区分。

支持无线双向传书、只读云端浏览与下载、手动 KUAL 同步、中英文界面和手机／电脑配对配置。配置信息与手动连接测试位于“配置设置”，品牌、版本和 UTC 构建时间统一显示在各 Web 页面底部。

安装包为 `inkpika-0.0.1.dev-armv5.zip` 及对应 `.zip.sha256`。安装方法见[使用说明](README_ZH.md#安装)，平台设置见[对象存储配置](docs/OBJECT_STORAGE_ZH.md)。测试结果和构建标识统一记录在[验证记录](docs/DEVICE_VALIDATION.md)，机型要求与适配工作见[设备兼容性文档](docs/DEVICE_COMPATIBILITY.md)。源码推送不等于发布 GitHub Release。

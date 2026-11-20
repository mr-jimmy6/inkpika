# InkPika 0.0.1

Transfer ebooks between a phone, computer and Kindle over Wi-Fi, or browse and download books from read-only object storage. Downloaded books open in the native Kindle reader. This version includes paired wireless upload and export, manual KUAL sync, English and Chinese interfaces, and a paired configuration editor. Settings shows masked credentials and offers connection tests.

- Download `inkpika-0.0.1-armv5.zip` and its `.zip.sha256` file. Verify the checksum before extracting the ZIP. See the [installation guide](README.md#installation).
- Installation requires a jailbroken Kindle with KUAL and compatible Mesquite/WAF. The package targets Linux ARMv5 and was built with Go 1.23.12.
- Supported storage protocols include Alibaba Cloud OSS, QingStor, Azure Blob and S3-compatible storage. See the [object storage guide](docs/OBJECT_STORAGE.md).
- This release was checked locally. Physical Kindle and cloud-account checks for the 0.0.1 binary are pending. See the [validation record](docs/DEVICE_VALIDATION.md) and [device compatibility ledger](docs/DEVICE_COMPATIBILITY.md).

The native reader determines which formats it can open. Format conversion, DRM handling, cloud uploads and remote deletion are outside the scope of this release. Code is licensed under MIT. See the [third-party notices](THIRD_PARTY.md).

## 简体中文

InkPika 支持通过 Wi-Fi 在手机、电脑与 Kindle 之间传书，也支持从只读对象存储浏览和下载电子书，再用 Kindle 原生阅读器打开。此版本包含配对传入与导出、KUAL 手动同步、中英文界面和配对配置编辑器。配置设置页可查看脱敏凭据并测试连接。

- 下载 `inkpika-0.0.1-armv5.zip` 和对应的 `.zip.sha256` 文件，核对摘要后解压。安装步骤见[使用说明](README_ZH.md#安装)。
- 安装需要已越狱的 Kindle、KUAL 和兼容的 Mesquite/WAF。本包使用 Go 1.23.12 构建，目标平台为 Linux ARMv5。
- 云存储支持阿里云 OSS、青云 QingStor、Azure Blob 和 S3 兼容服务。配置方法见[对象存储指南](docs/OBJECT_STORAGE_ZH.md)。
- 0.0.1 已完成本地检查，当前二进制仍待 Kindle 真机和真实云账号验证。详见[验证记录](docs/DEVICE_VALIDATION.md)和[设备兼容性记录](docs/DEVICE_COMPATIBILITY.md)。

可阅读的格式取决于 Kindle 原生阅读器。本版本不提供格式转换、DRM 处理、云端上传或远端删除。代码采用 MIT 许可证，第三方许可见[声明](THIRD_PARTY.md)。

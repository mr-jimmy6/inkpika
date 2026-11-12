# 参与开发

欢迎提交问题和 Pull Request。项目使用 MIT 许可证；提交贡献表示你有权提供这些内容，并同意按项目许可证分发。第三方材料保留原许可证，见 [THIRD_PARTY.md](THIRD_PARTY.md)。

## 本地开始

开发环境、构建检查、电脑预览，以及通过 USBNetwork Hack 配置 SSH 密钥登录并使用 Codex、Qoder 真机调试，见 [README_DEV.md](README_DEV.md)。自动测试和预览不需要真实云账号或设备。

修改前请阅读 [AGENTS.md](AGENTS.md)；架构与扩展约定见 [开发文档](docs/DEVELOPMENT.md)。

## 提交变更

- 说明具体问题、修改后的行为和验证方式。涉及旧配置、文件名、状态或安装路径时，提供兼容策略与回归测试。
- 新存储后端实现 `storage.Store`，在 `internal/providers/` 注册，补充配置校验、协议测试和示例。
- 界面保持 ES5，设备脚本保持 POSIX shell；只增加对目标设备有实际价值的依赖，保留已验证设备的兼容性。
- 设备兼容性改动需更新 [DEVICE_COMPATIBILITY.md](docs/DEVICE_COMPATIBILITY.md) 中对应机型 / 固件类别的已做工作、验证证据、问题和待办；新增真机证据后更新文首已验证设备表。
- 提交前运行检查和 ARM 打包。涉及 Mesquite/KUAL 的修改，注明是否完成真机验证；桌面预览不能代替设备测试。
- InkPika 处于开发期，版本固定为 `internal/buildinfo/VERSION` 中的 `0.0.1.dev`，产物为 `inkpika-0.0.1.dev-armv5.zip` 及校验文件。UTC 打包时间同时写入二进制与 `build.json`。README 保持简洁，测试结果放在 `docs/DEVICE_VALIDATION.md`，对象存储细节放在中英文独立指南。

不要提交真实配置、Bucket 标识、密钥、下载记录、会话地址、个人图书或设备截图。示例使用 `your-bucket` 等占位符。报告问题时附上版本、设备/固件、存储平台、复现步骤及脱敏日志。

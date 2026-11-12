# InkPika 开发指南

本文面向开发者，包含本地构建、预览、SSH 连接和真机调试。安装与日常使用见 [README](README.md)，提交规范见 [CONTRIBUTING.md](CONTRIBUTING.md)，架构与接口细节见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)。

当前为开发版本 **0.0.1.dev**，固定在 [`internal/buildinfo/VERSION`](internal/buildinfo/VERSION)。安装包为 `inkpika-0.0.1.dev-armv5.zip`，UTC 打包时间区分具体构建。测试结果统一维护在 [验证记录](docs/DEVICE_VALIDATION.md)。

## 开发环境

开发机需要 Git、Go（支持自动下载指定工具链）、Python 3.9+ 和 POSIX shell。推荐 Linux 或 macOS，Windows 可使用 WSL。真机调试另需 SSH 客户端及可连接的 Kindle。

| 项目 | 约束 |
| --- | --- |
| 产品目标 | 各代 Kindle；当前安装要求已越狱并安装 KUAL |
| 工程基线 | 日版 PW2 / 5.12.2.2 / Linux 3.0.35；其他机型见[设备兼容性与适配记录](docs/DEVICE_COMPATIBILITY.md) |
| Go 工具链 | [`.go-version`](.go-version) 固定为 1.23.12 |
| 交叉编译 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=5` |
| 设备脚本 | POSIX `sh` 与 BusyBox，不依赖 Bash、Python 或 Node.js |
| 设备 Web 界面 | Mesquite / 旧 WebKit，JavaScript 保持 ES5 |

PW2 使用 Linux 3.0.35，不能直接改用要求 Linux 3.2+ 的 Go 1.24 及更新工具链。Go 1.23 已退出官方安全维护，这是旧设备兼容性的维护限制。

兼容性工作按机型和固件类别记录：已有共享措施、专门适配、验证证据、已知问题与待办均写入 [DEVICE_COMPATIBILITY.md](docs/DEVICE_COMPATIBILITY.md)。真机记录仍写入 [DEVICE_VALIDATION.md](docs/DEVICE_VALIDATION.md)，验证通过后更新兼容性文档顶部的设备表；不能用构建或桌面预览替代设备验证。

## 构建与检查

在开发机执行：

```sh
git clone https://github.com/mr-jimmy6/inkpika.git inkpika
cd inkpika
sh scripts/check.sh
sh scripts/build.sh
```

- `check.sh`：Go 格式检查、race tests、vet、shell 语法、脚本生命周期和发行打包检查。
- `build.sh`：使用固定工具链生成 ARM 二进制，按白名单打包；版本从 `internal/buildinfo/VERSION` 读取；生成一次 UTC 打包时间，同时注入二进制和扩展中的 `build.json`，在 CLI `-version`、本机配置信息和配对配置页面显示。设置 `SOURCE_DATE_EPOCH` 可得到固定时间的可复现构建。源码直接运行时显示“未打包”，不使用当前时间冒充包时间。
- 当前产物：`dist/inkpika-0.0.1.dev-armv5.zip` 及对应 `.zip.sha256`。包内不包含个人配置、运行状态、日志或密钥。

自动测试使用本地夹具，不需要真实云账号或设备。若当前网络无法下载 Go 工具链，可仅为本次构建设置自己信任的镜像，例如：

```sh
GOPROXY=https://goproxy.cn sh scripts/build.sh
```

CI 执行相同检查和构建并保存产物，不会自动创建 GitHub Release。已发布安装包以 [GitHub Releases](https://github.com/mr-jimmy6/inkpika/releases) 为准，不能把本地构建或 CI 成功描述为已发布。

## 电脑端预览

在仓库根目录运行：

```sh
GOTOOLCHAIN=go1.23.12 go run ./cmd/preview -provider aliyun_oss
```

用浏览器打开终端输出的本机 URL。预览使用合成数据，不连接真实云服务；演示下载保存在 `local/preview/documents/`，预览入口不进入设备安装包。

`site/` 是面向用户的项目介绍页，与 Kindle 上的运行界面分开维护。在源码仓库中可运行：

```sh
python3 -m http.server 8000 --bind 127.0.0.1 --directory site
```

浏览器打开 `http://127.0.0.1:8000`，用 `Ctrl+C` 停止。本地预览不代表网站已上线；维护与 GitHub Pages 发布方式见[介绍页文档](https://github.com/mr-jimmy6/inkpika/blob/main/docs/WEBSITE.md)。

## 用 USBNetwork Hack 配置 SSH 密钥登录

**建议在开发机上配置 Kindle 的 SSH 免密登录，再使用 Coding Agent（例如 Codex、Qoder）协作开发和真机调试。** 可以使用 **USBNetwork Hack** 插件在 Kindle 上提供 SSH 连接能力。

先按适用设备及固件的插件说明安装、配置 USBNetwork Hack，并确认 SSH 可连接。USBNetwork Hack 的安装、公钥文件位置、USB 网络地址及 Wi-Fi SSH 开关以对应插件版本的文档为准；本项目不代为安装或开启它。

免密登录使用 SSH 密钥认证，仍需把开发机的公钥配置到设备上：

1. 在开发机准备一对设备 SSH 服务支持的密钥；若已有可用密钥，可以继续使用。旧版 SSH 服务可能需要 RSA。
2. 按 USBNetwork Hack 的说明安装**公钥**；私钥始终保留在开发机，不复制到 Kindle 或仓库。
3. 在开发机的 `~/.ssh/config` 中添加设备别名，填写实际地址和密钥路径。

例如，生成一对新的 RSA 密钥：

```sh
ssh-keygen -t rsa -b 3072 -f ~/.ssh/kindle_dev -C kindle-dev
```

如果该路径已有密钥，不要覆盖。加密的私钥可通过 `ssh-add ~/.ssh/kindle_dev` 加入本机 SSH Agent，避免每次连接重新输入口令。

下面的 IP 只是示例，请替换为设备当前的 USB 网络地址或局域网 IP；使用 Wi-Fi 时，还需按插件说明开启对应的 SSH 访问：

```sshconfig
Host kindle-dev
    HostName 192.168.1.100
    User root
    IdentityFile ~/.ssh/kindle_dev
    IdentitiesOnly yes
    ConnectTimeout 8
    ServerAliveInterval 5
    ServerAliveCountMax 2
```

首次连接核对设备身份并接受主机密钥后，检查密钥登录能否在无交互模式下工作：

```sh
ssh kindle-dev
# 退出设备终端后，在开发机执行：
ssh -o BatchMode=yes kindle-dev 'uname -a'
```

真机测试期间保持设备唤醒；需要操作屏幕时先在设备上解锁。屏幕密码不需要提供给 Coding Agent。

## 一键编译、打包与 SSH 安装

配置好上面的 SSH 密钥登录并核对主机密钥后，在源码仓库运行：

```sh
# 查看计划；不编译、不连接设备。
sh scripts/deploy.sh kindle-dev --dry-run

# 检查、交叉编译、打包，并安装到指定测试设备。
sh scripts/deploy.sh kindle-dev

# 也可以显式指定用户、端口和本机私钥。
sh scripts/deploy.sh root@192.168.1.100 -p 22 -i ~/.ssh/kindle_dev

# 当前代码已经通过检查时，可跳过检查；仍会重新编译和打包。
sh scripts/deploy.sh kindle-dev --skip-check

# 使用已打包的 ZIP；跳过源码检查、编译和打包。
sh scripts/deploy.sh kindle-dev --package dist/inkpika-0.0.1.dev-armv5.zip
```

`kindle-dev` 是开发机 SSH 配置中的示例别名，必须替换为明确授权安装的设备；脚本没有默认目标，也不查找设备。脚本使用非交互认证和严格主机密钥校验，沿用本机 SSH 配置，不自动放宽旧设备的加密算法。首次连接应先手工执行 `ssh kindle-dev` 核对主机密钥。通过脚本绝对路径调用时可以位于任意工作目录。

默认依次运行 `check.sh` 和 `build.sh`，沿用 `.go-version`、ARMv5 构建参数和发行白名单。ZIP 及 `.zip.sha256` 留在开发机 `dist/`；校验 ZIP 后，仅将白名单中的扩展文件通过 `tar` / SSH 发送到设备，固定安装路径为 `/mnt/us/extensions/inkpika`。设备需要 POSIX shell、BusyBox 的 `tar`、`sha256sum`、`mktemp`、`readlink` 等基础命令，不需要 SFTP、Python、Node.js 或解压 ZIP 的工具。

`--package ZIP` 直接使用已有安装包，不运行 `check.sh` 或 `build.sh`，也不修改该 ZIP。相对路径以调用命令时的工作目录为准，包可以放在仓库外；同目录必须有打包器生成的同名 `.zip.sha256` 文件。发送前检查 SHA-256、当前发行白名单和 32 位 ARM ELF 标识，缺失或不匹配时在 SSH 连接前退出。此模式可以与 `--dry-run`、`-i`、`-p` 一起使用，与 `--skip-check` 互斥；它仍执行下面的备份、安装和设备端校验。

部署时保持 Kindle 唤醒，并暂停操作本项目的 KUAL 菜单与网页。脚本先上传到 `/mnt/us/.inkpika-deploy.*` 暂存目录并校验摘要，再停止配置设置、无线传书和云端书架；有进程未退出或停止失败时拒绝替换文件。程序文件按同盘 rename 替换，并再次核对 SHA-256 和设置权限。设备已有的 `config.json`、`transfer.json`、下载/同步记录和图书不被覆盖；停止脚本会正常清理会话、刷新菜单及写入日志。安装后服务保持关闭，从 **KUAL → InkPika** 按需启动。脚本不修改 KUAL JAR、WAF 注册或唤醒设置，也不启动云端连接检查。

终端会显示设备上的备份目录，其中 `backup/` 保存本次替换前的程序文件，`existing-files` / `added-files` 记录原有和首次安装的路径；个人配置与状态不包含在这份程序备份中。替换或安装后摘要校验失败时，脚本尝试还原程序文件并返回失败，服务保持关闭。SSH 中断、设备断电或回退失败后，需要先检查该目录和设备状态再重试；不能保证断电时自动回退。确认安装可用后，可手工清理本次备份目录以释放空间。若提示 `/mnt/us/.inkpika-deploy.lock` 已存在，先确认没有部署仍在运行，再处理遗留锁。

自动部署、故障回退与数据保全的实际检查范围见[验证记录](docs/DEVICE_VALIDATION.md)；各机型适配见[设备兼容性记录](docs/DEVICE_COMPATIBILITY.md)。

## 使用 Coding Agent 开发和调试

在 Codex、Qoder 等工具中打开项目目录，要求 Agent 先阅读 [AGENTS.md](AGENTS.md) 和本指南。提供具体需求、验收标准、SSH 别名以及允许操作的设备和目录；密钥通过本机 SSH 配置使用，不需要粘贴到对话里。

可以使用下面的任务说明作为起点：

```text
阅读 AGENTS.md、README_DEV.md 和相关模块文档。
实现：<具体功能或问题>。
验收：<期望行为和需要覆盖的场景>。

开发机已通过 USBNetwork Hack 配置 SSH 密钥登录，设备别名为 kindle-dev。
先完成本地修改和适当验证，再把本次构建安装到这台测试设备并真机调试。
保留设备原配置、已有图书和下载/同步记录。
测试使用独立目录或仓库公开夹具，结束后清理测试数据并恢复原服务状态。
说明实际验证结果，区分电脑预览、自动测试与真机验证。
不要把真实配置、密钥、会话或设备截图提交到仓库。
```

以下是开发测试构建的迭代与设备调试流程；首次安装请按 [README](README.md#installation) 操作。这些备份、替换和回退措施不表示存在更早的公开发行版本。

推荐的工作流程：

1. **确认基线**：查看 Git 工作区及已有改动，记录设备型号、固件、服务状态和本次涉及的数据，备份配置与状态。
2. **本地开发**：修改相关模块，为行为或兼容性变化增加有意义的回归测试。界面先用合成数据预览。
3. **检查与构建**：运行 `sh scripts/check.sh` 和 `sh scripts/build.sh`，确认安装的是当前 ARM 产物。
4. **设备安装**：先停止本项目的三个服务，从包内更新 `extensions/inkpika/`。保留设备已有 `config.json`、`transfer.json` 和状态目录，核对安装后文件摘要。
5. **真机验证**：按场景操作 KUAL / Mesquite；每次先看当前屏幕，再执行一次操作并等待结果，避免按旧坐标连续点击。传输测试同时核对文件内容或哈希。
6. **收尾**：清理自己创建的测试文件，核对原配置、记录和图书，恢复原来的服务、网络及唤醒设置，记录验证范围和仍未覆盖的场景。

普通客户端修改不需要替换 KUAL JAR 或重启系统界面。只有明确修改导航补丁时，才按 [KUAL_NAVIGATION.md](docs/KUAL_NAVIGATION.md) 的适用版本、备份和回退流程操作。

电脑浏览器、自动测试和真实 PW2 验证各有不同用途。移动尺寸浏览器预览不能描述为实体手机测试，本地样例 Store 也不能描述为真实 OSS / QingStor 联调。

## 手工配置与诊断

日常设置优先使用 **配置设置 [web]**。开发或排查时，也可将以下示例复制到设备扩展目录并命名为 `config.json`：

- [阿里云 OSS 示例](extensions/inkpika/config.aliyun.example.json)
- [QingStor 示例](extensions/inkpika/config.qingstor.example.json)
- [S3 示例](extensions/inkpika/config.s3.example.json)，通过 `s3_preset` 选择 AWS / R2 / B2 / Spaces / TOS / IBM / GCS / COS / custom
- [无线传书示例](extensions/inkpika/transfer.example.json)，使用时命名为 `transfer.json`

手工编辑云端配置前停止云端书架，使用 UTF-8 JSON，修改后重新启动。JSON 不支持注释或结尾多余的逗号；不要把示例占位符当作有效账号。

| 配置字段 | 约定 |
| --- | --- |
| `provider` | 新配置明确使用 `aliyun_oss`、`qingstor`、`s3` 或 `azure_blob` |
| `bucket` / `region` | 实际 Bucket 与地域；OSS 地域不带 `oss-` 前缀 |
| `endpoint` | OSS / QingStor / AWS / B2 / Spaces / TOS / IBM / GCS / COS / Azure 可留空；R2 / 自定义 S3 必填不含 Bucket 或路径的 HTTPS 服务地址，可含端口 |
| `s3_preset` | `aws`、`r2`、`b2`、`spaces`、`tos`、`ibm`、`gcs`、`cos` 或 `custom`；仅用于 S3，默认 custom |
| `s3_addressing` | `path` 或 `virtual`；NAS / R2 / B2 / IBM / GCS 默认 path，AWS / Spaces 默认 virtual；TOS / COS 必须 virtual；带点 Bucket 用 path |
| `security_token` | 阿里云 / 支持临时凭据的 S3 平台使用；IBM / GCS / Azure 不接受；不自动续期 |
| `socks5_proxy` | 可选 `socks5://主机:端口` / `socks5h://用户名:密码@主机:端口`，目标 DNS 在代理解析；留空直连，不读取环境代理 |
| `ca_bundle` | 可选附加 PEM CA 证书绝对路径，普通文件不超过 1 MiB；用于 NAS 私有 CA，仍校验主机名 |
| `prefix` | 云端书架的目录范围；KUAL 同步要求留空或为 `sync/` |
| `download_dir` | 云端默认 `/mnt/us/documents/InkPika`；配置网页只允许 documents 内的路径 |
| `max_download_mb` | 云端单本上限，默认 512 MiB |
| `ebook_formats` | 扩展名数组，例如 `["azw3", "pdf"]`；省略或空数组使用内置格式 |
| `listen` | 云端 API 仅允许回环地址，默认 `127.0.0.1:8787` |
| `state_dir` | 可选绝对路径，默认按来源隔离；与图书目录须位于同一文件系统 |

Azure 示例为 [config.azure.example.json](extensions/inkpika/config.azure.example.json)，使用账户名、Base64 Account Key 和容器，region 为空；中国区等环境须填写相应 Blob Endpoint。四个 S3 预设及 Azure 的凭据、Endpoint 与限制见 [README](README.md#connect-to-object-storage)。

QingStor 的 `zone` 仅作兼容字段；新配置使用 `region`。OSS 使用原生 V4 签名和 ListObjectsV2；当前不支持 CNAME、Access Point 或 CloudBox。归档对象需先在云平台解冻，临时凭据过期后需手动更新。

无线传书默认 LAN 端口为 `8789`、Kindle 引导页端口为 `8790`；配置设置分别为 `8791`、`8792`。无线传书的保存目录、图书扫描目录、大小上限和端口可在 `transfer.json` 中调整，修改后重启对应服务。云端账号未就绪时仍可使用无线传书。

通过开发机发起只读云端连接检查：

```sh
ssh kindle-dev '/mnt/us/extensions/inkpika/bin/inkpika -config /mnt/us/extensions/inkpika/config.json -check'
```

启动失败时先查看对应日志：

| 服务 | 启动日志 | 运行日志 |
| --- | --- | --- |
| 云端书架 | 扩展目录的 `startup.log` | 当前 `state_dir/server.log` |
| 无线传书 | `transfer-startup.log` | `transfer-state/server.log` |
| 配置设置 | `settings-startup.log` | `settings-state/server.log` |

完整错误排查前先脱敏；不要直接输出配置全文或带令牌的启动地址。私有调试材料放在忽略目录 `local/`、`output/`，不要将 SSH 私钥、云端凭据、配对码、二维码或个人图书提交到 Git。

## 代码导航

| 路径 | 职责 |
| --- | --- |
| `cmd/inkpika` | 设备 CLI、进程与服务生命周期 |
| `cmd/preview` | 合成数据的电脑端预览 |
| `internal/providers`、`storage`、`cloudhttp` | 存储适配、统一只读接口、HTTPS 策略 |
| `internal/config`、`bookformat` | 配置校验与共享格式规则 |
| `internal/download`、`syncer`、`statefile` | 下载续传、同步校验与状态持久化 |
| `internal/web` | 云端书架的本机 API 与 ES5 界面 |
| `internal/transfer`、`settings` | 无线传书、配对连接页和配置编辑 |
| `extensions/inkpika` | KUAL 菜单、启动脚本与窗口引导 |
| `scripts`、`.github/workflows` | 校验、打包及 CI / 网站发布工作流 |

设备扩展路径和二进制统一使用 `inkpika`，界面展示名使用 InkPika。不要因品牌或版本调整改变已有图书命名、来源标识或状态文件。

新 S3 服务优先复用 `internal/providers/s3` 并新增预设 / 协议夹具；异构存储平台应实现 `storage.Store`，在 `providers.New` 注册并补充配置与协议测试。架构、API、签名、续传和同步规则见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)，持久化数据约定见 [docs/COMPATIBILITY.md](docs/COMPATIBILITY.md)。

## 内存 S3 集成测试

`sh scripts/check.sh` 自动运行 `internal/providers/s3/memory_test.go`，使用第三方 [gofakes3](https://github.com/johannesboyne/gofakes3) 的 `backend/s3mem`，固定版本 `v0.0.0-20250402064820-d479899d8cbe`，兼容 Go 1.23.12。可单独运行：

```sh
GOTOOLCHAIN=go1.23.12 go test -race ./internal/providers/s3 -run TestMemory -v
```

测试启动仅监听本机的临时 HTTPS S3 服务，通过附加 CA 信任其证书；真实的配置校验、provider 工厂、S3 HTTP 客户端、下载队列与同步引擎参与测试。覆盖 AWS / R2 / B2 / Spaces / custom 五种预设的读取、中文和特殊字符、HEAD、Range、404/416、递归目录与多页列举、5,000,000 字节文件中断续传及逐字节比对、版本变化、校验后更新和远端删除时保留本地文件。写入与删除仅用于准备内存测试数据，设备客户端仍只读。

测试服务选型限制：[IBM/s3mem-go](https://github.com/IBM/s3mem-go) 替换 AWS SDK 接口，不提供当前需要的 HTTP 服务，且缺少 ListObjectsV2、HEAD 和分页；[grafana/s3-mock](https://github.com/grafana/s3-mock) 当前要求 Go 1.24.2，检查的实现缺少 Range / delimiter，HEAD 长度固定为 0。因此采用 gofakes3，不提高设备工具链版本。mock 与其依赖仅供测试，不进入设备二进制或安装包。

固定版本的 gofakes3 正确生成 Range 内容与头，但漏写 HTTP 206；测试服务外层仅补正这一状态码，对应上游 [PR #121](https://github.com/johannesboyne/gofakes3/pull/121) 的修复，不放宽客户端校验。它不验证 SigV4、IAM 或 GET 的 If-Match，也不完整实现 `encoding-type=url`；独立协议夹具继续验证签名、URL 编码、不透明分页标记、条件请求、重定向和错误响应。内存服务测试不证明各云厂商、NAS 版本或 Kindle 真机兼容，OSS / QingStor 继续使用各自原生协议夹具。

## 贡献与验证记录

提交前按 [CONTRIBUTING.md](CONTRIBUTING.md) 描述问题、修改后的行为和验证结果。真实设备测试写入 [docs/DEVICE_VALIDATION.md](docs/DEVICE_VALIDATION.md)，注明日期、构建、设备环境、数据保护和实际覆盖范围。

项目代码与原创测试书采用 [MIT](LICENSE)；复用的二维码库、证书包等材料保留 [THIRD_PARTY.md](THIRD_PARTY.md) 所列许可证。公开测试书位于 [`testdata/books/`](testdata/books/README.md)。

五家平台的本地协议回归位于 `internal/providers/s3/presets_test.go` 与 `internal/providers/azureblob/client_test.go`，涵盖预设地址、V1/V2 分页、特殊对象键、签名、条件读取与错误处理；Azure 还覆盖拒绝重定向和响应关闭。配置保存、密钥保留与来源隔离、厂商元数据由相应包回归测试覆盖。五家平台尚无真实账号或 Kindle 联调证据。

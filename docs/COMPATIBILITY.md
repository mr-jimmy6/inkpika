# InkPika 配置与状态约定

本文记录 InkPika 0.0.1.dev 的当前数据格式和后续维护要求。设备兼容性另见 [DEVICE_COMPATIBILITY.md](DEVICE_COMPATIBILITY.md)，安装见 [README](../README.md)。

## 标识与路径

| 用途 | 标识 |
| --- | --- |
| 展示名，所有语言 | `InkPika` |
| 程序 / 扩展 | `inkpika` / `/mnt/us/extensions/inkpika` |
| 默认图书目录 | `/mnt/us/documents/InkPika` |
| 默认无线传入目录 | `/mnt/us/documents/InkPika/Wireless` |
| WAF 应用 | `com.inkpika.app` |
| 本机 API 认证头 | `X-InkPika-Token` |
| Go 模块 / 真实仓库 | `github.com/mr-jimmy6/inkpika` |

建议仓库名为 `inkpika`，实际远端和依赖它的有效链接暂不调整。

## 云端配置

`provider` 必须明确选择 `aliyun_oss`、`qingstor`、`s3` 或 `azure_blob`；未知或省略值失败。QingStor 的 `zone` 可作为 `region` 的供应商配置字段，同填时必须一致。凭据只用于所选平台；不改变供应商名称、签名或协议语义。

默认图书目录为 `InkPika`。省略 `state_dir` 时，以配置文件旁的 `state/<provider>-<source-hash>` 隔离来源。来源标识由 Endpoint / Bucket 组成，除 QingStor 外还包含 provider / region。密钥、语言、代理、附加 CA、S3 预设与寻址方式不进入来源标识；轮换凭据不改变任务和图书身份。

## 本地数据

- `jobs.json` 保存任务 ID、对象键、目标文件名和传输状态；重启后未完成任务等待手动继续。
- `sync.json` 保存来源、远端元数据、SHA-256 receipts 和快照；缺少完整远端集合时要求重新扫描。
- 下载记录的 `transfer_complete` 可省略；状态为 completed 时按已完成传输解释。页面另行检查本地文件是否存在、为普通文件且大小一致。
- 状态与图书必须在同一文件系统。`statefile.Write` 使用同目录临时文件和原子替换，断电耐久性仍受文件系统限制。
- 浏览器下载不能覆盖图书；同步只替换经身份、完整性和远端版本复核的已登记副本。远端删除不删除本地图书。
- 会话令牌、配对 Cookie、PID 和锁属于运行时数据，不放入发行包；服务重启后重新打开页面或配对。

后续版本若调整来源标识、任务 ID、文件名或持久化字段，应明确数据转换和回退策略，并在获准运行测试的工作中补充有效回归覆盖。安装器保留本地配置、状态和图书，其程序文件备份不替代用户数据备份。

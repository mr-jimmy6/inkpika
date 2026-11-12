# 对象存储配置

[← 使用说明](../README_ZH.md) · [English](OBJECT_STORAGE.md)

打开 **KUAL → InkPika → 配置设置 [web]**，可在 Kindle 查看已保存的配置、脱敏密钥、构建信息并进行连接测试。选择 **手机 / 电脑** 查看配对地址和二维码，再在对应浏览器中填写配置。

“存储平台”可直接选择阿里云 OSS、QingStor、Amazon S3、火山引擎 TOS、Microsoft Azure Blob Storage、IBM Cloud Object Storage、Google Cloud Storage、腾讯云 COS、Cloudflare R2、Backblaze B2、DigitalOcean Spaces 或自定义 S3 / NAS。页面会回填当前的平台、Bucket、地域、Endpoint、云端目录、保存目录、大小上限、允许格式和附加 CA 路径；代理只显示是否已设置。已有密钥最多显示首尾各两个字符；密钥输入框留空保留原值，填写新值才替换。保存时先校验配置，云端书架正在运行时会安全重启以应用修改，未完成的下载可手动继续。

## 连接对象存储

当前一次连接一个 Bucket。请使用自己的存储账号，并按 Bucket 的实际地域填写：

| 平台 | 地域示例 | Endpoint | 所需权限 |
| --- | --- | --- | --- |
| 阿里云 OSS | `cn-beijing`、`cn-hangzhou`，不带 `oss-` 前缀 | 通常留空自动生成；手工填写区域级公网 HTTPS 地址，不含 Bucket | 列举 Bucket，读取其中的对象 |
| QingStor | `pek3b` | 通常留空；自动生成 `https://区域.qingstor.com` | 列举 Bucket，读取其中的对象 |
| Amazon S3 | `us-east-1` 等 Bucket 实际地域 | 选择 Amazon S3；可留空生成 `https://s3.地域.amazonaws.com`（中国区使用 `.com.cn`） | `s3:ListBucket`、`s3:GetObject` |
| Cloudflare R2 | `auto` | 选择 Cloudflare R2；复制控制台的 HTTPS S3 API Endpoint，不含 Bucket | Bucket 范围内的对象列举、读取权限 |
| Backblaze B2 | `us-west-004` 等实际地域 | 选择 Backblaze B2；可留空生成 `https://s3.地域.backblazeb2.com` | 应用密钥的 `listFiles`、`readFiles` 权限 |
| DigitalOcean Spaces | `nyc3` 等实际地域 | 选择 DigitalOcean Spaces；可留空生成 `https://地域.digitaloceanspaces.com` | 对应 Space 的只读访问密钥 |
| 火山引擎 TOS | `cn-beijing` 等实际地域 | 留空生成 `https://tos-s3-地域.volces.com`；固定子域名式 | AccessKey ID / Secret；列举与读取权限 |
| Microsoft Azure Blob Storage | 不需要 | 留空生成 `https://账户名.blob.core.windows.net`；中国区等环境填写实际 Blob Endpoint，不含容器 | 账户名称 + Base64 Account Key；账户须允许 Shared Key |
| IBM Cloud Object Storage | 如 `us-south`；跨地域实例按控制台使用 `us` / `eu` / `ap` | 留空生成 `https://s3.地域.cloud-object-storage.appdomain.cloud` | 服务凭据中的 HMAC 密钥，授予目标 Bucket 读取权限；不是 IAM API Key |
| Google Cloud Storage | 默认 `auto` | 留空为 `https://storage.googleapis.com`，默认路径式 | 互操作性 HMAC 访问 ID / Secret；如 Storage Object Viewer 的列举、读取权限；不是服务账号 JSON |
| 腾讯云 COS | 如 `ap-guangzhou` | 留空生成 `https://cos.地域.myqcloud.com`；固定子域名式 | SecretId / SecretKey；列举、读取权限；Bucket 包含 `-APPID` 后缀 |
| S3 兼容存储 / NAS | 服务端配置的 region；留空为 `us-east-1` | 选择 自定义 S3 / NAS；必填 HTTPS 地址，可带端口，不含 Bucket 或路径 | 列举指定 Bucket、读取对象 |

火山、IBM、谷歌和腾讯通过 S3 兼容接口读取；火山 / 谷歌 / 腾讯使用标记分页，IBM 使用 ListObjectsV2。配置中的 `s3_preset` 分别为 `tos`、`ibm`、`gcs`、`cos`。Google Bucket 当前接受 3–63 位 DNS 风格名称，不支持下划线或更长的名称。IBM / Google 使用 HMAC 长期密钥，不填写临时安全令牌。

微软使用 `provider=azure_blob` 和原生 Blob REST 接口，不是 S3 网关。配置页将 Bucket 显示为“容器”，AccessKey ID 显示为“存储账户名称”，Secret 显示为“账户密钥”；不需要地域，不支持 SAS 或 Entra 登录。客户端仅发起列举、HEAD 和 GET；**Azure 账户密钥本身权限较大，并非只读凭据**，请仅在可信设备与局域网配置页面使用并妥善保管。手工配置见 [Azure 示例](../extensions/inkpika/config.azure.example.json)。

协议依据：[TOS S3 兼容说明](https://www.volcengine.com/docs/6349/147050)、[IBM S3 API](https://cloud.ibm.com/docs/cloud-object-storage?topic=cloud-object-storage-compatibility-api)、[Google 互操作性](https://cloud.google.com/storage/docs/interoperability)、[腾讯 COS S3 配置](https://cloud.tencent.com/document/product/436/41284)、[Azure Shared Key](https://learn.microsoft.com/en-us/rest/api/storageservices/authorize-with-shared-key)。本地协议测试不能替代真实账号验证。

阿里云推荐使用 RAM 用户的受限 AccessKey；使用 STS 时同时填写临时密钥与 SecurityToken，过期后需手动更新。无需给客户端授予云端上传或删除权限。AWS / 自定义 S3 的临时凭据可填写 `security_token`；不自动续期。切换存储平台时须重新填写对应密钥。

<details>
<summary>阿里云 OSS 只读授权示例</summary>

将两处 `your-bucket` 替换为实际 Bucket 名称，将策略授权给 AccessKey 所属的 RAM 用户，并确认策略版本已生效。列举权限对应 Bucket 本身，读取权限对应带 `/*` 的对象资源。

```json
{
  "Version": "1",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": ["oss:ListObjects"],
      "Resource": ["acs:oss:*:*:your-bucket"]
    },
    {
      "Effect": "Allow",
      "Action": ["oss:GetObject"],
      "Resource": ["acs:oss:*:*:your-bucket/*"]
    }
  ]
}
```

若能列出文件却无法下载，检查读取规则末尾的 `/*`，以及授权范围是否覆盖目标 Bucket。

</details>

保存后可进入 **配置设置 [web] → 配置信息 → 开始测试**，检查 HTTPS 连接、目录列举和文件抽样读取，查看具体失败原因。空目录只能确认列举成功，不能据此判断文件读取权限。

## 通过 S3 兼容协议连接 NAS

S3 API 是对象存储中广泛采用的事实标准。选择“自定义 S3 / NAS”，即可将 NAS 上提供 S3 接口的书库接入现有书架、下载队列和 KUAL `sync/` 同步；不需要在 Kindle 上安装额外的存储客户端。以下是可供部署或接入的常见方案，**列出软件不代表本项目已验证其具体版本**：

| NAS / 自建软件 | 选型说明 |
| --- | --- |
| [Garage](https://garagehq.deuxfleurs.fr/) | 开源 S3 对象存储；适合已有 NAS / Linux 主机，按设备架构和资源要求部署 |
| [SeaweedFS](https://github.com/seaweedfs/seaweedfs) | 提供 S3 接口，支持单机部署；使用其对象存储端点 |
| [MinIO / AIStor](https://www.min.io/pricing) | MinIO 生态中的对象存储方案；当前 AIStor Free 提供单节点免费版，采用商业许可；旧 MinIO 开源版已停止维护，不建议新部署使用旧版 |

通常选择**路径式**访问（`https://主机:端口/Bucket/对象`），AWS / Spaces 默认选择子域名式；带点的 Bucket 应选择路径式以避免通配符证书不匹配。Endpoint 只填服务地址，例如 `https://nas.example.com:9443`。Kindle 必须能访问这个地址；S3 协议本身不提供外网穿透。

书籍需要通过 NAS 软件的管理工具或 S3 客户端上传到 Bucket；普通共享文件夹不会自动成为 S3 书库。NAS 必须提供 HTTPS，可由反向代理终止 TLS，但不能改写对象路径；使用公开受信任证书，或先将私有 CA 的 PEM 文件复制到 Kindle，再填写“附加 CA 证书路径”。仍会校验证书和主机名，不提供跳过 TLS 校验的选项。仅支持 `ListObjectsV2`、HEAD/GET、Range 与条件读取等只读子集；不包含 WebDAV。

## 配置 SOCKS5 代理

在配置设置填写 **SOCKS5 代理**，支持 `socks5://主机:端口`、`socks5h://主机:端口` 和含用户名密码的 URL。例如 `socks5h://reader:password@192.168.1.10:1080`，密码中的 `@`、`:` 等特殊字符须 URL 编码。这个地址应指向 Kindle 可访问的代理服务；`127.0.0.1` 表示 Kindle 本身。

代理适用于全部对象存储平台的浏览、下载、同步与连接测试；两种 URL 写法均由代理解析目标域名，代理不可用时直接报错，不自动改为直连。已有代理不回显，留空保留，勾选“清除代理，恢复直连”才移除。无线传书、配置设置和回环 API 不经过此代理，也不读取系统 `HTTP_PROXY` / `HTTPS_PROXY` 环境变量。SOCKS5 代理服务需自行提供；代理认证链路本身不加密，远端对象存储流量仍受 HTTPS 保护。

## 公有云一年费用预估

**此为 Codex 预估，仅供参考，不是服务商报价或费用承诺。** 依据下表注明的已核实单价与预算假设，假设单价一年不变；实际价格、可用地域、免费额度、账单取整和最低出账规则以服务商与账号账单为准。

估算从空书库开始，每月月初新增一本 **5 MB（5,000,000 字节）** 的书，保留全年，每本下载一次：一年 12 本，年末约 **60 MB**，累计约 **0.39 GB·月**，下载约 **0.06 GB**。另外按每月 **100 次 LIST、200 次 GET/HEAD（含图书下载请求）**，全年 **12 次 PUT** 计请求费用；上传由电脑或外部工具完成，Kindle 客户端仍只读。按十进制 GB 近似计算，忽略 GB/GiB 与月份长度的小幅差异。

| 公有云 / 示例地域 | 采用的计费规则 | 第一年估算 | 人民币参考 |
| --- | --- | --- | --- |
| [阿里云 OSS](https://help.aliyun.com/zh/oss/product-overview/billing-overview) / 中国内地，标准 LRS | 存储 ¥0.12/GB·月；公网出站按 ¥0.50/GB；PUT/LIST、GET/HEAD 均按 ¥0.01/万次 | **约 ¥0.08** | 约 ¥0.08 |
| [QingStor](https://docsv4.qingcloud.com/user_guide/storage/object_storage/billing/price/) / 国内标准存储 | 前 10 GB 存储、每月前 1 GB 下载免费；API ¥0.01/万次 | **约 ¥0.004** | 不到 ¥0.01，用量价值不等于实际扣款 |
| [Amazon S3 Standard](https://aws.amazon.com/s3/pricing/) / 美国 `us-east-1` | 存储 $0.023/GB·月；PUT/LIST $0.005/千次，GET/HEAD $0.0004/千次；本用量落在每月 100 GB 免费出站额度内 | **约 $0.016** | 约 ¥0.12 |
| [火山 TOS / 北京，标准单 AZ](https://www.volcengine.com/pricing?product=TOS&tab=1) | 官网单价：¥0.099/GiB·月（本例约 0.363 GiB·月）；公网出站 ¥0.50/GB；请求 ¥0.01/万次；不抵扣赠金 | **约 ¥0.07** | 约 ¥0.07 |
| [Microsoft Azure Blob / East US，Hot LRS](https://azure.microsoft.com/en-us/pricing/details/storage/blobs/) | Retail Prices API：存储 $0.0208/GB·月，写入/LIST $0.05/万次、读取 $0.004/万次；出站暂按 $0.087/GB 假设，不抵扣免费额度 | **预算约 $0.020** | 约 ¥0.15 |
| [IBM Cloud Object Storage / us-south，Standard](https://www.ibm.com/products/cloud-object-storage/pricing) | 待核价假设：存储 $0.023/GB·月；写入/LIST $0.005/千次、读取 $0.0004/千次；出站 $0.09/GB；不计 Lite / 免费额度 | **试算约 $0.021** | 约 ¥0.15 |
| [Google Cloud Storage / us-central1，Standard](https://cloud.google.com/storage/pricing) | 待核价假设：存储 $0.020/GB·月；A 类 $0.005/千次、B 类 $0.0004/千次；出站 $0.12/GB；不抵扣按地域、目的地限制的免费额度 | **试算约 $0.022** | 约 ¥0.16 |
| [腾讯云 COS / 广州，标准单 AZ](https://buy.cloud.tencent.com/price/cos) | 待核价假设：存储 ¥0.118/GB·月；公网出站 ¥0.50/GB；请求 ¥0.01/万次；不计新用户试用或资源包 | **试算约 ¥0.08** | 约 ¥0.08 |
| [Cloudflare R2 Standard](https://developers.cloudflare.com/r2/pricing/) | 每月 10 GB·月、100 万次 A 类和 1,000 万次 B 类操作免费；出站免费 | **$0** | ¥0 |
| [Backblaze B2](https://www.backblaze.com/cloud-storage/pricing) | 前 10 GB 免费；常规 API 免费；每月平均存储量 3 倍以内出站免费 | **$0** | ¥0 |
| [DigitalOcean Spaces](https://www.digitalocean.com/pricing/spaces-object-storage) | 最低 $5/月，含 250 GiB 存储及 1 TiB 出站 | **$60** | 约 ¥432 |

核价状态（2026-11-09）：火山上述三项已查阅官方定价页；Azure 存储与请求单价已查阅 [Retail Prices API](https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices)，出站费率仍是假设。IBM、Google、腾讯未能在本次访问中完整核实按量单价，表内明确列为**预算假设**，不是已确认现价；应按实际地域、下载目的地及账号报价替换后重算。五家均按不抵扣免费额度或赠金试算，符合免费条件时实际费用可能更低。计算式为 `存储单价 × 0.39 + 出站单价 × 0.06 + 写入/LIST 单价 × 1212 + 读取单价 × 2400`（请求单价先换为每次，火山存储换为 GiB）；这些极小数值是用量价值，不代表最终扣款。

人民币换算仅采用 **$1 ≈ ¥7.2** 的演示汇率，不代表实时汇率。对使用免费额度的行，假设账号额度未被其他业务占用；不计新用户限时赠金、税费、代理订阅、域名、NAS 硬件、电费或其他增值服务，也未计重试、重复下载和额外控制台请求。OSS 流量与请求示例单价参考[官方计费案例](https://help.aliyun.com/zh/oss/product-overview/billing-examples)。AWS 免费出站为[账号共享额度](https://aws.amazon.com/blogs/aws/aws-free-tier-data-transfer-expansion-100-gb-from-regions-and-1-tb-from-amazon-cloudfront-per-month/)，不适用于 AWS 中国区等例外范围。免费额度内的 $0 不代表永久免费或无需绑定付款方式。对这个用量，优先考虑 R2 / B2；已有 OSS、QingStor 或 AWS 账号也足够便宜，Spaces 更适合已购买该套餐的用户。

# Object storage setup

[← User guide](../README.md) · [简体中文](OBJECT_STORAGE_ZH.md)

Open **KUAL → InkPika → Settings [web]** to view saved settings, masked credentials, build details and the connection test on Kindle. Select **Phone / computer** for the pairing URL and QR code, then edit settings in that browser.

The storage platform selector offers Alibaba Cloud OSS, QingStor, Amazon S3, Volcengine TOS, Microsoft Azure Blob Storage, IBM Cloud Object Storage, Google Cloud Storage, Tencent COS, Cloudflare R2, Backblaze B2, DigitalOcean Spaces, and custom S3 / NAS directly. The form is prefilled with the current platform, addressing style, bucket, region, endpoint, cloud prefix, download directory, size limit, allowed formats, and additional CA path. It only reports whether a proxy is configured. Stored credentials show at most their first and last two characters. Leave a credential input blank to retain its value; enter a new value to replace it. Configuration is validated before saving. If Cloud Bookshelf is running, it is safely restarted to apply the changes; unfinished downloads can be resumed manually.

## Connect to object storage

One bucket can be connected at a time. Use your own storage account and the bucket's actual region:

| Provider | Region examples | Endpoint | Required permissions |
| --- | --- | --- | --- |
| Alibaba Cloud OSS | `cn-beijing`, `cn-hangzhou`, without the `oss-` prefix | Usually leave blank for automatic selection; a custom value must be a public regional HTTPS endpoint without the bucket name | List the bucket and read its objects |
| QingStor | `pek3b` | Usually leave blank; defaults to `https://<region>.qingstor.com` | List the bucket and read its objects |
| Amazon S3 | Actual bucket region, e.g. `us-east-1` | Select Amazon S3; leave blank for `https://s3.<region>.amazonaws.com` (`.com.cn` in China) | `s3:ListBucket`, `s3:GetObject` |
| Cloudflare R2 | `auto` | Select Cloudflare R2; copy the HTTPS S3 API endpoint from its dashboard, without the bucket | Bucket-scoped object listing and read access |
| Backblaze B2 | Actual region, e.g. `us-west-004` | Select Backblaze B2; leave blank for `https://s3.<region>.backblazeb2.com` | Application key with `listFiles` and `readFiles` |
| DigitalOcean Spaces | Actual region, e.g. `nyc3` | Select DigitalOcean Spaces; leave blank for `https://<region>.digitaloceanspaces.com` | Read-only access key for that Space |
| Volcengine TOS | Actual region, e.g. `cn-beijing` | Defaults to `https://tos-s3-<region>.volces.com`; virtual-host only | AccessKey ID / Secret with list and read permissions |
| Microsoft Azure Blob Storage | Not needed | Defaults to `https://<account>.blob.core.windows.net`; specify the actual Blob endpoint for sovereign clouds, without the container | Account name + Base64 Account Key; Shared Key access must be enabled |
| IBM Cloud Object Storage | E.g. `us-south`; use `us` / `eu` / `ap` for cross-region instances as shown in the console | Defaults to `https://s3.<region>.cloud-object-storage.appdomain.cloud` | HMAC service credentials with access to list/read the bucket; not an IAM API key |
| Google Cloud Storage | Defaults to `auto` | Defaults to `https://storage.googleapis.com`, path-style | Interoperability HMAC access ID / secret with list/read access, e.g. Storage Object Viewer; not service-account JSON |
| Tencent COS | E.g. `ap-guangzhou` | Defaults to `https://cos.<region>.myqcloud.com`; virtual-host only | SecretId / SecretKey with list/read access; bucket must include its `-APPID` suffix |
| S3-compatible storage / NAS | Server's signing region; defaults to `us-east-1` | Select custom S3 / NAS; HTTPS endpoint required, optional port, no bucket or path | List the bucket and read objects |

TOS, IBM, Google and Tencent use the S3-compatible API with `s3_preset` values `tos`, `ibm`, `gcs` and `cos`. TOS / Google / Tencent use marker-based listing; IBM uses ListObjectsV2. Google buckets currently require DNS-style names of 3–63 characters; underscores and longer names are not supported. IBM / Google require HMAC keys without a temporary security token.

Microsoft uses `provider=azure_blob` with the native Blob REST API. The form shows Container, Storage account name and Account Key instead of Bucket and S3 credentials; no region is needed. SAS and Entra authentication are not supported. The client only lists and reads objects, but **the Azure account key itself has broad permissions and is not a read-only credential**. Use trusted devices and LANs, and protect the key. See the [Azure configuration example](../extensions/inkpika/config.azure.example.json).

Protocol references: [TOS compatibility](https://www.volcengine.com/docs/6349/147050), [IBM S3 API](https://cloud.ibm.com/docs/cloud-object-storage?topic=cloud-object-storage-compatibility-api), [Google interoperability](https://cloud.google.com/storage/docs/interoperability), [Tencent S3 setup](https://cloud.tencent.com/document/product/436/41284), and [Azure Shared Key](https://learn.microsoft.com/en-us/rest/api/storageservices/authorize-with-shared-key). Local protocol tests do not establish live-account compatibility.


For Alibaba Cloud, use an AccessKey belonging to a RAM user with limited permissions. For STS, enter the temporary credentials and SecurityToken together; update them manually when they expire. Cloud upload and deletion permissions are not needed.

<details>
<summary>Read-only Alibaba Cloud OSS policy example</summary>

Replace both occurrences of `your-bucket` with your bucket name. Grant the policy to the RAM user that owns the AccessKey, and make sure the policy version is active. Listing permission applies to the bucket itself; reading permission applies to object resources ending in `/*`.

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

If listing works but downloading fails, check the trailing `/*` in the read permission and confirm that the authorization covers the target bucket.

</details>

After saving, open **Settings [web] → Configuration → Start test**. This checks the HTTPS connection, directory listing, and a sample object read, reporting specific failure reasons. An empty directory can confirm listing access, but cannot establish permission to read objects.

## Connect a NAS through the S3-compatible API

S3 is a widely adopted object-storage API. Select **custom S3 / NAS** to use the same bookshelf, download queue, and KUAL `sync/` workflow with a NAS that exposes S3. No additional storage client is required on Kindle. Common deployment or integration choices include the following; **listing a product does not mean its versions have been tested with this project**.

| NAS / self-hosted software | Notes |
| --- | --- |
| [Garage](https://garagehq.deuxfleurs.fr/) | Open-source S3 storage for an existing NAS / Linux host; check architecture and resource requirements |
| [SeaweedFS](https://github.com/seaweedfs/seaweedfs) | Offers an S3 interface and single-machine deployment; connect to its object-storage endpoint |
| [MinIO / AIStor](https://www.min.io/pricing) | MinIO's current AIStor Free tier offers a free single-node deployment under a commercial license; the original open-source MinIO edition is no longer maintained and is not recommended for new deployments |

NAS installations usually use **path-style** addressing (`https://host:port/bucket/object`); AWS / Spaces default to virtual-host addressing. Buckets containing dots should use path-style addressing to avoid wildcard certificate mismatches. Enter only the service address, such as `https://nas.example.com:9443`. Kindle must be able to reach it; S3 does not provide NAT traversal.

Upload books into the bucket using the NAS software or an external S3 client; an ordinary shared folder is not automatically an S3 library. HTTPS is required. A reverse proxy may terminate TLS but must preserve object paths. Use a publicly trusted certificate, or copy a private CA's PEM certificate to Kindle and enter its absolute path in **Additional CA certificate path**. Certificate and hostname verification stay enabled. The client implements a read-only subset: `ListObjectsV2`, HEAD/GET, Range, and conditional reads. WebDAV is not included.

Temporary AWS / custom S3 credentials can use `security_token`; tokens do not renew automatically. Changing storage platforms requires the new service's access keys. The configuration page also prefills the selected platform, addressing style, and CA path. It only reports whether a proxy has been configured.

## Configure a SOCKS5 proxy

In configuration management, set **SOCKS5 proxy** to `socks5://host:port`, `socks5h://host:port`, or a URL containing a username and password. For example: `socks5h://reader:password@192.168.1.10:1080`. URL-encode special characters such as `@` and `:` in credentials. The proxy must be reachable from Kindle; `127.0.0.1` refers to Kindle itself.

The setting applies to browsing, downloads, sync, and connection checks for every object-storage provider. Both schemes resolve destination hostnames at the proxy. A proxy failure returns an error without falling back to a direct connection. Stored proxy URLs are never returned to the browser: leave the field blank to preserve one, or check **Clear proxy and connect directly** to remove it. Wireless transfer, configuration management, and loopback APIs do not use this proxy. Ambient `HTTP_PROXY` / `HTTPS_PROXY` variables are ignored. You must provide your own proxy; SOCKS5 authentication itself is not encrypted, while object-storage traffic remains protected by HTTPS.

## Estimated public-cloud cost for one year

**This is a Codex estimate, for reference only—not a provider quote or a promise of charges.** It uses the verified rates and budget assumptions identified below, assuming unchanged rates for a year. Actual regional prices, account allowances, billing rounding, and minimum billing thresholds prevail.

Start with an empty library, add one **5 MB (5,000,000-byte)** book at the beginning of each month, retain all books, and download each once. That is 12 books, approximately **60 MB** at year end, **0.39 GB-months** of storage, and **0.06 GB** downloaded. Allow **100 LIST and 200 GET/HEAD requests each month (including book downloads)** plus **12 PUT uploads per year**. Uploads are performed by a computer or external tool; the Kindle client remains read-only. Decimal GB approximations ignore small GB/GiB and calendar-month differences.

| Public cloud / example region | Rates and allowances used | First-year estimate | Approximate CNY |
| --- | --- | --- | --- |
| [Alibaba Cloud OSS](https://help.aliyun.com/zh/oss/product-overview/billing-overview) / mainland China, Standard LRS | ¥0.12/GB-month; ¥0.50/GB internet egress; ¥0.01/10,000 PUT/LIST or GET/HEAD requests | **About ¥0.08** | About ¥0.08 |
| [QingStor](https://docsv4.qingcloud.com/user_guide/storage/object_storage/billing/price/) / China, Standard | First 10 GB stored and 1 GB downloaded per month free; ¥0.01/10,000 API requests | **About ¥0.004** | Below ¥0.01; usage value may differ from actual billing |
| [Amazon S3 Standard](https://aws.amazon.com/s3/pricing/) / US `us-east-1` | $0.023/GB-month; $0.005/1,000 PUT/LIST and $0.0004/1,000 GET/HEAD; covered by 100 GB/month free egress | **About $0.016** | About ¥0.12 |
| [Volcengine TOS / Beijing, Standard single-AZ](https://www.volcengine.com/pricing?product=TOS&tab=1) | Published rates: ¥0.099/GiB-month (about 0.363 GiB-months here); ¥0.50/GB egress; ¥0.01/10,000 requests; no credits | **About ¥0.07** | About ¥0.07 |
| [Microsoft Azure Blob / East US, Hot LRS](https://azure.microsoft.com/en-us/pricing/details/storage/blobs/) | Retail Prices API: $0.0208/GB-month; $0.05/10,000 writes/LIST, $0.004/10,000 reads; egress assumed at $0.087/GB without free allowances | **Budget: about $0.020** | About ¥0.15 |
| [IBM Cloud Object Storage / us-south, Standard](https://www.ibm.com/products/cloud-object-storage/pricing) | Unverified budget assumptions: $0.023/GB-month; $0.005/1,000 writes/LIST, $0.0004/1,000 reads; $0.09/GB egress; no Lite/free allowances | **Illustration: about $0.021** | About ¥0.15 |
| [Google Cloud Storage / us-central1, Standard](https://cloud.google.com/storage/pricing) | Unverified budget assumptions: $0.020/GB-month; Class A $0.005/1,000, Class B $0.0004/1,000; $0.12/GB egress; no region/destination-dependent free allowances | **Illustration: about $0.022** | About ¥0.16 |
| [Tencent COS / Guangzhou, Standard single-AZ](https://buy.cloud.tencent.com/price/cos) | Unverified budget assumptions: ¥0.118/GB-month; ¥0.50/GB egress; ¥0.01/10,000 requests; no introductory offers or prepaid packages | **Illustration: about ¥0.08** | About ¥0.08 |
| [Cloudflare R2 Standard](https://developers.cloudflare.com/r2/pricing/) | 10 GB-months, 1 million Class A and 10 million Class B operations free monthly; free egress | **$0** | ¥0 |
| [Backblaze B2](https://www.backblaze.com/cloud-storage/pricing) | First 10 GB free; ordinary API calls free; egress up to 3× average monthly storage free | **$0** | ¥0 |
| [DigitalOcean Spaces](https://www.digitalocean.com/pricing/spaces-object-storage) | $5/month minimum, including 250 GiB storage and 1 TiB egress | **$60** | About ¥432 |

Price verification (2026-11-09): all three TOS rates were read from its official pricing page. Azure storage and request rates were read from the [Retail Prices API](https://learn.microsoft.com/en-us/rest/api/cost-management/retail-prices/azure-retail-prices); its egress rate remains an assumption. IBM, Google and Tencent pay-as-you-go rates could not be fully verified during this check, so their rows explicitly use **budget assumptions**, not confirmed current prices. Replace them with the quote for your region, download destination and account. These five estimates deduct no free allowances or credits; eligible accounts may pay less. Formula: `storage rate × 0.39 + egress rate × 0.06 + write/LIST rate × 1212 + read rate × 2400` (convert request rates to per-request prices and TOS storage to GiB). These small usage values do not establish the final amount billed.

The illustrative exchange rate is **$1 ≈ ¥7.2**, not a live rate. For rows that use free allowances, those allowances are assumed available without other account usage. Estimates exclude introductory credits, taxes, proxy subscriptions, domains, NAS hardware, electricity, add-on services, retries, repeat downloads, and extra console requests. OSS request and egress examples come from its [official billing examples](https://help.aliyun.com/zh/oss/product-overview/billing-examples). AWS's [free egress allowance](https://aws.amazon.com/blogs/aws/aws-free-tier-data-transfer-expansion-100-gb-from-regions-and-1-tb-from-amazon-cloudfront-per-month/) is shared across the account and excludes AWS China and other stated exceptions. A $0 estimate does not promise permanent free service or no payment-method requirement. R2 / B2 are good low-cost choices; existing OSS, QingStor, or AWS accounts are also inexpensive at this scale. Spaces makes more sense if you already pay for its package.

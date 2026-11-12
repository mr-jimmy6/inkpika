# AZW3 测试书

三本原创、无 DRM 的短篇测试文件，由同目录 HTML 使用 Calibre 3.48.0 转换为真正的 AZW3 / KF8（MOBI header version 8）。不包含外部版权文本、账户信息或密钥。

| 文件 | 大小 | 内容 |
| --- | ---: | --- |
| `01_中文排版测试.azw3` | 13,697 字节 | 中文标题、段落与标点 |
| `02_English + 空格测试.azw3` | 13,001 字节 | 英文文本，以及中文、空格、加号混合文件名 |
| `03_目录与列表测试.azw3` | 13,240 字节 | 章节目录、列表与阅读顺序 |

首次发布前的真机测试使用阿里云 OSS 北京地域的独立测试 Bucket。`oss-manifest.json` 保存测试对象键、大小和 SHA-256，不包含账户信息；复现时请自行将这些原创文件上传至自己的测试 Bucket 下的 `kindle-client-tests/2026-11-06/`。测试材料按仓库根目录的 MIT 许可证分发。

在 Kindle 上：KUAL → InkPika → 云端书架 [web] → kindle-client-tests → 2026-11-06。下载后回到主页，可按书名查找。书名见 `manifest.json`；客户端下载文件名会附加来源标识短码。

复现转换（电脑需安装 Calibre）：

```sh
ebook-convert '01_中文排版测试.html' '01_中文排版测试.azw3' \
  --title '云端书架测试一：中文排版' --authors '云端书架测试' --language zh-CN
```

不同 Calibre 版本或重新转换可能生成不同字节；现有 manifest 对应本目录已生成的文件。

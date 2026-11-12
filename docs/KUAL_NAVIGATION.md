# KUAL 同步菜单导航修正

普通子菜单导航不设置 `refresh: true`，进入 InkPika、全部书籍、书籍详情、查看状态与后台服务时不会主动重载菜单。

这是 InkPika 0.0.1.dev 的可选导航补丁，尚未构建或真机验证。安装后，InkPika 内的“刷新状态”“开始同步”“停止同步”保留当前位置；显式刷新时也保留路径及页码。后台任务完成后通过“刷新状态”读取结果。

## 原因与范围

原生 KUAL 在重新解析菜单时清空路径，刷新界面时又强制切回第 0 层。仅设置扩展的 `exitmenu: false` 无法保留位置。本补丁修改用户设备上现有 JAR 的 `KualBooklet.class`，增加一个导航辅助类；其余类和资源保持原样。

只在 `InkPika`内保存路径和分页位置。重新加载后按名称及父菜单寻找新条目，忽略已知书籍状态前缀、书籍/状态入口的动态统计，不复用可能变化的 ID；条目消失或截断后的名称有歧义时退到最近的有效父菜单。用户主动返回、进入其他菜单或点击 `/` 仍使用原来的导航行为。

这是单独安装的 KUAL 补丁，不包含在 InkPika 客户端 ZIP 中，也不更改客户端、书籍或应用注册。固件/KUAL 更新可能替换补丁；未知版本会被构建器和安装器拒绝，不能直接强制套用。

## 目标环境与输入要求

- 日版 PW2，固件 5.12.2.2，KUAL `v2.7.32-g74c69f9 (20240104,a)`。
- Kindle 自带 Java `1.8.0_211-ea`；辅助类最低需要 Java 8。
- 原始 JAR SHA-256：`c13fc2bbb2890530a5c6374b879b2049a6136028713c37794d9f40093ea8dae3`。
- 辅助类使用 `com.inkpika.kual` 命名空间；产物摘要以实际生成的 `manifest.json` 为准。
- 本版本未运行补丁构建、导航测试或设备 JVM 验证。

仓库只提供补丁源码，不分发原版或修改后的 KUAL JAR。请从自己的设备取得原始文件，保存在忽略目录 `local/`。已安装补丁的设备应使用原始备份，不能再次修补修改后的 JAR。

## 构建

电脑需要 Python 3、JDK 9+（`javac` 和 `java`），Kindle 不需要另外安装运行时。首次构建从 Maven Central 下载 ASM 9.7.1 到 `local/kual-navigation-build/`，校验固定 SHA-256；ASM 仅在电脑上使用。

从设备复制 `/opt/amazon/ebook/booklet/KUALBooklet.jar` 到 `local/KUALBooklet-original.jar`，然后在仓库根目录执行：

```sh
python3 scripts/kual-navigation/build.py \
  --input local/KUALBooklet-original.jar \
  --java-home /path/to/jdk
```

构建会运行导航回归测试，并逐项比对 JAR 内容，确认只有预期类发生变化。产物在 `dist/kual-navigation/`：补丁 JAR、校验值、测试 JAR、安装脚本和版本清单。构建不会修改输入文件。

## 安装与回退

将 `KUALBooklet.jar`、`KUALBooklet.jar.sha256`、`device-install.sh` 复制到 Kindle 的 `/tmp/kual-navigation/`。以下命令在 Kindle 的 SSH 会话中执行：

```sh
cd /tmp/kual-navigation
sh device-install.sh install KUALBooklet.jar \
  "$(awk '{print $1}' KUALBooklet.jar.sha256)"
```

安装器校验当前版本、备份原文件、校验临时副本后原子替换，并恢复根分区原来的只读状态。原始备份是 `/var/local/inkpika/kual-navigation/original.jar`，已安装补丁的校验值记录在同目录 `installed.sha256`。安装器还在原位置保留带 `.inkpika-<sha256>` 后缀的硬链接，避免旧内核因界面进程仍持有旧 JAR 而无法恢复只读挂载。

清理这些硬链接时，应先停止 framework，删除经过摘要核对且已备份的旧文件，再恢复根分区只读并启动 framework。PW2 的界面进程启动时也可能打开 booklet 目录中的备份 JAR；仅在重启后删除备份仍可能留下被占用的已删除 inode，导致只读重挂载失败。不要在未恢复原挂载状态时结束维护；界面重启后需由用户在设备上解锁。

KUAL 关闭后，系统界面进程仍可能缓存旧类。安装后需重启一次界面进程；这会关闭当前界面，有密码的设备需要在屏幕上重新解锁：

```sh
/sbin/initctl restart framework
```

如需恢复原版，在 Kindle 上执行：

```sh
sh /tmp/kual-navigation/device-install.sh rollback
/sbin/initctl restart framework
```

若 `/tmp` 已被清理，重新复制仓库中的安装脚本即可。回退同样会检查当前 JAR 是否与记录匹配，避免覆盖之后安装的其他 KUAL 版本。不要删除 `/var/local/inkpika/kual-navigation/` 中的原始备份。

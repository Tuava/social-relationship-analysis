# 预编译发布包安装

构建在 GitHub Actions 上执行。运行发布包不需要 Go、Node.js、npm 或本地编译；需要 PostgreSQL 16、Bash 和 OpenSSL。

## 下载与校验

打开 https://github.com/Tuava/social-relationship-analysis/releases，下载平台包及 `SHA256SUMS`：

| 平台 | 文件 |
| --- | --- |
| Linux x86_64 | `sra-linux-amd64.tar.gz` |
| Linux ARM64 | `sra-linux-arm64.tar.gz` |
| macOS Intel | `sra-darwin-amd64.tar.gz` |
| macOS Apple Silicon | `sra-darwin-arm64.tar.gz` |

```bash
# Linux（校验已下载的平台包）
sha256sum --ignore-missing -c SHA256SUMS
# macOS：计算文件哈希后与 SHA256SUMS 中同名文件比较
shasum -a 256 sra-darwin-arm64.tar.gz

tar -xzf sra-darwin-arm64.tar.gz
cd sra-darwin-arm64
./scripts/configure-release.sh
```

将 `backend/.env` 中的 `DATABASE_URL` 设置为你的 PostgreSQL 连接。示例使用本机 PostgreSQL 的操作系统用户名认证；远程数据库请填写用户名、密码、数据库名并启用合适的 TLS 设置。URL 中的特殊字符需要百分号编码。

```bash
# PostgreSQL 已启动且当前数据库用户具有建库权限时：
createdb social_relationship_analysis
./scripts/run-release.sh
```

访问 http://127.0.0.1:8000 。管理员用户名和随机密码在 `backend/.env` 中。启动时会自动执行未应用的迁移；前端和 API 由同一端口提供。脚本前台运行，Ctrl+C 停止。macOS 包为未公证的个人开发构建，企业策略可能限制运行。

## 账号和可选服务

登录后在“数据源配置”添加 NapCat HTTP/WS 地址和 Token。NapCat/QQ 必须单独安装并登录。需要展示 NapCat 返回的本地图片时，将 `NAPCAT_MEDIA_ROOT` 设置为实际媒体目录；远程 NapCat 的路径不能直接作为本机文件访问。

QZone bridge 源码位于仓库的 `integrations/onebot-qzone`，来源和改动见 `THIRD_PARTY_NOTICES.md`。它是独立的可选 Node.js 服务，不随主服务发布包自动启动；详见仓库 `docs/qzone-bridge.md`。

MCP 客户端 command 使用解压目录下的绝对路径 `scripts/mcp-server.sh`。服务使用 stdio，日志走 stderr。首次连接前先启动一次主服务以完成迁移。SQL 工具仅供受信任的本地客户端使用，不能将它当作数据库权限隔离机制。

## 升级、备份与停止

升级前备份 PostgreSQL、`backend/.env` 和 `data/`。停止旧进程，将新版本解压到独立目录，复制原配置并将 `OBJECT_ROOT` 指向原数据目录，然后启动。保留原 `SRA_CONFIG_ENCRYPTION_KEY`，否则无法解密已有凭据。迁移按完整文件名记录，不应改名已发布的迁移。

原始数据和凭据不包含在 GitHub 仓库或发布包中。历史私聊归属错误不会在升级时自动改写整个数据库；重新采集受影响消息会按明确的会话对端更新消息归属。历史关系事件和已有分析结果仍需单独复核。

默认监听本机；远程部署可通过 `HTTP_ADDR` 调整地址，并由部署方设置 HTTPS 反向代理与网络访问范围。

# 部署、升级与拆卸指南

这份文档面向把社会关系分析平台部署到自己的电脑或私有服务器的使用者。平台处理的是高敏感度的社会关系数据：默认只监听本机地址，默认不开放公网，也不会替你绕过登录态、验证码、访问控制或平台限制。

生产运行可直接使用 [GitHub 预编译发布包](release-install.md)，无需本地 Go/Node.js 或编译。下文的 `deploy.sh` 是**源码开发构建流程**，需要本地编译时才使用。

## 1. 组件和默认地址

| 组件 | 作用 | 默认地址 |
| --- | --- | --- |
| PostgreSQL 16 | 事实数据、原始证据和任务状态 | `127.0.0.1:5432` |
| Go API | 认证、采集编排、关系图、证据和管理 API | `127.0.0.1:8000` |
| Vue/Vuetify | 浏览器工作台 | `127.0.0.1:5173` |
| QZone bridge（可选） | 第三方空间接口适配 | `127.0.0.1:5700` |
| MCP（默认 stdio） | 给本机 AI 客户端使用的只读分析入口 | 无监听端口 |
| Vue MCP（开发辅助） | 给 Vue 运行时检查组件树和状态 | `127.0.0.1:8890` |

QZone bridge 是可选组件，不运行 QZone 采集时不需要启动它。它依赖第三方接口和当前登录态，不能保证接口长期稳定。
Vue MCP 仅在设置 `SRA_VUE_MCP=1` 时随 Vite 开发服务器启动，仅用于本地开发/验收；生产静态部署不需要它。后端 MCP 是给 AI 客户端接入的业务接口，两者不是同一个服务。

## 2. 系统要求

- macOS 或 Linux；
- Go（以 `go.mod` 的版本声明为准）；
- Node.js 和 npm；
- PostgreSQL 16（或项目实际兼容的较新版本）；
- `curl`、`openssl`、`psql`；
- 如果使用 QZone：NapCat、已授权的账号会话，以及 `integrations/onebot-qzone` 的依赖。

部署脚本会检查工具是否存在，但不会替你安装 PostgreSQL、NapCat 或系统服务管理器。

## 3. 快速部署（本地开发/研究机）

### 3.1 准备数据库

先启动 PostgreSQL，并创建一个空数据库。数据库用户名、密码和主机由 `DATABASE_URL` 决定：

```bash
createdb social_relationship_analysis
```

如果当前 PostgreSQL 账号有创建数据库权限，也可以让脚本尝试创建：

```bash
./scripts/deploy.sh --create-db
```

这个选项只会创建不存在的数据库，不会删除、清空或覆盖已存在的数据库。

### 3.2 一键准备依赖、迁移前置和构建产物

```bash
./scripts/deploy.sh --show-credentials
```

脚本会：

1. 检查 Go、Node/npm、PostgreSQL、curl 和 openssl；
2. 如果 `backend/.env` 不存在，生成本地配置和随机 JWT/加密密钥；
3. 运行后端测试并构建 `backend/server`；
4. 执行 `npm ci`、前端类型检查和生产构建；
5. 输出后端、前端和 MCP 的启动命令。

它不会启动 QZone，不会启动采集，不会上传数据，也不会覆盖已有的 `backend/.env`。已有配置需要修改时，请先备份，再手动编辑：

```bash
cp backend/.env backend/.env.backup
chmod 600 backend/.env
```

只检查、不创建配置、不构建、不启动：

```bash
./scripts/deploy.sh --check
```

检查整套运行栈（只读，不启动任何组件）：

```bash
./scripts/check-stack.sh
# 将 NapCat/QZone 这两个外部/可选组件也作为失败条件：
./scripts/check-stack.sh --strict
```

诊断会分别显示 PostgreSQL、SRA 后端、前端、MCP、NapCat HTTP/WS 和 QZone
bridge 的状态。NapCat 不属于本仓库，未安装或未启动会标为外部依赖不可用，
不会被误报成 SRA 代码故障。也可以使用 `./scripts/deploy.sh --check-stack`。

只构建其中一侧：

```bash
./scripts/deploy.sh --skip-frontend
./scripts/deploy.sh --skip-backend
```

### 3.3 启动服务

后端会在启动时自动执行尚未应用的 SQL 迁移，并确保管理员账号存在。推荐使用统一入口启动后端和前端：

```bash
./scripts/start-stack.sh
```

它只启动本项目的 Go API 和 Vue 工作台，不会启动 NapCat、不启动采集，也不会触发 QZone 登录。

如果 NapCat 已经安装、登录并监听 HTTP/WS，再显式启动 QZone bridge：

```bash
./scripts/start-stack.sh --with-qzone
```

`--with-qzone` 可能使用已有 NapCat 会话；如果本机没有可复用的授权态，第三方 bridge 的配置可能触发登录流程。因此不要在无人值守环境中盲目加这个参数。

也可以分别启动：

```bash
./scripts/start-server.sh
./scripts/start-frontend.sh
```

检查健康状态：

```bash
curl -fsS http://127.0.0.1:8000/health
```

浏览器打开 `http://127.0.0.1:5173/login`。管理员用户名和密码来自 `backend/.env` 的 `ADMIN_USERNAME`、`ADMIN_PASSWORD`。首次使用 `--show-credentials` 后，请将密码放进密码管理器，不要贴到 issue、聊天记录或 MCP 配置中。

停止本项目后端、前端和 QZone bridge：

```bash
./scripts/stop-stack.sh
```

这些停止脚本均会核对进程命令和工作目录，不会因为端口相同而杀掉其他项目。PostgreSQL 和外部 NapCat 不会被停止。

前端开发服务器用启动它的终端按 `Ctrl-C` 停止。

## 4. MCP 接入

MCP 默认是本地 stdio 服务，推荐让 AI 客户端直接执行项目脚本。先确保 `backend/.env` 已存在，并且 `backend/mcp-server` 已构建（部署脚本会构建后端；必要时也可单独构建 MCP 命令）：

```bash
cd backend
go build -o mcp-server ./cmd/mcp
```

客户端配置示例：

```json
{
  "mcpServers": {
    "social-relationship-analysis": {
      "command": "/绝对路径/social-relationship-analysis/scripts/mcp-server.sh"
    }
  }
}
```

完整工具、资源、提示词和只读边界见 [MCP 接入指南](mcp-guide.md)。不要把 MCP 改成公网 HTTP 服务，也不要把数据库密码、Cookie、NapCat token 或 JWT secret 放进 JSON 配置。

## 5. QZone bridge（可选）

只有在已经配置并授权 NapCat/QZone 账号时才启动：

```bash
./scripts/start-qzone.sh
```

停止：

```bash
./scripts/stop-qzone.sh
```

`start-qzone.sh` 会在需要时安装 bridge 的 Node 依赖，并尝试从本机 NapCat 会话获取 QZone Cookie；Cookie 不应写入仓库。脚本将进程 PID 写入 `data/run/qzone.pid`，停止脚本只清理能够识别为本项目 checkout 的进程，不会因为端口号相同而杀掉别的服务。

## 6. 配置与安全基线

推荐保持以下默认值，除非你已经配置了反向代理和访问控制：

```dotenv
HTTP_ADDR=127.0.0.1:8000
CORS_ORIGIN=http://localhost:5173
```

生产或多人环境至少应做到：

- 使用随机、独立的 `JWT_SECRET` 和 `SRA_CONFIG_ENCRYPTION_KEY`；
- `backend/.env` 权限为 `600`；
- 不提交 `.env`、数据库 dump、日志、媒体、原始消息和导出报告；
- 仅允许有权访问的数据进入采集范围；
- 如果必须远程访问，在反向代理层配置 TLS、认证、IP 白名单和合理的请求体/超时限制；
- 不把 PostgreSQL、8000、5700 或 MCP stdio 暴露到公网；
- 定期备份，并把备份当作同等敏感数据保护。

项目目前没有承诺“匿名 demo 数据自动生成”功能。公开演示请自行准备完全虚构或经过充分授权、不可回溯的 fixture，不要把真实 QQ 号、群号、聊天记录或 QZone 内容放进示例。

## 7. 备份、恢复与升级

### 7.1 备份

备份数据库和对象数据，两者缺一不可：

```bash
pg_dump --dbname="$DATABASE_URL" --format=custom --file /安全位置/sra-$(date +%Y%m%d).dump
tar -C . -czf /安全位置/sra-objects-$(date +%Y%m%d).tar.gz data/objects
```

不要把备份文件放在 Git 工作树中。若需要完整恢复，还应保存脱敏后的 `backend/.env` 结构，但不要把实际 secret 放进公开仓库。

### 7.2 恢复

先停止后端，再在确认目标数据库无误后恢复：

```bash
./scripts/stop-server.sh
pg_restore --clean --if-exists --dbname "$DATABASE_URL" /安全位置/sra-YYYYMMDD.dump
tar -C . -xzf /安全位置/sra-objects-YYYYMMDD.tar.gz
./scripts/start-server.sh
```

恢复前确认数据库和对象根目录属于同一备份时间点，避免事实表和媒体证据错位。

### 7.3 升级

1. 备份数据库、`data/objects` 和 `backend/.env`；
2. 阅读版本变更说明，确认迁移是否可逆；
3. 拉取代码并安装依赖；
4. 运行 `./scripts/deploy.sh --check`，再运行 `./scripts/deploy.sh`；
5. 启动后端，让它按顺序应用迁移；
6. 检查 `/health`、登录、采集任务、关系图和 MCP；
7. 迁移失败时停止继续采集，先根据日志和备份恢复。

部署脚本不会替你回滚数据库迁移。数据库回滚必须遵循具体版本的迁移说明，不能用卸载脚本代替。

## 8. 拆卸 / 卸载

### 8.1 默认安全拆卸

默认只停止本项目进程并删除构建产物、PID 文件和运行日志，保留数据库、采集数据、媒体、原始证据和配置：

```bash
./scripts/uninstall.sh
```

先预览将执行的动作：

```bash
./scripts/uninstall.sh --dry-run
```

### 8.2 分级删除

删除本地配置（包含管理员密码和密钥）：

```bash
./scripts/uninstall.sh --remove-config
```

删除本地 `data/`（包括消息、媒体和原始证据）：

```bash
./scripts/uninstall.sh --remove-data
```

删除配置数据库：

```bash
./scripts/uninstall.sh --remove-db
```

删除数据库会要求输入 `DELETE` 二次确认。自动化场景必须显式使用：

```bash
./scripts/uninstall.sh --remove-db --yes
```

彻底清理本 checkout 的运行现场（高风险，删除配置、数据和数据库；脚本要求显式确认）：

```bash
./scripts/uninstall.sh --purge --yes
```

`--purge` 不接受隐式确认；必须同时写出 `--yes`。这一步会删除数据库、配置和 `data/`，请把它当作不可逆操作：

```bash
./scripts/uninstall.sh --purge --yes
```

卸载不会卸载 PostgreSQL，不会删除其他数据库，也不会触碰非本项目占用的 8000/5700 进程。执行破坏性选项前，请确认备份已成功且路径正确。

## 9. 常见故障

### PostgreSQL unavailable / database does not exist

确认 PostgreSQL 已启动、`DATABASE_URL` 主机/端口/用户/数据库正确。必要时运行 `./scripts/deploy.sh --create-db`，再用 `psql --dbname="$DATABASE_URL" -Atqc 'SELECT 1'` 单独验证。

### 后端端口被占用

启动脚本不会杀掉未知进程。用 `lsof -nP -iTCP:8000 -sTCP:LISTEN` 找到占用者，确认它是否是旧的 SRA 进程后再处理。

### QZone 连接失败

先检查 NapCat HTTP 服务和登录态，再检查 `127.0.0.1:5700`。QZone bridge 是可选适配器，失败不代表 PostgreSQL 或主 API 已损坏。

### MCP 客户端看不到工具

确认客户端使用的是项目脚本的绝对路径、`backend/.env` 存在、`backend/mcp-server` 与源码版本匹配，并重启 MCP 客户端。MCP 默认不监听端口。

### 前端能打开但无法登录

确认后端 `/health` 正常、`CORS_ORIGIN` 与前端地址一致，并查看后端日志：

```bash
tail -f backend/server.log
```

## 10. 数据边界声明

本项目只应处理使用者有权访问、保存和分析的数据。请遵守适用的法律、平台条款和组织政策；不得使用本项目绕过隐私设置、访问控制、验证码或安全措施。分析结果是辅助研判材料，不应被当作未经核验的事实或对个人作出自动化高风险决定。

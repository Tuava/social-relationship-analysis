# 社会关系分析平台

本项目用于研究和分析经过授权获取的 QQ 空间、QQ群互动及其他关系数据，目标是构建以个体为中心的、可追溯的时序社会关系网络。

## 数据源定位

QQ 空间、NapCat 和后续接口都是同一社会网络的不同观测面，不预先区分主次。系统会尽量完整留存经过授权获取的原始聊天、空间动态、互动事件、头像、昵称和资料版本，再通过统一实体和事件模型进行关联。

后端采用 Go，前端采用 Vue 3/TypeScript。项目预留独立的 AI Gateway，用于证据约束下的主题提取、关系解释和候选推断。AI 不直接修改事实数据。

## 下载运行（无需本地编译）

从 [GitHub Releases](https://github.com/Tuava/social-relationship-analysis/releases) 下载对应平台的预编译包，按 [发布包安装指南](docs/release-install.md) 配置 PostgreSQL 并启动。构建、测试和打包由 GitHub Actions 执行。

## 当前阶段

第一阶段底座已经实现：Go API 服务、PostgreSQL 迁移、JWT 本地认证、多 NapCat 账号、WS 原始事件留存、HTTP 可见数据采集、人员/群/消息标准化、一阶关系图 API、操作确认审计、AI 证据包骨架和 Vue/Vuetify 工作台。

设计文档入口：[docs/design.md](docs/design.md)

MCP 接入指南：[docs/mcp-guide.md](docs/mcp-guide.md)

部署与卸载：[docs/deployment.md](docs/deployment.md)

安全策略：[SECURITY.md](SECURITY.md)

QQ 数据框架交接：[docs/qq-framework-handoff.md](docs/qq-framework-handoff.md)

前端设计：[docs/frontend.md](docs/frontend.md)

## 目录约定

```text
social-relationship-analysis/
├── backend/
├── frontend/
├── README.md
├── docs/
│   ├── design.md
│   ├── frontend.md
│   ├── feature-001-ego-network.md
│   ├── mcp-guide.md
│   ├── deployment.md
│   └── qq-framework-handoff.md
├── scripts/
│   ├── deploy.sh
│   └── uninstall.sh
└── data/              本地数据，不提交到仓库
```

## 本地启动

源码开发需要 PostgreSQL 16、Go 1.26 和 Node.js 24。当前 NapCat 配置是 WS `ws://127.0.0.1:3001`、HTTP `http://127.0.0.1:3000`，两端 Token 均为本机 NapCat 中配置的值。

```bash
# 安装并启动数据库（Homebrew）
brew install postgresql@16
brew services start postgresql@16
createdb social_relationship_analysis

# 后端（会生成并保护 backend/.env，凭据权限为 600）
./scripts/start-server.sh

# 另一个终端启动前端
cd frontend
npm ci
npm run serve -- --host 127.0.0.1
```

首次登录用户名为 `admin`，密码保存在本机 `backend/.env` 的 `ADMIN_PASSWORD` 中。数据库不存在或 PostgreSQL 未启动时，后端会明确退出并记录连接错误。

## 部署与卸载

推荐先阅读 [部署指南](docs/deployment.md)，或运行：

```bash
./scripts/deploy.sh --show-credentials
```

默认安全拆卸（保留数据库、配置和采集数据）：

```bash
./scripts/uninstall.sh
```

高风险删除选项见 [部署指南的卸载章节](docs/deployment.md#8-拆卸--卸载)，不会默认删库。

## 数据边界

本项目只处理使用者有权访问和分析的数据，不绕过隐私设置、访问控制、验证码或平台限制。原始数据、凭证和导出结果不应提交到公开代码仓库。

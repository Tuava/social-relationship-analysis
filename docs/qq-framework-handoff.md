# QQ 数据框架交接文档

## 1. 文档用途

本文档用于把本项目的 QQ 数据接入层交接给其他开发者或 AI。这里的“QQ 框架”不是单一 SDK，而是三部分组成的数据源体系：

```text
NapCat HTTP       主动查询 QQ 好友、群、成员、历史消息及资料
NapCat WebSocket  接收实时消息、通知、请求和心跳事件
onebot-qzone      复用 QQ 登录态，补充空间动态、评论、点赞和访客
```

所有数据最后进入 Go 后端的统一原始记录、实体、消息、内容、媒体和关系事件模型。

## 2. 权威文档

- NapCat 官方文档：<http://napneko.github.io/api/4.18.19>
- NapCat 4.18.18 API：<https://napneko.github.io/api/4.18.18>
- OneBot 11 协议仓库：<https://github.com/botuniverse/onebot-11>
- onebot-qzone 上游项目：<https://github.com/Gu-Heping/onebot-qzone>
- 本项目 NapCat OpenAPI 快照：`backend/internal/collectors/napcat/openapi-4.18.18.json`
- 本项目总体设计：`docs/design.md`
- 本项目 QZone 设计：`docs/qzone-bridge.md`

实现时以本地 OpenAPI 快照和实际 NapCat 响应为准。NapCat、QQ 和 QQ 空间接口可能随版本变化。

## 3. 当前运行拓扑

```text
                         实时事件
NapCat WS  127.0.0.1:3001 ──────────┐
                                    │
NapCat HTTP 127.0.0.1:3000 ─────────┼─ Go API :8000 ─ PostgreSQL 16
                                    │                    │
QZone HTTP/WS 127.0.0.1:5700 ───────┘                    └─ data/objects

Vue/Vuetify :5173 ──────────────── Go API :8000
```

当前默认地址：

```text
NapCat HTTP: http://127.0.0.1:3000
NapCat WS:   ws://127.0.0.1:3001
QZone:       http://127.0.0.1:5700
Go API:      http://127.0.0.1:8000
Frontend:    http://127.0.0.1:5173
```

Token、Cookie、JWT Secret 和账号密码不得写入本文档、日志、Git 或前端响应。

## 4. NapCat HTTP 约定

### 4.1 请求

项目统一向 NapCat action 发送 `POST` JSON：

```http
POST /get_group_list
Authorization: Bearer <napcat-token>
Content-Type: application/json

{"no_cache":true}
```

核心客户端位于：

```text
backend/internal/collectors/napcat/http.go
```

统一响应结构：

```json
{
  "status": "ok",
  "retcode": 0,
  "data": {},
  "message": "",
  "wording": ""
}
```

任何 HTTP 非 2xx、无法解析的 JSON、非零错误返回都必须作为显式错误处理。不能把错误响应当作空列表。

### 4.2 当前核心只读 action

```text
get_login_info             当前登录账号
get_friend_list            好友列表
get_stranger_info          用户详细资料
get_group_list             当前可见群
get_group_member_list      群成员列表
get_group_member_info      群成员详情
get_group_msg_history      群历史消息
get_friend_msg_history     私聊历史消息
get_group_root_files       群文件
get_qun_album_list         群相册
_get_group_notice          群公告
get_group_honor_info       群荣誉
get_recent_contact         最近联系人（以实际 OpenAPI 名称为准）
```

核心 action 使用强类型 Go 方法；其他 OpenAPI action 通过能力目录和通用调用器接入。不得静默遗漏未知 action。

### 4.3 历史消息分页

历史消息不能只请求固定 200 条。当前实现使用游标向更早消息翻页：

```text
backend/internal/collectors/napcat/collector_pagination.go
```

重要规则：

- `message_seq` 是翻页锚点，不是本地数组下标；
- 使用服务端返回的下一锚点，不猜测 `seq - 1`；
- 保存采集游标，失败后可以继续；
- 对重复页和重复消息做哈希/平台消息 ID 去重；
- 空页、重复锚点和接口错误必须分别记录；
- 原始响应先保存，再做消息标准化。

## 5. NapCat WebSocket 约定

客户端实现：

```text
backend/internal/collectors/napcat/ws.go
backend/internal/collectors/napcat/manager.go
```

连接要求：

- 使用 `Authorization: Bearer <token>`；
- 支持断线重连；
- 记录连接状态、最后连接时间和最后事件时间；
- 兼容单个 JSON 对象和 JSON 数组消息；
- 心跳观察周期约 30 秒；
- 所有未知事件仍写入原始记录；
- 事件按 NapCat 账号隔离来源；
- 实时接收不依赖手动采集任务。

常见事件分类：

```text
message      群消息、私聊消息
notice       入群、退群、撤回、戳一戳等通知
request      好友请求、加群请求
meta_event   生命周期、心跳
```

实时事件流程：

```text
WS payload
→ raw_records
→ raw_events
→ 消息/人员/会话标准化
→ relation_events
→ 媒体引用队列
```

实时事件不可只写统计计数；原始 JSON 和消息段必须保留。

## 6. QZone sidecar

QQ 空间不是由 NapCat 消息接口完整提供。本项目把 `integrations/onebot-qzone` 作为独立 sidecar：

```text
NapCat 提供当前 QQ 登录态
→ scripts/start-qzone.sh 获取 qzone.qq.com 临时 Cookie
→ onebot-qzone 暴露 OneBot HTTP/WS
→ Go qzone collector 保存和标准化响应
```

关键位置：

```text
integrations/onebot-qzone/
scripts/start-qzone.sh
backend/internal/collectors/qzone/
docs/qzone-bridge.md
```

当前重点只读能力：

```text
get_login_info
get_stranger_info
get_friend_list
get_emotion_list
get_msg
get_comment_list
get_like_list
get_visitor_list
get_album_list
get_photo_list
```

空间数据的限制必须显式保留：

- 好友动态流不一定覆盖全部历史动态；
- 点赞列表可能只是当前 HTML 能展开到的部分，不等于点赞计数；
- 评论、点赞和访客接口可能限流或降级；
- feeds3、详情接口和计数接口的结果可能不一致；
- 每条结果必须记录 action、参数摘要、采集时间、原始响应和可靠性标记。

不能仅凭计数创建具体点赞者，不能把推测的用户写成事实。

## 7. 采集任务与范围

采集任务 API：

```text
POST /api/v1/collection-runs
GET  /api/v1/collection-runs
GET  /api/v1/collection-runs/:id
GET  /api/v1/collection-runs/:id/modules
GET  /api/v1/collection-runs/:id/candidates
POST /api/v1/collection-runs/:id/continue
POST /api/v1/collection-runs/:id/cancel
```

任务类型：

```text
full_visible_data  NapCat 可见数据
profile_sync       人员详细资料
qzone_sync         QQ 空间数据
```

范围结构：

```json
{
  "entries": [
    {"type":"qq","id":"10000001","mode":"expand_people"},
    {"type":"group","id":"123456789","mode":"full_expand"}
  ],
  "excluded_groups": [],
  "excluded_qqs": [],
  "default_group_mode": "full_collect",
  "default_space_mode": "expand_interactions",
  "max_depth": 4
}
```

模式语义：

```text
group record_only         只保存群和成员观测
group full_collect        采集群资料、成员、消息和可见资源
group full_expand         完整采集，并把新人物/群加入候选扩散

space posts_only          只采集动态
space expand_interactions 采集动态、评论、点赞和访客
space expand_people       采集互动，并把互动人员加入候选扩散
```

递归不使用“请求预算”作为完成状态。入口、排除规则、用户选择和显式深度共同控制范围。千人群等大分支进入候选池，用户可以允许或排除后继续。

## 8. 原始数据与标准化

必须遵守以下顺序：

```text
平台响应
→ 保存 raw_records/raw_events
→ 标准化实体与事件
→ 建立媒体引用
→ 构建关系图
```

主要事实对象：

```text
persons / person_identifiers       人物与平台标识
person_profiles                    昵称、头像、资料时间版本
groups / group_memberships         群与成员历史
conversations                      私聊/群聊会话
messages / message_segments        消息原文和消息段
contents                           QQ 空间内容
relation_events                    原子关系事件
media_references / media_assets    媒体来源和归档对象
```

关系事件示例：

```text
member_of
sent_message
published
liked
commented
replied_to
mentioned
quoted
visited
joined_group
left_group
file_shared
message_recalled
```

昵称、头像、群名片和资料不能原地覆盖，应保存时间版本。分析结果不能覆盖原始事实。

## 9. 媒体资源与头像策略

媒体策略：

```text
头像优先自动下载（仅限官方标准 QQ 头像，严禁使用 QZone 防盗链头像）
图片、表情、语音、视频、文件按策略下载
数据库保存引用和元数据
文件按 SHA-256 保存到 data/objects/ab/cd/<sha256>
```

### 9.1 头像隔离与官方标准解析原则

- **严禁空间头像入库**：无论何时都不要把空间接口返回的防盗链头像（如 `store.qq.com/qzone/...`、`*.qpic.cn`、`figureurl`）作为用户头像入库或入队。这类链接依赖 Cookie/Referer 校验且时效性差，直接在浏览器展示会触发 403 / “未经允许不可引用”。
- **官方标准头像源**：
  - **人员头像**：统一通过 `https://q1.qlogo.cn/g?b=qq&nk={qq}&s=640` 或 `https://q.qlogo.cn/headimg_dl?dst_uin={qq}&spec=640` 拉取与下载；
  - **群头像**：统一通过 `https://p.qlogo.cn/gh/{group_id}/{group_id}/640/` 拉取与下载。
- **三级降级显示梯队**：
  1. 第一优先级：本地已持久化资产（`/api/v1/media/assets/{asset_id}`）；
  2. 第二优先级：后端统一代理服务（`/api/v1/media/avatars/person/{qq}`，由后端无 Referer 拉取官方高清源并本地缓存）；
  3. 第三优先级：前端直连官方标准 CDN（图片标签必须声明 `referrerpolicy="no-referrer"`）。

NapCat 消息段中的图片、表情、语音、视频、文件都要解析为媒体引用。下载失败不应导致消息丢失。媒体引用与归档文件必须分开统计，避免把“发现资源”误算成“已下载文件”。

## 10. 副作用与高权限 action

以下 action 不允许被采集任务或 AI 直接调用：

```text
发消息、撤回、点赞、评论、戳一戳
禁言、踢人、设置管理员、修改群信息
修改资料、发布/删除空间动态
上传/删除文件
退出登录、重启、清理缓存
Cookie、Credentials、ClientKey 等凭证接口
```

必须经过：

```text
操作预览
→ 用户确认
→ 执行
→ 保存请求与响应
→ 审计日志
```

## 11. 项目代码地图

```text
backend/internal/collectors/napcat/http.go
  NapCat HTTP 客户端和强类型 action

backend/internal/collectors/napcat/ws.go
  WebSocket 接收、重连和事件保存

backend/internal/collectors/napcat/manager.go
  多账号连接生命周期

backend/internal/collectors/napcat/collector.go
  可见数据采集与标准化入口

backend/internal/collectors/napcat/collector_pagination.go
  群聊/私聊历史分页和游标

backend/internal/collectors/napcat/profiles.go
  人员详细资料同步

backend/internal/collectors/napcat/capabilities.go
  OpenAPI 能力目录

backend/internal/collectors/qzone/
  QZone HTTP/WS 客户端、采集、分页和标准化

backend/internal/normalization/
  人员、群、消息、资料和媒体标准化

backend/internal/persistence/
  PostgreSQL 持久化与采集任务状态

backend/internal/api/server.go
  平台 API 路由和任务启动
```

## 12. 开发与验证

```bash
# 后端测试
cd backend
go test ./...

# 前端类型与构建
cd frontend
npx vue-tsc --noEmit
npm run build

# 启动
cd backend && ./server
cd frontend && npm run serve -- --host 127.0.0.1
./scripts/start-qzone.sh
```

不要为了验证代码自动触发全量 NapCat/QZone 采集。当前账号曾发生掉线和疑似风控。优先使用现有数据库、单元测试、`httptest` 和模拟 WS 服务。

## 13. 给另一个 AI 的工作约束

可以把下面这段作为任务前置提示：

```text
先阅读 docs/qq-framework-handoff.md、docs/design.md、docs/qzone-bridge.md。
NapCat HTTP/WS 与 onebot-qzone 是两个独立数据源，不要混为同一接口。
严禁把空间（QZone）接口的防盗链临时头像（store.qq.com/qpic.cn）当作用户头像入库或入队，用户头像统一走 QQ 官方标准接口。
所有平台响应先保存原始记录，再做标准化；未知事件不能丢弃。
不要写死采集数据上限，不要把前端渲染档位当成数据上限。
不要自动触发全量远程采集，不要恢复所有媒体下载。
不要调用发消息、点赞、评论、群管理、凭证等副作用/高权限 action。
不要覆盖用户已有改动，不要重置 Git 工作树。
完成改动后运行 go test ./...、vue-tsc、前端 build 和 git diff --check。
```

## 14. 当前核心原则

```text
数据完整性高于一次性渲染数量
原始记录高于聚合统计
关系事件高于静态边
证据链高于无来源结论
平台适配器相互独立
所有账号和来源可追溯
副作用必须确认和审计
```

# 社会关系分析平台 · QQ 机器人功能设计

> 状态:设计稿 v1.0 · 日期:2026-08-17
> 目标:给现有"采集 → 标准化 → 分析"系统增加**主动交互出口**,让 QQ 群/私聊可以直接向系统提问、触发采集、接收推送。

---

## 1. 设计目标

| # | 目标 | 说明 |
|---|------|------|
| 1 | **问答入口** | 群里 @机器人 就能查人物档案、关系路径、群情报、证据 |
| 2 | **主动推送** | 新消息速率、采集完成、覆盖盲区、风险事件 → 推送到订阅群 |
| 3 | **操作触发** | 群里发起采集/补采(走现有操作中心二次确认) |
| 4 | **零新依赖** | 复用现有 NapCat WS + HTTP action 通道,不引入新框架 |
| 5 | **可审计** | 所有机器人触发的副作用操作进入现有 operations 审计 |

---

## 2. 架构总览

```
                        ┌─────────────────────────────────────────────┐
                        │              NapCat (QQ 协议层)              │
                        │   WS 事件(消息/通知)  +  HTTP action(发消息)  │
                        └───────────────┬─────────────────▲────────────┘
                                        │ 事件流(已有)     │ action 调用(新增)
                         ┌──────────────▼─────────────────┴────────────┐
                         │              robot/ 包(新增)                 │
                         │  ┌──────────┐  ┌──────────┐  ┌───────────┐  │
                         │  │ Listener │→│ Dispatcher│→│ Executor  │  │
                         │  │ (订阅WS) │  │ (命令解析)│  │ (调服务)  │  │
                         │  └──────────┘  └────┬─────┘  └─────┬─────┘  │
                         │                     │              │        │
                         │  ┌──────────┐  ┌────▼─────┐  ┌─────▼─────┐  │
                         │  │ Scheduler│  │ Perms    │  │ Pusher    │  │
                         │  │ (定时推送)│  │ (权限)   │  │ (主动推送) │  │
                         │  └──────────┘  └──────────┘  └───────────┘  │
                         └──────────────────────┬───────────────────────┘
                                                │ 调用内部服务
                ┌───────────────────────────────▼───────────────────────────────┐
                │           现有后端(internal/ 包,全部复用)                     │
                │  api/(REST)  analysis/  persistence/  operations/  mcp/      │
                │  collectors/napcat(已复用其 WSClient 与 HTTP action)          │
                └───────────────────────────────────────────────────────────────┘
```

### 分层原则
- **robot 包是"门面"**:只做协议适配(QQ 消息 ⇄ 内部调用),不重复业务逻辑
- **查询走 persistence/analysis 直连**,避免再造一层 REST
- **副作用操作走 operations**(和 MCP 同一套确认/审计管线)
- **回复格式统一**:文本优先,支持 CQ 码(图片/at/JSON 卡片)

---

## 3. 模块设计

### 3.1 Listener(事件订阅)
- 复用现有 `collectors/napcat/WSClient`(同一 WS 连接,新增订阅者)
- 过滤 `post_type=message`(群/私聊),忽略机器人自身消息(根据 self_id)
- 事件去重(sequence 去重,复用现有 WSEvent.Sequence)
- 热重连已有 backoff 机制,无需改动

### 3.2 Dispatcher(命令解析)
- 触发条件:群内 **@机器人** 或前缀 `/`;私聊直接对话
- 解析器:`/命令 [参数]` + 自然语言兜底(简单关键词匹配,不接 LLM)
- 命令注册表:map[命令] → Handler + 权限等级 + 帮助文本
- 未知命令 → 回复帮助菜单

### 3.3 Executor(执行器)
- 每个 Handler 一个函数:`func(ctx, *CmdCtx) (*Reply, error)`
- CmdCtx 携带:来源群/人、发送者权限、参数
- Reply 支持:文本 / 图片(媒体URL) / at / 分段长文(>4096 自动分片)

### 3.4 Pusher(主动推送)
- **订阅表**:哪些群订阅了哪些推送主题(存 DB)
- 推送主题:
  - `rate`:消息接收速率指标(每 N 分钟)
  - `collection_done`:采集批次完成
  - `coverage_gap`:新增覆盖盲区
  - `risk_event`:风险信号(自毁/威胁表述,关键词规则)
- 幂等:同主题同批次只推一次(状态表)

### 3.5 Scheduler(定时任务)
- 复用现有 cron/后台协程模式(参考 mediaWorker)
- 每 5 分钟扫描推送队列,节流、合并

### 3.6 Perms(权限)
- 三级:**owner**(系统管理员)/ **admin**(群管理员/白名单)/ **member**(普通成员)
- 映射:QQ 号 → 角色(存 DB,可配置)
- 敏感命令(采集编排/推送订阅变更)要求 owner/admin

---

## 4. 命令集设计

### 4.1 查询类(所有人可用)
| 命令 | 参数 | 说明 |
|------|------|------|
| `/查人 <QQ/昵称>` | 必填 | 人物档案摘要(身份/群/互动/活跃) |
| `/关系 <QQ1> <QQ2>` | 必填×2 | 最短路径 + 路径证据 |
| `/查群 <群号>` | 必填 | 群情报(成员数/活跃榜/覆盖) |
| `/路径 <QQ>` | 必填 | ego 网络 Top N |
| `/消息 <QQ> [条数]` | 必填+选 | 最近消息摘要 |
| `/动态 <QQ> [条数]` | 必填+选 | 最近 QZone 动态 |
| `/覆盖 <QQ>` | 必填 | 该人数据覆盖地图 |
| `/统计` | 无 | 系统总览(表计数) |
| `/帮助` | 无 | 命令菜单 |

### 4.2 操作类(owner/admin,走操作中心)
| 命令 | 参数 | 说明 |
|------|------|------|
| `/采集 <目标> [模块]` | 必填 | 触发采集(复用 sra_collection_orchestrate 逻辑) |
| `/补采 <QQ> <源>` | 必填 | 单人多源补采 |
| `/订阅 <主题> on/off` | 必填 | 本群推送订阅开关 |

### 4.3 管理类(owner)
| 命令 | 参数 | 说明 |
|------|------|------|
| `/授权 <QQ> <角色>` | 必填×2 | 授予/撤销权限 |
| `/机器人状态` | 无 | 连接/订阅/队列状态 |

---

## 5. 数据模型(新增 3 张表)

```sql
-- 机器人订阅(群 → 推送主题)
CREATE TABLE IF NOT EXISTS robot_subscriptions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id      TEXT NOT NULL,          -- 订阅群
    topic         TEXT NOT NULL,          -- rate/collection_done/coverage_gap/risk_event
    enabled       BOOLEAN NOT NULL DEFAULT TRUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (group_id, topic)
);

-- 推送幂等(防止重复推送)
CREATE TABLE IF NOT EXISTS robot_push_log (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    topic       TEXT NOT NULL,
    batch_key   TEXT NOT NULL,            -- 批次标识(如 collection_run_id)
    group_id    TEXT NOT NULL,
    pushed_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (topic, batch_key, group_id)
);

-- 机器人权限
CREATE TABLE IF NOT EXISTS robot_users (
    qq          TEXT PRIMARY KEY,
    role        TEXT NOT NULL CHECK (role IN ('owner','admin','member')),
    note        TEXT DEFAULT '',
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

迁移编号:**028_robot.sql**(接在现有 026 之后)。

---

## 6. 关键流程

### 6.1 问答时序
```
用户 @机器人 /查人 10000001
  → Listener 收到群消息
  → Dispatcher 解析命令+参数+发送者权限
  → Executor 调 persistence(或 analysis)查询
  → Reply:分片文本回复(>4096 自动切)
  → HTTP action send_group_msg 发送
```

### 6.2 风险推送时序
```
WS 收到新消息 → 标准化入库(现有)
  → risk 扫描器(关键词规则,复用现有 ai/规则)命中
  → 写 push_log + 推送队列
  → Scheduler 每 5 分钟扫队列
  → Pusher 调 send_group_msg 推送到订阅群
```

---

## 7. 安全与审计

| 项 | 设计 |
|----|------|
| **副作用确认** | `/采集` 等命令 → 创建 operation 请求 → 需 owner 在管理台或群里确认(复用现有 operations 二次确认) |
| **审计** | 每个机器人动作写 operation_audits(action=robot_command, 来源=qq号) |
| **频率限制** | 每群 10 条/分钟,超限静默丢弃 + 告警 |
| **敏感信息** | 回复中不输出:数据库连接串、Token、未脱敏原始记录 |
| **权限默认拒绝** | 未授权用户 = member,操作类命令一律拒绝 |

---

## 8. 实现路线图

### Phase 1(基础骨架,0.5 天)
- `internal/robot/` 包:Listener + Dispatcher + Executor + 帮助菜单
- 查询类命令 `/查人 /统计 /帮助`
- 迁移 028 + robot_users 种子数据(owner=当前管理员)

### Phase 2(查询全量,1 天)
- `/关系 /查群 /路径 /消息 /动态 /覆盖`
- 长文分片、CQ 图片回复

### Phase 3(操作与推送,1 天)
- `/采集 /补采 /订阅`(走 operations)
- Pusher + Scheduler + 3 张表
- `rate` / `collection_done` 推送

### Phase 4(风险与打磨,1 天)
- `risk_event` 推送(关键词规则)
- 频率限制、审计、故障恢复、文档

**总计约 3.5 天**,全部复用现有组件,无新依赖。

---

## 9. 复用清单(不重复造轮子)

| 组件 | 复用方式 |
|------|---------|
| `collectors/napcat/WSClient` | 事件订阅(现有连接) |
| `collectors/napcat` HTTP action | 发消息/发图片(需确认 action 方法是否已封装,没有则补) |
| `persistence` | 人物/群/消息/关系查询 |
| `analysis` | 路径规划、ego 网络 |
| `operations` | 采集编排 + 二次确认 + 审计 |
| `mcp/handlers_*` | 可复用其查询函数(内部函数而非 MCP 协议) |
| `ai` 规则引擎 | risk_event 关键词扫描 |

---

## 10. 待确认问题

1. **回复渠道**:机器人只回命令来源群,还是支持"跨群转发"?(建议:只回来源)
2. **risk_event 推送**:是否要对"自毁/威胁"类表述做推送?(涉及隐私,建议仅 owner 订阅)
3. **图片回复**:QZone 动态回复带图,还是纯文本摘要?(建议先纯文本,Phase 2 加图)
4. **机器人 QQ 号**:用现有采集账号复用,还是单独小号?(建议单独小号,避免采集账号被群行为干扰)

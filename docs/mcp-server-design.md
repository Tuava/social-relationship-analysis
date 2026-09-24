# MCP 服务设计

> 状态：本文件是 2026-08 实施前的设计稿。实现已落地并迭代到 2026-07-28 协议（`server/discover` + 5 个协议版本），
> 当前行为以 [mcp-guide.md](./mcp-guide.md) 为准；此处的设计意图仍有效，具体细节如有出入以实现为准。

## 目标

让任何兼容 MCP 的 AI 客户端都能通过稳定的语义接口使用平台数据，同时满足两个硬约束：

- 尽量少的 token 调用量：先解析、先摘要、分页、紧凑输出、可追溯原文。
- 不绕过安全边界：模型默认只读；采集、媒体下载和副作用操作仍需要显式人工确认与审计。

协议基线已升级为官方 [MCP 2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28)（兼容 2024-11-05 起的旧版握手）。当前服务保留 stdio 传输，日志只写 stderr，stdout 保持纯 JSON-RPC。

## 调用模式

```text
resolve 定位实体
  -> 选择语义工具读取摘要
  -> 需要细节时再分页或读单条证据
  -> 不要把整库/整图一次性塞给模型
```

一条典型分析链：

```text
sra_resolve
  -> sra_person_dossier
  -> sra_message_history
  -> sra_relation_events
  -> sra_find_path
  -> sra_raw_evidence
```

## Token 优化策略

- `jsonResult` 使用紧凑 JSON，不再缩进美化。
- 列表工具统一返回：

```text
data, total, limit, offset, next_cursor, has_more
```

- 大字段默认截断，例如消息正文最多 500 字符。
- 详情工具支持 `sections`/`include_*`，默认只给摘要。
- 图谱默认 `summary` 格式，`full` 只用于确需完整拓扑。
- 关系事件默认只返回 `raw_record_id` 和证据数量，原始 payload 通过 `sra_raw_evidence` 单独拉取。
- 避免在 MCP 中手写重复的 BFS；`sra_find_path` 无关系过滤时复用后端 `analysis.Router`，保留证据方向和截断状态。

## 工具分层

### 定位与发现

- `sra_resolve`：QQ/昵称/群号到内部实体。
- `sra_search`：人员、群、内容的多面搜索。
- `sra_discover_connections`：按互动方向发现相关人。
- `sra_capabilities`：查询 NapCat 能力目录。
- `sra_schema`：只读数据模型概览。

### 画像与群组

- `sra_person_dossier`：人员档案。
- `sra_profile_evolution`：昵称、头像、签名变化。
- `sra_group_intel`：群成员、活跃贡献者、跨群重叠。
- `sra_mutual_analysis`：两人共同群、共同联系人、互动方向和强度。

### 网络与时间

- `sra_ego_network`：带中心度/桥接/社区指标的自我网络。
- `sra_find_path`：两个实体之间的路径与证据方向。
- `sra_activity_timeline`：活跃时间分布。
- `sra_interaction_stream`：关系事件流。

### 内容与消息

- `sra_content_feed`：空间动态与互动指标。
- `sra_message_history`：私聊/群聊消息分页检索。

### 证据与关系

- `sra_relation_events`：原子关系事件查询。
- `sra_raw_evidence`：原始接口记录。
- `sra_inferences`：AI 推断声明与审查状态。
- `sra_evidence_packs`：受控证据包。

### 采集与运营

- `sra_collection_orchestrate`：创建采集任务，属于显式副作用入口。
- `sra_collection_status`：单任务模块进度。
- `sra_collection_runs`：任务列表。
- `sra_collection_candidates`：递归候选队列。
- `sra_coverage_audit`：数据覆盖盲区。
- `sra_media_stats`：媒体下载汇总。
- `sra_media_references`：媒体归档分页。
- `sra_accounts`：数据源账号与连接状态，不含 token。
- `sra_source_connections`：传输连接状态。

### AI 与工作区

- `sra_ai_runs`：AI 运行元数据。
- `sra_research_workspaces`：研究工作区元数据。
- `sra_operations`：操作请求状态，只读。
- `sra_system_overview`：全库统计。
- `sra_sql_query`：受控只读 SQL，仅作为语义工具覆盖不到的逃生舱。

## 权限边界

- MCP 默认不返回 `ws_token`、`http_token`、`access_token`、JWT secret 或数据库密码。
- `sra_collection_orchestrate` 是唯一主动创建采集任务的 MCP 工具；后续仍需接入操作确认审计。
- 发消息、点赞、评论、禁言、群管理等 NapCat 副作用 API 不直接暴露为 MCP 工具。
- AI 推断不写入事实表；`inferences` 与 `relation_events`、`raw_records` 分层保存。
- `sra_sql_query` 运行在只读事务中，带 10 秒超时和 200 行上限。

# 社会关系分析平台 MCP 接入指南

本项目提供基于 stdio 的 MCP 服务，把已经采集并经过统一建模的数据以只读分析工具暴露给兼容 MCP 的客户端。MCP 只负责受控访问，不绕过平台权限，也不替代原始记录和证据链。

---

## 🌟 最新 MCP 规范特性支持

1. `server/discover` 和 `initialize`：返回协议版本及能力矩阵；
2. `tools/list` 和 `tools/call`：提供 67 个分层分析工具；每个工具带标题、只读/副作用注解、参数约束，核心语义工具声明 `outputSchema`；
3. `resources/list`、`resources/templates/list` 和 `resources/read`：提供人物、图谱、采集状态以及原始记录/内容/消息逐条证据资源；
4. `prompts/list` 和 `prompts/get`：提供两个分析工作流模板；
5. `completion/complete`：基于工具 schema 枚举与静态词表返回真实补全（如 `entity_type`、`sections`、`action_types`、`status` 等），并支持 prompt/resource 引用补全。

### 响应与错误约定

- `tools/list` 顶层返回 `resultType:"complete"` 和 `ttlMs`（缓存提示）。
- 工具执行失败以 `isError:true` 的结果返回，正文是 JSON：`{"ok":false,"code":"...","message":"..."}`，其中 `code` 取值 `not_found` / `invalid_arguments` / `database_error` / `internal_error`，不会伪装成 JSON-RPC 传输错误。
- 工具参数中的 QQ 号用 `^[1-9]\d{4,11}$` 约束，`limit`/`offset` 下限为 0；具体上限见各工具描述。

---

## 🚀 客户端接入配置 (Configuration)

在你的 MCP 配置文件（如 `claude_desktop_config.json`、Cursor `.cursor/mcp.json` 或 VSCode MCP 插件）中添加如下配置：

```json
{
  "mcpServers": {
    "social-relationship-analysis": {
      "command": "/absolute/path/to/social-relationship-analysis/scripts/mcp-server.sh"
    }
  }
}
```

---

## MCP 工具、资源与提示词清单

### 1. Tools (工具集)

#### 侦察与画像
- `sra_search`: 人员、群组和内容的多维检索
- `sra_discover_connections`: 按互动方向和类型发现连接
- `sra_person_dossier`: 人物身份、行为、社交、覆盖和演变档案
- `sra_group_intel`: 群成员、活跃贡献者和跨群重叠分析
- `sra_profile_evolution`: 昵称、头像和资料快照演变
- `sra_person_detail` / `sra_person_timeline`: 完整归档资料与时间线
- `sra_group_detail`: 群元数据、计数与成员名册

#### 网络与时间序列
- `sra_ego_network`: 带预计算指标的关系网络
- `sra_find_path`: 两个人物之间的多跳路径
- `sra_mutual_analysis`: 多人物共同群、共同联系人和互动分析
- `sra_activity_timeline`: 人物或群组的活跃时间分布
- `sra_interaction_stream`: 带上下文的关系事件流
- `sra_content_feed`: 带互动指标和媒体引用的内容流
- `sra_resolve`: 将 QQ、昵称或群号解析为内部实体
- `sra_message_history`: 私聊和群聊消息分页检索
- `sra_relation_events`: 原子关系事件查询
- `sra_media_stats`: 媒体下载状态汇总
- `sra_media_references`: 媒体归档分页
- `sra_capabilities`: NapCat 能力目录检索
- `sra_schema`: 只读数据模型概览
- `sra_accounts`: 数据源账号与连接状态
- `sra_collection_runs`: 采集任务列表
- `sra_collection_candidates`: 递归候选队列
- `sra_inferences`: AI 推断声明与审查状态
- `sra_ai_runs`: AI 运行元数据
- `sra_evidence_packs`: 受控证据包
- `sra_operations`: 操作请求状态
- `sra_research_workspaces`: 研究工作区元数据
- `sra_source_connections`: 传输连接状态
- `sra_realtime_stats`: 摄取新鲜度与最新时间戳
- `sra_qzone_connections`: QZone 连接状态

#### 内容、证据与假设核查（分析闭环）
- `sra_content_comments`: QZone 评论树（按发布时间升序、标记回复对象）
- `sra_visitor_stream`: 访客聚合（必须限定 `target_qq` 或 `actor_qq`）
- `sra_build_evidence_pack`: 按问题组装证据包（默认只预览，`persist=true` 才落库）
- `sra_hypothesis_check`: 支持/反证扫描，区分自述/转述/行为证据，输出是假设辅助而非事实判定
- `sra_trace_claim`: 从推断逐层回溯到关系事件和原始记录，断链标记 `missing_event_ids`
- `sra_content_detail` / `sra_content_likes`: 单条内容、点赞列表
- `sra_message_detail` / `sra_conversations` / `sra_conversation_context`: 单条消息与会话
- `sra_napcat_read`: 只读直连 NapCat 能力目录端点
- `sra_ai_run_detail` / `sra_evidence_pack_detail`: 单条 AI 运行与证据包明细
- `sra_operation_preview` / `sra_operation_audits`: 副作用操作必须人工确认

#### 采集、证据与运维
- `sra_collection_orchestrate`: 指定入口、模块和扩散范围的采集任务
- `sra_collection_status`: 采集批次、模块进度和候选队列
- `sra_coverage_audit`: 采集覆盖和数据盲区审计
- `sra_collection_modules` / `sra_collection_events`: 任务模块进度与发现事件
- `sra_raw_evidence` / `sra_evidence_details`: 读取单条 / 批量原始证据
- `sra_system_overview`: 系统统计和最新任务状态
- `sra_ego_networks` / `sra_ego_network_detail`: 已保存图谱快照

> 当前对外公布的工具目录为 67 个。标题、注解、输入/输出 schema 以运行时 `tools/list` 返回为准；上面是按用途的速览。旧版 9 个兼容别名仍可调用，但不计入公布目录。

旧版 9 个工具仍可通过 `tools/call` 调用，以兼容已有客户端，但不会出现在 `tools/list` 中；新接入应使用上面的工具名。

### 2. Resources (只读资源)
- `sra://collection-runs/latest`: 最新采集任务实时状态
- `sra://stats/overview`: 系统全量情报概览
- `sra://persons/{qq}`: 目标人物结构化档案 URI
- `sra://ego-networks/{qq}`: 目标关系拓扑 JSON URI
- `sra://raw/{id}`: 原始接口响应/事件 payload（哈希、采集时间）
- `sra://contents/{id}`: 单条 QZone 内容
- `sra://messages/{id}`: 单条消息记录

### 3. Prompts (分析模板)
- `osint_person_deep_dive`: 针对目标 QQ 的全方位 OSINT 深度研判与数字脚印画像生成模版
- `pairwise_relationship_audit`: 两目标交叉关系、社交圈重合度与互动方向性审计模版

## 安全和数据边界

- MCP 默认只读；采集编排属于显式任务入口，不能调用发消息、点赞、评论、群管理或凭证接口。
- `sra_sql_query` 在 PostgreSQL 只读事务中执行，带 10 秒语句超时和返回行数限制；禁止写入、DDL 和权限变更。
- 原始数据、媒体和敏感资料仍受平台登录态及本地账号权限约束。
- AI 输出只能作为分析结果或待确认假设，不能覆盖事实表，也不能把间接证据自动升级为确定事实。
- 不要把 Token、Cookie、JWT Secret 或数据库密码放进 MCP 配置、日志和聊天记录。

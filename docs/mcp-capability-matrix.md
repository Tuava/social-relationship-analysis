# MCP 能力边界对照表

本表把 HTTP API 与 MCP 工具逐项对照。目标不是让 MCP 直接复制所有 HTTP 接口，而是保证每个系统能力都有稳定、可审计的 MCP 语义入口。

- 数据库读取：MCP 语义工具直接查询统一模型。
- NapCat 只读接口：统一由 `sra_napcat_read` 访问。
- NapCat 副作用接口：统一由 `sra_operation_preview` 生成待确认操作，不在 MCP 中执行。
- 凭证：任何 MCP 工具都不返回 token、Cookie、ClientKey 或数据库密码。

## 账号与连接

| HTTP API | MCP 工具 | 说明 |
|---|---|---|
| `GET /accounts` | `sra_accounts` | 列表与状态 |
| `POST /accounts` | 不开放 | 创建账号包含凭证，应人工操作 |
| `GET /accounts/{id}` | `sra_accounts` | 状态已在列表返回 |
| `PATCH /accounts/{id}` | 不开放 | 包含凭证修改 |
| `DELETE /accounts/{id}` | 不开放 | 破坏性操作 |
| `POST /accounts/{id}/test` | 不开放 | 包含连接验证副作用 |
| `POST /accounts/{id}/connect` | 不开放 | 连接操作 |
| `POST /accounts/{id}/disconnect` | 不开放 | 连接操作 |
| `GET /qzone/connections` | `sra_qzone_connections` | 状态与时间 |
| `GET/PUT /accounts/{id}/qzone` | `sra_qzone_connections` 只读 | 更新含 token，不开放 |
| `POST /accounts/{id}/qzone/*` | 不开放 | 连接操作 |

## 采集

| HTTP API | MCP 工具 | 说明 |
|---|---|---|
| `GET /collection-runs` | `sra_collection_runs` | 分页列表 |
| `POST /collection-runs` | `sra_collection_orchestrate` | 唯一创建入口 |
| `GET /collection-runs/{id}` | `sra_collection_status` | 汇总状态 |
| `GET .../modules` | `sra_collection_modules` | 模块进度 |
| `GET .../events` | `sra_collection_events` | 发现事件 |
| `GET .../candidates` | `sra_collection_candidates` | 候选队列 |
| `PATCH .../candidates*` | 不开放 | 人工筛选 |
| `POST .../continue` | 不开放 | 建议人工确认 |
| `POST .../cancel` | 不开放 | 建议人工确认 |

## 人员与群组

| HTTP API | MCP 工具 |
|---|---|
| `GET /persons` | `sra_search` |
| `GET /persons/{id}` | `sra_person_detail`、`sra_person_dossier` |
| `GET /persons/{id}/relationship` | `sra_mutual_analysis` |
| `GET /persons/{id}/relationship-deep` | `sra_mutual_analysis`、`sra_discover_connections` |
| `GET /persons/{id}/coverage` | `sra_coverage_audit` |
| `POST /persons/{id}/collect` | `sra_collection_orchestrate` |
| `GET /persons/{id}/timeline` | `sra_person_timeline` |
| `GET /groups` | `sra_search` |
| `GET /groups/{id}` | `sra_group_detail` |
| `GET /groups/{id}/members` | `sra_group_detail`、`sra_group_intel` |
| `GET /groups/{id}/messages` | `sra_message_history` |
| `GET /groups/{id}/files` | `sra_napcat_read` |
| `GET /groups/{id}/albums` | `sra_napcat_read` |
| `GET /groups/{id}/notices` | `sra_napcat_read` |
| `GET /groups/{id}/honor` | `sra_napcat_read` |
| `GET /recent-contacts` | `sra_napcat_read` |
| `GET /groups/{id}/members/{qq}/info` | `sra_napcat_read` |

## 消息与内容

| HTTP API | MCP 工具 |
|---|---|
| `GET /conversations` | `sra_conversations` |
| `GET /conversations/{id}` | `sra_conversation_context` |
| `GET /conversations/{id}/messages` | `sra_message_history` |
| `GET /conversations/{id}/context` | `sra_conversation_context` |
| `POST /conversations/{id}/send` | `sra_operation_preview` |
| `POST /messages/{id}/recall` | `sra_operation_preview` |
| `POST /messages/{id}/reaction` | `sra_operation_preview` |
| `POST /messages/{id}/essence` | `sra_operation_preview` |
| `GET /messages` | `sra_message_history` |
| `GET /messages/{id}` | `sra_message_detail` |
| `GET /contents` | `sra_content_feed` |
| `GET /contents/{id}` | `sra_content_detail` |
| `GET /contents/{id}/likes` | `sra_content_likes` |

## QZone 分析闭环（MCP 语义工具，无 HTTP 对应）

| 能力 | MCP 工具 | 说明 |
|---|---|---|
| 评论树 | `sra_content_comments` | 按 `parent_content_id` 查评论，回复标记 `reply_to_content_id`，按发布时间升序 |
| 访客聚合 | `sra_visitor_stream` | 由 `relation_events(action_type='visited')` 聚合；必须限定 `target_qq` 或 `actor_qq` |
| 证据包 | `sra_build_evidence_pack` | 按问题合并消息/内容/互动事件，去重后按 token 预算截断；默认仅预览 |
| 假设核查 | `sra_hypothesis_check` | 支持/反证扫描，区分自述/转述/行为证据与已存推断；输出为假设辅助，不作事实判定 |
| 推断追溯 | `sra_trace_claim` | 推断 → 关系事件 → 原始记录逐层回溯，断链报告 `missing_event_ids` |

## 关系与证据

| HTTP API | MCP 工具 |
|---|---|
| `GET /relation-events` | `sra_relation_events` |
| `GET /raw-records/{id}` | `sra_raw_evidence` |
| `POST /analysis/routes` | `sra_find_path` |
| `POST /analysis/evidence-details` | `sra_evidence_details` |
| `POST /ego-networks` | `sra_ego_network` |
| `GET /ego-networks` | `sra_ego_networks` |
| `GET /ego-networks/{id}` | `sra_ego_network_detail` |
| `GET /ego-networks/{id}/view` | `sra_ego_network_detail` |
| `GET /ego-networks/{id}/expand` | 不开放，属于前端交互 |
| `GET /ego-networks/{id}/communities` | `sra_ego_network_detail` |

## AI、工作区与操作

| HTTP API | MCP 工具 |
|---|---|
| `POST /ai/runs` | 不开放；由用户显式启动 |
| `GET /ai/runs` | `sra_ai_runs` |
| `GET /ai/runs/{id}` | `sra_ai_run_detail` |
| `GET /ai/evidence-packs/{id}` | `sra_evidence_pack_detail` |
| `GET /research-workspaces/draft` | `sra_research_workspaces` |
| `PUT /research-workspaces/draft` | 不开放 |
| `GET /research-workspaces/snapshots` | `sra_research_workspaces` |
| `POST /research-workspaces/snapshots` | 不开放 |
| `POST /operations/preview` | `sra_operation_preview` |
| `GET /operations` | `sra_operations` |
| `POST /operations/{id}/confirm` | 不开放，必须人工确认 |
| `POST /operations/{id}/cancel` | 不开放，必须人工确认 |
| `GET /operation-audits` | `sra_operation_audits` |

## 统计与能力

| HTTP API | MCP 工具 |
|---|---|
| `GET /system/capabilities` | `sra_system_overview`、`sra_schema` |
| `GET /napcat/capabilities` | `sra_capabilities` |
| `GET /overview` | `sra_system_overview` |
| `GET /realtime/stats` | `sra_realtime_stats` |
| `GET /media/stats` | `sra_media_stats` |
| `GET /media/downloads` | `sra_media_stats` |
| `PATCH /media/downloads` | 不开放 |
| `GET /media/references` | `sra_media_references` |
| `POST /media/references/{id}/download` | 不开放，人工触发 |
| `POST /media/references/retry` | 不开放，人工触发 |

## 资源模板（Resources）

| URI 模板 | 内容 |
|---|---|
| `sra://persons/{qq}` | 人物档案摘要 |
| `sra://groups/{group_id}` | 群名册与元数据 |
| `sra://ego-networks/{qq}` | 一/二阶关系拓扑 |
| `sra://raw/{id}` | 原始接口响应/事件 payload |
| `sra://contents/{id}` | 单条 QZone 内容 |
| `sra://messages/{id}` | 单条消息记录 |

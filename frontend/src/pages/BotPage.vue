<template>
  <div class="page-wrap bot-dashboard">
    <!-- Top Header -->
    <header class="page-header">
      <div>
        <div class="eyebrow">AI AGENT INTELLIGENCE & AUTOMATION</div>
        <h1>QQ 智能助手 & 代理中枢</h1>
        <p>基于 SRA MCP 全域图谱与独立多轮会话记忆的大模型智能机器人 · 具备全息档案研判与实时群聊/私信交互能力</p>
      </div>
      <div class="header-actions">
        <v-btn
          v-if="instances.length"
          variant="tonal"
          color="primary"
          prepend-icon="mdi-chat-processing-outline"
          class="mr-2"
          @click="openPlayground(instances[0])"
        >
          联调沙盒
        </v-btn>
        <v-btn
          color="primary"
          variant="flat"
          prepend-icon="mdi-plus"
          @click="openCreate"
        >
          新建机器人
        </v-btn>
        <v-btn
          icon="mdi-refresh"
          variant="tonal"
          :loading="busy"
          title="刷新数据"
          @click="loadAll"
        />
      </div>
    </header>

    <v-alert
      v-if="error"
      type="error"
      variant="tonal"
      class="mb-4"
      closable
      @click:close="error = ''"
    >
      {{ error }}
    </v-alert>

    <!-- Runtime KPI Metrics -->
    <section class="runtime-metrics mb-6">
      <div class="runtime-metric">
        <span>机器人实例</span>
        <strong>{{ instances.length }}<small> 个</small></strong>
        <em>{{ runningCount }} 个在线运行中</em>
      </div>
      <div class="runtime-metric">
        <span>NLU 推理引擎</span>
        <strong class="text-truncate text-primary" style="font-size: 17px;">
          {{ instances[0]?.llm_model || 'DeepSeek-V4-Flash' }}
        </strong>
        <em>兼容 OpenAI / ChatCompletions</em>
      </div>
      <div class="runtime-metric">
        <span>挂载 SRA MCP 工具</span>
        <strong>18<small> 个</small></strong>
        <em>全息画像 · 关系链路 · 空间动态</em>
      </div>
      <div class="runtime-metric">
        <span>白名单守护群</span>
        <strong>{{ totalWhitelistedGroups }}<small> 个群</small></strong>
        <em>严格 @ 唤醒 · 私聊直连</em>
      </div>
      <div class="runtime-metric">
        <span>活跃记忆空间</span>
        <strong class="text-success">{{ activeSessions.length }}<small> 组</small></strong>
        <em>独立上下文 · 30分钟滚动窗口</em>
      </div>
      <div class="runtime-metric">
        <span>全链路审计流水</span>
        <strong>{{ audit.length }}<small> 条</small></strong>
        <em>{{ processingAuditCount > 0 ? `${processingAuditCount} 条处理中` : '100% 实时安全留痕' }}</em>
      </div>
    </section>

    <!-- Empty State -->
    <div v-if="!instances.length && !busy" class="empty-hero-card text-center py-12 px-4 mb-6">
      <v-icon icon="mdi-robot-confused-outline" size="64" color="primary" class="mb-3 opacity-60" />
      <h3 class="text-h6 font-weight-bold mb-2">尚未配置任何 QQ 机器人实例</h3>
      <p class="text-body-2 text-medium-emphasis mb-6" style="max-width: 460px; margin-inline: auto;">
        为已连接的 NapCat 账号创建专属机器人，即可通过自然语言在 QQ 群中直接查询人物档案、社交图谱与空间动态。
      </p>
      <v-btn color="primary" variant="flat" prepend-icon="mdi-plus" @click="openCreate">
        立即创建第一个机器人
      </v-btn>
    </div>

    <!-- Active Bot Cards Hub -->
    <section v-if="instances.length" class="bot-hero-grid mb-6">
      <div v-for="it in instances" :key="it.id" class="agent-master-card" :class="{ 'is-active': it.enabled }">
        <div class="agent-card-header">
          <div class="avatar-box">
            <v-icon icon="mdi-cat" size="36" color="primary" />
            <span class="status-dot" :class="it.enabled ? 'online' : 'offline'" />
          </div>
          <div class="agent-title-meta">
            <div class="d-flex align-center gap-2">
              <h2 class="agent-name">{{ it.name }}</h2>
              <v-chip size="x-small" :color="it.enabled ? 'success' : 'default'" variant="tonal">
                {{ it.enabled ? '● 正在运行' : '○ 已休眠' }}
              </v-chip>
            </div>
            <div class="agent-qq text-caption text-medium-emphasis">
              绑定账号：{{ it.account_name || 'NapCat 账号' }}（QQ: <span class="font-mono">{{ it.account_qq || it.account_id }}</span>）
            </div>
          </div>
          <v-spacer />
          <v-switch
            v-model="it.enabled"
            color="primary"
            density="compact"
            hide-details
            class="scale-90"
            @update:model-value="toggleEnabled(it)"
          />
        </div>

        <div class="agent-persona-box">
          <div class="persona-label">
            <v-icon icon="mdi-account-star-outline" size="14" class="mr-1" />
            <span>设定口癖：</span>
            <v-chip size="x-small" color="primary" variant="tonal" class="ml-1 font-weight-bold">
              {{ it.meow_sound || '老吴~' }}
            </v-chip>
          </div>
          <div class="persona-text text-truncate-2">
            {{ it.persona || '未配置系统人设提示词' }}
          </div>
        </div>

        <div class="agent-features-strip">
          <div class="feat-item">
            <span class="feat-label">推理模型</span>
            <span class="feat-value font-mono">{{ it.llm_model || 'deepseek-v4-flash' }}</span>
          </div>
          <div class="feat-item">
            <span class="feat-label">白名单群</span>
            <span class="feat-value">{{ it.whitelisted_groups }} 个群</span>
          </div>
          <div class="feat-item">
            <span class="feat-label">记忆空间</span>
            <span class="feat-value text-success">{{ getSessionsForBot(it.id).length }} 组活跃</span>
          </div>
          <div class="feat-item">
            <span class="feat-label">MCP 与全网能力</span>
            <span class="feat-value text-primary">27 个工具 (含全网实时搜索)</span>
          </div>
        </div>

        <div class="agent-card-actions">
          <v-btn
            size="small"
            color="primary"
            variant="tonal"
            prepend-icon="mdi-chat-processing-outline"
            @click="openPlayground(it)"
          >
            联调沙盒
          </v-btn>
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-brain"
            @click="openMemoryManager(it)"
          >
            记忆空间 ({{ getSessionsForBot(it.id).length }})
          </v-btn>
          <v-btn
            size="small"
            variant="outlined"
            prepend-icon="mdi-shield-account-outline"
            @click="openGroups(it)"
          >
            白名单群 ({{ it.whitelisted_groups }})
          </v-btn>
          <v-btn
            size="small"
            color="primary"
            variant="tonal"
            prepend-icon="mdi-tune-vertical"
            @click="activeTab = 'studio'; initEditForm(it)"
          >
            提示词与全域配置中心
          </v-btn>
          <v-spacer />
          <v-btn
            size="small"
            variant="text"
            color="error"
            prepend-icon="mdi-delete-outline"
            @click="removeInstance(it)"
          >
            删除
          </v-btn>
        </div>
      </div>
    </section>

    <!-- Main Workbench Tabs -->
    <section class="data-surface">
      <div class="surface-tabs-header border-b d-flex align-center px-4 pt-2">
        <v-tabs v-model="activeTab" color="primary" density="compact">
          <v-tab value="audit">
            <v-icon icon="mdi-timeline-text-outline" size="18" class="mr-1" />
            <span>实时交互与全链路审计流水</span>
            <v-badge
              v-if="processingAuditCount > 0"
              color="primary"
              :content="processingAuditCount"
              inline
              class="ml-1"
            />
          </v-tab>
          <v-tab value="memory">
            <v-icon icon="mdi-brain" size="18" class="mr-1" />
            <span>会话记忆空间管理器</span>
            <v-chip size="x-small" color="success" variant="flat" class="ml-2 font-weight-bold">
              {{ activeSessions.length }} 组
            </v-chip>
          </v-tab>
          <v-tab value="studio">
            <v-icon icon="mdi-tune-vertical" size="18" class="mr-1" />
            <span>提示词、关键词与全域配置中心</span>
          </v-tab>
        </v-tabs>
        <v-spacer />
        <div class="d-flex align-center gap-2 pb-2">
          <!-- Auto Refresh Live Switch -->
          <v-switch
            v-model="livePolling"
            color="primary"
            density="compact"
            hide-details
            label="实时监听"
            class="mr-2"
          />
          <v-text-field
            v-if="activeTab === 'audit'"
            v-model="searchAudit"
            placeholder="搜索 QQ / 群号 / 消息..."
            prepend-inner-icon="mdi-magnify"
            density="compact"
            variant="outlined"
            hide-details
            style="width: 220px;"
            clearable
          />
          <v-text-field
            v-else
            v-model="searchMemory"
            placeholder="搜索记忆 QQ / 群 / 关键词..."
            prepend-inner-icon="mdi-magnify"
            density="compact"
            variant="outlined"
            hide-details
            style="width: 220px;"
            clearable
          />
          <v-btn
            size="small"
            variant="tonal"
            prepend-icon="mdi-refresh"
            :loading="busy"
            @click="loadAll"
          >
            刷新
          </v-btn>
        </div>
      </div>

      <!-- Tab 1: Live Audit & Interaction Stream -->
      <div v-if="activeTab === 'audit'" class="pa-4">
        <div class="stream-hints d-flex align-center justify-space-between mb-3 text-caption text-medium-emphasis">
          <span>全量记录大模型每一次输入接收、正在思考阶段、MCP 工具调用与最终回复</span>
          <span v-if="livePolling" class="d-flex align-center text-primary">
            <span class="pulsing-live-dot mr-1" /> 实时长轮询监听中（1.5s 刷新）
          </span>
        </div>

        <div v-if="filteredAudit.length" class="audit-stream-list">
          <div
            v-for="row in filteredAudit"
            :key="row.id"
            class="audit-item-row"
            :class="{ 'is-processing': row.status === 'processing', 'is-failed': row.status === 'failed' }"
          >
            <div class="audit-meta-col">
              <span class="audit-time">{{ formatDate(row.created_at) }}</span>
              <div class="audit-source">
                <v-chip
                  size="x-small"
                  :color="row.group_id ? 'surface-variant' : 'secondary'"
                  variant="flat"
                  class="mr-1"
                >
                  {{ row.group_id ? `群 ${row.group_name || row.group_id}` : '私聊' }}
                </v-chip>
                <span class="user-tag font-mono">QQ: {{ row.user_qq }}</span>
              </div>

              <!-- Status Badge -->
              <div class="mt-2">
                <v-chip
                  v-if="row.status === 'processing'"
                  size="x-small"
                  color="warning"
                  variant="flat"
                  class="glowing-chip font-weight-bold"
                >
                  <v-progress-circular indeterminate size="10" width="2" color="white" class="mr-1" />
                  大模型推理中...
                </v-chip>
                <v-chip
                  v-else-if="row.status === 'failed'"
                  size="x-small"
                  color="error"
                  variant="tonal"
                  class="font-weight-bold"
                >
                  <v-icon icon="mdi-alert-circle" size="12" class="mr-1" />
                  调用失败 ({{ row.latency_ms }}ms)
                </v-chip>
                <v-chip
                  v-else
                  size="x-small"
                  color="success"
                  variant="tonal"
                  class="font-weight-bold"
                >
                  <v-icon icon="mdi-check-circle" size="12" class="mr-1" />
                  已响应 ({{ row.latency_ms }}ms)
                </v-chip>
              </div>

              <!-- Tools Called Chips -->
              <div v-if="row.tools_called && row.tools_called.length" class="tools-called-wrap mt-1">
                <v-chip
                  v-for="t in row.tools_called"
                  :key="t"
                  size="x-small"
                  color="primary"
                  variant="outlined"
                  class="mr-1 mb-1 font-mono"
                >
                  {{ t }}
                </v-chip>
              </div>
            </div>

            <div class="audit-dialogue-col flex-1">
              <div class="query-bubble">
                <v-icon icon="mdi-account" size="14" class="mr-1 opacity-70" />
                <span class="query-text">{{ row.command }}</span>
              </div>

              <!-- Live Thinking / Processing Indicator -->
              <div v-if="row.status === 'processing'" class="processing-placeholder-bubble mt-2 flex-column align-start">
                <div class="d-flex align-center w-100">
                  <v-progress-circular indeterminate size="14" width="2" color="primary" class="mr-2 flex-shrink-0" />
                  <span class="text-caption text-primary font-weight-medium">
                    {{ getLiveProcessingSummary(row) }}
                  </span>
                </div>
                <!-- Live tool chips while processing -->
                <div v-if="row.tools_called && row.tools_called.length" class="d-flex flex-wrap gap-1 mt-2">
                  <v-chip
                    v-for="(t, idx) in row.tools_called"
                    :key="idx"
                    size="x-small"
                    color="primary"
                    variant="tonal"
                    class="font-mono"
                  >
                    <v-icon icon="mdi-cog-sync" size="10" class="mr-1 spin-icon" />
                    {{ t }}
                  </v-chip>
                </div>
              </div>

              <div v-else-if="row.status === 'failed'" class="reply-bubble is-error mt-2">
                <v-icon icon="mdi-alert-outline" size="14" color="error" class="mr-1" />
                <span class="reply-text text-error">{{ row.error_message || '生成回复遇到错误' }}</span>
              </div>

              <div v-else class="reply-bubble mt-2">
                <v-icon icon="mdi-cat" size="14" color="primary" class="mr-1" />
                <span class="reply-text">{{ row.reply }}</span>
                <v-btn
                  icon="mdi-content-copy"
                  size="x-small"
                  variant="text"
                  class="copy-btn opacity-60 ml-2"
                  title="复制回复内容"
                  @click="copyText(row.reply)"
                />
              </div>

              <!-- Thought Chain & Tool Execution Trace (Expandable) -->
              <div v-if="row.thought_trace && row.thought_trace.length" class="thought-trace-panel mt-2">
                <v-btn
                  size="x-small"
                  variant="tonal"
                  color="purple"
                  class="toggle-trace-btn"
                  @click="toggleTraceExpanded(row.id)"
                >
                  <v-icon :icon="isTraceExpanded(row.id) ? 'mdi-chevron-up' : 'mdi-brain'" size="13" class="mr-1" />
                  {{ isTraceExpanded(row.id) ? '收起思考与工具链' : `查看完整思考与工具链 (${getToolCallCount(row.thought_trace)} 步推理)` }}
                </v-btn>

                <v-expand-transition>
                  <div v-if="isTraceExpanded(row.id) || row.status === 'processing'" class="thought-steps-list mt-2">
                    <div
                      v-for="(step, sIdx) in row.thought_trace"
                      :key="sIdx"
                      class="thought-step-card"
                      :class="`type-${step.type}`"
                    >
                      <div class="step-header d-flex align-center justify-space-between mb-1">
                        <div class="d-flex align-center gap-1">
                          <v-chip
                            size="x-small"
                            :color="step.type === 'tool_call' ? 'primary' : step.type === 'tool_result' ? 'success' : 'purple'"
                            variant="flat"
                            class="font-weight-bold"
                          >
                            <v-icon
                              :icon="step.type === 'tool_call' ? 'mdi-wrench' : step.type === 'tool_result' ? 'mdi-check-all' : 'mdi-brain'"
                              size="10"
                              class="mr-1"
                            />
                            {{ step.type === 'tool_call' ? `第 ${step.hop} 步调用: ${step.tool}` : step.type === 'tool_result' ? `${step.tool} 返回` : `第 ${step.hop} 步思考` }}
                          </v-chip>
                          <span v-if="step.latency_ms" class="text-caption font-mono opacity-70">({{ step.latency_ms }}ms)</span>
                        </div>
                        <span class="step-time font-mono text-caption opacity-60">{{ step.time }}</span>
                      </div>

                      <div v-if="step.thought" class="step-thought-content text-caption">
                        <span class="opacity-70 font-italic">思考过程：</span>{{ step.thought }}
                      </div>

                      <div v-if="step.arguments" class="step-args-content text-caption font-mono mt-1">
                        <span class="opacity-70">参数:</span> {{ step.arguments }}
                      </div>

                      <div v-if="step.result_preview" class="step-result-content text-caption font-mono mt-1">
                        <span class="opacity-70">结果:</span> {{ step.result_preview }}
                      </div>
                    </div>
                  </div>
                </v-expand-transition>
              </div>
            </div>
          </div>
        </div>

        <div v-else class="compact-empty py-8 text-center text-medium-emphasis">
          <v-icon icon="mdi-history" size="36" class="mb-2 opacity-50" />
          <div>暂无符合条件的审计记录</div>
        </div>
      </div>

      <!-- Tab 2: Memory Space Manager -->
      <div v-if="activeTab === 'memory'" class="pa-4">
        <div class="d-flex align-center justify-space-between mb-4">
          <div>
            <h3 class="text-subtitle-1 font-weight-bold">活跃会话记忆空间 (Memory Sessions)</h3>
            <p class="text-caption text-medium-emphasis mb-0">
              群聊按「群号+发送者QQ」独立隔离，私聊按「发送者QQ」独立隔离。系统自动维护最近 20 轮滚动上下文记忆。
            </p>
          </div>
          <div class="d-flex align-center gap-2">
            <v-btn
              v-if="instances.length"
              size="small"
              color="primary"
              variant="tonal"
              prepend-icon="mdi-plus"
              @click="openInjectMemory(null)"
            >
              注入初始记忆/偏好
            </v-btn>
            <v-btn
              size="small"
              color="error"
              variant="tonal"
              prepend-icon="mdi-broom"
              :disabled="activeSessions.length === 0"
              @click="clearAllMemory"
            >
              一键清空全部记忆
            </v-btn>
          </div>
        </div>

        <!-- Sessions Grid -->
        <div v-if="filteredSessions.length" class="memory-sessions-grid">
          <div v-for="sess in filteredSessions" :key="sess.key" class="session-card">
            <div class="session-card-header d-flex align-center justify-space-between border-b pb-2 mb-2">
              <div class="d-flex align-center gap-2">
                <v-chip
                  size="x-small"
                  :color="sess.type === 'private' ? 'secondary' : 'primary'"
                  variant="flat"
                  class="font-weight-bold"
                >
                  {{ sess.type === 'private' ? '私聊空间' : '群聊空间' }}
                </v-chip>
                <span v-if="sess.group_id" class="text-caption font-mono">群: {{ sess.group_id }}</span>
                <span class="text-caption font-mono font-weight-bold">用户: {{ sess.user_qq }}</span>
              </div>
              <v-chip size="x-small" color="success" variant="tonal" class="font-weight-bold font-mono">
                {{ sess.turn_count }} 轮对话
              </v-chip>
            </div>

            <div class="session-preview-box">
              <div class="preview-item">
                <span class="preview-role">最近提问：</span>
                <span class="preview-text text-truncate">{{ sess.last_query || '（暂无）' }}</span>
              </div>
              <div class="preview-item">
                <span class="preview-role">老吴回复：</span>
                <span class="preview-text text-truncate text-primary">{{ sess.last_reply || '（暂无）' }}</span>
              </div>
            </div>

            <div class="session-card-footer d-flex align-center justify-space-between pt-2 mt-2 border-t text-caption text-medium-emphasis">
              <span>活跃时间: {{ formatDate(sess.last_active) }}</span>
              <div class="d-flex align-center gap-1">
                <v-btn
                  size="x-small"
                  variant="text"
                  color="primary"
                  prepend-icon="mdi-eye-outline"
                  @click="inspectSession(sess)"
                >
                  查看上下文
                </v-btn>
                <v-btn
                  size="x-small"
                  variant="text"
                  color="warning"
                  icon="mdi-plus-box-outline"
                  title="向该会话注入记忆"
                  @click="openInjectMemory(sess)"
                />
                <v-btn
                  size="x-small"
                  variant="text"
                  color="error"
                  icon="mdi-delete-outline"
                  title="清空该会话记忆"
                  @click="deleteSession(sess)"
                />
              </div>
            </div>
          </div>
        </div>

        <div v-else class="compact-empty py-12 text-center text-medium-emphasis">
          <v-icon icon="mdi-brain-off-outline" size="48" class="mb-2 opacity-50" />
          <div class="text-subtitle-2 font-weight-bold mb-1">当前暂无活跃的会话记忆空间</div>
          <div class="text-caption">当用户在 QQ 群或私信中与机器人互动，系统将自动建立专属对话空间</div>
        </div>
      </div>

      <!-- Tab 3: Prompt Studio & Full Configuration Workbench -->
      <div v-if="activeTab === 'studio'" class="pa-4">
        <div class="studio-header d-flex align-center justify-space-between mb-4">
          <div class="d-flex align-center gap-2">
            <v-icon icon="mdi-tune-vertical" color="primary" size="22" />
            <div>
              <div class="text-subtitle-1 font-weight-bold">机器人提示词、关键词与全域配置中心</div>
              <div class="text-caption text-medium-emphasis">
                实时调节大模型人设、上下文工具调度指引、关键词随机娱乐回复库、控制指令模板与底层模型底座
              </div>
            </div>
          </div>
          <div class="d-flex align-center gap-2">
            <!-- Persona Preset Selector -->
            <v-menu>
              <template #activator="{ props }">
                <v-btn variant="outlined" color="primary" size="small" v-bind="props" prepend-icon="mdi-cat">
                  载入人设模板
                </v-btn>
              </template>
              <v-list density="compact">
                <v-list-item @click="applyPersonaPreset('cat')">
                  <template #prepend><v-icon icon="mdi-cat" color="primary" size="18" /></template>
                  <v-list-item-title>傲娇贴贴猫娘（老吴猫）</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyPersonaPreset('osint')">
                  <template #prepend><v-icon icon="mdi-shield-search" color="warning" size="18" /></template>
                  <v-list-item-title>硬核社交情报分析官</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyPersonaPreset('assistant')">
                  <template #prepend><v-icon icon="mdi-robot" color="info" size="18" /></template>
                  <v-list-item-title>严谨智能助手</v-list-item-title>
                </v-list-item>
              </v-list>
            </v-menu>

            <!-- MCP Tool Template Selector -->
            <v-menu>
              <template #activator="{ props }">
                <v-btn variant="tonal" color="teal" size="small" v-bind="props" prepend-icon="mdi-tools">
                  载入 MCP 调度模板
                </v-btn>
              </template>
              <v-list density="compact">
                <v-list-item @click="applyMCPTemplate('all_in_one')">
                  <template #prepend><v-icon icon="mdi-radar" color="teal" size="18" /></template>
                  <v-list-item-title>🔍 全能社交情报与全网搜索 (推荐)</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyMCPTemplate('vision_specialist')">
                  <template #prepend><v-icon icon="mdi-image-filter-center-focus" color="primary" size="18" /></template>
                  <v-list-item-title>👁️ 多模态视觉与表情包识别专精</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyMCPTemplate('web_search_expert')">
                  <template #prepend><v-icon icon="mdi-web" color="blue" size="18" /></template>
                  <v-list-item-title>🌐 全网实时搜索与即时问答</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyMCPTemplate('group_analyst')">
                  <template #prepend><v-icon icon="mdi-account-group" color="purple" size="18" /></template>
                  <v-list-item-title>👥 群聊生态与话题演化分析</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyMCPTemplate('friendly_companion')">
                  <template #prepend><v-icon icon="mdi-heart-outline" color="pink" size="18" /></template>
                  <v-list-item-title>🛡️ 安全友好与日常陪伴模式</v-list-item-title>
                </v-list-item>
              </v-list>
            </v-menu>
            <v-btn
              color="primary"
              variant="flat"
              size="small"
              prepend-icon="mdi-content-save"
              :loading="busy"
              @click="saveInstance(current || instances[0])"
            >
              保存并实时生效
            </v-btn>
          </div>
        </div>

        <v-row dense>
          <!-- Column 1: Persona & System Prompts -->
          <v-col cols="12" md="6">
            <v-card variant="outlined" class="mb-4 pa-4 config-card">
              <div class="d-flex align-center font-weight-bold mb-3 text-subtitle-2 text-primary">
                <v-icon icon="mdi-cat" size="18" class="mr-1" />
                <span>1. 核心人设与性格提示词 (System Persona Prompt)</span>
              </div>
              <v-row dense>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model="edit.name"
                    label="机器人名称"
                    variant="outlined"
                    density="compact"
                  />
                </v-col>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model="edit.meow_sound"
                    label="口癖 / 句尾特征"
                    placeholder="老吴~"
                    variant="outlined"
                    density="compact"
                  />
                </v-col>
                <v-col cols="12">
                  <v-textarea
                    v-model="edit.persona"
                    label="系统人设提示词 (System Persona)"
                    rows="4"
                    variant="outlined"
                    density="compact"
                    hint="定义机器人的语气、性格、口癖、情感倾向、边界与底线"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </v-card>

            <v-card variant="outlined" class="mb-4 pa-4 config-card">
              <div class="d-flex align-center justify-space-between mb-3 text-subtitle-2 text-primary font-weight-bold">
                <div class="d-flex align-center">
                  <v-icon icon="mdi-brain" size="18" class="mr-1" />
                  <span>2. 上下文与 MCP 工具调度指引 (Tool & Context Prompt)</span>
                </div>
                <v-menu>
                  <template #activator="{ props }">
                    <v-btn size="x-small" variant="tonal" color="teal" v-bind="props" prepend-icon="mdi-tools">
                      选择 MCP 模板
                    </v-btn>
                  </template>
                  <v-list density="compact">
                    <v-list-item @click="applyMCPTemplate('all_in_one')">
                      <template #prepend><v-icon icon="mdi-radar" color="teal" size="18" /></template>
                      <v-list-item-title>🔍 全能社交情报与全网搜索</v-list-item-title>
                    </v-list-item>
                    <v-list-item @click="applyMCPTemplate('vision_specialist')">
                      <template #prepend><v-icon icon="mdi-image-filter-center-focus" color="primary" size="18" /></template>
                      <v-list-item-title>👁️ 多模态视觉与表情包识别</v-list-item-title>
                    </v-list-item>
                    <v-list-item @click="applyMCPTemplate('web_search_expert')">
                      <template #prepend><v-icon icon="mdi-web" color="blue" size="18" /></template>
                      <v-list-item-title>🌐 全网实时搜索与即时问答</v-list-item-title>
                    </v-list-item>
                    <v-list-item @click="applyMCPTemplate('group_analyst')">
                      <template #prepend><v-icon icon="mdi-account-group" color="purple" size="18" /></template>
                      <v-list-item-title>👥 群聊生态与话题演化分析</v-list-item-title>
                    </v-list-item>
                    <v-list-item @click="applyMCPTemplate('friendly_companion')">
                      <template #prepend><v-icon icon="mdi-heart-outline" color="pink" size="18" /></template>
                      <v-list-item-title>🛡️ 安全友好与日常陪伴模式</v-list-item-title>
                    </v-list-item>
                  </v-list>
                </v-menu>
              </div>
              <v-row dense>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model="edit.command_prefix"
                    label="快捷指令前缀"
                    placeholder="/"
                    variant="outlined"
                    density="compact"
                  />
                </v-col>
                <v-col cols="12" sm="6">
                  <v-text-field
                    v-model="edit.greeting"
                    label="开机问候语"
                    placeholder="喵~ 老吴来啦，有事喊我喵！"
                    variant="outlined"
                    density="compact"
                  />
                </v-col>
                <v-col cols="12">
                  <v-textarea
                    v-model="edit.context_prompt"
                    label="工具调度与回答规则 (Context Guidelines)"
                    rows="4"
                    variant="outlined"
                    density="compact"
                    hint="指导模型在回答时何时调用 MCP 情报工具、如何处理查无结果或格式化输出"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </v-card>

            <v-card variant="outlined" class="pa-4 config-card">
              <div class="d-flex align-center font-weight-bold mb-3 text-subtitle-2 text-primary">
                <v-icon icon="mdi-slash-forward" size="18" class="mr-1" />
                <span>3. 快捷控制指令回复模板与异常兜底</span>
              </div>
              <v-row dense>
                <v-col cols="12">
                  <v-textarea
                    v-model="edit.cmd_enable_reply"
                    label="开启服务指令回复模板 (/老吴 开)"
                    rows="2"
                    variant="outlined"
                    density="compact"
                    hint="支持变量: {name}=机器人名, {type}=场景(群聊/私聊), {target}=群名/QQ"
                    persistent-hint
                  />
                </v-col>
                <v-col cols="12">
                  <v-textarea
                    v-model="edit.cmd_disable_reply"
                    label="关闭服务指令回复模板 (/老吴 关)"
                    rows="2"
                    variant="outlined"
                    density="compact"
                    hint="支持变量: {name}=机器人名, {type}=场景(群聊/私聊), {target}=群名/QQ"
                    persistent-hint
                  />
                </v-col>
                <v-col cols="12">
                  <v-text-field
                    v-model="edit.fallback_reply"
                    label="异常与超时兜底回复 (Fallback Reply)"
                    placeholder="喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~"
                    variant="outlined"
                    density="compact"
                    hint="当大模型超时或报错时自动发送的兜底语 (支持 {name})"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </v-card>
          </v-col>

          <!-- Column 2: Keywords & LLM Engine Settings -->
          <v-col cols="12" md="6">
            <v-card variant="outlined" class="mb-4 pa-4 config-card">
              <div class="d-flex align-center justify-space-between mb-3">
                <div class="d-flex align-center font-weight-bold text-subtitle-2 text-primary">
                  <v-icon icon="mdi-comment-quote-outline" size="18" class="mr-1" />
                  <span>4. 关键词与随机娱乐回复语料库 (Entertainment & Keywords)</span>
                </div>
                <div class="d-flex gap-1">
                  <v-btn size="x-small" variant="text" color="primary" @click="resetDefaultEntertainment">
                    重置默认
                  </v-btn>
                  <v-btn size="x-small" variant="text" color="error" @click="edit.entertainment_list = []">
                    清空
                  </v-btn>
                </div>
              </div>
              <v-row dense>
                <v-col cols="12">
                  <div class="text-caption text-medium-emphasis mb-1">
                    娱乐模式随机插话触发概率 (0% 为仅响应艾特与指令，>0% 会在群友闲聊时随机触发回复)：
                  </div>
                  <v-slider
                    v-model="edit.reply_probability"
                    min="0"
                    max="100"
                    step="1"
                    thumb-label
                    color="primary"
                    density="compact"
                  >
                    <template #append>
                      <span class="text-caption font-mono font-weight-bold">{{ edit.reply_probability }}%</span>
                    </template>
                  </v-slider>
                </v-col>
                <v-col cols="12">
                  <div class="d-flex gap-2 mb-2">
                    <v-text-field
                      v-model="newCorpusItem"
                      label="输入新语料词条"
                      placeholder="例如：老吴老吴，今天也要开心喵~"
                      variant="outlined"
                      density="compact"
                      hide-details
                      @keydown.enter.prevent="addCorpusItem"
                    />
                    <v-btn color="primary" variant="tonal" @click="addCorpusItem">
                      添加词条
                    </v-btn>
                  </div>
                  <div class="text-caption text-medium-emphasis mb-2">
                    快捷添加常用语料：
                    <span
                      v-for="word in defaultCorpusPresets"
                      :key="word"
                      class="quick-tag mr-2 text-primary cursor-pointer text-decoration-underline"
                      @click="quickAddCorpus(word)"
                    >
                      + {{ word }}
                    </span>
                  </div>
                  <div class="corpus-chips-box border rounded pa-2 bg-surface-variant-opacity">
                    <div v-if="!edit.entertainment_list || edit.entertainment_list.length === 0" class="text-caption text-medium-emphasis text-center py-2">
                      当前词库为空（可在上方输入添加或点击快捷语料）
                    </div>
                    <div v-else class="d-flex flex-wrap gap-2">
                      <v-chip
                        v-for="(item, idx) in edit.entertainment_list"
                        :key="idx"
                        closable
                        size="small"
                        color="primary"
                        variant="tonal"
                        @click:close="removeCorpusItem(Number(idx))"
                      >
                        {{ item }}
                      </v-chip>
                    </div>
                  </div>
                </v-col>
              </v-row>
            </v-card>

            <v-card variant="outlined" class="pa-4 config-card">
              <div class="d-flex align-center font-weight-bold mb-3 text-subtitle-2 text-primary">
                <v-icon icon="mdi-server-network" size="18" class="mr-1" />
                <span>5. 大模型核心底座与推理参数 (LLM Engine)</span>
              </div>
              <v-row dense>
                <v-col cols="12" sm="8">
                  <v-text-field
                    v-model="edit.llm_api_base"
                    label="API Base 端点地址"
                    placeholder="https://opencode.ai/zen/go/v1 或 https://api.deepseek.com/v1"
                    variant="outlined"
                    density="compact"
                    hint="OpenAI ChatCompletion 兼容协议地址"
                    persistent-hint
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model="edit.llm_model"
                    label="推理模型名称"
                    placeholder="deepseek-v4-flash"
                    variant="outlined"
                    density="compact"
                  />
                </v-col>
                <v-col cols="12">
                  <v-text-field
                    v-model="edit.llm_api_key"
                    label="API Key 密钥"
                    :type="showApiKey ? 'text' : 'password'"
                    placeholder="sk-..."
                    variant="outlined"
                    density="compact"
                    :append-inner-icon="showApiKey ? 'mdi-eye-off' : 'mdi-eye'"
                    @click:append-inner="showApiKey = !showApiKey"
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-slider
                    v-model="edit.llm_temperature"
                    label="发散度 (Temp)"
                    min="0"
                    max="1.5"
                    step="0.1"
                    density="compact"
                    thumb-label
                  >
                    <template #append>
                      <span class="text-caption font-mono">{{ edit.llm_temperature }}</span>
                    </template>
                  </v-slider>
                </v-col>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="edit.llm_max_tokens"
                    label="单次回复 Tokens"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hint="默认 600~2000"
                    persistent-hint
                  />
                </v-col>
                <v-col cols="12" sm="4">
                  <v-text-field
                    v-model.number="edit.max_tool_hops"
                    label="最大工具推理步数"
                    type="number"
                    variant="outlined"
                    density="compact"
                    hint="默认 10 步 (可调 1~25)"
                    persistent-hint
                  />
                </v-col>
              </v-row>
            </v-card>
          </v-col>
        </v-row>

        <div class="d-flex justify-end mt-4">
          <v-btn
            color="primary"
            variant="flat"
            size="large"
            prepend-icon="mdi-content-save"
            :loading="busy"
            @click="saveInstance(current || instances[0])"
          >
            保存全部配置并实时生效
          </v-btn>
        </div>
      </div>
    </section>

    <!-- Session Detail Modal (Full Multi-turn Context) -->
    <v-dialog v-model="showSessionDetail" max-width="640" scrollable>
      <v-card v-if="activeInspectSession" class="modal-card">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-brain" color="primary" class="mr-2" />
          <span class="font-weight-bold">会话上下文详情</span>
          <v-chip size="x-small" color="primary" variant="tonal" class="ml-2 font-mono">
            {{ activeInspectSession.type === 'private' ? '私聊' : `群: ${activeInspectSession.group_id}` }} · QQ: {{ activeInspectSession.user_qq }}
          </v-chip>
          <v-spacer />
          <v-btn icon="mdi-close" size="small" variant="text" @click="showSessionDetail = false" />
        </v-card-title>
        <v-card-text class="pa-4" style="max-height: 500px;">
          <div class="context-bubble-stream">
            <div
              v-for="(m, idx) in activeInspectSession.messages"
              :key="idx"
              class="context-msg-row"
              :class="m.role"
            >
              <div class="role-tag font-weight-bold mb-1 text-caption">
                {{ m.role === 'user' ? `提问者 (QQ: ${activeInspectSession.user_qq})` : '智能助手 (老吴)' }}
              </div>
              <div class="msg-bubble-content">
                {{ m.content }}
              </div>
            </div>
          </div>
        </v-card-text>
        <v-card-actions class="px-4 pb-3 border-t">
          <v-btn
            color="error"
            variant="tonal"
            size="small"
            prepend-icon="mdi-broom"
            @click="deleteSession(activeInspectSession); showSessionDetail = false"
          >
            清空此会话
          </v-btn>
          <v-spacer />
          <v-btn variant="text" size="small" @click="showSessionDetail = false">关闭</v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Inject Memory Modal -->
    <v-dialog v-model="showInjectMemory" max-width="500">
      <v-card class="modal-card">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-database-plus" color="primary" class="mr-2" />
          <span class="font-weight-bold">注入会话记忆偏好 / 事实</span>
        </v-card-title>
        <v-card-text class="pa-4">
          <p class="text-caption text-medium-emphasis mb-3">
            手动为指定会话植入记忆事实或用户特征偏好，大模型在后续对话中将自动读取并基于该记忆进行回复。
          </p>
          <v-text-field
            v-model="injectForm.key"
            label="会话空间 Key"
            placeholder="p:<bot_id>:<user_qq> 或 g:<bot_id>:<group_id>:<user_qq>"
            variant="outlined"
            density="compact"
            class="mb-2"
          />
          <v-select
            v-model="injectForm.role"
            label="记忆角色"
            :items="[
              { title: '助手记忆 (Assistant)', value: 'assistant' },
              { title: '用户偏好 (User)', value: 'user' },
              { title: '系统设定 (System)', value: 'system' }
            ]"
            variant="outlined"
            density="compact"
            class="mb-2"
          />
          <v-textarea
            v-model="injectForm.content"
            label="记忆内容 / 事实描述"
            placeholder="例如：该用户是群管理，喜欢被称呼为张总，重点关注 2841811721 的动态..."
            variant="outlined"
            density="compact"
            rows="3"
          />
        </v-card-text>
        <v-card-actions class="px-4 pb-3 border-t">
          <v-spacer />
          <v-btn variant="text" @click="showInjectMemory = false">取消</v-btn>
          <v-btn
            color="primary"
            variant="flat"
            :disabled="!injectForm.key || !injectForm.content"
            :loading="busy"
            @click="submitInjectMemory"
          >
            确认注入
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Interactive Sandbox Playground Modal -->
    <v-dialog v-model="showPlayground" max-width="720" scrollable>
      <v-card class="playground-modal">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-chat-processing" color="primary" class="mr-2" />
          <span class="font-weight-bold">老吴智能沙盒调试器 (Interactive Sandbox)</span>
          <v-chip size="x-small" color="primary" variant="tonal" class="ml-2 font-mono">
            {{ activeBot?.llm_model || 'deepseek-v4-flash' }}
          </v-chip>
          <v-spacer />
          <v-btn
            size="small"
            variant="tonal"
            color="warning"
            prepend-icon="mdi-broom"
            class="mr-2"
            :loading="chatBusy"
            @click="clearChatMemory"
          >
            重置会话记忆
          </v-btn>
          <v-btn icon="mdi-close" size="small" variant="text" @click="showPlayground = false" />
        </v-card-title>

        <v-card-text class="playground-body pa-4">
          <!-- Session Context Setting -->
          <div class="sandbox-context-bar d-flex align-center gap-3 mb-3 pa-2 rounded">
            <div class="text-caption text-medium-emphasis d-flex align-center">
              <v-icon icon="mdi-account-box-outline" size="14" class="mr-1" />
              测试身份：
            </div>
            <v-text-field
              v-model="testUserQQ"
              label="测试者 QQ"
              density="compact"
              variant="outlined"
              hide-details
              style="max-width: 160px;"
            />
            <v-text-field
              v-model="testGroupID"
              label="模拟群号 (留空为私聊)"
              density="compact"
              variant="outlined"
              hide-details
              style="max-width: 180px;"
            />
            <v-spacer />
            <v-chip size="x-small" color="success" variant="tonal" class="font-mono">
              {{ testGroupID ? `群聊上下文 [${testGroupID}]` : '私聊独立上下文' }}
            </v-chip>
          </div>

          <!-- Quick Presets -->
          <div class="quick-presets d-flex align-center flex-wrap gap-2 mb-3">
            <span class="preset-label text-caption text-medium-emphasis">快捷提问：</span>
            <v-chip
              size="x-small"
              variant="outlined"
              color="primary"
              class="clickable-chip"
              @click="sendPreset('查查 10000001 是谁')"
            >
              查查 10000001 是谁
            </v-chip>
            <v-chip
              size="x-small"
              variant="outlined"
              color="primary"
              class="clickable-chip"
              @click="sendPreset('帮我查查群 10000002 的情报')"
            >
              查群 10000002
            </v-chip>
            <v-chip
              size="x-small"
              variant="outlined"
              color="primary"
              class="clickable-chip"
              @click="sendPreset('老吴~ 你都会些什么技能呀？')"
            >
              你会些什么技能？
            </v-chip>
          </div>

          <!-- Chat Dialogue Viewport -->
          <div ref="chatViewportRef" class="chat-viewport">
            <div
              v-for="(msg, index) in chatMessages"
              :key="index"
              class="chat-bubble-row"
              :class="msg.role"
            >
              <div class="avatar-col">
                <v-avatar size="32" :color="msg.role === 'user' ? 'surface-variant' : 'primary'">
                  <v-icon :icon="msg.role === 'user' ? 'mdi-account' : 'mdi-cat'" size="18" />
                </v-avatar>
              </div>
              <div class="content-col">
                <div class="sender-name">
                  {{ msg.role === 'user' ? `你 (${testUserQQ})` : (activeBot?.name || '老吴') }}
                </div>
                <div class="bubble-text">
                  {{ msg.text }}
                </div>
              </div>
            </div>

            <div v-if="chatBusy" class="chat-bubble-row assistant">
              <div class="avatar-col">
                <v-avatar size="32" color="primary">
                  <v-icon icon="mdi-cat" size="18" />
                </v-avatar>
              </div>
              <div class="content-col">
                <div class="sender-name">{{ activeBot?.name || '老吴' }}</div>
                <div class="bubble-text typing-bubble d-flex align-center gap-2">
                  <v-progress-circular indeterminate size="16" width="2" color="primary" />
                  <span class="text-caption">正在深度检索社交图谱并组织回复喵...</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Input Bar -->
          <div class="chat-input-bar d-flex align-center gap-2 mt-3">
            <v-text-field
              v-model="inputMsg"
              placeholder="输入消息与老吴互动（支持上下文多轮追问）..."
              variant="outlined"
              density="compact"
              hide-details
              :disabled="chatBusy"
              @keydown.enter="sendChat"
            />
            <v-btn
              color="primary"
              variant="flat"
              prepend-icon="mdi-send"
              :loading="chatBusy"
              :disabled="!inputMsg.trim()"
              @click="sendChat"
            >
              发送
            </v-btn>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- Create Instance Modal -->
    <v-dialog v-model="showCreate" max-width="480">
      <v-card class="modal-card">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-robot-plus" color="primary" class="mr-2" />
          <span class="font-weight-bold">新建 QQ 机器人实例</span>
        </v-card-title>
        <v-card-text class="pa-4">
          <p class="text-caption text-medium-emphasis mb-3">
            选择一个已添加的 NapCat QQ 账号，系统将自动挂载 DeepSeek-V4-Flash 大模型与 SRA MCP 社交分析工具。
          </p>
          <v-select
            v-model="createAccountID"
            label="绑定 NapCat 账号"
            :items="accountOptions()"
            item-title="label"
            item-value="value"
            variant="outlined"
            density="compact"
            :no-data-text="accounts.length === 0 ? '尚未添加 NapCat 账号，请先前往「数据源配置」页面添加' : '所有可用账号均已创建实例'"
          />
        </v-card-text>
        <v-card-actions class="px-4 pb-3">
          <v-spacer />
          <v-btn variant="text" @click="showCreate = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :disabled="!createAccountID" :loading="busy" @click="create">
            立即创建
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Configuration Modal (LLM & Persona) -->
    <v-dialog v-model="showEdit" max-width="680" scrollable>
      <v-card v-if="current" class="modal-card">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-cog" color="primary" class="mr-2" />
          <span class="font-weight-bold">配置机器人：{{ current.name }}</span>
          <v-spacer />
          <v-btn icon="mdi-close" size="small" variant="text" @click="showEdit = false" />
        </v-card-title>
        <v-card-text class="pa-4">
          <!-- Section 1: LLM Engine -->
          <div class="config-section-title">
            <v-icon icon="mdi-brain" size="18" color="primary" class="mr-1" />
            <span>LLM 大模型推理引擎 (兼容 OpenAI 协议)</span>
          </div>
          <v-row dense class="mb-2">
            <v-col cols="12" sm="8">
              <v-text-field
                v-model="edit.llm_api_base"
                label="API Base 端点地址"
                placeholder="https://opencode.ai/zen/go/v1 或 https://api.deepseek.com/v1"
                variant="outlined"
                density="compact"
                hint="OpenAI ChatCompletion 兼容协议地址"
                persistent-hint
              />
            </v-col>
            <v-col cols="12" sm="4">
              <v-text-field
                v-model="edit.llm_model"
                label="推理模型名称"
                placeholder="deepseek-v4-flash"
                variant="outlined"
                density="compact"
              />
            </v-col>
            <v-col cols="12">
              <v-text-field
                v-model="edit.llm_api_key"
                label="API Key 密钥"
                :type="showApiKey ? 'text' : 'password'"
                placeholder="sk-..."
                variant="outlined"
                density="compact"
                :append-inner-icon="showApiKey ? 'mdi-eye-off' : 'mdi-eye'"
                @click:append-inner="showApiKey = !showApiKey"
              />
            </v-col>
            <v-col cols="12" sm="4">
              <v-slider
                v-model="edit.llm_temperature"
                label="发散度 (Temp)"
                min="0"
                max="1.5"
                step="0.1"
                density="compact"
                thumb-label
              >
                <template #append>
                  <span class="text-caption font-mono">{{ edit.llm_temperature }}</span>
                </template>
              </v-slider>
            </v-col>
            <v-col cols="12" sm="4">
              <v-text-field
                v-model.number="edit.llm_max_tokens"
                label="单次最大回复 Tokens"
                type="number"
                variant="outlined"
                density="compact"
                hint="默认 600~2000"
                persistent-hint
              />
            </v-col>
            <v-col cols="12" sm="4">
              <v-text-field
                v-model.number="edit.max_tool_hops"
                label="最大工具推理步数 (Tool Hops)"
                type="number"
                variant="outlined"
                density="compact"
                hint="宽松深度推理默认 10 步 (可调 1~25)"
                persistent-hint
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Section 2: Core System Persona Prompt -->
          <div class="config-section-title d-flex align-center justify-space-between mb-2">
            <div class="d-flex align-center">
              <v-icon icon="mdi-cat" size="18" color="primary" class="mr-1" />
              <span>核心人设与性格提示词 (Persona System Prompt)</span>
            </div>
            <!-- Preset Quick Select -->
            <v-menu>
              <template #activator="{ props }">
                <v-btn size="x-small" variant="tonal" color="primary" v-bind="props" prepend-icon="mdi-magic-staff">
                  载入人设预设
                </v-btn>
              </template>
              <v-list density="compact">
                <v-list-item @click="applyPersonaPreset('cat')">
                  <v-list-item-title>傲娇贴贴猫娘（老吴猫）</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyPersonaPreset('osint')">
                  <v-list-item-title>硬核社交情报分析官</v-list-item-title>
                </v-list-item>
                <v-list-item @click="applyPersonaPreset('assistant')">
                  <v-list-item-title>严谨智能助手</v-list-item-title>
                </v-list-item>
              </v-list>
            </v-menu>
          </div>
          <v-row dense>
            <v-col cols="12" sm="6">
              <v-text-field
                v-model="edit.name"
                label="机器人名字"
                variant="outlined"
                density="compact"
              />
            </v-col>
            <v-col cols="12" sm="6">
              <v-text-field
                v-model="edit.meow_sound"
                label="口癖 / 尾缀"
                placeholder="老吴~"
                variant="outlined"
                density="compact"
              />
            </v-col>
            <v-col cols="12">
              <v-textarea
                v-model="edit.persona"
                label="系统核心人设提示词 (System Persona Prompt)"
                rows="4"
                variant="outlined"
                density="compact"
                hint="定义机器人的语气、性格、口癖、情感倾向、边界与底线"
                persistent-hint
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Section 3: Context & Tool-calling Guidelines Prompt -->
          <div class="config-section-title mb-2">
            <v-icon icon="mdi-tune-vertical" size="18" color="primary" class="mr-1" />
            <span>上下文与工具调度指引提示词 (Context & Tool Guidance)</span>
          </div>
          <v-row dense>
            <v-col cols="12">
              <v-textarea
                v-model="edit.context_prompt"
                label="工具调度与回答规则提示词 (Context & Tool-calling Guidelines)"
                rows="3"
                variant="outlined"
                density="compact"
                hint="指导模型在回答时何时调用 MCP 情报工具、如何处理查无结果或格式化输出"
                persistent-hint
              />
            </v-col>
          </v-row>

          <v-divider class="my-4" />

          <!-- Section 4: Command & Event Reply Templates -->
          <div class="config-section-title mb-2">
            <v-icon icon="mdi-slash-forward" size="18" color="primary" class="mr-1" />
            <span>控制指令回复模板与异常兜底 (Command Templates & Fallback)</span>
          </div>
          <v-row dense>
            <v-col cols="12" sm="6">
              <v-textarea
                v-model="edit.cmd_enable_reply"
                label="开启指令回复模板 (/老吴 开)"
                rows="2"
                variant="outlined"
                density="compact"
                hint="支持变量: {name}=机器人名, {type}=场景(群聊/私聊), {target}=群名/QQ"
                persistent-hint
              />
            </v-col>
            <v-col cols="12" sm="6">
              <v-textarea
                v-model="edit.cmd_disable_reply"
                label="关闭指令回复模板 (/老吴 关)"
                rows="2"
                variant="outlined"
                density="compact"
                hint="支持变量: {name}=机器人名, {type}=场景(群聊/私聊), {target}=群名/QQ"
                persistent-hint
              />
            </v-col>
            <v-col cols="12">
              <v-text-field
                v-model="edit.fallback_reply"
                label="异常与超时兜底回复 (Fallback Reply)"
                placeholder="喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~"
                variant="outlined"
                density="compact"
                hint="当大模型超时或报错时自动发送的兜底语 (支持 {name})"
                persistent-hint
              />
            </v-col>
          </v-row>
        </v-card-text>
        <v-card-actions class="px-4 pb-3 border-t">
          <v-spacer />
          <v-btn variant="text" @click="showEdit = false">取消</v-btn>
          <v-btn color="primary" variant="flat" :loading="busy" @click="saveInstance(current)">
            保存配置
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <!-- Whitelist Management Modal (Groups & Users) -->
    <v-dialog v-model="showGroups" max-width="580" scrollable>
      <v-card v-if="current" class="modal-card">
        <v-card-title class="d-flex align-center py-3 px-4 border-b">
          <v-icon icon="mdi-shield-account" color="primary" class="mr-2" />
          <span class="font-weight-bold">全域白名单管理：{{ current.name }}</span>
          <v-spacer />
          <v-btn icon="mdi-close" size="small" variant="text" @click="showGroups = false" />
        </v-card-title>
        
        <div class="border-b px-4 pt-1">
          <v-tabs v-model="whitelistTab" color="primary" density="compact">
            <v-tab value="groups">
              <v-icon icon="mdi-account-group" size="18" class="mr-1" />
              群聊白名单 ({{ groupList.length }})
            </v-tab>
            <v-tab value="users">
              <v-icon icon="mdi-account-lock" size="18" class="mr-1" />
              私聊白名单 ({{ userList.length }})
            </v-tab>
          </v-tabs>
        </div>

        <v-card-text class="pa-4">
          <!-- Command Tip Alert -->
          <v-alert
            type="info"
            variant="tonal"
            density="compact"
            class="mb-3 text-caption"
            icon="mdi-slash-forward"
          >
            <strong>指令快捷开关：</strong>机器人账号在任意群或私聊发送 <code>/老吴 开</code> 或 <code>/老吴 关</code> 即可即时热生效！
          </v-alert>

          <!-- Groups Tab -->
          <div v-if="whitelistTab === 'groups'">
            <div class="d-flex gap-2 mb-3">
              <v-text-field
                v-model="addGroupID"
                label="群号"
                placeholder="输入 QQ 群号加入白名单"
                variant="outlined"
                density="compact"
                hide-details
              />
              <v-btn color="primary" variant="flat" :disabled="!addGroupID.trim()" :loading="busy" @click="addGroup">
                添加群
              </v-btn>
            </div>
            <p class="text-caption text-medium-emphasis mb-3">
              机器人仅会在白名单群内响应显式 @ 提问。未加入白名单的群聊将保持绝对静默。
            </p>

            <v-list v-if="groupList.length" density="compact" class="whitelist-group-list">
              <v-list-item
                v-for="g in groupList"
                :key="g.group_id"
                class="border rounded mb-2 px-3 py-2"
              >
                <template #prepend>
                  <v-icon icon="mdi-account-group" size="20" color="primary" class="mr-2" />
                </template>
                <v-list-item-title class="font-weight-bold font-mono text-body-2">
                  {{ g.group_name || `群号: ${g.group_id}` }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-caption text-medium-emphasis">
                  {{ g.group_name ? `群号: ${g.group_id} · ` : '' }}已激活白名单
                </v-list-item-subtitle>
                <template #append>
                  <div class="d-flex align-center gap-1">
                    <v-switch
                      v-model="g.enabled"
                      color="primary"
                      density="compact"
                      hide-details
                      class="mr-2 scale-90"
                      @update:model-value="toggleGroup(g)"
                    />
                    <v-btn
                      icon="mdi-trash-can-outline"
                      size="x-small"
                      variant="text"
                      color="error"
                      @click="removeGroup(g)"
                    />
                  </div>
                </template>
              </v-list-item>
            </v-list>
            <div v-else class="text-center py-6 text-medium-emphasis text-body-2">
              <v-icon icon="mdi-shield-outline" size="32" class="mb-2 opacity-50" />
              <div>暂无白名单群，请在上方输入群号或在群内发送 /老吴 开</div>
            </div>
          </div>

          <!-- Users Tab -->
          <div v-if="whitelistTab === 'users'">
            <div class="d-flex gap-2 mb-3">
              <v-text-field
                v-model="addUserID"
                label="用户 QQ"
                placeholder="输入用户 QQ 号加入私聊白名单"
                variant="outlined"
                density="compact"
                hide-details
              />
              <v-btn color="primary" variant="flat" :disabled="!addUserID.trim()" :loading="busy" @click="addUser">
                添加用户
              </v-btn>
            </div>
            <p class="text-caption text-medium-emphasis mb-3">
              只有在私聊白名单中的用户才能与机器人私聊。非白名单用户的私信将被静默忽略。
            </p>

            <v-list v-if="userList.length" density="compact" class="whitelist-group-list">
              <v-list-item
                v-for="u in userList"
                :key="u.user_qq"
                class="border rounded mb-2 px-3 py-2"
              >
                <template #prepend>
                  <v-icon icon="mdi-account" size="20" color="primary" class="mr-2" />
                </template>
                <v-list-item-title class="font-weight-bold font-mono text-body-2">
                  QQ: {{ u.user_qq }}
                </v-list-item-title>
                <v-list-item-subtitle class="text-caption text-medium-emphasis">
                  私聊授权 · {{ formatDate(u.created_at) }}
                </v-list-item-subtitle>
                <template #append>
                  <div class="d-flex align-center gap-1">
                    <v-switch
                      v-model="u.enabled"
                      color="primary"
                      density="compact"
                      hide-details
                      class="mr-2 scale-90"
                      @update:model-value="toggleUser(u)"
                    />
                    <v-btn
                      icon="mdi-trash-can-outline"
                      size="x-small"
                      variant="text"
                      color="error"
                      @click="removeUser(u)"
                    />
                  </div>
                </template>
              </v-list-item>
            </v-list>
            <div v-else class="text-center py-6 text-medium-emphasis text-body-2">
              <v-icon icon="mdi-account-off-outline" size="32" class="mb-2 opacity-50" />
              <div>暂无私聊白名单用户，请在上方输入 QQ 或在私信中发送 /老吴 开</div>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- Global Snackbar Notification -->
    <v-snackbar v-model="showSnackbar" :color="snackbarColor" timeout="2500" location="top">
      {{ snackbarText }}
    </v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "@/services/api";

const activeTab = ref("audit");
const instances = ref<any[]>([]);

watch(activeTab, (val) => {
  if (val === "studio" && (!current.value || !edit.value.persona)) {
    if (instances.value.length > 0) {
      initEditForm(activeBot.value || instances.value[0]);
    }
  }
});
const audit = ref<any[]>([]);
const accounts = ref<any[]>([]);
const activeSessions = ref<any[]>([]);

const error = ref("");
const busy = ref(false);
const showApiKey = ref(false);
const livePolling = ref(true);
let pollTimer: any = null;

const showCreate = ref(false);
const showEdit = ref(false);
const showGroups = ref(false);
const showPlayground = ref(false);
const showSessionDetail = ref(false);
const showInjectMemory = ref(false);

const createAccountID = ref("");
const addGroupID = ref("");
const addUserID = ref("");
const whitelistTab = ref("groups");
const groupList = ref<any[]>([]);
const userList = ref<any[]>([]);
const current = ref<any>(null);
const activeBot = ref<any>(null);
const activeInspectSession = ref<any>(null);

const searchAudit = ref("");
const searchMemory = ref("");

const expandedTraces = ref<Record<string, boolean>>({});

function toggleTraceExpanded(id: string) {
  expandedTraces.value[id] = !expandedTraces.value[id];
}

function isTraceExpanded(id: string): boolean {
  return !!expandedTraces.value[id];
}

function getToolCallCount(trace: any): number {
  if (!trace || !Array.isArray(trace)) return 0;
  return trace.filter((t: any) => t.type === "tool_call").length;
}

function getLiveProcessingSummary(row: any): string {
  if (row.thought_trace && Array.isArray(row.thought_trace) && row.thought_trace.length > 0) {
    const last = row.thought_trace[row.thought_trace.length - 1];
    if (last.type === "tool_call") {
      return `正在调用工具: ${last.tool} (第 ${last.hop} 步推理)...`;
    }
    if (last.type === "tool_result") {
      return `已完成 ${last.tool}，大模型正在综合推理下一步...`;
    }
    if (last.type === "thinking") {
      return `大模型深度思考中 (第 ${last.hop} 步)...`;
    }
  }
  if (row.tools_called && row.tools_called.length) {
    return `正在执行 MCP 工具调用 (${row.tools_called.length} 个工具)...`;
  }
  return "正在分析意图并挂载 SRA MCP 工具检索情报...";
}

// Playground Chat state
const testUserQQ = ref("");
const testGroupID = ref("");
const inputMsg = ref("");
const chatBusy = ref(false);
const chatViewportRef = ref<HTMLElement | null>(null);
const chatMessages = ref<Array<{ role: "user" | "assistant"; text: string }>>([
  { role: "assistant", text: "老吴~ 喵！我已经准备好了，想查什么情报或者随便聊聊都行喵！" },
]);

const edit = ref<any>({
  name: "老吴",
  meow_sound: "老吴~",
  command_prefix: "/",
  greeting: "喵~ 老吴来啦，有事喊我喵！",
  persona: "",
  context_prompt: "",
  cmd_enable_reply: "",
  cmd_disable_reply: "",
  fallback_reply: "",
  reply_probability: 0,
  entertainment_list: ["老吴~", "喵喵喵？", "抓你裤脚！", "老吴老吴，今天也要开心喵~", "谁在叫我？老吴在！"],
  llm_api_base: "",
  llm_api_key: "",
  llm_model: "deepseek-v4-flash",
  llm_temperature: 0.7,
  llm_max_tokens: 600,
  max_tool_hops: 10,
});

const newCorpusItem = ref("");
const defaultCorpusPresets = [
  "老吴~",
  "喵喵喵？",
  "抓你裤脚！",
  "老吴老吴，今天也要开心喵~",
  "谁在叫我？老吴在！",
  "摸鱼喵！",
  "蹭蹭你喵~",
];

function addCorpusItem() {
  const v = newCorpusItem.value.trim();
  if (!v) return;
  if (!Array.isArray(edit.value.entertainment_list)) {
    edit.value.entertainment_list = [];
  }
  if (!edit.value.entertainment_list.includes(v)) {
    edit.value.entertainment_list.push(v);
  }
  newCorpusItem.value = "";
}

function quickAddCorpus(word: string) {
  if (!Array.isArray(edit.value.entertainment_list)) {
    edit.value.entertainment_list = [];
  }
  if (!edit.value.entertainment_list.includes(word)) {
    edit.value.entertainment_list.push(word);
  }
}

function removeCorpusItem(idx: number) {
  if (Array.isArray(edit.value.entertainment_list)) {
    edit.value.entertainment_list.splice(idx, 1);
  }
}

function resetDefaultEntertainment() {
  edit.value.entertainment_list = [
    "老吴~",
    "喵喵喵？",
    "抓你裤脚！",
    "老吴老吴，今天也要开心喵~",
    "谁在叫我？老吴在！",
  ];
}

const injectForm = ref({
  key: "",
  role: "assistant",
  content: "",
});

const showSnackbar = ref(false);
const snackbarText = ref("");
const snackbarColor = ref("success");

function notify(text: string, color = "success") {
  snackbarText.value = text;
  snackbarColor.value = color;
  showSnackbar.value = true;
}

const runningCount = computed(() => instances.value.filter((i) => i.enabled).length);
const totalWhitelistedGroups = computed(() =>
  instances.value.reduce((acc, cur) => acc + (cur.whitelisted_groups || 0), 0)
);

const processingAuditCount = computed(() =>
  audit.value.filter((r) => r.status === "processing").length
);

const filteredAudit = computed(() => {
  const q = searchAudit.value.trim().toLowerCase();
  if (!q) return audit.value;
  return audit.value.filter(
    (row) =>
      String(row.command || "").toLowerCase().includes(q) ||
      String(row.reply || "").toLowerCase().includes(q) ||
      String(row.user_qq || "").toLowerCase().includes(q) ||
      String(row.group_name || row.group_id || "").toLowerCase().includes(q)
  );
});

const filteredSessions = computed(() => {
  const q = searchMemory.value.trim().toLowerCase();
  if (!q) return activeSessions.value;
  return activeSessions.value.filter(
    (sess) =>
      String(sess.user_qq || "").toLowerCase().includes(q) ||
      String(sess.group_id || "").toLowerCase().includes(q) ||
      String(sess.last_query || "").toLowerCase().includes(q) ||
      String(sess.last_reply || "").toLowerCase().includes(q)
  );
});

function getSessionsForBot(botId: string) {
  return activeSessions.value.filter((s) => s.bot_id === botId);
}

async function loadAll() {
  await Promise.all([loadInstances(), loadAccounts(), loadAudit(), loadSessions()]);
}

function initEditForm(it: any) {
  if (!it) return;
  current.value = it;
  activeBot.value = it;
  let corpus: string[] = [];
  try {
    if (Array.isArray(it.entertainment)) {
      corpus = it.entertainment;
    } else if (typeof it.entertainment === "string") {
      corpus = JSON.parse(it.entertainment);
    }
  } catch {
    corpus = [];
  }
  if (!Array.isArray(corpus)) {
    corpus = [];
  }
  edit.value = {
    name: it.name || "老吴",
    meow_sound: it.meow_sound ?? "老吴~",
    command_prefix: it.command_prefix || "/",
    greeting: it.greeting || "喵~ 老吴来啦，有事喊我喵！",
    persona: it.persona || "",
    context_prompt: it.context_prompt || "",
    cmd_enable_reply: it.cmd_enable_reply || "",
    cmd_disable_reply: it.cmd_disable_reply || "",
    fallback_reply: it.fallback_reply || "",
    reply_probability: it.reply_probability ?? 0,
    entertainment_list: [...corpus],
    llm_api_base: it.llm_api_base || "",
    llm_api_key: it.llm_api_key || "",
    llm_model: it.llm_model || "glm-4.5-air",
    llm_temperature: it.llm_temperature ?? 0.7,
    llm_max_tokens: it.llm_max_tokens ?? 1000000,
    max_tool_hops: it.max_tool_hops ?? 25,
  };
}

async function loadInstances(forceFormInit = false) {
  try {
    const res = await api.get("/api/v1/bot/instances");
    instances.value = res.data.data || [];
    if (instances.value.length > 0) {
      const match = instances.value.find((i: any) => i.id === activeBot.value?.id) || instances.value[0];
      activeBot.value = match;
      if (forceFormInit || !current.value || current.value.id !== match.id) {
        initEditForm(match);
      }
    }
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

async function loadAccounts() {
  try {
    const res = await api.get("/api/v1/accounts");
    accounts.value = res.data.data || [];
  } catch (e: any) {
    // silent
  }
}

async function loadAudit() {
  try {
    const res = await api.get("/api/v1/bot/audit", { params: { limit: 100 } });
    audit.value = res.data.data || [];
  } catch (e: any) {
    // silent
  }
}

async function loadSessions() {
  if (!instances.value.length) return;
  const bot = instances.value[0];
  try {
    const res = await api.get(`/api/v1/bot/instances/${bot.id}/sessions`);
    activeSessions.value = (res.data.data || []).map((s: any) => ({
      ...s,
      user_qq: s.user_id,
    }));
  } catch (e: any) {
    // silent
  }
}

function startLivePolling() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (!livePolling.value) return;
    await Promise.all([loadAudit(), loadSessions()]);
  }, 1500);
}

function accountOptions() {
  const used = new Set(instances.value.map((i) => i.account_id));
  return accounts.value
    .filter((a) => !used.has(a.id))
    .map((a) => ({
      label: `${a.name || "未命名"}（QQ: ${a.qq_uin || a.qq || "无"}）`,
      value: a.id,
    }));
}

function openCreate() {
  createAccountID.value = "";
  showCreate.value = true;
}

async function create() {
  if (!createAccountID.value) return;
  busy.value = true;
  try {
    await api.post("/api/v1/bot/instances", { account_id: createAccountID.value });
    showCreate.value = false;
    createAccountID.value = "";
    notify("机器人创建成功！");
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  } finally {
    busy.value = false;
  }
}

function openEdit(it: any) {
  initEditForm(it);
  showEdit.value = true;
}

function applyPersonaPreset(type: string) {
  if (type === "cat") {
    edit.value.name = "老吴猫";
    edit.value.meow_sound = "老吴~";
    edit.value.persona = "你是一只住在 QQ 群里的小猫咪，名字叫'老吴'。你说话简短、俏皮、粘人，喜欢在句尾加'老吴~'或'喵'。你拥有查阅社交情报数据库与全网实时搜索的能力。";
    edit.value.context_prompt = "请以符合你小猫咪人设的活泼口吻进行自然语言纯文本回答（不要输出Markdown加粗符号**）。\n【工具调用指引】\n1. 外部实时资讯/天气/时事/常识：主动调用 web_search 联网搜索；\n2. 图片识别与截屏分析：当群消息中包含 [附带图片URL: ...] 或用户要求识别/看图时，主动调用 sra_trigger_vision_analysis 解析图中内容与场景意图；\n3. 人物画像与性格分析：当用户要求研判特定人/QQ时，主动调用 sra_person_persona 或 sra_feed_dynamics；\n4. 群聊话题与争论总结：当用户询问群里在聊什么或发生了什么，调用 sra_trigger_dialogue_disentanglement 或 sra_dialogue_threads；\n5. 社交图谱与查人查记录：调用 sra_search, sra_person_dossier, sra_ego_network 或 sra_message_history。\n若工具未查到结果，请以萌态口吻自然回复说明。";
    edit.value.cmd_enable_reply = "老吴~ 喵！当前{type}【{target}】已加入白名单并开启服务！";
    edit.value.cmd_disable_reply = "老吴~ 喵！当前{type}【{target}】已关闭服务，老吴去睡觉啦~ Zzz (发送 /老吴 开 即可重新唤醒)";
    edit.value.fallback_reply = "喵呜……老吴脑子转不过来了，歇会儿再问我吧喵~";
  } else if (type === "osint") {
    edit.value.name = "SRA情报官";
    edit.value.meow_sound = "";
    edit.value.persona = "你是 SRA 社交网络与情报分析系统的专属 AI 助理。你的回复风格客观、严谨、结构清晰，专注于事实证据与多维度线索分析。";
    edit.value.context_prompt = "当用户查询实体时，优先调用 sra_resolve 解析唯一 ID，再调用 sra_person_dossier / sra_ego_network 获取深层图谱。查询外部公开事实或最新资讯时使用 web_search。严禁在没有证据时捏造事实。";
    edit.value.cmd_enable_reply = "【系统通知】当前{type}【{target}】情报分析助理服务已启动上线。";
    edit.value.cmd_disable_reply = "【系统通知】当前{type}【{target}】情报分析助理服务已暂停响应。";
    edit.value.fallback_reply = "系统数据通路发生波动，请稍后重试或联系系统管理员。";
  } else if (type === "assistant") {
    edit.value.name = "小吴助手";
    edit.value.meow_sound = "";
    edit.value.persona = "你是一个专业、礼貌、高效的群聊与个人 AI 助手。你善于倾听、条理分明，乐于协助解答各种问题。";
    edit.value.context_prompt = "请用亲切大方、通俗易懂的语言回答。涉及社交网络查询时使用相应工具，涉及外部实时资讯或网络知识时主动使用 web_search 搜索。";
    edit.value.cmd_enable_reply = "您好！当前{type}【{target}】的 AI 助手服务已开启，随时为您服务。";
    edit.value.cmd_disable_reply = "当前{type}【{target}】的 AI 助手服务已关闭。需要时随时发送 /老吴 开 重新开启。";
    edit.value.fallback_reply = "服务响应稍有延迟，请稍后再次向我提问~";
  }
  notify("已载入预设提示词");
}

function applyMCPTemplate(type: string) {
  if (type === "all_in_one") {
    edit.value.context_prompt = `【MCP 全能社交情报与全网搜索指引】：
1. 社交网络与图谱情报：
   - 涉及人物身份、曾用名、社交账号、特征标签查询时：必须先调用 sra_resolve 解析唯一 ID，再调用 sra_person_dossier 或 sra_ego_network 获取深层拓扑。
   - 涉及人物之间是否存在共同好友、关联路径或二度关系时：调用 sra_find_path 或 sra_mutual_analysis。
   - 涉及群聊情报与历史发言溯源时：调用 sra_group_intel、sra_message_history 或 sra_raw_evidence。
2. 多模态视觉与图像识别：
   - 当消息中包含「[附带图片URL:」或「[被引用消息内容:」且内含图片URL/文件时，你必须立即调用 sra_trigger_vision_analysis 工具（传入 url 参数）进行视觉识别，基于识别结果回答。
3. 全网实时知识与即时资讯：
   - 当用户询问实时天气、时政新闻、公共百科、股票汇率、技术文档等外部知识时，主动调用 web_search 联网搜索。
4. 证据链原则：查无结果时诚实说明事实边界，严禁在缺乏真实证据时凭空捏造人际关系。`;
  } else if (type === "vision_specialist") {
    edit.value.context_prompt = `【多模态视觉识别与图文分析规则】：
1. 视觉识别强制触发：只要用户消息中包含图片URL、表情包、本地图片路径或引用了包含图片的历史消息，第一步必须调用 sra_trigger_vision_analysis 工具。
2. 识别结果处理：
   - 详细解读画面元素、人物表情、场景环境与 OCR 提取的文字内容。
   - 若为梗图或表情包，解释其幽默点与群聊语境意义。
   - 若为聊天记录截图或支付单据，提取关键时间、金额、对话双方与重要线索。
3. 结合社交情报：若图片涉及群内人物，可联动 sra_search 或 sra_resolve 进行关联验证。`;
  } else if (type === "web_search_expert") {
    edit.value.context_prompt = `【全网实时搜索与即时问答指引】：
1. 联网搜索触发时机：
   - 询问今日/近期新闻、突发事件、热点话题。
   - 询问各地实时天气预报、路况、航班车次。
   - 询问专业技术文档、最新开源项目、学术论文或冷门词条。
2. 搜索调度规则：调用 web_search 传入精准关键词（必要时组合年份或具体限定词）。
3. 结构化输出：对搜索返回结果进行提炼归纳，分点回答，确保事实准确、时效性强。`;
  } else if (type === "group_analyst") {
    edit.value.context_prompt = `【群聊生态与互动关系分析规则】：
1. 群聊话题与动态分析：
   - 调用 sra_group_intel 获取群聊基础画像与活跃概况。
   - 调用 sra_interaction_stream 或 sra_dialogue_threads 分析近期群内热议话题与互动频率。
2. 亲密圈层与社交关系：
   - 分析成员互动亲密度时，调用 sra_mutual_analysis 与 sra_qzone_connections。
3. 回答风格：客观、宏观、条理清晰，洞察群聊氛围演变与社交拓扑核心节点。`;
  } else if (type === "friendly_companion") {
    edit.value.context_prompt = `【友好陪伴与日常互动指引】：
1. 互动定位：专注于日常趣味闲聊、生活关怀、情感陪伴与幽默接梗。
2. 视觉识别：收到图片或表情包时调用 sra_trigger_vision_analysis，以可爱生动的语气与用户互动。
3. 隐私与安全底线：不主动针对普通群成员进行深度个人隐私溯源，涉及敏感查询时以温和幽默的方式婉拒或做合规提醒。`;
  }
  notify("已载入 MCP 工具调度指引模板");
}

async function saveInstance(it?: any) {
  const target = it || current.value || activeBot.value || instances.value[0];
  if (!target || !target.id) {
    error.value = "未找到可保存的机器人实例，请先创建或选择机器人";
    return;
  }
  busy.value = true;
  try {
    const body: any = {
      name: edit.value.name,
      meow_sound: edit.value.meow_sound,
      command_prefix: edit.value.command_prefix,
      greeting: edit.value.greeting,
      persona: edit.value.persona,
      context_prompt: edit.value.context_prompt,
      cmd_enable_reply: edit.value.cmd_enable_reply,
      cmd_disable_reply: edit.value.cmd_disable_reply,
      fallback_reply: edit.value.fallback_reply,
      reply_probability: Number(edit.value.reply_probability || 0),
      entertainment: edit.value.entertainment_list || [],
      llm_api_base: edit.value.llm_api_base,
      llm_api_key: edit.value.llm_api_key,
      llm_model: edit.value.llm_model,
      llm_temperature: Number(edit.value.llm_temperature),
      llm_max_tokens: Number(edit.value.llm_max_tokens),
      max_tool_hops: Number(edit.value.max_tool_hops || 10),
    };
    await api.patch(`/api/v1/bot/instances/${target.id}`, body);
    showEdit.value = false;
    notify("人设、隐私铁律与提示词配置已成功保存并实时生效！");
    await loadInstances(true);
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  } finally {
    busy.value = false;
  }
}

async function toggleEnabled(it: any) {
  busy.value = true;
  try {
    await api.patch(`/api/v1/bot/instances/${it.id}`, { enabled: it.enabled });
    notify(it.enabled ? "机器人已启动上线" : "机器人已暂停服务");
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
    it.enabled = !it.enabled;
  } finally {
    busy.value = false;
  }
}

async function removeInstance(it: any) {
  if (!confirm(`确认删除机器人【${it.name}】吗？`)) return;
  busy.value = true;
  try {
    await api.delete(`/api/v1/bot/instances/${it.id}`);
    notify("机器人已删除", "info");
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  } finally {
    busy.value = false;
  }
}

async function openGroups(it: any) {
  current.value = it;
  try {
    const [gRes, uRes] = await Promise.all([
      api.get(`/api/v1/bot/instances/${it.id}/groups`),
      api.get(`/api/v1/bot/instances/${it.id}/users`),
    ]);
    groupList.value = gRes.data.data || [];
    userList.value = uRes.data.data || [];
    showGroups.value = true;
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

async function addGroup() {
  const gid = addGroupID.value.trim();
  if (!gid) return;
  busy.value = true;
  try {
    await api.post(`/api/v1/bot/instances/${current.value.id}/groups`, { group_id: gid });
    addGroupID.value = "";
    notify("已加入白名单群");
    await openGroups(current.value);
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  } finally {
    busy.value = false;
  }
}

async function toggleGroup(g: any) {
  try {
    await api.post(`/api/v1/bot/instances/${current.value.id}/groups`, {
      group_id: g.group_id,
      enabled: g.enabled,
    });
    notify(g.enabled ? "群已启用" : "群已暂停响应");
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

async function removeGroup(g: any) {
  try {
    await api.delete(`/api/v1/bot/instances/${current.value.id}/groups/${encodeURIComponent(g.group_id)}`);
    notify("已移出白名单");
    await openGroups(current.value);
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

async function addUser() {
  const uid = addUserID.value.trim();
  if (!uid) return;
  busy.value = true;
  try {
    await api.post(`/api/v1/bot/instances/${current.value.id}/users`, { user_qq: uid });
    addUserID.value = "";
    notify("已加入私聊白名单");
    await openGroups(current.value);
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  } finally {
    busy.value = false;
  }
}

async function toggleUser(u: any) {
  try {
    await api.post(`/api/v1/bot/instances/${current.value.id}/users`, {
      user_qq: u.user_qq,
      enabled: u.enabled,
    });
    notify(u.enabled ? "私聊已启用" : "私聊已暂停响应");
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

async function removeUser(u: any) {
  try {
    await api.delete(`/api/v1/bot/instances/${current.value.id}/users/${encodeURIComponent(u.user_qq)}`);
    notify("已移出私聊白名单");
    await openGroups(current.value);
    await loadAll();
  } catch (e: any) {
    error.value = e?.response?.data?.error || String(e?.message || e);
  }
}

function openMemoryManager(it: any) {
  activeBot.value = it;
  activeTab.value = "memory";
}

function inspectSession(sess: any) {
  activeInspectSession.value = sess;
  showSessionDetail.value = true;
}

function openInjectMemory(sess: any) {
  if (sess) {
    injectForm.value.key = sess.key;
  } else if (instances.value.length) {
    injectForm.value.key = `p:${instances.value[0].id}:${testUserQQ.value}`;
  }
  injectForm.value.content = "";
  showInjectMemory.value = true;
}

async function submitInjectMemory() {
  if (!injectForm.value.key || !injectForm.value.content) return;
  busy.value = true;
  try {
    const bot = instances.value[0];
    await api.post(`/api/v1/bot/instances/${bot.id}/sessions/inject`, injectForm.value);
    notify("记忆偏好已成功注入！");
    showInjectMemory.value = false;
    await loadSessions();
  } catch (e: any) {
    notify(e?.response?.data?.error || e.message, "error");
  } finally {
    busy.value = false;
  }
}

async function deleteSession(sess: any) {
  if (!instances.value.length) return;
  const bot = instances.value[0];
  try {
    await api.delete(`/api/v1/bot/instances/${bot.id}/sessions`, {
      params: { key: sess.key },
    });
    notify("该会话记忆已清空");
    await loadSessions();
  } catch (e: any) {
    notify(e?.response?.data?.error || e.message, "error");
  }
}

async function clearAllMemory() {
  if (!confirm("确认清空所有用户的会话上下文记忆吗？")) return;
  for (const s of activeSessions.value) {
    await deleteSession(s);
  }
  notify("全部会话记忆已清空");
}

function openPlayground(it: any) {
  activeBot.value = it;
  showPlayground.value = true;
  scrollToBottom();
}

function sendPreset(text: string) {
  inputMsg.value = text;
  sendChat();
}

async function sendChat() {
  const text = inputMsg.value.trim();
  if (!text || chatBusy.value || !activeBot.value) return;

  chatMessages.value.push({ role: "user", text });
  inputMsg.value = "";
  chatBusy.value = true;
  scrollToBottom();

  try {
    const res = await api.post(`/api/v1/bot/instances/${activeBot.value.id}/chat`, {
      message: text,
      user_id: testUserQQ.value || "web_tester",
      group_id: testGroupID.value || "",
    });
    chatMessages.value.push({
      role: "assistant",
      text: res.data.reply || "老吴~ 喵！",
    });
    await Promise.all([loadAudit(), loadSessions()]);
  } catch (e: any) {
    chatMessages.value.push({
      role: "assistant",
      text: `喵呜... 大脑走神了（${e?.response?.data?.error || e.message}）`,
    });
  } finally {
    chatBusy.value = false;
    scrollToBottom();
  }
}

async function clearChatMemory() {
  if (!activeBot.value) return;
  chatBusy.value = true;
  try {
    await api.delete(`/api/v1/bot/instances/${activeBot.value.id}/session`, {
      params: {
        user_id: testUserQQ.value || "web_tester",
        group_id: testGroupID.value || "",
      },
    });
    chatMessages.value = [
      { role: "assistant", text: "老吴~ 喵！会话记忆已重置，我们重新开始吧！" },
    ];
    notify("测试会话记忆已清空！");
    await loadSessions();
  } catch (e: any) {
    notify(e?.response?.data?.error || e.message, "error");
  } finally {
    chatBusy.value = false;
  }
}

function copyText(txt: string) {
  if (!txt) return;
  navigator.clipboard.writeText(txt);
  notify("已复制到剪贴板");
}

function formatDate(iso: string) {
  if (!iso) return "-";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function scrollToBottom() {
  nextTick(() => {
    if (chatViewportRef.value) {
      chatViewportRef.value.scrollTop = chatViewportRef.value.scrollHeight;
    }
  });
}

onMounted(() => {
  loadAll();
  startLivePolling();
});

onUnmounted(() => {
  if (pollTimer) clearInterval(pollTimer);
});
</script>

<style scoped>
.bot-dashboard {
  display: flex;
  flex-direction: column;
  gap: 1.25rem;
  max-width: 1440px;
  margin: 0 auto;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 1rem;
}

.eyebrow {
  font-size: 0.725rem;
  font-weight: 700;
  letter-spacing: 0;
  color: var(--v-theme-primary);
  margin-bottom: 0.25rem;
}

.page-header h1 {
  font-size: 1.75rem;
  font-weight: 800;
  margin: 0 0 0.25rem 0;
  letter-spacing: 0;
}

.page-header p {
  color: rgba(var(--v-theme-on-surface), 0.7);
  font-size: 0.875rem;
  margin: 0;
}

/* KPI Runtime Metrics */
.runtime-metrics {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 0.875rem;
}

.runtime-metric {
  background: rgba(var(--v-theme-surface), 0.7);
  backdrop-filter: blur(12px);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  padding: 0.875rem 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  transition: all 0.2s ease;
}

.runtime-metric:hover {
  border-color: rgba(var(--v-theme-primary), 0.35);
  transform: translateY(-2px);
}

.runtime-metric span {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.65);
}

.runtime-metric strong {
  font-size: 1.35rem;
  font-weight: 800;
  letter-spacing: 0;
}

.runtime-metric em {
  font-size: 0.7rem;
  color: rgba(var(--v-theme-on-surface), 0.5);
  font-style: normal;
}

/* Master Hero Grid */
.bot-hero-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
  gap: 1.25rem;
}

.agent-master-card {
  background: rgba(var(--v-theme-surface), 0.85);
  backdrop-filter: blur(16px);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  border-radius: 8px;
  padding: 1.25rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.04);
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.agent-master-card.is-active {
  border-color: rgba(var(--v-theme-primary), 0.4);
  box-shadow: 0 6px 24px rgba(var(--v-theme-primary), 0.08);
}

.agent-card-header {
  display: flex;
  align-items: center;
  gap: 0.875rem;
}

.avatar-box {
  position: relative;
  width: 52px;
  height: 52px;
  border-radius: 8px;
  background: rgba(var(--v-theme-primary), 0.1);
  display: flex;
  align-items: center;
  justify-content: center;
}

.status-dot {
  position: absolute;
  bottom: -2px;
  right: -2px;
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 2px solid rgb(var(--v-theme-surface));
}

.status-dot.online {
  background: #10b981;
  box-shadow: 0 0 8px #10b981;
}

.status-dot.offline {
  background: #94a3b8;
}

.agent-name {
  font-size: 1.15rem;
  font-weight: 800;
  margin: 0;
}

.agent-persona-box {
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px dashed rgba(var(--v-theme-on-surface), 0.12);
  border-radius: 8px;
  padding: 0.75rem;
}

.persona-label {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.7);
  display: flex;
  align-items: center;
  margin-bottom: 0.35rem;
}

.persona-text {
  font-size: 0.825rem;
  line-height: 1.45;
  color: rgba(var(--v-theme-on-surface), 0.85);
}

.agent-features-strip {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 0.5rem;
  background: rgba(var(--v-theme-surface), 0.5);
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
}

.feat-item {
  display: flex;
  flex-direction: column;
}

.feat-label {
  font-size: 0.675rem;
  color: rgba(var(--v-theme-on-surface), 0.5);
}

.feat-value {
  font-size: 0.8rem;
  font-weight: 700;
}

.agent-card-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.5rem;
  border-top: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  padding-top: 0.875rem;
}

/* Surface Container */
.data-surface {
  background: rgba(var(--v-theme-surface), 0.85);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.03);
  overflow: hidden;
}

/* Audit Stream */
.audit-stream-list {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.audit-item-row {
  background: rgba(var(--v-theme-surface), 0.6);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  padding: 0.875rem 1rem;
  display: grid;
  grid-template-columns: 220px 1fr;
  gap: 1.25rem;
  align-items: flex-start;
  transition: all 0.2s ease;
}

.audit-item-row.is-processing {
  border-color: rgba(var(--v-theme-warning), 0.5);
  background: rgba(var(--v-theme-warning), 0.03);
}

.audit-item-row.is-failed {
  border-color: rgba(var(--v-theme-error), 0.3);
}

.audit-meta-col {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.audit-time {
  font-size: 0.725rem;
  font-family: monospace;
  color: rgba(var(--v-theme-on-surface), 0.6);
}

.audit-source {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 0.25rem;
}

.user-tag {
  font-size: 0.75rem;
  color: rgba(var(--v-theme-on-surface), 0.8);
}

.audit-dialogue-col {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
}

.query-bubble {
  background: rgba(var(--v-theme-on-surface), 0.05);
  border-radius: 8px;
  padding: 0.4rem 0.75rem;
  font-size: 0.85rem;
  color: rgba(var(--v-theme-on-surface), 0.9);
  display: flex;
  align-items: center;
}

.processing-placeholder-bubble {
  background: rgba(var(--v-theme-primary), 0.08);
  border: 1px dashed rgba(var(--v-theme-primary), 0.3);
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  display: flex;
  align-items: center;
}

.reply-bubble {
  background: rgba(var(--v-theme-primary), 0.08);
  border-radius: 8px;
  padding: 0.6rem 0.875rem;
  font-size: 0.875rem;
  line-height: 1.5;
  color: rgba(var(--v-theme-on-surface), 0.95);
  position: relative;
}

.reply-bubble.is-error {
  background: rgba(var(--v-theme-error), 0.08);
  border: 1px solid rgba(var(--v-theme-error), 0.2);
}

.copy-btn {
  position: absolute;
  top: 4px;
  right: 4px;
}

/* Thought Trace & Streaming */
.thought-trace-panel {
  display: flex;
  flex-direction: column;
}

.toggle-trace-btn {
  align-self: flex-start;
  text-transform: none;
  font-size: 0.75rem;
}

.thought-steps-list {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  background: rgba(var(--v-theme-surface-variant), 0.25);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  padding: 0.625rem 0.75rem;
}

.thought-step-card {
  background: rgba(var(--v-theme-surface), 0.85);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 6px;
  padding: 0.5rem 0.625rem;
  font-size: 0.8rem;
}

.thought-step-card.type-tool_call {
  border-left: 3px solid rgb(var(--v-theme-primary));
}

.thought-step-card.type-tool_result {
  border-left: 3px solid #10b981;
  background: rgba(16, 185, 129, 0.03);
}

.thought-step-card.type-thinking {
  border-left: 3px solid #a855f7;
  background: rgba(168, 85, 247, 0.03);
}

.step-thought-content {
  color: rgba(var(--v-theme-on-surface), 0.85);
  line-height: 1.4;
  white-space: pre-wrap;
  word-break: break-word;
}

.step-args-content {
  background: rgba(var(--v-theme-on-surface), 0.04);
  border-radius: 4px;
  padding: 0.25rem 0.5rem;
  color: rgba(var(--v-theme-on-surface), 0.75);
  word-break: break-all;
}

.step-result-content {
  background: rgba(var(--v-theme-on-surface), 0.04);
  border-radius: 4px;
  padding: 0.25rem 0.5rem;
  color: rgba(var(--v-theme-on-surface), 0.8);
  word-break: break-all;
}

.spin-icon {
  animation: spin 2s linear infinite;
}

@keyframes spin {
  100% { transform: rotate(360deg); }
}


/* Memory Sessions Grid */
.memory-sessions-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
  gap: 1rem;
}

.session-card {
  background: rgba(var(--v-theme-surface), 0.6);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  border-radius: 8px;
  padding: 0.875rem 1rem;
  display: flex;
  flex-direction: column;
  transition: all 0.2s ease;
}

.session-card:hover {
  border-color: rgba(var(--v-theme-primary), 0.35);
  transform: translateY(-2px);
}

.session-preview-box {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  background: rgba(var(--v-theme-on-surface), 0.03);
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
}

.preview-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8rem;
}

.preview-role {
  color: rgba(var(--v-theme-on-surface), 0.5);
  font-size: 0.725rem;
  flex-shrink: 0;
}

.preview-text {
  flex: 1;
}

/* Sandbox Playground */
.playground-body {
  display: flex;
  flex-direction: column;
  height: 520px;
}

.sandbox-context-bar {
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.chat-viewport {
  flex: 1;
  overflow-y: auto;
  background: rgba(var(--v-theme-background), 0.6);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 0.875rem;
}

.chat-bubble-row {
  display: flex;
  gap: 0.75rem;
  max-width: 85%;
}

.chat-bubble-row.user {
  align-self: flex-end;
  flex-direction: row-reverse;
}

.chat-bubble-row.assistant {
  align-self: flex-start;
}

.content-col {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.chat-bubble-row.user .content-col {
  align-items: flex-end;
}

.sender-name {
  font-size: 0.7rem;
  color: rgba(var(--v-theme-on-surface), 0.6);
}

.bubble-text {
  background: rgba(var(--v-theme-surface), 0.95);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  border-radius: 8px;
  padding: 0.625rem 0.875rem;
  font-size: 0.875rem;
  line-height: 1.5;
  white-space: pre-wrap;
  word-break: break-word;
}

.chat-bubble-row.user .bubble-text {
  background: rgb(var(--v-theme-primary));
  color: rgb(var(--v-theme-on-primary));
  border-color: rgb(var(--v-theme-primary));
}

.context-bubble-stream {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.context-msg-row {
  background: rgba(var(--v-theme-surface), 0.7);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  border-radius: 8px;
  padding: 0.625rem 0.875rem;
}

.context-msg-row.user {
  border-left: 3px solid rgb(var(--v-theme-primary));
}

.context-msg-row.assistant {
  border-left: 3px solid #10b981;
}

.msg-bubble-content {
  font-size: 0.85rem;
  line-height: 1.5;
  white-space: pre-wrap;
}

.pulsing-live-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 6px #10b981;
  animation: pulse-ring 1.5s infinite;
}

@keyframes pulse-ring {
  0% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16, 185, 129, 0.7); }
  70% { transform: scale(1.1); box-shadow: 0 0 0 6px rgba(16, 185, 129, 0); }
  100% { transform: scale(0.95); box-shadow: 0 0 0 0 rgba(16, 185, 129, 0); }
}

.scale-90 {
  transform: scale(0.9);
}

.text-truncate-2 {
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

.clickable-chip {
  cursor: pointer;
  transition: all 0.15s ease;
}

.clickable-chip:hover {
  background: rgba(var(--v-theme-primary), 0.15);
}

@media (max-width: 768px) {
  .audit-item-row {
    grid-template-columns: 1fr;
  }
}
</style>

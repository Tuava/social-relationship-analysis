<template>
  <div class="page-wrap records-page settings-page">
    <!-- 顶部状态栏与操作 -->
    <header class="page-header records-header">
      <div class="records-heading">
        <h1 class="text-h5 font-weight-bold mb-1">系统设置与控制台</h1>
        <span class="text-caption text-medium-emphasis">AI 认知引擎、提示词工程、多模态视觉、采集频控与全局偏好配置</span>
      </div>
      <div class="records-header-actions d-flex align-center gap-2">
        <v-chip size="small" :color="health === 'ok' ? 'success' : 'error'" variant="tonal">
          {{ health === 'ok' ? '后端在线' : '服务异常' }}
        </v-chip>
        <v-btn
          color="primary"
          variant="tonal"
          size="small"
          prepend-icon="mdi-lightning-bolt"
          :loading="testingLLM"
          @click="testLLMConnection"
        >
          测试 LLM 连通性
        </v-btn>
        <v-btn icon="mdi-refresh" variant="text" size="small" :loading="loading" title="重新读取配置" @click="loadAll" />
      </div>
    </header>

    <!-- 错误与测试回执通知 -->
    <v-alert v-if="pageError" type="error" variant="tonal" class="mb-3" closable @click:close="pageError = ''">
      {{ pageError }}
    </v-alert>

    <v-alert
      v-if="testResult"
      :type="testResult.status === 'ok' ? 'success' : 'error'"
      variant="tonal"
      density="compact"
      class="mb-3"
      closable
      @click:close="testResult = null"
    >
      <div class="d-flex align-center justify-space-between flex-wrap gap-2">
        <div>
          <strong>{{ testResult.status === 'ok' ? '大模型连通测试成功' : '大模型连通测试失败' }}</strong>
          <span class="ml-2 text-caption">模型: {{ testResult.model || form['ai.llm_model'] }}</span>
          <span v-if="testResult.latency_ms" class="ml-2 text-caption font-weight-bold">
            时延: {{ testResult.latency_ms }} ms
          </span>
        </div>
        <span class="text-caption">{{ testResult.reply || testResult.error }}</span>
      </div>
    </v-alert>

    <!-- 主配置面板布局 (垂直选项卡 + 分组卡片) -->
    <div class="settings-container">
      <v-card variant="outlined" class="settings-card data-surface">
        <div class="settings-grid-layout">
          <!-- 左侧导航选项卡 -->
          <nav class="settings-nav">
            <v-tabs
              v-model="activeTab"
              direction="vertical"
              color="primary"
              density="comfortable"
              class="settings-tabs"
            >
              <v-tab value="ai" prepend-icon="mdi-brain">AI 认知引擎</v-tab>
              <v-tab value="prompts" prepend-icon="mdi-script-text-outline">提示词工程</v-tab>
              <v-tab value="vision" prepend-icon="mdi-image-filter-center-focus">多模态视觉</v-tab>
              <v-tab value="qzone" prepend-icon="mdi-orbit">空间与圈层</v-tab>
              <v-tab value="bot" prepend-icon="mdi-robot-outline">机器人策略</v-tab>
              <v-tab value="media" prepend-icon="mdi-folder-multiple-image">媒体与存储</v-tab>
              <v-tab value="crawler" prepend-icon="mdi-shield-bug-outline">采集与风控</v-tab>
              <v-tab value="ui" prepend-icon="mdi-palette-outline">界面与偏好</v-tab>
              <v-tab value="status" prepend-icon="mdi-connection">连接与状态</v-tab>
            </v-tabs>
          </nav>

          <!-- 右侧表单内容区 -->
          <main class="settings-content pa-4">
            <v-window v-model="activeTab">
              <!-- 1. AI 认知与画像引擎 -->
              <v-window-item value="ai">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-brain" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">AI 认知研判与大模型接入</h3>
                      <small class="text-caption text-medium-emphasis">用于全息画像研判、事实边界抽取、关系挖掘与智能问答</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12" md="6">
                      <v-select
                        v-model="form['ai.llm_provider']"
                        label="大模型服务商快捷预设"
                        :items="providerOptions"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                        @update:model-value="onProviderSelect"
                      />
                    </v-col>

                    <v-col cols="12" md="6">
                      <v-combobox
                        v-model="form['ai.llm_model']"
                        label="研判模型名称"
                        :items="modelSuggestions"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                        placeholder="例如: glm-4.5-air / deepseek-chat"
                      />
                    </v-col>

                    <v-col cols="12">
                      <v-text-field
                        v-model.trim="form['ai.llm_api_base']"
                        label="API Base URL (端点路径)"
                        placeholder="https://open.bigmodel.cn/api/paas/v4"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                        prepend-inner-icon="mdi-link-variant"
                      />
                    </v-col>

                    <v-col cols="12">
                      <v-text-field
                        v-model.trim="form['ai.llm_api_key']"
                        label="API Key (访问密钥)"
                        :type="showApiKey ? 'text' : 'password'"
                        :append-inner-icon="showApiKey ? 'mdi-eye-off' : 'mdi-eye'"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                        prepend-inner-icon="mdi-key-outline"
                        placeholder="填入您的 API Key..."
                        @click:append-inner="showApiKey = !showApiKey"
                      />
                      <small class="text-caption text-grey ml-1">
                        {{ form['ai.llm_api_key']?.includes('***') ? '密钥已保存在后端 (已脱敏掩码)' : '输入新的密钥将覆盖保存' }}
                      </small>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <div class="slider-control pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">单次研判历史消息深度</strong>
                          <v-chip size="x-small" color="primary">{{ form['ai.context_depth'] || 150 }} 条</v-chip>
                        </div>
                        <v-slider
                          v-model="form['ai.context_depth']"
                          :min="50"
                          :max="500"
                          :step="10"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">喂给大模型的上下文条数，建议 100~200 条以平衡 Token 与完整性</small>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <v-text-field
                        v-model.number="form['ai.persona_input_char_budget']"
                        type="number"
                        min="1000"
                        label="单次画像输入字符预算"
                        suffix="字符"
                        variant="outlined"
                        density="compact"
                        hint="系统会在完整时间跨度内均匀取样，避免单次请求触发 Token 速率限制"
                        persistent-hint
                      />
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <div class="slider-control pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">模型发散度 (Temperature)</strong>
                          <v-chip size="x-small" color="primary">{{ form['ai.temperature'] ?? 0.1 }}</v-chip>
                        </div>
                        <v-slider
                          v-model="form['ai.temperature']"
                          :min="0.0"
                          :max="1.0"
                          :step="0.05"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">刑侦级画像必须严谨，推荐保持在 0.0 ~ 0.2 之间</small>
                      </div>
                    </v-col>

                    <!-- 同模型并发队列控制 -->
                    <v-col cols="12" md="6" class="mt-3">
                      <div class="slider-control pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">同模型最大并发请求数 (Queue Limit)</strong>
                          <v-chip size="x-small" color="primary">{{ form['ai.max_concurrency'] || 2 }} 个并发槽位</v-chip>
                        </div>
                        <v-slider
                          v-model="form['ai.max_concurrency']"
                          :min="1"
                          :max="10"
                          :step="1"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">同模型同时发起的请求上限。超出部分进入 FIFO 队列排队，杜绝触发供应商 429 频控</small>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <div class="slider-control pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">模型排队最大等待超时</strong>
                          <v-chip size="x-small" color="primary">{{ form['ai.queue_timeout_seconds'] || 180 }} 秒</v-chip>
                        </div>
                        <v-slider
                          v-model="form['ai.queue_timeout_seconds']"
                          :min="30"
                          :max="600"
                          :step="10"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">请求在 FIFO 队列中排队等待空闲槽位的最大时限</small>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <v-text-field
                        v-model.number="form['ai.min_request_interval_ms']"
                        type="number"
                        min="0"
                        step="100"
                        label="同模型请求最小间隔"
                        suffix="毫秒"
                        variant="outlined"
                        density="compact"
                        hint="控制连续请求节奏；并发为 1 也会生效，0 表示关闭"
                        persistent-hint
                      />
                    </v-col>

                    <v-col cols="12" md="6" class="mt-3">
                      <v-text-field
                        v-model.number="form['ai.rate_limit_backoff_seconds']"
                        type="number"
                        min="0"
                        step="5"
                        label="429/503 默认冷却时间"
                        suffix="秒"
                        variant="outlined"
                        density="compact"
                        hint="服务商没有 Retry-After 时使用；冷却期间新请求会排队"
                        persistent-hint
                      />
                    </v-col>

                    <!-- 实时队列监控卡片 -->
                    <v-col cols="12" class="mt-2" v-if="queueStats?.length">
                      <div class="pa-3 rounded border bg-surface-variant">
                        <div class="d-flex align-center justify-space-between mb-2">
                          <div class="d-flex align-center">
                            <v-icon icon="mdi-tray-full" size="18" class="mr-2 text-primary" />
                            <strong class="text-subtitle-2">模型并发队列实时状态</strong>
                          </div>
                          <v-btn size="x-small" variant="text" prepend-icon="mdi-refresh" @click="loadAll">刷新队列</v-btn>
                        </div>
                        <div v-for="q in queueStats" :key="q.model_key" class="d-flex align-center justify-space-between py-1 border-b">
                          <div>
                            <code class="text-caption font-weight-bold text-primary">{{ q.model_key }}</code>
                            <span class="text-caption text-grey ml-2">累计处理: {{ q.total_processed }} 次</span>
                          </div>
                          <div class="d-flex align-center">
                            <v-chip size="x-small" :color="q.active_requests > 0 ? 'warning' : 'success'" class="mr-1">
                              活跃运行: {{ q.active_requests }}/{{ q.max_concurrency }}
                            </v-chip>
                            <v-chip size="x-small" :color="q.queued_requests > 0 ? 'error' : 'grey'" variant="outlined">
                              排队等待: {{ q.queued_requests }}
                            </v-chip>
                          </div>
                        </div>
                      </div>
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">反张冠李戴严格模式</strong>
                          <small class="text-caption text-medium-emphasis">严格区分第一人称自述与第三方调侃/反问，杜绝把他人特征扣在目标头上</small>
                        </div>
                        <v-switch v-model="form['ai.strict_mode']" color="primary" hide-details inset />
                      </div>
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <v-textarea
                        v-model="form['ai.custom_prompt']"
                        label="自定义附加系统提示词 (可选)"
                        placeholder="可在此输入额外的研判要求，例如：重点关注特定暗语与设备..."
                        variant="outlined"
                        density="compact"
                        rows="2"
                        hide-details="auto"
                      />
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 2. 提示词工程 (Prompts) -->
              <v-window-item value="prompts">
                <section class="config-section">
                  <div class="section-title d-flex justify-space-between align-center">
                    <div class="d-flex align-center">
                      <v-icon icon="mdi-script-text-outline" size="20" class="mr-2 text-primary" />
                      <div>
                        <h3 class="text-subtitle-1 font-weight-bold mb-0">系统提示词工程 (System Prompts)</h3>
                        <small class="text-caption text-medium-emphasis">直接查看、调整或修改各认知分析模块的完整 System Prompt，支持一键恢复官方默认</small>
                      </div>
                    </div>
                    <v-menu>
                      <template #activator="{ props }">
                        <v-btn size="small" variant="tonal" color="primary" v-bind="props" prepend-icon="mdi-magic-staff">
                          载入画像预设模板
                        </v-btn>
                      </template>
                      <v-list density="compact">
                        <v-list-item @click="resetPersonaPrompt">
                          <template #prepend><v-icon icon="mdi-shield-account" color="primary" size="18" /></template>
                          <v-list-item-title>刑侦级全息画像（官方严格反张冠李戴）</v-list-item-title>
                        </v-list-item>
                        <v-list-item @click="loadPersonaVariant('concise')">
                          <template #prepend><v-icon icon="mdi-text-box-outline" color="teal" size="18" /></template>
                          <v-list-item-title>简明事实画像（生平轨迹与兴趣图谱）</v-list-item-title>
                        </v-list-item>
                        <v-list-item @click="loadPersonaVariant('psychology')">
                          <template #prepend><v-icon icon="mdi-head-heart" color="purple" size="18" /></template>
                          <v-list-item-title>深度心理与口癖特征透视</v-list-item-title>
                        </v-list-item>
                      </v-list>
                    </v-menu>
                  </div>

                  <!-- 2.1 全息画像系统提示词 -->
                  <div class="prompt-box mt-3 pa-3 rounded border">
                    <div class="d-flex align-center justify-space-between mb-2">
                      <div>
                        <strong class="text-subtitle-2 text-primary">全息画像深度研判系统提示词 (Persona System Prompt)</strong>
                        <span class="text-caption text-grey ml-2">定义事实铁律、生平拼图、心理防御与口癖提取规则</span>
                      </div>
                      <v-btn size="x-small" variant="tonal" color="warning" prepend-icon="mdi-restore" @click="resetPersonaPrompt">
                        恢复默认画像提示词
                      </v-btn>
                    </div>
                    <v-textarea
                      v-model="form['ai.persona_system_prompt']"
                      variant="outlined"
                      density="compact"
                      rows="14"
                      hide-details="auto"
                      class="code-textarea"
                    />
                  </div>

                  <!-- 2.2 空间动态与情感分析系统提示词 -->
                  <div class="prompt-box mt-4 pa-3 rounded border">
                    <div class="d-flex align-center justify-space-between mb-2">
                      <div>
                        <strong class="text-subtitle-2 text-teal">空间动态与圈层演化系统提示词 (Feeds System Prompt)</strong>
                        <span class="text-caption text-grey ml-2">定义时序情感波动流、3-Tier 圈层与叙事里程碑提取规则</span>
                      </div>
                      <v-btn size="x-small" variant="tonal" color="warning" prepend-icon="mdi-restore" @click="resetFeedPrompt">
                        恢复默认空间提示词
                      </v-btn>
                    </div>
                    <v-textarea
                      v-model="form['ai.feed_system_prompt']"
                      variant="outlined"
                      density="compact"
                      rows="10"
                      hide-details="auto"
                      class="code-textarea"
                    />
                  </div>
                </section>
              </v-window-item>

              <!-- 3. 多模态视觉与 OCR -->
              <v-window-item value="vision">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-image-filter-center-focus" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">多模态视觉与配图解析</h3>
                      <small class="text-caption text-medium-emphasis">配置多模态模型以自动识别空间动态配图、聊天截图与 OCR</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12" md="6">
                      <v-combobox
                        v-model="form['vision.model']"
                        label="多模态视觉模型名称"
                        :items="visionModelSuggestions"
                        placeholder="例如: glm-4.6v-flash (推荐) / glm-4v-flash"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6">
                      <v-text-field
                        v-model.trim="form['vision.api_base']"
                        label="视觉 API Base URL (留空则继承主 LLM)"
                        placeholder="留空默认继承"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12">
                      <v-text-field
                        v-model.trim="form['vision.api_key']"
                        label="视觉 API Key (留空则继承主 LLM)"
                        :type="showVisionKey ? 'text' : 'password'"
                        :append-inner-icon="showVisionKey ? 'mdi-eye-off' : 'mdi-eye'"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                        placeholder="留空默认继承"
                        @click:append-inner="showVisionKey = !showVisionKey"
                      />
                    </v-col>

                    <!-- 识别范围与触发策略 -->
                    <v-col cols="12" md="6" class="mt-2">
                      <v-select
                        v-model="form['vision.scope']"
                        label="图片识别数据源范围"
                        :items="visionScopeOptions"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <v-select
                        v-model="form['vision.trigger_mode']"
                        label="图片识别触发时机策略"
                        :items="visionTriggerModeOptions"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <!-- 空间说说与配图识别 -->
                    <v-col cols="12" md="6" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">空间说说配图自动识别</strong>
                          <small class="text-caption text-medium-emphasis">抓取到说说后自动调用视觉大模型提取生活场景与情绪</small>
                        </div>
                        <v-switch v-model="form['vision.auto_parse_feed']" color="primary" hide-details inset />
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">单条动态最大分析图片数</strong>
                          <v-chip size="x-small" color="primary">{{ form['vision.max_images_per_feed'] || 3 }} 张</v-chip>
                        </div>
                        <v-slider
                          v-model="form['vision.max_images_per_feed']"
                          :min="1"
                          :max="9"
                          :step="1"
                          color="primary"
                          hide-details
                        />
                      </div>
                    </v-col>

                    <!-- 群聊配图与截图识别 -->
                    <v-col cols="12" md="6" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">群聊消息图片自动识别</strong>
                          <small class="text-caption text-medium-emphasis">建议搭配长图/截图过滤，避免表情包刷屏消耗 Token</small>
                        </div>
                        <v-switch v-model="form['vision.auto_parse_chat']" color="primary" hide-details inset />
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <v-select
                        v-model="form['vision.chat_filter_mode']"
                        label="群聊图片智能过滤策略"
                        :items="visionChatFilterOptions"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <!-- 单群每日配额与云端缓存 -->
                    <v-col cols="12" md="6" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">云端远程图片自动下载缓存</strong>
                          <small class="text-caption text-medium-emphasis">自动将远程 URL 预下载至本地高速缓存后交付大模型</small>
                        </div>
                        <v-switch v-model="form['vision.auto_download_cloud']" color="primary" hide-details inset />
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">单个群聊每日最大识别图数</strong>
                          <v-chip size="x-small" color="primary">{{ form['vision.max_chat_images_per_group_day'] || 50 }} 张/日</v-chip>
                        </div>
                        <v-slider
                          v-model="form['vision.max_chat_images_per_group_day']"
                          :min="10"
                          :max="200"
                          :step="10"
                          color="primary"
                          hide-details
                        />
                      </div>
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">单张图片最大下载与识别体积</strong>
                          <v-chip size="x-small" color="primary">{{ form['vision.max_file_size_mb'] || 10 }} MB</v-chip>
                        </div>
                        <v-slider
                          v-model="form['vision.max_file_size_mb']"
                          :min="1"
                          :max="50"
                          :step="1"
                          color="primary"
                          hide-details
                        />
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 4. 空间动态与圈层算法 -->
              <v-window-item value="qzone">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-orbit" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">空间动态与圈层算法参数</h3>
                      <small class="text-caption text-medium-emphasis">自定义时序情感分析时间跨度与 3-Tier 核心圈判定阈值</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12">
                      <v-select
                        v-model="form['qzone.time_window']"
                        label="空间动态研判时间窗口"
                        :items="[
                          { title: '近 3 个月', value: '3months' },
                          { title: '近 6 个月', value: '6months' },
                          { title: '近 1 年 (推荐)', value: '1year' },
                          { title: '全量历史动态', value: 'all' }
                        ]"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">Tier 1: 极速秒赞秒评时限</strong>
                          <v-chip size="x-small" color="primary">{{ form['qzone.tier1_minutes'] || 5 }} 分钟内</v-chip>
                        </div>
                        <v-slider
                          v-model="form['qzone.tier1_minutes']"
                          :min="1"
                          :max="30"
                          :step="1"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">发表后多少分钟内的互动判定为 Tier 1 极度亲密互动</small>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">Tier 2: 深度常评互动阈值</strong>
                          <v-chip size="x-small" color="teal">{{ form['qzone.tier2_min_count'] || 3 }} 次以上</v-chip>
                        </div>
                        <v-slider
                          v-model="form['qzone.tier2_min_count']"
                          :min="2"
                          :max="10"
                          :step="1"
                          color="teal"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">最少评论/回复多少次以上进入 Tier 2 深度圈层</small>
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 5. QQ 机器人策略 -->
              <v-window-item value="bot">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-robot-outline" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">QQ 机器人全局响应策略</h3>
                      <small class="text-caption text-medium-emphasis">控制挂机机器人在群聊中的响应模式、指令前缀与自动画像</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12" md="6">
                      <v-select
                        v-model="form['bot.response_mode']"
                        label="响应权限范围"
                        :items="[
                          { title: '仅限白名单群 (推荐/安全)', value: 'whitelist_only' },
                          { title: '全量已加入群聊', value: 'all' },
                          { title: '仅限管理员私聊与指定群', value: 'admin_only' }
                        ]"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6">
                      <v-text-field
                        v-model.trim="form['bot.command_prefix']"
                        label="指令触发前缀"
                        placeholder="/"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">新成员入群自动画像</strong>
                          <small class="text-caption text-medium-emphasis">检测到新人入群事件时，后台自动提取历史记录生成研判档案</small>
                        </div>
                        <v-switch v-model="form['bot.auto_persona_on_join']" color="primary" hide-details inset />
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 6. 媒体与存储 -->
              <v-window-item value="media">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-folder-multiple-image" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">媒体资源与本地存储策略</h3>
                      <small class="text-caption text-medium-emphasis">管理头像缓存、空间相册下载策略与磁盘配额</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12" md="6">
                      <v-select
                        v-model="form['media.avatar_policy']"
                        label="头像存储策略"
                        :items="[
                          { title: '自动按需缓存 (推荐)', value: 'auto_cache' },
                          { title: '全量离线持久化', value: 'persist_all' },
                          { title: '始终远程直连 (节省空间)', value: 'remote_only' }
                        ]"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6">
                      <v-select
                        v-model="form['media.feed_image_policy']"
                        label="空间相册/说说配图下载策略"
                        :items="[
                          { title: '仅下载已研判对象 (按需)', value: 'on_demand' },
                          { title: '全量离线下载', value: 'all' },
                          { title: '不下载配图', value: 'none' }
                        ]"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">本地媒体缓存最大容量上限</strong>
                          <v-chip size="x-small" color="primary">{{ form['media.max_cache_gb'] || 10 }} GB</v-chip>
                        </div>
                        <v-slider
                          v-model="form['media.max_cache_gb']"
                          :min="1"
                          :max="100"
                          :step="1"
                          color="primary"
                          hide-details
                        />
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 7. 采集与风控 -->
              <v-window-item value="crawler">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-shield-bug-outline" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">采集频控与账号安全防封</h3>
                      <small class="text-caption text-medium-emphasis">控制 QZone 与 OneBot 抓取速度，防止触发腾讯安全风控</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12">
                      <div class="pa-3 rounded border">
                        <div class="d-flex justify-space-between align-center mb-1">
                          <strong class="text-body-2">QZone 抓取请求间隔</strong>
                          <v-chip size="x-small" color="primary">{{ form['crawler.qzone_interval_seconds'] || 1.5 }} 秒</v-chip>
                        </div>
                        <v-slider
                          v-model="form['crawler.qzone_interval_seconds']"
                          :min="0.5"
                          :max="5.0"
                          :step="0.1"
                          color="primary"
                          hide-details
                        />
                        <small class="text-caption text-medium-emphasis">间隔越大越安全；建议设为 1.5 ~ 2.5 秒</small>
                      </div>
                    </v-col>

                    <v-col cols="12" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">异常自动熔断保护</strong>
                          <small class="text-caption text-medium-emphasis">连续失败 3 次时自动暂停当前采集任务，避免频繁重试导致账号冻结</small>
                        </div>
                        <v-switch v-model="form['crawler.auto_circuit_breaker']" color="primary" hide-details inset />
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 8. 界面与偏好 -->
              <v-window-item value="ui">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-palette-outline" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">界面偏好与研究工作区</h3>
                      <small class="text-caption text-medium-emphasis">调整浏览器的视图布局与关系图谱默认分析深度</small>
                    </div>
                  </div>

                  <v-row class="mt-2" dense>
                    <v-col cols="12" md="6">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">列表显示密度</strong>
                          <small class="text-caption text-medium-emphasis">影响人员、群聊与消息表格</small>
                        </div>
                        <v-btn-toggle v-model="preferences.density" mandatory density="compact" color="primary" variant="outlined">
                          <v-btn value="comfortable">舒适</v-btn>
                          <v-btn value="compact">紧凑</v-btn>
                        </v-btn-toggle>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">默认展开左侧导航栏</strong>
                          <small class="text-caption text-medium-emphasis">启动时自动保留侧栏展开状态</small>
                        </div>
                        <v-switch v-model="preferences.navigationOpen" color="primary" hide-details inset />
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <v-select
                        v-model="preferences.defaultDepth"
                        label="默认关系研究深度"
                        :items="[
                          { title: '1 阶 (直接关联)', value: 1 },
                          { title: '2 阶 (二度人脉/推荐)', value: 2 },
                          { title: '3 阶 (深度圈层)', value: 3 },
                          { title: '4 阶 (全网拓扑)', value: 4 }
                        ]"
                        variant="outlined"
                        density="compact"
                        hide-details="auto"
                      />
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">界面主题外观</strong>
                          <small class="text-caption text-medium-emphasis">切换暗黑极客模式或白天明亮模式</small>
                        </div>
                        <v-btn-toggle
                          v-model="currentTheme"
                          mandatory
                          density="compact"
                          color="primary"
                          variant="outlined"
                          @update:model-value="onThemeChange"
                        >
                          <v-btn value="analysisDark">
                            <v-icon icon="mdi-weather-night" size="16" class="mr-1" />
                            暗黑模式
                          </v-btn>
                          <v-btn value="analysisLight">
                            <v-icon icon="mdi-weather-sunny" size="16" class="mr-1" />
                            白天模式
                          </v-btn>
                        </v-btn-toggle>
                      </div>
                    </v-col>

                    <v-col cols="12" md="6" class="mt-2">
                      <div class="d-flex align-center justify-space-between pa-3 rounded border">
                        <div>
                          <strong class="text-body-2 d-block">自动保存研究草稿</strong>
                          <small class="text-caption text-medium-emphasis">保留图谱当前的筛选和分析视角</small>
                        </div>
                        <v-switch v-model="preferences.autoSave" color="primary" hide-details inset />
                      </div>
                    </v-col>
                  </v-row>
                </section>
              </v-window-item>

              <!-- 9. 连接与状态 -->
              <v-window-item value="status">
                <section class="config-section">
                  <div class="section-title">
                    <v-icon icon="mdi-connection" size="20" class="mr-2 text-primary" />
                    <div>
                      <h3 class="text-subtitle-1 font-weight-bold mb-0">数据源节点与会话状态</h3>
                      <small class="text-caption text-medium-emphasis">查看已挂载的 OneBot 客户端与 QZone 爬虫桥接器</small>
                    </div>
                  </div>

                  <div class="mt-3">
                    <h4 class="text-caption text-medium-emphasis font-weight-bold mb-2">已连接的账号通道</h4>
                    <div v-for="account in accounts" :key="account.id" class="d-flex align-center justify-space-between pa-3 mb-2 rounded border">
                      <div class="d-flex align-center gap-2">
                        <v-icon icon="mdi-qqchat" color="primary" />
                        <div>
                          <strong class="text-body-2 d-block">{{ account.name }}</strong>
                          <small class="text-caption text-medium-emphasis">QQ {{ account.qq_uin || '未识别' }}</small>
                        </div>
                      </div>
                      <v-chip size="small" :color="account.status === 'connected' ? 'success' : 'grey'" variant="tonal">
                        {{ account.status === 'connected' ? '正常运行' : '离线' }}
                      </v-chip>
                    </div>

                    <div v-for="conn in qzoneConns" :key="conn.id" class="d-flex align-center justify-space-between pa-3 mb-2 rounded border">
                      <div class="d-flex align-center gap-2">
                        <v-icon icon="mdi-orbit" color="teal" />
                        <div>
                          <strong class="text-body-2 d-block">QZone 抓取桥接器</strong>
                          <small class="text-caption text-medium-emphasis">{{ conn.http_url }}</small>
                        </div>
                      </div>
                      <v-chip size="small" :color="conn.status === 'connected' ? 'success' : 'grey'" variant="tonal">
                        {{ conn.status === 'connected' ? '正常' : '离线' }}
                      </v-chip>
                    </div>

                    <div v-if="!accounts.length && !qzoneConns.length" class="text-center pa-6 text-medium-emphasis">
                      暂无连接中的数据源
                    </div>

                    <div class="d-flex align-center justify-space-between pa-3 mt-4 rounded border bg-surface-variant">
                      <div class="d-flex align-center gap-3">
                        <v-avatar color="primary" size="36">
                          <span class="font-weight-bold">{{ username.slice(0, 1).toUpperCase() }}</span>
                        </v-avatar>
                        <div>
                          <strong class="text-body-2 d-block">{{ username }}</strong>
                          <small class="text-caption text-medium-emphasis">本地管理员权限</small>
                        </div>
                      </div>
                      <v-btn variant="tonal" color="error" size="small" prepend-icon="mdi-logout" @click="logout">
                        退出登录
                      </v-btn>
                    </div>
                  </div>
                </section>
              </v-window-item>
            </v-window>
          </main>
        </div>
      </v-card>
    </div>

    <!-- 底部悬浮保存栏 (检测到脏数据时显现) -->
    <v-fade-transition>
      <div v-if="isDirty" class="floating-save-bar">
        <div class="d-flex align-center justify-space-between w-100 px-4 py-2">
          <div class="d-flex align-center gap-2">
            <v-icon icon="mdi-alert-circle-outline" color="warning" />
            <span class="text-body-2 font-weight-medium">您有未保存的配置更改</span>
          </div>
          <div class="d-flex gap-2">
            <v-btn variant="text" size="small" @click="resetForm">放弃更改</v-btn>
            <v-btn color="primary" variant="flat" size="small" prepend-icon="mdi-content-save" :loading="saving" @click="saveAll">
              保存并热应用
            </v-btn>
          </div>
        </div>
      </div>
    </v-fade-transition>

    <v-snackbar v-model="snack" timeout="2500" color="success">
      配置已成功保存并实时热重载！
    </v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from "vue";
import { useTheme } from "vuetify";
import { api } from "@/services/api";
import { useSessionStore } from "@/stores/session";

const session = useSessionStore();
const theme = useTheme();
const currentTheme = ref(theme.global.name.value);

function onThemeChange(val: string) {
  theme.global.name.value = val;
  localStorage.setItem("sra.theme", val);
  document.documentElement.setAttribute("data-theme", val === "analysisLight" ? "light" : "dark");
}

const username = computed(() => session.username || "admin");
const health = ref("unknown");
const loading = ref(false);
const saving = ref(false);
const pageError = ref("");
const snack = ref(false);
const activeTab = ref("ai");
const isDirty = ref(false);

const showApiKey = ref(false);
const showVisionKey = ref(false);
const testingLLM = ref(false);
const testResult = ref<any>(null);

const accounts = ref<any[]>([]);
const qzoneConns = ref<any[]>([]);

const DEFAULT_PERSONA_PROMPT = `你是一名极其严谨的计算语言学与社交行为刑侦级研判专家。
你的任务是对目标用户的聊天流水进行【高保真、去伪存真、杜绝一切张冠李戴】的微观全息画像。

【最高研判铁律 - 坚决杜绝把别人的事安在目标头上】：
1. 【严格区分第一人称自述与第三方评价/调侃/反问】：
   - 只有当目标用户使用明确的【第一人称自述】（如“我妈带大的”、“我初中毕业才有手机”、“我过敏”、“我家也管”、“我小时候”）时，才能作为其本人的生平事实。
   - 【严禁张冠李戴】：用户对群友的调侃、回复、建议、疑问或对社会热点的讨论，绝对不能算作本人特征！
     * 错误示例反面教材：用户说“mtf女同吗，有点意思”是他在评价/反问别人的标签或话题，绝非自称跨性别！
     * 错误示例反面教材：用户对群友说“挂壁减小开销呢”是给他人出主意或接梗调侃，绝非本人经济现状！
     * 错误示例反面教材：讨论公共技术、医学、心理学名词时，绝不能直接扣在用户头上，除非他明确自述“我患有...”。
2. 【生平事实与成长轨迹 (biographical_anchors)】：
   - 必须是 100% 确定为用户本人亲历的客观事实拼图（家庭身世、亲人变故、求学阶段、早年数码设备购置经历、生理过敏反应、真实作息习惯等）。
   - 每条必须附带无可辩驳的第一人称原话佐证。
3. 【心理防御机制与人格暗线 (psychological_defense)】：
   - 深入剖析其【第一人称表露】的自卑、自嘲、防御模式（如主动自贬“我就是个躺平废人”、“不像我一样”以降低他人预期；对弱小流浪动物的深度共情与保护欲等）。
4. 【典型原话金句库 (verbatim_anchor_quotes)】：
   - 精选 3-5 句最能体现其真实身世经历或性格反差的【第一人称自述原话】及心理透视。
5. 【微观语言特征与口癖 (linguistic_fingerprint)】：
   - 必须是真实的高频口语词汇（如“这倒不至于”、“怕挨揍”、“不像我一样”、“废了”、“强强”、“真怕被人给爱了”），严禁纯标点或 emoji。
   - 提取圈子暗语或反讽黑话（如“带专”、“被人给爱了”）。
6. 【具象化领域图谱 (interest_spectrum)】：
   - 必须精确到具体微观领域（如“流浪动物救助与反虐猫关注”、“低预算设备与童年数码代偿”、“逆向与爬虫技术求知”）。
7. 【社群生态定位 (social_archetype)】：
   - 深入剖析其在群聊中的深层心理需求与角色定位。
8. 【JSON 语法规范】：
   - 字符串值内部若需引用词汇，必须使用单引号 'xxx'，绝对禁止使用未转义的英文字符双引号！

请直接以合法标准 JSON 格式输出：
{
  "biographical_anchors": [
    {"aspect": "维度名称", "detail": "极度具体的事实描述", "verbatim_evidence": "无可辩驳的第一人称原话证明"}
  ],
  "psychological_defense": {
    "core_wound_and_insecurity": "核心自卑点与敏感区深度剖析",
    "defense_mechanism": "防御模式剖析",
    "empathy_and_attachment": "共情与依恋特征"
  },
  "verbatim_anchor_quotes": [
    {"quote": "第一人称原话文本", "context": "心理透视与语境说明"}
  ],
  "linguistic_fingerprint": {
    "catchphrases": ["词1", "词2"],
    "slang_and_subculture": ["特定圈子用语或黑话"],
    "sentence_style": "句式与分段特征",
    "tone_baseline": "核心语调底色"
  },
  "interest_spectrum": [
    {"domain": "具象领域", "score": 0.90, "specific_entities": ["具体实体"], "context_description": "具体关注点"}
  ],
  "social_archetype": {
    "primary_role": "具象角色",
    "community_function": "在群内的心理定位与社交功能",
    "influence_score": 75,
    "deep_analysis": "深入且击中本质的行为学与心理学画像总结"
  }
}`;

const DEFAULT_FEED_PROMPT = `你是一名极其敏锐的社交网络时序叙事与圈层演化研判专家。
你的任务是对目标用户的空间说说流水进行【时序情感波动流】、【核心社交圈层划分】与【核心叙事里程碑】的深度研判。

请以合法标准 JSON 输出：
{
  "sentiment_flow": [
    {"month": "2026-07", "post_count": 5, "sentiment_score": 0.6, "dominant_emotion": "积极/低落/焦虑", "summary": "月度叙事摘要"}
  ],
  "circle_hierarchy": {
    "tier1_speedy": [{"qq": "123456", "name": "昵称", "count": 8}],
    "tier2_deep": [{"qq": "654321", "name": "昵称", "count": 5}],
    "tier3_casual": [{"qq": "987654", "name": "昵称", "count": 2}]
  },
  "narrative_milestones": [
    {"period": "2026年7月中旬", "event": "里程碑事件", "psychological_impact": "心理学透视"}
  ],
  "circle_summary": "圈层研判综述"
}`;

// 表单响应式数据
const form = reactive<Record<string, any>>({
  "ai.llm_provider": "zhipu",
  "ai.llm_api_base": "https://open.bigmodel.cn/api/paas/v4",
  "ai.llm_api_key": "",
  "ai.llm_model": "glm-4.5-air",
  "ai.context_depth": 150,
  "ai.persona_input_char_budget": 12000,
  "ai.temperature": 0.1,
  "ai.max_concurrency": 2,
  "ai.queue_timeout_seconds": 180,
  "ai.min_request_interval_ms": 3000,
  "ai.rate_limit_backoff_seconds": 45,
  "ai.strict_mode": true,
  "ai.custom_prompt": "",
  "ai.persona_system_prompt": DEFAULT_PERSONA_PROMPT,
  "ai.feed_system_prompt": DEFAULT_FEED_PROMPT,

  "vision.model": "gemini-2.5-flash",
  "vision.api_base": "",
  "vision.api_key": "",
  "vision.auto_parse": true,
  "vision.max_images_per_feed": 3,

  "qzone.time_window": "1year",
  "qzone.tier1_minutes": 5,
  "qzone.tier2_min_count": 3,

  "bot.response_mode": "whitelist_only",
  "bot.command_prefix": "/",
  "bot.auto_persona_on_join": false,

  "media.avatar_policy": "auto_cache",
  "media.feed_image_policy": "on_demand",
  "media.max_cache_gb": 10,

  "crawler.qzone_interval_seconds": 1.5,
  "crawler.auto_circuit_breaker": true,
});

type QueueStat = {
  model_key: string;
  active_requests: number;
  queued_requests: number;
  max_concurrency: number;
  total_processed: number;
  last_active_at: string;
  next_request_at?: string;
  cooldown_until?: string;
  min_interval_ms?: number;
};
const queueStats = ref<QueueStat[]>([]);

let originalSnapshot = JSON.stringify(form);

// 界面本地偏好
type Preferences = { density: "comfortable" | "compact"; navigationOpen: boolean; defaultDepth: number; autoSave: boolean };
const defaults: Preferences = { density: "comfortable", navigationOpen: true, defaultDepth: 2, autoSave: true };
const preferences = reactive<Preferences>({ ...defaults });
const storageKey = "social-relationship-analysis.preferences";

const providerOptions = [
  { title: "智谱 GLM 开放平台 (当前主力)", value: "zhipu" },
  { title: "DeepSeek 官方 API", value: "deepseek" },
  { title: "OpenAI 兼容 / 自定义代理", value: "openai_compatible" },
  { title: "Google Gemini 平台", value: "gemini" },
  { title: "本地 Ollama (Local)", value: "ollama" },
];

const modelSuggestions = [
  "glm-4.7-flash",
  "glm-4.6v-flash",
  "glm-4.5-air",
  "glm-4v-flash",
  "glm-4-air",
  "glm-4-flash",
  "deepseek-chat",
  "deepseek-reasoner",
  "gemini-2.5-flash",
  "gpt-4o-mini",
  "gpt-4o",
  "qwen-plus",
];

const visionModelSuggestions = [
  "glm-4.6v-flash",
  "glm-4v-flash",
  "gemini-2.5-flash",
  "gpt-4o-mini",
  "gpt-4o",
  "qwen-vl-max",
];

const visionScopeOptions = [
  { title: "全部识别 (群聊消息图片 + 空间动态说说配图)", value: "all" },
  { title: "仅空间动态说说配图 (推荐: 聚焦个人生活与圈层)", value: "feed_only" },
  { title: "仅群聊消息截图与配图 (聚焦群聊证据链与转账)", value: "chat_only" },
  { title: "仅手动/按需触发 (关闭自动后台解析)", value: "manual_only" },
];

const visionTriggerModeOptions = [
  { title: "按需即时识别 (推荐: 研判查看时即时解析，节约 Token)", value: "on_demand" },
  { title: "流式即刻识别 (爬虫一抓取到新图片立即调用大模型)", value: "immediate" },
  { title: "定时巡航识别 (后台定时批量解析未处理图片)", value: "batch_cron" },
];

const visionChatFilterOptions = [
  { title: "智能过滤纯表情包 (仅识别长图、单据与聊天截图)", value: "screenshot_and_doc" },
  { title: "全部识别 (包括表情包与小图)", value: "all" },
  { title: "仅重点监控目标 (仅解析重点人员发送的图片)", value: "target_users_only" },
];

function onProviderSelect(provider: string) {
  if (provider === "zhipu") {
    form["ai.llm_api_base"] = "https://open.bigmodel.cn/api/paas/v4";
    form["ai.llm_model"] = "glm-4.7-flash";
    form["vision.api_base"] = "https://open.bigmodel.cn/api/paas/v4";
    form["vision.model"] = "glm-4.6v-flash";
  } else if (provider === "deepseek") {
    form["ai.llm_api_base"] = "https://api.deepseek.com/v1";
    form["ai.llm_model"] = "deepseek-chat";
  } else if (provider === "gemini") {
    form["ai.llm_api_base"] = "https://generativelanguage.googleapis.com/v1beta/openai/";
    form["ai.llm_model"] = "gemini-2.5-flash";
    form["vision.api_base"] = "https://generativelanguage.googleapis.com/v1beta/openai/";
    form["vision.model"] = "gemini-2.5-flash";
  } else if (provider === "ollama") {
    form["ai.llm_api_base"] = "http://localhost:11434/v1";
    form["ai.llm_model"] = "qwen2.5:7b";
  }
}

function resetPersonaPrompt() {
  form["ai.persona_system_prompt"] = DEFAULT_PERSONA_PROMPT;
}

function resetFeedPrompt() {
  form["ai.feed_system_prompt"] = DEFAULT_FEED_PROMPT;
}

function loadPersonaVariant(variant: string) {
  if (variant === "concise") {
    form["ai.persona_system_prompt"] = `你是一名严谨的社交分析专家。请根据提供的用户发言流水，提取该用户的客观生平事实拼图、具体兴趣图谱与核心社交角色。
【研判铁律】：
1. 仅当用户本人第一人称自述时方可作为生平事实，严禁将他人评价或调侃当成本人特征。
2. 必须提供无可辩驳的第一人称原话证据。
请输出合法标准 JSON：
{
  "biographical_anchors": [{"aspect": "维度名称", "detail": "客观事实描述", "verbatim_evidence": "第一人称原话"}],
  "interest_spectrum": [{"domain": "领域名称", "detail": "具体爱好与偏好"}],
  "social_archetype": "社群角色定位",
  "linguistic_fingerprint": {"catchphrases": ["常用口头禅"]}
}`;
  } else if (variant === "psychology") {
    form["ai.persona_system_prompt"] = `你是一名计算语言学与心理防御机制研判专家。请对目标用户的发言记录进行深度心理透视与口癖特征提取。
【研判铁律】：
1. 深入剖析其第一人称表露的防御模式、自嘲机制、依恋特征与敏感区。
2. 提取其最典型的高频口癖、圈子黑话与代表性金句。
请输出合法标准 JSON：
{
  "psychological_defense": {
    "core_wound_and_insecurity": "核心自卑点与敏感区",
    "defense_mechanism": "心理防御模式剖析",
    "empathy_and_attachment": "共情与依恋特征"
  },
  "verbatim_anchor_quotes": [{"quote": "第一人称原话", "context": "语境与心理透视"}],
  "linguistic_fingerprint": {
    "catchphrases": ["口头禅1", "口头禅2"],
    "slang_and_subculture": ["圈子黑话"]
  }
}`;
  }
}

watch(
  [form, preferences],
  () => {
    isDirty.value = JSON.stringify(form) !== originalSnapshot;
  },
  { deep: true }
);

function restorePreferences() {
  try {
    Object.assign(preferences, JSON.parse(localStorage.getItem(storageKey) || "{}"));
  } catch {
    Object.assign(preferences, defaults);
  }
}

async function loadAll() {
  loading.value = true;
  pageError.value = "";
  testResult.value = null;

  try {
    const [statusRes, acctsRes, qzoneRes, settingsRes] = await Promise.allSettled([
      api.get("/health"),
      api.get("/api/v1/accounts"),
      api.get("/api/v1/qzone/connections"),
      api.get("/api/v1/system/settings"),
    ]);

    health.value = statusRes.status === "fulfilled" ? statusRes.value.data.status : "error";
    if (acctsRes.status === "fulfilled") accounts.value = acctsRes.value.data?.data || [];
    if (qzoneRes.status === "fulfilled") qzoneConns.value = qzoneRes.value.data?.data || [];

    if (settingsRes.status === "fulfilled") {
      const items = settingsRes.value.data?.items || [];
      queueStats.value = settingsRes.value.data?.queue_stats || [];
      for (const item of items) {
        if (item.key in form) {
          form[item.key] = item.value;
        }
      }
      originalSnapshot = JSON.stringify(form);
      isDirty.value = false;
    }
  } catch (e: any) {
    pageError.value = e.response?.data?.error || "读取配置失败";
  } finally {
    loading.value = false;
  }
}

function resetForm() {
  try {
    Object.assign(form, JSON.parse(originalSnapshot));
  } catch {}
  isDirty.value = false;
}

async function saveAll() {
  saving.value = true;
  pageError.value = "";
  try {
    // 1. 保存系统配置到后端
    const res = await api.put("/api/v1/system/settings", form);
    if (res.data?.queue_stats) {
      queueStats.value = res.data.queue_stats;
    }

    // 2. 保存本地 UI 偏好
    localStorage.setItem(storageKey, JSON.stringify(preferences));

    originalSnapshot = JSON.stringify(form);
    isDirty.value = false;
    snack.value = true;
  } catch (e: any) {
    pageError.value = e.response?.data?.error || "保存配置失败";
  } finally {
    saving.value = false;
  }
}

async function testLLMConnection() {
  testingLLM.value = true;
  testResult.value = null;
  pageError.value = "";

  try {
    const res = (
      await api.post("/api/v1/system/settings/test-llm", {
        api_base: form["ai.llm_api_base"],
        api_key: form["ai.llm_api_key"],
        model: form["ai.llm_model"],
      })
    ).data;
    testResult.value = res;
  } catch (e: any) {
    testResult.value = {
      status: "error",
      error: e.response?.data?.error || e.message || "请求失败",
    };
  } finally {
    testingLLM.value = false;
  }
}

function logout() {
  session.logout();
}

onMounted(() => {
  restorePreferences();
  loadAll();
});
</script>

<style scoped>
.settings-container {
  margin-top: 12px;
}
.settings-card {
  overflow: hidden;
  border-radius: 8px;
}
.settings-grid-layout {
  display: grid;
  grid-template-columns: 200px minmax(0, 1fr);
  min-height: 620px;
}
.settings-nav {
  border-right: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  background: rgba(var(--v-theme-surface), 0.4);
}
.settings-tabs {
  height: 100%;
}
.settings-content {
  overflow-y: auto;
  max-height: calc(100vh - 180px);
}
.config-section {
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.section-title {
  display: flex;
  align-items: center;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}
.slider-control {
  background: rgba(var(--v-theme-surface), 0.5);
}
.prompt-box {
  background: rgba(var(--v-theme-surface), 0.6);
}
.code-textarea :deep(textarea) {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace !important;
  font-size: 12.5px !important;
  line-height: 1.5 !important;
}
.floating-save-bar {
  position: fixed;
  bottom: 24px;
  left: 50%;
  transform: translateX(-50%);
  width: 90%;
  max-width: 600px;
  background: rgba(var(--v-theme-surface), 0.95);
  backdrop-filter: blur(12px);
  border: 1px solid rgb(var(--v-theme-primary));
  border-radius: 8px;
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.25);
  z-index: 1000;
}
@media (max-width: 860px) {
  .settings-grid-layout {
    grid-template-columns: 1fr;
  }
  .settings-nav {
    border-right: none;
    border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  }
}
</style>

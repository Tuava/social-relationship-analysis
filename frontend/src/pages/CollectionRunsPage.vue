<template>
  <div class="page-wrap collection-page">
    <header class="page-header records-header">
      <div class="records-heading"><h1>采集任务</h1><span>{{ runs.length }} 个任务 · {{ activeRunCount }} 个进行中</span></div>
      <div class="records-header-actions">
        <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
        <v-btn color="secondary" variant="tonal" prepend-icon="mdi-brain" @click="goToBatchPersona">批量画像研判</v-btn>
        <v-btn color="primary" prepend-icon="mdi-play" :disabled="!accounts.length" @click="openCreate">新建采集</v-btn>
      </div>
    </header>
    <v-alert v-if="error" type="error" variant="tonal" class="mb-3" closable @click:close="error = ''">{{ error }}</v-alert>

    <section class="data-surface records-surface">
      <v-data-table class="records-table" density="compact" :headers="headers" :items="runs" :loading="loading" :items-per-page="20">
        <template #item.type="{ item }"><button class="run-identity" @click="openRun(item)"><v-icon :icon="typeIcon(item.type)" size="18" /><span><strong>{{ typeText(item.type) }}</strong><small>{{ shortId(item.id) }}</small></span></button></template>
        <template #item.status="{ item }"><v-chip size="x-small" :color="statusColor(item.status)" variant="tonal">{{ statusText(item.status) }}</v-chip></template>
        <template #item.progress="{ item }"><div class="module-strip" :title="moduleSummary(item)"><i v-for="module in item.modules || []" :key="module.module" :class="'module-' + module.status" /><i v-if="!item.modules?.length" :class="'module-' + legacyModuleStatus(item.status)" /></div></template>
        <template #item.created_at="{ item }"><span class="table-time">{{ formatDate(item.created_at) }}</span></template>
        <template #item.error="{ item }"><span :class="item.error ? 'table-error' : 'table-muted'">{{ item.error || "—" }}</span></template>
        <template #item.actions="{ item }"><div class="run-actions"><v-btn icon="mdi-eye-outline" size="small" variant="text" title="查看任务" @click="openRun(item)" /><v-btn v-if="['queued', 'running'].includes(item.status)" size="small" icon="mdi-close-circle-outline" variant="text" color="warning" title="取消任务" @click="cancel(item)" /></div></template>
        <template #no-data><div class="empty-table"><v-icon icon="mdi-database-sync-outline" /><strong>暂无采集任务</strong></div></template>
      </v-data-table>
    </section>

    <v-dialog v-model="dialog" max-width="860" scrollable>
      <v-card class="collection-dialog">
        <v-card-title class="dialog-title"><span>新建采集任务</span><v-btn icon="mdi-close" variant="text" @click="dialog = false" /></v-card-title>
        <v-divider />
        <v-card-text class="collection-form">
          <div class="form-grid">
            <v-select v-model="form.account_id" :items="accounts" item-title="name" item-value="id" label="NapCat 账号" @update:model-value="syncAccountEntry" />
            <v-select v-model="form.type" :items="taskTypes" label="任务类型" />
            <v-select v-if="form.type === 'all_sources_sync' || form.type === 'full_visible_data'" v-model="form.scope.default_group_mode" :items="groupModes" label="默认群聊模式" />
            <v-select v-if="form.type === 'all_sources_sync' || form.type === 'qzone_sync'" v-model="form.scope.default_space_mode" :items="spaceModes" label="默认空间模式" />
          </div>

          <v-alert v-if="form.type === 'all_sources_sync'" type="info" variant="tonal" class="mb-3" density="compact">
            <strong>全源同步模式</strong>：将先通过 NapCat 全量采集可见好友、群聊、群成员与群消息，随后自动通过 QQ 空间桥接器无缝同步空间资料、动态说说、多级评论、点赞互动与访客记录，形成全链路完整情报图谱。
          </v-alert>

          <v-alert v-if="form.type === 'profile_sync'" type="info" variant="tonal" class="mb-3" density="compact">
            <template v-if="!form.scope.entries.length">
              <strong>全量同步模式</strong>：未指定入口时，将自动为数据库中 7 天内未同步的 QQ 联系人补充详细资料（包括年龄、性别、地区、签名、等级等）。您也可以在下方添加指定 QQ 或群聊进行靶向精准同步。
            </template>
            <template v-else>
              <strong>靶向同步模式</strong>：将仅为下方指定的 <strong>{{ form.scope.entries.length }} 个目标（QQ / 群成员）</strong> 补充详细名片与资料。
            </template>
          </v-alert>

          <section class="scope-section">
            <header>
              <strong>{{ form.type === 'profile_sync' ? '指定同步目标（可选）' : '指定采集入口（可选）' }}</strong>
              <span>{{ form.scope.entries.length }} 个</span>
            </header>
            <div class="entry-builder">
              <v-select v-model="entryDraft.type" :items="availableEntryTypes" label="类型" density="compact" hide-details />
              <v-combobox
                v-if="entryDraft.type !== 'post'"
                v-model="entryDraft.id"
                v-model:search="searchInput"
                :items="searchSuggestions"
                item-title="title"
                item-value="value"
                :label="entryIDLabel"
                :loading="searchLoading"
                density="compact"
                hide-details
                clearable
                placeholder="可搜昵称/群名或直接输号码"
                @update:search="onSearchInput"
                @focus="triggerSearch"
                @keyup.enter="addEntry"
              >
                <template #item="{ props: itemProps, item }">
                  <v-list-item v-bind="itemProps" :subtitle="item.raw.subtitle">
                    <template #prepend>
                      <v-avatar size="28" class="mr-2">
                        <AuthImage :src="item.raw.avatar" :fallback-srcs="item.raw.fallbacks" :alt="item.raw.title">
                          <span class="text-caption font-weight-bold">{{ initial(item.raw.title) }}</span>
                        </AuthImage>
                      </v-avatar>
                    </template>
                  </v-list-item>
                </template>
              </v-combobox>
              <v-text-field
                v-else
                v-model.trim="entryDraft.id"
                label="动态标识 (authorQQ:TID)"
                placeholder="例如 2915885421:abcde123"
                density="compact"
                hide-details
                @keyup.enter="addEntry"
              />
              <v-select v-if="form.type !== 'profile_sync'" v-model="entryDraft.mode" :items="entryModeOptions" label="分支模式" density="compact" hide-details />
              <v-btn icon="mdi-plus" color="primary" variant="tonal" title="添加入口" :disabled="!hasEntryId" @click="addEntry" />
            </div>
            <div class="entry-list">
              <div v-for="(entry, index) in form.scope.entries" :key="entry.type + ':' + entry.id" class="entry-row">
                <v-icon :icon="entryIcon(entry.type)" size="18" />
                <span>
                  <strong>{{ entry.id }}</strong>
                  <small>{{ entryTypeText(entry.type) }} {{ form.type !== 'profile_sync' ? '· ' + modeText(entry.mode) : '' }}</small>
                </span>
                <v-btn icon="mdi-close" size="x-small" variant="text" title="移除" @click="form.scope.entries.splice(index, 1)" />
              </div>
              <div v-if="!form.scope.entries.length" class="scope-empty">
                {{ form.type === 'all_sources_sync' ? '未指定入口时全量采集可见群聊、消息与 QQ 空间动态' : form.type === 'full_visible_data' ? '未指定入口时采集当前账号可见的全部好友与群' : form.type === 'qzone_sync' ? '未指定入口时从当前登录 QQ 空间开始' : '未指定入口时自动同步全库联系人' }}
              </div>
            </div>
          </section>

          <section v-if="form.type !== 'profile_sync'" class="scope-section">
            <header><strong>排除范围</strong></header>
            <div class="form-grid">
              <v-combobox v-if="form.type === 'all_sources_sync' || form.type === 'full_visible_data'" v-model="form.scope.excluded_groups" label="排除群号" multiple chips closable-chips clearable />
              <v-combobox v-model="form.scope.excluded_qqs" :label="form.type === 'qzone_sync' ? '排除 QQ' : '排除私聊 QQ'" multiple chips closable-chips clearable />
            </div>
          </section>

          <section v-if="form.type === 'all_sources_sync' || form.type === 'qzone_sync' || form.scope.default_group_mode === 'full_expand'" class="scope-section safety-row">
            <v-switch v-model="depthFuse" color="primary" label="递归深度保险" hide-details inset />
            <v-number-input v-if="depthFuse" v-model="maxDepth" :min="1" :max="20" control-variant="split" label="最大深度" hide-details />
            <v-switch v-if="(form.type === 'all_sources_sync' || form.type === 'qzone_sync') && form.scope.entries.length" v-model="includeBaseline" color="secondary" label="同时关联当前账号好友动态" hide-details inset />
          </section>
        </v-card-text>
        <v-divider />
        <v-card-actions class="pa-4"><v-spacer /><v-btn variant="text" @click="dialog = false">取消</v-btn><v-btn color="primary" :loading="saving" :disabled="!form.account_id" @click="create">开始采集</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="detailDialog" max-width="1040" scrollable>
      <v-card v-if="selectedRun" class="run-detail-dialog">
        <v-card-title class="dialog-title"><div><span>{{ typeText(selectedRun.type) }}</span><small>{{ shortId(selectedRun.id) }} · {{ formatDate(selectedRun.created_at) }}</small></div><v-btn icon="mdi-close" variant="text" @click="detailDialog = false" /></v-card-title>
        <v-tabs v-model="detailTab" density="compact">
          <v-tab value="events">实时动态事件</v-tab>
          <v-tab value="progress">模块进度</v-tab>
          <v-tab value="scope">采集范围</v-tab>
          <v-tab value="candidates">候选池 {{ candidateTotal }}</v-tab>
        </v-tabs>
        <v-divider />
        <v-card-text class="run-detail-body">
          <v-window v-model="detailTab">
            <v-window-item value="events">
              <div class="events-container">
                <!-- HUD Metrics Row -->
                <div class="events-hud-grid">
                  <div class="hud-stat-card">
                    <v-icon icon="mdi-account-multiple-outline" color="primary" size="22" />
                    <div>
                      <strong>{{ candidateTotal }}</strong>
                      <small>发现候选人</small>
                    </div>
                  </div>
                  <div class="hud-stat-card">
                    <v-icon icon="mdi-image-text" color="info" size="22" />
                    <div>
                      <strong>{{ getModuleRecord(selectedRun, 'qzone_posts') }}</strong>
                      <small>采集动态</small>
                    </div>
                  </div>
                  <div class="hud-stat-card">
                    <v-icon icon="mdi-thumb-up-outline" color="pink" size="22" />
                    <div>
                      <strong>{{ getModuleRecord(selectedRun, 'qzone_likes') }}</strong>
                      <small>穿透点赞</small>
                    </div>
                  </div>
                  <div class="hud-stat-card">
                    <v-icon icon="mdi-comment-outline" color="orange" size="22" />
                    <div>
                      <strong>{{ getModuleRecord(selectedRun, 'qzone_comments') }}</strong>
                      <small>互动评论</small>
                    </div>
                  </div>
                </div>

                <!-- Active Spotlight / Resume Banner -->
                <div v-if="selectedRun.status === 'running'" class="spotlight-banner spotlight-running">
                  <v-icon icon="mdi-orbit" class="spin-icon mr-2" color="primary" size="22" />
                  <div class="spotlight-content">
                    <strong>采集正在全速推进中 ({{ selectedRun.progress }}%)</strong>
                    <small>{{ moduleSummary(selectedRun) }}</small>
                  </div>
                  <v-spacer />
                  <v-progress-circular indeterminate size="22" width="2" color="primary" />
                </div>
                <div v-else-if="['partial', 'failed', 'cancelled'].includes(selectedRun.status)" class="spotlight-banner spotlight-interrupted">
                  <v-icon icon="mdi-alert-circle-outline" color="warning" class="mr-2" size="22" />
                  <div class="spotlight-content">
                    <strong>任务处于中断 / 部分完成状态</strong>
                    <small>已保存断点，可随时一键续爬剩余候选人</small>
                  </div>
                  <v-spacer />
                  <v-btn color="primary" variant="flat" size="small" prepend-icon="mdi-play-circle-outline" @click="continueCandidates">
                    断点继续采集
                  </v-btn>
                </div>
                <div v-else-if="['completed', 'succeeded'].includes(selectedRun.status)" class="spotlight-banner spotlight-completed">
                  <v-icon icon="mdi-check-circle-outline" color="success" class="mr-2" size="22" />
                  <div class="spotlight-content">
                    <strong>采集任务已顺利完成</strong>
                    <small>已同步 {{ candidateTotal }} 名候选对象与关联数据，可立即自动化生成全息画像</small>
                  </div>
                  <v-spacer />
                  <v-btn color="primary" variant="flat" size="small" prepend-icon="mdi-brain" @click="goToBatchPersona">
                    自动化生成人物画像
                  </v-btn>
                </div>

                <!-- Filter toolbar -->
                <div class="events-toolbar">
                  <v-btn-toggle v-model="eventFilter" mandatory density="compact" variant="outlined" divided class="state-toggle">
                    <v-btn value="all">全部事件 ({{ filteredRunEvents.length }})</v-btn>
                    <v-btn value="candidate">候选对象</v-btn>
                    <v-btn value="like">点赞互动</v-btn>
                    <v-btn value="comment">评论互动</v-btn>
                    <v-btn value="module">模块状态</v-btn>
                  </v-btn-toggle>
                  <v-spacer />
                  <v-btn icon="mdi-refresh" variant="text" size="small" :loading="eventsLoading" title="刷新事件" @click="loadRunEvents" />
                </div>

                <!-- Visual Event Stream -->
                <div v-if="filteredRunEvents.length" class="events-timeline-wrapper">
                  <div class="events-stream-list">
                    <article
                      v-for="ev in filteredRunEvents"
                      :key="ev.id"
                      class="osint-event-card"
                      :class="'event-kind-' + (ev.context || ev.type)"
                    >
                      <!-- Left Category Strip & Icon -->
                      <div class="event-left-badge">
                        <div class="event-icon-circle" :style="{ background: `rgba(var(--v-theme-${ev.color}), 0.12)`, color: `rgb(var(--v-theme-${ev.color}))` }">
                          <v-icon :icon="ev.icon" size="18" />
                        </div>
                      </div>

                      <!-- Main Event Content -->
                      <div class="event-body-main">
                        <div class="event-header-row">
                          <div class="event-title-badge-group">
                            <span class="event-primary-title">{{ ev.title }}</span>
                            <v-chip v-if="ev.context" size="x-small" :color="ev.color" variant="tonal" class="font-weight-medium">
                              {{ contextLabel(ev.context) }}
                            </v-chip>
                            <v-chip v-if="ev.depth" size="x-small" variant="outlined" color="primary">
                              深度 {{ ev.depth }}
                            </v-chip>
                            <v-chip v-if="ev.count" size="x-small" variant="tonal" color="info">
                              {{ ev.count }} 项
                            </v-chip>
                          </div>
                          <span class="event-timestamp font-mono">{{ formatDate(ev.timestamp) }}</span>
                        </div>

                        <!-- Target Person / Action Details -->
                        <div class="event-detail-row">
                          <div v-if="ev.avatar || ev.target_qq" class="event-person-avatar">
                            <AuthImage :src="ev.avatar" :alt="ev.target_name || ev.target_qq">
                              <span class="avatar-fallback">{{ initial(ev.target_name || ev.target_qq) }}</span>
                            </AuthImage>
                          </div>

                          <div class="event-text-block">
                            <div v-if="ev.target_qq" class="event-person-name">
                              <strong>{{ ev.target_name || ev.target_qq }}</strong>
                              <code class="qq-code-tag ml-2">{{ ev.target_qq }}</code>
                            </div>
                            <p class="event-description-text">{{ ev.description }}</p>
                          </div>

                          <!-- Action buttons -->
                          <div v-if="ev.target_qq" class="event-actions-slot">
                            <v-btn
                              size="small"
                              variant="tonal"
                              color="primary"
                              prepend-icon="mdi-graph-outline"
                              class="text-none"
                              @click.stop="openInGraph({ entity_type: 'qq', entity_id: ev.target_qq } as any)"
                            >
                              图谱穿透
                            </v-btn>
                            <v-btn
                              size="small"
                              variant="text"
                              icon="mdi-account-search-outline"
                              title="在人物档案中查看"
                              @click.stop="openInPersons({ entity_type: 'qq', entity_id: ev.target_qq } as any)"
                            />
                          </div>
                        </div>
                      </div>
                    </article>
                  </div>
                </div>
                <div v-else class="scope-empty">
                  <v-icon icon="mdi-radar" size="42" class="mb-2 opacity-50" />
                  <span>暂无实时事件记录</span>
                </div>
              </div>
            </v-window-item>
            <v-window-item value="progress">
              <div class="module-grid">
                <article v-for="module in selectedRun.modules || []" :key="module.module" class="module-card">
                  <header>
                    <v-icon :icon="moduleIcon(module.module)" size="18" />
                    <strong>{{ moduleText(module.module) }}</strong>
                    <v-chip v-if="moduleSourceTag(module.module)" size="x-small" variant="outlined" class="ml-1" :color="moduleSourceColor(module.module)">
                      {{ moduleSourceTag(module.module) }}
                    </v-chip>
                    <v-spacer />
                    <v-chip size="x-small" :color="moduleStatusColor(module.status)" variant="tonal">{{ moduleStatusText(module.status) }}</v-chip>
                  </header>
                  <div class="module-metrics"><span><strong>{{ formatNumber(module.records_collected) }}</strong>记录</span><span><strong>{{ module.pages_completed }}</strong>页</span></div>
                  <v-progress-linear :model-value="moduleProgress(module)" :indeterminate="module.status === 'running' && !module.pages_total" :color="moduleStatusColor(module.status)" height="5" />
                  <small v-if="module.error" class="module-error">{{ module.error }}</small><small v-else-if="Object.keys(module.cursor || {}).length" class="module-cursor">游标 {{ JSON.stringify(module.cursor) }}</small>
                </article>
                <div v-if="!selectedRun.modules?.length" class="scope-empty">该任务尚未写入模块进度</div>
              </div>
            </v-window-item>
            <v-window-item value="scope">
              <div class="scope-summary">
                <dl><dt>默认群聊模式</dt><dd>{{ modeText(runScope.default_group_mode) }}</dd><dt>默认空间模式</dt><dd>{{ modeText(runScope.default_space_mode) }}</dd><dt>深度保险</dt><dd>{{ runScope.max_depth || "未启用" }}</dd></dl>
                <section><strong>入口</strong><div class="summary-chips"><v-chip v-for="entry in runScope.entries || []" :key="entry.type + ':' + entry.id" size="small" variant="outlined" :prepend-icon="entryIcon(entry.type)">{{ entry.id }} · {{ modeText(entry.mode) }}</v-chip></div></section>
                <section><strong>排除群号</strong><div class="summary-chips"><v-chip v-for="id in runScope.excluded_groups || []" :key="id" size="small" color="warning" variant="tonal">{{ id }}</v-chip><span v-if="!runScope.excluded_groups?.length">—</span></div></section>
                <section><strong>排除 QQ</strong><div class="summary-chips"><v-chip v-for="id in runScope.excluded_qqs || []" :key="id" size="small" color="warning" variant="tonal">{{ id }}</v-chip><span v-if="!runScope.excluded_qqs?.length">—</span></div></section>
              </div>
            </v-window-item>
            <v-window-item value="candidates">
              <div class="candidate-container">
                <!-- Toolbar -->
                <div class="candidate-toolbar">
                  <v-btn-toggle v-model="candidateState" mandatory density="compact" variant="outlined" divided class="state-toggle">
                    <v-btn value="">全部</v-btn>
                    <v-btn value="discovered">待处理</v-btn>
                    <v-btn value="expandable">可扩散</v-btn>
                    <v-btn value="expanded">已扩散</v-btn>
                    <v-btn value="excluded">已排除</v-btn>
                  </v-btn-toggle>

                  <v-text-field
                    v-model.trim="candidateSearch"
                    density="compact"
                    variant="outlined"
                    placeholder="按名称或 QQ / 群号搜索..."
                    prepend-inner-icon="mdi-magnify"
                    hide-details
                    clearable
                    class="candidate-search-input"
                  />

                  <v-spacer />

                  <div class="candidate-actions">
                    <v-btn
                      size="small"
                      variant="tonal"
                      color="primary"
                      prepend-icon="mdi-vector-point-plus"
                      :disabled="!actionableCandidates.length"
                      @click="bulkCandidates('expandable', true)"
                    >
                      允许本页
                    </v-btn>
                    <v-btn
                      size="small"
                      variant="tonal"
                      color="warning"
                      prepend-icon="mdi-cancel"
                      :disabled="!actionableCandidates.length"
                      @click="bulkCandidates('excluded', false)"
                    >
                      排除本页
                    </v-btn>
                    <v-btn
                      size="small"
                      color="primary"
                      variant="flat"
                      prepend-icon="mdi-play"
                      :disabled="!selectedCandidateCount"
                      @click="continueCandidates"
                    >
                      继续采集扩散 {{ selectedCandidateCount ? `(${selectedCandidateCount})` : '' }}
                    </v-btn>
                    <v-btn icon="mdi-refresh" variant="text" size="small" :loading="candidateLoading" title="刷新候选池" @click="loadCandidates" />
                  </div>
                </div>

                <!-- Candidate List -->
                <div class="candidate-list">
                  <div
                    v-for="candidate in filteredCandidates"
                    :key="candidate.id"
                    class="candidate-card cursor-pointer"
                    :class="{
                      'candidate-card-selected': candidate.selected && candidate.state === 'expandable',
                      'candidate-card-excluded': candidate.state === 'excluded'
                    }"
                    @click="openDiscoveryPaths(candidate)"
                  >
                    <!-- Checkbox -->
                    <div class="candidate-check" @click.stop>
                      <v-checkbox-btn
                        :model-value="candidate.selected && candidate.state === 'expandable'"
                        :disabled="candidate.state === 'excluded'"
                        density="compact"
                        :aria-label="`选择 ${candidate.name || candidate.entity_id}`"
                        @update:model-value="setCandidate(candidate, $event ? 'expandable' : 'collected', Boolean($event))"
                      />
                    </div>

                    <!-- Avatar -->
                    <div class="candidate-avatar">
                      <AuthImage
                        :src="candidateAvatar(candidate)"
                        :fallback-srcs="candidateAvatarFallbacks(candidate)"
                        :alt="candidate.name || candidate.entity_id"
                      >
                        <span class="avatar-fallback">{{ initial(candidate.name || candidate.entity_id) }}</span>
                      </AuthImage>
                    </div>

                    <!-- Info -->
                    <div class="candidate-info">
                      <div class="candidate-name-row">
                        <strong class="candidate-name">{{ candidate.name || candidate.entity_id }}</strong>
                        <v-chip size="x-small" variant="outlined" :prepend-icon="entryIcon(candidate.entity_type)" class="entity-type-chip">
                          {{ entryTypeText(candidate.entity_type) }} {{ candidate.entity_id }}
                        </v-chip>
                        <v-chip size="x-small" :color="candidateColor(candidate.state)" variant="tonal" class="state-chip">
                          {{ candidateText(candidate.state) }}
                        </v-chip>
                      </div>
                      <div class="candidate-meta-row">
                        <span class="depth-tag">深度 {{ candidate.depth }}</span>
                        <span class="meta-dot">·</span>
                        <span class="context-tags">
                          <v-chip
                            v-for="ctx in (candidate.contexts || [])"
                            :key="ctx"
                            size="x-small"
                            variant="tonal"
                            color="secondary"
                            class="context-chip"
                          >
                            {{ contextLabel(ctx) }}
                          </v-chip>
                          <span v-if="!candidate.contexts?.length" class="text-muted">未知来源</span>
                        </span>
                        <span class="meta-dot">·</span>
                        <v-chip
                          size="x-small"
                          variant="tonal"
                          color="primary"
                          prepend-icon="mdi-routes"
                          class="discovery-paths-chip"
                          @click.stop="openDiscoveryPaths(candidate)"
                        >
                          {{ candidate.discovery_count }} 条发现路径
                        </v-chip>
                      </div>
                    </div>

                    <!-- Actions -->
                    <div class="candidate-ops">
                      <v-btn
                        size="small"
                        icon="mdi-graph-outline"
                        variant="text"
                        color="primary"
                        title="在关系图谱中打开"
                        @click.stop="openInGraph(candidate)"
                      />
                      <v-btn
                        size="small"
                        icon="mdi-account-outline"
                        variant="text"
                        color="secondary"
                        title="查看档案"
                        @click.stop="openInPersons(candidate)"
                      />
                      <v-btn
                        v-if="candidate.state !== 'expandable' && candidate.state !== 'expanded' && candidate.state !== 'excluded'"
                        size="small"
                        variant="tonal"
                        color="primary"
                        prepend-icon="mdi-vector-point-plus"
                        @click.stop="setCandidate(candidate, 'expandable', true)"
                      >
                        允许扩散
                      </v-btn>
                      <v-btn
                        v-else-if="candidate.state === 'expanded'"
                        size="small"
                        variant="tonal"
                        color="info"
                        prepend-icon="mdi-refresh"
                        title="重新加入扩散候选"
                        @click.stop="setCandidate(candidate, 'expandable', true)"
                      >
                        重新扩散
                      </v-btn>
                      <v-btn
                        v-else-if="candidate.state === 'expandable'"
                        size="small"
                        variant="tonal"
                        color="grey"
                        prepend-icon="mdi-bookmark-check-outline"
                        @click.stop="setCandidate(candidate, 'collected', false)"
                      >
                        仅记录
                      </v-btn>
                      <v-btn
                        v-if="candidate.state !== 'excluded'"
                        size="small"
                        icon="mdi-cancel"
                        variant="text"
                        color="warning"
                        title="排除该对象"
                        @click.stop="setCandidate(candidate, 'excluded', false)"
                      />
                      <v-btn
                        v-else
                        size="small"
                        variant="tonal"
                        color="info"
                        prepend-icon="mdi-restore"
                        @click.stop="setCandidate(candidate, 'discovered', false)"
                      >
                        恢复
                      </v-btn>
                    </div>
                  </div>

                  <div v-if="!filteredCandidates.length && !candidateLoading" class="scope-empty">
                    <v-icon icon="mdi-account-search-outline" size="32" class="mb-2" />
                    <div>当前状态没有匹配的候选对象</div>
                  </div>
                </div>

                <!-- Pagination -->
                <div v-if="candidateTotal > candidatePerPage" class="candidate-pagination">
                  <span>共 {{ candidateTotal }} 个候选对象（已选 {{ candidateServerSelected }} 个）</span>
                  <v-pagination v-model="candidatePage" :length="Math.ceil(candidateTotal / candidatePerPage)" :total-visible="7" density="compact" />
                </div>
              </div>
            </v-window-item>
          </v-window>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- Discovery Paths Dialog -->
    <v-dialog v-model="pathsDialog" max-width="580" scrollable>
      <v-card v-if="selectedCandidateForPaths" class="paths-dialog-card">
        <v-card-title class="dialog-title">
          <div class="d-flex align-center gap-3">
            <v-avatar size="36">
              <AuthImage :src="candidateAvatar(selectedCandidateForPaths)" :alt="selectedCandidateForPaths.name || selectedCandidateForPaths.entity_id">
                <span>{{ initial(selectedCandidateForPaths.name || selectedCandidateForPaths.entity_id) }}</span>
              </AuthImage>
            </v-avatar>
            <div>
              <strong>{{ selectedCandidateForPaths.name || selectedCandidateForPaths.entity_id }}</strong>
              <small>{{ entryTypeText(selectedCandidateForPaths.entity_type) }} {{ selectedCandidateForPaths.entity_id }} · 深度 {{ selectedCandidateForPaths.depth }} · {{ candidateText(selectedCandidateForPaths.state) }}</small>
            </div>
          </div>
          <v-btn icon="mdi-close" variant="text" size="small" @click="pathsDialog = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="paths-dialog-content">
          <div class="mb-3 d-flex align-center justify-space-between flex-wrap gap-2">
            <span class="text-caption text-muted">发现溯源路径 (共 {{ uniqueDiscoveryPaths(selectedCandidateForPaths).length }} 条)</span>
            <div class="d-flex gap-2">
              <v-btn size="x-small" variant="tonal" color="primary" prepend-icon="mdi-graph-outline" @click="openInGraph(selectedCandidateForPaths)">在图谱中分析</v-btn>
              <v-btn size="x-small" variant="outlined" prepend-icon="mdi-account-outline" @click="openInPersons(selectedCandidateForPaths)">查看档案</v-btn>
            </div>
          </div>

          <v-timeline density="compact" side="end" class="path-timeline">
            <v-timeline-item
              v-for="(path, idx) in uniqueDiscoveryPaths(selectedCandidateForPaths)"
              :key="idx"
              :dot-color="contextColor(path.context || '')"
              size="small"
            >
              <div class="path-item-card">
                <div class="d-flex align-center justify-space-between">
                  <v-chip size="x-small" :color="contextColor(path.context || '')" variant="tonal" class="font-weight-medium">
                    {{ contextLabel(path.context || '') }}
                  </v-chip>
                  <span v-if="path.time || selectedCandidateForPaths.last_discovered_at" class="text-caption text-muted">
                    {{ formatDate(path.time || selectedCandidateForPaths.last_discovered_at || '') }}
                  </span>
                </div>
                <div v-if="path.group_id" class="mt-1 text-caption">
                  关联群聊：<strong>{{ path.group_id }}</strong>
                </div>
                <div v-if="path.parent || path.parent_qq" class="mt-1 text-caption">
                  来源父级：<strong>{{ path.parent || path.parent_qq }}</strong>
                </div>
                <div v-if="path.post_id" class="mt-1 text-caption">
                  说说动态标识：<code>{{ path.post_id }}</code>
                </div>
                <div v-if="path.comment" class="mt-1 text-caption text-truncate">
                  评论内容：{{ path.comment }}
                </div>
              </div>
            </v-timeline-item>
          </v-timeline>
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from "vue";
import { useRouter, useRoute } from "vue-router";
import { api } from "@/services/api";
import type { Account } from "@/types/api";
import AuthImage from "@/components/AuthImage.vue";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";
import { qqOf } from "@/utils/identity";

const router = useRouter();

function goToBatchPersona() {
  router.push({ path: "/persons", query: { batch: "1" } });
}

type ScopeEntry = { type: string; id: string; mode: string; depth?: number; auto?: boolean };
type Scope = { entries: ScopeEntry[]; excluded_groups: string[]; excluded_qqs: string[]; default_group_mode: string; default_space_mode: string; max_depth?: number };
type Module = { module: string; status: string; pages_completed: number; pages_total?: number; records_collected: number; cursor?: Record<string, unknown>; error?: string };
type Run = { id: string; account_id?: string; type: string; status: string; progress: number; error?: string; created_at: string; config?: { scope?: Scope }; modules?: Module[] };
type Candidate = {
  id: string;
  entity_type: string;
  entity_id: string;
  name?: string;
  avatar_url?: string;
  state: string;
  depth: number;
  selected: boolean;
  discovery_count: number;
  contexts?: string[];
  discovery_paths?: Array<{ context?: string; group_id?: string; post_id?: string; parent?: string; parent_qq?: string; comment?: string; time?: string; [key: string]: any }>;
  first_discovered_at?: string;
  last_discovered_at?: string;
};

type RunEvent = {
  id: string;
  type: string;
  timestamp: string;
  icon: string;
  color: string;
  title: string;
  description: string;
  target_qq?: string;
  target_name?: string;
  avatar?: string;
  depth?: number;
  context?: string;
  count?: number;
};

const accounts = ref<Account[]>([]), runs = ref<Run[]>([]), loading = ref(false), saving = ref(false), dialog = ref(false), error = ref("");
const detailDialog = ref(false), detailTab = ref("events"), selectedRun = ref<Run | null>(null);
const runEvents = ref<RunEvent[]>([]), eventsLoading = ref(false), eventFilter = ref("all");
const candidateState = ref(""), candidateSearch = ref(""), candidates = ref<Candidate[]>([]), candidateTotal = ref(0), candidateLoading = ref(false);
const pathsDialog = ref(false), selectedCandidateForPaths = ref<Candidate | null>(null);
const systemCapabilities = useSystemCapabilitiesStore();
const candidatePage = ref(1), candidatePerPage = ref(20), candidateServerSelected = ref(0);
const depthFuse = ref(false), maxDepth = ref(3);
const entryDraft = reactive<ScopeEntry>({ type: "qq", id: "", mode: "expand_interactions" });
const form = reactive({ account_id: "", type: "all_sources_sync", scope: { entries: [] as ScopeEntry[], excluded_groups: [] as string[], excluded_qqs: [] as string[], default_group_mode: "full_collect", default_space_mode: "expand_interactions" } });
const taskTypes = [
  { title: "全源同步 (NapCat + QQ空间)", value: "all_sources_sync" },
  { title: "全部可见数据 (仅NapCat)", value: "full_visible_data" },
  { title: "QQ 空间同步 (仅空间)", value: "qzone_sync" },
  { title: "详细资料同步", value: "profile_sync" },
];
const groupModes = [{ title: "仅记录资料和成员", value: "record_only" }, { title: "完整采集", value: "full_collect" }, { title: "完整采集并允许扩散", value: "full_expand" }];
const spaceModes = [{ title: "仅采集动态", value: "posts_only" }, { title: "采集评论、点赞和访客", value: "expand_interactions" }, { title: "继续扩散互动人员", value: "expand_people" }];
const availableEntryTypes = computed(() => {
  if (form.type === "all_sources_sync") {
    return [
      { title: "群聊", value: "group" },
      { title: "目标 QQ", value: "qq" },
      { title: "私聊会话", value: "conversation" },
      { title: "动态 (QQ:动态ID)", value: "post" },
    ];
  }
  if (form.type === "full_visible_data") {
    return [
      { title: "群聊", value: "group" },
      { title: "私聊会话", value: "conversation" },
    ];
  }
  if (form.type === "qzone_sync") {
    return [
      { title: "QQ", value: "qq" },
      { title: "动态 (QQ:动态ID)", value: "post" },
    ];
  }
  if (form.type === "profile_sync") {
    return [
      { title: "目标 QQ", value: "qq" },
      { title: "目标群聊 (同步群成员)", value: "group" },
    ];
  }
  return [];
});
watch(() => form.type, (newType) => {
  if (newType === "all_sources_sync" || newType === "full_visible_data") {
    entryDraft.type = "group";
    entryDraft.mode = form.scope.default_group_mode;
  } else if (newType === "qzone_sync") {
    entryDraft.type = "qq";
    entryDraft.mode = form.scope.default_space_mode;
  } else if (newType === "profile_sync") {
    entryDraft.type = "qq";
    entryDraft.mode = "full_collect";
  }
});
const entryModeOptions = computed(() => entryDraft.type === "group" ? groupModes : ["qq", "post"].includes(entryDraft.type) ? spaceModes : [{ title: "完整采集", value: "full_collect" }]);
const entryIDLabel = computed(() => ({ qq: "目标 QQ 号", group: "目标群号", post: "动态标识 (authorQQ:TID)", conversation: "私聊 QQ 号 (private:QQ)" } as Record<string, string>)[entryDraft.type] || "标识");
const runScope = computed<Partial<Scope>>(() => selectedRun.value?.config?.scope || {});
const selectedCandidateCount = computed(() => candidateServerSelected.value);
const filteredCandidates = computed(() => {
  if (!candidateSearch.value) return candidates.value;
  const q = candidateSearch.value.toLowerCase();
  return candidates.value.filter((c) =>
    c.entity_id.toLowerCase().includes(q) ||
    (c.name && c.name.toLowerCase().includes(q))
  );
});
const actionableCandidates = computed(() => filteredCandidates.value.filter((candidate) => !["expanded", "excluded"].includes(candidate.state)));
const activeRunCount = computed(() => runs.value.filter((item) => ["queued", "running"].includes(item.status)).length);
const headers = [{ title: "类型", key: "type" }, { title: "状态", key: "status" }, { title: "模块", key: "progress", sortable: false, width: 180 }, { title: "创建时间", key: "created_at" }, { title: "结果", key: "error" }, { title: "", key: "actions", sortable: false, width: 90 }];
let timer: number | undefined;

function candidateAvatar(candidate: Candidate): string {
  if (candidate.avatar_url) return candidate.avatar_url;
  if (candidate.entity_type === "qq") return `/api/v1/media/avatars/person/${encodeURIComponent(candidate.entity_id)}`;
  if (candidate.entity_type === "group") return `/api/v1/media/avatars/group/${encodeURIComponent(candidate.entity_id)}`;
  return "";
}

function candidateAvatarFallbacks(candidate: Candidate): string[] {
  const fallbacks: string[] = [];
  if (candidate.entity_type === "qq") {
    fallbacks.push(`https://q1.qlogo.cn/g?b=qq&nk=${encodeURIComponent(candidate.entity_id)}&s=640`);
    fallbacks.push(`https://q.qlogo.cn/headimg_dl?dst_uin=${encodeURIComponent(candidate.entity_id)}&spec=640`);
  } else if (candidate.entity_type === "group") {
    fallbacks.push(`https://p.qlogo.cn/gh/${encodeURIComponent(candidate.entity_id)}/${encodeURIComponent(candidate.entity_id)}/640/`);
    fallbacks.push(`https://p.qlogo.cn/gh/${encodeURIComponent(candidate.entity_id)}/${encodeURIComponent(candidate.entity_id)}/100/`);
  }
  return fallbacks;
}

function uniqueDiscoveryPaths(candidate?: Candidate | null) {
  if (!candidate) return [];
  const rawPaths = candidate.discovery_paths || [];
  if (!rawPaths.length) {
    return [{ context: candidate.contexts?.[0] || 'discovered' }];
  }
  const seen = new Set<string>();
  const list: any[] = [];
  for (const path of rawPaths) {
    const key = `${path.context || ''}:${path.parent || path.parent_qq || ''}:${path.group_id || ''}:${path.post_id || ''}`;
    if (!seen.has(key)) {
      seen.add(key);
      list.push(path);
    }
  }
  return list.length ? list : [{ context: candidate.contexts?.[0] || 'discovered' }];
}

function initial(str?: string): string {
  if (!str) return "?";
  return String(str).trim().slice(0, 1).toUpperCase();
}

function contextLabel(ctx: string): string {
  const map: Record<string, string> = {
    entry: "初始入口",
    friend: "好友",
    group: "群聊",
    member: "群成员",
    group_member: "群成员",
    comment: "评论",
    like: "点赞",
    mention: "被艾特",
    replied_to: "被回复",
    visitor: "访客",
    message: "消息",
    feed: "空间动态",
  };
  return map[ctx] || ctx;
}

function contextColor(ctx: string): string {
  const map: Record<string, string> = {
    entry: "primary",
    friend: "teal",
    group: "deep-purple",
    member: "indigo",
    group_member: "indigo",
    comment: "orange",
    like: "pink",
    mention: "amber-darken-2",
    replied_to: "deep-orange",
    visitor: "cyan",
    message: "blue",
    feed: "purple",
  };
  return map[ctx] || "grey";
}

function openInGraph(candidate: Candidate) {
  if (candidate.entity_type === "qq") {
    void router.push({ path: "/ego-networks", query: { target: candidate.entity_id } });
  } else {
    void router.push({ path: "/ego-networks" });
  }
}

function openInPersons(candidate: Candidate) {
  if (candidate.entity_type === "qq") {
    void router.push({ path: "/persons", query: { query: candidate.entity_id } });
  } else if (candidate.entity_type === "group") {
    void router.push({ path: "/groups", query: { query: candidate.entity_id } });
  }
}

function openDiscoveryPaths(candidate: Candidate) {
  selectedCandidateForPaths.value = candidate;
  pathsDialog.value = true;
}

const searchLoading = ref(false);
const searchSuggestions = ref<{ title: string; value: string; subtitle: string; avatar: string; fallbacks: string[] }[]>([]);
const searchInput = ref("");

const hasEntryId = computed(() => {
  if (!entryDraft.id) return false;
  if (typeof entryDraft.id === "object") return Boolean((entryDraft.id as any).value || (entryDraft.id as any).title);
  return Boolean(String(entryDraft.id).trim());
});

let searchDebounce: number | undefined;

function triggerSearch() {
  void onSearchInput(searchInput.value || "");
}

async function onSearchInput(val: string) {
  if (entryDraft.type === "post") return;
  if (searchDebounce) window.clearTimeout(searchDebounce);
  searchDebounce = window.setTimeout(async () => {
    const q = String(val || "").trim();
    searchLoading.value = true;
    try {
      if (entryDraft.type === "qq" || entryDraft.type === "conversation") {
        const res = await api.get("/api/v1/persons", { params: { q, limit: 15 } });
        const items = res.data?.data || [];
        searchSuggestions.value = items.map((p: any) => {
          const qq = qqOf(p);
          const targetId = entryDraft.type === "conversation" ? `private:${qq}` : qq;
          return {
            title: p.display_name || qq,
            value: targetId,
            subtitle: `QQ: ${qq}`,
            avatar: p.avatar_uri || (qq ? `/api/v1/media/avatars/person/${qq}` : ""),
            fallbacks: qq ? [`https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=640`] : [],
          };
        });
      } else if (entryDraft.type === "group") {
        const res = await api.get("/api/v1/groups", { params: { q, limit: 15 } });
        const items = res.data?.data || [];
        searchSuggestions.value = items.map((g: any) => ({
          title: g.group_name || g.group_id,
          value: g.group_id,
          subtitle: `群号: ${g.group_id} · ${g.member_count || 0}人`,
          avatar: g.avatar_uri || `/api/v1/media/avatars/group/${g.group_id}`,
          fallbacks: [`https://p.qlogo.cn/gh/${g.group_id}/${g.group_id}/640/`],
        }));
      }
    } catch {
      searchSuggestions.value = [];
    } finally {
      searchLoading.value = false;
    }
  }, 200);
}

const includeBaseline = ref(false);

function openCreate() {
  if (!form.account_id && accounts.value.length) form.account_id = accounts.value[0].id;
  form.scope.entries = [];
  includeBaseline.value = false;
  dialog.value = true;
}
function syncAccountEntry() {
  // Account change handler - keep entries intact
}
function addEntry() {
  let targetId = "";
  if (typeof entryDraft.id === "object" && entryDraft.id !== null) {
    targetId = (entryDraft.id as any).value || (entryDraft.id as any).title || "";
  } else {
    targetId = String(entryDraft.id || "").trim();
  }
  if (!targetId) return;
  const existing = form.scope.entries.find((item) => item.type === entryDraft.type && item.id === targetId);
  if (existing) {
    existing.mode = entryDraft.mode;
  } else {
    form.scope.entries.push({ type: entryDraft.type, id: targetId, mode: entryDraft.mode });
  }
  entryDraft.id = "";
  searchInput.value = "";
  searchSuggestions.value = [];
}
watch(() => entryDraft.type, (type) => {
  entryDraft.id = "";
  searchInput.value = "";
  searchSuggestions.value = [];
  entryDraft.mode = type === "group" ? form.scope.default_group_mode : ["qq", "post"].includes(type) ? form.scope.default_space_mode : "full_collect";
  triggerSearch();
});
watch(candidateState, () => { candidatePage.value = 1; if (detailDialog.value) void loadCandidates(); });
watch(candidatePage, () => { if (detailDialog.value) void loadCandidates(); });

async function requestWithRetry<T>(request: () => Promise<T>, attempts = 2): Promise<T> {
  let lastError: unknown;
  for (let attempt = 0; attempt < attempts; attempt += 1) {
    try {
      return await request();
    } catch (requestError) {
      lastError = requestError;
      if (attempt + 1 < attempts) await new Promise((resolve) => window.setTimeout(resolve, 350));
    }
  }
  throw lastError;
}

function requestErrorMessage(errorValue: any, fallback: string): string {
  return errorValue?.response?.data?.error || errorValue?.message || fallback;
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [accountResult, runResult] = await Promise.allSettled([
      requestWithRetry(() => api.get("/api/v1/accounts")),
      requestWithRetry(() => api.get("/api/v1/collection-runs")),
    ]);
    const loadErrors: string[] = [];
    if (accountResult.status === "fulfilled") accounts.value = accountResult.value.data.data || [];
    else loadErrors.push(`账号列表：${requestErrorMessage(accountResult.reason, "请求失败")}`);
    if (runResult.status === "rejected") {
      loadErrors.push(`任务列表：${requestErrorMessage(runResult.reason, "请求失败")}`);
      if (!runs.value.length) runs.value = [];
    }
    const loaded = runResult.status === "fulfilled" ? (runResult.value.data.data || []) as Run[] : runs.value;
    const moduleRuns = loaded.slice(0, 20);
    const modules = await Promise.all(moduleRuns.map((run) => api.get("/api/v1/collection-runs/" + run.id + "/modules").then((response) => [run.id, response.data.data || []] as const).catch(() => [run.id, []] as const)));
    const moduleMap = new Map(modules);
    runs.value = loaded.map((run) => ({ ...run, modules: moduleMap.get(run.id) || run.modules || [] }));
    if (selectedRun.value) selectedRun.value = runs.value.find((run) => run.id === selectedRun.value?.id) || selectedRun.value;
    if (loadErrors.length) error.value = loadErrors.join("；");
  } catch (e: any) {
    error.value = requestErrorMessage(e, "无法加载采集任务");
  }
  finally { loading.value = false; }
}
async function create() {
  saving.value = true;
  error.value = "";
  try {
    const scope = {
      ...form.scope,
      entries: form.scope.entries.map((entry) => ({ type: entry.type, id: entry.id, mode: entry.mode })),
      max_depth: depthFuse.value ? maxDepth.value : undefined,
      include_account_baseline: form.scope.entries.length ? includeBaseline.value : true,
    };
    await api.post("/api/v1/collection-runs", { account_id: form.account_id, type: form.type, scope });
    dialog.value = false;
    await load();
  } catch (e: any) {
    error.value = e.response?.data?.error || "创建失败";
  } finally {
    saving.value = false;
  }
}
function getModuleRecord(run: Run | null, moduleName: string): number {
  if (!run || !run.modules) return 0;
  const mod = run.modules.find((m) => m.module === moduleName);
  return mod?.records_collected || 0;
}

const filteredRunEvents = computed(() => {
  if (eventFilter.value === "all") return runEvents.value;
  if (eventFilter.value === "candidate") return runEvents.value.filter((e) => e.type === "candidate");
  if (eventFilter.value === "like") return runEvents.value.filter((e) => e.context === "like" || e.icon?.includes("thumb"));
  if (eventFilter.value === "comment") return runEvents.value.filter((e) => e.context === "comment" || e.icon?.includes("comment"));
  if (eventFilter.value === "module") return runEvents.value.filter((e) => e.type === "module");
  return runEvents.value;
});

async function loadRunEvents() {
  if (!selectedRun.value) return;
  eventsLoading.value = true;
  try {
    const res = await api.get(`/api/v1/collection-runs/${selectedRun.value.id}/events`);
    runEvents.value = res.data.data || [];
  } catch {
    runEvents.value = [];
  } finally {
    eventsLoading.value = false;
  }
}

async function cancel(run: Run) { try { await api.post("/api/v1/collection-runs/" + run.id + "/cancel"); await load(); } catch (e: any) { error.value = e.response?.data?.error || "取消失败"; } }
async function openRun(run: Run) {
  selectedRun.value = run; detailTab.value = "events"; detailDialog.value = true; candidatePage.value = 1;
  try { const [detail, modules] = await Promise.all([api.get("/api/v1/collection-runs/" + run.id), api.get("/api/v1/collection-runs/" + run.id + "/modules")]); selectedRun.value = { ...detail.data.data, modules: modules.data.data || [] }; }
  catch { selectedRun.value = run; }
  await Promise.all([loadCandidates(), loadRunEvents()]);
}
async function loadCandidates() {
  if (!selectedRun.value) return;
  candidateLoading.value = true;
  try {
    const response = await api.get("/api/v1/collection-runs/" + selectedRun.value.id + "/candidates", {
      params: { state: candidateState.value || undefined, limit: candidatePerPage.value || 20, offset: (candidatePage.value - 1) * (candidatePerPage.value || 20) }
    });
    candidates.value = response.data.data || [];
    candidateTotal.value = response.data.total || 0;
    candidateServerSelected.value = response.data.selected || 0;
  } catch {
    candidates.value = [];
    candidateTotal.value = 0;
    candidateServerSelected.value = 0;
  } finally {
    candidateLoading.value = false;
  }
}
async function setCandidate(candidate: Candidate, state: string, selected: boolean) {
  if (!selectedRun.value) return;
  candidateLoading.value = true;
  try {
    await api.patch(`/api/v1/collection-runs/${selectedRun.value.id}/candidates/${candidate.id}`, { state, selected });
    await loadCandidates();
  } catch (e: any) {
    error.value = e.response?.data?.error || "更新候选对象失败";
  } finally {
    candidateLoading.value = false;
  }
}
async function bulkCandidates(state: string, selected: boolean) {
  if (!selectedRun.value) return;
  const ids = actionableCandidates.value.map((c) => c.id);
  if (!ids.length) return;
  candidateLoading.value = true;
  try {
    await api.patch(`/api/v1/collection-runs/${selectedRun.value.id}/candidates`, {
      ids,
      state,
      selected
    });
    await loadCandidates();
  } catch (e: any) {
    error.value = e.response?.data?.error || "候选池批量操作失败";
  } finally {
    candidateLoading.value = false;
  }
}
async function continueCandidates() {
  if (!selectedRun.value) return;
  try {
    const response = await api.post("/api/v1/collection-runs/" + selectedRun.value.id + "/continue");
    await load();
    await openRun(response.data.data);
  } catch (e: any) {
    error.value = e.response?.data?.error || "无法继续采集候选对象";
  }
}

const typeText = (value: string) => ({ all_sources_sync: "全源同步", full_visible_data: "全部可见数据", profile_sync: "详细资料同步", qzone_sync: "QQ 空间同步" } as Record<string, string>)[value] || value;
const typeIcon = (value: string) => ({ all_sources_sync: "mdi-database-sync-outline", full_visible_data: "mdi-chat-processing-outline", profile_sync: "mdi-account-sync-outline", qzone_sync: "mdi-orbit" } as Record<string, string>)[value] || "mdi-database-outline";
const moduleSourceTag = (value: string) => {
  if (["profile", "friends", "groups", "group_members", "group_messages", "private_messages"].includes(value)) return "NapCat";
  if (["qzone_profile", "qzone_posts", "qzone_comments", "qzone_likes", "qzone_visitors"].includes(value)) return "QQ空间";
  if (["media", "candidate_queue"].includes(value)) return "通用";
  return "";
};
const moduleSourceColor = (value: string) => {
  if (["profile", "friends", "groups", "group_members", "group_messages", "private_messages"].includes(value)) return "primary";
  if (["qzone_profile", "qzone_posts", "qzone_comments", "qzone_likes", "qzone_visitors"].includes(value)) return "secondary";
  return "info";
};
const statusText = (value: string) => ({ queued: "排队中", running: "运行中", completed: "已完成", partial: "部分完成", failed: "失败", cancelled: "已取消" } as Record<string, string>)[value] || value;
const statusColor = (value: string) => ({ queued: "grey", running: "info", completed: "success", partial: "warning", failed: "error", cancelled: "warning" } as Record<string, string>)[value] || "grey";
const moduleStatusText = (value: string) => ({ waiting: "等待", running: "运行中", complete: "完成", partial: "部分完成", failed: "失败", no_permission: "无权限", retrying: "重试", cancelled: "取消", skipped: "未请求" } as Record<string, string>)[value] || value;
const moduleStatusColor = (value: string) => ({ waiting: "grey", running: "info", complete: "success", partial: "warning", failed: "error", no_permission: "warning", retrying: "info", cancelled: "grey", skipped: "grey" } as Record<string, string>)[value] || "grey";
const moduleText = (value: string) => ({ profile: "账号资料", qzone_profile: "空间资料", friends: "好友", groups: "群列表", group_members: "群成员", group_messages: "群消息", private_messages: "私聊消息", qzone_posts: "空间动态", qzone_comments: "评论", qzone_likes: "点赞", qzone_visitors: "访客", media: "媒体", candidate_queue: "递归队列" } as Record<string, string>)[value] || value;
const moduleIcon = (value: string) => ({ profile: "mdi-account-outline", qzone_profile: "mdi-account-outline", friends: "mdi-account-multiple-outline", groups: "mdi-account-group-outline", group_members: "mdi-account-multiple-check-outline", group_messages: "mdi-message-text-outline", private_messages: "mdi-forum-outline", qzone_posts: "mdi-image-text", qzone_comments: "mdi-comment-outline", qzone_likes: "mdi-thumb-up-outline", qzone_visitors: "mdi-eye-outline", media: "mdi-folder-image", candidate_queue: "mdi-vector-link" } as Record<string, string>)[value] || "mdi-database-outline";
const modeText = (value?: string) => ({ record_only: "仅记录", full_collect: "完整采集", full_expand: "完整采集并扩散", posts_only: "仅动态", expand_interactions: "展开互动", expand_people: "展开互动人员" } as Record<string, string>)[value || ""] || value || "—";
const entryTypeText = (value: string) => ({ qq: "QQ", group: "群聊", post: "动态", conversation: "会话" } as Record<string, string>)[value] || value;
const entryIcon = (value: string) => ({ qq: "mdi-account-outline", group: "mdi-account-group-outline", post: "mdi-image-text", conversation: "mdi-forum-outline" } as Record<string, string>)[value] || "mdi-circle-outline";
const candidateText = (value: string) => ({ discovered: "待处理", collected: "已采集", expandable: "可扩散", expanded: "已扩散", excluded: "已排除" } as Record<string, string>)[value] || value;
const candidateColor = (value: string) => ({ discovered: "grey", collected: "info", expandable: "primary", expanded: "success", excluded: "warning" } as Record<string, string>)[value] || "grey";
const legacyModuleStatus = (value: string) => value === "completed" ? "complete" : value;
const moduleSummary = (run: Run) => run.modules?.map((module) => moduleText(module.module) + ": " + moduleStatusText(module.status)).join(" · ") || String(run.progress) + "%";
const moduleProgress = (module: Module) => module.pages_total ? Math.min(100, module.pages_completed / module.pages_total * 100) : ["complete", "partial", "skipped"].includes(module.status) ? 100 : 0;
const shortId = (value: string) => value ? "任务 " + value.slice(0, 8) : "任务";
const formatDate = (value: string) => value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(value)) : "—";
const formatNumber = (value: number) => new Intl.NumberFormat("zh-CN").format(Number(value) || 0);

async function pollLoop() {
  await load();
  if (detailDialog.value && selectedRun.value) {
    try {
      const [detail, modules] = await Promise.all([
        api.get("/api/v1/collection-runs/" + selectedRun.value.id),
        api.get("/api/v1/collection-runs/" + selectedRun.value.id + "/modules")
      ]);
      selectedRun.value = { ...detail.data.data, modules: modules.data.data || [] };
      if (['queued', 'running'].includes(selectedRun.value?.status ?? '')) {
        await Promise.all([loadCandidates(), loadRunEvents()]);
      }
    } catch {
      // ignore
    }
  }
}

const route = useRoute();
onMounted(async () => {
  await systemCapabilities.load();
  candidatePerPage.value = systemCapabilities.data?.lists.default_page_size || 20;
  await load();
  if (route.query.action === "sync_profiles") {
    form.type = "profile_sync";
    if (route.query.group_id) {
      form.scope.entries = [{ type: "group", id: String(route.query.group_id), mode: "full_collect" }];
    } else if (route.query.qq) {
      form.scope.entries = [{ type: "qq", id: String(route.query.qq), mode: "full_collect" }];
    }
    if (accounts.value.length && !form.account_id) {
      form.account_id = accounts.value[0].id;
    }
    dialog.value = true;
  }
  timer = window.setInterval(pollLoop, 2000);
});
onUnmounted(() => window.clearInterval(timer));
</script>

<style scoped>
.run-identity { display: grid; grid-template-columns: 24px minmax(0, 1fr); align-items: center; gap: 8px; border: 0; background: transparent; color: inherit; text-align: left; cursor: pointer; }
.run-identity strong, .run-identity small { display: block; }.run-identity small { margin-top: 2px; color: rgba(var(--v-theme-on-surface), .42); font-size: 10px; }.run-actions { display: flex; justify-content: flex-end; }
.module-strip { display: flex; width: 150px; height: 8px; gap: 2px; }.module-strip i { flex: 1 1 0; min-width: 5px; border-radius: 1px; background: rgba(var(--v-theme-on-surface), .14); }.module-strip .module-running, .module-strip .module-retrying { background: rgb(var(--v-theme-info)); }.module-strip .module-complete { background: rgb(var(--v-theme-success)); }.module-strip .module-partial, .module-strip .module-no_permission, .module-strip .module-cancelled { background: rgb(var(--v-theme-warning)); }.module-strip .module-failed { background: rgb(var(--v-theme-error)); }.module-strip .module-skipped { background: rgba(var(--v-theme-on-surface), .22); }
.dialog-title { display: flex; min-height: 62px; align-items: center; justify-content: space-between; padding: 12px 18px; }.dialog-title > div span, .dialog-title > div small { display: block; }.dialog-title > div small { margin-top: 3px; color: rgba(var(--v-theme-on-surface), .46); font-size: 11px; }
.collection-form { display: flex; flex-direction: column; gap: 18px; padding: 18px; }.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.scope-section { border-top: 1px solid rgba(var(--v-theme-on-surface), .08); padding-top: 14px; }.scope-section > header { display: flex; justify-content: space-between; margin-bottom: 10px; font-size: 12px; }.scope-section > header span { color: rgba(var(--v-theme-on-surface), .45); }
.entry-builder { display: grid; grid-template-columns: 110px minmax(220px, 1fr) minmax(160px, 1fr) 40px; align-items: center; gap: 8px; }.entry-list { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 6px; margin-top: 10px; }
.entry-row { display: grid; grid-template-columns: 24px minmax(0, 1fr) 28px; align-items: center; gap: 8px; min-width: 0; padding: 8px 9px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 4px; }.entry-row span, .entry-row strong, .entry-row small { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }.entry-row strong { font-size: 12px; }.entry-row small { color: rgba(var(--v-theme-on-surface), .45); font-size: 10px; }
.scope-empty { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 32px 16px; color: rgba(var(--v-theme-on-surface), .45); text-align: center; font-size: 12px; }
.safety-row { display: flex; align-items: center; gap: 18px; }.safety-row .v-number-input { max-width: 190px; }
.run-detail-body { min-height: 480px; padding: 16px; }
.module-grid { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 9px; }
.module-card { display: flex; min-width: 0; min-height: 142px; flex-direction: column; gap: 12px; padding: 12px; border: 1px solid rgba(var(--v-theme-on-surface), .09); border-radius: 5px; background: rgba(var(--v-theme-on-surface), .025); }
.module-card header { display: grid; grid-template-columns: 22px minmax(0, 1fr) auto; align-items: center; gap: 6px; }
.module-card header strong { font-size: 12px; }
.module-metrics { display: flex; gap: 20px; }
.module-metrics span { color: rgba(var(--v-theme-on-surface), .45); font-size: 10px; }
.module-metrics strong { display: block; color: rgb(var(--v-theme-on-surface)); font-size: 17px; }
.module-error, .module-cursor { display: -webkit-box; overflow: hidden; color: rgb(var(--v-theme-error)); font-size: 10px; -webkit-box-orient: vertical; -webkit-line-clamp: 2; }
.module-cursor { color: rgba(var(--v-theme-on-surface), .4); }
.scope-summary { display: flex; flex-direction: column; gap: 18px; }
.scope-summary dl { display: grid; grid-template-columns: 130px minmax(0, 1fr); gap: 10px; }
.scope-summary dt { color: rgba(var(--v-theme-on-surface), .48); }
.scope-summary section > strong { display: block; margin-bottom: 8px; font-size: 12px; }
.summary-chips { display: flex; flex-wrap: wrap; gap: 6px; color: rgba(var(--v-theme-on-surface), .4); }

/* Modern Candidate Pool Styles */
.candidate-container { display: flex; flex-direction: column; gap: 12px; }
.candidate-toolbar { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; padding: 8px 12px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 8px; background: rgba(var(--v-theme-on-surface), .02); }
.candidate-search-input { max-width: 240px; min-width: 180px; }
.candidate-actions { display: flex; flex-wrap: wrap; align-items: center; gap: 8px; }
.candidate-list { display: flex; flex-direction: column; gap: 6px; max-height: 520px; overflow-y: auto; padding-right: 4px; }
.candidate-card { display: grid; grid-template-columns: 36px 42px minmax(0, 1fr) auto; align-items: center; gap: 12px; padding: 8px 12px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 8px; background: rgb(var(--v-theme-surface)); transition: border-color .15s ease, background-color .15s ease; }
.candidate-card:hover { border-color: rgba(var(--v-theme-primary), .35); background: rgba(var(--v-theme-primary), .015); }
.candidate-card-selected { border-color: rgba(var(--v-theme-primary), .6) !important; background: rgba(var(--v-theme-primary), .04) !important; }
.candidate-card-excluded { opacity: .65; }
.candidate-check { display: flex; align-items: center; justify-content: center; }
.candidate-avatar { width: 38px; height: 38px; border-radius: 50%; overflow: hidden; background: rgba(var(--v-theme-on-surface), .08); display: flex; align-items: center; justify-content: center; }
.candidate-avatar img { width: 100%; height: 100%; object-fit: cover; }
.avatar-fallback { font-size: 14px; font-weight: 600; color: rgba(var(--v-theme-on-surface), .6); }
.candidate-info { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.candidate-name-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.candidate-name { font-size: 13px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 220px; }
.entity-type-chip { font-size: 10px; font-weight: 500; }
.state-chip { font-size: 10px; font-weight: 500; }
.candidate-meta-row { display: flex; align-items: center; gap: 6px; font-size: 11px; color: rgba(var(--v-theme-on-surface), .5); flex-wrap: wrap; }
.depth-tag { font-weight: 500; }
.meta-dot { opacity: .4; }
.context-tags { display: inline-flex; align-items: center; gap: 4px; }
.context-chip { font-size: 9px; height: 18px; }
.discovery-paths-chip { font-size: 10px; font-weight: 500; cursor: pointer; transition: transform .1s ease; }
.discovery-paths-chip:hover { transform: scale(1.03); }
.candidate-ops { display: flex; align-items: center; gap: 6px; }
.candidate-pagination { display: flex; min-height: 44px; align-items: center; justify-content: space-between; gap: 12px; color: rgba(var(--v-theme-on-surface), .55); font-size: 11px; padding: 4px 8px; }

/* Discovery Paths Modal */
.paths-dialog-card { border-radius: 8px; overflow: hidden; }
.paths-dialog-content { padding: 16px 20px; max-height: 480px; overflow-y: auto; }
.path-timeline { margin-top: 8px; }
.path-item-card { padding: 8px 12px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 6px; background: rgba(var(--v-theme-on-surface), .02); }
.cursor-pointer { cursor: pointer; }

/* Visual Event Stream & Telemetry */
.events-container { display: flex; flex-direction: column; gap: 14px; }
.events-hud-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 10px; }
.hud-stat-card { display: flex; align-items: center; gap: 12px; padding: 12px 14px; border: 1px solid rgba(var(--v-theme-on-surface), .08); border-radius: 8px; background: rgba(var(--v-theme-on-surface), .025); }
.hud-stat-card strong { display: block; font-size: 17px; font-weight: 700; color: rgb(var(--v-theme-on-surface)); line-height: 1.2; }
.hud-stat-card small { display: block; font-size: 11px; color: rgba(var(--v-theme-on-surface), .5); margin-top: 2px; }

.spotlight-banner { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-radius: 8px; border: 1px solid transparent; }
.spotlight-running { background: rgba(var(--v-theme-primary), .06); border-color: rgba(var(--v-theme-primary), .25); }
.spotlight-interrupted { background: rgba(var(--v-theme-warning), .08); border-color: rgba(var(--v-theme-warning), .3); }
.spotlight-content strong { display: block; font-size: 13px; font-weight: 600; }
.spotlight-content small { display: block; font-size: 11px; color: rgba(var(--v-theme-on-surface), .55); margin-top: 2px; }

.events-toolbar { display: flex; align-items: center; gap: 10px; }
.events-timeline-wrapper { max-height: 480px; overflow-y: auto; padding-right: 6px; }
.events-stream-list { display: flex; flex-direction: column; gap: 8px; }
.osint-event-card {
  display: grid;
  grid-template-columns: 44px minmax(0, 1fr);
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid rgba(var(--v-theme-on-surface), .08);
  border-radius: 8px;
  background: rgb(var(--v-theme-surface));
  box-shadow: 0 1px 3px rgba(0,0,0,0.03);
  transition: all .15s ease;
  position: relative;
}
.osint-event-card:hover {
  border-color: rgba(var(--v-theme-primary), .35);
  box-shadow: 0 2px 8px rgba(0,0,0,0.06);
}
.event-left-badge { display: flex; align-items: flex-start; justify-content: center; padding-top: 2px; }
.event-icon-circle {
  width: 36px;
  height: 36px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
}
.event-body-main { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.event-header-row { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.event-title-badge-group { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
.event-primary-title { font-size: 13px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); }
.event-timestamp { font-size: 11px; color: rgba(var(--v-theme-on-surface), .45); }
.event-detail-row { display: flex; align-items: center; gap: 12px; min-width: 0; }
.event-person-avatar { width: 36px; height: 36px; border-radius: 50%; overflow: hidden; background: rgba(var(--v-theme-on-surface), .08); flex-shrink: 0; }
.event-person-avatar img { width: 100%; height: 100%; object-fit: cover; }
.event-text-block { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1 1 0; }
.event-person-name { font-size: 13px; color: rgb(var(--v-theme-on-surface)); display: flex; align-items: center; }
.qq-code-tag { font-family: monospace; font-size: 11px; background: rgba(var(--v-theme-on-surface), .06); padding: 1px 5px; border-radius: 4px; color: rgba(var(--v-theme-on-surface), .8); }
.event-description-text { font-size: 12px; color: rgba(var(--v-theme-on-surface), .65); margin: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.event-actions-slot { display: flex; align-items: center; gap: 6px; flex-shrink: 0; }

@media (max-width: 820px) {
  .form-grid, .entry-list, .module-grid, .events-hud-grid { grid-template-columns: 1fr; }
  .candidate-card { grid-template-columns: 32px 36px minmax(0, 1fr); }
  .candidate-ops { grid-column: 1 / -1; justify-content: flex-end; padding-top: 4px; border-top: 1px dashed rgba(var(--v-theme-on-surface), .06); }
  .candidate-search-input { max-width: 100%; min-width: 100%; }
}
</style>

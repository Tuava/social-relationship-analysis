<template>
  <div class="page-wrap graph-page graph-console graph-workspace-page">
    <section class="graph-workspace-toolbar data-surface">
      <!-- 1. 查询目标发起组 -->
      <div class="graph-query-group">
        <v-combobox
          :model-value="target"
          :items="targetOptions"
          :loading="targetSearching"
          :search="targetSearchText"
          item-title="title"
          item-value="value"
          placeholder="搜索姓名或输入目标 QQ"
          prepend-inner-icon="mdi-account-search-outline"
          hide-details
          density="compact"
          variant="solo-filled"
          clearable
          class="graph-target-combobox"
          :disabled="workspace.graphBusy || loading"
          @update:search="onTargetSearchInput"
          @update:model-value="onTargetSelect"
          @focus="onTargetSearchFocus"
          @keyup.enter="build"
        >
          <template #item="{ props: itemProps, item }">
            <v-list-item v-bind="itemProps" :subtitle="item.raw.subtitle">
              <template #prepend>
                <v-avatar size="26" class="mr-2">
                  <AuthImage :src="item.raw.avatar" :fallback-srcs="item.raw.fallbacks" :alt="item.raw.title">
                    <span class="text-caption font-weight-bold">{{ item.raw.title?.slice(0, 1) }}</span>
                  </AuthImage>
                </v-avatar>
              </template>
            </v-list-item>
          </template>
        </v-combobox>
        <v-text-field
          v-model="depth"
          type="number"
          min="1"
          label="生成阶数"
          suffix="阶"
          aria-label="关系图生成深度"
          hide-details
          density="compact"
          variant="solo-filled"
          class="graph-depth-select"
          :disabled="workspace.graphBusy || loading"
          @keyup.enter="build"
        />
        <v-btn
          color="primary"
          size="small"
          prepend-icon="mdi-vector-polyline"
          :loading="loading || workspace.graphBusy"
          :disabled="!target"
          @click="build"
        >
          生成
        </v-btn>
      </div>

      <v-divider vertical class="mx-1" />

      <!-- 2. 中间：多视图模式切换 -->
      <div class="graph-view-toggle-wrap">
        <v-btn-toggle v-model="workspace.viewMode" mandatory density="compact" variant="outlined" divided>
          <v-btn value="graph" size="small" prepend-icon="mdi-graph-outline">图谱</v-btn>
          <v-btn value="table" size="small" prepend-icon="mdi-table-large">节点<span v-if="displayNodes.length" class="ml-1 text-caption">({{ displayNodes.length }})</span></v-btn>
          <v-btn value="communities" size="small" prepend-icon="mdi-account-multiple-outline">社区</v-btn>
        </v-btn-toggle>
        <v-progress-circular v-if="loading" indeterminate size="18" width="2" color="primary" class="ml-2" />
        <span v-if="progressiveRendering" class="graph-rendering-status">渲染 {{ formatNumber(progressiveRenderedCount) }}/{{ formatNumber(progressiveTotalCount) }}</span>
        <v-btn v-if="progressiveRendering" icon="mdi-stop-circle-outline" size="x-small" variant="text" color="warning" title="停止分批渲染，保留已加载节点" aria-label="停止分批渲染" @click="cancelProgressiveRender" />
      </div>

      <v-spacer />

      <!-- 3. 右侧：视口与功能操作组 -->
      <div class="graph-toolbar-actions">
        <v-menu location="bottom end" :close-on-content-click="false">
          <template #activator="{ props }">
            <v-btn v-bind="props" variant="tonal" size="small" prepend-icon="mdi-layers-triple-outline">
              {{ workspace.maxDistance }} 跳 · {{ formatNumber(displayNodes.length) }}/{{ formatNumber(currentDistanceTotal) }} 点
            </v-btn>
          </template>
          <div class="render-menu data-surface">
            <div class="render-menu-summary">
              <strong>全量图谱</strong>
              <span>{{ formatNumber(availableNodeCount) }} 节点 · {{ formatNumber(availableEdgeCount) }} 关系</span>
              <small>已发现最大 {{ availableDistance ? `${availableDistance} 跳` : '—' }}</small>
            </div>
            <div class="render-menu-row render-distance-row">
              <span>视图距离</span>
              <v-select v-model="workspace.maxDistance" :items="distanceOptions" item-title="title" item-value="value" hide-details density="compact" variant="outlined" class="render-number-input" />
            </div>
            <div class="render-menu-hint">当前 {{ workspace.maxDistance }} 跳范围内共有 {{ formatNumber(currentDistanceTotal) }} 个节点。</div>
            <div class="render-menu-row render-limit-row">
              <span>渲染上限</span><strong>{{ formatNumber(Math.min(workspace.maxNodes, currentDistanceTotal)) }}</strong>
              <v-select :model-value="effectiveMaxNodes" :items="renderNodeOptions" item-title="title" item-value="value" hide-details density="compact" variant="outlined" class="render-number-input" @update:model-value="selectNodeLimit" />
            </div>
            <v-switch v-model="workspace.collapseBeyond" color="primary" label="按视图距离加载" hide-details density="compact" inset />
          </div>
        </v-menu>

        <v-btn
          variant="tonal"
          size="small"
          :color="routeMode ? 'primary' : undefined"
          prepend-icon="mdi-map-marker-path"
          title="链路规划与算路导航"
          @click="toggleRouteMode"
        >
          算路导航<span v-if="routeResult?.routes?.length" class="toolbar-badge">{{ routeResult.routes.length }}</span>
        </v-btn>

        <v-btn
          variant="tonal"
          size="small"
          :color="workspace.filterPanelOpen ? 'primary' : undefined"
          :prepend-icon="workspace.filterPanelOpen ? 'mdi-filter-variant' : 'mdi-filter-variant-plus'"
          @click="workspace.filterPanelOpen = !workspace.filterPanelOpen"
        >
          筛选<span v-if="activeFilterCount" class="toolbar-badge">{{ activeFilterCount }}</span>
        </v-btn>

        <v-btn
          variant="tonal"
          size="small"
          :color="workspace.inspectorOpen ? 'primary' : undefined"
          prepend-icon="mdi-dock-right"
          title="资料面板"
          @click="workspace.inspectorOpen = !workspace.inspectorOpen"
        >
          资料
        </v-btn>

        <v-menu location="bottom end">
          <template #activator="{ props }">
            <v-btn v-bind="props" icon="mdi-dots-vertical" variant="text" size="small" title="更多操作" />
          </template>
          <v-list density="compact">
            <v-list-subheader>研究快照</v-list-subheader>
            <v-list-item prepend-icon="mdi-bookmark-plus-outline" title="保存当前快照" @click="snapshotDialog = true" />
            <v-list-item prepend-icon="mdi-bookmark-multiple-outline" title="管理快照" @click="loadSnapshots" />
            <v-divider class="my-1" />
            <v-list-subheader>视图控制</v-list-subheader>
            <v-list-item prepend-icon="mdi-crosshairs-gps" title="定位目标中心" @click="centerTarget" />
            <v-list-item prepend-icon="mdi-fit-to-screen-outline" title="适配全图" @click="fit" />
            <v-list-item prepend-icon="mdi-tag-outline" title="显示节点标签" :subtitle="showLabels ? '已开启' : '已关闭'" @click="showLabels = !showLabels" />
            <v-list-item prepend-icon="mdi-mouse-outline" title="滚轮缩放" :subtitle="wheelZoom ? '已开启' : '已关闭'" @click="wheelZoom = !wheelZoom" />
            <v-divider class="my-1" />
            <v-list-subheader>数据导出</v-list-subheader>
            <v-list-item prepend-icon="mdi-file-document-outline" title="Markdown 研判简报" @click="exportMarkdown" />
            <v-list-item prepend-icon="mdi-image-outline" title="PNG 高清拓扑" @click="exportPNG" />
            <v-list-item prepend-icon="mdi-code-json" title="JSON 完整拓扑" @click="exportJSON" />
            <v-list-item prepend-icon="mdi-file-delimited-outline" title="CSV 关系列表" @click="exportCSV" />
          </v-list>
        </v-menu>
      </div>
    </section>



    <v-dialog v-model="largeRenderDialog" max-width="420" persistent>
      <v-card>
        <v-card-title>确认节点上限</v-card-title>
        <v-card-text>一次渲染 {{ formatNumber(pendingNodeLimit || nodeLimitDraft) }} 个节点可能明显降低交互速度。</v-card-text>
        <v-card-actions><v-spacer /><v-btn variant="text" @click="cancelLargeRender">取消</v-btn><v-btn color="primary" variant="tonal" @click="confirmLargeRender">继续</v-btn></v-card-actions>
      </v-card>
    </v-dialog>

    <v-alert v-if="error" type="error" variant="tonal" class="graph-error" closable @click:close="error = ''">
      {{ error }}<span v-if="graph">，当前仍显示上一次成功的图谱。</span>
    </v-alert>

    <section class="graph-console-body graph-three-column" :class="{ 'graph-filter-collapsed': !workspace.filterPanelOpen, 'graph-inspector-collapsed': !workspace.inspectorOpen }">
      <button v-if="workspace.filterPanelOpen" class="graph-filter-scrim" aria-label="关闭筛选" @click="workspace.filterPanelOpen = false" />
      <aside v-if="workspace.filterPanelOpen" class="graph-filter-panel data-surface">
        <div class="panel-heading">
          <div><strong>图谱筛选</strong></div>
          <v-btn icon="mdi-close" size="x-small" variant="text" title="收起筛选" @click="workspace.filterPanelOpen = false" />
        </div>
        <div class="filter-section">
          <div class="filter-section-heading"><strong>节点类型</strong><span>{{ selectedNodeTypeCount }}/{{ nodeTypeOptions.length }}</span></div>
          <v-chip-group v-model="nodeTypes" multiple column class="filter-chip-group">
            <v-chip
              v-for="option in nodeTypeOptions"
              :key="option.value"
              :value="option.value"
              size="small"
              filter
              variant="outlined"
              tag="button"
              :aria-label="`${option.title} ${option.count}`"
              :aria-pressed="nodeTypes.includes(option.value)"
            >
              <i class="filter-color" :style="{ background: nodeColor(option.value) }" />{{ option.title }}<em>{{ option.count }}</em>
            </v-chip>
          </v-chip-group>
        </div>
        <v-divider />
        <div class="filter-section">
          <div class="filter-section-heading"><strong>关系类型</strong><span>{{ selectedRelationTypeCount }}/{{ relationTypeOptions.length }}</span></div>
          <v-chip-group v-model="relationTypes" multiple column class="filter-chip-group">
            <v-chip
              v-for="option in relationTypeOptions"
              :key="option.value"
              :value="option.value"
              size="small"
              filter
              variant="outlined"
              tag="button"
              :aria-label="`${option.title} ${option.count}`"
              :aria-pressed="relationTypes.includes(option.value)"
            >{{ option.title }}<em>{{ option.count }}</em></v-chip>
          </v-chip-group>
        </div>
        <v-divider />
        <div class="filter-section">
          <div class="filter-section-heading"><strong>互动强度</strong><span>至少 {{ minimumWeight }} 次</span></div>
         <div class="weight-input-row">
            <v-slider v-model.number="minimumWeight" :min="1" :max="Math.max(1, maximumWeight)" :step="1" hide-details density="compact" thumb-label="always" />
         </div>
        </div>
        <v-divider />
        <div class="filter-section filter-switches">
          <v-switch v-model="targetNeighborsOnly" color="primary" label="仅看目标邻域" hide-details density="compact" />
          <v-switch v-model="showLabels" color="primary" label="显示节点标签" hide-details density="compact" />
          <v-switch v-model="wheelZoom" color="primary" label="启用滚轮缩放" hide-details density="compact" />
        </div>
        <v-btn block variant="text" prepend-icon="mdi-filter-off-outline" @click="resetFilters">重置筛选</v-btn>
      </aside>

      <main class="graph-stage data-surface">
        <div v-if="workspace.viewMode === 'communities'" class="community-stage-toolbar">
          <div><strong>社区摘要</strong><small v-if="communities.length">{{ communities.length }} 个社区 · 基于当前范围</small></div>
          <v-spacer />
          <v-btn icon="mdi-refresh" size="small" variant="text" title="刷新社区" :loading="communityLoading" @click="loadCommunities" />
        </div>

        <div v-if="workspace.viewMode === 'graph'" class="graph-canvas redesigned">
          <div ref="container" class="graph-surface" aria-label="关系图画布" />

          <!-- 左上角悬浮岛：布局模式、社区返回与路线快捷控制 -->
          <div class="floating-island floating-island-top-left">
            <v-card class="glass-island-card pa-1 d-flex align-center ga-1" rounded="0" elevation="3">
              <v-btn-toggle v-model="layoutName" mandatory density="compact" variant="tonal" divided class="layout-toggle-strict-zero" @update:model-value="() => runLayout()">
                <v-btn value="concentric" size="x-small" class="zero-radius-btn">关系层级</v-btn>
                <v-btn value="cose" size="x-small" class="zero-radius-btn">自由力导</v-btn>
                <v-btn value="grid" size="x-small" class="zero-radius-btn">网格</v-btn>
              </v-btn-toggle>
              <v-btn v-if="layoutRunning" icon="mdi-stop-circle-outline" size="x-small" color="warning" variant="text" title="停止布局" @click="cancelLayout" />
              <template v-if="activeCommunityId">
                <v-divider vertical class="mx-1" />
                <v-btn size="x-small" variant="tonal" color="info" prepend-icon="mdi-arrow-left" @click="returnToFullGraph">
                  {{ communities.find((item) => item.id === activeCommunityId)?.label || '社区' }} (返回)
                </v-btn>
              </template>
              <template v-if="routeResult">
                <v-divider vertical class="mx-1" />
                <v-chip
                  size="x-small"
                  color="warning"
                  variant="flat"
                  prepend-icon="mdi-map-marker-path"
                  class="cursor-pointer"
                  @click="waybillExpanded = !waybillExpanded"
                >
                  {{ activeRoute?.title || '路线规划' }} ({{ activeRoute?.total_hops }}跳)
                </v-chip>
              </template>
            </v-card>
          </div>

          <!-- 顶部悬浮链路算路导航控制条 (Floating Route Planning Bar) -->
          <transition name="slide-down">
            <div v-if="routeMode" class="floating-route-planning-bar data-surface">
              <div class="route-inputs-row">
                <div class="route-input-group">
                  <span class="route-input-label">起点</span>
                  <v-combobox
                    :model-value="routeSourceQQ"
                    :items="sourceOptions"
                    :loading="sourceSearching"
                    :search="sourceSearchText"
                    item-title="title"
                    item-value="value"
                    placeholder="输入或搜索起点 QQ / 姓名"
                    prepend-inner-icon="mdi-map-marker-outline"
                    hide-details
                    density="compact"
                    variant="solo-filled"
                    clearable
                    class="route-combobox"
                    :disabled="routeLoading"
                    @update:search="onSourceSearchInput"
                    @update:model-value="onSourceSelect"
                    @focus="onSourceSearchFocus"
                    @keyup.enter="planRoute"
                  >
                    <template #item="{ props: itemProps, item }">
                      <v-list-item v-bind="itemProps" :subtitle="item.raw.subtitle">
                        <template #prepend>
                          <v-avatar size="24" class="mr-2">
                            <AuthImage :src="item.raw.avatar" :fallback-srcs="item.raw.fallbacks" :alt="item.raw.title">
                              <span class="text-caption font-weight-bold">{{ item.raw.title?.slice(0, 1) }}</span>
                            </AuthImage>
                          </v-avatar>
                        </template>
                      </v-list-item>
                    </template>
                  </v-combobox>
                </div>

                <v-btn
                  icon="mdi-swap-horizontal"
                  size="small"
                  variant="text"
                  title="互换起点与终点"
                  :disabled="routeLoading"
                  @click="swapRouteQQs"
                />

                <div class="route-input-group">
                  <span class="route-input-label">终点</span>
                  <v-combobox
                    :model-value="routeTargetQQ"
                    :items="routeTargetOptions"
                    :loading="routeTargetSearching"
                    :search="routeTargetSearchText"
                    item-title="title"
                    item-value="value"
                    placeholder="输入或搜索终点 QQ / 姓名"
                    prepend-inner-icon="mdi-flag-checkered"
                    hide-details
                    density="compact"
                    variant="solo-filled"
                    clearable
                    class="route-combobox"
                    :disabled="routeLoading"
                    @update:search="onRouteTargetSearchInput"
                    @update:model-value="onRouteTargetSelect"
                    @focus="onRouteTargetSearchFocus"
                    @keyup.enter="planRoute"
                  >
                    <template #item="{ props: itemProps, item }">
                      <v-list-item v-bind="itemProps" :subtitle="item.raw.subtitle">
                        <template #prepend>
                          <v-avatar size="24" class="mr-2">
                            <AuthImage :src="item.raw.avatar" :fallback-srcs="item.raw.fallbacks" :alt="item.raw.title">
                              <span class="text-caption font-weight-bold">{{ item.raw.title?.slice(0, 1) }}</span>
                            </AuthImage>
                          </v-avatar>
                        </template>
                      </v-list-item>
                    </template>
                  </v-combobox>
                </div>

                <div class="route-strategy-group">
                  <v-select
                    v-model="routeProfile"
                    :items="routeProfileOptions"
                    item-title="title"
                    item-value="value"
                    label="算路策略"
                    hide-details
                    density="compact"
                    variant="solo-filled"
                    class="route-select-strategy"
                    :disabled="routeLoading"
                  />
                </div>

                <div class="route-hops-group">
                  <v-text-field
                    v-model.number="routeMaxHops"
                    type="number"
                    min="1"
                    max="10"
                    label="最大跳数"
                    suffix="跳"
                    aria-label="最大算路中转跳数"
                    hide-details
                    density="compact"
                    variant="solo-filled"
                    class="route-select-hops"
                    :disabled="routeLoading"
                    @keyup.enter="planRoute"
                  />
                </div>

                <v-btn
                  color="primary"
                  size="small"
                  prepend-icon="mdi-navigation-variant-outline"
                  :loading="routeLoading"
                  :disabled="!routeSourceQQ || !routeTargetQQ"
                  @click="planRoute"
                >
                  开始规划算路
                </v-btn>

                <v-btn
                  v-if="routeResult"
                  variant="tonal"
                  size="small"
                  prepend-icon="mdi-refresh"
                  title="清除高亮并重置"
                  @click="clearRouteResult"
                >
                  清除高亮
                </v-btn>

                <v-btn
                  icon="mdi-close"
                  size="x-small"
                  variant="text"
                  title="收起算路控制条"
                  @click="closeRouteMode"
                />
              </div>

              <!-- 快捷填入辅助条 -->
              <div class="route-quick-helpers mt-2 d-flex align-center flex-wrap ga-2">
                <span class="text-caption text-medium-emphasis">快捷填入:</span>
                <v-chip
                  v-if="target"
                  size="x-small"
                  variant="tonal"
                  color="primary"
                  prepend-icon="mdi-crosshairs-gps"
                  @click="setRouteSource(target)"
                >
                  设中心目标 [{{ target }}] 为起点
                </v-chip>
                <v-chip
                  v-if="personQQ && personQQ !== target"
                  size="x-small"
                  variant="tonal"
                  color="secondary"
                  prepend-icon="mdi-account"
                  @click="setRouteTarget(personQQ)"
                >
                  设当前选中 [{{ personQQ }}] 为终点
                </v-chip>
                <v-chip
                  v-if="target && personQQ && personQQ !== target"
                  size="x-small"
                  variant="flat"
                  color="primary"
                  prepend-icon="mdi-navigation-variant-outline"
                  @click="quickPlanBetween(target, personQQ)"
                >
                  一键规划 [{{ target }} -> {{ personQQ }}]
                </v-chip>
              </div>

              <v-alert v-if="routeError" type="warning" variant="tonal" density="compact" class="mt-2 mb-0" closable @click:close="routeError = ''">
                {{ routeError }}
              </v-alert>
            </div>
          </transition>

          <!-- 右上角悬浮岛：快速节点搜索定位 -->
          <div class="floating-island floating-island-top-right">
            <v-autocomplete
              v-model="searchNode"
              :items="searchOptions"
              item-title="title"
              item-value="value"
              placeholder="搜索定位节点..."
              prepend-inner-icon="mdi-magnify"
              hide-details
              clearable
              density="compact"
              variant="solo"
              flat
              rounded="sm"
              menu-icon=""
              class="graph-search-input"
              :disabled="!graph"
              @update:model-value="focusNode"
            />
          </div>

          <!-- 右下角悬浮岛：视口缩放与对齐控制 -->
          <div class="floating-island floating-island-bottom-right">
            <v-card class="glass-island-card pa-1 d-flex align-center ga-1" rounded="sm" elevation="3">
              <v-btn icon="mdi-minus" size="x-small" variant="text" :disabled="!graph" title="缩小" @click="zoomBy(0.88)" />
              <span class="zoom-value-square">{{ Math.round(zoom * 100) }}%</span>
              <v-btn icon="mdi-plus" size="x-small" variant="text" :disabled="!graph" title="放大" @click="zoomBy(1.12)" />
              <v-divider vertical class="mx-1" />
              <v-btn icon="mdi-fit-to-screen-outline" size="x-small" variant="text" :disabled="!graph" title="适配全屏" @click="fit" />
              <v-btn icon="mdi-crosshairs-gps" size="x-small" variant="text" :disabled="!graph" title="定位目标中心" @click="centerTarget" />
            </v-card>
          </div>

          <!-- 底部悬浮横向地铁路轨 (Floating Subway Waybill Dock) -->
          <transition name="slide-up">
            <div v-if="routeResult && waybillExpanded" class="floating-subway-dock data-surface">
              <div class="subway-dock-header">
                <div class="d-flex align-center ga-2 flex-wrap">
                  <v-icon icon="mdi-map-marker-path" color="warning" size="18" />
                  <span class="subway-route-title">{{ activeRoute?.title || '推荐路线' }}</span>
                  <v-chip size="x-small" :color="(activeRoute?.confidence || 0) >= 80 ? 'success' : 'warning'" variant="tonal">
                    置信度 {{ activeRoute?.confidence }}%
                  </v-chip>
                  <span class="text-caption text-medium-emphasis">
                    {{ activeRoute?.total_hops }} 跳中转 · {{ activeRoute?.total_weight }} 条证据 · 综合阻抗 {{ activeRoute?.total_cost }}
                  </span>
                </div>

                <div class="d-flex align-center ga-1 ml-auto">
                  <v-btn-toggle
                    v-if="routeResult.routes?.length > 1"
                    :model-value="activeRouteIndex"
                    mandatory
                    density="compact"
                    variant="outlined"
                    divided
                    class="subway-route-switcher"
                    @update:model-value="onRouteSelect"
                  >
                    <v-btn v-for="(r, idx) in routeResult.routes" :key="idx" :value="idx" size="x-small">
                      路线 {{ idx + 1 }} ({{ r.total_hops }}跳)
                    </v-btn>
                  </v-btn-toggle>

                  <v-btn
                    icon="mdi-chevron-down"
                    size="x-small"
                    variant="text"
                    title="最小化为胶囊"
                    @click="waybillExpanded = false"
                  />
                  <v-btn
                    icon="mdi-close"
                    size="x-small"
                    variant="text"
                    title="清除路线"
                    @click="clearRouteResult"
                  />
                </div>
              </div>

              <!-- 水平地铁站点轨道 (Horizontal Subway Track) -->
              <div class="subway-track-scroll">
                <div class="subway-track-container">
                  <!-- 起点站 -->
                  <div
                    v-if="activeRoute?.steps?.length"
                    class="subway-station start-station"
                    :class="{ 'station-active': hoveredNodeKey === activeRoute.steps[0].source_key }"
                    @mouseenter="hoverNode(activeRoute.steps[0].source_key)"
                    @mouseleave="clearHoverNode"
                    @click="selectNodeById(activeRoute.steps[0].source_key)"
                  >
                    <div class="subway-node-pill">
                      <span class="subway-station-badge start-badge">起点</span>
                      <span class="neighbor-avatar">
                        <AuthImage
                          :src="avatarSource({ type: activeRoute.steps[0].source_type, metadata: activeRoute.steps[0].source_meta })"
                          :fallback-srcs="avatarFallbacks({ type: activeRoute.steps[0].source_type, metadata: activeRoute.steps[0].source_meta })"
                          :alt="activeRoute.steps[0].source_label"
                        >
                          <span class="neighbor-avatar-fallback">{{ String(activeRoute.steps[0].source_label || '?').slice(0, 1) }}</span>
                        </AuthImage>
                      </span>
                      <div class="subway-station-info">
                        <strong>{{ cleanDisplayText(activeRoute.steps[0].source_label) || '未命名' }}</strong>
                        <small>{{ activeRoute.steps[0].source_meta?.qq ? `QQ ${activeRoute.steps[0].source_meta.qq}` : nodeIdentifier({ type: activeRoute.steps[0].source_type, metadata: activeRoute.steps[0].source_meta }) }}</small>
                      </div>
                    </div>
                  </div>

                  <!-- 逐跳区间段与中继站 -->
                  <template v-for="(step, idx) in activeRoute?.steps || []" :key="step.step_number">
                    <div
                      class="subway-segment"
                      :class="{ 'segment-active': hoveredStepNumber === step.step_number }"
                      @mouseenter="highlightStep(step, false)"
                      @mouseleave="clearStepHighlight"
                      @click="highlightStep(step, true)"
                    >
                      <div class="subway-segment-line">
                        <span class="segment-dot-arrow" />
                      </div>
                      <div class="subway-relation-chip" @click.stop="openDetailedEvidenceForStep(step)">
                        <v-chip
                          size="x-small"
                          :color="getStepMediumInfo(step.medium_type, step.relation_type).color"
                          variant="flat"
                          :prepend-icon="getStepMediumInfo(step.medium_type, step.relation_type).icon"
                          class="interactive-subway-chip"
                          title="点击查阅此步骤包含的具体文字内容与互动证据"
                        >
                          {{ getStepMediumInfo(step.medium_type, step.relation_type).label }} ({{ step.weight }} 次)
                        </v-chip>
                        <span class="subway-step-time">{{ formatDate(step.last_seen) }}</span>
                      </div>
                    </div>

                    <div
                      class="subway-station"
                      :class="{
                        'end-station': idx === (activeRoute?.steps?.length || 0) - 1,
                        'transit-station': idx < (activeRoute?.steps?.length || 0) - 1,
                        'station-active': hoveredNodeKey === step.target_key
                      }"
                      @mouseenter="hoverNode(step.target_key)"
                      @mouseleave="clearHoverNode"
                      @click="selectNodeById(step.target_key)"
                    >
                      <div class="subway-node-pill">
                        <span v-if="idx === (activeRoute?.steps?.length || 0) - 1" class="subway-station-badge end-badge">终点</span>
                        <span v-else class="subway-station-badge transit-badge">中转 {{ idx + 1 }}</span>
                        <span class="neighbor-avatar">
                          <AuthImage
                            :src="avatarSource({ type: step.target_type, metadata: step.target_meta })"
                            :fallback-srcs="avatarFallbacks({ type: step.target_type, metadata: step.target_meta })"
                            :alt="step.target_label"
                          >
                            <span class="neighbor-avatar-fallback">{{ String(step.target_label || '?').slice(0, 1) }}</span>
                          </AuthImage>
                        </span>
                        <div class="subway-station-info">
                          <strong>{{ cleanDisplayText(step.target_label) || '未命名' }}</strong>
                          <small>{{ step.target_meta?.qq ? `QQ ${step.target_meta.qq}` : (step.target_meta?.group_id ? `群 ${step.target_meta.group_id}` : nodeIdentifier({ type: step.target_type, metadata: step.target_meta })) }}</small>
                        </div>
                      </div>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </transition>

          <!-- 底部最小化悬浮胶囊 (Collapsed Subway Pill) -->
          <div v-if="routeResult && !waybillExpanded" class="floating-waybill-pill">
            <v-btn
              color="warning"
              variant="flat"
              size="small"
              prepend-icon="mdi-map-marker-path"
              append-icon="mdi-chevron-up"
              class="elevation-4"
              @click="waybillExpanded = true"
            >
              {{ activeRoute?.title || '展开路书' }} ({{ activeRoute?.total_hops }}跳 · {{ activeRoute?.confidence }}%)
            </v-btn>
          </div>

          <div v-if="!graph && !loading" class="empty-state graph-overlay"><v-icon icon="mdi-vector-polyline" size="42" /><strong>暂无关系图</strong><span>输入目标 QQ 并点击“生成”以构建社交关系网</span></div>
          <div v-if="loading" class="graph-loading graph-overlay"><v-progress-circular indeterminate color="primary" size="28" /><span>正在生成关系图…</span></div>
        </div>

        <v-data-table v-if="workspace.viewMode === 'table'" :headers="nodeHeaders" :items="filteredNodeRows" :items-per-page="systemCapabilities.data?.lists.default_page_size" density="compact" class="graph-node-table" hover @click:row="openNodeRow">
          <template #item.label="{ item }"><div class="graph-table-identity"><AuthImage v-if="isAvatarNode(item)" :src="avatarSource(item)" :fallback-srcs="avatarFallbacks(item)" :alt="item.label"><span>{{ String(item.label || '?').slice(0, 1) }}</span></AuthImage><v-icon v-else :icon="nodeIcon(item.type)" size="18"/><span><strong>{{ item.label }}</strong><small>{{ nodeIdentifier({ ...item, id: item.key }) }}</small></span></div></template>
          <template #item.type="{ item }">{{ nodeTypeLabel(item.type) }}</template>
          <template #item.degree="{ item }"><span class="numeric-cell">{{ item.degree }}</span></template>
        </v-data-table>

        <div v-if="workspace.viewMode === 'communities'" class="community-view">
          <v-alert v-if="communityError" type="warning" variant="tonal" density="compact" class="ma-3">{{ communityError }}</v-alert>
          <div v-if="communityLoading" class="community-loading"><v-progress-circular indeterminate color="primary" /><span>正在计算社区…</span></div>
          <div v-else-if="!communities.length" class="empty-state community-empty"><v-icon icon="mdi-account-multiple-outline" size="42" /><strong>暂无社区数据</strong><span>先生成一张关系图</span></div>
          <div v-else class="community-grid">
            <article v-for="community in communities" :key="community.id" class="community-card" :class="{ 'community-card-target': community.target }">
              <header class="community-card-header">
                <div><span class="community-rank">{{ community.id.replace('c-', '社区 ') }}</span><h3>{{ community.label }}</h3></div>
                <v-chip v-if="community.target" size="x-small" color="primary" variant="tonal" prepend-icon="mdi-crosshairs-gps">目标</v-chip>
              </header>
              <div class="community-metrics">
                <div><strong>{{ formatNumber(community.node_count) }}</strong><span>节点</span></div>
                <div><strong>{{ formatNumber(community.edge_count) }}</strong><span>内部关系</span></div>
                <div><strong>{{ formatNumber(community.internal_weight) }}</strong><span>互动权重</span></div>
                <div><strong>{{ formatNumber(community.external_weight) }}</strong><span>外部连接</span></div>
              </div>
              <div class="community-types"><span>人员 {{ community.person_count }}</span><span>群 {{ community.group_count }}</span><span>内容 {{ community.content_count }}</span><span v-if="community.conversation_count">会话 {{ community.conversation_count }}</span><span v-if="community.message_count">消息 {{ community.message_count }}</span><span v-if="community.other_count">其他 {{ community.other_count }}</span></div>
              <div class="community-members">
                <button v-for="member in community.members.slice(0, 6)" :key="member.key" class="community-member" :title="member.label" @click="openCommunityMember(member.key)">
                  <AuthImage v-if="isAvatarNode(member)" :src="avatarSource(member)" :fallback-srcs="avatarFallbacks(member)" :alt="member.label"><span>{{ String(member.label || '?').slice(0, 1) }}</span></AuthImage>
                  <i v-else :style="{ background: nodeColor(member.type) }"><v-icon :icon="nodeIcon(member.type)" size="13" /></i>
                </button>
                <span v-if="community.node_count > community.members.length" class="community-more">+{{ community.node_count - community.members.length }}</span>
              </div>
              <footer><span class="community-connection" :class="{ muted: !community.external_weight }">{{ community.external_weight ? `连接 ${formatNumber(community.external_weight)} 次` : '暂无外部连接' }}</span><v-btn size="small" variant="tonal" prepend-icon="mdi-vector-point-plus" :loading="expandingCommunity === community.id" @click="openCommunity(community.id)">展开社区</v-btn></footer>
            </article>
          </div>
        </div>
      </main>

      <aside v-if="workspace.inspectorOpen" class="graph-inspector data-surface redesigned-inspector">
        <template v-if="selection?.kind === 'node'">
          <div v-if="selection.data.type === 'person'" class="inspector-top">
            <div class="profile-avatar" :style="{ backgroundColor: nodeColor(selection.data.type) }"><AuthImage :src="avatarSource(selection.data) || personDetail?.avatar_uri" :fallback-srcs="[personDetail?.avatar_uri || '', ...avatarFallbacks(selection.data)].filter(Boolean)" :alt="selection.data.label"><span>{{ String(selection.data.label || '?').slice(0, 1) }}</span></AuthImage></div>
            <div class="inspector-title"><span>{{ nodeTypeLabel(selection.data.type) }}</span><h2>{{ personDetail?.display_name || cleanCQText(selection.data.label) || '未命名用户' }}</h2><small>{{ personQQ || nodeIdentifier(selection.data) }}</small></div>
            <v-btn icon="mdi-close" size="x-small" variant="text" @click="clearSelection" />
          </div>
          <div v-else-if="selection.data.type === 'message'" class="inspector-top">
            <div class="profile-avatar" style="background: #546e7a; color: #fff;"><v-icon icon="mdi-message-text-outline" size="20" /></div>
            <div class="inspector-title">
              <span>消息记录</span>
              <h2>{{ cleanCQText(messageDetail?.text || selection.data.label) }}</h2>
              <small>{{ messageDetail?.sender ? `发送者: ${messageDetail.sender}` : (selection.data.metadata?.sender ? `发送者: ${selection.data.metadata.sender}` : '消息记录') }} · {{ formatDate(messageDetail?.sent_at || selection.data.metadata?.sent_at) }}</small>
            </div>
            <v-btn icon="mdi-close" size="x-small" variant="text" @click="clearSelection" />
          </div>
          <div v-else class="inspector-top">
            <div class="profile-avatar" :style="{ backgroundColor: nodeColor(selection.data.type) }"><AuthImage :src="avatarSource(selection.data) || groupDetail?.avatar_uri" :fallback-srcs="[groupDetail?.avatar_uri || '', ...avatarFallbacks(selection.data)].filter(Boolean)" :alt="selection.data.label"><v-icon :icon="nodeIcon(selection.data.type)" /></AuthImage></div>
            <div class="inspector-title"><span>{{ nodeKindLabel(selection.data) }}</span><h2>{{ selectionTitle(selection.data) }}</h2><small>{{ nodeIdentifier(selection.data) }}</small></div>
            <v-btn icon="mdi-close" size="x-small" variant="text" @click="clearSelection" />
          </div>

          <!-- 节点快捷研判与算路操作组 -->
          <div v-if="personQQ" class="px-3 pt-2 pb-1 node-quick-actions">
            <div class="d-flex ga-1 mb-1">
              <v-btn
                size="x-small"
                variant="tonal"
                color="primary"
                class="flex-1-1"
                prepend-icon="mdi-map-marker-outline"
                title="将此人设为算路起点"
                @click="setRouteSource(personQQ, true)"
              >
                设为起点
              </v-btn>
              <v-btn
                size="x-small"
                variant="tonal"
                color="secondary"
                class="flex-1-1"
                prepend-icon="mdi-flag-checkered"
                title="将此人设为算路终点"
                @click="setRouteTarget(personQQ, true)"
              >
                设为终点
              </v-btn>
              <v-btn
                v-if="target && target !== personQQ"
                size="x-small"
                variant="flat"
                color="primary"
                class="flex-1-1"
                prepend-icon="mdi-navigation-variant-outline"
                title="一键规划从当前中心目标到此人的路径"
                @click="quickPlanBetween(target, personQQ)"
              >
                规划到此人
              </v-btn>
            </div>
            <v-btn
              block
              size="x-small"
              color="surface-variant"
              variant="outlined"
              prepend-icon="mdi-crosshairs-gps"
              @click="pivotToTarget(personQQ)"
            >
              以此人为中心重构关系网 (Pivot)
            </v-btn>
            <v-btn
              block
              size="x-small"
              color="info"
              variant="tonal"
              class="mt-1"
              prepend-icon="mdi-account-details-outline"
              title="前往人员中心查看此人全息档案与言论记录"
              @click="goToPerson(personQQ)"
            >
              查看全息人员档案
            </v-btn>
          </div>

          <v-progress-linear v-if="personLoading || entityLoading" indeterminate color="primary" class="inspector-progress" />
          <v-alert v-if="personError || entityError" type="warning" variant="tonal" density="compact" class="ma-3">{{ personError || entityError }}</v-alert>
          <template v-if="selection.data.type === 'person'">
            <v-tabs v-model="inspectorTab" density="compact" grow show-arrows>
              <v-tab value="profile">资料</v-tab>
              <v-tab value="versions">版本 {{ personDetail?.profile_count || 0 }}</v-tab>
              <v-tab value="relationship">关系</v-tab>
              <v-tab value="timeline">时间线</v-tab>
              <v-tab value="groups">群组 {{ personDetail?.membership_count || 0 }}</v-tab>
              <v-tab value="messages">消息 {{ personDetail?.message_count || 0 }}</v-tab>
              <v-tab value="feeds">动态 {{ personDetail?.content_count ?? personDetail?.contents?.length ?? 0 }}</v-tab>
            </v-tabs>
            <div class="inspector-window">
              <div v-if="inspectorTab === 'profile'" class="inspector-pane">
                <v-alert
                  v-if="isBotAccount(selection.data, personDetail)"
                  type="info"
                  variant="tonal"
                  density="compact"
                  class="mb-3"
                  icon="mdi-robot"
                >
                  <div class="text-caption font-weight-bold">公共系统服务 / 机器人账号</div>
                  <div class="text-caption text-medium-emphasis mt-1">该节点为群管家或广播机器人，已在拓扑人际算路中自动隔离。其产生的数据（如入群欢迎、退群记录）仍作为群事件证据保留。</div>
                </v-alert>
                <dl class="inspector-list compact"><template v-for="field in profileFields" :key="field.key"><dt>{{ field.label }}</dt><dd>{{ field.value }}</dd></template><dt>直接关系</dt><dd>{{ selection.degree }} 条</dd><dt>首次发现</dt><dd>{{ formatDate(personDetail?.first_seen_at) }}</dd><dt>最后发现</dt><dd>{{ formatDate(personDetail?.last_seen_at) }}</dd></dl><div v-if="!profileFields.length" class="inspector-blank">暂无已采集的详细资料</div></div>
              <div v-else-if="inspectorTab === 'versions'" class="inspector-pane inspector-pane-list"><div v-for="profile in personDetail?.profiles || []" :key="`${profile.source}:${profile.version_number}:${profile.valid_from}`" class="profile-version-row"><span><strong>{{ cleanDisplayText(profile.nickname) || '未记录昵称' }}</strong><small>{{ profile.source || '未知来源' }} · 版本 {{ profile.version_number || 1 }}</small></span><span><small>{{ formatDate(profile.first_observed_at || profile.valid_from) }}</small><v-chip size="x-small" variant="tonal">观察 {{ profile.observation_count || 1 }} 次</v-chip></span></div><div v-if="!personDetail?.profiles?.length" class="inspector-blank">暂无资料版本</div></div>
              <div v-else-if="inspectorTab === 'relationship'" class="inspector-pane">
                <v-progress-linear v-if="relationshipLoading || relationshipDeepLoading" indeterminate color="primary" class="mb-2" />
                <template v-if="relationshipData || relationshipDeepData">
                  <!-- 算法推断与衍生研判指标 -->
                  <div v-if="relationshipDeepData" class="rel-section mb-3 pa-2 rounded border border-dashed" style="background: rgba(156, 39, 176, 0.04); border-color: rgba(156, 39, 176, 0.3) !important;">
                    <div class="d-flex align-center justify-space-between mb-2">
                      <div class="d-flex align-center gap-1">
                        <v-icon icon="mdi-calculator-variant-outline" color="purple" size="16" />
                        <strong class="text-caption font-weight-bold" style="color: #ba68c8;">算法推断指标 (Inferred)</strong>
                      </div>
                      <v-chip size="x-small" color="purple" variant="tonal">非原始事实 · 供参考</v-chip>
                    </div>

                    <!-- 深度关系定性与主动权动态 -->
                    <div class="rel-category-badge mb-2">
                      <v-chip size="small" :color="getCategoryMeta(relationshipDeepData.category).color" variant="tonal" :prepend-icon="getCategoryMeta(relationshipDeepData.category).icon">
                        {{ getCategoryMeta(relationshipDeepData.category).label }}
                      </v-chip>
                      <v-chip v-if="relationshipDeepData.directionality?.power_dynamics" size="small" variant="outlined" class="ml-2">
                        {{ getPowerDynamicsLabel(relationshipDeepData.directionality.power_dynamics) }}
                      </v-chip>
                    </div>

                    <!-- 特别关心 (秒赞秒评) 警示条 -->
                    <div v-if="relationshipDeepData?.response_latency?.special_attention_suspected" class="special-attention-alert mb-2">
                      <v-icon icon="mdi-bell-ring-outline" color="amber" class="mr-2" />
                      <div>
                        <strong>疑似特别关心 (秒赞秒评)</strong>
                        <p>发现 {{ relationshipDeepData.response_latency.fast_responses_under_5min }} 次 5 分钟内极速响应 · 平均延迟 {{ relationshipDeepData.response_latency.avg_latency_minutes }} 分钟</p>
                      </div>
                    </div>

                    <!-- 双向互动非对称流向 (A -> B vs B -> A) -->
                    <div v-if="relationshipDeepData?.directionality" class="rel-section mb-2">
                      <div class="rel-subhead d-flex justify-space-between align-center">
                        <span>双向互动流向 (Asymmetric Flow)</span>
                        <small>{{ relationshipDeepData.directionality.this_ratio_pct }}% vs {{ relationshipDeepData.directionality.target_ratio_pct }}%</small>
                      </div>
                      <div class="directional-flow-bar mt-2 mb-2">
                        <div class="flow-fill-this" :style="{ width: `${relationshipDeepData.directionality.this_ratio_pct}%` }" :title="`他 -> 目标: ${relationshipDeepData.directionality.this_ratio_pct}%`">
                          <span v-if="relationshipDeepData.directionality.this_ratio_pct >= 20">{{ relationshipDeepData.directionality.this_ratio_pct }}%</span>
                        </div>
                        <div class="flow-fill-target" :style="{ width: `${relationshipDeepData.directionality.target_ratio_pct}%` }" :title="`目标 -> 他: ${relationshipDeepData.directionality.target_ratio_pct}%`">
                          <span v-if="relationshipDeepData.directionality.target_ratio_pct >= 20">{{ relationshipDeepData.directionality.target_ratio_pct }}%</span>
                        </div>
                      </div>
                      <div class="directional-breakdown-grid">
                        <div class="breakdown-col this-col">
                          <div class="breakdown-head">他 -> 目标 (主动 {{ relationshipDeepData.directionality.this_to_target.total }} 次)</div>
                          <div class="breakdown-items">
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(personQQ || nodeIdentifier(selection?.data), target, 'liked')">点赞: <strong>{{ relationshipDeepData.directionality.this_to_target.likes }}</strong></button>
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(personQQ || nodeIdentifier(selection?.data), target, 'commented')">评论: <strong>{{ relationshipDeepData.directionality.this_to_target.comments }}</strong></button>
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(personQQ || nodeIdentifier(selection?.data), target, 'sent_message')">私聊: <strong>{{ relationshipDeepData.directionality.this_to_target.messages }}</strong></button>
                          </div>
                        </div>
                        <div class="breakdown-col target-col">
                          <div class="breakdown-head">目标 -> 他 (主动 {{ relationshipDeepData.directionality.target_to_this.total }} 次)</div>
                          <div class="breakdown-items">
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(target, personQQ || nodeIdentifier(selection?.data), 'liked')">点赞: <strong>{{ relationshipDeepData.directionality.target_to_this.likes }}</strong></button>
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(target, personQQ || nodeIdentifier(selection?.data), 'commented')">评论: <strong>{{ relationshipDeepData.directionality.target_to_this.comments }}</strong></button>
                            <button class="clickable-breakdown-btn" @click="openDetailedEvidenceBetween(target, personQQ || nodeIdentifier(selection?.data), 'sent_message')">私聊: <strong>{{ relationshipDeepData.directionality.target_to_this.messages }}</strong></button>
                          </div>
                        </div>
                      </div>
                    </div>

                    <!-- 24小时作息吻合度 (Circadian Alignment) -->
                    <div v-if="relationshipDeepData?.circadian_rhythm" class="rel-section mb-2">
                      <div class="rel-subhead d-flex justify-space-between align-center">
                        <span>24 小时作息吻合度 (余弦相似度推断)</span>
                        <v-chip size="x-small" color="purple" variant="tonal">{{ relationshipDeepData.circadian_rhythm.similarity_score_pct }}% 吻合</v-chip>
                      </div>
                      <div class="circadian-chart mt-2">
                        <div class="circadian-bars">
                          <div
                            v-for="hour in 24"
                            :key="hour - 1"
                            class="circadian-bar-col"
                            :title="`${hour - 1}:00 - 他: ${relationshipDeepData.circadian_rhythm.this_hours[hour - 1]} 次, 目标: ${relationshipDeepData.circadian_rhythm.target_hours[hour - 1]} 次`"
                          >
                            <div class="circadian-bar-inner">
                              <div
                                class="circadian-fill-this"
                                :style="{ height: `${Math.min(100, (relationshipDeepData.circadian_rhythm.this_hours[hour - 1] / maxCircadianCount) * 100)}%` }"
                              />
                              <div
                                class="circadian-fill-target"
                                :style="{ height: `${Math.min(100, (relationshipDeepData.circadian_rhythm.target_hours[hour - 1] / maxCircadianCount) * 100)}%` }"
                              />
                            </div>
                            <span v-if="(hour - 1) % 6 === 0" class="circadian-hour-label">{{ hour - 1 }}h</span>
                          </div>
                        </div>
                        <div class="circadian-legend">
                          <span><i class="legend-this" /> 该用户活跃频次</span>
                          <span><i class="legend-target" /> 目标用户活跃频次</span>
                        </div>
                      </div>
                    </div>

                    <!-- 共同关键人 / 中介 Triads -->
                    <div v-if="relationshipDeepData?.intermediary_triads?.length" class="rel-section">
                      <div class="rel-subhead">共同中介 Triads ({{ relationshipDeepData.intermediary_triads.length }})</div>
                      <div class="triad-list">
                        <button
                          v-for="triad in relationshipDeepData.intermediary_triads"
                          :key="triad.id"
                          class="triad-card"
                          @click="selectNodeById(`person:${triad.id}`)"
                        >
                          <span class="neighbor-avatar">
                            <AuthImage
                              :src="avatarSource({ type: 'person', metadata: { qq: triad.qq } })"
                              :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: triad.qq } })"
                              :alt="triad.name || triad.qq"
                            >
                              <span class="neighbor-avatar-fallback">{{ String(triad.name || triad.qq || '?').slice(0, 1) }}</span>
                            </AuthImage>
                          </span>
                          <div class="triad-info">
                            <strong>{{ cleanDisplayText(triad.name) || triad.qq }}</strong>
                            <small>QQ {{ triad.qq }} · 与他 {{ triad.weight_this }} 次 · 与目标 {{ triad.weight_target }} 次</small>
                          </div>
                          <v-chip size="x-small" variant="tonal" color="primary" class="ml-auto">
                            合计 {{ triad.total_weight }}
                          </v-chip>
                        </button>
                      </div>
                    </div>
                  </div>

                  <!-- 客观确凿事实与证据链 -->
                  <div class="rel-section mb-2 pa-2 rounded border" style="background: rgba(var(--v-theme-surface), 0.5);">
                    <div class="d-flex align-center justify-space-between mb-2">
                      <div class="d-flex align-center gap-1">
                        <v-icon icon="mdi-check-decagram-outline" color="success" size="16" />
                        <strong class="text-caption font-weight-bold text-success">客观确凿事实 (Ground Truth)</strong>
                      </div>
                      <v-chip size="x-small" color="success" variant="tonal">100% 原始证据</v-chip>
                    </div>

                  <!-- 基础关系统计 -->
                  <template v-if="relationshipData">
                    <div class="rel-section">
                      <div class="rel-stat-row"><span>共同群</span><strong>{{ relationshipData.common_group_count || 0 }} 个</strong></div>
                      <div class="rel-stat-row"><span>直接私聊</span><strong>{{ relationshipData.direct_chat_count || 0 }} 条</strong></div>
                      <div class="rel-stat-row"><span>总互动</span><strong>{{ relationshipData.total_interactions || 0 }} 次</strong></div>
                      <div class="rel-stat-row"><span>空间访客</span><strong>{{ relationshipData.visit_count || 0 }} 次</strong></div>
                    </div>
                    <div class="rel-section" v-if="relationshipData.qzone_interactions">
                      <div class="rel-stat-row"><span>他赞目标动态</span><strong>{{ relationshipData.qzone_interactions.this_liked_target_posts || 0 }}</strong></div>
                      <div class="rel-stat-row"><span>他评目标动态</span><strong>{{ relationshipData.qzone_interactions.this_commented_target_posts || 0 }}</strong></div>
                      <div class="rel-stat-row"><span>目标赞他动态</span><strong>{{ relationshipData.qzone_interactions.target_liked_this_posts || 0 }}</strong></div>
                      <div class="rel-stat-row"><span>目标评他动态</span><strong>{{ relationshipData.qzone_interactions.target_commented_this_posts || 0 }}</strong></div>
                    </div>
                    <div class="rel-section" v-if="relationshipData.first_interaction || relationshipData.last_interaction">
                      <div class="rel-stat-row"><span>首次互动</span><strong>{{ formatDate(relationshipData.first_interaction) }}</strong></div>
                      <div class="rel-stat-row"><span>最近互动</span><strong>{{ formatDate(relationshipData.last_interaction) }}</strong></div>
                      <div class="rel-stat-row"><span>趋势</span><v-chip size="x-small" :color="trendColor(relationshipData.trend)" variant="tonal">{{ trendLabel(relationshipData.trend) }}</v-chip></div>
                      <div class="rel-stat-row"><span>近30天</span><strong>{{ relationshipData.recent_30_days || 0 }}</strong></div>
                      <div class="rel-stat-row"><span>前30天</span><strong>{{ relationshipData.previous_30_days || 0 }}</strong></div>
                    </div>
                    <div class="rel-section" v-if="relationshipData.common_groups?.length">
                      <div class="rel-subhead">共同群</div>
                      <button v-for="g in relationshipData.common_groups" :key="g.id" class="detail-row" @click="selectNodeById(`group:${g.id}`)">
                        <span class="neighbor-avatar">
                          <AuthImage :src="avatarSource({ type: 'group', metadata: { group_id: g.group_id } })" :fallback-srcs="avatarFallbacks({ type: 'group', metadata: { group_id: g.group_id } })" :alt="g.group_name">
                            <span class="neighbor-avatar-fallback">群</span>
                          </AuthImage>
                        </span>
                        <span><strong>{{ cleanDisplayText(g.group_name) || '未命名群' }}</strong><small>群号 {{ g.group_id }}</small></span>
                      </button>
                    </div>
                    <div class="rel-section" v-if="relationshipData.common_contacts?.length">
                      <div class="rel-subhead">共同联系人 ({{ relationshipData.common_contact_count }})</div>
                      <button v-for="c in relationshipData.common_contacts.slice(0, 20)" :key="c.id" class="detail-row" @click="selectNodeById(`person:${c.id}`)">
                        <span class="neighbor-avatar">
                          <AuthImage :src="avatarSource({ type: 'person', metadata: { qq: c.qq, avatar_uri: c.avatar_uri } })" :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: c.qq } })" :alt="c.name || c.qq">
                            <span class="neighbor-avatar-fallback">{{ String(c.name || c.qq || '?').slice(0, 1) }}</span>
                          </AuthImage>
                        </span>
                        <span><strong>{{ cleanDisplayText(c.name) || c.qq }}</strong><small>QQ {{ c.qq }}</small></span>
                      </button>
                    </div>
                    <div class="rel-section" v-if="relationshipData.time_distribution?.length">
                      <div class="rel-subhead">月度互动分布</div>
                      <div class="rel-month-bars">
                        <div v-for="m in relationshipData.time_distribution" :key="m.month" class="rel-month-bar" :title="`${m.month}: ${m.count} 次`">
                          <div class="rel-month-fill" :style="{ height: `${Math.min(100, (m.count / maxMonthCount) * 100)}%` }" />
                          <small>{{ m.month.slice(5) }}</small>
                        </div>
                      </div>
                    </div>
                  </template>
                  </div>
                </template>
                <div v-else-if="!relationshipLoading && !relationshipDeepLoading" class="inspector-blank">{{ target ? '正在加载关系数据' : '请先设置目标 QQ' }}</div>
              </div>
              <div v-else-if="inspectorTab === 'timeline'" class="inspector-pane"><v-progress-linear v-if="timelineLoading" indeterminate color="primary" class="mb-2" /><div v-if="timelineData.length" class="timeline-list"><div v-for="event in timelineData" :key="event.id" class="timeline-item"><span class="timeline-dot" :style="{ backgroundColor: eventColor(event.event_type) }" /><div class="timeline-content"><strong>{{ eventLabel(event.event_type) }}</strong><small>{{ formatDate(event.occurred_at) }}</small><span v-if="event.details?.raw_text" class="timeline-text">{{ cleanDisplayText(event.details.raw_text) }}</span><span v-else-if="event.details?.nickname" class="timeline-text">昵称: {{ cleanDisplayText(event.details.nickname) }}</span><span v-else-if="event.details?.group_id" class="timeline-text">入群</span></div></div></div><div v-else-if="!timelineLoading" class="inspector-blank">暂无事件记录</div></div>
              <div v-else-if="inspectorTab === 'groups'" class="inspector-pane inspector-pane-list"><button v-for="group in personDetail?.memberships || []" :key="group.id" class="detail-row" @click="selectNodeById(`group:${group.id}`)"><span class="neighbor-avatar"><AuthImage :src="avatarSource({ type: 'group', metadata: { group_id: group.group_id, avatar_uri: group.avatar_uri } })" :fallback-srcs="avatarFallbacks({ type: 'group', metadata: { group_id: group.group_id } })" :alt="group.group_name"><span class="neighbor-avatar-fallback">群</span></AuthImage></span><span><strong>{{ cleanDisplayText(group.group_name) || '未命名群' }}</strong><small>群号 {{ group.group_id || '未知' }} · {{ roleLabel(group.role) }}{{ group.card ? ' · 群名片: ' + cleanDisplayText(group.card) : '' }} · {{ formatDate(group.valid_from) }}</small></span></button><div v-if="!personDetail?.memberships?.length" class="inspector-blank">暂无群组记录</div></div>
              <div v-else-if="inspectorTab === 'messages'" class="inspector-pane inspector-pane-list"><div v-for="message in personDetail?.messages || []" :key="message.id" class="message-row"><span>{{ cleanDisplayText(message.text) || '非文本消息' }}</span><small>{{ message.conversation_type || '未知会话' }} · {{ message.conversation_id || '未知' }} · {{ formatDate(message.sent_at) }}</small></div><div v-if="!personDetail?.messages?.length" class="inspector-blank">暂无消息记录</div></div>
              <div v-else-if="inspectorTab === 'feeds'" class="inspector-pane inspector-pane-list">
                <div v-if="personDetail?.contents?.length" class="person-feed-list">
                  <article
                    v-for="feed in personDetail.contents"
                    :key="feed.id"
                    class="person-feed-item"
                    @click="goToFeed(feed.id)"
                  >
                    <div class="person-feed-head">
                      <small class="feed-time">{{ formatDate(feed.published_at) }}</small>
                      <v-chip size="x-small" variant="tonal" color="primary">
                        {{ feed.context_type === "qzone_comment" ? "评论" : "动态" }}
                      </v-chip>
                    </div>
                    <p class="person-feed-body">{{ cleanDisplayText(feed.body) || "[无文本内容]" }}</p>
                    <div v-if="feed.metadata?.appShareTitle || feed.metadata?.musicShare?.songName" class="person-feed-share">
                      <v-icon icon="mdi-link-variant" size="13" color="primary" class="mr-1" />
                      <span>{{ cleanDisplayText(feed.metadata?.appShareTitle || feed.metadata?.musicShare?.songName) }}</span>
                    </div>
                    <div class="person-feed-foot">
                      <span><v-icon icon="mdi-thumb-up-outline" size="13" class="mr-1" />{{ feed.metadata?.likenum || feed.metadata?.like_count || 0 }}</span>
                      <span><v-icon icon="mdi-comment-outline" size="13" class="mr-1" />{{ feed.metadata?.cmtnum || feed.metadata?.comment_count || 0 }}</span>
                      <v-btn size="x-small" variant="text" color="primary" append-icon="mdi-arrow-right" class="ml-auto" @click.stop="goToFeed(feed.id)">在动态中查看</v-btn>
                    </div>
                  </article>
                </div>
                <div v-else class="inspector-blank">暂无已采集的空间动态</div>
              </div>
            </div>
          </template>
          <template v-else-if="isGroupLike(selection.data)">
            <v-tabs v-model="inspectorTab" density="compact" grow show-arrows>
              <v-tab value="profile">群资料</v-tab>
              <v-tab value="members">成员 {{ groupDetail?.member_count || 0 }}</v-tab>
              <v-tab value="messages">消息 {{ groupDetail?.message_count || 0 }}</v-tab>
            </v-tabs>
            <div class="inspector-window">
              <!-- 群资料 Tab -->
              <div v-if="inspectorTab === 'profile'" class="inspector-pane">
                <v-card variant="outlined" class="mb-3 pa-3 bg-surface-variant-subtle">
                  <div class="d-flex align-center gap-3 mb-2">
                    <v-avatar size="44" rounded="lg" color="indigo" variant="tonal">
                      <AuthImage
                        :src="avatarSource({ type: 'group', metadata: { group_id: groupDetail?.group_id || nodeIdentifier(selection.data), avatar_uri: groupDetail?.avatar_uri } })"
                        :fallback-srcs="avatarFallbacks({ type: 'group', metadata: { group_id: groupDetail?.group_id || nodeIdentifier(selection.data) } })"
                        alt=""
                      >
                        <span class="font-weight-bold">群</span>
                      </AuthImage>
                    </v-avatar>
                    <div class="flex-1 text-truncate">
                      <strong class="text-subtitle-2 font-weight-bold text-truncate d-block">{{ groupDetail?.group_name || selectionTitle(selection.data) }}</strong>
                      <span class="text-caption text-medium-emphasis">群号: {{ groupDetail?.group_id || nodeIdentifier(selection.data) }}</span>
                    </div>
                  </div>
                  <div class="d-flex align-center gap-2">
                    <v-chip size="x-small" color="primary" variant="tonal">
                      已采集成员 {{ groupDetail?.member_count || 0 }} 人
                    </v-chip>
                    <v-chip size="x-small" color="info" variant="tonal">
                      已采集消息 {{ groupDetail?.message_count || 0 }} 条
                    </v-chip>
                  </div>
                </v-card>

                <dl class="inspector-list compact">
                  <dt>直接拓扑关系</dt>
                  <dd>{{ selection.degree }} 条</dd>
                  <dt>首次发现时间</dt>
                  <dd>{{ formatDate(groupDetail?.first_seen_at) }}</dd>
                </dl>
                <div class="d-flex ga-2 mt-2 mb-2">
                  <v-btn
                    size="x-small"
                    color="primary"
                    variant="tonal"
                    class="flex-1-1"
                    prepend-icon="mdi-account-group-outline"
                    @click="goToGroup(groupDetail?.group_id || nodeIdentifier(selection.data))"
                  >
                    查看群组完整详情
                  </v-btn>
                  <v-btn
                    size="x-small"
                    color="secondary"
                    variant="tonal"
                    class="flex-1-1"
                    prepend-icon="mdi-crosshairs-gps"
                    @click="pivotToTarget(`group:${groupDetail?.group_id || nodeIdentifier(selection.data)}`)"
                  >
                    以此群为中心展开
                  </v-btn>
                </div>
              </div>

              <!-- 群成员 Tab -->
              <div v-else-if="inspectorTab === 'members'" class="inspector-pane inspector-pane-list">
                <v-list density="compact" class="pa-0 bg-transparent">
                  <v-list-item
                    v-for="member in groupDetail?.members || []"
                    :key="member.id"
                    class="px-2 py-1 mb-1 rounded border"
                    link
                    @click="selectNodeById(`person:${member.id}`)"
                  >
                    <template #prepend>
                      <v-avatar size="32" class="mr-2">
                        <AuthImage
                          :src="avatarSource({ type: 'person', metadata: { qq: member.qq, avatar_uri: member.avatar_uri } })"
                          :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: member.qq } })"
                          alt=""
                        >
                          <span style="font-size: 11px;">{{ String(member.display_name || '?').slice(0, 1) }}</span>
                        </AuthImage>
                      </v-avatar>
                    </template>
                    <v-list-item-title class="text-caption font-weight-bold">
                      {{ member.display_name || '未命名用户' }}
                    </v-list-item-title>
                    <v-list-item-subtitle class="text-caption text-medium-emphasis" style="font-size: 11px;">
                      QQ {{ member.qq || '未知' }}{{ member.card ? ' · 名片: ' + cleanDisplayText(member.card) : '' }}
                    </v-list-item-subtitle>
                    <template #append>
                      <v-chip size="x-small" variant="tonal" :color="member.role === 'owner' ? 'amber-darken-3' : (member.role === 'admin' ? 'primary' : 'default')">
                        {{ roleLabel(member.role) }}
                      </v-chip>
                    </template>
                  </v-list-item>
                </v-list>
                <div v-if="!groupDetail?.members?.length" class="inspector-blank">暂无成员记录</div>
              </div>

              <!-- 群消息 Tab -->
              <div v-else-if="inspectorTab === 'messages'" class="inspector-pane inspector-pane-list">
                <div v-for="message in groupDetail?.messages || []" :key="message.id" class="message-row pa-2 mb-2 rounded border">
                  <div class="d-flex align-center justify-space-between mb-1">
                    <strong class="text-caption font-weight-bold text-truncate" style="max-width: 140px;">{{ message.sender || '未知发送者' }}</strong>
                    <small class="text-caption text-medium-emphasis" style="font-size: 10px;">{{ formatDate(message.sent_at) }}</small>
                  </div>
                  <div class="drawer-evidence-bubble pa-2 rounded text-caption">
                    {{ cleanCQText(message.text) || '非文本消息' }}
                  </div>
                </div>
                <div v-if="!groupDetail?.messages?.length" class="inspector-blank">暂无群消息</div>
              </div>
            </div>
          </template>

          <!-- 动态/说说详情 -->
          <template v-else-if="selection.data.type === 'content'">
            <div class="content-inspector pa-2">
              <v-card variant="outlined" class="mb-3 pa-3 bg-surface-variant-subtle">
                <div class="d-flex align-center gap-2 mb-2">
                  <v-avatar size="32">
                    <v-icon icon="mdi-account-circle" size="28" color="primary" />
                  </v-avatar>
                  <div>
                    <strong class="text-caption font-weight-bold">{{ contentDetail?.author_name || selection.data.metadata?.author || '未知作者' }}</strong>
                    <div class="text-caption text-medium-emphasis" style="font-size: 11px;">
                      QQ {{ contentDetail?.author_qq || selection.data.metadata?.author_qq || '未知' }} · {{ formatDate(contentDetail?.published_at || selection.data.metadata?.published_at) }}
                    </div>
                  </div>
                </div>
                <div class="drawer-evidence-bubble pa-3 rounded text-caption mb-2">
                  {{ contentDetail?.body || selection.data.label || '[无文本内容]' }}
                </div>
              </v-card>

              <div v-if="extractContentShare(contentDetail || selection.data)" class="panel-share-card mt-2 mb-2" @click="openExternalUrl(extractContentShare(contentDetail || selection.data)?.url)">
                <v-icon icon="mdi-link-variant" size="18" color="primary" class="mr-2" />
                <div class="share-card-info">
                  <strong class="share-card-title">{{ extractContentShare(contentDetail || selection.data)?.title }}</strong>
                  <small v-if="extractContentShare(contentDetail || selection.data)?.subtitle" class="share-card-sub">{{ extractContentShare(contentDetail || selection.data)?.subtitle }}</small>
                </div>
              </div>

              <div v-if="contentDetail?.media?.length" class="content-inspector-media mb-3">
                <template v-for="media in contentDetail.media" :key="media.id">
                  <AuthImage v-if="media.kind === 'image' && media.asset_url" :src="media.asset_url" alt="" />
                  <img v-else-if="media.kind === 'image'" :src="media.source_url" alt="" referrerpolicy="no-referrer" />
                  <video v-else-if="media.kind === 'video'" :src="media.asset_url || media.source_url" controls preload="metadata" />
                </template>
              </div>

              <dl class="inspector-list compact mb-3">
                <dt>评论互动</dt>
                <dd>{{ contentDetail?.comments?.length || 0 }} 条</dd>
                <dt>点赞互动</dt>
                <dd>{{ contentDetail?.likes?.length || 0 }} 次</dd>
                <dt>直接拓扑关系</dt>
                <dd>{{ selection.degree }} 条</dd>
              </dl>

              <div class="inspector-section-title mb-2">
                <strong>互动评论明细 ({{ contentDetail?.comments?.length || 0 }})</strong>
              </div>
              <div v-for="comment in contentDetail?.comments || []" :key="comment.id" class="message-row pa-2 mb-2 rounded border">
                <div class="d-flex align-center justify-space-between mb-1">
                  <strong class="text-caption font-weight-bold">{{ comment.author_name || `QQ ${comment.author_qq}` }}</strong>
                  <small class="text-caption text-medium-emphasis" style="font-size: 10px;">{{ formatDate(comment.published_at) }}</small>
                </div>
                <div class="drawer-evidence-bubble pa-2 rounded text-caption">
                  {{ comment.body || '[无文本内容]' }}
                </div>
              </div>
            </div>
          </template>
          <template v-else-if="selection.data.type === 'group' && groupDetail">
            <dl class="inspector-list compact"><dt>群名称</dt><dd>{{ groupDetail.group_name || '未命名群' }}</dd><dt>群号</dt><dd>{{ groupDetail.group_id || '未知' }}</dd><dt>成员数</dt><dd>{{ groupDetail.members?.length || 0 }} 人</dd><dt>消息数</dt><dd>{{ groupDetail.messages?.length || 0 }} 条</dd><dt>直接关系</dt><dd>{{ selection.degree }} 条</dd><dt>首次发现</dt><dd>{{ formatDate(groupDetail.first_seen_at) }}</dd></dl>
            <div class="inspector-section-title"><strong>群成员</strong><span>{{ groupDetail.members?.length || 0 }}</span></div>
              <button v-for="m in (groupDetail.members || []).slice(0, 30)" :key="m.id" class="detail-row" @click="selectNodeById(`person:${m.id}`)"><span class="neighbor-avatar"><AuthImage :src="avatarSource({ type: 'person', metadata: { qq: m.qq, avatar_uri: m.avatar_uri } })" :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: m.qq } })" :alt="m.display_name"><span class="neighbor-avatar-fallback">{{ String(m.display_name || '?').slice(0, 1) }}</span></AuthImage></span><span class="neighbor-content"><strong>{{ m.display_name || '未命名' }}</strong><small>QQ {{ m.qq || '未知' }} · {{ roleLabel(m.role) }}</small></span></button>
            <div class="inspector-section-title"><strong>近期消息</strong><span>{{ groupDetail.messages?.length || 0 }}</span></div>
            <div v-for="msg in (groupDetail.messages || []).slice(0, 15)" :key="msg.id" class="message-row"><span>{{ cleanText(msg.text) }}</span><small>{{ msg.sender }} · {{ formatDate(msg.sent_at) }}</small></div>
          </template>
          <template v-else-if="selection.data.type === 'content' && contentDetail">
            <dl class="inspector-list compact"><dt>类型</dt><dd>{{ contentDetail.context_type === 'qzone_comment' ? '评论' : '动态' }}</dd><dt>作者</dt><dd>{{ contentDetail.author_name || 'QQ ' + (contentDetail.author_qq || '未知') }}</dd><dt>发布时间</dt><dd>{{ formatDate(contentDetail.published_at) }}</dd><dt>评论数</dt><dd>{{ contentDetail.comments?.length || 0 }}</dd><dt>直接关系</dt><dd>{{ selection.degree }} 条</dd></dl>
            <div v-if="contentDetail.body" class="content-body-preview">{{ cleanText(contentDetail.body) }}</div>
            <div v-if="contentDetail.media?.length" class="content-inspector-media"><template v-for="media in contentDetail.media" :key="media.id"><AuthImage v-if="media.kind === 'image' && media.asset_url" :src="media.asset_url" alt="图片"><span /></AuthImage><img v-else-if="media.kind === 'image' && media.source_url" :src="media.source_url" alt="图片" referrerpolicy="no-referrer" /></template></div>
            <div class="inspector-section-title"><strong>评论</strong><span>{{ contentDetail.comments?.length || 0 }}</span></div>
            <div v-for="c in (contentDetail.comments || []).slice(0, 20)" :key="c.id" class="message-row"><span>{{ cleanText(c.body || c.content || '') }}</span><small>{{ c.author_name || '匿名' }} · {{ formatDate(c.published_at) }}</small></div>
          </template>
          <template v-else-if="selection.data.type === 'message'">
            <div class="message-inspector pa-3">
              <!-- 发送者基本卡片 -->
              <div class="detail-row mb-3" style="background: rgba(255,255,255,0.03); border-radius: 4px; padding: 8px 10px;">
                <span class="neighbor-avatar">
                  <AuthImage
                    v-if="messageDetail?.sender_avatar"
                    :src="messageDetail.sender_avatar"
                    :alt="messageDetail.sender"
                  >
                    <span class="neighbor-avatar-fallback">{{ String(messageDetail.sender || '?').slice(0, 1) }}</span>
                  </AuthImage>
                  <i v-else style="background: #8ab4f8; color: #11161b; display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%;">
                    <v-icon icon="mdi-account" size="14" />
                  </i>
                </span>
                <span class="neighbor-content">
                  <strong>{{ messageDetail?.sender || selection.data.metadata?.sender || '未知发送者' }}</strong>
                  <small>QQ {{ messageDetail?.sender_qq || selection.data.metadata?.sender_qq || '未知' }}</small>
                </span>
                <v-chip size="x-small" color="primary" variant="tonal">
                  {{ messageDetail?.conversation_type === 'group' ? '群消息' : '私聊' }}
                </v-chip>
              </div>

              <!-- 消息正文气泡（解析 CQ 码并可读展示） -->
              <div class="message-bubble mb-3 pa-3" style="background: rgba(138, 180, 248, 0.08); border-left: 3px solid #8ab4f8; border-radius: 4px; font-size: 13px; line-height: 1.6; white-space: pre-wrap; word-break: break-word;">
                {{ cleanCQText(messageDetail?.text || selection.data.label) }}
              </div>

              <!-- 媒体附件 -->
              <div v-if="messageDetail?.media?.length" class="mb-3">
                <div class="inspector-section-title mb-2"><strong>附件媒体 ({{ messageDetail.media.length }})</strong></div>
                <div class="content-inspector-media">
                  <template v-for="m in messageDetail.media" :key="m.reference_id">
                    <AuthImage v-if="m.kind === 'image' && m.asset_url" :src="m.asset_url" alt="消息图片"><span /></AuthImage>
                    <audio v-else-if="m.kind === 'audio' && m.asset_url" :src="m.asset_url" controls style="grid-column: 1/-1; width: 100%;" />
                    <video v-else-if="m.kind === 'video' && m.asset_url" :src="m.asset_url" controls style="grid-column: 1/-1; width: 100%;" />
                  </template>
                </div>
              </div>

              <!-- 元数据列表 -->
              <dl class="inspector-list compact">
                <dt>发送时间</dt>
                <dd>{{ formatDate(messageDetail?.sent_at || selection.data.metadata?.sent_at) }}</dd>
                <dt>所属会话</dt>
                <dd>{{ messageDetail?.conversation_id ? (messageDetail.conversation_type === 'group' ? `群号 ${messageDetail.conversation_id}` : `QQ ${messageDetail.conversation_id}`) : (selection.data.metadata?.conversation_id || '未记录') }}</dd>
                <dt>直接关系</dt>
                <dd>{{ selection.degree }} 条</dd>
                <dt v-if="messageDetail?.source_message_id">消息 ID</dt>
                <dd v-if="messageDetail?.source_message_id">{{ messageDetail.source_message_id }}</dd>
              </dl>
            </div>
          </template>
          <template v-else>
            <dl class="inspector-list compact"><dt>节点类型</dt><dd>{{ nodeTypeLabel(selection.data.type) }}</dd><dt>直接关系</dt><dd>{{ selection.degree }} 条</dd></dl>
            <div class="inspector-blank">{{ entityError || '该节点暂无详细资料' }}</div>
          </template>
          <div class="inspector-actions">
            <v-btn size="small" variant="text" prepend-icon="mdi-vector-point-plus" :loading="expandingNode === selection.data.id" @click="expandNode(selection.data.id)">展开</v-btn>
            <v-btn v-if="selection.data.type === 'person'" size="small" :variant="workspace.compareNodeKeys.includes(selection.data.id) ? 'tonal' : 'text'" prepend-icon="mdi-compare-horizontal" @click="toggleCompare(selection.data.id)">比较</v-btn>
            <v-btn v-if="selection.data.type === 'message'" size="small" color="primary" variant="tonal" prepend-icon="mdi-forum-outline" title="跳转至聊天工作台追溯原始消息" @click="jumpToMessage(selection.data.id)">追溯原始消息</v-btn>
            <v-btn v-if="selection.data.type === 'content'" size="small" color="primary" variant="tonal" prepend-icon="mdi-newspaper-variant-outline" title="跳转至空间动态查看完整正文与点赞评论" @click="jumpToContent(selection.data.id)">查看动态详情</v-btn>
            <v-btn size="small" :variant="workspace.isolatedNodeKey === selection.data.id ? 'tonal' : 'text'" :color="workspace.isolatedNodeKey === selection.data.id ? 'primary' : undefined" :prepend-icon="workspace.isolatedNodeKey === selection.data.id ? 'mdi-filter-off-outline' : 'mdi-focus-field'" @click="toggleIsolation">{{ workspace.isolatedNodeKey === selection.data.id ? '显示全部' : '邻域' }}</v-btn>
            <v-btn size="small" variant="text" prepend-icon="mdi-target" title="在画布中居中显示" @click="centerSelected">居中</v-btn>
          </div>
          <div class="inspector-section-title"><strong>相邻节点</strong><span>{{ neighbors.length }}</span></div>
          <button v-for="neighbor in neighbors" :key="neighbor.id" class="neighbor-row" @click="selectNodeById(neighbor.id)"><span class="neighbor-avatar"><AuthImage v-if="neighbor.hasAvatar" :src="neighbor.avatarUri" :fallback-srcs="neighbor.avatarFallbacks" :alt="neighbor.label"><span class="neighbor-avatar-fallback">{{ String(neighbor.label || '?').slice(0, 1) }}</span></AuthImage><i v-else :style="{ background: nodeColor(neighbor.type) }"><v-icon :icon="nodeIcon(neighbor.type)" size="13" /></i></span><span class="neighbor-content"><strong>{{ neighbor.label || '未命名节点' }}</strong><small>{{ neighbor.typeLabel }} · {{ neighbor.identifier }}</small><small class="neighbor-relations">{{ neighbor.relations.join('、') }}</small></span><em>{{ neighbor.weight }} 次</em></button>
        </template>
        <template v-else-if="selection?.kind === 'edge'">
          <div class="inspector-top">
            <v-icon icon="mdi-vector-line" color="primary" />
            <div class="inspector-title">
              <span>关系分析</span>
              <h2>{{ relationLabel(selection.data.relationType) }}</h2>
            </div>
            <v-btn icon="mdi-close" size="x-small" variant="text" @click="clearSelection" />
          </div>

          <!-- 双实体画像连通卡片 -->
          <v-card variant="outlined" class="mx-3 mt-2 mb-3 bg-surface-variant-subtle pa-3">
            <div class="d-flex align-center justify-space-between gap-1">
              <!-- 起点实体 -->
              <div
                class="d-flex flex-column align-center text-center cursor-pointer flex-1"
                style="max-width: 44%;"
                @click="selection.sourceNode?.id && selectNodeById(selection.sourceNode.id)"
              >
                <span class="neighbor-avatar mb-1">
                  <AuthImage
                    v-if="selection.sourceNode"
                    :src="avatarSource({ type: selection.sourceNode.type, metadata: selection.sourceNode.metadata })"
                    :fallback-srcs="avatarFallbacks({ type: selection.sourceNode.type, metadata: selection.sourceNode.metadata })"
                    alt=""
                  >
                    <span class="neighbor-avatar-fallback">{{ String(selection.sourceNode.label || '?').slice(0, 1) }}</span>
                  </AuthImage>
                </span>
                <strong class="text-caption font-weight-bold text-truncate w-100" :title="selection.sourceNode?.label">
                  {{ cleanDisplayText(selection.sourceNode?.label) || '未命名' }}
                </strong>
                <span class="text-caption text-medium-emphasis text-truncate w-100">
                  {{ nodeIdentifier(selection.sourceNode) }}
                </span>
              </div>

              <!-- 中间关系导向胶囊 -->
              <div class="d-flex flex-column align-center px-1">
                <v-chip size="x-small" color="primary" variant="flat" class="font-weight-bold mb-1">
                  {{ relationLabel(selection.data.relationType) }}
                </v-chip>
                <v-icon icon="mdi-arrow-right-thin" size="20" color="primary" />
                <span class="text-caption font-weight-bold text-primary">{{ selection.data.weight }} 次</span>
              </div>

              <!-- 终点实体 -->
              <div
                class="d-flex flex-column align-center text-center cursor-pointer flex-1"
                style="max-width: 44%;"
                @click="selection.targetNode?.id && selectNodeById(selection.targetNode.id)"
              >
                <span class="neighbor-avatar mb-1">
                  <AuthImage
                    v-if="selection.targetNode"
                    :src="avatarSource({ type: selection.targetNode.type, metadata: selection.targetNode.metadata })"
                    :fallback-srcs="avatarFallbacks({ type: selection.targetNode.type, metadata: selection.targetNode.metadata })"
                    alt=""
                  >
                    <span class="neighbor-avatar-fallback">{{ String(selection.targetNode.label || '?').slice(0, 1) }}</span>
                  </AuthImage>
                </span>
                <strong class="text-caption font-weight-bold text-truncate w-100" :title="selection.targetNode?.label">
                  {{ cleanDisplayText(selection.targetNode?.label) || '未命名' }}
                </strong>
                <span class="text-caption text-medium-emphasis text-truncate w-100">
                  {{ nodeIdentifier(selection.targetNode) }}
                </span>
              </div>
            </div>
          </v-card>

          <!-- 关系指标列表 -->
          <dl class="inspector-list compact mb-3">
            <dt>首次发现</dt>
            <dd>{{ formatDate(selection.data.firstSeen) }}</dd>
            <dt>最后发现</dt>
            <dd>{{ formatDate(selection.data.lastSeen) }}</dd>
            <dt>证据引用</dt>
            <dd>{{ selection.data.evidenceIds?.length || 0 }} 条</dd>
          </dl>

          <!-- 全屏深度研判操作按钮 -->
          <div class="px-3 mb-3">
            <v-btn
              block
              color="primary"
              variant="tonal"
              prepend-icon="mdi-arrow-expand-all"
              size="small"
              @click="openDetailedEvidenceForEdge(selection.data)"
            >
              打开全屏研判模式 ({{ selection.data.evidenceIds?.length || 0 }} 条记录)
            </v-btn>
          </div>

          <!-- 抽屉内嵌真实互动明细流 -->
          <div class="inspector-section-title px-3 mb-2">
            <strong>真实互动记录流</strong>
            <span v-if="edgeEvidenceLoading">加载中...</span>
            <span v-else>{{ edgeEvidenceDetails.length }} 条明细</span>
          </div>

          <v-progress-linear v-if="edgeEvidenceLoading" indeterminate color="primary" class="mb-2" />

          <div v-if="edgeEvidenceDetails.length" class="edge-evidence-drawer-stream px-3 mb-4">
            <div
              v-for="item in edgeEvidenceDetails"
              :key="item.id"
              class="edge-drawer-item pa-2 mb-2 rounded border"
            >
              <div class="d-flex align-center justify-space-between mb-1">
                <div class="d-flex align-center gap-1">
                  <v-avatar size="24" class="mr-1">
                    <AuthImage
                      v-if="item.actor_avatar"
                      :src="item.actor_avatar"
                      :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: item.actor_qq } })"
                      :alt="item.actor_name"
                    >
                      <span class="mini-char">{{ String(item.actor_name || '?').slice(0, 1) }}</span>
                    </AuthImage>
                    <v-icon v-else icon="mdi-account" size="14" />
                  </v-avatar>
                  <strong class="text-caption font-weight-bold text-truncate" style="max-width: 100px;">{{ item.actor_name }}</strong>
                  <v-chip size="x-small" :color="getDetailedActionColor(item.action_type)" variant="flat" style="font-size: 10px; height: 16px; padding: 0 4px;">
                    {{ item.action_label }}
                  </v-chip>
                </div>
                <span class="text-caption text-medium-emphasis" style="font-size: 10px;">
                  {{ formatDate(item.occurred_at) }}
                </span>
              </div>

              <div v-if="item.context_name" class="text-caption text-medium-emphasis mb-1" style="font-size: 10px;">
                <v-icon icon="mdi-source-branch" size="10" class="mr-1" />
                {{ item.context_name }}
              </div>

              <div v-if="item.content_text" class="drawer-evidence-bubble pa-2 rounded mb-1 text-caption">
                {{ item.content_text }}
              </div>

              <div v-if="item.parent_snippet" class="drawer-quote-box pa-1 rounded text-caption mb-1">
                <div class="font-weight-bold text-primary" style="font-size: 10px;">
                  <v-icon icon="mdi-reply" size="10" />
                  {{ item.parent_title || '引用回复：' }}
                </div>
                <div class="text-truncate" style="font-size: 10px;">{{ item.parent_snippet }}</div>
              </div>

              <div v-if="item.id || item.action_type" class="d-flex justify-end gap-1">
                <v-btn
                  v-if="item.action_type === 'message' || item.context_type === 'group' || item.context_type === 'private'"
                  size="x-small"
                  variant="text"
                  color="primary"
                  density="compact"
                  prepend-icon="mdi-open-in-new"
                  style="font-size: 10px;"
                  @click="jumpToMessage(item.id)"
                >
                  定位聊天消息
                </v-btn>
                <v-btn
                  v-if="item.action_type === 'feed' || item.action_type === 'comment' || item.action_type === 'liked' || item.context_type?.includes('qzone')"
                  size="x-small"
                  variant="text"
                  color="primary"
                  density="compact"
                  prepend-icon="mdi-open-in-new"
                  style="font-size: 10px;"
                  @click="jumpToContent(item.id || item.target_object_id)"
                >
                  查看动态详情
                </v-btn>
              </div>
            </div>
          </div>
          <div v-else-if="!edgeEvidenceLoading" class="inspector-blank">
            暂无已解析的文字明细
          </div>

          <!-- 底层原始抓包审计 (可折叠/列表) -->
          <div class="inspector-section-title px-3 mb-1">
            <strong>底层数据抓包</strong>
            <span v-if="evidenceLoading">抓取中</span>
          </div>
          <button v-for="item in evidenceSummaries" :key="item.id" class="evidence-summary-row" @click="openEvidence(item)">
            <v-icon icon="mdi-file-document-outline" size="17" />
            <span>
              <strong>{{ item.endpoint }}</strong>
              <small>{{ formatDate(item.collected_at) }} · {{ item.source }}</small>
            </span>
            <v-icon icon="mdi-chevron-right" size="16" />
          </button>
          <div v-if="!evidenceLoading && !evidenceSummaries.length" class="inspector-blank">无可用证据抓包</div>
        </template>
        <div v-else class="inspector-placeholder"><v-icon icon="mdi-cursor-default-click-outline" size="30" /><strong>未选择对象</strong><span>点击节点或关系查看资料</span></div>
      </aside>
    </section>

    <!-- 具体互动记录与证据流深度研判弹窗 -->
    <v-dialog v-model="detailedEvidenceOpen" max-width="920" scrollable>
      <v-card class="evidence-dialog-card">
        <v-card-title class="d-flex align-center px-6 py-4 border-b">
          <div class="d-flex flex-column">
            <div class="d-flex align-center gap-2">
              <v-icon icon="mdi-message-text-clock-outline" color="primary" class="mr-2" />
              <h3 class="text-subtitle-1 font-weight-bold mb-0">{{ detailedEvidenceTitle || '具体互动明细研判' }}</h3>
            </div>
            <span class="text-caption text-medium-emphasis mt-1">{{ detailedEvidenceSubtitle }}</span>
          </div>
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" density="compact" @click="detailedEvidenceOpen = false" />
        </v-card-title>

        <div class="evidence-filter-bar px-6 py-3 border-b d-flex align-center flex-wrap justify-space-between gap-3">
          <v-chip-group
            v-model="detailedEvidenceFilter"
            mandatory
            class="filter-chips-group"
            variant="outlined"
            selected-class="text-primary font-weight-bold"
          >
            <v-chip
              value="all"
              size="small"
              filter
            >
              全部 ({{ detailedEvidenceList.length }})
            </v-chip>
            <v-chip
              v-if="detailedEvidenceCommonGroups.length || evidenceActionCounts['common_group']"
              value="common_group"
              size="small"
              color="indigo"
              filter
              prepend-icon="mdi-account-group-outline"
            >
              共同群聊 ({{ detailedEvidenceCommonGroups.length || evidenceActionCounts['common_group'] || 0 }})
            </v-chip>
            <v-chip
              v-if="evidenceActionCounts['message']"
              value="message"
              size="small"
              color="info"
              filter
              prepend-icon="mdi-chat-processing-outline"
            >
              聊天消息 ({{ evidenceActionCounts['message'] }})
            </v-chip>
            <v-chip
              v-if="evidenceActionCounts['commented'] || evidenceActionCounts['replied_to']"
              value="commented"
              size="small"
              color="success"
              filter
              prepend-icon="mdi-comment-text-multiple-outline"
            >
              说说评论 ({{ (evidenceActionCounts['commented'] || 0) + (evidenceActionCounts['replied_to'] || 0) }})
            </v-chip>
            <v-chip
              v-if="evidenceActionCounts['liked']"
              value="liked"
              size="small"
              color="amber-darken-2"
              filter
              prepend-icon="mdi-thumb-up-outline"
            >
              空间点赞 ({{ evidenceActionCounts['liked'] }})
            </v-chip>
            <v-chip
              v-if="evidenceActionCounts['published']"
              value="published"
              size="small"
              color="purple"
              filter
              prepend-icon="mdi-newspaper-variant-outline"
            >
              空间动态 ({{ evidenceActionCounts['published'] }})
            </v-chip>
          </v-chip-group>

          <v-text-field
            v-model="detailedEvidenceSearch"
            placeholder="搜索消息内容、人员或关键词..."
            density="compact"
            variant="outlined"
            hide-details
            clearable
            prepend-inner-icon="mdi-magnify"
            style="min-width: 280px; max-width: 320px;"
          />
        </div>

        <v-card-text class="pa-6" style="max-height: 65vh; overflow-y: auto;">
          <div v-if="detailedEvidenceLoading" class="py-12 text-center">
            <v-progress-circular indeterminate color="primary" size="36" />
            <div class="text-caption text-medium-emphasis mt-3">正在拉取并解析原始互动明细与证据链...</div>
          </div>

          <div v-else-if="!filteredDetailedEvidence.length" class="py-12 text-center text-medium-emphasis">
            <v-icon icon="mdi-text-box-remove-outline" size="48" class="mb-2 opacity-50" />
            <div class="font-weight-medium">暂无匹配的互动记录</div>
            <div class="text-caption mt-1">未检索到该筛选条件下的具体文字或抓包证据</div>
          </div>

          <div v-else class="evidence-timeline-stream">
            <!-- 双方共同加入的群聊卡片 -->
            <div v-if="detailedEvidenceCommonGroups.length && (detailedEvidenceFilter === 'all' || detailedEvidenceFilter === 'common_group')" class="common-groups-banner mb-4">
              <div class="d-flex align-center gap-1 mb-2">
                <v-icon icon="mdi-account-group" size="18" color="indigo" />
                <strong class="text-subtitle-2 font-weight-bold">双方共同加入的群聊 ({{ detailedEvidenceCommonGroups.length }})</strong>
              </div>
              <v-card v-for="g in detailedEvidenceCommonGroups" :key="g.id" variant="outlined" class="mb-2 pa-3 bg-surface-variant-subtle">
                <div class="d-flex align-center justify-space-between flex-wrap gap-2">
                  <div class="d-flex align-center gap-3">
                    <v-avatar size="40" rounded="lg" color="indigo" variant="tonal">
                      <AuthImage :src="g.avatar_uri" :alt="g.name">
                        <span class="font-weight-bold">群</span>
                      </AuthImage>
                    </v-avatar>
                    <div>
                      <strong class="text-body-2 font-weight-bold">{{ g.name }}</strong>
                      <div class="text-caption text-medium-emphasis">群号: {{ g.platform_group_id }} · 成员规模: {{ g.member_count }} 人</div>
                    </div>
                  </div>
                  <v-btn size="small" variant="tonal" color="primary" prepend-icon="mdi-target" @click="selectNodeById('group:' + g.id); detailedEvidenceOpen = false;">
                    在画布中定位群
                  </v-btn>
                </div>
                <v-divider class="my-2" />
                <div class="d-flex align-center gap-4 text-caption flex-wrap">
                  <div><span class="text-medium-emphasis">起点角色/名片:</span> {{ roleLabel(g.role_1) }}{{ g.card_1 ? ` · 名片: ${g.card_1}` : '' }}</div>
                  <div><span class="text-medium-emphasis">终点角色/名片:</span> {{ roleLabel(g.role_2) }}{{ g.card_2 ? ` · 名片: ${g.card_2}` : '' }}</div>
                </div>
              </v-card>
            </div>

            <div
              v-for="(item, idx) in filteredDetailedEvidence"
              :key="item.id || idx"
              class="mb-3"
            >
              <!-- QZone Feed Card for QZone comments / likes / posts -->
              <QZoneFeedCard
                v-if="item.feed_snapshot || isQZoneAction(item.action_type)"
                :feed="item.feed_snapshot || { body: item.parent_snippet || item.content_text, author_name: item.target_name || item.actor_name, author_qq: item.target_qq || item.actor_qq, published_at: item.occurred_at }"
                :interaction="item"
              />

              <!-- Standard Chat Message bubble card -->
              <div v-else class="evidence-stream-card">
                <div class="card-top-line">
                  <div class="actor-box">
                    <v-avatar size="28" class="mr-1">
                      <AuthImage
                        v-if="item.actor_avatar"
                        :src="item.actor_avatar"
                        :fallback-srcs="avatarFallbacks({ type: 'person', metadata: { qq: item.actor_qq } })"
                        :alt="item.actor_name"
                      >
                        <span class="avatar-fallback-char">{{ String(item.actor_name || '?').slice(0, 1) }}</span>
                      </AuthImage>
                      <v-icon v-else icon="mdi-account" size="18" />
                    </v-avatar>
                    <span class="actor-name font-weight-bold">{{ item.actor_name || '未知人员' }}</span>
                    <span v-if="item.actor_qq" class="actor-qq text-caption text-medium-emphasis">({{ item.actor_qq }})</span>
                    <v-chip
                      size="x-small"
                      variant="flat"
                      :color="getDetailedActionColor(item.action_type)"
                      class="ml-1"
                    >
                      {{ item.action_label }}
                    </v-chip>
                  </div>

                  <div class="context-box">
                    <span v-if="item.context_name || item.parent_title" class="context-tag">
                      <v-icon icon="mdi-source-branch" size="12" class="mr-1" />
                      {{ item.context_name || item.parent_title }}
                    </span>
                    <span class="time-stamp text-caption text-medium-emphasis ml-2">
                      {{ formatDate(item.occurred_at) }}
                    </span>
                  </div>
                </div>

                <!-- Main text content / bubble -->
                <div class="content-bubble-container">
                  <div v-if="item.content_text" class="content-text-bubble">
                    {{ item.content_text }}
                  </div>
                  <div v-else class="content-text-empty">
                    [无文本内容 / 状态变更]
                  </div>

                  <!-- Context Quote (Reply / Comment / Preceding message) -->
                  <div v-if="item.parent_snippet" class="quote-context-box">
                    <div class="quote-tag font-weight-bold">
                      <v-icon icon="mdi-reply" size="14" class="mr-1" />
                      {{ item.parent_title || '引用上下文：' }}
                    </div>
                    <div class="quote-text">{{ item.parent_snippet }}</div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </v-card-text>

        <v-divider />
        <v-card-actions class="px-6 py-3">
          <span class="text-caption text-medium-emphasis">
            当前展示 {{ filteredDetailedEvidence.length }} 条明细记录
          </span>
          <v-spacer />
          <v-btn variant="tonal" color="primary" @click="detailedEvidenceOpen = false">
            完成查阅
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>

    <v-dialog v-model="evidenceDialog" max-width="900"><v-card><v-card-title class="d-flex align-center pa-5"><span>原始证据</span><v-spacer /><v-btn icon="mdi-close" variant="text" @click="evidenceDialog = false" /></v-card-title><v-divider /><v-card-text class="pa-5"><div v-if="evidence" class="evidence-meta"><span>{{ evidence.source }}</span><span>{{ evidence.endpoint }}</span><span>{{ formatDate(evidence.collected_at) }}</span></div><pre v-if="evidence" class="json-viewer">{{ JSON.stringify(evidence.payload, null, 2) }}</pre></v-card-text></v-card></v-dialog>
    <v-dialog v-model="snapshotDialog" max-width="420"><v-card><v-card-title>保存研究快照</v-card-title><v-card-text><v-text-field v-model.trim="snapshotName" label="快照名称" autofocus hide-details @keyup.enter="createSnapshot" /></v-card-text><v-card-actions><v-spacer/><v-btn variant="text" @click="snapshotDialog=false">取消</v-btn><v-btn color="primary" :disabled="!snapshotName" :loading="snapshotSaving" @click="createSnapshot">保存</v-btn></v-card-actions></v-card></v-dialog>
    <v-dialog v-model="snapshotManagerOpen" max-width="620" scrollable>
      <v-card>
        <v-card-title class="d-flex align-center">研究快照<v-spacer /><v-btn icon="mdi-close" variant="text" title="关闭快照管理" @click="snapshotManagerOpen = false" /></v-card-title>
        <v-divider />
        <v-card-text class="pa-0">
          <div v-if="!snapshots.length" class="empty-table pa-8"><v-icon icon="mdi-bookmark-off-outline" /><strong>暂无研究快照</strong><span>先保存当前研究状态。</span></div>
          <div v-else class="snapshot-list">
            <div v-for="item in snapshots" :key="item.id" class="snapshot-row">
              <div><strong>{{ item.name || '未命名快照' }}</strong><small>{{ formatDate(item.created_at) }}</small></div>
              <div class="snapshot-actions"><v-btn size="small" variant="tonal" prepend-icon="mdi-history" @click="restoreSnapshot(item)">恢复</v-btn><v-btn icon="mdi-delete-outline" size="small" variant="text" color="error" title="删除快照" @click="deleteSnapshot(item.id)" /></div>
            </div>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import cytoscape, { type Core, type EventObject, type NodeSingular } from "cytoscape";
import fcose from "cytoscape-fcose";
cytoscape.use(fcose);
import { useGraphWorkspaceStore } from "@/stores/graphWorkspace";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";
import { api } from "@/services/api";
import { cachedMediaURL } from "@/services/mediaCache";
import AuthImage from "@/components/AuthImage.vue";
import QZoneFeedCard from "@/components/QZoneFeedCard.vue";
import GraphLayoutWorker from "@/workers/graphLayout.worker?worker";
import { useRoute, useRouter } from "vue-router";
import { cleanDisplayText } from "@/utils/text";
import { qqOf } from "@/utils/identity";

type GraphNode = { key: string; type: string; label: string; metadata?: Record<string, unknown> };
type GraphEdge = { source: string; target: string; relation_type: string; weight: number; evidence_ids: string[]; first_seen: string; last_seen: string };
type Graph = { nodes: GraphNode[]; edges: GraphEdge[] };
type GraphDistanceLevel = { distance: number; nodes: number };
type GraphLimits = { available_nodes: number; available_edges: number; available_max_distance: number; distance_levels: GraphDistanceLevel[]; server_max_nodes: number | null; server_max_distance: number | null };
type GraphView = { graph: Graph; partial: boolean; partialData?: boolean; totalNodes: number; totalEdges: number; scopeNodes: number; scopeEdges: number; limits?: GraphLimits };
type CommunityMember = { key: string; type: string; label: string; metadata?: Record<string, unknown> };
type Community = { id: string; label: string; node_count: number; edge_count: number; internal_weight: number; external_weight: number; person_count: number; group_count: number; content_count: number; conversation_count: number; message_count: number; other_count: number; target: boolean; members: CommunityMember[]; member_keys: string[] };
type PersonDetail = { id: string; display_name: string; avatar_uri?: string; first_seen_at?: string; last_seen_at?: string; identifiers?: any[]; profiles?: any[]; profile_count?: number; memberships?: any[]; membership_count?: number; messages?: any[]; message_count?: number; contents?: any[]; content_count?: number; relations?: any[]; relation_count?: number };
type RouteStep = {
  step_number: number;
  source_key: string;
  source_type: string;
  source_label: string;
  source_meta?: Record<string, any>;
  target_key: string;
  target_type: string;
  target_label: string;
  target_meta?: Record<string, any>;
  relation_type: string;
  weight: number;
  cost: number;
  medium_type: string;
  medium_summary: string;
  evidence_ids?: string[];
  first_seen?: string;
  last_seen?: string;
};
type PlannedRoute = {
  path_index: number;
  title: string;
  total_hops: number;
  total_weight: number;
  total_cost: number;
  confidence: number;
  summary: string;
  steps: RouteStep[];
  nodes?: GraphNode[];
  edges?: GraphEdge[];
};
type RouteResult = {
  source_qq: string;
  target_qq: string;
  profile: string;
  paths_found: number;
  routes: PlannedRoute[];
};
type RelationshipDeepData = {
  this_person?: any;
  target_person?: any;
  directionality?: {
    this_to_target: { total: number; likes: number; comments: number; messages: number };
    target_to_this: { total: number; likes: number; comments: number; messages: number };
    this_ratio_pct: number;
    target_ratio_pct: number;
    power_dynamics: string;
  };
  response_latency?: {
    fast_responses_under_5min: number;
    avg_latency_minutes: number;
    special_attention_suspected: boolean;
  };
  circadian_rhythm?: {
    this_hours: number[];
    target_hours: number[];
    similarity_score_pct: number;
  };
  intermediary_triads?: Array<{
    id: string;
    name: string;
    qq: string;
    weight_this: number;
    weight_target: number;
    total_weight: number;
  }>;
  category?: string;
};

const route = useRoute();
const router = useRouter();
const workspace = useGraphWorkspaceStore();
const systemCapabilities = useSystemCapabilitiesStore();
const graphLimits = ref<GraphLimits | null>(null);
const target = computed({ get: () => workspace.targetQQ, set: (v) => (workspace.targetQQ = v) });
const depth = computed({ get: () => workspace.depth, set: (v) => (workspace.depth = Number(v)) });

// 链路算路导航与深度关系状态
const routeMode = ref(false);
const routeSourceQQ = ref("");
const routeTargetQQ = ref("");
const routeProfile = ref<"least_cost" | "shortest" | "covert">("least_cost");
const routeMaxHops = ref<number>(4);
const routeLoading = ref(false);
const routeError = ref("");
const routeResult = ref<RouteResult | null>(null);
const activeRouteIndex = ref(0);
const hoveredStepNumber = ref<number | null>(null);
const waybillExpanded = ref(true);
const hoveredNodeKey = ref<string | null>(null);

function hoverNode(key: string) {
  hoveredNodeKey.value = key;
  if (!cy) return;
  const n = cy.getElementById(key);
  if (n && n.length) {
    n.addClass("step-highlighted");
  }
}

function clearHoverNode() {
  hoveredNodeKey.value = null;
  if (!cy) return;
  cy.nodes().removeClass("step-highlighted");
}

const sourceSearchText = ref("");
const sourceSearching = ref(false);
const sourceOptions = ref<Array<{ title: string; value: string; subtitle: string; avatar: string; fallbacks: string[] }>>([]);
let sourceSearchTimer: number | undefined;

const routeTargetSearchText = ref("");
const routeTargetSearching = ref(false);
const routeTargetOptions = ref<Array<{ title: string; value: string; subtitle: string; avatar: string; fallbacks: string[] }>>([]);
let routeTargetSearchTimer: number | undefined;

const relationshipDeepData = ref<RelationshipDeepData | null>(null);
const relationshipDeepLoading = ref(false);

const activeRoute = computed(() => {
  if (!routeResult.value?.routes?.length) return null;
  return routeResult.value.routes[activeRouteIndex.value] || routeResult.value.routes[0];
});

const routeProfileOptions = [
  { value: "least_cost", title: "最强加权主干道 (加权优先)" },
  { value: "shortest", title: "极简最少跳数 (直达)" },
  { value: "covert", title: "隐秘小道 (避开大群)" },
];

const routeHopOptions = [2, 3, 4, 5, 6].map((h) => ({
  value: h,
  title: `${h} 跳`,
}));

const maxCircadianCount = computed(() => {
  if (!relationshipDeepData.value?.circadian_rhythm) return 1;
  const rh = relationshipDeepData.value.circadian_rhythm;
  const maxThis = Math.max(...(rh.this_hours || [0]), 1);
  const maxTarget = Math.max(...(rh.target_hours || [0]), 1);
  return Math.max(maxThis, maxTarget, 1);
});

const targetSearchText = ref("");
const targetSearching = ref(false);
const targetOptions = ref<Array<{ title: string; value: string; subtitle: string; avatar: string; fallbacks: string[] }>>([]);
let targetSearchTimer: number | undefined;

function onTargetSearchFocus() {
  fetchTargetOptions(targetSearchText.value || "");
}

function onTargetSearchInput(text: string) {
  if (targetSearchTimer) window.clearTimeout(targetSearchTimer);
  targetSearchTimer = window.setTimeout(() => {
    fetchTargetOptions(text);
  }, 200);
}

async function fetchTargetOptions(query: string) {
  const q = String(query || "").trim();
  targetSearching.value = true;
  try {
    const res = await api.get("/api/v1/persons", { params: { q, limit: 15 } });
    const list = res.data?.data || [];
    targetOptions.value = list.map((m: any) => {
      const qq = qqOf(m);
      return {
        title: m.display_name || qq || m.id,
        value: qq || m.id,
        subtitle: qq ? `QQ: ${qq}` : "",
        avatar: m.avatar_uri || (qq ? `/api/v1/media/avatars/person/${qq}` : ""),
        fallbacks: qq ? [`https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=640`] : [],
      };
    });
  } catch {
    targetOptions.value = [];
  } finally {
    targetSearching.value = false;
  }
}

function onTargetSelect(item: any) {
  if (!item) return;
  if (typeof item === "object") {
    target.value = item.value || item.title || "";
  } else {
    target.value = String(item).trim();
  }
  build();
}

function pivotToTarget(newTarget: string) {
  if (!newTarget) return;
  target.value = String(newTarget).trim();
  build();
}

const nodeTypes = computed({ get: () => workspace.nodeTypes, set: (v) => (workspace.nodeTypes = v) });
const relationTypes = computed({ get: () => workspace.relationTypes, set: (v) => (workspace.relationTypes = v) });
const minimumWeight = computed({ get: () => workspace.minimumWeight, set: (v) => (workspace.minimumWeight = Math.max(1, Number(v) || 1)) });
const targetNeighborsOnly = computed({ get: () => workspace.targetNeighborsOnly, set: (v) => (workspace.targetNeighborsOnly = v) });
const showLabels = computed({ get: () => workspace.showLabels, set: (v) => (workspace.showLabels = v) });
const wheelZoom = computed({ get: () => workspace.wheelZoom, set: (v) => (workspace.wheelZoom = v) });
const layoutName = computed({ get: () => workspace.layout, set: (v) => (workspace.layout = v) });
const zoom = computed({ get: () => workspace.zoom, set: (v) => (workspace.zoom = v) });
const graph = ref<Graph | null>(null), loading = ref(false), error = ref(""), container = ref<HTMLElement>();
const graphPartial = ref(false), expandingNode = ref("");
const storedNodeCount = ref(0), storedEdgeCount = ref(0);
const scopeNodeCount = ref(0), scopeEdgeCount = ref(0);
const searchNode = ref(""), selection = ref<any>(null);
const inspectorTab = computed({ get: () => workspace.inspectorTab, set: (value) => (workspace.inspectorTab = value) });
const communities = ref<Community[]>([]), communityLoading = ref(false), communityError = ref(""), expandingCommunity = ref(""), activeCommunityId = ref("");
const evidenceSummaries = ref<any[]>([]), evidenceLoading = ref(false), evidenceDialog = ref(false), evidence = ref<any>(null);
const edgeEvidenceDetails = ref<any[]>([]), edgeEvidenceLoading = ref(false);
const detailedEvidenceOpen = ref(false), detailedEvidenceLoading = ref(false);
const detailedEvidenceTitle = ref(""), detailedEvidenceSubtitle = ref("");
const detailedEvidenceList = ref<any[]>([]);
const detailedEvidenceCommonGroups = ref<any[]>([]);
const detailedEvidenceFilter = ref("all");
const detailedEvidenceSearch = ref("");

const filteredDetailedEvidence = computed(() => {
  let list = detailedEvidenceList.value;
  if (detailedEvidenceFilter.value !== "all") {
    list = list.filter((item) => item.action_type === detailedEvidenceFilter.value);
  }
  if (detailedEvidenceSearch.value.trim()) {
    const q = detailedEvidenceSearch.value.trim().toLowerCase();
    list = list.filter(
      (item) =>
        (item.content_text && item.content_text.toLowerCase().includes(q)) ||
        (item.actor_name && item.actor_name.toLowerCase().includes(q)) ||
        (item.actor_qq && item.actor_qq.toLowerCase().includes(q)) ||
        (item.parent_snippet && item.parent_snippet.toLowerCase().includes(q)) ||
        (item.context_name && item.context_name.toLowerCase().includes(q))
    );
  }
  return list;
});

const evidenceActionCounts = computed(() => {
  const counts: Record<string, number> = { all: detailedEvidenceList.value.length };
  for (const item of detailedEvidenceList.value) {
    counts[item.action_type] = (counts[item.action_type] || 0) + 1;
  }
  return counts;
});

function getDetailedActionColor(actionType: string) {
  switch (actionType) {
    case "message":
      return "info";
    case "commented":
    case "replied_to":
      return "success";
    case "liked":
      return "amber-darken-2";
    case "published":
      return "purple";
    default:
      return "primary";
  }
}
const snapshots = ref<any[]>([]), snapshotDialog = ref(false), snapshotManagerOpen = ref(false), snapshotName = ref(""), snapshotSaving = ref(false);
const largeRenderDialog = ref(false), nodeLimitDraft = ref(workspace.maxNodes), pendingNodeLimit = ref<number | null>(null);
const personDetail = ref<PersonDetail | null>(null), personLoading = ref(false), personError = ref("");
const relationshipData = ref<any>(null), relationshipLoading = ref(false);
const timelineData = ref<any[]>([]), timelineTotal = ref(0), timelineLoading = ref(false);
const groupDetail = ref<any>(null), contentDetail = ref<any>(null), messageDetail = ref<any>(null), entityLoading = ref(false), entityError = ref("");
const visibleNodeCount = ref(0), visibleEdgeCount = ref(0);
const progressiveRendering = ref(false), progressiveRenderedCount = ref(0), progressiveTotalCount = ref(0);
const layoutRunning = ref(false);
let cy: Core | undefined;
let layoutWorker: Worker | undefined;
let currentLayout: any;
let layoutToken = 0;
let renderGeneration = 0;
let inspectorRequest = 0;
let evidenceRequest = 0;
let draftTimer = 0;
let graphReloadTimer = 0;
let viewportPersistTimer: number | undefined;
let labelDensityFrame: number | undefined;
let filterFrame: number | undefined;
let workspacePersistTimer: number | undefined;
let progressiveRenderFrame: number | undefined;
let progressiveRenderToken = 0;
let stopWorkspaceSubscription: (() => void) | undefined;
const avatarImages = new Map<string, string>();
const avatarFailures = new Map<string, number>();
const avatarLoads = new Map<string, Promise<string>>();
let labelsSuppressed = false;
function activeCy(): Core | undefined { return cy && !cy.destroyed() ? cy : undefined; }
const graphIndex = computed(() => {
  const nodeTypeCounts = new Map<string, number>();
  const relationTypeCounts = new Map<string, number>();
  const degrees = new Map<string, number>();
  let maximum = 1;
  for (const node of graph.value?.nodes || []) {
    if (node.type) nodeTypeCounts.set(node.type, (nodeTypeCounts.get(node.type) || 0) + 1);
  }
  for (const edge of graph.value?.edges || []) {
    if (edge.relation_type) relationTypeCounts.set(edge.relation_type, (relationTypeCounts.get(edge.relation_type) || 0) + 1);
    const weight = Number(edge.weight) || 1;
    maximum = Math.max(maximum, weight);
    degrees.set(edge.source, (degrees.get(edge.source) || 0) + weight);
    degrees.set(edge.target, (degrees.get(edge.target) || 0) + weight);
  }
  return { nodeTypeCounts, relationTypeCounts, degrees, maximum };
});
const maximumWeight = computed(() => graphIndex.value.maximum);
const nodeTypeOptions = computed(() => [...graphIndex.value.nodeTypeCounts].map(([value, count]) => ({ value, title: nodeTypeLabel(value), count })));
const relationTypeOptions = computed(() => [...graphIndex.value.relationTypeCounts].map(([value, count]) => ({ value, title: relationLabel(value), count })));
const searchOptions = computed(() => {
  return (displayNodes.value || []).map((n) => ({
    title: n.label + (n.metadata?.qq ? ` · ${n.metadata.qq}` : ""),
    value: n.key,
  }));
});
const selectedNodeTypeCount = computed(() => nodeTypes.value.length), selectedRelationTypeCount = computed(() => relationTypes.value.length);

// Keep one derived index for sorting, labels, filters, and table rows. Large
// snapshots used to rescan every edge several times per reactive update.
const nodeDegrees = computed(() => graphIndex.value.degrees);

// Keep nearby nodes before high-degree remote nodes. This preserves the
// target-centred reading order when a saved graph is larger than the viewport.
const displayNodes = computed(() => {
  const nodes = graph.value?.nodes || [];
  if (!nodes.length) return [];
  const scopedNodes = nodes.filter((node) => {
    const distance = Number(node.metadata?.distance);
    return !Number.isFinite(distance) || distance <= workspace.maxDistance;
  });
  const limit = workspace.maxNodes;
  if (scopedNodes.length <= limit) return scopedNodes;
  const targetKey = scopedNodes.find((n) => String(n.metadata?.qq || "") === target.value)?.key;
  const sorted = [...scopedNodes].sort((a, b) => {
    const distanceA = Number(a.metadata?.distance ?? Number.MAX_SAFE_INTEGER);
    const distanceB = Number(b.metadata?.distance ?? Number.MAX_SAFE_INTEGER);
    if (distanceA !== distanceB) return distanceA - distanceB;
    return (nodeDegrees.value.get(b.key) || 0) - (nodeDegrees.value.get(a.key) || 0);
  });
  const top = sorted.slice(0, limit);
  if (targetKey && !top.find((n) => n.key === targetKey)) {
    const targetNode = scopedNodes.find((n) => n.key === targetKey);
    if (targetNode) top.splice(Math.max(0, top.length - 1), 1, targetNode);
  }
  return top;
});

// Only edges between displayNodes
const displayEdges = computed(() => {
  const nodeKeys = new Set(displayNodes.value.map((n) => n.key));
  return (graph.value?.edges || []).filter((e) => nodeKeys.has(e.source) && nodeKeys.has(e.target));
});

const totalNodeCount = computed(() => graph.value?.nodes.length || 0);
const isPartialDisplay = computed(() => displayNodes.value.length < totalNodeCount.value);
const availableNodeCount = computed(() => graphLimits.value?.available_nodes || storedNodeCount.value || totalNodeCount.value || 0);
const availableEdgeCount = computed(() => graphLimits.value?.available_edges || storedEdgeCount.value || graph.value?.edges.length || 0);
const availableDistance = computed(() => graphLimits.value?.available_max_distance || 0);

const currentDistanceTotal = computed(() => {
  const levels = graphLimits.value?.distance_levels || [];
  if (levels.length) {
    return levels
      .filter((l) => l.distance <= workspace.maxDistance)
      .reduce((sum, l) => sum + l.nodes, 0);
  }
  const nodes = graph.value?.nodes || [];
  if (nodes.length) {
    return nodes.filter((n) => {
      const d = Number(n.metadata?.distance);
      return !Number.isFinite(d) || d <= workspace.maxDistance;
    }).length;
  }
  return availableNodeCount.value || 0;
});

const distanceOptions = computed(() => {
  const levels = graphLimits.value?.distance_levels || [];
  if (levels.length) {
    return levels
      .filter((level) => level.distance > 0)
      .map((level) => {
        const totalUpTo = levels
          .filter((l) => l.distance <= level.distance)
          .reduce((sum, l) => sum + l.nodes, 0);
        return {
          value: level.distance,
          title: `${level.distance} 跳 · ${formatNumber(totalUpTo)} 节点`,
        };
      });
  }
  const maxD = availableDistance.value || 1;
  const res = [];
  for (let i = 1; i <= maxD; i++) {
    const count = (graph.value?.nodes || []).filter((n) => Number(n.metadata?.distance ?? 99) <= i).length;
    res.push({ value: i, title: `${i} 跳 · ${formatNumber(count || availableNodeCount.value)} 节点` });
  }
  return res;
});

const renderLimitMinimum = computed(() => systemCapabilities.data?.graph.view.min_nodes || 1);

const renderNodeOptions = computed(() => {
  const total = currentDistanceTotal.value;
  if (!total) return [];
  const minimum = renderLimitMinimum.value;
  if (total <= minimum) {
    return [{ value: total, title: `全部范围 · ${formatNumber(total)} 节点` }];
  }

  const values = new Set<number>();
  values.add(total);

  if (total <= 100) {
    if (total > 30) values.add(30);
    if (total > 60) values.add(60);
  } else if (total <= 500) {
    values.add(50);
    values.add(100);
    if (total > 200) values.add(200);
    if (total > 350) values.add(350);
  } else if (total <= 2000) {
    values.add(100);
    values.add(300);
    values.add(500);
    values.add(1000);
  } else {
    values.add(300);
    values.add(1000);
    values.add(Math.ceil(total * 0.25));
    values.add(Math.ceil(total * 0.5));
    values.add(Math.ceil(total * 0.75));
  }

  if (workspace.maxNodes >= minimum && workspace.maxNodes <= total) {
    values.add(workspace.maxNodes);
  }

  return [...values]
    .filter((v) => v >= minimum && v <= total)
    .sort((a, b) => a - b)
    .map((value) => ({
      value,
      title: value === total ? `全部范围 · ${formatNumber(value)} 节点` : `${formatNumber(value)} 节点`,
    }));
});

const effectiveMaxNodes = computed(() => {
  return Math.min(workspace.maxNodes, currentDistanceTotal.value || workspace.maxNodes);
});

const activeFilterCount = computed(() => {
  const nodeFilters = Math.max(0, nodeTypeOptions.value.length - nodeTypes.value.filter((value) => nodeTypeOptions.value.some((option) => option.value === value)).length);
  const relationFilters = Math.max(0, relationTypeOptions.value.length - relationTypes.value.filter((value) => relationTypeOptions.value.some((option) => option.value === value)).length);
  return nodeFilters + relationFilters + (minimumWeight.value > 1 ? 1 : 0) + (targetNeighborsOnly.value ? 1 : 0);
});
const personQQ = computed(() => personDetail.value?.identifiers?.find((i: any) => i.platform === "qq")?.value || String(selection.value?.data?.metadata?.qq || ""));
const profileFields = computed(() => {
  const profiles = personDetail.value?.profiles || [];
  const strangerInfo = profiles.find((p: any) => p.source === "get_stranger_info");
  const loginInfo = profiles.find((p: any) => p.source === "get_login_info");
  const friendInfo = profiles.find((p: any) => p.source === "get_friend_list" && Object.keys(p.fields || {}).length > 0);
  const profileData = (profile: any) => profile ? {
    ...(profile.fields || {}),
    ...(profile.structured || {}),
    nickname: profile.nickname || profile.fields?.nickname,
    remark: profile.card_or_remark || profile.fields?.remark,
  } : {};
  const raw = { ...profileData(friendInfo), ...profileData(loginInfo), ...profileData(strangerInfo) };
  const labels: Record<string, string> = {
    user_id: "QQ 号", uin: "QQ 号", uid: "UID", qid: "QID",
    nickname: "昵称", nick: "昵称", remark: "好友备注",
    age: "年龄", sex: "性别",
    birthday_year: "出生年", birthday_month: "出生月", birthday_day: "出生日",
    constellation: "星座", shengXiao: "生肖",
    homeTown: "家乡", country: "国家", province: "省份", city: "城市", address: "地址",
    college: "学校", school: "学校", company: "公司", profession: "职业",
    customStatus: "自定义状态", long_nick: "个性签名", sig: "个性签名", signature: "个性签名",
    reg_time: "注册时间", regTime: "注册时间",
    status: "在线状态", is_vip: "VIP", is_years_vip: "年费 VIP", vip_level: "VIP 等级", qqLevel: "QQ 等级", qq_level: "QQ 等级",
    labels: "标签", makeFriendCareer: "交友意向",
    email: "邮箱", phone_num: "公开电话", login_days: "登录天数",
  };
  const allowedKeys = new Set(Object.keys(labels));
  const zeroAllowedKeys = new Set(["qqLevel", "qq_level"]);
  // Deduplicate: if nick and nickname both exist, keep only nickname; same for regTime/reg_time, uin/user_id
  const dedupSkip = new Set<string>();
  if (raw["nickname"] && raw["nick"]) dedupSkip.add("nick");
  if (raw["reg_time"] && raw["regTime"]) dedupSkip.add("regTime");
  if (raw["user_id"] && raw["uin"]) dedupSkip.add("uin");
  if (raw["qqLevel"] !== undefined && raw["qq_level"] !== undefined) dedupSkip.add("qq_level");
  return Object.entries(raw)
    .filter(([k, v]) => {
      if (!allowedKeys.has(k) || dedupSkip.has(k)) return false;
      if (v === null || v === undefined || v === "") return false;
      if (Array.isArray(v) && v.length === 0) return false;
      if (!zeroAllowedKeys.has(k) && (v === 0 || v === "0" || v === "-" || v === "未设置" || v === "未知")) return false;
      return true;
    })
    .slice(0, 30)
    .map(([key, value]) => ({ key, label: labels[key] || key, value: formatProfileValue(key, value) }));
});
const neighbors = computed(() => {
  const instance = activeCy();
  if (!instance || selection.value?.kind !== "node" || !selection.value?.data?.id) return [];
  const node = instance.getElementById(selection.value.data.id);
  return node.neighborhood("node").map((n: any) => {
    const data = n.data();
    const connectingEdges = n.edgesWith(node);
    return {
      id: n.id(),
      label: data.label,
      type: data.type,
      identifier: nodeIdentifier(data),
      avatarUri: avatarSource(data),
      avatarFallbacks: avatarFallbacks(data),
      hasAvatar: isAvatarNode(data),
      typeLabel: nodeKindLabel(data),
      relations: [...new Set(connectingEdges.map((edge: any) => relationLabel(edge.data("relationType"))))],
      weight: connectingEdges.reduce((sum: number, e: any) => sum + Number(e.data("weight") || 1), 0),
    };
  }).sort((a: any, b: any) => b.weight - a.weight || String(a.label).localeCompare(String(b.label), "zh-CN")).slice(0, 100);
});
const nodeHeaders = [{ title: "节点", key: "label" }, { title: "类型", key: "type", width: 110 }, { title: "直接关系", key: "degree", width: 110 }];
const filteredNodeRows = computed(() => {
  const degrees = new Map<string, number>();
  for (const edge of displayEdges.value) {
    if (!relationTypes.value.includes(edge.relation_type) || edge.weight < minimumWeight.value) continue;
    degrees.set(edge.source, (degrees.get(edge.source) || 0) + 1);
    degrees.set(edge.target, (degrees.get(edge.target) || 0) + 1);
  }
  return displayNodes.value.filter((node) => nodeTypes.value.includes(node.type) && (degrees.get(node.key) || 0) > 0).map((node) => ({ ...node, degree: degrees.get(node.key) || 0 }));
});

function persist() { workspace.persist(); }
function workspacePayload() {
  const { buildRequest: _buildRequest, graphStats: _graphStats, graphBusy: _graphBusy, ...state } = workspace.$state;
  return state;
}
async function loadNetworkView(id: string, limit?: number) {
  const result = (await api.get(`/api/v1/ego-networks/${id}/view`, {
    params: { limit, max_distance: workspace.collapseBeyond ? workspace.maxDistance : undefined },
  })).data.data;
  graphLimits.value = result.limits as GraphLimits | null;
  const graph = { nodes: result.nodes || [], edges: result.edges || [] } as Graph;
  return {
    graph,
    partial: Boolean(result.partial),
    partialData: Boolean(result.partial_data ?? result.partial),
    totalNodes: Number(result.total_nodes) || graph.nodes.length,
    totalEdges: Number(result.total_edges) || graph.edges.length,
    scopeNodes: Number(result.scope_nodes) || graph.nodes.length,
    scopeEdges: Number(result.scope_edges) || graph.edges.length,
    limits: result.limits as GraphLimits | undefined,
  } as GraphView;
}

function applyNodeLimitDraft() {
  const minimum = renderLimitMinimum.value;
  const configuredMaximum = systemCapabilities.data?.graph.view.max_nodes;
  const requested = Math.round(Number(nodeLimitDraft.value));
  const value = configuredMaximum && configuredMaximum > 0
    ? Math.min(configuredMaximum, Math.max(minimum, requested))
    : Math.max(minimum, requested);
  nodeLimitDraft.value = value;
  selectNodeLimit(value);
}

function selectNodeLimit(rawValue: number) {
  const value = Math.max(renderLimitMinimum.value, Math.round(Number(rawValue) || renderLimitMinimum.value));
  const maxAllowed = currentDistanceTotal.value || availableNodeCount.value;
  if (maxAllowed > 0 && value > maxAllowed && value > workspace.maxNodes) {
    pendingNodeLimit.value = value;
    largeRenderDialog.value = true;
    return;
  }
  workspace.maxNodes = value;
  nodeLimitDraft.value = value;
  persist();
}

function cancelLargeRender() {
  pendingNodeLimit.value = null;
  nodeLimitDraft.value = workspace.maxNodes;
  largeRenderDialog.value = false;
}

function confirmLargeRender() {
  if (pendingNodeLimit.value) workspace.maxNodes = pendingNodeLimit.value;
  nodeLimitDraft.value = workspace.maxNodes;
  pendingNodeLimit.value = null;
  largeRenderDialog.value = false;
  persist();
}
async function expandNode(nodeKey: string) {
  if (!workspace.lastNetworkId || expandingNode.value) return;
  expandingNode.value = nodeKey;
  try {
    const payload = (await api.get(`/api/v1/ego-networks/${workspace.lastNetworkId}/expand`, { params: { node_key: nodeKey, limit: systemCapabilities.data?.graph.view.expand_default } })).data.data;
    const incomingNodes = (payload.nodes || []) as GraphNode[];
    const incomingEdges = (payload.edges || []) as GraphEdge[];
    const knownNodes = new Set(graph.value?.nodes.map((node) => node.key) || []);
    const knownEdges = new Set(graph.value?.edges.map(edgeKey) || []);
    const addedNodes = incomingNodes.filter((node) => !knownNodes.has(node.key));
    const addedEdges = incomingEdges.filter((edge) => !knownEdges.has(edgeKey(edge)));
    if (!graph.value) graph.value = { nodes: [], edges: [] };
    graph.value.nodes.push(...addedNodes);
    graph.value.edges.push(...addedEdges);
    const instance = activeCy();
    if (instance && addedNodes.length) {
      const parent = instance.getElementById(nodeKey);
      const center = parent.length ? parent.position() : { x: instance.width() / 2, y: instance.height() / 2 };
      instance.batch(() => {
        addedNodes.forEach((node, index) => {
          const angle = (Math.PI * 2 * index) / Math.max(1, addedNodes.length);
          instance.add({ group: "nodes", data: nodeElementData(node, payload.target_key), position: { x: center.x + Math.cos(angle) * 120, y: center.y + Math.sin(angle) * 120 }, classes: avatarImages.has(node.key) ? "has-avatar" : "" });
        });
        for (const edge of addedEdges) {
          if (instance.getElementById(edge.source).length && instance.getElementById(edge.target).length) instance.add({ group: "edges", data: edgeElementData(edge) });
        }
      });
      applyFilters(false);
      void hydrateAvatars(addedNodes).then(() => {
        for (const node of addedNodes) {
          const image = avatarImages.get(node.key), element = activeCy()?.getElementById(node.key);
          if (image && element?.length) { element.data("avatarImage", image); element.addClass("has-avatar"); }
        }
        activeCy()?.style(graphStyles());
      });
    }
    if (!workspace.expandedNodeKeys.includes(nodeKey)) workspace.expandedNodeKeys.push(nodeKey);
    persist();
  } catch (e: any) {
    error.value = e.response?.data?.error || "节点展开失败";
  } finally {
    expandingNode.value = "";
  }
}
function toggleCompare(nodeKey: string) {
  const index = workspace.compareNodeKeys.indexOf(nodeKey);
  if (index >= 0) workspace.compareNodeKeys.splice(index, 1);
  else if (workspace.compareNodeKeys.length < 4) workspace.compareNodeKeys.push(nodeKey);
  persist();
}
function openNodeRow(_: unknown, row: any) {
  workspace.viewMode = "graph";
  nextTick(() => focusNode(row.item.key));
}
async function loadCommunities() {
  const networkID = workspace.lastNetworkId;
  if (!networkID) {
    communities.value = [];
    return;
  }
  communityLoading.value = true;
  communityError.value = "";
  try {
    const result = (await api.get(`/api/v1/ego-networks/${networkID}/communities`, { params: { max_distance: workspace.collapseBeyond ? workspace.maxDistance : undefined } })).data.data;
    communities.value = (result.communities || []) as Community[];
  } catch (e: any) {
    communityError.value = e.response?.data?.error || "社区计算失败";
    communities.value = [];
  } finally {
    communityLoading.value = false;
  }
}
async function openCommunity(communityID: string) {
  if (!workspace.lastNetworkId || expandingCommunity.value) return;
  expandingCommunity.value = communityID;
  communityError.value = "";
  try {
    const result = (await api.get(`/api/v1/ego-networks/${workspace.lastNetworkId}/communities/${communityID}`, { params: { limit: workspace.maxNodes } })).data.data;
    const nextGraph = { nodes: (result.nodes || []) as GraphNode[], edges: (result.edges || []) as GraphEdge[] };
    if (!nextGraph.nodes.length) throw new Error("社区没有可展示节点");
    graph.value = nextGraph;
    graphPartial.value = Boolean(result.partial);
    storedNodeCount.value = Number(result.total_nodes) || nextGraph.nodes.length;
    storedEdgeCount.value = Number(result.total_edges) || nextGraph.edges.length;
    scopeNodeCount.value = storedNodeCount.value;
    scopeEdgeCount.value = storedEdgeCount.value;
    activeCommunityId.value = communityID;
    syncGraphStats({ graph: nextGraph, partial: Boolean(result.partial), partialData: Boolean(result.partial), totalNodes: storedNodeCount.value, totalEdges: storedEdgeCount.value, scopeNodes: scopeNodeCount.value, scopeEdges: scopeEdgeCount.value }, nextGraph);
    workspace.viewMode = "graph";
    nodeTypes.value = [...new Set(nextGraph.nodes.map((node) => node.type))];
    relationTypes.value = [...new Set(nextGraph.edges.map((edge) => edge.relation_type))];
    await nextTick();
    await render({ fitToVisible: true });
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message || "社区展开失败";
  } finally {
    expandingCommunity.value = "";
  }
}
async function returnToFullGraph() {
  activeCommunityId.value = "";
  clearSelection();
  await restoreLastGraph();
}
function openCommunityMember(key: string) {
  workspace.viewMode = "graph";
  nextTick(() => focusNode(key));
}
async function loadSnapshots() {
  try { snapshots.value = (await api.get("/api/v1/research-workspaces/snapshots")).data.data || []; } catch { snapshots.value = []; }
  snapshotManagerOpen.value = true;
}
async function createSnapshot() {
  if (!snapshotName.value) return;
  snapshotSaving.value = true;
  try {
    await api.post("/api/v1/research-workspaces/snapshots", { name: snapshotName.value, state: workspacePayload() });
    snapshotName.value = ""; snapshotDialog.value = false; await loadSnapshots();
  } catch (e: any) { error.value = e.response?.data?.error || "快照保存失败"; }
  finally { snapshotSaving.value = false; }
}
async function deleteSnapshot(id: string) {
  try { await api.delete(`/api/v1/research-workspaces/snapshots/${id}`); await loadSnapshots(); }
  catch (e: any) { error.value = e.response?.data?.error || "快照删除失败"; }
}
async function restoreSnapshot(item: any) {
  snapshotManagerOpen.value = false;
  workspace.$patch(item.state || {});
  workspace.persist();
  clearSelection();
  graph.value = null;
  await restoreLastGraph();
}
function queueDraftSave() {
  window.clearTimeout(draftTimer);
  draftTimer = window.setTimeout(saveDraftNow, 800);
}
function saveDraftNow() {
  window.clearTimeout(draftTimer);
  const token = localStorage.getItem("sra_token");
  void fetch("/api/v1/research-workspaces/draft", {
    method: "PUT",
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: JSON.stringify({ state: workspacePayload() }),
    keepalive: true,
  }).catch(() => undefined);
}
async function restoreServerDraft() {
  try {
    const draft = (await api.get("/api/v1/research-workspaces/draft")).data.data;
    const saved = draft?.state;
    let localUpdatedAt = 0;
    try { localUpdatedAt = Number(JSON.parse(localStorage.getItem("sra.graph-workspace.v1") || "{}").updatedAt) || 0; } catch { /* local storage may be malformed */ }
    const serverUpdatedAt = Date.parse(draft?.updated_at || "") || 0;
    if (localUpdatedAt > serverUpdatedAt) return;
    if (saved && typeof saved === "object" && Object.keys(saved).length > 0) {
      workspace.$patch(saved);
      const repair: Record<string, number> = {};
      const normalize = (key: string, minimum: number, maximum: number, fallback: number) => {
        if (!(key in saved)) return;
        const value = Number(saved[key]);
        if (!Number.isFinite(value) || value < minimum || value > maximum) repair[key] = fallback;
        else if (value !== saved[key]) repair[key] = value;
      };
      const capabilities = systemCapabilities.data;
      if (capabilities) {
        normalize("maxDistance", capabilities.graph.view.min_distance, capabilities.graph.view.max_distance ?? Number.MAX_SAFE_INTEGER, workspace.maxDistance);
        normalize("maxNodes", capabilities.graph.view.min_nodes, capabilities.graph.view.max_nodes ?? Number.MAX_SAFE_INTEGER, workspace.maxNodes);
        normalize("depth", capabilities.graph.depth.min, capabilities.graph.depth.max ?? Number.MAX_SAFE_INTEGER, workspace.depth);
      }
      if (Object.keys(repair).length) {
        workspace.$patch(repair);
        saveDraftNow();
      }
      workspace.persist();
    }
  } catch { /* Local state remains the offline fallback. */ }
}
async function build() {
  if (!target.value) return;
  loading.value = true; error.value = "";
  const previous = graph.value;
  const previousNetworkId = workspace.lastNetworkId;
  let response: any;
  try {
    response = (await api.post("/api/v1/ego-networks", { target_qq: target.value, depth: depth.value })).data;
  } catch (e: any) {
    error.value = e.response?.status === 401 ? "登录状态已失效，请重新登录" : (e.response?.data?.error || "关系图生成失败");
    graph.value = previous;
    loading.value = false;
    return;
  }
  try {
    const result = response;
    const nextNetworkId = result.network_id || workspace.lastNetworkId;
    const isNewNetwork = Boolean(result.network_id && result.network_id !== previousNetworkId);
    workspace.lastNetworkId = nextNetworkId;
    const view = result.network_id ? await loadNetworkView(result.network_id, workspace.maxNodes) : null;
    graph.value = view?.graph || result.data;
    graphPartial.value = Boolean(view?.partialData ?? view?.partial);
    storedNodeCount.value = view?.totalNodes || graph.value?.nodes.length || 0;
    storedEdgeCount.value = view?.totalEdges || graph.value?.edges.length || 0;
    scopeNodeCount.value = view?.scopeNodes || graph.value?.nodes.length || 0;
    scopeEdgeCount.value = view?.scopeEdges || graph.value?.edges.length || 0;
    syncGraphStats(view, graph.value);
    activeCommunityId.value = "";
    if (isNewNetwork) {
      workspace.viewportNetworkId = result.network_id;
      workspace.zoom = 1;
      workspace.pan = { x: 0, y: 0 };
      workspace.selectedNodeKey = null;
      workspace.selectedEdgeKey = null;
      workspace.isolatedNodeKey = null;
      workspace.isolatedNetworkId = null;
      selection.value = null;
      personDetail.value = null;
      evidenceSummaries.value = [];
    }
    if (!graph.value?.nodes || !graph.value?.edges) {
      error.value = "关系图数据为空";
      graph.value = previous;
      return;
    }
    const types = [...new Set(graph.value.nodes.map((n) => n.type))]; const relations = [...new Set(graph.value.edges.map((e) => e.relation_type))];
    nodeTypes.value = nodeTypes.value.filter((v) => types.includes(v)); if (!nodeTypes.value.length) nodeTypes.value = types;
    relationTypes.value = relationTypes.value.filter((v) => relations.includes(v)); if (!relationTypes.value.length) relationTypes.value = relations;
    if (minimumWeight.value > maximumWeight.value) workspace.minimumWeight = 1;
    await nextTick(); await render({ fitToVisible: true }); persist();
  } catch (e: any) {
    console.error("ego network render failed", e);
    error.value = `关系图渲染失败：${e instanceof Error ? e.message : "页面组件异常"}`;
    if (!graph.value) graph.value = previous;
  } finally { loading.value = false; }
}
async function render(options: { fitToVisible?: boolean } = {}) {
  if (workspace.viewMode !== "graph" || !container.value || !graph.value || !Array.isArray(graph.value.nodes)) return;
  const fitToVisible = options.fitToVisible === true;
  const canRestoreViewport = !fitToVisible && workspace.viewportNetworkId === workspace.lastNetworkId;
  const instance = activeCy();
  const previousZoom = instance?.zoom() ?? workspace.zoom; const previousPan = instance?.pan() ?? workspace.pan;
  const targetKey = graph.value.nodes.find((n) => String(n.metadata?.qq || "") === target.value)?.key;
  try { await renderGraph(previousZoom, previousPan, canRestoreViewport, targetKey); } catch (e) { console.error("renderGraph error", e); }
}
async function renderGraph(previousZoom: number, previousPan: { x: number; y: number }, canRestoreViewport: boolean, targetKey?: string) {
  if (workspace.viewMode !== "graph" || !container.value || !graph.value || !Array.isArray(graph.value.nodes) || !Array.isArray(graph.value.edges)) return;
  const generation = ++renderGeneration;
  progressiveRenderToken += 1;
  progressiveRendering.value = false;
  cancelLayout();
  try { activeCy()?.destroy(); } catch { /* instance may be partially destroyed */ }
  const nodesToRender = displayNodes.value;
  const edgesToRender = displayEdges.value;
  if (progressiveRenderFrame !== undefined) window.cancelAnimationFrame(progressiveRenderFrame);
  const progressive = nodesToRender.length > 1200;
  const targetNode = targetKey ? nodesToRender.find((node) => node.key === targetKey) : undefined;
  const nearbyNodes = nodesToRender.filter((node) => Number(node.metadata?.distance ?? 99) <= 1 && node.key !== targetKey);
  const remainingNodes = nodesToRender.filter((node) => node.key !== targetKey && !nearbyNodes.includes(node));
  const orderedNodes = progressive ? [targetNode, ...nearbyNodes, ...remainingNodes].filter(Boolean) as GraphNode[] : nodesToRender;
  const initialNodes = progressive ? orderedNodes.slice(0, 500) : orderedNodes;
  const initialNodeKeys = new Set(initialNodes.map((node) => node.key));
  const initialEdges = progressive ? edgesToRender.filter((edge) => initialNodeKeys.has(edge.source) && initialNodeKeys.has(edge.target)) : edgesToRender;
  const layoutPositions = progressive ? layeredPositions() : layeredPositions();
  const initialLayout = layeredLayoutConfig(layoutPositions);
  const instance = cytoscape({
    container: container.value,
    minZoom: 0.05,
    maxZoom: 5,
    wheelSensitivity: 0.18,
    pixelRatio: 'auto',
    textureOnViewport: false,
    hideEdgesOnViewport: false,
    hideLabelsOnViewport: true,
    userZoomingEnabled: wheelZoom.value,
    boxSelectionEnabled: false,
    elements: [...initialNodes.map((n) => ({ data: nodeElementData(n, targetKey), classes: avatarImages.has(n.key) ? "has-avatar" : "" })), ...initialEdges.map((e) => ({ data: edgeElementData(e) }))],
    style: graphStyles(),
    layout: initialLayout as any,
  });
  // Resize after creation to ensure canvas fills container
  instance.ready(() => {
    instance.resize();
    instance.nodes().forEach((n: any) => {
      const cached = avatarImages.get(n.id());
      if (cached) n.style("background-image", cached);
    });
  });
  cy = instance;
  labelsSuppressed = false;
  instance.on("zoom", () => { if (!instance.destroyed()) { scheduleLabelDensity(instance); scheduleViewportPersist(); } });
  instance.on("pan", () => { if (!instance.destroyed()) { scheduleViewportPersist(); } });
  instance.on("tap", "node", (e: EventObject) => selectNode(e.target as NodeSingular)); instance.on("dbltap", "node", (e: EventObject) => void expandNode(String(e.target.id()))); instance.on("tap", "edge", (e: EventObject) => selectEdge(e.target)); instance.on("tap", (e: EventObject) => { if (e.target === instance) clearSelection(); });
  applyFilters(false);
  if (canRestoreViewport && instance.elements(":visible").length) { instance.zoom(previousZoom); instance.pan(previousPan); } else fitInitial(targetKey);
  syncLabelDensity(instance, true);
  // Double-check sizing after layout settles
  setTimeout(() => { if (!instance.destroyed()) { instance.resize(); } }, 200);
  if (workspace.selectedNodeKey) {
    const n = instance.getElementById(workspace.selectedNodeKey);
    if (n.length) selectNode(n, false);
  } else if (workspace.selectedEdgeKey) {
    const e = instance.getElementById(workspace.selectedEdgeKey);
    if (e.length) selectEdge(e, false);
  }
  updateCounts();
  // Avatar hydration is intentionally viewport-first. The full asset set is
  // retained by the media layer, but preloading thousands of images blocks the
  // graph thread and provides no value before a node is inspected.
  const avatarPriority = displayNodes.value;
  void hydrateAvatars(avatarPriority).then(() => {
    if (generation !== renderGeneration || instance.destroyed()) return;
    instance.nodes().forEach((node: any) => {
      const avatar = avatarImages.get(node.id());
      if (avatar) {
        node.data("avatarImage", avatar);
        node.addClass("has-avatar");
        node.style("background-image", avatar);
      }
    });
  });
  if (layoutName.value === "cose" && nodesToRender.length > 1) void runLayout(false);
  if (progressive) queueProgressiveGraphLoad(instance, orderedNodes, edgesToRender, initialNodeKeys, targetKey, generation, layoutPositions);
}
function queueProgressiveGraphLoad(instance: Core, nodes: GraphNode[], edges: GraphEdge[], loadedKeys: Set<string>, targetKey: string | undefined, generation: number, layoutPositions?: Record<string, { x: number; y: number }>) {
  const token = progressiveRenderToken;
  const positions = layoutPositions || layeredPositions();
  progressiveRendering.value = true;
  progressiveRenderedCount.value = loadedKeys.size;
  progressiveTotalCount.value = nodes.length;
  const edgeBuckets = new Map<string, GraphEdge[]>();
  for (const edge of edges) {
    const source = edgeBuckets.get(edge.source) || [];
    source.push(edge);
    edgeBuckets.set(edge.source, source);
    const target = edgeBuckets.get(edge.target) || [];
    target.push(edge);
    edgeBuckets.set(edge.target, target);
  }
  const edgeQueue = new Map<string, GraphEdge>();
  let cursor = loadedKeys.size;
  const append = () => {
    if (token !== progressiveRenderToken || generation !== renderGeneration || instance.destroyed()) return;
    const batch = nodes.slice(cursor, cursor + 300);
    if (!batch.length) {
      progressiveRenderFrame = undefined;
      progressiveRendering.value = false;
      progressiveRenderedCount.value = nodes.length;
      applyFilters(false);
      updateCounts();
      scheduleLabelDensity(instance);
      return;
    }
    cursor += batch.length;
    for (const node of batch) {
      loadedKeys.add(node.key);
      for (const edge of edgeBuckets.get(node.key) || []) edgeQueue.set(edgeKey(edge), edge);
    }
    const edgesToAdd = [...edgeQueue.values()].filter((edge) => loadedKeys.has(edge.source) && loadedKeys.has(edge.target));
    for (const edge of edgesToAdd) edgeQueue.delete(edgeKey(edge));
    instance.batch(() => {
      for (const node of batch) {
        instance.add({ group: "nodes", data: nodeElementData(node, targetKey), position: positions[node.key] || { x: 0, y: 0 }, classes: avatarImages.has(node.key) ? "has-avatar" : "" });
      }
      for (const edge of edgesToAdd) {
        if (!instance.getElementById(edgeKey(edge)).length) instance.add({ group: "edges", data: edgeElementData(edge) });
      }
    });
    progressiveRenderedCount.value = loadedKeys.size;
    if (cursor >= nodes.length || cursor % 1200 === 0) {
      applyFilters(false);
      updateCounts();
    }
    scheduleLabelDensity(instance);
    progressiveRenderFrame = window.requestAnimationFrame(append);
  };
  progressiveRenderFrame = window.requestAnimationFrame(append);
}
function cancelProgressiveRender() {
  progressiveRenderToken += 1;
  if (progressiveRenderFrame !== undefined) window.cancelAnimationFrame(progressiveRenderFrame);
  progressiveRenderFrame = undefined;
  progressiveRendering.value = false;
  applyFilters(false);
  updateCounts();
}
async function hydrateAvatars(nodes: GraphNode[]) {
  const pending = nodes.filter((node) => isAvatarNode(node));
  const batchSize = 20;
  for (let i = 0; i < pending.length; i += batchSize) {
    const batch = pending.slice(i, i + batchSize);
    await Promise.all(batch.map(loadNodeAvatar));
    const inst = activeCy();
    if (!inst || inst.destroyed()) continue;
    inst.batch(() => {
      for (const node of batch) {
        const img = avatarImages.get(node.key);
        if (img) {
          const el = inst.getElementById(node.key);
          if (el?.length) {
            el.data("avatarImage", img);
            el.addClass("has-avatar");
            el.style({
              "background-image": img,
              "background-fit": "cover",
              "background-clip": "node",
            });
          }
        }
      }
    });
  }
}
async function loadNodeAvatar(node: GraphNode) {
  if (avatarImages.has(node.key)) return avatarImages.get(node.key) || "";
  const existing = avatarLoads.get(node.key);
  if (existing) return existing;
  const request = (async () => {
    const primary = avatarSource(node);
    const fallbacks = avatarFallbacks(node);
    const candidates = [...new Set([
      primary,
      ...fallbacks.filter((uri) => uri.startsWith("/")),
      ...fallbacks.filter((uri) => !uri.startsWith("/")),
    ].filter(Boolean))];
    for (const uri of candidates) {
      try {
        const resolved = /^https?:\/\//i.test(uri) ? (await preloadImage(uri), uri) : await cachedMediaURL(uri);
        if (resolved) {
          avatarImages.set(node.key, resolved);
          avatarFailures.delete(node.key);
          return resolved;
        }
      } catch {
        // Continue through local archive and stable QQ CDN fallbacks.
      }
    }
    avatarFailures.set(node.key, Date.now());
    return "";
  })().finally(() => avatarLoads.delete(node.key));
  avatarLoads.set(node.key, request);
  return request;
}
function preloadImage(source: string) {
  return new Promise<void>((resolve, reject) => {
    const image = new Image();
    const timer = window.setTimeout(() => { image.src = ""; reject(new Error("avatar timeout")); }, 5000);
    image.onload = () => { window.clearTimeout(timer); resolve(); };
    image.onerror = () => { window.clearTimeout(timer); reject(new Error("avatar unavailable")); };
    image.referrerPolicy = "no-referrer";
    image.src = source;
  });
}
function nodeElementData(node: GraphNode, targetKey?: string) { return { id: node.key, label: cleanCQText(node.label), type: node.type, metadata: node.metadata, distance: Number(node.metadata?.distance ?? 10), weightedDegree: nodeDegrees.value.get(node.key) || 0, isTarget: node.key === targetKey, isGroupConversation: isGroupConversation(node), avatarUri: avatarSource(node), avatarImage: avatarImages.get(node.key) || "" }; }
function edgeKey(edge: GraphEdge) { return `edge:${edge.source}:${edge.target}:${edge.relation_type}`; }
function edgeElementData(edge: GraphEdge) { return { id: edgeKey(edge), source: edge.source, target: edge.target, relationType: edge.relation_type, weight: edge.weight, evidenceIds: edge.evidence_ids, firstSeen: edge.first_seen, lastSeen: edge.last_seen }; }
function graphStyles(): any[] {
  const denseGraph = displayNodes.value.length > 180;
  const showLabel = showLabels.value;
  const nodeSize = denseGraph ? 24 : 30;
  const baseStyles: any[] = [
    {
      selector: "node",
      style: {
        width: nodeSize,
        height: nodeSize,
        "background-color": "#8ab4f8",
        "background-image": "none",
        "background-fit": "cover",
        "background-clip": "node",
        "border-width": 1.5,
        "border-color": "#cadcf7",
        label: showLabel ? "data(label)" : "",
        "text-opacity": showLabel ? 1 : 0,
        color: "#e8eaed",
        "font-family": "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif",
        "font-size": 11,
        "font-weight": 500,
        "min-zoomed-font-size": 6,
        "text-wrap": "ellipsis",
        "text-max-width": 120,
        "text-valign": "bottom",
        "text-margin-y": 6,
        "text-outline-color": "#171c22",
        "text-outline-width": 2,
        "text-outline-opacity": 0.95,
        "overlay-opacity": 0,
      }
    },
  ];
  baseStyles.push({ selector: "node.has-avatar", style: { "background-image": "data(avatarImage)" } });
  baseStyles.push(
    { selector: 'node[type="group"]', style: { "background-color": "#67c2a4", shape: "round-rectangle", width: denseGraph ? 28 : 34, height: denseGraph ? 28 : 34 } },
    { selector: 'node[type="conversation"]', style: { "background-color": "#e5b963", shape: "diamond", "background-image": "none" } },
    { selector: 'node[isGroupConversation]', style: { "background-color": "#67c2a4", shape: "round-rectangle", width: denseGraph ? 28 : 34, height: denseGraph ? 28 : 34 } },
    { selector: 'node[isGroupConversation].has-avatar', style: { "background-image": "data(avatarImage)" } },
    { selector: 'node[type="content"]', style: { "background-color": "#d79bd3", shape: "round-tag", width: denseGraph ? 29 : 34, height: denseGraph ? 23 : 28, "background-image": "none" } },
    { selector: "node[isTarget]", style: { width: denseGraph ? 40 : 48, height: denseGraph ? 40 : 48, "border-width": 4, "border-color": "#fff", "font-size": 13, "font-weight": 700 } },
    { selector: "edge", style: denseGraph
      ? { width: "mapData(weight,1,100,0.8,3)", "line-color": "#596571", "target-arrow-color": "#596571", "target-arrow-shape": "none", "curve-style": "straight", opacity: 0.25, "overlay-opacity": 0 }
      : { width: "mapData(weight,1,100,1.2,6)", "line-color": "#596571", "target-arrow-color": "#596571", "target-arrow-shape": "triangle", "arrow-scale": 0.65, "curve-style": "bezier", opacity: 0.65, "overlay-opacity": 0 } },
    { selector: "edge:selected", style: { "line-color": "#f3c778", "target-arrow-color": "#f3c778", "target-arrow-shape": "triangle", "arrow-scale": 0.75, opacity: 1, width: 3 } },
    { selector: "node:selected", style: { "border-color": "#f3c778", "border-width": 4, opacity: 1 } },
    { selector: "node.route-highlighted", style: { "border-color": "#ff9800", "border-width": 4.5, opacity: 1, "z-index": 99, "text-opacity": 1 } },
    { selector: "edge.route-highlighted", style: { "line-color": "#ff9800", "target-arrow-color": "#ff9800", "target-arrow-shape": "triangle", "arrow-scale": 0.85, opacity: 1, width: 4.5, "z-index": 99 } },
    { selector: "node.step-highlighted", style: { "border-color": "#00e5ff", "border-width": 6, opacity: 1, "z-index": 101, "text-opacity": 1 } },
    { selector: "edge.step-highlighted", style: { "line-color": "#00e5ff", "target-arrow-color": "#00e5ff", "target-arrow-shape": "triangle", "arrow-scale": 1, opacity: 1, width: 6, "z-index": 101 } },
    { selector: ".faded", style: { opacity: 0.05, "text-opacity": 0 } },
    { selector: "node.labels-suppressed", style: { "text-opacity": 0 } },
    { selector: "node.labels-suppressed[isTarget], node.labels-suppressed:selected, node.labels-suppressed.matched, node.labels-suppressed.route-highlighted, node.labels-suppressed.step-highlighted", style: { "text-opacity": 1 } },
    { selector: ".matched", style: { "border-color": "#f3c778", "border-width": 5 } },
  );
  return baseStyles;
}
function syncLabelDensity(instance: Core, force = false) {
  if (instance.destroyed()) return;
  const suppress = showLabels.value && instance.nodes().length > 180 && instance.zoom() < 0.72;
  if (!force && suppress === labelsSuppressed) return;
  labelsSuppressed = suppress;
  instance.batch(() => instance.nodes().toggleClass("labels-suppressed", suppress));
}
function scheduleLabelDensity(instance: Core) {
  if (labelDensityFrame !== undefined) return;
  labelDensityFrame = window.requestAnimationFrame(() => {
    labelDensityFrame = undefined;
    if (!instance.destroyed()) syncLabelDensity(instance);
  });
}
function scheduleViewportPersist() {
  window.clearTimeout(viewportPersistTimer);
  viewportPersistTimer = window.setTimeout(() => {
    viewportPersistTimer = undefined;
    const instance = activeCy();
    if (instance && !instance.destroyed()) {
      zoom.value = instance.zoom() || 1;
      workspace.setViewport(instance.zoom(), instance.pan());
    } else {
      persist();
    }
  }, 200);
}
function scheduleWorkspacePersist() {
  window.clearTimeout(workspacePersistTimer);
  workspacePersistTimer = window.setTimeout(() => {
    workspacePersistTimer = undefined;
    persist();
  }, 180);
}
function scheduleFilterApply() {
  if (filterFrame !== undefined) return;
  filterFrame = window.requestAnimationFrame(() => {
    filterFrame = undefined;
    applyFilters();
  });
}
function layeredPositions() {
  const nodes = displayNodes.value;
  const positions: Record<string, { x: number; y: number }> = {};
  if (!nodes.length) return positions;
  const knownDistances = nodes.map((node) => Number(node.metadata?.distance)).filter((value) => Number.isFinite(value) && value >= 0);
  const unknownDistance = Math.max(0, ...knownDistances) + 1;
  const rings = new Map<number, GraphNode[]>();
  for (const node of nodes) {
    const raw = Number(node.metadata?.distance);
    const distance = Number.isFinite(raw) && raw >= 0 ? raw : unknownDistance;
    const ring = rings.get(distance) || [];
    ring.push(node);
    rings.set(distance, ring);
  }
  const typeOrder: Record<string, number> = { person: 0, group: 1, conversation: 2, content: 3, message: 4 };
  let previousRadius = 0;
  for (const [distance, ring] of [...rings.entries()].sort((left, right) => left[0] - right[0])) {
    ring.sort((left, right) => (typeOrder[left.type] ?? 9) - (typeOrder[right.type] ?? 9)
      || (nodeDegrees.value.get(right.key) || 0) - (nodeDegrees.value.get(left.key) || 0)
      || String(left.label).localeCompare(String(right.label), "zh-CN"));
    if (distance === 0) {
      ring.forEach((node, index) => { positions[node.key] = index === 0 ? { x: 0, y: 0 } : { x: index * 54, y: 0 }; });
      continue;
    }
    let cursor = 0;
    let radius = Math.max(210 + (distance - 1) * 190, previousRadius + 155);
    while (cursor < ring.length) {
      const capacity = Math.max(12, Math.floor((Math.PI * 2 * radius) / 78));
      const slice = ring.slice(cursor, cursor + capacity);
      const start = -Math.PI / 2 + ((distance + cursor) % 2 ? 0 : Math.PI / Math.max(2, slice.length));
      slice.forEach((node, index) => {
        const angle = start + (Math.PI * 2 * index) / Math.max(1, slice.length);
        positions[node.key] = { x: Math.cos(angle) * radius, y: Math.sin(angle) * radius };
      });
      cursor += slice.length;
      previousRadius = radius;
      radius += 115;
    }
  }
  return positions;
}
function layeredLayoutConfig(positions = layeredPositions()) {
  return { name: "preset", padding: 64, fit: true, animate: false, positions: (node: any) => positions[node.id()] || { x: 0, y: 0 } };
}
function layoutConfig() {
  const nodeCount = displayNodes.value.length || 0;
  if (layoutName.value === "concentric") return layeredLayoutConfig();
  if (layoutName.value === "grid") return { name: "grid", padding: 46, avoidOverlap: true, condense: false, fit: true };
  const idealLen = nodeCount > 100 ? 80 : 65;
  return {
    name: "fcose",
    quality: "default",
    animate: false,
    padding: 50,
    nodeRepulsion: () => 4000,
    idealEdgeLength: () => idealLen,
    edgeElasticity: 0.45,
    gravity: 0.25,
    gravityRangeCompound: 1.5,
    numIter: nodeCount > 100 ? 100 : 150,
    samplingType: true,
    sampleSize: 25,
    tile: true,
    tilingPaddingVertical: 30,
    tilingPaddingHorizontal: 30,
    packComponents: true,
    nodeSeparation: 35,
    randomize: false,
  } as any;
}
function cancelLayout() {
  layoutToken += 1;
  layoutWorker?.terminate();
  layoutWorker = undefined;
  try { currentLayout?.stop?.(); } catch { /* layout can already be stopped */ }
  currentLayout = undefined;
  layoutRunning.value = false;
}
async function runLayout(fitAfter = true) {
  const instance = activeCy();
  if (!instance) return;
  cancelLayout();
  const token = layoutToken;
  layoutRunning.value = true;
  try {
    if (layoutName.value === "cose" && instance.nodes().length > 15 && instance.nodes().length <= 1500) {
      const worker = new GraphLayoutWorker();
      layoutWorker = worker;
      const positions = await new Promise<Record<string, { x: number; y: number }>>((resolve, reject) => {
        worker.onmessage = (event: MessageEvent<{ token: number; positions: Record<string, { x: number; y: number }> }>) => {
          if (event.data.token === token) resolve(event.data.positions);
        };
        worker.onerror = () => reject(new Error("layout worker failed"));
        worker.postMessage({
          token,
          nodes: instance.nodes().map((node: any) => ({ id: node.id(), degree: node.degree(false) })),
          edges: instance.edges().map((edge: any) => ({ source: edge.source().id(), target: edge.target().id(), weight: Number(edge.data("weight")) || 1 })),
        });
      });
      if (token !== layoutToken || instance.destroyed()) return;
      instance.batch(() => instance.nodes().positions((node: any) => positions[node.id()] || node.position()));
      worker.terminate();
      layoutWorker = undefined;
    } else if (layoutName.value === "cose" && instance.nodes().length > 1500) {
      const positions = layeredPositions();
      await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()));
      if (token !== layoutToken || instance.destroyed()) return;
      instance.batch(() => instance.nodes().positions((node: any) => positions[node.id()] || node.position()));
    } else {
      await new Promise<void>((resolve) => window.requestAnimationFrame(() => resolve()));
      if (token !== layoutToken || instance.destroyed()) return;
      currentLayout = instance.layout(layoutConfig() as any);
      currentLayout.run();
      currentLayout = undefined;
    }
    if (fitAfter) window.requestAnimationFrame(() => {
      const selected = workspace.selectedNodeKey ? instance.getElementById(workspace.selectedNodeKey) : instance.collection();
      const targetNode = instance.nodes("[isTarget]");
      const focus = selected.length && selected.visible() ? selected : targetNode;
      instance.fit(instance.elements(":visible"), 52);
      const minimumReadableZoom = selected.length ? 0.46 : 0.34;
      if (instance.nodes(":visible").length > 400 && instance.zoom() < minimumReadableZoom && focus.length) {
        instance.center(focus);
        instance.zoom({ level: minimumReadableZoom, position: focus.first().position() });
      }
      workspace.setViewport(instance.zoom(), instance.pan());
    });
  } catch { /* layout may fail on empty graph */ }
  finally { if (token === layoutToken) layoutRunning.value = false; }
  persist();
}
function applyFilters(clearHiddenSelection = true) {
  const instance = activeCy();
  if (!instance) return;
  const selectedId = (selection.value?.kind === "node" && selection.value?.data?.id) ? selection.value.data.id : (selection.value?.kind === "edge" && selection.value?.data?.id) ? selection.value.data.id : null;
  const targetNode: any = instance.nodes("[isTarget]");
  const allowedIds = new Set<string>(instance.nodes().map((node: any) => node.id()));
  const restrictToNeighbors = (node: any) => {
    allowedIds.clear();
    if (!node?.length) return;
    allowedIds.add(node.id());
    node.connectedEdges().forEach((edge: any) => {
      allowedIds.add(edge.source().id());
      allowedIds.add(edge.target().id());
    });
  };
  if (workspace.isolatedNodeKey) restrictToNeighbors(instance.getElementById(workspace.isolatedNodeKey));
  else if (targetNeighborsOnly.value) restrictToNeighbors(targetNode);
  const visibleNodeIds = new Set<string>();
  const connectedNodeIds = new Set<string>();
  instance.batch(() => {
    instance.elements().forEach((element: any) => element.show());
    instance.nodes().forEach((n: any) => {
      if (nodeTypes.value.includes(String(n.data("type"))) && allowedIds.has(n.id())) visibleNodeIds.add(n.id());
      else n.hide();
    });
    instance.edges().forEach((e: any) => {
      const valid = relationTypes.value.includes(String(e.data("relationType")))
        && Number(e.data("weight")) >= minimumWeight.value
        && visibleNodeIds.has(e.source().id())
        && visibleNodeIds.has(e.target().id());
      if (valid) {
        connectedNodeIds.add(e.source().id());
        connectedNodeIds.add(e.target().id());
      } else e.hide();
    });
    instance.nodes().forEach((n: any) => {
      if (visibleNodeIds.has(n.id()) && !connectedNodeIds.has(n.id()) && !n.data("isTarget")) n.hide();
    });
  });
  updateCounts();
  if (clearHiddenSelection && selectedId) { const selected: any = instance.getElementById(selectedId); if (!selected.length || !selected.visible()) clearSelection(); }
  scheduleWorkspacePersist();
}
function resetFilters() { nodeTypes.value = [...new Set(graph.value?.nodes.map((n) => n.type) || [])]; relationTypes.value = [...new Set(graph.value?.edges.map((e) => e.relation_type) || [])]; workspace.minimumWeight = 1; targetNeighborsOnly.value = false; workspace.isolatedNodeKey = null; workspace.isolatedNetworkId = null; persist(); }
function focusNode(target: any) {
  const instance = activeCy();
  if (!instance || !target) return;
  const id = typeof target === "object" && target !== null ? target.value || target.id : String(target);
  if (!id) return;
  instance.nodes().removeClass("matched");
  const n = instance.getElementById(id);
  if (n.length && n.visible && n.visible()) {
    n.addClass("matched");
    instance.animate({ center: { eles: n }, zoom: Math.min(2.5, Math.max(1.2, instance.zoom())) }, { duration: 250 });
    selectNode(n);
  }
}
function updateCounts() {
  const instance = activeCy();
  visibleNodeCount.value = instance?.nodes(":visible").length || 0;
  visibleEdgeCount.value = instance?.edges(":visible").length || 0;
  const stats = ensureGraphStats();
  stats.visibleNodes = visibleNodeCount.value;
  stats.visibleEdges = visibleEdgeCount.value;
  stats.partialDisplay = isPartialDisplay.value;
  stats.hasGraph = Boolean(graph.value);
}
function ensureGraphStats() {
  if (!workspace.graphStats) {
    workspace.graphStats = {
      hasGraph: false, visibleNodes: 0, visibleEdges: 0, loadedNodes: 0, loadedEdges: 0,
      scopeNodes: 0, scopeEdges: 0, totalNodes: 0, totalEdges: 0, partialDisplay: false, partialData: false,
    };
  }
  return workspace.graphStats;
}
function syncGraphStats(view: GraphView | null, current: Graph | null) {
  Object.assign(ensureGraphStats(), {
    hasGraph: Boolean(current),
    visibleNodes: current?.nodes.length || 0,
    visibleEdges: current?.edges.length || 0,
    loadedNodes: current?.nodes.length || 0,
    loadedEdges: current?.edges.length || 0,
    scopeNodes: view?.scopeNodes || current?.nodes.length || 0,
    scopeEdges: view?.scopeEdges || current?.edges.length || 0,
    totalNodes: view?.totalNodes || current?.nodes.length || 0,
    totalEdges: view?.totalEdges || current?.edges.length || 0,
    partialData: Boolean(view?.partialData ?? view?.partial),
  });
}
function zoomBy(f: number) { const instance = activeCy(); if (instance) instance.zoom({ level: Math.max(instance.minZoom(), Math.min(instance.maxZoom(), instance.zoom() * f)), renderedPosition: { x: instance.width() / 2, y: instance.height() / 2 } }); }
function fit() { const instance = activeCy(); if (instance?.elements(":visible").length) { instance.fit(instance.elements(":visible"), 52); workspace.setViewport(instance.zoom(), instance.pan()); } }
function fitInitial(targetKey?: string) {
  const instance = activeCy();
  if (!instance?.elements(":visible").length) return;
  instance.fit(instance.elements(":visible"), 52);
  const targetNode = targetKey ? instance.getElementById(targetKey) : instance.nodes("[isTarget]");
  if (instance.nodes(":visible").length > 400 && instance.zoom() < 0.34 && targetNode?.length) {
    instance.center(targetNode);
    instance.zoom({ level: 0.34, position: targetNode.first().position() });
  }
  workspace.setViewport(instance.zoom(), instance.pan());
}
function centerTarget() { const instance = activeCy(); const n = instance?.nodes("[isTarget]"); if (instance && n?.length) instance.animate({ center: { eles: n }, zoom: Math.max(instance.minZoom(), Math.min(instance.maxZoom(), 1.08)) }, { duration: 220 }); }
function selectNode(n: NodeSingular, fadeNeighborhood = true) { const instance = activeCy(); if (!instance || instance.destroyed()) return; const sameNode = workspace.selectedNodeKey === n.id(); instance.elements().removeClass("faded"); if (fadeNeighborhood) n.closedNeighborhood().complement().addClass("faded"); n.select(); const data = n.data(); selection.value = { kind: "node", data, degree: n.connectedEdges().length }; workspace.inspectorOpen = true; workspace.selectedNodeKey = n.id(); workspace.selectedEdgeKey = null; personDetail.value = null; groupDetail.value = null; contentDetail.value = null; messageDetail.value = null; personError.value = ""; entityError.value = ""; personLoading.value = false; entityLoading.value = false; if (!sameNode) inspectorTab.value = "profile"; persist(); const request = ++inspectorRequest; if (data.type === "person") loadPerson(String(data.id).replace(/^person:/, ""), request); else if (isGroupLike(data)) loadGroup(groupEntityID(data), request); else if (data.type === "content") loadContent(String(data.id).replace(/^content:/, ""), request); else if (data.type === "message") loadMessage(String(data.id).replace(/^message:/, ""), request); }
function selectNodeById(id: string) { const instance = activeCy(); const n = instance?.getElementById(id); if (n?.length) selectNode(n); }
async function loadPerson(id: string, request = ++inspectorRequest) { if (request !== inspectorRequest) return; personLoading.value = true; relationshipData.value = null; relationshipDeepData.value = null; timelineData.value = []; try { const result = (await api.get(`/api/v1/persons/${id}`)).data.data; if (request === inspectorRequest) personDetail.value = result; if (target.value) loadRelationship(id, target.value, request); loadTimeline(id, request); } catch (e: any) { if (request === inspectorRequest) personError.value = e.response?.data?.error || "资料加载失败"; } finally { if (request === inspectorRequest) personLoading.value = false; } }
async function loadRelationship(personId: string, targetQQ: string, request = inspectorRequest) {
  if (request !== inspectorRequest) return;
  relationshipLoading.value = true;
  relationshipDeepLoading.value = true;
  try {
    const [basicRes, deepRes] = await Promise.allSettled([
      api.get(`/api/v1/persons/${personId}/relationship`, { params: { target_qq: targetQQ } }),
      api.get(`/api/v1/persons/${personId}/relationship-deep`, { params: { target_qq: targetQQ } }),
    ]);
    if (request === inspectorRequest) {
      if (basicRes.status === "fulfilled") {
        relationshipData.value = basicRes.value.data?.data || null;
      } else {
        relationshipData.value = null;
      }
      if (deepRes.status === "fulfilled") {
        relationshipDeepData.value = deepRes.value.data?.data || null;
      } else {
        relationshipDeepData.value = null;
      }
    }
  } catch {
    if (request === inspectorRequest) {
      relationshipData.value = null;
      relationshipDeepData.value = null;
    }
  } finally {
    if (request === inspectorRequest) {
      relationshipLoading.value = false;
      relationshipDeepLoading.value = false;
    }
  }
}
async function loadTimeline(personId: string, request = inspectorRequest) { if (request !== inspectorRequest) return; timelineLoading.value = true; try { const result = (await api.get(`/api/v1/persons/${personId}/timeline`, { params: { limit: systemCapabilities.data?.lists.default_page_size } })).data; if (request === inspectorRequest) { timelineData.value = result.data || []; timelineTotal.value = result.total || 0; } } catch { if (request === inspectorRequest) { timelineData.value = []; timelineTotal.value = 0; } } finally { if (request === inspectorRequest) timelineLoading.value = false; } }
async function loadGroup(id: string, request = ++inspectorRequest) { if (request !== inspectorRequest) return; entityLoading.value = true; try { const result = (await api.get(`/api/v1/groups/${id}`)).data.data; if (request === inspectorRequest) groupDetail.value = result; } catch (e: any) { if (request === inspectorRequest) entityError.value = e.response?.data?.error || "群资料加载失败"; } finally { if (request === inspectorRequest) entityLoading.value = false; } }
async function loadContent(id: string, request = ++inspectorRequest) { if (request !== inspectorRequest) return; entityLoading.value = true; try { const result = (await api.get(`/api/v1/contents/${id}`)).data.data; if (request === inspectorRequest) contentDetail.value = result; } catch (e: any) { if (request === inspectorRequest) entityError.value = e.response?.data?.error || "内容加载失败"; } finally { if (request === inspectorRequest) entityLoading.value = false; } }
async function loadMessage(id: string, request = ++inspectorRequest) { if (request !== inspectorRequest) return; entityLoading.value = true; try { const result = (await api.get(`/api/v1/messages/${id}`)).data.data; if (request === inspectorRequest) messageDetail.value = result; } catch (e: any) { if (request === inspectorRequest) entityError.value = e.response?.data?.error || "消息详情加载失败"; } finally { if (request === inspectorRequest) entityLoading.value = false; } }
function jumpToMessage(id?: string) {
  if (!id) return;
  const cleanId = String(id).replace(/^message:/, '');
  router.push({ path: '/messages', query: { message_id: cleanId } });
}
function jumpToContent(id?: string) {
  if (!id) return;
  const cleanId = String(id).replace(/^content:/, '');
  router.push({ path: '/contents', query: { content_id: cleanId } });
}
function goToPerson(qq?: string) {
  if (!qq) return;
  const cleanQQ = String(qq).replace(/^person:/, '').replace(/^QQ\s*/, '').trim();
  router.push({ path: '/persons', query: { q: cleanQQ } });
}
function goToGroup(groupId?: string) {
  if (!groupId) return;
  const cleanId = String(groupId).replace(/^group:/, '').trim();
  router.push({ path: '/groups', query: { q: cleanId } });
}
function selectEdge(e: any, fadeRelationship = true) {
  ++inspectorRequest;
  const request = ++evidenceRequest;
  const inst = activeCy();
  if (!inst || inst.destroyed()) return;
  inst.elements().removeClass("faded");
  if (fadeRelationship) e.connectedNodes().union(e).complement().addClass("faded");
  e.select();
  const srcData = e.source().data();
  const tgtData = e.target().data();
  selection.value = {
    kind: "edge",
    data: e.data(),
    sourceNode: srcData,
    targetNode: tgtData,
    sourceLabel: `${srcData.label} · ${nodeIdentifier(srcData)}`,
    targetLabel: `${tgtData.label} · ${nodeIdentifier(tgtData)}`
  };
  workspace.inspectorOpen = true;
  workspace.selectedEdgeKey = e.id();
  workspace.selectedNodeKey = null;
  inspectorTab.value = "evidence";
  personDetail.value = null;
  groupDetail.value = null;
  contentDetail.value = null;
  messageDetail.value = null;
  personLoading.value = false;
  entityLoading.value = false;
  personError.value = "";
  entityError.value = "";
  evidenceSummaries.value = [];
  edgeEvidenceDetails.value = [];
  persist();
  loadEvidenceSummaries(e.data("evidenceIds") || [], request);
  loadEdgeDetailedEvidence(e.data("evidenceIds") || [], request);
}

function clearSelection() {
  inspectorRequest += 1;
  evidenceRequest += 1;
  const instance = activeCy();
  instance?.elements().removeClass("faded");
  instance?.elements().unselect();
  selection.value = null;
  personDetail.value = null;
  groupDetail.value = null;
  contentDetail.value = null;
  messageDetail.value = null;
  relationshipDeepData.value = null;
  personLoading.value = false;
  entityLoading.value = false;
  personError.value = "";
  entityError.value = "";
  evidenceSummaries.value = [];
  edgeEvidenceDetails.value = [];
  workspace.selectedNodeKey = null;
  workspace.selectedEdgeKey = null;
  persist();
}

async function loadEdgeDetailedEvidence(ids: string[], request = evidenceRequest) {
  if (!ids || !ids.length) {
    edgeEvidenceDetails.value = [];
    return;
  }
  edgeEvidenceLoading.value = true;
  try {
    const res = await api.post("/api/v1/analysis/evidence-details", {
      evidence_ids: ids,
      limit: 30
    });
    if (request === evidenceRequest) {
      edgeEvidenceDetails.value = res.data.data || [];
    }
  } catch (err) {
    console.error("Failed to load edge evidence details:", err);
    if (request === evidenceRequest) {
      edgeEvidenceDetails.value = [];
    }
  } finally {
    if (request === evidenceRequest) {
      edgeEvidenceLoading.value = false;
    }
  }
}
function toggleIsolation() { if (!selection.value || selection.value.kind !== "node") return; if (workspace.isolatedNodeKey === selection.value.data.id) { workspace.isolatedNodeKey = null; workspace.isolatedNetworkId = null; } else { workspace.isolatedNodeKey = selection.value.data.id; workspace.isolatedNetworkId = workspace.lastNetworkId; } applyFilters(false); centerSelected(); persist(); }
function centerSelected() { const instance = activeCy(); if (instance && selection.value?.kind === "node") instance.animate({ center: { eles: instance.getElementById(selection.value.data.id) }, zoom: 1.15 }, { duration: 220 }); }
async function loadEvidenceSummaries(ids: string[], request = ++evidenceRequest) { evidenceLoading.value = true; try { const results = (await Promise.all(ids.slice(0, 20).map((id) => api.get(`/api/v1/raw-records/${id}`).then((r) => r.data.data).catch(() => null)))).filter(Boolean); if (request === evidenceRequest) evidenceSummaries.value = results; } finally { if (request === evidenceRequest) evidenceLoading.value = false; } }
function openEvidence(item: any) { evidence.value = item; evidenceDialog.value = true; }

async function openDetailedEvidenceForStep(step: any) {
  detailedEvidenceOpen.value = true;
  detailedEvidenceLoading.value = true;
  detailedEvidenceTitle.value = `${cleanDisplayText(step.source_label)} 与 ${cleanDisplayText(step.target_label)} 的互动证据明细`;
  detailedEvidenceSubtitle.value = `链路第 ${step.step_number} 跳 · ${step.medium_summary || '关系记录'}`;
  detailedEvidenceFilter.value = "all";
  detailedEvidenceSearch.value = "";
  detailedEvidenceList.value = [];
  detailedEvidenceCommonGroups.value = [];

  try {
    const res = await api.post("/api/v1/analysis/evidence-details", {
      evidence_ids: step.evidence_ids || [],
      source_qq: step.source_meta?.qq || "",
      target_qq: step.target_meta?.qq || "",
      limit: 100
    });
    detailedEvidenceList.value = res.data.data || [];
    detailedEvidenceCommonGroups.value = res.data.common_groups || [];
    if (detailedEvidenceCommonGroups.value.length && (!detailedEvidenceList.value.length || step.medium_type === 'group' || step.relation_type === 'co_member')) {
      detailedEvidenceFilter.value = "common_group";
    }
  } catch (err: any) {
    console.error("Failed to load evidence details for step:", err);
  } finally {
    detailedEvidenceLoading.value = false;
  }
}

async function openDetailedEvidenceForEdge(edgeData: any) {
  detailedEvidenceOpen.value = true;
  detailedEvidenceLoading.value = true;
  detailedEvidenceTitle.value = `${selection.value?.sourceLabel || '起点'} -> ${selection.value?.targetLabel || '终点'} 关系明细`;
  detailedEvidenceSubtitle.value = `共 ${edgeData.weight || 0} 次互动 · 关系类型: ${relationLabel(edgeData.relationType)}`;
  detailedEvidenceFilter.value = "all";
  detailedEvidenceSearch.value = "";
  detailedEvidenceList.value = [];
  detailedEvidenceCommonGroups.value = [];

  try {
    const srcQQ = selection.value?.sourceNode?.metadata?.qq || "";
    const tgtQQ = selection.value?.targetNode?.metadata?.qq || "";
    const res = await api.post("/api/v1/analysis/evidence-details", {
      evidence_ids: edgeData.evidenceIds || [],
      source_qq: srcQQ,
      target_qq: tgtQQ,
      limit: 100
    });
    detailedEvidenceList.value = res.data.data || [];
    detailedEvidenceCommonGroups.value = res.data.common_groups || [];
  } catch (err: any) {
    console.error("Failed to load edge evidence details:", err);
  } finally {
    detailedEvidenceLoading.value = false;
  }
}

async function openDetailedEvidenceBetween(sourceQQ: string, targetQQ: string, relationType = "") {
  if (!sourceQQ || !targetQQ) return;
  detailedEvidenceOpen.value = true;
  detailedEvidenceLoading.value = true;
  detailedEvidenceTitle.value = `QQ ${sourceQQ} 与 QQ ${targetQQ} 的具体互动记录`;
  detailedEvidenceSubtitle.value = relationType ? `筛选类型: ${relationLabel(relationType)}` : "全部互动流水";
  detailedEvidenceFilter.value = "all";
  detailedEvidenceSearch.value = "";
  detailedEvidenceList.value = [];
  detailedEvidenceCommonGroups.value = [];

  try {
    const res = await api.post("/api/v1/analysis/evidence-details", {
      source_qq: String(sourceQQ),
      target_qq: String(targetQQ),
      relation_type: relationType,
      limit: 100
    });
    detailedEvidenceList.value = res.data.data || [];
    detailedEvidenceCommonGroups.value = res.data.common_groups || [];
  } catch (err: any) {
    console.error("Failed to load evidence details between QQs:", err);
  } finally {
    detailedEvidenceLoading.value = false;
  }
}
function download(name: string, data: BlobPart, type: string) { const url = URL.createObjectURL(new Blob([data], { type })); const a = document.createElement("a"); a.href = url; a.download = name; a.click(); window.setTimeout(() => URL.revokeObjectURL(url), 0); }
function exportPNG() { const instance = activeCy(); if (instance) { const a = document.createElement("a"); a.href = instance.png({ full: true, scale: 2, bg: "#11161b" }); a.download = `关系图-${target.value}.png`; a.click(); } }
function exportJSON() { if (graph.value) download(`关系图-${target.value}.json`, JSON.stringify(graph.value, null, 2), "application/json"); }
function exportMarkdown() {
  if (!graph.value) return;
  const labels = new Map(graph.value.nodes.map((n) => [n.key, n.label]));
  let md = `# 社会关系研判简报 — ${target.value}\n\n`;
  md += `> 导出时间: ${new Date().toLocaleString("zh-CN")}\n`;
  md += `> 研判中心目标: QQ ${target.value}\n`;
  md += `> 拓扑规模: ${graph.value.nodes.length} 节点 / ${graph.value.edges.length} 条关系边\n\n`;
  md += `## 一、 核心关系统计\n\n`;
  md += `| 起点 | 终点 | 关系类型 | 频次 | 首次发现 | 最近发现 |\n`;
  md += `|---|---|---|---|---|---|\n`;
  for (const e of graph.value.edges.slice(0, 100)) {
    md += `| ${labels.get(e.source) || e.source} | ${labels.get(e.target) || e.target} | ${e.relation_type} | ${e.weight} | ${e.first_seen || '—'} | ${e.last_seen || '—'} |\n`;
  }
  if (graph.value.edges.length > 100) {
    md += `\n*(仅展示前 100 条关系，完整拓扑请导出 JSON)*\n`;
  }
  download(`研判简报-${target.value}.md`, md, "text/markdown;charset=utf-8");
}
function exportCSV() { if (!graph.value) return; const labels = new Map(graph.value.nodes.map((n) => [n.key, n.label])); const rows = [["起点", "终点", "关系", "次数", "首次发现", "最后发现"], ...graph.value.edges.map((e) => [labels.get(e.source) || e.source, labels.get(e.target) || e.target, e.relation_type, String(e.weight), e.first_seen, e.last_seen])]; download(`关系图-${target.value}.csv`, rows.map((r) => r.map((v) => `"${String(v).replaceAll('"', '""')}"`).join(",")).join("\n"), "text/csv;charset=utf-8"); }
function metadataLabel(v: string) { return ({ qq: "QQ 号", group_id: "群号", conversation_type: "会话类型", platform_id: "平台标识" } as Record<string, string>)[v] || v; }
function formatProfileValue(key: string, value: unknown) {
  if (["is_vip", "is_years_vip"].includes(key)) return value ? "是" : "否";
  if ((key === "reg_time" || key === "regTime") && Number(value) > 0) return formatDate(new Date(Number(value) * 1000).toISOString());
  if (key === "sex") return ({ male: "男", female: "女", unknown: "未知" } as Record<string, string>)[String(value)] || String(value);
  if (key === "constellation") return ["", "白羊", "金牛", "双子", "巨蟹", "狮子", "处女", "天秤", "天蝎", "射手", "摩羯", "水瓶", "双鱼"][Number(value)] || String(value);
  if (key === "shengXiao") return ["", "鼠", "牛", "虎", "兔", "龙", "蛇", "马", "羊", "猴", "鸡", "狗", "猪"][Number(value)] || String(value);
  if (key === "status") return ({ 10: "在线", 20: "离线", 30: "隐身", 40: "忙碌", 50: "Q我吧", 60: "请勿打扰" } as Record<number, string>)[Number(value)] || String(value);
  if (key === "homeTown" && String(value).match(/\d+-\d+-\d+/)) return "未设置";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}
function goToFeed(id: string) {
  if (!id) return;
  void router.push({ path: "/contents", query: { content_id: id } });
}
function isBotAccount(data: any, detail?: any): boolean {
  if (!data && !detail) return false;
  const qq = String(data?.metadata?.qq || detail?.identifiers?.find((i: any) => i.platform === 'qq')?.value || '').trim();
  const label = String(data?.label || detail?.display_name || '').trim();
  if (['2854196310', '2854196308', '2854196309', '2854196311', '2854196312', '2854212450', '4000000000', '10000', '1000000', '66600000'].includes(qq)) return true;
  if (data?.metadata?.is_bot) return true;
  if (label.includes('Q群管家') || label.includes('群管家') || label.includes('签到助手')) return true;
  return false;
}

function isQZoneAction(type?: string): boolean {
  if (!type) return false;
  return ['liked', 'commented', 'feed_reply', 'published', 'qzone_like', 'qzone_comment', 'qzone_reply', 'qzone_post', 'visited'].includes(type);
}

const nodeTypeLabel = (v: string) => ({ person: "人员", group: "群", conversation: "会话", content: "内容", message: "消息" } as Record<string, string>)[v] || v;
const nodeColor = (v: string) => ({ person: "#8ab4f8", group: "#67c2a4", conversation: "#e5b963", content: "#d79bd3", message: "#a8b3bf" } as Record<string, string>)[v] || "#8ab4f8";
const nodeIcon = (v: string) => ({ person: "mdi-account-outline", group: "mdi-account-group-outline", conversation: "mdi-forum-outline", content: "mdi-image-text", message: "mdi-message-text-outline" } as Record<string, string>)[v] || "mdi-help-circle-outline";
function avatarSource(data: any) {
  const metadata = data?.metadata || data || {};
  const qq = String(metadata.qq || data?.qq || "").trim();
  if (qq) {
    return `/api/v1/media/avatars/person/${encodeURIComponent(qq)}`;
  }
  const groupId = String(metadata.group_id || data?.group_id || "").trim();
  if (groupId) {
    return `/api/v1/media/avatars/group/${encodeURIComponent(groupId)}`;
  }
  let uri = String(data?.avatarUri || data?.avatar_uri || metadata.avatar_uri || "");
  if (uri.startsWith("/api/v1/media/assets/")) return uri;
  if (uri.includes("store.qq.com") || uri.includes("/qzone/")) {
    const match = uri.match(/qzone\/(\d+)/);
    if (match && match[1]) return `/api/v1/media/avatars/person/${match[1]}`;
  }
  return uri;
}
function isGroupConversation(data: any) {
  const metadata = data?.metadata || data || {};
  return String(data?.type || metadata.type || "") === "conversation" && String(metadata.conversation_type || "") === "group";
}
function isGroupLike(data: any) { return String(data?.type || "") === "group" || isGroupConversation(data); }
function isAvatarNode(data: any) { return String(data?.type || "") === "person" || isGroupLike(data); }
function groupEntityID(data: any) {
  const metadata = data?.metadata || {};
  return String(metadata.group_entity_id || data?.id || "").replace(/^group:/, "");
}
function avatarFallbacks(data: any) {
  const metadata = data?.metadata || data || {};
  const type = String(data?.type || metadata.type || "");
  const candidates: string[] = [];
  if (type === "person") {
    const qq = String(metadata.qq || data?.qq || "");
    if (qq) {
      candidates.push(`/api/v1/media/avatars/person/${encodeURIComponent(qq)}`);
      candidates.push(`https://q1.qlogo.cn/g?b=qq&nk=${encodeURIComponent(qq)}&s=100`);
      candidates.push(`https://q.qlogo.cn/headimg_dl?dst_uin=${encodeURIComponent(qq)}&spec=100`);
    }
  } else if (type === "group" || isGroupConversation(data)) {
    const groupID = String(metadata.group_id || data?.group_id || "");
    if (groupID) {
      candidates.push(`/api/v1/media/avatars/group/${encodeURIComponent(groupID)}`);
      candidates.push(`https://p.qlogo.cn/gh/${encodeURIComponent(groupID)}/${encodeURIComponent(groupID)}/100/`);
    }
  }
  const primary = avatarSource(data);
  return [...new Set(candidates.filter((value) => value && value !== primary))];
}
function cleanCQText(raw?: string): string {
  if (!raw && raw !== "0") return "[无文本]";
  return cleanDisplayText(String(raw)) || "[无文本]";
}
const nodeIdentifier = (data: any) => {
  const metadata = data?.metadata || {};
  if (data?.type === "person") return metadata.qq ? `QQ ${metadata.qq}` : "QQ 未记录";
  if (isGroupLike(data)) return metadata.group_id ? `群号 ${metadata.group_id}` : "群号未记录";
  if (data?.type === "conversation") return [metadata.conversation_type, metadata.platform_id].filter(Boolean).join(" · ") || "平台标识未记录";
  if (data?.type === "content") return [metadata.author_qq ? `QQ ${metadata.author_qq}` : "", formatDate(metadata.published_at)].filter(Boolean).join(" · ");
  if (data?.type === "message") return [metadata.sender ? `发送者: ${metadata.sender}` : (metadata.sender_qq ? `QQ ${metadata.sender_qq}` : ''), formatDate(metadata.sent_at)].filter(Boolean).join(" · ") || "消息记录";
  return data?.id ? String(data.id) : "标识未记录";
};
const nodeKindLabel = (data: any) => isGroupConversation(data) ? "群会话" : nodeTypeLabel(String(data?.type || ""));
const selectionTitle = (data: any) => {
  if (data?.type === "message") return cleanCQText(data?.label);
  return data?.label || (data?.type === "group" ? "未命名群" : data?.type === "conversation" ? "未命名会话" : "未命名节点");
};
const relationLabel = (v: string) => ({
  friend_visible: "好友可见",
  member_of: "共同在群",
  co_member: "共同在群",
  group_membership: "群成员变更",
  sent_message: "发送消息",
  message: "发送消息",
  published: "发布说说",
  published_feed: "发布说说",
  mentioned: "@提及",
  at_message: "@提及",
  replied_to: "引用回复",
  quote_reply: "引用回复",
  commented: "说说评论",
  qzone_comment: "说说评论",
  feed_reply: "说说回复",
  liked: "空间点赞",
  qzone_like: "空间点赞",
  visited: "空间访问",
  qzone_visit: "空间访问"
} as Record<string, string>)[v] || v;

const trendLabel = (v: string) => ({ increasing: "上升", decreasing: "下降", stable: "平稳", new: "新建立", none: "无数据" } as Record<string, string>)[v] || v;
const trendColor = (v: string) => ({ increasing: "success", decreasing: "error", stable: "info", new: "warning", none: "grey" } as Record<string, string>)[v] || "grey";

const eventLabel = (v: string) => ({
  sent_message: "发送消息",
  message: "发送消息",
  member_of: "加入群聊",
  group_membership: "群成员变更",
  profile_change: "资料变更",
  liked: "空间点赞",
  qzone_like: "空间点赞",
  commented: "说说评论",
  qzone_comment: "说说评论",
  feed_reply: "说说回复",
  replied_to: "引用回复",
  quote_reply: "引用回复",
  mentioned: "@提及",
  at_message: "@提及",
  published: "发布说说",
  published_feed: "发布说说",
  visited: "空间访问",
  friend_visible: "好友可见"
} as Record<string, string>)[v] || v;

const eventColor = (v: string) => ({
  sent_message: "#8ab4f8",
  message: "#8ab4f8",
  member_of: "#67c2a4",
  group_membership: "#67c2a4",
  profile_change: "#f3c778",
  liked: "#f28b82",
  qzone_like: "#f28b82",
  commented: "#d79bd3",
  qzone_comment: "#d79bd3",
  feed_reply: "#ba68c8",
  replied_to: "#e5b963",
  quote_reply: "#e5b963",
  mentioned: "#9dd5c0",
  at_message: "#9dd5c0",
  published: "#ab47bc",
  published_feed: "#ab47bc",
  visited: "#c8a8e9",
  friend_visible: "#a8b3bf"
} as Record<string, string>)[v] || "#8ab4f8";
const maxMonthCount = computed(() => Math.max(1, ...(relationshipData.value?.time_distribution || []).map((m: any) => m.count)));
const roleLabel = (v: string) => ({ owner: "群主", admin: "管理员", member: "成员" } as Record<string, string>)[v] || v || "成员";
const cleanText = (v?: string) => cleanCQText(v);
const formatNumber = (value: number) => new Intl.NumberFormat("zh-CN").format(Number(value) || 0);
const formatDate = (v?: string) => v ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "short", timeStyle: "short" }).format(new Date(v)) : "—";
function extractContentShare(data: any) {
  if (!data) return null;
  const meta = data.metadata || {};
  let url = meta.musicShare?.playUrl || meta.share_url || meta.url || meta.link || "";
  let title = meta.appShareTitle || meta.musicShare?.songName || meta.title || "";
  let subtitle = meta.musicShare?.artistName || meta.appName || "";

  if (!url && typeof meta.likeUnikey === "string") {
    if (meta.likeUnikey.startsWith("http://") || meta.likeUnikey.startsWith("https://")) {
      url = meta.likeUnikey;
    } else if (meta.likeUnikey.includes("fakeUrl=")) {
      const match = meta.likeUnikey.match(/fakeUrl=([^&]+)/);
      if (match) url = decodeURIComponent(match[1]);
    }
  }

  if (url.includes("fakeUrl=")) {
    const match = url.match(/fakeUrl=([^&]+)/);
    if (match) url = decodeURIComponent(match[1]);
  }

  title = title.replace(/[\t\r\n]+/g, " ").trim();
  subtitle = subtitle.replace(/[\t\r\n]+/g, " ").trim();

  if (!url && !title) return null;
  return { url, title: title || url, subtitle };
}
function openExternalUrl(url?: string) {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
}

function toggleRouteMode() {
  routeMode.value = !routeMode.value;
  if (routeMode.value) {
    if (!routeSourceQQ.value && target.value) {
      routeSourceQQ.value = target.value;
    }
    if (!routeTargetQQ.value && selection.value?.data?.type === "person") {
      const pqq = personQQ.value;
      if (pqq && pqq !== routeSourceQQ.value) {
        routeTargetQQ.value = pqq;
      }
    }
  }
}

function closeRouteMode() {
  routeMode.value = false;
}

function swapRouteQQs() {
  const temp = routeSourceQQ.value;
  routeSourceQQ.value = routeTargetQQ.value;
  routeTargetQQ.value = temp;
}

function setRouteSource(qq: string, openBar = false) {
  routeSourceQQ.value = String(qq).trim();
  if (openBar) routeMode.value = true;
}

function setRouteTarget(qq: string, openBar = false) {
  routeTargetQQ.value = String(qq).trim();
  if (openBar) routeMode.value = true;
}

function quickPlanBetween(src: string, dst: string) {
  routeSourceQQ.value = String(src).trim();
  routeTargetQQ.value = String(dst).trim();
  routeMode.value = true;
  void planRoute();
}

function onSourceSearchFocus() {
  fetchSourceOptions(sourceSearchText.value || "");
}

function onSourceSearchInput(text: string) {
  if (sourceSearchTimer) window.clearTimeout(sourceSearchTimer);
  sourceSearchTimer = window.setTimeout(() => {
    fetchSourceOptions(text);
  }, 200);
}

async function fetchSourceOptions(query: string) {
  const q = String(query || "").trim();
  sourceSearching.value = true;
  try {
    const res = await api.get("/api/v1/persons", { params: { q, limit: 15 } });
    const list = res.data?.data || [];
    sourceOptions.value = list.map((m: any) => {
      const qq = qqOf(m);
      return {
        title: m.display_name || qq || m.id,
        value: qq || m.id,
        subtitle: qq ? `QQ: ${qq}` : "",
        avatar: m.avatar_uri || (qq ? `/api/v1/media/avatars/person/${qq}` : ""),
        fallbacks: qq ? [`https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=640`] : [],
      };
    });
  } catch {
    sourceOptions.value = [];
  } finally {
    sourceSearching.value = false;
  }
}

function onSourceSelect(item: any) {
  if (!item) return;
  if (typeof item === "object") {
    routeSourceQQ.value = item.value || item.title || "";
  } else {
    routeSourceQQ.value = String(item).trim();
  }
}

function onRouteTargetSearchFocus() {
  fetchRouteTargetOptions(routeTargetSearchText.value || "");
}

function onRouteTargetSearchInput(text: string) {
  if (routeTargetSearchTimer) window.clearTimeout(routeTargetSearchTimer);
  routeTargetSearchTimer = window.setTimeout(() => {
    fetchRouteTargetOptions(text);
  }, 200);
}

async function fetchRouteTargetOptions(query: string) {
  const q = String(query || "").trim();
  routeTargetSearching.value = true;
  try {
    const res = await api.get("/api/v1/persons", { params: { q, limit: 15 } });
    const list = res.data?.data || [];
    routeTargetOptions.value = list.map((m: any) => {
      const qq = qqOf(m);
      return {
        title: m.display_name || qq || m.id,
        value: qq || m.id,
        subtitle: qq ? `QQ: ${qq}` : "",
        avatar: m.avatar_uri || (qq ? `/api/v1/media/avatars/person/${qq}` : ""),
        fallbacks: qq ? [`https://q1.qlogo.cn/g?b=qq&nk=${qq}&s=640`] : [],
      };
    });
  } catch {
    routeTargetOptions.value = [];
  } finally {
    routeTargetSearching.value = false;
  }
}

function onRouteTargetSelect(item: any) {
  if (!item) return;
  if (typeof item === "object") {
    routeTargetQQ.value = item.value || item.title || "";
  } else {
    routeTargetQQ.value = String(item).trim();
  }
}

function clearRouteResult() {
  routeResult.value = null;
  activeRouteIndex.value = 0;
  clearRouteHighlight();
}

function clearRouteHighlight() {
  const instance = activeCy();
  if (!instance || instance.destroyed()) return;
  instance.batch(() => {
    instance.elements().removeClass("route-highlighted step-highlighted faded");
  });
}

async function planRoute() {
  const src = String(routeSourceQQ.value || "").trim();
  const dst = String(routeTargetQQ.value || "").trim();
  if (!src || !dst) {
    routeError.value = "请填写起点 QQ 和终点 QQ";
    return;
  }
  routeError.value = "";
  routeLoading.value = true;
  try {
    const res = await api.post("/api/v1/analysis/routes", {
      source_qq: src,
      target_qq: dst,
      profile: routeProfile.value,
      max_hops: Number(routeMaxHops.value) || 4,
      max_paths: 3,
    });
    const result: RouteResult = res.data?.data;
    if (!result || !result.routes?.length) {
      routeError.value = "未找到可达的关联路径，可尝试增加跳数或更换算路策略";
      routeResult.value = null;
      clearRouteHighlight();
      return;
    }
    routeResult.value = result;
    activeRouteIndex.value = 0;
    waybillExpanded.value = true;
    routeMode.value = false;
    applyRouteHighlight(result.routes[0]);
  } catch (err: any) {
    routeError.value = err.response?.data?.error || err.message || "规划算路失败";
    routeResult.value = null;
    clearRouteHighlight();
  } finally {
    routeLoading.value = false;
  }
}

function onRouteSelect(index: number) {
  if (index === undefined || index === null) return;
  activeRouteIndex.value = index;
  const r = routeResult.value?.routes?.[index];
  if (r) {
    applyRouteHighlight(r);
  }
}

function applyRouteHighlight(route: PlannedRoute) {
  const instance = activeCy();
  if (!instance || instance.destroyed()) return;

  instance.batch(() => {
    if (route.nodes?.length) {
      for (const n of route.nodes) {
        if (!instance.getElementById(n.key).length) {
          instance.add({
            group: "nodes",
            data: nodeElementData(n, target.value),
          });
        }
      }
    }
    if (route.edges?.length) {
      for (const e of route.edges) {
        const id = edgeKey(e);
        if (!instance.getElementById(id).length) {
          instance.add({
            group: "edges",
            data: edgeElementData(e),
          });
        }
      }
    }

    const nodeKeySet = new Set<string>();
    const edgeKeySet = new Set<string>();

    for (const step of route.steps) {
      nodeKeySet.add(step.source_key);
      nodeKeySet.add(step.target_key);
      const directEdge = instance.edges(`[source = "${step.source_key}"][target = "${step.target_key}"]`);
      if (directEdge.length) {
        directEdge.forEach((el) => { edgeKeySet.add(el.id()); });
      } else {
        const reverseEdge = instance.edges(`[source = "${step.target_key}"][target = "${step.source_key}"]`);
        reverseEdge.forEach((el) => { edgeKeySet.add(el.id()); });
      }
    }

    instance.elements().removeClass("route-highlighted step-highlighted faded");

    let routeEles = instance.collection();
    for (const nk of nodeKeySet) {
      const n = instance.getElementById(nk);
      if (n.length) routeEles = routeEles.union(n);
    }
    for (const ek of edgeKeySet) {
      const e = instance.getElementById(ek);
      if (e.length) routeEles = routeEles.union(e);
    }

    if (routeEles.length) {
      routeEles.addClass("route-highlighted");
      instance.elements().difference(routeEles).addClass("faded");
    }
  });

  const highlightedEles = instance.elements(".route-highlighted");
  if (highlightedEles.length) {
    instance.animate({
      fit: {
        eles: highlightedEles,
        padding: 60,
      },
      duration: 350,
    });
  }
}

function highlightStep(step: RouteStep, zoomTo = false) {
  hoveredStepNumber.value = step.step_number;
  const instance = activeCy();
  if (!instance || instance.destroyed()) return;

  instance.batch(() => {
    instance.elements().removeClass("step-highlighted");
    const srcNode = instance.getElementById(step.source_key);
    const tgtNode = instance.getElementById(step.target_key);
    const directEdge = instance.edges(`[source = "${step.source_key}"][target = "${step.target_key}"]`);
    const reverseEdge = instance.edges(`[source = "${step.target_key}"][target = "${step.source_key}"]`);

    let stepEles = instance.collection();
    if (srcNode.length) stepEles = stepEles.union(srcNode);
    if (tgtNode.length) stepEles = stepEles.union(tgtNode);
    if (directEdge.length) stepEles = stepEles.union(directEdge);
    if (reverseEdge.length) stepEles = stepEles.union(reverseEdge);

    stepEles.addClass("step-highlighted");
  });

  if (zoomTo) {
    const stepEles = instance.elements(".step-highlighted");
    if (stepEles.length) {
      instance.animate({
        fit: {
          eles: stepEles,
          padding: 80,
        },
        duration: 250,
      });
    }
  }
}

function clearStepHighlight() {
  hoveredStepNumber.value = null;
  const instance = activeCy();
  if (!instance || instance.destroyed()) return;
  instance.elements().removeClass("step-highlighted");
}

function getStepMediumInfo(mediumType: string, relationType: string) {
  switch (mediumType) {
    case "direct_message":
      return { label: "发送消息", color: "#67c2a4", icon: "mdi-chat-processing-outline" };
    case "group_membership":
      return { label: "共同在群", color: "#8ab4f8", icon: "mdi-account-group-outline" };
    case "qzone_like":
      return { label: "空间点赞", color: "#f28b82", icon: "mdi-thumb-up-outline" };
    case "qzone_comment":
      return { label: "说说评论", color: "#d79bd3", icon: "mdi-comment-text-outline" };
    case "qzone_reply":
      return { label: "说说回复", color: "#ba68c8", icon: "mdi-reply-outline" };
    case "qzone_post":
      return { label: "发布说说", color: "#ab47bc", icon: "mdi-image-text" };
    case "quote_reply":
      return { label: "引用回复", color: "#e5b963", icon: "mdi-format-quote-close" };
    case "mention":
      return { label: "群聊@提及", color: "#9dd5c0", icon: "mdi-at" };
    default:
      return { label: relationLabel(relationType || mediumType) || "关联链路", color: "#8ab4f8", icon: "mdi-vector-polyline" };
  }
}

function getCategoryMeta(category?: string) {
  switch (category) {
    case "intimate":
      return { label: "核心亲密羁绊", color: "pink", icon: "mdi-heart-pulse" };
    case "frequent":
      return { label: "高频紧密互动", color: "deep-purple", icon: "mdi-account-multiple-check" };
    case "casual":
      return { label: "常规日常往来", color: "primary", icon: "mdi-account-outline" };
    case "distant":
      return { label: "低频偶发节点", color: "grey", icon: "mdi-account-clock-outline" };
    default:
      return { label: "社交关联", color: "primary", icon: "mdi-account-group" };
  }
}

function getPowerDynamicsLabel(dynamics: string) {
  switch (dynamics) {
    case "heavily_this":
      return "主动方: 极度倾向此人";
    case "slightly_this":
      return "主动方: 此人略多";
    case "balanced":
      return "势均力敌: 双向平衡互流";
    case "slightly_target":
      return "主动方: 目标略多";
    case "heavily_target":
      return "主动方: 极度倾向目标";
    default:
      return "流向关系";
  }
}
watch([nodeTypes, relationTypes, minimumWeight, targetNeighborsOnly], scheduleFilterApply, { deep: true });
watch(() => [workspace.targetQQ, workspace.depth], persist, { deep: true });
watch(() => workspace.buildRequest, (value, previous) => { if (value !== previous) build(); });
watch(loading, (value) => { workspace.graphBusy = value; }, { immediate: true });
watch(showLabels, () => { const instance = activeCy(); instance?.style(graphStyles()); if (instance) syncLabelDensity(instance, true); persist(); }); watch(wheelZoom, () => { activeCy()?.userZoomingEnabled(wheelZoom.value); persist(); }); watch(layoutName, persist);
watch(() => workspace.maxNodes, (value) => { nodeLimitDraft.value = value; });
watch(availableDistance, (value) => {
  if (value > 0 && workspace.maxDistance > value) {
    workspace.maxDistance = value;
    persist();
  }
}, { immediate: true });
watch(() => [workspace.maxDistance, workspace.maxNodes, workspace.collapseBeyond], () => {
  persist();
  window.clearTimeout(graphReloadTimer);
  graphReloadTimer = window.setTimeout(async () => {
    const id = workspace.lastNetworkId;
    if (!id) return;
    loading.value = true;
    try {
      const view = await loadNetworkView(id, workspace.maxNodes);
      graph.value = view.graph;
      graphPartial.value = view.partialData ?? view.partial;
      storedNodeCount.value = view.totalNodes;
      storedEdgeCount.value = view.totalEdges;
      scopeNodeCount.value = view.scopeNodes;
      scopeEdgeCount.value = view.scopeEdges;
      syncGraphStats(view, graph.value);
      await nextTick();
      await render({ fitToVisible: true });
    } catch (e: any) {
      error.value = e.response?.data?.error || "图谱视图加载失败";
    } finally {
      loading.value = false;
    }
  }, 280);
});
watch(() => workspace.viewMode, async (mode) => {
  persist();
  if (mode === "graph") {
    await nextTick();
    await render();
  } else if (mode === "communities") {
    progressiveRenderToken += 1;
    progressiveRendering.value = false;
    cancelLayout();
    const instance = activeCy();
    if (instance) { workspace.setViewport(instance.zoom(), instance.pan()); instance.destroy(); cy = undefined; }
    await loadCommunities();
  } else {
    progressiveRenderToken += 1;
    progressiveRendering.value = false;
    cancelLayout();
    const instance = activeCy();
    if (instance) { workspace.setViewport(instance.zoom(), instance.pan()); instance.destroy(); cy = undefined; }
  }
});
watch(() => workspace.filterPanelOpen, persist); watch(() => workspace.inspectorOpen, persist); watch(inspectorTab, persist);
async function restoreLastGraph() {
  try {
    let id = workspace.lastNetworkId;
    const limit = workspace.maxNodes;
    if (!id) {
      const latest = (await api.get("/api/v1/ego-networks")).data.data?.[0];
      id = latest?.id || null;
      if (!id) return;
      workspace.lastNetworkId = id;
    }
    if (workspace.isolatedNetworkId && workspace.isolatedNetworkId !== id) {
      workspace.isolatedNodeKey = null;
      workspace.isolatedNetworkId = null;
    }
    let meta = (await api.get(`/api/v1/ego-networks/${id}`)).data.data;
    // A previously truncated snapshot must not remain the default once a
    // newer complete snapshot for the same target exists.
    if (meta?.truncated) {
      const candidates = (await api.get("/api/v1/ego-networks")).data.data || [];
      const replacement = candidates.find((item: any) =>
        item.target_qq === meta.target_qq && item.status === "completed" && !item.truncated && Number(item.node_count) > Number(meta.node_count),
      );
      if (replacement) {
        id = replacement.id;
        meta = (await api.get(`/api/v1/ego-networks/${id}`)).data.data;
        workspace.lastNetworkId = id;
        workspace.viewportNetworkId = id;
        persist();
      }
    }
    if (meta?.target_qq) workspace.targetQQ = meta.target_qq;
    if (!id) return;
    const view = await loadNetworkView(id, limit);
    if (!view?.graph?.nodes || !view?.graph?.edges || !Array.isArray(view.graph.nodes) || !Array.isArray(view.graph.edges)) {
      error.value = "已保存的图谱数据为空，请重新生成";
      return;
    }
    graph.value = view.graph;
    graphPartial.value = view.partialData ?? view.partial;
    storedNodeCount.value = view.totalNodes;
    storedEdgeCount.value = view.totalEdges;
    scopeNodeCount.value = view.scopeNodes;
    scopeEdgeCount.value = view.scopeEdges;
    syncGraphStats(view, graph.value);
    const types = [...new Set(graph.value.nodes.map((node) => node.type))];
    const relations = [...new Set(graph.value.edges.map((edge) => edge.relation_type))];
    nodeTypes.value = nodeTypes.value.filter((value) => types.includes(value));
    relationTypes.value = relationTypes.value.filter((value) => relations.includes(value));
    if (!nodeTypes.value.length) nodeTypes.value = types;
    if (!relationTypes.value.length) relationTypes.value = relations;
    const maxW = graph.value.edges.length ? Math.max(1, ...graph.value.edges.map((edge) => edge.weight)) : 1;
    if (minimumWeight.value > maxW) minimumWeight.value = 1;
    await nextTick();
    workspace.lastNetworkId = id;
    activeCommunityId.value = "";
    if (workspace.viewMode === "graph") {
      await render();
      if (workspace.selectedNodeKey) {
        const instance = activeCy();
        const n = instance?.getElementById(workspace.selectedNodeKey);
        if (n?.length && n.visible()) selectNode(n, false);
      }
    } else if (workspace.viewMode === "communities") {
      await loadCommunities();
    }
  } catch (e: any) {
    console.error("restore ego network failed", e);
    if (e.response?.status === 404 && target.value) {
      workspace.lastNetworkId = null;
      persist();
      await build();
    } else {
      error.value = e.response?.status === 401 ? "登录状态已失效，请重新登录" : "已保存的图谱暂时无法恢复，请重新生成";
      persist();
    }
  }
}
function normalizeWorkspaceToCapabilities() {
  const capabilities = systemCapabilities.data;
  if (!capabilities) return;
  const depth = capabilities.graph.depth;
  const view = capabilities.graph.view;
  workspace.depth = Math.max(depth.min, workspace.depth);
  if (depth.max !== null) workspace.depth = Math.min(depth.max, workspace.depth);
  workspace.maxDistance = Math.max(view.min_distance, workspace.maxDistance);
  if (view.max_distance !== null) workspace.maxDistance = Math.min(view.max_distance, workspace.maxDistance);
  const viewMaxNodes = view.max_nodes ?? Number.MAX_SAFE_INTEGER;
  workspace.maxNodes = Math.min(viewMaxNodes, Math.max(view.min_nodes, workspace.maxNodes));
  nodeLimitDraft.value = workspace.maxNodes;
  persist();
}
let resizeTimer: number | undefined;
function handleWindowResize() {
  window.clearTimeout(resizeTimer);
  resizeTimer = window.setTimeout(() => {
    const instance = activeCy();
    if (instance && !instance.destroyed()) {
      instance.resize();
    }
  }, 250);
}
onMounted(async () => {
  await systemCapabilities.load();
  normalizeWorkspaceToCapabilities();
  await restoreServerDraft();
  normalizeWorkspaceToCapabilities();
  stopWorkspaceSubscription = workspace.$subscribe(queueDraftSave, { detached: true });

  const queryTarget = String(route.query.target || "").trim();
  if (queryTarget) {
    target.value = queryTarget;
    await build();
  } else {
    await restoreLastGraph();
  }

  window.addEventListener("resize", handleWindowResize);
});
onBeforeUnmount(() => {
  window.removeEventListener("resize", handleWindowResize);
  window.clearTimeout(resizeTimer);
  window.clearTimeout(viewportPersistTimer);
  window.clearTimeout(workspacePersistTimer);
  if (labelDensityFrame !== undefined) window.cancelAnimationFrame(labelDensityFrame);
  if (filterFrame !== undefined) window.cancelAnimationFrame(filterFrame);
  progressiveRenderToken += 1;
  progressiveRendering.value = false;
  saveDraftNow();
  window.clearTimeout(graphReloadTimer);
  stopWorkspaceSubscription?.();
  workspace.graphBusy = false;
  renderGeneration += 1;
  cancelLayout();
  const instance = activeCy();
  if (instance) { workspace.setViewport(instance.zoom(), instance.pan()); instance.destroy(); }
  cy = undefined;
});
</script>

<style scoped>
.graph-workspace-toolbar {
  height: 56px !important;
  min-height: 56px !important;
  max-height: 56px !important;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 0 12px;
  margin-bottom: 10px;
  border: 1px solid var(--card-border);
  border-radius: 6px;
  background: var(--bg-surface);
}
.graph-filter-scrim { display: none; }
.graph-query-group {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}
.graph-target-combobox {
  width: 220px;
  min-width: 170px;
}
.graph-depth-select {
  width: 98px;
  min-width: 92px;
}
.graph-view-toggle-wrap {
  display: flex;
  align-items: center;
}
.graph-rendering-status {
  margin-left: 10px;
  color: var(--text-sub);
  font-size: 10px;
  white-space: nowrap;
  font-variant-numeric: tabular-nums;
}
.graph-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-left: auto;
  flex: 0 0 auto;
}
.render-menu { width: 330px; margin-top: 6px; padding: 14px; border: 1px solid var(--card-border); border-radius: 6px; background: var(--bg-surface); }
.render-menu-summary { display: grid; grid-template-columns: auto 1fr; gap: 4px 8px; padding: 2px 0 12px; margin-bottom: 12px; border-bottom: 1px solid var(--card-border); }
.render-menu-summary strong { color: var(--text-main); font-size: 12px; }
.render-menu-summary span, .render-menu-summary small { color: var(--text-sub); font-size: 11px; }
.render-menu-summary small { grid-column: 2; }
.render-menu-row { display: grid; grid-template-columns: 82px minmax(0,1fr); align-items: center; gap: 10px; margin-bottom: 12px; font-size: 11px; }
.render-menu-row > span { color: var(--text-sub); }
.render-menu-hint { margin: -4px 0 12px 92px; color: var(--text-dim); font-size: 10px; line-height: 1.4; }
.render-limit-row { grid-template-columns: 82px minmax(0,1fr) auto; }
.render-limit-row strong { min-width: 44px; text-align: right; }
.render-number-input { width: 124px; justify-self: end; }
.render-distance-row .render-number-input { grid-column: 2; }
.render-limit-row .v-slider { grid-column: 1 / -1; margin: -4px 4px 0; }
.render-presets { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 5px; margin: -2px 0 10px; }
.render-presets .v-btn { min-width: 0; padding-inline: 5px; }
.toolbar-badge {
  min-width: 16px;
  height: 16px;
  display: inline-grid;
  place-items: center;
  margin-left: 5px;
  padding: 0 4px;
  border-radius: 8px;
  background: #8ab4f8;
  color: var(--bg-surface-subtle);
  font-size: 9px;
  font-weight: 700;
  line-height: 16px;
}
.graph-error { margin: 0 0 10px; }
.panel-heading { min-height: 52px; display: flex; align-items: center; justify-content: space-between; padding: 10px 13px; border-bottom: 1px solid var(--card-border); }
.panel-heading strong { display: block; font-size: 13px; }
.filter-section { padding: 12px 13px; }
.filter-section-heading { display: flex; justify-content: space-between; align-items: center; margin-bottom: 7px; font-size: 11px; }
.filter-section-heading span { color: rgba(255,255,255,.36); font-size: 10px; }
.filter-chip-group { padding: 0; }
.filter-chip-group .v-chip { margin: 0 5px 5px 0; }
.filter-chip-group em { margin-left: 6px; color: var(--text-dim); font-size: 10px; font-style: normal; }
.filter-color { width: 7px; height: 7px; margin-right: 6px; border-radius: 50%; }
.weight-input-row { display: flex; }
.graph-node-table { min-height: 640px; }
.graph-table-identity { display: grid; grid-template-columns: 32px minmax(0,1fr); align-items: center; gap: 10px; min-width: 0; }
.graph-table-identity > :first-child { width: 32px; height: 32px; border-radius: 50%; overflow: hidden; background: #28323c; display: grid; place-items: center; }
.graph-table-identity img { width: 100%; height: 100%; object-fit: cover; }
.graph-table-identity > span { min-width: 0; }
.graph-table-identity strong,.graph-table-identity small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.graph-table-identity small { color: rgba(255,255,255,.44); font-size: 10px; }
.profile-avatar { width: 42px; height: 42px; flex: 0 0 42px; display: grid; place-items: center; border-radius: 50%; background-size: cover; background-position: center; color: var(--bg-app); font-weight: 700; }
.profile-avatar img, .neighbor-avatar img { width: 100%; height: 100%; object-fit: cover; border-radius: inherit; }
.profile-version-row { display: grid; grid-template-columns: minmax(0, 1fr) auto; align-items: center; gap: 12px; min-height: 54px; padding: 8px 2px; border-bottom: 1px solid rgba(var(--v-theme-on-surface), .08); }
.profile-version-row > span, .profile-version-row strong, .profile-version-row small { display: block; min-width: 0; }
.profile-version-row strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
.profile-version-row small { margin-top: 3px; color: rgba(var(--v-theme-on-surface), .52); font-size: 10px; }
.profile-version-row > span:last-child { text-align: right; }
.graph-console-body.graph-three-column > .redesigned-inspector { display: block !important; }
.neighbor-avatar { display: grid; width: 26px; height: 26px; place-items: center; flex: 0 0 26px; }
.neighbor-avatar > i { display: grid; place-items: center; width: 26px; height: 26px; border-radius: 50%; color: var(--bg-surface-subtle); font-style: normal; }
.neighbor-avatar-fallback { display: grid; width: 100%; height: 100%; place-items: center; border-radius: 50%; background: #28323c; color: #dce4ec; font-size: 10px; font-weight: 700; }
.inspector-top { position: relative; display: grid; grid-template-columns: 42px minmax(0, 1fr) 26px; gap: 9px; align-items: center; padding: 12px 13px 4px; }
.inspector-title { min-width: 0; flex: 1; }
.inspector-title span, .inspector-title small { display: block; color: var(--text-dim); font-size: 10px; }
.inspector-title h2 { margin: 2px 0; overflow: hidden; color: #fff; font-size: 14px; text-overflow: ellipsis; white-space: nowrap; }
.inspector-progress { margin-top: -1px; }
.inspector-window { height: auto !important; min-height: 0; margin: 0; overflow: visible; }
.inspector-window :deep(.v-window__container) { display: block !important; height: auto !important; min-height: 0; }
.inspector-window :deep(.v-window-item) { position: static !important; overflow: visible; transform: none !important; }
.inspector-pane { padding: 12px 13px 14px; }
.inspector-pane-list { padding: 6px 0 12px; }
.inspector-pane .inspector-list { margin: 0; }
.inspector-pane-list .detail-row, .inspector-pane-list .message-row { padding-inline: 13px; }
.detail-row, .message-row, .relation-summary-row { width: 100%; display: flex; align-items: center; gap: 8px; padding: 9px 13px; border: 0; border-bottom: 1px solid rgba(255,255,255,.05); background: transparent; color: rgba(255,255,255,.78); text-align: left; }
.detail-row > span:not(.neighbor-avatar) { min-width: 0; flex: 1; }
.detail-row strong, .detail-row small, .message-row small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.detail-row strong { font-size: 11px; }
.detail-row small, .message-row small { margin-top: 2px; color: rgba(255,255,255,.38); font-size: 9px; }
.neighbor-relations { color: rgba(243,199,120,.72) !important; }
.neighbor-row { display: grid; grid-template-columns: 26px minmax(0, 1fr) auto; align-items: center; width: 100%; gap: 8px; padding: 9px 13px; border: 0; border-bottom: 1px solid rgba(255,255,255,.05); background: transparent; color: rgba(255,255,255,.78); text-align: left; }
.neighbor-row > .neighbor-content { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.neighbor-row > .neighbor-content strong, .neighbor-row > .neighbor-content small { display: block; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.message-row { display: block; font-size: 11px; line-height: 1.45; }
.message-row span { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 3; }
.relation-summary-row { flex-wrap: wrap; font-size: 11px; }
.relation-summary-row strong { margin-left: auto; color: #f3c778; }
.relation-summary-row small { width: 100%; color: rgba(255,255,255,.38); font-size: 9px; }
.inspector-blank { padding: 26px 14px; color: rgba(255,255,255,.34); font-size: 11px; text-align: center; }
.content-inspector { padding-bottom: 10px; }
.content-inspector > p { padding: 14px; margin: 0; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.65; font-size: 13px; }
.content-author { display: flex; align-items: center; gap: 8px; padding: 13px 14px; border-bottom: 1px solid var(--card-border); }
.content-author > span { display: flex; min-width: 0; flex-direction: column; }
.content-author small { color: rgba(255,255,255,.48); font-size: 10px; }
.content-inspector-media { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 3px; padding: 0 14px 12px; }
.content-inspector-media img, .content-inspector-media video { width: 100%; aspect-ratio: 1; object-fit: cover; background: #0b0e11; border-radius: 3px; }
.panel-share-card { display: flex; align-items: flex-start; gap: 8px; margin: 8px 14px; padding: 8px 10px; border: 1px solid rgba(138,180,248,0.25); border-radius: 6px; background: rgba(138,180,248,0.05); cursor: pointer; }
.panel-share-card:hover { border-color: rgba(138,180,248,0.5); background: rgba(138,180,248,0.1); }
.share-card-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.share-card-title { font-size: 12px; font-weight: 600; color: #e8eaed; overflow: hidden; text-overflow: ellipsis; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.share-card-sub { color: rgba(255,255,255,0.55); font-size: 10px; }
.share-card-link { font-size: 10px; color: #8ab4f8; text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-flex; align-items: center; margin-top: 2px; }
.share-card-link:hover { text-decoration: underline; }

/* 画布与悬浮岛 */
.graph-stage { display: flex !important; flex-direction: column; min-width: 0; min-height: 0; overflow: hidden; position: relative; }
.graph-canvas { position: relative; width: 100%; height: 100%; }
.graph-surface { position: absolute; inset: 0; width: 100%; height: 100%; }
.graph-canvas.redesigned { flex: 1 1 0; min-height: 480px; width: 100%; overflow: hidden; }
.graph-loading { position: absolute; inset: 0; display: grid; place-content: center; justify-items: center; gap: 10px; color: rgba(255,255,255,.52); background: rgba(17,22,27,.58); font-size: 11px; z-index: 5; }
.graph-overlay { z-index: 2; }

/* 统一悬浮控制岛的尺寸，避免不同控件在画布上失去比例。 */
.floating-island {
  position: absolute;
  z-index: 8;
  pointer-events: none;
  max-width: calc(100% - 24px);
}
.floating-island > * {
  pointer-events: auto;
}
.floating-island-top-left {
  top: 12px;
  left: 12px;
}
.floating-island-top-right {
  top: 12px;
  right: 12px;
}
.floating-island-bottom-right {
  bottom: 12px;
  right: 12px;
}
.floating-island-top-left .glass-island-card {
  height: 40px !important;
  min-height: 40px !important;
  max-height: 40px !important;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 4px !important;
  border: 1px solid var(--card-border);
  background: rgba(var(--v-theme-surface), 0.92) !important;
  backdrop-filter: blur(16px) saturate(180%);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15) !important;
}
.layout-toggle-strict-zero {
  height: 28px !important;
}
.layout-toggle-strict-zero,
.layout-toggle-strict-zero .v-btn,
.zero-radius-btn {
  border-radius: 0 !important;
  font-size: 11px !important;
  height: 30px !important;
  min-height: 30px !important;
  line-height: 30px !important;
  padding: 0 11px !important;
}
.graph-search-input {
  width: min(260px, calc(100vw - 48px));
  height: 40px !important;
}
.graph-search-input :deep(.v-field) {
  background: rgba(var(--v-theme-surface), 0.92) !important;
  backdrop-filter: blur(16px) saturate(180%);
  border: 1px solid var(--card-border) !important;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.12) !important;
  min-height: 40px !important;
  height: 40px !important;
  max-height: 40px !important;
  border-radius: 4px !important;
  padding-inline: 10px !important;
  box-sizing: border-box;
}
.graph-search-input :deep(.v-field__prepend-inner) {
  padding-inline-end: 8px !important;
  align-items: center;
}
.graph-search-input :deep(.v-field__prepend-inner .v-icon) {
  font-size: 16px;
  opacity: 0.75;
}
.graph-search-input :deep(.v-field__input) {
  padding: 0 !important;
  font-size: 12px;
  min-height: 36px;
  height: 36px;
  align-items: center;
}
.graph-search-input :deep(.v-field__append-inner) {
  align-items: center;
  padding: 0 4px;
}
.floating-island-bottom-right .glass-island-card {
  height: 40px !important;
  min-height: 40px !important;
  max-height: 40px !important;
  box-sizing: border-box;
  display: flex;
  align-items: center;
  padding: 4px 6px !important;
  border: 1px solid var(--card-border);
  background: rgba(var(--v-theme-surface), 0.92) !important;
  backdrop-filter: blur(16px) saturate(180%);
  box-shadow: 0 8px 24px rgba(0, 0, 0, 0.15) !important;
}
.zoom-value-square {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-main);
  min-width: 38px;
  text-align: center;
  user-select: none;
}

/* 社区视图 */
.community-stage-toolbar { display: flex; align-items: center; gap: 10px; min-height: 48px; padding: 8px 12px; border-bottom: 1px solid var(--card-border); }
.community-stage-toolbar > div { min-width: 0; }
.community-stage-toolbar strong { font-size: 12px; }
.community-stage-toolbar small { margin-top: 2px; color: rgba(255,255,255,.4); font-size: 10px; }
.community-view { flex: 1 1 auto; min-height: 0; overflow: auto; padding: 14px; }
.community-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(250px, 1fr)); align-content: start; gap: 10px; }
.community-card { display: flex; min-width: 0; flex-direction: column; gap: 12px; padding: 14px; border: 1px solid var(--card-border); border-radius: 6px; background: var(--bg-surface); }
.community-card-target { border-color: rgba(138,180,248,.55); box-shadow: inset 3px 0 #8ab4f8; }
.community-card-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
.community-card-header h3 { margin: 3px 0 0; font-size: 14px; font-weight: 600; }
.community-rank { color: var(--text-dim); font-size: 10px; }
.community-metrics { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 5px; }
.community-metrics div { min-width: 0; padding: 7px 5px; border-radius: 4px; background: rgba(255,255,255,.035); text-align: center; }
.community-metrics strong, .community-metrics span { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.community-metrics strong { color: #fff; font-size: 14px; font-variant-numeric: tabular-nums; }
.community-metrics span { margin-top: 2px; color: var(--text-dim); font-size: 9px; }
.community-types { display: flex; flex-wrap: wrap; gap: 6px; color: rgba(255,255,255,.52); font-size: 10px; }
.community-members { display: flex; align-items: center; min-height: 30px; }
.community-member { width: 30px; height: 30px; padding: 0; margin-right: -5px; overflow: hidden; border: 2px solid var(--bg-surface); border-radius: 50%; background: #2a333d; color: #dce4ec; }
.community-member img, .community-member .v-img { width: 100%; height: 100%; object-fit: cover; }
.community-member > i { display: grid; width: 100%; height: 100%; place-items: center; font-style: normal; }
.community-more { margin-left: 10px; color: rgba(255,255,255,.45); font-size: 10px; }
.community-card footer { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.community-connection { color: rgba(243,199,120,.8); font-size: 10px; }
.community-connection.muted { color: rgba(255,255,255,.35); }
.community-loading, .community-empty { display: grid; min-height: 260px; place-content: center; justify-items: center; gap: 10px; color: var(--text-dim); font-size: 11px; }
.snapshot-list { display: grid; }
.snapshot-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; min-height: 58px; padding: 10px 20px; border-bottom: 1px solid var(--card-border); }
.snapshot-row:last-child { border-bottom: 0; }
.snapshot-row > div:first-child { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.snapshot-row strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.snapshot-row small { color: rgba(255,255,255,.45); font-size: 10px; }
.snapshot-actions { display: flex; align-items: center; flex: 0 0 auto; gap: 2px; }

/* 响应式断点 */
@media (max-width: 1180px) {
  .graph-workspace-toolbar { flex-wrap: wrap; }
  .graph-query-group { width: 100%; order: 1; }
  .graph-view-toggle-wrap { order: 2; }
  .graph-toolbar-actions { order: 3; }
}
@media (max-width: 920px) {
  .graph-workspace-toolbar { align-items: stretch; flex-direction: column; }
  .graph-query-group, .graph-toolbar-actions { width: 100%; }
  .graph-toolbar-actions { justify-content: flex-end; }
  .floating-island-top-left { top: 10px; left: 10px; }
  .floating-island-top-right { top: 58px; right: 10px; }
  .floating-island-bottom-right { bottom: 10px; right: 10px; }
}
@media (max-width: 760px) {
  .community-view { padding: 10px; }
  .community-grid { grid-template-columns: 1fr; }
}
.graph-console-body.graph-filter-collapsed { grid-template-columns: minmax(0, 1fr) 324px; }
.graph-console-body.graph-inspector-collapsed { grid-template-columns: 228px minmax(0, 1fr); }
.graph-console-body.graph-filter-collapsed.graph-inspector-collapsed { grid-template-columns: minmax(0, 1fr); }
.graph-console-body.graph-inspector-collapsed .redesigned-inspector { display: none; }
@media (max-width: 1180px) {
  .graph-console-body.graph-filter-collapsed { grid-template-columns: minmax(0, 1fr) 290px; }
  .graph-console-body.graph-inspector-collapsed { grid-template-columns: 210px minmax(0, 1fr); }
}
@media (max-width: 760px) {
  .graph-console-body.graph-filter-collapsed,
  .graph-console-body.graph-inspector-collapsed,
  .graph-console-body.graph-filter-collapsed.graph-inspector-collapsed { grid-template-columns: 1fr; }
}
.rel-section { padding: 8px 0; border-bottom: 1px solid rgba(var(--v-theme-on-surface), .07); }
.rel-section:last-child { border-bottom: none; }
.rel-stat-row { display: flex; justify-content: space-between; align-items: center; padding: 4px 0; font-size: 12px; }
.rel-stat-row span { color: rgba(var(--v-theme-on-surface), .6); }
.rel-stat-row strong { font-size: 13px; }
.rel-subhead { font-size: 11px; text-transform: uppercase; letter-spacing: 0; color: rgba(var(--v-theme-on-surface), .45); margin: 6px 0 4px; }
.rel-month-bars { display: flex; align-items: flex-end; gap: 2px; height: 48px; overflow-x: auto; }
.rel-month-bar { display: flex; flex-direction: column; align-items: center; min-width: 24px; height: 100%; justify-content: flex-end; }
.rel-month-fill { width: 16px; background: linear-gradient(180deg, #8ab4f8, rgba(138,180,248,.3)); border-radius: 2px 2px 0 0; min-height: 2px; }
.rel-month-bar small { font-size: 9px; color: rgba(var(--v-theme-on-surface), .4); margin-top: 2px; }
.timeline-list { padding: 4px 0; }
.timeline-item { display: flex; gap: 8px; padding: 6px 0; border-bottom: 1px solid rgba(var(--v-theme-on-surface), .05); }
.timeline-dot { width: 8px; height: 8px; border-radius: 50%; margin-top: 4px; flex-shrink: 0; }
.timeline-content { flex: 1; min-width: 0; }
.timeline-content strong { font-size: 12px; display: block; }
.timeline-content small { font-size: 10px; color: rgba(var(--v-theme-on-surface), .4); }
.timeline-text { font-size: 11px; color: rgba(var(--v-theme-on-surface), .7); display: block; margin-top: 2px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

.person-feed-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px 0;
}
.person-feed-item {
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  background: rgba(var(--v-theme-on-surface), 0.02);
  cursor: pointer;
  transition: all 0.15s ease;
  display: flex;
  flex-direction: column;
  gap: 5px;
}
.person-feed-item:hover {
  border-color: rgba(var(--v-theme-primary), 0.35);
  background: rgba(var(--v-theme-primary), 0.03);
}
.person-feed-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.person-feed-head .feed-time {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.person-feed-body {
  margin: 0;
  font-size: 12px;
  line-height: 1.5;
  color: rgba(var(--v-theme-on-surface), 0.88);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.person-feed-share {
  display: flex;
  align-items: center;
  padding: 5px 8px;
  border-radius: 4px;
  background: rgba(var(--v-theme-primary), 0.06);
  font-size: 11px;
  color: rgb(var(--v-theme-primary));
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.person-feed-foot {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 2px;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}

/* =========================================================
   链路规划 / 悬浮算路导航控制条 (Floating Route Planning Bar)
   ========================================================= */
.floating-route-planning-bar {
  position: absolute;
  top: 14px;
  left: 16px;
  right: 16px;
  max-width: 1040px;
  margin: 0 auto;
  z-index: 120;
  background: rgba(var(--v-theme-surface), 0.94);
  backdrop-filter: blur(20px) saturate(160%);
  border: 1px solid var(--card-border);
  border-radius: 8px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.2);
  padding: 10px 14px;
}

.route-inputs-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.route-input-group {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1 1 200px;
  min-width: 180px;
}

.route-input-label {
  font-size: 12px;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.7);
  white-space: nowrap;
}

.route-combobox {
  flex: 1;
}

.route-combobox :deep(.v-field) {
  font-size: 13px;
  border-radius: 6px;
}

.route-strategy-group {
  width: 210px;
}

.route-strategy-group :deep(.v-field),
.route-hops-group :deep(.v-field) {
  font-size: 13px;
  border-radius: 6px;
}

.route-hops-group {
  width: 105px;
}

/* =========================================================
   底部横向地铁线路轨 (Floating Subway Waybill Dock)
   ========================================================= */
.floating-subway-dock {
  position: absolute;
  bottom: 14px;
  left: 16px;
  right: 16px;
  z-index: 110;
  background: rgba(var(--v-theme-surface), 0.94);
  backdrop-filter: blur(24px) saturate(160%);
  border: 1px solid var(--card-border);
  border-radius: 8px;
  box-shadow: 0 12px 36px rgba(0, 0, 0, 0.2);
  padding: 8px 14px 12px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 220px;
  overflow: hidden;
}

.subway-dock-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--card-border);
}

.subway-route-title {
  font-size: 13px;
  font-weight: 700;
  color: #ffb74d;
}

.subway-track-scroll {
  overflow-x: auto;
  overflow-y: hidden;
  padding: 4px 2px 6px;
  scrollbar-width: thin;
}

.subway-track-container {
  display: inline-flex;
  align-items: center;
  gap: 0;
  min-width: 100%;
  padding: 4px 8px;
}

.subway-station {
  position: relative;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  flex-shrink: 0;
}

.subway-node-pill {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 10px;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 8px;
  transition: all 0.2s ease;
}

.subway-station:hover .subway-node-pill,
.subway-station.station-active .subway-node-pill {
  background: rgba(255, 179, 0, 0.18);
  border-color: #ffb300;
  transform: scale(1.04);
  box-shadow: 0 0 14px rgba(255, 179, 0, 0.35);
}

.subway-station-badge {
  font-size: 9px;
  font-weight: 700;
  padding: 1px 5px;
  border-radius: 4px;
  text-transform: uppercase;
  letter-spacing: 0;
}

.start-badge {
  background: #4caf50;
  color: #0b1926;
}

.transit-badge {
  background: #42a5f5;
  color: #0b1926;
}

.end-badge {
  background: #ff7043;
  color: #ffffff;
}

.subway-station-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.subway-station-info strong {
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.subway-station-info small {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.55);
  white-space: nowrap;
  max-width: 110px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.subway-segment {
  position: relative;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  min-width: 130px;
  padding: 0 8px;
  cursor: pointer;
  flex-shrink: 0;
}

.subway-segment-line {
  position: relative;
  width: 100%;
  height: 3px;
  background: linear-gradient(90deg, #ffb300, #ff9100);
  border-radius: 2px;
  margin-bottom: 6px;
  box-shadow: 0 0 8px rgba(255, 179, 0, 0.4);
}

.subway-segment:hover .subway-segment-line,
.subway-segment.segment-active .subway-segment-line {
  height: 4px;
  background: #00e5ff;
  box-shadow: 0 0 14px rgba(0, 229, 255, 0.7);
}

.subway-relation-chip {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.subway-step-time {
  font-size: 9px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}

.floating-waybill-pill {
  position: absolute;
  bottom: 16px;
  left: 16px;
  z-index: 110;
}

/* Transitions */
.slide-up-enter-active,
.slide-up-leave-active,
.slide-down-enter-active,
.slide-down-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
}

.slide-up-enter-from,
.slide-up-leave-to {
  opacity: 0;
  transform: translateY(20px);
}

.slide-down-enter-from,
.slide-down-leave-to {
  opacity: 0;
  transform: translateY(-20px);
}

/* =========================================================
   深度关系分析 (Deep Relationship UI)
   ========================================================= */
.rel-category-badge {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
}

.special-attention-alert {
  display: flex;
  align-items: flex-start;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(255, 152, 0, 0.12);
  border: 1px solid rgba(255, 152, 0, 0.35);
  color: #ffa726;
}

.special-attention-alert strong {
  font-size: 13px;
  font-weight: 700;
  display: block;
  margin-bottom: 2px;
}

.special-attention-alert p {
  margin: 0;
  font-size: 11px;
  line-height: 1.4;
  color: var(--text-main);
}

/* 双向非对称流动条 */
.directional-flow-bar {
  display: flex;
  height: 20px;
  width: 100%;
  border-radius: 8px;
  overflow: hidden;
  background: rgba(255, 255, 255, 0.05);
  border: 1px solid rgba(255, 255, 255, 0.1);
}

.flow-fill-this {
  background: linear-gradient(90deg, #42a5f5, #29b6f6);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  color: #0b1926;
  transition: width 0.3s ease;
}

.flow-fill-target {
  background: linear-gradient(90deg, #ab47bc, #ec407a);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-weight: 700;
  color: #ffffff;
  transition: width 0.3s ease;
}

.directional-breakdown-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  margin-top: 8px;
}

.breakdown-col {
  padding: 8px 10px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--card-border-subtle);
}

.breakdown-col.this-col {
  border-top: 2px solid #29b6f6;
}

.breakdown-col.target-col {
  border-top: 2px solid #ec407a;
}

.breakdown-head {
  font-size: 11px;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.8);
  margin-bottom: 6px;
}

.breakdown-items {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.breakdown-items span {
  display: flex;
  justify-content: space-between;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.6);
}

.breakdown-items span strong {
  color: rgba(var(--v-theme-on-surface), 0.9);
}

/* 24 小时作息柱状图 */
.circadian-chart {
  background: rgba(0, 0, 0, 0.15);
  border-radius: 8px;
  padding: 10px 8px 6px 8px;
  border: 1px solid var(--card-border-subtle);
}

.circadian-bars {
  display: grid;
  grid-template-columns: repeat(24, 1fr);
  height: 60px;
  align-items: flex-end;
  gap: 2px;
}

.circadian-bar-col {
  display: flex;
  flex-direction: column;
  align-items: center;
  height: 100%;
  justify-content: flex-end;
  position: relative;
}

.circadian-bar-inner {
  display: flex;
  align-items: flex-end;
  gap: 1px;
  width: 100%;
  height: 48px;
}

.circadian-fill-this {
  width: 50%;
  background: #29b6f6;
  border-radius: 2px 2px 0 0;
  min-height: 2px;
  transition: height 0.3s ease;
}

.circadian-fill-target {
  width: 50%;
  background: #ec407a;
  border-radius: 2px 2px 0 0;
  min-height: 2px;
  transition: height 0.3s ease;
}

.circadian-hour-label {
  font-size: 9px;
  color: rgba(var(--v-theme-on-surface), 0.4);
  margin-top: 2px;
  position: absolute;
  bottom: -14px;
}

.circadian-legend {
  display: flex;
  justify-content: flex-end;
  gap: 14px;
  margin-top: 14px;
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.6);
}

.circadian-legend span {
  display: flex;
  align-items: center;
  gap: 5px;
}

.circadian-legend i {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 2px;
}

.circadian-legend .legend-this {
  background: #29b6f6;
}

.circadian-legend .legend-target {
  background: #ec407a;
}

/* 共同关键人 / 中介 Triads 列表 */
.triad-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.triad-card {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 10px;
  border-radius: 6px;
  background: rgba(255, 255, 255, 0.02);
  border: 1px solid var(--card-border-subtle);
  cursor: pointer;
  transition: all 0.15s ease;
  width: 100%;
  text-align: left;
}

.triad-card:hover {
  background: rgba(var(--v-theme-primary), 0.06);
  border-color: rgba(var(--v-theme-primary), 0.3);
}

.triad-info {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.triad-info strong {
  font-size: 12px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.triad-info small {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.5);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 交互证据弹窗与时间线卡片样式 */
.evidence-dialog-card {
  background: var(--bg-surface-subtle) !important;
  border: 1px solid var(--card-border);
  border-radius: 8px;
}

.evidence-filter-bar {
  background: rgba(255, 255, 255, 0.02);
}

.evidence-timeline-stream {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.evidence-stream-card {
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid var(--card-border);
  border-radius: 8px;
  padding: 12px 14px;
  transition: all 0.15s ease;
}

.evidence-stream-card:hover {
  background: rgba(255, 255, 255, 0.05);
  border-color: rgba(var(--v-theme-primary), 0.3);
}

.card-top-line {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.actor-box {
  display: flex;
  align-items: center;
  gap: 6px;
}

.actor-avatar-wrapper {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: var(--card-border);
}

.actor-name {
  font-size: 13px;
}

.actor-qq {
  font-size: 11px;
}

.context-box {
  display: flex;
  align-items: center;
}

.context-tag {
  font-size: 11px;
  padding: 2px 6px;
  background: var(--card-border-subtle);
  border-radius: 4px;
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.content-bubble-container {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.content-text-bubble {
  font-size: 14px;
  line-height: 1.5;
  color: var(--text-main);
  background: var(--card-border-subtle);
  padding: 8px 12px;
  border-radius: 8px;
  border-left: 3px solid rgba(var(--v-theme-primary), 0.8);
  word-break: break-word;
}

.content-text-empty {
  font-size: 12px;
  color: var(--text-dim);
  font-style: italic;
}

.quote-context-box {
  background: rgba(0, 0, 0, 0.25);
  border: 1px dashed var(--card-border);
  border-radius: 6px;
  padding: 6px 10px;
  margin-top: 4px;
}

.quote-tag {
  font-size: 11px;
  color: rgba(var(--v-theme-primary), 0.8);
  margin-bottom: 2px;
}

.quote-text {
  font-size: 12px;
  color: var(--text-sub);
  line-height: 1.4;
  word-break: break-word;
}

.interactive-subway-chip {
  cursor: pointer !important;
  transition: transform 0.15s ease, box-shadow 0.15s ease;
}

.interactive-subway-chip:hover {
  transform: scale(1.05);
  box-shadow: 0 0 10px rgba(var(--v-theme-primary), 0.4);
}

.clickable-breakdown-btn {
  background: transparent;
  border: none;
  color: inherit;
  font: inherit;
  cursor: pointer;
  padding: 2px 4px;
  border-radius: 4px;
  transition: background 0.15s ease;
}

.clickable-breakdown-btn:hover {
  background: rgba(var(--v-theme-primary), 0.15);
  color: rgb(var(--v-theme-primary));
}

.bg-surface-variant-subtle {
  background: rgba(255, 255, 255, 0.03);
  border-radius: 8px;
}

.actor-mini-avatar {
  width: 20px;
  height: 20px;
  border-radius: 50%;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(255, 255, 255, 0.1);
  flex-shrink: 0;
}

.actor-mini-avatar img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.mini-char {
  font-size: 10px;
  font-weight: bold;
}

.drawer-evidence-bubble {
  background: rgba(255, 255, 255, 0.05);
  border-left: 2px solid rgba(var(--v-theme-primary), 0.8);
  font-size: 11px;
  line-height: 1.4;
  word-break: break-word;
}

.drawer-quote-box {
  background: rgba(0, 0, 0, 0.3);
  border-left: 2px solid rgba(var(--v-theme-info), 0.8);
  margin-top: 3px;
}
</style>

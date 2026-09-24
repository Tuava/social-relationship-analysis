<template>
  <div class="page-wrap records-page">
    <!-- 顶部搜索工具栏 -->
    <section class="list-toolbar records-toolbar data-surface">
      <v-text-field
        v-model.trim="query"
        label="昵称或 QQ 号"
        prepend-inner-icon="mdi-magnify"
        hide-details
        clearable
        density="compact"
        variant="outlined"
        @keyup.enter="resetAndLoad"
        @click:clear="resetAndLoad"
      />
      <v-btn color="primary" variant="tonal" :loading="loading" @click="resetAndLoad">搜索</v-btn>
      <v-btn color="primary" variant="flat" prepend-icon="mdi-brain" @click="openBatchPersonaDialog">批量全息研判</v-btn>
      <span class="result-hint">{{ new Intl.NumberFormat('zh-CN').format(total) }} 人</span>
      <v-btn icon="mdi-refresh" variant="text" :loading="loading" title="刷新" @click="load" />
    </section>

    <!-- 顶层错误提示 -->
    <v-alert v-if="pageError" type="error" variant="tonal" class="mt-3" closable @click:close="pageError = ''">
      {{ pageError }}
    </v-alert>

    <!-- 用户数据表格 -->
    <section class="data-surface records-surface mt-3">
      <v-data-table-server
        class="records-table"
        density="compact"
        :headers="headers"
        :items="persons"
        :loading="loading"
        :items-length="total"
        :items-per-page="pageSize"
        :page="page"
        :items-per-page-options="pageSizeOptions"
        hover
        @click:row="openPersonRow"
        @update:page="onPageChange"
        @update:items-per-page="onPageSizeChange"
      >
        <!-- 用户身份列 -->
        <template #item.display_name="{ item }">
          <div class="identity-cell">
            <v-avatar size="34" color="surface-variant" class="id-avatar">
              <AuthImage :src="detailAvatar(item)" :fallback-srcs="avatarFallbacks(qqOf(item))" :alt="item.display_name || ''">
                <span>{{ initials(item.display_name) }}</span>
              </AuthImage>
            </v-avatar>
            <div>
              <strong>{{ item.display_name || "未命名用户" }}</strong>
              <small>QQ {{ qqOf(item) || "未知" }}</small>
            </div>
          </div>
        </template>

        <!-- 首次发现时间 -->
        <template #item.first_seen_at="{ item }">
          <span class="text-caption text-medium-emphasis">{{ formatDate(item.first_seen_at) }}</span>
        </template>

        <!-- 最后发现时间 -->
        <template #item.last_seen_at="{ item }">
          <span class="text-caption text-medium-emphasis">{{ formatDate(item.last_seen_at) }}</span>
        </template>

        <!-- 操作列 -->
        <template #item.actions="{ item }">
          <v-btn icon="mdi-chevron-right" size="small" variant="text" @click.stop="openPersonDirect(item)" />
        </template>

        <!-- 空数据提示 -->
        <template #no-data>
          <div class="empty-table pa-6 text-center">
            <v-icon icon="mdi-account-search-outline" size="36" class="mb-2 text-medium-emphasis" />
            <div class="font-weight-medium mb-1">没有匹配用户</div>
            <span class="text-caption text-medium-emphasis">请先完成一次采集，或换一个 QQ 号搜索。</span>
          </div>
        </template>
      </v-data-table-server>
    </section>

    <!-- 用户全量详情抽屉 -->
    <v-navigation-drawer v-model="drawer" location="right" temporary width="580">
      <div class="detail-drawer" v-if="drawer">
        <!-- 抽屉顶部关闭条 -->
        <div class="d-flex justify-space-between align-center mb-2">
          <span class="text-caption text-medium-emphasis">用户全息档案</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="drawer = false" />
        </div>

        <!-- 抽屉内部错误提示 -->
        <v-alert v-if="drawerError" type="error" variant="tonal" density="compact" class="mb-3" closable @click:close="drawerError = ''">
          {{ drawerError }}
        </v-alert>

        <!-- 骨架屏加载 -->
        <v-skeleton-loader v-if="detailLoading" type="avatar,heading,paragraph,list-item-three-line@3" />

        <template v-else-if="detail">
          <!-- 统一紧凑的 Hero 卡片 (去除重复名字与QQ) -->
          <div class="person-hero-card">
            <v-avatar size="64" color="primary" class="hero-avatar elevation-1">
              <AuthImage :src="detailAvatar(detail)" :fallback-srcs="avatarFallbacks(detailQQ)" :alt="detail.display_name">
                <span class="text-h6 font-weight-bold">{{ initials(detail.display_name) }}</span>
              </AuthImage>
            </v-avatar>
            <div class="hero-body">
              <div class="d-flex align-center gap-2 flex-wrap mb-1">
                <h2 class="text-h6 font-weight-bold mb-0 text-truncate" style="max-width: 240px;">{{ detail.display_name || "未命名用户" }}</h2>
                <v-chip v-if="detailQQ" size="small" color="primary" variant="flat" class="font-weight-medium">
                  QQ {{ detailQQ }}
                </v-chip>
              </div>
              <div class="text-caption text-medium-emphasis">
                最后活跃：{{ formatDate(detail.last_seen_at) }}
              </div>
            </div>
            <div class="hero-actions">
              <v-btn size="small" variant="tonal" prepend-icon="mdi-account-sync-outline" :loading="profileSyncing" @click="syncProfile">
                补充资料
              </v-btn>
              <v-btn size="small" variant="tonal" color="secondary" prepend-icon="mdi-graph-outline" @click="researchPerson">
                关系研究
              </v-btn>
            </div>
          </div>

          <!-- 导航选项卡 -->
          <v-tabs v-model="tab" density="compact" class="detail-tabs mt-3" show-arrows>
            <v-tab value="profile">资料</v-tab>
            <v-tab value="persona">全息画像</v-tab>
            <v-tab value="feed_dynamics">空间研判</v-tab>
            <v-tab value="coverage">采集</v-tab>
            <v-tab value="groups">群组 ({{ detail.memberships?.length || 0 }})</v-tab>
            <v-tab value="messages">消息 ({{ detail.messages?.length || 0 }})</v-tab>
            <v-tab value="feeds">动态 ({{ detail?.content_count ?? feeds.length }})</v-tab>
            <v-tab value="timeline">时间线</v-tab>
          </v-tabs>

          <v-window v-model="tab" @update:model-value="onTabChange">
            <!-- 1. 基础资料 -->
            <v-window-item value="profile">
              <!-- 个性签名 (如有) -->
              <div v-if="userSignature" class="signature-card mb-3">
                <v-icon icon="mdi-format-quote-open" size="16" color="primary" class="mr-1" />
                <span class="text-body-2 font-italic">{{ cleanDisplayText(userSignature) }}</span>
              </div>

              <!-- 结构化清晰资料 (彻底过滤 0, unknown, 未知等脏数据) -->
              <div class="detail-section" v-if="profileFields.length">
                <h3>详细资料</h3>
                <div class="profile-info-grid">
                  <div v-for="field in profileFields" :key="field.key" class="info-grid-item">
                    <span class="info-label">{{ field.label }}</span>
                    <strong class="info-value">{{ cleanDisplayText(field.value) }}</strong>
                  </div>
                </div>
              </div>

              <!-- 其它多平台标识 (如有) -->
              <div class="detail-section" v-if="extraIdentifiers.length">
                <h3>其他平台标识</h3>
                <div v-for="identifier in extraIdentifiers" :key="identifier.platform + identifier.value" class="key-value-row">
                  <span>{{ (identifier.platform || '').toUpperCase() }}</span>
                  <strong>{{ identifier.value }}</strong>
                </div>
              </div>

              <!-- 头像历史 (自动去重) -->
              <div class="detail-section">
                <h3>头像版本 ({{ avatarHistory.length }})</h3>
                <div v-if="avatarHistory.length" class="avatar-history">
                  <div v-for="(profile, pIdx) in avatarHistory" :key="pIdx" class="avatar-history-item">
                    <v-avatar size="44" color="surface-variant">
                      <AuthImage :src="profile.avatar_uri" :alt="profile.nickname">
                        <span>{{ initials(profile.nickname) }}</span>
                      </AuthImage>
                    </v-avatar>
                    <small>{{ formatDate(profile.valid_from) }}</small>
                  </div>
                </div>
                <div v-else class="compact-empty">暂无头像版本</div>
              </div>

              <!-- 昵称变更历史 (自动去重) -->
              <div class="detail-section">
                <h3>历史更名与名片 ({{ nicknameHistory.length }})</h3>
                <div v-if="nicknameHistory.length">
                  <div v-for="(profile, nIdx) in nicknameHistory" :key="nIdx" class="timeline-row">
                    <i />
                    <div>
                      <strong>{{ cleanDisplayText(profile.nickname) || "未记录昵称" }}</strong>
                      <span v-if="profile.card_or_remark && profile.card_or_remark !== profile.nickname" class="text-caption text-medium-emphasis d-block">
                        名片 / 备注：{{ cleanDisplayText(profile.card_or_remark) }}
                      </span>
                      <small>{{ profile.source }} · {{ formatDate(profile.valid_from) }}</small>
                    </div>
                  </div>
                </div>
                <div v-else class="compact-empty">暂无更名记录</div>
              </div>

              <!-- 关系事件汇总 -->
              <div class="detail-section" v-if="detail.relations?.length">
                <h3>关系与互动汇总</h3>
                <div v-for="relation in detail.relations" :key="relation.action_type + relation.context_type" class="key-value-row">
                  <span>{{ relationLabel(relation.action_type) }}</span>
                  <v-chip size="x-small" variant="tonal" color="primary">{{ relation.count }} 次</v-chip>
                </div>
              </div>
            </v-window-item>

            <!-- 2. 全息画像与行为心理研判 -->
            <v-window-item value="persona">
              <div class="detail-section">
                <div class="d-flex align-center justify-space-between mb-3">
                  <h3 class="mb-0">全息画像与行为心理研判</h3>
                  <v-btn size="small" variant="tonal" color="primary" prepend-icon="mdi-brain" :loading="personaLoading" @click="loadPersona(true)">
                    重新深度研判
                  </v-btn>
                </div>

                <v-skeleton-loader v-if="personaLoading" type="paragraph,list-item-three-line@4" />

                <template v-else-if="persona">
                  <!-- 2.1 生平事实与成长轨迹拼图 -->
                  <div class="persona-card mb-3" v-if="persona.biographical_anchors?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-card-account-details-outline" size="18" class="mr-1 text-primary" />
                      生平事实与成长轨迹拼图
                    </div>
                    <div v-for="(anchor, aIdx) in persona.biographical_anchors" :key="aIdx" class="anchor-item mb-2 pa-3 rounded">
                      <div class="d-flex align-center justify-space-between mb-1">
                        <v-chip size="x-small" color="primary" variant="flat" class="font-weight-bold">{{ anchor.aspect }}</v-chip>
                      </div>
                      <p class="text-body-2 font-weight-medium mb-1 text-high-emphasis">{{ anchor.detail }}</p>
                      <div class="verbatim-evidence-box" v-if="anchor.verbatim_evidence">
                        <v-icon icon="mdi-format-quote-open" size="14" class="mr-1 text-primary" />
                        <span class="evidence-text">原话佐证: "{{ anchor.verbatim_evidence }}"</span>
                      </div>
                    </div>
                  </div>

                  <!-- 2.2 心理防御机制与人格暗线 -->
                  <div class="persona-card mb-3" v-if="persona.psychological_defense">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-shield-account-outline" size="18" class="mr-1 text-warning" />
                      心理防御机制与人格暗线
                    </div>
                    <div class="defense-block mb-3 pa-2 rounded" v-if="persona.psychological_defense.core_wound_and_insecurity">
                      <div class="d-flex align-center gap-1 mb-1">
                        <v-icon icon="mdi-alert-decagram-outline" size="14" color="error" />
                        <strong class="text-caption text-error font-weight-bold">核心自卑与敏感区:</strong>
                      </div>
                      <p class="text-body-2 text-high-emphasis mb-0 pl-4">{{ persona.psychological_defense.core_wound_and_insecurity }}</p>
                    </div>
                    <div class="defense-block mb-3 pa-2 rounded" v-if="persona.psychological_defense.defense_mechanism">
                      <div class="d-flex align-center gap-1 mb-1">
                        <v-icon icon="mdi-shield-alert-outline" size="14" color="warning" />
                        <strong class="text-caption text-warning font-weight-bold">防御与自保模式:</strong>
                      </div>
                      <p class="text-body-2 text-high-emphasis mb-0 pl-4">{{ persona.psychological_defense.defense_mechanism }}</p>
                    </div>
                    <div class="defense-block pa-2 rounded" v-if="persona.psychological_defense.empathy_and_attachment">
                      <div class="d-flex align-center gap-1 mb-1">
                        <v-icon icon="mdi-hand-heart-outline" size="14" color="teal" />
                        <strong class="text-caption text-teal font-weight-bold">共情与情感依恋:</strong>
                      </div>
                      <p class="text-body-2 text-high-emphasis mb-0 pl-4">{{ persona.psychological_defense.empathy_and_attachment }}</p>
                    </div>
                  </div>

                  <!-- 2.3 代表性原话金句库 -->
                  <div class="persona-card mb-3" v-if="persona.verbatim_anchor_quotes?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-format-quote-close-outline" size="18" class="mr-1 text-primary" />
                      典型原话与心路透视
                    </div>
                    <div v-for="(vq, vIdx) in persona.verbatim_anchor_quotes" :key="vIdx" class="quote-card mb-2 pa-3 rounded">
                      <div class="d-flex align-top mb-1">
                        <v-icon icon="mdi-format-quote-open" size="16" color="primary" class="mr-1 flex-shrink-0" />
                        <strong class="text-body-2 text-primary font-weight-bold">{{ vq.quote }}</strong>
                      </div>
                      <div class="quote-context-text pl-5">
                        <span class="text-caption text-high-emphasis">{{ vq.context }}</span>
                      </div>
                    </div>
                  </div>

                  <!-- 2.4 语言特征与微观口癖 -->
                  <div class="persona-card mb-3" v-if="persona.linguistic_fingerprint">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-fingerprint" size="18" class="mr-1 text-deep-purple" />
                      微观语言指纹与暗语
                    </div>
                    <div class="mb-3" v-if="persona.linguistic_fingerprint.catchphrases?.length">
                      <strong class="text-caption text-high-emphasis d-block mb-1">高频口癖:</strong>
                      <div class="d-flex flex-wrap gap-1">
                        <v-chip v-for="cp in persona.linguistic_fingerprint.catchphrases" :key="cp" size="small" color="primary" variant="tonal" class="font-weight-bold">{{ cp }}</v-chip>
                      </div>
                    </div>
                    <div class="mb-3" v-if="persona.linguistic_fingerprint.slang_and_subculture?.length">
                      <strong class="text-caption text-high-emphasis d-block mb-1">黑话与圈子暗语:</strong>
                      <div class="d-flex flex-wrap gap-1">
                        <v-chip v-for="sl in persona.linguistic_fingerprint.slang_and_subculture" :key="sl" size="small" color="deep-orange" variant="tonal" class="font-weight-bold">{{ sl }}</v-chip>
                      </div>
                    </div>
                    <div class="key-value-row text-body-2 mb-1" v-if="persona.linguistic_fingerprint.tone_baseline">
                      <span class="text-caption text-medium-emphasis">语调底色</span><strong class="text-high-emphasis">{{ persona.linguistic_fingerprint.tone_baseline }}</strong>
                    </div>
                    <div class="key-value-row text-body-2" v-if="persona.linguistic_fingerprint.sentence_style">
                      <span class="text-caption text-medium-emphasis">句式特征</span><strong class="text-high-emphasis">{{ persona.linguistic_fingerprint.sentence_style }}</strong>
                    </div>
                  </div>

                  <!-- 2.5 具象化领域图谱 -->
                  <div class="persona-card mb-3" v-if="persona.interest_spectrum?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-compass-outline" size="18" class="mr-1 text-teal" />
                      具象领域图谱
                    </div>
                    <div v-for="dom in persona.interest_spectrum" :key="dom.domain" class="interest-domain-card mb-3 pa-3 rounded">
                      <div class="d-flex justify-space-between align-center mb-1">
                        <strong class="text-body-2 text-high-emphasis">{{ dom.domain }}</strong>
                        <v-chip size="x-small" color="primary" variant="flat" class="font-weight-bold">{{ Math.round((dom.score || 0) * 100) }}%</v-chip>
                      </div>
                      <v-progress-linear :model-value="(dom.score || 0) * 100" color="primary" height="6" rounded class="mb-2" />
                      <p class="text-caption text-high-emphasis mb-2" v-if="dom.context_description">{{ dom.context_description }}</p>
                      <div class="d-flex flex-wrap gap-1" v-if="dom.specific_entities?.length">
                        <v-chip v-for="ent in dom.specific_entities" :key="ent" size="x-small" variant="tonal" color="teal" class="font-weight-bold">{{ ent }}</v-chip>
                      </div>
                    </div>
                  </div>

                  <!-- 2.6 社群生态定位与深层分析 -->
                  <div class="persona-card mb-3" v-if="persona.social_archetype">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-account-star-outline" size="18" class="mr-1 text-deep-purple" />
                      社群生态定位与心理画像
                    </div>
                    <div class="d-flex align-center justify-space-between mb-2">
                      <v-chip color="deep-purple" variant="flat" size="small" class="font-weight-bold">{{ persona.social_archetype.primary_role || "未定型" }}</v-chip>
                      <span class="text-caption text-high-emphasis" v-if="persona.social_archetype.influence_score">影响力指数: <strong class="text-primary">{{ persona.social_archetype.influence_score }}/100</strong></span>
                    </div>
                    <div class="text-caption mb-1" v-if="persona.social_archetype.community_function">
                      <strong class="text-medium-emphasis">社群功能:</strong> <span class="text-high-emphasis">{{ persona.social_archetype.community_function }}</span>
                    </div>
                    <p class="text-body-2 text-high-emphasis mb-0 mt-2 pa-3 rounded border bg-surface-variant">{{ persona.social_archetype.deep_analysis || persona.social_archetype.description }}</p>
                  </div>

                  <!-- 2.7 严密证据链 -->
                  <div class="persona-card" v-if="persona.evidence_message_ids?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-shield-check-outline" size="18" class="mr-1 text-success" />
                      严密证据链 ({{ persona.evidence_count }} 条支撑)
                    </div>
                    <small class="text-medium-emphasis d-block mb-2">本画像推断基于数据库中 {{ persona.evidence_count }} 条客观历史发言分析生成，已锚定原始证据：</small>
                    <div class="d-flex flex-wrap gap-1">
                      <v-chip v-for="eid in (persona.evidence_message_ids || []).slice(0, 10)" :key="eid" size="x-small" variant="tonal" color="primary" class="font-weight-medium" prepend-icon="mdi-message-text-outline">
                        消息 #{{ eid.slice(0, 8) }}
                      </v-chip>
                    </div>
                  </div>
                </template>
                <div v-else class="compact-empty">
                  <div>暂未生成该用户的深度全息画像</div>
                  <v-btn size="small" variant="tonal" color="primary" class="mt-2" prepend-icon="mdi-brain" :loading="personaLoading" @click="loadPersona(true)">
                    立即执行深度全息研判
                  </v-btn>
                </div>
              </div>
            </v-window-item>

            <!-- 3. 空间动态与情感叙事 -->
            <v-window-item value="feed_dynamics">
              <div class="detail-section">
                <div class="d-flex align-center justify-space-between mb-3">
                  <h3 class="mb-0">空间动态与情感叙事</h3>
                  <v-btn size="small" variant="tonal" color="primary" prepend-icon="mdi-chart-timeline-variant" :loading="feedDynamicsLoading" @click="loadFeedDynamics(true)">
                    重新研判
                  </v-btn>
                </div>

                <v-skeleton-loader v-if="feedDynamicsLoading" type="paragraph,list-item-three-line@3" />

                <template v-else-if="feedDynamics">
                  <div class="persona-card mb-3" v-if="feedDynamics.sentiment_flow?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-heart-pulse" size="18" class="mr-1" />
                      时序情感波动流
                    </div>
                    <div v-for="sf in feedDynamics.sentiment_flow" :key="sf.month" class="mb-2 pb-2 border-b">
                      <div class="d-flex justify-space-between align-center mb-1">
                        <strong>{{ sf.month }} ({{ sf.post_count }}条动态)</strong>
                        <v-chip size="x-small" :color="sf.sentiment_score >= 0 ? 'success' : 'error'" variant="tonal">
                          {{ sf.dominant_emotion }} ({{ sf.sentiment_score > 0 ? '+' : '' }}{{ sf.sentiment_score }})
                        </v-chip>
                      </div>
                      <p class="text-body-2 text-medium-emphasis mb-0">{{ sf.summary }}</p>
                    </div>
                  </div>

                  <div class="persona-card mb-3" v-if="feedDynamics.circle_hierarchy">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-account-group-outline" size="18" class="mr-1" />
                      3-Tier 核心社交圈层
                    </div>

                    <div class="mb-2" v-if="feedDynamics.circle_hierarchy.tier1_speedy?.length">
                      <div class="text-caption font-weight-bold text-primary mb-1">Tier 1: 5分钟极速互动 (秒赞秒评)</div>
                      <div class="d-flex flex-wrap">
                        <v-chip v-for="m in feedDynamics.circle_hierarchy.tier1_speedy" :key="m.qq" size="small" color="primary" variant="flat" class="mr-1 mb-1" @click="openPersonByQQ(m.qq)">
                          {{ m.name || m.qq }} ({{ m.count }}次)
                        </v-chip>
                      </div>
                    </div>

                    <div class="mb-2" v-if="feedDynamics.circle_hierarchy.tier2_deep?.length">
                      <div class="text-caption font-weight-bold text-teal mb-1">Tier 2: 深度常评互动</div>
                      <div class="d-flex flex-wrap">
                        <v-chip v-for="m in feedDynamics.circle_hierarchy.tier2_deep" :key="m.qq" size="small" color="teal" variant="tonal" class="mr-1 mb-1" @click="openPersonByQQ(m.qq)">
                          {{ m.name || m.qq }} ({{ m.count }}次)
                        </v-chip>
                      </div>
                    </div>

                    <div class="mb-2" v-if="feedDynamics.circle_hierarchy.tier3_casual?.length">
                      <div class="text-caption font-weight-bold text-grey mb-1">Tier 3: 泛社交与点赞</div>
                      <div class="d-flex flex-wrap">
                        <v-chip v-for="m in (feedDynamics.circle_hierarchy.tier3_casual || []).slice(0, 10)" :key="m.qq" size="small" variant="outlined" class="mr-1 mb-1" @click="openPersonByQQ(m.qq)">
                          {{ m.name || m.qq }} ({{ m.count }}次)
                        </v-chip>
                      </div>
                    </div>
                  </div>

                  <div class="persona-card mb-3" v-if="feedDynamics.narrative_milestones?.length">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-flag-triangle" size="18" class="mr-1" />
                      核心叙事里程碑
                    </div>
                    <div v-for="ms in feedDynamics.narrative_milestones" :key="ms.period" class="timeline-row mb-2">
                      <i />
                      <div>
                        <strong>{{ ms.period }}: {{ ms.event }}</strong>
                        <span class="text-body-2">{{ ms.psychological_impact }}</span>
                      </div>
                    </div>
                  </div>

                  <div class="persona-card" v-if="feedDynamics.circle_summary">
                    <div class="persona-card-title">
                      <v-icon icon="mdi-text-box-search-outline" size="18" class="mr-1" />
                      圈层研判综述
                    </div>
                    <p class="text-body-2 mb-0">{{ feedDynamics.circle_summary }}</p>
                  </div>
                </template>
                <div v-else class="compact-empty">
                  <div>暂未生成空间动态研判</div>
                  <v-btn size="small" variant="tonal" color="primary" class="mt-2" prepend-icon="mdi-chart-timeline-variant" :loading="feedDynamicsLoading" @click="loadFeedDynamics(true)">
                    立即执行空间研判
                  </v-btn>
                </div>
              </div>
            </v-window-item>

            <!-- 4. 采集覆盖 -->
            <v-window-item value="coverage">
              <div class="detail-section flush">
                <h3>采集覆盖</h3>
                <v-skeleton-loader v-if="coverageLoading" type="list-item-three-line@4" />
                <template v-else>
                  <div v-for="source in coverage" :key="source.source" class="coverage-row">
                    <div class="coverage-info">
                      <strong>{{ source.label || source.source }}</strong>
                      <small>{{ source.source }}</small>
                    </div>
                    <v-chip size="x-small" :color="coverageColor(source.status)" variant="tonal">
                      {{ coverageLabel(source.status) }}
                    </v-chip>
                    <div class="coverage-meta">
                      <span>{{ source.items_collected }} 条</span>
                      <small v-if="source.last_collected_at">{{ formatDate(source.last_collected_at) }}</small>
                      <small v-if="source.last_error" class="coverage-error">{{ source.last_error }}</small>
                    </div>
                    <v-btn size="x-small" variant="tonal" prepend-icon="mdi-download-outline" :loading="collecting === source.source" :disabled="source.status === 'collecting'" @click="collectSource(source.source)">
                      采集
                    </v-btn>
                  </div>
                </template>
              </div>
            </v-window-item>

            <!-- 5. 群组关系 -->
            <v-window-item value="groups">
              <div class="detail-section flush">
                <div v-for="group in (detail.memberships || [])" :key="group.id" class="group-card-row cursor-pointer" @click="openGroup(group.group_id)">
                  <v-avatar rounded="sm" size="40" color="surface-variant">
                    <v-icon icon="mdi-account-group-outline" />
                  </v-avatar>
                  <div class="flex-grow-1">
                    <strong>{{ cleanDisplayText(group.group_name) || group.group_id }}</strong>
                    <small>群号 {{ group.group_id }} · {{ roleLabel(group.role) }}</small>
                    <small v-if="group.card">名片：{{ cleanDisplayText(group.card) }}</small>
                  </div>
                  <v-icon icon="mdi-chevron-right" size="18" color="grey" />
                </div>
                <div v-if="!detail.memberships?.length" class="compact-empty">未发现群成员关系</div>
              </div>
            </v-window-item>

            <!-- 6. 消息记录 -->
            <v-window-item value="messages">
              <div class="detail-section flush">
                <div v-for="message in (detail.messages || [])" :key="message.id" class="message-row cursor-pointer" @click="openMessage(message)">
                  <p>{{ cleanDisplayText(message.text) || "[非文本消息]" }}</p>
                  <small>{{ message.conversation_type === 'group' ? '群聊' : '私聊' }} {{ message.conversation_id }} · {{ formatDate(message.sent_at) }}</small>
                </div>
                <div v-if="!detail.messages?.length" class="compact-empty">暂无消息记录</div>
              </div>
            </v-window-item>

            <!-- 7. 空间动态 -->
            <v-window-item value="feeds">
              <div class="detail-section flush">
                <v-skeleton-loader v-if="feedsLoading" type="list-item-three-line@3" />
                <template v-else>
                  <div v-if="feeds.length" class="person-feed-list">
                    <article v-for="feed in feeds" :key="feed.id" class="person-feed-item" @click="goToFeed(feed.id)">
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
                  <div v-else class="compact-empty">
                    <div>暂未记录该用户的空间动态</div>
                    <v-btn size="small" variant="tonal" color="primary" class="mt-2" prepend-icon="mdi-download-outline" :loading="collecting === 'qzone_feeds'" @click="collectSource('qzone_feeds')">
                      发起空间动态采集
                    </v-btn>
                  </div>
                </template>
              </div>
            </v-window-item>

            <!-- 8. 时间线 -->
            <v-window-item value="timeline">
              <div class="detail-section flush">
                <v-skeleton-loader v-if="timelineLoading" type="list-item-three-line@4" />
                <template v-else>
                  <div v-for="event in timeline" :key="event.id" class="timeline-row cursor-pointer" @click="openTimelineEvent(event)">
                    <i />
                    <div>
                      <strong>{{ eventLabel(event.event_type) }}</strong>
                      <span v-if="eventDetails(event)">{{ eventDetails(event) }}</span>
                      <small>{{ formatDate(event.occurred_at) }}</small>
                    </div>
                    <v-icon v-if="event.target_object_id || event.source_id" icon="mdi-open-in-new" size="14" class="timeline-jump-ico ml-auto" />
                  </div>
                  <div v-if="!timeline.length" class="compact-empty">暂无时间线事件</div>
                </template>
              </div>
            </v-window-item>
          </v-window>
        </template>
      </div>
    </v-navigation-drawer>

    <!-- 批量全息画像研判任务调度中心 -->
    <v-dialog v-model="batchPersonaDialog" max-width="840" scrollable>
      <v-card class="batch-persona-dialog data-surface">
        <v-card-title class="dialog-title d-flex align-center justify-space-between pa-4">
          <div class="d-flex align-center">
            <v-icon icon="mdi-brain" color="primary" class="mr-2" size="24" />
            <div>
              <strong class="text-subtitle-1 font-weight-bold">批量全息画像研判调度中心</strong>
              <small class="text-caption text-medium-emphasis d-block">对目标人物群实施刑侦级全息画像提炼、心理防御透视与任务总结</small>
            </div>
          </div>
          <v-btn icon="mdi-close" variant="text" size="small" @click="batchPersonaDialog = false" />
        </v-card-title>
        <v-divider />

        <v-card-text class="pa-4">
          <!-- 阶段一：任务配置 (未运行时) -->
          <div v-if="!batchRunning && !batchSummary" class="batch-config-panel">
            <v-row dense>
              <v-col cols="12" md="6">
                <v-select
                  v-model="batchScope"
                  :items="batchScopeOptions"
                  label="研判目标范围"
                  density="compact"
                  variant="outlined"
                />
              </v-col>
              <v-col cols="12" md="4">
                <v-number-input v-model="batchMaxRetries" :min="0" :max="10" label="最大重试次数" density="compact" control-variant="split" hide-details />
              </v-col>
              <v-col cols="12" md="4">
                <v-number-input v-model="batchRetryBackoffSeconds" :min="1" :max="3600" label="重试等待（秒）" density="compact" control-variant="split" hide-details />
              </v-col>
              <v-col cols="12" md="4">
                <v-number-input v-model="batchQueueWaitSeconds" :min="1" :max="3600" label="队列等待超时（秒）" density="compact" control-variant="split" hide-details />
              </v-col>
              <v-col cols="12" md="6">
                <v-slider
                  v-model="batchConcurrency"
                  :min="1"
                  :max="5"
                  :step="1"
                  label="研判并发数"
                  thumb-label="always"
                  color="primary"
                  density="compact"
                  class="mt-2"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-switch
                  v-model="batchAutoRetry"
                  color="warning"
                  :label="`网络波动/限流自动重试（指数退避，最多${batchMaxRetries}次）`"
                  hide-details
                  inset
                  density="compact"
                />
              </v-col>
              <v-col cols="12" md="6">
                <v-switch
                  v-model="batchForceAnalyze"
                  color="primary"
                  label="强制重新研判 (覆盖已有画像缓存)"
                  hide-details
                  inset
                  density="compact"
                />
              </v-col>
            </v-row>

            <v-alert type="info" variant="tonal" class="mt-3 mb-0" density="compact">
              <strong>研判提示</strong>：将自动读取所选目标的历史聊天流水，按事实铁律锚定生平轨迹、透视心理防御机制与口癖，并生成全局任务画像总结。
            </v-alert>
          </div>

          <!-- 阶段二：实时执行与进度监控 -->
          <div v-if="batchRunning || (batchCompleted && !batchSummary)" class="batch-running-panel">
            <!-- 实时进度条 -->
            <div class="mb-4">
              <div class="d-flex justify-space-between align-center mb-1">
                <strong class="text-subtitle-2 text-primary">
                  {{ batchRunning ? "正在执行批量画像研判..." : "批量研判任务已完成！" }}
                </strong>
                <span class="text-caption font-weight-bold">{{ batchProgressPercent }}%</span>
              </div>
              <v-progress-linear
                :model-value="batchProgressPercent"
                :color="batchFailedCount > 0 ? 'warning' : 'primary'"
                height="8"
                rounded
                striped
                :indeterminate="batchProgressPercent === 0 && batchRunning"
              />
            </div>

            <!-- HUD 统计指标 -->
            <div class="batch-hud-grid mb-4">
              <div class="batch-hud-card">
                <span class="text-caption text-medium-emphasis">目标总数</span>
                <strong class="text-h6 text-primary">{{ batchTotalCount }}</strong>
              </div>
              <div class="batch-hud-card">
                <span class="text-caption text-medium-emphasis">研判成功</span>
                <strong class="text-h6 text-success">{{ batchSuccessCount }}</strong>
              </div>
              <div class="batch-hud-card">
                <span class="text-caption text-medium-emphasis">重试/失败</span>
                <strong class="text-h6 text-error">{{ batchFailedCount }}</strong>
              </div>
              <div class="batch-hud-card">
                <span class="text-caption text-medium-emphasis">剩余排队</span>
                <strong class="text-h6 text-medium-emphasis">{{ batchPendingCount }}</strong>
              </div>
            </div>

            <!-- 当前活跃目标提示 -->
            <div v-if="batchCurrentTarget" class="current-target-card pa-3 rounded border mb-3 d-flex align-center">
              <v-avatar size="36" color="primary" class="mr-3">
                <span>{{ initials(batchCurrentTarget.display_name) }}</span>
              </v-avatar>
              <div class="flex-grow-1">
                <div class="d-flex align-center gap-2">
                  <strong class="text-body-2">{{ batchCurrentTarget.display_name }}</strong>
                  <v-chip size="x-small" color="primary" variant="flat">QQ {{ qqOf(batchCurrentTarget) }}</v-chip>
                </div>
                <small class="text-caption text-medium-emphasis">正在分析历史发言流水、事实锚点与心理防御...</small>
              </div>
              <v-progress-circular indeterminate size="20" width="2" color="primary" />
            </div>

            <!-- 实时事件控制台 -->
            <div class="batch-log-console pa-3 rounded border">
              <div class="text-caption text-medium-emphasis mb-2 font-weight-bold">实时研判事件日志</div>
              <div class="log-stream-box">
                <div v-for="(log, lIdx) in batchLogs" :key="lIdx" class="log-line" :class="'log-' + log.type">
                  <span class="log-time">{{ log.time }}</span>
                  <span class="log-msg">{{ log.message }}</span>
                </div>
                <div v-if="!batchLogs.length" class="text-caption text-medium-emphasis text-center py-4">
                  等待任务调度启动...
                </div>
              </div>
            </div>
          </div>

          <!-- 阶段三：任务画像总结 (Task Persona Summary) -->
          <div v-if="batchSummary" class="batch-summary-panel">
            <div class="d-flex align-center justify-space-between mb-3">
              <h3 class="text-subtitle-1 font-weight-bold mb-0 text-primary">
                <v-icon icon="mdi-chart-pie" class="mr-1" />
                全员任务画像总结与社群洞察报告
              </h3>
              <v-btn size="x-small" variant="tonal" color="primary" prepend-icon="mdi-download" @click="exportBatchSummary">
                导出研判报告 (Markdown)
              </v-btn>
            </div>

            <!-- 1. 核心人格与社会角色分布 -->
            <div class="summary-section pa-3 rounded border mb-3">
              <strong class="text-subtitle-2 text-deep-purple d-block mb-2">1. 核心社群角色与心理分布</strong>
              <div class="d-flex flex-wrap gap-2">
                <v-chip v-for="role in batchSummary.roles" :key="role.name" color="deep-purple" variant="tonal" class="font-weight-bold">
                  {{ role.name }}: {{ role.count }}人 ({{ role.percent }}%)
                </v-chip>
              </div>
            </div>

            <!-- 2. 全员 TOP 高频口癖与黑话词云 -->
            <div class="summary-section pa-3 rounded border mb-3">
              <strong class="text-subtitle-2 text-primary d-block mb-2">2. 全员高频口癖与圈子黑话</strong>
              <div class="d-flex flex-wrap gap-1">
                <v-chip v-for="cp in batchSummary.catchphrases" :key="cp.phrase" color="primary" variant="flat" size="small" class="font-weight-bold">
                  {{ cp.phrase }} ({{ cp.count }}次)
                </v-chip>
                <v-chip v-for="sl in batchSummary.slang" :key="sl.phrase" color="deep-orange" variant="tonal" size="small" class="font-weight-bold">
                  {{ sl.phrase }} ({{ sl.count }}次)
                </v-chip>
              </div>
            </div>

            <!-- 3. TOP 关注领域图谱 -->
            <div class="summary-section pa-3 rounded border mb-3">
              <strong class="text-subtitle-2 text-teal d-block mb-2">3. 核心领域与兴趣图谱分布</strong>
              <div v-for="dom in batchSummary.domains" :key="dom.name" class="mb-2">
                <div class="d-flex justify-space-between align-center text-caption mb-1">
                  <strong>{{ dom.name }}</strong>
                  <span class="text-teal font-weight-bold">{{ dom.count }}人涉足 · 平均投入度 {{ dom.avgScore }}%</span>
                </div>
                <v-progress-linear :model-value="dom.avgScore" color="teal" height="6" rounded />
              </div>
            </div>
          </div>
        </v-card-text>

        <v-divider />
        <v-card-actions class="pa-4">
          <v-btn v-if="batchSummary" variant="tonal" color="secondary" @click="batchSummary = null">
            查看实时日志
          </v-btn>
          <v-spacer />
          <v-btn v-if="!batchRunning && !batchSummary" variant="text" @click="batchPersonaDialog = false">
            取消
          </v-btn>
          <v-btn v-if="!batchRunning && !batchSummary" color="primary" prepend-icon="mdi-play" @click="startBatchPersona">
            开始批量研判
          </v-btn>
          <v-btn v-if="batchRunning" color="error" variant="tonal" prepend-icon="mdi-stop" @click="stopBatchPersona">
            中断任务
          </v-btn>
          <v-btn v-if="batchCompleted && !batchSummary" color="primary" prepend-icon="mdi-file-document-outline" @click="generateBatchSummary">
            查看任务画像总结
          </v-btn>
        </v-card-actions>
      </v-card>
    </v-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue";
import { api } from "@/services/api";
import AuthImage from "@/components/AuthImage.vue";
import { useRoute, useRouter } from "vue-router";
import { useGraphWorkspaceStore } from "@/stores/graphWorkspace";
import { useListStateStore } from "@/stores/listState";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";
import { cleanDisplayText } from "@/utils/text";
import { qqOf } from "@/utils/identity";

const PAGE_KEY = "persons_page_state";
const listState = useListStateStore();
const systemCapabilities = useSystemCapabilitiesStore();
const router = useRouter();
const route = useRoute();
const graphWorkspace = useGraphWorkspaceStore();

// 列表查询与分页状态 (保持持久化)
const savedState = listState.load(PAGE_KEY, {});
const query = ref(String(savedState.query || ""));
const page = ref(Number(savedState.page) > 0 ? Number(savedState.page) : 1);
const pageSize = ref(Number(savedState.pageSize) > 0 ? Number(savedState.pageSize) : 20);
const pageSizeOptions = [
  { value: 10, title: "10" },
  { value: 20, title: "20" },
  { value: 50, title: "50" },
  { value: 100, title: "100" },
];

const persons = ref<any[]>([]);
const total = ref(0);
const loading = ref(false);
const pageError = ref("");
const drawerError = ref("");

// 抽屉详情状态
const drawer = ref(false);
const detail = ref<any>(null);
const detailLoading = ref(false);
const tab = ref("profile");
const profileSyncing = ref(false);

const coverage = ref<any[]>([]);
const coverageLoading = ref(false);
const collecting = ref<string>("");
const timeline = ref<any[]>([]);
const timelineLoading = ref(false);
const feeds = ref<any[]>([]);
const feedsLoading = ref(false);
const persona = ref<any>(null);
const personaLoading = ref(false);
const feedDynamics = ref<any>(null);
const feedDynamicsLoading = ref(false);

const headers = [
  { title: "用户", key: "display_name" },
  { title: "首次发现", key: "first_seen_at", width: 170 },
  { title: "最后发现", key: "last_seen_at", width: 170 },
  { title: "", key: "actions", sortable: false, width: 52 },
];

const detailQQ = computed(() => {
  if (!detail.value) return "";
  return qqOf(detail.value);
});

async function load() {
  loading.value = true;
  pageError.value = "";
  try {
    const size = pageSize.value > 0 ? pageSize.value : 20;
    const offset = Math.max(0, (page.value - 1) * size);
    const result = (
      await api.get("/api/v1/persons", {
        params: {
          q: query.value,
          limit: size,
          offset: offset,
        },
      })
    ).data;
    persons.value = Array.isArray(result.data) ? result.data : [];
    total.value = Number(result.total ?? persons.value.length);
  } catch (e: any) {
    pageError.value = e.response?.data?.error || "用户数据加载失败";
    persons.value = [];
  } finally {
    loading.value = false;
  }
  persistState();
}

function persistState() {
  listState.save(PAGE_KEY, {
    query: query.value,
    page: page.value,
    pageSize: pageSize.value,
  });
}

function onPageChange(newPage: number) {
  page.value = newPage;
  persistState();
  load();
}

function onPageSizeChange(newSize: number) {
  pageSize.value = newSize;
  page.value = 1;
  persistState();
  load();
}

function resetAndLoad() {
  page.value = 1;
  load();
}

function researchPerson() {
  const qq = detailQQ.value;
  if (!qq) return;
  graphWorkspace.targetQQ = qq;
  router.push({ path: "/ego", query: { target: qq } });
}

function openGroup(groupId: string) {
  if (!groupId) return;
  router.push({ path: "/groups", query: { q: groupId } });
}

function openMessage(message: any) {
  if (!message) return;
  router.push({ path: "/messages", query: { message_id: message.id, group_id: message.conversation_id } });
}

async function openPersonDirect(item: any) {
  if (!item?.id) return;
  await fetchAndOpenPerson(item.id);
}

async function openPersonRow(_event: unknown, row: any) {
  const targetId = row?.item?.id || row?.id;
  if (!targetId) return;
  await fetchAndOpenPerson(targetId);
}

async function fetchAndOpenPerson(id: string) {
  drawer.value = true;
  detailLoading.value = true;
  drawerError.value = "";
  tab.value = "profile";
  coverage.value = [];
  timeline.value = [];
  feeds.value = [];
  persona.value = null;
  feedDynamics.value = null;
  try {
    const res = (await api.get(`/api/v1/persons/${id}`)).data?.data;
    if (!res) throw new Error("未获取到有效用户数据");
    detail.value = res;
    feeds.value = Array.isArray(res.contents) ? res.contents : [];
    // Instant prefetch from DB cache in background without blocking UI
    loadPersona(false);
    loadFeedDynamics(false);
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || e.message || "用户详情加载失败";
  } finally {
    detailLoading.value = false;
  }
}

async function syncProfile() {
  if (!detail.value?.id) return;
  profileSyncing.value = true;
  drawerError.value = "";
  try {
    await api.post(`/api/v1/persons/${detail.value.id}/sync`, {});
    detail.value = (
      await api.get(`/api/v1/persons/${detail.value.id}`)
    ).data?.data;
    await load();
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "详细资料获取失败";
  } finally {
    profileSyncing.value = false;
  }
}

async function onTabChange(value: string) {
  if (!detail.value?.id) return;
  if (value === "coverage" && !coverage.value.length) await loadCoverage();
  if (value === "timeline" && !timeline.value.length) await loadTimeline();
  if (value === "feeds" && !feeds.value.length) await loadFeeds();
  if (value === "persona" && !persona.value) await loadPersona();
  if (value === "feed_dynamics" && !feedDynamics.value) await loadFeedDynamics();
}

async function loadPersona(force = false) {
  if (!detail.value?.id) return;
  personaLoading.value = true;
  drawerError.value = "";
  try {
    const res = force
      ? (await api.post(`/api/v1/persons/${detail.value.id}/analyze-persona`)).data?.data
      : (await api.get(`/api/v1/persons/${detail.value.id}/persona`)).data?.data;
    persona.value = res;
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "全息画像分析失败";
  } finally {
    personaLoading.value = false;
  }
}

async function loadFeedDynamics(force = false) {
  if (!detail.value?.id) return;
  feedDynamicsLoading.value = true;
  drawerError.value = "";
  try {
    const res = force
      ? (await api.post(`/api/v1/persons/${detail.value.id}/analyze-feeds`)).data?.data
      : (await api.get(`/api/v1/persons/${detail.value.id}/feed-dynamics`)).data?.data;
    feedDynamics.value = res;
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "空间研判分析失败";
  } finally {
    feedDynamicsLoading.value = false;
  }
}

// 批量全息画像研判任务调度系统
const batchPersonaDialog = ref(false);
const batchRunning = ref(false);
const batchCompleted = ref(false);
const batchScope = ref("with_messages");
const batchConcurrency = ref(3);
const batchAutoRetry = ref(true);
const batchMaxRetries = ref(3);
const batchRetryBackoffSeconds = ref(15);
const batchQueueWaitSeconds = ref(180);
const batchForceAnalyze = ref(false);
const batchTotalCount = ref(0);
const batchSuccessCount = ref(0);
const batchFailedCount = ref(0);
const batchPendingCount = ref(0);
const batchCurrentTarget = ref<any>(null);
const batchLogs = ref<Array<{ time: string; message: string; type: "info" | "success" | "warn" | "error" }>>([]);
const batchSummary = ref<any>(null);
const batchResults = ref<any[]>([]);
let batchCancelled = false;

const batchScopeOptions = computed(() => [
  { title: "全库有发言记录的人物（动态全量）", value: "with_messages" },
  { title: "全库所有人物（动态全量）", value: "all_persons" },
  { title: `当前页目标 (${persons.value.length} 人)`, value: "current_page" },
]);

const batchProgressPercent = computed(() => {
  if (!batchTotalCount.value) return 0;
  return Math.min(100, Math.round(((batchSuccessCount.value + batchFailedCount.value) / batchTotalCount.value) * 100));
});

function addBatchLog(message: string, type: "info" | "success" | "warn" | "error" = "info") {
  const time = new Date().toLocaleTimeString("zh-CN", { hour12: false });
  batchLogs.value.unshift({ time, message, type });
  if (batchLogs.value.length > 200) batchLogs.value.pop();
}

function openBatchPersonaDialog() {
  router.push({ path: "/persona-pipelines", query: { create: "1" } });
}

async function startBatchPersona() {
  batchRunning.value = true;
  batchCompleted.value = false;
  batchCancelled = false;
  batchSummary.value = null;
  batchLogs.value = [];
  batchResults.value = [];
  batchSuccessCount.value = 0;
  batchFailedCount.value = 0;

  addBatchLog("🚀 正在向后端任务调度中心下发批量画像流水线...", "info");

  try {
    let payload: any = {
      title: `批量全息画像研判流水线_${new Date().toLocaleTimeString("zh-CN", { hour12: false })}`,
      scope_type: batchScope.value,
      concurrency: batchConcurrency.value,
      auto_retry: batchAutoRetry.value,
      max_retries: batchMaxRetries.value,
      retry_backoff_seconds: batchRetryBackoffSeconds.value,
      queue_wait_seconds: batchQueueWaitSeconds.value,
      force_analyze: batchForceAnalyze.value,
    };

    if (batchScope.value === "current_page") {
      payload.scope_type = "custom_qqs";
      payload.person_ids = persons.value.map((p) => p.id);
    }

    const res = (await api.post("/api/v1/pipelines/batch-persona", payload)).data;
    activePipelineId.value = res.pipeline_id;
    addBatchLog(`✅ 后端流水线任务已创建并启动 (Pipeline ID: ${res.pipeline_id.slice(0, 8)}...)`, "success");

    startPollingPipeline(res.pipeline_id);
  } catch (e: any) {
    addBatchLog(`❌ 下发流水线任务失败: ${e.response?.data?.error || e.message}`, "error");
    batchRunning.value = false;
  }
}

const activePipelineId = ref("");
let pollInterval: any = null;

function startPollingPipeline(pipelineId: string) {
  if (pollInterval) clearInterval(pollInterval);
  const seenStatusMap = new Map<string, string>();

  pollInterval = setInterval(async () => {
    try {
      const res = (await api.get(`/api/v1/pipelines/batch-persona/${pipelineId}`)).data;
      const pipeline = res.data;
      const items = res.items || [];

      batchTotalCount.value = pipeline.total_targets;
      batchSuccessCount.value = pipeline.completed_targets;
      batchFailedCount.value = pipeline.failed_targets;
      batchPendingCount.value = Math.max(0, pipeline.total_targets - pipeline.completed_targets - pipeline.failed_targets);

      // Track active processing item
      const activeItem = items.find((it: any) => it.status === "processing");
      if (activeItem) {
        batchCurrentTarget.value = {
          display_name: activeItem.display_name,
          platform_user_id: activeItem.qq,
        };
      } else {
        batchCurrentTarget.value = null;
      }

      // Log status changes
      for (const it of items) {
        const lastStatus = seenStatusMap.get(it.id);
        if (lastStatus !== it.status) {
          seenStatusMap.set(it.id, it.status);
          const name = it.display_name || it.qq || it.person_id.slice(0, 8);
          if (it.status === "processing") {
            addBatchLog(`🔍 开始深度研判目标: ${name}...`, "info");
          } else if (it.status === "completed") {
            addBatchLog(`✅ 目标 ${name} 研判成功 (重试次数: ${it.attempts})`, "success");
          } else if (it.status === "failed") {
            addBatchLog(`❌ 目标 ${name} 研判失败: ${it.error_message || "未知错误"}`, "error");
          }
        }
      }

      // Check final pipeline state
      if (["completed", "partial", "failed", "cancelled"].includes(pipeline.status)) {
        clearInterval(pollInterval);
        pollInterval = null;
        batchRunning.value = false;
        batchCompleted.value = true;

        if (pipeline.status === "cancelled") {
          addBatchLog("🛑 批量研判流水线已在后台成功中止", "warn");
        } else {
          addBatchLog(`🎉 批量画像流水线执行完毕！状态: ${pipeline.status} · 成功: ${pipeline.completed_targets} · 失败: ${pipeline.failed_targets}`, "success");
          await fetchBackendTaskSummary(pipelineId);
        }
      }
    } catch (err: any) {
      console.warn("Poll pipeline error:", err);
    }
  }, 1500);
}

async function stopBatchPersona() {
  if (!activePipelineId.value) return;
  try {
    await api.post(`/api/v1/pipelines/batch-persona/${activePipelineId.value}/cancel`);
    addBatchLog("🛑 已向后端发送中止指令...", "warn");
  } catch (err: any) {
    addBatchLog(`中止指令发送失败: ${err.message}`, "error");
  }
}

async function fetchBackendTaskSummary(pipelineId: string) {
  try {
    const res = (await api.get(`/api/v1/pipelines/batch-persona/${pipelineId}/summary`)).data?.data;
    if (res && res.total_persons > 0) {
      batchSummary.value = {
        total: res.total_persons,
        roles: (res.roles || []).map((r: any) => ({ name: r.name, count: r.count, percent: r.percent })),
        catchphrases: res.catchphrases || [],
        slang: res.slang || [],
        domains: (res.domains || []).map((d: any) => ({ name: d.name, count: d.count, avgScore: d.avg_score })),
      };
    }
  } catch (e: any) {
    console.error("fetch summary error:", e);
  }
}

function generateBatchSummary() {
  if (activePipelineId.value) {
    fetchBackendTaskSummary(activePipelineId.value);
  }
}

function exportBatchSummary() {
  if (!batchSummary.value) return;
  const s = batchSummary.value;
  let md = `# 全员全息画像研判总结与社群洞察报告\n\n`;
  md += `- **研判样本规模**: ${s.total} 人\n`;
  md += `- **生成时间**: ${new Date().toLocaleString("zh-CN")}\n\n`;

  md += `## 1. 核心社群角色与心理防御分布\n`;
  for (const r of s.roles) {
    md += `- **${r.name}**: ${r.count} 人 (${r.percent}%)\n`;
  }

  md += `\n## 2. 全员 TOP 高频口癖与黑话暗语\n`;
  md += `**高频口癖**: ` + s.catchphrases.map((c: any) => `${c.phrase} (${c.count}次)`).join(", ") + `\n\n`;
  md += `**圈子黑话**: ` + s.slang.map((c: any) => `${c.phrase} (${c.count}次)`).join(", ") + `\n\n`;

  md += `## 3. 核心领域与兴趣图谱\n`;
  for (const d of s.domains) {
    md += `- **${d.name}**: ${d.count} 人涉足 (平均投入度 ${d.avgScore}%)\n`;
  }

  const blob = new Blob([md], { type: "text/markdown;charset=utf-8" });
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `批量全息画像研判总结_${Date.now()}.md`;
  a.click();
  URL.revokeObjectURL(url);
}

function openPersonByQQ(qq: string) {
  if (!qq) return;
  query.value = qq;
  resetAndLoad();
}

async function loadCoverage() {
  if (!detail.value?.id) return;
  coverageLoading.value = true;
  try {
    coverage.value = (
      await api.get(`/api/v1/persons/${detail.value.id}/coverage`)
    ).data?.data || [];
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "采集覆盖加载失败";
  } finally {
    coverageLoading.value = false;
  }
}

async function collectSource(source: string) {
  if (!detail.value?.id) return;
  collecting.value = source;
  try {
    await api.post(`/api/v1/persons/${detail.value.id}/collect`, {
      data_source: source,
    });
    await loadCoverage();
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "采集请求失败";
  } finally {
    collecting.value = "";
  }
}

async function loadTimeline() {
  if (!detail.value?.id) return;
  timelineLoading.value = true;
  try {
    timeline.value = (
      await api.get(`/api/v1/persons/${detail.value.id}/timeline`, {
        params: { limit: systemCapabilities.data?.lists.default_page_size || 50 },
      })
    ).data?.data || [];
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "时间线加载失败";
  } finally {
    timelineLoading.value = false;
  }
}

async function loadFeeds() {
  if (!detail.value) return;
  feedsLoading.value = true;
  try {
    const qq = detailQQ.value;
    const result = (
      await api.get("/api/v1/contents", {
        params: { author: qq || detail.value.display_name, limit: 50 },
      })
    ).data;
    const items = Array.isArray(result?.data) ? result.data : [];
    feeds.value = items.length ? items : (detail.value.contents || []);
  } catch (e: any) {
    drawerError.value = e.response?.data?.error || "空间动态加载失败";
  } finally {
    feedsLoading.value = false;
  }
}

function goToFeed(id: string) {
  if (!id) return;
  void router.push({ path: "/contents", query: { content_id: id } });
}

function openTimelineEvent(event: any) {
  if (!event) return;
  const targetId = event.target_object_id || event.source_id || event.id;
  if (!targetId) return;
  const type = String(event.event_type || '').toLowerCase();
  if (type.includes('message') || type.includes('chat')) {
    void router.push({ path: '/messages', query: { message_id: targetId } });
  } else if (type.includes('feed') || type.includes('comment') || type.includes('qzone') || type.includes('post') || type.includes('content') || type.includes('like')) {
    void router.push({ path: '/contents', query: { content_id: targetId } });
  }
}

// 提取个性签名
const userSignature = computed(() => {
  const profiles = detail.value?.profiles || [];
  for (const p of profiles) {
    const sig = p.signature || p.fields?.signature || p.fields?.long_nick || p.fields?.sign || p.structured?.signature;
    if (sig && sig !== "未设置" && sig !== "未知" && sig !== "-") {
      return String(sig).trim();
    }
  }
  return "";
});

// 其他平台标识 (排除已在顶部显示的 QQ)
const extraIdentifiers = computed(() => {
  const list = detail.value?.identifiers || [];
  return list.filter((i: any) => i.platform?.toLowerCase() !== "qq");
});

// 提取并清洗结构化资料 (彻底过滤 0 / unknown / 未知等噪声数据)
const profileFields = computed(() => {
  const profiles = detail.value?.profiles || [];
  const merged: Record<string, any> = {};

  // 从后往前合并，保证取到最准确的非空数据
  for (const p of [...profiles].reverse()) {
    const src = { ...(p.fields || {}), ...(p.structured || {}) };
    for (const [k, v] of Object.entries(src)) {
      if (v !== null && v !== undefined && v !== "") {
        merged[k] = v;
      }
    }
    if (p.sex) merged.sex = p.sex;
    if (p.age) merged.age = p.age;
    if (p.area) merged.area = p.area;
    if (p.reg_time) merged.reg_time = p.reg_time;
    if (p.login_days) merged.login_days = p.login_days;
  }

  const fieldDefs: { key: string; label: string; format: (v: any) => string | null }[] = [
    {
      key: "gender",
      label: "性别",
      format: (v) => {
        const s = String(v).toLowerCase().trim();
        if (s === "male" || s === "1" || s === "男" || s === "m") return "男";
        if (s === "female" || s === "2" || s === "女" || s === "f") return "女";
        return null; // 过滤 unknown / 0 / 未知
      }
    },
    {
      key: "sex",
      label: "性别",
      format: (v) => {
        const s = String(v).toLowerCase().trim();
        if (s === "male" || s === "1" || s === "男" || s === "m") return "男";
        if (s === "female" || s === "2" || s === "女" || s === "f") return "女";
        return null;
      }
    },
    {
      key: "age",
      label: "年龄",
      format: (v) => {
        const n = Number(v);
        return n > 0 && n < 120 ? `${n} 岁` : null; // 过滤 0
      }
    },
    {
      key: "qzone_age",
      label: "Q龄",
      format: (v) => {
        const n = Number(v);
        return n > 0 ? `${n} 年` : (v && v !== "0" ? String(v) : null);
      }
    },
    {
      key: "qq_level",
      label: "QQ等级",
      format: (v) => {
        const n = Number(v);
        return n > 0 ? `Lv.${n}` : null; // 过滤 0
      }
    },
    {
      key: "qqLevel",
      label: "QQ等级",
      format: (v) => {
        const n = Number(v);
        return n > 0 ? `Lv.${n}` : null;
      }
    },
    {
      key: "level",
      label: "等级",
      format: (v) => {
        const n = Number(v);
        return n > 0 ? `Lv.${n}` : null;
      }
    },
    {
      key: "area",
      label: "所在地",
      format: (v) => (v && v !== "未知" && v !== "-" ? String(v) : null)
    },
    {
      key: "location",
      label: "所在地",
      format: (v) => (v && v !== "未知" && v !== "-" ? String(v) : null)
    },
    {
      key: "hometown",
      label: "故乡",
      format: (v) => (v && v !== "未知" && v !== "-" ? String(v) : null)
    },
    {
      key: "reg_time",
      label: "注册时间",
      format: (v) => {
        const n = Number(v);
        return n > 100000000 ? formatDate(new Date(n * 1000).toISOString()) : null;
      }
    },
    {
      key: "login_days",
      label: "活跃天数",
      format: (v) => {
        const n = Number(v);
        return n > 0 ? `${n} 天` : null;
      }
    },
    {
      key: "birthday",
      label: "生日",
      format: (v) => (v && v !== "0" && v !== "0-0-0" && v !== "-" ? String(v) : null)
    },
    {
      key: "school",
      label: "学校",
      format: (v) => (v && v !== "未知" && v !== "-" ? String(v) : null)
    },
    {
      key: "company",
      label: "公司",
      format: (v) => (v && v !== "未知" && v !== "-" ? String(v) : null)
    },
    {
      key: "email",
      label: "邮箱",
      format: (v) => (v && v !== "-" ? String(v) : null)
    },
    {
      key: "phone",
      label: "电话",
      format: (v) => (v && v !== "-" ? String(v) : null)
    },
    {
      key: "phone_num",
      label: "电话",
      format: (v) => (v && v !== "-" ? String(v) : null)
    },
    {
      key: "is_vip",
      label: "QQ VIP",
      format: (v) => (v && v !== "0" && v !== 0 && v !== false ? "VIP 用户" : null)
    },
  ];

  const results: { key: string; label: string; value: string }[] = [];
  const seenLabels = new Set<string>();

  for (const def of fieldDefs) {
    if (seenLabels.has(def.label)) continue;
    if (merged[def.key] !== undefined) {
      const formatted = def.format(merged[def.key]);
      if (formatted) {
        results.push({
          key: def.key,
          label: def.label,
          value: formatted,
        });
        seenLabels.add(def.label);
      }
    }
  }

  return results;
});

// 头像版本历史 (去重)
const avatarHistory = computed(() => {
  const list = (detail.value?.profiles || []).filter((p: any) => p.avatar_uri && p.avatar_uri.trim());
  const deduped: any[] = [];
  const seen = new Set<string>();
  for (const p of list) {
    if (!seen.has(p.avatar_uri)) {
      seen.add(p.avatar_uri);
      deduped.push(p);
    }
  }
  return deduped;
});

// 昵称与名片历史 (去重：相同昵称且无新备注时不重复刷屏)
const nicknameHistory = computed(() => {
  const list = detail.value?.profiles || [];
  const deduped: any[] = [];
  const seen = new Set<string>();
  for (const p of list) {
    const nick = cleanDisplayText(p.nickname || "");
    const card = cleanDisplayText(p.card_or_remark || "");
    if (!nick && !card) continue;
    const key = `${nick}__${card}`;
    if (!seen.has(key)) {
      seen.add(key);
      deduped.push(p);
    }
  }
  return deduped;
});

function eventDetails(event: any): string {
  const d = event.details || {};
  if (event.event_type === "sent_message")
    return d.raw_text || d.source_message_id || "";
  if (event.event_type === "profile_change")
    return [d.nickname, d.source].filter(Boolean).join(" · ");
  if (event.event_type === "group_membership")
    return d.role ? `角色 ${d.role}` : "";
  if (d.context_type) return d.context_type;
  return "";
}

const initials = (name?: string) =>
  String(name || "?")
    .trim()
    .slice(0, 1)
    .toUpperCase();

const detailAvatar = (value: any) =>
  value?.profiles?.find((p: any) => p.avatar_uri)?.avatar_uri ||
  value?.avatar_uri ||
  value?.avatar_url ||
  "";

const avatarFallbacks = (qq?: string | number) => {
  if (!qq) return [];
  const s = String(qq).trim();
  if (!s || s === "未知" || s === "未记录 QQ") return [];
  return [
    `/api/v1/media/avatars/person/${encodeURIComponent(s)}`,
    `https://q1.qlogo.cn/g?b=qq&nk=${encodeURIComponent(s)}&s=640`,
    `https://q.qlogo.cn/headimg_dl?dst_uin=${encodeURIComponent(s)}&spec=640`,
  ];
};

const formatDate = (value?: string | null) => {
  if (!value) return "—";
  try {
    const d = new Date(value);
    if (isNaN(d.getTime())) return "—";
    return new Intl.DateTimeFormat("zh-CN", {
      dateStyle: "medium",
      timeStyle: "short",
    }).format(d);
  } catch {
    return "—";
  }
};

const roleLabel = (value: string) =>
  ({ owner: "群主", admin: "管理员", member: "成员" })[value] ||
  value ||
  "成员";

const relationLabel = (value: string) =>
  ({
    member_of: "共同在群",
    co_member: "共同在群",
    group_membership: "群成员变更",
    sent_message: "发送消息",
    message: "发送消息",
    friend_visible: "好友可见",
    commented: "说说评论",
    qzone_comment: "说说评论",
    feed_reply: "说说回复",
    replied_to: "引用回复",
    quote_reply: "引用回复",
    mentioned: "@提及",
    at_message: "@提及",
    liked: "空间点赞",
    qzone_like: "空间点赞",
    published: "发布说说",
    published_feed: "发布说说",
    visited: "空间访问",
  })[value] || value;

const eventLabel = (value: string) =>
  ({
    sent_message: "发送消息",
    message: "发送消息",
    profile_change: "资料变更",
    group_membership: "群成员变更",
    member_of: "加入群聊",
    commented: "说说评论",
    qzone_comment: "说说评论",
    feed_reply: "说说回复",
    replied_to: "引用回复",
    quote_reply: "引用回复",
    mentioned: "@提及",
    at_message: "@提及",
    liked: "空间点赞",
    qzone_like: "空间点赞",
    published: "发布说说",
    published_feed: "发布说说",
    visited: "空间访问",
    friend_visible: "好友可见",
  })[value] || value;

const coverageLabel = (value: string) =>
  ({
    not_collected: "未采集",
    collecting: "采集中",
    partial: "部分采集",
    complete: "已采集",
    no_results: "无结果",
    failed: "失败",
    inferred: "已推断",
  })[value] || value;

const coverageColor = (value: string) =>
  ({
    not_collected: "default",
    collecting: "info",
    partial: "warning",
    complete: "success",
    no_results: "default",
    failed: "error",
    inferred: "info",
  })[value] || "default";

onMounted(async () => {
  try {
    await systemCapabilities.load();
    if (!pageSize.value || pageSize.value <= 0) {
      pageSize.value = systemCapabilities.data?.lists.default_page_size || 20;
    }
    const initialQ = String(route.query.q || route.query.query || "");
    if (initialQ) {
      query.value = initialQ;
      page.value = 1;
    }
    await load();
    if (initialQ && persons.value.length) {
      const match = persons.value.find((p) => qqOf(p) === initialQ || p.display_name === initialQ) || persons.value[0];
      if (match?.id) {
        await fetchAndOpenPerson(match.id);
      }
    }
    if (route.query.batch === "1" || route.query.batch === "true") {
      router.replace({ path: "/persona-pipelines", query: { create: "1" } });
    }
  } catch (err: any) {
    pageError.value = err.message || "页面初始化失败";
  }
});
</script>

<style scoped>
.id-avatar {
  flex-shrink: 0;
}
.person-hero-card {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 14px;
  background: rgba(var(--v-theme-surface), 0.8);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
}
.hero-avatar {
  flex-shrink: 0;
}
.hero-body {
  flex-grow: 1;
  min-width: 0;
}
.hero-actions {
  display: flex;
  flex-direction: column;
  gap: 6px;
  flex-shrink: 0;
}
.signature-card {
  display: flex;
  align-items: flex-start;
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(var(--v-theme-primary), 0.05);
  border-left: 3px solid rgb(var(--v-theme-primary));
}
.profile-info-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px 12px;
  padding: 10px 12px;
  background: rgba(var(--v-theme-surface), 0.6);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.06);
  border-radius: 8px;
}
.info-grid-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12.5px;
  padding: 4px 0;
}
.info-label {
  color: rgba(var(--v-theme-on-surface), 0.55);
}
.info-value {
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.9);
}
.coverage-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  grid-template-rows: auto auto;
  gap: 4px 10px;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.055);
}
.coverage-row > .v-chip {
  grid-row: 1;
  grid-column: 2;
  justify-self: end;
}
.coverage-row > .v-btn {
  grid-row: 2;
  grid-column: 2;
  justify-self: end;
}
.coverage-info {
  display: flex;
  flex-direction: column;
}
.coverage-info strong {
  font-size: 13px;
  font-weight: 600;
}
.coverage-info small {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.coverage-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: rgba(var(--v-theme-on-surface), 0.7);
}
.coverage-error {
  color: rgb(var(--v-theme-error));
}
.group-card-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 0;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.055);
}
.group-card-row strong {
  display: block;
  font-size: 13px;
}
.group-card-row small {
  display: block;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.message-row {
  padding: 10px 0;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.055);
}
.message-row p {
  margin: 0 0 4px;
  font-size: 13px;
  line-height: 1.45;
}
.message-row small {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.avatar-history {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.avatar-history-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
}
.avatar-history-item small {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.timeline-row {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 8px 0;
  position: relative;
}
.timeline-row i {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: rgb(var(--v-theme-primary));
  margin-top: 6px;
  flex-shrink: 0;
}
.timeline-row div {
  flex-grow: 1;
}
.timeline-row strong {
  display: block;
  font-size: 13px;
}
.timeline-row small {
  display: block;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.key-value-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.055);
  font-size: 13px;
}
.key-value-row span {
  color: rgba(var(--v-theme-on-surface), 0.6);
}
.key-value-row strong {
  font-weight: 500;
  text-align: right;
  max-width: 65%;
  overflow-wrap: anywhere;
}
.detail-drawer {
  padding: 16px;
  height: 100%;
  overflow-y: auto;
}
.detail-tabs {
  margin-bottom: 16px;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}
.detail-section {
  margin-bottom: 20px;
}
.detail-section h3 {
  font-size: 13.5px;
  font-weight: 600;
  margin-bottom: 8px;
  color: rgb(var(--v-theme-primary));
}
.detail-section.flush {
  margin-bottom: 0;
}
.compact-empty {
  padding: 24px 0;
  text-align: center;
  color: rgba(var(--v-theme-on-surface), 0.5);
  font-size: 13px;
}
.person-feed-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.person-feed-item {
  padding: 10px 12px;
  border-radius: 8px;
  background: rgba(var(--v-theme-surface), 0.7);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  cursor: pointer;
  transition: all 0.2s ease;
}
.person-feed-item:hover {
  background: rgba(var(--v-theme-primary), 0.04);
  border-color: rgba(var(--v-theme-primary), 0.3);
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
  font-size: 13px;
  line-height: 1.55;
  color: rgba(var(--v-theme-on-surface), 0.88);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}
.person-feed-share {
  display: flex;
  align-items: center;
  padding: 6px 10px;
  border-radius: 4px;
  background: rgba(var(--v-theme-primary), 0.06);
  font-size: 11.5px;
  color: rgb(var(--v-theme-primary));
  font-weight: 500;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.person-feed-foot {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 4px;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.5);
}
.persona-card {
  padding: 14px 16px;
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--card-border);
  border-radius: 8px;
  box-shadow: 0 2px 10px rgba(0, 0, 0, 0.04);
}
.persona-card-title {
  display: flex;
  align-items: center;
  font-weight: 700;
  font-size: 13.5px;
  margin-bottom: 10px;
  color: rgb(var(--v-theme-primary));
}
.anchor-item {
  background: rgba(var(--v-theme-on-surface), 0.02);
  border: 1px solid var(--card-border-subtle);
  border-left: 3px solid rgb(var(--v-theme-primary));
  border-radius: 6px;
  padding: 10px 12px;
}
.quote-card {
  background: rgba(var(--v-theme-primary), 0.05);
  border: 1px solid rgba(var(--v-theme-primary), 0.2) !important;
  border-radius: 8px;
  padding: 10px 12px;
}
.verbatim-evidence-box {
  margin-top: 6px;
  padding: 6px 10px;
  background: rgba(var(--v-theme-primary), 0.08);
  border-radius: 6px;
  border-left: 2px solid rgb(var(--v-theme-primary));
  font-size: 12px;
  color: var(--text-main);
}
.evidence-text {
  font-weight: 500;
  color: var(--text-main);
}
.defense-block {
  background: rgba(var(--v-theme-surface-variant), 0.35);
  border: 1px solid var(--card-border-subtle);
}
.interest-domain-card {
  background: rgba(var(--v-theme-surface-variant), 0.35);
  border: 1px solid var(--card-border-subtle);
}
.batch-hud-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}
.batch-hud-card {
  padding: 10px;
  border-radius: 8px;
  background: rgba(var(--v-theme-surface-variant), 0.35);
  border: 1px solid var(--card-border-subtle);
  display: flex;
  flex-direction: column;
  align-items: center;
  text-align: center;
}
.batch-log-console {
  background: var(--bg-app);
}
.log-stream-box {
  max-height: 200px;
  overflow-y: auto;
  font-family: monospace;
  font-size: 11.5px;
  display: flex;
  flex-direction: column;
  gap: 3px;
}
.log-line {
  display: flex;
  gap: 8px;
  line-height: 1.45;
}
.log-time {
  color: var(--text-dim);
  flex-shrink: 0;
}
.log-line.log-info { color: var(--text-main); }
.log-line.log-success { color: #10b981; }
.log-line.log-warn { color: #f59e0b; }
.log-line.log-error { color: #ef4444; }
.current-target-card {
  background: rgba(var(--v-theme-primary), 0.05);
  border-color: rgba(var(--v-theme-primary), 0.2) !important;
}
.summary-section {
  background: rgb(var(--v-theme-surface));
  border-color: var(--card-border) !important;
}
</style>

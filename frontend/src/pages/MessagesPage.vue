<template>
  <div class="qq-chat-app">
    <!-- 1. 左侧：会话导航列表 -->
    <aside class="chat-sidebar data-surface">
      <!-- 顶部：NapCat 账号切换器 -->
      <div class="account-switcher-bar">
        <v-menu location="bottom start">
          <template #activator="{ props }">
            <div v-bind="props" class="active-account-chip" title="点击切换 NapCat 操作账号">
              <v-avatar size="24" class="mr-2">
                <AuthImage :src="avatarFallback(activeAccount?.qq_uin)" :alt="activeAccount?.name || 'NapCat 账号'">
                  <span>{{ (activeAccount?.name || 'Q').slice(0, 1) }}</span>
                </AuthImage>
              </v-avatar>
              <div class="account-name-box">
                <strong class="account-name">{{ activeAccount?.name || '未选择账号' }}</strong>
                <small class="account-qq">QQ: {{ activeAccount?.qq_uin || '—' }}</small>
              </div>
              <v-icon icon="mdi-chevron-down" size="16" class="account-arrow" />
            </div>
          </template>
          <v-list density="compact" min-width="220" class="account-dropdown-menu">
            <v-list-subheader>切换 NapCat 操作账号 (共 {{ napcatAccounts.length }} 个)</v-list-subheader>
            <v-list-item
              v-for="acc in napcatAccounts"
              :key="acc.id"
              :active="activeAccount?.id === acc.id"
              :title="acc.name || `QQ ${acc.qq_uin}`"
              :subtitle="`QQ: ${acc.qq_uin} · ${acc.enabled ? '已启用' : '已停用'}`"
              @click="switchAccount(acc)"
            >
              <template #prepend>
                <v-avatar size="28" class="mr-2">
                  <AuthImage :src="avatarFallback(acc.qq_uin)" :alt="acc.name">
                    <span>{{ (acc.name || 'Q').slice(0, 1) }}</span>
                  </AuthImage>
                </v-avatar>
              </template>
              <template #append>
                <v-icon v-if="activeAccount?.id === acc.id" icon="mdi-check" color="primary" size="18" />
              </template>
            </v-list-item>
          </v-list>
        </v-menu>
      </div>

      <!-- 搜索与类型过滤 -->
      <div class="sidebar-top">
        <v-text-field
          v-model.trim="searchQuery"
          placeholder="搜索群聊、好友或记录..."
          prepend-inner-icon="mdi-magnify"
          density="compact"
          variant="solo-filled"
          hide-details
          clearable
          class="sidebar-search-input mb-2"
          @update:model-value="onSearchChange"
        />
        <div class="sidebar-filters">
          <v-btn-toggle
            v-model="activeTypeFilter"
            mandatory
            density="compact"
            color="primary"
            variant="outlined"
            class="type-toggle"
          >
            <v-btn value="" size="x-small">全部</v-btn>
            <v-btn value="group" size="x-small">群聊 ({{ groupCount }})</v-btn>
            <v-btn value="private" size="x-small">私聊 ({{ privateCount }})</v-btn>
          </v-btn-toggle>
          <div class="sync-status-indicator" :title="autoSync ? '实时长轮询同步中' : '同步已暂停'">
            <i class="sync-dot" :class="{ active: autoSync }" />
            <span>{{ autoSync ? '实时同步' : '离线' }}</span>
          </div>
        </div>
      </div>

      <v-divider />

      <!-- 会话滚动列表 -->
      <div class="conversation-scroll-list" :class="{ 'is-loading': conversationsLoading && !conversations.length }">
        <div
          v-for="conv in conversations"
          :key="conv.id"
          class="conversation-card"
          :class="{ active: currentConv?.id === conv.id }"
          @click="selectConversation(conv)"
        >
          <div class="conv-avatar">
            <AuthImage :src="conv.avatar_uri" :alt="conv.title">
              <span class="avatar-letter">{{ (conv.title || '?').slice(0, 1) }}</span>
            </AuthImage>
            <span class="conv-badge" :class="conv.conversation_type">
              {{ conv.conversation_type === 'group' ? '群' : '友' }}
            </span>
          </div>

          <div class="conv-body">
            <div class="conv-head">
              <strong class="conv-name" :title="conv.title">{{ conv.title || defaultConvTitle(conv) }}</strong>
              <time class="conv-time">{{ shortTime(conv.last_sent_at) }}</time>
            </div>
            <div class="conv-sub">
              <span class="conv-last-msg">
                <template v-if="conv.last_message">
                  <em v-if="conv.conversation_type === 'group' && conv.last_message.sender_name" class="sender-prefix">{{ conv.last_message.sender_name }}: </em>
                  {{ conv.last_message.text || '[媒体内容]' }}
                </template>
                <template v-else>暂无消息</template>
              </span>
              <span class="conv-count-pill">{{ formatCompactNumber(conv.message_count) }}</span>
            </div>
          </div>
        </div>

        <div v-if="!conversationsLoading && !conversations.length" class="sidebar-empty">
          <v-icon icon="mdi-forum-outline" size="36" />
          <span>没有匹配的会话记录</span>
        </div>

        <div v-if="conversationsLoading" class="sidebar-loading">
          <v-progress-circular indeterminate size="22" color="primary" width="2" />
          <span>正在载入会话...</span>
        </div>
      </div>
    </aside>

    <!-- 2. 中间：聊天主视口 -->
    <main class="chat-viewport data-surface">
      <template v-if="currentConv">
        <!-- 会话 Header -->
        <div class="chat-active-bar">
          <div class="chat-active-info">
            <div class="chat-active-avatar">
              <AuthImage :src="currentConv.avatar_uri" :alt="currentConv.title">
                <span>{{ (currentConv.title || '?').slice(0, 1) }}</span>
              </AuthImage>
            </div>
            <div class="chat-active-titles">
              <div class="d-flex align-center gap-2">
                <h2 class="chat-active-name">{{ currentConv.title || defaultConvTitle(currentConv) }}</h2>
                <v-chip size="x-small" :color="currentConv.conversation_type === 'group' ? 'primary' : 'teal'" variant="tonal">
                  {{ currentConv.conversation_type === 'group' ? '群聊' : '私聊' }}
                </v-chip>
              </div>
              <p class="chat-active-sub">
                <span>{{ currentConv.conversation_type === 'group' ? '群号' : 'QQ 账号' }}: {{ currentConv.platform_conversation_id }}</span>
                <span v-if="contextInfo?.total_members" class="ml-2">· {{ contextInfo.total_members }} 位成员</span>
                <span class="ml-2">· 累计 {{ formatNumber(currentConv.message_count) }} 条消息</span>
              </p>
            </div>
          </div>

          <!-- 顶部右侧快捷工具 -->
          <div class="chat-active-tools">
            <v-btn
              color="primary"
              variant="tonal"
              size="small"
              prepend-icon="mdi-graph-outline"
              title="在关系图谱中分析当前会话网络"
              @click="researchConversationInGraph"
            >
              在图谱中分析
            </v-btn>
            <v-btn
              v-if="currentConv.conversation_type === 'group'"
              variant="tonal"
              size="small"
              prepend-icon="mdi-account-group-outline"
              title="查看群组档案"
              @click="openGroupDetail(currentConv.platform_conversation_id)"
            >
              群档案
            </v-btn>
            <v-btn
              v-else
              variant="tonal"
              size="small"
              prepend-icon="mdi-account-outline"
              title="查看个人档案"
              @click="openPersonDetail(currentConv.platform_conversation_id)"
            >
              个人档案
            </v-btn>
            <v-btn
              variant="tonal"
              size="small"
              prepend-icon="mdi-forum-outline"
              :color="threadsDrawerOpen ? 'primary' : undefined"
              title="查看群聊解缠话题小节"
              @click="toggleThreadsDrawer"
            >
              话题解缠 ({{ threads.length || 0 }})
            </v-btn>
            <v-btn
              icon="mdi-magnify"
              variant="text"
              size="small"
              :color="filterBarOpen ? 'primary' : undefined"
              title="会话内全文检索"
              @click="toggleFilterBar"
            />
            <v-btn
              icon="mdi-dock-right"
              variant="text"
              size="small"
              :color="inspectorOpen ? 'primary' : undefined"
              title="研判侧边栏 (成员榜 / 媒体墙)"
              @click="inspectorOpen = !inspectorOpen"
            />
          </div>
        </div>

        <!-- 会话内即时检索条 -->
        <div v-if="filterBarOpen" class="in-chat-filter-bar">
          <v-text-field
            v-model.trim="inChatQuery"
            placeholder="搜索当前会话消息内容、发送者昵称或 QQ..."
            prepend-inner-icon="mdi-filter-variant"
            density="compact"
            variant="solo-filled"
            hide-details
            clearable
            autofocus
            class="in-chat-input"
            @keyup.enter="applyInChatFilter"
            @click:clear="clearInChatFilter"
          />
          <v-btn color="primary" size="small" variant="tonal" :loading="messagesLoading" @click="applyInChatFilter">检索</v-btn>
          <v-btn icon="mdi-close" size="x-small" variant="text" @click="filterBarOpen = false; clearInChatFilter()" />
        </div>

        <!-- 聊天气泡流滚动区 -->
        <div ref="chatStreamRef" class="chat-stream-scroll" @scroll="onStreamScroll">
          <!-- 向上加载更早历史 -->
          <div class="history-loader-zone">
            <v-btn
              v-if="hasMoreHistory"
              variant="text"
              size="small"
              color="primary"
              :loading="loadingMoreHistory"
              prepend-icon="mdi-history"
              @click="loadEarlierMessages"
            >
              加载更早的历史记录 (已呈现 {{ messages.length }} / {{ formatNumber(currentConv.message_count) }})
            </v-btn>
            <span v-else-if="messages.length" class="history-end-text">—— 已回溯至最初记录 ——</span>
          </div>

          <!-- 消息列表与跨天分割线 -->
          <div class="messages-container">
            <template v-for="(msg, index) in messages" :key="msg.id">
              <!-- 跨天分割线 -->
              <div v-if="shouldShowDateDivider(msg, messages[index - 1])" class="chat-date-divider">
                <span>{{ formatDateDivider(msg.sent_at) }}</span>
              </div>

              <!-- 消息行 (已撤回消息依然完整展示，并标注已撤回状态标签) -->
              <div
                :id="`msg-item-${msg.id}`"
                :data-source-id="msg.source_message_id"
                class="chat-msg-row"
                :class="{
                  'is-self': isSelfMsg(msg),
                  'is-recalled': isRecalledMsg(msg),
                  'is-highlighted': inChatQuery && isMatch(msg),
                  'is-target-focused': targetMsgId === msg.id || (msg.source_message_id && targetMsgId === msg.source_message_id),
                }"
              >
                <!-- 发送者头像 -->
                <div class="msg-sender-avatar" @click="openPersonDetail(msg.sender_qq)">
                  <AuthImage :src="msg.sender_avatar || avatarFallback(msg.sender_qq)" :alt="msg.sender">
                    <span>{{ (msg.sender || '?').slice(0, 1) }}</span>
                  </AuthImage>
                </div>

                <!-- 消息主体 -->
                <div class="msg-content-wrapper">
                  <!-- 发送者 Header -->
                  <div class="msg-meta-bar">
                    <strong class="sender-name" @click="openPersonDetail(msg.sender_qq)">
                      {{ isSelfMsg(msg) ? (activeAccount?.name ? `${activeAccount.name} (我)` : '我') : (msg.sender || '未知发送者') }}
                    </strong>
                    <span v-if="msg.sender_qq" class="sender-qq">QQ {{ msg.sender_qq }}</span>
                    <span v-if="getSenderRole(msg.sender_qq)" class="role-badge" :class="getSenderRole(msg.sender_qq)">
                      {{ roleLabel(getSenderRole(msg.sender_qq)) }}
                    </span>
                    <v-chip
                      v-if="isRecalledMsg(msg)"
                      size="x-small"
                      color="warning"
                      variant="tonal"
                      density="compact"
                      class="recalled-status-chip"
                    >
                      <v-icon icon="mdi-undo-variant" size="10" class="mr-1" />
                      已撤回
                    </v-chip>
                    <time class="msg-exact-time">{{ formatTimeExact(msg.sent_at) }}</time>
                  </div>

                  <!-- 气泡盒子 -->
                  <div class="msg-bubble-card" :class="{ 'is-recalled-bubble': isRecalledMsg(msg) }">
                    <!-- 1. 引用回复卡片 -->
                    <div
                      v-if="extractReplyInfo(msg)"
                      class="reply-card cursor-pointer"
                      :title="extractReplyInfo(msg)?.id ? '点击定位被引用的原始消息' : '引用回复'"
                      @click.stop="jumpToQuotedMsg(extractReplyInfo(msg)?.id)"
                    >
                      <v-icon icon="mdi-format-quote-open" size="14" class="reply-icon" />
                      <div class="reply-info">
                        <strong v-if="extractReplyInfo(msg)?.sender">{{ extractReplyInfo(msg)?.sender }}:</strong>
                        <span>{{ extractReplyInfo(msg)?.text }}</span>
                      </div>
                      <v-icon
                        v-if="extractReplyInfo(msg)?.id"
                        icon="mdi-arrow-right-thin"
                        size="13"
                        class="reply-jump-ico ml-auto"
                      />
                    </div>

                    <!-- 2. 富文本与 CQ 码消息段渲染 -->
                    <div class="msg-segments-flow">
                      <template v-if="msg.segments && msg.segments.length">
                        <template v-for="(seg, segIdx) in msg.segments" :key="segIdx">
                          <!-- 纯文本 -->
                          <span v-if="seg.type === 'text'" class="seg-text">{{ seg.data?.text }}</span>
                          <!-- QQ 表情 -->
                          <span v-else-if="seg.type === 'face'" class="seg-face" :title="`表情 ${seg.data?.id}`">
                            [{{ faceName(seg.data?.id) || `表情 ${seg.data?.id}` }}]
                          </span>
                          <!-- @ 提及 -->
                          <span v-else-if="seg.type === 'at'" class="seg-at">@{{ seg.data?.name || seg.data?.qq || '全体成员' }}</span>
                          <!-- 图片 / 表情包：原生优雅渲染，点击即刻进入大图灯箱 -->
                          <div
                            v-else-if="seg.type === 'image'"
                            class="chat-image-preview position-relative"
                            :class="{ 'is-sticker': isSticker(seg.data) }"
                            @click.stop="openLightbox(getImageURL(seg.data))"
                          >
                            <AuthImage
                              :cover="false"
                              :src="getImageURL(seg.data)"
                              :alt="seg.data?.summary || '聊天图片'"
                              loading="lazy"
                              referrerpolicy="no-referrer"
                              class="chat-inline-img"
                              @error="onImageError($event, seg.data)"
                            />
                            <div class="image-action-overlay" title="视觉多模态研判与跨群传播溯源" @click.stop="openImageDiffusion(seg.data?.asset_id || seg.data?.file_id || seg.data?.file, getImageURL(seg.data))">
                              <v-icon icon="mdi-image-search-outline" size="12" class="mr-1" />
                              <span>研判/溯源</span>
                            </div>
                          </div>
                          <!-- 语音 -->
                          <div v-else-if="seg.type === 'record'" class="seg-voice-box">
                            <v-icon icon="mdi-waveform" size="18" color="primary" />
                            <span>语音片段</span>
                            <small v-if="seg.data?.magic">变声</small>
                          </div>
                          <!-- 文件 -->
                          <div v-else-if="seg.type === 'file'" class="seg-file-box">
                            <v-icon icon="mdi-file-document-outline" size="24" color="primary" />
                            <div class="file-details">
                              <strong class="file-name">{{ seg.data?.file || '文件附件' }}</strong>
                              <small class="file-size">{{ formatBytes(seg.data?.file_size || 0) }}</small>
                            </div>
                          </div>
                          <!-- 结构化卡片 / 小程序 -->
                          <div v-else-if="seg.type === 'json' || seg.type === 'xml'" class="seg-card-box">
                            <v-icon icon="mdi-code-json" size="16" color="primary" class="mr-1" />
                            <span>[结构化卡片消息]</span>
                          </div>
                        </template>
                      </template>
                      <template v-else>
                        <span class="seg-text">{{ msg.text || '[非文本消息]' }}</span>
                      </template>
                    </div>

                    <!-- 3. 关联媒体相册 (仅在 segments 中未包含 image 时作为回退展示，杜绝重复渲染) -->
                    <div v-if="!hasSegmentImage(msg) && msg.media && msg.media.length" class="msg-media-wall">
                      <div
                        v-for="m in msg.media"
                        :key="m.reference_id"
                        class="media-wall-thumbnail"
                        @click.stop="openLightbox(getMediaAssetURL(m))"
                      >
                        <AuthImage
                          :cover="false"
                          :src="getMediaAssetURL(m)"
                          :alt="m.filename || '图片'"
                          referrerpolicy="no-referrer"
                          class="wall-img"
                          @error="onMediaWallError"
                        />
                      </div>
                    </div>

                    <!-- 4. 悬浮快捷工具栏 (回复、表情回应、撤回、图谱分析、档案、证据) -->
                    <div class="msg-hover-toolbar">
                      <v-btn
                        icon="mdi-reply-outline"
                        size="x-small"
                        variant="text"
                        title="引用回复此消息"
                        @click.stop="setReplyQuote(msg)"
                      />
                      <v-menu location="top">
                        <template #activator="{ props }">
                          <v-btn
                            v-bind="props"
                            icon="mdi-emoticon-happy-outline"
                            size="x-small"
                            variant="text"
                            title="添加表情回应"
                          />
                        </template>
                        <div class="emoji-reaction-picker data-surface">
                          <span v-for="e in quickReactions" :key="e.id" class="reaction-emoji-btn" @click="sendReaction(msg, e.id)">
                            {{ e.icon }}
                          </span>
                        </div>
                      </v-menu>
                      <v-btn
                        v-if="msg.sender_qq"
                        icon="mdi-graph-outline"
                        size="x-small"
                        variant="text"
                        color="primary"
                        title="在图谱中分析此发送者"
                        @click.stop="researchPersonInGraph(msg.sender_qq)"
                      />
                      <v-btn
                        v-if="msg.sender_qq"
                        icon="mdi-account-outline"
                        size="x-small"
                        variant="text"
                        title="查看个人档案"
                        @click.stop="openPersonDetail(msg.sender_qq)"
                      />
                      <v-menu location="bottom end">
                        <template #activator="{ props }">
                          <v-btn v-bind="props" icon="mdi-dots-horizontal" size="x-small" variant="text" title="更多操作" />
                        </template>
                        <v-list density="compact">
                          <v-list-item prepend-icon="mdi-content-copy" title="复制文本" @click="copyText(msg.text)" />
                          <v-list-item prepend-icon="mdi-link-variant" title="复制追溯链接" @click="copyDeepLink(msg)" />
                          <v-list-item prepend-icon="mdi-identifier" title="复制消息 ID" @click="copyMessageId(msg)" />
                          <v-list-item v-if="currentConv?.conversation_type === 'group'" prepend-icon="mdi-star-outline" title="设为精华消息" @click="setEssence(msg)" />
                          <v-list-item prepend-icon="mdi-delete-sweep-outline" title="撤回消息" @click="recallMsg(msg)" />
                          <v-divider class="my-1" />
                          <v-list-item v-if="msg.raw_record_id" prepend-icon="mdi-code-braces" title="查看原始证据 JSON" @click="viewRawEvidence(msg)" />
                        </v-list>
                      </v-menu>
                    </div>
                  </div>
                </div>
              </div>
            </template>
          </div>

          <div v-if="!messagesLoading && !messages.length" class="stream-empty-box">
            <v-icon icon="mdi-message-outline" size="48" />
            <h3>暂无历史消息记录</h3>
            <p>可通过下方发送框发起会话交流，或在“采集任务”中开启消息同步</p>
          </div>
        </div>

        <!-- 3. 发送框区域 (Native QQ Chat Box) -->
        <div class="chat-send-container">
          <!-- 引用回复提示条 -->
          <div v-if="replyQuoteTarget" class="reply-target-bar">
            <div class="d-flex align-center gap-1">
              <v-icon icon="mdi-reply" size="14" color="primary" />
              <strong>回复 {{ replyQuoteTarget.sender || '消息' }}:</strong>
              <span class="reply-target-text">{{ replyQuoteTarget.text || '[媒体消息]' }}</span>
            </div>
            <v-btn icon="mdi-close" size="x-small" variant="text" @click="replyQuoteTarget = null" />
          </div>

          <!-- 发送工具栏 -->
          <div class="chat-send-toolbar">
            <!-- 表情选择器 Popover -->
            <v-menu v-model="emojiPickerOpen" :close-on-content-click="false" location="top start">
              <template #activator="{ props }">
                <v-btn v-bind="props" icon="mdi-emoticon-outline" size="small" variant="text" title="插入表情" />
              </template>
              <div class="qq-emoji-palette data-surface">
                <div class="emoji-palette-header">QQ 常用表情</div>
                <div class="emoji-palette-grid">
                  <button
                    v-for="(name, fid) in faceMap"
                    :key="fid"
                    class="emoji-palette-item"
                    :title="name"
                    @click="insertFace(fid, name)"
                  >
                    {{ faceEmoji(fid) }}
                  </button>
                </div>
              </div>
            </v-menu>

            <!-- 图片上传/URL 发送弹窗 -->
            <v-menu v-model="imageSendMenuOpen" :close-on-content-click="false" location="top start">
              <template #activator="{ props }">
                <v-btn v-bind="props" icon="mdi-image-outline" size="small" variant="text" title="发送图片" />
              </template>
              <div class="image-send-popover data-surface pa-3">
                <div class="text-caption font-weight-bold mb-2">发送图片链接或绝对路径</div>
                <v-text-field
                  v-model.trim="inputImageURL"
                  placeholder="https://... 或 本地绝对路径"
                  density="compact"
                  variant="outlined"
                  hide-details
                  class="mb-2"
                />
                <div class="d-flex justify-end gap-2">
                  <v-btn size="small" variant="text" @click="imageSendMenuOpen = false">取消</v-btn>
                  <v-btn size="small" color="primary" variant="tonal" :disabled="!inputImageURL" @click="attachImage">添加图片</v-btn>
                </div>
              </div>
            </v-menu>

            <!-- @ 提醒群成员 -->
            <v-menu v-if="currentConv.conversation_type === 'group'" location="top start">
              <template #activator="{ props }">
                <v-btn v-bind="props" icon="mdi-at" size="small" variant="text" title="提醒群成员" />
              </template>
              <v-list density="compact" max-height="240" class="at-member-menu">
                <v-list-item prepend-icon="mdi-account-multiple" title="@全体成员" @click="insertAt('all', '全体成员')" />
                <v-divider class="my-1" />
                <v-list-item
                  v-for="m in contextInfo?.active_rank || []"
                  :key="m.qq"
                  :title="m.display_name"
                  :subtitle="`QQ ${m.qq}`"
                  @click="insertAt(m.qq, m.display_name)"
                >
                  <template #prepend>
                    <v-avatar size="24" class="mr-2">
                      <AuthImage :src="m.avatar_uri || avatarFallback(m.qq)" :alt="m.display_name">
                        <span>{{ (m.display_name || '?').slice(0, 1) }}</span>
                      </AuthImage>
                    </v-avatar>
                  </template>
                </v-list-item>
              </v-list>
            </v-menu>

            <!-- 已附带图片预览胶囊 -->
            <div v-if="pendingImages.length" class="attached-images-strip">
              <v-chip
                v-for="(img, idx) in pendingImages"
                :key="idx"
                closable
                size="small"
                variant="tonal"
                color="primary"
                prepend-icon="mdi-image"
                @click:close="pendingImages.splice(idx, 1)"
              >
                图片 {{ idx + 1 }}
              </v-chip>
            </div>

            <v-spacer />

            <span class="send-shortcut-hint">按 Enter 发送，Shift + Enter 换行</span>
          </div>

          <!-- 输入文本框与发送按钮 -->
          <div class="chat-input-row">
            <textarea
              ref="inputTextareaRef"
              v-model="inputMessageText"
              class="chat-textarea"
              :placeholder="`发送给 ${currentConv.title || defaultConvTitle(currentConv)}...`"
              rows="2"
              @keydown.enter.exact.prevent="sendCurrentMessage"
            />
            <div class="send-action-box">
              <v-btn
                color="primary"
                variant="flat"
                size="small"
                prepend-icon="mdi-send"
                :loading="sendingMessage"
                :disabled="!canSend"
                class="send-btn"
                @click="sendCurrentMessage"
              >
                发送
              </v-btn>
            </div>
          </div>
        </div>
      </template>

      <!-- 未选择会话时的欢迎引导 -->
      <div v-else class="chat-welcome-state">
        <div class="welcome-icon-box">
          <v-icon icon="mdi-forum" size="48" color="primary" />
        </div>
        <h2>请选择一个会话进行研判</h2>
        <p>在左侧列表中点击任意群聊或私聊，即可展开完整聊天气泡流、活跃发言排行榜与媒体相册</p>
        <div class="welcome-stats-row">
          <div class="welcome-stat-pill">
            <span>活跃会话</span>
            <strong>{{ conversationsTotal }}</strong>
          </div>
          <div class="welcome-stat-pill">
            <span>群聊会话</span>
            <strong>{{ groupCount }}</strong>
          </div>
          <div class="welcome-stat-pill">
            <span>私聊好友</span>
            <strong>{{ privateCount }}</strong>
          </div>
        </div>
      </div>
    </main>

    <!-- 4. 右侧：研判侧边栏 (群成员发言榜 / 媒体墙 / 会话档案) -->
    <aside v-if="inspectorOpen && currentConv" class="chat-inspector data-surface">
      <div class="inspector-top-tabs">
        <v-tabs v-model="inspectorTab" density="compact" grow color="primary">
          <v-tab value="rank">
            <v-icon icon="mdi-account-star-outline" size="15" class="mr-1" />
            发言榜
          </v-tab>
          <v-tab value="media">
            <v-icon icon="mdi-image-multiple-outline" size="15" class="mr-1" />
            媒体墙 ({{ contextInfo?.total_media || contextInfo?.media_items?.length || 0 }})
          </v-tab>
          <v-tab value="info">
            <v-icon icon="mdi-information-outline" size="15" class="mr-1" />
            档案
          </v-tab>
        </v-tabs>
        <v-btn icon="mdi-close" size="x-small" variant="text" title="收起侧边栏" @click="inspectorOpen = false" />
      </div>

      <div class="inspector-scroll-area">
        <v-progress-linear v-if="contextLoading" indeterminate color="primary" height="2" />

        <!-- Tab 1: 活跃发言成员榜 -->
        <div v-if="inspectorTab === 'rank'" class="tab-pane">
          <div class="pane-header-row">
            <strong>发言排行榜 TOP 20</strong>
            <span>共 {{ contextInfo?.active_rank?.length || 0 }} 位成员</span>
          </div>

          <div v-if="contextInfo?.active_rank?.length" class="active-member-list">
            <div
              v-for="(member, rankIdx) in contextInfo.active_rank"
              :key="member.person_id || member.qq"
              class="member-rank-item"
              @click="openPersonDetail(member.qq)"
            >
              <span class="rank-pos" :class="{ top: rankIdx < 3 }">{{ rankIdx + 1 }}</span>
              <div class="member-avatar">
                <AuthImage :src="member.avatar_uri || avatarFallback(member.qq)" :alt="member.display_name">
                  <span>{{ (member.display_name || '?').slice(0, 1) }}</span>
                </AuthImage>
              </div>
              <div class="member-meta">
                <div class="member-meta-top">
                  <strong class="member-name">{{ member.display_name }}</strong>
                  <span class="member-count">{{ formatNumber(member.message_count) }} 条</span>
                </div>
                <div class="member-ratio-bar">
                  <div
                    class="member-ratio-fill"
                    :style="{ width: `${Math.min(100, (member.message_count / (contextInfo.active_rank[0]?.message_count || 1)) * 100)}%` }"
                  />
                </div>
                <div class="member-meta-sub">
                  <small>QQ: {{ member.qq || '未知' }}</small>
                  <small v-if="member.last_active_at">{{ shortTime(member.last_active_at) }}</small>
                </div>
              </div>
              <v-btn
                icon="mdi-graph-outline"
                size="x-small"
                variant="text"
                color="primary"
                title="在图谱中分析此人"
                @click.stop="researchPersonInGraph(member.qq)"
              />
            </div>
          </div>
          <div v-else class="pane-empty">
            <v-icon icon="mdi-account-outline" size="32" />
            <span>暂无成员发言统计</span>
          </div>
        </div>

        <!-- Tab 2: 媒体图片墙 -->
        <div v-if="inspectorTab === 'media'" class="tab-pane">
          <div class="pane-header-row">
            <strong>历史图片与媒体</strong>
            <span>{{ contextInfo?.media_items?.length || 0 }} 项</span>
          </div>

          <div v-if="contextInfo?.media_items?.length" class="media-grid-wall">
            <div
              v-for="item in contextInfo.media_items"
              :key="item.id || item.reference_id"
              class="media-grid-item"
              @click="openLightbox(item.asset_url)"
            >
              <AuthImage :cover="false" :src="item.asset_url" :alt="item.filename || '图片'" referrerpolicy="no-referrer" class="grid-wall-img" />
              <div class="media-overlay-tag">
                <span>{{ item.sender_name }}</span>
                <time>{{ shortTime(item.sent_at) }}</time>
              </div>
            </div>
          </div>
          <div v-else class="pane-empty">
            <v-icon icon="mdi-image-off-outline" size="32" />
            <span>本会话暂未收录媒体资源</span>
          </div>
        </div>

        <!-- Tab 3: 会话档案 -->
        <div v-if="inspectorTab === 'info'" class="tab-pane">
          <div class="summary-cards-grid">
            <div class="summary-card">
              <span class="card-label">会话类型</span>
              <strong>{{ currentConv.conversation_type === 'group' ? '群聊会话' : '私聊消息' }}</strong>
            </div>
            <div class="summary-card">
              <span class="card-label">{{ currentConv.conversation_type === 'group' ? '群号' : 'QQ 账号' }}</span>
              <code>{{ currentConv.platform_conversation_id }}</code>
            </div>
            <div class="summary-card">
              <span class="card-label">总消息数</span>
              <strong>{{ formatNumber(currentConv.message_count) }} 条</strong>
            </div>
            <div class="summary-card">
              <span class="card-label">媒体总数</span>
              <strong>{{ formatNumber(contextInfo?.total_media || 0) }} 个</strong>
            </div>
            <div v-if="currentConv.conversation_type === 'group'" class="summary-card">
              <span class="card-label">群内成员</span>
              <strong>{{ formatNumber(contextInfo?.total_members || 0) }} 位</strong>
            </div>
            <div class="summary-card">
              <span class="card-label">最后发言时间</span>
              <span>{{ formatDateExact(currentConv.last_sent_at) }}</span>
            </div>
            <div class="summary-card span-full">
              <span class="card-label">会话 UUID</span>
              <code class="uuid-text">{{ currentConv.id }}</code>
            </div>
          </div>

          <div class="info-actions mt-4">
            <v-btn
              block
              color="primary"
              variant="tonal"
              prepend-icon="mdi-graph-outline"
              class="mb-2"
              @click="researchConversationInGraph"
            >
              在图谱中展开拓扑关系
            </v-btn>
            <v-btn
              v-if="currentConv.conversation_type === 'group'"
              block
              variant="outlined"
              prepend-icon="mdi-account-group-outline"
              @click="openGroupDetail(currentConv.platform_conversation_id)"
            >
              查看完整群组档案
            </v-btn>
            <v-btn
              v-else
              block
              variant="outlined"
              prepend-icon="mdi-account-outline"
              @click="openPersonDetail(currentConv.platform_conversation_id)"
            >
              查看个人档案
            </v-btn>
          </div>
        </div>
      </div>
    </aside>

    <!-- 5. 图片全功能大图灯箱预览 (支持缩放/旋转/原图打开) -->
    <v-dialog v-model="lightboxOpen" max-width="900" scrim="rgba(0,0,0,0.85)">
      <v-card class="lightbox-modal-card">
        <div class="lightbox-top-toolbar">
          <span class="text-caption text-medium-emphasis">图片预览</span>
          <div class="lightbox-actions">
            <v-btn icon="mdi-magnify-plus-outline" size="small" variant="text" title="放大" @click="zoomImage(0.2)" />
            <v-btn icon="mdi-magnify-minus-outline" size="small" variant="text" title="缩小" @click="zoomImage(-0.2)" />
            <v-btn icon="mdi-rotate-right" size="small" variant="text" title="向右旋转 90°" @click="rotateImage" />
            <v-btn icon="mdi-restore" size="small" variant="text" title="重置视角" @click="resetImageTransform" />
            <v-btn v-if="lightboxSrc" icon="mdi-open-in-new" size="small" variant="text" title="在新标签页查看原图" :href="lightboxSrc" target="_blank" />
            <v-btn icon="mdi-close" size="small" variant="text" title="关闭" @click="lightboxOpen = false" />
          </div>
        </div>
        <div class="lightbox-stage">
          <img
            :src="lightboxSrc"
            alt="大图预览"
            referrerpolicy="no-referrer"
            class="lightbox-img"
            :style="{
              transform: `scale(${imgScale}) rotate(${imgRotate}deg)`,
            }"
          />
        </div>
      </v-card>
    </v-dialog>

    <!-- 6. 原始证据 JSON 弹窗 -->
    <v-dialog v-model="evidenceDialogOpen" max-width="720">
      <v-card class="data-surface">
        <v-card-title class="d-flex align-center justify-between pa-4">
          <span>原始记录与证据详情</span>
          <v-btn icon="mdi-close" variant="text" size="small" @click="evidenceDialogOpen = false" />
        </v-card-title>
        <v-card-text class="pa-4">
          <div v-if="selectedEvidence" class="evidence-body">
            <div class="mb-2"><strong>消息 ID:</strong> <code>{{ selectedEvidence.id }}</code></div>
            <div class="mb-2"><strong>原始记录 ID:</strong> <code>{{ selectedEvidence.raw_record_id }}</code></div>
            <div class="mb-2"><strong>发送时间:</strong> <span>{{ selectedEvidence.sent_at }}</span></div>
            <div class="mb-2"><strong>Segments 结构:</strong></div>
            <pre class="evidence-pre">{{ JSON.stringify(selectedEvidence.segments || selectedEvidence, null, 2) }}</pre>
          </div>
        </v-card-text>
      </v-card>
    </v-dialog>

    <!-- 7. 话题解缠抽屉 -->
    <v-navigation-drawer
      v-model="threadsDrawerOpen"
      location="right"
      temporary
      width="440"
      class="threads-drawer"
    >
      <div class="threads-drawer-content pa-3">
        <div class="d-flex align-center justify-space-between mb-3 border-b pb-2">
          <div class="d-flex align-center">
            <v-icon icon="mdi-forum-outline" color="primary" class="mr-2" />
            <strong class="text-subtitle-1">群聊会话解缠 (Threads)</strong>
          </div>
          <div class="d-flex align-center gap-1">
            <v-btn
              size="x-small"
              variant="tonal"
              color="primary"
              prepend-icon="mdi-refresh"
              :loading="threadsLoading"
              @click="triggerDisentangle"
            >
              重新解缠
            </v-btn>
            <v-btn icon="mdi-close" variant="text" size="x-small" @click="threadsDrawerOpen = false" />
          </div>
        </div>

        <v-skeleton-loader v-if="threadsLoading" type="list-item-three-line@4" />
        <template v-else-if="threads.length">
          <div class="threads-list">
            <div
              v-for="(th, idx) in threads"
              :key="th.id"
              class="thread-card mb-3 pa-3"
              :class="{ active: selectedThreadId === th.id }"
              @click="selectThread(th)"
            >
              <div class="d-flex align-center justify-space-between mb-1">
                <span class="text-caption font-weight-bold text-primary">#{{ idx + 1 }} {{ th.title }}</span>
                <v-chip size="x-small" :color="stanceColor(th.stance)" variant="tonal">
                  {{ stanceLabel(th.stance) }}
                </v-chip>
              </div>

              <p class="text-body-2 text-medium-emphasis mb-2">{{ th.summary }}</p>

              <div class="d-flex align-center justify-space-between text-caption text-grey">
                <span>{{ th.message_count }} 条消息 · {{ th.participant_count }} 人参与</span>
                <span>{{ formatTimeRange(th.started_at, th.ended_at) }}</span>
              </div>

              <div class="d-flex flex-wrap mt-2" v-if="th.key_entities?.length">
                <v-chip v-for="ent in th.key_entities" :key="ent" size="x-small" variant="outlined" class="mr-1 mb-1">
                  {{ ent }}
                </v-chip>
              </div>
            </div>
          </div>
        </template>
        <div v-else class="text-caption text-grey pa-4 text-center">
          <div>暂未对该会话进行解缠分析</div>
          <v-btn size="small" variant="tonal" color="primary" class="mt-2" prepend-icon="mdi-forum-outline" :loading="threadsLoading" @click="triggerDisentangle">
            立即解缠对话流
          </v-btn>
        </div>
      </div>
    </v-navigation-drawer>

    <!-- 8. 视觉多模态与跨群传播弹窗 -->
    <ImageDiffusionModal
      v-model="diffusionModalOpen"
      :asset-id="selectedAssetId"
      :image-url="selectedImageUrl"
    />

    <!-- 提示消息通知条 -->
    <v-snackbar v-model="snackOpen" :color="snackColor" timeout="2500" location="top">
      {{ snackMessage }}
    </v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { api } from '@/services/api'
import AuthImage from '@/components/AuthImage.vue'
import { cachedMediaURL } from '@/services/mediaCache'
import ImageDiffusionModal from '@/components/ImageDiffusionModal.vue'
import type { ChatMessage, ConversationContext, ConversationItem } from '@/types/api'

const route = useRoute()
const router = useRouter()

// 0. NapCat 账号管理
const napcatAccounts = ref<any[]>([])
const activeAccount = ref<any>(null)

// 1. 会话列表状态
const conversations = ref<ConversationItem[]>([])
const conversationsTotal = ref(0)
const conversationsLoading = ref(false)
const searchQuery = ref('')
const activeTypeFilter = ref<string>('')
const currentConv = ref<ConversationItem | null>(null)
const autoSync = ref(true)

// 2. 消息流状态
const messages = ref<ChatMessage[]>([])
const messagesLoading = ref(false)
const loadingMoreHistory = ref(false)
const hasMoreHistory = ref(false)
const chatStreamRef = ref<HTMLElement | null>(null)
const inChatQuery = ref('')
const filterBarOpen = ref(false)

// 2.5 话题解缠与视觉分析状态
const threadsDrawerOpen = ref(false)
const threads = ref<any[]>([])
const threadsLoading = ref(false)
const selectedThreadId = ref<string>('')
const diffusionModalOpen = ref(false)
const selectedAssetId = ref<string>('')
const selectedImageUrl = ref<string>('')

// 3. 发送框状态
const inputMessageText = ref('')
const pendingImages = ref<string[]>([])
const replyQuoteTarget = ref<ChatMessage | null>(null)
const sendingMessage = ref(false)
const emojiPickerOpen = ref(false)
const imageSendMenuOpen = ref(false)
const inputImageURL = ref('')
const inputTextareaRef = ref<HTMLTextAreaElement | null>(null)

// 4. 研判侧边栏状态
const inspectorOpen = ref(true)
const inspectorTab = ref<'rank' | 'media' | 'info'>('rank')
const contextInfo = ref<ConversationContext | null>(null)
const contextLoading = ref(false)

// 5. 灯箱与大图预览状态
const lightboxOpen = ref(false)
const lightboxSrc = ref('')
const imgScale = ref(1)
const imgRotate = ref(0)
const evidenceDialogOpen = ref(false)
const selectedEvidence = ref<any>(null)
const snackOpen = ref(false)
const snackMessage = ref('')
const snackColor = ref('success')

// 轮询定时器
let syncTimer: any = null

const groupCount = computed(() => conversations.value.filter(c => c.conversation_type === 'group').length)
const privateCount = computed(() => conversations.value.filter(c => c.conversation_type === 'private').length)
const canSend = computed(() => inputMessageText.value.trim().length > 0 || pendingImages.value.length > 0)

function showSnack(msg: string, color = 'success') {
  snackMessage.value = msg
  snackColor.value = color
  snackOpen.value = true
}

function defaultConvTitle(conv: ConversationItem): string {
  return conv.conversation_type === 'group' ? `群 ${conv.platform_conversation_id}` : `QQ ${conv.platform_conversation_id}`
}

// 加载 NapCat 账号列表
async function loadAccounts() {
  try {
    const res = (await api.get('/api/v1/accounts')).data
    napcatAccounts.value = res.data || []
    if (napcatAccounts.value.length > 0) {
      const savedAccId = localStorage.getItem('active_napcat_account_id')
      const matched = napcatAccounts.value.find(a => a.id === savedAccId)
      activeAccount.value = matched || napcatAccounts.value[0]
    }
  } catch (e) {
    console.error('加载 NapCat 账号失败', e)
  }
}

function switchAccount(acc: any) {
  activeAccount.value = acc
  localStorage.setItem('active_napcat_account_id', acc.id)
  showSnack(`已切换操作账号为: ${acc.name || acc.qq_uin}`, 'success')
}

function isSelfMsg(msg: ChatMessage): boolean {
  if (!activeAccount.value?.qq_uin) return false
  return String(msg.sender_qq) === String(activeAccount.value.qq_uin)
}

function isRecalledMsg(msg: ChatMessage): boolean {
  return msg.text === '[该消息已被撤回]' || (msg as any).is_recalled === true
}

const targetMsgId = ref<string>('')

// 滚动定位并高亮目标消息
async function scrollToAndHighlight(msgId: string) {
  targetMsgId.value = msgId
  await nextTick()
  let attempts = 0
  const tryScroll = () => {
    const el = document.getElementById(`msg-item-${msgId}`) || document.querySelector(`[data-source-id="${msgId}"]`)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'center' })
      showSnack('已定位并高亮原始证据消息', 'info')
    } else if (attempts < 6) {
      attempts++
      setTimeout(tryScroll, 120)
    }
  }
  setTimeout(tryScroll, 80)

  // 保持高亮光晕 4.5 秒
  setTimeout(() => {
    if (targetMsgId.value === msgId) {
      targetMsgId.value = ''
    }
  }, 4500)
}

// 话题解缠与视觉分析方法
async function loadThreads() {
  if (!currentConv.value) return
  threadsLoading.value = true
  try {
    const res = (await api.get(`/api/v1/conversations/${currentConv.value.id}/threads`)).data
    threads.value = res.data || []
  } catch (e) {
    console.error('加载话题列表失败', e)
  } finally {
    threadsLoading.value = false
  }
}

async function triggerDisentangle() {
  if (!currentConv.value) return
  threadsLoading.value = true
  try {
    const res = (await api.post(`/api/v1/conversations/${currentConv.value.id}/disentangle`, {}, { params: { limit: 80 } })).data
    threads.value = res.data || []
    showSnack('群聊会话解缠分析完成', 'success')
  } catch (e: any) {
    showSnack(e.response?.data?.error || '会话解缠失败', 'error')
  } finally {
    threadsLoading.value = false
  }
}

function toggleThreadsDrawer() {
  threadsDrawerOpen.value = !threadsDrawerOpen.value
  if (threadsDrawerOpen.value && !threads.value.length) {
    loadThreads()
  }
}

function selectThread(th: any) {
  selectedThreadId.value = th.id
  if (th.messages && th.messages.length) {
    jumpToQuotedMsg(th.messages[0].message_id)
  }
}

function openImageDiffusion(assetId?: string, url?: string) {
  selectedAssetId.value = assetId || ''
  selectedImageUrl.value = url || ''
  diffusionModalOpen.value = true
}

function stanceColor(stance?: string) {
  switch (stance) {
    case 'debate': return 'error'
    case 'question_answer': return 'info'
    case 'collaboration': return 'success'
    default: return 'primary'
  }
}

function stanceLabel(stance?: string) {
  switch (stance) {
    case 'debate': return '争议探讨'
    case 'question_answer': return '问答求助'
    case 'collaboration': return '协同合作'
    default: return '日常交流'
  }
}

function formatTimeRange(start?: string, end?: string) {
  if (!start) return ''
  const s = new Date(start)
  const e = end ? new Date(end) : s
  const f = (d: Date) => `${d.getHours().toString().padStart(2, '0')}:${d.getMinutes().toString().padStart(2, '0')}`
  return `${f(s)} ~ ${f(e)}`
}

// 加载会话列表
async function loadConversations(silent = false) {
  if (!silent) conversationsLoading.value = true
  try {
    const res = (await api.get('/api/v1/conversations', {
      params: {
        limit: 100,
        offset: 0,
        q: searchQuery.value,
        type: activeTypeFilter.value,
      },
    })).data
    conversations.value = res.data || []
    conversationsTotal.value = res.total || 0
  } catch (e) {
    console.error('加载会话列表失败', e)
  } finally {
    if (!silent) conversationsLoading.value = false
  }
}

// 统一解析路由参数进行深度消息穿透定位
async function handleRouteQuery() {
  const queryMsgId = (route.query.message_id || route.query.message) as string
  const queryConvID = route.query.conversation as string
  const queryGroupID = route.query.group_id as string
  const queryQQ = (route.query.qq || route.query.user_id) as string
  const querySearch = (route.query.q || route.query.query) as string

  if (querySearch && !queryMsgId) {
    searchQuery.value = querySearch
  }

  // 1. 如果传了消息 ID，则通过 /api/v1/messages/{id} 穿透定位
  if (queryMsgId) {
    targetMsgId.value = queryMsgId
    try {
      const res = (await api.get(`/api/v1/messages/${queryMsgId}`)).data
      const msg = res.data
      if (msg) {
        const convUUID = msg.conversation_uuid || msg.conversation_id
        let targetConv = conversations.value.find(c => c.id === convUUID || c.platform_conversation_id === msg.conversation_id)
        if (!targetConv && convUUID) {
          try {
            const convRes = (await api.get(`/api/v1/conversations/${convUUID}`)).data
            if (convRes.data) {
              const foundConv = convRes.data
              targetConv = foundConv
              if (foundConv && !conversations.value.some(c => c.id === foundConv.id)) {
                conversations.value.unshift(foundConv)
              }
            }
          } catch (err) {
            console.error('加载消息所在会话失败', err)
          }
        }
        if (targetConv) {
          currentConv.value = targetConv
          loadContext(targetConv.id)
          await loadMessagesAround(targetConv.id, msg.id)
          return
        }
      }
    } catch (e) {
      console.error('查找追溯消息失败', e)
    }
  }

  // 2. 如果指定了会话 ID
  if (queryConvID) {
    const match = conversations.value.find(c => c.id === queryConvID || c.platform_conversation_id === queryConvID)
    if (match) selectConversation(match)
    else loadDirectConversation(queryConvID)
    return
  }

  // 3. 如果指定了群号
  if (queryGroupID) {
    const match = conversations.value.find(c => c.platform_conversation_id === queryGroupID && c.conversation_type === 'group')
    if (match) selectConversation(match)
    else loadDirectConversation(queryGroupID)
    return
  }

  // 4. 如果指定了私聊 QQ
  if (queryQQ) {
    const match = conversations.value.find(c => c.platform_conversation_id === queryQQ && c.conversation_type === 'private')
    if (match) selectConversation(match)
    return
  }

  // 5. 默认选中第一条会话
  if (!currentConv.value && conversations.value.length > 0) {
    selectConversation(conversations.value[0])
  }
}

// 直接加载指定会话
async function loadDirectConversation(id: string) {
  try {
    const res = (await api.get(`/api/v1/conversations/${id}`)).data
    if (res.data) {
      currentConv.value = res.data
      if (!conversations.value.some(c => c.id === res.data.id)) {
        conversations.value.unshift(res.data)
      }
      loadMessages(res.data.id)
      loadContext(res.data.id)
    }
  } catch (e) {
    console.error('获取指定会话失败', e)
  }
}

// 选中会话
function selectConversation(conv: ConversationItem) {
  if (currentConv.value?.id === conv.id) return
  currentConv.value = conv
  messages.value = []
  inChatQuery.value = ''
  replyQuoteTarget.value = null
  loadMessages(conv.id)
  loadContext(conv.id)
}

// 加载指定消息及其周围上下文
async function loadMessagesAround(convID: string, msgId: string) {
  messagesLoading.value = true
  try {
    const res = (await api.get(`/api/v1/conversations/${convID}/messages`, {
      params: {
        limit: 50,
        around: msgId,
      },
    })).data
    const list: ChatMessage[] = (res.data || []).reverse()
    messages.value = list
    hasMoreHistory.value = (res.total || 0) > list.length
    await scrollToAndHighlight(msgId)
  } catch (e) {
    console.error('加载定位消息失败', e)
  } finally {
    messagesLoading.value = false
  }
}

// 加载会话消息
async function loadMessages(convID: string, isSilentPoll = false) {
  if (!isSilentPoll) messagesLoading.value = true
  try {
    const res = (await api.get(`/api/v1/conversations/${convID}/messages`, {
      params: {
        limit: 50,
        q: inChatQuery.value,
      },
    })).data
    const list: ChatMessage[] = (res.data || []).reverse()

    // 如果是轮询且已有新消息，平滑追加
    if (isSilentPoll && messages.value.length > 0) {
      const newIncoming = list.filter(m => !messages.value.some(existing => existing.id === m.id))
      // 同步撤回状态 (保留原始内容，仅标记 is_recalled)
      list.forEach(item => {
        const local = messages.value.find(m => m.id === item.id)
        if (local && (item.is_recalled || item.text === '[该消息已被撤回]')) {
          local.is_recalled = true
        }
      })
      if (newIncoming.length > 0) {
        const isNearBottom = chatStreamRef.value && (chatStreamRef.value.scrollHeight - chatStreamRef.value.scrollTop - chatStreamRef.value.clientHeight < 120)
        messages.value = [...messages.value, ...newIncoming]
        if (isNearBottom) {
          await nextTick()
          scrollToBottom()
        }
      }
    } else {
      messages.value = list
      hasMoreHistory.value = (res.total || 0) > list.length
      await nextTick()
      scrollToBottom()
    }
  } catch (e) {
    console.error('加载会话消息失败', e)
  } finally {
    if (!isSilentPoll) messagesLoading.value = false
  }
}

// 向上加载历史
async function loadEarlierMessages() {
  if (!currentConv.value || loadingMoreHistory.value || !messages.value.length) return
  loadingMoreHistory.value = true

  const oldestMsg = messages.value[0]
  const prevScrollHeight = chatStreamRef.value?.scrollHeight || 0

  try {
    const res = (await api.get(`/api/v1/conversations/${currentConv.value.id}/messages`, {
      params: {
        limit: 50,
        before: oldestMsg.sent_at,
        q: inChatQuery.value,
      },
    })).data
    const earlierList: ChatMessage[] = (res.data || []).reverse()
    if (earlierList.length > 0) {
      messages.value = [...earlierList, ...messages.value]
      hasMoreHistory.value = messages.value.length < (res.total || 0)

      await nextTick()
      if (chatStreamRef.value) {
        const newScrollHeight = chatStreamRef.value.scrollHeight
        chatStreamRef.value.scrollTop = newScrollHeight - prevScrollHeight
      }
    } else {
      hasMoreHistory.value = false
    }
  } catch (e) {
    console.error('加载历史消息失败', e)
  } finally {
    loadingMoreHistory.value = false
  }
}

// 发送消息
async function sendCurrentMessage() {
  if (!currentConv.value || !canSend.value || sendingMessage.value) return
  sendingMessage.value = true

  const textToSend = inputMessageText.value.trim()
  const replyTo = replyQuoteTarget.value?.source_message_id || ''
  const imagesToSend = [...pendingImages.value]

  try {
    const res = await api.post(`/api/v1/conversations/${currentConv.value.id}/send`, {
      text: textToSend,
      reply_to: replyTo,
      images: imagesToSend,
      account_id: activeAccount.value?.id,
    })

    if (res.data?.status === 'ok') {
      inputMessageText.value = ''
      pendingImages.value = []
      replyQuoteTarget.value = null
      showSnack('消息发送成功', 'success')
      // 立即拉取最新一条
      await loadMessages(currentConv.value.id, true)
      await nextTick()
      scrollToBottom()
    }
  } catch (e: any) {
    showSnack(e.response?.data?.error || '消息发送失败，请检查 NapCat 账号连接', 'error')
  } finally {
    sendingMessage.value = false
  }
}

// 引用回复
function setReplyQuote(msg: ChatMessage) {
  replyQuoteTarget.value = msg
  inputTextareaRef.value?.focus()
}

// 表情回应 (NapCat set_msg_emoji_like)
const quickReactions = [
  { id: 128077, icon: '👍' },
  { id: 128522, icon: '😊' },
  { id: 128557, icon: '😭' },
  { id: 128293, icon: '🔥' },
  { id: 10084, icon: '❤️' },
]

async function sendReaction(msg: ChatMessage, emojiId: number) {
  try {
    await api.post(`/api/v1/messages/${msg.id}/reaction`, { emoji_id: emojiId, set: true })
    showSnack('表情回应成功', 'success')
  } catch (e: any) {
    showSnack(e.response?.data?.error || '表情回应失败', 'error')
  }
}

// 设为精华消息 (NapCat set_essence_msg)
async function setEssence(msg: ChatMessage) {
  try {
    await api.post(`/api/v1/messages/${msg.id}/essence`)
    showSnack('已成功设为精华消息', 'success')
  } catch (e: any) {
    showSnack(e.response?.data?.error || '设为精华消息失败', 'error')
  }
}

// 撤回消息 (NapCat delete_msg)
async function recallMsg(msg: ChatMessage) {
  try {
    const res = await api.post(`/api/v1/messages/${msg.id}/recall`)
    if (res.data?.status === 'ok') {
      msg.is_recalled = true
      showSnack('消息已在 QQ 端撤回，系统中已打上「已撤回」标记', 'success')
    }
  } catch (e: any) {
    showSnack(e.response?.data?.error || '撤回失败 (可能超出撤回时限)', 'error')
  }
}

// 插入表情到输入框
function insertFace(id: string, name: string) {
  inputMessageText.value += `[${name}]`
  emojiPickerOpen.value = false
  inputTextareaRef.value?.focus()
}

// 插入 @ 到输入框
function insertAt(qq: string, name: string) {
  inputMessageText.value += `@${name} `
  inputTextareaRef.value?.focus()
}

// 附带图片
function attachImage() {
  if (inputImageURL.value.trim()) {
    pendingImages.value.push(inputImageURL.value.trim())
    inputImageURL.value = ''
    imageSendMenuOpen.value = false
  }
}

// 获取图片实际 URL (通过后端代理直连 NapCat 本地/远程资源，解决 Tencent 临时 URL 过期问题)
function getImageURL(data: any): string {
  if (!data) return ''
  const file = data.file || ''
  const url = data.url || ''
  if (file || url) {
    const params = new URLSearchParams()
    if (file) params.set('file', file)
    if (url) params.set('url', url)
    return `/api/v1/media/chat-image?${params.toString()}`
  }
  return ''
}

function hasSegmentImage(msg: ChatMessage): boolean {
  return Array.isArray(msg.segments) && msg.segments.some(s => s.type === 'image')
}

function isSticker(data: any): boolean {
  if (!data) return false
  return data.sub_type === 1 || data.summary === '[动画表情]' || data.subType === 1
}

function getMediaAssetURL(m: any): string {
  if (m.asset_url) return m.asset_url
  if (m.filename || m.source_url) {
    const params = new URLSearchParams()
    if (m.filename) params.set('file', m.filename)
    if (m.source_url) params.set('url', m.source_url)
    return `/api/v1/media/chat-image?${params.toString()}`
  }
  return ''
}

function onMediaWallError(e: Event) {
  const target = e.target as HTMLImageElement
  if (target) {
    target.style.display = 'none'
    const parent = target.parentElement
    if (parent && !parent.querySelector('.img-error-badge')) {
      const badge = document.createElement('span')
      badge.className = 'img-error-badge'
      badge.innerText = '[加载失败]'
      parent.appendChild(badge)
    }
  }
}

function onImageError(e: Event, data: any) {
  const target = e.target as HTMLImageElement
  if (target) {
    target.style.display = 'none'
    const parent = target.parentElement
    if (parent && !parent.querySelector('.img-error-badge')) {
      const badge = document.createElement('span')
      badge.className = 'img-error-badge'
      badge.innerText = '[图片加载失败]'
      parent.appendChild(badge)
    }
  }
}

// 打开大图灯箱
let lightboxRequest = 0
async function openLightbox(src?: string) {
  if (!src) return
  const request = ++lightboxRequest
  let resolved: string
  try { resolved = src.startsWith('/api/') ? await cachedMediaURL(src) : src }
  catch { return }
  if (request !== lightboxRequest) return
  lightboxSrc.value = resolved
  resetImageTransform()
  lightboxOpen.value = true
}

function zoomImage(delta: number) {
  imgScale.value = Math.max(0.4, Math.min(3.5, imgScale.value + delta))
}

function rotateImage() {
  imgRotate.value = (imgRotate.value + 90) % 360
}

function resetImageTransform() {
  imgScale.value = 1
  imgRotate.value = 0
}

// 加载侧边栏数据
async function loadContext(convID: string) {
  contextLoading.value = true
  try {
    const res = (await api.get(`/api/v1/conversations/${convID}/context`)).data
    contextInfo.value = res.data || null
  } catch (e) {
    console.error('加载侧边栏失败', e)
  } finally {
    contextLoading.value = false
  }
}

// 搜索防抖
let searchTimer: any = null
function onSearchChange() {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    loadConversations()
  }, 300)
}

watch(activeTypeFilter, () => {
  loadConversations()
})

function toggleFilterBar() {
  filterBarOpen.value = !filterBarOpen.value
  if (!filterBarOpen.value && inChatQuery.value) {
    clearInChatFilter()
  }
}

function applyInChatFilter() {
  if (currentConv.value) loadMessages(currentConv.value.id)
}

function clearInChatFilter() {
  inChatQuery.value = ''
  if (currentConv.value) loadMessages(currentConv.value.id)
}

function isMatch(msg: ChatMessage) {
  if (!inChatQuery.value) return false
  const q = inChatQuery.value.toLowerCase()
  return (
    msg.text?.toLowerCase().includes(q) ||
    msg.sender?.toLowerCase().includes(q) ||
    msg.sender_qq?.includes(q)
  )
}

function scrollToBottom() {
  if (chatStreamRef.value) {
    chatStreamRef.value.scrollTop = chatStreamRef.value.scrollHeight
  }
}

function onStreamScroll() {
  if (chatStreamRef.value && chatStreamRef.value.scrollTop === 0 && hasMoreHistory.value && !loadingMoreHistory.value) {
    loadEarlierMessages()
  }
}

function shouldShowDateDivider(current: ChatMessage, prev?: ChatMessage): boolean {
  if (!prev) return true
  if (!current.sent_at || !prev.sent_at) return false
  const curDate = new Date(current.sent_at).toDateString()
  const prevDate = new Date(prev.sent_at).toDateString()
  return curDate !== prevDate
}

const replyCache = ref<Record<string, { sender: string; text: string }>>({})
const replyFetching = new Set<string>()

async function fetchReplyMessage(replyId: string) {
  if (!replyId || replyCache.value[replyId] || replyFetching.has(replyId)) return
  replyFetching.add(replyId)
  try {
    const res = (await api.get(`/api/v1/messages/${replyId}`)).data.data
    if (res) {
      const sender = res.sender || (res.sender_qq ? `QQ ${res.sender_qq}` : '用户')
      const text = res.text || (res.media?.length ? '[图片/媒体]' : '[消息]')
      replyCache.value = {
        ...replyCache.value,
        [replyId]: { sender, text },
      }
    }
  } catch (e) {
    replyCache.value = {
      ...replyCache.value,
      [replyId]: { sender: '', text: `引用消息 #${replyId}` },
    }
  } finally {
    replyFetching.delete(replyId)
  }
}

function extractReplyInfo(msg: ChatMessage) {
  const replySeg = msg.segments?.find(s => s.type === 'reply')
  const replyId = String(msg.reply_to || replySeg?.data?.id || '').trim()
  if (!replyId && !replySeg) return null

  // 1. Direct backend resolved fields
  if (msg.reply_sender || msg.reply_text) {
    return {
      id: replyId,
      sender: msg.reply_sender,
      text: msg.reply_text || '[媒体内容]',
    }
  }

  // 2. Direct local cache in current conversation
  if (replyId) {
    const found = messages.value.find(m => m.id === replyId || m.source_message_id === replyId)
    if (found) {
      const sender = isSelfMsg(found)
        ? (activeAccount.value?.name ? `${activeAccount.value.name} (我)` : '我')
        : (found.sender || (found.sender_qq ? `QQ ${found.sender_qq}` : '用户'))
      const text = found.text || (found.media?.length ? '[图片/媒体]' : '[无文本内容]')
      return {
        id: replyId,
        sender,
        text,
      }
    }

    // 3. Checked async cache
    if (replyCache.value[replyId]) {
      return {
        id: replyId,
        sender: replyCache.value[replyId].sender,
        text: replyCache.value[replyId].text,
      }
    }

    // Trigger async fetch
    void fetchReplyMessage(replyId)
  }

  // 4. Check if text in reply segment is valid
  if (replySeg?.data?.text && replySeg.data.text !== '被回复消息') {
    return {
      id: replyId,
      sender: replySeg.data?.sender || '',
      text: replySeg.data.text,
    }
  }

  // 5. Look for an @ segment in this message
  const atSeg = msg.segments?.find(s => s.type === 'at')
  const atTarget = atSeg?.data?.name || (atSeg?.data?.qq ? `QQ ${atSeg.data.qq}` : '')
  if (atTarget) {
    return {
      id: replyId,
      sender: atTarget,
      text: replyId ? `引用消息 #${replyId}` : '引用消息',
    }
  }

  return {
    id: replyId,
    sender: replySeg?.data?.sender || '',
    text: replyId ? `引用消息 #${replyId}` : '引用消息',
  }
}

async function jumpToQuotedMsg(replyId?: string) {
  if (!replyId) return
  const found = messages.value.find(m => m.id === replyId || m.source_message_id === replyId)
  if (found) {
    await scrollToAndHighlight(found.id)
  } else if (currentConv.value) {
    await loadMessagesAround(currentConv.value.id, replyId)
  }
}

function getSenderRole(qq: string): string {
  if (!contextInfo.value?.active_rank) return ''
  const member = contextInfo.value.active_rank.find(m => m.qq === qq)
  return member?.role || ''
}

function roleLabel(role: string): string {
  if (role === 'owner') return '群主'
  if (role === 'admin') return '管理员'
  return ''
}

function formatDateDivider(dateStr?: string): string {
  if (!dateStr) return '时间未记录'
  const d = new Date(dateStr)
  return new Intl.DateTimeFormat('zh-CN', {
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    weekday: 'long',
  }).format(d)
}

function formatTimeExact(dateStr?: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  return new Intl.DateTimeFormat('zh-CN', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(d)
}

function formatDateExact(dateStr?: string): string {
  if (!dateStr) return '—'
  return new Intl.DateTimeFormat('zh-CN', {
    dateStyle: 'medium',
    timeStyle: 'medium',
  }).format(new Date(dateStr))
}

function shortTime(dateStr?: string): string {
  if (!dateStr) return ''
  const d = new Date(dateStr)
  const now = new Date()
  const isToday = d.toDateString() === now.toDateString()
  if (isToday) {
    return new Intl.DateTimeFormat('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false }).format(d)
  }
  return new Intl.DateTimeFormat('zh-CN', { month: 'numeric', day: 'numeric' }).format(d)
}

function formatNumber(num?: number): string {
  return new Intl.NumberFormat('zh-CN').format(num || 0)
}

function formatCompactNumber(num?: number): string {
  if (!num) return '0'
  if (num >= 10000) return (num / 10000).toFixed(1) + 'w'
  if (num >= 1000) return (num / 1000).toFixed(1) + 'k'
  return String(num)
}

function formatBytes(bytes?: number): string {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return (bytes / Math.pow(k, i)).toFixed(1) + ' ' + sizes[i]
}

const faceMap: Record<string, string> = {
  '14': '微笑', '1': '撇嘴', '2': '色', '3': '发呆', '4': '得意', '5': '流泪', '6': '害羞', '7': '闭嘴',
  '8': '睡', '9': '大哭', '10': '尴尬', '11': '发怒', '12': '调皮', '13': '呲牙', '15': '难过',
  '16': '酷', '18': '抓狂', '21': '可爱', '28': '亲亲', '32': '白眼', '96': '冷汗', '76': '点赞',
}
const faceEmojiMap: Record<string, string> = {
  '14': '😊', '1': '😒', '2': '😍', '3': '😶', '4': '😎', '5': '😢', '6': '😳', '7': '🤐',
  '8': '😴', '9': '😭', '10': '😅', '11': '😡', '12': '😜', '13': '😁', '15': '🙁',
  '16': '🕶️', '18': '😫', '21': '🥺', '28': '😘', '32': '🙄', '96': '😰', '76': '👍',
}
function faceName(id: string): string {
  return faceMap[id] || ''
}
function faceEmoji(id: string): string {
  return faceEmojiMap[id] || '😀'
}

function avatarFallback(qq?: string): string {
  if (!qq) return ''
  return `/api/v1/media/avatars/person/${qq}`
}

function researchConversationInGraph() {
  if (!currentConv.value) return
  const target = currentConv.value.conversation_type === 'group'
    ? `group:${currentConv.value.platform_conversation_id}`
    : currentConv.value.platform_conversation_id
  router.push(`/ego-networks?target=${encodeURIComponent(target)}`)
}

function researchPersonInGraph(qq: string) {
  if (!qq) return
  router.push(`/ego-networks?target=${encodeURIComponent(qq)}`)
}

function openPersonDetail(qq?: string) {
  if (!qq) return
  router.push(`/persons?query=${encodeURIComponent(qq)}`)
}

function openGroupDetail(gid?: string) {
  if (!gid) return
  router.push(`/groups?query=${encodeURIComponent(gid)}`)
}

function viewRawEvidence(msg: ChatMessage) {
  selectedEvidence.value = msg
  evidenceDialogOpen.value = true
}

async function copyText(text?: string) {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    showSnack('已复制到剪贴板', 'success')
  } catch (e) {
    console.error('复制失败', e)
  }
}

function copyDeepLink(msg: ChatMessage) {
  const url = `${window.location.origin}/messages?message_id=${msg.id}`
  navigator.clipboard.writeText(url)
  showSnack('已复制消息追溯链接到剪贴板', 'success')
}

function copyMessageId(msg: ChatMessage) {
  navigator.clipboard.writeText(msg.id)
  showSnack('已复制消息 ID 到剪贴板', 'success')
}

// 开启实时自动轮询同步 (长轮询同步机制)
function startSync() {
  syncTimer = setInterval(() => {
    if (autoSync.value && !messagesLoading.value && currentConv.value) {
      loadMessages(currentConv.value.id, true)
      loadConversations(true)
    }
  }, 2500)
}

watch(
  () => route.query,
  async () => {
    await handleRouteQuery()
  },
  { deep: true },
)

onMounted(async () => {
  await loadAccounts()
  await loadConversations()
  await handleRouteQuery()
  startSync()
})

onBeforeUnmount(() => {
  if (syncTimer) clearInterval(syncTimer)
})
</script>

<style scoped>
.qq-chat-app {
  display: grid;
  grid-template-columns: 310px minmax(0, 1fr) 320px;
  gap: 10px;
  height: calc(100vh - 56px);
  padding: 10px 14px 14px;
  background: var(--bg-app);
  box-sizing: border-box;
  overflow: hidden;
}

/* 1. 左侧会话栏 */
.chat-sidebar {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 6px;
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.account-switcher-bar {
  padding: 8px 12px;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.07);
  background: rgba(var(--v-theme-on-surface), 0.02);
}

.active-account-chip {
  display: flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 6px;
  cursor: pointer;
  background: rgba(var(--v-theme-primary), 0.06);
  border: 1px solid rgba(var(--v-theme-primary), 0.2);
  transition: background 0.15s ease;
}

.active-account-chip:hover {
  background: rgba(var(--v-theme-primary), 0.12);
}

.account-name-box {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.account-name {
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.account-qq {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.45);
}

.account-arrow {
  color: rgba(var(--v-theme-on-surface), 0.5);
  margin-left: 4px;
}

.sidebar-top {
  padding: 8px 12px;
}

.sidebar-filters {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.sync-status-indicator {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.45);
}

.sync-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: rgba(var(--v-theme-on-surface), 0.25);
}

.sync-dot.active {
  background: #4ade80;
  box-shadow: 0 0 6px rgba(74, 222, 128, 0.6);
}

.conversation-scroll-list {
  flex: 1;
  overflow-y: auto;
  padding: 6px 8px;
}

.conversation-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 10px;
  border-radius: 6px;
  cursor: pointer;
  margin-bottom: 4px;
  border: 1px solid transparent;
  transition: background-color 0.15s ease, border-color 0.15s ease;
}

.conversation-card:hover {
  background: rgba(var(--v-theme-primary), 0.04);
}

.conversation-card.active {
  background: rgba(var(--v-theme-primary), 0.1);
  border-color: rgba(var(--v-theme-primary), 0.3);
}

.conv-avatar {
  position: relative;
  width: 40px;
  height: 40px;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(var(--v-theme-on-surface), 0.08);
  flex-shrink: 0;
  display: grid;
  place-items: center;
}

.conv-avatar :deep(img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.avatar-letter {
  font-size: 15px;
  font-weight: 700;
  color: rgb(var(--v-theme-primary));
}

.conv-badge {
  position: absolute;
  bottom: 0;
  right: 0;
  font-size: 8.5px;
  font-weight: 700;
  padding: 1px 3px;
  border-radius: 3px;
  line-height: 1;
  color: #fff;
}

.conv-badge.group {
  background: #1976d2;
}

.conv-badge.private {
  background: #00897b;
}

.conv-body {
  flex: 1;
  min-width: 0;
}

.conv-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 2px;
}

.conv-name {
  font-size: 13px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.conv-time {
  font-size: 10.5px;
  color: rgba(var(--v-theme-on-surface), 0.4);
  flex-shrink: 0;
  margin-left: 4px;
}

.conv-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 6px;
}

.conv-last-msg {
  font-size: 11.5px;
  color: rgba(var(--v-theme-on-surface), 0.5);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sender-prefix {
  font-style: normal;
  color: rgba(var(--v-theme-on-surface), 0.7);
}

.conv-count-pill {
  font-size: 9.5px;
  padding: 1px 5px;
  background: rgba(var(--v-theme-on-surface), 0.08);
  color: rgba(var(--v-theme-on-surface), 0.6);
  border-radius: 8px;
  flex-shrink: 0;
}

.sidebar-empty,
.sidebar-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px 16px;
  color: rgba(var(--v-theme-on-surface), 0.4);
  font-size: 12px;
}

/* 2. 中间聊天主视口 */
.chat-viewport {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 8px;
  min-width: 0;
  position: relative;
  background: var(--bg-app);
  border: 1px solid var(--card-border);
}

.chat-active-bar {
  height: 54px;
  padding: 0 16px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--card-border);
  background: rgb(var(--v-theme-surface));
  flex-shrink: 0;
}

.chat-active-info {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}

.chat-active-avatar {
  width: 36px;
  height: 36px;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(var(--v-theme-primary), 0.1);
  display: grid;
  place-items: center;
  color: rgb(var(--v-theme-primary));
  font-weight: 700;
  flex-shrink: 0;
}

.chat-active-name {
  font-size: 14.5px;
  font-weight: 600;
  margin: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 280px;
}

.chat-active-sub {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.45);
  margin: 0;
}

.chat-active-tools {
  display: flex;
  align-items: center;
  gap: 6px;
}

.in-chat-filter-bar {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 16px;
  background: rgba(var(--v-theme-primary), 0.05);
  border-bottom: 1px solid rgba(var(--v-theme-primary), 0.15);
}

.in-chat-input {
  max-width: 380px;
}

/* 消息气泡滚动区 */
.chat-stream-scroll {
  flex: 1;
  overflow-y: auto;
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
}

.history-loader-zone {
  display: flex;
  justify-content: center;
  padding: 4px 0 14px;
}

.history-end-text {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.35);
}

.chat-date-divider {
  display: flex;
  justify-content: center;
  margin: 18px 0 12px;
}

.chat-date-divider span {
  font-size: 10.5px;
  color: rgba(var(--v-theme-on-surface), 0.5);
  background: rgba(var(--v-theme-on-surface), 0.06);
  padding: 2px 10px;
  border-radius: 8px;
}

/* 撤回状态标签 */
.recalled-status-chip {
  font-size: 10px !important;
  height: 18px !important;
  font-weight: 600;
  letter-spacing: 0;
}

/* 普通消息行 */
.chat-msg-row {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  margin-bottom: 8px;
}

/* 已撤回消息行：保持内容可读，并增加撤回样式标记 */
.chat-msg-row.is-recalled .msg-bubble-card {
  opacity: 0.9;
  border: 1px dashed rgba(var(--v-theme-warning), 0.45);
  background: rgba(var(--v-theme-warning), 0.045);
}

.chat-msg-row.is-recalled.is-self .msg-bubble-card {
  border: 1px dashed rgba(var(--v-theme-warning), 0.55);
  background: rgba(var(--v-theme-primary), 0.14);
}

/* 自己发送的消息：右侧对齐 */
.chat-msg-row.is-self {
  flex-direction: row-reverse;
}

.chat-msg-row.is-self .msg-meta-bar {
  flex-direction: row-reverse;
}

.chat-msg-row.is-self .msg-exact-time {
  margin-left: 0;
  margin-right: auto;
}

.chat-msg-row.is-self .msg-content-wrapper {
  align-items: flex-end;
}

.chat-msg-row.is-self .msg-bubble-card {
  background: rgb(var(--v-theme-primary)) !important;
  color: #ffffff !important;
  border: 1px solid rgb(var(--v-theme-primary)) !important;
  border-radius: 8px 3px 8px 8px;
  box-shadow: 0 2px 8px rgba(var(--v-theme-primary), 0.25);
}

.chat-msg-row.is-self .msg-bubble-card .text-content,
.chat-msg-row.is-self .msg-bubble-card p,
.chat-msg-row.is-self .msg-bubble-card span {
  color: #ffffff !important;
}

.chat-msg-row.is-self .msg-bubble-card .reply-card {
  background: rgba(255, 255, 255, 0.15) !important;
  border-left-color: #ffffff !important;
  color: #ffffff !important;
}

.chat-msg-row.is-self .msg-bubble-card .reply-card span,
.chat-msg-row.is-self .msg-bubble-card .reply-card p {
  color: rgba(255, 255, 255, 0.9) !important;
}

.chat-msg-row.is-highlighted .msg-bubble-card {
  border-color: rgb(var(--v-theme-primary));
  box-shadow: 0 0 0 1px rgb(var(--v-theme-primary));
}

.chat-msg-row.is-target-focused .msg-bubble-card {
  border-color: #3b82f6 !important;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.6), 0 0 16px rgba(59, 130, 246, 0.45) !important;
  animation: targetMsgPulse 1.2s ease-in-out infinite alternate !important;
}

@keyframes targetMsgPulse {
  0% {
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.4), 0 0 8px rgba(59, 130, 246, 0.2);
  }
  100% {
    box-shadow: 0 0 0 3.5px rgba(59, 130, 246, 0.85), 0 0 24px rgba(59, 130, 246, 0.65);
  }
}

.msg-sender-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(var(--v-theme-on-surface), 0.08);
  cursor: pointer;
  flex-shrink: 0;
  display: grid;
  place-items: center;
  font-weight: 700;
  font-size: 13px;
  color: rgb(var(--v-theme-primary));
  transition: transform 0.15s ease;
}

.msg-sender-avatar:hover {
  transform: scale(1.05);
}

.msg-content-wrapper {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  min-width: 0;
  max-width: 65%;
}

.msg-meta-bar {
  display: flex;
  align-items: center;
  gap: 5px;
  margin-bottom: 2px;
  font-size: 11px;
}

.sender-name {
  font-size: 11.5px;
  font-weight: 600;
  color: rgba(var(--v-theme-on-surface), 0.7);
  cursor: pointer;
}

.sender-name:hover {
  color: rgb(var(--v-theme-primary));
}

.sender-qq {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.35);
}

.role-badge {
  font-size: 8.5px;
  padding: 0 3px;
  border-radius: 2px;
  line-height: 1.3;
}

.role-badge.owner {
  background: rgba(245, 158, 11, 0.15);
  color: #f59e0b;
  border: 1px solid rgba(245, 158, 11, 0.3);
}

.role-badge.admin {
  background: rgba(59, 130, 246, 0.15);
  color: #60a5fa;
  border: 1px solid rgba(59, 130, 246, 0.3);
}

.msg-exact-time {
  font-size: 9.5px;
  color: rgba(var(--v-theme-on-surface), 0.3);
  margin-left: auto;
}

.msg-bubble-card {
  position: relative;
  width: fit-content;
  max-width: 100%;
  background: rgb(var(--v-theme-surface));
  border: 1px solid var(--card-border);
  border-radius: 3px 12px 12px 12px;
  padding: 8px 12px;
  font-size: 13px;
  line-height: 1.5;
  word-break: break-word;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
  color: var(--text-main);
}

.msg-bubble-card:hover .msg-hover-toolbar {
  opacity: 1;
  pointer-events: auto;
}

.msg-hover-toolbar {
  position: absolute;
  top: -12px;
  right: 6px;
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-on-surface), 0.15);
  border-radius: 4px;
  padding: 1px 3px;
  display: flex;
  align-items: center;
  gap: 1px;
  opacity: 0;
  pointer-events: none;
  transition: opacity 0.15s ease;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);
  z-index: 10;
}

/* 表情回应弹窗 */
.emoji-reaction-picker {
  display: flex;
  gap: 6px;
  padding: 6px 10px;
  border-radius: 8px;
}

.reaction-emoji-btn {
  font-size: 18px;
  cursor: pointer;
  transition: transform 0.12s ease;
}

.reaction-emoji-btn:hover {
  transform: scale(1.3);
}

/* 引用回复卡片 */
.reply-card {
  display: flex;
  align-items: flex-start;
  gap: 6px;
  background: rgba(var(--v-theme-on-surface), 0.05);
  border-left: 3px solid rgb(var(--v-theme-primary));
  padding: 5px 8px;
  border-radius: 4px;
  margin-bottom: 6px;
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.7);
  transition: all 0.15s ease;
  cursor: pointer;
}

.reply-card:hover {
  background: rgba(var(--v-theme-primary), 0.12);
  color: rgb(var(--v-theme-on-surface));
}

.reply-jump-ico {
  opacity: 0.5;
  margin-top: 2px;
  transition: opacity 0.15s ease, transform 0.15s ease;
}

.reply-card:hover .reply-jump-ico {
  opacity: 1;
  transform: translateX(2px);
}

.reply-icon {
  margin-top: 2px;
  color: rgb(var(--v-theme-primary));
}

.reply-info strong {
  color: rgb(var(--v-theme-on-surface));
  margin-right: 4px;
}

/* 富文本段落 */
.seg-text {
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.seg-face {
  color: #f59e0b;
  font-weight: 600;
  margin: 0 2px;
}

.seg-at {
  color: rgb(var(--v-theme-primary));
  background: rgba(var(--v-theme-primary), 0.1);
  padding: 1px 4px;
  border-radius: 3px;
  margin: 0 2px;
  font-weight: 500;
}

/* 原生图片预览与表情包 */
.chat-image-preview {
  margin-top: 6px;
  border-radius: 6px;
  overflow: hidden;
  display: inline-block;
  cursor: zoom-in;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  background: rgba(var(--v-theme-on-surface), 0.02);
  vertical-align: middle;
}

.chat-image-preview.is-sticker {
  border: none;
  background: transparent;
}

.chat-inline-img {
  max-width: 240px;
  max-height: 200px;
  display: block;
  object-fit: cover;
  transition: transform 0.15s ease;
}

.chat-image-preview.is-sticker .chat-inline-img {
  max-width: 140px;
  max-height: 140px;
  object-fit: contain;
}

.chat-inline-img:hover {
  transform: scale(1.02);
}

.img-error-badge {
  font-size: 11px;
  color: rgba(var(--v-theme-on-surface), 0.4);
  padding: 4px 8px;
  display: inline-block;
}

.seg-voice-box,
.seg-file-box {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(var(--v-theme-on-surface), 0.035);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.07);
  padding: 6px 10px;
  border-radius: 6px;
  margin-top: 6px;
}

.seg-card-box {
  display: flex;
  align-items: center;
  color: rgba(var(--v-theme-on-surface), 0.6);
  font-size: 11.5px;
}

.msg-media-wall {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(72px, 1fr));
  gap: 5px;
  margin-top: 6px;
}

.media-wall-thumbnail {
  height: 72px;
  border-radius: 4px;
  overflow: hidden;
  cursor: zoom-in;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.wall-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* 3. 发送框容器 (Native QQ Chat Box) */
.chat-send-container {
  display: flex;
  flex-direction: column;
  border-top: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  background: rgba(var(--v-theme-on-surface), 0.015);
  padding: 6px 14px 10px;
  flex-shrink: 0;
}

.reply-target-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  background: rgba(var(--v-theme-primary), 0.08);
  border-left: 3px solid rgb(var(--v-theme-primary));
  padding: 4px 10px;
  border-radius: 4px;
  margin-bottom: 6px;
  font-size: 11.5px;
}

.reply-target-text {
  color: rgba(var(--v-theme-on-surface), 0.6);
  max-width: 480px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chat-send-toolbar {
  display: flex;
  align-items: center;
  gap: 4px;
  margin-bottom: 4px;
}

.send-shortcut-hint {
  font-size: 10.5px;
  color: rgba(var(--v-theme-on-surface), 0.35);
}

.attached-images-strip {
  display: flex;
  gap: 6px;
  margin-left: 6px;
}

.chat-input-row {
  display: flex;
  align-items: flex-end;
  gap: 10px;
}

.chat-textarea {
  flex: 1;
  height: 58px;
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.1);
  border-radius: 6px;
  padding: 8px 10px;
  color: rgb(var(--v-theme-on-surface));
  font-size: 13px;
  line-height: 1.5;
  resize: none;
  outline: none;
  font-family: inherit;
  transition: border-color 0.15s ease;
}

.chat-textarea:focus {
  border-color: rgb(var(--v-theme-primary));
}

.send-btn {
  height: 38px;
  padding: 0 16px;
}

/* 表情调色板 */
.qq-emoji-palette {
  padding: 10px;
  border-radius: 8px;
  max-width: 300px;
}

.emoji-palette-header {
  font-size: 11px;
  font-weight: 700;
  color: rgba(var(--v-theme-on-surface), 0.5);
  margin-bottom: 8px;
}

.emoji-palette-grid {
  display: grid;
  grid-template-columns: repeat(6, 1fr);
  gap: 6px;
}

.emoji-palette-item {
  font-size: 20px;
  padding: 4px;
  border-radius: 4px;
  border: none;
  background: transparent;
  cursor: pointer;
  transition: background 0.12s ease;
}

.emoji-palette-item:hover {
  background: rgba(var(--v-theme-primary), 0.15);
}

/* 欢迎引导空状态 */
.chat-welcome-state {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 36px;
  text-align: center;
}

.welcome-icon-box {
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: rgba(var(--v-theme-primary), 0.08);
  border: 1px solid rgba(var(--v-theme-primary), 0.2);
  display: grid;
  place-items: center;
  margin-bottom: 18px;
}

.chat-welcome-state h2 {
  font-size: 18px;
  font-weight: 600;
  margin: 0 0 8px;
}

.chat-welcome-state p {
  font-size: 12.5px;
  color: rgba(var(--v-theme-on-surface), 0.5);
  max-width: 420px;
  margin: 0 0 24px;
}

.welcome-stats-row {
  display: flex;
  gap: 12px;
}

.welcome-stat-pill {
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 6px;
  padding: 10px 16px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.welcome-stat-pill span {
  font-size: 10.5px;
  color: rgba(var(--v-theme-on-surface), 0.45);
}

.welcome-stat-pill strong {
  font-size: 18px;
  color: rgb(var(--v-theme-on-surface));
}

/* 4. 右侧研判侧边栏 */
.chat-inspector {
  display: flex;
  flex-direction: column;
  overflow: hidden;
  border-radius: 6px;
  background: rgb(var(--v-theme-surface));
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.inspector-top-tabs {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 4px 8px;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), 0.08);
}

.inspector-scroll-area {
  flex: 1;
  overflow-y: auto;
  padding: 12px;
}

.pane-header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}

.pane-header-row strong {
  font-size: 12px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
}

.pane-header-row span {
  font-size: 10.5px;
  color: rgba(var(--v-theme-on-surface), 0.4);
}

.active-member-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.member-rank-item {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 6px 8px;
  background: rgba(var(--v-theme-on-surface), 0.02);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.05);
  border-radius: 4px;
  cursor: pointer;
  transition: background-color 0.15s ease;
}

.member-rank-item:hover {
  background: rgba(var(--v-theme-primary), 0.06);
}

.rank-pos {
  font-size: 10.5px;
  font-weight: 700;
  color: rgba(var(--v-theme-on-surface), 0.35);
  width: 14px;
  text-align: center;
}

.rank-pos.top {
  color: #f59e0b;
}

.member-avatar {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(var(--v-theme-on-surface), 0.08);
  flex-shrink: 0;
  display: grid;
  place-items: center;
  font-size: 11px;
  color: rgb(var(--v-theme-primary));
}

.member-meta {
  flex: 1;
  min-width: 0;
}

.member-meta-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 2px;
}

.member-name {
  font-size: 11.5px;
  font-weight: 600;
  color: rgb(var(--v-theme-on-surface));
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.member-count {
  font-size: 10.5px;
  color: rgb(var(--v-theme-primary));
  font-weight: 600;
}

.member-ratio-bar {
  height: 3px;
  background: rgba(var(--v-theme-on-surface), 0.07);
  border-radius: 2px;
  overflow: hidden;
  margin-bottom: 2px;
}

.member-ratio-fill {
  height: 100%;
  background: rgb(var(--v-theme-primary));
  border-radius: 2px;
}

.member-meta-sub {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 9.5px;
  color: rgba(var(--v-theme-on-surface), 0.35);
}

.media-grid-wall {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 5px;
}

.media-grid-item {
  position: relative;
  aspect-ratio: 1;
  border-radius: 4px;
  overflow: hidden;
  background: rgba(var(--v-theme-on-surface), 0.05);
  cursor: zoom-in;
  border: 1px solid rgba(var(--v-theme-on-surface), 0.07);
}

.grid-wall-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.media-overlay-tag {
  position: absolute;
  inset: auto 0 0 0;
  padding: 3px 4px;
  background: linear-gradient(to top, rgba(0, 0, 0, 0.8), transparent);
  font-size: 9px;
  color: #fff;
  display: flex;
  justify-content: space-between;
  opacity: 0;
  transition: opacity 0.15s ease;
}

.media-grid-item:hover .media-overlay-tag {
  opacity: 1;
}

.summary-cards-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 6px;
}

.summary-card {
  background: rgba(var(--v-theme-on-surface), 0.025);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.06);
  border-radius: 4px;
  padding: 8px 10px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.summary-card.span-full {
  grid-column: 1 / -1;
}

.card-label {
  font-size: 10px;
  color: rgba(var(--v-theme-on-surface), 0.4);
}

.summary-card strong {
  font-size: 12px;
  color: rgb(var(--v-theme-on-surface));
}

.summary-card code,
.uuid-text {
  font-size: 10.5px;
  color: rgb(var(--v-theme-primary));
  background: rgba(var(--v-theme-primary), 0.08);
  padding: 1px 4px;
  border-radius: 3px;
  overflow-wrap: break-word;
}

.pane-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 36px 12px;
  color: rgba(var(--v-theme-on-surface), 0.35);
  font-size: 11.5px;
}

.evidence-pre {
  background: rgba(0, 0, 0, 0.3);
  padding: 10px;
  border-radius: 4px;
  font-size: 10.5px;
  color: rgb(var(--v-theme-primary));
  max-height: 320px;
  overflow: auto;
}

/* 5. 灯箱样式 */
.lightbox-modal-card {
  background: #14181f !important;
  color: #fff;
  overflow: hidden;
}

.lightbox-top-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 14px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
  background: rgba(0, 0, 0, 0.4);
}

.lightbox-actions {
  display: flex;
  align-items: center;
  gap: 2px;
}

.lightbox-stage {
  min-height: 480px;
  max-height: 76vh;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  padding: 16px;
  background: rgba(0, 0, 0, 0.6);
}

.lightbox-img {
  max-width: 100%;
  max-height: 70vh;
  object-fit: contain;
  transition: transform 0.2s cubic-bezier(0.2, 0, 0, 1);
}

/* 响应式断点适配 */
@media (max-width: 1200px) {
  .qq-chat-app {
    grid-template-columns: 280px minmax(0, 1fr);
  }
  .chat-inspector {
    display: none;
  }
}

@media (max-width: 768px) {
  .qq-chat-app {
    grid-template-columns: 1fr;
  }
  .chat-sidebar {
    display: none;
  }
}

.image-action-overlay {
  position: absolute;
  bottom: 4px;
  right: 4px;
  background: rgba(0, 0, 0, 0.65);
  color: #fff;
  font-size: 10px;
  padding: 2px 6px;
  border-radius: 4px;
  display: flex;
  align-items: center;
  opacity: 0;
  transition: opacity 0.2s ease;
  cursor: pointer;
}
.chat-image-preview:hover .image-action-overlay {
  opacity: 1;
}
.thread-card {
  background: rgba(var(--v-theme-surface), 0.7);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
}
.thread-card:hover {
  background: rgba(var(--v-theme-primary), 0.04);
  border-color: rgba(var(--v-theme-primary), 0.3);
}
.thread-card.active {
  border-color: rgb(var(--v-theme-primary));
  background: rgba(var(--v-theme-primary), 0.08);
}
</style>

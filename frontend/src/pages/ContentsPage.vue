<template>
  <div class="page-wrap content-page records-page">
    <header class="content-page-header">
      <div><span class="eyebrow">QZONE</span><h1>空间动态</h1><p>{{ totalLabel }} · 按时间回看内容与互动证据</p></div>
      <div class="header-actions">
        <v-btn v-if="isNarrow && detail" icon="mdi-dock-right" variant="tonal" title="查看当前内容" @click="detailOpen = true" />
        <v-btn icon="mdi-refresh" variant="tonal" title="刷新" :loading="loading" @click="resetAndLoad" />
      </div>
    </header>

    <section class="content-toolbar records-toolbar data-surface">
      <div class="content-filter-primary">
        <v-text-field v-model.trim="query" placeholder="搜索正文、昵称或 QQ" prepend-inner-icon="mdi-magnify" hide-details clearable density="compact" @keyup.enter="resetAndLoad" @click:clear="resetAndLoad" />
        <v-text-field v-model.trim="author" placeholder="作者昵称或 QQ" prepend-inner-icon="mdi-account-search-outline" hide-details clearable density="compact" @keyup.enter="resetAndLoad" @click:clear="resetAndLoad" />
      </div>
      <div class="content-filter-secondary">
        <v-select v-model="dateRange" :items="dateOptions" hide-details density="compact" />
        <v-select v-model="mediaFilter" :items="mediaOptions" hide-details density="compact" />
        <v-select v-model="completenessFilter" :items="completenessOptions" hide-details density="compact" />
        <v-btn-toggle v-model="contextType" mandatory density="compact" color="primary" variant="outlined">
          <v-btn value="qzone_post">动态</v-btn><v-btn value="qzone_comment">评论</v-btn><v-btn value="">全部</v-btn>
        </v-btn-toggle>
      </div>
    </section>

    <v-alert v-if="error" type="error" variant="tonal" class="mt-3" closable @click:close="error = ''">{{ error }}</v-alert>

    <div class="research-layout">
      <!-- 左侧动态时间流 -->
      <main class="feed-column data-surface records-surface">
        <div class="feed-column-header"><strong>内容时间流</strong><span>{{ contents.length }} 条已载入</span></div>
        <section class="feed-list" :class="{ 'is-loading': loading && !contents.length }">
          <article
            v-for="content in visibleContents"
            :key="content.id"
            :id="'content-item-' + content.id"
            class="feed-item"
            :class="{
              selected: detail?.id === content.id,
              'is-target-focused': targetContentId === content.id || (content.platform_content_id && targetContentId === content.platform_content_id),
            }"
            @click="open(content)"
          >
            <div class="author-avatar"><AuthImage :src="content.author_avatar || avatarFallback(content.author_qq)" :fallback-srcs="avatarFallbacks(content.author_qq)" :alt="content.author_name"><span>{{ initial(content.author_name || content.author_qq) }}</span></AuthImage></div>
            <div class="feed-main">
              <div class="feed-head">
                <div><strong>{{ cleanDisplayText(content.author_name) || `QQ ${content.author_qq}` }}</strong><small>QQ {{ content.author_qq || "未知" }} · {{ formatDate(content.published_at) }}</small></div>
                <div class="feed-badges"><v-chip size="x-small" variant="tonal">{{ content.context_type === "qzone_comment" ? "评论" : "动态" }}</v-chip><v-chip size="x-small" :color="completenessColor(content)" variant="tonal">{{ completenessLabel(content) }}</v-chip></div>
              </div>
              <p class="feed-body">{{ cleanDisplayText(content.body) || "[无文本内容]" }}</p>

              <!-- 外链分享卡片 -->
              <div v-if="extractShareInfo(content)" class="feed-share-card" @click.stop="openShareLink(extractShareInfo(content)?.url)">
                <v-icon icon="mdi-link-variant" size="20" color="primary" class="share-card-icon" />
                <div class="share-card-info">
                  <strong class="share-card-title">{{ cleanDisplayText(extractShareInfo(content)?.title) }}</strong>
                  <small v-if="extractShareInfo(content)?.subtitle" class="share-card-sub">{{ cleanDisplayText(extractShareInfo(content)?.subtitle) }}</small>
                  <a v-if="extractShareInfo(content)?.url" :href="extractShareInfo(content)?.url" target="_blank" rel="noopener noreferrer" class="share-card-link" @click.stop>
                    <v-icon icon="mdi-open-in-new" size="12" class="mr-1" />
                    {{ extractShareInfo(content)?.url }}
                  </a>
                </div>
              </div>

              <ContentMedia v-if="content.media?.length" :items="content.media.slice(0, 9)" @archive="archive" />
              <div class="feed-foot">
                <span><v-icon icon="mdi-thumb-up-outline" size="16" />{{ metric(content, "likenum", "_likenum", "like_count") }}</span>
                <span><v-icon icon="mdi-comment-outline" size="16" />{{ metric(content, "cmtnum", "_cmtnum", "comment_count") }}</span>
                <span><v-icon icon="mdi-share-outline" size="16" />{{ metric(content, "fwdnum", "_fwdnum", "forward_count") }}</span>
                <span v-if="content.media?.length"><v-icon icon="mdi-paperclip" size="16" />{{ content.media.length }}</span>
              </div>
            </div>
          </article>
        </section>

        <div v-if="loadingMore" class="feed-progress"><v-progress-circular indeterminate size="22" width="2" /><span>正在加载</span></div>
        <button v-else-if="hasMore" ref="loadSentinel" class="load-more" @click="loadMore">加载更多</button>
        <div v-else-if="contents.length" class="feed-end">已加载 {{ contents.length }} 条</div>
        <div v-if="!loading && !visibleContents.length" class="feed-empty"><v-icon icon="mdi-image-search-outline" size="30" /><strong>没有符合筛选条件的动态</strong></div>
      </main>

      <!-- 右侧精美详情研判侧边栏 -->
      <aside v-if="!isNarrow" class="research-sidebar data-surface">
        <template v-if="detail">
          <!-- 头部作者名片区 -->
          <div class="sidebar-hero">
            <div class="hero-top">
              <div class="hero-avatar">
                <AuthImage :src="detail.author_avatar || avatarFallback(detail.author_qq)" :fallback-srcs="avatarFallbacks(detail.author_qq)" :alt="detail.author_name">
                  <span class="avatar-letter">{{ initial(detail.author_name || detail.author_qq) }}</span>
                </AuthImage>
              </div>
              <div class="hero-info">
                <div class="hero-title-row">
                  <h3 class="hero-name">{{ cleanDisplayText(detail.author_name) || `QQ ${detail.author_qq}` }}</h3>
                  <v-chip size="x-small" :color="completenessColor(detail)" variant="tonal" class="hero-chip">
                    {{ completenessLabel(detail) }}
                  </v-chip>
                </div>
                <div class="hero-meta-row">
                  <span class="hero-qq">QQ {{ detail.author_qq || '未知' }}</span>
                  <span class="meta-dot">·</span>
                  <span class="hero-time">{{ formatDate(detail.published_at) }}</span>
                </div>
              </div>
            </div>

            <div class="hero-actions-strip">
              <v-btn
                color="primary"
                variant="flat"
                size="small"
                prepend-icon="mdi-graph-outline"
                :disabled="!detail.author_qq"
                class="flex-grow-1"
                @click="researchAuthor"
              >
                在图谱中分析此人
              </v-btn>
              <v-btn
                v-if="detail.author_qq"
                variant="tonal"
                size="small"
                icon="mdi-forum-outline"
                title="去聊天工作台检索该作者消息"
                @click="openInMessages(String(detail.author_qq))"
              />
              <v-btn
                v-if="detail.author_qq"
                variant="tonal"
                size="small"
                icon="mdi-account-outline"
                title="查看人员档案"
                @click="openPersonDetail(String(detail.author_qq))"
              />
              <v-btn
                variant="tonal"
                size="small"
                icon="mdi-link-variant"
                title="复制该动态追溯链接"
                @click="copyContentDeepLink(detail)"
              />
            </div>
          </div>

          <v-progress-linear v-if="detailLoading" indeterminate color="primary" height="2" />

          <!-- 可滚动内容主体（聚焦深度研判：点赞、评论、元数据） -->
          <div class="sidebar-scroll-body">
            <!-- 分类页签切换置顶 -->
            <div class="sidebar-tabs-wrap">
              <v-tabs v-model="activeTab" density="compact" grow color="primary" class="sidebar-tabs">
                <v-tab value="likes">
                  <v-icon icon="mdi-thumb-up-outline" size="15" class="mr-1" />
                  点赞 {{ likes.length || metric(detail, "likenum", "_likenum", "like_count") }}
                </v-tab>
                <v-tab value="comments">
                  <v-icon icon="mdi-comment-outline" size="15" class="mr-1" />
                  评论 {{ detail.comments?.length || 0 }}
                </v-tab>
                <v-tab value="status">
                  <v-icon icon="mdi-information-outline" size="15" class="mr-1" />
                  元数据
                </v-tab>
              </v-tabs>
            </div>

            <!-- Tab 1: 评论列表 -->
            <div v-if="activeTab === 'comments'" class="tab-pane">
              <div v-if="detail.comments?.length" class="comments-list">
                <div v-for="c in detail.comments" :key="c.id" class="comment-item-card">
                  <div class="comment-item-head">
                    <div class="comment-user">
                      <div class="liker-avatar-box">
                        <AuthImage :src="c.author_avatar || avatarFallback(c.author_qq)" :fallback-srcs="avatarFallbacks(c.author_qq)" :alt="c.author_name">
                          <span class="avatar-letter-mini">{{ initial(c.author_name || c.author_qq) }}</span>
                        </AuthImage>
                      </div>
                      <div>
                        <strong class="comment-user-name">{{ cleanDisplayText(c.author_name) || `QQ ${c.author_qq}` }}</strong>
                        <small class="comment-user-time">QQ {{ c.author_qq || '未知' }} · {{ formatDate(c.published_at) }}</small>
                      </div>
                    </div>
                    <v-btn
                      v-if="c.author_qq"
                      icon="mdi-graph-outline"
                      size="x-small"
                      variant="text"
                      color="primary"
                      title="在图谱中研究此人"
                      @click="researchQQ(String(c.author_qq))"
                    />
                  </div>
                  <p class="comment-item-body">{{ cleanDisplayText(c.body) || '[无文本内容]' }}</p>
                  <ContentMedia v-if="c.media?.length" :items="c.media" variant="compact" @archive="archive" />
                </div>
              </div>
              <div v-else class="sidebar-empty-box">
                <v-icon icon="mdi-comment-outline" size="32" class="empty-ico" />
                <span>暂无已采集评论</span>
              </div>
            </div>

            <!-- Tab 2: 点赞人员 -->
            <div v-if="activeTab === 'likes'" class="tab-pane">
              <div v-if="likes.length" class="likers-list">
                <div v-for="(like, index) in likes" :key="like.id || like.uin || index" class="liker-card">
                  <div class="liker-avatar-box">
                    <AuthImage :src="like.avatar_uri || like.avatar_url || avatarFallback(like.uin || like.qq)" :fallback-srcs="avatarFallbacks(like.uin || like.qq)" :alt="like.nickname || like.name">
                      <span class="avatar-letter-mini">{{ initial(like.nickname || like.name || like.uin || like.qq) }}</span>
                    </AuthImage>
                  </div>
                  <div class="liker-detail">
                    <strong class="liker-name">{{ cleanDisplayText(like.nickname || like.name) || `QQ ${like.uin || like.qq || '未知'}` }}</strong>
                    <small class="liker-sub">
                      QQ {{ like.uin || like.qq || '未知' }}
                      <span v-if="like.occurred_at"> · {{ formatDate(like.occurred_at) }}</span>
                    </small>
                  </div>
                  <div class="liker-ops">
                    <v-btn
                      v-if="like.uin || like.qq"
                      icon="mdi-graph-outline"
                      size="small"
                      variant="tonal"
                      color="primary"
                      title="在关系图谱中研究"
                      @click="researchQQ(String(like.uin || like.qq))"
                    />
                    <v-btn
                      v-if="like.uin || like.qq"
                      icon="mdi-account-outline"
                      size="small"
                      variant="tonal"
                      title="查看人员档案"
                      @click="openPersonDetail(String(like.uin || like.qq))"
                    />
                  </div>
                </div>
              </div>
              <div v-else class="sidebar-empty-box">
                <div class="metric-circle">
                  <strong>{{ metric(detail, "likenum", "_likenum", "like_count") }}</strong>
                  <span>已记录赞</span>
                </div>
                <span class="mt-2">点赞人员明细尚未采集或已隐藏</span>
              </div>
            </div>

            <!-- Tab 3: 元数据与溯源 -->
            <div v-if="activeTab === 'status'" class="tab-pane">
              <div class="meta-card-grid">
                <div class="meta-card">
                  <span class="meta-label">完整度评估</span>
                  <v-chip size="small" :color="completenessColor(detail)" variant="tonal">{{ completenessLabel(detail) }}</v-chip>
                </div>
                <div class="meta-card">
                  <span class="meta-label">评论互动</span>
                  <span class="meta-val">{{ detail.comments?.length || 0 }} / {{ metric(detail, "cmtnum", "_cmtnum", "comment_count") || '未知' }}</span>
                </div>
                <div class="meta-card">
                  <span class="meta-label">点赞互动</span>
                  <span class="meta-val">{{ likes.length }} / {{ metric(detail, "likenum", "_likenum", "like_count") || '未知' }}</span>
                </div>
                <div class="meta-card">
                  <span class="meta-label">媒体归档进度</span>
                  <span class="meta-val">{{ archivedCount(detail.media) }} / {{ detail.media?.length || 0 }}</span>
                </div>
                <div class="meta-card span-full">
                  <span class="meta-label">内容唯一标识 (TID)</span>
                  <code class="meta-code">{{ detail.content_id || detail.id }}</code>
                </div>
                <div v-if="detail.raw_record_id" class="meta-card span-full">
                  <span class="meta-label">原始证据记录 ID</span>
                  <code class="meta-code">{{ detail.raw_record_id }}</code>
                </div>
              </div>
            </div>
          </div>
        </template>

        <!-- 未选择任何动态时的空面板 -->
        <div v-else class="sidebar-unselected">
          <div class="unselected-icon-ring">
            <v-icon icon="mdi-cursor-default-click-outline" size="36" color="primary" />
          </div>
          <h3>选择一篇动态</h3>
          <p>在左侧列表中点击任意动态，在此查看完整正文、互动人员、点赞明细与社交下钻</p>
        </div>
      </aside>
    </div>

    <!-- 移动端侧边抽屉 -->
    <div v-if="isNarrow && detailOpen" class="mobile-scrim" @click.self="detailOpen = false">
      <aside class="research-sidebar mobile-sidebar data-surface">
        <template v-if="detail">
          <div class="sidebar-hero">
            <div class="hero-top">
              <div class="hero-avatar">
                <AuthImage :src="detail.author_avatar" :alt="detail.author_name">
                  <span class="avatar-letter">{{ initial(detail.author_name || detail.author_qq) }}</span>
                </AuthImage>
              </div>
              <div class="hero-info">
                <div class="hero-title-row">
                  <h3 class="hero-name">{{ detail.author_name || `QQ ${detail.author_qq}` }}</h3>
                  <v-chip size="x-small" :color="completenessColor(detail)" variant="tonal" class="hero-chip">
                    {{ completenessLabel(detail) }}
                  </v-chip>
                </div>
                <div class="hero-meta-row">
                  <span class="hero-qq">QQ {{ detail.author_qq || '未知' }}</span>
                  <span class="meta-dot">·</span>
                  <span class="hero-time">{{ formatDate(detail.published_at) }}</span>
                </div>
              </div>
              <v-btn icon="mdi-close" variant="text" size="small" @click="detailOpen = false" />
            </div>

            <div class="hero-actions-strip mt-2">
              <v-btn
                color="primary"
                variant="flat"
                size="small"
                prepend-icon="mdi-graph-outline"
                :disabled="!detail.author_qq"
                class="flex-grow-1"
                @click="researchAuthor"
              >
                在图谱中分析
              </v-btn>
              <v-btn
                v-if="detail.author_qq"
                variant="tonal"
                size="small"
                icon="mdi-account-outline"
                @click="openPersonDetail(String(detail.author_qq))"
              />
            </div>
          </div>

          <div class="sidebar-scroll-body">
            <div class="detail-quote-card">
              <p class="detail-quote-text">{{ detail.body || "[无文本内容]" }}</p>
            </div>

            <div v-if="extractShareInfo(detail)" class="sidebar-share-card" @click="openShareLink(extractShareInfo(detail)?.url)">
              <div class="share-icon-wrap">
                <v-icon icon="mdi-link-variant" size="20" color="primary" />
              </div>
              <div class="share-text-wrap">
                <strong class="share-main-title">{{ extractShareInfo(detail)?.title }}</strong>
                <small v-if="extractShareInfo(detail)?.subtitle" class="share-sub-title">{{ extractShareInfo(detail)?.subtitle }}</small>
              </div>
            </div>

            <div v-if="detail.media?.length" class="sidebar-section">
              <ContentMedia :items="detail.media" variant="stack" @archive="archive" />
            </div>

            <div class="sidebar-tabs-wrap">
              <v-tabs v-model="activeTab" density="compact" grow color="primary">
                <v-tab value="comments">评论 {{ detail.comments?.length || 0 }}</v-tab>
                <v-tab value="likes">点赞 {{ likes.length }}</v-tab>
                <v-tab value="status">元数据</v-tab>
              </v-tabs>
            </div>

            <div v-if="activeTab === 'comments'" class="tab-pane">
              <div v-if="detail.comments?.length" class="comments-list">
                <div v-for="c in detail.comments" :key="c.id" class="comment-item-card">
                  <div class="comment-item-head">
                    <strong class="comment-user-name">{{ c.author_name || `QQ ${c.author_qq}` }}</strong>
                    <small class="comment-user-time">{{ formatDate(c.published_at) }}</small>
                  </div>
                  <p class="comment-item-body">{{ c.body || '[无文本内容]' }}</p>
                </div>
              </div>
              <div v-else class="sidebar-empty-box">暂无已采集评论</div>
            </div>

            <div v-if="activeTab === 'likes'" class="tab-pane">
              <div v-if="likes.length" class="likers-list">
                <div v-for="(like, index) in likes" :key="like.id || like.uin || index" class="liker-card">
                  <div class="liker-detail">
                    <strong class="liker-name">{{ like.nickname || like.name || `QQ ${like.uin || like.qq}` }}</strong>
                    <small class="liker-sub">QQ {{ like.uin || like.qq }}</small>
                  </div>
                  <v-btn icon="mdi-graph-outline" size="small" variant="tonal" color="primary" @click="researchQQ(String(like.uin || like.qq))" />
                </div>
              </div>
              <div v-else class="sidebar-empty-box">暂无点赞人员记录</div>
            </div>
          </div>
        </template>
      </aside>
    </div>

    <v-snackbar v-model="snack" timeout="2400">{{ snackText || "已加入下载队列" }}</v-snackbar>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useDisplay } from "vuetify";
import { useRoute, useRouter } from "vue-router";
import { api } from "@/services/api";
import { useGraphWorkspaceStore } from "@/stores/graphWorkspace";
import AuthImage from "@/components/AuthImage.vue";
import ContentMedia from "@/components/ContentMedia.vue";
import { useListStateStore } from "@/stores/listState";
import { useSystemCapabilitiesStore } from "@/stores/systemCapabilities";
import { cleanDisplayText } from "@/utils/text";

const route = useRoute(), router = useRouter(), graphWorkspace = useGraphWorkspaceStore();
const { width } = useDisplay();
const isNarrow = computed(() => width.value <= 1280);
const listState = useListStateStore();
const systemCapabilities = useSystemCapabilitiesStore();
const pageSize = ref(0);
const PAGE_KEY = "contents";
const restored = listState.load(PAGE_KEY, { query: "", author: "", contextType: "qzone_post", dateRange: "all", mediaFilter: "all", completenessFilter: "all" });
const contents = ref<any[]>([]), query = ref(String(restored.query || "")), author = ref(String(restored.author || "")), contextType = ref(String(restored.contextType || "qzone_post"));
const dateRange = ref(String(restored.dateRange || "all")), mediaFilter = ref(String(restored.mediaFilter || "all")), completenessFilter = ref(String(restored.completenessFilter || "all"));
const loading = ref(false), loadingMore = ref(false), error = ref(""), detailOpen = ref(false), detailLoading = ref(false), detail = ref<any>(null), snack = ref(false), snackText = ref("");
const targetContentId = ref("");
const offset = ref(0), nextCursor = ref(""), hasMore = ref(true), total = ref<number | null>(null), activeTab = ref("comments");
const loadSentinel = ref<HTMLElement | null>(null);
let observer: IntersectionObserver | null = null;
let loadGeneration = 0;

const dateOptions = [{ title: "全部时间", value: "all" }, { title: "最近 7 天", value: "7d" }, { title: "最近 30 天", value: "30d" }, { title: "最近一年", value: "365d" }];
const mediaOptions = [{ title: "全部媒体", value: "all" }, { title: "含媒体", value: "with" }, { title: "无媒体", value: "without" }, { title: "图片 / 表情", value: "image" }, { title: "音视频", value: "av" }];
const completenessOptions = [{ title: "全部完整度", value: "all" }, { title: "完整", value: "complete" }, { title: "部分采集", value: "partial" }, { title: "不可用", value: "unavailable" }];
const totalLabel = computed(() => total.value === null ? `已加载 ${contents.value.length}` : `${new Intl.NumberFormat("zh-CN").format(total.value)} 条`);

const visibleContents = computed(() => contents.value.filter((item) => {
  const authorText = `${item.author_name || ""} ${item.author_qq || ""}`.toLowerCase();
  if (author.value && !authorText.includes(author.value.toLowerCase())) return false;
  if (dateRange.value !== "all") {
    const days = Number(dateRange.value.replace("d", ""));
    const at = new Date(item.published_at || 0).getTime();
    if (!at || at < Date.now() - days * 86400000) return false;
  }
  const media = Array.isArray(item.media) ? item.media : [];
  if (mediaFilter.value === "with" && !media.length) return false;
  if (mediaFilter.value === "without" && media.length) return false;
  if (mediaFilter.value === "image" && !media.some((m: any) => ["image", "sticker", "avatar"].includes(m.kind) || String(m.mime_type || "").startsWith("image/"))) return false;
  if (mediaFilter.value === "av" && !media.some((m: any) => ["audio", "video", "record"].includes(m.kind) || /^(audio|video)\//.test(m.mime_type || ""))) return false;
  return completenessFilter.value === "all" || completeness(item) === completenessFilter.value;
}));

const likes = computed(() => {
  const source = detail.value?.likes || detail.value?.likers || detail.value?.metadata?.likes || detail.value?.metadata?.like_list || [];
  return Array.isArray(source) ? source : [];
});

function avatarFallback(qq?: string | number): string {
  if (!qq) return "";
  const s = String(qq).trim();
  if (!s) return "";
  return `/api/v1/media/avatars/person/${encodeURIComponent(s)}`;
}

function avatarFallbacks(qq?: string | number): string[] {
  if (!qq) return [];
  const s = String(qq).trim();
  if (!s) return [];
  return [
    `/api/v1/media/avatars/person/${encodeURIComponent(s)}`,
    `https://q1.qlogo.cn/g?b=qq&nk=${encodeURIComponent(s)}&s=640`,
    `https://q.qlogo.cn/headimg_dl?dst_uin=${encodeURIComponent(s)}&spec=640`,
  ];
}

async function fetchPage(append: boolean) {
  const generation = loadGeneration;
  append ? loadingMore.value = true : loading.value = true;
  error.value = "";
  try {
    const response = (await api.get("/api/v1/contents", { params: { q: query.value, author: author.value, context_type: contextType.value, date_range: dateRange.value, media: mediaFilter.value, completeness: completenessFilter.value, limit: pageSize.value, offset: offset.value, cursor: nextCursor.value || undefined } })).data;
    if (generation !== loadGeneration) return;
    const rows = Array.isArray(response.data) ? response.data : Array.isArray(response.data?.data) ? response.data.data : [];
    const pageInfo = response.page || response.pagination || {};
    const known = new Set(contents.value.map((item) => item.id));
    const incoming = append ? rows.filter((item: any) => !known.has(item.id)) : rows;
    contents.value = append ? [...contents.value, ...incoming] : incoming;
    offset.value += rows.length;
    nextCursor.value = String(pageInfo.next_cursor || response.next_cursor || "");
    total.value = Number.isFinite(Number(pageInfo.total ?? response.total)) ? Number(pageInfo.total ?? response.total) : null;
    hasMore.value = nextCursor.value ? true : rows.length >= pageSize.value;
    if (!detail.value && contents.value.length) await open(contents.value[0], false);
  } catch (e: any) { error.value = e.response?.data?.error || "空间动态加载失败"; hasMore.value = false; }
  finally { loading.value = false; loadingMore.value = false; await nextTick(); observeSentinel(); }
}
function resetAndLoad() { loadGeneration++; contents.value = []; offset.value = 0; nextCursor.value = ""; hasMore.value = true; total.value = null; fetchPage(false); }
function loadMore() { if (!loading.value && !loadingMore.value && hasMore.value) fetchPage(true); }
async function open(content: any, showMobile = true) {
  detail.value = content;
  if (showMobile && isNarrow.value) detailOpen.value = true;
  detailLoading.value = true;
  try {
    const res = (await api.get(`/api/v1/contents/${content.id}`)).data.data;
    detail.value = res;
    if (res?.likes?.length && !res?.comments?.length) {
      activeTab.value = "likes";
    } else if (res?.comments?.length && !res?.likes?.length) {
      activeTab.value = "comments";
    }
  } catch (e: any) {
    error.value = e.response?.data?.error || "内容详情加载失败";
  } finally {
    detailLoading.value = false;
  }
}
async function archive(media: any) {
  try { await api.post(`/api/v1/media/references/${media.id || media.reference_id}/download`); media.status = "pending"; showNotification("已加入媒体下载队列"); }
  catch (e: any) { error.value = e.response?.data?.error || "加入下载队列失败"; }
}
function researchAuthor() { const qq = String(detail.value?.author_qq || ""); if (!qq) return; researchQQ(qq); }
function researchQQ(qq: string) { if (!qq) return; graphWorkspace.targetQQ = qq; graphWorkspace.requestBuild(); void router.push("/ego-networks"); }
function openPersonDetail(qq: string) { if (!qq) return; void router.push({ path: "/persons", query: { query: qq } }); }
function openInMessages(qq: string) { if (!qq) return; void router.push({ path: "/messages", query: { qq } }); }

function showNotification(msg: string) {
  snackText.value = msg;
  snack.value = true;
}

function copyContentDeepLink(item?: any) {
  const c = item || detail.value;
  if (!c?.id) return;
  const url = `${window.location.origin}/contents?content_id=${c.id}`;
  navigator.clipboard.writeText(url);
  showNotification("已复制动态追溯链接到剪贴板");
}

// 滚动定位并高亮目标动态
async function scrollToAndHighlightContent(cid: string) {
  targetContentId.value = cid;
  await nextTick();
  let attempts = 0;
  const tryScroll = () => {
    const el = document.getElementById(`content-item-${cid}`);
    if (el) {
      el.scrollIntoView({ behavior: "smooth", block: "center" });
      showNotification("已定位并高亮目标动态内容");
    } else if (attempts < 6) {
      attempts++;
      setTimeout(tryScroll, 120);
    }
  };
  setTimeout(tryScroll, 80);

  setTimeout(() => {
    if (targetContentId.value === cid) {
      targetContentId.value = "";
    }
  }, 4500);
}

// 处理路由查询参数进行深度定位
async function handleRouteQuery() {
  const queryContentId = (route.query.content_id || route.query.id) as string;
  const queryAuthor = (route.query.author || route.query.qq) as string;
  const queryQ = (route.query.q || route.query.query) as string;

  if (queryAuthor) {
    author.value = queryAuthor;
  }
  if (queryQ && !queryContentId) {
    query.value = queryQ;
  }

  if (queryContentId) {
    targetContentId.value = queryContentId;
    try {
      const res = (await api.get(`/api/v1/contents/${queryContentId}`)).data.data;
      if (res) {
        detail.value = res;
        if (!contents.value.some(c => c.id === res.id)) {
          contents.value.unshift(res);
        }
        await open(res, true);
        await scrollToAndHighlightContent(res.id);
        return;
      }
    } catch (e) {
      console.error("加载指定动态失败", e);
    }
  }

  if (queryAuthor || queryQ) {
    resetAndLoad();
  }
}

function observeSentinel() { observer?.disconnect(); if (!loadSentinel.value || !hasMore.value) return; observer = new IntersectionObserver((entries) => { if (entries[0]?.isIntersecting) loadMore(); }, { rootMargin: "240px" }); observer.observe(loadSentinel.value); }

function extractShareInfo(content: any) {
  if (!content) return null;
  const meta = content.metadata || {};
  let url = meta.musicShare?.playUrl || meta.share_url || meta.url || meta.link || "";
  let title = meta.appShareTitle || meta.musicShare?.songName || meta.title || "";
  let subtitle = meta.musicShare?.artistName || meta.appName || "";

  if (!url && typeof meta.likeUnikey === "string") {
    if (meta.likeUnikey.startsWith("http://") || meta.likeUnikey.startsWith("https://")) {
      url = meta.likeUnikey;
    } else if (meta.likeUnikey.includes("fakeUrl=")) {
      const match = meta.likeUnikey.match(/fakeUrl=([^&]+)/);
      if (match) {
        url = decodeURIComponent(match[1]);
      }
    }
  }

  if (url.includes("fakeUrl=")) {
    const match = url.match(/fakeUrl=([^&]+)/);
    if (match) {
      url = decodeURIComponent(match[1]);
    }
  }

  title = title.replace(/[\t\r\n]+/g, " ").trim();
  subtitle = subtitle.replace(/[\t\r\n]+/g, " ").trim();

  if (!url && !title) return null;
  return { url, title: title || url, subtitle };
}

function openShareLink(url?: string) {
  if (!url) return;
  window.open(url, "_blank", "noopener,noreferrer");
}

function metadataStatus(item: any) { return item?.collection_status || item?.completeness || item?.metadata?.collection_status || item?.metadata?.completeness || ""; }
function completeness(item: any) { const value = String(metadataStatus(item)).toLowerCase(); if (["complete", "completed", "full"].includes(value)) return "complete"; if (["unavailable", "failed", "blocked"].includes(value)) return "unavailable"; return "partial"; }
function completenessLabel(item: any) { return ({ complete: "完整", partial: "部分采集", unavailable: "不可用" } as Record<string, string>)[completeness(item)]; }
function completenessColor(item: any) { return ({ complete: "success", partial: "warning", unavailable: "error" } as Record<string, string>)[completeness(item)]; }
function metric(content: any, ...keys: string[]) { for (const key of keys) { const direct = content?.[key]; const value = direct ?? content?.metadata?.[key]; if (value !== undefined && value !== null && value !== "") return Number(value) || 0; } return 0; }
function archivedCount(media: any[]) { return (media || []).filter((item) => item.asset_url || item.status === "completed").length; }
function initial(value: string) { return String(value || "?").trim().slice(0, 1).toUpperCase(); }
function formatDate(value?: string) { return value ? new Intl.DateTimeFormat("zh-CN", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "时间未知"; }

watch([contextType, dateRange, mediaFilter, completenessFilter], resetAndLoad);
watch(() => route.query, async () => {
  await handleRouteQuery();
}, { deep: true });

onMounted(async () => {
  await systemCapabilities.load();
  pageSize.value = systemCapabilities.data?.lists.default_page_size || 0;
  if (pageSize.value) await resetAndLoad();
  await handleRouteQuery();
});
onBeforeUnmount(() => observer?.disconnect());
</script>

<style scoped>
.content-page { max-width: 1680px; }
.header-actions { display: flex; align-items: center; justify-content: flex-end; gap: 8px; color: rgba(var(--v-theme-on-surface), .58); font-size: 12px; }
.content-toolbar { display: block; padding: 12px; }
.content-filter-primary, .content-filter-secondary { display: grid; gap: 8px; }
.content-filter-primary { grid-template-columns: minmax(0, 1.5fr) minmax(190px, 1fr); }
.content-filter-secondary { grid-template-columns: repeat(3, minmax(120px, 1fr)) auto; margin-top: 8px; }
.content-filter-secondary .v-btn-toggle { justify-self: end; }
.content-page-header { display: flex; align-items: flex-end; justify-content: space-between; gap: 16px; margin-bottom: 14px; }
.content-page-header h1 { margin: 3px 0 0; font-size: 20px; line-height: 1.25; }
.content-page-header p { margin: 5px 0 0; color: rgba(var(--v-theme-on-surface), .46); font-size: 11px; }

.research-layout { display: grid; grid-template-columns: minmax(0, 1fr) 420px; gap: 14px; align-items: start; }
.feed-column { overflow: hidden; border-radius: 8px; }
.feed-column-header { height: 48px; display: flex; align-items: center; justify-content: space-between; padding: 0 16px; border-bottom: 1px solid rgba(var(--v-theme-on-surface), .08); }
.feed-column-header strong { font-size: 13px; }
.feed-column-header span { color: rgba(var(--v-theme-on-surface), .45); font-size: 11px; }

.feed-list { min-width: 0; transition: opacity .2s; }
.feed-list.is-loading { opacity: .5; }
.feed-item { display: grid; grid-template-columns: 42px minmax(0, 1fr); gap: 12px; padding: 14px 16px; border-bottom: 1px solid rgba(var(--v-theme-on-surface), .07); cursor: pointer; transition: background-color .15s ease; }
.feed-item:hover, .feed-item.selected { background: rgba(var(--v-theme-primary), .045); }
.feed-item.selected { box-shadow: inset 3px 0 rgb(var(--v-theme-primary)); }
.feed-item.is-target-focused {
  border-left: 4px solid #3b82f6 !important;
  background: rgba(59, 130, 246, 0.09) !important;
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.4), 0 0 16px rgba(59, 130, 246, 0.3) !important;
  animation: feedItemGlow 1.2s ease-in-out infinite alternate !important;
}

@keyframes feedItemGlow {
  0% {
    box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.4), 0 0 8px rgba(59, 130, 246, 0.2);
  }
  100% {
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.8), 0 0 20px rgba(59, 130, 246, 0.5);
  }
}

.author-avatar { width: 42px; height: 42px; align-self: start; border-radius: 50%; overflow: hidden; background: rgba(var(--v-theme-on-surface), .08); display: grid; place-items: center; color: rgb(var(--v-theme-primary)); font-weight: 700; }
.author-avatar :deep(img) { width: 100%; height: 100%; object-fit: cover; }
.feed-main { min-width: 0; }
.feed-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.feed-head > div:first-child { display: flex; min-width: 0; flex-direction: column; }
.feed-head strong { font-size: 13px; color: rgb(var(--v-theme-on-surface)); }
.feed-head small { color: rgba(var(--v-theme-on-surface), .5); font-size: 11px; }
.feed-badges { display: flex; flex-wrap: wrap; gap: 5px; justify-content: flex-end; }
.feed-body { margin: 8px 0 10px; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.6; font-size: 13px; color: rgba(var(--v-theme-on-surface), .88); }
.feed-foot { display: flex; align-items: center; gap: 18px; margin-top: 10px; color: rgba(var(--v-theme-on-surface), .5); font-size: 11px; }
.feed-foot span { display: flex; align-items: center; gap: 4px; }

/* Share Card in List */
.feed-share-card {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  margin: 8px 0 10px;
  padding: 8px 12px;
  border: 1px solid rgba(var(--v-theme-primary), 0.25);
  border-radius: 6px;
  background: rgba(var(--v-theme-primary), 0.035);
  cursor: pointer;
  transition: all 0.15s ease;
}
.feed-share-card:hover {
  border-color: rgba(var(--v-theme-primary), 0.5);
  background: rgba(var(--v-theme-primary), 0.07);
}
.share-card-info { display: flex; flex-direction: column; gap: 2px; min-width: 0; flex: 1; }
.share-card-title { font-size: 13px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); overflow: hidden; text-overflow: ellipsis; white-space: normal; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; }
.share-card-sub { color: rgba(var(--v-theme-on-surface), 0.6); font-size: 11px; }
.share-card-link { font-size: 11px; color: rgb(var(--v-theme-primary)); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: inline-flex; align-items: center; margin-top: 2px; }

/* ========================================================= */
/* PREMIUM RIGHT RESEARCH SIDEBAR                            */
/* ========================================================= */
.research-sidebar {
  position: sticky;
  top: 76px;
  height: calc(100vh - 100px);
  min-height: 580px;
  display: flex;
  flex-direction: column;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid rgba(var(--v-theme-on-surface), .08);
  background: rgb(var(--v-theme-surface));
}

.sidebar-hero {
  padding: 16px 18px 14px;
  border-bottom: 1px solid rgba(var(--v-theme-on-surface), .08);
  background: linear-gradient(180deg, rgba(var(--v-theme-primary), .05) 0%, rgba(var(--v-theme-surface), 1) 100%);
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hero-top { display: flex; align-items: center; gap: 12px; }
.hero-avatar {
  width: 46px;
  height: 46px;
  border-radius: 50%;
  overflow: hidden;
  background: rgba(var(--v-theme-primary), .12);
  display: grid;
  place-items: center;
  box-shadow: 0 2px 8px rgba(0,0,0,.15);
  flex-shrink: 0;
}
.hero-avatar :deep(img) { width: 100%; height: 100%; object-fit: cover; }
.avatar-letter { font-size: 18px; font-weight: 700; color: rgb(var(--v-theme-primary)); }

.hero-info { display: flex; flex-direction: column; min-width: 0; flex: 1; gap: 2px; }
.hero-title-row { display: flex; align-items: center; justify-content: space-between; gap: 6px; }
.hero-name { margin: 0; font-size: 15px; font-weight: 700; color: rgb(var(--v-theme-on-surface)); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.hero-chip { font-weight: 600; font-size: 10px; }
.hero-meta-row { display: flex; align-items: center; gap: 6px; font-size: 11px; color: rgba(var(--v-theme-on-surface), .55); }
.hero-qq { font-weight: 500; }
.meta-dot { opacity: .4; }

.hero-actions-strip { display: flex; align-items: center; gap: 8px; }

.sidebar-scroll-body {
  flex: 1;
  overflow-y: auto;
  padding: 14px 18px 24px;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

/* Quote Card for Post Text */
.detail-quote-card {
  padding: 14px 16px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), .07);
  background: rgba(var(--v-theme-on-surface), .025);
  border-left: 3px solid rgb(var(--v-theme-primary));
}
.detail-quote-text {
  margin: 0;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  line-height: 1.65;
  font-size: 13.5px;
  color: rgb(var(--v-theme-on-surface));
}

/* Share Card in Sidebar */
.sidebar-share-card {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 14px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-primary), .3);
  background: rgba(var(--v-theme-primary), .05);
  cursor: pointer;
  transition: all .15s ease;
}
.sidebar-share-card:hover {
  border-color: rgba(var(--v-theme-primary), .6);
  background: rgba(var(--v-theme-primary), .09);
  transform: translateY(-1px);
}
.share-icon-wrap { margin-top: 2px; }
.share-text-wrap { display: flex; flex-direction: column; gap: 3px; min-width: 0; flex: 1; }
.share-main-title { font-size: 13px; font-weight: 600; line-height: 1.4; color: rgb(var(--v-theme-on-surface)); }
.share-sub-title { font-size: 11px; color: rgba(var(--v-theme-on-surface), .65); }
.share-href { font-size: 11px; color: rgb(var(--v-theme-primary)); text-decoration: none; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; display: flex; align-items: center; margin-top: 2px; }

.sidebar-section { display: flex; flex-direction: column; gap: 8px; }
.section-title { display: flex; align-items: center; font-size: 12px; font-weight: 600; color: rgba(var(--v-theme-on-surface), .6); }

.sidebar-tabs-wrap { border-bottom: 1px solid rgba(var(--v-theme-on-surface), .08); margin-top: 4px; }

/* Tab Pane Styling */
.tab-pane { display: flex; flex-direction: column; gap: 10px; margin-top: 6px; }

/* Comments stream */
.comments-list { display: flex; flex-direction: column; gap: 8px; }
.comment-item-card {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), .07);
  background: rgba(var(--v-theme-on-surface), .02);
}
.comment-item-head { display: flex; align-items: center; justify-content: space-between; gap: 8px; }
.comment-user { display: flex; align-items: center; gap: 8px; min-width: 0; }
.comment-avatar-mini { width: 26px; height: 26px; border-radius: 50%; background: rgba(var(--v-theme-primary), .15); color: rgb(var(--v-theme-primary)); font-weight: 700; font-size: 11px; display: grid; place-items: center; flex-shrink: 0; }
.comment-user-name { font-size: 12px; color: rgb(var(--v-theme-on-surface)); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.comment-user-time { color: rgba(var(--v-theme-on-surface), .45); font-size: 10px; margin-left: 6px; }
.comment-item-body { margin: 0; font-size: 12.5px; line-height: 1.5; color: rgba(var(--v-theme-on-surface), .85); white-space: pre-wrap; overflow-wrap: anywhere; }

/* Likers List & Cards */
.likers-list { display: flex; flex-direction: column; gap: 6px; }
.liker-card {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid rgba(var(--v-theme-on-surface), .07);
  background: rgba(var(--v-theme-on-surface), .02);
  transition: all .15s ease;
}
.liker-card:hover {
  border-color: rgba(var(--v-theme-primary), .35);
  background: rgba(var(--v-theme-primary), .03);
}
.liker-avatar-box { width: 34px; height: 34px; border-radius: 50%; overflow: hidden; background: rgba(var(--v-theme-primary), .1); display: grid; place-items: center; flex-shrink: 0; }
.liker-avatar-box :deep(img) { width: 100%; height: 100%; object-fit: cover; }
.avatar-letter-mini { font-size: 13px; font-weight: 700; color: rgb(var(--v-theme-primary)); }
.liker-detail { display: flex; flex-direction: column; min-width: 0; flex: 1; }
.liker-name { font-size: 13px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.liker-sub { font-size: 10.5px; color: rgba(var(--v-theme-on-surface), .5); }
.liker-ops { display: flex; align-items: center; gap: 4px; }

/* Meta Grid */
.meta-card-grid { display: grid; grid-template-columns: repeat(2, 1fr); gap: 8px; }
.meta-card { padding: 10px 12px; border-radius: 6px; border: 1px solid rgba(var(--v-theme-on-surface), .06); background: rgba(var(--v-theme-on-surface), .02); display: flex; flex-direction: column; gap: 4px; }
.span-full { grid-column: 1 / -1; }
.meta-label { font-size: 10.5px; color: rgba(var(--v-theme-on-surface), .5); }
.meta-val { font-size: 13px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); }
.meta-code { font-size: 10px; font-family: monospace; color: rgb(var(--v-theme-primary)); word-break: break-all; }

/* Empty Boxes */
.sidebar-empty-box { display: flex; flex-direction: column; align-items: center; justify-content: center; padding: 32px 16px; color: rgba(var(--v-theme-on-surface), .45); font-size: 12px; text-align: center; }
.empty-ico { opacity: .4; margin-bottom: 8px; }
.metric-circle { width: 56px; height: 56px; border-radius: 50%; border: 2px dashed rgba(var(--v-theme-primary), .35); display: flex; flex-direction: column; align-items: center; justify-content: center; color: rgb(var(--v-theme-primary)); }
.metric-circle strong { font-size: 18px; line-height: 1; }
.metric-circle span { font-size: 9px; opacity: .7; }

/* Unselected placeholder */
.sidebar-unselected { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; padding: 40px 24px; text-align: center; color: rgba(var(--v-theme-on-surface), .45); }
.unselected-icon-ring { width: 68px; height: 68px; border-radius: 50%; background: rgba(var(--v-theme-primary), .08); display: grid; place-items: center; margin-bottom: 16px; }
.sidebar-unselected h3 { font-size: 16px; font-weight: 600; color: rgb(var(--v-theme-on-surface)); margin-bottom: 6px; }
.sidebar-unselected p { font-size: 12px; max-width: 260px; line-height: 1.6; margin: 0; }

.feed-progress, .feed-end, .feed-empty, .load-more { width: 100%; min-height: 54px; display: flex; align-items: center; justify-content: center; gap: 8px; color: rgba(var(--v-theme-on-surface), .5); font-size: 12px; }
.load-more { border: 0; background: transparent; cursor: pointer; }
.feed-empty { min-height: 180px; flex-direction: column; }

.mobile-scrim { position: fixed; z-index: 1010; inset: 0; background: rgba(0,0,0,.5); display: flex; justify-content: flex-end; }
.mobile-sidebar { position: relative; top: 0; width: min(440px, 94vw); height: 100%; min-height: 0; border-radius: 0; }

@media (max-width: 1280px) {
  .research-layout { grid-template-columns: 1fr; }
}
@media (max-width: 700px) {
  .content-page-header { align-items: flex-start; }
  .content-filter-primary { grid-template-columns: 1fr; }
  .content-filter-secondary { grid-template-columns: 1fr 1fr; }
  .content-filter-secondary .v-select:last-of-type, .content-filter-secondary .v-btn-toggle { grid-column: 1/-1; }
  .content-filter-secondary .v-btn-toggle { width: 100%; }
  .content-filter-secondary .v-btn { flex: 1; }
  .feed-item { grid-template-columns: 34px minmax(0, 1fr); padding-inline: 10px; }
  .author-avatar { width: 34px; height: 34px; }
  .feed-badges { flex-direction: column; align-items: flex-end; }
  .feed-foot { gap: 13px; }
}
</style>

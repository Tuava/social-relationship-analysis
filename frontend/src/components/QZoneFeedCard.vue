<template>
  <v-card variant="outlined" class="qzone-feed-card bg-surface-variant-subtle mb-3" :class="{ 'qzone-feed-card--compact': compact }">
    <!-- Header: Author info and QZone source tag -->
    <div class="d-flex align-center justify-space-between pa-3 pb-2">
      <div class="d-flex align-center gap-2">
        <v-avatar size="36" color="amber-darken-2" variant="tonal">
          <AuthImage
            v-if="feedAuthorAvatar"
            :src="feedAuthorAvatar"
            :fallback-srcs="authorAvatarFallbacks"
            :alt="feedAuthorName"
          >
            <span class="text-caption font-weight-bold">{{ String(feedAuthorName || '?').slice(0, 1) }}</span>
          </AuthImage>
          <v-icon v-else icon="mdi-account" size="20" />
        </v-avatar>
        <div class="d-flex flex-column">
          <div class="d-flex align-center gap-1">
            <span class="text-body-2 font-weight-bold text-truncate" style="max-width: 220px;">
              {{ feedAuthorName }}
            </span>
            <v-chip size="x-small" color="amber-darken-3" variant="tonal" class="ml-1 px-1 font-weight-medium">
              QQ空间说说
            </v-chip>
          </div>
          <div class="text-caption text-medium-emphasis">
            QQ {{ feedAuthorQQ || '未知' }} · 发表于 {{ formatDateTime(feedPublishedAt) }}
          </div>
        </div>
      </div>

      <div class="d-flex align-center gap-1">
        <v-btn
          v-if="feedContentId"
          size="x-small"
          variant="tonal"
          color="primary"
          prepend-icon="mdi-open-in-new"
          title="在内容检索中查看完整动态与全量评论"
          @click.stop="openContentDetail(feedContentId)"
        >
          查看全貌
        </v-btn>
      </div>
    </div>

    <!-- Body text -->
    <div class="px-3 py-1">
      <div v-if="feedBody" class="qzone-feed-text text-body-2">
        {{ feedBody }}
      </div>
      <div v-else class="text-caption text-medium-emphasis font-italic">
        [纯图片 / 转发无文字说说]
      </div>
    </div>

    <!-- Image grid if media attached -->
    <div v-if="feedImages && feedImages.length" class="px-3 pt-2 pb-1">
      <div class="qzone-images-grid" :class="`grid-cols-${Math.min(feedImages.length, 3)}`">
        <div
          v-for="(img, idx) in feedImages"
          :key="idx"
          class="qzone-image-cell cursor-pointer"
          @click.stop="previewImage(img)"
        >
          <AuthImage :src="img" :alt="`配图 ${idx + 1}`">
            <div class="image-fallback-slot">
              <v-icon icon="mdi-image-outline" size="20" />
            </div>
          </AuthImage>
        </div>
      </div>
    </div>

    <!-- Interaction overlay banner (like / comment / reply by target) -->
    <div v-if="interaction" class="px-3 pt-2 pb-3">
      <v-divider class="my-2" />
      <div class="qzone-interaction-banner" :class="`banner-${interaction.action_type || 'interaction'}`">
        <div class="d-flex align-center gap-2">
          <v-avatar size="24" class="flex-shrink-0">
            <AuthImage
              v-if="interaction.actor_avatar"
              :src="interaction.actor_avatar"
              :fallback-srcs="actorAvatarFallbacks"
              :alt="interaction.actor_name"
            >
              <span class="text-caption">{{ String(interaction.actor_name || '?').slice(0, 1) }}</span>
            </AuthImage>
            <v-icon v-else icon="mdi-account" size="14" />
          </v-avatar>

          <div class="interaction-text text-caption flex-grow-1">
            <strong>{{ interaction.actor_name || '互动用户' }}</strong>
            <span v-if="interaction.actor_qq" class="text-medium-emphasis"> ({{ interaction.actor_qq }})</span>

            <template v-if="interaction.action_type === 'liked'">
              <v-chip size="x-small" color="pink" variant="flat" class="ml-1 px-1">
                <v-icon icon="mdi-thumb-up" start size="11" />
                空间点赞
              </v-chip>
              <span class="ml-1">赞了该条说说</span>
            </template>

            <template v-else-if="interaction.action_type === 'commented'">
              <v-chip size="x-small" color="purple" variant="flat" class="ml-1 px-1">
                <v-icon icon="mdi-comment-text-outline" start size="11" />
                说说评论
              </v-chip>
              <span class="ml-1">发表评论:</span>
              <div class="interaction-comment-bubble mt-1">
                {{ interaction.content_text }}
              </div>
            </template>

            <template v-else-if="interaction.action_type === 'feed_reply'">
              <v-chip size="x-small" color="amber-darken-3" variant="flat" class="ml-1 px-1">
                <v-icon icon="mdi-reply" start size="11" />
                说说回复
              </v-chip>
              <span class="ml-1">回复评论:</span>
              <div class="interaction-comment-bubble mt-1">
                {{ interaction.content_text }}
              </div>
            </template>

            <template v-else>
              <v-chip size="x-small" color="primary" variant="flat" class="ml-1 px-1">
                {{ interaction.action_label || '空间互动' }}
              </v-chip>
              <div v-if="interaction.content_text" class="interaction-comment-bubble mt-1">
                {{ interaction.content_text }}
              </div>
            </template>
          </div>

          <div v-if="interaction.occurred_at" class="interaction-time text-caption text-medium-emphasis flex-shrink-0">
            {{ formatDateTime(interaction.occurred_at) }}
          </div>
        </div>
      </div>
    </div>

    <!-- Image preview dialog -->
    <v-dialog v-model="imagePreviewOpen" max-width="800">
      <v-card class="bg-surface">
        <v-card-title class="d-flex align-center pa-3">
          <span class="text-subtitle-2">查看大图</span>
          <v-spacer />
          <v-btn icon="mdi-close" variant="text" size="small" @click="imagePreviewOpen = false" />
        </v-card-title>
        <v-divider />
        <v-card-text class="pa-2 d-flex justify-center bg-black">
          <AuthImage :cover="false" :src="previewImageSrc" style="max-width: 100%; max-height: 80vh; object-fit: contain;" alt="大图预览" />
        </v-card-text>
      </v-card>
    </v-dialog>
  </v-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { useRouter } from "vue-router";
import AuthImage from "@/components/AuthImage.vue";

const props = defineProps<{
  feed: {
    content_id?: string;
    tid?: string;
    author_name?: string;
    author_qq?: string;
    author_avatar?: string;
    body?: string;
    published_at?: string;
    images?: string[];
  };
  interaction?: {
    action_type?: string;
    action_label?: string;
    actor_name?: string;
    actor_qq?: string;
    actor_avatar?: string;
    content_text?: string;
    occurred_at?: string;
  };
  compact?: boolean;
}>();

const router = useRouter();

const feedAuthorName = computed(() => props.feed?.author_name || "空间作者");
const feedAuthorQQ = computed(() => props.feed?.author_qq || "");
const feedAuthorAvatar = computed(() => props.feed?.author_avatar || (feedAuthorQQ.value ? `/api/v1/media/avatars/person/${feedAuthorQQ.value}` : ""));
const feedBody = computed(() => props.feed?.body || "");
const feedPublishedAt = computed(() => props.feed?.published_at || "");
const feedContentId = computed(() => props.feed?.content_id || "");
const feedImages = computed(() => props.feed?.images || []);

const authorAvatarFallbacks = computed(() => [
  feedAuthorQQ.value ? `https://q1.qlogo.cn/g?b=qq&nk=${feedAuthorQQ.value}&s=640` : "",
  feedAuthorQQ.value ? `https://q2.qlogo.cn/headimg_dl?dst_uin=${feedAuthorQQ.value}&spec=640` : ""
].filter(Boolean));

const actorAvatarFallbacks = computed(() => [
  props.interaction?.actor_qq ? `https://q1.qlogo.cn/g?b=qq&nk=${props.interaction.actor_qq}&s=640` : "",
  props.interaction?.actor_qq ? `https://q2.qlogo.cn/headimg_dl?dst_uin=${props.interaction.actor_qq}&spec=640` : ""
].filter(Boolean));

const imagePreviewOpen = ref(false);
const previewImageSrc = ref("");

function previewImage(src: string) {
  previewImageSrc.value = src;
  imagePreviewOpen.value = true;
}

function openContentDetail(id: string) {
  if (!id) return;
  void router.push({ path: "/contents", query: { content_id: id } });
}

function formatDateTime(val?: string) {
  if (!val) return "—";
  try {
    return new Intl.DateTimeFormat("zh-CN", {
      month: "numeric",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit"
    }).format(new Date(val));
  } catch {
    return val;
  }
}
</script>

<style scoped>
.qzone-feed-card {
  border-radius: 8px;
  border-color: rgba(var(--v-border-color), 0.16);
  transition: all 0.2s ease;
}
.qzone-feed-card:hover {
  border-color: rgba(var(--v-theme-primary), 0.35);
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.12);
}
.qzone-feed-text {
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  color: rgba(var(--v-theme-on-surface), 0.92);
}
.qzone-images-grid {
  display: grid;
  gap: 6px;
  max-width: 480px;
}
.grid-cols-1 {
  grid-template-columns: minmax(0, 260px);
}
.grid-cols-2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}
.grid-cols-3 {
  grid-template-columns: repeat(3, minmax(0, 1fr));
}
.qzone-image-cell {
  aspect-ratio: 1;
  border-radius: 6px;
  overflow: hidden;
  background: rgba(var(--v-theme-surface-variant), 0.5);
}
.qzone-image-cell :deep(img) {
  width: 100%;
  height: 100%;
  object-fit: cover;
  transition: transform 0.2s ease;
}
.qzone-image-cell:hover :deep(img) {
  transform: scale(1.04);
}
.image-fallback-slot {
  width: 100%;
  height: 100%;
  display: grid;
  place-items: center;
  color: rgba(var(--v-theme-on-surface), 0.4);
}
.qzone-interaction-banner {
  border-radius: 6px;
  padding: 8px 10px;
  background: rgba(var(--v-theme-surface), 0.8);
  border: 1px solid rgba(var(--v-border-color), 0.1);
}
.banner-liked {
  border-left: 3px solid rgb(var(--v-theme-pink, 233, 30, 99));
}
.banner-commented {
  border-left: 3px solid rgb(var(--v-theme-purple, 156, 39, 176));
}
.banner-feed_reply {
  border-left: 3px solid rgb(var(--v-theme-warning, 255, 152, 0));
}
.interaction-comment-bubble {
  padding: 6px 10px;
  background: rgba(var(--v-theme-surface-variant), 0.6);
  border-radius: 6px;
  font-size: 13px;
  line-height: 1.5;
  color: rgba(var(--v-theme-on-surface), 0.95);
  word-break: break-word;
}
</style>

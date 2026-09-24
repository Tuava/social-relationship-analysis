<template>
  <div v-if="items.length" class="content-media" :class="`content-media--${variant}`">
    <article v-for="(item, index) in items" :key="mediaKey(item, index)" class="media-item" :class="`media-item--${mediaKind(item)}`">
      <div v-if="isImage(item)" class="media-visual">
        <AuthImage v-if="item.asset_url" :src="item.asset_url" :alt="mediaLabel(item)">
          <div class="media-placeholder"><v-icon icon="mdi-image-broken-variant" /><span>图片不可用</span></div>
        </AuthImage>
        <img v-else-if="item.source_url" :src="item.source_url" :alt="mediaLabel(item)" loading="lazy" referrerpolicy="no-referrer" />
        <div v-else class="media-placeholder"><v-icon icon="mdi-image-off-outline" /><span>等待资源地址</span></div>

        <div
          v-if="item.asset_id || item.id || item.asset_url"
          class="media-diffusion-badge"
          title="查看视觉理解与跨群传播溯源"
          @click.stop="openDiffusion(item)"
        >
          <v-icon icon="mdi-image-search-outline" size="12" class="mr-1" />
          <span>研判/溯源</span>
        </div>
      </div>

      <div v-else-if="isVideo(item)" class="media-visual media-player">
        <video v-if="playableURL(item)" :src="playableURL(item)" controls preload="metadata" />
        <button v-else class="media-placeholder media-load" :disabled="loadingID === item.id" @click="loadPlayable(item)">
          <v-progress-circular v-if="loadingID === item.id" indeterminate size="24" width="2" />
          <v-icon v-else icon="mdi-play-circle-outline" size="30" />
          <span>{{ item.asset_url ? "加载归档视频" : "视频不可用" }}</span>
        </button>
      </div>

      <div v-else-if="isAudio(item)" class="media-file media-audio">
        <v-icon icon="mdi-waveform" size="22" />
        <audio v-if="playableURL(item)" :src="playableURL(item)" controls preload="metadata" />
        <v-btn v-else-if="item.asset_url" size="small" variant="text" prepend-icon="mdi-play" :loading="loadingID === item.id" @click="loadPlayable(item)">加载语音</v-btn>
        <span v-else>{{ mediaLabel(item) }}</span>
      </div>

      <div v-else class="media-file">
        <v-icon :icon="kindIcon(item)" size="22" />
        <div><strong>{{ mediaLabel(item) }}</strong><small>{{ mimeLabel(item) }}</small></div>
        <AuthDownload v-if="item.asset_url" :src="item.asset_url" :filename="item.filename || mediaLabel(item)" />
      </div>

      <div class="media-state">
        <v-chip v-if="item.status === 'completed' || item.asset_url" size="x-small" color="success" variant="tonal">已归档</v-chip>
        <v-chip v-else-if="item.status === 'pending' || item.status === 'downloading'" size="x-small" color="info" variant="tonal">{{ item.status === "downloading" ? "下载中" : "等待下载" }}</v-chip>
        <v-btn v-else icon="mdi-download-outline" size="x-small" variant="tonal" title="归档此资源" @click.stop="emit('archive', item)" />
      </div>
    </article>

    <ImageDiffusionModal
      v-model="diffusionOpen"
      :asset-id="selectedAssetId"
      :image-url="selectedImageUrl"
    />
  </div>
</template>

<script setup lang="ts">
import { onBeforeUnmount, ref } from "vue";
import { api } from "@/services/api";
import AuthDownload from "@/components/AuthDownload.vue";
import AuthImage from "@/components/AuthImage.vue";
import ImageDiffusionModal from "@/components/ImageDiffusionModal.vue";

type MediaItem = Record<string, any>;

withDefaults(defineProps<{ items: MediaItem[]; variant?: "grid" | "stack" | "compact" }>(), { variant: "grid" });
const emit = defineEmits<{ archive: [item: MediaItem] }>();
const objectURLs = new Map<string, string>();
const playableURLs = ref<Record<string, string>>({});
const loadingID = ref("");

const diffusionOpen = ref(false);
const selectedAssetId = ref("");
const selectedImageUrl = ref("");

function openDiffusion(item: MediaItem) {
  selectedAssetId.value = String(item.asset_id || item.id || "");
  selectedImageUrl.value = item.asset_url || item.source_url || "";
  diffusionOpen.value = true;
}

function mediaKey(item: MediaItem, index: number) { return String(item.id || item.reference_id || item.asset_url || item.source_url || index); }
function mediaKind(item: MediaItem) { return String(item.kind || item.media_kind || item.segment_type || "file").toLowerCase(); }
function mime(item: MediaItem) { return String(item.mime_type || "").toLowerCase(); }
function isImage(item: MediaItem) { return ["image", "sticker", "avatar"].includes(mediaKind(item)) || mime(item).startsWith("image/"); }
function isVideo(item: MediaItem) { return mediaKind(item) === "video" || mime(item).startsWith("video/"); }
function isAudio(item: MediaItem) { return ["audio", "record", "voice"].includes(mediaKind(item)) || mime(item).startsWith("audio/"); }
function mediaLabel(item: MediaItem) { return item.filename || item.original_filename || (mediaKind(item) === "sticker" ? "动画表情" : kindLabel(mediaKind(item))); }
function mimeLabel(item: MediaItem) { return item.mime_type || item.status || "文件"; }
function kindLabel(kind: string) { return ({ image: "图片", sticker: "表情", video: "视频", audio: "语音", record: "语音", file: "文件" } as Record<string, string>)[kind] || "资源"; }
function kindIcon(item: MediaItem) { return ({ sticker: "mdi-emoticon-outline", file: "mdi-file-outline" } as Record<string, string>)[mediaKind(item)] || "mdi-paperclip"; }
function playableURL(item: MediaItem) { return playableURLs.value[mediaKey(item, 0)] || (!item.asset_url ? item.source_url || "" : ""); }

async function loadPlayable(item: MediaItem) {
  if (!item.asset_url) return;
  const key = mediaKey(item, 0);
  loadingID.value = String(item.id || key);
  try {
    const response = await api.get(item.asset_url, { responseType: "blob" });
    const url = URL.createObjectURL(response.data);
    objectURLs.set(key, url);
    playableURLs.value = { ...playableURLs.value, [key]: url };
  } finally {
    loadingID.value = "";
  }
}

onBeforeUnmount(() => objectURLs.forEach((url) => URL.revokeObjectURL(url)));
</script>

<style scoped>
.content-media{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:4px;max-width:660px}.content-media--stack{grid-template-columns:1fr;max-width:none;gap:8px}.content-media--compact{grid-template-columns:repeat(4,minmax(0,1fr));max-width:none}.media-item{position:relative;min-width:0;overflow:hidden;border-radius:4px;background:rgb(var(--v-theme-surface-variant))}.media-visual{aspect-ratio:1;display:grid;place-items:center;overflow:hidden;position:relative}.content-media--stack .media-visual{aspect-ratio:auto;min-height:180px;max-height:520px;background:#0c1014}.media-visual :deep(img),.media-visual>img,.media-visual video{width:100%;height:100%;object-fit:cover}.content-media--stack .media-visual :deep(img),.content-media--stack .media-visual>img,.content-media--stack .media-visual video{max-height:520px;object-fit:contain}.media-placeholder{width:100%;height:100%;min-height:90px;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:7px;color:rgba(var(--v-theme-on-surface),.5);font-size:12px}.media-load{border:0;cursor:pointer}.media-file{min-height:54px;display:flex;align-items:center;gap:10px;padding:10px 12px}.media-file>div{display:flex;min-width:0;flex:1;flex-direction:column}.media-file strong,.media-file small{overflow:hidden;text-overflow:ellipsis;white-space:nowrap}.media-file strong{font-size:12px}.media-file small{font-size:11px;color:rgba(var(--v-theme-on-surface),.52)}.media-audio audio{width:100%;height:36px}.media-state{position:absolute;right:6px;bottom:6px;display:flex;gap:4px}.media-file+.media-state{position:static;padding:0 10px 8px;justify-content:flex-end}@media(max-width:700px){.content-media,.content-media--compact{grid-template-columns:repeat(2,minmax(0,1fr))}}
.media-diffusion-badge{position:absolute;top:4px;right:4px;background:rgba(0,0,0,0.65);color:#fff;font-size:10px;padding:2px 6px;border-radius:4px;display:flex;align-items:center;cursor:pointer;opacity:0;transition:opacity 0.2s ease}.media-visual:hover .media-diffusion-badge{opacity:1}
</style>

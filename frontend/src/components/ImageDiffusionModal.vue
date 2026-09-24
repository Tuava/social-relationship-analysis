<template>
  <v-dialog v-model="visible" max-width="880" scrollable>
    <v-card class="diffusion-modal-card">
      <v-card-title class="d-flex align-center justify-space-between border-b px-4 py-3">
        <div class="d-flex align-center">
          <v-icon icon="mdi-image-search-outline" color="primary" class="mr-2" />
          <span class="text-h6 font-weight-bold">视觉多模态分析与跨群传播溯源</span>
        </div>
        <v-btn icon="mdi-close" variant="text" size="small" @click="visible = false" />
      </v-card-title>

      <v-card-text class="pa-4">
        <v-row>
          <!-- 左侧：图片与视觉/OCR分析 -->
          <v-col cols="12" md="6" class="border-e-md">
            <div class="image-preview-box mb-3">
              <AuthImage :cover="false" :src="imageUrl" alt="分析图片" class="modal-main-img" />
            </div>

            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-subtitle-2 font-weight-bold">视觉理解与 OCR 识别</span>
              <v-btn
                size="x-small"
                variant="tonal"
                color="primary"
                prepend-icon="mdi-refresh"
                :loading="analysisLoading"
                @click="triggerVisionAnalysis"
              >
                重新分析
              </v-btn>
            </div>

            <v-skeleton-loader v-if="analysisLoading" type="paragraph,list-item-two-line" />
            <template v-else-if="visionData">
              <!-- 分类与标签 -->
              <div class="mb-2" v-if="visionData.category || visionData.visual_tags?.length">
                <v-chip v-if="visionData.category" color="deep-purple" size="small" variant="flat" class="mr-1 mb-1">
                  {{ visionData.category }}
                </v-chip>
                <v-chip v-for="tag in visionData.visual_tags" :key="tag" size="x-small" variant="tonal" class="mr-1 mb-1">
                  {{ tag }}
                </v-chip>
              </div>

              <!-- OCR 提取文字 -->
              <div class="ocr-result-box mb-3" v-if="visionData.ocr_text">
                <div class="d-flex justify-space-between align-center mb-1">
                  <span class="text-caption font-weight-bold text-grey">OCR 提取文本:</span>
                  <v-btn size="x-small" variant="text" prepend-icon="mdi-content-copy" @click="copyText(visionData.ocr_text)">复制</v-btn>
                </div>
                <div class="ocr-text-content">{{ visionData.ocr_text }}</div>
              </div>

              <!-- 画面描述与意图 -->
              <div class="scene-desc-box" v-if="visionData.scene_description || visionData.transcript_text">
                <span class="text-caption font-weight-bold text-grey d-block mb-1">画面描述与社交意图:</span>
                <p class="text-body-2 text-medium-emphasis mb-0">
                  {{ visionData.scene_description || visionData.transcript_text }}
                </p>
              </div>
            </template>
            <div v-else class="text-caption text-grey pa-3 text-center bg-surface-variant rounded">
              点击上方“重新分析”按钮，利用视觉多模态大模型提取文字与画面意图。
            </div>
          </v-col>

          <!-- 右侧：跨群/跨人员传播链 -->
          <v-col cols="12" md="6">
            <div class="d-flex align-center justify-space-between mb-2">
              <span class="text-subtitle-2 font-weight-bold">跨群与人员传播溯源</span>
              <v-chip size="x-small" color="primary" variant="outlined" v-if="diffusionData">
                共发现 {{ diffusionData.total_occurrences }} 次流转 · 跨 {{ diffusionData.group_count }} 群
              </v-chip>
            </div>

            <v-skeleton-loader v-if="diffusionLoading" type="list-item-three-line@3" />
            <template v-else-if="diffusionData && diffusionData.timeline?.length">
              <div class="diffusion-timeline">
                <div
                  v-for="(item, idx) in diffusionData.timeline"
                  :key="item.id + idx"
                  class="diffusion-item"
                  :class="{ 'is-origin': idx === 0 }"
                >
                  <div class="diffusion-node-dot">
                    <v-icon :icon="idx === 0 ? 'mdi-flag-triangle' : 'mdi-arrow-right-bottom'" size="14" />
                  </div>
                  <div class="diffusion-content">
                    <div class="d-flex align-center justify-space-between">
                      <strong class="text-body-2">{{ item.sender_name || item.sender_qq }}</strong>
                      <small class="text-grey">{{ formatDate(item.occurred_at) }}</small>
                    </div>
                    <div class="text-caption text-grey">
                      <span v-if="item.group_name">在 <strong>{{ item.group_name }}</strong> ({{ item.group_id }})</span>
                      <span v-else>在 私聊对话</span>
                      <v-chip size="x-small" :color="item.match_method === 'exact_hash' ? 'success' : 'info'" variant="tonal" class="ml-1">
                        {{ item.match_method === 'exact_hash' ? '原图精确匹配' : '感知相似度匹配' }}
                      </v-chip>
                    </div>
                    <p class="text-caption text-medium-emphasis mt-1 mb-0" v-if="item.message_snippet">
                      "{{ item.message_snippet }}"
                    </p>
                  </div>
                </div>
              </div>
            </template>
            <div v-else class="text-caption text-grey pa-4 text-center">
              暂未检测到该图片在其他群组或消息中的跨群转发记录。
            </div>
          </v-col>
        </v-row>
      </v-card-text>
    </v-card>
  </v-dialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { api } from "@/services/api";
import AuthImage from "@/components/AuthImage.vue";

const props = defineProps<{
  modelValue: boolean;
  assetId?: string;
  imageUrl?: string;
}>();

const emit = defineEmits<{
  (e: "update:modelValue", val: boolean): void;
}>();

const visible = computed({
  get: () => props.modelValue,
  set: (val) => emit("update:modelValue", val),
});

const analysisLoading = ref(false);
const visionData = ref<any>(null);
const diffusionLoading = ref(false);
const diffusionData = ref<any>(null);

watch(
  () => props.modelValue,
  (open) => {
    if (open && props.assetId) {
      loadDiffusion();
      // Try fetching asset info if exists
      loadAssetInfo();
    }
  }
);

async function loadAssetInfo() {
  if (!props.assetId) return;
  try {
    const res = (await api.get(`/api/v1/media/assets/${props.assetId}`)).data.data;
    if (res && (res.ocr_text || res.transcript_text)) {
      visionData.value = res;
    }
  } catch (e) {
    // Ignore
  }
}

async function loadDiffusion() {
  if (!props.assetId) return;
  diffusionLoading.value = true;
  try {
    const res = (await api.get(`/api/v1/media/${props.assetId}/diffusion`)).data.data;
    diffusionData.value = res;
  } catch (e) {
    // Ignore
  } finally {
    diffusionLoading.value = false;
  }
}

async function triggerVisionAnalysis() {
  if (!props.assetId) return;
  analysisLoading.value = true;
  try {
    const res = (await api.post(`/api/v1/media/${props.assetId}/analyze`)).data.data;
    visionData.value = res;
  } catch (e) {
    // Ignore
  } finally {
    analysisLoading.value = false;
  }
}

function formatDate(dateStr?: string) {
  if (!dateStr) return "未知时间";
  return new Intl.DateTimeFormat("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(dateStr));
}

function copyText(txt?: string) {
  if (!txt) return;
  navigator.clipboard.writeText(txt);
}
</script>

<style scoped>
.diffusion-modal-card {
  background: rgb(var(--v-theme-surface));
}
.image-preview-box {
  display: flex;
  justify-content: center;
  align-items: center;
  background: rgba(0, 0, 0, 0.2);
  border-radius: 8px;
  overflow: hidden;
  max-height: 240px;
}
.modal-main-img {
  max-width: 100%;
  max-height: 240px;
  object-fit: contain;
}
.ocr-result-box {
  background: rgba(var(--v-theme-on-surface), 0.04);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.08);
  border-radius: 6px;
  padding: 8px 10px;
}
.ocr-text-content {
  font-size: 12px;
  line-height: 1.5;
  white-space: pre-wrap;
  max-height: 120px;
  overflow-y: auto;
  font-family: monospace;
}
.scene-desc-box {
  background: rgba(var(--v-theme-primary), 0.04);
  border-left: 3px solid rgb(var(--v-theme-primary));
  padding: 8px 10px;
  border-radius: 0 6px 6px 0;
}
.diffusion-timeline {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-height: 400px;
  overflow-y: auto;
}
.diffusion-item {
  display: flex;
  gap: 10px;
  position: relative;
}
.diffusion-node-dot {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: rgba(var(--v-theme-primary), 0.15);
  color: rgb(var(--v-theme-primary));
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.diffusion-item.is-origin .diffusion-node-dot {
  background: rgb(var(--v-theme-primary));
  color: #fff;
}
.diffusion-content {
  flex-grow: 1;
  background: rgba(var(--v-theme-on-surface), 0.03);
  border: 1px solid rgba(var(--v-theme-on-surface), 0.06);
  border-radius: 6px;
  padding: 6px 10px;
}
</style>

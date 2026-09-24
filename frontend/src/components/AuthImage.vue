<template>
  <img
    v-if="resolvedURL"
    :src="resolvedURL"
    :alt="alt"
    :style="imgStyle"
    referrerpolicy="no-referrer"
    @error="onError"
  />
  <slot v-else />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { cachedMediaURL } from '@/services/mediaCache'

const props = withDefaults(
  defineProps<{
    src?: string
    /** Additional sources used when the primary source is stale or unavailable. */
    fallbackSrcs?: string[]
    alt?: string
    /** When the image lives inside a fixed-size container, set cover=true to force object-fit: cover. */
    cover?: boolean
  }>(),
  { src: '', fallbackSrcs: () => [], alt: '', cover: true },
)

const objectURL = ref('')
const currentSource = ref('')
const sources = ref<string[]>([])
const sourceIndex = ref(0)
let loadRequest = 0
const failed = ref(false)

const isExternal = (value: string) =>
  /^https?:\/\//i.test(value) || value.startsWith('//')

function sanitizeSource(url: string): string {
  if (!url) return ''
  // If it's a raw QZone URL with referer anti-theft, convert to safe local proxy
  if (url.includes('store.qq.com') || url.includes('/qzone/')) {
    const match = url.match(/qzone\/(\d+)/)
    if (match && match[1]) {
      return `/api/v1/media/avatars/person/${match[1]}`
    }
  }
  return url
}

const resolvedURL = computed(() => {
  if (failed.value) return ''
  if (currentSource.value && isExternal(currentSource.value)) return currentSource.value
  return objectURL.value
})

const imgStyle = computed(() =>
  props.cover
    ? ({ objectFit: 'cover' as const, width: '100%', height: '100%', display: 'block' })
    : ({ maxWidth: '100%', height: 'auto', display: 'block' }),
)

function releaseObjectURL() {
  objectURL.value = ''
}

async function resolveCurrent(request: number) {
  const value = sources.value[sourceIndex.value]
  if (!value) return
  if (isExternal(value)) {
    currentSource.value = value
    return
  }
  try {
    const resolved = await cachedMediaURL(value)
    if (request !== loadRequest) return
    objectURL.value = resolved
    currentSource.value = value
  } catch {
    if (request === loadRequest) await advanceSource(request)
  }
}

async function advanceSource(request = loadRequest) {
  if (request !== loadRequest) return
  currentSource.value = ''
  if (sourceIndex.value + 1 < sources.value.length) {
    sourceIndex.value += 1
    await resolveCurrent(request)
  } else {
    failed.value = true
    currentSource.value = ''
  }
}

async function load(value?: string) {
  const request = ++loadRequest
  const rawList = [value || '', ...props.fallbackSrcs].filter(Boolean)
  const sanitizedList = rawList.map(sanitizeSource).filter(Boolean)
  const nextSources = [...new Set(sanitizedList)]
  failed.value = false
  sources.value = nextSources
  sourceIndex.value = 0
  if (!nextSources.length) {
    currentSource.value = ''
    releaseObjectURL()
    return
  }
  await resolveCurrent(request)
}

function onError() {
  // The direct <img> (external URL or stale blob) failed to render.
  void advanceSource()
}

watch(
  () => [props.src, ...props.fallbackSrcs].filter(Boolean).join('\n'),
  () => { void load(props.src) },
  { immediate: true },
)

onBeforeUnmount(() => {
  releaseObjectURL()
})
</script>

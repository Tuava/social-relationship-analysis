<template>
  <v-btn icon="mdi-download-outline" size="x-small" variant="text" title="下载资源" :loading="loading" @click="download" />
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { api } from '@/services/api'

const props = defineProps<{ src: string; filename?: string }>()
const loading = ref(false)

async function download() {
  loading.value = true
  try {
    const response = await api.get(props.src, { responseType: 'blob' })
    const url = URL.createObjectURL(response.data)
    const link = document.createElement('a')
    link.href = url
    link.download = props.filename || 'media'
    link.click()
    URL.revokeObjectURL(url)
  } finally {
    loading.value = false
  }
}
</script>

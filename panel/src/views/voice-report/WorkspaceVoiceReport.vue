<script setup lang="ts">
// panel/src/views/voice-report/WorkspaceVoiceReport.vue
//
// Per-workspace live voice report view.

import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { WorkspaceService } from '@/services/apiService'
import VoiceReportView from '@/components/voice-report/VoiceReportView.vue'
import type { VoiceReportData } from '@/components/voice-report/types'
import '@/components/voice-report/print.css'

const route = useRoute()
const data = ref<VoiceReportData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)

onMounted(async () => {
  const wID = String(route.params.wID ?? '')
  try {
    data.value = await WorkspaceService.fetchVoiceReportData(wID, 7)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || 'Failed to load voice report data'
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div class="container-fluid py-3">
    <VoiceReportView :data="data" :loading="loading" :error="error" />
  </div>
</template>
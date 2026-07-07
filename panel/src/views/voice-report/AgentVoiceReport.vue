<script setup lang="ts">
// panel/src/views/voice-report/AgentVoiceReport.vue
//
// Per-agent live voice report view. Loads
// `GET /agents/{id}/reports/agent_detail/data` and feeds it into
// `VoiceReportView` for rendering.

import { onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { AgentService } from '@/services/apiService'
import VoiceReportView from '@/components/voice-report/VoiceReportView.vue'
import type { VoiceReportData } from '@/components/voice-report/types'
import '@/components/voice-report/print.css'

const route = useRoute()
const data = ref<VoiceReportData | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const agentId = ref<number | string>('')

onMounted(async () => {
  agentId.value = String(route.params.aID ?? '')
  const wID = String(route.params.wID ?? '')
  try {
    data.value = await AgentService.fetchAgentReportData(agentId.value, 7)
  } catch (e: any) {
    error.value = e?.response?.data?.error || e?.message || 'Failed to load voice report data'
  } finally {
    loading.value = false
  }
  void wID
})
</script>

<template>
  <div class="container-fluid py-3">
    <VoiceReportView :data="data" :loading="loading" :error="error" />
  </div>
</template>
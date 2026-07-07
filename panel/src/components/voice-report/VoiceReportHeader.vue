<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportHeader.vue
// Mirrors the brand + report-id header block in voice-quality-report.html.

import type { VoiceReportMeta } from './types'

defineProps<{
  meta: VoiceReportMeta
}>()

function fmtDate(iso: string): string {
  try {
    return new Date(iso).toLocaleString('en-US', {
      dateStyle: 'medium',
      timeStyle: 'short',
      timeZone: 'UTC',
    }) + ' UTC'
  } catch {
    return iso
  }
}
</script>

<template>
  <header class="vr-report-header">
    <div class="vr-brand">
      <div class="vr-brand-mark">
        <svg viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" width="20" height="20">
          <polyline points="2 12 6 12 9 5 13 19 16 12 22 12" />
        </svg>
      </div>
      <div>
        <div class="vr-brand-name">netwatcher<span class="tld">.io</span></div>
        <div class="vr-brand-sub">Open Source Network Monitoring</div>
      </div>
    </div>
    <div class="vr-report-title">
      <h1>Voice Quality Report</h1>
      <div class="rid vr-mono" data-testid="report-id">{{ meta.report_id }}</div>
      <div class="rid">{{ fmtDate(meta.generated_at) }}</div>
    </div>
  </header>
</template>
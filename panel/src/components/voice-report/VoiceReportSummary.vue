<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportSummary.vue
// Page 1 of voice-quality-report.html: meta grid, verdict banner,
// KPI cards, and the latency/jitter chart.

import { computed } from 'vue'
import type { VoiceReportData } from './types'
import { lineChartSVG, statusFor, STATUS_LABEL } from './charts'

const props = defineProps<{
  data: VoiceReportData
}>()

const meta = computed(() => props.data.meta)
const summary = computed(() => props.data.summary)
const thresholds = computed(() => props.data.thresholds)
const metrics = computed(() => props.data.metrics)
const ts = computed(() => props.data.timeseries ?? {})

const ringDashArray = computed(() => (2 * Math.PI * 37).toFixed(1))
const ringPct = computed(() => {
  const mos = summary.value.mos || 0
  return Math.max(0, Math.min(1, (mos - 1) / 4))
})
const ringOffset = computed(() => (2 * Math.PI * 37 * (1 - ringPct.value)).toFixed(1))
const ringColor = computed(() => {
  const mos = summary.value.mos || 0
  if (mos >= 4.0) return '#22c55e'
  if (mos >= 3.6) return '#f59e0b'
  return '#ef4444'
})

const metaRows = computed(() => {
  const m = meta.value
  const t = m.target
  const source = m.agent?.name ?? m.workspace?.name ?? '—'
  const sourceLoc = m.agent?.location ?? ''
  const targetLabel = t
    ? `${t.host ?? t.name ?? ''}${t.host ? `:${t.host}` : ''}`
    : '—'
  return [
    ['Source Agent', sourceLoc ? `${source} · ${sourceLoc}` : source],
    ['Target', targetLabel],
    ['Test Type', m.test?.type ?? '—'],
    ['Codec Profile', m.test?.codec ?? '—'],
    ['Duration', m.test?.duration ?? '—'],
    ['Packet Interval', m.test?.interval ?? '—'],
    ['Packets Sent', (m.test?.packets_sent ?? 0).toLocaleString()],
    ['DSCP Marking', m.test?.dscp ?? '—'],
  ]
})

const latencyAvg = computed(() => metrics.value.latency?.avg ?? 0)
const jitterMax = computed(() => metrics.value.jitter?.max ?? 0)
const lossPct = computed(() => metrics.value.packets?.loss_pct ?? 0)
const dupPct = computed(() => metrics.value.packets?.dup_pct ?? 0)
const oooPct = computed(() => metrics.value.packets?.ooo_pct ?? 0)
const discardPct = computed(() => metrics.value.packets?.discard_pct ?? 0)

const kpiCards = computed(() => [
  {
    label: 'Avg Latency (RTT)',
    value: latencyAvg.value.toFixed(1),
    unit: 'ms',
    sub: `min ${(metrics.value.latency?.min ?? 0).toFixed(1)} / max ${(metrics.value.latency?.max ?? 0).toFixed(1)} ms`,
    st: statusFor(latencyAvg.value, thresholds.value.warning_jitter_ms * 5, thresholds.value.warning_jitter_ms * 8),
  },
  {
    label: 'Avg Jitter',
    value: (metrics.value.jitter?.avg ?? 0).toFixed(1),
    unit: 'ms',
    sub: `peak ${jitterMax.value.toFixed(1)} ms`,
    st: statusFor(jitterMax.value, thresholds.value.warning_jitter_ms * 0.66, thresholds.value.warning_jitter_ms),
  },
  {
    label: 'Packet Loss',
    value: lossPct.value.toFixed(2),
    unit: '%',
    sub: `${(metrics.value.packets?.lost ?? 0).toLocaleString()} of ${(metrics.value.packets?.sent ?? 0).toLocaleString()} packets`,
    st: statusFor(lossPct.value, thresholds.value.warning_loss_pct * 0.5, thresholds.value.warning_loss_pct),
  },
  {
    label: 'Duplicate Packets',
    value: dupPct.value.toFixed(2),
    unit: '%',
    sub: `${metrics.value.packets?.duplicates ?? 0} duplicates`,
    st: statusFor(dupPct.value, thresholds.value.critical_loss_pct, thresholds.value.critical_loss_pct * 5),
  },
  {
    label: 'Out-of-Order',
    value: oooPct.value.toFixed(2),
    unit: '%',
    sub: `${metrics.value.packets?.out_of_order ?? 0} reordered`,
    st: statusFor(oooPct.value, thresholds.value.out_of_sequence_pct, thresholds.value.out_of_sequence_pct * 4),
  },
  {
    label: 'Jitter-Buffer Discards',
    value: discardPct.value.toFixed(2),
    unit: '%',
    sub: `${metrics.value.packets?.discarded_jitter_buffer ?? 0} discarded (60 ms buffer)`,
    st: statusFor(discardPct.value, 0.5, 2),
  },
])

const latencyChart = computed(() => {
  const rtt = ts.value.rtt ?? []
  const jitter = ts.value.jitter ?? []
  if (rtt.length === 0 && jitter.length === 0) return ''
  const series = []
  if (rtt.length > 0) series.push({ name: 'RTT', data: rtt, color: '#3b82f6', fill: true })
  if (jitter.length > 0) series.push({ name: 'Jitter', data: jitter, color: '#10b981' })
  return lineChartSVG(series, {
    hline: thresholds.value.warning_jitter_ms * 8,
    durationSec: 120,
    height: 150,
  })
})
</script>

<template>
  <div class="vr-meta-grid" data-testid="meta-grid">
    <div v-for="[k, v] in metaRows" :key="k" class="vr-meta-cell">
      <div class="k">{{ k }}</div>
      <div class="v">{{ v }}</div>
    </div>
  </div>

  <div class="vr-verdict" data-testid="verdict">
    <div class="vr-mos-ring">
      <svg viewBox="0 0 86 86">
        <circle cx="43" cy="43" r="37" fill="none" stroke="rgba(255,255,255,.12)" stroke-width="7" />
        <circle
          cx="43"
          cy="43"
          r="37"
          fill="none"
          :stroke="ringColor"
          stroke-width="7"
          stroke-linecap="round"
          :stroke-dasharray="ringDashArray"
          :stroke-dashoffset="ringOffset"
        />
      </svg>
      <div class="val">
        <b>{{ summary.mos.toFixed(2) }}</b>
        <span>MOS</span>
      </div>
    </div>
    <div class="vr-verdict-body">
      <h2>{{ summary.verdict_title }}</h2>
      <p>{{ summary.verdict_text }}</p>
    </div>
    <div class="vr-grade-chip">
      <b>{{ summary.r_factor.toFixed(1) }}</b>
      <span>R-Factor</span>
    </div>
    <div class="vr-grade-chip">
      <b>{{ summary.grade }}</b>
      <span>Grade</span>
    </div>
  </div>

  <div class="vr-section">
    <div class="vr-section-title">Key Metrics</div>
    <div class="vr-kpi-grid">
      <div v-for="c in kpiCards" :key="c.label" class="vr-kpi" :class="c.st">
        <div class="label">
          <span>{{ c.label }}</span>
          <span class="vr-badge" :class="c.st">{{ STATUS_LABEL[c.st] }}</span>
        </div>
        <div class="value">{{ c.value }}<small> {{ c.unit }}</small></div>
        <div class="sub">{{ c.sub }}</div>
      </div>
    </div>
  </div>

  <div class="vr-section" v-if="latencyChart">
    <div class="vr-section-title">Latency &amp; Jitter Over Test Duration</div>
    <div class="vr-chart-box">
      <div class="vr-chart-head">
        <b>Round-trip time &amp; jitter (ms)</b>
        <div class="vr-legend">
          <span><i style="background:#3b82f6"></i>RTT</span>
          <span><i style="background:#10b981"></i>Jitter</span>
          <span><i style="background:#ef4444;height:1px;border-top:1px dashed #ef4444;background:none"></i>Threshold</span>
        </div>
      </div>
      <div v-html="latencyChart"></div>
    </div>
  </div>
</template>
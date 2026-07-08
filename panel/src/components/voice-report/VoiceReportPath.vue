<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportPath.vue
// Page 3 of voice-quality-report.html: traceroute table + per-hop
// bars + test configuration.

import { computed } from 'vue'
import type { VoiceReportData } from './types'
import { barChartSVG } from './charts'

const props = defineProps<{
  data: VoiceReportData
}>()

const meta = computed(() => props.data.meta)
const trace = computed(() => props.data.traceroute)
const reverseTrace = computed(() => props.data.traceroute_reverse)

// Both directions, when present. Each direction's MTR comes from a
// different reporter (this agent forward, the far-end agent back),
// so they are separate traces with separate hop tables.
const traces = computed(() => {
  const out: { key: string; label: string; t: NonNullable<typeof trace.value> }[] = []
  if (trace.value) out.push({ key: 'fwd', label: trace.value.note || 'Forward path', t: trace.value })
  if (reverseTrace.value) out.push({ key: 'rev', label: reverseTrace.value.note || 'Return path', t: reverseTrace.value })
  return out
})

function maxAvgOf(t: { hops: { avg: number }[] }): number {
  if (t.hops.length === 0) return 0
  return Math.max(...t.hops.map((h) => h.avg))
}

function rowStatus(loss: number): 'ok' | 'warn' | 'crit' {
  if (loss >= 5) return 'crit'
  if (loss > 0) return 'warn'
  return 'ok'
}

function barColor(loss: number): string {
  const st = rowStatus(loss)
  if (st === 'crit') return '#ef4444'
  if (st === 'warn') return '#f59e0b'
  return '#10b981'
}

const hopBars = computed(() => {
  const hops = trace.value?.hops ?? []
  return hops.map((h) => ({
    label: `hop ${h.hop}`,
    v: h.avg,
    color: barColor(h.loss),
  }))
})

const hopBarsSvg = computed(() => (hopBars.value.length > 0 ? barChartSVG(hopBars.value, { height: 130 }) : ''))

const configRows = computed(() => {
  const m = meta.value
  const t = m.target
  const source = m.agent
  const target = t
    ? `${t.host ?? t.name ?? '—'}${t.host ? ` (${t.host})` : ''} : ${t.host?.split(':').pop() ?? '—'}/udp`
    : '—'
  const profile = m.test?.type ? `${m.test.type} — ${m.test.codec ?? 'G.711'}` : '—'
  const payload = m.test?.payload_size ? `${m.test.payload_size} every ${m.test.interval} (${(m.test.packets_sent ?? 0).toLocaleString()} packets over ${m.test.duration})` : '—'
  return [
    ['Report ID', m.report_id],
    ['Generated', new Date(m.generated_at).toUTCString()],
    ['Agent', source ? `${source.name} (${source.ip ?? '—'}) — ${source.location ?? ''}` : '—'],
    ['Target', target],
    ['Simulation profile', profile],
    ['Payload / interval', payload],
    ['QoS marking', m.test?.dscp ?? '—'],
    ['Traceroute method', trace.value?.protocol ?? '—'],
  ]
})
</script>

<template>
  <div v-if="traces.length === 0" class="vr-section">
    <div class="vr-section-title">Traceroute / MTR</div>
    <div class="vr-section-note">No MTR trace data in this window for either direction.</div>
  </div>

  <div v-for="entry in traces" :key="entry.key" class="vr-section">
    <div class="vr-section-title">
      Traceroute / MTR — <span class="vr-mono">{{ entry.label }}</span>
    </div>
    <table class="vr-table">
      <thead>
        <tr>
          <th style="width:26px">#</th>
          <th>Host</th>
          <th>IP / ASN</th>
          <th class="num">Loss</th>
          <th style="width:64px"></th>
          <th class="num">Sent</th>
          <th class="num">Last</th>
          <th class="num">Avg</th>
          <th class="num">Best</th>
          <th class="num">Worst</th>
          <th class="num">StDev</th>
        </tr>
      </thead>
      <tbody>
        <tr
          v-for="h in entry.t.hops"
          :key="h.hop"
          :class="rowStatus(h.loss) === 'ok' ? '' : 'vr-row-' + rowStatus(h.loss)"
        >
          <td class="vr-mono">{{ h.hop }}</td>
          <td class="vr-mono">{{ h.host }}</td>
          <td class="vr-mono" style="color: var(--nw-slate-500)">
            {{ h.ip }}<span v-if="h.asn && h.asn !== '—'"> · {{ h.asn }}</span>
          </td>
          <td class="num vr-mono">{{ h.loss.toFixed(1) }}%</td>
          <td>
            <span class="vr-bar-track">
              <span class="vr-bar-fill" :style="{ width: maxAvgOf(entry.t) > 0 ? `${Math.min(100, (h.avg / maxAvgOf(entry.t)) * 100)}%` : '0%', background: barColor(h.loss) }"></span>
            </span>
          </td>
          <td class="num vr-mono">{{ h.sent }}</td>
          <td class="num vr-mono">{{ h.last.toFixed(1) }}</td>
          <td class="num vr-mono"><b>{{ h.avg.toFixed(1) }}</b></td>
          <td class="num vr-mono">{{ h.best.toFixed(1) }}</td>
          <td class="num vr-mono">{{ h.worst.toFixed(1) }}</td>
          <td class="num vr-mono">{{ h.stdev.toFixed(1) }}</td>
        </tr>
      </tbody>
    </table>
  </div>

  <div v-if="hopBarsSvg" class="vr-section">
    <div class="vr-section-title">Per-Hop Average Latency (ms)</div>
    <div class="vr-chart-box"><div v-html="hopBarsSvg"></div></div>
  </div>

  <div class="vr-section">
    <div class="vr-section-title">Test Configuration</div>
    <table class="vr-table">
      <tbody>
        <tr v-for="[k, v] in configRows" :key="k">
          <td style="width:170px;color:var(--nw-slate-500)">{{ k }}</td>
          <td class="vr-mono">{{ v }}</td>
        </tr>
      </tbody>
    </table>
  </div>
</template>
<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportDetails.vue
// Page 2 of voice-quality-report.html: delay/jitter stats table,
// packet table, quality breakdown, threshold reference.

import { computed } from 'vue'
import type { VoiceReportData } from './types'
import { lineChartSVG, statusFor, STATUS_LABEL } from './charts'

const props = defineProps<{
  data: VoiceReportData
}>()

const metrics = computed(() => props.data.metrics)
const thresholds = computed(() => props.data.thresholds)
const quality = computed(() => props.data.quality ?? [])
const ts = computed(() => props.data.timeseries ?? {})

const statsRows = computed(() => {
  const t = thresholds.value
  const m = metrics.value
  return [
    {
      name: 'Round-trip latency',
      stats: m.latency,
      thr: `${t.warning_jitter_ms * 8} ms`,
      st: statusFor(m.latency.avg, t.warning_jitter_ms * 5, t.warning_jitter_ms * 8),
    },
    ...(m.one_way_up
      ? [{
          name: 'One-way delay (agent → target)',
          stats: m.one_way_up,
          thr: `${t.warning_jitter_ms * 4} ms`,
          st: statusFor(m.one_way_up.avg, t.warning_jitter_ms * 3, t.warning_jitter_ms * 4),
        }]
      : []),
    ...(m.one_way_down
      ? [{
          name: 'One-way delay (target → agent)',
          stats: m.one_way_down,
          thr: `${t.warning_jitter_ms * 4} ms`,
          st: statusFor(m.one_way_down.avg, t.warning_jitter_ms * 3, t.warning_jitter_ms * 4),
        }]
      : []),
    {
      name: 'Interarrival jitter (RFC 3550)',
      stats: m.jitter,
      thr: `${t.warning_jitter_ms} ms`,
      st: statusFor(m.jitter.max, t.warning_jitter_ms * 0.66, t.warning_jitter_ms),
    },
  ]
})

const packetRows = computed(() => {
  const p = metrics.value.packets
  const t = thresholds.value
  return [
    ['Packets sent', p.sent, '100%', 'ok'],
    ['Packets received', p.received, p.sent > 0 ? `${((p.received / p.sent) * 100).toFixed(2)}%` : '0%', 'ok'],
    ['Lost', p.lost, `${p.loss_pct.toFixed(2)}%`, statusFor(p.loss_pct, t.warning_loss_pct * 0.5, t.warning_loss_pct)],
    ['Duplicates', p.duplicates, `${p.dup_pct.toFixed(2)}%`, statusFor(p.dup_pct, t.warning_loss_pct, t.warning_loss_pct * 5)],
    ['Out-of-order', p.out_of_order, `${p.ooo_pct.toFixed(2)}%`, statusFor(p.ooo_pct, t.out_of_sequence_pct, t.out_of_sequence_pct * 4)],
    ['Jitter-buffer discards', p.discarded_jitter_buffer, `${p.discard_pct.toFixed(2)}%`, statusFor(p.discard_pct, 0.5, 2)],
  ]
})

const lossChart = computed(() => {
  const loss = ts.value.loss_per_interval ?? []
  if (loss.length === 0) return ''
  return lineChartSVG(
    [{ name: 'Loss', data: loss, color: '#ef4444', fill: true }],
    { durationSec: 120, height: 118 }
  )
})

// ── Direction comparison (forward vs return) ───────────────────────────
// Each direction is measured independently (client vs far-end reporter),
// so the stats can legitimately differ — surfacing the variance is the
// point: a clean forward next to a degraded return localizes the problem
// to one path.
const pair = computed(() => props.data.pairs?.[0] ?? null)
const fwdPath = computed(() => pair.value?.forward ?? null)
const revPath = computed(() => pair.value?.reverse ?? null)
const hasBothDirections = computed(() => !!fwdPath.value && !!revPath.value)

interface DirRow {
  name: string
  unit: string
  fwd: number
  rev: number
  decimals: number
  st: 'ok' | 'warn' | 'crit'
}

function varianceStatus(fwd: number, rev: number, absFloor: number): 'ok' | 'warn' | 'crit' {
  const hi = Math.max(fwd, rev)
  const lo = Math.min(fwd, rev)
  const absDelta = hi - lo
  if (absDelta < absFloor || hi === 0) return 'ok'
  const ratio = absDelta / hi
  if (ratio > 0.6) return 'crit'
  if (ratio > 0.3) return 'warn'
  return 'ok'
}

const directionRows = computed<DirRow[]>(() => {
  const f = fwdPath.value
  const r = revPath.value
  if (!f || !r) return []
  return [
    // MOS variance floor tied to the asymmetry heuristic's spirit: small
    // deltas are noise, > ~0.3 MOS is a perceptible quality difference.
    { name: 'MOS', unit: '', fwd: f.mos_score, rev: r.mos_score, decimals: 2, st: varianceStatus(f.mos_score, r.mos_score, 0.3) },
    { name: 'Avg latency (one-way)', unit: 'ms', fwd: f.avg_latency_ms, rev: r.avg_latency_ms, decimals: 1, st: varianceStatus(f.avg_latency_ms, r.avg_latency_ms, 10) },
    { name: 'Jitter (avg)', unit: 'ms', fwd: f.jitter_avg_ms, rev: r.jitter_avg_ms, decimals: 1, st: varianceStatus(f.jitter_avg_ms, r.jitter_avg_ms, 3) },
    { name: 'Packet loss', unit: '%', fwd: f.packet_loss_pct, rev: r.packet_loss_pct, decimals: 2, st: varianceStatus(f.packet_loss_pct, r.packet_loss_pct, 0.5) },
  ]
})

const directionNote = computed(() => {
  const f = fwdPath.value
  const r = revPath.value
  if (!f || !r) return ''
  const flagged = directionRows.value.filter((row) => row.st !== 'ok')
  if (flagged.length === 0) {
    return 'Both directions are within normal variance of each other.'
  }
  return `Directional variance detected on ${flagged.map((row) => row.name).join(', ')} — asymmetric degradation usually points at one path (e.g. upstream saturation or a policer in one direction).`
})
</script>

<template>
  <div class="vr-section">
    <div class="vr-section-title">Delay &amp; Jitter Statistics</div>
    <table class="vr-table">
      <thead>
        <tr>
          <th>Metric</th>
          <th class="num">Min</th>
          <th class="num">Avg</th>
          <th class="num">Max</th>
          <th class="num">Std Dev</th>
          <th class="num">Threshold</th>
          <th>Status</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in statsRows" :key="row.name" :class="row.st === 'ok' ? '' : 'vr-row-' + row.st">
          <td>{{ row.name }}</td>
          <td class="num vr-mono">{{ row.stats.min.toFixed(1) }}</td>
          <td class="num vr-mono"><b>{{ row.stats.avg.toFixed(1) }}</b></td>
          <td class="num vr-mono">{{ row.stats.max.toFixed(1) }}</td>
          <td class="num vr-mono">{{ row.stats.stddev.toFixed(1) }}</td>
          <td class="num vr-mono">{{ row.thr }}</td>
          <td>
            <span class="vr-status-dot" :class="'vr-dot-' + row.st"></span>
            {{ STATUS_LABEL[row.st] }}
          </td>
        </tr>
      </tbody>
    </table>
    <div class="vr-section-note">
      Latency figures are round-trip (RTT) unless noted. Jitter is computed per RFC 3550 (interarrival jitter).
    </div>
  </div>

  <div v-if="hasBothDirections" class="vr-section">
    <div class="vr-section-title">Direction Comparison</div>
    <table class="vr-table">
      <thead>
        <tr>
          <th>Metric</th>
          <th class="num">→ {{ fwdPath?.source_agent_name || 'Forward' }} → {{ fwdPath?.target_agent_name || 'target' }}</th>
          <th class="num">← {{ revPath?.source_agent_name || 'Return' }} → {{ revPath?.target_agent_name || 'source' }}</th>
          <th class="num">Δ</th>
          <th>Variance</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="row in directionRows" :key="row.name" :class="row.st === 'ok' ? '' : 'vr-row-' + row.st">
          <td>{{ row.name }}</td>
          <td class="num vr-mono">{{ row.fwd.toFixed(row.decimals) }}{{ row.unit }}</td>
          <td class="num vr-mono">{{ row.rev.toFixed(row.decimals) }}{{ row.unit }}</td>
          <td class="num vr-mono">{{ (row.fwd - row.rev >= 0 ? '+' : '') + (row.fwd - row.rev).toFixed(row.decimals) }}{{ row.unit }}</td>
          <td>
            <span class="vr-status-dot" :class="'vr-dot-' + row.st"></span>
            {{ STATUS_LABEL[row.st] }}
          </td>
        </tr>
      </tbody>
    </table>
    <div class="vr-section-note">{{ directionNote }}</div>
  </div>

  <div class="vr-section">
    <div class="vr-section-title">Packet Delivery Analysis</div>
    <div class="vr-two-col">
      <table class="vr-table">
        <thead>
          <tr><th>Counter</th><th class="num">Count</th><th class="num">% of Sent</th><th>Status</th></tr>
        </thead>
        <tbody>
          <tr v-for="[name, count, pct, st] in packetRows" :key="name">
            <td>{{ name }}</td>
            <td class="num vr-mono">{{ count.toLocaleString() }}</td>
            <td class="num vr-mono">{{ pct }}</td>
            <td>
              <span class="vr-status-dot" :class="'vr-dot-' + st"></span>
              {{ STATUS_LABEL[st as keyof typeof STATUS_LABEL] }}
            </td>
          </tr>
        </tbody>
      </table>
      <div class="vr-chart-box" v-if="lossChart">
        <div class="vr-chart-head"><b>Packet loss per interval (%)</b></div>
        <div v-html="lossChart"></div>
      </div>
    </div>
  </div>

  <div class="vr-section" v-if="quality.length > 0">
    <div class="vr-section-title">Quality Score Breakdown</div>
    <table class="vr-table">
      <thead>
        <tr><th>Component</th><th class="num">Value</th><th>Interpretation</th></tr>
      </thead>
      <tbody>
        <tr v-for="q in quality" :key="q.component">
          <td>{{ q.component }}</td>
          <td class="num vr-mono"><b>{{ q.value }}</b></td>
          <td>{{ q.note }}</td>
        </tr>
      </tbody>
    </table>
  </div>

  <div class="vr-section">
    <div class="vr-section-title">Reference Thresholds (ITU-T G.114 / G.107)</div>
    <div class="vr-threshold-grid">
      <div class="vr-threshold-card">
        <b>Latency (RTT)</b>
        <div class="rng"><span>Good</span><span>&lt; {{ thresholds.warning_jitter_ms * 4 }} ms</span></div>
        <div class="rng"><span>Degraded</span><span>{{ thresholds.warning_jitter_ms * 4 }} – {{ thresholds.warning_jitter_ms * 8 }} ms</span></div>
        <div class="rng"><span>Poor</span><span>&gt; {{ thresholds.warning_jitter_ms * 8 }} ms</span></div>
      </div>
      <div class="vr-threshold-card">
        <b>Jitter</b>
        <div class="rng"><span>Good</span><span>&lt; {{ (thresholds.warning_jitter_ms * 0.66).toFixed(0) }} ms</span></div>
        <div class="rng"><span>Degraded</span><span>{{ (thresholds.warning_jitter_ms * 0.66).toFixed(0) }} – {{ thresholds.warning_jitter_ms }} ms</span></div>
        <div class="rng"><span>Poor</span><span>&gt; {{ thresholds.warning_jitter_ms }} ms</span></div>
      </div>
      <div class="vr-threshold-card">
        <b>Packet Loss</b>
        <div class="rng"><span>Good</span><span>&lt; {{ thresholds.warning_loss_pct }}%</span></div>
        <div class="rng"><span>Degraded</span><span>{{ thresholds.warning_loss_pct }} – {{ thresholds.critical_loss_pct }}%</span></div>
        <div class="rng"><span>Poor</span><span>&gt; {{ thresholds.critical_loss_pct }}%</span></div>
      </div>
      <div class="vr-threshold-card">
        <b>MOS (G.107 E-model)</b>
        <div class="rng"><span>Excellent</span><span>{{ thresholds.excellent_mos }} – 5.0</span></div>
        <div class="rng"><span>Good</span><span>{{ thresholds.good_mos }} – {{ thresholds.excellent_mos }}</span></div>
        <div class="rng"><span>Fair / Poor</span><span>&lt; {{ thresholds.good_mos }}</span></div>
      </div>
      <div class="vr-threshold-card">
        <b>Duplicates</b>
        <div class="rng"><span>Good</span><span>&lt; 0.1%</span></div>
        <div class="rng"><span>Degraded</span><span>0.1 – 0.5%</span></div>
        <div class="rng"><span>Poor</span><span>&gt; 0.5%</span></div>
      </div>
      <div class="vr-threshold-card">
        <b>Out-of-Order</b>
        <div class="rng"><span>Good</span><span>&lt; {{ thresholds.out_of_sequence_pct }}%</span></div>
        <div class="rng"><span>Degraded</span><span>{{ thresholds.out_of_sequence_pct }} – {{ thresholds.out_of_sequence_pct * 4 }}%</span></div>
        <div class="rng"><span>Poor</span><span>&gt; {{ thresholds.out_of_sequence_pct * 4 }}%</span></div>
      </div>
    </div>
  </div>
</template>
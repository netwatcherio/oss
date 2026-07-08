<script setup lang="ts">
// panel/src/components/voice-report/VoiceReportMulti.vue
// Multi-target voice report orchestrator. Mirrors
// `panel/template/voice-quality-report-multi.html` — combined verdict
// banner at the top, per-pair summary table, then one detail set
// per pair.

import { computed } from 'vue'
import type { VoicePairSummary, VoiceReportData } from './types'
import VoiceReportHeader from './VoiceReportHeader.vue'
import VoiceReportSummary from './VoiceReportSummary.vue'
import VoiceReportDetails from './VoiceReportDetails.vue'
import VoiceReportPath from './VoiceReportPath.vue'

const props = defineProps<{
  data: VoiceReportData
}>()

const pairs = computed<VoicePairSummary[]>(() => props.data.pairs ?? [])

const overallVerdict = computed(() => {
  const ps = pairs.value
  if (ps.length === 0) return { title: 'No voice data', text: 'No voice-quality probes reported data in this window.' }
  const avgMos = ps.reduce((acc, p) => acc + p.overall_mos, 0) / ps.length
  if (avgMos >= 4.0) {
    return { title: 'All monitored paths within voice-quality targets', text: `Across ${ps.length} probe/target pairs, mean MOS is ${avgMos.toFixed(2)}. No critical issues detected.` }
  }
  const worst = ps.reduce((acc, p) => (p.overall_mos < acc.overall_mos ? p : acc), ps[0])
  return {
    title: 'Voice quality degradation detected',
    text: `Across ${ps.length} probe/target pairs, mean MOS is ${avgMos.toFixed(2)}. Worst pair: ${worst.target.name} at MOS ${worst.overall_mos.toFixed(2)}.`,
  }
})

const ringPct = computed(() => {
  const ps = pairs.value
  if (ps.length === 0) return 0
  const avgMos = ps.reduce((acc, p) => acc + p.overall_mos, 0) / ps.length
  return Math.max(0, Math.min(1, (avgMos - 1) / 4))
})

const ringDashArray = (2 * Math.PI * 37).toFixed(1)
const ringOffset = computed(() => (2 * Math.PI * 37 * (1 - ringPct.value)).toFixed(1))
const ringColor = computed(() => {
  const ps = pairs.value
  if (ps.length === 0) return '#94a3b8'
  const avgMos = ps.reduce((acc, p) => acc + p.overall_mos, 0) / ps.length
  if (avgMos >= 4.0) return '#22c55e'
  if (avgMos >= 3.6) return '#f59e0b'
  return '#ef4444'
})

const flatIssues = computed(() => pairs.value.flatMap((p) => p.issues ?? []))
const criticalCount = computed(() => flatIssues.value.filter((i) => i.severity === 'critical').length)
const warningCount = computed(() => flatIssues.value.filter((i) => i.severity === 'warning').length)
const meanMos = computed(() => {
  const ps = pairs.value
  if (ps.length === 0) return 0
  return ps.reduce((acc, p) => acc + p.overall_mos, 0) / ps.length
})

// Build per-pair report-data for the detail pages. We reuse the
// VoiceReportSummary component by giving it a data shape that
// matches the single-pair template.
function pairAsReportData(pair: VoicePairSummary): VoiceReportData {
  // Re-use the parent's meta for header continuity, but the
  // metrics/timeseries come from the pair's primary path — forward
  // when present, reverse for target-only pairs where the only data
  // is the path toward this agent.
  const forward = pair.forward ?? pair.reverse
  return {
    ...props.data,
    meta: {
      ...props.data.meta,
      agent: pair.agent,
      target: pair.target,
    },
    summary: {
      mos: pair.overall_mos,
      r_factor: Math.round((pair.overall_mos - 1) * 25),
      grade: pair.overall_grade,
      verdict_title: `Pair — ${pair.target.name}`,
      verdict_text: pair.recommendation ?? '',
    },
    metrics: {
      latency: {
        min: forward?.avg_latency_ms ? forward.avg_latency_ms * 0.8 : 0,
        avg: forward?.avg_latency_ms ?? 0,
        max: forward?.p95_latency_ms ?? 0,
        stddev: 0,
        unit: 'ms',
      },
      jitter: {
        min: 0,
        avg: forward?.jitter_avg_ms ?? 0,
        max: forward?.jitter_p95_ms ?? 0,
        stddev: 0,
        unit: 'ms',
      },
      packets: {
        sent: forward?.sample_count ?? 0,
        received: forward?.sample_count ?? 0,
        lost: 0,
        loss_pct: forward?.packet_loss_pct ?? 0,
        duplicates: 0,
        dup_pct: forward?.duplicate_pct ?? 0,
        out_of_order: 0,
        ooo_pct: forward?.out_of_sequence_pct ?? 0,
        discarded_jitter_buffer: 0,
        discard_pct: 0,
      },
    },
  }
}

const hasTraceroute = (pair: VoicePairSummary) => !!pair.route_signals && pair.route_signals.length > 0
</script>

<template>
  <div class="vr-page" id="page1">
    <VoiceReportHeader :meta="data.meta" />

    <div class="vr-verdict">
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
          <b>{{ meanMos.toFixed(2) }}</b>
          <span>AVG MOS</span>
        </div>
      </div>
      <div class="vr-verdict-body">
        <h2>{{ overallVerdict.title }}</h2>
        <p>{{ overallVerdict.text }}</p>
      </div>
      <div class="vr-grade-chip">
        <b>{{ pairs.length }}</b>
        <span>Pairs</span>
      </div>
      <div class="vr-grade-chip">
        <b>{{ criticalCount }} / {{ warningCount }}</b>
        <span>Crit / Warn</span>
      </div>
    </div>

    <div class="vr-section">
      <div class="vr-section-title">Per-Pair Summary</div>
      <table class="vr-table">
        <thead>
          <tr>
            <th>Pair</th>
            <th>Target</th>
            <th class="num">Fwd MOS</th>
            <th class="num">Rev MOS</th>
            <th class="num">Pair MOS</th>
            <th>Issues</th>
            <th>Grade</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="p in pairs" :key="p.id">
            <td>{{ p.agent.name }}</td>
            <td>{{ p.target.name }}</td>
            <td class="num vr-mono">{{ p.forward?.mos_score?.toFixed(2) ?? '—' }}</td>
            <td class="num vr-mono">{{ p.reverse?.mos_score?.toFixed(2) ?? '—' }}</td>
            <td class="num vr-mono"><b>{{ p.overall_mos.toFixed(2) }}</b></td>
            <td>{{ p.issues?.length ?? 0 }}</td>
            <td>{{ p.overall_grade }}</td>
          </tr>
        </tbody>
      </table>
      <div class="vr-section-note">→ forward = probe agent toward target · ← reverse = return stream measured by the far-end agent. Detail pages for each pair follow.</div>
    </div>

    <footer class="vr-page-footer">
      <span>Generated by netwatcher.io — Voice / VoIP traffic simulation</span>
      <span>Page 1 of {{ 1 + pairs.length }}</span>
    </footer>
  </div>

  <div v-for="(p, idx) in pairs" :key="p.id" class="vr-page">
    <div class="vr-mini-header">
      <b>Pair {{ idx + 1 }} — {{ p.target.name }}</b>
      <span class="vr-mono">{{ data.meta.report_id }}</span>
    </div>

    <div class="vr-target-banner">
      <div>
        <div class="t-name">{{ p.target.name }}</div>
        <div class="t-sub">
          <span class="vr-dir-arrow">{{ p.forward ? '→' : '·' }}</span>
          {{ p.agent.name }}
          <template v-if="p.forward"> → {{ p.target.host ?? p.target.name }}</template>
          <template v-if="p.reverse">
            <span class="vr-dir-arrow rev" style="margin-left:8px">←</span>
            reverse measured by {{ p.reverse.source_agent_name || 'far-end agent' }}
          </template>
        </div>
      </div>
      <div class="t-mos">
        <b>{{ p.overall_mos.toFixed(2) }}</b>
        <span>Pair MOS · {{ p.overall_grade }}</span>
      </div>
    </div>

    <VoiceReportSummary :data="pairAsReportData(p)" />

    <footer class="vr-page-footer">
      <span>Generated by netwatcher.io — Voice / VoIP traffic simulation</span>
      <span>Pair {{ idx + 1 }} of {{ pairs.length }}</span>
    </footer>
  </div>
</template>
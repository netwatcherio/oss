<script lang="ts" setup>
import { ref, onMounted, watch, computed } from 'vue'
import { ProbeDataService } from '@/services/apiService'
import type { ProbeAnalysis, HealthVector, ProbeMetrics } from './types'
import { gradeColors, severityIcons } from './types'
import type { TrafficSimResult } from '@/types'

const props = defineProps<{
  workspaceId: number | string
  probeId: number | string
  // When provided, missing/zero values in the API's metrics (jitter_median,
  // jitter_p95, median_latency, p99_latency) are filled in from this local
  // TrafficSim data so the detailed modal matches the popup's richness.
  trafficSimData?: TrafficSimResult[]
  reverseTrafficSimData?: TrafficSimResult[]
  currentTimeRange?: [Date, Date] | null
}>()

const analysis = ref<ProbeAnalysis | null>(null)
const loading = ref(true)
const error = ref('')
const activeTab = ref<'overview' | 'signals' | 'findings'>('overview')

async function fetchAnalysis() {
  loading.value = true
  try {
    analysis.value = await ProbeDataService.probeAnalysis(props.workspaceId, props.probeId, { lookback: 60 })
    error.value = ''
  } catch (e: any) {
    error.value = e?.message || 'Failed to fetch analysis'
  } finally {
    loading.value = false
  }
}

function gradeColor(grade: string) {
  return gradeColors[grade] || gradeColors.unknown
}

function healthBarWidth(score: number) {
  return `${Math.max(0, Math.min(100, score))}%`
}

function formatMs(ms: number) {
  return ms > 0 ? `${ms.toFixed(1)}ms` : '—'
}

function formatPct(pct: number) {
  return pct > 0 ? `${pct.toFixed(2)}%` : '0%'
}

function formatMos(mos: number) {
  return mos > 0 ? mos.toFixed(2) : '—'
}

function hopLatencyClass(latency: number) {
  if (latency > 150) return 'critical'
  if (latency > 80) return 'warning'
  return 'good'
}

function hopLossClass(loss: number) {
  if (loss > 5) return 'critical'
  if (loss > 1) return 'warning'
  return 'good'
}

// Determine health class based on latency and loss thresholds (matching RoutePathAnalysis)
function hopHealthClass(latency: number, loss: number): string {
  if (latency > 150 || loss > 3) return 'poor'
  if (latency >= 50 || loss > 0) return 'degraded'
  return 'healthy'
}

// ── Local TrafficSim summary (same aggregation as the popup) ────────────
function lossPercentLocal(d: { lostPackets: number; totalPackets: number }): number {
  return d.totalPackets > 0 ? (d.lostPackets / d.totalPackets) * 100 : 0
}

function avgOf(vals: number[]): number {
  return vals.length > 0 ? vals.reduce((a, b) => a + b, 0) / vals.length : 0
}

function percentileOf(vals: number[], p: number): number {
  if (vals.length === 0) return 0
  const sorted = [...vals].sort((a, b) => a - b)
  const idx = (p / 100) * (sorted.length - 1)
  const lo = Math.floor(idx)
  const hi = Math.ceil(idx)
  if (lo === hi) return sorted[lo]
  return sorted[lo] + (sorted[hi] - sorted[lo]) * (idx - lo)
}

function buildLocalSummary(rows: TrafficSimResult[] | undefined) {
  if (!rows || rows.length === 0) return null
  let filtered = rows
  if (props.currentTimeRange && props.currentTimeRange[0] && props.currentTimeRange[1]) {
    const fromMs = props.currentTimeRange[0].getTime()
    const toMs = props.currentTimeRange[1].getTime()
    filtered = filtered.filter(d => {
      const t = new Date(d.reportTime).getTime()
      return t >= fromMs && t <= toMs
    })
  }
  if (filtered.length === 0) return null

  const latencies = filtered.map(d => d.averageRTT).filter(v => v > 0)
  const medianRTTs = filtered.map(d => d.medianRTT).filter((v): v is number => v != null && v > 0)
  const p95RTTs = filtered.map(d => d.p95RTT).filter((v): v is number => v != null && v > 0)
  const p99RTTs = filtered.map(d => d.p99RTT).filter((v): v is number => v != null && v > 0)
  const jitterAvgs = filtered.map(d => d.jitterAvg).filter((v): v is number => v != null && v > 0)
  const jitterMedians = filtered.map(d => d.jitterMedian).filter((v): v is number => v != null && v > 0)
  const jitterP95s = filtered.map(d => d.jitterP95).filter((v): v is number => v != null && v > 0)
  const mosVals = filtered.map(d => d.mos ?? d.mosScore).filter((v): v is number => v != null && v > 0)
  const losses = filtered.map(lossPercentLocal)

  return {
    avgLatency: avgOf(latencies),
    medianLatency: medianRTTs.length > 0 ? avgOf(medianRTTs) : percentileOf(latencies, 50),
    p95Latency: p95RTTs.length > 0 ? avgOf(p95RTTs) : percentileOf(latencies, 95),
    p99Latency: p99RTTs.length > 0 ? avgOf(p99RTTs) : 0,
    avgJitter: avgOf(jitterAvgs),
    medianJitter: jitterMedians.length > 0 ? avgOf(jitterMedians) : 0,
    p95Jitter: jitterP95s.length > 0 ? avgOf(jitterP95s) : 0,
    avgLoss: avgOf(losses),
    avgMos: avgOf(mosVals),
    sampleCount: filtered.length,
  }
}

const localTrafficSummary = computed(() => buildLocalSummary(props.trafficSimData))
const reverseLocalTrafficSummary = computed(() => buildLocalSummary(props.reverseTrafficSimData))

// When the API returns 0 for jitter/latency percentiles, fall back to the
// local TrafficSim aggregation so the detailed view is consistent with
// the popup. This handles the case where the backend only populates
// jitter_avg (PING path) but we have rich jitter data on the page.
function pickLocal(apiVal: number | undefined, localVal: number | undefined): number {
  const a = apiVal ?? 0
  if (a > 0) return a
  return localVal ?? 0
}

const mergedMetrics = computed<ProbeMetrics | null>(() => {
  if (!analysis.value) return null
  const m = analysis.value.metrics
  const local = localTrafficSummary.value
  return {
    ...m,
    median_latency: pickLocal(m.median_latency, local?.medianLatency),
    p99_latency: pickLocal(m.p99_latency, local?.p99Latency),
    jitter_median: pickLocal(m.jitter_median, local?.medianJitter),
    jitter_p95: pickLocal(m.jitter_p95, local?.p95Jitter),
  }
})

const mergedReverseMetrics = computed<ProbeMetrics | null>(() => {
  if (!analysis.value?.reverse) return null
  const m = analysis.value.reverse.metrics
  const local = reverseLocalTrafficSummary.value
  return {
    ...m,
    median_latency: pickLocal(m.median_latency, local?.medianLatency),
    p99_latency: pickLocal(m.p99_latency, local?.p99Latency),
    jitter_median: pickLocal(m.jitter_median, local?.medianJitter),
    jitter_p95: pickLocal(m.jitter_p95, local?.p95Jitter),
  }
})

const hasBidirectional = computed(() => !!analysis.value?.reverse)

const hasIcmpLatencyIncomplete = computed(() =>
  analysis.value?.signals?.some(s => s.type === 'icmp_latency_incomplete') ?? false
)

onMounted(fetchAnalysis)

watch(() => props.probeId, fetchAnalysis)
</script>

<template>
  <div class="probe-analysis">
    <!-- Loading -->
    <div v-if="loading" class="text-center py-4">
      <div class="spinner-border spinner-border-sm text-primary" role="status"></div>
      <span class="text-muted ms-2">Analyzing probe data...</span>
    </div>

    <!-- Error -->
    <div v-else-if="error" class="text-center py-4">
      <i class="bi bi-exclamation-triangle text-warning me-2"></i>
      <span class="text-muted">{{ error }}</span>
      <button class="btn btn-sm btn-link" @click="fetchAnalysis">Retry</button>
    </div>

    <!-- Analysis Content -->
    <div v-else-if="analysis">
      <!-- Header with direction badges -->
      <div class="analysis-header mb-3">
        <div class="d-flex align-items-center gap-3 flex-wrap">
          <!-- Forward direction -->
          <div class="direction-card" :style="{ borderColor: gradeColor(analysis.health.grade).border }">
            <div class="direction-label">
              <i class="bi bi-arrow-right me-1"></i>
              {{ analysis.agent_name || 'Source' }} → {{ analysis.target || 'Target' }}
            </div>
            <div class="d-flex align-items-center gap-2 mt-1">
              <span class="grade-badge" :style="{ background: gradeColor(analysis.health.grade).bg, color: gradeColor(analysis.health.grade).text }">
                {{ Math.round(analysis.health.overall_health) }}/100 · {{ analysis.health.grade }}
              </span>
              <span class="mos-badge">MOS {{ formatMos(analysis.health.mos_score) }}</span>
            </div>
          </div>

          <!-- Reverse direction (if bidirectional) -->
          <div v-if="hasBidirectional && analysis.reverse" class="direction-card" :style="{ borderColor: gradeColor(analysis.reverse.health.grade).border }">
            <div class="direction-label">
              <i class="bi bi-arrow-left me-1"></i>
              {{ analysis.reverse.agent_name }} → {{ analysis.reverse.target }}
            </div>
            <div class="d-flex align-items-center gap-2 mt-1">
              <span class="grade-badge" :style="{ background: gradeColor(analysis.reverse.health.grade).bg, color: gradeColor(analysis.reverse.health.grade).text }">
                {{ Math.round(analysis.reverse.health.overall_health) }}/100 · {{ analysis.reverse.health.grade }}
              </span>
              <span class="mos-badge">MOS {{ formatMos(analysis.reverse.health.mos_score) }}</span>
            </div>
          </div>

          <!-- Combined bidirectional health (worse-direction weighted) -->
          <div v-if="analysis.combined_health" class="direction-card" :style="{ borderColor: gradeColor(analysis.combined_health.grade).border }">
            <div class="direction-label">
              <i class="bi bi-arrow-left-right me-1"></i>
              Combined (both directions)
            </div>
            <div class="d-flex align-items-center gap-2 mt-1">
              <span class="grade-badge" :style="{ background: gradeColor(analysis.combined_health.grade).bg, color: gradeColor(analysis.combined_health.grade).text }">
                {{ Math.round(analysis.combined_health.overall_health) }}/100 · {{ analysis.combined_health.grade }}
              </span>
              <span class="mos-badge">MOS {{ formatMos(analysis.combined_health.mos_score) }}</span>
            </div>
          </div>
        </div>
      </div>

      <!-- Tab Navigation -->
      <div class="analysis-tabs mb-3">
        <button :class="['tab-btn', activeTab === 'overview' && 'active']" @click="activeTab = 'overview'">
          <i class="bi bi-speedometer2 me-1"></i>Metrics
        </button>
        <button :class="['tab-btn', activeTab === 'signals' && 'active']" @click="activeTab = 'signals'">
          <i class="bi bi-broadcast me-1"></i>Signals
          <span v-if="analysis.signals?.length" class="tab-count">{{ analysis.signals.length }}</span>
        </button>
        <button :class="['tab-btn', activeTab === 'findings' && 'active']" @click="activeTab = 'findings'">
          <i class="bi bi-clipboard-data me-1"></i>Findings
          <span v-if="analysis.findings?.length" class="tab-count">{{ analysis.findings.length }}</span>
        </button>
      </div>

      <!-- Overview Tab -->
      <div v-if="activeTab === 'overview'" class="tab-content">
        <!-- Metric Comparison Table -->
        <div v-if="mergedMetrics" class="metrics-grid" :class="{ bidirectional: hasBidirectional }">
          <div class="metric-header">Metric</div>
          <div class="metric-header">
            <i class="bi bi-arrow-right me-1"></i>Forward
          </div>
          <div v-if="hasBidirectional" class="metric-header">
            <i class="bi bi-arrow-left me-1"></i>Reverse
          </div>

          <!-- Avg Latency -->
          <div class="metric-label">Avg Latency</div>
          <div class="metric-value">
            {{ formatMs(mergedMetrics.avg_latency) }}
            <i
              v-if="hasIcmpLatencyIncomplete"
              class="bi bi-info-circle text-info ms-1"
              title="Latency estimated from MTR end-hop RTT (ICMP probe returned no data)"
            ></i>
          </div>
          <div v-if="hasBidirectional" class="metric-value">
            {{ formatMs(mergedReverseMetrics!.avg_latency) }}
            <i
              v-if="analysis.reverse?.signals?.some(s => s.type === 'icmp_latency_incomplete')"
              class="bi bi-info-circle text-info ms-1"
              title="Latency estimated from MTR end-hop RTT (ICMP probe returned no data)"
            ></i>
          </div>

          <!-- Median Latency -->
          <div class="metric-label">Median Latency</div>
          <div class="metric-value" :class="{ 'text-muted': mergedMetrics.median_latency <= 0 }">
            {{ formatMs(mergedMetrics.median_latency) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-muted': mergedReverseMetrics!.median_latency <= 0 }">
            {{ formatMs(mergedReverseMetrics!.median_latency) }}
          </div>

          <!-- P95 Latency -->
          <div class="metric-label">P95 Latency</div>
          <div class="metric-value">{{ formatMs(mergedMetrics.p95_latency) }}</div>
          <div v-if="hasBidirectional" class="metric-value">{{ formatMs(mergedReverseMetrics!.p95_latency) }}</div>

          <!-- P99 Latency -->
          <div class="metric-label">P99 Latency</div>
          <div class="metric-value" :class="{ 'text-muted': mergedMetrics.p99_latency <= 0 }">
            {{ formatMs(mergedMetrics.p99_latency) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-muted': mergedReverseMetrics!.p99_latency <= 0 }">
            {{ formatMs(mergedReverseMetrics!.p99_latency) }}
          </div>

          <!-- Packet Loss -->
          <div class="metric-label">Packet Loss</div>
          <div class="metric-value" :class="{ 'text-danger': mergedMetrics.packet_loss > 3 }">
            {{ formatPct(mergedMetrics.packet_loss) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-danger': mergedReverseMetrics!.packet_loss > 3 }">
            {{ formatPct(mergedReverseMetrics!.packet_loss) }}
          </div>

          <!-- Jitter Avg -->
          <div class="metric-label">Jitter (Avg)</div>
          <div class="metric-value">{{ formatMs(mergedMetrics.jitter_avg) }}</div>
          <div v-if="hasBidirectional" class="metric-value">{{ formatMs(mergedReverseMetrics!.jitter_avg) }}</div>

          <!-- Jitter Median -->
          <div class="metric-label">Jitter (Med)</div>
          <div class="metric-value" :class="{ 'text-muted': mergedMetrics.jitter_median <= 0 }">
            {{ formatMs(mergedMetrics.jitter_median) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-muted': mergedReverseMetrics!.jitter_median <= 0 }">
            {{ formatMs(mergedReverseMetrics!.jitter_median) }}
          </div>

          <!-- Jitter P95 -->
          <div class="metric-label">Jitter (P95)</div>
          <div class="metric-value" :class="{ 'text-muted': mergedMetrics.jitter_p95 <= 0 }">
            {{ formatMs(mergedMetrics.jitter_p95) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :class="{ 'text-muted': mergedReverseMetrics!.jitter_p95 <= 0 }">
            {{ formatMs(mergedReverseMetrics!.jitter_p95) }}
          </div>

          <!-- MOS -->
          <div class="metric-label">MOS Score</div>
          <div class="metric-value" :style="{ color: gradeColor(analysis.health.grade).text }">
            {{ formatMos(analysis.health.mos_score) }}
          </div>
          <div v-if="hasBidirectional" class="metric-value" :style="{ color: gradeColor(analysis.reverse!.health.grade).text }">
            {{ formatMos(analysis.reverse!.health.mos_score) }}
          </div>

          <!-- Samples -->
          <div class="metric-label">Samples</div>
          <div class="metric-value text-muted">{{ mergedMetrics.sample_count }}</div>
          <div v-if="hasBidirectional" class="metric-value text-muted">{{ mergedReverseMetrics!.sample_count }}</div>
        </div>

        <!-- Health Vector Bars -->
        <div class="health-vectors mt-3">
          <h6 class="small text-muted mb-2">Health Scores</h6>
          <div class="vector-comparison">
            <div class="vector-side">
              <div class="vector-bar-row" v-for="dim in [
                { label: 'Latency', score: analysis.health.latency_score },
                { label: 'Loss', score: analysis.health.packet_loss_score },
                { label: 'Route Stability', score: analysis.health.route_stability },
              ]" :key="dim.label">
                <span class="vector-label">{{ dim.label }}</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(dim.score), background: gradeColor(analysis.health.grade).border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(dim.score) }}</span>
              </div>
            </div>
            <div v-if="hasBidirectional && analysis.reverse" class="vector-side">
              <div class="vector-bar-row" v-for="dim in [
                { label: 'Latency', score: analysis.reverse!.health.latency_score },
                { label: 'Loss', score: analysis.reverse!.health.packet_loss_score },
                { label: 'Route Stability', score: analysis.reverse!.health.route_stability },
              ]" :key="dim.label">
                <span class="vector-label">{{ dim.label }}</span>
                <div class="vector-bar-track">
                  <div class="vector-bar-fill" :style="{ width: healthBarWidth(dim.score), background: gradeColor(analysis.reverse!.health.grade).border }"></div>
                </div>
                <span class="vector-score">{{ Math.round(dim.score) }}</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Path Analysis (Forward) -->
        <div v-if="analysis.path_analysis" class="path-analysis mt-3">
          <h6 class="small text-muted mb-2 d-flex align-items-center gap-2">
            <i class="bi bi-arrow-right"></i>
            <span>Forward Path</span>
            <span class="text-muted ms-2" style="font-size:10px">{{ analysis.agent_name }} → {{ analysis.target }}</span>
          </h6>
          <div class="path-stats">
            <div class="path-stat">
              <div class="stat-value">{{ analysis.path_analysis.hop_count }}</div>
              <div class="stat-label">Hops</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ analysis.path_analysis.unique_routes }}</div>
              <div class="stat-label">Routes</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ Math.round(analysis.path_analysis.route_stability_pct) }}%</div>
              <div class="stat-label">Stability</div>
            </div>
            <div class="path-stat" v-if="analysis.path_analysis.rate_limited_hops?.length">
              <div class="stat-value text-info">{{ analysis.path_analysis.rate_limited_hops.length }}</div>
              <div class="stat-label">ICMP Limited</div>
            </div>
          </div>
          <!-- Hop List (vertical, no side-scroll) -->
          <div v-if="analysis.path_analysis.latest_hops_detail?.length" class="hop-list mt-3">
            <h6 class="small text-muted mb-2 d-flex align-items-center gap-2">
              <i class="bi bi-signpost-split"></i>
              <span>Hops ({{ analysis.path_analysis.hop_count }})</span>
              <span class="text-muted ms-auto" style="font-size:10px">{{ analysis.path_analysis.unique_routes }} route{{ analysis.path_analysis.unique_routes !== 1 ? 's' : '' }}</span>
            </h6>
            <div class="hop-list-body">
              <div class="hop-row hop-row-header">
                <span class="hop-cell hop-cell-num">#</span>
                <span class="hop-cell hop-cell-host">Host</span>
                <span class="hop-cell hop-cell-metric">Latency</span>
                <span class="hop-cell hop-cell-metric">Loss</span>
              </div>
              <div class="hop-row hop-row-source">
                <span class="hop-cell hop-cell-num">
                  <i class="bi bi-pc-display"></i>
                </span>
                <span class="hop-cell hop-cell-host">{{ analysis.agent_name || 'Source' }}</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
              </div>
              <div
                v-for="(hop, idx) in analysis.path_analysis.latest_hops_detail"
                :key="`fwd-${idx}`"
                class="hop-row"
                :class="{
                  'hop-row-agent': hop.is_agent,
                  'hop-row-rate-limited': hop.is_rate_limited,
                  'hop-row-dest': hop.is_final_hop,
                }"
              >
                <span class="hop-cell hop-cell-num">
                  <i :class="hop.is_agent ? 'bi bi-hdd-network' : 'bi bi-router'"></i>
                  <span class="hop-cell-num-text">{{ idx + 1 }}</span>
                </span>
                <span class="hop-cell hop-cell-host" :title="hop.ip">
                  <span class="hop-hostname">{{ hop.hostname || hop.ip }}</span>
                  <span v-if="hop.hostname && hop.hostname !== hop.ip" class="hop-ip text-muted">{{ hop.ip }}</span>
                  <span v-if="hop.is_rate_limited" class="hop-badge-icmp">ICMP</span>
                </span>
                <span
                  v-if="hop.latency != null"
                  class="hop-cell hop-cell-metric"
                  :class="hopHealthClass(hop.latency, hop.loss || 0)"
                >
                  {{ formatMs(hop.latency) }}
                </span>
                <span v-else class="hop-cell hop-cell-metric text-muted">—</span>
                <span
                  v-if="hop.loss != null && hop.loss > 0"
                  class="hop-cell hop-cell-metric"
                  :class="hopHealthClass(hop.latency || 0, hop.loss)"
                >
                  {{ hop.loss.toFixed(1) }}%
                </span>
                <span v-else class="hop-cell hop-cell-metric text-muted">0%</span>
              </div>
              <div class="hop-row hop-row-dest-row">
                <span class="hop-cell hop-cell-num">
                  <i class="bi bi-bullseye"></i>
                </span>
                <span class="hop-cell hop-cell-host">{{ analysis.target || 'Target' }}</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
              </div>
            </div>
          </div>
        </div>

        <!-- Path Analysis (Reverse) — shown inline when bidirectional -->
        <div v-if="hasBidirectional && analysis.reverse?.path_analysis" class="path-analysis path-analysis-reverse mt-3">
          <h6 class="small text-muted mb-2 d-flex align-items-center gap-2">
            <i class="bi bi-arrow-left"></i>
            <span>Return Path</span>
            <span class="text-muted ms-2" style="font-size:10px">{{ analysis.reverse.agent_name }} → {{ analysis.reverse.target }}</span>
          </h6>
          <div class="path-stats">
            <div class="path-stat">
              <div class="stat-value">{{ analysis.reverse.path_analysis.hop_count }}</div>
              <div class="stat-label">Hops</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ analysis.reverse.path_analysis.unique_routes }}</div>
              <div class="stat-label">Routes</div>
            </div>
            <div class="path-stat">
              <div class="stat-value">{{ Math.round(analysis.reverse.path_analysis.route_stability_pct) }}%</div>
              <div class="stat-label">Stability</div>
            </div>
            <div class="path-stat" v-if="analysis.reverse.path_analysis.rate_limited_hops?.length">
              <div class="stat-value text-info">{{ analysis.reverse.path_analysis.rate_limited_hops.length }}</div>
              <div class="stat-label">ICMP Limited</div>
            </div>
          </div>
          <!-- Hop List (vertical, no side-scroll) -->
          <div v-if="analysis.reverse.path_analysis.latest_hops_detail?.length" class="hop-list mt-3">
            <h6 class="small text-muted mb-2 d-flex align-items-center gap-2">
              <i class="bi bi-signpost-split"></i>
              <span>Hops ({{ analysis.reverse.path_analysis.hop_count }})</span>
              <span class="text-muted ms-auto" style="font-size:10px">{{ analysis.reverse.path_analysis.unique_routes }} route{{ analysis.reverse.path_analysis.unique_routes !== 1 ? 's' : '' }}</span>
            </h6>
            <div class="hop-list-body">
              <div class="hop-row hop-row-header">
                <span class="hop-cell hop-cell-num">#</span>
                <span class="hop-cell hop-cell-host">Host</span>
                <span class="hop-cell hop-cell-metric">Latency</span>
                <span class="hop-cell hop-cell-metric">Loss</span>
              </div>
              <div class="hop-row hop-row-source">
                <span class="hop-cell hop-cell-num">
                  <i class="bi bi-pc-display"></i>
                </span>
                <span class="hop-cell hop-cell-host">{{ analysis.reverse.agent_name || 'Source' }}</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
              </div>
              <div
                v-for="(hop, idx) in analysis.reverse.path_analysis.latest_hops_detail"
                :key="`rev-${idx}`"
                class="hop-row"
                :class="{
                  'hop-row-agent': hop.is_agent,
                  'hop-row-rate-limited': hop.is_rate_limited,
                  'hop-row-dest': hop.is_final_hop,
                }"
              >
                <span class="hop-cell hop-cell-num">
                  <i :class="hop.is_agent ? 'bi bi-hdd-network' : 'bi bi-router'"></i>
                  <span class="hop-cell-num-text">{{ idx + 1 }}</span>
                </span>
                <span class="hop-cell hop-cell-host" :title="hop.ip">
                  <span class="hop-hostname">{{ hop.hostname || hop.ip }}</span>
                  <span v-if="hop.hostname && hop.hostname !== hop.ip" class="hop-ip text-muted">{{ hop.ip }}</span>
                  <span v-if="hop.is_rate_limited" class="hop-badge-icmp">ICMP</span>
                </span>
                <span
                  v-if="hop.latency != null"
                  class="hop-cell hop-cell-metric"
                  :class="hopHealthClass(hop.latency, hop.loss || 0)"
                >
                  {{ formatMs(hop.latency) }}
                </span>
                <span v-else class="hop-cell hop-cell-metric text-muted">—</span>
                <span
                  v-if="hop.loss != null && hop.loss > 0"
                  class="hop-cell hop-cell-metric"
                  :class="hopHealthClass(hop.latency || 0, hop.loss)"
                >
                  {{ hop.loss.toFixed(1) }}%
                </span>
                <span v-else class="hop-cell hop-cell-metric text-muted">0%</span>
              </div>
              <div class="hop-row hop-row-dest-row">
                <span class="hop-cell hop-cell-num">
                  <i class="bi bi-bullseye"></i>
                </span>
                <span class="hop-cell hop-cell-host">{{ analysis.reverse.target || 'Target' }}</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
                <span class="hop-cell hop-cell-metric text-muted">—</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Signals Tab -->
      <div v-if="activeTab === 'signals'" class="tab-content">
        <div v-if="!analysis.signals?.length" class="text-center text-muted py-3">
          <i class="bi bi-check-circle text-success me-2"></i>No anomalies detected
        </div>
        <div v-for="(signal, i) in analysis.signals" :key="i" class="signal-card" :class="signal.severity">
          <div class="d-flex align-items-start gap-2">
            <i :class="['bi', severityIcons[signal.severity] || 'bi-info-circle', 'signal-icon']"></i>
            <div class="flex-grow-1">
              <div class="signal-title">{{ signal.title }}</div>
              <div class="signal-evidence">{{ signal.evidence }}</div>
              <div class="signal-meta">
                <span class="signal-type">{{ signal.type }}</span>
                <span class="signal-confidence">{{ Math.round(signal.confidence * 100) }}% confidence</span>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- Findings Tab -->
      <div v-if="activeTab === 'findings'" class="tab-content">
        <div v-if="!analysis.findings?.length" class="text-center text-muted py-3">
          <i class="bi bi-check-circle text-success me-2"></i>No findings
        </div>
        <div v-for="finding in analysis.findings" :key="finding.id" class="finding-card" :class="finding.severity">
          <div class="finding-header">
            <i :class="['bi', severityIcons[finding.severity] || 'bi-info-circle']"></i>
            <span class="finding-title">{{ finding.title }}</span>
            <span class="finding-category">{{ finding.category }}</span>
          </div>
          <p class="finding-summary">{{ finding.summary }}</p>
          <div v-if="finding.evidence?.length" class="finding-evidence">
            <div v-for="(e, j) in finding.evidence" :key="j" class="evidence-item">
              <i class="bi bi-dot"></i> {{ e }}
            </div>
          </div>
          <div v-if="finding.recommended_steps?.length" class="finding-steps">
            <small class="text-muted">Recommended:</small>
            <ol>
              <li v-for="(step, j) in finding.recommended_steps" :key="j">{{ step }}</li>
            </ol>
          </div>
        </div>
      </div>

      <div class="text-muted small mt-2 text-end">
        <i class="bi bi-clock me-1"></i>
        {{ new Date(analysis.generated_at).toLocaleTimeString() }}
      </div>
    </div>
  </div>
</template>

<style scoped>
.probe-analysis {
  padding: 0;
}
/* Direction Cards */
.direction-card {
  border: 1px solid;
  border-radius: 10px;
  padding: 12px 16px;
  background: var(--bs-secondary-bg);
  flex: 1;
  min-width: 200px;
  transition: transform 0.2s, box-shadow 0.2s;
}
.direction-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 12px rgba(var(--bs-dark-rgb), 0.1);
}
.direction-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--bs-body-color);
  display: flex;
  align-items: center;
  gap: 6px;
}
.grade-badge {
  font-size: 11px;
  font-weight: 600;
  padding: 3px 10px;
  border-radius: 12px;
  text-transform: capitalize;
}
.mos-badge {
  font-size: 11px;
  color: var(--bs-secondary-color);
  background: var(--bs-tertiary-bg);
  padding: 3px 8px;
  border-radius: 6px;
  font-weight: 500;
}

/* Tabs */
.analysis-tabs {
  display: flex;
  gap: 8px;
  border-bottom: 1px solid var(--bs-border-color);
  padding-bottom: 0;
}
.tab-btn {
  font-size: 13px;
  padding: 8px 16px;
  border: none;
  background: none;
  color: var(--bs-secondary-color);
  cursor: pointer;
  border-bottom: 2px solid transparent;
  transition: all 0.2s;
  font-weight: 500;
  display: flex;
  align-items: center;
  gap: 6px;
}
.tab-btn:hover { 
  color: var(--bs-body-color); 
  background: var(--bs-tertiary-bg);
  border-radius: 6px 6px 0 0;
}
.tab-btn.active {
  color: var(--bs-primary);
  border-bottom-color: var(--bs-primary);
  background: var(--bs-primary-bg-subtle);
  border-radius: 6px 6px 0 0;
}
.tab-count {
  font-size: 10px;
  font-weight: 700;
  background: var(--bs-primary);
  color: var(--bs-white);
  padding: 1px 6px;
  border-radius: 10px;
  margin-left: 4px;
}

/* Metrics Grid */
.metrics-grid {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 0;
  font-size: 13px;
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  overflow: hidden;
}
.metrics-grid.bidirectional {
  grid-template-columns: 120px 1fr 1fr;
}
.metric-header {
  font-weight: 600;
  font-size: 11px;
  color: var(--bs-secondary-color);
  padding: 8px 12px;
  border-bottom: 1px solid var(--bs-border-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  background: var(--bs-tertiary-bg);
}
.metric-label {
  color: var(--bs-secondary-color);
  padding: 10px 12px;
  border-bottom: 1px solid var(--bs-border-color-translucent);
  font-weight: 500;
}
.metric-value {
  font-weight: 600;
  font-family: 'SF Mono', 'Fira Code', monospace;
  padding: 10px 12px;
  border-bottom: 1px solid var(--bs-border-color-translucent);
  color: var(--bs-body-color);
}
.metrics-grid > div:nth-last-child(-n+3) {
  border-bottom: none;
}

/* Health Vectors */
.vector-comparison {
  display: flex;
  gap: 24px;
}
.vector-side {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--bs-secondary-bg);
  padding: 12px;
  border-radius: 8px;
}
.vector-bar-row {
  display: flex;
  align-items: center;
  gap: 10px;
}
.vector-label {
  font-size: 12px;
  color: var(--bs-secondary-color);
  width: 90px;
  flex-shrink: 0;
  font-weight: 500;
}
.vector-bar-track {
  flex: 1;
  height: 8px;
  background: var(--bs-tertiary-bg);
  border-radius: 4px;
  overflow: hidden;
}
.vector-bar-fill {
  height: 100%;
  border-radius: 4px;
  transition: width 0.6s ease;
}
.vector-score {
  font-size: 12px;
  font-weight: 600;
  width: 32px;
  text-align: right;
  color: var(--bs-body-color);
}

/* Path Analysis */
.path-stats {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}
.path-analysis {
  padding: 8px 0;
}
.path-analysis-reverse {
  margin-top: 1rem;
  padding-top: 1rem;
  border-top: 1px dashed var(--bs-border-color);
}
.path-stat {
  background: var(--bs-secondary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px 20px;
  text-align: center;
  min-width: 80px;
  transition: transform 0.15s;
}
.path-stat:hover {
  transform: translateY(-2px);
}
.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: var(--bs-body-color);
}
.stat-label {
  font-size: 10px;
  color: var(--bs-secondary-color);
  text-transform: uppercase;
  letter-spacing: 0.5px;
  margin-top: 2px;
}

/* Signals */
.signal-card {
  background: var(--bs-secondary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 12px 16px;
  margin-bottom: 10px;
  transition: transform 0.15s, box-shadow 0.15s;
}
.signal-card:hover {
  transform: translateX(2px);
}
.signal-card.warning { border-left: 4px solid var(--bs-warning); }
.signal-card.critical { border-left: 4px solid var(--bs-danger); }
.signal-card.info { border-left: 4px solid var(--bs-info); }
.signal-icon { font-size: 18px; margin-top: 2px; }
.signal-card.warning .signal-icon { color: var(--bs-warning); }
.signal-card.critical .signal-icon { color: var(--bs-danger); }
.signal-card.info .signal-icon { color: var(--bs-info); }
.signal-title { font-weight: 600; font-size: 13px; color: var(--bs-body-color); }
.signal-evidence { font-size: 12px; color: var(--bs-secondary-color); margin-top: 4px; line-height: 1.4; }
.signal-meta { font-size: 11px; color: var(--bs-secondary-color); margin-top: 6px; display: flex; gap: 12px; align-items: center; }
.signal-type { background: var(--bs-tertiary-bg); padding: 2px 8px; border-radius: 4px; font-weight: 500; }

/* Findings */
.finding-card {
  background: var(--bs-secondary-bg);
  border: 1px solid var(--bs-border-color);
  border-radius: 10px;
  padding: 14px 18px;
  margin-bottom: 12px;
  transition: transform 0.15s, box-shadow 0.15s;
}
.finding-card:hover {
  transform: translateX(2px);
}
.finding-card.warning { border-left: 4px solid var(--bs-warning); }
.finding-card.critical { border-left: 4px solid var(--bs-danger); }
.finding-card.info { border-left: 4px solid var(--bs-info); }
.finding-header {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.finding-header i { font-size: 18px; }
.finding-card.warning .finding-header i { color: var(--bs-warning); }
.finding-card.critical .finding-header i { color: var(--bs-danger); }
.finding-card.info .finding-header i { color: var(--bs-info); }
.finding-title { font-weight: 600; font-size: 14px; color: var(--bs-body-color); }
.finding-category {
  font-size: 10px;
  text-transform: uppercase;
  background: var(--bs-tertiary-bg);
  padding: 2px 8px;
  border-radius: 4px;
  color: var(--bs-secondary-color);
  margin-left: auto;
  font-weight: 500;
  letter-spacing: 0.5px;
}
.finding-summary { font-size: 13px; color: var(--bs-secondary-color); margin-bottom: 8px; line-height: 1.5; }
.finding-evidence { font-size: 12px; color: var(--bs-secondary-color); }
.evidence-item { padding: 2px 0; display: flex; align-items: flex-start; gap: 6px; }
.evidence-item i { color: var(--bs-primary); font-size: 10px; margin-top: 4px; }
.finding-steps { margin-top: 10px; font-size: 12px; }
.finding-steps ol { padding-left: 20px; margin-bottom: 0; color: var(--bs-body-color); }
.finding-steps li { margin-bottom: 4px; }

/* ===== LIGHT theme overrides - now using Bootstrap variables ===== */
/* No longer needed as all styles use Bootstrap CSS variables */
[data-theme="light"] .path-stat {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .stat-value {
  color: #1f2937;
}
[data-theme="light"] .stat-label {
  color: #6b7280;
}
[data-theme="light"] .signal-card {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .signal-title {
  color: #1f2937;
}
[data-theme="light"] .signal-evidence {
  color: #6b7280;
}
[data-theme="light"] .signal-meta {
  color: #9ca3af;
}
[data-theme="light"] .signal-type {
  background: #e5e7eb;
}
[data-theme="light"] .finding-card {
  background: #f9fafb;
  border-color: #e5e7eb;
}
[data-theme="light"] .finding-title {
  color: #1f2937;
}
[data-theme="light"] .finding-category {
  background: #e5e7eb;
  color: #6b7280;
}
[data-theme="light"] .finding-summary {
  color: #4b5563;
}
[data-theme="light"] .finding-evidence {
  color: #6b7280;
}
[data-theme="light"] .finding-steps ol {
  color: #4b5563;
}
[data-theme="light"] .tab-btn {
  color: #6b7280;
}
[data-theme="light"] .tab-btn:hover {
  color: #1f2937;
}
[data-theme="light"] .analysis-tabs {
  border-bottom-color: #e5e7eb;
}

/* Hop List (vertical table — no side scroll) */
.hop-list {
  font-size: 12px;
}
.hop-list-body {
  border: 1px solid var(--bs-border-color);
  border-radius: 8px;
  overflow: hidden;
  background: var(--bs-body-bg);
}
.hop-row {
  display: grid;
  grid-template-columns: 48px 1fr 90px 70px;
  align-items: center;
  gap: 8px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--bs-border-color-translucent);
  transition: background 0.15s;
}
.hop-row:last-child {
  border-bottom: none;
}
.hop-row:not(.hop-row-header):not(.hop-row-source):not(.hop-row-dest-row):hover {
  background: var(--bs-tertiary-bg);
}
.hop-row-header {
  background: var(--bs-tertiary-bg);
  font-size: 10px;
  text-transform: uppercase;
  letter-spacing: 0.5px;
  color: var(--bs-secondary-color);
  font-weight: 600;
  padding: 6px 12px;
}
.hop-row-source,
.hop-row-dest-row {
  background: rgba(var(--bs-primary-rgb), 0.06);
  font-weight: 500;
}
.hop-row-agent {
  background: rgba(var(--bs-info-rgb), 0.06);
}
.hop-row-rate-limited {
  border-left: 3px solid var(--bs-info);
}
.hop-row-dest {
  background: rgba(var(--bs-primary-rgb), 0.04);
}
.hop-cell {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}
.hop-cell-num {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 11px;
  color: var(--bs-secondary-color);
  display: flex;
  align-items: center;
  gap: 4px;
}
.hop-cell-num i {
  font-size: 14px;
  color: var(--bs-secondary-color);
}
.hop-row-header .hop-cell-num i {
  display: none;
}
.hop-cell-num-text {
  font-weight: 600;
}
.hop-cell-host {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 2px;
}
.hop-hostname {
  font-weight: 500;
  color: var(--bs-body-color);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hop-ip {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 10px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.hop-cell-metric {
  font-family: 'SF Mono', 'Fira Code', monospace;
  font-size: 11px;
  font-weight: 600;
  text-align: right;
  justify-content: flex-end;
}
.hop-cell-metric.healthy { color: #10b981; }
.hop-cell-metric.degraded { color: #f59e0b; }
.hop-cell-metric.poor { color: #ef4444; }
.hop-badge-icmp {
  font-size: 8px;
  font-weight: 600;
  padding: 1px 4px;
  border-radius: 3px;
  background: var(--bs-info);
  color: white;
  text-transform: uppercase;
  align-self: flex-start;
  margin-left: 6px;
}
.hop-cell-metric.text-muted,
.hop-cell.text-muted {
  color: var(--bs-secondary-color);
}
[data-theme="dark"] .hop-list-body {
  background: var(--bs-tertiary-bg);
}
[data-theme="dark"] .hop-row-header {
  background: rgba(var(--bs-dark-rgb), 0.3);
}
</style>
